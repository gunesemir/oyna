package main

import (
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type downloads struct {
	client  *http.Client
	retries int
	cache   string
}

func newDownloads(home string) (*downloads, error) {
	retries := 3
	if s, ok := os.LookupEnv("UMU_HTTP_RETRIES"); ok {
		n, e := strconv.Atoi(s)
		if e != nil || n < 0 {
			return nil, fmt.Errorf("invalid UMU_HTTP_RETRIES %q", s)
		}
		retries = n
	}
	timeout := 5.0
	if s, ok := os.LookupEnv("UMU_HTTP_TIMEOUT"); ok {
		n, e := strconv.ParseFloat(s, 64)
		if e != nil || n < 0 || math.IsNaN(n) || math.IsInf(n, 0) || n > 86400 {
			return nil, fmt.Errorf("invalid UMU_HTTP_TIMEOUT %q", s)
		}
		timeout = n
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	duration := time.Duration(timeout * float64(time.Second))
	transport.TLSHandshakeTimeout = duration
	transport.ResponseHeaderTimeout = duration
	// Bound each socket read while allowing large archives to take longer overall.
	transport.DialContext = (&timeoutDialer{duration: duration}).dial
	cache := os.Getenv("XDG_CACHE_HOME")
	if cache == "" {
		cache = filepath.Join(home, ".cache")
	}
	return &downloads{client: &http.Client{Transport: transport, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("too many redirects")
		}
		return secureURL(req.URL.String())
	}}, retries: retries, cache: filepath.Join(cache, "umu")}, nil
}

type timeoutDialer struct{ duration time.Duration }
type timeoutConn struct {
	net.Conn
	duration time.Duration
}

func (d *timeoutDialer) dial(ctx context.Context, network, address string) (net.Conn, error) {
	c, err := (&net.Dialer{Timeout: d.duration, KeepAlive: 30 * time.Second}).DialContext(ctx, network, address)
	if err != nil {
		return nil, err
	}
	return &timeoutConn{c, d.duration}, nil
}
func (c *timeoutConn) Read(p []byte) (int, error) {
	if c.duration > 0 {
		_ = c.Conn.SetReadDeadline(time.Now().Add(c.duration))
	}
	return c.Conn.Read(p)
}

func secureURL(address string) error {
	u, err := url.Parse(address)
	if err != nil {
		return err
	}
	if u.Scheme == "https" {
		return nil
	}
	// Loopback HTTP is used by local fixtures and local artifact mirrors.
	if ip := net.ParseIP(u.Hostname()); u.Scheme == "http" && ip != nil && ip.IsLoopback() {
		return nil
	}
	return fmt.Errorf("download URL requires HTTPS: %s", address)
}
func (d *downloads) request(url string, offset int64) (*http.Response, error) {
	if err := secureURL(url); err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "oyna/0.2.0")
	req.Header.Set("Accept-Encoding", "identity")
	if offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}
	return d.client.Do(req)
}
func (d *downloads) bytes(url string) ([]byte, error) {
	var last error
	for trial := 0; trial <= d.retries; trial++ {
		resp, err := d.request(url, 0)
		if err == nil {
			if resp.StatusCode != http.StatusOK {
				err = fmt.Errorf("%s: HTTP %s", url, resp.Status)
			} else {
				var b []byte
				b, err = io.ReadAll(io.LimitReader(resp.Body, (8<<20)+1))
				if err == nil && len(b) > 8<<20 {
					err = fmt.Errorf("metadata too large: %s", url)
				}
				if err == nil {
					resp.Body.Close()
					return b, nil
				}
			}
			resp.Body.Close()
			if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != 429 {
				return nil, err
			}
		}
		last = err
	}
	return nil, last
}
func cacheLock(path string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return func() { f.Close() }, nil
}
func hashFile(path string) (string, error) { return fileDigest(path, 32) }
func fileDigest(path string, size int) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	var h hash.Hash = sha256.New()
	if size == 64 {
		h = sha512.New()
	}
	_, err = io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil)), err
}

func (d *downloads) archive(url, digest string) (string, error) {
	raw, err := hex.DecodeString(digest)
	if err != nil || (len(raw) != 32 && len(raw) != 64) {
		return "", fmt.Errorf("invalid checksum for %s", url)
	}
	path := filepath.Join(d.cache, digest+".parts")
	unlock, err := cacheLock(path + ".lock")
	if err != nil {
		return "", err
	}
	defer unlock()
	if got, err := fileDigest(path, len(raw)); err == nil && got == digest {
		return path, nil
	}
	var last error
	for trial := 0; trial <= d.retries; trial++ {
		var offset int64
		if st, err := os.Stat(path); err == nil {
			offset = st.Size()
		}
		resp, err := d.request(url, offset)
		if err != nil {
			last = err
			continue
		}
		switch resp.StatusCode {
		case http.StatusOK:
			offset = 0
		case http.StatusPartialContent:
			if !strings.HasPrefix(resp.Header.Get("Content-Range"), fmt.Sprintf("bytes %d-", offset)) {
				resp.Body.Close()
				return "", fmt.Errorf("invalid Content-Range from %s", url)
			}
		case http.StatusRequestedRangeNotSatisfiable:
			resp.Body.Close()
			_ = os.Remove(path)
			last = fmt.Errorf("invalid cached length for %s", url)
			continue
		default:
			resp.Body.Close()
			last = fmt.Errorf("%s: HTTP %s", url, resp.Status)
			if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != 429 {
				return "", last
			}
			continue
		}
		flags := os.O_CREATE | os.O_WRONLY
		if offset == 0 {
			flags |= os.O_TRUNC
		} else {
			flags |= os.O_APPEND
		}
		f, err := os.OpenFile(path, flags, 0600)
		if err != nil {
			resp.Body.Close()
			return "", err
		}
		_, err = io.Copy(f, resp.Body)
		resp.Body.Close()
		closeErr := f.Close()
		if err != nil {
			last = err
			continue
		}
		if closeErr != nil {
			return "", closeErr
		}
		got, err := fileDigest(path, len(raw))
		if err != nil {
			return "", err
		}
		if got != digest {
			_ = os.Remove(path)
			last = fmt.Errorf("checksum mismatch for %s", url)
			continue
		}
		return path, nil
	}
	return "", last
}

func checksum(data []byte, name string) (string, error) {
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && strings.TrimPrefix(fields[len(fields)-1], "*") == name {
			digest := strings.ToLower(fields[0])
			b, err := hex.DecodeString(digest)
			if err == nil && (len(b) == 32 || len(b) == 64) {
				return digest, nil
			}
		}
	}
	return "", fmt.Errorf("checksum entry for %s missing", name)
}

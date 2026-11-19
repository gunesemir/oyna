package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/ulikunitz/xz"
)

type archiveEntry struct {
	name, body, link string
	kind             byte
}

func makeArchive(t *testing.T, entries []archiveEntry, compressedXZ bool) []byte {
	t.Helper()
	var b bytes.Buffer
	var writer io.WriteCloser
	if compressedXZ {
		var err error
		writer, err = xz.NewWriter(&b)
		if err != nil {
			t.Fatal(err)
		}
	} else {
		writer = gzip.NewWriter(&b)
	}
	tw := tar.NewWriter(writer)
	for _, e := range entries {
		kind := e.kind
		if kind == 0 {
			kind = tar.TypeReg
		}
		h := &tar.Header{Name: e.name, Mode: 0755, Size: int64(len(e.body)), Typeflag: kind, Linkname: e.link}
		if kind != tar.TypeReg {
			h.Size = 0
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if h.Size > 0 {
			if _, err := io.WriteString(tw, e.body); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func digest(b []byte) string { return fmt.Sprintf("%x", sha256.Sum256(b)) }

func TestProvisionInstallUpdateRecovery(t *testing.T) {
	base := t.TempDir()
	t.Setenv("OYNA_FOLDERS_PATH", filepath.Join(base, "legacy"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(base, "xdg"))
	t.Setenv("HOST_XDG_DATA_HOME", filepath.Join(base, "host-xdg"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(base, "cache"))
	t.Setenv("OYNA_HTTP_RETRIES", "0")
	proton := func(version string) []byte {
		return makeArchive(t, []archiveEntry{
			{name: version + "/toolmanifest.vdf", body: `"manifest" { "commandline" "/proton %verb%" "compatmanager_layer_name" "proton" "require_tool_appid" "1628350" }`},
			{name: version + "/proton", body: "#!/bin/sh\nexit 0\n"},
		}, false)
	}
	rt := makeArchive(t, []archiveEntry{
		{name: "SteamLinuxRuntime_sniper/toolmanifest.vdf", body: `"manifest" { "commandline" "/_v2-entry-point --verb=%verb% --" "compatmanager_layer_name" "container-runtime" }`},
		{name: "SteamLinuxRuntime_sniper/_v2-entry-point", body: "#!/bin/sh\nexit 0\n"},
		{name: "SteamLinuxRuntime_sniper/pressure-vessel/bin/pv-verify", body: "#!/bin/sh\ntest \"$1\" = --quiet && test \"$2\" = --minimized-runtime && test -f \"$3/intact\"\n"},
		{name: "SteamLinuxRuntime_sniper/sniper_platform_test/files/intact", body: "yes"},
	}, true)
	var mu sync.Mutex
	version := "GE-Proton9-1"
	offline := false
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if offline {
			http.Error(w, "offline", 503)
			return
		}
		payload := proton(version)
		switch r.URL.Path {
		case "/repos/GloriousEggroll/proton-ge-custom/releases/latest":
			_ = json.NewEncoder(w).Encode(protonRelease{Assets: []releaseAsset{{Name: version + ".tar.gz", URL: srv.URL + "/proton"}, {Name: version + ".sha512sum", URL: srv.URL + "/sha512"}, {Name: version + ".sha256sum", URL: srv.URL + "/sum"}}})
		case "/sum":
			fmt.Fprintf(w, "%s  %s.tar.gz\n", digest(payload), version)
		case "/sha512":
			fmt.Fprintf(w, "%x  %s.tar.gz\n", sha512.Sum512(payload), version)
		case "/proton":
			_, _ = w.Write(payload)
		case "/steamrt3/images/latest-public-beta.txt":
			fmt.Fprint(w, "test-version\n")
		case "/steamrt3/images/test-version/SHA256SUMS":
			fmt.Fprintf(w, "%s  SteamLinuxRuntime_sniper.tar.xz\n", digest(rt))
		case "/steamrt3/images/test-version/SteamLinuxRuntime_sniper.tar.xz":
			_, _ = w.Write(rt)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	p, err := newProvisioner(base)
	if err != nil {
		t.Fatal(err)
	}
	p.api = srv.URL
	p.steam = srv.URL
	if p.base != filepath.Join(base, ".oyna") || p.downloads.cache != filepath.Join(base, ".oyna", ".cache") {
		t.Fatalf("unexpected managed paths %s %s", p.base, p.downloads.cache)
	}
	first, err := p.prepare(base, "")
	if err != nil {
		t.Fatal(err)
	}
	if first != filepath.Join(base, ".oyna", "proton", "GE-Proton9-1") {
		t.Fatalf("unexpected tool %s", first)
	}
	var wg sync.WaitGroup
	for n := 0; n < 2; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := p.prepare(base, ""); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	mu.Lock()
	version = "GE-Proton9-2"
	mu.Unlock()
	second, err := p.prepare(base, "GE-Latest")
	if err != nil {
		t.Fatal(err)
	}
	if first == second || filepath.Base(second) != "GE-Proton9-2" {
		t.Fatalf("failed update %s", second)
	}
	if !regular(filepath.Join(first, "proton")) {
		t.Fatal("previous build was lost")
	}
	// An invalid runtime update must preserve the validated installation.
	mu.Lock()
	goodRuntime := rt
	rt = makeArchive(t, []archiveEntry{{name: "SteamLinuxRuntime_sniper/toolmanifest.vdf", body: `"manifest" { "commandline" "/_v2-entry-point --verb=%verb% --" }`}}, true)
	mu.Unlock()
	if _, err = p.prepare(base, "GE-Latest"); err != nil {
		t.Fatalf("failed-update fallback: %v", err)
	}
	if err = validateRuntime(filepath.Join(base, ".oyna", "rt", "steamrt3"), runtimes["1628350"]); err != nil {
		t.Fatalf("failed update damaged installed runtime: %v", err)
	}
	mu.Lock()
	rt = goodRuntime
	mu.Unlock()
	// A missing file must fail verification, then be restored from the verified archive.
	if err = os.Remove(filepath.Join(base, ".oyna", "rt", "steamrt3", "sniper_platform_test", "files", "intact")); err != nil {
		t.Fatal(err)
	}
	if _, err = p.prepare(base, "GE-Latest"); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	offline = true
	mu.Unlock()
	if _, err = p.prepare(base, "GE-Latest"); err != nil {
		t.Fatalf("offline fallback: %v", err)
	}
	// Invalid runtime plus unavailable downloads must fail closed.
	if err = os.Remove(filepath.Join(base, ".oyna", "rt", "steamrt3", "sniper_platform_test", "files", "intact")); err != nil {
		t.Fatal(err)
	}
	if _, err = p.prepare(base, "GE-Latest"); err == nil {
		t.Fatal("launched corrupt runtime offline")
	}
}

func TestResumableDownload(t *testing.T) {
	payload := []byte(strings.Repeat("download", 1000))
	sum := digest(payload)
	var resumed atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") == "bytes=123-" {
			resumed.Store(true)
			w.Header().Set("Content-Range", fmt.Sprintf("bytes 123-%d/%d", len(payload)-1, len(payload)))
			w.WriteHeader(206)
			_, _ = w.Write(payload[123:])
			return
		}
		_, _ = w.Write(payload)
	}))
	defer server.Close()
	d := &downloads{client: server.Client(), retries: 1, cache: t.TempDir()}
	if err := os.WriteFile(filepath.Join(d.cache, sum+".parts"), payload[:123], 0600); err != nil {
		t.Fatal(err)
	}
	path, err := d.archive(server.URL, sum)
	if err != nil {
		t.Fatal(err)
	}
	if !resumed.Load() {
		t.Fatal("partial download was not resumed")
	}
	if got, _ := hashFile(path); got != sum {
		t.Fatal("download checksum incorrect")
	}
	if _, err = d.archive(server.URL, strings.Repeat("0", 64)); err == nil {
		t.Fatal("accepted checksum mismatch")
	}
}

func TestArchiveRejectsEscapes(t *testing.T) {
	for _, entries := range [][]archiveEntry{
		{{name: "../escape", body: "bad"}},
		{{name: "root/link", kind: tar.TypeSymlink, link: "../../escape"}},
		{{name: "root/link", kind: tar.TypeSymlink, link: "target"}, {name: "root/link/file", body: "bad"}},
		{{name: "root/hard", kind: tar.TypeLink, link: "../outside"}},
		{{name: "root/a", kind: tar.TypeSymlink, link: ".."}, {name: "root/b", kind: tar.TypeSymlink, link: "a/../../outside"}},
	} {
		dir := t.TempDir()
		path := filepath.Join(dir, "archive.gz")
		if err := os.WriteFile(path, makeArchive(t, entries, false), 0600); err != nil {
			t.Fatal(err)
		}
		dest := filepath.Join(dir, "dest")
		if err := os.Mkdir(dest, 0755); err != nil {
			t.Fatal(err)
		}
		if err := extractArchive(path, "archive.gz", dest); err == nil {
			t.Fatalf("accepted malicious entries %#v", entries)
		}
	}
}

func TestInterruptedDownloadResumes(t *testing.T) {
	payload := []byte(strings.Repeat("archive", 1000))
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Content-Length", fmt.Sprint(len(payload)))
			_, _ = w.Write(payload[:123]) // Deliberately short body produces unexpected EOF.
			return
		}
		if r.Header.Get("Range") != "bytes=123-" {
			t.Errorf("unexpected resume range %q", r.Header.Get("Range"))
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes 123-%d/%d", len(payload)-1, len(payload)))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(payload[123:])
	}))
	defer server.Close()
	d := &downloads{client: server.Client(), retries: 1, cache: t.TempDir()}
	if _, err := d.archive(server.URL, digest(payload)); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatalf("expected interrupted request and resume, got %d", calls.Load())
	}
}

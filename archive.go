package main

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ulikunitz/xz"
	"golang.org/x/sys/unix"
)

// Archive paths and links must stay inside the fresh staging directory.
func archivePath(root, name string) (string, error) {
	if filepath.IsAbs(name) {
		return "", fmt.Errorf("absolute archive path %q", name)
	}
	clean := filepath.Clean(name)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("archive path escapes destination: %q", name)
	}
	return filepath.Join(root, clean), nil
}
func safeParents(root, path string) error {
	relative, err := filepath.Rel(root, filepath.Dir(path))
	if err != nil {
		return err
	}
	current := root
	for _, part := range strings.Split(relative, string(os.PathSeparator)) {
		current = filepath.Join(current, part)
		st, err := os.Lstat(current)
		if os.IsNotExist(err) {
			if err = os.Mkdir(current, 0755); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		if !st.IsDir() {
			return fmt.Errorf("non-directory archive parent %s", current)
		}
	}
	return nil
}
func extractArchive(archive, name, dest string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	var stream io.Reader = f
	switch {
	case strings.HasSuffix(name, ".gz"):
		r, err := gzip.NewReader(f)
		if err != nil {
			return err
		}
		defer r.Close()
		stream = r
	case strings.HasSuffix(name, ".xz"):
		r, err := xz.NewReader(f)
		if err != nil {
			return err
		}
		stream = r
	default:
		return fmt.Errorf("unsupported archive %q", name)
	}
	tr := tar.NewReader(stream)
	type hardlink struct{ path, target string }
	var links []hardlink
	var symlinks []string
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		path, err := archivePath(dest, h.Name)
		if err != nil {
			return err
		}
		if path == dest && h.Typeflag == tar.TypeDir {
			continue
		}
		if err = safeParents(dest, path); err != nil {
			return err
		}
		mode := os.FileMode(h.Mode) & 0777
		switch h.Typeflag {
		case tar.TypeDir:
			if err = os.Mkdir(path, mode|0700); err != nil && !os.IsExist(err) {
				return err
			}
			st, err := os.Lstat(path)
			if err != nil {
				return err
			}
			if !st.IsDir() {
				return fmt.Errorf("invalid archive directory %s", path)
			}
		case tar.TypeReg, tar.TypeRegA:
			out, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
			if err != nil {
				return err
			}
			_, err = io.Copy(out, tr)
			closeErr := out.Close()
			if err != nil {
				return err
			}
			if closeErr != nil {
				return closeErr
			}
		case tar.TypeSymlink:
			if _, err = archivePath(dest, filepath.Join(filepath.Dir(h.Name), h.Linkname)); err != nil {
				return err
			}
			if filepath.IsAbs(h.Linkname) {
				return fmt.Errorf("absolute symlink in archive %s", h.Name)
			}
			symlinks = append(symlinks, h.Name)
			if err = os.Symlink(h.Linkname, path); err != nil {
				return err
			}
		case tar.TypeLink:
			target, err := archivePath(dest, h.Linkname)
			if err != nil {
				return err
			}
			links = append(links, hardlink{path, target})
		default:
			return fmt.Errorf("unsupported archive entry %q (type %d)", h.Name, h.Typeflag)
		}
	}
	for _, link := range links {
		if err = safeParents(dest, link.target); err != nil {
			return err
		}
		st, err := os.Lstat(link.target)
		if err != nil {
			return err
		}
		if !st.Mode().IsRegular() {
			return fmt.Errorf("invalid hard link target %s", link.target)
		}
		if err = os.Link(link.target, link.path); err != nil {
			return err
		}
	}
	for _, name := range symlinks {
		if err = validateArchiveLink(dest, name); err != nil {
			return err
		}
	}
	return nil
}

// Resolve components before cleaning '..', including links to missing files.
func validateArchiveLink(root, name string) error {
	pending := strings.Split(name, "/")
	var stack []string
	followed := 0
	for len(pending) > 0 {
		part := pending[0]
		pending = pending[1:]
		switch part {
		case "", ".":
			continue
		case "..":
			if len(stack) == 0 {
				return fmt.Errorf("symlink escapes archive: %s", name)
			}
			stack = stack[:len(stack)-1]
			continue
		}
		path := filepath.Join(append([]string{root}, append(stack, part)...)...)
		info, err := os.Lstat(path)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			followed++
			if followed > 40 {
				return fmt.Errorf("symlink cycle in archive: %s", name)
			}
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			if filepath.IsAbs(target) {
				return fmt.Errorf("absolute symlink in archive: %s", name)
			}
			pending = append(strings.Split(target, "/"), pending...)
		} else {
			stack = append(stack, part)
		}
	}
	return nil
}

// Exchange whole validated directories atomically. Staging is on the same FS.
func activateDirectory(staged, target string) error {
	if _, err := os.Lstat(target); os.IsNotExist(err) {
		return os.Rename(staged, target)
	} else if err != nil {
		return err
	}
	return unix.Renameat2(unix.AT_FDCWD, staged, unix.AT_FDCWD, target, unix.RENAME_EXCHANGE)
}
func atomicText(path, text string) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".oyna-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.WriteString(text); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

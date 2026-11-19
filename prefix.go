package main

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"syscall"
)

func preparePrefix(dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(dir, "pfx.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	pfx := filepath.Join(dir, "pfx")
	info, err := os.Lstat(pfx)
	switch {
	case os.IsNotExist(err):
		if err = os.Symlink(".", pfx); err != nil {
			return err
		}
	case err != nil:
		return err
	case info.Mode()&os.ModeSymlink != 0:
		temp, err := os.MkdirTemp(dir, ".oyna-pfx-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(temp)
		replacement := filepath.Join(temp, "pfx")
		if err = os.Symlink(".", replacement); err != nil {
			return err
		}
		// Rename replaces the link itself atomically under the prefix lock.
		if err = os.Rename(replacement, pfx); err != nil {
			return err
		}
	case !info.IsDir():
		return fmt.Errorf("expected directory at %s", pfx)
	}
	for _, sub := range []string{"shadercache", "gstreamer-1.0"} {
		if err = os.MkdirAll(filepath.Join(dir, sub), 0755); err != nil {
			return err
		}
	}
	f, err := os.OpenFile(filepath.Join(dir, "tracked_files"), os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	u, err := user.Current()
	if err != nil {
		return err
	}
	users := filepath.Join(pfx, "drive_c", "users")
	steam := filepath.Join(users, "steamuser")
	login := filepath.Join(users, u.Username)
	_, serr := os.Stat(steam)
	_, uerr := os.Stat(login)
	if serr != nil && !os.IsNotExist(serr) {
		return serr
	}
	if uerr != nil && !os.IsNotExist(uerr) {
		return uerr
	}
	if os.IsNotExist(serr) {
		if uerr == nil {
			return os.Symlink(u.Username, steam)
		}
		if err = os.MkdirAll(steam, 0755); err != nil {
			return err
		}
	}
	if login != steam && os.IsNotExist(uerr) {
		return os.Symlink("steamuser", login)
	}
	return nil
}

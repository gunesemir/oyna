package main

import (
	"os"
	"path/filepath"
)

// This script runs inside pressure-vessel, where the host launcher's path may
// be unavailable. Publish it atomically alongside the user's installed runtime.
func runtimeShim(base string) (string, error) {
	dir := filepath.Join(base, "umu")
	path := filepath.Join(dir, "umu-shim")
	if regular(path) {
		return path, nil
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(dir, ".umu-shim-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	const script = `#!/bin/sh
case "$XDG_CURRENT_DESKTOP:$XDG_SESSION_DESKTOP" in
    gamescope:*|*:gamescope)
        if [ "$STEAM_MULTIPLE_XWAYLANDS" = 1 ] && [ -z "$DISPLAY" ]; then
            export DISPLAY=:1
        fi
        ;;
esac
exec "$@"
`
	if _, err = f.WriteString(script); err == nil {
		err = f.Chmod(0700)
	}
	closeErr := f.Close()
	if err != nil {
		return "", err
	}
	if closeErr != nil {
		return "", closeErr
	}
	if err = os.Link(f.Name(), path); err != nil && !(os.IsExist(err) && regular(path)) {
		return "", err
	}
	return path, nil
}

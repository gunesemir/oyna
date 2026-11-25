//go:build linux

package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func (i invocation) runNative(home string) error {
	exe, err := filepath.Abs(expandHome(i.Exe, home))
	if err != nil {
		return err
	}
	info, err := os.Stat(exe)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("expected a native game file at %q", exe)
	}
	i.Exe = exe
	cwd := filepath.Dir(exe)
	var cmd []string
	if i.Runtime != "" {
		cmd, err = nativeRuntimeCommand(home, i.Runtime)
		if err != nil {
			return err
		}
		// Expose the game directory to pressure-vessel, including external drives.
		if err := os.Setenv("STEAM_COMPAT_INSTALL_PATH", cwd); err != nil {
			return err
		}
	}
	cmd = append(cmd, "/bin/sh")
	if info.Mode().Perm()&0111 != 0 {
		// Honor executable binaries and shebangs. Paths and arguments stay in argv.
		cmd = append(cmd, "-c", `exec "$@"`, "oyna")
	} else if filepath.Ext(exe) != ".sh" {
		return fmt.Errorf("native game %q needs execute permission", exe)
	}
	return i.launch(cmd, cwd)
}

func nativeRuntimeCommand(home, name string) ([]string, error) {
	for appID, spec := range runtimes {
		if spec.variant != name {
			continue
		}
		p, err := newProvisioner(home)
		if err != nil {
			return nil, err
		}
		if err := p.runtime(appID); err != nil {
			return nil, err
		}
		return runtimeCommand(filepath.Join(p.base, "rt", name), "waitforexitandrun")
	}
	return nil, fmt.Errorf("unknown Steam Linux Runtime %q", name)
}

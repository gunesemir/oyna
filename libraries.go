package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Combine the game directory and glibc's search directories for pressure-vessel.
func libraryPaths(install string) string {
	paths := filepath.SplitList(os.Getenv("LD_LIBRARY_PATH"))
	if install != "" {
		paths = append(paths, install)
	}
	ldconfig, err := exec.LookPath("ldconfig")
	if err != nil {
		ldconfig = "/sbin/ldconfig"
	}
	cmd := exec.Command(ldconfig, "-p")
	cmd.Env = []string{"LC_ALL=C"}
	if out, err := cmd.Output(); err == nil {
		seen := map[string]bool{}
		for _, line := range strings.Split(string(out), "\n") {
			_, lib, ok := strings.Cut(line, " => ")
			if !ok || !filepath.IsAbs(lib) {
				continue
			}
			dir := filepath.Dir(lib)
			if seen[dir] {
				continue
			}
			seen[dir] = true
			if real, err := filepath.EvalSymlinks(dir); err == nil {
				paths = append(paths, real)
			}
		}
	}
	return strings.Join(paths, ":")
}

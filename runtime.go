package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

type runtimeSpec struct{ variant, name, archive string }

var runtimes = map[string]runtimeSpec{
	"1391110": {"steamrt2", "soldier", "SteamLinuxRuntime_soldier.tar.xz"},
	"1628350": {"steamrt3", "sniper", "SteamLinuxRuntime_sniper.tar.xz"},
	"4183110": {"steamrt4", "steamrt4", "SteamLinuxRuntime_4.tar.xz"},
	"4185400": {"steamrt4-arm64", "steamrt4", "SteamLinuxRuntime_4.tar.xz"},
}

func validateRuntime(dir string, spec runtimeSpec) error {
	if spec.variant == "steamrt4-arm64" && runtime.GOARCH == "amd64" {
		return fmt.Errorf("ARM64 runtime on x86-64 host")
	}
	if _, err := executableLayer(dir, "run"); err != nil {
		return err
	}
	matches, err := filepath.Glob(filepath.Join(dir, spec.name+"_platform_*"))
	if err != nil {
		return err
	}
	sort.Sort(sort.Reverse(sort.StringSlice(matches)))
	platform := ""
	for _, match := range matches {
		if st, err := os.Stat(match); err == nil && st.IsDir() {
			platform = match
			break
		}
	}
	if platform == "" {
		return fmt.Errorf("%s: runtime platform missing", dir)
	}
	cmd := exec.Command(filepath.Join(dir, "pressure-vessel", "bin", "pv-verify"), "--quiet", "--minimized-runtime", filepath.Join(platform, "files"))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s validation: %w: %s", spec.variant, err, strings.TrimSpace(string(out)))
	}
	return nil
}
func (p *provisioner) runtime(appID string) error {
	if appID == "" {
		return nil
	}
	spec, ok := runtimes[appID]
	if !ok {
		return fmt.Errorf("unknown runtime app ID %q", appID)
	}
	if spec.variant == "steamrt4-arm64" && runtime.GOARCH == "amd64" {
		return fmt.Errorf("ARM64 runtime on x86-64 host")
	}
	dir := filepath.Join(p.base, "umu", spec.variant)
	unlock, err := cacheLock(filepath.Join(p.base, "umu", "umu.lock"))
	if err != nil {
		return err
	}
	defer unlock()
	validation := validateRuntime(dir, spec)
	if validation == nil && os.Getenv("UMU_RUNTIME_UPDATE") == "0" {
		return nil
	}
	err = p.installRuntime(dir, spec, validation == nil)
	if err != nil && validation == nil {
		fmt.Fprintf(os.Stderr, "oyna: runtime update failed, using validated installation: %v\n", err)
		return nil
	}
	if err != nil {
		return fmt.Errorf("runtime setup failed (validation: %v): %w", validation, err)
	}
	return nil
}
func (p *provisioner) installRuntime(dir string, spec runtimeSpec, valid bool) error {
	images := p.steam + "/" + strings.TrimSuffix(spec.variant, "-arm64") + "/images/"
	data, err := p.downloads.bytes(images + "latest-public-beta.txt")
	if err != nil {
		return err
	}
	version := strings.TrimSpace(string(data))
	if version == "" || version == "." || version == ".." || filepath.Base(version) != version {
		return fmt.Errorf("invalid runtime version %q", version)
	}
	endpoint := images + version + "/"
	sums, err := p.downloads.bytes(endpoint + "SHA256SUMS")
	if err != nil {
		return err
	}
	digest, err := checksum(sums, spec.archive)
	if err != nil {
		return err
	}
	installed, _ := os.ReadFile(filepath.Join(dir, ".oyna-sha256"))
	if valid && strings.TrimSpace(string(installed)) == digest {
		return nil
	}
	fmt.Fprintf(os.Stderr, "oyna: downloading %s (%s)\n", spec.variant, version)
	path, err := p.downloads.archive(endpoint+spec.archive, digest)
	if err != nil {
		return err
	}
	stage, err := os.MkdirTemp(filepath.Dir(dir), ".oyna-runtime-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if err = extractArchive(path, spec.archive, stage); err != nil {
		return err
	}
	extracted := filepath.Join(stage, strings.TrimSuffix(spec.archive, ".tar.xz"))
	if err = validateRuntime(extracted, spec); err != nil {
		return err
	}
	if err = atomicText(filepath.Join(extracted, ".oyna-sha256"), digest+"\n"); err != nil {
		return err
	}
	return activateDirectory(extracted, dir)
}

func resolveTool(home, proton string) (string, error) {
	dir := expandHome(proton, home)
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(dataHome(home), "Steam", "compatibilitytools.d", dir)
	}
	return filepath.EvalSymlinks(dir)
}
func (p *provisioner) prepare(home, proton string) (string, error) {
	dir, err := p.proton(proton)
	if err != nil {
		return "", err
	}
	dir, err = resolveTool(home, dir)
	if err != nil {
		return "", err
	}
	tool, err := executableLayer(dir, "run")
	if err != nil {
		return "", err
	}
	if err = p.runtime(tool.required); err != nil {
		return "", err
	}
	return dir, nil
}

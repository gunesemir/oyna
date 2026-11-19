package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

const githubAPI = "https://api.github.com"
const steamRepo = "https://repo.steampowered.com"

type releaseAsset struct {
	Name string
	URL  string `json:"browser_download_url"`
}
type protonRelease struct {
	Assets []releaseAsset `json:"assets"`
}

type provisioner struct {
	downloads        *downloads
	base, api, steam string
}

func newProvisioner(home string) (*provisioner, error) {
	d, err := newDownloads(home)
	if err != nil {
		return nil, err
	}
	return &provisioner{d, dataHome(home), githubAPI, steamRepo}, nil
}
func protonFamily(alias string) string {
	switch alias {
	case "", "UMU-Proton", "UMU-Latest":
		return "UMU-Proton"
	case "GE-Proton", "GE-Latest":
		return "GE-Proton"
	}
	return ""
}
func (p *provisioner) proton(alias string) (string, error) {
	family := protonFamily(alias)
	if family == "" {
		return alias, nil
	}
	if alias == "" {
		alias = "UMU-Latest"
	}
	unlock, err := cacheLock(filepath.Join(p.base, "umu", "compatibilitytools.d.lock"))
	if err != nil {
		return "", err
	}
	defer unlock()
	dir, err := p.installProton(alias, family)
	if err == nil {
		return dir, nil
	}
	if cached := p.cachedProton(alias, family); cached != "" {
		fmt.Fprintf(os.Stderr, "oyna: Proton update failed, using %s: %v\n", cached, err)
		return cached, nil
	}
	return "", err
}
func (p *provisioner) cachedProton(alias, family string) string {
	aliasDir := filepath.Join(p.base, "umu", "compatibilitytools", alias)
	if _, err := executableLayer(aliasDir, "run"); err == nil {
		return aliasDir
	}
	var candidates []string
	for _, root := range []string{filepath.Join(p.base, "Steam", "compatibilitytools.d"), filepath.Join(p.base, "umu", "compatibilitytools")} {
		entries, _ := os.ReadDir(root)
		for _, entry := range entries {
			if !strings.HasPrefix(entry.Name(), family) {
				continue
			}
			path := filepath.Join(root, entry.Name())
			if _, err := executableLayer(path, "run"); err == nil {
				candidates = append(candidates, path)
			}
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return naturalLess(filepath.Base(candidates[i]), filepath.Base(candidates[j])) })
	if len(candidates) > 0 {
		return candidates[len(candidates)-1]
	}
	return ""
}

var numericParts = regexp.MustCompile(`[0-9]+|[^0-9]+`)

func naturalLess(a, b string) bool {
	aa, bb := numericParts.FindAllString(a, -1), numericParts.FindAllString(b, -1)
	for i := 0; i < len(aa) && i < len(bb); i++ {
		if aa[i] == bb[i] {
			continue
		}
		an, ae := strconv.ParseUint(aa[i], 10, 64)
		bn, be := strconv.ParseUint(bb[i], 10, 64)
		if ae == nil && be == nil {
			return an < bn
		}
		return aa[i] < bb[i]
	}
	return len(aa) < len(bb)
}
func (p *provisioner) installProton(alias, family string) (string, error) {
	repo := "Open-Wine-Components/umu-proton"
	if family == "GE-Proton" {
		repo = "GloriousEggroll/proton-ge-custom"
	}
	data, err := p.downloads.bytes(p.api + "/repos/" + repo + "/releases/latest")
	if err != nil {
		return "", err
	}
	var release protonRelease
	if err = json.Unmarshal(data, &release); err != nil {
		return "", err
	}
	var archive, sum releaseAsset
	for _, asset := range release.Assets {
		arm := strings.Contains(asset.Name, "aarch64") || strings.Contains(asset.Name, "arm64")
		if arm != (runtime.GOARCH == "arm64") {
			continue
		}
		if strings.HasPrefix(asset.Name, family) && (strings.HasSuffix(asset.Name, ".tar.gz") || strings.HasSuffix(asset.Name, ".tar.xz")) {
			archive = asset
		}
	}
	if archive.URL == "" {
		return "", fmt.Errorf("no %s release archive for %s", family, runtime.GOARCH)
	}
	root := strings.TrimSuffix(strings.TrimSuffix(archive.Name, ".tar.gz"), ".tar.xz")
	for _, asset := range release.Assets {
		if asset.Name == root+".sha512sum" || asset.Name == archive.Name+".sha512sum" {
			sum = asset
			break
		}
		if asset.Name == root+".sha256sum" || asset.Name == archive.Name+".sha256sum" {
			sum = asset
		}
	}
	if sum.URL == "" {
		return "", fmt.Errorf("no checksum asset for %s", archive.Name)
	}
	if filepath.Base(root) != root {
		return "", fmt.Errorf("invalid release name %q", root)
	}
	parent := filepath.Join(p.base, "Steam", "compatibilitytools.d")
	target := filepath.Join(parent, root)
	if _, err := executableLayer(target, "run"); err != nil {
		sums, err := p.downloads.bytes(sum.URL)
		if err != nil {
			return "", err
		}
		digest, err := checksum(sums, archive.Name)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(os.Stderr, "oyna: downloading %s\n", archive.Name)
		path, err := p.downloads.archive(archive.URL, digest)
		if err != nil {
			return "", err
		}
		if err = os.MkdirAll(parent, 0755); err != nil {
			return "", err
		}
		stage, err := os.MkdirTemp(parent, ".oyna-proton-*")
		if err != nil {
			return "", err
		}
		defer os.RemoveAll(stage)
		if err = extractArchive(path, archive.Name, stage); err != nil {
			return "", err
		}
		extracted := filepath.Join(stage, root)
		if _, err = executableLayer(extracted, "run"); err != nil {
			return "", err
		}
		if err = activateDirectory(extracted, target); err != nil {
			return "", err
		}
	}
	if strings.HasSuffix(alias, "-Latest") {
		dir := filepath.Join(p.base, "umu", "compatibilitytools")
		if err = os.MkdirAll(dir, 0755); err != nil {
			return "", err
		}
		tmp, err := os.MkdirTemp(dir, ".oyna-alias-*")
		if err != nil {
			return "", err
		}
		defer os.RemoveAll(tmp)
		link := filepath.Join(tmp, "link")
		if err = os.Symlink(target, link); err != nil {
			return "", err
		}
		aliasPath := filepath.Join(dir, alias)
		if err = activateDirectory(link, aliasPath); err != nil {
			return "", err
		}
		return aliasPath, nil
	}
	return target, nil
}

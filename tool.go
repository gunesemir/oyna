package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/andygrunwald/vdf"
	"github.com/google/shlex"
)

type layer struct {
	dir, kind, required string
	command             []string
}

func readLayer(dir, verb string) (layer, error) {
	l := layer{dir: dir}
	f, err := os.Open(filepath.Join(dir, "toolmanifest.vdf"))
	if err != nil {
		return l, err
	}
	defer f.Close()
	doc, err := vdf.NewParser(f).Parse()
	if err != nil {
		return l, fmt.Errorf("%s: %w", f.Name(), err)
	}
	m, _ := doc["manifest"].(map[string]interface{})
	line, _ := m["commandline"].(string)
	l.kind, _ = m["compatmanager_layer_name"].(string)
	l.required, _ = m["require_tool_appid"].(string)
	cmd, err := shlex.Split(strings.ReplaceAll(line, "%verb%", verb))
	if err != nil || len(cmd) == 0 || !strings.HasPrefix(cmd[0], "/") {
		return l, fmt.Errorf("invalid commandline in %s", f.Name())
	}
	if l.kind == "container-runtime" && cmd[0] == "/_v2-entry-point" && regular(filepath.Join(dir, "umu")) {
		cmd[0] = "/umu"
	}
	cmd[0] = filepath.Join(dir, cmd[0])
	l.command = cmd
	return l, nil
}

func executableLayer(dir, verb string) (layer, error) {
	l, err := readLayer(dir, verb)
	if err != nil {
		return l, err
	}
	st, err := os.Stat(l.command[0])
	if err != nil {
		return l, err
	}
	if !st.Mode().IsRegular() || st.Mode().Perm()&0111 == 0 {
		return l, fmt.Errorf("tool entry point is not executable: %s", l.command[0])
	}
	return l, nil
}

func regular(path string) bool {
	s, err := os.Stat(path)
	return err == nil && s.Mode().IsRegular()
}

func dataHome(home string) string {
	if p, ok := os.LookupEnv("UMU_FOLDERS_PATH"); ok {
		return p
	}
	key := "XDG_DATA_HOME"
	if os.Getenv("container") == "flatpak" {
		key = "HOST_XDG_DATA_HOME"
	}
	if p := os.Getenv(key); p != "" {
		return p
	}
	return filepath.Join(home, ".local", "share")
}

func installedCommand(home, proton, verb string) (layer, string, []string, error) {
	var tool layer
	base := dataHome(home)
	if proton == "" {
		return tool, "", nil, fmt.Errorf("set PROTONPATH to an installed tool (automatic installation is pending)")
	}
	dir := expandHome(proton, home)
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(base, "Steam", "compatibilitytools.d", dir)
	}
	dir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return tool, "", nil, err
	}
	tool, err = readLayer(dir, verb)
	if err != nil {
		return tool, "", nil, err
	}
	if tool.required == "" {
		shim, err := runtimeShim(base)
		if err != nil {
			return tool, "", nil, err
		}
		return tool, "", append([]string{shim}, tool.command...), nil
	}
	variants := map[string]string{"1391110": "steamrt2", "1628350": "steamrt3", "4183110": "steamrt4", "4185400": "steamrt4-arm64"}
	variant := variants[tool.required]
	if variant == "" {
		return tool, "", nil, fmt.Errorf("unknown runtime app ID %q", tool.required)
	}
	if variant == "steamrt4-arm64" && runtime.GOARCH == "amd64" {
		return tool, "", nil, fmt.Errorf("ARM64 runtime on x86-64 host")
	}
	rt := filepath.Join(base, "umu", variant)
	container, err := readLayer(rt, verb)
	if err != nil {
		return tool, "", nil, fmt.Errorf("installed %s runtime: %w", variant, err)
	}
	if container.required != "" || container.kind != "container-runtime" {
		return tool, "", nil, fmt.Errorf("unexpected runtime manifest in %s", rt)
	}
	shim, err := runtimeShim(base)
	if err != nil {
		return tool, "", nil, err
	}
	cmd := append(container.command, shim)
	return tool, rt, append(cmd, tool.command...), nil
}

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/google/shlex"
)

type invocation struct {
	Exe, Prefix, Proton, GameID, Store string
	Runtime                            string
	Args                               []string
	Wrapper                            []string
	LaunchEnv                          map[string]string
}

func (i invocation) native() bool {
	value, ok := i.LaunchEnv["OYNA_NO_PROTON"]
	if !ok {
		value = os.Getenv("OYNA_NO_PROTON")
	}
	return value == "1"
}

// Proton and its graphics layers can add runtime options independently of Oyna.
var protonOptionName = regexp.MustCompile(`^(?:(?:PROTON_|WINE_|DXVK_|VKD3D_)[A-Z0-9_]+|WINEDLLOVERRIDES|WINEDEBUG|WINEARCH|STEAM_COMPAT_CONFIG|HOST_LC_ALL|FNA3D_FORCE_DRIVER)$`)
var envName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func isProtonOption(key string) bool {
	return key != "PROTON_VERB" && protonOptionName.MatchString(key)
}

func (i *invocation) parseArgs(value string) error {
	words, err := shlex.Split(value)
	if err != nil {
		return err
	}
	command := slices.Index(words, "%command%")
	if command < 0 {
		i.Args = append(i.Args, words...)
		return nil
	}
	if slices.Contains(words[command+1:], "%command%") {
		return fmt.Errorf("expected exactly one %%command%%")
	}
	wrapper := words[:command]
	for len(wrapper) > 0 {
		key, value, ok := strings.Cut(wrapper[0], "=")
		if !ok || !envName.MatchString(key) {
			break
		}
		i.LaunchEnv[key] = value
		wrapper = wrapper[1:]
	}
	if len(wrapper) > 0 {
		if len(i.Wrapper) > 0 {
			return fmt.Errorf("put all launch wrappers on one arg line")
		}
		i.Wrapper = wrapper
	}
	i.Args = append(i.Args, words[command+1:]...)
	return nil
}

func expandHome(s, home string) string {
	if s == "~" {
		return home
	}
	if strings.HasPrefix(s, "~/") {
		return filepath.Join(home, s[2:])
	}
	return s
}
func fromFile(path, home string) (invocation, error) {
	i := invocation{LaunchEnv: make(map[string]string)}
	path = expandHome(path, home)
	data, err := os.ReadFile(path)
	if err != nil {
		return i, err
	}
	fields := map[string]*string{
		"exe":        &i.Exe,
		"runtime":    &i.Runtime,
		"WINEPREFIX": &i.Prefix,
		"PROTONPATH": &i.Proton,
		"GAMEID":     &i.GameID,
		"STORE":      &i.Store,
	}
	for index, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSuffix(line, "\r")
		content := strings.TrimSpace(line)
		if content == "" || strings.HasPrefix(content, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return i, fmt.Errorf("%s:%d: expected key=value", path, index+1)
		}
		switch key {
		case "art":
			continue
		case "arg":
			if err := i.parseArgs(value); err != nil {
				return i, fmt.Errorf("%s:%d: invalid arg value: %w", path, index+1, err)
			}
			continue
		}
		field, ok := fields[key]
		switch {
		case ok:
			*field = value
		case key == "OYNA_NO_PROTON" || isProtonOption(key):
			i.LaunchEnv[key] = value
		default:
			return i, fmt.Errorf("%s:%d: unknown key %q", path, index+1, key)
		}
	}
	if i.Exe == "" {
		return i, fmt.Errorf("%s: exe is required", path)
	}
	i.Exe = expandHome(i.Exe, home)
	native := i.native()
	protonShortcut := !native && (i.Exe == "winecfg.exe" || i.Exe == "createprefix")
	if !protonShortcut && !regular(i.Exe) {
		return i, fmt.Errorf("expected executable file at %q", i.Exe)
	}
	if native {
		return i, nil
	}
	if i.Runtime != "" {
		return i, fmt.Errorf("%s: runtime requires OYNA_NO_PROTON=1", path)
	}
	for _, dir := range []*string{&i.Prefix, &i.Proton} {
		if strings.TrimSpace(*dir) == "" {
			*dir = ""
			continue
		}
		*dir = expandHome(*dir, home)
		s, e := os.Stat(*dir)
		if e != nil {
			return i, e
		}
		if !s.IsDir() {
			return i, fmt.Errorf("expected directory at %q", *dir)
		}
	}
	// Paths from the config are relative to the calling directory.
	if i.Proton != "" {
		i.Proton, err = filepath.Abs(i.Proton)
	}
	return i, err
}

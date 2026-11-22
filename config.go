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
	Args                               []string
	ProtonOptions                      map[string]string
}

// Proton and its graphics layers can add runtime options independently of Oyna.
var protonOptionName = regexp.MustCompile(`^(?:(?:PROTON_|WINE_|DXVK_|VKD3D_)[A-Z0-9_]+|WINEDLLOVERRIDES|WINEDEBUG|WINEARCH|STEAM_COMPAT_CONFIG|HOST_LC_ALL|FNA3D_FORCE_DRIVER)$`)

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
	for _, word := range words[:command] {
		key, value, ok := strings.Cut(word, "=")
		if !ok || !isProtonOption(key) {
			return fmt.Errorf("expected a Proton environment assignment before %%command%%, got %q", word)
		}
		i.ProtonOptions[key] = value
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
	i := invocation{ProtonOptions: make(map[string]string)}
	path = expandHome(path, home)
	data, err := os.ReadFile(path)
	if err != nil {
		return i, err
	}
	fields := map[string]*string{
		"exe":        &i.Exe,
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
		case isProtonOption(key):
			i.ProtonOptions[key] = value
		default:
			return i, fmt.Errorf("%s:%d: unknown key %q", path, index+1, key)
		}
	}
	for _, key := range []string{"exe", "WINEPREFIX", "PROTONPATH"} {
		field := fields[key]
		if *field == "" {
			return i, fmt.Errorf("%s: %s is required", path, key)
		}
		*field = expandHome(*field, home)
	}
	for _, p := range []string{i.Prefix, i.Proton} {
		s, e := os.Stat(p)
		if e != nil {
			return i, e
		}
		if !s.IsDir() {
			return i, fmt.Errorf("expected directory at %q", p)
		}
	}
	if !regular(i.Exe) {
		return i, fmt.Errorf("expected executable file at %q", i.Exe)
	}
	// Paths from the config are relative to the calling directory.
	i.Proton, err = filepath.Abs(i.Proton)
	return i, err
}

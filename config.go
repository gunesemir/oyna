package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/shlex"
)

type invocation struct {
	Exe, Prefix, Proton, GameID, Store string
	Args                               []string
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
	var i invocation
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
			args, err := shlex.Split(value)
			if err != nil {
				return i, fmt.Errorf("%s:%d: invalid arg value: %w", path, index+1, err)
			}
			i.Args = append(i.Args, args...)
			continue
		}
		field, ok := fields[key]
		if !ok {
			return i, fmt.Errorf("%s:%d: unknown key %q", path, index+1, key)
		}
		*field = value
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

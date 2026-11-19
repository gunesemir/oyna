package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
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
	type launchConfig struct {
		Exe, Prefix, Proton, Store string
		GameID                     string `toml:"game_id"`
		Args                       any    `toml:"launch_args"`
	}
	var cfg struct {
		Oyna *launchConfig
	}
	var i invocation
	if _, err := toml.DecodeFile(expandHome(path, home), &cfg); err != nil {
		return i, err
	}
	c := cfg.Oyna
	if c == nil {
		return i, fmt.Errorf("configuration requires [oyna]")
	}
	i = invocation{Exe: expandHome(c.Exe, home), Prefix: expandHome(c.Prefix, home), Proton: expandHome(c.Proton, home), GameID: c.GameID, Store: c.Store}
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
	switch args := c.Args.(type) {
	case nil:
	case string:
		i.Args = strings.Split(args, " ")
	case []any:
		for _, v := range args {
			s, ok := v.(string)
			if !ok {
				return i, fmt.Errorf("launch_args must contain strings")
			}
			i.Args = append(i.Args, s)
		}
	default:
		return i, fmt.Errorf("invalid launch_args")
	}
	// Paths from the config are relative to the calling directory.
	var err error
	i.Proton, err = filepath.Abs(i.Proton)
	return i, err
}

//go:build linux

package main

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
)

const usage = "Usage: oyna [PROTON_VERB] EXE [ARG ...]\n       oyna --config FILE\n       oyna --help | --version\nSet PROTONPATH to an installed tool. Use an empty EXE or createprefix to initialize a prefix."

func validVerb(v string) bool {
	switch v {
	case "waitforexitandrun", "run", "runinprefix", "destroyprefix", "getcompatpath", "getnativepath":
		return true
	}
	return false
}
func run(args []string) error {
	if len(args) == 0 {
		return errors.New(usage)
	}
	switch args[0] {
	case "--help", "-h":
		fmt.Println(usage)
		return nil
	case "--version", "-v":
		fmt.Println("oyna 0.1.0")
		return nil
	}
	if os.Geteuid() == 0 {
		return errors.New("run oyna as an unprivileged user")
	}
	if strings.Contains(os.Getenv("LD_LIBRARY_PATH"), "musl") {
		return errors.New("a glibc host is required")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	i := invocation{Proton: os.Getenv("PROTONPATH"), Prefix: os.Getenv("WINEPREFIX"), GameID: os.Getenv("GAMEID"), Store: os.Getenv("STORE")}
	verb, override := os.LookupEnv("PROTON_VERB")
	if args[0] == "--config" {
		if len(args) != 2 {
			return errors.New("expected --config FILE")
		}
		i, err = fromFile(args[1], home)
		if err != nil {
			return err
		}
	} else {
		if validVerb(args[0]) {
			if !override {
				verb = args[0]
			}
			args = args[1:]
		}
		if len(args) == 0 {
			return errors.New("missing executable after Proton verb")
		}
		i.Exe, i.Args = args[0], args[1:]
		if _, ok := os.LookupEnv("WINEPREFIX"); ok && i.Prefix == "" {
			return errors.New("WINEPREFIX is empty")
		}
	}
	if !validVerb(verb) {
		verb = "waitforexitandrun"
	}
	if os.Getenv("UMU_NO_PROTON") == "1" || os.Getenv("UMU_CONTAINER_NSENTER") == "1" {
		return errors.New("native passthrough and container re-entry are pending")
	}
	if strings.HasSuffix(i.Exe, "winetricks") {
		return errors.New("winetricks support is pending")
	}
	tool, rt, cmd, err := installedCommand(home, i.Proton, verb)
	if err != nil {
		return err
	}
	if i.GameID == "" {
		i.GameID = "umu-default"
	}
	if i.Prefix == "" {
		i.Prefix = filepath.Join(home, "Games", "umu", i.GameID)
		if i.Store != "" {
			i.Prefix = filepath.Join(home, "Games", i.Store)
		}
	}
	prefix, err := filepath.Abs(expandHome(i.Prefix, home))
	if err != nil {
		return err
	}
	if tool.kind == "proton" {
		if err = preparePrefix(prefix); err != nil {
			return err
		}
	}
	if resolved, e := filepath.EvalSymlinks(prefix); e == nil {
		prefix = resolved
	}

	install := os.Getenv("STEAM_COMPAT_INSTALL_PATH")
	if i.Exe == "createprefix" || i.Exe == "" {
		i.Exe = ""
		install = ""
	} else {
		path, e := filepath.Abs(expandHome(i.Exe, home))
		if e != nil {
			return e
		}
		if _, e = os.Stat(path); e == nil {
			i.Exe = path
			if install == "" {
				install = filepath.Dir(path)
			}
		} else if !os.IsNotExist(e) {
			return e
		}
	}
	steamID := "0"
	if regexp.MustCompile(`^umu-[\pL\pN_]+$`).MatchString(i.GameID) {
		steamID = strings.TrimPrefix(i.GameID, "umu-")
	}
	id := make([]byte, 16)
	if _, err = rand.Read(id); err != nil {
		return err
	}
	mounts := tool.dir
	if rt != "" {
		mounts += ":" + rt
	}
	vars := map[string]string{
		"GAMEID":                           i.GameID,
		"UMU_ID":                           i.GameID,
		"STORE":                            i.Store,
		"WINEPREFIX":                       prefix,
		"PROTONPATH":                       tool.dir,
		"RUNTIMEPATH":                      rt,
		"EXE":                              i.Exe,
		"PROTON_VERB":                      verb,
		"SteamAppId":                       steamID,
		"SteamGameId":                      steamID,
		"UMU_STEAM_GAME_ID":                os.Getenv("SteamGameId"),
		"UMU_INVOCATION_ID":                hex.EncodeToString(id),
		"STEAM_COMPAT_APP_ID":              fmt.Sprintf("%x", md5.Sum([]byte(prefix))),
		"STEAM_COMPAT_TOOL_PATHS":          mounts,
		"STEAM_COMPAT_MOUNTS":              mounts,
		"STEAM_COMPAT_DATA_PATH":           prefix,
		"STEAM_COMPAT_SHADER_PATH":         filepath.Join(prefix, "shadercache"),
		"STEAM_COMPAT_INSTALL_PATH":        install,
		"STEAM_RUNTIME_LIBRARY_PATH":       libraryPaths(install),
		"STEAM_COMPAT_CLIENT_INSTALL_PATH": "",
		"PROTON_CRASH_REPORT_DIR":          filepath.Join(os.TempDir(), "umu_crashreports"),
	}
	for k, v := range vars {
		if err = os.Setenv(k, v); err != nil {
			return err
		}
	}
	cmd = append(cmd, i.Exe)
	cmd = append(cmd, i.Args...)
	if log := os.Getenv("UMU_LOG"); log == "1" || log == "debug" {
		fmt.Fprintf(os.Stderr, "oyna: launching %q\n", cmd)
	}
	return syscall.Exec(cmd[0], cmd, os.Environ())
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "oyna:", err)
		os.Exit(1)
	}
}

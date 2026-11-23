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
)

const usage = "Usage: oyna [PROTON_VERB] EXE [ARG ...]\n       oyna --config FILE\n       oyna --help | --version\nUnset PROTONPATH selects GE-Proton. PROTONPATH accepts an installed tool, UMU-Proton, GE-Proton, UMU-Latest or GE-Latest. Use an empty EXE or createprefix to initialize a prefix."

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
		fmt.Println("oyna 0.1")
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
	if os.Getenv("OYNA_NO_PROTON") == "1" {
		return errors.New("native passthrough is unsupported")
	}
	isWinetricks := strings.HasSuffix(i.Exe, "winetricks")
	if isWinetricks {
		if err = checkWinetricks(i.Args, ""); err != nil {
			return err
		}
	}
	provision, err := newProvisioner(home)
	if err != nil {
		return err
	}
	i.Proton, err = provision.prepare(home, i.Proton)
	if err != nil {
		return err
	}
	tool, rt, cmd, err := installedCommand(home, i.Proton, verb)
	if err != nil {
		return err
	}
	if i.GameID == "" {
		i.GameID = "umu-default"
	}
	if i.Prefix == "" {
		i.Prefix = filepath.Join(oynaHome(home), "prefixes", "default")
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

	cwd := ""
	if isWinetricks {
		if tool.kind != "proton" {
			return errors.New("winetricks requires Proton")
		}
		if err = checkWinetricks(i.Args, prefix); err != nil {
			return err
		}
		cwd = filepath.Join(tool.dir, "protonfixes")
		i.Exe = "winetricks"
		if bundled := filepath.Join(cwd, "winetricks"); regular(bundled) {
			i.Exe = bundled
		}
		for n, arg := range i.Args {
			if strings.Contains(arg, ".") && regular(arg) {
				i.Args[n], err = filepath.Abs(arg)
				if err != nil {
					return err
				}
			}
		}
		i.Args = append([]string{"-q"}, i.Args...)
	}
	install := os.Getenv("STEAM_COMPAT_INSTALL_PATH")
	if isWinetricks {
		// Winetricks resolves inside Proton and runs from protonfixes.
	} else if i.Exe == "createprefix" || i.Exe == "" {
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
		"OYNA_STEAM_GAME_ID":               os.Getenv("SteamGameId"),
		"OYNA_INVOCATION_ID":               hex.EncodeToString(id),
		"STEAM_COMPAT_APP_ID":              fmt.Sprintf("%x", md5.Sum([]byte(prefix))),
		"STEAM_COMPAT_TOOL_PATHS":          mounts,
		"STEAM_COMPAT_MOUNTS":              mounts,
		"STEAM_COMPAT_DATA_PATH":           prefix,
		"STEAM_COMPAT_SHADER_PATH":         filepath.Join(prefix, "shadercache"),
		"STEAM_COMPAT_INSTALL_PATH":        install,
		"STEAM_RUNTIME_LIBRARY_PATH":       libraryPaths(install),
		"STEAM_COMPAT_CLIENT_INSTALL_PATH": "",
	}
	if _, ok := os.LookupEnv("PROTON_CRASH_REPORT_DIR"); !ok {
		vars["PROTON_CRASH_REPORT_DIR"] = filepath.Join(os.TempDir(), "oyna_crashreports")
	}
	for key, value := range i.ProtonOptions {
		vars[key] = value
	}
	if isWinetricks {
		vars["WINETRICKS_SUPER_QUIET"] = "1"
		if log := os.Getenv("OYNA_LOG"); log == "1" || log == "debug" {
			vars["WINETRICKS_SUPER_QUIET"] = ""
		}
	}
	if gamescopeSession() {
		vars["LD_PRELOAD"] = ""
	}
	for k, v := range vars {
		if err = os.Setenv(k, v); err != nil {
			return err
		}
	}
	if !isWinetricks {
		cmd, err = containerCommand(tool, rt, verb, cmd)
		if err != nil {
			return err
		}
	}
	cmd = append(cmd, i.Exe)
	cmd = append(cmd, i.Args...)
	if log := os.Getenv("OYNA_LOG"); log == "1" || log == "debug" {
		fmt.Fprintf(os.Stderr, "oyna: launching %q\n", cmd)
	}
	return supervise(cmd, cwd)
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		var exit *launchExit
		if errors.As(err, &exit) {
			os.Exit(exit.code)
		}
		fmt.Fprintln(os.Stderr, "oyna:", err)
		os.Exit(1)
	}
}

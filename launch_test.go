package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0755); err != nil {
		t.Fatal(err)
	}
}

func TestInstalledLaunch(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("upstream refuses root launches")
	}
	base := t.TempDir()
	bin := filepath.Join(base, "oyna")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	tool := filepath.Join(base, ".oyna", "proton", "GE Test")
	rt := filepath.Join(base, ".oyna", "rt", "steamrt3")
	fixture(t, filepath.Join(tool, "toolmanifest.vdf"), `"manifest" {
 "commandline" "/proton %verb%"
 "compatmanager_layer_name" "proton"
 "require_tool_appid" "1628350"
 }`)
	fixture(t, filepath.Join(rt, "toolmanifest.vdf"), `"manifest" {
 "commandline" "/_v2-entry-point --verb=%verb% --"
 "compatmanager_layer_name" "container-runtime"
 }`)
	fixture(t, filepath.Join(rt, "sniper_platform_test", "files", "keep"), "")
	fixture(t, filepath.Join(rt, "pressure-vessel", "bin", "pv-verify"), "#!/bin/sh\nexit 0\n")
	fixture(t, filepath.Join(rt, "umu"), "#!/bin/sh\nset -eu\ntest \"$1\" = --verb=runinprefix\ntest \"$2\" = --\nshift 2\nexec \"$@\"\n")
	fixture(t, filepath.Join(tool, "proton"), `#!/bin/sh
set -eu
test "$1" = runinprefix
test "$2" = "$EXE"
test "$3" = 'two words'
test "$4" = '--help'
test "$5" = ''
test "$#" = 5
test "$SteamAppId" = 42
test "$OYNA_STEAM_GAME_ID" = 123
test "${#OYNA_INVOCATION_ID}" = 32
test "$WINEPREFIX" = "$STEAM_COMPAT_DATA_PATH"
test "$STEAM_COMPAT_MOUNTS" = "$PROTONPATH:$RUNTIMEPATH"
test -d "$WINEPREFIX/pfx/drive_c/users/steamuser"
exit 37
`)
	exe := filepath.Join(base, "some game.exe")
	fixture(t, exe, "")
	prefix := filepath.Join(base, ".oyna", "prefixes", "default")
	config := filepath.Join(base, "config.toml")
	fixture(t, config, fmt.Sprintf("[oyna]\nproton=%q\nprefix=%q\nexe=%q\ngame_id='umu-42'\nlaunch_args=['two words','--help','']\n", tool, prefix, exe))
	for _, args := range [][]string{{"run", exe, "two words", "--help", ""}, {"--config", config}} {
		cmd := exec.Command(bin, args...)
		cmd.Env = []string{"HOME=" + base, "PATH=/usr/bin:/bin", "OYNA_FOLDERS_PATH=" + base, "PROTONPATH=GE Test", "PROTON_VERB=runinprefix", "GAMEID=umu-42", "SteamGameId=123", "OYNA_RUNTIME_UPDATE=0"}
		out, err := cmd.CombinedOutput()
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != 37 {
			t.Fatalf("oyna %q: %v: %s", args, err, out)
		}
	}
}

func TestConfigRequiresOynaTable(t *testing.T) {
	base := t.TempDir()
	exe := filepath.Join(base, "game.exe")
	fixture(t, exe, "")
	config := filepath.Join(base, "config.toml")
	fixture(t, config, fmt.Sprintf("[umu]\nprefix=%q\nproton=%q\nexe=%q\n", base, base, exe))
	if _, err := fromFile(config, base); err == nil || !strings.Contains(err.Error(), "requires [oyna]") {
		t.Fatalf("expected missing [oyna] error, got %v", err)
	}
}

func TestPrefixExistingUser(t *testing.T) {
	dir := t.TempDir()
	fixture(t, filepath.Join(dir, "pfx", "drive_c", "keep"), "save")
	for n := 0; n < 2; n++ {
		if err := preparePrefix(dir); err != nil {
			t.Fatal(err)
		}
	}
	if !regular(filepath.Join(dir, "pfx", "drive_c", "keep")) {
		t.Fatal("existing prefix lost")
	}
}

func TestPrefixRepairsSymlinks(t *testing.T) {
	for _, kind := range []string{"external-directory", "relative-directory", "file", "broken", "self", "absolute-root", "dot"} {
		t.Run(kind, func(t *testing.T) {
			base := t.TempDir()
			dir := filepath.Join(base, "prefix")
			fixture(t, filepath.Join(dir, "drive_c", "keep"), "prefix data")
			target := filepath.Join(base, "target")
			fixture(t, filepath.Join(target, "keep"), "target data")
			pfx := filepath.Join(dir, "pfx")
			link := target
			switch kind {
			case "relative-directory":
				link = "../target"
			case "file":
				link = filepath.Join(target, "keep")
			case "broken":
				link = filepath.Join(base, "missing")
			case "self":
				link = "pfx"
			case "absolute-root":
				link = dir
			case "dot":
				link = "."
			}
			if err := os.Symlink(link, pfx); err != nil {
				t.Fatal(err)
			}
			for n := 0; n < 2; n++ {
				if err := preparePrefix(dir); err != nil {
					t.Fatal(err)
				}
				if got, err := os.Readlink(pfx); err != nil || got != "." {
					t.Fatalf("expected pfx -> ., got %q: %v", got, err)
				}
			}
			for path, want := range map[string]string{filepath.Join(dir, "drive_c", "keep"): "prefix data", filepath.Join(target, "keep"): "target data"} {
				if got, err := os.ReadFile(path); err != nil || string(got) != want {
					t.Fatalf("data changed at %s: %q, %v", path, got, err)
				}
			}
			if _, err := os.Stat(filepath.Join(dir, "drive_c", "users", "steamuser")); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(target, "drive_c")); !os.IsNotExist(err) {
				t.Fatalf("former symlink target was modified: %v", err)
			}
		})
	}
}

func TestMissingRuntime(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("OYNA_FOLDERS_PATH", dir)
	fixture(t, filepath.Join(dir, "toolmanifest.vdf"), `"manifest" { "commandline" "/proton %verb%" "require_tool_appid" "1628350" }`)
	if _, _, _, err := installedCommand(dir, dir, "run"); err == nil {
		t.Fatal("accepted missing runtime")
	}
}

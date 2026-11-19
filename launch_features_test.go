package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"syscall"
	"testing"
	"time"
)

func TestSupervisorHelper(t *testing.T) {
	if os.Getenv("OYNA_SUPERVISOR_HELPER") != "1" {
		return
	}
	err := supervise([]string{"/bin/sh", "-c", os.Getenv("OYNA_SUPERVISOR_SCRIPT")}, "")
	var exit *launchExit
	if errors.As(err, &exit) {
		os.Exit(exit.code)
	}
	if err != nil {
		os.Exit(99)
	}
	os.Exit(0)
}
func TestSupervisorWaitsForOrphan(t *testing.T) {
	dir := t.TempDir()
	script := "(sleep 0.2; echo survived > '" + dir + "/done') & exit 17"
	cmd := exec.CommandContext(testContext(t), os.Args[0], "-test.run=^TestSupervisorHelper$")
	cmd.Env = append(os.Environ(), "OYNA_SUPERVISOR_HELPER=1", "OYNA_SUPERVISOR_SCRIPT="+script)
	out, err := cmd.CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 17 {
		t.Fatalf("%v: %s", err, out)
	}
	if !regular(filepath.Join(dir, "done")) {
		t.Fatal("supervisor exited before orphan finished")
	}
}
func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}
func TestSupervisorForwardsTermination(t *testing.T) {
	dir := t.TempDir()
	ready := filepath.Join(dir, "ready")
	stopped := filepath.Join(dir, "stopped")
	script := "trap 'echo stopped > " + stopped + "; exit 42' TERM; echo ready > " + ready + "; while :; do sleep 1; done"
	cmd := exec.CommandContext(testContext(t), os.Args[0], "-test.run=^TestSupervisorHelper$")
	cmd.Env = append(os.Environ(), "OYNA_SUPERVISOR_HELPER=1", "OYNA_SUPERVISOR_SCRIPT="+script)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cmd.ProcessState == nil {
			for pid := range processTree(cmd.Process.Pid) {
				_ = syscall.Kill(pid, syscall.SIGKILL)
			}
		}
	})
	deadline := time.Now().Add(5 * time.Second)
	for !regular(ready) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !regular(ready) {
		t.Fatal("child did not become ready")
	}
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	err := cmd.Wait()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 42 {
		t.Fatalf("signal result %v", err)
	}
	if !regular(stopped) {
		t.Fatal("child did not receive SIGTERM")
	}
}
func TestWinetricksLaunch(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("unprivileged launch required")
	}
	base := t.TempDir()
	bin := filepath.Join(base, "oyna")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	tool := filepath.Join(base, "proton")
	fixture(t, filepath.Join(tool, "toolmanifest.vdf"), `"manifest" { "commandline" "/proton %verb%" "compatmanager_layer_name" "proton" }`)
	fixture(t, filepath.Join(tool, "proton"), `#!/bin/sh
set -eu
test "$1" = waitforexitandrun
shift
exec "$@"
`)
	fixture(t, filepath.Join(tool, "protonfixes", "winetricks"), `#!/bin/sh
set -eu
test "$1" = -q
test "$2" = vcrun2022
test "$WINETRICKS_SUPER_QUIET" = 1
test "$PWD" = "$PROTONPATH/protonfixes"
printf 'vcrun2022\n' >> "$WINEPREFIX/winetricks.log"
`)
	for n := 0; n < 2; n++ {
		cmd := exec.CommandContext(testContext(t), bin, "winetricks", "vcrun2022")
		cmd.Env = []string{"HOME=" + base, "PATH=/usr/bin:/bin", "PROTONPATH=" + tool}
		out, err := cmd.CombinedOutput()
		if n == 0 && err != nil {
			t.Fatalf("%v: %s", err, out)
		}
		if n == 1 && err == nil {
			t.Fatal("reinstalled dependency verb")
		}
	}
	prefix := filepath.Join(base, "Games", "umu", "umu-default")
	fixture(t, filepath.Join(prefix, "winetricks.log"), "win10\n")
	if err := checkWinetricks([]string{"win10"}, prefix); err != nil {
		t.Fatal(err)
	}
	if err := checkWinetricks([]string{"invalid value"}, prefix); err == nil {
		t.Fatal("accepted invalid verb")
	}
}
func TestContainerReentry(t *testing.T) {
	base := t.TempDir()
	rt := filepath.Join(base, "runtime")
	tool := filepath.Join(base, "tool")
	fixture(t, filepath.Join(rt, "toolmanifest.vdf"), `"manifest" { "commandline" "/_v2-entry-point --verb=%verb% --" "compatmanager_layer_name" "container-runtime" }`)
	fixture(t, filepath.Join(tool, "toolmanifest.vdf"), `"manifest" { "commandline" "/proton %verb%" "compatmanager_layer_name" "proton" }`)
	fixture(t, filepath.Join(rt, "pressure-vessel", "bin", "steam-runtime-launch-client"), "#!/bin/sh\necho --bus-name=com.steampowered.Apptestprefix\n")
	t.Setenv("UMU_CONTAINER_NSENTER", "1")
	t.Setenv("STEAM_COMPAT_APP_ID", "testprefix")
	t.Setenv("STEAM_COMPAT_LAUNCHER_SERVICE", "")
	t.Setenv("PROTON_VERB", "run")
	l, err := readLayer(tool, "run")
	if err != nil {
		t.Fatal(err)
	}
	got, err := containerCommand(l, rt, "run", []string{"wrapped"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(rt, "pressure-vessel", "bin", "steam-runtime-launch-client"), "--bus-name=com.steampowered.Apptestprefix", "--", filepath.Join(tool, "proton"), "runinprefix"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%q != %q", got, want)
	}
	if os.Getenv("STEAM_COMPAT_LAUNCHER_SERVICE") != "proton" || os.Getenv("PROTON_VERB") != "runinprefix" {
		t.Fatal("missing service environment")
	}
}
func TestSteamAssignedID(t *testing.T) {
	for _, key := range []string{"STEAM_COMPAT_TRANSCODED_MEDIA_PATH", "STEAM_COMPAT_MEDIA_PATH", "STEAM_FOSSILIZE_DUMP_PATH", "DXVK_STATE_CACHE_PATH"} {
		t.Setenv(key, "")
	}
	t.Setenv("UMU_STEAM_GAME_ID", "180388626432") // 42 << 32
	if steamAppID() != 42 {
		t.Fatal("lost inherited shortcut ID")
	}
	t.Setenv("STEAM_COMPAT_MEDIA_PATH", "/cache/123/media")
	if steamAppID() != 123 {
		t.Fatal("media path ID did not take precedence")
	}
}

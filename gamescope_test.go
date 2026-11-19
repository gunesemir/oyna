package main

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

func virtualDisplay(t *testing.T) string {
	t.Helper()
	bin, err := exec.LookPath("Xvfb")
	if err != nil {
		t.Skip("Xvfb is required for the X11 integration test")
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	cmd := exec.Command(bin, "-displayfd", "3", "-screen", "0", "800x600x24", "-ac", "-nolisten", "tcp")
	cmd.ExtraFiles = []*os.File{writer}
	cmd.Stderr = os.Stderr
	if err = cmd.Start(); err != nil {
		writer.Close()
		t.Fatal(err)
	}
	writer.Close()
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	ready := make(chan string, 1)
	go func() { line, _ := bufio.NewReader(reader).ReadString('\n'); ready <- strings.TrimSpace(line) }()
	select {
	case number := <-ready:
		if number == "" {
			t.Fatal("Xvfb did not start")
		}
		return ":" + number
	case <-time.After(5 * time.Second):
		t.Fatal("Xvfb startup timed out")
	}
	return ""
}
func TestGamescopeWindowTags(t *testing.T) {
	primaryDisplay, secondaryDisplay := virtualDisplay(t), virtualDisplay(t)
	t.Setenv("OYNA_STEAM_GAME_ID", "180388626432")
	for _, key := range []string{"STEAM_COMPAT_TRANSCODED_MEDIA_PATH", "STEAM_COMPAT_MEDIA_PATH", "STEAM_FOSSILIZE_DUMP_PATH", "DXVK_STATE_CACHE_PATH"} {
		t.Setenv(key, "")
	}
	primary, err := xgb.NewConnDisplay(primaryDisplay)
	if err != nil {
		t.Fatal(err)
	}
	defer primary.Close()
	secondary, err := xgb.NewConnDisplay(secondaryDisplay)
	if err != nil {
		t.Fatal(err)
	}
	defer secondary.Close()
	base, err := atom(primary, "GAMESCOPECTRL_BASELAYER_APPID")
	if err != nil {
		t.Fatal(err)
	}
	value := make([]byte, 4)
	xgb.Put32(value, 42)
	if err = xproto.ChangePropertyChecked(primary, xproto.PropModeReplace, xproto.Setup(primary).DefaultScreen(primary).Root, base, xproto.AtomCardinal, 32, 1, value).Check(); err != nil {
		t.Fatal(err)
	}
	pidAtom, err := atom(secondary, "_NET_WM_PID")
	if err != nil {
		t.Fatal(err)
	}
	gameAtom, err := atom(secondary, "STEAM_GAME")
	if err != nil {
		t.Fatal(err)
	}
	create := func(pid int) xproto.Window {
		t.Helper()
		w, err := xproto.NewWindowId(secondary)
		if err != nil {
			t.Fatal(err)
		}
		screen := xproto.Setup(secondary).DefaultScreen(secondary)
		if err = xproto.CreateWindowChecked(secondary, screen.RootDepth, w, screen.Root, 0, 0, 100, 100, 0, xproto.WindowClassInputOutput, screen.RootVisual, 0, nil).Check(); err != nil {
			t.Fatal(err)
		}
		v := make([]byte, 4)
		xgb.Put32(v, uint32(pid))
		if err = xproto.ChangePropertyChecked(secondary, xproto.PropModeReplace, w, pidAtom, xproto.AtomCardinal, 32, 1, v).Check(); err != nil {
			t.Fatal(err)
		}
		return w
	}
	first, foreign := create(os.Getpid()), create(99999999)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- watchGamescopeDisplays(ctx, primaryDisplay, secondaryDisplay) }()
	awaitTag := func(w xproto.Window) {
		t.Helper()
		deadline := time.Now().Add(4 * time.Second)
		for time.Now().Before(deadline) {
			v, err := windowCardinal(secondary, w, gameAtom)
			if err == nil && len(v) == 4 && xgb.Get32(v) == 42 {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatal("game window was not assigned its Steam ID")
	}
	awaitTag(first)
	awaitTag(create(os.Getpid()))
	if v, _ := windowCardinal(secondary, foreign, gameAtom); len(v) > 0 {
		t.Fatal("tagged unrelated application's window")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("window monitor did not stop")
	}
}

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

func gamescopeSession() bool {
	return os.Getenv("XDG_CURRENT_DESKTOP") == "gamescope" || os.Getenv("XDG_SESSION_DESKTOP") == "gamescope"
}
func steamAppID() uint32 {
	for _, item := range []struct {
		key string
		up  int
	}{{"STEAM_COMPAT_TRANSCODED_MEDIA_PATH", 0}, {"STEAM_COMPAT_MEDIA_PATH", 1}, {"STEAM_FOSSILIZE_DUMP_PATH", 2}, {"DXVK_STATE_CACHE_PATH", 1}} {
		path := os.Getenv(item.key)
		if path == "" {
			continue
		}
		path = filepath.Clean(path)
		for n := 0; n < item.up; n++ {
			path = filepath.Dir(path)
		}
		if id, err := strconv.ParseUint(filepath.Base(path), 10, 32); err == nil {
			return uint32(id)
		}
	}
	id, _ := strconv.ParseUint(os.Getenv("UMU_STEAM_GAME_ID"), 10, 64)
	return uint32(id >> 32)
}
func atom(c *xgb.Conn, name string) (xproto.Atom, error) {
	r, err := xproto.InternAtom(c, false, uint16(len(name)), name).Reply()
	if err != nil {
		return 0, err
	}
	return r.Atom, nil
}
func windowCardinal(c *xgb.Conn, w xproto.Window, a xproto.Atom) ([]byte, error) {
	r, err := xproto.GetProperty(c, false, w, a, xproto.AtomCardinal, 0, 1024).Reply()
	if err != nil {
		return nil, err
	}
	if r.Format != 32 {
		return nil, nil
	}
	return r.Value, nil
}
func monitorGamescope(ctx context.Context) {
	if !gamescopeSession() || os.Getenv("STEAM_MULTIPLE_XWAYLANDS") != "1" || os.Getenv("PROTON_VERB") != "waitforexitandrun" {
		return
	}
	if err := watchGamescope(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "oyna: Gamescope window monitoring unavailable: %v\n", err)
	}
}
func watchGamescope(ctx context.Context) error { return watchGamescopeDisplays(ctx, ":0", ":1") }
func watchGamescopeDisplays(ctx context.Context, primaryDisplay, secondaryDisplay string) error {
	primary, err := xgb.NewConnDisplay(primaryDisplay)
	if err != nil {
		return err
	}
	defer primary.Close()
	baselayer, err := atom(primary, "GAMESCOPECTRL_BASELAYER_APPID")
	if err != nil {
		return err
	}
	seq, err := windowCardinal(primary, xproto.Setup(primary).DefaultScreen(primary).Root, baselayer)
	if err != nil {
		return err
	}
	if len(seq) == 0 {
		return nil
	}
	secondary, err := xgb.NewConnDisplay(secondaryDisplay)
	if err != nil {
		return err
	}
	defer secondary.Close()
	pidAtom, err := atom(secondary, "_NET_WM_PID")
	if err != nil {
		return err
	}
	gameAtom, err := atom(secondary, "STEAM_GAME")
	if err != nil {
		return err
	}
	appID := steamAppID()
	if appID == 0 {
		return fmt.Errorf("Steam-assigned app ID is unavailable")
	}
	value := make([]byte, 4)
	xgb.Put32(value, appID)
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
		pids := processTree(os.Getpid())
		root := xproto.Setup(secondary).DefaultScreen(secondary).Root
		tree, err := xproto.QueryTree(secondary, root).Reply()
		if err != nil {
			return err
		}
		for _, window := range tree.Children {
			prop, err := windowCardinal(secondary, window, pidAtom)
			if err != nil || len(prop) < 4 {
				continue
			}
			pid := int(xgb.Get32(prop))
			if !pids[pid] && !invocationPID(pid) {
				continue
			}
			current, _ := windowCardinal(secondary, window, gameAtom)
			if len(current) == 4 && xgb.Get32(current) == appID {
				continue
			}
			_ = xproto.ChangePropertyChecked(secondary, xproto.PropModeReplace, window, gameAtom, xproto.AtomCardinal, 32, 1, value).Check()
		}
	}
}
func invocationPID(pid int) bool {
	id := os.Getenv("UMU_INVOCATION_ID")
	if id == "" {
		return false
	}
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/environ", pid))
	if err != nil {
		return false
	}
	for _, s := range strings.Split(string(data), "\x00") {
		if s == "UMU_INVOCATION_ID="+id {
			return true
		}
	}
	return false
}

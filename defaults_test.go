package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDefaultProtonDoesNotFallBackToUMU(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OYNA_HTTP_RETRIES", "0")
	umu := filepath.Join(home, ".oyna", "proton", "UMU-Proton9-1")
	fixture(t, filepath.Join(umu, "toolmanifest.vdf"), `"manifest" { "commandline" "/proton %verb%" }`)
	fixture(t, filepath.Join(umu, "proton"), "#!/bin/sh\nexit 0\n")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "offline", http.StatusServiceUnavailable) }))
	defer server.Close()
	p, err := newProvisioner(home)
	if err != nil {
		t.Fatal(err)
	}
	p.api = server.URL
	if path, err := p.proton(""); err == nil {
		t.Fatalf("default selected %s despite no GE-Proton installation", path)
	}
	if path, err := p.proton("UMU-Proton"); err != nil || path != umu {
		t.Fatalf("explicit UMU selection: %s, %v", path, err)
	}
}

func TestResolveManagedAndLegacyTools(t *testing.T) {
	home := t.TempDir()
	legacy := filepath.Join(home, "legacy")
	t.Setenv("OYNA_FOLDERS_PATH", legacy)
	managed := filepath.Join(home, ".oyna", "proton", "GE-Test")
	old := filepath.Join(legacy, "Steam", "compatibilitytools.d", "GE-Test")
	fixture(t, filepath.Join(old, "proton"), "")
	if path, err := resolveTool(home, "GE-Test"); err != nil || path != old {
		t.Fatalf("legacy lookup: %s, %v", path, err)
	}
	fixture(t, filepath.Join(managed, "proton"), "")
	if path, err := resolveTool(home, "GE-Test"); err != nil || path != managed {
		t.Fatalf("managed lookup: %s, %v", path, err)
	}
	if path, err := resolveTool(home, old); err != nil || path != old {
		t.Fatalf("explicit legacy path: %s, %v", path, err)
	}
}

func TestLaunchDefaultPrefixAndGameIDs(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("unprivileged launch required")
	}
	home := t.TempDir()
	bin := filepath.Join(home, "oyna")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	tool := filepath.Join(home, ".oyna", "proton", "GE-Test")
	fixture(t, filepath.Join(tool, "toolmanifest.vdf"), `"manifest" { "commandline" "/proton %verb%" "compatmanager_layer_name" "proton" }`)
	fixture(t, filepath.Join(tool, "proton"), `#!/bin/sh
set -eu
test "$GAMEID" = "$EXPECTED_GAMEID"
test "$UMU_ID" = "$EXPECTED_UMU_ID"
test "$SteamAppId" = "$EXPECTED_STEAM_ID"
test "$SteamGameId" = "$EXPECTED_STEAM_ID"
test "$WINEPREFIX" = "$EXPECTED_PREFIX"
test "$STEAM_COMPAT_DATA_PATH" = "$EXPECTED_PREFIX"
test -d "$WINEPREFIX/pfx/drive_c/users/steamuser"
exit 27
`)
	for _, tc := range []struct{ name, gameID, umuID, steamID, prefix string }{
		{name: "default", umuID: "umu-default", steamID: "default"},
		{name: "custom-id", gameID: "oyna-default", umuID: "oyna-default", steamID: "0"},
		{name: "explicit-default", gameID: "umu-default", umuID: "umu-default", steamID: "default"},
		{name: "game-specific-id", gameID: "umu-42", umuID: "umu-42", steamID: "42"},
		{name: "custom-prefix", gameID: "umu-42", umuID: "umu-42", steamID: "42", prefix: filepath.Join(home, "custom-prefix")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			expectedPrefix := filepath.Join(home, ".oyna", "prefixes", "default")
			if tc.prefix != "" {
				expectedPrefix = tc.prefix
			}
			expectedGameID := tc.gameID
			if expectedGameID == "" {
				expectedGameID = "umu-default"
			}
			cmd := exec.Command(bin, "createprefix")
			cmd.Env = []string{"HOME=" + home, "PATH=/usr/bin:/bin", "PROTONPATH=GE-Test", "OYNA_LOG=debug", "STORE=gog", "GAMEID=" + tc.gameID, "EXPECTED_GAMEID=" + expectedGameID, "EXPECTED_UMU_ID=" + tc.umuID, "EXPECTED_STEAM_ID=" + tc.steamID, "EXPECTED_PREFIX=" + expectedPrefix}
			if tc.prefix != "" {
				cmd.Env = append(cmd.Env, "WINEPREFIX="+tc.prefix)
			}
			out, err := cmd.CombinedOutput()
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 27 {
				t.Fatalf("launch: %v: %s", err, out)
			}
			if !strings.Contains(string(out), "oyna: launching") {
				t.Fatalf("OYNA_LOG did not enable launch diagnostics: %s", out)
			}
		})
	}
}

func TestDownloadOptionNames(t *testing.T) {
	for _, name := range []string{"OYNA_HTTP_RETRIES", "OYNA_HTTP_TIMEOUT"} {
		t.Setenv(name, "")
		if err := os.Unsetenv(name); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("UMU_HTTP_RETRIES", "invalid")
	t.Setenv("UMU_HTTP_TIMEOUT", "invalid")
	d, err := newDownloads(t.TempDir())
	if err != nil {
		t.Fatalf("obsolete option names affected downloads: %v", err)
	}
	transport := d.client.Transport.(*http.Transport)
	t.Cleanup(transport.CloseIdleConnections)
	if d.retries != 3 || transport.ResponseHeaderTimeout != 5*time.Second {
		t.Fatal("unexpected download defaults")
	}
	t.Setenv("OYNA_HTTP_RETRIES", "0")
	t.Setenv("OYNA_HTTP_TIMEOUT", "7")
	d, err = newDownloads(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	transport = d.client.Transport.(*http.Transport)
	t.Cleanup(transport.CloseIdleConnections)
	if d.retries != 0 || transport.ResponseHeaderTimeout != 7*time.Second {
		t.Fatal("OYNA download options were ignored")
	}
}

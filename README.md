# oyna

A Linux launcher written in Go with the goal of replacing umu-run. The first milestone launches a locally installed compatibility tool and its Steam Linux Runtime. Proton, pressure-vessel and game files are external to the launcher binary.

## Build and run

Use Go 1.23 or newer on Linux. Run the launcher as your normal user.

```sh
CGO_ENABLED=0 go build -o oyna .
go test ./...
PROTONPATH="/path/to/GE-Proton" GAMEID=umu-42 ./oyna "/path/to/game.exe" -fullscreen
```

An installed tool can also be selected by directory name under `$XDG_DATA_HOME/Steam/compatibilitytools.d`. Its manifest identifies the runtime expected under `$XDG_DATA_HOME/umu` (XDG data defaults to `~/.local/share`). For example, app ID 1628350 selects `steamrt3`. A tool with no runtime dependency runs on the host.

Use `WINEPREFIX` to select an existing or new prefix. The default is `~/Games/umu/$GAMEID` with `GAMEID=umu-default`, or `~/Games/$STORE` when a store is supplied. Pass `createprefix` or an empty executable argument for prefix creation. The CLI accepts a leading Proton verb or an upstream-style `--config FILE` with a TOML `[umu]` table.

## Upstream behavior and milestones

The reference is [umu-launcher at e2b203a](https://github.com/Open-Wine-Components/umu-launcher/tree/e2b203a1fdd2af9f35166f5713cb3f85d72587e2). Its CLI and TOML reader select the tool and prefix. Its Proton and runtime installers acquire and update those components. The launch path reads Valve manifests, creates the prefix layout, exports Steam and protonfixes variables, and runs the resulting argument vector. Proton handles Windows execution and game fixes.

1. **Installed-tool launch (implemented).** Resolve tool and runtime manifests, substitute the Proton verb, prepare prefixes under a file lock, and forward arguments and exit status. The integration test exercises CLI and TOML launches with instrumented runtime and Proton entry points.
2. **Provision a clean machine.** Resolve default and named Proton releases, verify and install downloads, handle interrupted or concurrent installs, update runtimes, and reuse validated offline installations.
3. **Additional launch modes.** Implement Winetricks, native runtime passthrough, and launcher-service container re-entry.
4. **Process and desktop integration.** Match supervision and signal handling, game-drive mounts, Gamescope and Flatpak behavior. Compare against upstream using real Proton, persistent prefixes and Windows programs before declaring drop-in compatibility.

Current prefix handling preserves existing directories and reports unexpected pfx symlink targets. An existing Steam library or umu installation is useful for testing, and the launcher can create its runtime shim if needed.

# oyna

A Linux launcher written in Go, aiming for umu-run compatibility. The launcher builds as one static binary. Proton and the Steam Linux Runtime are installed separately by the launcher when needed.

## Build and run

Use Go 1.23 or newer on Linux. Run oyna as your normal user.

```sh
CGO_ENABLED=0 go build -trimpath -o oyna .
./oyna /path/to/game.exe -fullscreen
GAMEID=umu-42 PROTONPATH=GE-Proton ./oyna /path/to/game.exe
WINEPREFIX=~/Games/example ./oyna createprefix
```

An unset `PROTONPATH` selects `UMU-Latest`. `UMU-Proton`, `GE-Proton`, `UMU-Latest`, and `GE-Latest` select the latest release of their respective Proton family. On ARM64, release selection requires an ARM64 asset. An absolute Proton path or an installed directory name under `Steam/compatibilitytools.d` selects a specific installation.

Proton is downloaded from its official GitHub release and checked against the published SHA512 or SHA256 checksum. Its `toolmanifest.vdf` determines the Steam runtime. Runtime downloads are checked against Valve's SHA256 manifest, and their platform files are validated with the bundled `pv-verify` before activation and on subsequent launches.

Managed installs use `$XDG_DATA_HOME` (default `~/.local/share`). Proton versions live under `Steam/compatibilitytools.d`, latest aliases under `umu/compatibilitytools`, and runtimes under `umu/steamrt*`. `UMU_FOLDERS_PATH` overrides that base. In Flatpak, `HOST_XDG_DATA_HOME` supplies the default base. Download caches use `$XDG_CACHE_HOME/umu` (default `~/.cache/umu`).

The default Wine prefix is `~/Games/umu/$GAMEID`, with `GAMEID=umu-default`, or `~/Games/$STORE` when a store is supplied. `WINEPREFIX` overrides it. Use `createprefix` or an empty executable argument to initialize a prefix. Existing prefix directories are preserved, and unexpected `pfx` symlink targets produce an error.

## Updates and recovery

Alias launches check for Proton updates. Required runtimes also check for updates. Interrupted archive downloads resume using HTTP ranges. Install and cache locks coordinate concurrent launches, and validated staging directories are activated atomically. Offline launches can fall back to an existing Proton build and a successfully validated runtime. A damaged runtime must be repaired before launch.

Set `UMU_RUNTIME_UPDATE=0` to skip runtime updates when the installed runtime passes validation. Selecting a specific Proton path pins that tool. `UMU_HTTP_TIMEOUT` controls connection and socket read timeouts in seconds (default 5, zero disables them), and `UMU_HTTP_RETRIES` sets retries after the initial request (default 3).

Updates currently download complete archives. Previous versioned Proton builds and verified cached archives are retained for reuse and existing launches. Optional upstream delta patches are a future optimization.

## Launch integrations

```sh
PROTONPATH=GE-Proton ./oyna winetricks vcrun2022
UMU_CONTAINER_NSENTER=1 ./oyna /path/to/game.exe
```

Winetricks uses Proton's bundled script when available, runs from `protonfixes`, and supplies `-q`. Dependency verbs already recorded in `winetricks.log` are rejected, while repeatable settings such as `win10` remain usable. `UMU_LOG=1` or `debug` enables launch diagnostics and disables Winetricks' extra quiet setting. Custom verb files are supported.

Container re-entry looks for the prefix's existing launcher-service bus through the runtime's `steam-runtime-launch-client`. A matching service receives the unwrapped tool command with `runinprefix`. If the service is absent after five checks, the regular launch creates a new container service.

In Gamescope sessions with `STEAM_MULTIPLE_XWAYLANDS=1`, the default `waitforexitandrun` launch monitors X11 displays `:0` and `:1`. It assigns `STEAM_GAME` to game windows using Steam's inherited application ID and process ownership. X11 failures are reported and game execution continues. The runtime shim supplies `DISPLAY=:1` when it is missing, and the launch environment clears `LD_PRELOAD` in Gamescope sessions.

The launcher starts a separate process session, becomes a Linux child subreaper, forwards INT/TERM/HUP to descendants, reaps orphaned children, and waits for descendants to finish. It returns the main launch process's exit code. Signal deaths use `128 + signal`.

## Configuration files

```toml
[umu]
prefix = "~/Games/example"
proton = "~/.local/share/Steam/compatibilitytools.d/GE-Proton-example"
exe = "~/Games/example/game.exe"
game_id = "umu-42"
store = "gog"
launch_args = ["-fullscreen"]
```

Run `oyna --config config.toml`. As in upstream's config mode, `prefix`, `proton`, and `exe` must identify existing directories or files. Paths are relative to the calling directory. A leading Proton verb and the `PROTON_VERB` environment variable are supported in command-line mode.

## Validation and compatibility

```sh
go test ./...
go vet ./...
```

Tests exercise installation and updates through local HTTP servers, SHA512/SHA256 verification, resumed downloads, concurrent installs, offline fallback, runtime repair, archive traversal rejection, CLI/TOML launches, Winetricks, container re-entry, signal forwarding, and orphan reaping. The X11 integration test starts two virtual displays and verifies new game windows receive their Steam ID while unrelated windows remain untouched. It runs when `Xvfb` is on `PATH`.

Live upstream downloads and a real Windows game launch remain unverified in the development cloud environment, whose network proxy returned HTTP 403 for the GitHub release API and Steam runtime repository. Gamescope property handling has been tested with Xvfb. Steam Deck and Flatpak sessions require hardware/session validation.

Native runtime passthrough (`UMU_NO_PROTON=1`), automatic game-drive mount detection, and complete edge-case parity remain outside this milestone. Full drop-in compatibility is still in progress.

The upstream behavior reference is [umu-launcher at e2b203a](https://github.com/Open-Wine-Components/umu-launcher/tree/e2b203a1fdd2af9f35166f5713cb3f85d72587e2).

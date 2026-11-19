# oyna

Oyna launches Windows games on Linux through Proton, without adding them to Steam. It's written in Go and builds into a single static binary. On a fresh setup, it downloads GE-Proton and the Steam Linux Runtime needed to run it.

The project follows umu-run's launch behavior, with its own defaults, file locations, and configuration names. Compatibility work is still ongoing.

## Build and run

You'll need Linux and Go 1.23 or newer to build oyna. Run it as your regular user.

```sh
CGO_ENABLED=0 go build -trimpath -o oyna .
./oyna "/path/to/game.exe" -fullscreen
```

The first launch downloads the required components. Later launches check for updates. You can choose another Proton build or give a game its own Wine prefix.

```sh
PROTONPATH="/path/to/GE-Proton" ./oyna "/path/to/game.exe"
WINEPREFIX="$HOME/Games/my-game" ./oyna "/path/to/game.exe"
```

To create a prefix before installing a game, run `./oyna createprefix`. Passing an empty executable argument also initializes a prefix.

## Choosing Proton and game fixes

With `PROTONPATH` unset, oyna selects the latest GE-Proton release. If a download fails, it can use an installed GE-Proton build. You can also select `UMU-Proton`, `GE-Latest`, or `UMU-Latest` explicitly.

For a specific installed build, set `PROTONPATH` to its absolute path or its directory name under `~/.oyna/proton`. ARM64 hosts require an ARM64 release asset.

`GAMEID` and `STORE` tell Proton's protonfixes which game and store you're using.

```sh
GAMEID=umu-42 STORE=gog ./oyna "/path/to/game.exe"
```

The default `GAMEID` is `umu-default`. Game IDs pass directly to `UMU_ID`, the variable Proton and protonfixes read. The default produces `SteamAppId=default`.

You can pass a Proton verb before the executable or set `PROTON_VERB`. The default verb is `waitforexitandrun`.

## Where files go

Oyna keeps its managed files under `$HOME/.oyna`.

| Content | Location |
| --- | --- |
| Proton versions and latest release selections | `~/.oyna/proton` |
| Steam runtimes | `~/.oyna/rt/<runtime>` |
| Download cache | `~/.oyna/.cache` |
| Default Wine prefix | `~/.oyna/prefixes/default` |

**The default prefix is shared across games**, even when you set `GAMEID` or `STORE`. Set `WINEPREFIX` whenever you want a separate prefix for a game.

You can use an existing prefix by setting `WINEPREFIX` to its path. Existing `pfx` directories are used directly. If `pfx` is a symlink, prefix setup replaces it with `pfx -> .`, including when the old link is broken. The former target's contents stay where they are.

Named Proton lookup also searches existing Steam and umu compatibility directories. Those lookup locations use `XDG_DATA_HOME`, or `HOST_XDG_DATA_HOME` in Flatpak. `OYNA_FOLDERS_PATH` supplies an alternate base for the existing `Steam/compatibilitytools.d` and `umu/compatibilitytools` layouts. Managed downloads and caches use the paths in the table above.

Files from an older installation can be selected by explicit path or migrated manually. A Flatpak installation needs permission to access the managed home directory.

## Updates and recovery

When you select a release such as `GE-Proton`, oyna checks for a newer version. Selecting a specific installed Proton path pins that build. Required Steam runtimes also receive update checks.

Downloads are checked against the published SHA512 or SHA256 checksums. Oyna uses each tool's `toolmanifest.vdf` to choose its runtime, then runs the runtime's bundled `pv-verify` to check its platform files before installation and on later launches.

Interrupted downloads can resume. Installation locks coordinate simultaneous launches, and a replacement installation takes over only after validation succeeds. If an update fails, an existing Proton build and a validated runtime can still be used. A damaged runtime needs repair before launch.

| Option | Effect |
| --- | --- |
| `OYNA_RUNTIME_UPDATE=0` | Skip runtime updates when the installed runtime passes validation |
| `OYNA_HTTP_TIMEOUT` | Connection and socket read timeout in seconds, default `5`. Set `0` to disable |
| `OYNA_HTTP_RETRIES` | Retries after the first request, default `3` |

Updates currently download complete archives. Previous Proton versions and verified cached archives are retained for reuse and running games. Delta updates are still pending.

## Winetricks and desktop integration

You can install Windows dependencies through Proton's bundled Winetricks.

```sh
./oyna winetricks vcrun2022
```

Oyna adds `-q` and runs Winetricks from Proton's `protonfixes` directory. It checks `winetricks.log` to catch dependency verbs that are already installed. Settings such as `win10` can be applied repeatedly, and custom verb files are supported.

Set `OYNA_LOG=1` or `OYNA_LOG=debug` to see the launch command and disable Winetricks' extra quiet setting.

To launch another command inside a prefix's running container, enable container re-entry.

```sh
OYNA_CONTAINER_NSENTER=1 ./oyna "/path/to/game.exe"
```

Oyna looks for the existing launcher service up to five times. When it finds one, it launches through `steam-runtime-launch-client` using `runinprefix`. Otherwise, it starts the regular container launch with the service enabled.

In Gamescope sessions with `STEAM_MULTIPLE_XWAYLANDS=1`, oyna tracks game windows on displays `:0` and `:1` and assigns their Steam application ID. This runs with the default `waitforexitandrun` verb. The launcher supplies `DISPLAY=:1` when needed, clears `LD_PRELOAD` for Gamescope, and reports X11 failures while allowing the game to continue.

Oyna forwards INT, TERM, and HUP signals to the game's process tree. It runs as a Linux child subreaper, cleans up exited children, and waits for helper processes to finish. The main launch process determines the exit code. Signal deaths return `128 + signal`.

## Configuration files

If you'd rather save a game's settings in a file, use a TOML config.

```toml
[oyna]
prefix = "~/Games/my-game"
proton = "~/.oyna/proton/GE-Proton-example"
exe = "~/Games/my-game/game.exe"
game_id = "umu-42"
store = "gog"
launch_args = ["-fullscreen"]
```

```sh
./oyna --config config.toml
```

The `[oyna]` table and the `prefix`, `proton`, and `exe` fields are required. In config mode, the prefix and Proton directories and the executable file must already exist. Replace the example paths with your own. Relative paths are resolved from the directory where you run oyna.

`game_id`, `store`, and `launch_args` are optional. Launch arguments can be an array of strings or a string split on spaces.

When moving a config from umu-run, change its `[umu]` header to `[oyna]`. Launcher options use the `OYNA_*` names shown here. Upstream tool names and Proton-facing variables such as `UMU_ID` keep their required spelling.

## Development and current limitations

```sh
go vet ./...
```

Real upstream downloads and Windows game launches still need validation. The development environment returned HTTP 403 for the GitHub release API and Steam runtime repository. Gamescope window handling has been tested with Xvfb. Steam Deck and Flatpak sessions need testing on those systems.

Native runtime passthrough (`OYNA_NO_PROTON=1`) and automatic game-drive mount detection are still pending. Further compatibility differences include nested tool chains and updates on filesystems without atomic directory-exchange support.

The upstream reference used during development is [umu-launcher at e2b203a](https://github.com/Open-Wine-Components/umu-launcher/tree/e2b203a1fdd2af9f35166f5713cb3f85d72587e2).

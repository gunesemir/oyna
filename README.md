# Oyna

Oyna launches native Linux games and runs Windows games through Proton. It downloads GE-Proton and the Steam Linux Runtime as needed and stores them under `~/.oyna`.

## Install

Download the `.tar.xz` archive for your system from the [latest release](https://github.com/gunesemir/oyna/releases/latest). Extract it and run Oyna as your regular user. For example, on x86-64:

```sh
tar -xJf oyna-v0.1-linux-amd64.tar.xz
./oyna --version
```

To build from source, run `CGO_ENABLED=0 go build -o oyna .` with Go 1.23 or newer.

## Launch a game

```sh
./oyna "/path/to/game.exe" -fullscreen
```

Oyna uses `~/.oyna/prefix/default` as its Wine prefix. Set `WINEPREFIX` to give a game its own prefix, and set `PROTONPATH` to select an installed Proton version.

```sh
WINEPREFIX="$HOME/.oyna/prefixes/my-game" PROTON_LOG=1 ./oyna "/path/to/game.exe"
```

Run `./oyna run winecfg.exe` to open Wine Configuration. Set `WINEPREFIX` and `PROTONPATH` as you would when launching the game.

For a native game, use `OYNA_NO_PROTON=1 ./oyna "/path/to/start.sh"` or save a native config as shown below.

## Save per-game settings

Create a `game.conf` file with one `key=value` entry per line. Oyna uses Proton by default. A blank or omitted `WINEPREFIX` uses `~/.oyna/prefix/default`. A blank or omitted `PROTONPATH` selects GE-Proton. You can override either independently with an existing directory.

```ini
exe=/path/to/My Game/game.exe
WINEPREFIX=~/.oyna/prefixes/my-game
PROTONPATH=/path/to/GE-Proton
arg=PROTON_LOG=1 %command% -fullscreen
```

Launch it with `./oyna --config game.conf`. `GAMEID`, `STORE`, and `arg` are optional.

### Native Linux games

Set `OYNA_NO_PROTON=1` in the game config or run `OYNA_NO_PROTON=1 ./oyna --config game.conf`. Oyna launches the game through the host's `/bin/sh` from the game's directory. Executables run with their own interpreter or as native binaries. A readable POSIX `.sh` script can run without the execute bit.

```ini
exe=/games/My Game/start.sh
OYNA_NO_PROTON=1
arg=gamemoderun %command% -fullscreen
```

Add `runtime=steamrt3` to run the game inside the Steam Linux Runtime. Oyna installs and verifies the selected runtime on demand. Supported names are `steamrt2`, `steamrt3`, `steamrt4`, and `steamrt4-arm64` for ARM64.

### Launch options and wrappers

Use `arg=-fullscreen` for plain game arguments. To use Steam-style launch options, put environment assignments first, any wrappers and their flags next, then `%command%` and the game arguments.

```ini
arg=PROTON_LOG=1 gamescope -f -- gamemoderun %command% -fullscreen
```

Oyna expands `%command%` to the native or Proton launch, including the Steam Linux Runtime when selected. Install any wrappers you use and make them available on `PATH`. Put the whole wrapper chain on one `arg=` line. You can add other `arg=` lines, and game arguments are passed in file order.

Quote values and arguments that contain spaces. Oyna uses shell-style quoting and runs the command directly. Values such as `$HOME` are passed literally, so use an absolute path for environment variables that take a path. Use the dedicated config fields for Oyna settings such as `WINEPREFIX` and `PROTONPATH`.

You can also put a Proton option on its own line, such as `PROTON_LOG=1`. The options available depend on the Proton build you select. [Valve Proton options](https://github.com/ValveSoftware/Proton#runtime-config-options) · [GE-Proton options](https://github.com/GloriousEggroll/proton-ge-custom#options)

Ludus requires `oyna` on your `PATH`. It launches games through Oyna using the game configs in `~/.oyna/`, so the same `arg=` syntax works there. [Ludus](https://github.com/gunesemir/ludus)

Licensed under [0BSD](LICENSE).

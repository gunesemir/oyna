# Oyna

Oyna launches Windows games on Linux through Proton. On first use, it downloads GE-Proton and the Steam Linux Runtime as needed and stores them under `~/.oyna`.

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

Oyna uses `~/.oyna/prefixes/default` as its Wine prefix. Set `WINEPREFIX` to give a game its own prefix, and set `PROTONPATH` to select an installed Proton version.

```sh
WINEPREFIX="$HOME/.oyna/prefixes/my-game" PROTON_LOG=1 ./oyna "/path/to/game.exe"
```

Run `./oyna run winecfg.exe` to open Wine Configuration. Set `WINEPREFIX` and `PROTONPATH` as you would when launching the game.

## Save per-game settings

Create a `game.conf` file with one `key=value` entry per line. The executable and both directories must already exist.

```ini
exe=/path/to/My Game/game.exe
WINEPREFIX=~/.oyna/prefixes/my-game
PROTONPATH=/path/to/GE-Proton
arg=PROTON_LOG=1 %command% -fullscreen
```

Launch it with `./oyna --config game.conf`. `GAMEID`, `STORE`, and `arg` are optional.

Use `arg=-fullscreen` for plain game arguments. With Steam-style options, put environment assignments before `%command%` and game arguments after it. Quote any argument that contains spaces. You can also put a Proton option on its own line, such as `PROTON_LOG=1`. The options available depend on the Proton build you select. [Valve Proton options](https://github.com/ValveSoftware/Proton#runtime-config-options) · [GE-Proton options](https://github.com/GloriousEggroll/proton-ge-custom#options)

For [Ludus](https://github.com/gunesemir/ludus), add `oyna` to your `PATH` and set `runner=oyna` in the game entry.

Licensed under [0BSD](LICENSE).

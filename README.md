# oyna

Oyna is a small Linux launcher for Windows games. It runs games through Proton and downloads GE-Proton and the Steam Linux Runtime as needed. It's a work in progress.

## Run a game

Build with Go 1.23 or newer. Run oyna as your regular user.

```sh
CGO_ENABLED=0 go build -o oyna .
./oyna "/path/to/game.exe" -fullscreen
```

Oyna keeps its downloads under `~/.oyna`. Games share `~/.oyna/prefixes/default` unless you set `WINEPREFIX`. Set `PROTONPATH` to use a specific Proton installation.

```sh
WINEPREFIX="$HOME/.oyna/prefixes/my-game" ./oyna "/path/to/game.exe"
```

Set Proton options as environment variables when launching from a shell.

```sh
PROTON_USE_WINED3D=1 PROTON_LOG=1 ./oyna "/path/to/game.exe" -fullscreen
```

If you use [Ludus](https://github.com/gunesemir/ludus), put `oyna` on your `PATH` and set `runner=oyna` in the game entry.

## Config files

For standalone use, save your settings in a `game.conf` file with one `key=value` pair per line.

```text
exe=/path/to/My Game/game.exe
WINEPREFIX=~/.oyna/prefixes/my-game
PROTONPATH=/path/to/GE-Proton
GAMEID=umu-42
STORE=gog
PROTON_LOG=1
arg=-novid -nojoy -fullscreen
```

Run it with `./oyna --config game.conf`. The executable, prefix, and Proton paths must already exist. `GAMEID`, `STORE`, and `arg` are optional. List game arguments on the `arg=` line and quote any argument that contains spaces.

An `arg=` line can also contain Steam-style Proton options.

```text
arg=PROTON_USE_WINED3D=1 PROTON_LOG_DIR="/path/to/game logs" %command% -fullscreen
```

Put environment assignments before `%command%` and game arguments after it. With no `%command%`, every word on the line is a game argument. Oyna uses shell-style quoting to split the line, passes values literally, and appends arguments in file order.

Proton options can also be set on their own lines. Both forms accept `PROTON_*`, `WINE_*`, `DXVK_*`, `VKD3D_*`, `WINEDLLOVERRIDES`, `WINEDEBUG`, `WINEARCH`, `STEAM_COMPAT_CONFIG`, `HOST_LC_ALL`, and `FNA3D_FORCE_DRIVER`. Oyna uses `PROTON_VERB` from the shell environment to select the launch verb. For repeated options, the last value in the file wins and overrides the shell environment. The selected Proton build determines which options take effect. See the [Proton runtime options](https://github.com/ValveSoftware/Proton#runtime-config-options) and [GE-Proton options](https://github.com/GloriousEggroll/proton-ge-custom#options).

Licensed under [0BSD](LICENSE).

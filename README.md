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

If you use [Ludus](https://github.com/gunesemir/ludus), put `oyna` on your `PATH` and set `runner=oyna` in the game entry.

## Config files

For standalone use, save your settings in a `game.conf` file with one `key=value` pair per line.

```text
exe=/path/to/My Game/game.exe
WINEPREFIX=~/.oyna/prefixes/my-game
PROTONPATH=/path/to/GE-Proton
GAMEID=umu-42
STORE=gog
arg=-novid -nojoy -fullscreen
```

Run it with `./oyna --config game.conf`. The executable, prefix, and Proton paths must already exist. `GAMEID`, `STORE`, and `arg` are optional. List game arguments on the `arg=` line and quote any argument that contains spaces.

Licensed under [0BSD](LICENSE).

# Omarchy Steam Plugin

Bar widget for Steam: installed vs owned games, lifetime playtime, and recent titles.

![Steam panel](preview.png)

## Install

```bash
omarchy plugin add https://github.com/sebday/omarchy-plugin-steam.git
omarchy plugin enable evo.steam
```

## Requirements

- Steam installed, with a library under `~/.local/share/Steam` (override with `STEAM_DIR`)
- `steam` on `PATH` for launching games from the panel

Middle-click launches the Steam desktop shortcut the same way the programs menu does. Omarchy already floats that window via `default/hypr/apps/steam.lua` (centered, 1100×700).

Library counts, playtime, and recent titles are read from local Steam files by `bin/steam-status`. The owned-game count needs Steam's binary `appinfo.vdf` and `packageinfo.vdf` caches, which shell tools can't parse reliably, so that part is in Go. Everything else is plain text VDF and app manifests.

```bash
omarchy-shell shell toggle evo.steam '{}'
omarchy-shell evo.steam refresh
```

| Call | Action |
|---|---|
| `open` / `show` | Open the panel |
| `close` / `hide` | Close the panel |
| `toggle` | Toggle the panel |
| `refresh` | Refresh status |

## License

MIT.

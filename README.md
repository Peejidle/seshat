# Cephalon Seshat

*Keeper of the codex, measurer of mastery.*

A terminal companion for tracking your Warframe mastery grind. Seshat lists
every masterable item (Warframes, weapons, companions, Archwing, Necramechs,
K-Drives, amps, kitguns, zaws), and you check off what you own and what
you've mastered.

## TOS-safe by design

Seshat never touches the game client, your account, or any Digital Extremes
API. Item data comes from the community-maintained
[WFCD warframe-items](https://github.com/WFCD/warframe-items) dataset (the
same data that powers warframestat.us), which is regenerated from the
game's public exports within a day or so of each update. All marks are
entered by hand.

## Install

```sh
go install github.com/Peejidle/seshat@latest
```

or from a checkout: `go build -o seshat . && install -Dm755 seshat ~/.local/bin/seshat`

## Use

```
seshat                 open the archive (TUI)
seshat update          sync item data now
seshat stats           print mastery totals
seshat export FILE     back up your records
seshat import FILE     restore records from a backup
```

In the TUI press `?` for every key. The main ones:

| key | does |
|---|---|
| `space` / `m` | toggle mastered (also marks owned) |
| `o` | toggle owned |
| `v` | start a range selection; `space`/`o` then mark the whole range |
| `V` | select everything shown (e.g. search "prime", then `V`, `space`) |
| `H` | hide / show mastered items (remembered between sessions) |
| `tab` / `1`-`0` | change category |
| `/` | search |
| `f` | filter: all, owned but not mastered, mastered, new |
| `s` | sort: name, type, newest release |
| `enter` | details with a large image |
| `w` / `i` | open the wiki page (drop locations) / the full-size image |
| `T` | theme: follow your terminal's colours (default), or Seshat's own |
| `x` | ledger: star chart, steel path & intrinsics XP for the MR estimate |
| `u` | sync item data now |

## How new items show up

Seshat checks upstream every 6 hours when you launch it, or whenever you
press `u`. It uses HTTP ETags, so an unchanged dataset costs almost nothing.
Items that weren't there before are marked `✦` until you quit, and the
`new` filter lists them.

## Mastery estimate

Item XP is max rank × 100 for weapons and amps, or × 200 for Warframes,
companions, Archwings, Necramechs, K-Drives and the Plexus. Kuva, Tenet and
Coda weapons and Necramechs rank to 40. Star chart, Steel Path and
Railjack/Drifter intrinsics XP can't be enumerated, so enter them in the
ledger (`x`) from your in-game profile.

WFCD's `masterable` flag is unreliable for modular gear, so Seshat applies
its own rules: amp prisms, kitgun chambers and K-Drive decks count; other
parts don't. The Plexus is missing from WFCD and is added by hand in
`internal/catalog/catalog.go` (`Supplements`).

## Files

- `~/.local/share/seshat/progress.json`: your marks (back this up, or use `E`)
- `~/.cache/seshat/`: cached item data and icons (safe to delete)

## Theme

By default Seshat uses your terminal's 16 ANSI colours, so it follows your
terminal theme, pywal and so on. Press `T` to switch to Seshat's own
lapis-and-gold palette. The choice is remembered.

## Images

Seshat picks the best way your terminal can draw item icons:

- **kitty / Ghostty:** real, full-resolution images via the kitty graphics
  protocol (Unicode placeholders, so they redraw cleanly with the TUI).
- **Everything else** (Alacritty, foot, WezTerm, …): truecolor half-block
  characters. They work anywhere but are soft, since each cell holds only two
  pixels. Inside tmux/screen Seshat also uses this mode.

Override with `SESHAT_IMAGES=kitty|blocks|off`. The details view (`enter`)
shows a large icon, and `i` opens the full-size file in your image viewer.
The side-pane icon appears once the window is about 104×50 or larger.

## License

MIT. See [LICENSE](LICENSE).

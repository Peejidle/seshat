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
seshat import-profile FILE
                       mark mastery from your saved Warframe profile
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
| `f` | filter: all, owned but not mastered, in progress, collecting parts, mastered, new |
| `s` | sort: name, type, newest release |
| `enter` | details with a large image; check off parts there (`j`/`k` pick, `p` have it, `a` all/none) |
| `w` / `i` | open the wiki page (drop locations) / the full-size image |
| `T` | theme: follow your terminal's colours (default), or Seshat's own |
| `x` | ledger: star chart, steel path & intrinsics XP for the MR estimate |
| `u` | sync item data now |
| `P` | import mastery from your saved Warframe profile |

## Collecting parts

Open an item with `enter` and its foundry list becomes a checklist: move with
`j`/`k`, press `p` to mark a blueprint or part as held (parts needed twice,
like Dual Kamas Prime blades, count 0 → 1 → 2), or `a` to fill or clear them
all. Resources such as Orokin Cells are listed but not tracked. Items you're
collecting show `▪2/4` in the list until you mark them owned, and the
`collecting parts` filter (`f`) lists them all — handy before a relic run.

## Importing from your profile

Instead of checking off hundreds of items by hand, you can import mastery
from Warframe's public profile, the same data the in-game profile viewer
shows. **Seshat never contacts DE's servers.** You fetch the profile once,
in your own browser, and Seshat reads the saved file. DE rate-limits that
endpoint hard (bad or repeated requests can block your IP from the game for
a while) and hasn't sanctioned third-party use, so the one request stays
yours.

1. **Find your account ID.** Log in at warframe.com, press F12 and open
   Storage (Firefox) or Application (Chrome) → Cookies →
   `https://www.warframe.com` → `user-info`. Copy the 24-character
   `user_id`. (The old `warframe.com/api/user-data` page and the `EE.log`
   method no longer work.)
2. **Save your profile.** Open this once, with your ID on the end, and save
   the page (`Ctrl+S`; in Firefox, the *Raw Data* tab → *Save*):
   `https://api.warframe.com/cdn/getProfileViewingData.php?playerId=<ID>`
   Consoles use `api-ps4`, `api-xb1`, `api-swi`, `api-mob` or `api-and` in
   place of `api`.
3. **Import it:** press `P` in Seshat (it suggests the newest file in
   `~/Downloads`), or run `seshat import-profile FILE`.

Items at max rank are marked mastered. Items below it, such as a Kuva, Tenet
or Coda weapon at rank 30 of 40 or something you levelled and sold, are
marked in progress (◐) with their rank, and the XP they've earned counts
toward your MR estimate. Railjack/Drifter intrinsics are filled into the
ledger. The import never unmarks anything and leaves *owned*
alone, since the profile also lists items you've sold. Star Chart and Steel
Path XP still go in the ledger by hand.

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

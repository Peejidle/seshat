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
| `enter` | details with a large image; check off parts there (`j`/`k` pick, `p` have it, `a` all/none), or type a number to set its rank |
| `w` / `i` | open the wiki page (drop locations) / the full-size image |
| `T` | theme: follow your terminal's colours (default), or Seshat's own |
| `N` | next up: the easiest items left to master, and where each MR lands |
| `R` | relic finder: relics holding parts you need; there `enter` opens one, `/` looks up any part or relic, `c` counts your relics, `o` lists them |
| `x` | ledger: star chart & steel path XP, intrinsic ranks, for the MR estimate |
| `u` | sync item data now |
| `P` | import mastery from your saved Warframe profile |

## Collecting parts

Open an item with `enter` and its foundry list becomes a checklist: move with
`j`/`k`, press `p` to mark a blueprint or part as held (parts needed twice,
like Dual Kamas Prime blades, count 0 → 1 → 2), or `a` to fill or clear them
all. Weapons a recipe consumes (the two Broncos in an Akbronco) are parts
too. Resources aren't shown: WFCD only has the final recipe's resources,
not each part's, so any total would be wrong. Items you're
collecting show `▪2/4` in the list until you mark them owned, and the
`collecting parts` filter (`f`) lists them all — handy before a relic run.

## Relic finder

Press `R` for every relic that holds a part you still need — parts of
items you haven't built or mastered and haven't checked off — with the
relics holding the most of them first. Each part shows its rarity and its
chance intact → radiant. By default only relics you can farm right now are
listed; `v` adds vaulted ones (for trading or Prime Resurgence).

Press `/` in the finder to look anything up instead: a part (`nyx prime
chassis`, or just `nyx prime` for the whole set) lists every relic that
drops it, vaulted or not, and a relic's name (`axi s8`) lists everything in
it. Parts you still need are marked `●`. `esc` clears the search.

`s` switches between most-needed-first and sorting by name (Lith A1, A2 …
A13), which also shrinks each relic to one line with its needed parts, for
browsing: search `lith a`, press `s`, and every Lith A relic is listed in
order. `enter` on one opens it in full.

### A relic's window

`enter` on a relic opens everything about it: each reward with whether you
still need it (`[ ]` needed, `[◐]` some held, `[✓]` held, `[●]` built,
`[★]` mastered), and the missions that drop it with their rotation and
chance, plus any bounties. You can edit from there too: `+`/`-` or type a
number and `enter` for the count, and `j`/`k` then `p` to check off a part
you just got. `w` opens the relic on the wiki; any other key goes back.

### Counting your relics

Seshat can't see your inventory (that would mean reading the game), so you
count relics in by hand, made as quick as possible:

- Press `c` in the finder and type `tier name count`, as many as you like
  per line, separated by commas: `a s8 12, l n1 4, meso s3 2`. The tier can
  be written out or just its first letter. Enter saves, and the box stays
  open for the next line; `esc` when you're done. If any entry is wrong,
  nothing on that line is saved and the text stays so you can fix it.
- `+2` or `-1` in place of a count adds or removes, for after a run:
  `a s8 -1, l n1 +3`.
- A count on its own sets the selected relic, so `/` to find a relic, then
  `c` and `5`. `+` and `-` (no box) change it by one.
- `o` shows **my relics**: everything you hold, the ones with the most parts
  you need first, every reward listed and the needed ones marked `●`.

Counts don't need to be exact; they only steer the finder. Rather than
logging every relic you open, recount now and then: press `r` and Seshat
walks through your relics in the order the game lists them sorted by name,
so you can go down both lists side by side. Enter keeps a count, a number
replaces it, `0` removes the relic, and `↑` goes back. If the game shows a
relic Seshat's list doesn't have, type it in full (`l b4 3`); it's slotted
into place and the recount carries on after it. `esc` pauses, and `r` later
carries on where you stopped. "My relics" shows how long ago you last
finished one.

Relics you hold are counted (`×12`) wherever they appear, show up in the
finder even when vaulted, and lead the drop line under each part in the
details (`have Axi S8 ×12 11%`). Refinements aren't tracked separately.

The details view also shows, under each missing part, the best farmable
relics for it, or how many vaulted relics hold it.

Vanguard relics, from a one-off event, are included too. They hold
ordinary prime parts but no longer drop, so they're marked `event` and only
show up when you hold some, search for them, or include vaulted relics.
Count them like any other: `v c1 2`.

Relic contents come from [WFCD's mirror](https://drops.warframestat.us) of
DE's published drop tables, synced with the rest of the archive. A relic
counts as farmable when some mission or bounty in those tables drops it.

## Next up: the easiest mastery left

Press `N` for every item you haven't mastered that's easy to get, easiest
first, with the most XP first within each group:

1. **Owned: just level it**, including Kuva, Tenet and Coda weapons stuck
   below rank 40.
2. **Every part in hand**: straight to the foundry.
3. **Market blueprint for credits, then resources.**
4. **Clan dojo research**: blueprints and parts for credits, then
   resources.
5. **Prime with every missing part in a relic you can farm or already
   hold** (counted in the relic finder), fewest parts missing first.
6. **Started: some parts in hand**, whatever the item, with the ones
   missing fewest parts first.
7. **Market blueprint, then parts from missions**, e.g. Warframes whose
   parts drop from a boss.

Each row shows the XP it gives and how much you'd still need for the next
rank once it's mastered, and a line marks where each rank would be reached
if you worked down the list. Credit and platinum prices are shown where
WFCD has them. It follows the category tab (`2` then `N` for Warframes
only), and `enter` opens an item's details and comes back. `tab` steps
through the groups one at a time, skipping empty ones, and `1`–`7` jump
straight to one (`0` for all). The choice is kept, so while running
relics, `N` opens straight on the primes worth cracking them for, and
`enter` on one shows which of your relics hold its parts. Items needing a
higher MR than yours, and ones with no easy way in (vaulted primes, quests,
Kuva Liches and so on), are left out.

WFCD doesn't say where a blueprint is sold, so the dojo is told apart by
its research parts (Chem, Bio, Energy and Orokin labs) and a list checked
against the game (the Tenno Lab and a few strays). The odd item may
still be in the wrong group.

## Weapons needed for other weapons

Items that another recipe consumes say **⚒ Keep for crafting** in their
details, with what they build — e.g. Dual Skana → Akjagara, Dark
Split-Sword, Redeemer. Once that item is mastered it's shown dimmed with ★,
so you know the weapon is safe to sell.

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
Coda weapons and Necramechs rank to 40: to count one part way, open its
details and type the rank it has reached, e.g. `34` then `enter`. Star chart
and Steel Path XP can't be enumerated, so enter them in the ledger (`x`)
from your in-game profile: the XP itself, not how many nodes are done. Intrinsics are entered as ranks: add up every school's rank (each
0–10), so Railjack is out of 50 and Drifter out of 40. Each rank is worth
1,500 mastery XP.

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

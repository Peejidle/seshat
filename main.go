// Seshat is a terminal companion for tracking Warframe mastery progress.
// Item data comes from the community WFCD warframe-items dataset; ownership
// and mastery are marked by hand and never read from the game or account.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Peejidle/seshat/internal/catalog"
	"github.com/Peejidle/seshat/internal/mastery"
	"github.com/Peejidle/seshat/internal/profile"
	"github.com/Peejidle/seshat/internal/store"
	"github.com/Peejidle/seshat/internal/termimg"
	"github.com/Peejidle/seshat/internal/ui"
)

const usage = `seshat — Cephalon Seshat, keeper of the codex

usage:
  seshat                 open the archive
  seshat update          sync item data from WFCD
  seshat stats           print mastery totals
  seshat export FILE     back up your records
  seshat import FILE     restore records from a backup
  seshat import-profile FILE
                         mark mastery from your saved Warframe profile
                         (see README: "Importing from your profile")
`

func main() {
	paths, err := resolvePaths()
	if err != nil {
		die(err)
	}
	prog, err := store.Load(paths.Progress)
	if err != nil {
		die(fmt.Errorf("reading %s: %w", paths.Progress, err))
	}

	args := os.Args[1:]
	if len(args) == 0 {
		p := tea.NewProgram(ui.New(paths, prog), tea.WithAltScreen())
		_, err := p.Run()
		termimg.Cleanup()
		if err != nil {
			die(err)
		}
		return
	}

	switch {
	case args[0] == "update" && len(args) == 1:
		n, err := catalog.Refresh(paths.Data)
		if err != nil {
			die(err)
		}
		fmt.Printf("archive synced: %d file(s) changed\n", n)
	case args[0] == "stats" && len(args) == 1:
		if err := stats(paths, prog); err != nil {
			die(err)
		}
	case args[0] == "export" && len(args) == 2:
		if err := prog.Export(args[1]); err != nil {
			die(err)
		}
		fmt.Printf("records written to %s\n", args[1])
	case args[0] == "import-profile" && len(args) == 2:
		if err := importProfile(paths, prog, args[1]); err != nil {
			die(err)
		}
	case args[0] == "import" && len(args) == 2:
		if err := prog.Import(args[1]); err != nil {
			die(err)
		}
		if err := prog.Save(paths.Progress); err != nil {
			die(err)
		}
		fmt.Printf("restored %d marks from %s\n", len(prog.Marks), args[1])
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
}

func loadItems(paths ui.Paths) ([]catalog.Item, error) {
	if !catalog.HasCache(paths.Data) {
		if _, err := catalog.Refresh(paths.Data); err != nil {
			return nil, err
		}
	}
	return catalog.Load(paths.Data)
}

func importProfile(paths ui.Paths, prog *store.Progress, file string) error {
	b, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	p, err := profile.Parse(b)
	if err != nil {
		return err
	}
	items, err := loadItems(paths)
	if err != nil {
		return err
	}
	r := profile.Apply(p, items, prog)
	if err := prog.Save(paths.Progress); err != nil {
		return err
	}
	fmt.Println(profile.Summary(r))
	return nil
}

func stats(paths ui.Paths, prog *store.Progress) error {
	items, err := loadItems(paths)
	if err != nil {
		return err
	}
	type t struct{ n, m, xp int }
	per := map[catalog.Category]*t{}
	var all t
	for _, it := range items {
		c := per[it.Category]
		if c == nil {
			c = &t{}
			per[it.Category] = c
		}
		c.n++
		all.n++
		if prog.Get(it.ID).Mastered {
			c.m++
			c.xp += it.MasteryXP()
			all.m++
			all.xp += it.MasteryXP()
		}
	}
	for _, cat := range catalog.Categories {
		c := per[cat]
		if c == nil {
			continue
		}
		fmt.Printf("%-18s %4d/%-4d %10d xp\n", cat, c.m, c.n, c.xp)
	}
	xp := all.xp + prog.Extra.Total()
	rank, _, _ := mastery.Rank(xp)
	fmt.Printf("%-18s %4d/%-4d %10d xp  (+%d ledger)  ≈ %s\n", "Total", all.m, all.n, xp, prog.Extra.Total(), mastery.Label(rank))
	return nil
}

func resolvePaths() (ui.Paths, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return ui.Paths{}, err
	}
	data := os.Getenv("XDG_DATA_HOME")
	if data == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ui.Paths{}, err
		}
		data = filepath.Join(home, ".local", "share")
	}
	return ui.Paths{
		Data:     filepath.Join(cache, "seshat", "data"),
		Images:   filepath.Join(cache, "seshat", "img"),
		Progress: filepath.Join(data, "seshat", "progress.json"),
	}, nil
}

func die(err error) {
	fmt.Fprintln(os.Stderr, "seshat:", err)
	os.Exit(1)
}

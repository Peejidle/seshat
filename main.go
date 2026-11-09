// Seshat is a terminal companion for tracking Warframe mastery progress.
// Item data comes from the community WFCD warframe-items dataset; ownership
// and mastery are marked by hand and never read from the game or account.
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Peejidle/seshat/internal/catalog"
	"github.com/Peejidle/seshat/internal/market"
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
                         mark mastery from your saved Warframe profile;
                         FILE - reads it from standard input, e.g.
                         wl-paste | seshat import-profile -
                         (see README: "Importing from your profile")
  seshat price NAME      look up what an item sells for on warframe.market
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
	case args[0] == "price" && len(args) > 1:
		if err := price(paths, strings.Join(args[1:], " ")); err != nil {
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
	var b []byte
	var err error
	if file == "-" {
		b, err = io.ReadAll(os.Stdin) // "-" is the usual way to say "read the pipe"
	} else {
		b, err = os.ReadFile(file)
	}
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
	// Without a synced star chart the ledger's node XP is left as it was;
	// the summary says so.
	nodes, _ := profile.LoadNodeXP(filepath.Join(paths.Data, catalog.NodesFile))
	r := profile.Apply(p, items, prog, nodes)
	if err := prog.Save(paths.Progress); err != nil {
		return err
	}
	for _, l := range profile.Summary(r) {
		fmt.Println(l)
	}
	return nil
}

// price prints what an item goes for, from the cache while it's fresh.
func price(paths ui.Paths, name string) error {
	if !catalog.HasCache(paths.Data) {
		if _, err := catalog.Refresh(paths.Data); err != nil {
			return err
		}
	}
	mc, err := market.Load(filepath.Join(paths.Data, market.ItemsFile))
	if err != nil {
		return err
	}
	found := mc.Find(name, nil)
	switch {
	case len(found) == 0:
		return fmt.Errorf("nothing on warframe.market is called %q", name)
	case len(found) > 1:
		fmt.Printf("%q could be any of these: say which.\n", name)
		for i, it := range found {
			if i == 15 {
				fmt.Printf("  …and %d more\n", len(found)-i)
				break
			}
			fmt.Println("  " + it.Name)
		}
		return nil
	}
	it := found[0]

	cache := market.LoadPrices(filepath.Join(paths.Data, market.PricesFile))
	p, fresh := cache.Get(it.Slug)
	if !fresh {
		if p, err = market.NewClient(catalog.UserAgent).Fetch(it); err != nil {
			return err
		}
		cache.Put(p)
		if err := cache.Save(); err != nil {
			return err
		}
	}

	fmt.Println(it.Name)
	ranks := []int{0}
	if it.MaxRank > 0 {
		ranks = append(ranks, it.MaxRank)
	}
	for _, r := range ranks {
		pr := p.Ranks[r]
		label := ""
		if it.MaxRank > 0 {
			label = fmt.Sprintf("rank %d:  ", r)
		}
		fmt.Printf("  %smedian %s (%d sold in 48h) · cheapest seller online %s · best buyer online %s\n",
			label, platText(pr.Median), pr.Sold, platText(pr.Lowest), platText(pr.TopBuy))
	}
	if it.Ducats > 0 {
		fmt.Printf("  or %d ducats at Baro's kiosks\n", it.Ducats)
	}
	checked := "just now"
	if ago := time.Since(p.Fetched).Round(time.Minute); ago > 0 {
		checked = strings.TrimSuffix(ago.String(), "0s") + " ago" // "1h5m" rather than "1h5m0s"
	}
	fmt.Printf("  checked %s · https://warframe.market/items/%s\n", checked, it.Slug)
	return nil
}

// platText shows a price, or a dash where there's none.
func platText(n int) string {
	if n == 0 {
		return "–"
	}
	return fmt.Sprintf("%dp", n)
}

func stats(paths ui.Paths, prog *store.Progress) error {
	items, err := loadItems(paths)
	if err != nil {
		return err
	}
	for _, cat := range catalog.Categories {
		if t := mastery.Count(items, prog, cat); t.Total > 0 {
			fmt.Printf("%-18s %4d/%-4d %10d xp\n", cat, t.Mastered, t.Total, t.XP)
		}
	}
	all := mastery.Count(items, prog, "")
	xp := mastery.Estimate(items, prog)
	rank, _, _ := mastery.Rank(xp)
	fmt.Printf("%-18s %4d/%-4d %10d xp  (+%d ledger)  ≈ %s\n", "Total", all.Mastered, all.Total, xp, prog.Extra.Total(), mastery.Label(rank))
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

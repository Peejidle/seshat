package mastery

import (
	"github.com/Peejidle/seshat/internal/catalog"
	"github.com/Peejidle/seshat/internal/store"
)

// Tally sums a checklist over a set of items.
type Tally struct {
	Total, Owned, Mastered, Partial int
	XP, XPTotal                     int // earned so far, and available
}

// Count tallies the items in cat, or every item when cat is "".
func Count(items []catalog.Item, p *store.Progress, cat catalog.Category) Tally {
	var t Tally
	for _, it := range items {
		if cat != "" && it.Category != cat {
			continue
		}
		mk := p.Get(it.ID)
		t.Total++
		t.XPTotal += it.MasteryXP()
		if mk.Owned {
			t.Owned++
		}
		if mk.Mastered {
			t.Mastered++
		} else if mk.Rank > 0 {
			t.Partial++
		}
		t.XP += it.EarnedXP(mk.Mastered, mk.Rank)
	}
	return t
}

// Estimate is the account's total mastery XP: every item plus the ledger.
func Estimate(items []catalog.Item, p *store.Progress) int {
	return Count(items, p, "").XP + p.Extra.Total()
}

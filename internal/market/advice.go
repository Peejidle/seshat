package market

import "fmt"

// Advice on selling: what to list and at what price, worked out from the
// prices looked up. It's rules of thumb, so every step says why.

// Holding is what's held of one item to sell. Parts only use Copies.
type Holding struct {
	Maxed  int
	Copies int // loose: rank 0, or for a part, how many
}

type Advice struct {
	Steps []string // what to do, in order, e.g. "list 2 maxed at 119p"
	Worth int      // the platinum those steps should bring, after any buying
	// Ducats is what Baro gives for the parts advised his way.
	Ducats int
}

// junk is the price at or under which a part goes to Baro for ducats,
// and an arcane copy isn't worth a trade.
const junk = 3

// ListPrice suggests what to list one at: the cheapest seller online's
// price, kept within 10% of the median people have been paying, so one
// seller dumping (or asking too much) doesn't set it. 0 if no price is known.
func ListPrice(p Price) int {
	switch {
	case p.Median == 0:
		return p.Lowest
	case p.Lowest == 0:
		return p.Median
	}
	lo, hi := (p.Median*9+5)/10, (p.Median*11+5)/10
	return min(max(p.Lowest, lo), hi)
}

// Advise works out how best to sell h of it at prices p.
func Advise(it Item, h Holding, p Prices) Advice {
	if !it.IsArcane() {
		return advisePart(it, h.Copies, p.Ranks[0])
	}
	var a Advice
	top, base := p.Ranks[it.MaxRank], p.Ranks[0]
	pm, p0 := ListPrice(top), ListPrice(base)
	if h.Maxed > 0 {
		if pm > 0 {
			a.Steps = append(a.Steps, fmt.Sprintf("list %d maxed at %dp", h.Maxed, pm))
			a.Worth += h.Maxed * pm
		} else {
			a.Steps = append(a.Steps, fmt.Sprintf("%d maxed: no recent price", h.Maxed))
		}
	}
	if h.Copies > 0 {
		adviseCopies(&a, it, h.Copies, pm, p0, base)
	}
	return a
}

// adviseCopies decides what to do with loose copies: fuse full sets if a
// maxed one is worth more, top up a set that's nearly there if buying the
// rest pays, and otherwise sell them as they are, or not bother.
func adviseCopies(a *Advice, it Item, copies, pm, p0 int, base Price) {
	n := CopiesToMax(it.MaxRank)
	if p0 == 0 && pm == 0 {
		a.Steps = append(a.Steps, fmt.Sprintf("%d copies: no recent price", copies))
		return
	}
	sets, rest := copies/n, copies%n
	if sets > 0 && pm > n*p0 {
		a.Steps = append(a.Steps, fmt.Sprintf("fuse %d into %d maxed (%dp, against %dp as singles)", sets*n, sets, pm, n*p0))
		a.Worth += sets * pm
	} else {
		rest = copies // not worth fusing: they're all singles
	}
	if rest == 0 {
		return
	}

	// Topping up means buying the missing copies at about what the
	// cheapest seller asks, then selling one maxed.
	buy := base.Lowest
	if buy == 0 {
		buy = p0
	}
	need := n - rest
	gain := pm - need*buy - rest*p0 // over selling them as singles
	if rest < n && pm > 0 && buy > 0 && gain >= max(10, pm/5) {
		a.Steps = append(a.Steps, fmt.Sprintf("buy %d more at ~%dp and fuse: a maxed one sells for %dp, %dp more than singles", need, buy, pm, gain))
		a.Worth += pm - need*buy
		return
	}
	if p0 <= junk {
		a.Steps = append(a.Steps, fmt.Sprintf("%d copies sell for %dp: dissolve them for Vosfor", rest, p0))
		return
	}
	a.Steps = append(a.Steps, fmt.Sprintf("sell %d singles at %dp", rest, p0))
	a.Worth += rest * p0
}

func advisePart(it Item, count int, p Price) Advice {
	var a Advice
	pl := ListPrice(p)
	switch {
	case pl > junk:
		a.Steps = append(a.Steps, fmt.Sprintf("list %d at %dp", count, pl))
		a.Worth = count * pl
		if it.Ducats > 0 {
			a.Steps = append(a.Steps, fmt.Sprintf("or %d ducats each", it.Ducats))
		}
	case it.Ducats > 0:
		a.Steps = append(a.Steps, fmt.Sprintf("only %dp: give Baro them for %d ducats", pl, count*it.Ducats))
		a.Ducats = count * it.Ducats
	case pl > 0:
		a.Steps = append(a.Steps, fmt.Sprintf("list %d at %dp", count, pl))
		a.Worth = count * pl
	default:
		a.Steps = append(a.Steps, "no recent price")
	}
	return a
}

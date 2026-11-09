package market

import (
	"strings"
	"testing"
)

func TestListPrice(t *testing.T) {
	for _, c := range []struct {
		p    Price
		want int
	}{
		{Price{Median: 120, Lowest: 119}, 119}, // match the cheapest seller
		{Price{Median: 120, Lowest: 60}, 108},  // someone's dumping: stay near the median
		{Price{Median: 120, Lowest: 200}, 132}, // or asking too much
		{Price{Median: 20}, 20},
		{Price{Lowest: 5}, 5},
		{Price{}, 0},
	} {
		if got := ListPrice(c.p); got != c.want {
			t.Errorf("ListPrice(%+v) = %d, want %d", c.p, got, c.want)
		}
	}
}

func arcane(rank0, maxed Price) (Item, Prices) {
	return Item{Name: "Arcane Test", Tags: []string{"arcane_enhancement"}, MaxRank: 5},
		Prices{Ranks: map[int]Price{0: rank0, 5: maxed}}
}

func TestAdviseArcanes(t *testing.T) {
	// Singles sell for more than a maxed one: 21 × 5p beats 75p.
	it, p := arcane(Price{Median: 5, Lowest: 4}, Price{Median: 78, Lowest: 75})
	a := Advise(it, Holding{Maxed: 2, Copies: 14}, p)
	if a.Worth != 2*75+14*5 || !strings.Contains(strings.Join(a.Steps, "; "), "sell 14 singles at 5p") {
		t.Errorf("singles: %+v", a)
	}

	// A maxed one is worth far more than its copies: fuse full sets, and
	// top up a set that's nearly there.
	it, p = arcane(Price{Median: 2, Lowest: 2}, Price{Median: 100, Lowest: 100})
	a = Advise(it, Holding{Copies: 21 + 18}, p)
	steps := strings.Join(a.Steps, "; ")
	if !strings.Contains(steps, "fuse 21 into 1 maxed") || !strings.Contains(steps, "buy 3 more at ~2p") {
		t.Errorf("fuse and top up: %q", steps)
	}
	if a.Worth != 100+100-3*2 {
		t.Errorf("worth %d, want 194", a.Worth)
	}

	// Cheap copies a few short of nothing worth much: not worth a trade.
	it, p = arcane(Price{Median: 1, Lowest: 1}, Price{Median: 15, Lowest: 15})
	a = Advise(it, Holding{Copies: 6}, p)
	if a.Worth != 0 || !strings.Contains(a.Steps[0], "dissolve") {
		t.Errorf("junk: %+v", a)
	}
}

func TestAdviseParts(t *testing.T) {
	part := Item{Name: "Nekros Prime Neuroptics Blueprint", Tags: []string{"prime", "component"}, Ducats: 45}
	a := Advise(part, Holding{Copies: 2}, Prices{Ranks: map[int]Price{0: {Median: 20, Lowest: 22}}})
	if a.Worth != 44 || a.Steps[0] != "list 2 at 22p" {
		t.Errorf("worth selling: %+v", a)
	}
	part.Ducats = 15
	a = Advise(part, Holding{Copies: 3}, Prices{Ranks: map[int]Price{0: {Median: 2, Lowest: 2}}})
	if a.Worth != 0 || a.Ducats != 45 {
		t.Errorf("Baro's: %+v", a)
	}
}

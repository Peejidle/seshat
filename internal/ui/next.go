package ui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Peejidle/seshat/internal/catalog"
	"github.com/Peejidle/seshat/internal/mastery"
)

// "Next up": the unmastered items that are easiest to get, easiest first,
// with a running XP total that shows where the next ranks land.

// route is how an item can be got, easiest first.
type route int

const (
	rLevel   route = iota // owned already: just level it
	rBuild                // every part in hand: straight to the foundry
	rMarket               // a credits blueprint in the Market, then resources
	rDojo                 // a credits blueprint in the clan dojo, then resources
	rRelics               // a prime whose missing parts are in relics you can farm or hold
	rStarted              // some parts in hand, the rest from wherever they drop
	rFarm                 // a credits blueprint, then parts from missions
)

var routeNames = [...]string{
	rLevel:   "Owned: just level it",
	rBuild:   "Every part in hand: ready for the foundry",
	rMarket:  "Market blueprint for credits, then resources",
	rDojo:    "Clan dojo research: blueprints for credits, then resources",
	rRelics:  "Primes whose missing parts are all in relics you can farm or hold",
	rStarted: "Started: some parts in hand, fewest missing first",
	rFarm:    "Market blueprint, then parts from missions",
}

// routeShort names each route in the group picker.
var routeShort = [...]string{
	rLevel: "Owned", rBuild: "Build", rMarket: "Market", rDojo: "Dojo",
	rRelics: "Relics", rStarted: "Started", rFarm: "Farm",
}

type nextItem struct {
	idx     int // into m.items
	route   route
	xp      int    // mastery XP still to earn from it
	missing int    // parts still to collect
	note    string // what's left to do, e.g. "2 of 4 parts"
}

// nextItems is nextAll narrowed to the chosen group, if there is one.
func (m Model) nextItems() []nextItem {
	all := m.nextAll()
	if m.nextGroup == 0 {
		return all
	}
	var out []nextItem
	for _, n := range all {
		if n.route == route(m.nextGroup-1) {
			out = append(out, n)
		}
	}
	return out
}

// nextAll lists what can be mastered, easiest first and the most XP first
// within each route. Items needing a higher MR than yours are left out, as
// are ones with no easy way in (vaulted primes, quests, bosses...).
func (m Model) nextAll() []nextItem {
	rank, _, _ := mastery.Rank(mastery.Estimate(m.items, m.prog))
	var out []nextItem
	for i, it := range m.items {
		if m.tab > 0 && it.Category != catalog.Categories[m.tab-1] {
			continue // follow the category tab, e.g. only Warframes
		}
		mk := m.prog.Get(it.ID)
		if mk.Mastered || it.MasteryReq > rank {
			continue
		}
		held, need := m.partsProgress(it)
		n := nextItem{idx: i, xp: it.MasteryXP() - it.EarnedXP(false, mk.Rank), missing: need - held}
		if r, note, ok := m.routeFor(it); ok {
			n.route, n.note = r, note
			out = append(out, n)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.route != b.route {
			return a.route < b.route
		}
		if (a.route == rRelics || a.route == rStarted) && a.missing != b.missing {
			return a.missing < b.missing // one part from done comes first
		}
		if a.xp != b.xp {
			return a.xp > b.xp
		}
		return m.items[a.idx].Name < m.items[b.idx].Name
	})
	return out
}

// routeFor works out an item's easiest route and a note on what's left.
func (m Model) routeFor(it catalog.Item) (route, string, bool) {
	mk := m.prog.Get(it.ID)
	if mk.Owned {
		return rLevel, fmt.Sprintf("rank %d of %d", mk.Rank, it.MaxRank), true
	}

	parts := it.Parts()
	held, need := m.partsProgress(it)
	if need > 0 && held == need {
		return rBuild, "", true
	}

	// e.g. "bp 15,000 cr · 175p": the blueprint's price in credits, and
	// the built item's in platinum if you'd rather skip the farming.
	var price []string
	if it.CreditBP > 0 {
		price = append(price, fmt.Sprintf("bp %s cr", commas(it.CreditBP)))
	}
	if it.Platinum > 0 {
		price = append(price, fmt.Sprintf("%dp", it.Platinum))
	}
	cost := strings.Join(price, " · ")
	// Only the item's own blueprint to get, the rest is resources.
	resourcesOnly := len(parts) == 1 && parts[0].Name == "Blueprint"
	switch {
	case it.Dojo && resourcesOnly:
		return rDojo, cost, true
	case it.Dojo:
		// Dojo parts are researched there too, so no farming either.
		return rDojo, strings.TrimSuffix(fmt.Sprintf("%d of %d parts in hand · %s", held, need, cost), " · "), true
	case it.CreditBP > 0 && resourcesOnly:
		return rMarket, cost, true
	case it.Prime && m.allInReach(it):
		return rRelics, fmt.Sprintf("%d of %d parts in hand", held, need), true
	case held > 0:
		return rStarted, strings.TrimSuffix(fmt.Sprintf("%d of %d parts in hand · %s", held, need, cost), " · "), true
	case it.CreditBP > 0 && !it.Dojo:
		return rFarm, fmt.Sprintf("%d of %d parts in hand · %s", held, need, cost), true
	}
	return 0, "", false
}

// allInReach reports whether every part still needed for it drops from a
// relic you can farm right now or already hold. Primes that no relic holds,
// such as the Founders' Excalibur Prime, don't count.
func (m Model) allInReach(it catalog.Item) bool {
	inRelics := false
	for _, c := range it.Parts() {
		inRelics = inRelics || len(m.drops[partKey(it.ID, c.ID)]) > 0
	}
	if !inRelics {
		return false
	}
	for _, c := range it.Parts() {
		if !m.stillNeeded(it, c) {
			continue
		}
		reach := false
		for _, d := range m.drops[partKey(it.ID, c.ID)] {
			reach = reach || d.Relic.Farmable || m.prog.HeldRelics(d.Relic.Name) > 0
		}
		if !reach {
			return false
		}
	}
	return true
}

// nextLine is one line of the view: a route heading, a rank marker or an
// item (item >= 0, an index into nextItems).
type nextLine struct {
	text string
	item int
}

// nextLines lays out the view: items under route headings, with a marker
// wherever the running XP total reaches a new rank.
func (m Model) nextLines(ns []nextItem) []nextLine {
	xp := mastery.Estimate(m.items, m.prog)
	rank, _, _ := mastery.Rank(xp)
	var lines []nextLine
	for i, n := range ns {
		if i == 0 || ns[i-1].route != n.route {
			if i > 0 {
				lines = append(lines, nextLine{"", -1})
			}
			lines = append(lines, nextLine{sSub.Render(routeNames[n.route]), -1})
		}
		lines = append(lines, nextLine{"", i})
		xp += n.xp
		for r, _, _ := mastery.Rank(xp); rank < r; {
			rank++
			lines = append(lines, nextLine{sGold.Render("   ── " + mastery.Label(rank) + " reached here ──"), -1})
		}
	}
	return lines
}

// nextRows is how many lines the list gets: the window less the box, the
// heading, the group picker and the footer.
func (m Model) nextRows() int { return max(4, m.height-11) }

func (m *Model) openNext() {
	m.modal, m.nextAt, m.nextTop = mNext, 0, 0
}

func (m Model) nextKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	ns := m.nextItems()
	switch k.String() {
	case "down", "j":
		m.nextAt = min(m.nextAt+1, max(len(ns)-1, 0))
	case "up", "k":
		m.nextAt = max(m.nextAt-1, 0)
	case "pgdown", "ctrl+d":
		m.nextAt = min(m.nextAt+m.nextRows()/2, max(len(ns)-1, 0))
	case "pgup", "ctrl+u":
		m.nextAt = max(m.nextAt-m.nextRows()/2, 0)
	case "g", "home":
		m.nextAt = 0
	case "G", "end":
		m.nextAt = max(len(ns)-1, 0)
	case "tab", "shift+tab":
		m.cycleNextGroup(k.String() == "tab")
		return m, nil
	case "0", "1", "2", "3", "4", "5", "6", "7":
		m.nextGroup = int(k.String()[0] - '0')
		m.nextAt, m.nextTop = 0, 0
		return m, nil
	case "enter":
		if m.nextAt < len(ns) {
			m.showItem(ns[m.nextAt].idx)
			m.modal, m.detailBack, m.partAt = mDetail, mNext, 0
			return m, m.wantImage()
		}
	default:
		m.modal = mNone
		return m, m.wantImage()
	}
	m.scrollNext(ns)
	return m, nil
}

// cycleNextGroup moves to the next (or previous) group that has anything
// in it, by way of "all".
func (m *Model) cycleNextGroup(forward bool) {
	has := map[int]bool{0: true}
	for _, n := range m.nextAll() {
		has[int(n.route)+1] = true
	}
	step := 1
	if !forward {
		step = len(routeShort)
	}
	for g := (m.nextGroup + step) % (len(routeShort) + 1); ; g = (g + step) % (len(routeShort) + 1) {
		if has[g] {
			m.nextGroup = g
			break
		}
	}
	m.nextAt, m.nextTop = 0, 0
}

// groupPicker is the row of groups, the chosen one lit, each with its
// count; empty ones are dimmed.
func (m Model) groupPicker() string {
	counts := make([]int, len(routeShort))
	all := m.nextAll()
	for _, n := range all {
		counts[n.route]++
	}
	pick := func(g int, label string, n int) string {
		s := fmt.Sprintf("%d %s %d", g, label, n)
		switch {
		case g == m.nextGroup:
			return sGold.Render("[" + s + "]")
		case n == 0:
			return sDim.Render(" " + s + " ")
		}
		return sText.Render(" " + s + " ")
	}
	parts := []string{pick(0, "All", len(all))}
	for r, label := range routeShort {
		parts = append(parts, pick(r+1, label, counts[r]))
	}
	return strings.Join(parts, "")
}

// showItem puts the list cursor on m.items[idx], clearing the search and
// filter if they hide it, so the details view can show it.
func (m *Model) showItem(idx int) {
	find := func() bool {
		for i, v := range m.visible {
			if v == idx {
				m.cursor = i
				return true
			}
		}
		return false
	}
	if !find() {
		m.search.SetValue("")
		m.filter = fAll
		m.rebuild()
		find()
	}
	m.clamp()
}

// scrollNext moves the first shown line so the selected item is on screen.
func (m *Model) scrollNext(ns []nextItem) {
	lines := m.nextLines(ns)
	at := 0
	for i, l := range lines {
		if l.item == m.nextAt {
			at = i
		}
	}
	if at < m.nextTop {
		m.nextTop = at
		// Show the route heading when moving up onto a group's first item.
		for m.nextTop > 0 && lines[m.nextTop-1].item < 0 && lines[m.nextTop-1].text != "" {
			m.nextTop--
		}
	}
	if at >= m.nextTop+m.nextRows() {
		m.nextTop = at - m.nextRows() + 1
	}
}

func (m Model) nextView() string {
	ns := m.nextItems()
	w := max(20, m.width-6)
	xp := mastery.Estimate(m.items, m.prog)
	rank, _, next := mastery.Rank(xp)

	var b strings.Builder
	b.WriteString(sTitle.Render("Next Up") + "\n")
	total := 0
	for _, n := range ns {
		total += n.xp
	}
	scope := ""
	if m.tab > 0 {
		scope = " · " + tabLabels[m.tab] + " only"
	}
	b.WriteString(sSub.Render(trunc(fmt.Sprintf("%s · %s XP to %s · these %d items hold %s XP%s",
		mastery.Label(rank), commas(next-xp), mastery.Label(rank+1), len(ns), commas(total), scope), w)) + "\n")
	b.WriteString(trunc(m.groupPicker(), w) + "\n\n")

	lines := m.nextLines(ns)
	if len(ns) == 0 {
		msg := "Nothing easy left here: what remains is vaulted, from quests or bosses."
		if m.nextGroup > 0 {
			msg = "Nothing in this group: tab to another, or 0 for all."
		}
		b.WriteString(sDim.Render(msg) + "\n")
	}
	shown := 0
	running := xp // your XP once everything up to this line is mastered
	for i, l := range lines {
		if l.item >= 0 {
			running += ns[l.item].xp
		}
		if i < m.nextTop || shown >= m.nextRows() {
			continue
		}
		shown++
		if l.item < 0 {
			b.WriteString(trunc(l.text, w) + "\n")
			continue
		}
		n := ns[l.item]
		it := m.items[n.idx]
		sel, name := "  ", sText.Render(pad(trunc(it.Name, 24), 25))
		if l.item == m.nextAt {
			sel, name = sGold.Render("▸ "), sTitle.Render(pad(trunc(it.Name, 24), 25))
		}
		r, _, next := mastery.Rank(running)
		left := fmt.Sprintf("%s to %s", commas(next-running), mastery.Label(r+1))
		row := sel + name + sDim.Render(pad(trunc(it.Type, 12), 13)) +
			sGold.Render(fmt.Sprintf("%7s", "+"+commas(n.xp))) + "  " +
			sDim.Render(pad(left, 18)) + sText.Render(n.note)
		b.WriteString(trunc(row, w) + "\n")
	}
	b.WriteString(strings.Repeat("\n", max(m.nextRows()-shown, 0)))

	b.WriteString("\n" + sDim.Render(trunc("XP it gives · XP left to the next rank once it's mastered · follows the category tab", w)) + "\n")
	b.WriteString(sDim.Render(trunc("j/k move · tab or 0-7 group · enter details · esc close", w)))
	return b.String()
}

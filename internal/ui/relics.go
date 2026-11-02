package ui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Peejidle/seshat/internal/catalog"
	"github.com/Peejidle/seshat/internal/relics"
)

// The relic finder: which relics hold the parts you still need.

func partKey(itemID, partID string) string { return itemID + "\x00" + partID }

// matchDrops works out once, after loading, where every part drops.
func (m *Model) matchDrops() {
	m.drops = map[string][]relics.Drop{}
	if m.relics == nil {
		return
	}
	for _, it := range m.items {
		parts := it.Parts()
		names := make([]string, len(parts))
		for i, c := range parts {
			names[i] = c.Name
		}
		for i, ds := range m.relics.Match(it.Name, names) {
			if len(ds) > 0 {
				m.drops[partKey(it.ID, parts[i].ID)] = ds
			}
		}
	}
}

// partLabel is a part's name as the game shows it, when the drop tables
// know it ("Hilt" rather than DE's internal "Handle").
func (m Model) partLabel(it catalog.Item, c catalog.Component) string {
	if ds := m.drops[partKey(it.ID, c.ID)]; len(ds) > 0 {
		return ds[0].PartName(it.Name)
	}
	return c.Name
}

// stillNeeded reports whether a part is worth farming: the item isn't built
// or mastered, and fewer of the part are held than the recipe takes.
func (m Model) stillNeeded(it catalog.Item, c catalog.Component) bool {
	mk := m.prog.Get(it.ID)
	return !mk.Owned && !mk.Mastered && m.prog.HeldParts(it.ID, c.ID) < c.Count
}

// relicNeed is one needed part a relic can give.
type relicNeed struct {
	item, part string
	reward     relics.Reward
}

type relicEntry struct {
	relic *relics.Relic
	needs []relicNeed
}

// relicEntries lists the relics holding parts you need, most needed parts
// first. Vaulted relics are left out unless asked for.
func (m Model) relicEntries() []relicEntry {
	byRelic := map[*relics.Relic][]relicNeed{}
	for _, it := range m.items {
		for _, c := range it.Parts() {
			if !m.stillNeeded(it, c) {
				continue
			}
			for _, d := range m.drops[partKey(it.ID, c.ID)] {
				if d.Relic.Farmable || m.relicVaulted {
					byRelic[d.Relic] = append(byRelic[d.Relic], relicNeed{it.Name, m.partLabel(it, c), d.Reward})
				}
			}
		}
	}
	var out []relicEntry
	for r, ns := range byRelic {
		sort.Slice(ns, func(i, j int) bool { return ns[i].reward.Intact > ns[j].reward.Intact })
		out = append(out, relicEntry{r, ns})
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if len(a.needs) != len(b.needs) {
			return len(a.needs) > len(b.needs)
		}
		if a.relic.Farmable != b.relic.Farmable {
			return a.relic.Farmable
		}
		if a.relic.Tier != b.relic.Tier {
			return a.relic.Tier < b.relic.Tier
		}
		return a.relic.Name < b.relic.Name
	})
	return out
}

func (m *Model) openRelics() {
	if m.relics == nil {
		m.fail("No relic data yet: press u to sync the archive.")
		return
	}
	m.modal, m.relicAt, m.relicTop = mRelics, 0, 0
}

func (m Model) relicKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	n := len(m.relicEntries())
	switch k.String() {
	case "down", "j":
		m.relicAt = min(m.relicAt+1, max(n-1, 0))
	case "up", "k":
		m.relicAt = max(m.relicAt-1, 0)
	case "g", "home":
		m.relicAt = 0
	case "G", "end":
		m.relicAt = max(n-1, 0)
	case "v":
		m.relicVaulted = !m.relicVaulted
		m.relicAt, m.relicTop = 0, 0
	default:
		m.modal = mNone
		return m, m.wantImage()
	}
	m.scrollRelics()
	return m, nil
}

// relicRows is how many lines the finder has for relics.
func (m Model) relicRows() int { return max(4, m.height-12) }

// scrollRelics moves the first shown relic so the selected one fits. Each
// relic takes a line per needed part plus its heading.
func (m *Model) scrollRelics() {
	es := m.relicEntries()
	if m.relicAt < m.relicTop {
		m.relicTop = m.relicAt
	}
	for m.relicTop < m.relicAt {
		lines := 0
		for i := m.relicTop; i <= m.relicAt; i++ {
			lines += 1 + len(es[i].needs)
		}
		if lines <= m.relicRows() {
			break
		}
		m.relicTop++
	}
}

func (m Model) relicView() string {
	es := m.relicEntries()
	var b strings.Builder
	b.WriteString(sTitle.Render("Relic Finder") + "\n")
	scope := "farmable now"
	if m.relicVaulted {
		scope = "including vaulted"
	}
	b.WriteString(sSub.Render(fmt.Sprintf("Relics holding parts you still need · %s · %d relics", scope, len(es))) + "\n\n")
	if len(es) == 0 {
		b.WriteString(sDim.Render("Nothing to farm: every part you're missing is in vaulted relics,") + "\n")
		b.WriteString(sDim.Render("or the parts list is all checked off. Press v to include vaulted.") + "\n")
	}

	lines := 0
	for i := m.relicTop; i < len(es) && lines+1+len(es[i].needs) <= m.relicRows(); i++ {
		e := es[i]
		sel, name := "  ", sText.Bold(true).Render(pad(e.relic.Name, 10))
		if i == m.relicAt {
			sel, name = sGold.Render("▸ "), sTitle.Render(pad(e.relic.Name, 10))
		}
		state := sNew.Render("farmable")
		if !e.relic.Farmable {
			state = sDim.Render("vaulted · trade or Prime Resurgence")
		}
		b.WriteString(sel + name + sDim.Render(fmt.Sprintf("%d needed  ", len(e.needs))) + state + "\n")
		for _, n := range e.needs {
			what := trunc(n.item+" "+n.part, 40)
			b.WriteString("     " + sText.Render(pad(what, 41)) + chance(n.reward) + "\n")
		}
		lines += 1 + len(e.needs)
	}
	b.WriteString("\n" + sDim.Render("chance shown intact → radiant · j/k move · v vaulted on/off · esc close"))
	return b.String()
}

// chance renders a drop chance coloured by rarity.
func chance(r relics.Reward) string {
	st := sDim
	switch r.Rarity {
	case "Uncommon":
		st = sLapis
	case "Rare":
		st = sGold
	}
	return st.Render(fmt.Sprintf("%-8s %2.0f → %2.0f%%", strings.ToLower(r.Rarity), r.Intact, r.Radiant))
}

// dropLine is the short "where does this drop" note under a part in the
// details: the best few farmable relics, or how many vaulted ones hold it.
func (m Model) dropLine(ds []relics.Drop, w int) string {
	var farm []string
	for _, d := range ds {
		if d.Relic.Farmable && len(farm) < 3 {
			farm = append(farm, fmt.Sprintf("%s %.0f%%", d.Relic.Name, d.Reward.Intact))
		}
	}
	if len(farm) > 0 {
		return sNew.Render(trunc(strings.Join(farm, " · "), w))
	}
	return sDim.Render(fmt.Sprintf("vaulted · in %d relic%s", len(ds), plural(len(ds), "", "s")))
}

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

// relicRow is one reward line under a relic in the finder.
type relicRow struct {
	name   string // e.g. "Nyx Prime Chassis"
	reward relics.Reward
	needed bool // a part you still need
}

type relicEntry struct {
	relic *relics.Relic
	rows  []relicRow
}

// relicEntries lists what the finder shows:
//   - normally, the relics holding parts you need: farmable ones, ones you
//     hold, and vaulted ones if asked for;
//   - in "my relics" (o), every relic you hold, with all its rewards;
//   - with a search, every relic with a matching reward or name, vaulted or
//     not (you're looking something up, so hiding results would confuse),
//     narrowed to the ones you hold in "my relics".
//
// Relics with the most parts you need come first.
func (m Model) relicEntries() []relicEntry {
	q := strings.ToLower(strings.TrimSpace(m.relicQuery.Value()))
	byRelic := map[*relics.Relic][]relicRow{}
	needed := map[string]bool{} // reward names of needed parts
	for _, it := range m.items {
		for _, c := range it.Parts() {
			if !m.stillNeeded(it, c) {
				continue
			}
			for _, d := range m.drops[partKey(it.ID, c.ID)] {
				needed[d.Reward.Item] = true
				if q == "" && !m.relicMine && (d.Relic.Farmable || m.relicVaulted || m.prog.HeldRelics(d.Relic.Name) > 0) {
					byRelic[d.Relic] = append(byRelic[d.Relic], relicRow{it.Name + " " + m.partLabel(it, c), d.Reward, true})
				}
			}
		}
	}
	if q != "" || m.relicMine {
		for _, r := range m.relics.Relics {
			if m.relicMine && m.prog.HeldRelics(r.Name) == 0 {
				continue
			}
			whole := q == "" || strings.Contains(strings.ToLower(r.Name), q) // "axi s8" shows everything in it
			for _, rw := range r.Rewards {
				if whole || strings.Contains(strings.ToLower(rw.Item), q) {
					byRelic[r] = append(byRelic[r], relicRow{rw.Item, rw, needed[rw.Item]})
				}
			}
		}
	}

	var out []relicEntry
	for r, rows := range byRelic {
		sort.SliceStable(rows, func(i, j int) bool {
			if rows[i].needed != rows[j].needed {
				return rows[i].needed
			}
			return rows[i].reward.Intact > rows[j].reward.Intact
		})
		out = append(out, relicEntry{r, rows})
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.wanted() != b.wanted() {
			return a.wanted() > b.wanted()
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

// wanted is how many of a relic's listed rewards are parts you need.
func (e relicEntry) wanted() int {
	n := 0
	for _, r := range e.rows {
		if r.needed {
			n++
		}
	}
	return n
}

// selectedRelic is the relic under the finder's cursor, or nil.
func (m Model) selectedRelic() *relics.Relic {
	if es := m.relicEntries(); m.relicAt < len(es) {
		return es[m.relicAt].relic
	}
	return nil
}

func (m *Model) openRelics() {
	if m.relics == nil {
		m.fail("No relic data yet: press u to sync the archive.")
		return
	}
	m.modal, m.relicAt, m.relicTop, m.relicMsg = mRelics, 0, 0, ""
}

func (m Model) relicKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.relicCounting {
		return m.countKey(k)
	}
	// While typing a search, keys go to the text box, as in the main list.
	if m.relicTyping {
		switch k.String() {
		case "esc":
			m.relicQuery.SetValue("")
			fallthrough
		case "enter", "down", "up":
			m.relicTyping = false
			m.relicQuery.Blur()
		default:
			var cmd tea.Cmd
			m.relicQuery, cmd = m.relicQuery.Update(k)
			m.relicAt, m.relicTop = 0, 0
			return m, cmd
		}
		return m, nil
	}

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
	case "o":
		m.relicMine = !m.relicMine
		m.relicAt, m.relicTop = 0, 0
	case "+", "=", "-":
		if r := m.selectedRelic(); r != nil {
			step := 1
			if k.String() == "-" {
				step = -1
			}
			m.prog.SetRelics(r.Name, m.prog.HeldRelics(r.Name)+step)
			m.relicMsg, m.relicMsgErr = fmt.Sprintf("%s ×%d", r.Name, m.prog.HeldRelics(r.Name)), false
			m.save()
			m.followRelic(r) // in "my relics", a relic at 0 drops out of the list
		}
	case "c", "#":
		m.relicCounting, m.relicMsg = true, ""
		return m, m.relicCount.Focus()
	case "/":
		m.relicTyping = true
		return m, m.relicQuery.Focus()
	case "esc":
		if m.relicQuery.Value() != "" { // first esc clears the search
			m.relicQuery.SetValue("")
			m.relicAt, m.relicTop = 0, 0
			return m, nil
		}
		fallthrough
	default:
		m.modal = mNone
		return m, m.wantImage()
	}
	m.scrollRelics()
	return m, nil
}

// followRelic keeps the cursor on r after the list changed under it, or
// in place if r has left the list.
func (m *Model) followRelic(r *relics.Relic) {
	es := m.relicEntries()
	for i, e := range es {
		if e.relic == r {
			m.relicAt = i
			return
		}
	}
	m.relicAt = min(m.relicAt, max(len(es)-1, 0))
}

// relicRows is how many lines the finder has for relics, after the
// heading, the footer and the count box when it's open.
func (m Model) relicRows() int {
	n := m.height - 13
	if m.relicCounting {
		n -= 2
	}
	if m.relicMsg != "" {
		n--
	}
	return max(4, n)
}

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
			lines += 1 + len(es[i].rows)
		}
		if lines <= m.relicRows() {
			break
		}
		m.relicTop++
	}
}

func (m Model) relicView() string {
	es := m.relicEntries()
	searching := m.relicQuery.Value() != ""
	var b strings.Builder
	b.WriteString(sTitle.Render("Relic Finder") + "\n")
	var scope string
	switch {
	case m.relicMine && searching:
		scope = "Your relics matching the search"
	case m.relicMine:
		scope = fmt.Sprintf("Your relics · %d held in all", m.heldRelics())
	case searching:
		scope = "Every relic, vaulted too"
	case m.relicVaulted:
		scope = "Relics holding parts you still need · including vaulted"
	default:
		scope = "Relics holding parts you still need · farmable or held"
	}
	b.WriteString(sSub.Render(fmt.Sprintf("%s · %d relics", scope, len(es))) + "\n")
	switch {
	case m.relicTyping:
		b.WriteString(m.relicQuery.View() + "\n\n")
	case searching:
		b.WriteString(sLapis.Render("⌕ "+m.relicQuery.Value()) + "\n\n")
	default:
		b.WriteString(sDim.Render("/ search parts or relics") + "\n\n")
	}
	switch {
	case len(es) > 0:
	case searching:
		b.WriteString(sDim.Render("No relic here holds anything by that name.") + "\n")
	case m.relicMine:
		b.WriteString(sDim.Render("No relics counted yet. Press c and type them in, e.g.") + "\n")
		b.WriteString(sDim.Render("  a s8 12, l n1 4, meso s3 2") + "\n")
	default:
		b.WriteString(sDim.Render("Nothing to farm: every part you're missing is in vaulted relics,") + "\n")
		b.WriteString(sDim.Render("or the parts list is all checked off. Press v to include vaulted.") + "\n")
	}

	// Outside the plain "parts you need" list, rows aren't all needed, so
	// the needed ones get a mark.
	marked := searching || m.relicMine
	lines := 0
	for i := m.relicTop; i < len(es) && lines+1+len(es[i].rows) <= m.relicRows(); i++ {
		e := es[i]
		sel, name := "  ", sText.Bold(true).Render(pad(e.relic.Name, 10))
		if i == m.relicAt {
			sel, name = sGold.Render("▸ "), sTitle.Render(pad(e.relic.Name, 10))
		}
		held := "    "
		if n := m.prog.HeldRelics(e.relic.Name); n > 0 {
			held = sGold.Render(pad(fmt.Sprintf("×%d", n), 4))
		}
		state := sNew.Render("farmable")
		if !e.relic.Farmable {
			state = sDim.Render("vaulted")
		}
		b.WriteString(sel + name + held + " " + sDim.Render(fmt.Sprintf("%d needed  ", e.wanted())) + state + "\n")
		for _, r := range e.rows {
			mark, st := "", sText
			if marked {
				mark, st = "  ", sDim
				if r.needed {
					mark, st = sNew.Render("● "), sText
				}
			}
			b.WriteString("     " + mark + st.Render(pad(trunc(r.name, 40), 41)) + chance(r.reward) + "\n")
		}
		lines += 1 + len(e.rows)
	}

	// The modal's border and padding take 6 columns; long lines are cut so
	// the box never grows past the window.
	w := max(20, m.width-6)
	b.WriteString("\n")
	if m.relicCounting {
		m.relicCount.Width = w - 10 // scrolls sideways past this; m is a copy, so this is just for drawing
		b.WriteString(m.relicCount.View() + "\n")
		b.WriteString(sDim.Render(trunc("e.g. a s8 12 · commas for more · +2/-1 add/remove · bare number: selected", w)) + "\n")
	}
	if m.relicMsg != "" {
		st := sNew
		if m.relicMsgErr {
			st = sErr
		}
		b.WriteString(st.Render(trunc(m.relicMsg, w)) + "\n")
	}
	legend := "chance intact → radiant"
	if marked {
		legend = "● part you still need · " + legend
	}
	b.WriteString(sDim.Render(trunc(legend, w)) + "\n")
	keys := "j/k move · / search · o my relics · c count · +/- one · v vaulted · esc close"
	if m.relicCounting {
		keys = "enter saves and stays open · esc done"
	}
	b.WriteString(sDim.Render(trunc(keys, w)))
	return b.String()
}

// heldRelics is how many relics the user holds in all.
func (m Model) heldRelics() int {
	n := 0
	for _, c := range m.prog.Relics {
		n += c
	}
	return n
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
// details: relics you hold first, then the best farmable ones, or how many
// vaulted relics hold it.
func (m Model) dropLine(ds []relics.Drop, w int) string {
	var have, farm []string
	for _, d := range ds {
		if n := m.prog.HeldRelics(d.Relic.Name); n > 0 {
			have = append(have, fmt.Sprintf("%s ×%d %.0f%%", d.Relic.Name, n, d.Reward.Intact))
		} else if d.Relic.Farmable && len(farm) < 3 {
			farm = append(farm, fmt.Sprintf("%s %.0f%%", d.Relic.Name, d.Reward.Intact))
		}
	}
	switch {
	case len(have) > 0:
		line := sGold.Render("have " + strings.Join(have, " · "))
		if len(farm) > 0 {
			line += sDim.Render(" · ") + sNew.Render(strings.Join(farm, " · "))
		}
		return trunc(line, w)
	case len(farm) > 0:
		return sNew.Render(trunc(strings.Join(farm, " · "), w))
	}
	return sDim.Render(fmt.Sprintf("vaulted · in %d relic%s", len(ds), plural(len(ds), "", "s")))
}

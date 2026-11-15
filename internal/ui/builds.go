package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Peejidle/seshat/internal/builds"
	"github.com/Peejidle/seshat/internal/catalog"
	"github.com/Peejidle/seshat/internal/store"
)

// The Builds screen: the mods and arcanes planned for your warframes and
// weapons, what they cost, and what each item still needs in the game (a
// Reactor or Catalyst, Forma, an Exilus Adapter) so a build can wait until
// you have them. Like adding to Sell, everything is picked from a list.

// buildStage is the step the Builds screen is at.
type buildStage int

const (
	buildList    buildStage = iota
	buildFind               // choosing an item to plan a build for
	buildEdit               // the build itself, slot by slot
	buildPicking            // choosing the mod or arcane for a slot
	buildNoting             // typing the config's name and notes
)

// The views of the list, as tab steps through them.
const (
	buildsAll = iota
	buildsToDo
	buildsDone
)

var buildViews = []string{"All", "To do", "Done"}

// Slot numbers in the editor: the mod slots as package builds numbers
// them, then the arcane slots.
const slotArcane = builds.NumSlots

// buildRow is one build in the list.
type buildRow struct {
	it   catalog.Item
	b    store.Build
	plan builds.Plan
}

// itemByID finds an item a build can be for: one in the archive, or an
// exalted weapon.
func (m Model) itemByID(id string) (catalog.Item, bool) {
	for _, list := range [][]catalog.Item{m.items, m.exalted} {
		for _, it := range list {
			if it.ID == id {
				return it, true
			}
		}
	}
	return catalog.Item{}, false
}

// frameOf is the warframe an exalted weapon belongs to.
func (m Model) frameOf(ex catalog.Item) (catalog.Item, bool) {
	for _, f := range m.items {
		if builds.KindOf(f) != builds.Warframe {
			continue
		}
		for _, e := range catalog.ExaltedFor(f, m.exalted) {
			if e.ID == ex.ID {
				return f, true
			}
		}
	}
	return catalog.Item{}, false
}

// kindName says what an item is in the Builds lists: its category, or
// whose exalted weapon it is.
func (m Model) kindName(it catalog.Item) string {
	if it.Exalted {
		if f, ok := m.frameOf(it); ok {
			return "Exalted · " + f.Name
		}
		return "Exalted"
	}
	return string(it.Category)
}

// nextOfFrame is what X goes to from it: a warframe's first exalted
// weapon, the next one, and after the last, the warframe again.
func (m Model) nextOfFrame(it catalog.Item) (catalog.Item, bool) {
	frame := it
	if it.Exalted {
		f, ok := m.frameOf(it)
		if !ok {
			return it, false
		}
		frame = f
	}
	chain := append([]catalog.Item{frame}, catalog.ExaltedFor(frame, m.exalted)...)
	if len(chain) == 1 {
		return it, false
	}
	for i, c := range chain {
		if c.ID == it.ID {
			return chain[(i+1)%len(chain)], true
		}
	}
	return it, false
}

// buildRows lists the builds in the chosen view: what's still to do first,
// then by name.
func (m Model) buildRows(view int) []buildRow {
	var rows []buildRow
	for id, b := range m.prog.Builds {
		it, ok := m.itemByID(id)
		if !ok {
			continue // gone from the archive
		}
		r := buildRow{it, b, builds.Work(m.bdata, it, b)}
		switch {
		case view == buildsToDo && r.plan.Done(), view == buildsDone && !r.plan.Done():
			continue
		}
		rows = append(rows, r)
	}
	sort.Slice(rows, func(i, j int) bool {
		if a, b := rows[i].plan.Done(), rows[j].plan.Done(); a != b {
			return b
		}
		return rows[i].it.Name < rows[j].it.Name
	})
	return rows
}

// buildIndex is where the selected build is in rows: the first, if none
// is selected yet or it isn't in this view.
func (m Model) buildIndex(rows []buildRow) int {
	for i, r := range rows {
		if r.it.ID == m.buildListID {
			return i
		}
	}
	return 0
}

func (m Model) buildRowsShown() int { return max(3, m.height-12) }

// openBuilds opens the list of builds.
func (m *Model) openBuilds() {
	m.modal, m.buildStep, m.buildMsg, m.buildBack = mBuilds, buildList, "", mNone
	if m.bdata == nil {
		m.buildMsg, m.buildMsgErr = "The mod data isn't synced yet: close this and press u.", true
	}
}

// openBuild opens the build for it, starting one if there isn't one.
// back is where esc goes from it: mDetail when opened from the details,
// otherwise the list.
func (m *Model) openBuild(it catalog.Item, back modal) bool {
	if builds.KindOf(it) == builds.NoKind {
		m.fail("Builds are for warframes and primary, secondary and melee weapons.")
		return false
	}
	if m.bdata == nil {
		m.fail("The mod data isn't synced yet: press u.")
		return false
	}
	if _, ok := m.prog.Build(it.ID); !ok {
		m.prog.SetBuild(it.ID, builds.New(it))
		m.save()
	}
	m.modal, m.buildStep, m.buildBack = mBuilds, buildEdit, back
	m.buildID, m.buildSlot, m.buildSwap, m.buildMsg, m.buildSure = it.ID, 0, -1, "", ""
	m.buildListID = it.ID
	return true
}

func (m Model) buildKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.buildStep {
	case buildFind:
		return m.buildFindKey(k)
	case buildEdit:
		return m.buildEditKey(k)
	case buildPicking:
		return m.buildPickKey(k)
	case buildNoting:
		return m.buildNoteKey(k)
	}
	rows := m.buildRows(m.buildView)
	at := m.buildIndex(rows)
	move := func(i int) {
		if len(rows) > 0 {
			m.buildListID = rows[min(max(i, 0), len(rows)-1)].it.ID
		}
	}
	switch k.String() {
	case "down", "j":
		move(at + 1)
	case "up", "k":
		move(at - 1)
	case "g", "home":
		move(0)
	case "G", "end":
		move(len(rows) - 1)
	case "tab", "right", "l", "shift+tab", "left", "h":
		step := 1
		if s := k.String(); s == "shift+tab" || s == "left" || s == "h" {
			step = len(buildViews) - 1
		}
		m.buildView = (m.buildView + step) % len(buildViews)
		m.buildTop = 0
	case "a":
		if m.bdata != nil {
			return m, m.startBuildFind()
		}
	case "enter", "e":
		if len(rows) > 0 {
			m.openBuild(rows[at].it, mNone)
		}
		return m, nil
	default:
		m.modal = mNone
		return m, m.wantImage()
	}
	// Keep the selection in view.
	at = m.buildIndex(m.buildRows(m.buildView))
	if at < m.buildTop {
		m.buildTop = at
	}
	if at >= m.buildTop+m.buildRowsShown() {
		m.buildTop = at - m.buildRowsShown() + 1
	}
	return m, nil
}

// ---- choosing an item --------------------------------------------------

func (m *Model) startBuildFind() tea.Cmd {
	m.buildStep, m.buildPick, m.buildMsg = buildFind, 0, ""
	m.buildIn.Placeholder = "a warframe or weapon, e.g. limbo prime"
	m.buildIn.SetValue("")
	return m.buildIn.Focus()
}

// words is whether name has every word of the query in it, ignoring case.
func words(name, query string) bool {
	name = strings.ToLower(name)
	for _, w := range strings.Fields(strings.ToLower(query)) {
		if !strings.Contains(name, w) {
			return false
		}
	}
	return true
}

func (m Model) buildFindMatches() []catalog.Item {
	q := m.buildIn.Value()
	if strings.TrimSpace(q) == "" {
		return nil
	}
	var out []catalog.Item
	for _, list := range [][]catalog.Item{m.items, m.exalted} {
		for _, it := range list {
			if builds.KindOf(it) != builds.NoKind && words(it.Name, q) {
				out = append(out, it)
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (m Model) buildFindKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	found := m.buildFindMatches()
	switch k.String() {
	case "esc":
		m.buildStep = buildList
		m.buildIn.Blur()
		return m, nil
	case "down", "tab", "ctrl+n":
		m.buildPick = min(m.buildPick+1, max(len(found)-1, 0))
		return m, nil
	case "up", "shift+tab", "ctrl+p":
		m.buildPick = max(m.buildPick-1, 0)
		return m, nil
	case "enter":
		if len(found) > 0 {
			m.buildIn.Blur()
			m.openBuild(found[min(m.buildPick, len(found)-1)], mNone)
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.buildIn, cmd = m.buildIn.Update(k)
	m.buildPick = 0
	return m, cmd
}

// ---- the build ----------------------------------------------------------

// editing is the item and build open in the editor.
func (m Model) editing() (catalog.Item, store.Build, bool) {
	it, ok := m.itemByID(m.buildID)
	if !ok {
		return it, store.Build{}, false
	}
	b, ok := m.prog.Build(m.buildID)
	return it, b, ok
}

// buildGrid is how the editor lays out an item's slots, as the game does
// near enough: the aura (or stance) and exilus, the eight mods in pairs,
// then the arcanes. -1 is a gap.
func buildGrid(k builds.Kind) [][2]int {
	top := [2]int{-1, builds.SlotExilus}
	if k.HasAura() {
		top[0] = builds.SlotAura
	}
	g := [][2]int{top}
	for i := 0; i < builds.NumMods; i += 2 {
		g = append(g, [2]int{i, i + 1})
	}
	arc := [2]int{slotArcane, -1}
	if k.ArcaneSlots() > 1 {
		arc[1] = slotArcane + 1
	}
	return append(g, arc)
}

// moveSlot moves the editor's selection one step through the grid.
func (m *Model) moveSlot(k builds.Kind, dr, dc int) {
	g := buildGrid(k)
	r, c := 0, 0
	for i, row := range g {
		for j, s := range row {
			if s == m.buildSlot {
				r, c = i, j
			}
		}
	}
	if dc != 0 {
		if s := g[r][1-c]; s >= 0 {
			m.buildSlot = s
		}
		return
	}
	for nr := r + dr; nr >= 0 && nr < len(g); nr += dr {
		switch {
		case g[nr][c] >= 0:
			m.buildSlot = g[nr][c]
			return
		case g[nr][1-c] >= 0:
			m.buildSlot = g[nr][1-c]
			return
		}
	}
}

func (m Model) buildEditKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	it, b, ok := m.editing()
	if !ok {
		m.buildStep = buildList
		return m, nil
	}
	kind := builds.KindOf(it)
	key := k.String()
	if key != m.buildSure {
		m.buildSure = "" // pressing anything else calls off a delete or copy
	}
	cfg := &b.Configs[b.Active]
	m.buildMsg = ""
	save := func() {
		m.prog.SetBuild(it.ID, b)
		m.save()
	}
	isMod := m.buildSlot < slotArcane
	switch key {
	case "up", "k":
		m.moveSlot(kind, -1, 0)
	case "down", "j":
		m.moveSlot(kind, 1, 0)
	case "left", "h", "right", "l":
		m.moveSlot(kind, 0, 1)
	case "enter":
		if m.buildSwap >= 0 {
			return m.swapSlots(it, b)
		}
		m.buildStep, m.buildPick = buildPicking, 0
		m.buildIn.Placeholder = "type to narrow, e.g. serration"
		if !isMod {
			m.buildIn.Placeholder = "type to narrow, e.g. energize"
		}
		m.buildIn.SetValue("")
		return m, m.buildIn.Focus()
	case "m", "M":
		if m.buildSwap >= 0 {
			return m.swapSlots(it, b)
		}
		switch {
		case key == "m" && isMod:
			m.buildSwap, m.buildSwapAll = m.buildSlot, false
			m.buildMsg, m.buildMsgErr = "Moving the mod: go to its new slot and press m or enter. esc to stop.", false
		case key == "M" && m.buildSlot < builds.NumMods:
			m.buildSwap, m.buildSwapAll = m.buildSlot, true
			m.buildMsg, m.buildMsgErr = "Moving the slot, polarity and all: go to where it should be and press M or enter. esc to stop.", false
		case key == "M":
			m.buildMsg, m.buildMsgErr = "Only the eight ordinary slots move around: the aura, exilus and arcanes stay put.", false
		}
	case "x", "delete", "backspace":
		if isMod {
			cfg.Mods[m.buildSlot] = store.BuildMod{}
		} else {
			cfg.Arcanes[m.buildSlot-slotArcane] = store.BuildMod{}
		}
		save()
	case "]", "+", "=", "[", "-":
		step := 1
		if key == "[" || key == "-" {
			step = -1
		}
		if isMod {
			s := &cfg.Mods[m.buildSlot]
			if md, ok := m.bdata.Mod(s.ID); ok {
				s.Rank = min(max(s.Rank+step, 0), md.MaxRank)
			}
		} else {
			a := &cfg.Arcanes[m.buildSlot-slotArcane]
			if ar, ok := m.bdata.Arcane(a.ID); ok {
				a.Rank = min(max(a.Rank+step, 0), ar.MaxRank)
			}
		}
		save()
	case "p", "P":
		if isMod {
			ps := builds.SlotPolarities(kind, m.buildSlot)
			step := 1
			if key == "P" {
				step = len(ps) - 1
			}
			s := &b.Slots[m.buildSlot]
			s.Polarity = string(nextPolarity(ps, builds.Polarity(s.Polarity), builds.Polarity(s.Has), step))
			save()
		}
	case "f":
		if isMod {
			s := &b.Slots[m.buildSlot]
			if s.Polarity == s.Has {
				m.buildMsg, m.buildMsgErr = "This slot has no Forma planned: p plans one.", false
			} else {
				s.Has = s.Polarity
				save()
				m.buildMsg, m.buildMsgErr = "✓ Forma done: the slot is "+builds.Polarity(s.Has).Title()+" in the game now.", false
			}
		}
	case "r":
		b.Reactor = !b.Reactor
		save()
		m.buildMsg, m.buildMsgErr = "✓ "+kind.Reactor()+": not installed yet.", false
		if b.Reactor {
			m.buildMsg = "✓ " + kind.Reactor() + " installed."
		}
	case "u":
		// The exilus and arcane slots are locked until an adapter is
		// used on them in the game.
		m.buildMsgErr = false
		switch {
		case m.buildSlot == builds.SlotExilus:
			b.Exilus = !b.Exilus
			m.buildMsg = "✓ Exilus slot: still locked."
			if b.Exilus {
				m.buildMsg = "✓ Exilus slot unlocked."
			}
		case !isMod:
			open := &b.ArcanesUnlocked[m.buildSlot-slotArcane]
			*open = !*open
			m.buildMsg = "✓ Arcane slot: still locked."
			if *open {
				m.buildMsg = "✓ Arcane slot unlocked."
			}
		default:
			m.buildMsg = "Only the exilus and arcane slots need unlocking: select one, then u."
			return m, nil
		}
		save()
	case "X":
		next, ok := m.nextOfFrame(it)
		if !ok {
			m.buildMsg, m.buildMsgErr = "X goes between a warframe and its exalted weapons; this has none.", false
			return m, nil
		}
		m.openBuild(next, m.buildBack)
		return m, nil
	case "n":
		m.buildStep, m.buildOnName = buildNoting, cfg.Name == ""
		m.buildName.SetValue(cfg.Name)
		m.buildNotes.SetValue(cfg.Notes)
		m.buildName.CursorEnd()
		m.buildNotes.CursorEnd()
		return m, m.focusNoteBox()
	case "c":
		b.Active = (b.Active + 1) % len(b.Configs)
		save()
		m.buildMsg, m.buildMsgErr = "Config "+configLabel(b.Active, b.Configs[b.Active])+".", false
		if configEmpty(b.Configs[b.Active]) && b.Active > 0 {
			m.buildMsg += " It's empty: C copies config " + configName(b.Active-1) + " into it."
		}
	case "C":
		if b.Active == 0 {
			m.buildMsg, m.buildMsgErr = "C copies the config before into this one, and A is the first: c goes to B.", false
			return m, nil
		}
		from := configName(b.Active - 1)
		if !configEmpty(*cfg) && m.buildSure != "C" {
			m.buildSure = "C"
			m.buildMsg, m.buildMsgErr = "Press C again to replace config "+configName(b.Active)+" with a copy of "+from+".", true
			return m, nil
		}
		m.buildSure = ""
		src := b.Configs[b.Active-1]
		*cfg = store.BuildConfig{
			Mods:    append([]store.BuildMod(nil), src.Mods...),
			Arcanes: append([]store.BuildMod(nil), src.Arcanes...),
			Notes:   src.Notes,
		}
		save()
		m.buildMsg, m.buildMsgErr = "✓ Config "+configName(b.Active)+" is now a copy of "+from+".", false
	case "D":
		if m.buildSure != "D" {
			m.buildSure = "D"
			m.buildMsg, m.buildMsgErr = "Press D again to delete this build.", true
			return m, nil
		}
		m.prog.DeleteBuild(it.ID)
		m.save()
		m.buildSure = ""
		m.buildStep = buildList
		m.say("Deleted the build for %s.", it.Name)
		m.buildMsg, m.buildMsgErr = "✓ Deleted the build for "+it.Name+".", false
		if m.buildBack == mDetail {
			m.modal = mDetail
		}
	case "esc":
		switch {
		case m.buildSwap >= 0:
			m.buildSwap = -1
		case m.buildBack == mDetail:
			m.modal = mDetail
			return m, m.wantImage()
		default:
			m.buildStep = buildList
		}
	}
	return m, nil
}

// configName is a config's letter, as the game names them: A, B, C.
func configName(i int) string { return string(rune('A' + i)) }

// configLabel is a config's letter and, if it has one, its name:
// "A", or "B · Steel Path".
func configLabel(i int, c store.BuildConfig) string {
	if c.Name == "" {
		return configName(i)
	}
	return configName(i) + " · " + c.Name
}

// configEmpty is whether a config has no mods or arcanes planned.
func configEmpty(c store.BuildConfig) bool {
	for _, list := range [][]store.BuildMod{c.Mods, c.Arcanes} {
		for _, x := range list {
			if x.ID != "" {
				return false
			}
		}
	}
	return true
}

// nextPolarity is the polarity step places after p in ps. A slot with a
// polarity in the game can't be made unpolarized again, so "none" is
// skipped unless it has none.
func nextPolarity(ps []builds.Polarity, p, has builds.Polarity, step int) builds.Polarity {
	at := 0
	for i, q := range ps {
		if q == p {
			at = i
		}
	}
	for range ps {
		at = (at + step) % len(ps)
		if ps[at] != builds.None || has == builds.None {
			return ps[at]
		}
	}
	return p
}

// swapSlots puts the mod being moved into the selected slot, and what was
// there into the slot it came from. With m the slots keep their
// polarities, as in the game; with M the polarities move too, for laying
// the slots out as the game has them, which WFCD doesn't say. As that's
// about the item, M moves the slot's mod in every config.
func (m Model) swapSlots(it catalog.Item, b store.Build) (tea.Model, tea.Cmd) {
	from, to := m.buildSwap, m.buildSlot
	m.buildSwap = -1
	if to >= slotArcane || from == to {
		return m, nil
	}
	if m.buildSwapAll {
		if to >= builds.NumMods {
			m.buildMsg, m.buildMsgErr = "A slot can only swap places with another of the eight ordinary ones.", true
			return m, nil
		}
		b.Slots[from], b.Slots[to] = b.Slots[to], b.Slots[from]
		for _, cfg := range b.Configs {
			cfg.Mods[from], cfg.Mods[to] = cfg.Mods[to], cfg.Mods[from]
		}
		m.prog.SetBuild(it.ID, b)
		m.save()
		return m, nil
	}
	mods := b.Configs[b.Active].Mods
	for _, pair := range [][2]int{{from, to}, {to, from}} {
		if md, ok := m.bdata.Mod(mods[pair[0]].ID); ok && !builds.Fits(md, it, pair[1]) {
			m.buildMsg, m.buildMsgErr = md.Name+" can't go in that slot.", true
			return m, nil
		}
	}
	mods[from], mods[to] = mods[to], mods[from]
	m.prog.SetBuild(it.ID, b)
	m.save()
	return m, nil
}

// ---- choosing a mod or arcane ---------------------------------------------

// pickRow is one choice in the mod or arcane list.
type pickRow struct {
	id, name string
	pol      builds.Polarity
	cost     int    // in this slot at max rank; mods only
	note     string // e.g. "in another slot"
	maxRank  int
}

func (m Model) pickRows(it catalog.Item, b store.Build) []pickRow {
	q := m.buildIn.Value()
	var rows []pickRow
	if m.buildSlot >= slotArcane {
		for _, a := range m.bdata.Arcanes {
			if !builds.ArcaneFits(a, it) || !words(a.Name, q) {
				continue
			}
			r := pickRow{id: a.ID, name: a.Name, maxRank: a.MaxRank}
			for i, have := range b.Config().Arcanes {
				if have.ID == a.ID && i != m.buildSlot-slotArcane {
					r.note = "in the other slot"
				}
			}
			rows = append(rows, r)
		}
		return rows
	}
	slotPol := builds.Polarity(b.Slots[m.buildSlot].Polarity)
	for _, md := range m.bdata.Mods {
		if !builds.Fits(md, it, m.buildSlot) || !words(md.Name, q) {
			continue
		}
		r := pickRow{id: md.ID, name: md.Name, pol: md.Polarity, maxRank: md.MaxRank,
			cost: builds.Drain(md, md.MaxRank, slotPol)}
		for i, s := range b.Config().Mods {
			if s.ID == md.ID && i != m.buildSlot {
				r.note = "in another slot: moves here"
			}
		}
		rows = append(rows, r)
	}
	return rows
}

func (m Model) buildPickKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	it, b, ok := m.editing()
	if !ok {
		m.buildStep = buildList
		return m, nil
	}
	rows := m.pickRows(it, b)
	switch k.String() {
	case "esc":
		m.buildStep = buildEdit
		m.buildIn.Blur()
		return m, nil
	case "down", "tab", "ctrl+n":
		m.buildPick = min(m.buildPick+1, max(len(rows)-1, 0))
		return m, nil
	case "up", "shift+tab", "ctrl+p":
		m.buildPick = max(m.buildPick-1, 0)
		return m, nil
	case "pgdown":
		m.buildPick = min(m.buildPick+10, max(len(rows)-1, 0))
		return m, nil
	case "pgup":
		m.buildPick = max(m.buildPick-10, 0)
		return m, nil
	case "enter":
		if len(rows) == 0 {
			return m, nil
		}
		r := rows[min(m.buildPick, len(rows)-1)]
		cfg := b.Config()
		list, at := cfg.Mods, m.buildSlot
		if m.buildSlot >= slotArcane {
			list, at = cfg.Arcanes, m.buildSlot-slotArcane
		}
		// A mod goes on once, and the same arcane twice doesn't stack, so
		// one already in moves here.
		for i := range list {
			if list[i].ID == r.id {
				list[i] = store.BuildMod{}
			}
		}
		list[at] = store.BuildMod{ID: r.id, Rank: r.maxRank}
		m.prog.SetBuild(it.ID, b)
		m.save()
		m.buildStep = buildEdit
		m.buildIn.Blur()
		m.buildMsg, m.buildMsgErr = fmt.Sprintf("✓ %s at rank %d: [ and ] change the rank.", r.name, r.maxRank), false
		return m, nil
	}
	var cmd tea.Cmd
	m.buildIn, cmd = m.buildIn.Update(k)
	m.buildPick = 0
	return m, cmd
}

// focusNoteBox puts the cursor in the name or notes box, whichever
// buildOnName says.
func (m *Model) focusNoteBox() tea.Cmd {
	m.buildName.Blur()
	m.buildNotes.Blur()
	if m.buildOnName {
		return m.buildName.Focus()
	}
	return m.buildNotes.Focus()
}

// buildNoteKey handles the n form: the config's name and its notes, two
// boxes saved together.
func (m Model) buildNoteKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "esc":
		m.buildStep = buildEdit
		m.buildName.Blur()
		m.buildNotes.Blur()
		return m, nil
	case "tab", "shift+tab", "up", "down":
		m.buildOnName = !m.buildOnName
		return m, m.focusNoteBox()
	case "enter":
		if it, b, ok := m.editing(); ok {
			cfg := &b.Configs[b.Active]
			cfg.Name = strings.TrimSpace(m.buildName.Value())
			cfg.Notes = strings.TrimSpace(m.buildNotes.Value())
			m.prog.SetBuild(it.ID, b)
			m.save()
			m.buildMsg, m.buildMsgErr = "✓ Saved config "+configLabel(b.Active, *cfg)+".", false
		}
		m.buildStep = buildEdit
		m.buildName.Blur()
		m.buildNotes.Blur()
		return m, nil
	}
	var cmd tea.Cmd
	if m.buildOnName {
		m.buildName, cmd = m.buildName.Update(k)
	} else {
		m.buildNotes, cmd = m.buildNotes.Update(k)
	}
	return m, cmd
}

// ---- drawing ------------------------------------------------------------

func (m Model) buildsView() string {
	w := max(20, m.width-6)
	var lines []string
	keys := ""
	switch m.buildStep {
	case buildList:
		lines, keys = m.buildListLines(w), "a new build · enter open · tab view · j/k move · esc close"
	case buildFind:
		lines, keys = m.buildFindLines(w), "type part of the name · ↑/↓ choose · enter open · esc back"
	case buildEdit:
		lines = m.buildEditLines(w)
		keys = "enter pick · x clear · [ ] rank · m move mod · M move slot · X exalted · n name & notes · D delete · esc back\n" +
			"p/P plan a polarity · f its Forma's done · r reactor's in · u slot's unlocked (exilus, arcane)"
	case buildPicking:
		lines, keys = m.buildPickLines(w), "type to narrow · ↑/↓ choose · enter put it in · esc back"
	case buildNoting:
		lines, keys = m.buildEditLines(w), "tab other box · enter save both · esc cancel"
	}
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(l + "\n")
	}
	b.WriteString(strings.Repeat("\n", max(m.height-6-len(lines)-strings.Count(keys, "\n"), 0)))
	if m.buildMsg != "" {
		st := sText
		if m.buildMsgErr {
			st = sErr
		}
		b.WriteString(st.Render(trunc(m.buildMsg, w)))
	}
	for _, l := range strings.Split(keys, "\n") {
		b.WriteString("\n" + sDim.Render(trunc(l, w)))
	}
	return b.String()
}

func (m Model) buildListLines(w int) []string {
	lines := []string{
		sTitle.Render("Builds"),
		sSub.Render(trunc("What each warframe and weapon is to become, and what it still needs", w)),
	}
	var tabs []string
	for i, name := range buildViews {
		name = fmt.Sprintf("%s %d", name, len(m.buildRows(i)))
		if i == m.buildView {
			tabs = append(tabs, sTitle.Render("["+name+"]"))
		} else {
			tabs = append(tabs, sDim.Render(" "+name+" "))
		}
	}
	lines = append(lines, strings.Join(tabs, " "), "")
	if m.bdata == nil {
		return lines
	}
	rows := m.buildRows(m.buildView)
	if len(rows) == 0 {
		msg := "No builds yet. Press a to plan one, or b on an item's details."
		if len(m.prog.Builds) > 0 {
			msg = "Nothing in this view. Press tab for the others."
		}
		return append(lines, sDim.Render(trunc(msg, w)))
	}
	at := m.buildIndex(rows)
	for i := m.buildTop; i < len(rows) && i < m.buildTop+m.buildRowsShown(); i++ {
		r := rows[i]
		sel, name := "  ", sText.Render(pad(trunc(r.it.Name, 28), 30))
		if i == at {
			sel, name = sGold.Render("▸ "), sTitle.Render(pad(trunc(r.it.Name, 28), 30))
		}
		kind := sDim.Render(pad(trunc(m.kindName(r.it), 27), 29))
		state := sGold.Render("needs " + strings.Join(r.plan.Missing(builds.KindOf(r.it)), " · "))
		switch {
		case r.plan.Free() < 0:
			state = sErr.Render(fmt.Sprintf("%d over capacity", -r.plan.Free()))
		case r.plan.Done():
			state = sDim.Render("✓ ready: everything's in")
		}
		lines = append(lines, trunc(sel+name+kind+state, w))
	}
	return lines
}

func (m Model) buildFindLines(w int) []string {
	lines := []string{sTitle.Render("New build: for which warframe or weapon?"), m.buildIn.View(), ""}
	found := m.buildFindMatches()
	room := m.buildRowsShown()
	switch {
	case strings.TrimSpace(m.buildIn.Value()) == "":
		return append(lines, sDim.Render("Any words of its name will do."))
	case len(found) == 0:
		return append(lines, sErr.Render("No warframe or primary, secondary or melee weapon matches that."))
	}
	pick := min(m.buildPick, len(found)-1)
	top := max(0, pick-room+1)
	for i := top; i < len(found) && i < top+room; i++ {
		it := found[i]
		sel, name := "  ", sText.Render(pad(trunc(it.Name, 34), 36))
		if i == pick {
			sel, name = sGold.Render("▸ "), sTitle.Render(pad(trunc(it.Name, 34), 36))
		}
		note := m.kindName(it)
		if _, ok := m.prog.Build(it.ID); ok {
			note += " · has a build"
		}
		lines = append(lines, trunc(sel+name+sDim.Render(note), w))
	}
	return lines
}

// buildHeader is the item's name and what the build adds up to.
func (m Model) buildHeader(it catalog.Item, b store.Build, w int) []string {
	kind := builds.KindOf(it)
	p := builds.Work(m.bdata, it, b)
	head := sTitle.Render(it.Name) + sDim.Render(" · build")
	if it.Exalted {
		head += sDim.Render(" · " + m.kindName(it))
	}
	if next, ok := m.nextOfFrame(it); ok {
		head += sDim.Render(" · ") + sKey.Render("X") + sDim.Render(" "+next.Name)
	}
	capText := fmt.Sprintf("Capacity %d of %d", p.Used, p.Capacity+p.Bonus)
	if p.Bonus > 0 {
		capText += fmt.Sprintf(" (%d + %d from the %s)", p.Capacity, p.Bonus, strings.ToLower(kind.SlotName(builds.SlotAura)))
	}
	capLine := sText.Render(capText)
	switch free := p.Free(); {
	case free < 0:
		capLine += sErr.Render(fmt.Sprintf(" · %d over, even with the %s", -free, kind.Reactor()))
	default:
		capLine += sDim.Render(fmt.Sprintf(" · %d free", free))
	}
	need := sDim.Render("✓ The item has everything: this build can go on now.")
	if miss := p.Missing(kind); len(miss) > 0 {
		need = sGold.Render("Needs " + strings.Join(miss, " · "))
	}
	for _, c := range p.Over {
		if c != b.Active {
			need += sErr.Render(" · config " + configName(c) + " doesn't fit")
		}
	}
	have := func(what string, yes bool, key string) string {
		if yes {
			return sText.Render(what+" ✓") + sDim.Render(" ("+key+")")
		}
		return sDim.Render(what + " ✗ (" + key + ")")
	}
	open := 0
	for _, u := range b.ArcanesUnlocked {
		if u {
			open++
		}
	}
	arcanes := fmt.Sprintf("arcane slots %d/%d unlocked", open, len(b.ArcanesUnlocked))
	if len(b.ArcanesUnlocked) == 1 {
		arcanes = "arcane slot unlocked"
	}
	inGame := sDim.Render("In the game: ") + have(kind.Reactor(), b.Reactor, "r") + sDim.Render(" · ") +
		have("exilus unlocked", b.Exilus, "u") + sDim.Render(" · ") +
		have(arcanes, open == len(b.ArcanesUnlocked), "u")
	return []string{trunc(head, w), m.configTabs(b), trunc(capLine, w), trunc(need, w), trunc(inGame, w), ""}
}

// configTabs shows the configs, the one being looked at picked out and
// empty ones dimmed.
func (m Model) configTabs(b store.Build) string {
	tabs := sDim.Render("Config ")
	for i, c := range b.Configs {
		name := configLabel(i, c)
		switch {
		case i == b.Active:
			tabs += sTabOn.Render(name)
		case configEmpty(c):
			tabs += sTab.Render(name)
		default:
			tabs += sText.Padding(0, 1).Render(name)
		}
	}
	return tabs + sDim.Render("  c switch · C copy the one before · n name it")
}

func (m Model) buildEditLines(w int) []string {
	it, b, ok := m.editing()
	if !ok {
		return []string{sErr.Render("That build's item is gone from the archive.")}
	}
	kind := builds.KindOf(it)
	lines := m.buildHeader(it, b, w)
	cellW := min(42, (w-2)/2)
	for row, pair := range buildGrid(kind) {
		if row == 1 || row == len(buildGrid(kind))-1 {
			lines = append(lines, "")
		}
		var cells []string
		for _, s := range pair {
			cells = append(cells, pad(m.slotCell(it, b, s, cellW), cellW))
		}
		lines = append(lines, strings.Join(cells, "  "))
	}
	lines = append(lines, "")
	if m.buildStep == buildNoting {
		mark := func(on bool) string {
			if on {
				return sGold.Render("▸ ")
			}
			return "  "
		}
		lines = append(lines,
			mark(m.buildOnName)+sText.Render("Name:  ")+m.buildName.View(),
			mark(!m.buildOnName)+sText.Render("Notes: ")+m.buildNotes.View())
	} else if notes := b.Config().Notes; notes != "" {
		lines = append(lines, sDim.Render("Notes: ")+sText.Render(trunc(notes, w-7)))
	} else {
		lines = append(lines, sDim.Render("Notes: none yet (n): what this config's for, what to swap in later"))
	}
	lines = append(lines, "")
	lines = append(lines, m.slotDetail(it, b, w)...)
	lines = append(lines, "")
	return append(lines, sDim.Render(trunc("M Madurai · V Vazarin · N Naramon · Z Zenurik · U Umbra · A Any · ◆ Forma to do", w)))
}

// slotCell draws one slot: its polarity, the mod, its rank and cost.
func (m Model) slotCell(it catalog.Item, b store.Build, slot, w int) string {
	if slot < 0 {
		return ""
	}
	sel := "  "
	switch {
	case slot == m.buildSwap:
		sel = sErr.Render("↕ ")
	case slot == m.buildSlot:
		sel = sGold.Render("▸ ")
	}
	nameSt := sText
	if slot == m.buildSlot {
		nameSt = sTitle
	}
	if slot >= slotArcane {
		a, open := b.Config().Arcanes[slot-slotArcane], b.ArcanesUnlocked[slot-slotArcane]
		ar, ok := m.bdata.Arcane(a.ID)
		if !ok {
			return sel + sDim.Render("[✦]  Arcane: empty") + locked(open)
		}
		return sel + sDim.Render("[✦]  ") + nameSt.Render(trunc(ar.Name, w-22)) + sDim.Render(fmt.Sprintf(" R%d", a.Rank)) + locked(open)
	}
	s, mod := b.Slots[slot], b.Config().Mods[slot]
	pol := builds.Polarity(s.Polarity)
	box := sDim.Render("[" + pol.Symbol() + "]")
	if s.Polarity != s.Has {
		box = sGold.Render("[" + pol.Symbol() + "]◆")
	} else {
		box += " "
	}
	label := kindLabel(builds.KindOf(it), slot)
	if slot == builds.SlotExilus && !b.Exilus {
		label = "Exilus (locked): "
	}
	md, ok := m.bdata.Mod(mod.ID)
	if !ok {
		return sel + box + " " + sDim.Render(label+"empty")
	}
	cost := builds.Drain(md, mod.Rank, pol)
	costSt := sText
	switch {
	case pol == builds.None:
	case pol == md.Polarity:
		costSt = sLapis // matched: cheaper, or more capacity
	default:
		costSt = sErr
	}
	costText := fmt.Sprintf("%3d", cost)
	if cost < 0 {
		costText = fmt.Sprintf("+%d", -cost)
	}
	nameW := w - 13 - len(label)
	return sel + box + " " + sDim.Render(label) + nameSt.Render(pad(trunc(md.Name, nameW), nameW)) +
		sDim.Render(fmt.Sprintf("R%-2d", mod.Rank)) + costSt.Render(pad(costText, 3))
}

// locked marks a slot still locked in the game.
func locked(unlocked bool) string {
	if unlocked {
		return ""
	}
	return sGold.Render(" · locked")
}

func kindLabel(k builds.Kind, slot int) string {
	if n := k.SlotName(slot); n != "" {
		return n + ": "
	}
	return ""
}

// slotDetail sets out the selected slot's mod or arcane in full, a few
// lines long: what it is and costs, the slot, then what it does, a line
// for each of its stats, wrapped.
func (m Model) slotDetail(it catalog.Item, b store.Build, w int) []string {
	if m.buildSlot >= slotArcane {
		a := b.Config().Arcanes[m.buildSlot-slotArcane]
		ar, ok := m.bdata.Arcane(a.ID)
		if !ok {
			return []string{sDim.Render("Empty arcane slot: enter to pick one.")}
		}
		head := sTitle.Render(ar.Name) + sDim.Render(fmt.Sprintf(" · %s · rank %d of %d", ar.Type, a.Rank, ar.MaxRank))
		return append([]string{trunc(head, w)}, statLines(ar.Stats, a.Rank, w)...)
	}
	s, mod := b.Slots[m.buildSlot], b.Config().Mods[m.buildSlot]
	pol, has := builds.Polarity(s.Polarity), builds.Polarity(s.Has)
	slot := "Slot: " + pol.Title()
	if s.Polarity != s.Has {
		slot += " · a Forma from " + has.Title() + " still to do (f once it's done)"
	}
	md, ok := m.bdata.Mod(mod.ID)
	if !ok {
		return []string{sDim.Render(trunc(slot, w)), sDim.Render("Empty: enter to pick a mod.")}
	}
	cost := builds.Drain(md, mod.Rank, pol)
	costText := fmt.Sprintf("costs %d here", cost)
	if cost < 0 {
		costText = fmt.Sprintf("gives %d capacity here", -cost)
	}
	switch {
	case pol == builds.None:
	case pol == md.Polarity || pol == builds.Universal:
		slot += sLapis.Render(" · matches, so it costs less")
		if cost < 0 {
			slot = "Slot: " + pol.Title() + sLapis.Render(" · matches, so it gives double")
		}
	default:
		slot += sErr.Render(" · doesn't match its " + md.Polarity.Title())
	}
	head := sTitle.Render(md.Name) + sDim.Render(fmt.Sprintf(" · %s · rank %d of %d · %s",
		md.Polarity.Title(), mod.Rank, md.MaxRank, costText))
	return append([]string{trunc(head, w), sDim.Render(trunc(slot, w))}, statLines(md.Stats, mod.Rank, w)...)
}

// statLines is what a mod or arcane does at rank: each stat on its own
// line, wrapped to w with the continuation lines indented.
func statLines(stats [][]string, rank, w int) []string {
	if rank < 0 || rank >= len(stats) {
		return nil
	}
	var lines []string
	for _, st := range stats[rank] {
		wrapped := lipgloss.NewStyle().Width(max(w-4, 10)).Render(st)
		for i, l := range strings.Split(wrapped, "\n") {
			lead := "    "
			if i == 0 {
				lead = sGold.Render("  • ")
			}
			lines = append(lines, lead+sText.Render(strings.TrimRight(l, " ")))
		}
	}
	return lines
}

func (m Model) buildPickLines(w int) []string {
	it, b, ok := m.editing()
	if !ok {
		return nil
	}
	what := "mod"
	if m.buildSlot >= slotArcane {
		what = "arcane"
	} else if n := builds.KindOf(it).SlotName(m.buildSlot); n != "" {
		what = strings.ToLower(n) + " mod"
	}
	lines := []string{sTitle.Render(trunc(fmt.Sprintf("%s: which %s?", it.Name, what), w)), m.buildIn.View(), ""}
	rows := m.pickRows(it, b)
	if len(rows) == 0 {
		return append(lines, sErr.Render("Nothing that fits matches that."))
	}
	if m.buildSlot < slotArcane {
		pol := builds.Polarity(b.Slots[m.buildSlot].Polarity)
		lines = append(lines, sDim.Render(trunc(fmt.Sprintf("Cost at max rank in this %s slot", pol.Title()), w)))
	}
	room := m.buildRowsShown() - 2
	pick := min(m.buildPick, len(rows)-1)
	top := max(0, pick-room+1)
	for i := top; i < len(rows) && i < top+room; i++ {
		r := rows[i]
		sel, name := "  ", sText.Render(pad(trunc(r.name, 34), 36))
		if i == pick {
			sel, name = sGold.Render("▸ "), sTitle.Render(pad(trunc(r.name, 34), 36))
		}
		info := ""
		if m.buildSlot < slotArcane {
			c := fmt.Sprintf("%3d", r.cost)
			if r.cost < 0 {
				c = fmt.Sprintf(" +%d", -r.cost)
			}
			info = "[" + r.pol.Symbol() + "] " + c + "  "
		}
		lines = append(lines, trunc(sel+name+sDim.Render(info)+sGold.Render(r.note), w))
	}
	if more := len(rows) - (top + room); more > 0 {
		lines = append(lines, sDim.Render(fmt.Sprintf("  and %d more: type to narrow", more)))
	}
	return lines
}

// newBuildInputs makes the Builds screen's text boxes.
func newBuildInputs() (find, name, notes textinput.Model) {
	find = textinput.New()
	find.Prompt = "⌕ "
	find.CharLimit = 64
	name = textinput.New()
	name.Prompt = ""
	name.Placeholder = "e.g. Steel Path, or Survival"
	name.CharLimit = 24
	notes = textinput.New()
	notes.Prompt = ""
	notes.Placeholder = "e.g. max duration for Cataclysm; swap in Primed Flow later"
	notes.CharLimit = 300
	return find, name, notes
}

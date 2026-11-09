package ui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/Peejidle/seshat/internal/catalog"
	"github.com/Peejidle/seshat/internal/market"
	"github.com/Peejidle/seshat/internal/store"
)

// The Sell screen: the arcanes and spare prime parts you hold, priced from
// warframe.market, with what to list and for how much. Items are added
// by finding them by name and filling in how many; prices are looked up
// one item at a time in the background, no faster than the market allows.

// partRef is the Seshat item and part a market item is.
type partRef struct {
	item catalog.Item
	part catalog.Component
}

// sellRow is one item held to sell.
type sellRow struct {
	it     market.Item
	hold   market.Holding
	price  market.Prices
	priced bool // its prices have been looked up, if perhaps a while ago
	adv    market.Advice
}

// priceMsg is a price lookup done.
type priceMsg struct {
	p   market.Prices
	err error
}

// sellable is what the Sell screen deals in.
func sellable(it market.Item) bool { return it.IsArcane() || it.IsPrimePart() }

// matchMarket finds which Seshat part each prime part on the market is, so
// the Sell screen can warn before you sell what you still need.
func (m *Model) matchMarket() {
	m.partOf = map[string]partRef{}
	if m.market == nil {
		return
	}
	for _, it := range m.items {
		for _, c := range it.Parts() {
			if mi, ok := m.market.ForPart(c.ID); ok {
				m.partOf[mi.GameRef] = partRef{it, c}
			}
		}
	}
}

// sellRows lists what's held to sell: the most platinum first, then what
// Baro wants, then what's still being priced.
func (m Model) sellRows() []sellRow {
	if m.market == nil {
		return nil
	}
	var rows []sellRow
	add := func(id string, h market.Holding) {
		it, ok := m.market.Get(id)
		if !ok {
			return // no longer on the market
		}
		r := sellRow{it: it, hold: h}
		r.price, r.priced = m.prices.Entries[it.Slug]
		if r.priced {
			r.adv = market.Advise(it, h, r.price)
		}
		rows = append(rows, r)
	}
	for id, a := range m.prog.Arcanes {
		add(id, market.Holding{Maxed: a.Maxed, Copies: a.Copies})
	}
	for id, n := range m.prog.Spares {
		add(id, market.Holding{Copies: n})
	}
	sort.Slice(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		switch {
		case a.priced != b.priced:
			return a.priced
		case a.adv.Worth != b.adv.Worth:
			return a.adv.Worth > b.adv.Worth
		case a.adv.Ducats != b.adv.Ducats:
			return a.adv.Ducats > b.adv.Ducats
		}
		return a.it.Name < b.it.Name
	})
	return rows
}

// sellIndex is where the selected item is in rows. The selection follows
// the item rather than the line, since rows move as their prices come in.
func (m Model) sellIndex(rows []sellRow) int {
	for i, r := range rows {
		if r.it.GameRef == m.sellID {
			return i
		}
	}
	return 0
}

func (m Model) sellRowsShown() int { return max(3, m.height-15) }

func (m *Model) openSell() tea.Cmd {
	m.modal, m.sellMsg, m.sellTop, m.sellStep = mSell, "", 0, sellList
	if m.market == nil {
		m.sellMsg, m.sellMsgErr = "warframe.market's item list isn't synced yet: close this and press u.", true
		return nil
	}
	var stale []market.Item
	for _, r := range m.sellRows() {
		if _, fresh := m.prices.Get(r.it.Slug); !fresh {
			stale = append(stale, r.it)
		}
	}
	return m.queuePrices(stale...)
}

// queuePrices adds items to the price lookups, starting them if none are
// running. Each lookup's priceMsg starts the next.
func (m *Model) queuePrices(items ...market.Item) tea.Cmd {
	idle := len(m.pricing) == 0
	for _, it := range items {
		queued := false
		for _, q := range m.pricing {
			queued = queued || q.Slug == it.Slug
		}
		if !queued {
			m.pricing = append(m.pricing, it)
		}
	}
	if idle && len(m.pricing) > 0 {
		return fetchPrice(m.mclient, m.pricing[0])
	}
	return nil
}

func fetchPrice(c *market.Client, it market.Item) tea.Cmd {
	return func() tea.Msg {
		p, err := c.Fetch(it)
		return priceMsg{p, err}
	}
}

func (m Model) priced(msg priceMsg) (tea.Model, tea.Cmd) {
	if len(m.pricing) == 0 {
		return m, nil
	}
	it := m.pricing[0]
	m.pricing = m.pricing[1:]
	if msg.err != nil {
		// Most likely the network or the market is down: stop rather than
		// fail the same way for every item. Opening Sell again retries.
		m.pricing = nil
		m.sellMsg, m.sellMsgErr = fmt.Sprintf("couldn't price %s: %v", it.Name, msg.err), true
		return m, nil
	}
	m.prices.Put(msg.p)
	if err := m.prices.Save(); err != nil {
		m.sellMsg, m.sellMsgErr = "couldn't save prices: "+err.Error(), true
	}
	if len(m.pricing) > 0 {
		return m, fetchPrice(m.mclient, m.pricing[0])
	}
	return m, nil
}

// sellStage is the step the Sell screen is at. Adding an item is two
// steps, like a form: find it by name, then fill in how many you hold.
type sellStage int

const (
	sellList   sellStage = iota
	sellFind             // typing a name, choosing from what it matches
	sellCounts           // filling in how many of sellItem are held
)

// The count boxes: an arcane has both, a part just the copies.
const (
	fieldMaxed = iota
	fieldCopies
)

func (m Model) sellKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.sellStep {
	case sellFind:
		return m.sellFindKey(k)
	case sellCounts:
		return m.sellCountKey(k)
	}
	rows := m.sellRows()
	at := m.sellIndex(rows)
	move := func(i int) {
		if len(rows) > 0 {
			m.sellID = rows[min(max(i, 0), len(rows)-1)].it.GameRef
		}
	}
	switch k.String() {
	case "down", "j":
		move(at + 1)
	case "up", "k":
		move(at - 1)
	case "pgdown", "ctrl+d":
		move(at + m.sellRowsShown()/2)
	case "pgup", "ctrl+u":
		move(at - m.sellRowsShown()/2)
	case "g", "home":
		move(0)
	case "G", "end":
		move(len(rows) - 1)
	case "a":
		if m.market == nil {
			return m, nil
		}
		return m, m.startFind()
	case "enter", "e":
		if len(rows) > 0 {
			return m, m.editCounts(rows[at].it, sellList)
		}
	case "+", "=", "-":
		if len(rows) > 0 {
			step := 1
			if k.String() == "-" {
				step = -1
			}
			r := rows[at]
			m.setHolding(r.it, market.Holding{Maxed: r.hold.Maxed, Copies: r.hold.Copies + step})
			m.sellMsg, m.sellMsgErr = "✓ "+holdingText(r.it, m.holding(r.it)), false
			m.save()
		}
	case "r":
		if len(rows) > 0 {
			m.sellMsg = ""
			return m, m.queuePrices(rows[at].it)
		}
	case "w":
		if len(rows) > 0 {
			if err := openExternal("https://warframe.market/items/" + rows[at].it.Slug); err != nil {
				m.sellMsg, m.sellMsgErr = "could not open the browser: "+err.Error(), true
			}
		}
	default:
		m.modal = mNone
		return m, m.wantImage()
	}
	m.scrollSell()
	return m, nil
}

func (m *Model) scrollSell() {
	at := m.sellIndex(m.sellRows())
	if at < m.sellTop {
		m.sellTop = at
	}
	if at >= m.sellTop+m.sellRowsShown() {
		m.sellTop = at - m.sellRowsShown() + 1
	}
}

// startFind opens the name box, empty, for the next item to add.
func (m *Model) startFind() tea.Cmd {
	m.sellStep, m.sellPick = sellFind, 0
	m.sellIn.SetValue("")
	return m.sellIn.Focus()
}

// sellMatches is what the name typed so far could be.
func (m Model) sellMatches() []market.Item {
	if m.market == nil {
		return nil
	}
	return m.market.Find(m.sellIn.Value(), sellable)
}

func (m Model) sellFindKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	found := m.sellMatches()
	switch k.String() {
	case "esc":
		m.sellStep = sellList
		m.sellIn.Blur()
		return m, nil
	case "down", "tab", "ctrl+n":
		m.sellPick = min(m.sellPick+1, max(len(found)-1, 0))
		return m, nil
	case "up", "shift+tab", "ctrl+p":
		m.sellPick = max(m.sellPick-1, 0)
		return m, nil
	case "enter":
		if len(found) == 0 {
			return m, nil
		}
		m.sellIn.Blur()
		return m, m.editCounts(found[min(m.sellPick, len(found)-1)], sellFind)
	}
	var cmd tea.Cmd
	m.sellIn, cmd = m.sellIn.Update(k)
	m.sellPick = 0 // the matches have changed
	return m, cmd
}

// editCounts opens the count boxes for it, filled in with what's held now.
// back is the step esc goes back to.
func (m *Model) editCounts(it market.Item, back sellStage) tea.Cmd {
	m.sellStep, m.sellBack, m.sellItem, m.sellMsg = sellCounts, back, it, ""
	h := m.holding(it)
	for i, n := range []int{h.Maxed, h.Copies} {
		m.sellNum[i].SetValue("")
		if n > 0 {
			m.sellNum[i].SetValue(strconv.Itoa(n))
		}
		m.sellNum[i].CursorEnd()
	}
	m.sellField = fieldMaxed
	if !it.IsArcane() {
		m.sellField = fieldCopies
	}
	return m.focusField()
}

func (m *Model) focusField() tea.Cmd {
	for i := range m.sellNum {
		m.sellNum[i].Blur()
	}
	return m.sellNum[m.sellField].Focus()
}

func (m Model) sellCountKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	it := m.sellItem
	switch k.String() {
	case "esc":
		m.sellStep = m.sellBack
		if m.sellBack == sellFind {
			return m, m.sellIn.Focus()
		}
		return m, nil
	case "tab", "shift+tab", "down", "up":
		if it.IsArcane() {
			m.sellField = 1 - m.sellField
		}
		return m, m.focusField()
	case "enter":
		h := market.Holding{Copies: boxCount(m.sellNum[fieldCopies])}
		if it.IsArcane() {
			h.Maxed = boxCount(m.sellNum[fieldMaxed])
		}
		m.setHolding(it, h)
		m.save()
		m.sellID = it.GameRef
		m.sellMsg, m.sellMsgErr = "✓ Saved "+holdingText(it, h), false
		if h == (market.Holding{}) {
			m.sellMsg = "✓ Removed " + it.Name
		}
		var cmd tea.Cmd
		if _, fresh := m.prices.Get(it.Slug); !fresh && h != (market.Holding{}) {
			cmd = m.queuePrices(it)
		}
		// Adding goes straight on to the next name, as there are likely
		// more; changing one from the list goes back to it.
		m.sellStep = sellList
		m.scrollSell()
		if m.sellBack == sellFind {
			cmd = tea.Batch(cmd, m.startFind())
		}
		return m, cmd
	}
	// Only digits go in, so a count can't be mistyped as something else.
	if k.Type == tea.KeyRunes {
		k.Runes = []rune(strings.Map(func(r rune) rune {
			if r < '0' || r > '9' {
				return -1 // dropped
			}
			return r
		}, string(k.Runes)))
		if len(k.Runes) == 0 {
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.sellNum[m.sellField], cmd = m.sellNum[m.sellField].Update(k)
	return m, cmd
}

// boxCount is the number in a count box; empty is none.
func boxCount(t textinput.Model) int {
	n, _ := strconv.Atoi(t.Value())
	return n
}

func (m Model) holding(it market.Item) market.Holding {
	if it.IsArcane() {
		a := m.prog.HeldArcane(it.GameRef)
		return market.Holding{Maxed: a.Maxed, Copies: a.Copies}
	}
	return market.Holding{Copies: m.prog.HeldSpares(it.GameRef)}
}

func (m *Model) setHolding(it market.Item, h market.Holding) {
	if it.IsArcane() {
		m.prog.SetArcane(it.GameRef, store.Arcane{Maxed: h.Maxed, Copies: h.Copies})
		return
	}
	m.prog.SetSpares(it.GameRef, h.Copies)
}

// holdingText says what's held of it, e.g. "Arcane Energize: 2 maxed, 5 copies".
func holdingText(it market.Item, h market.Holding) string {
	if !it.IsArcane() {
		return fmt.Sprintf("%s ×%d", it.Name, h.Copies)
	}
	return it.Name + ": " + arcaneCounts(h)
}

func arcaneCounts(h market.Holding) string {
	var s []string
	if h.Maxed > 0 {
		s = append(s, fmt.Sprintf("%d maxed", h.Maxed))
	}
	if h.Copies > 0 {
		s = append(s, fmt.Sprintf("%d %s", h.Copies, plural(h.Copies, "copy", "copies")))
	}
	if len(s) == 0 {
		return "none"
	}
	return strings.Join(s, ", ")
}

// unbuiltNote warns when a part is one an item you haven't built needs,
// and you haven't marked one as put aside for it (in its details, with p).
func (m Model) unbuiltNote(it market.Item) string {
	ref, ok := m.partOf[it.GameRef]
	if !ok || !m.stillNeeded(ref.item, ref.part) {
		return ""
	}
	return fmt.Sprintf("%s isn't built: keep %d for it?", ref.item.Name, ref.part.Count)
}

func (m Model) sellView() string {
	rows := m.sellRows()
	w := max(20, m.width-6)
	var b strings.Builder
	b.WriteString(sTitle.Render("Sell") + "\n")

	worth, ducats, oldest := 0, 0, time.Time{}
	for _, r := range rows {
		worth += r.adv.Worth
		ducats += r.adv.Ducats
		if r.priced && (oldest.IsZero() || r.price.Fetched.Before(oldest)) {
			oldest = r.price.Fetched
		}
	}
	sub := "Arcanes and spare prime parts, priced from warframe.market"
	if !oldest.IsZero() {
		sub += " · checked " + ago(oldest)
	}
	if n := len(m.pricing); n > 0 {
		sub += fmt.Sprintf(" · pricing, %d to go…", n)
	}
	b.WriteString(sSub.Render(trunc(sub, w)) + "\n")
	total := ""
	if len(rows) > 0 {
		total = fmt.Sprintf("Worth about %sp as advised", commas(worth))
		if ducats > 0 {
			total += fmt.Sprintf(", and %s ducats for Baro", commas(ducats))
		}
	}
	b.WriteString(sGold.Render(trunc(total, w)) + "\n\n")

	// The middle is the list, or while adding, the form in its place.
	var lines []string
	switch m.sellStep {
	case sellFind:
		lines = m.sellFindLines(w)
	case sellCounts:
		lines = m.sellCountLines(w)
	default:
		lines = m.sellListLines(rows, w)
	}
	for _, l := range lines {
		b.WriteString(l + "\n")
	}
	b.WriteString(strings.Repeat("\n", max(m.sellRowsShown()-len(lines), 0)))

	// The selected item's prices in full, as the advice was worked from.
	b.WriteString("\n")
	if m.sellStep == sellList && len(rows) > 0 && rows[m.sellIndex(rows)].priced {
		b.WriteString(sDim.Render(trunc(priceLine(rows[m.sellIndex(rows)]), w)))
	}
	b.WriteString("\n")
	if m.sellMsg != "" {
		st := sText
		if m.sellMsgErr {
			st = sErr
		}
		b.WriteString(st.Render(trunc(m.sellMsg, w)))
	}
	b.WriteString("\n")
	keys := "a add · enter change counts · +/- one copy · r re-price · w open on warframe.market · j/k move · esc close"
	switch {
	case m.sellStep == sellFind:
		keys = "type part of the name · ↑/↓ choose · enter pick · esc done"
	case m.sellStep == sellCounts && m.sellItem.IsArcane():
		keys = "tab other box · enter save · esc back · leave both empty to remove it"
	case m.sellStep == sellCounts:
		keys = "enter save · esc back · leave it empty to remove it"
	}
	b.WriteString(sDim.Render(trunc(keys, w)))
	return b.String()
}

func (m Model) sellListLines(rows []sellRow, w int) []string {
	if len(rows) == 0 && m.market != nil {
		return []string{sDim.Render(trunc("Nothing to sell yet. Press a to add an arcane or prime part you hold.", w))}
	}
	var lines []string
	at := m.sellIndex(rows)
	for i := m.sellTop; i < len(rows) && len(lines) < m.sellRowsShown(); i++ {
		r := rows[i]
		sel, name := "  ", sText.Render(pad(trunc(r.it.Name, 34), 35))
		if i == at {
			sel, name = sGold.Render("▸ "), sTitle.Render(pad(trunc(r.it.Name, 34), 35))
		}
		held := fmt.Sprintf("×%d", r.hold.Copies)
		if r.it.IsArcane() {
			held = arcaneCounts(r.hold)
		}
		val, advice := sDim.Render(pad("", 8)), sDim.Render("pricing…")
		if r.priced {
			val = sGold.Render(pad(fmt.Sprintf("%7s", "~"+commas(r.adv.Worth)+"p"), 8))
			advice = sText.Render(strings.Join(r.adv.Steps, " · "))
		}
		if note := m.unbuiltNote(r.it); note != "" {
			advice = sErr.Render(note) + sDim.Render(" · ") + advice
		}
		lines = append(lines, trunc(sel+name+sDim.Render(pad(held, 20))+val+" "+advice, w))
	}
	return lines
}

// sellFindLines is the first step of adding: the name box and what it
// matches, with what's already held of each.
func (m Model) sellFindLines(w int) []string {
	lines := []string{
		sTitle.Render("Add: which arcane or prime part?"),
		m.sellIn.View(),
		"",
	}
	found := m.sellMatches()
	room := m.sellRowsShown() - len(lines) - 1
	switch {
	case strings.TrimSpace(m.sellIn.Value()) == "":
		return append(lines, sDim.Render(trunc("Any words of its name will do, e.g. energize, or nekros neuro.", w)))
	case len(found) == 0:
		return append(lines, sErr.Render(trunc("No arcane or prime part matches that.", w)))
	}
	pick := min(m.sellPick, len(found)-1)
	top := max(0, pick-room+1) // keep the choice in view
	for i := top; i < len(found) && i < top+room; i++ {
		it := found[i]
		sel, name := "  ", sText.Render(pad(trunc(it.Name, 40), 42))
		if i == pick {
			sel, name = sGold.Render("▸ "), sTitle.Render(pad(trunc(it.Name, 40), 42))
		}
		held := ""
		if h := m.holding(it); h != (market.Holding{}) {
			held = "you have " + strings.TrimPrefix(holdingText(it, h), it.Name+": ")
		}
		lines = append(lines, trunc(sel+name+sDim.Render(held), w))
	}
	if more := len(found) - (top + room); more > 0 {
		lines = append(lines, sDim.Render(fmt.Sprintf("  and %d more: type more of the name", more)))
	}
	return lines
}

// sellCountLines is the second step: how many of the item are held.
func (m Model) sellCountLines(w int) []string {
	it := m.sellItem
	box := func(field int, label string) string {
		sel := "  "
		if field == m.sellField {
			sel = sGold.Render("▸ ")
		}
		return sel + sText.Render(pad(label, 20)) + m.sellNum[field].View()
	}
	if !it.IsArcane() {
		about := it.Name
		if it.Ducats > 0 {
			about += fmt.Sprintf(" · %d ducats each", it.Ducats)
		}
		lines := []string{sTitle.Render(trunc(about, w)), "", box(fieldCopies, "How many to sell")}
		if note := m.unbuiltNote(it); note != "" {
			lines = append(lines, "", sErr.Render(trunc(note, w)))
		}
		return lines
	}
	// Ranks between are made of copies fused, so they count as those.
	var made []string
	for r := 1; r < it.MaxRank; r++ {
		made = append(made, fmt.Sprintf("rank %d = %d", r, market.CopiesToMax(r)))
	}
	return []string{
		sTitle.Render(trunc(fmt.Sprintf("%s · maxes at rank %d, from %d copies", it.Name, it.MaxRank, market.CopiesToMax(it.MaxRank)), w)),
		"",
		box(fieldMaxed, fmt.Sprintf("Maxed (rank %d)", it.MaxRank)),
		box(fieldCopies, "Rank 0 copies"),
		"",
		sDim.Render(trunc("One part-ranked counts as the rank 0 copies in it: "+strings.Join(made, ", "), w)),
	}
}

// priceLine sets out an item's prices, rank by rank.
func priceLine(r sellRow) string {
	ranks := []int{0}
	if r.it.MaxRank > 0 {
		ranks = append(ranks, r.it.MaxRank)
	}
	var parts []string
	for _, rk := range ranks {
		p := r.price.Ranks[rk]
		label := ""
		if r.it.MaxRank > 0 {
			label = fmt.Sprintf("rank %d: ", rk)
			if rk == r.it.MaxRank {
				label = "maxed: "
			}
		}
		parts = append(parts, fmt.Sprintf("%smedian %s, %d sold in 48h, cheapest %s, best buyer %s",
			label, platOrDash(p.Median), p.Sold, platOrDash(p.Lowest), platOrDash(p.TopBuy)))
	}
	return strings.Join(parts, "  ·  ")
}

func platOrDash(n int) string {
	if n == 0 {
		return "–"
	}
	return fmt.Sprintf("%dp", n)
}

// ago says how long since t, roughly: "just now", "25m ago", "3h ago".
func ago(t time.Time) string {
	switch d := time.Since(t); {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

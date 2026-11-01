package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/Peejidle/seshat/internal/catalog"
	"github.com/Peejidle/seshat/internal/mastery"
)

// Seshat's palette: lapis, gold leaf, papyrus, faience and red ochre.
var (
	lapis    = lipgloss.Color("#4A7FD0")
	gold     = lipgloss.Color("#D9B44A")
	papyrus  = lipgloss.Color("#E8DCC0")
	dim      = lipgloss.Color("#7D7566")
	faience  = lipgloss.Color("#3FC1AE")
	ochre    = lipgloss.Color("#C4613F")
	selectBg = lipgloss.Color("#1E2C48")

	sTitle    = lipgloss.NewStyle().Foreground(gold).Bold(true)
	sSub      = lipgloss.NewStyle().Foreground(dim).Italic(true)
	sText     = lipgloss.NewStyle().Foreground(papyrus)
	sDim      = lipgloss.NewStyle().Foreground(dim)
	sGold     = lipgloss.NewStyle().Foreground(gold)
	sLapis    = lipgloss.NewStyle().Foreground(lapis)
	sNew      = lipgloss.NewStyle().Foreground(faience)
	sErr      = lipgloss.NewStyle().Foreground(ochre)
	sTab      = lipgloss.NewStyle().Foreground(dim).Padding(0, 1)
	sTabOn    = lipgloss.NewStyle().Foreground(lipgloss.Color("#10131C")).Background(gold).Bold(true).Padding(0, 1)
	sSel      = lipgloss.NewStyle().Background(selectBg)
	sKey      = lipgloss.NewStyle().Foreground(gold)
	sPane     = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lapis).Padding(0, 1)
	sModal    = lipgloss.NewStyle().Border(lipgloss.DoubleBorder()).BorderForeground(gold).Padding(1, 2)
	sBarFull  = lipgloss.NewStyle().Foreground(gold)
	sBarEmpty = lipgloss.NewStyle().Foreground(lipgloss.Color("#3A3A44"))
)

const (
	chromeTop    = 5
	chromeBottom = 2
	paneWidth    = 46
	wideEnough   = 104
)

func (m Model) listHeight() int { return max(1, m.height-chromeTop-chromeBottom) }

func (m Model) sidePane() bool { return m.width >= wideEnough }

// imageCols sizes item icons to fit the detail pane's width and the window height.
func (m Model) imageCols() int {
	c := min(paneWidth-14, (m.height-28)*2)
	if c < 12 {
		return 0
	}
	return c
}

func (m Model) View() string {
	if m.width == 0 {
		return ""
	}
	if len(m.items) == 0 {
		msg := m.loading
		if msg == "" {
			msg = "Consulting the archive for the first time…"
		}
		if m.status != "" {
			msg = m.statusLine()
		}
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center,
			lipgloss.JoinVertical(lipgloss.Center, banner(), "", sSub.Render(msg)))
	}

	switch m.modal {
	case mHelp:
		return m.overlay(helpView())
	case mLedger:
		return m.overlay(m.ledgerView())
	case mImport:
		return m.overlay(m.importView())
	case mDetail:
		if it, ok := m.current(); ok {
			return m.overlay(m.detail(it, paneWidth+10))
		}
	}

	listW := m.width
	if m.sidePane() {
		listW = m.width - paneWidth
	}
	body := m.listView(listW)
	if m.sidePane() {
		var pane string
		if it, ok := m.current(); ok {
			pane = m.detail(it, paneWidth-2)
		}
		pane = sPane.Width(paneWidth - 2).Height(m.listHeight() - 2).MaxHeight(m.listHeight()).Render(pane)
		body = lipgloss.JoinHorizontal(lipgloss.Top, body, pane)
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		m.header(), m.tabs(), m.toolbar(listW), body, m.statusLine(), m.hints())
}

func banner() string {
	return lipgloss.JoinVertical(lipgloss.Center,
		sTitle.Render("◈  C E P H A L O N   S E S H A T  ◈"),
		sSub.Render("keeper of the codex · measurer of mastery"))
}

func (m Model) header() string {
	all := m.tallyFor("")
	xp := all.xp + m.prog.Extra.Total()
	rank, cur, next := mastery.Rank(xp)
	left := sTitle.Render(" ◈ CEPHALON SESHAT")
	if m.width >= 120 {
		left += sSub.Render("  keeper of the codex")
	}

	frac := float64(xp-cur) / float64(next-cur)
	right := fmt.Sprintf("%s %s %s  %s",
		sGold.Bold(true).Render(mastery.Label(rank)),
		bar(frac, 16),
		sDim.Render(fmt.Sprintf("%s/%s to %s", commas(xp-cur), commas(next-cur), mastery.Label(rank+1))),
		sText.Render(fmt.Sprintf("%d/%d ", all.mastered, all.total)))
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return trunc(left+" "+right, m.width)
	}
	return left + strings.Repeat(" ", gap) + right
}

func bar(frac float64, w int) string {
	frac = min(max(frac, 0), 1)
	n := int(frac*float64(w) + 0.5)
	return sBarFull.Render(strings.Repeat("━", n)) + sBarEmpty.Render(strings.Repeat("━", w-n))
}

// tabs renders the category bar, sliding it so the active tab stays visible
// on narrow terminals.
func (m Model) tabs() string {
	parts := make([]string, len(tabLabels))
	for i, l := range tabLabels {
		if i == m.tab {
			parts[i] = sTabOn.Render(l)
		} else {
			parts[i] = sTab.Render(l)
		}
	}
	lo, hi := 0, len(parts)
	width := func() int {
		w := 2
		for _, p := range parts[lo:hi] {
			w += lipgloss.Width(p)
		}
		return w
	}
	for width() > m.width-2 && hi-lo > 1 {
		if m.tab-lo > hi-1-m.tab {
			lo++
		} else {
			hi--
		}
	}
	left, right := " ", ""
	if lo > 0 {
		left = sDim.Render("‹")
	}
	if hi < len(parts) {
		right = sDim.Render("›")
	}
	return trunc(left+strings.Join(parts[lo:hi], "")+right, m.width)
}

func (m Model) toolbar(w int) string {
	var cat catalog.Category
	name := "Entire archive"
	if m.tab > 0 {
		cat = catalog.Categories[m.tab-1]
		name = string(cat)
	}
	t := m.tallyFor(cat)

	var search string
	switch {
	case m.searching:
		search = m.search.View()
	case m.search.Value() != "":
		search = sLapis.Render("⌕ " + m.search.Value())
	default:
		search = sDim.Render("/ search")
	}
	left := fmt.Sprintf(" %s  %s  %s %s",
		sGold.Render(name),
		sText.Render(fmt.Sprintf("%d/%d mastered · %d owned", t.mastered, t.total, t.owned)),
		bar(float64(t.mastered)/float64(max(t.total, 1)), 10),
		sDim.Render(fmt.Sprintf("%s/%s xp", commas(t.xp), commas(t.xpTotal))))
	hidden := ""
	if m.prog.Settings.HideMastered && m.filter != fMastered {
		hidden = " · mastered hidden"
	}
	right := sDim.Render(fmt.Sprintf("show %s%s · sort %s  ", m.filter, hidden, m.sort))
	line1 := left
	line2 := " " + search + strings.Repeat(" ", max(1, w-lipgloss.Width(search)-lipgloss.Width(right)-1)) + right

	head := sDim.Render(fmt.Sprintf("    %s %s  %-*s %-16s %4s %7s",
		"M", "O", max(8, w-40), "Name", "Type", "Req", "XP"))
	return trunc(line1, w) + "\n" + trunc(line2, w) + "\n" + trunc(head, w)
}

func (m Model) listView(w int) string {
	h := m.listHeight()
	nameW := max(8, w-40)
	var rows []string
	if len(m.visible) == 0 {
		rows = append(rows, sSub.Render("   Nothing in the archive matches."))
	}
	for i := m.offset; i < len(m.visible) && i < m.offset+h; i++ {
		it := m.items[m.visible[i]]
		mk := m.prog.Get(it.ID)

		mast := sDim.Render("·")
		if mk.Mastered {
			mast = sGold.Render("★")
		}
		own := sDim.Render("○")
		if mk.Owned {
			own = sLapis.Render("●")
		}
		cursor := "  "
		if i == m.cursor {
			cursor = sGold.Render("▸ ")
		}
		name := it.Name
		if m.newIDs[it.ID] {
			name += " ✦"
		}
		nameStyle := sText
		if mk.Mastered {
			nameStyle = sDim
		}
		typ := it.Type
		if it.Prime {
			typ += " ⁂"
		}
		row := fmt.Sprintf("%s  %s %s  %s %s %s %s",
			cursor, mast, own,
			nameStyle.Render(pad(trunc(name, nameW), nameW)),
			sDim.Render(pad(trunc(typ, 16), 16)),
			sDim.Render(fmt.Sprintf("%4s", req(it.MasteryReq))),
			sGold.Render(fmt.Sprintf("%7s", commas(it.MasteryXP()))))
		if m.newIDs[it.ID] {
			row = strings.Replace(row, "✦", sNew.Render("✦"), 1)
		}
		row = pad(trunc(row, w), w)
		if i == m.cursor {
			row = sSel.Render(row)
		}
		rows = append(rows, row)
	}
	for len(rows) < h {
		rows = append(rows, strings.Repeat(" ", w))
	}
	return strings.Join(rows, "\n")
}

func req(n int) string {
	if n == 0 {
		return "—"
	}
	return fmt.Sprintf("MR%d", n)
}

func pad(s string, w int) string {
	if d := w - lipgloss.Width(s); d > 0 {
		return s + strings.Repeat(" ", d)
	}
	return s
}

func (m Model) detail(it catalog.Item, w int) string {
	var b strings.Builder
	if img, ok := m.images[it.ImageName]; ok && img != "" {
		b.WriteString(lipgloss.PlaceHorizontal(w-2, lipgloss.Center, img) + "\n\n")
	} else if m.imgCols > 0 && it.ImageName != "" && !m.imgErr[it.ImageName] {
		b.WriteString(lipgloss.PlaceHorizontal(w-2, lipgloss.Center, sSub.Render("summoning likeness…")) + "\n\n")
	}

	title := sTitle.Render(it.Name)
	if m.newIDs[it.ID] {
		title += sNew.Render("  ✦ new")
	}
	b.WriteString(title + "\n")
	b.WriteString(sDim.Render(fmt.Sprintf("%s · %s", it.Category, it.Type)) + "\n\n")

	mk := m.prog.Get(it.ID)
	state := func(on bool, yes, no string) string {
		if on {
			return sGold.Render(yes)
		}
		return sDim.Render(no)
	}
	b.WriteString(state(mk.Mastered, "★ Mastered", "· Not mastered") + "   " +
		state(mk.Owned, "● Owned", "○ Not owned") + "\n\n")

	kv := func(k, v string) {
		if v != "" {
			b.WriteString(sDim.Render(pad(k, 12)) + sText.Render(v) + "\n")
		}
	}
	kv("Mastery", fmt.Sprintf("%s XP  (rank %d × %d)", commas(it.MasteryXP()), it.MaxRank, it.XPPerRank))
	if it.MasteryReq > 0 {
		kv("Requires", fmt.Sprintf("MR %d", it.MasteryReq))
	}
	kv("Released", it.ReleaseDate)
	kv("Introduced", it.Introduced)
	var flags []string
	if it.Prime {
		flags = append(flags, "Prime")
	}
	if it.Vaulted {
		flags = append(flags, "Vaulted")
	}
	kv("Status", strings.Join(flags, " · "))

	if len(it.Components) > 0 {
		b.WriteString("\n" + sLapis.Render("Foundry") + "\n")
		for _, c := range it.Components {
			n := c.Name
			if c.Count > 1 {
				n = fmt.Sprintf("%s × %s", n, commas(c.Count))
			}
			b.WriteString(sText.Render("  · "+trunc(n, w-6)) + "\n")
		}
	}
	if it.Description != "" {
		b.WriteString("\n" + sSub.Width(w-2).Render(it.Description) + "\n")
	}
	if it.WikiURL != "" {
		b.WriteString("\n" + sDim.Render("press ") + sKey.Render("w") + sDim.Render(" for drop locations on the wiki"))
	}
	return b.String()
}

func (m Model) statusLine() string {
	s := m.status
	if m.loading != "" {
		s = m.loading
	}
	if s == "" {
		return ""
	}
	st := sSub
	if m.statusErr {
		st = sErr
	}
	return trunc(" "+st.Render(s), m.width)
}

func (m Model) hints() string {
	k := func(key, what string) string { return sKey.Render(key) + sDim.Render(" "+what) }
	return trunc(" "+strings.Join([]string{
		k("space", "mastered"), k("o", "owned"), k("H", "hide mastered"), k("/", "search"), k("f", "filter"),
		k("s", "sort"), k("⇥", "category"), k("enter", "details"), k("x", "ledger"),
		k("u", "sync"), k("?", "help"), k("q", "quit"),
	}, sDim.Render(" · ")), m.width)
}

func (m Model) overlay(content string) string {
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, sModal.Render(content))
}

func helpView() string {
	rows := [][2]string{
		{"space / m", "toggle mastered (also marks owned)"},
		{"o", "toggle owned"},
		{"↑↓ j k", "move · pgup/pgdn · g/G top/bottom"},
		{"⇥ ⇤ ← →", "change category · 1-0 jump to category"},
		{"/", "search names and types · esc clears"},
		{"H", "hide / show mastered items (remembered)"},
		{"f", "cycle filter: all, owned, mastered, new"},
		{"s", "cycle sort: name, type, newest"},
		{"enter", "full details · w opens the wiki page"},
		{"x", "ledger: star chart, steel path & intrinsic XP"},
		{"u", "sync the archive with WFCD now"},
		{"E", "export records to ~/seshat-backup-DATE.json"},
		{"I", "import records from a backup"},
		{"q", "quit"},
	}
	var b strings.Builder
	b.WriteString(banner() + "\n\n")
	for _, r := range rows {
		b.WriteString(sKey.Render(pad(r[0], 12)) + sText.Render(r[1]) + "\n")
	}
	b.WriteString("\n" + sSub.Render("★ mastered  ● owned  ✦ new since last sync  ⁂ prime") + "\n")
	b.WriteString(sSub.Render("Item data: WFCD warframe-items (community-maintained, refreshed every 6h)."))
	return b.String()
}

func (m Model) ledgerView() string {
	var b strings.Builder
	b.WriteString(sTitle.Render("The Ledger") + "\n")
	b.WriteString(sSub.Render("Mastery from sources the archive cannot count. Find these") + "\n")
	b.WriteString(sSub.Render("in-game under Profile → Mastery Rank.") + "\n\n")
	for i, l := range ledgerFields {
		cur := "  "
		if i == m.ledgerAt {
			cur = sGold.Render("▸ ")
		}
		b.WriteString(cur + sText.Render(pad(l, 22)) + m.ledger[i].View() + "\n")
	}

	b.WriteString("\n" + sLapis.Render("Tally by category") + "\n")
	for _, c := range catalog.Categories {
		t := m.tallyFor(c)
		b.WriteString(fmt.Sprintf("  %s %s %s\n",
			sText.Render(pad(string(c), 18)),
			bar(float64(t.mastered)/float64(max(t.total, 1)), 12),
			sDim.Render(fmt.Sprintf("%3d/%-3d  %9s xp", t.mastered, t.total, commas(t.xp)))))
	}
	all := m.tallyFor("")
	xp := all.xp + m.prog.Extra.Total()
	rank, _, _ := mastery.Rank(xp)
	b.WriteString(fmt.Sprintf("\n  %s %s  %s\n",
		sText.Render(pad("Estimated total", 18)), sGold.Bold(true).Render(commas(xp)+" xp"),
		sGold.Render(mastery.Label(rank))))
	b.WriteString("\n" + sDim.Render("enter save · tab next field · esc cancel"))
	return b.String()
}

func (m Model) importView() string {
	return sTitle.Render("Restore records") + "\n" +
		sSub.Render("This replaces your current marks and ledger.") + "\n\n" +
		m.pathIn.View() + "\n\n" + sDim.Render("enter import · esc cancel")
}

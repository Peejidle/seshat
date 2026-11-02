package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/Peejidle/seshat/internal/catalog"
	"github.com/Peejidle/seshat/internal/mastery"
	"github.com/Peejidle/seshat/internal/profile"
	"github.com/Peejidle/seshat/internal/termimg"
)

type theme struct {
	name                                    string
	accent, info, text, dim, fresh, err     lipgloss.TerminalColor
	selBg, selFg, barEmpty, onAccent, muted lipgloss.TerminalColor
}

var themes = []theme{
	// The terminal's own 16 ANSI colours, so Seshat follows whatever palette
	// the terminal (or pywal, a Hyprland rice, …) defines.
	{
		name:   "terminal",
		accent: lipgloss.Color("3"), info: lipgloss.Color("4"), text: lipgloss.NoColor{},
		dim: lipgloss.Color("8"), fresh: lipgloss.Color("6"), err: lipgloss.Color("1"),
		selBg: lipgloss.Color("8"), selFg: lipgloss.Color("15"), barEmpty: lipgloss.Color("8"),
		onAccent: lipgloss.Color("0"), muted: lipgloss.Color("8"),
	},
	// Seshat's own palette: lapis, gold leaf, papyrus, faience and red ochre.
	{
		name:   "seshat",
		accent: lipgloss.Color("#D9B44A"), info: lipgloss.Color("#4A7FD0"), text: lipgloss.Color("#E8DCC0"),
		dim: lipgloss.Color("#7D7566"), fresh: lipgloss.Color("#3FC1AE"), err: lipgloss.Color("#C4613F"),
		selBg: lipgloss.Color("#1E2C48"), selFg: lipgloss.Color("#E8DCC0"), barEmpty: lipgloss.Color("#3A3A44"),
		onAccent: lipgloss.Color("#10131C"), muted: lipgloss.Color("#7D7566"),
	},
}

func themeIndex(name string) int {
	for i, t := range themes {
		if t.name == name {
			return i
		}
	}
	return 0
}

var (
	cur                                      theme
	sTitle, sSub, sText, sDim, sGold, sLapis lipgloss.Style
	sNew, sErr, sTab, sTabOn, sKey, sPane    lipgloss.Style
	sModal, sBarFull, sBarEmpty              lipgloss.Style
)

func applyTheme(t theme) {
	cur = t
	s := lipgloss.NewStyle
	sTitle = s().Foreground(t.accent).Bold(true)
	sSub = s().Foreground(t.dim).Italic(true)
	sText = s().Foreground(t.text)
	sDim = s().Foreground(t.dim)
	sGold = s().Foreground(t.accent)
	sLapis = s().Foreground(t.info)
	sNew = s().Foreground(t.fresh)
	sErr = s().Foreground(t.err)
	sTab = s().Foreground(t.muted).Padding(0, 1)
	sTabOn = s().Foreground(t.onAccent).Background(t.accent).Bold(true).Padding(0, 1)
	sKey = s().Foreground(t.accent)
	sPane = s().Border(lipgloss.RoundedBorder()).BorderForeground(t.info).Padding(0, 1)
	sModal = s().Border(lipgloss.DoubleBorder()).BorderForeground(t.accent).Padding(1, 2)
	sBarFull = s().Foreground(t.accent)
	sBarEmpty = s().Foreground(t.barEmpty)
}

func init() { applyTheme(themes[0]) }

const (
	chromeTop    = 5
	chromeBottom = 2
	paneWidth    = 46
	wideEnough   = 104
)

func (m Model) listHeight() int { return max(1, m.height-chromeTop-chromeBottom) }

func (m Model) sidePane() bool { return m.width >= wideEnough }

// paneImageCols sizes the side-pane icon to leave room for the details.
func (m Model) paneImageCols() int {
	if !m.sidePane() {
		return 0
	}
	return atLeast(min(paneWidth-14, (m.height-28)*2), 12)
}

// modalImageCols sizes the large icon in the full details view, which sits
// beside the text. Bigger is sharper: each cell carries two pixels.
func (m Model) modalImageCols() int {
	return atLeast(min(96, m.width-paneWidth-16, (m.height-8)*2), 16)
}

// wantCols is the icon size the current screen shows.
func (m Model) wantCols() int {
	if m.imgMode == termimg.Off {
		return 0
	}
	if m.modal == mDetail {
		return m.modalImageCols()
	}
	return m.paneImageCols()
}

func atLeast(n, floor int) int {
	if n < floor {
		return 0
	}
	return n
}

func imgKey(name string, cols int) string { return fmt.Sprintf("%s@%d", name, cols) }

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
		return m.overlay(m.helpView())
	case mLedger:
		return m.overlay(m.ledgerView())
	case mImport:
		return m.overlay(m.importView())
	case mRelics:
		return m.overlayFull(m.relicView())
	case mDetail:
		if it, ok := m.current(); ok {
			body := m.detail(it, paneWidth+4, 0)
			if c := m.modalImageCols(); c > 0 && it.ImageName != "" {
				body = lipgloss.JoinHorizontal(lipgloss.Center, m.image(it, c, c), "   ", body)
			}
			return m.overlay(body)
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
			pane = m.detail(it, paneWidth-2, m.paneImageCols())
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
	all := mastery.Count(m.items, m.prog, "")
	xp := mastery.Estimate(m.items, m.prog)
	rank, floor, next := mastery.Rank(xp)
	left := sTitle.Render(" ◈ CEPHALON SESHAT")
	if m.width >= 120 {
		left += sSub.Render("  keeper of the codex")
	}

	frac := float64(xp-floor) / float64(next-floor)
	right := fmt.Sprintf("%s %s %s  %s",
		sGold.Bold(true).Render(mastery.Label(rank)),
		bar(frac, 16),
		sDim.Render(fmt.Sprintf("%s/%s to %s", commas(xp-floor), commas(next-floor), mastery.Label(rank+1))),
		sText.Render(fmt.Sprintf("%d/%d ", all.Mastered, all.Total)))
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
	t := mastery.Count(m.items, m.prog, cat)

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
		sText.Render(fmt.Sprintf("%d/%d mastered · %d owned", t.Mastered, t.Total, t.Owned)),
		bar(float64(t.Mastered)/float64(max(t.Total, 1)), 10),
		sDim.Render(fmt.Sprintf("%s/%s xp", commas(t.XP), commas(t.XPTotal))))
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
		rows = append(rows, m.row(i, w, nameW))
	}
	for len(rows) < h {
		rows = append(rows, strings.Repeat(" ", w))
	}
	return strings.Join(rows, "\n")
}

// row renders one list line. Highlighted rows (cursor or bulk selection) put
// the background on every segment, since an outer style can't survive the
// resets inside already-styled text.
func (m Model) row(i, w, nameW int) string {
	it := m.items[m.visible[i]]
	mk := m.prog.Get(it.ID)
	text, dim, gold, info, fresh := sText, sDim, sGold, sLapis, sNew
	lit := i == m.cursor || m.inSelection(i)
	if lit {
		bg := func(s lipgloss.Style) lipgloss.Style { return s.Background(cur.selBg) }
		text = bg(text.Foreground(cur.selFg))
		dim = text // dim-on-highlight is unreadable in many palettes
		gold, info, fresh = bg(gold), bg(info), bg(fresh)
	}

	cursor := "  "
	switch {
	case i == m.cursor:
		cursor = "▸ "
	case m.inSelection(i):
		cursor = "┃ "
	}
	mast, own := dim.Render("·"), dim.Render("○")
	if mk.Mastered {
		mast = gold.Render("★")
	} else if mk.Rank > 0 {
		mast = gold.Render("◐")
	}
	if mk.Owned {
		own = info.Render("●")
	}
	nameStyle := text
	if mk.Mastered && !lit {
		nameStyle = dim
	}
	name := trunc(it.Name, nameW)
	if m.newIDs[it.ID] {
		name = trunc(it.Name, nameW-2)
	}
	typ := it.Type
	if it.Prime {
		typ += " ⁂"
	}
	if !mk.Mastered && mk.Rank > 0 {
		typ = fmt.Sprintf("%s %d/%d", typ, mk.Rank, it.MaxRank)
	} else if held, need := m.partsProgress(it); !mk.Owned && held > 0 {
		typ = fmt.Sprintf("%s ▪%d/%d", typ, held, need)
	}

	var b strings.Builder
	b.WriteString(gold.Render(cursor) + text.Render("  ") + mast + text.Render(" ") + own + text.Render("  "))
	b.WriteString(nameStyle.Render(name))
	used := lipgloss.Width(name)
	if m.newIDs[it.ID] {
		b.WriteString(fresh.Render(" ✦"))
		used += 2
	}
	b.WriteString(text.Render(strings.Repeat(" ", max(0, nameW-used)+1)))
	b.WriteString(dim.Render(pad(trunc(typ, 16), 16)) + text.Render(" "))
	b.WriteString(dim.Render(fmt.Sprintf("%4s", req(it.MasteryReq))) + text.Render(" "))
	b.WriteString(gold.Render(fmt.Sprintf("%7s", commas(it.MasteryXP()))))
	out := trunc(b.String(), w)
	if d := w - lipgloss.Width(out); d > 0 {
		out += text.Render(strings.Repeat(" ", d))
	}
	return out
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

// image renders an item's icon, or a placeholder of the same footprint while
// it loads, so the layout doesn't jump.
func (m Model) image(it catalog.Item, cols, boxW int) string {
	if img := m.images[imgKey(it.ImageName, cols)]; img != "" {
		return lipgloss.PlaceHorizontal(boxW, lipgloss.Center, img)
	}
	msg := "summoning likeness…"
	if m.imgErr[it.ImageName] {
		msg = "no likeness in the archive"
	}
	return lipgloss.Place(boxW, cols/2, lipgloss.Center, lipgloss.Center, sSub.Render(msg))
}

func (m Model) detail(it catalog.Item, w, imgCols int) string {
	var b strings.Builder
	if imgCols > 0 && it.ImageName != "" && !m.imgErr[it.ImageName] {
		b.WriteString(m.image(it, imgCols, w-2) + "\n\n")
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
	progress := state(mk.Mastered, "★ Mastered", "· Not mastered")
	if !mk.Mastered && mk.Rank > 0 {
		progress = sGold.Render(fmt.Sprintf("◐ Rank %d of %d", mk.Rank, it.MaxRank))
	}
	b.WriteString(progress + "   " + state(mk.Owned, "● Owned", "○ Not owned") + "\n")
	if !mk.Mastered && mk.Rank > 0 {
		left := it.MasteryXP() - it.EarnedXP(false, mk.Rank)
		b.WriteString(sDim.Render(fmt.Sprintf("%s XP still to earn", commas(left))) + "\n")
	}
	b.WriteString("\n")

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

	// Resources aren't shown: WFCD only has the final recipe's, not each
	// part's, so the totals would be misleading.
	if parts := it.Parts(); len(parts) > 0 {
		held, need := m.partsProgress(it)
		b.WriteString("\n" + sLapis.Render("Parts") + sDim.Render(fmt.Sprintf("  %d/%d", held, need)) + "\n")
		for part, c := range parts {
			n := m.partLabel(it, c)
			have := min(m.prog.HeldParts(it.ID, c.ID), c.Count)
			box, st := sDim.Render("[ ]"), sText
			switch {
			case have == c.Count:
				box = sGold.Render("[✓]")
			case have > 0:
				box = sGold.Render("[◐]")
			}
			if c.Count > 1 {
				n = fmt.Sprintf("%s %d/%d", n, have, c.Count)
			}
			sel := "  "
			if m.modal == mDetail && part == m.partAt {
				sel, st = sGold.Render("▸ "), sTitle
			}
			b.WriteString(sel + box + " " + st.Render(trunc(n, w-8)) + "\n")
			if ds := m.drops[partKey(it.ID, c.ID)]; len(ds) > 0 && m.stillNeeded(it, c) {
				b.WriteString("      " + m.dropLine(ds, w-8) + "\n")
			}
		}
		if m.modal == mDetail {
			b.WriteString(sDim.Render("  ") + sKey.Render("j/k") + sDim.Render(" pick · ") +
				sKey.Render("p") + sDim.Render(" have it · ") + sKey.Render("a") + sDim.Render(" all/none") + "\n")
		}
	}
	if len(it.UsedIn) > 0 {
		b.WriteString("\n" + sGold.Render("⚒ Keep for crafting") + "\n")
		for _, u := range it.UsedIn {
			n := u.Name
			if u.Count > 1 {
				n = fmt.Sprintf("%s (needs %d)", n, u.Count)
			}
			if m.prog.Get(u.ID).Mastered {
				b.WriteString(sDim.Render(trunc("  ★ "+n+" · mastered", w-4)) + "\n")
			} else {
				b.WriteString(sText.Render(trunc("  · "+n, w-4)) + "\n")
			}
		}
	}
	if it.Description != "" {
		b.WriteString("\n" + sSub.Width(w-2).Render(it.Description) + "\n")
	}
	if it.WikiURL != "" {
		b.WriteString("\n" + sKey.Render("w") + sDim.Render(" wiki / drops"))
	}
	if it.ImageName != "" {
		b.WriteString(sDim.Render(" · ") + sKey.Render("i") + sDim.Render(" full image"))
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
		k("space", "mastered"), k("o", "owned"), k("v", "select"), k("H", "hide mastered"), k("/", "search"), k("f", "filter"),
		k("s", "sort"), k("⇥", "category"), k("enter", "details"), k("R", "relics"), k("x", "ledger"),
		k("u", "sync"), k("?", "help"), k("q", "quit"),
	}, sDim.Render(" · ")), m.width)
}

func (m Model) overlay(content string) string {
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, sModal.Render(content))
}

// overlayFull draws a modal that fills the window whatever its content, for
// views you scroll through: a box that grew and shrank with each screen of
// text would jump around under you. Width and Height include the padding
// but not the border, hence the 2s.
func (m Model) overlayFull(content string) string {
	return sModal.Width(m.width - 2).Height(m.height - 2).Render(content)
}

func (m Model) helpView() string {
	rows := [][2]string{
		{"space / m", "toggle mastered (also marks owned)"},
		{"o", "toggle owned"},
		{"v", "start/stop a range selection; space/o mark it all"},
		{"V", "select everything shown (after a search or filter)"},
		{"↑↓ j k", "move · pgup/pgdn · g/G top/bottom"},
		{"⇥ ⇤ ← →", "change category · 1-0 jump to category"},
		{"/", "search names and types · esc clears"},
		{"H", "hide / show mastered items (remembered)"},
		{"f", "cycle filter: all, owned, in progress, collecting, mastered, new"},
		{"s", "cycle sort: name, type, newest"},
		{"enter", "details: check off parts with j/k and p (a = all)"},
		{"R", "relic finder · enter a relic · / look up · c count · o yours · r recount"},
		{"w / i", "open the wiki page / full-size image"},
		{"T", "theme: follow terminal colours, or Seshat's own"},
		{"x", "ledger: star chart, steel path & intrinsic XP"},
		{"u", "sync the archive with WFCD now"},
		{"E", "export records to ~/seshat-backup-DATE.json"},
		{"I", "import records from a backup"},
		{"P", "import mastery from your saved Warframe profile"},
		{"q", "quit"},
	}
	var b strings.Builder
	b.WriteString(banner() + "\n\n")
	for _, r := range rows {
		b.WriteString(sKey.Render(pad(r[0], 12)) + sText.Render(r[1]) + "\n")
	}
	b.WriteString("\n" + sSub.Render("★ mastered  ◐ in progress  ● owned  ▪ parts held  ✦ new  ⁂ prime") + "\n")
	b.WriteString(sSub.Render("Item data: WFCD warframe-items (community-maintained, refreshed every 6h).") + "\n")
	b.WriteString(sSub.Render(fmt.Sprintf("Images: %s mode (set SESHAT_IMAGES=kitty|blocks|off to override).", m.imgMode)))
	return b.String()
}

func (m Model) ledgerView() string {
	var b strings.Builder
	b.WriteString(sTitle.Render("The Ledger") + "\n")
	b.WriteString(sSub.Render("Mastery from sources the archive cannot count. Find these") + "\n")
	b.WriteString(sSub.Render("in-game under Profile → Mastery Rank. For intrinsics, add up") + "\n")
	b.WriteString(sSub.Render("the rank (0–10) of every school; each rank is 1,500 xp.") + "\n\n")
	for i, f := range ledgerFields {
		sel := "  "
		if i == m.ledgerAt {
			sel = sGold.Render("▸ ")
		}
		unit := "xp"
		if f.maxRank > 0 {
			unit = fmt.Sprintf("ranks of %d, all schools added up", f.maxRank)
		}
		b.WriteString(sel + sText.Render(pad(f.label, 22)) + pad(m.ledger[i].View(), 13) + sDim.Render(unit) + "\n")
	}

	b.WriteString("\n" + sLapis.Render("Tally by category") + "\n")
	for _, c := range catalog.Categories {
		t := mastery.Count(m.items, m.prog, c)
		b.WriteString(fmt.Sprintf("  %s %s %s\n",
			sText.Render(pad(string(c), 18)),
			bar(float64(t.Mastered)/float64(max(t.Total, 1)), 12),
			sDim.Render(fmt.Sprintf("%3d/%-3d  %9s xp", t.Mastered, t.Total, commas(t.XP)))))
	}
	xp := mastery.Estimate(m.items, m.prog)
	rank, _, _ := mastery.Rank(xp)
	b.WriteString(fmt.Sprintf("\n  %s %s  %s\n",
		sText.Render(pad("Estimated total", 18)), sGold.Bold(true).Render(commas(xp)+" xp"),
		sGold.Render(mastery.Label(rank))))
	b.WriteString("\n" + sDim.Render("enter save & next · tab/↑↓ move · esc close (drops unsaved edits)"))
	if st := m.statusLine(); st != "" {
		b.WriteString("\n" + st)
	}
	return b.String()
}

func (m Model) importView() string {
	if m.fromProf {
		step := func(n, s string) string { return sGold.Render(n+" ") + sText.Render(s) + "\n" }
		return sTitle.Render("Import mastery from your Warframe profile") + "\n" +
			sSub.Render("Seshat never contacts DE. You fetch your public profile once, yourself.") + "\n\n" +
			step("1", "Find your account ID: log in at warframe.com, press F12 →") +
			sDim.Render("    Storage/Application → Cookies → user-info → the 24-character user_id") + "\n" +
			step("2", "Open this in your browser, with your ID on the end, and save the page:") +
			sLapis.Render("    "+profile.URL+"<ID>") + "\n" +
			sDim.Render("    Open it once and check the ID: repeated or bad requests can get your") + "\n" +
			sDim.Render("    IP blocked from the game for a while. (Consoles: api-ps4, api-xb1, api-swi.)") + "\n" +
			step("3", "Point Seshat at the saved file:") + "\n" +
			m.pathIn.View() + "\n\n" +
			sDim.Render("Marks maxed items as mastered and fills in intrinsics. It never unmarks") + "\n" +
			sDim.Render("anything, and leaves owned alone (the profile includes sold items).") + "\n\n" +
			sDim.Render("enter import · esc cancel")
	}
	return sTitle.Render("Restore records") + "\n" +
		sSub.Render("This replaces your current marks and ledger.") + "\n\n" +
		m.pathIn.View() + "\n\n" + sDim.Render("enter import · esc cancel")
}

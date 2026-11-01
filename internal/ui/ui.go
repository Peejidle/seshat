// Package ui is Seshat's terminal interface.
package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/Peejidle/seshat/internal/catalog"
	"github.com/Peejidle/seshat/internal/profile"
	"github.com/Peejidle/seshat/internal/store"
	"github.com/Peejidle/seshat/internal/termimg"
)

// Paths locates Seshat's files on disk.
type Paths struct {
	Data     string // cached WFCD json
	Images   string // cached item icons
	Progress string // the user's checklist
}

// StaleAfter is how old the cached catalog may get before a background check.
const StaleAfter = 6 * time.Hour

type filterMode int

const (
	fAll filterMode = iota
	fOwnedUnmastered
	fInProgress
	fCollecting
	fMastered
	fNew
	numFilters
)

func (f filterMode) String() string {
	return [...]string{"all", "owned · not mastered", "in progress", "collecting parts", "mastered", "new"}[f]
}

type sortMode int

const (
	sName sortMode = iota
	sType
	sNewest
	numSorts
)

func (s sortMode) String() string { return [...]string{"name", "type", "newest"}[s] }

type modal int

const (
	mNone modal = iota
	mHelp
	mLedger
	mImport
	mDetail
)

// tab 0 is "All"; tab i>0 is catalog.Categories[i-1].
var tabLabels = []string{"All", "Frames", "Primary", "Secondary", "Melee", "Companions", "Robotic", "Archwing", "Vehicles", "Modular"}

type Model struct {
	paths Paths
	prog  *store.Progress

	items   []catalog.Item
	newIDs  map[string]bool
	visible []int // indexes into items
	loading string

	tab    int
	filter filterMode
	sort   sortMode
	cursor int
	offset int
	anchor int // start of a bulk selection, or -1

	search    textinput.Model
	searching bool

	modal    modal
	ledger   []textinput.Model
	ledgerAt int
	pathIn   textinput.Model
	fromProf bool // the import modal reads a Warframe profile, not a backup
	partAt   int  // selected part in the details view

	imgMode termimg.Mode
	images  map[string]string // imgKey(name, cols) -> rendered, "" while pending
	imgErr  map[string]bool   // imageName -> failed to download or decode

	status    string
	statusErr bool
	width     int
	height    int
}

type (
	catalogMsg struct {
		items []catalog.Item
		err   error
	}
	refreshMsg struct {
		changed int
		err     error
	}
	imageMsg struct {
		name, out string
		cols      int
		err       error
	}
	imageTick struct {
		name string
		cols int
	}
	openedMsg struct{ err error }
)

func New(paths Paths, prog *store.Progress) Model {
	s := textinput.New()
	s.Prompt = "⌕ "
	s.Placeholder = "search the archive"
	s.CharLimit = 64

	p := textinput.New()
	p.Prompt = "path › "
	p.CharLimit = 512

	m := Model{
		paths: paths, prog: prog, search: s, pathIn: p,
		newIDs: map[string]bool{}, anchor: -1, imgMode: termimg.Detect(),
		images: map[string]string{}, imgErr: map[string]bool{},
	}
	applyTheme(themes[themeIndex(prog.Settings.Theme)])
	return m
}

func (m Model) Init() tea.Cmd {
	if catalog.HasCache(m.paths.Data) {
		cmds := []tea.Cmd{loadCatalog(m.paths.Data)}
		if time.Since(catalog.LastChecked(m.paths.Data)) > StaleAfter {
			cmds = append(cmds, refresh(m.paths.Data))
		}
		return tea.Batch(cmds...)
	}
	return refresh(m.paths.Data)
}

func loadCatalog(dir string) tea.Cmd {
	return func() tea.Msg {
		items, err := catalog.Load(dir)
		return catalogMsg{items, err}
	}
}

func refresh(dir string) tea.Cmd {
	return func() tea.Msg {
		n, err := catalog.Refresh(dir)
		return refreshMsg{n, err}
	}
}

func (m *Model) say(format string, a ...any) {
	m.status, m.statusErr = fmt.Sprintf(format, a...), false
}

func (m *Model) fail(format string, a ...any) {
	m.status, m.statusErr = fmt.Sprintf(format, a...), true
}

func (m *Model) save() {
	if err := m.prog.Save(m.paths.Progress); err != nil {
		m.fail("could not inscribe record: %v", err)
	}
}

// ---- update ---------------------------------------------------------------

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.clamp()
		return m, m.wantImage()

	case catalogMsg:
		m.loading = ""
		if msg.err != nil {
			m.fail("archive unreadable: %v (press u to re-sync)", msg.err)
			return m, nil
		}
		m.items = msg.items
		m.markNew()
		m.rebuild()
		return m, m.wantImage()

	case refreshMsg:
		m.loading = ""
		switch {
		case msg.err != nil && len(m.items) == 0:
			m.fail("could not reach the archive: %v", msg.err)
			return m, nil
		case msg.err != nil:
			m.fail("sync failed, using cached archive: %v", msg.err)
			return m, nil
		case msg.changed == 0 && len(m.items) > 0:
			m.say("The archive is current.")
			return m, nil
		}
		return m, loadCatalog(m.paths.Data)

	case imageTick:
		if it, ok := m.current(); ok && it.ImageName == msg.name && m.wantCols() == msg.cols {
			key := imgKey(msg.name, msg.cols)
			if _, have := m.images[key]; !have {
				m.images[key] = ""
				return m, fetchImage(m.paths.Images, msg.name, msg.cols, m.imgMode)
			}
		}
		return m, nil

	case imageMsg:
		key := imgKey(msg.name, msg.cols)
		if msg.err != nil {
			delete(m.images, key)
			m.imgErr[msg.name] = true
		} else {
			m.images[key] = msg.out
		}
		return m, nil

	case openedMsg:
		if msg.err != nil {
			m.fail("could not open image: %v", msg.err)
		}
		return m, nil

	case tea.KeyMsg:
		return m.key(msg)
	}
	return m, nil
}

func (m *Model) markNew() {
	first := len(m.prog.Seen) == 0
	for _, it := range m.items {
		if !m.prog.Seen[it.ID] {
			if !first {
				m.newIDs[it.ID] = true
			}
			m.prog.Seen[it.ID] = true
		}
	}
	switch n := len(m.newIDs); {
	case first:
		m.say("Archive inscribed: %d masterable items catalogued.", len(m.items))
	case n > 0:
		m.say("✦ %d new entr%s added to the archive. Press f to filter by new.", n, plural(n, "y", "ies"))
	}
	m.save()
}

func (m Model) key(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if k.String() == "ctrl+c" {
		return m, tea.Quit
	}
	switch m.modal {
	case mHelp:
		m.modal = mNone
		return m, nil
	case mDetail:
		switch k.String() {
		case "o", "m", " ", "w", "i":
			// marking, the wiki link and the image viewer still work here
		case "down", "j", "up", "k", "p", "a":
			m.partKey(k.String())
			return m, nil
		default:
			m.modal = mNone
			return m, m.wantImage()
		}
	case mLedger:
		return m.ledgerKey(k)
	case mImport:
		return m.importKey(k)
	}

	if m.searching {
		switch k.String() {
		case "esc":
			m.searching = false
			m.search.SetValue("")
			m.search.Blur()
			m.rebuild()
			return m, m.wantImage()
		case "enter", "down", "up":
			m.searching = false
			m.search.Blur()
			if k.String() == "enter" {
				return m, nil
			}
		default:
			var cmd tea.Cmd
			m.search, cmd = m.search.Update(k)
			m.rebuild()
			return m, tea.Batch(cmd, m.wantImage())
		}
	}

	moved := false
	switch k.String() {
	case "q":
		return m, tea.Quit
	case "esc":
		if m.anchor >= 0 {
			m.anchor = -1
			m.say("Selection released.")
		} else if m.search.Value() != "" {
			m.search.SetValue("")
			m.rebuild()
			moved = true
		}
	case "/":
		m.searching = true
		return m, m.search.Focus()
	case "tab", "right", "l":
		m.tab = (m.tab + 1) % len(tabLabels)
		m.cursor, m.offset = 0, 0
		m.rebuild()
		moved = true
	case "shift+tab", "left", "h":
		m.tab = (m.tab + len(tabLabels) - 1) % len(tabLabels)
		m.cursor, m.offset = 0, 0
		m.rebuild()
		moved = true
	case "1", "2", "3", "4", "5", "6", "7", "8", "9", "0":
		n, _ := strconv.Atoi(k.String())
		if n == 0 {
			n = 10
		}
		m.tab = n - 1
		m.cursor, m.offset = 0, 0
		m.rebuild()
		moved = true
	case "down", "j":
		m.cursor++
		moved = true
	case "up", "k":
		m.cursor--
		moved = true
	case "pgdown", "ctrl+d":
		m.cursor += m.listHeight() / 2
		moved = true
	case "pgup", "ctrl+u":
		m.cursor -= m.listHeight() / 2
		moved = true
	case "g", "home":
		m.cursor = 0
		moved = true
	case "G", "end":
		m.cursor = len(m.visible) - 1
		moved = true
	case " ", "m":
		m.markMastered()
	case "o":
		m.markOwned()
	case "v":
		if m.anchor >= 0 {
			m.anchor = -1
			m.say("Selection released.")
		} else if len(m.visible) > 0 {
			m.anchor = m.cursor
			m.say("Selecting: move to extend, space/o to mark the range, esc to cancel.")
		}
	case "V":
		if len(m.visible) > 0 {
			m.anchor, m.cursor = 0, len(m.visible)-1
			m.say("Selected all %d shown: space/o to mark them, esc to cancel.", len(m.visible))
			moved = true
		}
	case "i":
		if it, ok := m.current(); ok && it.ImageName != "" {
			m.say("Opening the likeness of %s…", it.Name)
			return m, openImage(m.paths.Images, it.ImageName)
		}
	case "T":
		i := (themeIndex(m.prog.Settings.Theme) + 1) % len(themes)
		m.prog.Settings.Theme = themes[i].name
		applyTheme(themes[i])
		m.save()
		m.say("Theme: %s.", themes[i].name)
	case "f":
		m.filter = (m.filter + 1) % numFilters
		if m.filter == fNew && len(m.newIDs) == 0 {
			m.filter = fAll
		}
		m.cursor, m.offset = 0, 0
		m.rebuild()
		moved = true
	case "H":
		m.prog.Settings.HideMastered = !m.prog.Settings.HideMastered
		m.save()
		if m.prog.Settings.HideMastered {
			m.say("Mastered items hidden. Press H to show them again.")
		} else {
			m.say("Mastered items shown.")
		}
		m.rebuildKeep()
		moved = true
	case "s":
		m.sort = (m.sort + 1) % numSorts
		m.rebuild()
		moved = true
	case "enter":
		if _, ok := m.current(); ok {
			m.modal, m.partAt = mDetail, 0
			return m, m.wantImage()
		}
	case "w":
		if it, ok := m.current(); ok && it.WikiURL != "" {
			if err := exec.Command("xdg-open", it.WikiURL).Start(); err != nil {
				m.fail("could not open browser: %v", err)
			} else {
				m.say("Opening %s on the wiki…", it.Name)
			}
		}
	case "u":
		if m.loading == "" {
			m.loading = "Consulting the archive…"
			return m, refresh(m.paths.Data)
		}
	case "x":
		m.openLedger()
		return m, nil
	case "E":
		home, _ := os.UserHomeDir()
		path := filepath.Join(home, "seshat-backup-"+time.Now().Format("2006-01-02")+".json")
		if err := m.prog.Export(path); err != nil {
			m.fail("export failed: %v", err)
		} else {
			m.say("Records copied to %s", path)
		}
	case "I", "P":
		m.modal, m.fromProf = mImport, k.String() == "P"
		m.pathIn.SetValue("")
		if m.fromProf {
			m.pathIn.SetValue(guessProfilePath())
			m.pathIn.CursorEnd()
		}
		return m, m.pathIn.Focus()
	case "?":
		m.modal = mHelp
	}
	if moved {
		m.clamp()
		return m, m.wantImage()
	}
	return m, nil
}

var ledgerFields = []string{"Star Chart", "Steel Path", "Railjack intrinsics", "Drifter intrinsics", "Other"}

func (m *Model) extraPtrs() []*int {
	e := &m.prog.Extra
	return []*int{&e.StarChart, &e.SteelPath, &e.Railjack, &e.Drifter, &e.Other}
}

func (m *Model) openLedger() {
	m.modal, m.ledgerAt = mLedger, 0
	m.ledger = make([]textinput.Model, len(ledgerFields))
	for i, p := range m.extraPtrs() {
		t := textinput.New()
		t.Prompt = ""
		t.CharLimit = 9
		t.Width = 12
		t.SetValue(strconv.Itoa(*p))
		m.ledger[i] = t
	}
	m.ledger[0].Focus()
}

func (m Model) ledgerKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "esc":
		m.modal = mNone
		m.say("Ledger unchanged.")
		return m, nil
	case "enter":
		for i, p := range m.extraPtrs() {
			v, err := strconv.Atoi(strings.ReplaceAll(strings.TrimSpace(m.ledger[i].Value()), ",", ""))
			if err != nil || v < 0 {
				m.fail("%s must be a whole number", ledgerFields[i])
				return m, nil
			}
			*p = v
		}
		m.save()
		m.modal = mNone
		m.say("Ledger inscribed.")
		return m, nil
	case "tab", "down":
		m.ledger[m.ledgerAt].Blur()
		m.ledgerAt = (m.ledgerAt + 1) % len(m.ledger)
		return m, m.ledger[m.ledgerAt].Focus()
	case "shift+tab", "up":
		m.ledger[m.ledgerAt].Blur()
		m.ledgerAt = (m.ledgerAt + len(m.ledger) - 1) % len(m.ledger)
		return m, m.ledger[m.ledgerAt].Focus()
	}
	var cmd tea.Cmd
	m.ledger[m.ledgerAt], cmd = m.ledger[m.ledgerAt].Update(k)
	return m, cmd
}

func (m Model) importKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "esc":
		m.modal = mNone
		return m, nil
	case "enter":
		path := strings.TrimSpace(m.pathIn.Value())
		if strings.HasPrefix(path, "~/") {
			home, _ := os.UserHomeDir()
			path = filepath.Join(home, path[2:])
		}
		if m.fromProf {
			if err := m.importProfile(path); err != nil {
				m.fail("profile import failed: %v", err)
				return m, nil // keep the prompt open to fix the path
			}
		} else if err := m.prog.Import(path); err != nil {
			m.fail("import failed: %v", err)
		} else {
			m.save()
			m.say("Records restored from %s (%d marks).", path, len(m.prog.Marks))
			m.rebuildKeep()
		}
		m.modal = mNone
		return m, nil
	}
	var cmd tea.Cmd
	m.pathIn, cmd = m.pathIn.Update(k)
	return m, cmd
}

func (m *Model) importProfile(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	p, err := profile.Parse(b)
	if err != nil {
		return err
	}
	r := profile.Apply(p, m.items, m.prog)
	m.save()
	m.say("%s", profile.Summary(r))
	m.rebuildKeep()
	return nil
}

// guessProfilePath suggests the newest profile the browser saved to
// ~/Downloads, since browsers name it after the endpoint.
func guessProfilePath() string {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, "Downloads")
	entries, _ := os.ReadDir(dir)
	var best string
	var newest time.Time
	for _, e := range entries {
		if !strings.HasPrefix(strings.ToLower(e.Name()), "getprofileviewingdata") {
			continue
		}
		if info, err := e.Info(); err == nil && info.ModTime().After(newest) {
			best, newest = e.Name(), info.ModTime()
		}
	}
	if best == "" {
		return "~/Downloads/"
	}
	return "~/Downloads/" + best
}

// ---- list state -----------------------------------------------------------

func (m *Model) rebuild() {
	q := strings.ToLower(strings.TrimSpace(m.search.Value()))
	m.anchor = -1
	m.visible = m.visible[:0]
	for i, it := range m.items {
		if m.tab > 0 && it.Category != catalog.Categories[m.tab-1] {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(it.Name), q) && !strings.Contains(strings.ToLower(it.Type), q) {
			continue
		}
		mk := m.prog.Get(it.ID)
		// The "mastered" filter overrides hiding, or it would always be empty.
		if mk.Mastered && m.prog.Settings.HideMastered && m.filter != fMastered {
			continue
		}
		switch m.filter {
		case fOwnedUnmastered:
			if !mk.Owned || mk.Mastered {
				continue
			}
		case fInProgress:
			if mk.Mastered || mk.Rank == 0 {
				continue
			}
		case fCollecting:
			if held, _ := m.partsProgress(it); mk.Owned || held == 0 {
				continue
			}
		case fMastered:
			if !mk.Mastered {
				continue
			}
		case fNew:
			if !m.newIDs[it.ID] {
				continue
			}
		}
		m.visible = append(m.visible, i)
	}
	items := m.items
	switch m.sort {
	case sType:
		sort.SliceStable(m.visible, func(a, b int) bool {
			ia, ib := items[m.visible[a]], items[m.visible[b]]
			if ia.Category != ib.Category {
				return catIndex(ia.Category) < catIndex(ib.Category)
			}
			return ia.Type < ib.Type
		})
	case sNewest:
		sort.SliceStable(m.visible, func(a, b int) bool {
			return items[m.visible[a]].ReleaseDate > items[m.visible[b]].ReleaseDate
		})
	}
	m.clamp()
}

// rebuildKeep rebuilds but keeps the cursor near the same row, so marking an
// item that then drops out of the filter doesn't jump to the top.
func (m *Model) rebuildKeep() {
	c := m.cursor
	m.rebuild()
	m.cursor = c
	m.clamp()
}

func catIndex(c catalog.Category) int {
	for i, x := range catalog.Categories {
		if x == c {
			return i
		}
	}
	return len(catalog.Categories)
}

func (m *Model) clamp() {
	if m.cursor >= len(m.visible) {
		m.cursor = len(m.visible) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	h := m.listHeight()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+h {
		m.offset = m.cursor - h + 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

func (m Model) inSelection(i int) bool {
	if m.anchor < 0 {
		return false
	}
	return i >= min(m.anchor, m.cursor) && i <= max(m.anchor, m.cursor)
}

// targets is what a mark applies to: the bulk selection, or the cursor row.
func (m Model) targets() []catalog.Item {
	if m.anchor < 0 {
		if it, ok := m.current(); ok {
			return []catalog.Item{it}
		}
		return nil
	}
	var out []catalog.Item
	for i := min(m.anchor, m.cursor); i <= max(m.anchor, m.cursor); i++ {
		out = append(out, m.items[m.visible[i]])
	}
	return out
}

// markMastered toggles mastery on the targets. A mixed selection gets
// mastered; only an all-mastered one is cleared. Mastering implies owning.
func (m *Model) markMastered() {
	ts := m.targets()
	if len(ts) == 0 {
		return
	}
	on, xp := false, 0
	for _, it := range ts {
		if !m.prog.Get(it.ID).Mastered {
			on = true
		}
	}
	for _, it := range ts {
		mk := m.prog.Get(it.ID)
		if on && !mk.Mastered {
			xp += it.MasteryXP()
		}
		mk.Mastered = on
		if on {
			mk.Owned = true
		}
		m.prog.Set(it.ID, mk)
	}
	switch {
	case len(ts) == 1 && on:
		m.say("%s — mastery inscribed. +%s XP", ts[0].Name, commas(xp))
	case len(ts) == 1:
		m.say("%s — mastery struck from the record.", ts[0].Name)
	case on:
		m.say("Mastery inscribed for %d items. +%s XP", len(ts), commas(xp))
	default:
		m.say("Mastery struck from %d items.", len(ts))
	}
	m.save()
	m.rebuildKeep()
}

// markOwned toggles ownership the same way as markMastered.
func (m *Model) markOwned() {
	ts := m.targets()
	if len(ts) == 0 {
		return
	}
	on := false
	for _, it := range ts {
		if !m.prog.Get(it.ID).Owned {
			on = true
		}
	}
	for _, it := range ts {
		mk := m.prog.Get(it.ID)
		mk.Owned = on
		m.prog.Set(it.ID, mk)
	}
	switch {
	case len(ts) == 1 && on:
		m.say("%s — marked as in your arsenal.", ts[0].Name)
	case len(ts) == 1:
		m.say("%s — marked as not owned.", ts[0].Name)
	case on:
		m.say("%d items marked as in your arsenal.", len(ts))
	default:
		m.say("%d items marked as not owned.", len(ts))
	}
	m.save()
	m.rebuildKeep()
}

// partKey moves through and checks off the current item's parts: p steps
// the selected part through 0..needed, a fills or clears them all.
func (m *Model) partKey(key string) {
	it, ok := m.current()
	if !ok {
		return
	}
	parts := it.Parts()
	if len(parts) == 0 {
		m.say("%s has no parts to collect.", it.Name)
		return
	}
	m.partAt = min(max(m.partAt, 0), len(parts)-1)
	switch key {
	case "down", "j":
		m.partAt = min(m.partAt+1, len(parts)-1)
		return
	case "up", "k":
		m.partAt = max(m.partAt-1, 0)
		return
	case "p":
		c := parts[m.partAt]
		m.prog.SetPart(it.ID, c.ID, (m.prog.HeldParts(it.ID, c.ID)+1)%(c.Count+1))
	case "a":
		held, need := m.partsProgress(it)
		for _, c := range parts {
			n := 0
			if held < need {
				n = c.Count
			}
			m.prog.SetPart(it.ID, c.ID, n)
		}
	}
	if held, need := m.partsProgress(it); held == need {
		m.say("All parts for %s collected: ready for the foundry.", it.Name)
	} else {
		m.say("%s: %d of %d parts collected.", it.Name, held, need)
	}
	m.save()
}

// partsProgress counts the parts held toward an item and the parts needed.
func (m Model) partsProgress(it catalog.Item) (held, need int) {
	for _, c := range it.Parts() {
		held += min(m.prog.HeldParts(it.ID, c.ID), c.Count)
		need += c.Count
	}
	return held, need
}

func (m Model) current() (catalog.Item, bool) {
	if m.cursor < 0 || m.cursor >= len(m.visible) {
		return catalog.Item{}, false
	}
	return m.items[m.visible[m.cursor]], true
}

// wantImage schedules a (debounced) icon load for the selected item.
func (m Model) wantImage() tea.Cmd {
	it, ok := m.current()
	cols := m.wantCols()
	if !ok || it.ImageName == "" || cols == 0 || m.imgErr[it.ImageName] {
		return nil
	}
	if _, have := m.images[imgKey(it.ImageName, cols)]; have {
		return nil
	}
	name := it.ImageName
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg { return imageTick{name, cols} })
}

func fetchImage(dir, name string, cols int, mode termimg.Mode) tea.Cmd {
	return func() tea.Msg {
		path, err := catalog.Image(dir, name)
		if err != nil {
			return imageMsg{name: name, cols: cols, err: err}
		}
		render := termimg.Render
		if mode == termimg.Kitty {
			render = termimg.RenderKitty
		}
		out, err := render(path, cols, cols/2)
		return imageMsg{name, out, cols, err}
	}
}

// openImage shows the full-resolution icon in the desktop's image viewer.
func openImage(dir, name string) tea.Cmd {
	return func() tea.Msg {
		path, err := catalog.Image(dir, name)
		if err == nil {
			err = exec.Command("xdg-open", path).Start()
		}
		return openedMsg{err}
	}
}

// ---- stats ----------------------------------------------------------------

type tally struct{ total, owned, mastered, partial, xp, xpTotal int }

func (m Model) tallyFor(cat catalog.Category) tally {
	var t tally
	for _, it := range m.items {
		if cat != "" && it.Category != cat {
			continue
		}
		mk := m.prog.Get(it.ID)
		t.total++
		t.xpTotal += it.MasteryXP()
		if mk.Owned {
			t.owned++
		}
		if mk.Mastered {
			t.mastered++
		} else if mk.Rank > 0 {
			t.partial++
		}
		t.xp += it.EarnedXP(mk.Mastered, mk.Rank)
	}
	return t
}

// ---- helpers --------------------------------------------------------------

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func commas(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

func trunc(s string, w int) string { return ansi.Truncate(s, w, "…") }

// Package ui is Seshat's terminal interface.
package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/Peejidle/seshat/internal/catalog"
	"github.com/Peejidle/seshat/internal/relics"
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
	mRelics
	mNext
)

// tab 0 is "All"; tab i>0 is catalog.Categories[i-1].
var tabLabels = []string{"All", "Frames", "Primary", "Secondary", "Melee", "Companions", "Robotic", "Archwing", "Vehicles", "Modular"}

type Model struct {
	paths Paths
	prog  *store.Progress

	items   []catalog.Item
	relics  *relics.Table            // nil until the drop tables are cached
	drops   map[string][]relics.Drop // partKey -> relics that drop it, best first
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
	// rankDigits is a rank being typed in the details view, for items
	// levelled part way, such as a Kuva weapon at 34 of 40.
	rankDigits string
	// detailBack is the modal the details return to: none, or "next up"
	// when opened from there.
	detailBack modal

	nextAt, nextTop int // selected item and first shown line in "next up"

	relicAt, relicTop int  // selected and first shown relic in the finder
	relicVaulted      bool // the finder includes vaulted relics
	relicMine         bool // the finder lists the relics you hold
	relicByName       bool // the finder sorts by name instead of parts needed
	relicQuery        textinput.Model
	relicTyping       bool // keys go to relicQuery
	relicCount        textinput.Model
	relicCounting     bool   // keys go to relicCount
	relicMsg          string // result of the last count entry
	relicMsgErr       bool
	relicOpen         *relics.Relic // the relic window, over the finder; nil when closed
	rewardAt          int           // selected reward in the relic window
	relicDigits       string        // a count being typed in the relic window
	rewardParts       map[string]rewardPart

	recounting    bool                     // the finder is walking through a recount
	recountList   []*relics.Relic          // relics to recount, in name order
	recountAt     int                      // index into recountList
	recountDone   map[*relics.Relic]string // what each relic's step did
	recountResume *relics.Relic            // where a paused recount carries on

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
		items  []catalog.Item
		relics *relics.Table // nil if the drop tables couldn't be read
		err    error
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

	r := textinput.New()
	r.Prompt = "⌕ "
	r.Placeholder = "a part or relic, e.g. nyx prime, axi s8"
	r.CharLimit = 64

	c := textinput.New()
	c.Prompt = "count › "
	c.Placeholder = "a s8 12, l n1 4, m s3 +2"
	c.CharLimit = 256

	m := Model{
		paths: paths, prog: prog, search: s, pathIn: p, relicQuery: r, relicCount: c,
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
		// Relic data is optional: everything else works without it.
		table, _ := relics.Load(filepath.Join(dir, catalog.DropsFile))
		return catalogMsg{items, table, err}
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
		m.items, m.relics = msg.items, msg.relics
		m.matchDrops()
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
		key := k.String()
		switch {
		case key == "o", key == "m", key == " ", key == "w", key == "i":
			// marking, the wiki link and the image viewer still work here
		case key == "down", key == "j", key == "up", key == "k", key == "p", key == "a":
			m.partKey(key)
			return m, nil
		case len(key) == 1 && key[0] >= '0' && key[0] <= '9':
			// Typing a number sets the rank once enter is pressed.
			if len(m.rankDigits) < 2 {
				m.rankDigits += key
			}
			return m, nil
		case key == "backspace":
			m.rankDigits = m.rankDigits[:max(len(m.rankDigits)-1, 0)]
			return m, nil
		case key == "enter" && m.rankDigits != "":
			n, _ := strconv.Atoi(m.rankDigits)
			m.setRank(n)
			m.rankDigits = ""
			return m, nil
		case key == "esc" && m.rankDigits != "":
			m.rankDigits = "" // first esc drops a half-typed rank
			return m, nil
		default:
			m.rankDigits = ""
			m.modal, m.detailBack = m.detailBack, mNone
			m.rebuildKeep() // marks made in the details may have filtered it out
			if m.modal == mNext {
				m.nextAt = min(m.nextAt, max(len(m.nextItems())-1, 0)) // it may be mastered now
				m.scrollNext(m.nextItems())
			}
			return m, m.wantImage()
		}
	case mLedger:
		return m.ledgerKey(k)
	case mImport:
		return m.importKey(k)
	case mRelics:
		return m.relicKey(k)
	case mNext:
		return m.nextKey(k)
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
			m.modal, m.partAt, m.anchor = mDetail, 0, -1
			return m, m.wantImage()
		}
	case "w":
		if it, ok := m.current(); ok && it.WikiURL != "" {
			if err := openExternal(it.WikiURL); err != nil {
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
	case "N":
		m.openNext()
	case "R":
		m.openRelics()
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
			err = openExternal(path)
		}
		return openedMsg{err}
	}
}

// openExternal hands a URL or file to the desktop's default application.
func openExternal(target string) error {
	return exec.Command("xdg-open", target).Start()
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

package ui

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/Peejidle/seshat/internal/profile"
)

// The ledger and the import prompts.

// ledgerField is one ledger entry. Intrinsics are entered as total ranks,
// the number the game shows, and stored as XP.
type ledgerField struct {
	label   string
	perUnit int // XP per entered unit
	maxRank int // 0 for XP fields
}

var ledgerFields = []ledgerField{
	{"Star Chart", 1, 0},
	{"Steel Path", 1, 0},
	{"Railjack intrinsics", 1500, 50}, // 5 schools x rank 10
	{"Drifter intrinsics", 1500, 40},  // 4 schools x rank 10
	{"Other", 1, 0},
}

func (m *Model) extraPtrs() []*int {
	e := &m.prog.Extra
	return []*int{&e.StarChart, &e.SteelPath, &e.Railjack, &e.Drifter, &e.Other}
}

func (m *Model) openLedger() {
	m.modal, m.ledgerAt = mLedger, 0
	m.status = ""
	m.ledger = make([]textinput.Model, len(ledgerFields))
	for i, p := range m.extraPtrs() {
		t := textinput.New()
		t.Prompt = ""
		t.CharLimit = 9
		t.Width = 12
		t.SetValue(strconv.Itoa(*p / ledgerFields[i].perUnit))
		m.ledger[i] = t
	}
	m.ledger[0].Focus()
}

func (m Model) ledgerKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "esc":
		m.modal = mNone
		m.say("Ledger closed.")
		return m, nil
	case "enter":
		// Save and move on, so several fields can be filled in one visit.
		vals := make([]int, len(m.ledger))
		for i := range m.ledger {
			f := ledgerFields[i]
			v, err := strconv.Atoi(strings.ReplaceAll(strings.TrimSpace(m.ledger[i].Value()), ",", ""))
			switch {
			case err != nil || v < 0:
				m.fail("%s must be a whole number", f.label)
				return m, nil
			case f.maxRank > 0 && v > f.maxRank:
				m.fail("%s: %d ranks at most (every school at 10)", f.label, f.maxRank)
				return m, nil
			}
			vals[i] = v * f.perUnit
		}
		for i, p := range m.extraPtrs() {
			*p = vals[i]
		}
		m.say("✓ Ledger inscribed.")
		m.save() // replaces the message if writing fails
		m.ledger[m.ledgerAt].Blur()
		m.ledgerAt = (m.ledgerAt + 1) % len(m.ledger)
		return m, m.ledger[m.ledgerAt].Focus()
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
	p, err := profile.Load(path)
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

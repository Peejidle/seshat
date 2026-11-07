package ui

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// The ledger and the import prompts.

// ledgerField is one ledger entry. Intrinsics are entered as total ranks,
// the number the game shows, and stored as XP.
type ledgerField struct {
	label   string
	perUnit int    // XP per entered unit
	maxRank int    // 0 for XP fields
	unit    string // what to type, shown beside the box
}

var ledgerFields = []ledgerField{
	// Nodes give different XP each, so a count of them can't be turned
	// into XP: it has to be the game's own figure.
	{"Star Chart", 1, 0, "xp, not the number of nodes"},
	{"Steel Path", 1, 0, "xp, not the number of nodes"},
	{"Railjack intrinsics", 1500, 50, "ranks of 50, all schools added up"}, // 5 schools x rank 10
	{"Drifter intrinsics", 1500, 40, "ranks of 40, all schools added up"},  // 4 schools x rank 10
	{"Other", 1, 0, "xp"},
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
		path := expandHome(strings.TrimSpace(m.pathIn.Value()))
		if err := m.prog.Import(path); err != nil {
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

// expandHome turns a leading ~/ into the home folder, as a shell would:
// Go's file functions take paths literally.
func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}

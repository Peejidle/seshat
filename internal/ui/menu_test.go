package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestMenu(t *testing.T) {
	m := press(testModel(t), ",")
	if m.modal != mMenu {
		t.Fatalf(", should open the menu, modal %d", m.modal)
	}
	// A letter does what it says, as on the main list.
	m = press(m, "H")
	if m.modal != mNone || !m.prog.Settings.HideMastered {
		t.Errorf("H from the menu: modal %d, hidden %v", m.modal, m.prog.Settings.HideMastered)
	}
	// So does choosing a line: down to the ledger, enter.
	m = key(press(m, ",", "j"), tea.KeyEnter)
	if m.modal != mLedger {
		t.Errorf("the ledger line should open the ledger, modal %d", m.modal)
	}
	// q in the menu closes it rather than quitting.
	m = key(m, tea.KeyEsc)
	if m = press(m, ",", "q"); m.modal != mNone {
		t.Errorf("q should close the menu, modal %d", m.modal)
	}
}

// The key bar fits a 160 column terminal whole: past that, the last keys
// (help and quit) get cut off.
func TestHintsFit(t *testing.T) {
	m := testModel(t)
	m.width = 1000
	if w := lipgloss.Width(m.hints()); w > 160 {
		t.Errorf("the key bar is %d wide", w)
	}
}

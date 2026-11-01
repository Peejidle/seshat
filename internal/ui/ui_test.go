package ui

import (
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Peejidle/seshat/internal/catalog"
	"github.com/Peejidle/seshat/internal/store"
)

func testModel(t *testing.T) Model {
	t.Helper()
	m := New(Paths{Progress: filepath.Join(t.TempDir(), "p.json")}, store.New())
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	var items []catalog.Item
	for _, n := range []string{"Ash", "Atlas", "Banshee", "Caliban"} {
		items = append(items, catalog.Item{ID: n, Name: n, Category: catalog.CatWarframes, MaxRank: 30, XPPerRank: 200})
	}
	mm, _ = mm.Update(catalogMsg{items: items})
	return mm.(Model)
}

func press(m Model, keys ...string) Model {
	for _, k := range keys {
		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		if k == " " {
			msg = tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}
		}
		mm, _ := m.Update(msg)
		m = mm.(Model)
	}
	return m
}

func TestBulkMark(t *testing.T) {
	m := press(testModel(t), "j", " ") // master Atlas alone
	m = press(m, "k", "v", "j", "j", " ")
	// Ash..Banshee was mixed, so all three become mastered (and owned).
	for _, id := range []string{"Ash", "Atlas", "Banshee"} {
		if mk := m.prog.Get(id); !mk.Mastered || !mk.Owned {
			t.Errorf("%s = %+v, want mastered+owned", id, mk)
		}
	}
	if m.prog.Get("Caliban").Mastered {
		t.Error("Caliban was outside the selection")
	}
	if m.anchor != -1 {
		t.Error("selection should end after marking")
	}

	// Selecting everything that's already mastered clears it.
	m = press(m, "g", "v", "j", "j", " ")
	if m.prog.Get("Ash").Mastered || m.prog.Get("Banshee").Mastered {
		t.Error("all-mastered selection should be cleared")
	}
}

func TestHideMastered(t *testing.T) {
	m := press(testModel(t), "H", " ")
	if len(m.visible) != 3 {
		t.Fatalf("visible = %d, want 3 after hiding a mastered item", len(m.visible))
	}
	if it, _ := m.current(); it.Name != "Atlas" {
		t.Errorf("cursor on %s, want Atlas (next row)", it.Name)
	}
}

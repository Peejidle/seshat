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

func TestParts(t *testing.T) {
	m := testModel(t)
	m.items[0].Components = []catalog.Component{
		{ID: "bp", Name: "Blueprint", Count: 1, Part: true},
		{ID: "cell", Name: "Orokin Cell", Count: 3},
		{ID: "blade", Name: "Blade", Count: 2, Part: true},
	}
	m.rebuild()
	m = press(m, "enter", "p", "j", "p")
	if held, need := m.partsProgress(m.items[0]); held != 2 || need != 3 {
		t.Fatalf("parts = %d/%d, want 2/3", held, need)
	}
	if m.modal != mDetail {
		t.Fatal("checking off parts closed the details view")
	}
	m = press(m, "p", "p") // 2 of 2 blades, then wraps to 0
	if n := m.prog.HeldParts("Ash", "blade"); n != 0 {
		t.Errorf("blade count = %d, want wrap to 0", n)
	}
	m = press(m, "esc", "f", "f", "f")
	if m.filter != fCollecting || len(m.visible) != 1 {
		t.Errorf("collecting filter shows %d, want just Ash", len(m.visible))
	}
	m = press(m, "enter", "a")
	if _, ok := m.prog.Parts["Ash"]; !ok || m.prog.HeldParts("Ash", "cell") != 0 {
		t.Errorf("a should fill parts only, not resources: %v", m.prog.Parts)
	}
}

func TestLedgerStaysOpen(t *testing.T) {
	m := press(testModel(t), "x")
	m.ledger[0].SetValue("1,000")
	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = mm.(Model)
	if m.modal != mLedger || m.ledgerAt != 1 || m.prog.Extra.StarChart != 1000 {
		t.Fatalf("after enter: modal %v, field %d, star chart %d", m.modal, m.ledgerAt, m.prog.Extra.StarChart)
	}
	m.ledger[1].SetValue("abc")
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m = mm.(Model); !m.statusErr || m.ledgerAt != 1 {
		t.Errorf("bad number should be rejected in place")
	}
	m.ledger[1].SetValue("0")
	m.ledger[2].SetValue("25") // Railjack intrinsic ranks
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m = mm.(Model); m.prog.Extra.Railjack != 25*1500 {
		t.Errorf("25 Railjack ranks stored as %d xp, want %d", m.prog.Extra.Railjack, 25*1500)
	}
	m.ledger[2].SetValue("51")
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m = mm.(Model); !m.statusErr {
		t.Errorf("51 Railjack ranks should be rejected")
	}
	m.ledger[2].SetValue("25")
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m = mm.(Model); m.modal != mNone || m.prog.Extra.StarChart != 1000 {
		t.Errorf("esc should close and keep what was saved")
	}
}

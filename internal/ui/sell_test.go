package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Peejidle/seshat/internal/catalog"
	"github.com/Peejidle/seshat/internal/market"
	"github.com/Peejidle/seshat/internal/store"
)

const marketSample = `{"data":[
	{"slug":"arcane_energize","gameRef":"/arcane/energize","tags":["arcane_enhancement"],"maxRank":5,"i18n":{"en":{"name":"Arcane Energize"}}},
	{"slug":"arcane_grace","gameRef":"/arcane/grace","tags":["arcane_enhancement"],"maxRank":5,"i18n":{"en":{"name":"Arcane Grace"}}},
	{"slug":"nekros_prime_neuroptics_blueprint","gameRef":"/nekros/HelmetBlueprint","tags":["prime","component","blueprint"],"ducats":45,"i18n":{"en":{"name":"Nekros Prime Neuroptics Blueprint"}}},
	{"slug":"nekros_prime_set","gameRef":"/nekros/set","tags":["prime","set"],"i18n":{"en":{"name":"Nekros Prime Set"}}}
]}`

// sellModel is a test model with the market synced and Nekros Prime unbuilt.
func sellModel(t *testing.T) Model {
	t.Helper()
	m := testModel(t)
	mc, err := market.Parse([]byte(marketSample))
	if err != nil {
		t.Fatal(err)
	}
	items := append(m.items, catalog.Item{ID: "/nekros", Name: "Nekros Prime", Category: catalog.CatWarframes,
		Components: []catalog.Component{{ID: "/nekros/HelmetComponent", Name: "Neuroptics", Count: 1, Part: true}}})
	mm, _ := m.Update(catalogMsg{items: items, market: mc})
	return press(mm.(Model), "S")
}

// key sends one special key, such as enter or tab.
func key(m Model, t tea.KeyType) Model {
	mm, _ := m.Update(tea.KeyMsg{Type: t})
	return mm.(Model)
}

// typeIn types text into whichever box has the keys.
func typeIn(m Model, text string) Model {
	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text)})
	return mm.(Model)
}

// addSell adds an item through the form: a, its name, enter, then the
// counts box by box (maxed first for an arcane), and enter to save.
func addSell(m Model, name string, counts ...string) Model {
	if m.sellStep != sellFind {
		m = press(m, "a")
	}
	m = key(typeIn(m, name), tea.KeyEnter)
	for i, c := range counts {
		if i > 0 {
			m = key(m, tea.KeyTab)
		}
		m = typeIn(m, c)
	}
	return key(m, tea.KeyEnter)
}

func TestSellAdd(t *testing.T) {
	m := press(sellModel(t), "a")
	m = typeIn(m, "energ")
	if v := m.sellView(); !strings.Contains(v, "Arcane Energize") {
		t.Errorf("the name box should list what it matches:\n%s", v)
	}
	m = key(m, tea.KeyEnter)
	if m.sellStep != sellCounts || m.sellItem.Name != "Arcane Energize" {
		t.Fatalf("enter should pick Energize and ask how many: step %d, %q", m.sellStep, m.sellItem.Name)
	}
	if v := m.sellView(); !strings.Contains(v, "Maxed (rank 5)") || !strings.Contains(v, "Rank 0 copies") {
		t.Errorf("an arcane should ask for maxed and rank 0:\n%s", v)
	}
	m = typeIn(m, "2x") // only digits go in
	m = key(m, tea.KeyTab)
	m = typeIn(m, "5")
	if m.prog.HeldArcane("/arcane/energize") != (store.Arcane{}) {
		t.Error("nothing is saved before enter")
	}
	m = key(m, tea.KeyEnter)
	if a := m.prog.HeldArcane("/arcane/energize"); a != (store.Arcane{Maxed: 2, Copies: 5}) {
		t.Errorf("energize = %+v, want 2 maxed and 5 copies", a)
	}
	if m.sellStep != sellFind || m.sellIn.Value() != "" {
		t.Errorf("adding should go on to the next name: step %d, %q", m.sellStep, m.sellIn.Value())
	}

	// A part has the one box; several matches are chosen between with ↓.
	m = addSell(m, "nekros neuro", "2")
	if n := m.prog.HeldSpares("/nekros/HelmetBlueprint"); n != 2 {
		t.Errorf("neuroptics = %d, want 2", n)
	}
	m = typeIn(m, "arcane")
	m = key(key(m, tea.KeyDown), tea.KeyEnter)
	if m.sellItem.Name != "Arcane Grace" {
		t.Errorf("↓ should choose the second match, got %q", m.sellItem.Name)
	}
	m = key(key(m, tea.KeyEsc), tea.KeyEsc) // back to the name, then done
	if m.sellStep != sellList {
		t.Errorf("esc twice should be back at the list, at step %d", m.sellStep)
	}
	if len(m.pricing) != 2 {
		t.Errorf("pricing queue = %d, want 2", len(m.pricing))
	}

	// Changing one from the list: its boxes are filled in already.
	m.sellID = "/arcane/energize"
	m = key(m, tea.KeyEnter)
	if m.sellNum[fieldMaxed].Value() != "2" || m.sellNum[fieldCopies].Value() != "5" {
		t.Errorf("boxes = %q, %q, want 2 and 5", m.sellNum[0].Value(), m.sellNum[1].Value())
	}
	m = key(key(m, tea.KeyTab), tea.KeyBackspace)
	m = key(typeIn(m, "8"), tea.KeyEnter)
	if a := m.prog.HeldArcane("/arcane/energize"); a.Copies != 8 || m.sellStep != sellList {
		t.Errorf("after the change = %+v, step %d", a, m.sellStep)
	}
	m = press(m, "-")
	if a := m.prog.HeldArcane("/arcane/energize"); a.Copies != 7 {
		t.Errorf("after - = %+v", a)
	}

	// Emptying the boxes removes it.
	m = key(key(m, tea.KeyEnter), tea.KeyBackspace)
	m = key(key(m, tea.KeyTab), tea.KeyBackspace)
	m = key(m, tea.KeyEnter)
	if a := m.prog.HeldArcane("/arcane/energize"); a != (store.Arcane{}) || !strings.Contains(m.sellMsg, "Removed") {
		t.Errorf("emptied = %+v, %q", a, m.sellMsg)
	}
}

func TestSellView(t *testing.T) {
	m := addSell(sellModel(t), "energize", "2")
	m = addSell(m, "grace", "1")
	m = key(addSell(m, "nekros neuro", "1"), tea.KeyEsc)
	if v := m.sellView(); !strings.Contains(v, "pricing…") {
		t.Errorf("unpriced rows should say so:\n%s", v)
	}

	// Prices arrive: Grace is worth more, so it moves to the top, and the
	// selection stays on what was selected.
	m.sellID = "/arcane/energize"
	now := time.Now()
	for _, p := range []market.Prices{
		{Slug: "arcane_energize", Fetched: now, Ranks: map[int]market.Price{0: {Median: 7}, 5: {Median: 120, Lowest: 119}}},
		{Slug: "arcane_grace", Fetched: now, Ranks: map[int]market.Price{0: {Median: 9}, 5: {Median: 400, Lowest: 390}}},
		{Slug: "nekros_prime_neuroptics_blueprint", Fetched: now, Ranks: map[int]market.Price{0: {Median: 20, Lowest: 22}}},
	} {
		m.prices.Put(p)
	}
	rows := m.sellRows()
	if rows[0].it.Name != "Arcane Grace" || rows[m.sellIndex(rows)].it.Name != "Arcane Energize" {
		t.Errorf("order %v, selected %d", []string{rows[0].it.Name, rows[1].it.Name, rows[2].it.Name}, m.sellIndex(rows))
	}
	m.width = 200
	v := m.sellView()
	for _, want := range []string{"Worth about 650p", "list 2 maxed at 119p", "Nekros Prime isn't built: keep 1"} {
		if !strings.Contains(v, want) {
			t.Errorf("view lacks %q:\n%s", want, v)
		}
	}

	// Once its Neuroptics are put aside for building it, the spares are spare.
	m.prog.SetPart("/nekros", "/nekros/HelmetComponent", 1)
	if strings.Contains(m.sellView(), "isn't built") {
		t.Error("a part already put aside shouldn't be warned about")
	}
}

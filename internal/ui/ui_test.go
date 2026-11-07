package ui

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Peejidle/seshat/internal/catalog"
	"github.com/Peejidle/seshat/internal/mastery"
	"github.com/Peejidle/seshat/internal/relics"
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

func TestMarkInDetailsKeepsItem(t *testing.T) {
	m := press(testModel(t), "H", "j", "enter", " ")
	if it, ok := m.current(); !ok || it.Name != "Atlas" || m.modal != mDetail {
		t.Fatalf("details now show %q (modal %v), want Atlas still open", it.Name, m.modal)
	}
	m = press(m, "q") // any other key closes; the hidden item then drops out
	if it, _ := m.current(); it.Name == "Atlas" {
		t.Errorf("Atlas is mastered and hidden, yet still selected after closing")
	}
}

func TestRelicFinder(t *testing.T) {
	m := testModel(t)
	rel := &relics.Relic{Name: "Lith A1", Farmable: true}
	m.items[0].Components = []catalog.Component{{ID: "bp", Name: "Blueprint", Count: 1, Part: true}}
	m.items[1].Components = []catalog.Component{{ID: "bp2", Name: "Blueprint", Count: 1, Part: true}}
	m.drops = map[string][]relics.Drop{
		partKey("Ash", "bp"):    {{Relic: rel, Reward: relics.Reward{Item: "Ash Blueprint"}}},
		partKey("Atlas", "bp2"): {{Relic: rel}},
	}
	m.relics = &relics.Table{}
	m.prog.Set("Atlas", store.Mark{Owned: true}) // built already, so not needed
	m = press(m, "R")
	es := m.relicEntries()
	if m.modal != mRelics || len(es) != 1 || len(es[0].rows) != 1 || es[0].rows[0].name != "Ash Blueprint" {
		t.Fatalf("entries = %+v", es)
	}
	m.prog.SetPart("Ash", "bp", 1) // now held
	if es := m.relicEntries(); len(es) != 0 {
		t.Errorf("held part still listed: %+v", es)
	}
}

func TestRelicSearch(t *testing.T) {
	m := testModel(t)
	m.relics = &relics.Table{Relics: []*relics.Relic{
		{Name: "Lith A1", Farmable: true, Rewards: []relics.Reward{{Item: "Nyx Prime Chassis Blueprint", Intact: 11}, {Item: "Forma Blueprint", Intact: 25}}},
		{Name: "Axi S8", Rewards: []relics.Reward{{Item: "Nyx Prime Systems Blueprint", Intact: 2}, {Item: "Forma Blueprint", Intact: 25}}},
	}}
	m = press(m, "R", "/", "n", "y", "x")
	es := m.relicEntries()
	if len(es) != 2 || len(es[0].rows) != 1 || !es[0].relic.Farmable {
		t.Fatalf("search nyx = %+v", es) // vaulted Axi S8 is included, after the farmable one
	}
	m.relicQuery.SetValue("axi s8") // a relic's name lists all of it
	if es := m.relicEntries(); len(es) != 1 || len(es[0].rows) != 2 {
		t.Errorf("search axi s8 = %+v", es)
	}
	m = press(m, "q") // still typing: q goes into the search box
	if m.modal != mRelics {
		t.Error("typing q closed the finder")
	}
}

func TestRelicCounts(t *testing.T) {
	m := testModel(t)
	s8 := &relics.Relic{Name: "Axi S8", Tier: 3, Rewards: []relics.Reward{{Item: "Forma Blueprint"}}}
	n1 := &relics.Relic{Name: "Lith N1", Tier: 0, Farmable: true, Rewards: []relics.Reward{{Item: "Forma Blueprint"}}}
	m.relics = &relics.Table{Relics: []*relics.Relic{s8, n1}}

	enter := func(m Model, line string) Model {
		m.relicCount.SetValue(line)
		mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		return mm.(Model)
	}
	m = press(m, "R", "c")
	m = enter(m, "a s8 12, lith n1 4")
	if m.prog.HeldRelics("Axi S8") != 12 || m.prog.HeldRelics("Lith N1") != 4 || !m.relicCounting {
		t.Fatalf("counts %v, still open %v", m.prog.Relics, m.relicCounting)
	}
	m = enter(m, "a s8 +2, l n1 -4")
	if m.prog.HeldRelics("Axi S8") != 14 || m.prog.HeldRelics("Lith N1") != 0 {
		t.Errorf("relative counts %v", m.prog.Relics)
	}
	m = enter(m, "a s8 1, a z9 3") // one bad entry applies nothing
	if !m.relicMsgErr || m.prog.HeldRelics("Axi S8") != 14 || m.relicCount.Value() == "" {
		t.Errorf("bad entry: msg %q, counts %v", m.relicMsg, m.prog.Relics)
	}

	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = press(mm.(Model), "o") // my relics: only Axi S8 is held
	if es := m.relicEntries(); len(es) != 1 || es[0].relic != s8 {
		t.Fatalf("my relics = %+v", es)
	}
	m = press(m, "+")
	if m.prog.HeldRelics("Axi S8") != 15 {
		t.Errorf("+ gave %d", m.prog.HeldRelics("Axi S8"))
	}
	m = press(m, "c")
	if m = enter(m, "3"); m.prog.HeldRelics("Axi S8") != 3 { // a bare count is for the selected relic
		t.Errorf("bare count gave %d", m.prog.HeldRelics("Axi S8"))
	}
}

func TestRelicWindow(t *testing.T) {
	m := testModel(t)
	rel := &relics.Relic{Name: "Lith A1", Farmable: true, Rewards: []relics.Reward{
		{Item: "Ash Blueprint", Intact: 25}, {Item: "Forma Blueprint", Intact: 25},
	}}
	m.relics = &relics.Table{Relics: []*relics.Relic{rel}}
	m.items[0].Components = []catalog.Component{{ID: "bp", Name: "Blueprint", Count: 1, Part: true}}
	m.drops = map[string][]relics.Drop{partKey("Ash", "bp"): {{Relic: rel, Reward: rel.Rewards[0]}}}
	m.matchRewards()

	m = press(m, "R")
	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m = mm.(Model); m.relicOpen != rel {
		t.Fatal("enter didn't open the relic")
	}
	m = press(m, "1", "2")
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = press(mm.(Model), "-")
	if m.prog.HeldRelics("Lith A1") != 11 || m.relicOpen == nil {
		t.Fatalf("count = %d, open %v", m.prog.HeldRelics("Lith A1"), m.relicOpen != nil)
	}
	m = press(m, "p") // got the Ash blueprint
	if m.prog.HeldParts("Ash", "bp") != 1 {
		t.Error("p didn't mark the part held")
	}
	m = press(m, "j", "p") // Forma isn't a tracked part
	if !m.relicMsgErr {
		t.Error("p on Forma should explain itself")
	}
	m = press(m, "q")
	if m.relicOpen != nil || m.modal != mRelics {
		t.Errorf("q should go back to the list: open %v, modal %v", m.relicOpen != nil, m.modal)
	}
}

func TestRelicSortByName(t *testing.T) {
	m := testModel(t)
	a13 := &relics.Relic{Name: "Lith A13", Rewards: []relics.Reward{{Item: "Ash Blueprint"}}}
	a2 := &relics.Relic{Name: "Lith A2", Rewards: []relics.Reward{{Item: "Forma Blueprint"}}}
	m.relics = &relics.Table{Relics: []*relics.Relic{a13, a2}}
	m.items[0].Components = []catalog.Component{{ID: "bp", Name: "Blueprint", Count: 1, Part: true}}
	m.drops = map[string][]relics.Drop{partKey("Ash", "bp"): {{Relic: a13, Reward: a13.Rewards[0]}}}

	m = press(m, "R")
	m.relicQuery.SetValue("lith a")
	if es := m.relicEntries(); len(es) != 2 || es[0].relic != a13 {
		t.Fatalf("most needed first: %+v", es) // A13 holds a part Ash needs
	}
	if m = press(m, "s"); m.relicEntries()[0].relic != a2 || m.relicAt != 1 {
		t.Errorf("by name: A2 first, cursor still on A13 (at %d)", m.relicAt)
	}
}

func TestRecount(t *testing.T) {
	m := testModel(t)
	var rs []*relics.Relic
	for _, n := range []string{"Lith A1", "Lith A2", "Lith B4", "Lith C1", "Lith C2"} {
		rs = append(rs, &relics.Relic{Name: n})
	}
	m.relics = &relics.Table{Relics: rs}
	m.prog.SetRelics("Lith A1", 3)
	m.prog.SetRelics("Lith A2", 1)
	m.prog.SetRelics("Lith C1", 5)
	m.prog.SetRelics("Lith C2", 2)

	enter := func(m Model, line string) Model {
		m.relicCount.SetValue(line)
		mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		return mm.(Model)
	}
	m = press(m, "R", "r")
	m = enter(m, "")       // Lith A1 stays 3
	m = enter(m, "0")      // Lith A2 is gone
	m = enter(m, "l b4 7") // new one, slotted in after A2
	if m.recountList[m.recountAt].Name != "Lith C1" {
		t.Fatalf("after l b4: at %s", m.recountList[m.recountAt].Name)
	}
	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc}) // pause at C1
	m = press(mm.(Model), "r")
	if m.recountList[m.recountAt].Name != "Lith C1" {
		t.Fatalf("resumed at %s", m.recountList[m.recountAt].Name)
	}
	m = enter(m, "4")
	if !m.prog.RelicsCounted.IsZero() {
		t.Error("stamped before the end")
	}
	m = enter(m, "") // Lith C2, the last
	got := m.prog.Relics
	if m.recounting || got["Lith A1"] != 3 || got["Lith A2"] != 0 || got["Lith B4"] != 7 || got["Lith C1"] != 4 || got["Lith C2"] != 2 {
		t.Errorf("recounting %v, counts %v", m.recounting, got)
	}
	if m.prog.RelicsCounted.IsZero() || m.countedAgo() != "recounted today" {
		t.Errorf("not stamped: %v", m.prog.RelicsCounted)
	}
}

func TestNextUp(t *testing.T) {
	m := testModel(t) // Ash, Atlas, Banshee, Caliban: frames, 6,000 XP each
	bp := catalog.Component{ID: "bp", Name: "Blueprint", Count: 1, Part: true}
	chassis := catalog.Component{ID: "ch", Name: "Chassis", Count: 1, Part: true}
	// Ash: a Market blueprint, but parts to farm.
	m.items[0].CreditBP = 35000
	m.items[0].Components = []catalog.Component{bp, chassis}
	// Atlas: every part held.
	m.items[1].Components = []catalog.Component{bp, chassis}
	// Banshee: a dojo blueprint, then only resources.
	m.items[2].CreditBP, m.items[2].Dojo = 15000, true
	m.items[2].Components = []catalog.Component{bp}
	// Caliban: no easy route, so not listed.
	m.items[3].Components = []catalog.Component{bp}
	m.prog.SetPart("Atlas", "bp", 1)
	m.prog.SetPart("Atlas", "ch", 1)

	var got []string
	for _, n := range m.nextItems() {
		got = append(got, m.items[n.idx].Name)
	}
	if strings.Join(got, ",") != "Atlas,Banshee,Ash" {
		t.Errorf("next up = %v, want ready to build, then dojo, then farm", got)
	}

	m = press(m, "N")
	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = mm.(Model)
	if it, _ := m.current(); m.modal != mDetail || it.Name != "Atlas" {
		t.Fatalf("enter should open Atlas's details, got %v %s", m.modal, it.Name)
	}
	m = press(m, "m", "q") // master it from the details, then go back
	if m.modal != mNext || len(m.nextItems()) != 2 {
		t.Errorf("back in next up with Atlas gone: modal %v, %d items", m.modal, len(m.nextItems()))
	}

	// Groups: tab skips the empty ones, a digit picks one outright.
	m = press(m, "tab")
	if m.nextGroup != int(rDojo)+1 || len(m.nextItems()) != 1 {
		t.Errorf("tab: group %d with %d items, want Dojo with Banshee", m.nextGroup, len(m.nextItems()))
	}
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if m = mm.(Model); m.nextGroup != 0 {
		t.Errorf("shift+tab from the first group should go back to all, got %d", m.nextGroup)
	}
	m = press(m, "7")
	if len(m.nextItems()) != 1 || m.items[m.nextItems()[0].idx].Name != "Ash" || m.modal != mNext {
		t.Errorf("7 should show only the farm group, Ash")
	}
}

func TestNextUpHeldRelicsAndStarted(t *testing.T) {
	m := testModel(t)
	bp := catalog.Component{ID: "bp", Name: "Blueprint", Count: 1, Part: true}
	ch := catalog.Component{ID: "ch", Name: "Chassis", Count: 1, Part: true}
	nv := catalog.Component{ID: "nv", Name: "Neuroptics", Count: 1, Part: true}
	vaulted := &relics.Relic{Name: "Lith O3"}
	// Ash Prime: chassis held, blueprint only in a vaulted relic you hold.
	m.items[0].Prime = true
	m.items[0].Components = []catalog.Component{bp, ch}
	m.drops = map[string][]relics.Drop{partKey("Ash", "bp"): {{Relic: vaulted}}}
	m.prog.SetPart("Ash", "ch", 1)
	// Atlas: 1 of 3 parts, Banshee: 2 of 3, from nowhere Seshat knows.
	for _, n := range []string{"Atlas", "Banshee"} {
		m.items[slices.IndexFunc(m.items, func(it catalog.Item) bool { return it.Name == n })].Components = []catalog.Component{bp, ch, nv}
	}
	m.prog.SetPart("Atlas", "bp", 1)
	m.prog.SetPart("Banshee", "bp", 1)
	m.prog.SetPart("Banshee", "ch", 1)

	names := func() string {
		var got []string
		for _, n := range m.nextItems() {
			got = append(got, m.items[n.idx].Name)
		}
		return strings.Join(got, ",")
	}
	if got := names(); got != "Ash,Banshee,Atlas" {
		t.Errorf("without the relic: %s, want Ash as started, then fewest parts missing", got)
	}
	m.prog.SetRelics("Lith O3", 1)
	if r, _, _ := m.routeFor(m.items[0]); r != rRelics {
		t.Errorf("holding the vaulted relic should make Ash Prime a relic route, got %v", r)
	}
}

func TestTypeRank(t *testing.T) {
	m := testModel(t)
	m.items[0].MaxRank, m.items[0].XPPerRank = 40, 100 // Ash as a Kuva weapon would be
	m = press(m, "enter", "3", "4", "enter")
	if mk := m.prog.Get("Ash"); mk.Rank != 34 || mk.Mastered || !mk.Owned || m.modal != mDetail {
		t.Fatalf("after typing 34: %+v, modal %v", mk, m.modal)
	}
	if xp := m.items[0].EarnedXP(false, 34); mastery.Estimate(m.items, m.prog) != xp {
		t.Errorf("estimate = %d, want %d", mastery.Estimate(m.items, m.prog), xp)
	}
	m = press(m, "9", "esc") // esc drops the half-typed rank, not the details
	if m.rankDigits != "" || m.modal != mDetail || m.prog.Get("Ash").Rank != 34 {
		t.Errorf("esc: digits %q, modal %v, rank %d", m.rankDigits, m.modal, m.prog.Get("Ash").Rank)
	}
	m = press(m, "4", "5", "enter") // past the top: mastered
	if mk := m.prog.Get("Ash"); !mk.Mastered || mk.Rank != 0 {
		t.Errorf("after typing 45: %+v, want mastered", mk)
	}
}

func TestProfileImport(t *testing.T) {
	m := testModel(t) // frames: 200 XP a rank, so rank 30 takes 900,000 affinity
	m.nodeXP = map[string]int{"SolNode108": 25, "MercuryJunction": 1000}
	m = press(m, "P")
	if m.modal != mProfile || m.profStep != profAskID {
		t.Fatalf("P with no ID should ask for it: modal %v, step %v", m.modal, m.profStep)
	}
	// The whole URL pasted in works as well as the ID.
	m.profID.SetValue("https://api.warframe.com/cdn/getProfileViewingData.php?playerId=5A1B2C3D4E5F60718293A4B5")
	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m = mm.(Model); m.prog.Settings.ProfileID != "5a1b2c3d4e5f60718293a4b5" || m.profStep != profCopy {
		t.Fatalf("ID %q, step %v", m.prog.Settings.ProfileID, m.profStep)
	}

	// Something else on the clipboard: an error, and still waiting.
	mm, _ = m.Update(clipboardMsg{data: []byte("hello")})
	if m = mm.(Model); m.profErr == "" || m.profStep != profCopy {
		t.Errorf("a non-profile should be refused: err %q, step %v", m.profErr, m.profStep)
	}
	profileJSON := `{"Results":[{"DisplayName":"Tenno","PlayerLevel":1,
		"LoadOutInventory":{"XPInfo":[{"ItemType":"Ash","XP":900000}]},
		"Missions":[{"Tag":"SolNode108","Completes":1,"Tier":1},{"Tag":"MercuryJunction","Completes":1}]}]}`
	mm, _ = m.Update(clipboardMsg{data: []byte(profileJSON)})
	m = mm.(Model)
	if m.profStep != profDone || !m.prog.Get("Ash").Mastered {
		t.Fatalf("after reading the clipboard: step %v, Ash %+v, err %q", m.profStep, m.prog.Get("Ash"), m.profErr)
	}
	if m.prog.Extra.StarChart != 1025 || m.prog.Extra.SteelPath != 25 {
		t.Errorf("star chart %d, steel path %d, want 1025 and 25", m.prog.Extra.StarChart, m.prog.Extra.SteelPath)
	}
	// 6,000 + 1,050 = 7,050 XP is MR 1, as the profile says.
	if !strings.Contains(strings.Join(m.profLines, " "), "matches the game") {
		t.Errorf("report = %q", m.profLines)
	}
	if m = press(m, "x"); m.modal != mNone {
		t.Error("any key should close the report")
	}

	// Next time the ID is remembered, and a terminal paste works too.
	m = press(m, "P")
	if m.profStep != profCopy {
		t.Fatalf("with an ID saved, P should go straight to copying: step %v", m.profStep)
	}
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(profileJSON), Paste: true})
	if m = mm.(Model); m.profStep != profDone {
		t.Errorf("a pasted profile should import: step %v, err %q", m.profStep, m.profErr)
	}
}

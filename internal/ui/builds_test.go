package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Peejidle/seshat/internal/builds"
	"github.com/Peejidle/seshat/internal/catalog"
)

const buildModsSample = `[
	{"uniqueName":"/Mods/Flow","name":"Flow","polarity":"naramon","baseDrain":4,"fusionLimit":5,"type":"Warframe Mod","compatName":"WARFRAME","levelStats":[{"stats":["+25% Energy Max"]},{"stats":["+50%"]},{"stats":["+75%"]},{"stats":["+100%"]},{"stats":["+125%"]},{"stats":["+150% Energy Max"]}]},
	{"uniqueName":"/Mods/Intensify","name":"Intensify","polarity":"madurai","baseDrain":6,"fusionLimit":5,"type":"Warframe Mod","compatName":"WARFRAME"},
	{"uniqueName":"/Mods/Drift","name":"Power Drift","polarity":"zenurik","baseDrain":4,"fusionLimit":5,"type":"Warframe Mod","compatName":"WARFRAME","isUtility":true},
	{"uniqueName":"/Mods/Corrosive","name":"Corrosive Projection","polarity":"naramon","baseDrain":-2,"fusionLimit":5,"type":"Warframe Mod","compatName":"AURA"},
	{"uniqueName":"/Mods/Serration","name":"Serration","polarity":"madurai","baseDrain":4,"fusionLimit":10,"type":"Primary Mod","compatName":"Rifle"}
]`

const buildArcanesSample = `[
	{"uniqueName":"/Arc/Energize","name":"Arcane Energize","type":"Warframe Arcane","levelStats":[{"stats":["a"]},{"stats":["b"]},{"stats":["c"]},{"stats":["d"]},{"stats":["e"]},{"stats":["f"]}]}
]`

// buildModel is a test model with the mod data loaded and Limbo Prime in
// the archive, its details open.
func buildModel(t *testing.T) Model {
	t.Helper()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, catalog.ModsFile), []byte(buildModsSample), 0o644)
	os.WriteFile(filepath.Join(dir, catalog.ArcanesFile), []byte(buildArcanesSample), 0o644)
	bd, err := builds.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	m := testModel(t)
	limbo := catalog.Item{ID: "/limbo", Name: "Limbo Prime", Category: catalog.CatWarframes, MaxRank: 30,
		Polarities: []string{"madurai", "naramon"}, AuraPolarity: "naramon"}
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	mm, _ = mm.Update(catalogMsg{items: []catalog.Item{limbo}, builds: bd})
	return key(mm.(Model), tea.KeyEnter) // its details
}

func TestBuildEditor(t *testing.T) {
	m := press(buildModel(t), "b")
	if m.modal != mBuilds || m.buildStep != buildEdit {
		t.Fatalf("b in the details should open the editor: modal %d, step %d", m.modal, m.buildStep)
	}
	b, ok := m.prog.Build("/limbo")
	if !ok || b.Slots[0].Has != "madurai" || len(b.ArcanesUnlocked) != 2 {
		t.Fatalf("new build = %+v, %v", b, ok)
	}

	// The first slot: pick Flow by name. It's naramon in a madurai slot.
	m = key(typeIn(key(m, tea.KeyEnter), "flow"), tea.KeyEnter)
	b, _ = m.prog.Build("/limbo")
	if b.Config().Mods[0].ID != "/Mods/Flow" || b.Config().Mods[0].Rank != 5 || m.buildStep != buildEdit {
		t.Fatalf("after picking: %+v, step %d", b.Slots[0], m.buildStep)
	}
	if v := m.buildsView(); !strings.Contains(v, "Flow") || !strings.Contains(v, "Capacity 11 of 30") {
		t.Errorf("view after Flow:\n%s", v)
	}

	// Plan a Forma: p steps madurai -> vazarin -> naramon, which Flow matches.
	m = press(m, "p", "p")
	v := m.buildsView()
	if !strings.Contains(v, "Capacity 5 of 30") || !strings.Contains(v, "Needs 1 Forma (N)") {
		t.Errorf("after planning a Forma:\n%s", v)
	}
	m = press(m, "f") // done in the game
	if v := m.buildsView(); !strings.Contains(v, "everything") {
		t.Errorf("after the Forma's done:\n%s", v)
	}

	// The rank goes down with [.
	m = press(m, "[")
	if b, _ = m.prog.Build("/limbo"); b.Config().Mods[0].Rank != 4 {
		t.Errorf("rank after [ = %d", b.Config().Mods[0].Rank)
	}

	// Moving Flow to the next slot along.
	m = press(m, "m", "l", "m")
	if b, _ = m.prog.Build("/limbo"); b.Config().Mods[1].ID != "/Mods/Flow" || b.Config().Mods[0].ID != "" {
		t.Errorf("after moving: %q, %q", b.Config().Mods[0].ID, b.Config().Mods[1].ID)
	}

	// Left and up to the aura: only auras are offered.
	m = press(m, "h", "k")
	if m.buildSlot != builds.SlotAura {
		t.Fatalf("up from the first row should reach the aura, at %d", m.buildSlot)
	}
	m = key(m, tea.KeyEnter)
	it, cur, _ := m.editing()
	if rows := m.pickRows(it, cur); len(rows) != 1 || rows[0].name != "Corrosive Projection" {
		t.Errorf("aura choices = %+v", rows)
	}
	m = key(m, tea.KeyEnter)
	if v := m.buildsView(); !strings.Contains(v, "+14 from the aura") && !strings.Contains(v, "14 from the aura") {
		t.Errorf("aura bonus missing:\n%s", v)
	}

	// The exilus needs an adapter until u says it's unlocked.
	m = press(m, "l")
	m = key(typeIn(key(m, tea.KeyEnter), "drift"), tea.KeyEnter)
	if v := m.buildsView(); !strings.Contains(v, "Exilus Warframe Adapter") {
		t.Errorf("exilus adapter not asked for:\n%s", v)
	}
	m = press(m, "u")
	if v := m.buildsView(); strings.Contains(v, "Needs") {
		t.Errorf("nothing should be needed now:\n%s", v)
	}

	// An arcane, down at the bottom: its slot needs unlocking, with u on it.
	m = press(m, "u") // on the exilus: locks it again
	m = press(m, "u") // and unlocks it
	m = press(m, "j", "j", "j", "j", "j")
	if m.buildSlot != slotArcane+1 {
		t.Fatalf("down from the exilus should reach the second arcane slot, at %d", m.buildSlot)
	}
	m = key(typeIn(key(m, tea.KeyEnter), "energize"), tea.KeyEnter)
	if v := m.buildsView(); !strings.Contains(v, "Needs Warframe Arcane Adapter") || !strings.Contains(v, "locked") {
		t.Errorf("a locked arcane slot should ask for an adapter:\n%s", v)
	}
	m = press(m, "u")
	if v := m.buildsView(); strings.Contains(v, "Needs") {
		t.Errorf("unlocked, nothing should be needed:\n%s", v)
	}
	m = press(m, "k", "k", "u") // u on an ordinary slot does nothing
	if !strings.Contains(m.buildMsg, "Only the exilus and arcane") {
		t.Errorf("u on a mod slot: %q", m.buildMsg)
	}

	// Notes.
	m = typeIn(press(m, "n"), "duration build")
	m = key(m, tea.KeyEnter)
	if b, _ = m.prog.Build("/limbo"); b.Config().Notes != "duration build" {
		t.Errorf("notes = %q", b.Config().Notes)
	}

	// esc goes back to the details it came from.
	if m = key(m, tea.KeyEsc); m.modal != mDetail {
		t.Errorf("esc should go back to the details, modal %d", m.modal)
	}
}

func TestBuildList(t *testing.T) {
	m := key(buildModel(t), tea.KeyEsc)
	m = press(m, "B")
	if m.modal != mBuilds || !strings.Contains(m.buildsView(), "No builds yet") {
		t.Fatalf("empty list:\n%s", m.buildsView())
	}
	// a, a name, enter: a new build, open in the editor.
	m = key(typeIn(press(m, "a"), "limbo"), tea.KeyEnter)
	if m.buildStep != buildEdit || m.buildID != "/limbo" {
		t.Fatalf("after adding: step %d, %q", m.buildStep, m.buildID)
	}
	// One Intensify fits as Limbo comes: nothing needed, so it's done.
	m = key(typeIn(key(m, tea.KeyEnter), "intensify"), tea.KeyEnter)
	m = key(m, tea.KeyEsc)
	if m.buildStep != buildList {
		t.Fatalf("esc from the editor opened from the list goes back to it, step %d", m.buildStep)
	}
	if v := m.buildsView(); !strings.Contains(v, "Limbo Prime") || !strings.Contains(v, "Done 1") {
		t.Errorf("list:\n%s", v)
	}
	// Deleting, in the editor, takes D twice.
	m = press(key(m, tea.KeyEnter), "D")
	if _, ok := m.prog.Build("/limbo"); !ok {
		t.Error("one D shouldn't delete")
	}
	m = press(m, "D")
	if _, ok := m.prog.Build("/limbo"); ok {
		t.Error("D twice should delete")
	}
}

func TestBuildMoveSlot(t *testing.T) {
	m := press(buildModel(t), "b") // Limbo: madurai, naramon, then six unpolarized
	m = key(typeIn(key(m, tea.KeyEnter), "flow"), tea.KeyEnter)

	// M moves the first slot, Madurai, Flow and all, down to the third.
	m = press(m, "M", "j", "M")
	b, _ := m.prog.Build("/limbo")
	if b.Slots[2].Polarity != "madurai" || b.Slots[2].Has != "madurai" || b.Config().Mods[2].ID != "/Mods/Flow" {
		t.Errorf("third slot after M = %+v", b.Slots[2])
	}
	if b.Slots[0].Polarity != "" || b.Config().Mods[0].ID != "" {
		t.Errorf("first slot after M = %+v, want what the third had", b.Slots[0])
	}
	if p := builds.Work(m.bdata, catalog.Item{MaxRank: 30, Category: catalog.CatWarframes}, b); len(p.Forma) != 0 {
		t.Errorf("moving a slot shouldn't plan a Forma: %v", p.Forma)
	}

	// m moves just the mod: the polarity stays.
	m = press(m, "m", "k", "m")
	b, _ = m.prog.Build("/limbo")
	if b.Config().Mods[0].ID != "/Mods/Flow" || b.Slots[0].Polarity != "" || b.Slots[2].Polarity != "madurai" {
		t.Errorf("after m: first %+v, third %+v", b.Slots[0], b.Slots[2])
	}

	// The aura doesn't move with M.
	m = press(m, "k", "M")
	if m.buildSwap >= 0 || !strings.Contains(m.buildMsg, "stay put") {
		t.Errorf("M on the aura: swap %d, %q", m.buildSwap, m.buildMsg)
	}
}

// Each step of the screen fills the window exactly: a line more and the
// terminal scrolls the top border away.
func TestBuildsFitWindow(t *testing.T) {
	m := press(buildModel(t), "b")
	check := func(what string) {
		t.Helper()
		if h := lipgloss.Height(m.View()); h != m.height {
			t.Errorf("%s is %d lines high in a %d line window", what, h, m.height)
		}
	}
	check("the editor")
	m = key(m, tea.KeyEnter)
	check("the mod picker")
	m = key(typeIn(m, "flow"), tea.KeyEnter)
	check("the editor with a mod")
	m = press(m, "n")
	check("the notes")
	m = key(key(m, tea.KeyEsc), tea.KeyEsc)
	m = key(m, tea.KeyEsc) // back to the main list
	m = press(m, "B")
	check("the list")
	m = press(m, "a")
	check("the item finder")
	m = key(m, tea.KeyEsc)
	m = key(m, tea.KeyEsc)
	m = press(m, "S")
	check("the Sell screen")
}

// Opening the list, the build enter would open is the one highlighted.
func TestBuildListSelection(t *testing.T) {
	m := press(buildModel(t), "b")
	m = key(key(m, tea.KeyEsc), tea.KeyEsc) // back to the main list
	m.buildListID = ""                      // as on a fresh start
	m = press(m, "B")
	if v := m.buildsView(); !strings.Contains(v, "▸ Limbo Prime") {
		t.Errorf("the first build should be highlighted:\n%s", v)
	}
}

func TestBuildExalted(t *testing.T) {
	m := buildModel(t)
	titania := catalog.Item{ID: "/Lotus/Powersuits/Fairy/FairyPrime", Name: "Titania Prime", Category: catalog.CatWarframes, MaxRank: 30}
	exalted := []catalog.Item{
		{ID: "/Lotus/Powersuits/Fairy/FlightSword", Name: "Diwata", Category: catalog.CatMelee, Type: "Melee", MaxRank: 30, Exalted: true},
		{ID: "/Lotus/Powersuits/Fairy/PrimeFlightPistols", Name: "Dex Pixia Prime", Category: catalog.CatSecondary, Type: "Pistol", MaxRank: 30, Exalted: true},
		{ID: "/Lotus/Powersuits/Fairy/PrimeFlightSword", Name: "Diwata Prime", Category: catalog.CatMelee, Type: "Melee", MaxRank: 30, Exalted: true},
	}
	mm, _ := m.Update(catalogMsg{items: []catalog.Item{titania}, builds: m.bdata, exalted: exalted})
	m = mm.(Model)
	m.openBuild(titania, mNone)

	// X goes through Titania Prime's own (Prime) exalted weapons and back.
	var seen []string
	for range 3 {
		m = press(m, "X")
		it, _, _ := m.editing()
		seen = append(seen, it.Name)
	}
	if got := strings.Join(seen, ", "); got != "Dex Pixia Prime, Diwata Prime, Titania Prime" {
		t.Errorf("X went to %s", got)
	}

	// An exalted weapon's build is a weapon's: one arcane slot, its own Catalyst.
	m = press(m, "X")
	b, _ := m.prog.Build("/Lotus/Powersuits/Fairy/PrimeFlightPistols")
	if len(b.ArcanesUnlocked) != 1 {
		t.Errorf("Dex Pixia Prime has %d arcane slots, want 1", len(b.ArcanesUnlocked))
	}
	if v := m.buildsView(); !strings.Contains(v, "Exalted · Titania Prime") || !strings.Contains(v, "Orokin Catalyst") {
		t.Errorf("exalted header:\n%s", v)
	}

	// The list says whose it is, and they can be found by name.
	m = key(m, tea.KeyEsc)
	if v := m.buildsView(); !strings.Contains(v, "Exalted · Titania Prime") {
		t.Errorf("list:\n%s", v)
	}
	m = typeIn(press(m, "a"), "diwata")
	if found := m.buildFindMatches(); len(found) != 2 {
		t.Errorf("finding diwata: %d matches", len(found))
	}
}

func TestBuildConfigs(t *testing.T) {
	m := press(buildModel(t), "b")
	m = key(typeIn(key(m, tea.KeyEnter), "flow"), tea.KeyEnter) // Flow in A's first slot

	// c goes to B, empty, and offers to copy A.
	m = press(m, "c")
	b, _ := m.prog.Build("/limbo")
	if b.Active != 1 || b.Config().Mods[0].ID != "" || !strings.Contains(m.buildMsg, "C copies config A") {
		t.Fatalf("after c: active %d, B = %+v, %q", b.Active, b.Config().Mods[0], m.buildMsg)
	}
	// C copies A into it; B's own changes then leave A alone.
	m = press(m, "C")
	if b, _ = m.prog.Build("/limbo"); b.Config().Mods[0].ID != "/Mods/Flow" {
		t.Fatalf("C should copy A into B: %+v", b.Config().Mods[0])
	}
	m = press(m, "x")
	if b, _ = m.prog.Build("/limbo"); b.Configs[0].Mods[0].ID != "/Mods/Flow" || b.Configs[1].Mods[0].ID != "" {
		t.Errorf("clearing B's slot touched A: A %+v, B %+v", b.Configs[0].Mods[0], b.Configs[1].Mods[0])
	}

	// Polarities are the item's: planning one in B shows in A too.
	m = press(m, "p", "c", "c") // plan a Forma, then round to A
	if b, _ = m.prog.Build("/limbo"); b.Active != 0 || b.Slots[0].Polarity == b.Slots[0].Has {
		t.Errorf("the Forma planned in B should be the item's: %+v, active %d", b.Slots[0], b.Active)
	}

	// Copying over a config with mods in it takes C twice.
	m = press(m, "c")
	m = key(typeIn(key(m, tea.KeyEnter), "intensify"), tea.KeyEnter) // B has a mod now
	m = press(m, "c")                                                // to C, and give it a mod
	m = key(typeIn(key(m, tea.KeyEnter), "flow"), tea.KeyEnter)
	m = press(m, "C")
	if !strings.Contains(m.buildMsg, "Press C again") {
		t.Errorf("C over a config with mods should ask first: %q", m.buildMsg)
	}
	m = press(m, "C")
	if b, _ = m.prog.Build("/limbo"); b.Configs[2].Mods[0].ID != b.Configs[1].Mods[0].ID {
		t.Errorf("second C should copy B into C: %+v", b.Configs[2].Mods[0])
	}

	// The tabs show which config is open.
	if v := m.buildsView(); !strings.Contains(v, "Config") || !strings.Contains(v, "c switch") {
		t.Errorf("no config tabs:\n%s", v)
	}
}

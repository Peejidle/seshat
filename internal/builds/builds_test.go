package builds

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Peejidle/seshat/internal/catalog"
	"github.com/Peejidle/seshat/internal/store"
)

const modsSample = `[
	{"uniqueName":"/Mods/Beginner/Serration","name":"Serration","polarity":"madurai","baseDrain":2,"fusionLimit":3,"type":"Primary Mod","compatName":"Rifle"},
	{"uniqueName":"/Mods/Serration","name":"Serration","polarity":"madurai","baseDrain":4,"fusionLimit":10,"type":"Primary Mod","compatName":"Rifle"},
	{"uniqueName":"/Mods/Vigilante","name":"Vigilante Armaments","polarity":"naramon","baseDrain":4,"fusionLimit":5,"type":"Primary Mod","compatName":"Rifle","isUtility":true},
	{"uniqueName":"/Mods/Bow","name":"Thunderbolt","polarity":"madurai","baseDrain":4,"fusionLimit":3,"type":"Primary Mod","compatName":"Bow"},
	{"uniqueName":"/Mods/Tiberon","name":"Entropy Burst","polarity":"madurai","baseDrain":6,"fusionLimit":3,"type":"Primary Mod","compatName":"Tiberon"},
	{"uniqueName":"/Mods/Blaze","name":"Blaze","polarity":"madurai","baseDrain":6,"fusionLimit":3,"type":"Shotgun Mod","compatName":"Shotgun"},
	{"uniqueName":"/Mods/Flow","name":"Flow","polarity":"naramon","baseDrain":4,"fusionLimit":5,"type":"Warframe Mod","compatName":"WARFRAME"},
	{"uniqueName":"/Mods/Cataclysmic","name":"Cataclysmic Continuum","polarity":"zenurik","baseDrain":6,"fusionLimit":3,"type":"Warframe Mod","compatName":"Limbo"},
	{"uniqueName":"/Mods/Corrosive","name":"Corrosive Projection","polarity":"naramon","baseDrain":-2,"fusionLimit":5,"type":"Warframe Mod","compatName":"AURA"},
	{"uniqueName":"/Mods/Riven","name":"Rifle Riven Mod","polarity":"naramon","baseDrain":10,"fusionLimit":8,"type":"Rifle Riven Mod","compatName":"Rifle"},
	{"uniqueName":"/Mods/Stance","name":"Crossing Snakes","polarity":"madurai","baseDrain":-2,"fusionLimit":3,"type":"Stance Mod","compatName":"Swords"}
]`

const arcanesSample = `[
	{"uniqueName":"/Arc/Energize","name":"Arcane Energize","type":"Warframe Arcane","levelStats":[{"stats":["a"]},{"stats":["b"]},{"stats":["c"]},{"stats":["d"]},{"stats":["e"]},{"stats":["f"]}]},
	{"uniqueName":"/Arc/Merciless","name":"Primary Merciless","type":"Primary Arcane","levelStats":[{"stats":["a"]},{"stats":["b"]}]}
]`

func sampleData(t *testing.T) *Data {
	t.Helper()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, catalog.ModsFile), []byte(modsSample), 0o644)
	os.WriteFile(filepath.Join(dir, catalog.ArcanesFile), []byte(arcanesSample), 0o644)
	d, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

var (
	limbo   = catalog.Item{ID: "/limbo", Name: "Limbo Prime", Category: catalog.CatWarframes, MaxRank: 30, Polarities: []string{"madurai", "naramon"}, AuraPolarity: "naramon"}
	tiberon = catalog.Item{ID: "/tiberon", Name: "Tiberon Prime", Category: catalog.CatPrimary, Type: "Rifle", MaxRank: 30, Polarities: []string{"madurai"}, ExilusPolarity: "madurai"}
	hek     = catalog.Item{ID: "/hek", Name: "Vaykor Hek", Category: catalog.CatPrimary, Type: "Shotgun", MaxRank: 30}
)

func TestLoad(t *testing.T) {
	d := sampleData(t)
	m, ok := d.Mod("/Mods/Serration")
	if !ok || m.MaxRank != 10 {
		t.Errorf("Serration = %+v, %v: want the plain rank 10 one", m, ok)
	}
	if _, ok := d.Mod("/Mods/Beginner/Serration"); ok {
		t.Error("the beginner Serration should be left out")
	}
	if _, ok := d.Mod("/Mods/Riven"); ok {
		t.Error("rivens should be left out")
	}
	if a, _ := d.Arcane("/Arc/Energize"); a.MaxRank != 5 {
		t.Errorf("Energize max rank = %d, want 5", a.MaxRank)
	}
}

func TestFits(t *testing.T) {
	d := sampleData(t)
	fits := func(it catalog.Item, slot int) (names []string) {
		for _, m := range d.Mods {
			if Fits(m, it, slot) {
				names = append(names, m.Name)
			}
		}
		return names
	}
	for _, c := range []struct {
		it   catalog.Item
		slot int
		want []string
	}{
		{limbo, 0, []string{"Cataclysmic Continuum", "Flow"}},
		{limbo, SlotAura, []string{"Corrosive Projection"}},
		{tiberon, 0, []string{"Entropy Burst", "Serration", "Vigilante Armaments"}},
		{tiberon, SlotExilus, []string{"Vigilante Armaments"}},
		{tiberon, SlotAura, nil},
		{hek, 0, []string{"Blaze"}},
	} {
		if got := fits(c.it, c.slot); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s slot %d fits %q, want %q", c.it.Name, c.slot, got, c.want)
		}
	}
	e, _ := d.Arcane("/Arc/Energize")
	if !ArcaneFits(e, limbo) || ArcaneFits(e, tiberon) {
		t.Error("Energize should fit Limbo and not the Tiberon")
	}
}

func TestDrain(t *testing.T) {
	d := sampleData(t)
	s, _ := d.Mod("/Mods/Serration")  // 4 + 10 = 14 at max
	cp, _ := d.Mod("/Mods/Corrosive") // gives 2 + 5 = 7 at max
	for _, c := range []struct {
		m    Mod
		rank int
		slot Polarity
		want int
	}{
		{s, 10, None, 14},
		{s, 10, Madurai, 7},
		{s, 9, Madurai, 7},   // 13 halves to 6.5, rounded up
		{s, 10, Naramon, 18}, // 17.5 rounds to 18
		{s, 9, Naramon, 16},  // 16.25 rounds to 16
		{s, 10, Universal, 7},
		{cp, 5, None, -7},
		{cp, 5, Naramon, -14},
		{cp, 5, Madurai, -5}, // 5.25
	} {
		if got := Drain(c.m, c.rank, c.slot); got != c.want {
			t.Errorf("%s rank %d in %q = %d, want %d", c.m.Name, c.rank, c.slot, got, c.want)
		}
	}
}

func TestNew(t *testing.T) {
	b := New(limbo)
	if len(b.Slots) != NumSlots || len(b.ArcanesUnlocked) != 2 || len(b.Configs) != 3 || len(b.Configs[2].Arcanes) != 2 {
		t.Fatalf("Limbo: %d slots, %d arcane slots, %d configs", len(b.Slots), len(b.ArcanesUnlocked), len(b.Configs))
	}
	if b.Slots[0].Has != "madurai" || b.Slots[1].Has != "naramon" || b.Slots[SlotAura].Has != "naramon" || b.Slots[2].Has != "" {
		t.Errorf("Limbo's polarities: %+v", b.Slots)
	}
	if len(New(tiberon).ArcanesUnlocked) != 1 {
		t.Error("a weapon has one arcane slot")
	}
}

func TestWork(t *testing.T) {
	d := sampleData(t)
	b := New(limbo)
	put := func(slot int, mod string, rank int) { b.Configs[0].Mods[slot] = store.BuildMod{ID: mod, Rank: rank} }
	put(SlotAura, "/Mods/Corrosive", 5) // +14 in its naramon slot
	put(0, "/Mods/Flow", 5)             // 9, mismatched in madurai: 11
	put(1, "/Mods/Flow", 5)             // as if a second; naramon: 5
	p := Work(d, limbo, b)
	if p.Used != 16 || p.Bonus != 14 || p.Capacity != 30 || p.NeedReactor || !p.Done() {
		t.Errorf("small build: %+v", p)
	}

	// More than 30 + 14 needs a Reactor.
	for i := 2; i < NumMods; i++ {
		put(i, "/Mods/Cataclysmic", 3) // 9 each unpolarized
	}
	p = Work(d, limbo, b)
	if p.Used != 70 || !p.NeedReactor || p.Capacity != 60 || p.Free() != 4 {
		t.Errorf("big build: %+v", p)
	}
	if got := p.Missing(Warframe); !reflect.DeepEqual(got, []string{"Orokin Reactor"}) {
		t.Errorf("missing %q", got)
	}

	// Plans: two Forma, an Aura Forma, the exilus, and the Reactor in.
	b.Reactor = true
	b.Slots[2].Polarity, b.Slots[3].Polarity = "zenurik", "vazarin"
	b.Slots[SlotAura].Polarity = "madurai"
	put(SlotExilus, "/Mods/Flow", 0)
	p = Work(d, limbo, b)
	want := []string{"2 Forma (Z, V)", "Aura Forma", "Exilus Warframe Adapter"}
	if got := p.Missing(Warframe); !reflect.DeepEqual(got, want) || p.Done() {
		t.Errorf("missing %q, want %q", got, want)
	}

	// Done in the game, and with room made (the aura's mismatched now, so
	// gives less): nothing left.
	for i := 4; i < NumMods; i++ {
		put(i, "", 0)
	}
	b.Slots[2].Has, b.Slots[3].Has, b.Slots[SlotAura].Has = "zenurik", "vazarin", "madurai"
	b.Exilus = true
	if p = Work(d, limbo, b); !p.Done() {
		t.Errorf("all applied, still missing %q", p.Missing(Warframe))
	}
	if p.Free() < 0 {
		t.Errorf("should fit: %+v", p)
	}

	// Arcanes in locked slots each need an adapter; an empty slot doesn't.
	b.Configs[0].Arcanes[0].ID, b.Configs[0].Arcanes[1].ID = "/Arc/Energize", "/Arc/Energize"
	if got := Work(d, limbo, b).Missing(Warframe); !reflect.DeepEqual(got, []string{"2 Warframe Arcane Adapters"}) {
		t.Errorf("two locked arcanes: missing %q", got)
	}
	b.ArcanesUnlocked[0] = true
	if got := Work(d, limbo, b).Missing(Warframe); !reflect.DeepEqual(got, []string{"Warframe Arcane Adapter"}) {
		t.Errorf("one locked arcane: missing %q", got)
	}
	b.Configs[0].Arcanes[1] = store.BuildMod{}
	if p = Work(d, limbo, b); !p.Done() {
		t.Errorf("unlocked, or no arcane planned: still missing %q", p.Missing(Warframe))
	}
}

func TestCleanStat(t *testing.T) {
	for in, want := range map[string][]string{
		"Abilities that inflict a <DT_POISON_COLOR>Toxin status effect": {"Abilities that inflict a Toxin status effect"},
		"+30% chance to apply <DT_SLASH_COLOR> on Critical":             {"+30% chance to apply Slash on Critical"},
		"+6% <DT_SENTIENT>Tau Resistance":                               {"+6% Tau Resistance"},
		"On Energy Pickup:\\n60% chance to replenish 25 Energy\\n15s cooldown": {
			"On Energy Pickup: 60% chance to replenish 25 Energy", "15s cooldown"},
		"Reload <LOWER_IS_BETTER>8s":       {"Reload 8s"},
		"Line one<LINE_SEPARATOR>Line two": {"Line one", "Line two"},
	} {
		if got := cleanStat(in); !reflect.DeepEqual(got, want) {
			t.Errorf("cleanStat(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestConfigs(t *testing.T) {
	d := sampleData(t)
	b := New(limbo)
	// A is small; B is too big for 30 + the aura, so the item needs a
	// Reactor, whichever config is being looked at.
	b.Configs[0].Mods[0] = store.BuildMod{ID: "/Mods/Flow", Rank: 5}
	for i := 0; i < NumMods; i++ {
		b.Configs[1].Mods[i] = store.BuildMod{ID: "/Mods/Cataclysmic", Rank: 3}
	}
	p := Work(d, limbo, b)
	if p.Used != 11 || !p.NeedReactor || p.Capacity != 60 {
		t.Errorf("looking at A: %+v", p)
	}
	b.Active = 1
	if p = Work(d, limbo, b); p.Used == 11 || !p.NeedReactor {
		t.Errorf("looking at B: %+v", p)
	}

	// The exilus and arcanes in any config count.
	b.Configs[2].Mods[SlotExilus] = store.BuildMod{ID: "/Mods/Flow"}
	b.Configs[2].Arcanes[1] = store.BuildMod{ID: "/Arc/Energize", Rank: 5}
	p = Work(d, limbo, b)
	if !p.NeedExilus || p.ArcaneLocks != 1 {
		t.Errorf("C's exilus and arcane: %+v", p)
	}
}

func TestUpgradeOldBuild(t *testing.T) {
	// A build saved before configs: the mods were in the slots.
	old := `{"builds":{"/limbo":{"slots":[{"mod":"/Mods/Flow","rank":5,"polarity":"naramon","has":"naramon"}],
		"arcanes":[{"id":"/Arc/Energize","rank":5,"unlocked":true},{}],"notes":"duration","reactor":true}}}`
	dir := t.TempDir()
	path := filepath.Join(dir, "p.json")
	os.WriteFile(path, []byte(old), 0o644)
	p, err := store.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	b := p.Builds["/limbo"]
	if len(b.Configs) != 3 || b.Configs[0].Mods[0] != (store.BuildMod{ID: "/Mods/Flow", Rank: 5}) ||
		b.Configs[0].Arcanes[0].ID != "/Arc/Energize" || b.Configs[0].Notes != "duration" ||
		!b.ArcanesUnlocked[0] || b.ArcanesUnlocked[1] || !b.Reactor || b.Slots[0].Has != "naramon" {
		t.Errorf("upgraded: %+v", b)
	}
	if len(b.Configs[1].Mods) != 1 || len(b.Configs[1].Arcanes) != 2 {
		t.Errorf("B should be empty but shaped like A: %+v", b.Configs[1])
	}
}

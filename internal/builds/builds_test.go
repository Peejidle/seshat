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

func TestParseStat(t *testing.T) {
	for line, want := range map[string]struct {
		s  Stat
		v  float64
		ok bool
	}{
		"+440% Health":                  {Health, 4.4, true},
		"-60% Ability Range":            {Range, -0.6, true},
		"+27.5% Ability Duration":       {Duration, 0.275, true},
		"+185% Energy Max":              {Energy, 1.85, true},
		"+30% Ability Strength on kill": {0, 0, false},
		"Enemies lose -18% Armor":       {0, 0, false},
	} {
		s, v, ok := parseStat(line)
		if ok != want.ok || (ok && (s != want.s || v < want.v-1e-9 || v > want.v+1e-9)) {
			t.Errorf("parseStat(%q) = %v, %v, %v", line, s, v, ok)
		}
	}
}

func TestFrameStats(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, catalog.ModsFile), []byte(`[
		{"uniqueName":"/Vit","name":"Vitality","polarity":"vazarin","baseDrain":2,"fusionLimit":1,"type":"Warframe Mod","compatName":"WARFRAME","levelStats":[{"stats":["+40% Health"]},{"stats":["+440% Health"]}]},
		{"uniqueName":"/Over","name":"Overextended","polarity":"madurai","baseDrain":6,"fusionLimit":0,"type":"Warframe Mod","compatName":"WARFRAME","levelStats":[{"stats":["+90% Ability Range","-60% Ability Strength"]}]},
		{"uniqueName":"/Cat","name":"Catalyzing Shields","polarity":"vazarin","baseDrain":2,"fusionLimit":0,"type":"Warframe Mod","compatName":"WARFRAME","levelStats":[{"stats":["x0.20 Max Shield Capacity"]}]},
		{"uniqueName":"/Aug","name":"Some Augment","polarity":"zenurik","baseDrain":6,"fusionLimit":0,"type":"Warframe Mod","compatName":"Titania","levelStats":[{"stats":["+20% Ability Duration","Lantern explodes on kill"]}]}
	]`), 0o644)
	os.WriteFile(filepath.Join(dir, catalog.ArcanesFile), []byte(`[]`), 0o644)
	d, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Titania Prime as WFCD has her (rank 0); the arsenal shows 465
	// health, 370 shields, 135 armor and 265 energy at rank 30.
	titania := catalog.Item{Name: "Titania Prime", Category: catalog.CatWarframes, MaxRank: 30,
		Health: 365, Shield: 270, Armor: 135, Energy: 215, SprintSpeed: 1}
	b := New(titania)
	st := FrameStats(d, titania, b)
	if st.Base[Health] != 465 || st.Base[Shields] != 370 || st.Base[Armor] != 135 || st.Base[Energy] != 265 || st.Base[Strength] != 1 {
		t.Errorf("rank 30 base: %v", st.Base)
	}

	b.Configs[0].Mods[0] = store.BuildMod{ID: "/Vit", Rank: 1}
	b.Configs[0].Mods[1] = store.BuildMod{ID: "/Over"}
	b.Configs[0].Mods[2] = store.BuildMod{ID: "/Aug"}
	b.Configs[0].Mods[3] = store.BuildMod{ID: "/Cat"}
	b.Shards[0] = store.BuildShard{ID: "crimson-strength", Tauforged: true} // +15%
	b.Shards[1] = store.BuildShard{ID: "azure-health"}                      // +150
	b.Shards[2] = store.BuildShard{ID: "amber-casting"}                     // not on the panel
	st = FrameStats(d, titania, b)
	near := func(a, b float64) bool { return a > b-1e-6 && a < b+1e-6 }
	for s, want := range map[Stat]float64{
		Health:   465*5.4 + 150,
		Range:    1.9,
		Strength: 1 - 0.6 + 0.15,
		Duration: 1.2,
		Energy:   265,
		Shields:  370 * 0.2,
	} {
		if !near(st.Final[s], want) {
			t.Errorf("%s = %v, want %v", s.Name(), st.Final[s], want)
		}
	}
	if len(st.Uncounted) != 1 || st.Uncounted[0] != "Some Augment" {
		t.Errorf("uncounted = %q", st.Uncounted)
	}

	// Shards not yet in the game are what the item still needs.
	b.Shards[1].Installed = true
	got := Work(d, titania, b).Missing(Warframe)
	if !reflect.DeepEqual(got, []string{"2 Archon Shards (Tauforged Crimson, Amber)"}) {
		t.Errorf("missing %q", got)
	}
}

func TestShardAmount(t *testing.T) {
	for id, want := range map[string][2]string{
		"crimson-strength":   {"+10% Ability Strength", "+15% Ability Strength"},
		"azure-regen":        {"+5 Health/s Regenerated", "+7.5 Health/s Regenerated"},
		"topaz-blast-health": {"Gain 1 Health per Blast kill, max 300 Health", "Gain 2 Health per Blast kill, max 450 Health"},
		"violet-melee-crit":  {"+25% Melee Critical Damage, doubled over 500 max Energy", "+37% Melee Critical Damage, doubled over 500 max Energy"},
		"violet-pickups":     {"Health pickups give +20% Energy, Energy pickups +20% Health", "Health pickups give +30% Energy, Energy pickups +30% Health"},
	} {
		o, ok := ShardByID(id)
		if !ok {
			t.Fatalf("no shard %q", id)
		}
		if got := o.Amount(false); got != want[0] {
			t.Errorf("%s = %q, want %q", id, got, want[0])
		}
		if got := o.Amount(true); got != want[1] {
			t.Errorf("Tauforged %s = %q, want %q", id, got, want[1])
		}
	}
	colours := map[string]int{}
	for _, o := range ShardOptions {
		colours[o.Colour]++
	}
	if len(colours) != 6 {
		t.Errorf("colours: %v", colours)
	}
}

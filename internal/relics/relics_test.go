package relics

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// A cut-down drops file: two relics (one vaulted), each in two refinements,
// an event relic, and two missions and a bounty that reward one of them.
const sample = `{
 "missionRewards": {"Earth": {"Mantle": {"gameMode": "Capture", "rewards": [
  {"itemName": "Lith N1 Relic", "rarity": "Uncommon", "chance": 10}
 ]}, "Lith": {"gameMode": "Defense", "rewards": {"A": [], "B": [
  {"itemName": "Lith N1 Relic", "rarity": "Uncommon", "chance": 20}
 ]}}}},
 "cetusBountyRewards": [{"rewards": {"A": [{"itemName": "Lith N1 Relic", "chance": 5}]}}],
 "relics": [
  {"tier": "Lith", "relicName": "N1", "state": "Intact", "rewards": [
   {"itemName": "Nikana Prime Blueprint", "rarity": "Uncommon", "chance": 25.33},
   {"itemName": "Nikana Prime Hilt", "rarity": "Rare", "chance": 2}]},
  {"tier": "Lith", "relicName": "N1", "state": "Radiant", "rewards": [
   {"itemName": "Nikana Prime Blueprint", "rarity": "Uncommon", "chance": 16.67},
   {"itemName": "Nikana Prime Hilt", "rarity": "Uncommon", "chance": 10}]},
  {"tier": "Axi", "relicName": "V1", "state": "Intact", "rewards": [
   {"itemName": "Nikana Prime Blueprint", "rarity": "Uncommon", "chance": 11},
   {"itemName": "Nikana Prime Blade", "rarity": "Uncommon", "chance": 11}]},
  {"tier": "Vanguard", "relicName": "C1", "state": "Intact", "rewards": [
   {"itemName": "Forma Blueprint", "rarity": "Uncommon", "chance": 25.33}]},
  {"tier": "Requiem", "state": "Intact", "rewards": [{"itemName": "Lohk", "chance": 11}]}
 ]
}`

func load(t *testing.T) *Table {
	t.Helper()
	path := filepath.Join(t.TempDir(), "drops.json")
	if err := os.WriteFile(path, []byte(sample), 0o644); err != nil {
		t.Fatal(err)
	}
	tb, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return tb
}

func TestLoad(t *testing.T) {
	tb := load(t)
	if len(tb.Relics) != 3 {
		t.Fatalf("got %d relics, want 3 (Requiem skipped, refinements merged)", len(tb.Relics))
	}
	if v := tb.Relics[2]; v.Name != "Vanguard C1" || !v.Event() || v.Farmable || tb.Relics[0].Event() {
		t.Errorf("event relic = %+v", v)
	}
	lith, axi := tb.Relics[0], tb.Relics[1]
	if lith.Name != "Lith N1" || !lith.Farmable || axi.Farmable {
		t.Errorf("farmable: %s %v, %s %v", lith.Name, lith.Farmable, axi.Name, axi.Farmable)
	}
	hilt := lith.Rewards[1]
	if hilt.Intact != 2 || hilt.Radiant != 10 || hilt.Rarity != "Rare" || lith.Rewards[0].Rarity != "Common" {
		t.Errorf("rewards = %+v", lith.Rewards)
	}
}

func TestMatch(t *testing.T) {
	tb := load(t)
	// Seshat calls the hilt "Handle", after DE's internal name.
	got := tb.Match("Nikana Prime", []string{"Blueprint", "Blade", "Handle"})
	bp, blade, hilt := got[0], got[1], got[2]
	if len(bp) != 2 || bp[0].Relic.Name != "Lith N1" {
		t.Errorf("blueprint drops = %+v, want both relics, farmable first", bp)
	}
	if len(blade) != 1 || blade[0].Relic.Name != "Axi V1" {
		t.Errorf("blade drops = %+v", blade)
	}
	if len(hilt) != 1 || hilt[0].PartName("Nikana Prime") != "Hilt" {
		t.Errorf("leftover Handle should pair with the Hilt: %+v", hilt)
	}
	if got := tb.Match("Braton", []string{"Blueprint"}); got[0] != nil {
		t.Errorf("non-prime matched %+v", got)
	}
}

func TestFind(t *testing.T) {
	tb := load(t)
	for _, q := range [][2]string{{"Lith", "N1"}, {"lith", "n1"}, {"l", "n1"}} {
		if r := tb.Find(q[0], q[1]); r == nil || r.Name != "Lith N1" {
			t.Errorf("Find(%q, %q) = %v", q[0], q[1], r)
		}
	}
	if r := tb.Find("v", "c1"); r == nil || r.Name != "Vanguard C1" {
		t.Errorf("Find(v, c1) = %v", r)
	}
	if r := tb.Find("m", "n1"); r != nil {
		t.Errorf("wrong tier found %v", r.Name)
	}
}

func TestSources(t *testing.T) {
	lith := load(t).Relics[0]
	want := []Source{{"Earth", "Lith", "Defense", "B", 20}, {"Earth", "Mantle", "Capture", "", 10}}
	if len(lith.Missions) != 2 || lith.Missions[0] != want[0] || lith.Missions[1] != want[1] {
		t.Errorf("missions = %+v", lith.Missions)
	}
	if len(lith.Elsewhere) != 1 || lith.Elsewhere[0] != "Cetus bounties" {
		t.Errorf("elsewhere = %v", lith.Elsewhere)
	}
}

func TestLess(t *testing.T) {
	r := func(name string, tier int) *Relic { return &Relic{Name: name, Tier: tier} }
	in := []*Relic{r("Meso A1", 1), r("Lith A13", 0), r("Lith B1", 0), r("Lith A2", 0)}
	sort.Slice(in, func(i, j int) bool { return Less(in[i], in[j]) })
	var got []string
	for _, x := range in {
		got = append(got, x.Name)
	}
	if strings.Join(got, ", ") != "Lith A2, Lith A13, Lith B1, Meso A1" {
		t.Errorf("order = %v", got)
	}
}

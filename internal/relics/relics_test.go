package relics

import (
	"os"
	"path/filepath"
	"testing"
)

// A cut-down drops file: two relics (one vaulted), each in two refinements,
// and a mission that rewards one of them.
const sample = `{
 "missionRewards": {"Earth": {"Mantle": {"rewards": {"A": [
  {"itemName": "Lith N1 Relic", "rarity": "Uncommon", "chance": 10}
 ]}}}},
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
	if len(tb.Relics) != 2 {
		t.Fatalf("got %d relics, want 2 (Requiem skipped, refinements merged)", len(tb.Relics))
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
	if r := tb.Find("m", "n1"); r != nil {
		t.Errorf("wrong tier found %v", r.Name)
	}
}

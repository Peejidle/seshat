package catalog

import (
	"strings"
	"testing"
)

func TestClassifyModular(t *testing.T) {
	cases := []struct {
		file string
		raw  rawItem
		cat  Category
		typ  string
		xp   int
	}{
		{"Misc", rawItem{UniqueName: "/Lotus/Weapons/Sentients/OperatorAmplifiers/Set1/Barrel/SentAmpSet1BarrelPartA", Type: "Amp"}, CatModular, "Amp", 3000},
		{"Misc", rawItem{UniqueName: "/Lotus/Weapons/Sentients/OperatorAmplifiers/SentTrainingAmplifier/SentAmpTrainingBarrel", Type: "Amp"}, CatModular, "Amp", 3000},
		{"Misc", rawItem{UniqueName: "/Lotus/Weapons/Infested/Pistols/InfKitGun/Barrels/InfBarrelEgg/InfModularBarrelEggPart", Type: "Pistol"}, CatModular, "Kitgun", 3000},
		{"Misc", rawItem{UniqueName: "/Lotus/Types/Vehicles/Hoverboard/HoverboardParts/PartComponents/HoverboardCorpusA/HoverboardCorpusADeck", Type: "K-Drive Component"}, CatVehicles, "K-Drive", 6000},
		{"Warframes", rawItem{UniqueName: "/Lotus/Powersuits/EntratiMech/ThanoTech", ProductCategory: "MechSuits", Masterable: true}, CatVehicles, "Necramech", 8000},
		{"Primary", rawItem{UniqueName: "/Lotus/Weapons/Grineer/KuvaLich/LongGuns/Karak/KuvaKarak", Type: "Rifle", Masterable: true, MaxLevelCap: 40}, CatPrimary, "Rifle", 4000},
	}
	for _, c := range cases {
		it, ok := classify(c.file, c.raw)
		if !ok || it.Category != c.cat || it.Type != c.typ || it.MasteryXP() != c.xp {
			t.Errorf("%s: got ok=%v %s/%s %d xp, want %s/%s %d", c.raw.UniqueName, ok, it.Category, it.Type, it.MasteryXP(), c.cat, c.typ, c.xp)
		}
	}

	for _, u := range []string{
		"/Lotus/Weapons/Sentients/OperatorAmplifiers/Set1/Grip/SentAmpSet1GripPartA",
		"/Lotus/Types/Vehicles/Hoverboard/HoverboardParts/PartComponents/HoverboardCorpusA/HoverboardCorpusAEngine",
	} {
		if _, ok := classify("Misc", rawItem{UniqueName: u, Type: "Amp"}); ok {
			t.Errorf("%s should not count for mastery", u)
		}
	}
}

func TestPartName(t *testing.T) {
	for in, want := range map[string]string{
		"/Lotus/Types/Recipes/WarframeRecipes/GaussPrimeHelmetComponent": "Neuroptics",
		"/Lotus/Types/Recipes/WarframeRecipes/GaussPrimeBlueprint":       "Blueprint",
		"/Lotus/Types/Recipes/Weapons/WeaponParts/BratonPrimeBarrel":     "Barrel",
	} {
		if got := partName(in); got != want {
			t.Errorf("partName(%s) = %q, want %q", in, got, want)
		}
	}
}

func TestIsDojo(t *testing.T) {
	kesheg := rawItem{Name: "Kesheg", BPCost: 15000, UniqueName: "/Lotus/Weapons/Grineer/Melee/GrineerHalberd/GrnHalberd"}
	kesheg.Components = make([]struct {
		UniqueName string `json:"uniqueName"`
		ItemCount  int    `json:"itemCount"`
	}, 1)
	kesheg.Components[0].UniqueName = "/Lotus/Weapons/ClanTech/Chemical/GrnHalberdBlueprint"
	for _, c := range []struct {
		raw  rawItem
		want bool
	}{
		{kesheg, true}, // Chem Lab: found by its ClanTech blueprint
		{rawItem{Name: "Veldt", BPCost: 15000, UniqueName: "/Lotus/Weapons/Tenno/LongGuns/TnRevolverRifle/TnRevolverRifleGun"}, true}, // Tenno Lab: listed
		{rawItem{Name: "Amesha", UniqueName: "/Lotus/Powersuits/Archwing/SupportJetPack/SupportJetPack"}, true},                       // listed, no bpCost
		{rawItem{Name: "Boltor", BPCost: 15000, UniqueName: "/Lotus/Weapons/Tenno/Rifle/BoltoRifle"}, false},                          // Market
	} {
		if got := isDojo(c.raw); got != c.want {
			t.Errorf("isDojo(%s) = %v, want %v", c.raw.Name, got, c.want)
		}
	}
}

func TestExaltedFor(t *testing.T) {
	ex := []Item{
		{ID: "/Lotus/Powersuits/Excalibur/DoomSword", Name: "Exalted Blade"},
		{ID: "/Lotus/Powersuits/Excalibur/DoomSwordPrime", Name: "Exalted Prime Blade"},
		{ID: "/Lotus/Powersuits/Excalibur/DoomSwordUmbra", Name: "Exalted Umbra Blade"},
		{ID: "/Lotus/Powersuits/Pacifist/PacifistFist", Name: "Desert Wind"},
	}
	for _, c := range []struct {
		frame Item
		want  string
	}{
		{Item{ID: "/Lotus/Powersuits/Excalibur/Excalibur", Name: "Excalibur"}, "Exalted Blade"},
		{Item{ID: "/Lotus/Powersuits/Excalibur/ExcaliburPrime", Name: "Excalibur Prime"}, "Exalted Prime Blade"},
		{Item{ID: "/Lotus/Powersuits/Excalibur/ExcaliburUmbra", Name: "Excalibur Umbra"}, "Exalted Umbra Blade"},
		{Item{ID: "/Lotus/Powersuits/Pacifist/BaruukPrime", Name: "Baruuk Prime"}, "Desert Wind"}, // no Prime one here: the plain one
		{Item{ID: "/Lotus/Powersuits/Ninja/Ash", Name: "Ash"}, ""},
	} {
		frame, want := c.frame, c.want
		var got []string
		for _, e := range ExaltedFor(frame, ex) {
			got = append(got, e.Name)
		}
		if strings.Join(got, ",") != want {
			t.Errorf("%s: %q, want %q", frame.Name, got, want)
		}
	}
}

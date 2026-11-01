package catalog

import "testing"

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

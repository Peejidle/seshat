package market

import "testing"

// A few items as api.warframe.market/v2/items sends them, trimmed.
const sample = `{"apiVersion":"0.25.0","data":[
	{"slug":"arcane_energize","gameRef":"/Lotus/Upgrades/CosmeticEnhancers/Utility/EnergyOnEnergyPickup",
	 "tags":["legendary","arcane_enhancement"],"maxRank":5,"i18n":{"en":{"name":"Arcane Energize"}}},
	{"slug":"arcane_acceleration","gameRef":"/Lotus/Upgrades/CosmeticEnhancers/Offensive/FireRateOnCrit",
	 "tags":["uncommon","arcane_enhancement"],"maxRank":5,"i18n":{"en":{"name":"Arcane Acceleration"}}},
	{"slug":"nekros_prime_neuroptics_blueprint","gameRef":"/Lotus/Types/Recipes/WarframeRecipes/NekrosPrimeHelmetBlueprint",
	 "tags":["component","prime","warframe","blueprint"],"ducats":45,"vaulted":true,"i18n":{"en":{"name":"Nekros Prime Neuroptics Blueprint"}}},
	{"slug":"lex_prime_barrel","gameRef":"/Lotus/Types/Recipes/Weapons/WeaponParts/LexPrimeBarrel",
	 "tags":["component","prime","weapon"],"ducats":15,"i18n":{"en":{"name":"Lex Prime Barrel"}}}
],"error":null}`

func TestParse(t *testing.T) {
	c, err := Parse([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	ar := c.Arcanes()
	if len(ar) != 2 || ar[0].Name != "Arcane Acceleration" || ar[1].MaxRank != 5 {
		t.Errorf("arcanes = %+v, want both, by name", ar)
	}
	// WFCD names a warframe part after what it builds; the market trades
	// its blueprint.
	nk, ok := c.ForPart("/Lotus/Types/Recipes/WarframeRecipes/NekrosPrimeHelmetComponent")
	if !ok || nk.Slug != "nekros_prime_neuroptics_blueprint" || nk.Ducats != 45 || !nk.Vaulted {
		t.Errorf("Nekros neuroptics = %+v, %v", nk, ok)
	}
	if lex, ok := c.ForPart("/Lotus/Types/Recipes/Weapons/WeaponParts/LexPrimeBarrel"); !ok || lex.Ducats != 15 {
		t.Errorf("Lex barrel = %+v, %v", lex, ok)
	}
	if _, ok := c.ForPart("/not/traded"); ok {
		t.Error("an unknown part shouldn't be found")
	}
}

func TestCopiesToMax(t *testing.T) {
	if CopiesToMax(5) != 21 || CopiesToMax(3) != 10 {
		t.Errorf("CopiesToMax(5) = %d, (3) = %d; want 21 and 10", CopiesToMax(5), CopiesToMax(3))
	}
}

// Package catalog turns the community-maintained WFCD warframe-items dataset
// into a flat list of masterable items grouped into Seshat's categories.
package catalog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Category is a top-level tab in the UI.
type Category string

const (
	CatWarframes   Category = "Warframes"
	CatPrimary     Category = "Primary"
	CatSecondary   Category = "Secondary"
	CatMelee       Category = "Melee"
	CatCompanions  Category = "Companions"
	CatCompWeapons Category = "Robotic Weapons"
	CatArchwing    Category = "Archwing"
	CatVehicles    Category = "Mechs & Vehicles"
	CatModular     Category = "Amps & Modular"
)

// Categories in display order.
var Categories = []Category{
	CatWarframes, CatPrimary, CatSecondary, CatMelee, CatCompanions,
	CatCompWeapons, CatArchwing, CatVehicles, CatModular,
}

// Component is one ingredient in an item's foundry recipe.
type Component struct {
	ID    string // uniqueName
	Name  string
	Count int
	Part  bool // a blueprint, part or ingredient item, as opposed to a resource
}

// Use is another item whose recipe consumes this one.
type Use struct {
	ID    string
	Name  string
	Count int
}

// Item is a single masterable thing.
type Item struct {
	ID          string // WFCD uniqueName, stable across updates
	Name        string
	Category    Category
	Type        string // finer grouping, e.g. "Shotgun", "Kavat", "Necramech"
	MasteryReq  int
	MaxRank     int
	XPPerRank   int
	ImageName   string
	Description string
	ReleaseDate string
	Introduced  string
	Vaulted     bool
	Prime       bool
	WikiURL     string
	Components  []Component
	UsedIn      []Use // recipes that consume this item, e.g. Bronco -> Akbronco

	// How it's bought, where WFCD knows.
	CreditBP int  // blueprint price in credits, in the Market or the dojo
	Platinum int  // price of the built item in the Market
	Dojo     bool // a clan dojo research blueprint
}

// MasteryXP is the account mastery granted by ranking the item to max.
func (it Item) MasteryXP() int { return it.MaxRank * it.XPPerRank }

// EarnedXP is the mastery an item has granted so far: all of it once
// mastered, otherwise whatever its highest rank was worth.
func (it Item) EarnedXP(mastered bool, rank int) int {
	if mastered {
		return it.MasteryXP()
	}
	return min(max(rank, 0), it.MaxRank) * it.XPPerRank
}

// Parts are the components worth collecting: blueprints and crafted parts.
func (it Item) Parts() []Component {
	var ps []Component
	for _, c := range it.Components {
		if c.Part {
			ps = append(ps, c)
		}
	}
	return ps
}

// SourceFiles are the WFCD category files Seshat consumes.
var SourceFiles = []string{
	"Warframes", "Primary", "Secondary", "Melee", "Sentinels", "SentinelWeapons",
	"Pets", "Archwing", "Arch-Gun", "Arch-Melee", "Misc",
}

type rawItem struct {
	UniqueName      string `json:"uniqueName"`
	Name            string `json:"name"`
	Category        string `json:"category"`
	Type            string `json:"type"`
	ProductCategory string `json:"productCategory"`
	Masterable      bool   `json:"masterable"`
	MasteryReq      int    `json:"masteryReq"`
	MaxLevelCap     int    `json:"maxLevelCap"`
	ImageName       string `json:"imageName"`
	Description     string `json:"description"`
	ReleaseDate     string `json:"releaseDate"`
	Introduced      *struct {
		Name string `json:"name"`
	} `json:"introduced"`
	Vaulted    bool   `json:"vaulted"`
	IsPrime    bool   `json:"isPrime"`
	BPCost     int    `json:"bpCost"`
	MarketCost int    `json:"marketCost"`
	WikiaURL   string `json:"wikiaUrl"`
	Components []struct {
		UniqueName string `json:"uniqueName"`
		ItemCount  int    `json:"itemCount"`
	} `json:"components"`
}

// Supplements are masterable items WFCD does not ship at all.
var Supplements = []Item{
	{
		ID: "/Lotus/Types/Game/CrewShip/RailJack/DefaultHarness", Name: "Plexus", Category: CatVehicles, Type: "Railjack",
		MaxRank: 30, XPPerRank: 200,
		Description: "Railjack's modular loadout. Ranks like a Warframe for mastery.",
		WikiURL:     "https://wiki.warframe.com/w/Plexus",
	},
}

// Load parses the cached WFCD files in dir.
func Load(dir string) ([]Item, error) {
	names := map[string]string{} // uniqueName -> display name, for components
	type tagged struct {
		file string
		raw  rawItem
	}
	var all []tagged
	for _, f := range SourceFiles {
		b, err := os.ReadFile(filepath.Join(dir, f+".json"))
		if err != nil {
			return nil, err
		}
		var raws []rawItem
		if err := json.Unmarshal(b, &raws); err != nil {
			return nil, fmt.Errorf("%s.json: %w", f, err)
		}
		for _, r := range raws {
			names[r.UniqueName] = r.Name
			all = append(all, tagged{f, r})
		}
	}

	seen := map[string]bool{}
	var items []Item
	for _, t := range all {
		it, ok := classify(t.file, t.raw)
		if !ok || seen[it.ID] {
			continue
		}
		seen[it.ID] = true
		items = append(items, it)
	}
	// Components are resolved once every item is known, since some recipes
	// consume other masterable items.
	byID := map[string]int{}
	for i, it := range items {
		byID[it.ID] = i
	}
	for _, t := range all {
		i, ok := byID[t.raw.UniqueName]
		if !ok || items[i].Components != nil {
			continue
		}
		items[i].Components = components(t.raw, names, byID)
		for _, c := range items[i].Components {
			if j, ok := byID[c.ID]; ok {
				items[j].UsedIn = append(items[j].UsedIn, Use{ID: items[i].ID, Name: items[i].Name, Count: c.Count})
			}
		}
	}
	items = append(items, Supplements...)
	sort.SliceStable(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items, nil
}

// components lists a recipe's ingredients, merging repeats: WFCD lists the
// two Broncos an Akbronco needs as two entries.
func components(r rawItem, names map[string]string, items map[string]int) []Component {
	var cs []Component
	at := map[string]int{}
	for _, c := range r.Components {
		if i, ok := at[c.UniqueName]; ok {
			cs[i].Count += c.ItemCount
			continue
		}
		n := names[c.UniqueName]
		if n == "" {
			n = partName(c.UniqueName)
		}
		_, isItem := items[c.UniqueName]
		at[c.UniqueName] = len(cs)
		cs = append(cs, Component{ID: c.UniqueName, Name: n, Count: c.ItemCount, Part: isItem || isPart(c.UniqueName)})
	}
	return cs
}

// isPart tells collectable blueprints and parts from resources.
func isPart(u string) bool {
	return strings.HasPrefix(u, "/Lotus/Types/Recipes/") ||
		strings.HasSuffix(u, "Blueprint") ||
		strings.Contains(u, "/Resources/Mechs/") // Necramech parts
}

// listedDojo is dojo research the data gives no sign of, by name: the whole
// Tenno Lab, whose research lives under ordinary /Tenno/ paths and looks
// just like Market items, plus the odd item from the other labs with no
// credits price. Checked against the game's labs in November 2026.
var listedDojo = map[string]bool{
	// Tenno Lab warframes (Volt's main blueprint is in the Market, his parts here)
	"Banshee": true, "Nezha": true, "Volt": true, "Wukong": true, "Zephyr": true,
	// Primary
	"Veldt": true, "Tenora": true, "Baza": true, "Sybaris": true, "Daikyu": true,
	"Attica": true, "Scourge": true,
	// Secondary
	"Akstiletto": true, "Zakti": true, "Pyrana": true, "Castanas": true, "Talons": true,
	// Melee
	"Gunsen": true, "Nikana": true, "Venka": true, "Anku": true, "Cassowar": true,
	"Dual Raza": true, "Gazal Machete": true, "Nami Skyla": true, "Tonbo": true,
	"Venato": true, "Shaku": true, "Okina": true, "Masseter": true, "Lacera": true,
	"Guandao": true, "Endura": true, "Dark Split-Sword": true, "Silva & Aegis": true,
	// Archwing and its guns
	"Fluctus": true, "Larkspur": true, "Velocitus": true,
	"Amesha": true, "Elytron": true, "Itzal": true,
	// Bio Lab
	"Djinn": true,
}

// isDojo spots clan dojo research. WFCD doesn't say where a blueprint is
// sold, but dojo ones cost credits and sit under ClanTech or need research
// parts (Mutagen Mass, Fieldron and the like) to build. The Tenno Lab and
// a few others are the exception, listed above.
func isDojo(r rawItem) bool {
	if listedDojo[r.Name] {
		return true
	}
	if r.BPCost == 0 {
		return false
	}
	if strings.Contains(r.UniqueName, "/ClanTech/") {
		return true
	}
	for _, c := range r.Components {
		if strings.Contains(c.UniqueName, "/ClanTech/") || strings.Contains(c.UniqueName, "/Items/Research/") {
			return true
		}
	}
	return false
}

// classify decides whether a raw entry counts for mastery and where it goes.
// WFCD's masterable flag is right for whole items but wrong for modular
// parts, so those are matched on uniqueName instead.
func classify(file string, r rawItem) (Item, bool) {
	it := Item{
		ID: r.UniqueName, Name: r.Name, Type: r.Type,
		MasteryReq: r.MasteryReq, MaxRank: 30, XPPerRank: 100,
		ImageName: r.ImageName, Description: r.Description,
		ReleaseDate: r.ReleaseDate, Vaulted: r.Vaulted, Prime: r.IsPrime,
		WikiURL: r.WikiaURL, CreditBP: r.BPCost, Platinum: r.MarketCost,
		Dojo: isDojo(r),
	}
	if r.Introduced != nil {
		it.Introduced = r.Introduced.Name
	}
	if r.MaxLevelCap > 0 {
		it.MaxRank = r.MaxLevelCap
	}
	u := r.UniqueName

	switch file {
	case "Misc":
		switch {
		case r.Type == "Amp" && (strings.Contains(u, "/Barrel/") || strings.HasSuffix(u, "TrainingBarrel")):
			it.Category, it.Type = CatModular, "Amp"
		case strings.Contains(u, "SUModularSecondarySet1/Barrel/") || strings.Contains(u, "InfKitGun/Barrels/"):
			it.Category, it.Type = CatModular, "Kitgun"
		case r.Type == "K-Drive Component" && strings.HasSuffix(u, "Deck"):
			it.Category, it.Type, it.XPPerRank = CatVehicles, "K-Drive", 200
		default:
			return it, false
		}
		return it, true
	case "Pets":
		if !r.Masterable {
			return it, false
		}
		it.Category, it.XPPerRank, it.Type = CatCompanions, 200, petType(r.Name)
		return it, true
	}

	if !r.Masterable {
		return it, false
	}
	switch file {
	case "Warframes":
		it.XPPerRank = 200
		if r.ProductCategory == "MechSuits" {
			it.Category, it.Type, it.MaxRank = CatVehicles, "Necramech", 40
		} else {
			it.Category, it.Type = CatWarframes, "Warframe"
		}
	case "Primary":
		if r.ProductCategory == "OperatorAmps" {
			it.Category, it.Type = CatModular, "Amp"
		} else {
			it.Category = CatPrimary
		}
	case "Secondary":
		it.Category = CatSecondary
	case "Melee":
		if r.Type == "Zaw Component" {
			it.Category, it.Type = CatModular, "Zaw"
		} else {
			it.Category, it.Type = CatMelee, "Melee"
		}
	case "Sentinels":
		it.Category, it.Type, it.XPPerRank = CatCompanions, "Sentinel", 200
	case "SentinelWeapons":
		it.Category, it.Type = CatCompWeapons, "Robotic"
	case "Archwing":
		it.Category, it.Type, it.XPPerRank = CatArchwing, "Archwing", 200
	case "Arch-Gun", "Arch-Melee":
		it.Category, it.Type = CatArchwing, file
	default:
		return it, false
	}
	return it, true
}

func petType(name string) string {
	for _, k := range []string{"Kavat", "Kubrow", "Vulpaphyla", "Predasite", "Moa", "Hound", "Charger"} {
		if strings.HasSuffix(name, k) {
			return k
		}
	}
	return "Pet"
}

var (
	camel     = regexp.MustCompile(`([a-z0-9])([A-Z])`)
	partWords = []struct{ key, label string }{
		{"Helmet", "Neuroptics"}, {"Chassis", "Chassis"}, {"Systems", "Systems"},
		{"Barrel", "Barrel"}, {"Receiver", "Receiver"}, {"Stock", "Stock"},
		{"Blade", "Blade"}, {"Handle", "Handle"}, {"Hilt", "Hilt"}, {"Guard", "Guard"},
		{"Grip", "Grip"}, {"String", "String"}, {"Link", "Link"}, {"Pouch", "Pouch"},
		{"Stars", "Stars"}, {"Head", "Head"}, {"Ornament", "Ornament"}, {"Disc", "Disc"},
		{"Boot", "Boot"}, {"Gauntlet", "Gauntlet"}, {"Carapace", "Carapace"},
		{"Cerebrum", "Cerebrum"}, {"Harness", "Harness"}, {"Wings", "Wings"},
		{"Blueprint", "Blueprint"}, {"Engine", "Engine"}, {"WeaponPod", "Weapon Pod"},
		{"LowerLimb", "Lower Limb"}, {"UpperLimb", "Upper Limb"},
	}
)

// partName guesses a readable name for a recipe part that WFCD doesn't list,
// e.g. ".../GaussPrimeHelmetComponent" -> "Neuroptics".
func partName(uniqueName string) string {
	last := uniqueName[strings.LastIndex(uniqueName, "/")+1:]
	base := strings.TrimSuffix(strings.TrimSuffix(last, "Component"), "Item")
	for _, p := range partWords {
		if strings.HasSuffix(base, p.key) {
			return p.label
		}
	}
	return camel.ReplaceAllString(base, "$1 $2")
}

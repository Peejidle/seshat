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
	Part  bool // a blueprint or crafted part, as opposed to a resource
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
		for _, c := range t.raw.Components {
			n := names[c.UniqueName]
			if n == "" {
				n = partName(c.UniqueName)
			}
			it.Components = append(it.Components, Component{
				ID: c.UniqueName, Name: n, Count: c.ItemCount,
				Part: strings.HasPrefix(c.UniqueName, "/Lotus/Types/Recipes/"),
			})
		}
		items = append(items, it)
	}
	items = append(items, Supplements...)
	sort.SliceStable(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items, nil
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
		WikiURL: r.WikiaURL,
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
		{"Blueprint", "Blueprint"},
	}
)

// partName guesses a readable name for a recipe part that WFCD doesn't list,
// e.g. ".../GaussPrimeHelmetComponent" -> "Neuroptics".
func partName(uniqueName string) string {
	last := uniqueName[strings.LastIndex(uniqueName, "/")+1:]
	base := strings.TrimSuffix(last, "Component")
	for _, p := range partWords {
		if strings.HasSuffix(base, p.key) {
			return p.label
		}
	}
	return camel.ReplaceAllString(base, "$1 $2")
}

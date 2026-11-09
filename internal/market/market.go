// Package market reads warframe.market's item list: the arcanes Seshat can
// count, the ducats each prime part is worth, and the slugs price lookups
// need. warframe.market is a community site, not DE's.
package market

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
)

// ItemsFile is the cached item list, kept fresh by catalog.Refresh.
const ItemsFile = "market.json"

type Item struct {
	Slug string // its name in market URLs, e.g. "arcane_energize"
	// GameRef is the game's own ID for it, the same as WFCD's uniqueName,
	// which is what joins the market's items to Seshat's.
	GameRef string
	Name    string
	Tags    []string
	MaxRank int // 0 for anything that doesn't rank up
	Ducats  int // what Baro's kiosks give for it; 0 if they take none
	Vaulted bool
}

func (it Item) IsArcane() bool { return it.has("arcane_enhancement") }

// IsPrimePart is a prime part or blueprint, not a whole set.
func (it Item) IsPrimePart() bool {
	return it.has("prime") && (it.has("component") || it.has("blueprint")) && !it.has("set")
}

func (it Item) has(tag string) bool {
	for _, t := range it.Tags {
		if t == tag {
			return true
		}
	}
	return false
}

// CopiesToMax is how many copies of an arcane fuse into one at maxRank:
// each rank takes one more copy than the last, so 1+2+…+(maxRank+1).
// That's 21 for most arcanes (rank 5) and 10 for the rank 3 ones.
func CopiesToMax(maxRank int) int { return (maxRank + 1) * (maxRank + 2) / 2 }

type Catalog struct {
	items []Item
	byRef map[string]int // GameRef -> index into items
}

// rawItem is one item as the API sends it.
type rawItem struct {
	Slug    string   `json:"slug"`
	GameRef string   `json:"gameRef"`
	Tags    []string `json:"tags"`
	MaxRank int      `json:"maxRank"`
	Ducats  int      `json:"ducats"`
	Vaulted bool     `json:"vaulted"`
	I18n    map[string]struct {
		Name string `json:"name"`
	} `json:"i18n"`
}

func Load(path string) (*Catalog, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(b)
}

// Parse reads the reply of api.warframe.market/v2/items.
func Parse(b []byte) (*Catalog, error) {
	var raw struct {
		Data []rawItem `json:"data"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	c := &Catalog{byRef: map[string]int{}}
	for _, r := range raw.Data {
		c.byRef[r.GameRef] = len(c.items)
		c.items = append(c.items, Item{
			Slug: r.Slug, GameRef: r.GameRef, Name: r.I18n["en"].Name, Tags: r.Tags,
			MaxRank: r.MaxRank, Ducats: r.Ducats, Vaulted: r.Vaulted,
		})
	}
	return c, nil
}

// Arcanes lists every tradeable arcane, by name.
func (c *Catalog) Arcanes() []Item {
	var out []Item
	for _, it := range c.items {
		if it.IsArcane() {
			out = append(out, it)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Find looks items up by name, ignoring case: an exact match if there is
// one, otherwise every item whose name has each word of the query in it,
// so "nekros neuro" finds Nekros Prime Neuroptics Blueprint. keep, if not
// nil, narrows the search, e.g. to arcanes.
func (c *Catalog) Find(query string, keep func(Item) bool) []Item {
	q := strings.ToLower(strings.Join(strings.Fields(query), " "))
	words := strings.Fields(q)
	var out []Item
	for _, it := range c.items {
		if keep != nil && !keep(it) {
			continue
		}
		name := strings.ToLower(it.Name)
		if name == q {
			return []Item{it}
		}
		all := len(words) > 0
		for _, w := range words {
			all = all && strings.Contains(name, w)
		}
		if all {
			out = append(out, it)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Get finds an item by its game ID.
func (c *Catalog) Get(gameRef string) (Item, bool) {
	i, ok := c.byRef[gameRef]
	if !ok {
		return Item{}, false
	}
	return c.items[i], true
}

// ForPart finds the market's listing for a part of a Seshat item. A
// warframe's parts are traded as blueprints, so where WFCD names the built
// part (…NekrosPrimeHelmetComponent) the market has the blueprint
// (…NekrosPrimeHelmetBlueprint). Weapon parts and main blueprints share
// one ID.
func (c *Catalog) ForPart(id string) (Item, bool) {
	if it, ok := c.Get(id); ok {
		return it, true
	}
	if base, ok := strings.CutSuffix(id, "Component"); ok {
		return c.Get(base + "Blueprint")
	}
	return Item{}, false
}

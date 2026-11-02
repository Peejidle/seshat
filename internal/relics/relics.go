// Package relics reads WFCD's mirror of DE's published drop tables to tell
// which void relics hold which prime parts, and which relics can be farmed
// right now (the rest are vaulted).
package relics

import (
	"encoding/json"
	"os"
	"regexp"
	"sort"
	"strings"
)

// Tiers in the order they unlock.
var Tiers = []string{"Lith", "Meso", "Neo", "Axi"}

// Reward is one possible drop from a relic.
type Reward struct {
	Item            string // as the drop tables name it, e.g. "Rhino Prime Chassis Blueprint"
	Rarity          string // Common, Uncommon or Rare
	Intact, Radiant float64
}

type Relic struct {
	Name     string // e.g. "Lith E1"
	Tier     int    // index into Tiers
	Farmable bool   // drops from some mission or bounty right now
	Rewards  []Reward
}

// Drop is a relic that can give a particular item.
type Drop struct {
	Relic  *Relic
	Reward Reward
}

type Table struct {
	Relics []*Relic
	byItem map[string][]Drop // lowercased reward name -> where it drops
}

type rawFile struct {
	Relics []struct {
		Tier      string `json:"tier"`
		RelicName string `json:"relicName"`
		State     string `json:"state"` // Intact, Exceptional, Flawless or Radiant
		Rewards   []struct {
			ItemName string  `json:"itemName"`
			Chance   float64 `json:"chance"`
		} `json:"rewards"`
	} `json:"relics"`
}

// relicDrop matches a relic given as a reward anywhere else in the file:
// missions, bounties and so on. A relic that appears nowhere is vaulted.
var relicDrop = regexp.MustCompile(`"itemName":\s*"(Lith|Meso|Neo|Axi) (\w+) Relic`)

// Load parses the drop tables file (all.json from drops.warframestat.us).
func Load(path string) (*Table, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw rawFile
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}

	farmable := map[string]bool{}
	for _, m := range relicDrop.FindAllSubmatch(b, -1) {
		farmable[string(m[1])+" "+string(m[2])] = true
	}

	// Each relic is listed once per refinement; merge them, keeping the
	// intact and radiant chances.
	t := &Table{byItem: map[string][]Drop{}}
	byName := map[string]*Relic{}
	for _, r := range raw.Relics {
		tier := tierIndex(r.Tier)
		if tier < 0 || r.RelicName == "" {
			continue // Requiem relics hold no prime parts
		}
		name := r.Tier + " " + r.RelicName
		rel := byName[name]
		if rel == nil {
			rel = &Relic{Name: name, Tier: tier, Farmable: farmable[name]}
			for _, rw := range r.Rewards {
				rel.Rewards = append(rel.Rewards, Reward{Item: rw.ItemName})
			}
			byName[name] = rel
			t.Relics = append(t.Relics, rel)
		}
		for _, rw := range r.Rewards {
			for i := range rel.Rewards {
				if rel.Rewards[i].Item != rw.ItemName {
					continue
				}
				switch r.State {
				case "Intact":
					rel.Rewards[i].Intact = rw.Chance
				case "Radiant":
					rel.Rewards[i].Radiant = rw.Chance
				}
			}
		}
	}

	for _, rel := range t.Relics {
		for i := range rel.Rewards {
			rel.Rewards[i].Rarity = rarity(rel.Rewards[i].Intact)
		}
		for _, rw := range rel.Rewards {
			k := strings.ToLower(rw.Item)
			t.byItem[k] = append(t.byItem[k], Drop{rel, rw})
		}
	}
	for _, ds := range t.byItem {
		sort.Slice(ds, func(i, j int) bool { return better(ds[i], ds[j]) })
	}
	sort.Slice(t.Relics, func(i, j int) bool {
		a, b := t.Relics[i], t.Relics[j]
		if a.Tier != b.Tier {
			return a.Tier < b.Tier
		}
		return a.Name < b.Name
	})
	return t, nil
}

// better orders drops: farmable relics first, then the likelier drop, then
// the lower tier.
func better(a, b Drop) bool {
	if a.Relic.Farmable != b.Relic.Farmable {
		return a.Relic.Farmable
	}
	if a.Reward.Intact != b.Reward.Intact {
		return a.Reward.Intact > b.Reward.Intact
	}
	if a.Relic.Tier != b.Relic.Tier {
		return a.Relic.Tier < b.Relic.Tier
	}
	return a.Relic.Name < b.Relic.Name
}

// Match finds where each of an item's parts drops, best relic first; parts
// that no relic holds get nil. parts are Seshat's names ("Chassis",
// "Barrel", "Blueprint").
//
// Seshat's names are guessed from DE's internal ones, which sometimes differ
// from the game's ("Handle" is shown in game as "Hilt"). So exact names are
// matched first, and if exactly one part and one of the item's rewards are
// left over, they're paired.
func (t *Table) Match(item string, parts []string) [][]Drop {
	out := make([][]Drop, len(parts))
	if t == nil {
		return out
	}
	taken := map[string]bool{}
	var unmatched []int
	for i, p := range parts {
		for _, n := range rewardNames(item, p) {
			if ds := t.byItem[strings.ToLower(n)]; len(ds) > 0 {
				out[i] = ds
				taken[strings.ToLower(n)] = true
				break
			}
		}
		if out[i] == nil {
			unmatched = append(unmatched, i)
		}
	}
	if len(unmatched) != 1 {
		return out
	}
	var left []string
	prefix := strings.ToLower(item) + " "
	for k := range t.byItem {
		if strings.HasPrefix(k, prefix) && !taken[k] {
			left = append(left, k)
		}
	}
	if len(left) == 1 {
		out[unmatched[0]] = t.byItem[left[0]]
	}
	return out
}

// PartName is the game's name for the part a drop gives, e.g. "Hilt" for
// "Nikana Prime Hilt".
func (d Drop) PartName(item string) string {
	n := strings.TrimPrefix(d.Reward.Item, item+" ")
	if n == "Blueprint" {
		return n
	}
	return strings.TrimSuffix(n, " Blueprint")
}

// rewardNames lists how the drop tables may name a part: Warframe parts end
// in "Blueprint" ("Rhino Prime Chassis Blueprint"), weapon parts don't
// ("Braton Prime Barrel"), and the main blueprint is "<item> Blueprint".
func rewardNames(item, part string) []string {
	if part == "Blueprint" {
		return []string{item + " Blueprint"}
	}
	return []string{item + " " + part, item + " " + part + " Blueprint"}
}

// rarity names a reward's slot from its intact chance. The file's own
// rarity labels are unreliable (commons are marked "Uncommon"), but every
// relic has three 25.33% commons, two 11% uncommons and one 2% rare.
func rarity(intact float64) string {
	switch {
	case intact > 20:
		return "Common"
	case intact > 5:
		return "Uncommon"
	default:
		return "Rare"
	}
}

// Find looks a relic up by tier and name, ignoring case. The tier can be
// written out or as its first letter: ("axi", "s8") or ("a", "s8").
func (t *Table) Find(tier, name string) *Relic {
	tier, name = strings.ToLower(tier), strings.ToLower(name)
	for _, r := range t.Relics {
		full := strings.ToLower(Tiers[r.Tier])
		if (tier == full || tier == full[:1]) && strings.ToLower(r.Name) == full+" "+name {
			return r
		}
	}
	return nil
}

func tierIndex(s string) int {
	for i, t := range Tiers {
		if t == s {
			return i
		}
	}
	return -1
}

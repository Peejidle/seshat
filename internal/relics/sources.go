package relics

import (
	"encoding/json"
	"sort"
	"strings"
)

// Source is a mission that drops a relic.
type Source struct {
	Planet, Node, Mode string
	Rotation           string // A, B or C; "" for missions without rotations
	Chance             float64
}

// otherSections names the parts of the drops file besides missions that
// can give relics, as players know them.
var otherSections = []struct{ key, name string }{
	{"cetusBountyRewards", "Cetus bounties"},
	{"solarisBountyRewards", "Orb Vallis bounties"},
	{"deimosRewards", "Cambion Drift bounties"},
	{"zarimanRewards", "Zariman bounties"},
	{"entratiLabRewards", "Albrecht's Laboratories bounties"},
	{"hexRewards", "Höllvania bounties"},
	{"keyRewards", "key missions"},
	{"transientRewards", "other rewards (caches, events)"},
	{"sortieRewards", "sorties"},
}

type rawMission struct {
	GameMode string `json:"gameMode"`
	IsEvent  bool   `json:"isEvent"`
	// Either a list of rewards, or rotation ("A", "B", "C") -> list.
	Rewards json.RawMessage `json:"rewards"`
}

type rawReward struct {
	ItemName string  `json:"itemName"`
	Chance   float64 `json:"chance"`
}

// addSources fills in each relic's Missions and Elsewhere from the drops
// file. byName maps "Lith N1" to its relic.
func addSources(b []byte, byName map[string]*Relic) error {
	var file map[string]json.RawMessage
	if err := json.Unmarshal(b, &file); err != nil {
		return err
	}

	var missions map[string]map[string]rawMission // planet -> node -> mission
	if raw, ok := file["missionRewards"]; ok {
		if err := json.Unmarshal(raw, &missions); err != nil {
			return err
		}
	}
	for planet, nodes := range missions {
		for node, ms := range nodes {
			if ms.IsEvent {
				continue // gone once the event ends
			}
			for rot, rws := range rotations(ms.Rewards) {
				for _, rw := range rws {
					if r := byName[relicName(rw.ItemName)]; r != nil {
						r.Missions = append(r.Missions, Source{planet, node, ms.GameMode, rot, rw.Chance})
					}
				}
			}
		}
	}

	// The other sections are shaped differently from each other, so they're
	// only searched for relic names, not unpicked.
	for _, s := range otherSections {
		seen := map[string]bool{}
		for _, m := range relicDrop.FindAllSubmatch(file[s.key], -1) {
			name := string(m[1]) + " " + string(m[2])
			if r := byName[name]; r != nil && !seen[name] {
				seen[name] = true
				r.Elsewhere = append(r.Elsewhere, s.name)
			}
		}
	}

	for _, r := range byName {
		sort.Slice(r.Missions, func(i, j int) bool {
			a, b := r.Missions[i], r.Missions[j]
			if a.Chance != b.Chance {
				return a.Chance > b.Chance
			}
			return a.Planet+a.Node < b.Planet+b.Node
		})
	}
	return nil
}

// rotations reads a mission's rewards, which are either one list or a list
// per rotation.
func rotations(raw json.RawMessage) map[string][]rawReward {
	var list []rawReward
	if json.Unmarshal(raw, &list) == nil {
		return map[string][]rawReward{"": list}
	}
	var byRot map[string][]rawReward
	json.Unmarshal(raw, &byRot) // anything else holds no relics
	return byRot
}

// relicName turns a reward like "Lith N1 Relic" into "Lith N1". Rewards
// that aren't relics come out unchanged and match no relic.
func relicName(item string) string { return strings.TrimSuffix(item, " Relic") }

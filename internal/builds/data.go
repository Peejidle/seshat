// Package builds plans mod builds: which mods and arcanes go on a warframe
// or weapon, what they cost in capacity, and what the item still needs in
// the game (an Orokin Reactor or Catalyst, Forma, an Exilus Adapter) before
// the build can go on. Mod data is WFCD's, like the rest of Seshat's.
package builds

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/Peejidle/seshat/internal/catalog"
)

// Mod is one mod as a build uses it.
type Mod struct {
	ID       string // WFCD uniqueName
	Name     string
	Polarity Polarity
	// BaseDrain is the capacity it costs at rank 0; each rank adds one.
	// Auras and stances have a negative one: they give capacity instead.
	BaseDrain int
	MaxRank   int
	Rarity    string
	Type      string // e.g. "Warframe Mod", "Primary Mod", "Stance Mod"
	// Compat is what it fits, as WFCD puts it: "WARFRAME", "AURA",
	// "Rifle", "Sniper", or one frame or weapon ("Limbo", "Soma Prime").
	Compat string
	Exilus bool       // fits the exilus slot
	Stats  [][]string // what it does, rank by rank
}

// Aura is whether it goes in a warframe's aura slot.
func (m Mod) Aura() bool { return m.Compat == "AURA" }

// Stance is whether it goes in a melee weapon's stance slot.
func (m Mod) Stance() bool { return m.Type == "Stance Mod" }

// Arcane is one arcane as a build uses it.
type Arcane struct {
	ID      string
	Name    string
	Type    string // e.g. "Warframe Arcane", "Primary Arcane"
	MaxRank int
	Stats   [][]string
}

// Data is every mod and arcane a build can use.
type Data struct {
	Mods    []Mod
	Arcanes []Arcane
	mods    map[string]int // ID -> index into Mods
	arcanes map[string]int
}

// rawMod is one mod as WFCD sends it.
type rawMod struct {
	UniqueName  string `json:"uniqueName"`
	Name        string `json:"name"`
	Polarity    string `json:"polarity"`
	Rarity      string `json:"rarity"`
	BaseDrain   *int   `json:"baseDrain"`
	FusionLimit int    `json:"fusionLimit"`
	Type        string `json:"type"`
	CompatName  string `json:"compatName"`
	IsUtility   bool   `json:"isUtility"`
	LevelStats  []struct {
		Stats []string `json:"stats"`
	} `json:"levelStats"`
}

// buildTypes are the kinds of mod Seshat plans builds with.
var buildTypes = map[string]bool{
	"Warframe Mod": true, "Primary Mod": true, "Shotgun Mod": true,
	"Secondary Mod": true, "Melee Mod": true, "Stance Mod": true,
}

// Load reads the cached WFCD mod and arcane files in dir.
func Load(dir string) (*Data, error) {
	var mods []rawMod
	if err := readJSON(filepath.Join(dir, catalog.ModsFile), &mods); err != nil {
		return nil, err
	}
	var arcs []rawMod // arcanes have the same fields, less the mod ones
	if err := readJSON(filepath.Join(dir, catalog.ArcanesFile), &arcs); err != nil {
		return nil, err
	}
	d := &Data{mods: map[string]int{}, arcanes: map[string]int{}}
	for _, r := range pickVersions(mods) {
		if !buildTypes[r.Type] || r.BaseDrain == nil || strings.Contains(r.Name, "Riven") {
			continue
		}
		d.Mods = append(d.Mods, Mod{
			ID: r.UniqueName, Name: r.Name, Polarity: Polarity(r.Polarity),
			BaseDrain: *r.BaseDrain, MaxRank: r.FusionLimit, Rarity: r.Rarity,
			Type: r.Type, Compat: r.CompatName, Exilus: r.IsUtility, Stats: stats(r),
		})
	}
	for _, r := range arcs {
		if !strings.HasSuffix(r.Type, "Arcane") || len(r.LevelStats) == 0 {
			continue
		}
		d.Arcanes = append(d.Arcanes, Arcane{
			ID: r.UniqueName, Name: r.Name, Type: r.Type,
			MaxRank: len(r.LevelStats) - 1, Stats: stats(r),
		})
	}
	sort.Slice(d.Mods, func(i, j int) bool { return d.Mods[i].Name < d.Mods[j].Name })
	sort.Slice(d.Arcanes, func(i, j int) bool { return d.Arcanes[i].Name < d.Arcanes[j].Name })
	for i, m := range d.Mods {
		d.mods[m.ID] = i
	}
	for i, a := range d.Arcanes {
		d.arcanes[a.ID] = i
	}
	return d, nil
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func stats(r rawMod) [][]string {
	var out [][]string
	for _, l := range r.LevelStats {
		var rank []string
		seen := map[string]bool{}
		for _, s := range l.Stats {
			for _, line := range cleanStat(s) {
				if !seen[line] { // WFCD repeats some, e.g. Arcane Revive
					seen[line] = true
					rank = append(rank, line)
				}
			}
		}
		out = append(out, rank)
	}
	return out
}

// tagWords are the words the game's text tags stand for. Most tags come
// right before their word, "<DT_POISON_COLOR>Toxin", to colour it, but some
// stand in for it: "apply <DT_SLASH_COLOR> on Critical".
var tagWords = map[string]string{
	"DT_ELECTRICITY_COLOR": "Electricity", "DT_FIRE_COLOR": "Heat", "DT_FREEZE_COLOR": "Cold",
	"DT_POISON_COLOR": "Toxin", "DT_SLASH_COLOR": "Slash", "DT_PUNCTURE_COLOR": "Puncture",
	"DT_IMPACT_COLOR": "Impact", "DT_RADIATION_COLOR": "Radiation", "DT_MAGNETIC_COLOR": "Magnetic",
	"DT_EXPLOSION_COLOR": "Blast", "DT_VIRAL_COLOR": "Viral", "DT_CORROSIVE_COLOR": "Corrosive",
	"DT_SENTIENT": "Tau", "DT_RADIANT_COLOR": "Void", "DT_GAS_COLOR": "Gas",
	"ACTIVATE_ABILITY_1": "the ability", "PRE_ATTACK": "aim", "SECONDARY_FIRE": "alternate fire",
	"USE": "use",
}

var tagRE = regexp.MustCompile(`<([A-Z0-9_]+)>\s*`)

// cleanStat turns the game's text tags into plain words: a tag before its
// own word goes, one standing in for a word becomes it, and the rest
// (colours, "lower is better" marks, icons) go. It's split into lines where
// WFCD breaks it (a real newline, a "\n" or a tag), but a line that only
// says when, "On Energy Pickup:", stays with the one after.
func cleanStat(s string) []string {
	s = strings.ReplaceAll(s, `\n`, "\n")
	s = strings.ReplaceAll(s, "<LINE_SEPARATOR>", "\n")
	var b strings.Builder
	for {
		loc := tagRE.FindStringSubmatchIndex(s)
		if loc == nil {
			b.WriteString(s)
			break
		}
		b.WriteString(s[:loc[0]])
		tag, rest := s[loc[2]:loc[3]], s[loc[1]:]
		if w, ok := tagWords[tag]; ok && !strings.HasPrefix(rest, w) {
			b.WriteString(w + " ")
		}
		s = rest
	}
	var lines []string
	for _, l := range strings.Split(b.String(), "\n") {
		l = strings.Join(strings.Fields(l), " ")
		switch {
		case l == "":
		case len(lines) > 0 && strings.HasSuffix(lines[len(lines)-1], ":"):
			lines[len(lines)-1] += " " + l
		default:
			lines = append(lines, l)
		}
	}
	return lines
}

// pickVersions keeps one mod of each name. Several share a name: the
// Flawed ones and the starter versions the game hands out early, which
// live under Beginner, Intermediate and Expert paths and rank lower (or
// oddly). Where there's a plain one, it's the one you'd build with.
func pickVersions(mods []rawMod) []rawMod {
	byName := map[string][]rawMod{}
	var names []string
	for _, m := range mods {
		if byName[m.Name] == nil {
			names = append(names, m.Name)
		}
		byName[m.Name] = append(byName[m.Name], m)
	}
	var out []rawMod
	for _, n := range names {
		versions := byName[n]
		best := versions[0]
		for _, v := range versions {
			if variant(best.UniqueName) && !variant(v.UniqueName) {
				best = v
			}
		}
		out = append(out, best)
	}
	return out
}

func variant(id string) bool {
	for _, dir := range []string{"/Beginner/", "/Intermediate/", "/Expert/"} {
		if strings.Contains(id, dir) {
			return true
		}
	}
	return false
}

// Mod finds a mod by ID.
func (d *Data) Mod(id string) (Mod, bool) {
	i, ok := d.mods[id]
	if !ok {
		return Mod{}, false
	}
	return d.Mods[i], true
}

// Arcane finds an arcane by ID.
func (d *Data) Arcane(id string) (Arcane, bool) {
	i, ok := d.arcanes[id]
	if !ok {
		return Arcane{}, false
	}
	return d.Arcanes[i], true
}

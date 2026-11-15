// Package store persists the user's checklist. It is keyed by WFCD
// uniqueName so marks survive renames and data refreshes.
package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

type Mark struct {
	Owned    bool `json:"owned,omitempty"`
	Mastered bool `json:"mastered,omitempty"`
	// Rank is the highest rank reached on an item that isn't mastered, e.g. a
	// Coda weapon at 30 of 40. Partial ranks still earn mastery XP.
	Rank int `json:"rank,omitempty"`
}

// ExtraXP is mastery from sources Seshat can't enumerate, entered by hand.
type ExtraXP struct {
	StarChart int `json:"starChart"`
	SteelPath int `json:"steelPath"`
	Railjack  int `json:"railjackIntrinsics"`
	Drifter   int `json:"drifterIntrinsics"`
	Other     int `json:"other"`
}

func (e ExtraXP) Total() int {
	return e.StarChart + e.SteelPath + e.Railjack + e.Drifter + e.Other
}

// Arcane counts the copies of one arcane held, for working out what to sell.
type Arcane struct {
	Maxed int `json:"maxed,omitempty"`
	// Copies are the loose ones: rank 0, or a partly ranked arcane counted
	// as the copies fused into it.
	Copies int `json:"copies,omitempty"`
}

// Build is what's planned for one item, and how far it's got in the game.
// As in the game, the item has up to three configs, each with its own mods
// and arcanes, while the slots' polarities, the Reactor or Catalyst and
// the unlocked slots belong to the item and so are shared. IDs are WFCD
// uniqueNames; polarities are as WFCD names them ("madurai"), "" for none.
type Build struct {
	// Slots holds the ordinary mod slots, then the aura or stance slot,
	// then the exilus slot (package builds numbers them).
	Slots []BuildSlot `json:"slots"`
	// ArcanesUnlocked says, for each arcane slot, whether it's been
	// unlocked in the game, which takes an adapter.
	ArcanesUnlocked []bool `json:"arcanesUnlocked"`
	// Configs are A, B and C; Active is the one last looked at.
	Configs []BuildConfig `json:"configs"`
	Active  int           `json:"active,omitempty"`
	// Reactor is an Orokin Reactor (a warframe) or Catalyst (a weapon)
	// installed in the game, and Exilus the exilus slot unlocked with an
	// adapter.
	Reactor bool `json:"reactor,omitempty"`
	Exilus  bool `json:"exilus,omitempty"`
}

// BuildSlot is one mod slot: the polarity it's planned to have and the one
// it has in the game now. The two differ until a Forma is spent on it.
type BuildSlot struct {
	Polarity string `json:"polarity,omitempty"`
	Has      string `json:"has,omitempty"`
}

// BuildConfig is one config: a mod for each slot, an arcane for each
// arcane slot, and notes on what it's for.
type BuildConfig struct {
	Mods    []BuildMod `json:"mods"`
	Arcanes []BuildMod `json:"arcanes"`
	Notes   string     `json:"notes,omitempty"`
}

// BuildMod is a mod or arcane, and the rank it's planned at. An empty ID
// is an empty slot.
type BuildMod struct {
	ID   string `json:"id,omitempty"`
	Rank int    `json:"rank,omitempty"`
}

// Config is the config being looked at.
func (b Build) Config() BuildConfig { return b.Configs[b.Active] }

// legacyBuild is a build as saved before configs: the one set of mods
// and arcanes in the slots themselves.
type legacyBuild struct {
	Slots []struct {
		Mod  string `json:"mod"`
		Rank int    `json:"rank"`
	} `json:"slots"`
	Arcanes []struct {
		ID       string `json:"id"`
		Rank     int    `json:"rank"`
		Unlocked bool   `json:"unlocked"`
	} `json:"arcanes"`
	Notes string `json:"notes"`
}

// upgradeBuilds moves builds saved before configs into config A, and
// gives every build its three configs.
func upgradeBuilds(p *Progress, raw []byte) {
	var old struct {
		Builds map[string]legacyBuild `json:"builds"`
	}
	_ = json.Unmarshal(raw, &old)
	for id, b := range p.Builds {
		if len(b.Configs) == 0 {
			lb := old.Builds[id]
			a := BuildConfig{Notes: lb.Notes}
			for _, s := range lb.Slots {
				a.Mods = append(a.Mods, BuildMod{s.Mod, s.Rank})
			}
			for _, ar := range lb.Arcanes {
				a.Arcanes = append(a.Arcanes, BuildMod{ar.ID, ar.Rank})
				b.ArcanesUnlocked = append(b.ArcanesUnlocked, ar.Unlocked)
			}
			b.Configs = []BuildConfig{a}
		}
		for len(b.Configs) < NumConfigs {
			b.Configs = append(b.Configs, BuildConfig{
				Mods:    make([]BuildMod, len(b.Slots)),
				Arcanes: make([]BuildMod, len(b.ArcanesUnlocked)),
			})
		}
		b.Active = min(max(b.Active, 0), NumConfigs-1)
		p.Builds[id] = b
	}
}

// NumConfigs is how many configs an item has: A, B and C.
const NumConfigs = 3

// Settings are UI preferences that persist between sessions.
type Settings struct {
	HideMastered bool   `json:"hideMastered"`
	Theme        string `json:"theme,omitempty"` // "" follows the terminal
	// ProfileID is the player's Warframe account ID, remembered so a
	// profile import only asks for it once.
	ProfileID string `json:"profileId,omitempty"`
}

type Progress struct {
	Version int             `json:"version"`
	Marks   map[string]Mark `json:"marks"`
	Extra   ExtraXP         `json:"extra"`
	// Seen holds every item ID the archive has shown before, so items added
	// upstream can be flagged as new.
	Seen map[string]bool `json:"seen"`
	// Parts counts the blueprints and parts held toward an item:
	// item ID -> component ID -> how many.
	Parts map[string]map[string]int `json:"parts,omitempty"`
	// Relics counts the void relics held, by name ("Axi S8"), refinements
	// together.
	Relics map[string]int `json:"relics,omitempty"`
	// RelicsCounted is when the relic counts were last checked against the
	// game, by finishing a recount.
	RelicsCounted time.Time `json:"relicsCounted,omitzero"`
	// Arcanes counts the arcanes held, by game ID.
	Arcanes map[string]Arcane `json:"arcanes,omitempty"`
	// Spares counts the prime parts held to sell, by game ID: the extras,
	// unlike Parts, which counts toward building an item.
	Spares map[string]int `json:"spares,omitempty"`
	// Builds are the builds planned, by item ID.
	Builds   map[string]Build `json:"builds,omitempty"`
	Settings Settings         `json:"settings"`
	Updated  time.Time        `json:"updated"`
}

func New() *Progress {
	return &Progress{Version: 1, Marks: map[string]Mark{}, Seen: map[string]bool{}}
}

func Load(path string) (*Progress, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return New(), nil
	}
	if err != nil {
		return nil, err
	}
	return parse(b)
}

func parse(b []byte) (*Progress, error) {
	p := New()
	if err := json.Unmarshal(b, p); err != nil {
		return nil, err
	}
	if p.Marks == nil {
		p.Marks = map[string]Mark{}
	}
	if p.Seen == nil {
		p.Seen = map[string]bool{}
	}
	upgradeBuilds(p, b)
	// The Plexus briefly used a made-up ID before its real one was known.
	if mk, ok := p.Marks["seshat:plexus"]; ok {
		p.Marks["/Lotus/Types/Game/CrewShip/RailJack/DefaultHarness"] = mk
		delete(p.Marks, "seshat:plexus")
	}
	return p, nil
}

// Save writes atomically so a crash never truncates the checklist.
func (p *Progress) Save(path string) error {
	p.Updated = time.Now()
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// HeldParts is how many of a component the user holds toward an item.
func (p *Progress) HeldParts(item, part string) int { return p.Parts[item][part] }

// SetPart records n of a component held toward an item.
func (p *Progress) SetPart(item, part string, n int) {
	if n <= 0 {
		delete(p.Parts[item], part)
		if len(p.Parts[item]) == 0 {
			delete(p.Parts, item)
		}
		return
	}
	if p.Parts == nil {
		p.Parts = map[string]map[string]int{}
	}
	if p.Parts[item] == nil {
		p.Parts[item] = map[string]int{}
	}
	p.Parts[item][part] = n
}

// HeldRelics is how many of a relic the user holds.
func (p *Progress) HeldRelics(name string) int { return p.Relics[name] }

// SetRelics records n of a relic held.
func (p *Progress) SetRelics(name string, n int) {
	if n <= 0 {
		delete(p.Relics, name)
		return
	}
	if p.Relics == nil {
		p.Relics = map[string]int{}
	}
	p.Relics[name] = n
}

// HeldArcane is how many of an arcane the user holds.
func (p *Progress) HeldArcane(id string) Arcane { return p.Arcanes[id] }

// SetArcane records the copies of an arcane held.
func (p *Progress) SetArcane(id string, a Arcane) {
	if a.Maxed <= 0 && a.Copies <= 0 {
		delete(p.Arcanes, id)
		return
	}
	if p.Arcanes == nil {
		p.Arcanes = map[string]Arcane{}
	}
	p.Arcanes[id] = Arcane{Maxed: max(0, a.Maxed), Copies: max(0, a.Copies)}
}

// HeldSpares is how many of a part the user holds to sell.
func (p *Progress) HeldSpares(id string) int { return p.Spares[id] }

// SetSpares records n of a part held to sell.
func (p *Progress) SetSpares(id string, n int) {
	if n <= 0 {
		delete(p.Spares, id)
		return
	}
	if p.Spares == nil {
		p.Spares = map[string]int{}
	}
	p.Spares[id] = n
}

// Build is the build planned for an item, if there is one.
func (p *Progress) Build(id string) (Build, bool) {
	b, ok := p.Builds[id]
	return b, ok
}

// SetBuild records the build planned for an item.
func (p *Progress) SetBuild(id string, b Build) {
	if p.Builds == nil {
		p.Builds = map[string]Build{}
	}
	p.Builds[id] = b
}

// DeleteBuild forgets the build planned for an item.
func (p *Progress) DeleteBuild(id string) { delete(p.Builds, id) }

func (p *Progress) Get(id string) Mark { return p.Marks[id] }

func (p *Progress) Set(id string, m Mark) {
	if m == (Mark{}) {
		delete(p.Marks, id)
		return
	}
	p.Marks[id] = m
}

// Import replaces marks, parts, relics, arcanes, spares, builds and extra XP from a
// backup file, keeping Seen.
func (p *Progress) Import(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	in, err := parse(b)
	if err != nil {
		return err
	}
	p.Marks, p.Parts, p.Extra = in.Marks, in.Parts, in.Extra
	p.Relics, p.RelicsCounted = in.Relics, in.RelicsCounted
	p.Arcanes, p.Spares, p.Builds = in.Arcanes, in.Spares, in.Builds
	return nil
}

// Export writes a backup copy to path.
func (p *Progress) Export(path string) error {
	cp := *p
	cp.Seen = nil
	b, err := json.MarshalIndent(&cp, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

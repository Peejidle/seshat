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
	Settings      Settings  `json:"settings"`
	Updated       time.Time `json:"updated"`
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

func (p *Progress) Get(id string) Mark { return p.Marks[id] }

func (p *Progress) Set(id string, m Mark) {
	if m == (Mark{}) {
		delete(p.Marks, id)
		return
	}
	p.Marks[id] = m
}

// Import replaces marks, parts, relics and extra XP from a backup file,
// keeping Seen.
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

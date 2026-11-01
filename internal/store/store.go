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

}

type Progress struct {
	Version int             `json:"version"`
	Marks   map[string]Mark `json:"marks"`
	Extra   ExtraXP         `json:"extra"`
	// Seen holds every item ID the archive has shown before, so items added
	// upstream can be flagged as new.
	Seen     map[string]bool `json:"seen"`
	Settings Settings        `json:"settings"`
	Updated  time.Time       `json:"updated"`
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

func (p *Progress) Get(id string) Mark { return p.Marks[id] }

func (p *Progress) Set(id string, m Mark) {
	if m == (Mark{}) {
		delete(p.Marks, id)
		return
	}
	p.Marks[id] = m
}

// Import replaces marks and extra XP from a backup file, keeping Seen.
func (p *Progress) Import(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	in, err := parse(b)
	if err != nil {
		return err
	}
	p.Marks, p.Extra = in.Marks, in.Extra
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

package store

import (
	"path/filepath"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "progress.json")
	p := New()
	p.Set("/a", Mark{Owned: true, Rank: 12})
	p.SetPart("/a", "/a/bp", 1)
	p.SetRelics("Axi S8", 3)
	p.SetArcane("/arcane", Arcane{Maxed: 2, Copies: 14})
	p.SetSpares("/part", 3)
	p.Extra.StarChart = 100
	if err := p.Save(path); err != nil {
		t.Fatal(err)
	}
	q, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if q.Get("/a") != (Mark{Owned: true, Rank: 12}) || q.HeldParts("/a", "/a/bp") != 1 || q.HeldRelics("Axi S8") != 3 ||
		q.HeldArcane("/arcane") != (Arcane{Maxed: 2, Copies: 14}) || q.HeldSpares("/part") != 3 || q.Extra.StarChart != 100 {
		t.Errorf("reloaded %+v", q)
	}
}

func TestEmptyEntriesAreDropped(t *testing.T) {
	p := New()
	p.Set("/a", Mark{Owned: true})
	p.Set("/a", Mark{})
	p.SetPart("/a", "/a/bp", 2)
	p.SetPart("/a", "/a/bp", 0)
	p.SetRelics("Axi S8", 1)
	p.SetRelics("Axi S8", 0)
	p.SetArcane("/arcane", Arcane{Copies: 3})
	p.SetArcane("/arcane", Arcane{})
	p.SetSpares("/part", 1)
	p.SetSpares("/part", 0)
	if len(p.Marks) != 0 || len(p.Parts) != 0 || len(p.Relics) != 0 || len(p.Arcanes) != 0 || len(p.Spares) != 0 {
		t.Errorf("cleared entries linger: %v %v %v %v %v", p.Marks, p.Parts, p.Relics, p.Arcanes, p.Spares)
	}
}

func TestPlexusMigration(t *testing.T) {
	p, err := parse([]byte(`{"marks":{"seshat:plexus":{"mastered":true}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if !p.Get("/Lotus/Types/Game/CrewShip/RailJack/DefaultHarness").Mastered || len(p.Marks) != 1 {
		t.Errorf("marks = %v", p.Marks)
	}
}

func TestMissingFileIsEmpty(t *testing.T) {
	p, err := Load(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil || len(p.Marks) != 0 {
		t.Errorf("Load of a missing file = %v, %v", p, err)
	}
}

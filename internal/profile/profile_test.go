package profile

import (
	"strings"
	"testing"

	"github.com/Peejidle/seshat/internal/catalog"
	"github.com/Peejidle/seshat/internal/store"
)

const sample = `{"Results":[{
	"DisplayName":"Tenno` + "" + `",
	"LoadOutInventory":{"XPInfo":[
		{"ItemType":"/frame/maxed","XP":900000},
		{"ItemType":"/weapon/half","XP":112500},
		{"ItemType":"/kuva/maxed","XP":800000},
		{"ItemType":"/kuva/rank30","XP":450000},
		{"ItemType":"/not/in/catalog","XP":5}
	]},
	"PlayerSkills":{"LPS_PILOTING":10,"LPS_GUNNERY":5,"LPS_DRIFT_RIDING":3},
	"PlayerLevel":4,
	"Missions":[
		{"Tag":"SolNode108","Completes":3,"Tier":1},
		{"Tag":"SolNode2","Completes":1},
		{"Tag":"MercuryJunction","Completes":1,"Tier":1},
		{"Tag":"EventNode5","Completes":2}
	]
}]}`

// A few lines of the wiki's star chart, as it's written.
const wikiNodes = `
		{ Name = "Tolstoj", Planet = "Mercury", Drops = {"Cronus","Seer"}, MasteryExp = 25, InternalName = "SolNode108", PreviousNodes = { "Suisei" } },
		{ Name = "Aphrodite", Planet = "Venus", MasteryExp = 18, InternalName = "SolNode2" },
		{ Name = "Larunda Relay", Planet = "Mercury", MasteryExp = 0, InternalName = "MercuryHUB" },
		{ Name = "Lunaro Arena", MasteryExp = 0, InternalName = "" },
		{ Name = "Mercury Junction", InternalName = "MercuryJunction", MasteryExp = 1000 },
`

func TestApply(t *testing.T) {
	p, err := Parse([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	if p.DisplayName != "Tenno" {
		t.Errorf("name = %q, want platform glyph stripped", p.DisplayName)
	}
	items := []catalog.Item{
		{ID: "/frame/maxed", MaxRank: 30, XPPerRank: 200},
		{ID: "/weapon/half", MaxRank: 30, XPPerRank: 100},
		{ID: "/kuva/maxed", MaxRank: 40, XPPerRank: 100},
		{ID: "/kuva/rank30", MaxRank: 40, XPPerRank: 100},
		{ID: "/never/used", MaxRank: 30, XPPerRank: 100},
	}
	prog := store.New()
	prog.Set("/kuva/maxed", store.Mark{Mastered: true, Owned: true})

	r := Apply(p, items, prog, ParseNodeXP([]byte(wikiNodes)))
	if r.NewlyMastered != 1 || r.AlreadyMastered != 1 || r.Partial != 2 {
		t.Errorf("result = %+v, want 1 new, 1 already, 2 partial", r)
	}
	if mk := prog.Get("/frame/maxed"); !mk.Mastered || mk.Owned {
		t.Errorf("frame = %+v, want mastered and owned untouched", mk)
	}
	if mk := prog.Get("/kuva/rank30"); mk.Mastered || mk.Rank != 30 {
		t.Errorf("kuva at 30/40 = %+v, want in progress at rank 30", mk)
	}
	if mk := prog.Get("/weapon/half"); mk.Rank != 15 {
		t.Errorf("half-levelled weapon rank = %d, want 15", mk.Rank)
	}
	if got := items[3].EarnedXP(false, 30); got != 3000 {
		t.Errorf("rank 30 of 40 earns %d, want 3000", got)
	}
	if prog.Extra.Railjack != 15*1500 || prog.Extra.Drifter != 3*1500 {
		t.Errorf("intrinsics = %d / %d", prog.Extra.Railjack, prog.Extra.Drifter)
	}
	// Star Chart: Tolstoj 25 + Aphrodite 18 + the junction 1,000; the event
	// node isn't in the wiki's chart and gives nothing. Steel Path: the two
	// cleared at Tier 1.
	if prog.Extra.StarChart != 1043 || prog.Extra.SteelPath != 1025 {
		t.Errorf("star chart %d, steel path %d, want 1043 and 1025", prog.Extra.StarChart, prog.Extra.SteelPath)
	}
	// 14,500 item XP + 27,000 intrinsics + 2,068 nodes = 43,568: MR 4.
	if r.MR != 4 || r.Estimate != 4 || !strings.Contains(strings.Join(Summary(r), " "), "matches the game") {
		t.Errorf("MR %d, estimate %d, summary %q", r.MR, r.Estimate, Summary(r))
	}
}

func TestApplyWithoutNodes(t *testing.T) {
	p, _ := Parse([]byte(sample))
	prog := store.New()
	prog.Extra.StarChart = 500 // typed in by hand: kept when there's no chart
	r := Apply(p, nil, prog, nil)
	if prog.Extra.StarChart != 500 || r.Nodes {
		t.Errorf("star chart %d, nodes %v: should be left alone", prog.Extra.StarChart, r.Nodes)
	}
	if !strings.Contains(strings.Join(Summary(r), " "), "isn't marked right") {
		t.Errorf("an MR mismatch should be reported: %q", Summary(r))
	}
}

func TestParseHTMLWrapped(t *testing.T) {
	// What Chrome's "Webpage, HTML only" save produces.
	page := `<html><head><meta name="color-scheme" content="light dark"></head><body><pre style="word-wrap: break-word;">` +
		`{"Results":[{"DisplayName":"A&amp;B","LoadOutInventory":{"XPInfo":[{"ItemType":"/x","XP":1}]}}]}` +
		`</pre><div class="json-formatter-container"></div></body></html>`
	p, err := Parse([]byte(page))
	if err != nil {
		t.Fatal(err)
	}
	if p.DisplayName != "A&B" || p.XP["/x"] != 1 {
		t.Errorf("got %+v", p)
	}
}

func TestParseRejects(t *testing.T) {
	for _, in := range []string{`<html></html>`, `{"Results":[]}`, `not json`, `{"foo":1}`} {
		if _, err := Parse([]byte(in)); err == nil {
			t.Errorf("Parse(%q) should fail", in)
		}
	}
}

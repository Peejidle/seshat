// Package profile reads a Warframe public profile that the user saved from
// their own browser, and applies it to their checklist. Seshat never
// requests the profile itself: DE rate-limits that endpoint aggressively and
// hasn't sanctioned third-party use, so the one request is the user's.
package profile

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"math"
	"strings"
	"unicode"

	"github.com/Peejidle/seshat/internal/catalog"
	"github.com/Peejidle/seshat/internal/mastery"
	"github.com/Peejidle/seshat/internal/store"
)

// URL is the profile endpoint for PC; consoles use api-ps4, api-xb1,
// api-swi, api-mob or api-and in place of api.
const URL = "https://api.warframe.com/cdn/getProfileViewingData.php?playerId="

// Intrinsic schools, each rank worth 1,500 mastery XP.
var (
	railjackSkills = []string{"LPS_COMMAND", "LPS_ENGINEERING", "LPS_GUNNERY", "LPS_PILOTING", "LPS_TACTICAL"}
	drifterSkills  = []string{"LPS_DRIFT_COMBAT", "LPS_DRIFT_ENDURANCE", "LPS_DRIFT_OPPORTUNITY", "LPS_DRIFT_RIDING"}
)

const xpPerIntrinsic = 1500

type Profile struct {
	DisplayName string
	MR          int            // the Mastery Rank the game shows
	XP          map[string]int // item uniqueName -> affinity earned
	Skills      map[string]int
	Missions    []Mission
}

// Mission is a star chart node the player has cleared, by its internal name
// ("SolNode108"). Tier 1 means it has been cleared on the Steel Path too.
type Mission struct {
	Tag       string `json:"Tag"`
	Completes int    `json:"Completes"`
	Tier      int    `json:"Tier"`
}

type rawResult struct {
	DisplayName      string    `json:"DisplayName"`
	PlayerLevel      int       `json:"PlayerLevel"`
	Missions         []Mission `json:"Missions"`
	LoadOutInventory struct {
		XPInfo []struct {
			ItemType string `json:"ItemType"`
			XP       int    `json:"XP"`
		} `json:"XPInfo"`
	} `json:"LoadOutInventory"`
	PlayerSkills map[string]int `json:"PlayerSkills"`
}

var ErrNotProfile = errors.New("this doesn't look like a Warframe profile file")

// Parse accepts the endpoint's response as saved by a browser, or just the
// single result inside it. A page saved as HTML (Chrome's "Webpage" save
// wraps the JSON in <pre>) is unwrapped first.
func Parse(b []byte) (*Profile, error) {
	b = bytes.TrimSpace(b)
	if len(b) > 0 && b[0] == '<' {
		start, end := bytes.IndexByte(b, '{'), bytes.LastIndexByte(b, '}')
		if start < 0 || end < start {
			return nil, errors.New("this is an HTML page without profile data in it: save the page as raw JSON (Firefox: \"Raw Data\" tab → Save)")
		}
		b = []byte(html.UnescapeString(string(b[start : end+1])))
	}
	var wrapped struct {
		Results []json.RawMessage `json:"Results"`
	}
	if err := json.Unmarshal(b, &wrapped); err != nil {
		return nil, ErrNotProfile
	}
	body := b
	if len(wrapped.Results) > 0 {
		body = wrapped.Results[0]
	}
	var r rawResult
	if err := json.Unmarshal(body, &r); err != nil || len(r.LoadOutInventory.XPInfo) == 0 {
		return nil, ErrNotProfile
	}

	p := &Profile{
		DisplayName: cleanName(r.DisplayName), MR: r.PlayerLevel,
		XP: map[string]int{}, Skills: r.PlayerSkills, Missions: r.Missions,
	}
	for _, x := range r.LoadOutInventory.XPInfo {
		p.XP[x.ItemType] += x.XP
	}
	return p, nil
}

// cleanName drops the platform icon (a private-use glyph) that cross-save
// accounts carry at the end of their name.
func cleanName(s string) string {
	return strings.TrimRightFunc(s, func(r rune) bool { return unicode.Is(unicode.Co, r) })
}

// Level converts affinity to rank: reaching rank n costs xpPerRank*5*n².
func Level(xp, xpPerRank, maxRank int) int {
	if xp <= 0 || xpPerRank <= 0 {
		return 0
	}
	return min(int(math.Sqrt(float64(xp)/float64(xpPerRank*5))), maxRank)
}

// NodeXP adds up the mastery from cleared nodes, given the XP each node is
// worth: once for the Star Chart, and again for each also cleared on the
// Steel Path.
func (p *Profile) NodeXP(nodes map[string]int) (starChart, steelPath int) {
	for _, ms := range p.Missions {
		if ms.Completes > 0 {
			starChart += nodes[ms.Tag]
		}
		if ms.Tier == 1 {
			steelPath += nodes[ms.Tag]
		}
	}
	return starChart, steelPath
}

type Result struct {
	Name              string
	NewlyMastered     int
	AlreadyMastered   int
	Partial           int // levelled but below max rank; their rank is kept
	Railjack, Drifter int // intrinsic ranks
	// StarChart and SteelPath are the node XP worked out, if the wiki's
	// star chart was at hand (Nodes).
	StarChart, SteelPath int
	Nodes                bool
	// MR is the rank the game shows, Estimate the one Seshat now works out.
	MR, Estimate int
}

// Apply marks every item the profile shows at max rank as mastered and
// copies intrinsics and, given the XP per node (nil if the wiki's star
// chart isn't cached), Star Chart and Steel Path XP into the ledger. It
// never unmarks anything, and leaves "owned" alone: the profile also lists
// items that have since been sold.
func Apply(p *Profile, items []catalog.Item, prog *store.Progress, nodes map[string]int) Result {
	res := Result{Name: p.DisplayName}
	for _, it := range items {
		xp := p.XP[it.ID]
		if xp == 0 {
			continue
		}
		mk := prog.Get(it.ID)
		switch lvl := Level(xp, it.XPPerRank, it.MaxRank); {
		case lvl < it.MaxRank:
			res.Partial++
			if !mk.Mastered && lvl > mk.Rank {
				mk.Rank = lvl
				prog.Set(it.ID, mk)
			}
		case mk.Mastered:
			res.AlreadyMastered++
		default:
			mk.Mastered = true
			prog.Set(it.ID, mk)
			res.NewlyMastered++
		}
	}
	if p.Skills != nil {
		res.Railjack, res.Drifter = sum(p.Skills, railjackSkills), sum(p.Skills, drifterSkills)
		prog.Extra.Railjack = res.Railjack * xpPerIntrinsic
		prog.Extra.Drifter = res.Drifter * xpPerIntrinsic
	}
	if len(nodes) > 0 && len(p.Missions) > 0 {
		res.StarChart, res.SteelPath = p.NodeXP(nodes)
		prog.Extra.StarChart, prog.Extra.SteelPath = res.StarChart, res.SteelPath
		res.Nodes = true
	}
	res.MR = p.MR
	res.Estimate, _, _ = mastery.Rank(mastery.Estimate(items, prog))
	return res
}

// Summary reports an import, a line for each thing it did.
func Summary(r Result) []string {
	lines := []string{
		fmt.Sprintf("Profile of %s read: %d newly mastered, %d already marked, %d in progress.",
			r.Name, r.NewlyMastered, r.AlreadyMastered, r.Partial),
	}
	if r.Railjack+r.Drifter > 0 {
		lines = append(lines, fmt.Sprintf("Intrinsics: %d Railjack, %d Drifter.", r.Railjack, r.Drifter))
	}
	if r.Nodes {
		lines = append(lines, fmt.Sprintf("Star Chart %d XP, Steel Path %d XP.", r.StarChart, r.SteelPath))
	} else {
		lines = append(lines, "Star Chart and Steel Path left as they were: sync the archive to fetch the XP per node.")
	}
	switch {
	case r.MR == 0:
		// an old or trimmed profile without the rank
	case r.Estimate == r.MR:
		lines = append(lines, fmt.Sprintf("Seshat's estimate matches the game: %s.", mastery.Label(r.MR)))
	default:
		lines = append(lines, fmt.Sprintf("The game says %s but Seshat works out %s: something isn't marked right.",
			mastery.Label(r.MR), mastery.Label(r.Estimate)))
	}
	return lines
}

func sum(m map[string]int, keys []string) int {
	n := 0
	for _, k := range keys {
		n += m[k]
	}
	return n
}

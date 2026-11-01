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
	"math"
	"strings"
	"unicode"

	"github.com/Peejidle/seshat/internal/catalog"
	"github.com/Peejidle/seshat/internal/store"
)

// URL is the profile endpoint for PC; consoles use api-ps4, api-xb1,
// api-swi, api-mob or api-and in place of api.
const URL = "https://api.warframe.com/cdn/getProfileViewingData.php?playerId="

// AccountIDURL shows a logged-in user their account ID ("user_id").
const AccountIDURL = "https://www.warframe.com/api/user-data"

// Intrinsic schools, each rank worth 1,500 mastery XP.
var (
	railjackSkills = []string{"LPS_COMMAND", "LPS_ENGINEERING", "LPS_GUNNERY", "LPS_PILOTING", "LPS_TACTICAL"}
	drifterSkills  = []string{"LPS_DRIFT_COMBAT", "LPS_DRIFT_ENDURANCE", "LPS_DRIFT_OPPORTUNITY", "LPS_DRIFT_RIDING"}
)

const xpPerIntrinsic = 1500

type Profile struct {
	DisplayName string
	XP          map[string]int // item uniqueName -> affinity earned
	Skills      map[string]int
}

type rawResult struct {
	DisplayName      string `json:"DisplayName"`
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
// single result inside it.
func Parse(b []byte) (*Profile, error) {
	b = bytes.TrimSpace(b)
	if len(b) > 0 && b[0] == '<' {
		return nil, errors.New("this is an HTML page, not the profile data: save the page as raw JSON (Firefox: \"Raw Data\" tab → Save)")
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

	p := &Profile{DisplayName: cleanName(r.DisplayName), XP: map[string]int{}, Skills: r.PlayerSkills}
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

type Result struct {
	Name              string
	NewlyMastered     int
	AlreadyMastered   int
	Partial           int // levelled but below max rank
	Railjack, Drifter int // intrinsic ranks
}

// Apply marks every item the profile shows at max rank as mastered and
// copies intrinsics into the ledger. It never unmarks anything, and leaves
// "owned" alone: the profile also lists items that have since been sold.
func Apply(p *Profile, items []catalog.Item, prog *store.Progress) Result {
	res := Result{Name: p.DisplayName}
	for _, it := range items {
		xp := p.XP[it.ID]
		if xp == 0 {
			continue
		}
		mk := prog.Get(it.ID)
		switch {
		case Level(xp, it.XPPerRank, it.MaxRank) < it.MaxRank:
			res.Partial++
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
	return res
}

// Summary is a one-line report of an import.
func Summary(r Result) string {
	s := fmt.Sprintf("Profile of %s read: %d newly mastered, %d already marked, %d levelled but not maxed.",
		r.Name, r.NewlyMastered, r.AlreadyMastered, r.Partial)
	if r.Railjack+r.Drifter > 0 {
		s += fmt.Sprintf(" Intrinsics: %d Railjack, %d Drifter.", r.Railjack, r.Drifter)
	}
	return s
}

func sum(m map[string]int, keys []string) int {
	n := 0
	for _, k := range keys {
		n += m[k]
	}
	return n
}

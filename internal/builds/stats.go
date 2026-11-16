package builds

import (
	"math"
	"regexp"
	"strconv"

	"github.com/Peejidle/seshat/internal/catalog"
	"github.com/Peejidle/seshat/internal/store"
)

// A warframe's stats, as the arsenal shows them, and what a build's mods
// and Archon shards make of them. Only what adds up plainly is counted:
// a mod line like "+55% Ability Duration". Conditional effects ("on
// kill", "while airborne"), set bonuses and arcanes aren't, and the panel
// says which mods have something it couldn't count.

// Stat is one of the numbers the stats panel shows.
type Stat int

const (
	Health Stat = iota
	Shields
	Armor
	Energy
	Sprint
	Strength
	Duration
	Range
	Efficiency
	NumStats
)

// Name is how the panel labels it.
func (s Stat) Name() string {
	return [...]string{"Health", "Shields", "Armor", "Energy", "Sprint Speed",
		"Ability Strength", "Ability Duration", "Ability Range", "Ability Efficiency"}[s]
}

// Ability is whether it's one of the four ability stats, shown as a
// percentage that starts at 100%.
func (s Stat) Ability() bool { return s >= Strength }

// modStats are the words mods use for each stat, as in "+440% Health".
var modStats = map[string]Stat{
	"Health": Health, "Shield Capacity": Shields, "Armor": Armor, "Energy Max": Energy,
	"Sprint Speed": Sprint, "Ability Strength": Strength, "Ability Duration": Duration,
	"Ability Range": Range, "Ability Efficiency": Efficiency,
}

var (
	statLine = regexp.MustCompile(`^([+-]?\d+(?:\.\d+)?)% (.+)$`)
	// timesLine is a stat multiplied, as Catalyzing Shields writes it:
	// "x0.20 Max Shield Capacity".
	timesLine = regexp.MustCompile(`^x(\d+(?:\.\d+)?) Max (.+)$`)
)

// parseStat reads a plain stat line, "+440% Health" or "-60% Ability
// Range", as the stat and the fraction (4.4, -0.6). ok is false for
// anything else.
func parseStat(line string) (Stat, float64, bool) {
	m := statLine.FindStringSubmatch(line)
	if m == nil {
		return 0, 0, false
	}
	s, ok := modStats[m[2]]
	if !ok {
		return 0, 0, false
	}
	v, _ := strconv.ParseFloat(m[1], 64)
	return s, v / 100, true
}

// parseTimes reads a multiplied stat, "x0.20 Max Shield Capacity", as the
// stat and the factor.
func parseTimes(line string) (Stat, float64, bool) {
	m := timesLine.FindStringSubmatch(line)
	if m == nil {
		return 0, 0, false
	}
	s, ok := modStats[m[2]]
	if !ok {
		return 0, 0, false
	}
	v, _ := strconv.ParseFloat(m[1], 64)
	return s, v, true
}

// Rank 30 adds this much to a warframe's health, shields and energy over
// what WFCD lists, which is rank 0; armor doesn't grow. (Checked on
// Titania Prime: 365 health in WFCD, 465 in the arsenal at rank 30.)
var rankGrowth = [NumStats]float64{Health: 100, Shields: 100, Energy: 50}

// Stats is a warframe's stats at rank 30 with no mods (Base), and with the
// build's active config and Archon shards (Final).
type Stats struct {
	Base, Final [NumStats]float64
	// Uncounted names the mods with an effect beyond plain stats, which
	// the panel can't add up.
	Uncounted []string
}

// exact is a mod's percentage at a rank as the game counts it, not as it
// shows it. Most mods grow by the same step each rank, a step that's often
// not a round number: Vitality is +100% at rank 10, so +9.09% a rank, and
// shows +91% at rank 9 where it's really +90.9%. So where the shown value
// is the rounded step, the step is used (v is the shown fraction, line j).
func exact(md Mod, rank, j int, s Stat, v float64) float64 {
	top := len(md.Stats) - 1
	if rank == top || j >= len(md.Stats[top]) {
		return v
	}
	ts, tv, ok := parseStat(md.Stats[top][j])
	if !ok || ts != s {
		return v
	}
	e := tv * float64(rank+1) / float64(top+1)
	if math.Abs(e-v) > 0.005+1e-9 { // more than the rounding: not even steps
		return v
	}
	return e
}

// FrameStats works out a warframe's stats with a build's active config
// and shards. Percentages from mods multiply the rank 30 value; shards
// add theirs on top (flat amounts after the mods, percentages alongside
// the mods').
func FrameStats(d *Data, it catalog.Item, b store.Build) Stats {
	var st Stats
	st.Base[Health], st.Base[Shields] = float64(it.Health), float64(it.Shield)
	st.Base[Armor], st.Base[Energy] = float64(it.Armor), float64(it.Energy)
	st.Base[Sprint] = it.SprintSpeed
	for s := range Stat(Sprint) {
		if st.Base[s] > 0 { // a frame without shields or energy doesn't gain any
			st.Base[s] += rankGrowth[s]
		}
	}
	for s := Strength; s < NumStats; s++ {
		st.Base[s] = 1
	}

	var pct, flat [NumStats]float64
	times := [NumStats]float64{}
	for s := range NumStats {
		times[s] = 1
	}
	for i, pm := range b.Config().Mods {
		md, ok := d.Mod(pm.ID)
		if !ok || i >= len(b.Slots) || pm.Rank >= len(md.Stats) {
			continue
		}
		counted := true
		for j, line := range md.Stats[pm.Rank] {
			if s, v, ok := parseStat(line); ok {
				pct[s] += exact(md, pm.Rank, j, s, v)
			} else if s, v, ok := parseTimes(line); ok {
				times[s] *= v
			} else {
				counted = false
			}
		}
		if !counted {
			st.Uncounted = append(st.Uncounted, md.Name)
		}
	}
	for _, sh := range b.Shards {
		opt, ok := ShardByID(sh.ID)
		if !ok || !opt.Counted {
			continue
		}
		v := opt.Value
		if sh.Tauforged {
			v = opt.Tau
		}
		if opt.Stat.Ability() {
			v /= 100 // a percentage
		}
		if opt.Stat.Ability() {
			pct[opt.Stat] += v
		} else {
			flat[opt.Stat] += v
		}
	}
	for s := range NumStats {
		if s.Ability() {
			st.Final[s] = st.Base[s] + pct[s]
		} else {
			st.Final[s] = (st.Base[s]*(1+pct[s]) + flat[s]) * times[s]
		}
	}
	return st
}

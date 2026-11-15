package builds

import (
	"strconv"
	"strings"
)

// Archon shards: five slots on a warframe, each holding a shard of one
// colour with one of that colour's bonuses, half as much again if it's
// Tauforged. WFCD doesn't have them, so they're written out here, and
// only the colours checked so far are.

// NumShards is how many shard slots a warframe has.
const NumShards = 5

// ShardOption is one bonus a shard can give.
type ShardOption struct {
	ID     string // what a build saves, e.g. "crimson-strength"
	Colour string
	// Text says the bonus as the game does, shortened to fit: {} is where
	// Value (or Tau, Tauforged) goes, and {2} Value2 (Tau2) where there's
	// a second number, such as a cap.
	Text         string
	Value, Tau   float64
	Value2, Tau2 float64
	// Stat is the stats panel line it adds to, if Counted: a flat amount,
	// or a percentage for the ability stats. Most bonuses aren't on the
	// panel: they depend on what you're doing or are about weapons.
	Stat    Stat
	Counted bool
}

// ShardOptions lists every bonus, by colour, as the game's shard menu
// does (checked against it in November 2026).
var ShardOptions = []ShardOption{
	{ID: "crimson-melee-crit", Colour: "Crimson", Text: "+{}% Melee Critical Damage", Value: 25, Tau: 37.5},
	{ID: "crimson-primary-status", Colour: "Crimson", Text: "+{}% Primary Status Chance", Value: 25, Tau: 37.5},
	{ID: "crimson-secondary-crit", Colour: "Crimson", Text: "+{}% Secondary Critical Chance", Value: 25, Tau: 37.5},
	{ID: "crimson-strength", Colour: "Crimson", Text: "+{}% Ability Strength", Value: 10, Tau: 15, Stat: Strength, Counted: true},
	{ID: "crimson-duration", Colour: "Crimson", Text: "+{}% Ability Duration", Value: 10, Tau: 15, Stat: Duration, Counted: true},

	{ID: "azure-health", Colour: "Azure", Text: "+{} Health", Value: 150, Tau: 225, Stat: Health, Counted: true},
	{ID: "azure-shields", Colour: "Azure", Text: "+{} Shield Capacity", Value: 150, Tau: 225, Stat: Shields, Counted: true},
	{ID: "azure-energy", Colour: "Azure", Text: "+{} Energy Max", Value: 50, Tau: 75, Stat: Energy, Counted: true},
	{ID: "azure-armor", Colour: "Azure", Text: "+{} Armor", Value: 150, Tau: 225, Stat: Armor, Counted: true},
	{ID: "azure-regen", Colour: "Azure", Text: "+{} Health/s Regenerated", Value: 5, Tau: 7.5},

	{ID: "amber-initial-energy", Colour: "Amber", Text: "+{}% Maximum Energy is filled on Spawn", Value: 30, Tau: 45},
	{ID: "amber-health-orbs", Colour: "Amber", Text: "+{}% Effectiveness on Health Orbs", Value: 100, Tau: 150},
	{ID: "amber-energy-orbs", Colour: "Amber", Text: "+{}% Effectiveness on Energy Orbs", Value: 50, Tau: 75},
	{ID: "amber-casting", Colour: "Amber", Text: "+{}% Casting Speed", Value: 25, Tau: 37.5},
	{ID: "amber-parkour", Colour: "Amber", Text: "+{}% Parkour Velocity", Value: 15, Tau: 22.5},

	{ID: "emerald-toxin", Colour: "Emerald", Text: "Toxin Status Effects deal +{}% more damage", Value: 30, Tau: 45},
	{ID: "emerald-toxin-health", Colour: "Emerald", Text: "Recover +{} Health each time enemies take Toxin Status damage", Value: 2, Tau: 3},
	{ID: "emerald-corrosive-damage", Colour: "Emerald", Text: "+{}% Ability Damage on enemies with Corrosive Status", Value: 10, Tau: 15},
	{ID: "emerald-corrosive-stacks", Colour: "Emerald", Text: "+{} max stacks of Corrosive Status", Value: 2, Tau: 3},

	{ID: "topaz-blast-health", Colour: "Topaz", Text: "Gain {} Health per Blast kill, max {2} Health", Value: 1, Tau: 2, Value2: 300, Tau2: 450},
	{ID: "topaz-blast-shields", Colour: "Topaz", Text: "Regenerate +{} Shields per Blast kill", Value: 5, Tau: 7.5},
	{ID: "topaz-heat-crit", Colour: "Topaz", Text: "+{}% Secondary Critical Chance per Heat Status kill, max {2}%", Value: 1, Tau: 1.5, Value2: 50, Tau2: 75},
	{ID: "topaz-radiation", Colour: "Topaz", Text: "+{}% Ability Damage on enemies with Radiation Status", Value: 10, Tau: 15},

	{ID: "violet-electricity", Colour: "Violet", Text: "+{}% Ability Damage on enemies with Electricity Status", Value: 10, Tau: 15},
	{ID: "violet-primary-electricity", Colour: "Violet", Text: "+{}% Primary Electricity Damage, +{2}% per Crimson, Azure or Violet shard", Value: 30, Tau: 45, Value2: 10, Tau2: 15},
	// The game's own figure for Tauforged here is 37, not 37.5.
	{ID: "violet-melee-crit", Colour: "Violet", Text: "+{}% Melee Critical Damage, doubled over 500 max Energy", Value: 25, Tau: 37},
	{ID: "violet-pickups", Colour: "Violet", Text: "Health pickups give +{}% Energy, Energy pickups +{}% Health", Value: 20, Tau: 30},
}

// ShardByID finds a shard bonus by its ID.
func ShardByID(id string) (ShardOption, bool) {
	for _, o := range ShardOptions {
		if o.ID == id {
			return o, true
		}
	}
	return ShardOption{}, false
}

// Amount says the bonus with its numbers, e.g. "+10% Ability Strength",
// or with tauforged, "+15% Ability Strength".
func (o ShardOption) Amount(tauforged bool) string {
	v, v2 := o.Value, o.Value2
	if tauforged {
		v, v2 = o.Tau, o.Tau2
	}
	s := strings.ReplaceAll(o.Text, "{2}", trimFloat(v2))
	return strings.ReplaceAll(s, "{}", trimFloat(v))
}

// trimFloat writes 15 as "15" and 37.5 as "37.5".
func trimFloat(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

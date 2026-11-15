package builds

import (
	"fmt"
	"strings"

	"github.com/Peejidle/seshat/internal/catalog"
	"github.com/Peejidle/seshat/internal/store"
)

// Polarity is a mod's or a slot's polarity, as WFCD names it.
type Polarity string

const (
	None      Polarity = ""
	Madurai   Polarity = "madurai"
	Vazarin   Polarity = "vazarin"
	Naramon   Polarity = "naramon"
	Zenurik   Polarity = "zenurik"
	Unairu    Polarity = "unairu"
	Penjaga   Polarity = "penjaga"
	Umbra     Polarity = "umbra"
	Universal Polarity = "universal" // matches any mod
)

// SlotPolarities are the polarities a Forma can give a slot, in the order
// p steps through them: the four, Umbra, and for an aura or stance slot,
// Any. WFCD also has Penjaga (companion mods) and Unairu (a few stances
// and stance slots); they're understood where the data has them, so those
// costs come out right, but not offered.
func SlotPolarities(k Kind, slot int) []Polarity {
	if slot == SlotAura {
		return []Polarity{None, Madurai, Vazarin, Naramon, Zenurik, Universal}
	}
	return []Polarity{None, Madurai, Vazarin, Naramon, Zenurik, Umbra}
}

// Symbol is a polarity in one letter, its first: the game's own symbols
// are pictures a terminal can't show. Unairu, only in the data, gets a
// small u so as not to look like Umbra.
func (p Polarity) Symbol() string {
	switch p {
	case Madurai:
		return "M"
	case Vazarin:
		return "V"
	case Naramon:
		return "N"
	case Zenurik:
		return "Z"
	case Umbra:
		return "U"
	case Unairu:
		return "u"
	case Penjaga:
		return "P"
	case Universal, "aura":
		return "A"
	}
	return " "
}

// Title is its name, e.g. "Madurai".
func (p Polarity) Title() string {
	switch p {
	case None:
		return "no polarity"
	case Universal, "aura":
		return "Any"
	}
	return strings.ToUpper(string(p[:1])) + string(p[1:])
}

// Kind is the kind of item a build is for, which decides its slots and
// what mods and arcanes fit.
type Kind int

const (
	NoKind Kind = iota
	Warframe
	Primary
	Secondary
	Melee
)

// KindOf says what kind of build an item takes, or NoKind if Seshat
// doesn't plan builds for it (yet: companions, archwing and the like).
func KindOf(it catalog.Item) Kind {
	switch it.Category {
	case catalog.CatWarframes:
		return Warframe
	case catalog.CatPrimary:
		return Primary
	case catalog.CatSecondary:
		return Secondary
	case catalog.CatMelee:
		return Melee
	}
	return NoKind
}

// The slots of a build, as store.Build.Slots holds them: eight ordinary
// ones, then the aura (a warframe) or stance (a melee weapon), then the
// exilus.
const (
	NumMods    = 8
	SlotAura   = 8 // also the stance slot
	SlotExilus = 9
	NumSlots   = 10
)

// HasAura is whether the item has a slot at SlotAura: a warframe's aura
// or a melee weapon's stance.
func (k Kind) HasAura() bool { return k == Warframe || k == Melee }

// SlotName names the special slots: "Aura", "Stance", "Exilus", or "".
func (k Kind) SlotName(slot int) string {
	switch {
	case slot == SlotAura && k == Warframe:
		return "Aura"
	case slot == SlotAura && k == Melee:
		return "Stance"
	case slot == SlotExilus:
		return "Exilus"
	}
	return ""
}

// ArcaneSlots is how many arcanes it takes: two on a warframe, one on a weapon.
func (k Kind) ArcaneSlots() int {
	if k == Warframe {
		return 2
	}
	return 1
}

// Reactor is what doubles its capacity.
func (k Kind) Reactor() string {
	if k == Warframe {
		return "Orokin Reactor"
	}
	return "Orokin Catalyst"
}

// ExilusAdapter is what unlocks its exilus slot.
func (k Kind) ExilusAdapter() string {
	if k == Warframe {
		return "Exilus Warframe Adapter"
	}
	return "Exilus Weapon Adapter"
}

// ArcaneAdapter is what unlocks one of its arcane slots.
func (k Kind) ArcaneAdapter() string {
	switch k {
	case Primary:
		return "Primary Arcane Adapter"
	case Secondary:
		return "Secondary Arcane Adapter"
	case Melee:
		return "Melee Arcane Adapter"
	}
	return "Warframe Arcane Adapter"
}

// New starts a build for it, with its slots' polarities as it comes and
// its three configs empty.
func New(it catalog.Item) store.Build {
	k := KindOf(it)
	b := store.Build{
		Slots:           make([]store.BuildSlot, NumSlots),
		ArcanesUnlocked: make([]bool, k.ArcaneSlots()),
	}
	for range store.NumConfigs {
		b.Configs = append(b.Configs, store.BuildConfig{
			Mods:    make([]store.BuildMod, NumSlots),
			Arcanes: make([]store.BuildMod, k.ArcaneSlots()),
		})
	}
	if k == Warframe {
		b.Shards = make([]store.BuildShard, NumShards)
	}
	set := func(i int, p string) { b.Slots[i].Polarity, b.Slots[i].Has = p, p }
	for i, p := range it.Polarities {
		if i < NumMods {
			set(i, p)
		}
	}
	switch k {
	case Warframe:
		set(SlotAura, it.AuraPolarity)
	case Melee:
		set(SlotAura, it.StancePolarity)
	}
	set(SlotExilus, it.ExilusPolarity)
	return b
}

// Fits is whether a mod can go in a slot of a build for it.
func Fits(m Mod, it catalog.Item, slot int) bool {
	k := KindOf(it)
	if slot == SlotAura {
		return (k == Warframe && m.Aura()) || (k == Melee && m.Stance())
	}
	if m.Aura() || m.Stance() || (slot == SlotExilus && !m.Exilus) {
		return false
	}
	c := m.Compat
	switch k {
	case Warframe:
		return m.Type == "Warframe Mod" && (c == "WARFRAME" || named(c, it.Name))
	case Primary:
		if it.Type == "Shotgun" {
			return m.Type == "Shotgun Mod" ||
				(m.Type == "Primary Mod" && (c == "PRIMARY" || c == "Shotgun" || named(c, it.Name)))
		}
		if m.Type != "Primary Mod" {
			return false
		}
		switch c {
		case "Rifle", "Assault Rifle", "Rifle (No Aoe)", "PRIMARY", "Unique":
			return true
		case "Sniper", "Bow":
			return it.Type == c
		case "Shotgun":
			return false
		}
		return named(c, it.Name)
	case Secondary:
		if m.Type != "Secondary Mod" {
			return false
		}
		switch c {
		case "Pistol", "Pistol (No Aoe)", "Thrown", "Tome":
			return true
		}
		return named(c, it.Name)
	case Melee:
		// WFCD doesn't say what class a melee weapon is (sword, polearm
		// and so on), so the mods for a class fit them all.
		return m.Type == "Melee Mod" && (c == "Melee" || c == "Thrown Melee" || meleeClasses[c] || named(c, it.Name))
	}
	return false
}

// named is whether compat names the item: an augment for the frame
// ("Limbo" for Limbo Prime) or a mod for one weapon ("Hek" for Vaykor Hek).
func named(compat, name string) bool {
	return compat != "" && strings.Contains(" "+name+" ", " "+compat+" ")
}

// meleeClasses are the melee classes mods are made for.
var meleeClasses = map[string]bool{
	"Daggers": true, "Dual Daggers": true, "Polearms": true, "Whips": true,
	"Dual Cleavers": true, "Swords": true, "Heavy Blade": true, "Nikanas": true,
	"Claws": true, "Fists": true, "Glaives": true, "Hammers": true, "Staves": true,
}

// ArcaneFits is whether an arcane can go on it.
func ArcaneFits(a Arcane, it catalog.Item) bool {
	switch KindOf(it) {
	case Warframe:
		return a.Type == "Warframe Arcane"
	case Primary:
		return a.Type == "Primary Arcane" || (a.Type == "Bow Arcane" && it.Type == "Bow") ||
			(a.Type == "Shotgun Arcane" && it.Type == "Shotgun")
	case Secondary:
		return a.Type == "Secondary Arcane"
	case Melee:
		return a.Type == "Melee Arcane"
	}
	return false
}

func matches(mod, slot Polarity) bool {
	return slot == mod || slot == Universal || mod == Universal || mod == "aura"
}

// Drain is the capacity a mod at rank takes in a slot of polarity slot.
// A matching polarity halves it, rounding up; a different one adds a
// quarter, rounding to the nearest. Auras and stances give capacity
// instead, so theirs is negative: doubled by a match, cut by a quarter
// by a mismatch.
func Drain(m Mod, rank int, slot Polarity) int {
	if m.BaseDrain < 0 {
		give := -m.BaseDrain + rank
		switch {
		case slot == None:
		case matches(m.Polarity, slot):
			give *= 2
		default:
			give = (give*3 + 2) / 4
		}
		return -give
	}
	cost := m.BaseDrain + rank
	switch {
	case slot == None:
		return cost
	case matches(m.Polarity, slot):
		return (cost + 1) / 2
	}
	return (cost*5 + 2) / 4
}

// Plan is what a build adds up to: the capacity of the config being
// looked at, and what the item still needs in the game for any of its
// configs.
type Plan struct {
	Used  int // capacity the config's mods take
	Bonus int // capacity its aura or stance gives
	// Base is the item's capacity at max rank without a Reactor or
	// Catalyst; Capacity is what it has as planned, doubled if one is in or
	// needed.
	Base, Capacity int
	NeedReactor    bool // some config's mods don't fit without one, and it isn't in yet
	// Forma lists the polarities ordinary Forma are still to give slots.
	// The aura, stance and umbra ones take their own kinds of Forma.
	Forma       []Polarity
	AuraForma   int
	StanceForma int
	UmbraForma  int
	NeedExilus  bool // a config has a mod in the exilus slot, and it's locked
	// ArcaneLocks is how many locked arcane slots a config has an arcane for.
	ArcaneLocks int
	// Over lists the configs (0 for A) that don't fit even with a Reactor
	// or Catalyst.
	Over []int
	// Shards lists the Archon shards planned but not in the game yet, e.g.
	// "Crimson", "Tauforged Azure".
	Shards []string
}

// Free is the capacity left over in the config being looked at; negative
// if its mods don't fit even with a Reactor or Catalyst.
func (p Plan) Free() int { return p.Capacity + p.Bonus - p.Used }

// Done is whether the item has everything its configs need, and they all fit.
func (p Plan) Done() bool {
	return !p.NeedReactor && len(p.Forma) == 0 && p.AuraForma == 0 && p.StanceForma == 0 &&
		p.UmbraForma == 0 && !p.NeedExilus && p.ArcaneLocks == 0 && len(p.Over) == 0 && len(p.Shards) == 0
}

// Work adds up a build for it: the active config's capacity, and the
// item's needs across all of them.
func Work(d *Data, it catalog.Item, b store.Build) Plan {
	k := KindOf(it)
	p := Plan{Base: it.MaxRank}
	for i, s := range b.Slots {
		if s.Polarity == s.Has {
			continue
		}
		switch {
		case i == SlotAura && k == Warframe:
			p.AuraForma++
		case i == SlotAura:
			p.StanceForma++
		case Polarity(s.Polarity) == Umbra:
			p.UmbraForma++
		default:
			p.Forma = append(p.Forma, Polarity(s.Polarity))
		}
	}

	locked := make([]bool, len(b.ArcanesUnlocked)) // locked slots some config uses
	for c, cfg := range b.Configs {
		used, bonus := configCost(d, b, cfg)
		if c == b.Active {
			p.Used, p.Bonus = used, bonus
		}
		if used > p.Base+bonus && !b.Reactor {
			p.NeedReactor = true
		}
		if used > 2*p.Base+bonus {
			p.Over = append(p.Over, c)
		}
		if m := cfg.Mods[SlotExilus]; m.ID != "" && !b.Exilus {
			p.NeedExilus = true
		}
		for i, a := range cfg.Arcanes {
			if _, ok := d.Arcane(a.ID); ok && i < len(locked) && !b.ArcanesUnlocked[i] {
				locked[i] = true
			}
		}
	}
	for _, l := range locked {
		if l {
			p.ArcaneLocks++
		}
	}
	for _, sh := range b.Shards {
		if o, ok := ShardByID(sh.ID); ok && !sh.Installed {
			name := o.Colour
			if sh.Tauforged {
				name = "Tauforged " + name
			}
			p.Shards = append(p.Shards, name)
		}
	}
	p.Capacity = p.Base
	if b.Reactor || p.NeedReactor {
		p.Capacity = 2 * p.Base
	}
	return p
}

// configCost is the capacity a config's mods take, and what its aura or
// stance gives.
func configCost(d *Data, b store.Build, cfg store.BuildConfig) (used, bonus int) {
	for i, s := range b.Slots {
		if i >= len(cfg.Mods) {
			break
		}
		m, ok := d.Mod(cfg.Mods[i].ID)
		if !ok {
			continue
		}
		if dr := Drain(m, cfg.Mods[i].Rank, Polarity(s.Polarity)); dr < 0 {
			bonus -= dr
		} else {
			used += dr
		}
	}
	return used, bonus
}

// Missing lists what the item still needs for the build, in the order
// you'd get them, e.g. "Orokin Catalyst", "2 Forma (V, D)". nil when
// there's nothing left to do.
func (p Plan) Missing(k Kind) []string {
	var out []string
	if p.NeedReactor {
		out = append(out, k.Reactor())
	}
	if n := len(p.Forma); n > 0 {
		var syms []string
		for _, f := range p.Forma {
			if f == None {
				syms = append(syms, "clear")
			} else {
				syms = append(syms, f.Symbol())
			}
		}
		out = append(out, fmt.Sprintf("%d Forma (%s)", n, strings.Join(syms, ", ")))
	}
	if p.UmbraForma > 0 {
		out = append(out, fmt.Sprintf("%d Umbra Forma", p.UmbraForma))
	}
	if p.AuraForma > 0 {
		out = append(out, "Aura Forma")
	}
	if p.StanceForma > 0 {
		out = append(out, "Stance Forma")
	}
	if p.NeedExilus {
		out = append(out, k.ExilusAdapter())
	}
	switch n := len(p.Shards); {
	case n == 1:
		out = append(out, "Archon Shard ("+p.Shards[0]+")")
	case n > 1:
		out = append(out, fmt.Sprintf("%d Archon Shards (%s)", n, strings.Join(p.Shards, ", ")))
	}
	switch {
	case p.ArcaneLocks == 1:
		out = append(out, k.ArcaneAdapter())
	case p.ArcaneLocks > 1:
		out = append(out, fmt.Sprintf("%d %ss", p.ArcaneLocks, k.ArcaneAdapter()))
	}
	return out
}

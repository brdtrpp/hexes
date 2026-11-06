// Package loot rolls the parts that drop from enemies, by rarity weighted
// for depth, and holds the player's inventory of them. Rolls are pure
// functions of a seed, so a run replays the same drops, and none of the
// rolling or inventory code allocates: it runs in the middle of the
// simulation when something dies.
package loot

import "hexes/skill"

// RNG is SplitMix64, the same generator the floor and hex layouts use:
// identical on every platform and Go version.
type RNG struct{ s uint64 }

// NewRNG seeds a generator.
func NewRNG(seed uint64) RNG { return RNG{seed} }

// Next returns 64 random bits.
func (r *RNG) Next() uint64 {
	r.s += 0x9e3779b97f4a7c15
	z := r.s
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}

// Float is uniform in [0, 1).
func (r *RNG) Float() float32 { return float32(r.Next()>>40) / (1 << 24) }

// Intn is uniform in [0, n).
func (r *RNG) Intn(n int) int { return int(r.Next() % uint64(n)) }

// Range is uniform in [lo, hi).
func (r *RNG) Range(lo, hi float32) float32 { return lo + (hi-lo)*r.Float() }

// pick returns an index with probability proportional to its weight.
func (r *RNG) pick(w []float32) int {
	var sum float32
	for _, x := range w {
		sum += x
	}
	x := r.Float() * sum
	for i, v := range w {
		if x < v {
			return i
		}
		x -= v
	}
	return len(w) - 1
}

// Seed mixes a floor seed and an entity into the seed for that entity's
// drop, so who drops what doesn't depend on kill order.
func Seed(floorSeed uint64, entity int) uint64 {
	r := RNG{floorSeed ^ (uint64(entity)+1)*0xd1b54a32d192ed03}
	return r.Next()
}

// Rarity weights by depth. Common thins out with depth and the rest
// thicken, each to a ceiling. Placeholder curve, all constants.
const (
	CommonBase, CommonPerDepth, CommonFloor             = 72, -2.5, 20
	UncommonBase, UncommonPerDepth, UncommonCap         = 22, 1.0, 40
	RareBase, RarePerDepth, RareCap                     = 5, 0.6, 28
	ExoticBase, ExoticPerDepth, ExoticCap               = 1, 0.3, 14
	RareQuirkChance                             float32 = 0.35 // exotics always bend a rule
)

// RarityWeights is the relative chance of each tier at a depth.
func RarityWeights(depth int) [skill.NumRarities]float32 {
	d := float32(max(0, depth-1))
	return [skill.NumRarities]float32{
		max(CommonFloor, CommonBase+CommonPerDepth*d),
		min(UncommonCap, UncommonBase+UncommonPerDepth*d),
		min(RareCap, RareBase+RarePerDepth*d),
		min(ExoticCap, ExoticBase+ExoticPerDepth*d),
	}
}

// RollRarity picks a tier for a drop at depth.
func RollRarity(r *RNG, depth int) skill.Rarity {
	w := RarityWeights(depth)
	return skill.Rarity(r.pick(w[:]))
}

// Table rolls parts against a damage type registry.
type Table struct {
	Reg *skill.Registry
	// Damage is the base damage amount per type, before the roll's spread
	// and rarity.
	Damage [skill.MaxTypes]float32
	// Allow limits what rolls to what the player has been taught. The
	// zero value allows everything.
	Allow Allow
}

// Allow is what a roll may make. With Limit off everything rolls; with
// it on only the part kinds in Kinds (one bit per skill.PartKind) roll,
// sustain effects only with Sustain, spend and empty gates only with
// Spend, and exotics, quirks, channel and watch triggers only with
// Quirks. Filtering only zeroes weights, so it never allocates.
type Allow struct {
	Limit   bool
	Kinds   uint16
	Sustain bool
	Spend   bool
	Quirks  bool
	// New is which families of the class parts may roll, one bit each
	// (FamForms and on; see newparts.go). They roll only when it's set,
	// whatever Limit says, so a table without it rolls exactly as before.
	New uint16
}

// KindBit is a part kind's bit in Allow.Kinds.
func KindBit(k skill.PartKind) uint16 { return 1 << k }

func (a Allow) kind(k skill.PartKind) bool { return !a.Limit || a.Kinds&KindBit(k) != 0 }
func (a Allow) sustain() bool              { return !a.Limit || a.Sustain }
func (a Allow) spend() bool                { return !a.Limit || a.Spend }
func (a Allow) quirks() bool               { return !a.Limit || a.Quirks }

// Any reports whether anything at all can roll.
func (a Allow) Any() bool {
	if !a.Limit {
		return true
	}
	for _, k := range partKinds {
		if a.Kinds&KindBit(k) != 0 {
			return true
		}
	}
	return false
}

// Base damage per type for drops. Nanite is high because it arrives as
// ticks; kinetic is low because every hit also shoves. Unlisted types use
// DefaultDamage.
var baseDamage = map[string]float32{
	"plasma": 18, "cryo": 15, "kinetic": 10, "nanite": 28, "drone": 14, "cloak": 16,
}

// DefaultDamage is the base damage for a type with no entry above.
const DefaultDamage = 16

// NewTable builds a table for a registry.
func NewTable(reg *skill.Registry) *Table {
	t := &Table{Reg: reg}
	for i := 0; i < reg.Len(); i++ {
		t.Damage[i] = DefaultDamage
		if v, ok := baseDamage[reg.Get(skill.TypeID(i)).Name]; ok {
			t.Damage[i] = v
		}
	}
	return t
}

// Kind weights: effect, target, multiplier, trigger, gate, charge, call.
// Triggers are rare finds, about 1 drop in 25.
var kindWeights = [...]float32{40, 22, 22, 4.5, 10, 8, 7}

// Exotics always carry a quirk, so they skip targets, gates, charges and
// calls, which have none. The exotic trigger is the watch.
var exoticKindWeights = [...]float32{45, 0, 30, 4, 0, 0, 0}

var partKinds = [...]skill.PartKind{skill.KindEffect, skill.KindTarget, skill.KindMult, skill.KindTrigger, skill.KindGate, skill.KindCharge, skill.KindCall}

// CallTargets is how many equipped hexes a dropped Call can point at.
// CallSelfChance is how often it rolls pointing at its own hex instead.
// The editor can point it anywhere.
const (
	CallTargets            = 4
	CallSelfChance float32 = 0.3
)

var gateWeights = [...]float32{
	skill.GateHull: 30, skill.GateHeat: 18, skill.GatePower: 14, skill.GateStatus: 22, skill.GateCrowd: 16,
	skill.GateSpend: 18, skill.GateEmpty: 6,
}

// DrainChance is how often a charge part rolls as a drain.
const DrainChance = 0.25

// Spend gates roll needing SpendMin to SpendMax charge, less for rarer
// tiers (one less per tier, never under 2).
const (
	SpendMin = 3
	SpendMax = 8
)

// Trigger weights within a tier: a trigger's kind sets its tier (see
// TriggerKind.Tier), so a rarity roll picks among the kinds of that tier.
var triggerWeights = [...]float32{
	skill.OnCast: 26, skill.OnHit: 20, skill.OnKill: 20, skill.FromStealth: 4,
	skill.FromBehind: 4, skill.Periodic: 16, skill.OnHurt: 10, skill.Channel: 12, skill.Watch: 1,
}

var targetWeights = [...]float32{
	skill.TargetSelf: 4, skill.TargetNearest: 26, skill.TargetCone: 20,
	skill.TargetAdjacent: 26, skill.TargetChain: 24,
}

var effectWeights = [skill.NumEffects]float32{skill.EffectDamage: 62, skill.EffectPush: 14, skill.EffectStatus: 24}

// sustainWeights join the effect roll at uncommon and rare: Mend, Purge
// and Shield never drop common, so you can't count on one. Exotics always
// bend a rule and no quirk fits a sustain part, so they skip it.
var sustainWeights = [skill.NumEffects]float32{skill.EffectMend: 6, skill.EffectPurge: 6, skill.EffectShield: 6}

// Sustain rolls: Mend and Shield take 6 to 12% and 10 to 18% of max
// hull, Purge 10 to 20 Heat, all before rarity scales them.
const (
	MendMin, MendMax     = 0.06, 0.12
	ShieldMin, ShieldMax = 0.10, 0.18
	PurgeMin, PurgeMax   = 10, 20
)

var verbWeights = [...]float32{
	skill.VerbDamage: 34, skill.VerbPush: 10, skill.VerbDuration: 12,
	skill.VerbCount: 14, skill.VerbRadius: 14, skill.VerbAll: 16,
}

// Drop rolls whether something drops at all, then what.
func (t *Table) Drop(r *RNG, depth int, chance float32) (skill.Part, bool) {
	if r.Float() >= chance || !t.Allow.Any() {
		return skill.Part{}, false
	}
	return t.Roll(r, depth), true
}

// Roll makes one part for a drop at depth. Without quirks allowed no
// exotic rolls.
func (t *Table) Roll(r *RNG, depth int) skill.Part {
	w := RarityWeights(depth)
	if !t.Allow.quirks() {
		w[skill.Exotic] = 0
	}
	rar := skill.Rarity(r.pick(w[:]))
	if t.Allow.New != 0 && r.Float() < NewChance {
		return t.RollNew(r, rar)
	}
	return t.RollAt(r, rar)
}

// RollAt makes one part of a given tier.
func (t *Table) RollAt(r *RNG, rar skill.Rarity) skill.Part {
	kw := kindWeights
	if rar == skill.Exotic {
		kw = exoticKindWeights
	}
	sum := float32(0)
	for i, k := range partKinds {
		if !t.Allow.kind(k) {
			kw[i] = 0
		}
		sum += kw[i]
	}
	if sum == 0 { // nothing taught fits this tier: a plain effect
		kw[0] = 1
	}
	var p skill.Part
	switch partKinds[r.pick(kw[:])] {
	case skill.KindTrigger:
		p = t.trigger(r, rar)
	case skill.KindTarget:
		p = t.target(r)
	case skill.KindEffect:
		p = t.effect(r, rar)
	case skill.KindGate:
		p = t.gate(r)
	case skill.KindCharge:
		p = charge(r)
	case skill.KindCall:
		p = call(r)
	default:
		p = t.mult(r)
	}
	p = p.WithRarity(rar)
	if p.Kind == skill.KindGate && p.Gate == skill.GateSpend {
		p.Count = uint8(max(2, int(p.Count)-int(rar)))
	}
	if t.Allow.quirks() && (rar == skill.Exotic || (rar == skill.Rare && r.Float() < RareQuirkChance)) {
		p.Quirk = quirkFor(p)
	}
	return p
}

// quirkFor is the one quirk that fits a part, or none.
func quirkFor(p skill.Part) skill.Quirk {
	for q := skill.Quirk(1); q < skill.NumQuirks; q++ {
		if q.Fits(p) {
			return q
		}
	}
	return skill.QuirkNone
}

func (t *Table) trigger(r *RNG, rar skill.Rarity) skill.Part {
	w := triggerWeights
	if !t.Allow.quirks() {
		w[skill.Channel], w[skill.Watch] = 0, 0
	}
	sum := float32(0)
	for k := range w {
		if skill.TriggerKind(k).Tier() != rar {
			w[k] = 0
		}
		sum += w[k]
	}
	if sum == 0 { // a tier whose only triggers aren't taught yet
		return skill.NewTrigger(skill.OnCast)
	}
	k := skill.TriggerKind(r.pick(w[:]))
	if k == skill.Periodic {
		return skill.NewPeriodic(half(r.Range(2.5, 5.5)))
	}
	return skill.NewTrigger(k)
}

func (t *Table) target(r *RNG) skill.Part {
	switch k := skill.TargetKind(r.pick(targetWeights[:])); k {
	case skill.TargetNearest:
		return skill.NewTarget(k, 0, uint8(1+r.Intn(3)))
	case skill.TargetCone:
		return skill.NewTarget(k, float32(2+r.Intn(3)), 0)
	case skill.TargetAdjacent:
		return skill.NewTarget(k, float32(1+r.Intn(3)), 0)
	case skill.TargetChain:
		return skill.NewTarget(k, float32(3+r.Intn(2)), uint8(2+r.Intn(3)))
	default:
		return skill.NewTarget(k, 0, 0)
	}
}

func (t *Table) effect(r *RNG, rar skill.Rarity) skill.Part {
	w := effectWeights
	if rar == skill.Exotic {
		w[skill.EffectPush] = 0
	}
	if (rar == skill.Uncommon || rar == skill.Rare) && t.Allow.sustain() {
		for k, v := range sustainWeights {
			w[k] += v
		}
	}
	ty := skill.TypeID(r.Intn(t.Reg.Len()))
	switch skill.EffectKind(r.pick(w[:])) {
	case skill.EffectMend:
		return skill.NewMend(hundredth(r.Range(MendMin, MendMax)))
	case skill.EffectShield:
		return skill.NewShield(hundredth(r.Range(ShieldMin, ShieldMax)))
	case skill.EffectPurge:
		return skill.NewPurge(float32(int(r.Range(PurgeMin, PurgeMax) + 0.5)))
	case skill.EffectPush:
		return skill.NewPush(float32(1 + r.Intn(3)))
	case skill.EffectStatus:
		return skill.NewStatus(ty, half(r.Range(1.5, 4.5)))
	default:
		amt := float32(int(t.Damage[ty]*r.Range(0.8, 1.25) + 0.5))
		return skill.NewDamage(ty, max(1, amt))
	}
}

// gate rolls a condition and its threshold. Status gates skip types whose
// status is a cloak: casting breaks stealth, so it would rarely pass.
func (t *Table) gate(r *RNG) skill.Part {
	gw := gateWeights
	if !t.Allow.spend() {
		gw[skill.GateSpend], gw[skill.GateEmpty] = 0, 0
	}
	switch k := skill.GateKind(r.pick(gw[:])); k {
	case skill.GateHull:
		return skill.NewGate(k, twentieth(r.Range(0.2, 0.5)))
	case skill.GateHeat, skill.GatePower:
		return skill.NewGate(k, twentieth(r.Range(0.35, 0.75)))
	case skill.GateCrowd:
		return skill.NewCrowdGate(float32(1+r.Intn(2)), uint8(2+r.Intn(3)))
	case skill.GateSpend:
		return skill.NewSpendGate(rollBank(r), uint8(SpendMin+r.Intn(SpendMax-SpendMin+1)))
	case skill.GateEmpty:
		return skill.NewEmptyGate(rollBank(r))
	default:
		var ok [skill.MaxTypes]skill.TypeID
		n := 0
		for i := 0; i < t.Reg.Len(); i++ {
			if st := t.Reg.Get(skill.TypeID(i)).Status; st != skill.StatusNone && st != skill.StatusCloak {
				ok[n] = skill.TypeID(i)
				n++
			}
		}
		if n == 0 {
			return skill.NewGate(skill.GateHull, 0)
		}
		return skill.NewStatusGate(ok[r.Intn(n)])
	}
}

// charge rolls an add or a drain of 1.
func charge(r *RNG) skill.Part {
	b := rollBank(r)
	if r.Float() < DrainChance {
		return skill.NewDrain(b, 1)
	}
	return skill.NewCharge(b, 1)
}

// call rolls a Call at its own hex or one of the equipped ones.
func call(r *RNG) skill.Part {
	if r.Float() < CallSelfChance {
		return skill.NewCall(skill.CallSelf)
	}
	return skill.NewCall(uint8(1 + r.Intn(CallTargets)))
}

// rollBank is mostly A, sometimes B. The editor can switch it.
func rollBank(r *RNG) uint8 {
	if r.Float() < 0.3 {
		return 1
	}
	return 0
}

// hundredth rounds to the nearest 1%.
func hundredth(x float32) float32 { return float32(int(x*100+0.5)) / 100 }

// twentieth rounds to the nearest 5%.
func twentieth(x float32) float32 { return float32(int(x*20+0.5)) / 20 }

func (t *Table) mult(r *RNG) skill.Part {
	v := skill.Verb(r.pick(verbWeights[:]))
	lo, hi := float32(1.25), float32(2.05)
	if v == skill.VerbAll {
		lo, hi = 1.15, 1.55
	}
	return skill.NewMult(v, float32(int(r.Range(lo, hi)*20))/20)
}

// half rounds to the nearest half.
func half(x float32) float32 { return float32(int(x*2+0.5)) / 2 }

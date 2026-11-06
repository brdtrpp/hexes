package skill

import "fmt"

// PartKind is what a part does in a hex.
type PartKind uint8

const (
	KindNone PartKind = iota
	KindTrigger
	KindTarget
	KindEffect
	KindMult
	// KindGate tests a condition and splits the chain two ways. See the
	// routing rules on Compile.
	KindGate
	// KindCharge adds to or drains one of the hex's charge banks: counters
	// that keep their value between firings. See Bank.
	KindCharge
	// KindCall runs another equipped hex, or its own hex again, once this
	// one finishes. See the call rules on Compile.
	KindCall
	// KindForm is how a hex shows up in the world: a bolt, a beam, an
	// aura, a nova, a dash, a detonation, or something it leaves behind
	// (a zone, a turret, a trap, a drone). It sits in a target socket and
	// picks who the slots beside it hit, like a target does.
	KindForm
	// KindCost changes what a hex burns when it fires: hull instead of
	// Heat (sacrifice), or wrecks (a rare cost).
	KindCost
	// KindStance makes a hex an always-on mode: casting it toggles it,
	// and while it's on it swaps the rig's stats and pulses its effects.
	KindStance
)

// NumPartKinds is how many part kinds exist, KindNone included.
const NumPartKinds = 11

func (k PartKind) String() string {
	switch k {
	case KindTrigger:
		return "trigger"
	case KindTarget:
		return "target"
	case KindEffect:
		return "effect"
	case KindMult:
		return "mult"
	case KindGate:
		return "gate"
	case KindCharge:
		return "charge"
	case KindCall:
		return "call"
	case KindForm:
		return "form"
	case KindCost:
		return "cost"
	case KindStance:
		return "stance"
	}
	return "none"
}

// TriggerKind is the event that fires a hex.
type TriggerKind uint8

const (
	OnCast      TriggerKind = iota // player presses it
	OnHit                          // caster lands a hit; subject is the foe hit
	OnKill                         // caster kills; subject is the corpse
	FromStealth                    // cast while stealthed
	FromBehind                     // cast at a foe's back
	Periodic                       // fires on its own every Period seconds
	OnHurt                         // caster takes a hit; subject is the attacker
	Channel                        // fires every ChannelEvery seconds while CAST is held
	Watch                          // fires when its condition or library hex comes out true
	OnWreck                        // a foe within WreckRange leaves a wreck; subject is the wreck
	OnDodge                        // a shot passes within a hex of the caster and misses
	OnVent                         // the caster vents; subject is the nearest foe
)

// NumTriggers is how many trigger kinds exist.
const NumTriggers = 12

// WreckRange is how near a wreck has to fall to fire on-wreck hexes.
const WreckRange = 6

var triggerNames = [...]string{"on-cast", "on-hit", "on-kill", "from-stealth", "from-behind", "periodic", "on-hurt", "channel", "watch", "on-wreck", "on-dodge", "on-vent"}

// Tier is how rare a trigger kind is: it drops only at that tier.
func (t TriggerKind) Tier() Rarity {
	switch t {
	case OnCast:
		return Common
	case OnHit, OnKill, OnHurt, OnWreck:
		return Uncommon
	case Watch:
		return Exotic
	}
	return Rare
}

// Channel and watch timing (placeholders to tune in play). A channel hex
// runs every ChannelEvery seconds while held, paying its cost each run. A
// watch checks every WatchEvery seconds, paying WatchHeat each check true
// or not, and rests WatchRest seconds after it fires. A watch with a
// condition that reads foes tests the foes within WatchRange hexes.
const (
	ChannelEvery float32 = 0.25
	WatchEvery   float32 = 0.25
	WatchHeat    float32 = 0.5
	WatchRest    float32 = 1
	WatchRange   float32 = 4
)

func (t TriggerKind) String() string { return triggerNames[t] }

// Stealthy reports whether the trigger counts as a surprise attack, which
// turns on a damage type's StealthMult.
func (t TriggerKind) Stealthy() bool { return t == FromStealth || t == FromBehind }

// TargetKind picks who an effect lands on.
type TargetKind uint8

const (
	TargetSelf     TargetKind = iota // the caster
	TargetNearest                    // nearest Count foes to the origin
	TargetCone                       // foes in a wedge out from the origin, Value hexes long
	TargetAdjacent                   // foes within Value hexes of the origin, origin excluded
	TargetChain                      // hop to the nearest unvisited foe Count times
	TargetWreck                      // the nearest Count wrecks within Value hexes of the origin
	TargetWeakest                    // the Count foes in reach with the least hull left, by share
	TargetMarked                     // up to Count marked foes in reach, nearest first

	// The rest only come from forms (see Form), never as target parts.

	TargetLine   // foes on a line from the origin toward the nearest foe, Value hexes long
	TargetBoth   // a line both ways
	TargetAllies // the caster and its allies (minions, devices) within Value hexes of the origin
	TargetAura   // foes within Value hexes of the caster or any of its allies
)

// NumTargetParts is how many target kinds drop as target parts.
const NumTargetParts = 8

var targetNames = [...]string{"self", "nearest", "cone", "adjacent", "chain", "wreck", "weakest", "marked", "line", "both", "allies", "aura"}

// Counted reports whether the target picks at most Count.
func (t TargetKind) Counted() bool {
	switch t {
	case TargetNearest, TargetChain, TargetWreck, TargetWeakest, TargetMarked:
		return true
	}
	return false
}

// Ranged reports whether the target's Value is a radius or length a
// radius multiplier stretches.
func (t TargetKind) Ranged() bool { return t != TargetSelf && t != TargetNearest }

func (t TargetKind) String() string { return targetNames[t] }

// EffectKind is the verb an effect part performs.
type EffectKind uint8

const (
	EffectDamage EffectKind = iota
	EffectPush
	EffectStatus
	// EffectMend restores hull: Value of the max hull of whoever it lands
	// on, less for each recent Mend (see Unit.Mend). With no target part
	// beside it, it lands on the caster.
	EffectMend
	// EffectPurge takes Value Heat off the caster's rig. It ignores
	// targets and pays for itself in Power, not Heat.
	EffectPurge
	// EffectShield puts a hull buffer worth Value of max hull on whoever
	// it lands on (the caster with no target beside it), soaking hits
	// until it runs out or ShieldTime passes (see Unit.AddShield).
	EffectShield
	// EffectTaunt forces the foes it lands on to come at the caster for
	// Value seconds, dealing TauntCut less while they do.
	EffectTaunt
	// EffectThorns hands back ThornsFrac of every hit the caster takes to
	// whoever dealt it, for Value seconds.
	EffectThorns
	// EffectReanimate raises the wrecks it lands on as the caster's
	// drones for Value seconds. With no target beside it, it finds the
	// nearest wreck.
	EffectReanimate
	// EffectHaste speeds the caster up by HasteFrac for Value seconds.
	EffectHaste
	// EffectBlink jumps the caster Value hexes: next to the target beside
	// it, or away from the nearest foe with none.
	EffectBlink
)

// NumEffects is how many effect kinds exist.
const NumEffects = 11

// Tuning for the newer effects.
const (
	TauntCut   = 0.25 // a taunted foe deals this much less
	ThornsFrac = 0.5  // share of a hit thorns hand back
	HasteFrac  = 0.35 // how much faster haste moves you
	// AmbushMult is how hard the first hit out of stealth lands. Any hit
	// breaks stealth.
	AmbushMult = 2
	// MinionMax is how many raised drones one caster keeps; raising
	// more lets the oldest go.
	MinionMax = 4
	// MinionLife is how long a drone raised by a kill lasts.
	MinionLife = 10
	// RiderTime is the status a Rider damage effect leaves; RiderHit is
	// the damage a Rider status effect deals.
	RiderTime = 1.5
	RiderHit  = 6
	RiderCost = 0.3 // a rider costs this much more
)

var effectNames = [...]string{"damage", "push", "status", "mend", "purge", "shield", "taunt", "thorns", "reanimate", "haste", "blink"}

func (e EffectKind) String() string { return effectNames[e] }

// Sustain reports whether the effect keeps you going (mend, purge,
// shield) rather than acting on foes.
func (e EffectKind) Sustain() bool { return e == EffectMend || e == EffectPurge || e == EffectShield }

// OnSelf reports whether the effect only ever lands on the caster
// (thorns, haste), whoever sits beside it.
func (e EffectKind) OnSelf() bool { return e == EffectThorns || e == EffectHaste }

// Typed reports whether the effect carries a damage type: damage and
// status effects do; push and the sustain effects don't.
func (e EffectKind) Typed() bool { return e == EffectDamage || e == EffectStatus }

// Verb is the multiplier that scales the effect's number. Damage
// multipliers scale the sustain effects' amounts too.
func (e EffectKind) Verb() Verb {
	switch e {
	case EffectPush:
		return VerbPush
	case EffectStatus, EffectTaunt, EffectThorns, EffectReanimate, EffectHaste:
		return VerbDuration
	case EffectBlink:
		return VerbPush
	}
	return VerbDamage
}

// Verb is the number a multiplier scales.
type Verb uint8

const (
	VerbDamage   Verb = iota // damage amounts
	VerbPush                 // push distance
	VerbDuration             // status duration
	VerbCount                // target count and chain hops
	VerbRadius               // target radius and cone length
	VerbAll                  // every number above
	VerbMarked               // damage, but only on marked foes
)

var verbNames = [...]string{"damage", "push", "duration", "count", "radius", "all", "marked"}

func (v Verb) String() string { return verbNames[v] }

// GateKind is the condition a Gate part tests.
type GateKind uint8

const (
	// GateHull passes foes whose hull is below Value of its max (0.3 is
	// "under 30%").
	GateHull GateKind = iota
	// GateHeat passes everyone when the caster's Heat is above Value of
	// max, and no one otherwise.
	GateHeat
	// GatePower passes everyone when the caster's Power is above Value of
	// max, and no one otherwise.
	GatePower
	// GateStatus passes foes suffering Type's status: burning or eaten by
	// toxin of that type, chilled or frozen, staggered, marked. A cloak
	// type tests the caster instead: everyone passes while stealthed.
	GateStatus
	// GateCrowd passes foes with at least Count other foes within Value
	// hexes of them.
	GateCrowd
	// GateSpend reads charge bank Bank: if it holds at least Count and the
	// group isn't empty, it spends Count from the bank and the whole group
	// goes yes; otherwise the bank is untouched and the group goes no.
	// With Count 1 this is a counter machine's decrement-or-branch-on-zero.
	GateSpend
	// GateEmpty reads charge bank Bank: the whole group goes yes when the
	// bank is zero, no otherwise. It never changes the bank.
	GateEmpty
	// GateCond tests the condition the player built in Cond (see
	// cond.go). The fixed kinds above are its presets.
	GateCond
	// GateBehind passes foes facing away from the caster.
	GateBehind
	// GateHurt passes everyone while the caster's own hull is under Value
	// of its max (0.5 is "under half"), and no one otherwise.
	GateHurt
)

// NumGates is how many gate conditions exist.
const NumGates = 10

var gateNames = [...]string{"hull", "heat", "power", "status", "crowd", "spend", "empty", "condition", "behind", "hurt"}

func (g GateKind) String() string { return gateNames[g] }

// PerFoe reports whether the gate tests each foe on its own. The others
// test the caster's rig, so the whole group goes one way.
func (g GateKind) PerFoe() bool {
	return g == GateHull || g == GateStatus || g == GateCrowd || g == GateBehind
}

// Charged reports whether the gate reads a charge bank.
func (g GateKind) Charged() bool { return g == GateSpend || g == GateEmpty }

// ChargeKind is what a Charge part does to its bank.
type ChargeKind uint8

const (
	// ChargeAdd adds Value to the bank each time the slot runs.
	ChargeAdd ChargeKind = iota
	// ChargeDrain takes Value off the bank each time the slot runs,
	// stopping at zero.
	ChargeDrain
)

var chargeNames = [...]string{"add", "drain"}

func (c ChargeKind) String() string { return chargeNames[c] }

// MaxBanks is how many charge banks (A, B, C, D) one equipped hex has.
// Every Charge part and charge gate names one; parts naming the same
// letter share it, nested hexes included.
const MaxBanks = 4

// BoardBanks is how many shared board banks (X, Y) there are. Bank
// numbers MaxBanks and up name them: any hex can charge, spend and read
// them. NumBanks is every bank a part can name.
const (
	BoardBanks = 2
	NumBanks   = MaxBanks + BoardBanks
)

// BoardBank is the board's shared banks, X and Y.
type BoardBank [BoardBanks]uint32

// BankName is a bank's letter.
func BankName(b uint8) string {
	switch {
	case b < MaxBanks:
		return string(rune('A' + b))
	case b < NumBanks:
		return string(rune('X' + b - MaxBanks))
	}
	return "?"
}

// Charge defaults.
const (
	ChargePowerCost   = 0.5
	DefaultSpendEvery = 5
)

// CallSelf is the Call value that runs the calling hex again. Any other
// value n runs equipped hex n (1 is the first).
const CallSelf = 0

// Call defaults. A Call costs CallPowerCost and CallHeatCost to carry,
// paid with the rest of the hex. Each time it actually runs a hex, it adds
// that hex's Heat on top (at least CallMinHeat): Heat is what stops a
// loop.
const (
	CallPowerCost = 1
	CallHeatCost  = 0.5
	CallMinHeat   = 1
)

// CallName is how a Call's target reads: "self" or "hex 2".
func CallName(c uint8) string {
	if c == CallSelf {
		return "self"
	}
	return fmt.Sprintf("hex %d", c)
}

// Gate defaults.
const (
	DefaultHullGate  = 0.3
	DefaultHeatGate  = 0.5
	DefaultPowerGate = 0.5
	DefaultCrowd     = 3
	GatePowerCost    = 1
)

// Rarity is a loot tier. It scales a part's numbers when compiled.
type Rarity uint8

const (
	Common Rarity = iota
	Uncommon
	Rare
	Exotic
)

// NumRarities is how many loot tiers exist.
const NumRarities = 4

var rarityScale = [...]float32{1, 1.15, 1.35, 1.6}

// Scale is how much the tier boosts a part's base value (or a
// multiplier's bonus above 1).
func (r Rarity) Scale() float32 { return rarityScale[r] }

func (r Rarity) String() string {
	return [...]string{"common", "uncommon", "rare", "exotic"}[r]
}

// Quirk is a rule a rare part bends. Quirks change how the compiler wires
// the hex, not just how big the numbers are.
type Quirk uint8

const (
	QuirkNone Quirk = iota
	// QuirkRadiant (multipliers): scales every part in its ring, not just
	// the one before it. Nested hexes in the ring are scaled too.
	QuirkRadiant
	// QuirkRelay (damage and status effects): after landing, arcs once to
	// everything next to each target, damage at RelayFrac, statuses at full
	// duration. A target's target without spending a nesting level.
	QuirkRelay
	// QuirkEcho (triggers): the whole hex runs twice per firing. The second
	// run picks its targets again, so it finds whoever the first one missed.
	QuirkEcho
	// QuirkRider (damage and status effects): a second effect rides
	// along. Damage also leaves its type's status for RiderTime; a status
	// also deals RiderHit damage of its type.
	QuirkRider
	// QuirkHeated (multipliers): the part it scales grows with Heat, the
	// way an Overflow frame does.
	QuirkHeated
)

// NumQuirks counts quirks, QuirkNone included.
const NumQuirks = 6

var quirkNames = [...]string{"", "radiant", "relay", "echo", "rider", "heated"}

func (q Quirk) String() string { return quirkNames[q] }

// Fits reports whether a part kind (and effect verb) can carry the quirk.
func (q Quirk) Fits(p Part) bool {
	switch q {
	case QuirkNone:
		return true
	case QuirkRadiant:
		return p.Kind == KindMult
	case QuirkRelay:
		return p.Kind == KindEffect && p.Effect.Typed()
	case QuirkEcho:
		return p.Kind == KindTrigger
	case QuirkRider:
		return p.Kind == KindEffect && p.Effect.Typed()
	case QuirkHeated:
		return p.Kind == KindMult
	}
	return false
}

// Quirk tuning.
const (
	RelayFrac       = 0.5  // relay arcs land at this share of the value
	RelayCost       = 0.5  // a relay effect costs this much more Power and Heat
	EchoCost        = 0.25 // an echo hex costs this much more in total
	RadiantHeatCost = 2    // extra Heat for a radiant multiplier
)

// Part is one piece of loot that goes in a hex slot. Which fields matter
// depends on Kind.
type Part struct {
	Name    string
	Kind    PartKind
	Rarity  Rarity
	Trigger TriggerKind
	Target  TargetKind
	Effect  EffectKind
	Type    TypeID // damage and status effects
	Verb    Verb   // multipliers
	Gate    GateKind
	Charge  ChargeKind // charge parts
	// Bank is the charge bank a Charge part or charge gate uses (0 is A).
	Bank uint8
	// Call is the hex a Call part runs: CallSelf, or n for equipped hex n.
	Call uint8
	// Value means: trigger damage multiplier, target radius, damage amount,
	// push distance in hexes, status duration in seconds, multiplier factor,
	// a gate's threshold (a fraction of max, or the crowd radius), or how
	// much a Charge part adds or drains.
	Value float32
	// Count is the max targets for Nearest, the hop count for Chain, the
	// foes a crowd gate needs and the charge a spend gate takes.
	Count uint8
	// Power is spent and Heat added each time the hex fires.
	Power, Heat float32
	// Period is seconds between firings for a Periodic trigger.
	Period float32
	// Quirk is the rule this part bends, if any. Only rare and exotic loot
	// rolls one.
	Quirk Quirk
	// Lib, on a Call part, names a library hex (a module on the board) to
	// run right where the call sits, instead of queueing an equipped hex
	// for later. Empty means the run-later Call.
	Lib string `json:",omitempty"`
	// Opts are the options riding on a run-now Call: multipliers that
	// scale the called hex's numbers, the way a mult after a nested hex
	// does. They use no shape slot.
	Opts Opts `json:",omitzero"`
	// Cond is a condition gate's tiles (GateCond).
	Cond Cond `json:",omitzero"`
	// Form, Cost and Stance are what a form, cost or stance part is.
	Form   FormKind   `json:",omitempty"`
	Cost   CostKind   `json:",omitempty"`
	Stance StanceKind `json:",omitempty"`
}

// Opt is one multiplier riding on a call as an option. It keeps what it
// needs to go back to the pack as the same part.
type Opt struct {
	Verb        Verb
	Rarity      Rarity
	Quirk       Quirk `json:",omitempty"`
	Value       float32
	Power, Heat float32
}

// Opts is a call's options, filled from the front.
type Opts [MaxOpts]Opt

// Len is how many options are on.
func (o Opts) Len() int {
	n := 0
	for _, x := range o {
		if x.Value != 0 {
			n++
		}
	}
	return n
}

// OptOf turns a multiplier into an option.
func OptOf(m Part) Opt {
	return Opt{Verb: m.Verb, Rarity: m.Rarity, Quirk: m.Quirk, Value: m.Value, Power: m.Power, Heat: m.Heat}
}

// Part is the option as the multiplier it came from.
func (o Opt) Part() Part {
	return Part{Kind: KindMult, Verb: o.Verb, Rarity: o.Rarity, Quirk: o.Quirk, Value: o.Value, Power: o.Power, Heat: o.Heat}
}

// WithOpt returns the call with multiplier m added as an option, and
// false if it isn't a run-now call, m isn't a multiplier, or it's full.
func (p Part) WithOpt(m Part) (Part, bool) {
	if !p.RunsNow() || m.Kind != KindMult {
		return p, false
	}
	for i := range p.Opts {
		if p.Opts[i].Value == 0 {
			p.Opts[i] = OptOf(m)
			return p, true
		}
	}
	return p, false
}

// WithoutOpt returns the call with option i taken off, the rest moved up,
// and the multiplier it was.
func (p Part) WithoutOpt(i int) (Part, Part, bool) {
	if i < 0 || i >= MaxOpts || p.Opts[i].Value == 0 {
		return p, Part{}, false
	}
	m := p.Opts[i].Part()
	copy(p.Opts[i:], p.Opts[i+1:])
	p.Opts[MaxOpts-1] = Opt{}
	return p, m, true
}

func (p Part) String() string {
	if p.Name != "" {
		return p.Name
	}
	switch p.Kind {
	case KindTrigger:
		return p.Trigger.String()
	case KindTarget:
		return p.Target.String()
	case KindEffect:
		return p.Effect.String()
	case KindMult:
		return fmt.Sprintf("x%.2g %s", p.Value, p.Verb)
	case KindGate:
		if p.Gate.Charged() {
			return "gate " + p.Gate.String() + " " + BankName(p.Bank)
		}
		return "gate " + p.Gate.String()
	case KindCharge:
		return fmt.Sprintf("%s %s %g", p.Charge, BankName(p.Bank), p.Value)
	case KindCall:
		return "call " + CallName(p.Call)
	case KindForm:
		return p.Form.String()
	case KindCost:
		return "cost " + p.Cost.String()
	case KindStance:
		return "stance " + p.Stance.String()
	}
	return "empty"
}

// The constructors below fill in default costs. Loot generation can
// override any field afterward.

// NewTrigger makes a trigger part. Surprise triggers carry a damage bonus.
func NewTrigger(k TriggerKind) Part {
	mult := float32(1)
	switch k {
	case FromStealth:
		mult = 2
	case FromBehind:
		mult = 1.5
	}
	p := Part{Kind: KindTrigger, Trigger: k, Value: mult, Power: 2}
	if k == Periodic {
		p.Period = DefaultPeriod
	}
	return p
}

// DefaultPeriod is how often a Periodic trigger fires unless loot says
// otherwise.
const DefaultPeriod = 3

// NewPeriodic makes a trigger that fires every given number of seconds.
func NewPeriodic(seconds float32) Part {
	p := NewTrigger(Periodic)
	p.Period = seconds
	return p
}

// NewTarget makes a target part. Radius and count fall back to sensible
// defaults when zero.
func NewTarget(k TargetKind, radius float32, count uint8) Part {
	p := Part{Kind: KindTarget, Target: k, Value: radius, Count: count}
	switch k {
	case TargetSelf:
		p.Power = 0
	case TargetNearest:
		p.Power = 1
		if p.Count == 0 {
			p.Count = 1
		}
	case TargetCone:
		p.Power = 2
		if p.Value == 0 {
			p.Value = 3
		}
	case TargetAdjacent:
		p.Power = 2
		if p.Value == 0 {
			p.Value = 1
		}
	case TargetChain:
		p.Power = 3
		if p.Count == 0 {
			p.Count = 3
		}
		if p.Value == 0 {
			p.Value = 3
		}
	}
	return p
}

// NewDamage makes a damage effect.
func NewDamage(t TypeID, amount float32) Part {
	return Part{Kind: KindEffect, Effect: EffectDamage, Type: t, Value: amount,
		Power: amount * 0.2, Heat: amount * 0.1}
}

// NewPush makes a push effect that shoves targets away from the caster.
func NewPush(hexes float32) Part {
	return Part{Kind: KindEffect, Effect: EffectPush, Value: hexes, Power: 2 * hexes, Heat: 1}
}

// NewStatus applies a damage type's status for a duration in seconds.
func NewStatus(t TypeID, seconds float32) Part {
	return Part{Kind: KindEffect, Effect: EffectStatus, Type: t, Value: seconds,
		Power: 2 + seconds, Heat: 1}
}

// Sustain tuning. A Mend or Shield amount is a share of the max hull of
// whoever it lands on; a Purge amount is Heat.
const (
	DefaultMend   = 0.10
	DefaultShield = 0.15
	DefaultPurge  = 15
	// MendCap and ShieldCap: one Mend never restores more than this share
	// of max hull, and a shield never holds more than this share.
	MendCap   = 0.5
	ShieldCap = 0.6
	// ShieldTime is how long a shield lasts after its last top-up.
	ShieldTime = 4
	// StrainHalfLife is how fast strain fades. Each Mend (or Shield) that
	// lands on a unit adds 1 to its mend (or shield) strain, strain halves
	// every StrainHalfLife seconds, and an amount is divided by 1 + strain.
	// However fast one fires, it tops out near 70% of its amount a second.
	StrainHalfLife = 1
)

// NewMend makes a Mend: restores frac of max hull (zero means
// DefaultMend).
func NewMend(frac float32) Part {
	if frac <= 0 {
		frac = DefaultMend
	}
	return Part{Kind: KindEffect, Effect: EffectMend, Value: frac, Power: 4 + 40*frac, Heat: 1 + 20*frac}
}

// NewShield makes a Shield: a buffer of frac of max hull (zero means
// DefaultShield).
func NewShield(frac float32) Part {
	if frac <= 0 {
		frac = DefaultShield
	}
	return Part{Kind: KindEffect, Effect: EffectShield, Value: frac, Power: 3 + 30*frac, Heat: 1 + 12*frac}
}

// NewPurge makes a Purge: takes heat off the rig (zero means
// DefaultPurge). It costs a Power for every Heat it removes, plus one,
// and adds no Heat.
func NewPurge(heat float32) Part {
	if heat <= 0 {
		heat = DefaultPurge
	}
	return Part{Kind: KindEffect, Effect: EffectPurge, Value: heat, Power: 1 + heat}
}

// NewMult makes a multiplier. Bigger factors run hotter.
func NewMult(v Verb, factor float32) Part {
	return Part{Kind: KindMult, Verb: v, Value: factor, Power: 3, Heat: 1 + 4*(factor-1)}
}

// NewGate makes a gate. A zero value takes the default threshold: hull
// under 30%, Heat or Power over half, three foes within one hex.
func NewGate(k GateKind, value float32) Part {
	p := Part{Kind: KindGate, Gate: k, Value: value, Power: GatePowerCost}
	if k == GateCrowd {
		p.Count = DefaultCrowd
	}
	if p.Value > 0 {
		return p
	}
	switch k {
	case GateHull:
		p.Value = DefaultHullGate
	case GateHeat:
		p.Value = DefaultHeatGate
	case GatePower:
		p.Value = DefaultPowerGate
	case GateCrowd:
		p.Value = 1
	}
	return p
}

// NewStatusGate passes foes suffering a damage type's status.
func NewStatusGate(t TypeID) Part {
	return Part{Kind: KindGate, Gate: GateStatus, Type: t, Power: GatePowerCost}
}

// NewCrowdGate passes foes with at least count others within radius hexes.
func NewCrowdGate(radius float32, count uint8) Part {
	p := NewGate(GateCrowd, radius)
	if count > 0 {
		p.Count = count
	}
	return p
}

// NewCharge adds amount to a bank every time its slot runs. Zero means 1.
func NewCharge(bank uint8, amount float32) Part {
	if amount <= 0 {
		amount = 1
	}
	return Part{Kind: KindCharge, Charge: ChargeAdd, Bank: bank % NumBanks, Value: amount, Power: ChargePowerCost}
}

// NewDrain takes amount off a bank every time its slot runs, never below
// zero. Zero means 1.
func NewDrain(bank uint8, amount float32) Part {
	p := NewCharge(bank, amount)
	p.Charge = ChargeDrain
	return p
}

// NewSpendGate passes its group (and spends n from the bank) once the
// bank holds n. Zero means DefaultSpendEvery. With a Charge part adding 1
// on every kill, it fires the yes slot on every nth kill.
func NewSpendGate(bank uint8, n uint8) Part {
	if n == 0 {
		n = DefaultSpendEvery
	}
	return Part{Kind: KindGate, Gate: GateSpend, Bank: bank % NumBanks, Count: n, Power: GatePowerCost}
}

// NewEmptyGate passes its group when the bank is zero.
func NewEmptyGate(bank uint8) Part {
	return Part{Kind: KindGate, Gate: GateEmpty, Bank: bank % NumBanks, Power: GatePowerCost}
}

// NewCall makes a Call part that runs hex target (CallSelf, or n for
// equipped hex n) once the hex holding it finishes.
func NewCall(target uint8) Part {
	return Part{Kind: KindCall, Call: target, Power: CallPowerCost, Heat: CallHeatCost}
}

// NewLibCall makes a run-now Call part that runs the library hex named
// lib inline, on the group its slot lands on.
func NewLibCall(lib string) Part {
	p := NewCall(CallSelf)
	p.Lib = lib
	return p
}

// MaxOpts is the most options one call carries.
const MaxOpts = 3

// RunsNow reports whether the part is a run-now Call to a library hex.
func (p Part) RunsNow() bool { return p.Kind == KindCall && p.Lib != "" }

// Watches reports whether the part is a watch trigger on a library hex.
func (p Part) Watches() bool { return p.Kind == KindTrigger && p.Trigger == Watch && p.Lib != "" }

// NeedsLib reports whether the part names a library hex to link.
func (p Part) NeedsLib() bool { return p.RunsNow() || p.Watches() }

// WithBank returns the part reading a different bank. Only Charge parts
// and charge gates have one; anything else comes back unchanged.
func (p Part) WithBank(b uint8) Part {
	if p.Kind == KindCharge || (p.Kind == KindGate && p.Gate.Charged()) {
		p.Bank = b % NumBanks
	}
	return p
}

// UsesBank reports whether the part reads or writes a charge bank.
func (p Part) UsesBank() bool {
	return p.Kind == KindCharge || (p.Kind == KindGate && p.Gate.Charged())
}

// WithRarity returns the part at a different tier.
func (p Part) WithRarity(r Rarity) Part { p.Rarity = r; return p }

// FormKind is how a hex shows up in the world. The first six act at
// once, picking who the slots beside them hit; the rest are placed
// (Placed) and keep running the hex from where they are.
type FormKind uint8

const (
	// FormBolt hits the nearest foe. Rare: it pierces everyone on the
	// line. Exotic: it pierces and splits onto the foes beside the first.
	FormBolt FormKind = iota
	// FormBeam hits every foe on a short line toward the nearest. Rare:
	// both ways. Exotic: it fans into a wedge.
	FormBeam
	// FormAura hits every foe close around you, wherever the hex fires
	// from. Rare: around your allies too. Exotic: and every marked foe.
	// With Mend or Shield beside it, it lands on you and your allies.
	FormAura
	// FormNova bursts around where the hex fires from. Rare: around you
	// too. Exotic: wider.
	FormNova
	// FormDash carries you along a line to the nearest foe, hitting
	// everyone on it. Rare: you dash back after. Exotic: back, unseen.
	FormDash
	// FormDetonate blows up the nearest wreck, hitting every foe around
	// it, and uses it up. Rare: it sets off the next wreck too. Exotic:
	// both, wider.
	FormDetonate
	// FormZone leaves a field on the nearest foe that runs the hex
	// around itself every ZoneEvery for ZoneLife, at ZoneScale. Rare:
	// it drifts after foes. Exotic: it drifts and chills.
	FormZone
	// FormTurret places a turret beside you that runs the hex at the
	// nearest foe every TurretEvery for TurretLife. A turret carrying an
	// aura is a totem. Rare: two can stand. Exotic: three.
	FormTurret
	// FormTrap leaves a trap where you stand that runs the hex on what
	// steps on it. Rare: it rearms twice. Exotic: and it stuns.
	FormTrap
	// FormDrone launches a drone that follows you and runs the hex at
	// the nearest foe every DroneEvery for DroneLife. Rare: two can fly.
	// Exotic: it bursts the hex once more when it ends.
	FormDrone
)

// NumForms is how many forms exist.
const NumForms = 10

var formNames = [...]string{"bolt", "beam", "aura", "nova", "dash", "detonate", "zone", "turret", "trap", "drone"}

func (f FormKind) String() string { return formNames[f] }

// Placed reports whether the form leaves something behind that keeps
// running the hex.
func (f FormKind) Placed() bool { return f >= FormZone }

// Form and device tuning (placeholders to tune in play).
const (
	BoltReach   = 9   // a piercing bolt's line
	BeamLength  = 5   // a beam's line
	AuraRadius  = 2   // round you
	NovaRadius  = 2   // round where it fires
	DashLength  = 4   // hexes a dash carries you
	BlastRadius = 2   // round a detonated wreck
	WreckReach  = 6   // how far a form or reanimate looks for a wreck
	ZoneRadius  = 1.5 // a zone's field
	ZoneEvery   = 0.5
	ZoneLife    = 6
	ZoneScale   = 0.35 // a zone's numbers, it runs often
	TurretEvery = 1
	TurretLife  = 12
	TrapLife    = 25
	TrapRadius  = 1
	DroneEvery  = 1.2
	DroneLife   = 14
	FormPower   = 1 // a form's Power beyond what it replaces
	DevicePower = 4 // a placed form's Power
	DeviceHeat  = 2
)

// Rule is what a form part's tier adds: 0 below rare, 1 rare, 2 exotic.
func (p Part) Rule() uint8 {
	switch {
	case p.Rarity >= Exotic:
		return 2
	case p.Rarity >= Rare:
		return 1
	}
	return 0
}

// NewForm makes a form part.
func NewForm(f FormKind) Part {
	p := Part{Kind: KindForm, Form: f, Power: FormPower}
	switch f {
	case FormBolt:
	case FormBeam, FormNova:
		p.Power, p.Heat = 2, 1
	case FormAura:
		p.Power, p.Heat = 2, 1
	case FormDash:
		p.Power, p.Heat = 3, 2
	case FormDetonate:
		p.Power, p.Heat = 2, 1
	default:
		p.Power, p.Heat = DevicePower, DeviceHeat
	}
	return p
}

// CostKind is what a cost part makes a hex burn instead of Heat.
type CostKind uint8

const (
	// CostBlood (sacrifice): the hex adds no Heat and takes BloodPerHeat
	// of your max hull for each Heat it would have added. It won't fire
	// on the last of your hull.
	CostBlood CostKind = iota
	// CostWrecks (rare): the hex adds no Heat and uses up one wreck near
	// you each firing; with none in reach it doesn't fire.
	CostWrecks
)

var costNames = [...]string{"blood", "wrecks"}

func (c CostKind) String() string { return costNames[c] }

// Cost tuning.
const (
	BloodPerHeat = 0.004 // hull share per Heat a sacrifice pays
	WreckCostN   = 1
	CostPower    = 1
)

// NewCost makes a cost part.
func NewCost(c CostKind) Part {
	return Part{Kind: KindCost, Cost: c, Power: CostPower}
}

// StanceKind is the mode a stance hex holds you in.
type StanceKind uint8

const (
	// StanceBulwark: take BulwarkGuard less damage, move BulwarkSlow
	// slower.
	StanceBulwark StanceKind = iota
	// StanceHunter: deal HunterMight more, move HunterSpeed faster, take
	// HunterFrail more.
	StanceHunter
	// StanceFurnace: Heat cools FurnaceCool times as fast, Power comes
	// back FurnaceCharge as fast.
	StanceFurnace
)

// NumStances is how many stances exist.
const NumStances = 3

var stanceNames = [...]string{"bulwark", "hunter", "furnace"}

func (s StanceKind) String() string { return stanceNames[s] }

// Stance tuning.
const (
	StancePulse   = 2 // seconds between a stance hex's pulses while it's on
	BulwarkGuard  = 0.35
	BulwarkSlow   = 0.3
	HunterMight   = 0.25
	HunterSpeed   = 0.2
	HunterFrail   = 0.2
	FurnaceCool   = 2
	FurnaceCharge = 0.5
	StancePower   = 2
)

// NewStance makes a stance part.
func NewStance(k StanceKind) Part {
	return Part{Kind: KindStance, Stance: k, Power: StancePower}
}

// Constructors for the newer effects. Zero takes the default.

// NewTaunt makes a Taunt that holds foes on you for seconds.
func NewTaunt(seconds float32) Part {
	if seconds <= 0 {
		seconds = 3
	}
	return Part{Kind: KindEffect, Effect: EffectTaunt, Value: seconds, Power: 3, Heat: 1}
}

// NewThorns makes Thorns that last seconds.
func NewThorns(seconds float32) Part {
	if seconds <= 0 {
		seconds = 4
	}
	return Part{Kind: KindEffect, Effect: EffectThorns, Value: seconds, Power: 4, Heat: 2}
}

// NewReanimate makes a Reanimate whose drones last seconds.
func NewReanimate(seconds float32) Part {
	if seconds <= 0 {
		seconds = 12
	}
	return Part{Kind: KindEffect, Effect: EffectReanimate, Value: seconds, Power: 5, Heat: 2}
}

// NewHaste makes a Haste that lasts seconds.
func NewHaste(seconds float32) Part {
	if seconds <= 0 {
		seconds = 3
	}
	return Part{Kind: KindEffect, Effect: EffectHaste, Value: seconds, Power: 3, Heat: 1}
}

// NewBlink makes a Blink of hexes.
func NewBlink(hexes float32) Part {
	if hexes <= 0 {
		hexes = 3
	}
	return Part{Kind: KindEffect, Effect: EffectBlink, Value: hexes, Power: 3, Heat: 2}
}

// NewMarkedMult makes the "x2 on marked" multiplier.
func NewMarkedMult(factor float32) Part {
	if factor <= 0 {
		factor = 2
	}
	return NewMult(VerbMarked, factor)
}

// NewBehindGate passes foes facing away from you.
func NewBehindGate() Part {
	return Part{Kind: KindGate, Gate: GateBehind, Power: GatePowerCost}
}

// NewHurtGate fires while your own hull is under frac (zero: half).
func NewHurtGate(frac float32) Part {
	if frac <= 0 {
		frac = 0.5
	}
	return Part{Kind: KindGate, Gate: GateHurt, Value: frac, Power: GatePowerCost}
}

// ComboBank is the bank a combo counts on: the hex's own A, or the
// board's X for a rare combo, so it carries between hexes.
func ComboBank(r Rarity) uint8 {
	if r >= Rare {
		return MaxBanks // X
	}
	return 0
}

// ComboHits is how many points a combo finisher spends.
const ComboHits = 3

// NewCombo is a combo builder: a Charge that adds a point each time its
// slot runs, named so it reads as one.
func NewCombo(r Rarity) Part {
	p := NewCharge(ComboBank(r), 1).WithRarity(r)
	p.Name = "combo"
	return p
}

// NewFinisher is the combo's finisher: a spend gate that pays out once
// the combo holds ComboHits.
func NewFinisher(r Rarity) Part {
	p := NewSpendGate(ComboBank(r), ComboHits).WithRarity(r)
	p.Name = "finisher"
	return p
}

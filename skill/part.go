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
)

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
)

// NumTriggers is how many trigger kinds exist.
const NumTriggers = 9

var triggerNames = [...]string{"on-cast", "on-hit", "on-kill", "from-stealth", "from-behind", "periodic", "on-hurt", "channel", "watch"}

// Tier is how rare a trigger kind is: it drops only at that tier.
func (t TriggerKind) Tier() Rarity {
	switch t {
	case OnCast:
		return Common
	case OnHit, OnKill, OnHurt:
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
)

var targetNames = [...]string{"self", "nearest", "cone", "adjacent", "chain"}

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
)

// NumEffects is how many effect kinds exist.
const NumEffects = 6

var effectNames = [...]string{"damage", "push", "status", "mend", "purge", "shield"}

func (e EffectKind) String() string { return effectNames[e] }

// Sustain reports whether the effect keeps you going (mend, purge,
// shield) rather than acting on foes.
func (e EffectKind) Sustain() bool { return e == EffectMend || e == EffectPurge || e == EffectShield }

// Typed reports whether the effect carries a damage type: damage and
// status effects do; push and the sustain effects don't.
func (e EffectKind) Typed() bool { return e == EffectDamage || e == EffectStatus }

// Verb is the multiplier that scales the effect's number. Damage
// multipliers scale the sustain effects' amounts too.
func (e EffectKind) Verb() Verb {
	switch e {
	case EffectPush:
		return VerbPush
	case EffectStatus:
		return VerbDuration
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
)

var verbNames = [...]string{"damage", "push", "duration", "count", "radius", "all"}

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
)

// NumGates is how many gate conditions exist.
const NumGates = 8

var gateNames = [...]string{"hull", "heat", "power", "status", "crowd", "spend", "empty", "condition"}

func (g GateKind) String() string { return gateNames[g] }

// PerFoe reports whether the gate tests each foe on its own. The others
// test the caster's rig, so the whole group goes one way.
func (g GateKind) PerFoe() bool { return g == GateHull || g == GateStatus || g == GateCrowd }

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
)

// NumQuirks counts quirks, QuirkNone included.
const NumQuirks = 4

var quirkNames = [...]string{"", "radiant", "relay", "echo"}

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

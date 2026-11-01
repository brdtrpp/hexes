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
)

// NumTriggers is how many trigger kinds exist.
const NumTriggers = 7

var triggerNames = [...]string{"on-cast", "on-hit", "on-kill", "from-stealth", "from-behind", "periodic", "on-hurt"}

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
)

var effectNames = [...]string{"damage", "push", "status"}

func (e EffectKind) String() string { return effectNames[e] }

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

// Rarity is a loot tier. It scales a part's numbers when compiled.
type Rarity uint8

const (
	Common Rarity = iota
	Uncommon
	Rare
	Epic
)

var rarityScale = [...]float32{1, 1.15, 1.35, 1.6}

// Scale is how much the tier boosts a part's base value (or a
// multiplier's bonus above 1).
func (r Rarity) Scale() float32 { return rarityScale[r] }

func (r Rarity) String() string {
	return [...]string{"common", "uncommon", "rare", "epic"}[r]
}

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
	// Value means: trigger damage multiplier, target radius, damage amount,
	// push distance in hexes, status duration in seconds, or multiplier factor.
	Value float32
	// Count is the max targets for Nearest and the hop count for Chain.
	Count uint8
	// Power is spent and Heat added each time the hex fires.
	Power, Heat float32
	// Period is seconds between firings for a Periodic trigger.
	Period float32
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

// NewMult makes a multiplier. Bigger factors run hotter.
func NewMult(v Verb, factor float32) Part {
	return Part{Kind: KindMult, Verb: v, Value: factor, Power: 3, Heat: 1 + 4*(factor-1)}
}

// WithRarity returns the part at a different tier.
func (p Part) WithRarity(r Rarity) Part { p.Rarity = r; return p }

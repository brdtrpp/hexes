package skill

import "math"

// EntityID names a unit in the world.
type EntityID int32

// NoEntity is the empty ID.
const NoEntity EntityID = -1

// MaxDots is how many damage-over-time effects a unit carries at once.
const MaxDots = 4

// Dot is one damage-over-time effect.
type Dot struct {
	PerTick       float32
	Left          uint8 // ticks remaining
	Type          TypeID
	AttackerLevel int16
	Owner         EntityID
}

// Unit is everything combat needs to know about a creature.
type Unit struct {
	ID     EntityID
	Team   uint8
	Alive  bool
	HP     float32
	MaxHP  float32
	Level  int
	Resist [MaxTypes]float32 // adaptive, 0 to Rules.MaxResist

	Chill     uint8
	Frozen    bool
	FrozenBy  TypeID
	SlowFrac  float32
	SlowLeft  float32
	StunLeft  float32
	MarkLeft  float32
	MarkOwner EntityID
	Stealth   float32 // seconds of stealth left
	// StealthRest is seconds before stealth can be taken again, from when
	// it last ended: a cloak can't be kept up back to back.
	StealthRest float32
	// Hidden is set while a cast that started out of stealth runs, for
	// the cloak gate (Ambush is the hit bonus, which only damage reads).
	Hidden bool
	Dots   [MaxDots]Dot

	// Shield is hull buffer that soaks hits before hull does; it lasts
	// ShieldLeft more seconds. MendStrain and ShieldStrain are how many
	// recent Mends and Shields landed (fading, see StrainHalfLife).
	Shield       float32
	ShieldLeft   float32
	MendStrain   float32
	ShieldStrain float32

	// TauntLeft is seconds this foe is held on TauntBy (see EffectTaunt).
	TauntLeft float32
	TauntBy   EntityID
	// ThornsLeft and HasteLeft are seconds of thorns and haste left.
	ThornsLeft float32
	HasteLeft  float32
	// Might scales the damage this unit's hexes deal (zero is 1), for
	// stances. Ambush is set while a cast or strike out of stealth runs:
	// its damage lands at AmbushMult.
	Might  float32
	Ambush bool
}

// Mend restores share of max hull, divided by 1 + mend strain, then adds
// 1 strain. It returns the hull restored. Dead units can't be mended.
func (u *Unit) Mend(share float32) float32 {
	if !u.Alive || share <= 0 {
		return 0
	}
	amt := min(share, MendCap) * u.MaxHP / (1 + u.MendStrain)
	u.MendStrain++
	amt = min(amt, u.MaxHP-u.HP)
	u.HP += amt
	return amt
}

// AddShield tops the shield up by share of max hull, divided by 1 +
// shield strain, to at most ShieldCap of max hull, and resets its timer.
// It returns the buffer added.
func (u *Unit) AddShield(share float32) float32 {
	if !u.Alive || share <= 0 {
		return 0
	}
	amt := share * u.MaxHP / (1 + u.ShieldStrain)
	u.ShieldStrain++
	amt = min(amt, ShieldCap*u.MaxHP-u.Shield)
	amt = max(amt, 0)
	u.Shield += amt
	u.ShieldLeft = ShieldTime
	return amt
}

// Soak runs damage through the shield and returns what's left for hull.
func (u *Unit) Soak(d float32) float32 {
	if u.Shield <= 0 || d <= 0 {
		return d
	}
	s := min(u.Shield, d)
	u.Shield -= s
	if u.Shield <= 0 {
		u.Shield, u.ShieldLeft = 0, 0
	}
	return d - s
}

// TickSustain fades strain and runs down the shield's timer.
func (u *Unit) TickSustain(dt float32) {
	if u.MendStrain > 0 || u.ShieldStrain > 0 {
		k := float32(math.Exp2(-float64(dt) / StrainHalfLife))
		u.MendStrain *= k
		u.ShieldStrain *= k
		if u.MendStrain < 0.01 {
			u.MendStrain = 0
		}
		if u.ShieldStrain < 0.01 {
			u.ShieldStrain = 0
		}
	}
	if u.ShieldLeft > 0 {
		u.ShieldLeft -= dt
		if u.ShieldLeft <= 0 {
			u.Shield, u.ShieldLeft = 0, 0
		}
	}
}

// AddDot queues damage over time, replacing the weakest slot when full.
func (u *Unit) AddDot(d Dot) {
	best := 0
	for i := range u.Dots {
		if u.Dots[i].Left == 0 {
			u.Dots[i] = d
			return
		}
		if float32(u.Dots[i].Left)*u.Dots[i].PerTick < float32(u.Dots[best].Left)*u.Dots[best].PerTick {
			best = i
		}
	}
	u.Dots[best] = d
}

// Rules holds the tunable combat numbers. Defaults are a starting point;
// the design doc lists the exact curve as an open question.
type Rules struct {
	// Per-hit cap is CapAtEven of the foe's max health at even level and
	// doubles every CapDoubling levels the attacker is above the foe (halves
	// every CapDoubling below), clamped to [CapMin, 1].
	CapAtEven   float32
	CapDoubling float32
	CapMin      float32
	// Adaptive resistance: each hit of a type moves that type's resistance
	// AdaptGain of the way toward MaxResist; every other type decays by
	// AdaptDecay (multiplicative) per hit and by DecayPerSec over time.
	AdaptGain   float32
	AdaptDecay  float32
	DecayPerSec float32
	MaxResist   float32
	// WallImpact is terrain damage (fraction of max health) when any push
	// slams a foe into a wall; a type's WallImpactFrac adds to it.
	WallImpact float32
	// DotInterval is seconds between damage-over-time ticks.
	DotInterval float32
	// DotAdapt scales how much one damage-over-time tick builds resistance
	// compared with a direct hit, so ticks stay the anti-boss tool.
	DotAdapt float32
	// RaiseHP is the share of max health a raised minion comes back with.
	RaiseHP float32
}

// DefaultRules returns the prototype's tuning.
func DefaultRules() Rules {
	return Rules{
		CapAtEven:   0.25,
		CapDoubling: 5,
		CapMin:      0.01,
		AdaptGain:   0.15,
		AdaptDecay:  0.02,
		DecayPerSec: 0.05,
		MaxResist:   0.6,
		WallImpact:  0.10,
		DotInterval: 0.5,
		DotAdapt:    0.25,
		RaiseHP:     0.5,
	}
}

// HitCap is the most one hit can deal to a foe.
func (r *Rules) HitCap(attacker, defender int, maxHP float32) float32 {
	gap := float64(attacker - defender)
	frac := float64(r.CapAtEven) * math.Exp2(gap/float64(r.CapDoubling))
	if frac < float64(r.CapMin) {
		frac = float64(r.CapMin)
	}
	if frac > 1 {
		frac = 1
	}
	return float32(frac) * maxHP
}

// Resolve applies one hit of raw damage of type t to u and returns what was
// dealt and whether the cap cut it. Terrain hits skip resistance, the cap
// and adaptation: pits and walls are the real one-shot.
func (r *Rules) Resolve(u *Unit, raw float32, t TypeID, attacker int, terrain bool) (dealt float32, capped bool) {
	return r.resolve(u, raw, t, attacker, terrain, 1)
}

// ResolveTick is Resolve for one damage-over-time tick.
func (r *Rules) ResolveTick(u *Unit, raw float32, t TypeID, attacker int) (float32, bool) {
	return r.resolve(u, raw, t, attacker, false, r.DotAdapt)
}

func (r *Rules) resolve(u *Unit, raw float32, t TypeID, attacker int, terrain bool, adaptW float32) (dealt float32, capped bool) {
	if !u.Alive || raw <= 0 {
		return 0, false
	}
	dealt = raw
	if !terrain {
		dealt *= 1 - u.Resist[t]
		if lim := r.HitCap(attacker, u.Level, u.MaxHP); dealt > lim {
			dealt, capped = lim, true
		}
		r.adapt(u, t, adaptW)
		dealt = u.Soak(dealt) // shields soak hits, not walls and pits
	}
	if dealt > u.HP {
		dealt = u.HP
	}
	u.HP -= dealt
	if u.HP <= 0 {
		u.HP = 0
		u.Alive = false
	}
	return dealt, capped
}

func (r *Rules) adapt(u *Unit, t TypeID, w float32) {
	for i := range u.Resist {
		if TypeID(i) == t {
			u.Resist[i] += w * r.AdaptGain * (r.MaxResist - u.Resist[i])
		} else {
			u.Resist[i] *= 1 - w*r.AdaptDecay
		}
	}
}

// Decay lets resistances fade with time.
func (r *Rules) Decay(u *Unit, dt float32) {
	k := float32(math.Exp(-float64(r.DecayPerSec * dt)))
	for i := range u.Resist {
		u.Resist[i] *= k
	}
}

// Reveal ends a unit's stealth, if it has any, and starts its rest.
func (u *Unit) Reveal() {
	if u.Stealth > 0 {
		u.Stealth = 0
		u.StealthRest = StealthRest
	}
}

// TickStealth runs stealth down by dt; running out starts the rest.
func (u *Unit) TickStealth(dt float32) {
	u.StealthRest = max(0, u.StealthRest-dt)
	if u.Stealth > 0 {
		u.Stealth -= dt
		if u.Stealth <= 0 {
			u.Stealth = 0
			u.StealthRest = StealthRest
		}
	}
}

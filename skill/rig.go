package skill

import "math"

// Points are the attribute points a player has spent on their rig.
type Points struct {
	Capacity   int // max Power
	Recharge   int // Power regained per second
	Cooling    int // Heat vented per second
	Processing int // hex nesting depth
	Optics     int // light radius: how far the rig's lamp reaches
}

// SoftKnee is where every stat's soft cap starts biting.
const SoftKnee = 10

// SoftCap turns raw points into effective points: one for one up to the
// knee, then logarithmic. The curve is smooth at the knee, never stops
// rising, and makes the 30th point in one stat worth far less than the 1st
// point in another, so nobody maxes everything.
func SoftCap(points int, knee float64) float64 {
	p := float64(points)
	if p <= knee {
		return p
	}
	return knee * (1 + math.Log(p/knee))
}

// Light radius tuning, in hexes: the base reach with no Optics points,
// and what each effective (soft-capped) point adds. Placeholder balance.
const (
	LightBase = 6
	LightPer  = 0.5
)

// LightFor is the light radius, in hexes, that p's Optics points give.
func LightFor(p Points) float32 {
	return LightBase + LightPer*float32(SoftCap(p.Optics, SoftKnee))
}

// Rig is a player's power and heat state.
type Rig struct {
	MaxPower, Power float32
	RechargeRate    float32
	MaxHeat, Heat   float32
	CoolRate        float32
	MaxDepth        int
	// Light is the lamp's reach in hexes, from Optics points.
	Light float32
	// ShutdownLeft is seconds until an overheated rig can fire again.
	ShutdownLeft float32
	// ChargeMul and CoolMul scale recharge and cooling (zero is 1), for
	// stances.
	ChargeMul, CoolMul float32
}

// OverheatShutdown is how long a rig is locked after hitting max heat.
const OverheatShutdown = 3

// NewRig builds a full, cool rig from spent points.
func NewRig(p Points) Rig {
	eff := func(n int) float32 { return float32(SoftCap(n, SoftKnee)) }
	r := Rig{
		MaxPower:     100 + 8*eff(p.Capacity),
		RechargeRate: 8 + 1.5*eff(p.Recharge),
		MaxHeat:      100,
		CoolRate:     6 + 1.5*eff(p.Cooling),
		MaxDepth:     int(SoftCap(p.Processing, SoftKnee) / 5),
		Light:        LightFor(p),
	}
	r.Power = r.MaxPower
	return r
}

// TryFire spends a program's Power and adds its Heat. It refuses while shut
// down or short on Power. Reaching max heat fires the hex but shuts the
// rig down.
func (r *Rig) TryFire(p *Program) bool {
	if r.ShutdownLeft > 0 {
		return false
	}
	return r.Pay(p)
}

// Pay is TryFire without the lock check, for an on-vent hex firing into
// the lock its own vent started. It still needs the Power.
func (r *Rig) Pay(p *Program) bool {
	if r.Power < p.Power {
		return false
	}
	r.Power -= p.Power
	r.Heat = max(0, r.Heat+p.Heat)
	if r.Heat >= r.MaxHeat {
		r.Heat = r.MaxHeat
		r.ShutdownLeft = OverheatShutdown + p.LockExtra
	}
	return true
}

// AddHeat adds Heat outside a firing, like a watch's check. Reaching max
// heat shuts the rig down.
func (r *Rig) AddHeat(h float32) {
	r.Heat = max(0, r.Heat+h)
	if r.Heat >= r.MaxHeat {
		r.Heat = r.MaxHeat
		r.ShutdownLeft = OverheatShutdown
	}
}

// Tick recharges and vents.
func (r *Rig) Tick(dt float32) {
	charge, cool := r.RechargeRate, r.CoolRate
	if r.ChargeMul > 0 {
		charge *= r.ChargeMul
	}
	if r.CoolMul > 0 {
		cool *= r.CoolMul
	}
	r.Power = min(r.MaxPower, r.Power+charge*dt)
	r.Heat = max(0, r.Heat-cool*dt)
	r.ShutdownLeft = max(0, r.ShutdownLeft-dt)
}

// VentLock is how long a manual vent locks the rig.
const VentLock = 1

// Vent dumps all Heat at once and locks the rig for VentLock seconds: the
// panic button that trades a beat of tempo for not overheating, which locks
// for OverheatShutdown. It does nothing while already locked or cool.
func (r *Rig) Vent() bool {
	if r.ShutdownLeft > 0 || r.Heat <= 0 {
		return false
	}
	r.Heat = 0
	r.ShutdownLeft = VentLock
	return true
}

// Overheated reports a lock from overheating rather than a vent: an
// overheat locks for OverheatShutdown, longer than a vent's VentLock.
func (r *Rig) Overheated() bool { return r.ShutdownLeft > VentLock }

// Locked reports whether the rig is shut down (overheat or vent).
func (r *Rig) Locked() bool { return r.ShutdownLeft > 0 }

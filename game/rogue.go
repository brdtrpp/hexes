package game

import "hexes/skill"

// The Rogue's signature: mark, then cash in. Each cloak hit you land
// exposes the foe a stack deeper (up to ExposeMax, fading after
// ExposeTime). Hitting an exposed foe from behind or out of stealth
// cashes the stacks in for ExposeBonus more of that hit per stack. An
// exposed foe dying hands you back Heat for each stack and lets your
// cloak hide you again at once, so one kill opens the next.

const (
	ExposeMax    = 3
	ExposeTime   = 6
	ExposeBonus  = 0.5 // extra share of the cashing hit, per stack
	ExposeRefund = 10  // Heat back per stack when an exposed foe dies
)

// cloakType is the Rogue school's damage type.
func (s *Sim) cloakType() skill.TypeID { return SchoolType(s.Reg, SchoolRogue) }

// expose is a hit of yours on foe id: it cashes the stacks in when it
// lands from behind or out of stealth, and a cloak hit that doesn't
// adds a stack.
func (s *Sim) expose(id skill.EntityID, dealt float32, t skill.TypeID) {
	a := &s.Actors[id]
	p := s.P()
	if s.cashing || a.extra() || a.Kind == KindPlayer || a.U.Team == p.U.Team {
		return
	}
	if a.Exposed > 0 && a.U.Alive && (p.U.Ambush || s.Behind(s.Player, id)) {
		bonus := dealt * ExposeBonus * float32(a.Exposed)
		a.Exposed, a.ExposedT = 0, 0
		if bonus <= 0 {
			return
		}
		s.cashing = true
		extra := min(a.U.HP, bonus)
		a.U.HP -= extra
		if a.U.HP <= 0 {
			a.U.HP, a.U.Alive = 0, false
			a.Exposed = 1 // a kill on the cash-in still pays back
		}
		s.addFX(FX{Kind: FXAmbush, At: a.At, From: p.At, Value: extra, Life: 0.5})
		s.addFX(FX{Kind: FXNumber, At: a.At, Value: extra, Type: t, Mag: magOf(extra, a.U.MaxHP), Life: 0.8})
		s.cashing = false
		return
	}
	if t == s.cloakType() && a.U.Alive {
		a.Exposed = min(ExposeMax, a.Exposed+1)
		a.ExposedT = ExposeTime
	}
}

// exposedDied pays back a kill on an exposed foe.
func (s *Sim) exposedDied(a *Actor) {
	if a.Exposed == 0 {
		return
	}
	n := float32(a.Exposed)
	a.Exposed, a.ExposedT = 0, 0
	p := s.P()
	if !p.U.Alive {
		return
	}
	s.Rig.Heat = max(0, s.Rig.Heat-ExposeRefund*n)
	p.U.StealthRest = 0
}

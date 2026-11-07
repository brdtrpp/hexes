package game

import "hexes/skill"

// The Warden's signature. Your kinetic hits drag foes a step toward you,
// so they end up where your guard is; and every bit of a blow you turn
// aside (a taunt's cut, the bulwark stance, a shield soaking it) builds
// Brace. When Brace reaches BraceFull of your hull it goes off as a
// kinetic burst around you, BurstMult times what was stored, and throws
// what it hits back a step.

const (
	PullRest   = 0.5  // seconds before the same foe can be pulled again
	BraceFull  = 0.2  // share of max hull that sets the burst off
	BurstMult  = 3    // the burst's damage, per point of Brace stored
	BurstReach = 2    // hexes around you the burst reaches
	BraceFade  = 0.04 // share of BraceFull lost each second once nothing's blocked lately
	BraceHold  = 5    // seconds Brace holds before it starts to fade
)

// kineticType is the Warden school's damage type.
func (s *Sim) kineticType() skill.TypeID { return SchoolType(s.Reg, SchoolWarden) }

// pull drags foe id one step toward you after a kinetic hit of yours.
// Nothing moves into a wall, a pit or another body, and a foe already at
// your side stays put.
func (s *Sim) pull(id skill.EntityID) {
	a := &s.Actors[id]
	p := s.P()
	if !a.U.Alive || a.extra() || a.Free || a.Kind == KindPlayer || a.Fell || s.bursting ||
		skill.Dist(a.At, p.At) < 2 || s.Time-a.PulledAt < PullRest {
		return
	}
	next := a.At.Add(skill.HexDirs[dirTo(a.At, p.At)])
	if !s.Floor.In(next) || !s.Floor.At(next).Walkable() || s.occupied(next) || next == p.At {
		return
	}
	start := a.At
	s.occ[s.Floor.Index(a.At)] = skill.NoEntity
	s.occ[s.Floor.Index(next)] = id
	a.At, a.From, a.MoveT, a.MoveDur = next, start, 0, 0.12
	a.PulledAt = s.Time
	s.addFX(FX{Kind: FXPush, From: start, At: a.At, Life: 0.3})
}

// blocked banks amount of a blow you turned aside into Brace, and sets
// the burst off once it's full.
func (s *Sim) blocked(amount float32) {
	p := s.P()
	if amount <= 0 || !p.U.Alive {
		return
	}
	s.Brace += amount
	s.braceT = 0
	if s.Brace >= BraceFull*p.U.MaxHP {
		s.burst()
	}
}

// BraceShare is how full the Brace is, 0 to 1, for the HUD.
func (s *Sim) BraceShare() float32 {
	p := s.P()
	if p.U.MaxHP <= 0 {
		return 0
	}
	return min(1, s.Brace/(BraceFull*p.U.MaxHP))
}

// fadeBrace lets an unused Brace drain away.
func (s *Sim) fadeBrace(dt float32) {
	if s.Brace <= 0 {
		return
	}
	s.braceT += dt
	if s.braceT > BraceHold {
		s.Brace = max(0, s.Brace-BraceFade*BraceFull*s.P().U.MaxHP*dt)
	}
}

// burst lets the Brace go: kinetic damage on every foe around you, and
// a shove back.
func (s *Sim) burst() {
	p := s.P()
	dmg := s.Brace * BurstMult
	s.Brace, s.braceT = 0, 0
	kin := s.kineticType()
	x, y := p.Pos()
	s.addFX(FX{Kind: FXWave, At: p.At, X: x, Y: y, Value: BurstReach, Type: kin, Mag: 1, Life: 0.6})
	s.bursting = true
	defer func() { s.bursting = false }()
	for i := range s.Actors {
		a := &s.Actors[i]
		id := skill.EntityID(i)
		if id == s.Player || !a.U.Alive || a.extra() || a.U.Team == p.U.Team || skill.Dist(a.At, p.At) > BurstReach {
			continue
		}
		dealt, _ := s.Rules.Resolve(&a.U, dmg, kin, p.U.Level, false)
		s.Hit(s.Player, id, dealt, kin, false)
		if a.U.Alive {
			s.Push(id, s.Player, 1)
			continue
		}
		s.Killed(s.Player, id)
		s.Disp.Emit(skill.Event{Trigger: skill.OnKill, Actor: s.Player, Subject: id, Chain: 1, Slot: -1})
	}
}

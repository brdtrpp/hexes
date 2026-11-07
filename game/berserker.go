package game

import "hexes/skill"

// The Berserker's signature: the fight feeds the rage. Your nanite
// eating at a foe fills the rage meter, faster the longer you keep
// hitting (each nanite hit inside FuryWindow of the last adds to the
// streak), on top of what kills and hull spent already put in. The
// meter's rungs are stages you feel whether or not a rage hex spends
// them: at II you hit harder and move faster, at III harder still and
// your nanite feeds you hull back.

const (
	FuryWindow = 2.5  // seconds a streak waits for the next nanite hit
	FuryStreak = 12   // the longest streak
	FuryGrow   = 0.12 // each step of streak adds this much to rage gained
	FuryRage   = 160  // rage per share of your max hull your nanite deals
	// The stages: Might added and speed at rung II and III, and the
	// share of nanite damage that comes back as hull at III.
	FuryMight2 = 0.15
	FuryMight3 = 0.35
	FurySpeed  = 0.15
	FuryLeech  = 0.12
)

// nanite is the Berserker school's damage type.
func (s *Sim) naniteType() skill.TypeID { return SchoolType(s.Reg, SchoolBerserker) }

// rageRung is the stage your rage stands at, 1 to 3.
func (s *Sim) rageRung() uint8 {
	if l := s.Disp.Loadout(s.Player); l != nil {
		return skill.RageRung(l.Rage)
	}
	return 1
}

// RageShare is how full the rage meter is, 0 to 1, for the HUD.
func (s *Sim) RageShare() float32 {
	if l := s.Disp.Loadout(s.Player); l != nil {
		return l.Rage / skill.RageMax
	}
	return 0
}

// furyHit counts a nanite hit of yours toward the streak.
func (s *Sim) furyHit() {
	if s.Time-s.furyAt > FuryWindow {
		s.furyStreak = 0
	}
	s.furyStreak = min(FuryStreak, s.furyStreak+1)
	s.furyAt = s.Time
}

// fury is dealt damage of your nanite landing: it fills the rage, and at
// rung III some of it comes back to you as hull.
func (s *Sim) fury(dealt float32) {
	p := s.P()
	l := s.Disp.Loadout(s.Player)
	if l == nil || dealt <= 0 || !p.U.Alive {
		return
	}
	if s.Time-s.furyAt > FuryWindow {
		s.furyStreak = 0
	}
	if skill.RageRung(l.Rage) >= 3 {
		p.U.HP = min(p.U.MaxHP, p.U.HP+dealt*FuryLeech)
	}
	gain := FuryRage * dealt / p.U.MaxHP * (1 + FuryGrow*float32(s.furyStreak))
	l.Rage = min(skill.RageMax, l.Rage+gain)
}

// furyMight is what your rage stage adds to Might.
func (s *Sim) furyMight() float32 {
	switch s.rageRung() {
	case 3:
		return FuryMight3
	case 2:
		return FuryMight2
	}
	return 0
}

// Stances is which signature stances your hexes hold right now, for the
// looks: the Berserker's rage and the Mage's hot rig.
func (s *Sim) Stances() (berserk, mage bool) { return s.berserk, s.mage }

// RageStage is the stage your rage stands at, 1 to 3, for the looks.
func (s *Sim) RageStage() int { return int(s.rageRung()) }

// DevStances is for screenshots: depth d in both stances with rage full
// and the rig hot, three foes stood round you carrying one, two and
// three Rogue marks, and one just pulled in.
func (s *Sim) DevStances(d int) {
	s.load(d, 1)
	s.devStance = true
	p := s.P()
	if l := s.Disp.Loadout(s.Player); l != nil {
		l.Rage = skill.RageMax
	}
	s.Rig.Heat = 0.8 * s.Rig.MaxHeat
	spots := s.openAround(p.At, 8)
	n := 0
	for i := range s.Actors {
		a := &s.Actors[i]
		if n >= 3 || n*2+1 >= len(spots) || !a.U.Alive || a.Kind == KindPlayer || a.extra() {
			continue
		}
		to := spots[n*2+1]
		s.occ[s.Floor.Index(a.At)] = skill.NoEntity
		s.occ[s.Floor.Index(to)] = skill.EntityID(i)
		a.At, a.From, a.MoveT = to, to, 1
		a.X, a.Y = to.Pixel()
		n++
		a.Exposed, a.ExposedT = uint8(n), ExposeTime
		if n == 3 {
			s.addFX(FX{Kind: FXPull, From: p.At, At: a.At, Life: 30})
		}
	}
}

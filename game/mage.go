package game

import "hexes/skill"

// The Mage's signature: charge and release, with the rig as the charge.
// While your hexes carry plasma, Heat is power: the hotter the rig runs,
// the harder all your hexes hit (up to HeatMight more at full Heat). A
// vent lets it all go as a plasma nova around you, as big as the Heat it
// dumped. Run hot, then vent into a crowd.

const (
	HeatMight  = 0.5 // Might added at full Heat
	VentNova   = 2.2 // nova damage per point of Heat vented, at level 1
	VentReach  = 2   // hexes around you the nova reaches
	VentMinHot = 0.2 // share of max Heat a vent needs to make a nova
)

// plasmaType is the Mage school's damage type.
func (s *Sim) plasmaType() skill.TypeID { return SchoolType(s.Reg, SchoolMage) }

// Carries reports whether any online hex deals or applies school sc's
// damage type.
func (s *Sim) Carries(sc School) bool {
	t := SchoolType(s.Reg, sc)
	for i := 0; i < MaxSlots; i++ {
		if !s.HexOpen(i) || s.Load.Hexes[i] == nil {
			continue
		}
		for _, sl := range s.Load.Hexes[i].Slots {
			if sl.Kind == skill.SlotPart && Typed(sl.Part) && sl.Part.Type == t {
				return true
			}
		}
	}
	return false
}

// heatMight is what a hot rig adds to Might for a Mage.
func (s *Sim) heatMight() float32 {
	if !s.mage || s.Rig.MaxHeat <= 0 {
		return 0
	}
	return HeatMight * min(1, s.Rig.Heat/s.Rig.MaxHeat)
}

// ventNova lets heat vented go off as a plasma nova around you.
func (s *Sim) ventNova(heat float32) {
	p := s.P()
	if !s.mage || !p.U.Alive || heat < VentMinHot*s.Rig.MaxHeat {
		return
	}
	pl := s.plasmaType()
	dmg := heat * VentNova * scale(p.U.Level)
	x, y := p.Pos()
	s.addFX(FX{Kind: FXWave, At: p.At, X: x, Y: y, Value: VentReach, Type: pl, Mag: min(1, heat/s.Rig.MaxHeat), Life: 0.7})
	for i := range s.Actors {
		a := &s.Actors[i]
		id := skill.EntityID(i)
		if id == s.Player || !a.U.Alive || a.extra() || a.U.Team == p.U.Team || skill.Dist(a.At, p.At) > VentReach {
			continue
		}
		dealt, _ := s.Rules.Resolve(&a.U, dmg, pl, p.U.Level, false)
		s.Hit(s.Player, id, dealt, pl, false)
		if !a.U.Alive {
			s.Killed(s.Player, id)
			s.Disp.Emit(skill.Event{Trigger: skill.OnKill, Actor: s.Player, Subject: id, Chain: 1, Slot: -1})
		}
	}
}

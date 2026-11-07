package game

import (
	"testing"

	"hexes/skill"
)

func TestBerserkerFuryStages(t *testing.T) {
	s := classSim(t, "Berserker")
	if !s.Carries(SchoolBerserker) {
		t.Skip("the Berserker example carries no nanite")
	}
	l := s.Disp.Loadout(s.Player)
	p := s.P()
	p.U.MaxHP, p.U.HP = 1000, 500
	f := foeAt(s, 0, 2, 1e6)
	nan := s.naniteType()
	l.Rage = 0
	s.Time = 5
	// A streak fills faster than the same damage spread out.
	s.Hit(s.Player, f, 0, nan, false)
	s.fury(20)
	one := l.Rage
	for k := 0; k < 8; k++ {
		s.Time += 0.5
		s.Hit(s.Player, f, 0, nan, false)
	}
	before := l.Rage
	s.fury(20)
	if l.Rage-before <= one*1.5 {
		t.Fatalf("a streak of 9 gained %v, a lone hit %v", l.Rage-before, one)
	}
	// Stage II hits harder and moves faster; III feeds you hull.
	l.Rage = skill.RageMax / 2
	s.stances()
	if p.U.Might <= 1 || s.speedMul() <= 1 {
		t.Fatalf("stage II: might %v speed %v", p.U.Might, s.speedMul())
	}
	l.Rage = skill.RageMax
	s.stances()
	m3 := p.U.Might
	hp := p.U.HP
	s.fury(100)
	if m3 <= 1+FuryMight2 || p.U.HP <= hp {
		t.Fatalf("stage III: might %v, hull %v -> %v", m3, hp, p.U.HP)
	}
	// Nanite ticking on a foe feeds the rage too.
	l.Rage = 0
	g := foeAt(s, 2, 2, 1e6)
	s.Actors[g].U.AddDot(skill.Dot{PerTick: 30, Left: 4, Type: nan, AttackerLevel: 1, Owner: s.Player})
	for k := 0; k < 120; k++ {
		s.timers(1.0 / 60)
	}
	if l.Rage <= 0 {
		t.Fatal("ticking nanite never fed the rage")
	}
}

func TestMageHeatAndVentNova(t *testing.T) {
	s := classSim(t, "Mage: Overload")
	if !s.Carries(SchoolMage) {
		t.Skip("the Mage example carries no plasma")
	}
	p := s.P()
	s.Rig.Heat = 0
	s.stances()
	cold := p.U.Might
	s.Rig.Heat = s.Rig.MaxHeat * 0.9
	s.stances()
	if p.U.Might <= cold+0.3 {
		t.Fatalf("hot rig might %v, cold %v", p.U.Might, cold)
	}
	// A vent from hot goes off as a nova on what's near.
	f := foeAt(s, 0, 1, 1e5)
	g := foeAt(s, 3, 2, 1e5)
	far := foeAt(s, 1, 5, 1e5)
	hf, hg, hfar := s.Actors[f].U.HP, s.Actors[g].U.HP, s.Actors[far].U.HP
	s.Input.Vent = true
	s.playerInput(1.0 / 60)
	s.Input.Vent = false
	if s.Actors[f].U.HP >= hf || s.Actors[g].U.HP >= hg {
		t.Fatal("the vent nova missed what was near")
	}
	if s.Actors[far].U.HP < hfar {
		t.Fatal("the vent nova reached too far")
	}
	// A vent from cool makes none.
	s.Rig.ShutdownLeft = 0
	s.Rig.Heat = 5
	hf = s.Actors[f].U.HP
	s.Input.Vent = true
	s.playerInput(1.0 / 60)
	if s.Actors[f].U.HP < hf {
		t.Fatal("a cool vent made a nova")
	}
}

func TestRogueExposeAndCashIn(t *testing.T) {
	s := classSim(t, "Rogue")
	cl := s.cloakType()
	p := s.P()
	// Behind the foe: it faces away from you.
	f := foeAt(s, 0, 1, 1e4)
	s.Actors[f].Facing = 0
	front := foeAt(s, 3, 1, 1e4)
	s.Actors[front].Facing = 0 // it faces you
	if !s.Behind(s.Player, f) || s.Behind(s.Player, front) {
		t.Skip("couldn't set up a back and a front")
	}
	// Cloak hits from the front stack, up to the cap.
	for k := 0; k < 5; k++ {
		s.Hit(s.Player, front, 1, cl, false)
	}
	if s.Actors[front].Exposed != ExposeMax {
		t.Fatalf("stacks %d, want %d", s.Actors[front].Exposed, ExposeMax)
	}
	// From behind, any hit cashes them in.
	s.Actors[f].Exposed, s.Actors[f].ExposedT = 2, ExposeTime
	hp := s.Actors[f].U.HP
	s.Hit(s.Player, f, 100, SchoolType(s.Reg, SchoolMage), false)
	if s.Actors[f].Exposed != 0 || hp-s.Actors[f].U.HP < 100*ExposeBonus*2-0.01 {
		t.Fatalf("cash-in: stacks %d, extra %v", s.Actors[f].Exposed, hp-s.Actors[f].U.HP)
	}
	// An exposed foe dying pays Heat back and lets you hide again.
	s.Rig.Heat = 50
	p.U.StealthRest = 3
	s.Actors[front].U.HP, s.Actors[front].U.Alive = 0, false
	s.died(front)
	if s.Rig.Heat >= 50 || p.U.StealthRest != 0 {
		t.Fatalf("exposed kill: heat %v, rest %v", s.Rig.Heat, p.U.StealthRest)
	}
	// Stacks fade.
	s.Actors[f].Exposed, s.Actors[f].ExposedT = 1, ExposeTime
	for k := 0; k < 60*(ExposeTime+1); k++ {
		s.timers(1.0 / 60)
	}
	if s.Actors[f].Exposed != 0 {
		t.Fatal("stacks never faded")
	}
}

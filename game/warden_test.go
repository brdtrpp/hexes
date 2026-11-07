package game

import (
	"testing"

	"hexes/skill"
)

func TestWardenPullsAndBursts(t *testing.T) {
	s := classSim(t, "Warden")
	kin := s.kineticType()
	plasma := SchoolType(s.Reg, SchoolMage)

	// A plasma hit leaves a foe where it stands; a kinetic one drags it
	// a step in, and not again straight away.
	f := foeAt(s, 0, 4, 1e5)
	start := s.Actors[f].At
	s.Time = 10
	s.Hit(s.Player, f, 5, plasma, false)
	if s.Actors[f].At != start {
		t.Fatal("a plasma hit pulled")
	}
	s.Hit(s.Player, f, 5, kin, false)
	if d := skill.Dist(s.Actors[f].At, s.P().At); d != 3 {
		t.Fatalf("kinetic hit left the foe %d away, want 3", d)
	}
	s.Hit(s.Player, f, 5, kin, false)
	if d := skill.Dist(s.Actors[f].At, s.P().At); d != 3 {
		t.Fatal("pulled twice inside the rest")
	}
	// A foe at your side stays there.
	g := foeAt(s, 3, 1, 1e5)
	at := s.Actors[g].At
	s.Time += 1
	s.Hit(s.Player, g, 5, kin, false)
	if s.Actors[g].At != at {
		t.Fatal("pulled a foe already beside you")
	}

	// Blocked blows build Brace; a full Brace bursts on what's near.
	p := s.P()
	p.U.MaxHP, p.U.HP = 1000, 1000
	p.U.AddShield(skill.ShieldCap)
	hp := s.Actors[g].U.HP
	s.claw(g, 0.1*p.U.MaxHP)
	if s.Brace <= 0 || s.BraceShare() >= 1 {
		t.Fatalf("brace %v after one soaked blow", s.Brace)
	}
	for k := 0; k < 4 && s.Actors[g].U.HP >= hp; k++ {
		s.claw(g, 0.1*p.U.MaxHP)
	}
	if s.Actors[g].U.HP >= hp {
		t.Fatal("brace never burst")
	}
	if s.Brace >= BraceFull*p.U.MaxHP {
		t.Fatal("brace stayed full after the burst")
	}
	if skill.Dist(s.Actors[g].At, p.At) < 2 {
		t.Fatal("the burst didn't throw the foe back")
	}

	// An unused Brace drains away after a while.
	s.Brace = 0.5 * BraceFull * p.U.MaxHP
	s.braceT = 0
	for k := 0; k < 60*(BraceHold+30); k++ {
		s.fadeBrace(1.0 / 60)
	}
	if s.Brace != 0 {
		t.Fatalf("brace %v never drained", s.Brace)
	}
}

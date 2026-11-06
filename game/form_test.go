package game

import (
	"testing"

	"hexes/dungeon"
	"hexes/skill"
)

// ring clears the floor round the player and puts tough grunts on every
// neighbouring hex and the ring past it.
func ring(t *testing.T, s *Sim) {
	t.Helper()
	p := s.P()
	for i := range s.Actors {
		if skill.EntityID(i) != s.Player {
			s.Actors[i].U.Alive = false
			s.occ[s.Floor.Index(s.Actors[i].At)] = skill.NoEntity
		}
	}
	for r := 1; r <= 3; r++ {
		for _, d := range skill.HexDirs {
			c := p.At
			for k := 0; k < r; k++ {
				c = c.Add(d)
			}
			s.Floor.Set(c, dungeon.Ground)
			if r <= 2 {
				s.spawn(KindGrunt, 1, c, 1, 1e6)
			}
		}
	}
}

// Every example build, fired into a crowd, shows the areas it touched as
// area effects, and hits inside an area draw no line back to the caster.
func TestAreaFormsFromExamples(t *testing.T) {
	kinds := map[FXKind]int{}
	for i, e := range Examples(skill.DefaultRegistry()) {
		s := NewVeteran(9)
		if err := s.SetTestShapes(true); err != nil {
			t.Fatal(err)
		}
		if err := s.LoadExample(i); err != nil {
			t.Fatalf("%s: %v", e.Name, err)
		}
		ring(t, s)
		for slot := range s.Load.Progs {
			if s.Load.Progs[slot] == nil {
				continue
			}
			s.FX.Clear()
			s.Rig.Heat, s.Rig.Power, s.Rig.ShutdownLeft = 0, s.Rig.MaxPower, 0
			s.Disp.Cast(s.Player, slot, s.nearestFoe(s.P().At, NearestRange))
			area, impacts := 0, 0
			s.FX.Each(func(f *FX) {
				kinds[f.Kind]++
				switch f.Kind {
				case FXWave, FXCone, FXChain, FXBeam:
					area++
					if f.Mag <= 0 {
						t.Errorf("%s hex %d: area form with no magnitude", e.Name, slot)
					}
				case FXBolt:
					if f.Impact {
						impacts++
					}
				}
			})
			if impacts > 0 && area == 0 {
				t.Errorf("%s hex %d: %d area hits but no area effect", e.Name, slot, impacts)
			}
			if s.nforms != 0 {
				t.Errorf("%s hex %d: %d forms left after firing", e.Name, slot, s.nforms)
			}
		}
	}
	if kinds[FXWave]+kinds[FXCone]+kinds[FXChain] == 0 {
		t.Fatalf("no area effects from any example: %v", kinds)
	}
	t.Logf("effects: wave %d cone %d chain %d bolt %d ring %d", kinds[FXWave], kinds[FXCone], kinds[FXChain], kinds[FXBolt], kinds[FXRing])
}

// The cast ring carries what the hex cost.
func TestRingCarriesCost(t *testing.T) {
	s := NewVeteran(3)
	ring(t, s)
	s.Rig.Heat = 0
	if o := s.Disp.Cast(s.Player, 0, s.nearestFoe(s.P().At, NearestRange)); o != skill.Fired {
		t.Fatalf("cast %s", o)
	}
	found := false
	s.FX.Each(func(f *FX) {
		if f.Kind == FXRing && f.Slot == 0 {
			found = true
			p := s.Load.Progs[0]
			if f.Power != p.Power || f.Heat != p.Heat {
				t.Fatalf("ring cost %v/%v, hex %v/%v", f.Power, f.Heat, p.Power, p.Heat)
			}
		}
	})
	if !found {
		t.Fatal("no ring")
	}
}

// The starter build shows its chain (Arc Lance) and its rings (Cryo
// Bloom's chill lands with no damage and still shows).
func TestStarterForms(t *testing.T) {
	s := NewVeteran(3)
	ring(t, s)
	kinds := map[FXKind]int{}
	for slot := range s.Load.Progs {
		if s.Load.Progs[slot] == nil {
			continue
		}
		s.Rig.Heat, s.Rig.Power, s.Rig.ShutdownLeft = 0, s.Rig.MaxPower, 0
		s.Disp.Cast(s.Player, slot, s.nearestFoe(s.P().At, NearestRange))
	}
	s.FX.Each(func(f *FX) {
		kinds[f.Kind]++
		if f.Kind == FXChain && f.NPts < 3 {
			t.Errorf("chain of %d points", f.NPts)
		}
	})
	if kinds[FXChain] == 0 || kinds[FXWave] < 2 {
		t.Fatalf("starter effects %v", kinds)
	}
}

// A status-only hit (Cryo Bloom's chill, no damage) still shows its ring.
func TestStatusOnlyShowsArea(t *testing.T) {
	s := NewVeteran(3)
	ring(t, s)
	p := s.Load.Progs[2] // Cryo Bloom: adjacent chill, damage only behind a full bank
	s.FX.Clear()
	s.Eval.Run(p, s, skill.Ctx{Caster: s.Player, Subject: s.Player, Level: 1})
	s.flushForms(s.Player, p, skill.Fired)
	waves := 0
	s.FX.Each(func(f *FX) {
		if f.Kind == FXWave {
			waves++
		}
	})
	if waves != 1 {
		t.Fatalf("%d waves from a chill", waves)
	}
}

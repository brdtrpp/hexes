package game

import (
	"encoding/json"
	"testing"

	"hexes/loot"
	"hexes/skill"
)

// killWith drops foe f as if a hit of type t finished it.
func killWith(s *Sim, f skill.EntityID, t skill.TypeID) {
	a := &s.Actors[f]
	a.LastHit = t
	a.U.HP, a.U.Alive = 0, false
	s.Killed(s.Player, f)
}

func TestNecroArmyRisesStaysAndMarches(t *testing.T) {
	s := classSim(t, "Necromancer")
	necro := s.necroType()
	other := SchoolType(s.Reg, SchoolMage)

	// A plasma kill leaves a wreck; a drone kill gets up.
	f := foeAt(s, 0, 2, 40)
	killWith(s, f, other)
	if s.Actors[f].Minion || !s.wreck(int(f)) {
		t.Fatal("a plasma kill raised the body")
	}
	g := foeAt(s, 1, 2, 40)
	killWith(s, g, necro)
	if !s.Actors[g].Minion || !s.Actors[g].U.Alive {
		t.Fatal("a drone kill didn't raise the body")
	}
	// Past MinionMax the bodies stay down as wrecks.
	for k := 0; k < skill.MinionMax+2; k++ {
		h := foeAt(s, k%6, 3+k/6, 40)
		killWith(s, h, necro)
	}
	if n := s.minions(s.Player); n != skill.MinionMax {
		t.Fatalf("army of %d, want %d", n, skill.MinionMax)
	}
	// No clock: a long while later they're all still up.
	for k := 0; k < 60*60; k++ {
		s.extras(1.0 / 60)
	}
	if n := s.minions(s.Player); n != skill.MinionMax {
		t.Fatalf("army shrank to %d with nothing to fight", n)
	}

	// A save keeps the army, and a reload sets it down beside you.
	b, err := s.Save()
	if err != nil {
		t.Fatal(err)
	}
	var sv Save
	if err := json.Unmarshal(b, &sv); err != nil {
		t.Fatal(err)
	}
	if len(sv.Army) != skill.MinionMax {
		t.Fatalf("saved %d raised, want %d", len(sv.Army), skill.MinionMax)
	}
	r := NewVeteran(11)
	if err := r.Restore(sv); err != nil {
		t.Fatal(err)
	}
	for k := 0; k < 30; k++ {
		r.extras(1.0 / 60)
	}
	if n := r.minions(r.Player); n != skill.MinionMax {
		t.Fatalf("reload brought %d raised, want %d", n, skill.MinionMax)
	}

	// Down the stairs they come too, at least a level under yours.
	r.State, r.StateT = Descending, DescendTime
	depth := r.Depth
	r.Step(1.0 / 60)
	for k := 0; k < 30; k++ {
		r.extras(1.0 / 60)
	}
	if r.Depth != depth+1 {
		t.Fatalf("depth %d, want %d", r.Depth, depth+1)
	}
	if n := r.minions(r.Player); n != skill.MinionMax {
		t.Fatalf("stairs brought %d raised, want %d", n, skill.MinionMax)
	}
	for i := range r.Actors {
		if a := &r.Actors[i]; a.Minion && a.U.Level < r.Depth-1 {
			t.Fatalf("raised body at level %d on depth %d", a.U.Level, r.Depth)
		}
	}

	// Your death scatters it.
	r.P().U.HP, r.P().U.Alive = 0, false
	r.Step(1.0 / 60)
	r.Input.Restart = true
	r.StateT = 1
	r.Step(1.0 / 60)
	if n := r.minions(r.Player); n != 0 {
		t.Fatalf("%d raised survived your death", n)
	}
}

// TestSignatureLean checks every school's drops come up its own parts
// more often, and plain hits less.
func TestSignatureLean(t *testing.T) {
	s := NewVeteran(5)
	tab := s.Stash.table
	tab.Allow = loot.Allow{New: loot.FamAll}
	count := func(sc School, lean bool) (sig, plain int) {
		r := loot.NewRNG(99)
		open := sc.Bit()
		roll := func() skill.Part { return tab.RollTaught(&r, skill.Common) }
		if lean {
			roll = s.leaning(&r, tab, skill.Common, open, roll)
		}
		for k := 0; k < 4000; k++ {
			p := s.schoolRoll(&r, open, roll, nil)
			switch {
			case Typed(p) && p.Kind == skill.KindEffect:
				plain++
			case SchoolOf(p, s.Reg) == sc.Bit():
				sig++
			}
		}
		return
	}
	for sc := range NumSchools {
		s0, p0 := count(sc, false)
		s1, p1 := count(sc, true)
		if s1 <= s0*11/10 || p1 > p0*2/3 {
			t.Errorf("%s: signature %d -> %d, plain %d -> %d", sc, s0, s1, p0, p1)
		}
	}
}

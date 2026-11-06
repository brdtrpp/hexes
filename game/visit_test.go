package game

import (
	"testing"

	"hexes/skill"
)

// TestVisitMixed checks the test menu's visits: the floor holds the kinds
// asked for, and the visit leaves no mark on the run. Stairs, a death and
// a save made during it all come back home.
func TestVisitMixed(t *testing.T) {
	s := NewVeteran(5)
	s.load(6, 1)
	best, deaths, data := s.Best, s.Deaths, s.DataTotal()
	for _, n := range []int{2, 3} {
		d := s.VisitMixed(n)
		if d != VisitDepth(n) || s.Depth != d {
			t.Fatalf("visit to %d kinds went to depth %d", n, s.Depth)
		}
		if len(s.Floor.Regions) != n {
			t.Fatalf("visit to %d kinds holds %d", n, len(s.Floor.Regions))
		}
		if s.Visit != 6 {
			t.Fatalf("visit remembers home as %d", s.Visit)
		}
	}
	if s.Best != best || s.DataTotal() != data {
		t.Fatalf("a visit moved best %d to %d or Data %d to %d", best, s.Best, data, s.DataTotal())
	}

	// A save during a visit is a save at home.
	b, err := s.Save()
	if err != nil {
		t.Fatal(err)
	}
	r := NewVeteran(5)
	if err := r.LoadSave(b); err != nil {
		t.Fatal(err)
	}
	if r.Depth != 6 || r.Visiting() {
		t.Fatalf("a save on a visit loads at depth %d (visiting %v)", r.Depth, r.Visiting())
	}

	// A death costs nothing and goes home.
	c := s.Stash.Inv.Add(skill.NewPush(1))
	s.Stash.Fresh[c] = true
	kill(s)
	if s.Deaths != deaths || s.Cache.Active || s.LastDeath.LetGo != 0 {
		t.Fatalf("a visit death counted: deaths %d, cache %v, let go %d", s.Deaths, s.Cache.Active, s.LastDeath.LetGo)
	}
	s.respawn()
	if s.Depth != 6 || s.Visiting() {
		t.Fatalf("after a visit death you're on depth %d (visiting %v)", s.Depth, s.Visiting())
	}

	// The stairs go home too, with no floor-end draft.
	s.VisitMixed(2)
	p := s.P()
	p.At, p.From, p.MoveT = s.Floor.Exit, s.Floor.Exit, 1
	s.Step(1.0 / 60)
	if s.State != Descending {
		t.Fatalf("on a visit's stairs the state is %v, want descending", s.State)
	}
	s.load(s.Depth+1, 1)
	if s.Depth != 6 || s.Visiting() || s.Best != best {
		t.Fatalf("a visit's stairs went to depth %d (best %d)", s.Depth, s.Best)
	}
}

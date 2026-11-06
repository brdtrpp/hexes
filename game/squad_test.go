package game

import (
	"testing"

	"hexes/skill"
)

// A pack closing in spreads round you instead of queueing on one side.
func TestPackSurrounds(t *testing.T) {
	s := openArena(34, 30)
	p := s.P()
	at := p.At
	for i := 0; i < 5; i++ {
		c := at.Add(skill.Coord{Q: 7, R: i - 2})
		id := s.spawn(KindGrunt, 1, c, 1, 1e6)
		s.Actors[id].Squad = 1
	}
	s.formSquads()
	run(s, 0, 0, 8, nil)
	var sides [6]bool
	n := 0
	for i := range s.Actors {
		a := &s.Actors[i]
		if a.Kind != KindGrunt || skill.Dist(a.At, p.At) != 1 {
			continue
		}
		n++
		sides[dirTo(p.At, a.At)] = true
	}
	far := 0
	for k, ok := range sides {
		if ok && (sides[(k+3)%6] || sides[(k+2)%6] || sides[(k+4)%6]) {
			far++
		}
	}
	if n < 4 || far == 0 {
		t.Fatalf("%d of 5 grunts next to you, sides %v: they queued instead of surrounding", n, sides)
	}
}

// A hit on one sleeping foe wakes its pack, a beat later.
func TestHitCallsPack(t *testing.T) {
	s := openArena(60, 20)
	p := s.P()
	a := s.spawn(KindGrunt, 1, p.At.Add(skill.Coord{Q: 20}), 1, 100)
	b := s.spawn(KindGrunt, 1, p.At.Add(skill.Coord{Q: 22}), 1, 100)
	c := s.spawn(KindGrunt, 1, p.At.Add(skill.Coord{Q: 20, R: -3}), 1, 100)
	s.Actors[a].Squad, s.Actors[b].Squad = 2, 2
	s.formSquads()
	s.Hit(s.Player, a, 5, 0, false)
	if !s.Actors[b].Awake || s.Actors[b].WakeIn <= 0 {
		t.Fatal("the hit grunt's packmate didn't hear the call")
	}
	if !s.Actors[c].Awake {
		t.Fatal("a grunt three hexes from the call slept through it")
	}
}

// A gunner holds its range band and keeps shooting.
func TestGunnerKeepsRange(t *testing.T) {
	s := openArena(40, 30)
	p := s.P()
	g := s.spawn(KindGunner, 1, p.At.Add(skill.Coord{Q: 9}), 1, 1e6)
	s.Actors[g].Awake = true
	hp := p.U.HP
	close := 0
	run(s, 0, 0, 12, func() {
		if skill.Dist(s.Actors[g].At, p.At) < GunMin-1 {
			close++
		}
	})
	if close > 0 {
		t.Fatalf("the gunner came within %d hexes for %d frames", GunMin-1, close)
	}
	if p.U.HP >= hp || s.Actors[g].Shots < 3 {
		t.Fatalf("the gunner fired %d shots and did no harm", s.Actors[g].Shots)
	}
	// Walk at it: it backs off.
	gx, _ := s.Actors[g].Pos()
	run(s, 1, 0, 1.5, nil)
	if x, _ := s.Actors[g].Pos(); x <= gx {
		t.Fatalf("the gunner didn't back off: %.1f to %.1f", gx, x)
	}
}

// A badly hurt grunt falls back once, then comes again.
func TestHurtFoeFallsBack(t *testing.T) {
	s := openArena(40, 30)
	p := s.P()
	g := s.spawn(KindGrunt, 1, p.At.Add(skill.HexDirs[0]), 1, 1e6)
	s.Actors[g].Awake = true
	s.Actors[g].U.HP = 2e5
	run(s, 0, 0, 1.2, nil)
	if d := skill.Dist(s.Actors[g].At, p.At); d < 2 {
		t.Fatalf("a grunt at a fifth of its hull stayed in your face (%d away)", d)
	}
	back := false
	run(s, 0, 0, 6, func() { back = back || skill.Dist(s.Actors[g].At, p.At) == 1 })
	if !back {
		t.Fatal("it never came back")
	}
}

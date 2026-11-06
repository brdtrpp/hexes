package game

import (
	"testing"

	"hexes/dungeon"
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

// corridor is a one-wide east-west tunnel of length n with the player at
// its west end.
func corridor(n int) *Sim {
	s := openArena(n+4, 9)
	f := s.Floor
	mid := f.Start.R
	for i := range f.Tiles {
		c := f.CoordOf(i)
		col := c.Q + c.R>>1
		if c.R != mid || col < 1 || col > n+2 {
			f.Tiles[i] = dungeon.Wall
		}
	}
	p := s.P()
	s.occ[f.Index(p.At)] = skill.NoEntity
	start := skill.Coord{Q: 2 - mid>>1, R: mid}
	f.Start = start
	p.At, p.From = start, start
	s.occ[f.Index(start)] = s.Player
	p.placeFree(start)
	s.safe = start
	s.refreshField()
	s.refreshSight()
	return s
}

// Gunners queued in a tunnel shuffle back down it to their range instead
// of shooting from point blank.
func TestJammedGunnersBackOff(t *testing.T) {
	s := corridor(30)
	p := s.P()
	var gs [3]skill.EntityID
	for i := range gs {
		gs[i] = s.spawn(KindGunner, 1, p.At.Add(skill.Coord{Q: 2 + i}), 1, 1e6)
		s.Actors[gs[i]].Awake = true
		s.Actors[gs[i]].Squad = 3
	}
	s.formSquads()
	close := 0
	run(s, 0, 0, 4, func() {
		for _, g := range gs {
			if a := &s.Actors[g]; a.Aim > 0 && skill.Dist(a.At, p.At) < GunMin {
				close++
			}
		}
	})
	if close > 0 {
		t.Fatalf("jammed gunners lined up %d frames of point-blank shots", close)
	}
	for _, g := range gs {
		if d := skill.Dist(s.Actors[g].At, p.At); d < GunMin {
			t.Fatalf("a gunner is still %d hexes away after 4 s in the tunnel", d)
		}
	}
}

// A gunner with a grunt behind it in a tunnel trades places with it.
func TestJammedGunnerSwapsWithMelee(t *testing.T) {
	s := corridor(30)
	p := s.P()
	g := s.spawn(KindGunner, 1, p.At.Add(skill.Coord{Q: 2}), 1, 1e6)
	b := s.spawn(KindBrute, 1, p.At.Add(skill.Coord{Q: 3}), 1, 1e6)
	for _, id := range []skill.EntityID{g, b} {
		s.Actors[id].Awake = true
		s.Actors[id].Squad = 4
	}
	s.formSquads()
	run(s, 0, 0, 1.5, nil)
	if dg, db := skill.Dist(s.Actors[g].At, p.At), skill.Dist(s.Actors[b].At, p.At); dg <= db {
		t.Fatalf("gunner %d away, brute %d: they didn't trade places", dg, db)
	}
}

// A gunner with no way back at all swings instead of shooting point blank.
func TestCorneredGunnerSwings(t *testing.T) {
	s := corridor(3)
	p := s.P()
	g := s.spawn(KindGunner, 1, p.At.Add(skill.Coord{Q: 1}), 1, 1e6)
	s.Actors[g].Awake = true
	// Wall the gunner in behind.
	f := s.Floor
	for q := 2; q < 8; q++ {
		f.Tiles[f.Index(p.At.Add(skill.Coord{Q: q}))] = dungeon.Wall
	}
	s.refreshField()
	hp := p.U.HP
	run(s, 0, 0, 3, nil)
	if s.Actors[g].Shots > 0 {
		t.Fatalf("a cornered gunner fired %d point-blank shots", s.Actors[g].Shots)
	}
	if p.U.HP >= hp {
		t.Fatal("a cornered gunner did nothing at all")
	}
}

package game

import (
	"math"
	"testing"

	"hexes/dungeon"
	"hexes/skill"
)

// openFloor is a w by h floor of ground with a wall rim.
func openFloor(w, h int) *dungeon.Floor {
	f := &dungeon.Floor{W: w, H: h, Tiles: make([]dungeon.Tile, w*h)}
	for i := range f.Tiles {
		c := f.CoordOf(i)
		col := c.Q + c.R>>1
		if c.R == 0 || c.R == h-1 || col == 0 || col == w-1 {
			f.Tiles[i] = dungeon.Wall
		} else {
			f.Tiles[i] = dungeon.Ground
		}
	}
	return f
}

func center(f *dungeon.Floor) skill.Coord {
	r := f.H / 2
	return skill.Coord{Q: f.W/2 - r>>1, R: r}
}

func TestSightReachesRadius(t *testing.T) {
	f := openFloor(40, 40)
	at := center(f)
	var v Sight
	v.Update(f, at, 6)
	for i := range f.Tiles {
		c := f.CoordOf(i)
		d := skill.Dist(at, c)
		if d <= 6 && !v.Vis[i] {
			t.Fatalf("hex %v at %d not lit", c, d)
		}
		if d > 6 && v.Vis[i] {
			t.Fatalf("hex %v at %d lit past the radius", c, d)
		}
		if v.Seen[i] != (d <= 5) {
			t.Fatalf("hex %v at %d: seen %v, want %v", c, d, v.Seen[i], d <= 5)
		}
	}
}

func TestWallsBlockLight(t *testing.T) {
	f := openFloor(40, 40)
	at := center(f)
	// A wall line two hexes east, three tall.
	for dr := -2; dr <= 2; dr++ {
		f.Set(skill.Coord{Q: at.Q + 2 - dr/2, R: at.R + dr}, dungeon.Wall)
	}
	var v Sight
	v.Update(f, at, 8)
	behind := skill.Coord{Q: at.Q + 5, R: at.R}
	if v.Lit(f, behind) {
		t.Fatal("light went through the wall")
	}
	if wall := (skill.Coord{Q: at.Q + 2, R: at.R}); !v.Lit(f, wall) {
		t.Fatal("the wall itself should be lit")
	}
	west := skill.Coord{Q: at.Q - 5, R: at.R}
	if !v.Lit(f, west) {
		t.Fatal("open side should be lit")
	}
}

func TestSeenKeepsAndResets(t *testing.T) {
	f := openFloor(60, 30)
	a := center(f)
	var v Sight
	v.Update(f, a, 5)
	gen := v.Gen
	v.Update(f, a, 5) // same hex, same radius: nothing to do
	if v.Gen != gen {
		t.Fatal("standing still re-charted")
	}
	b := skill.Coord{Q: a.Q + 15, R: a.R}
	v.Update(f, b, 5)
	if !v.Seen[f.Index(a)] {
		t.Fatal("explored ground was forgotten")
	}
	if v.Vis[f.Index(a)] {
		t.Fatal("old spot still lit after walking away")
	}
	g := openFloor(60, 30)
	v.Update(g, a, 5)
	if v.Seen[g.Index(b)] {
		t.Fatal("a new floor kept the old record")
	}
}

func TestLightFalloff(t *testing.T) {
	px := func(hexes float64) float64 { return hexes * 1.7320508 }
	if l := LightAt(0, 0, px(2), 0, 6); l != 1 {
		t.Fatalf("core %v, want 1", l)
	}
	if l := LightAt(0, 0, px(6), 0, 6); l != 0 {
		t.Fatalf("edge %v, want 0", l)
	}
	prev := float32(1)
	for d := 3.0; d <= 6; d += 0.25 {
		l := LightAt(0, 0, px(d), 0, 6)
		if l > prev {
			t.Fatalf("light rises at %v", d)
		}
		prev = l
	}
}

func TestLightRadiusFromOptics(t *testing.T) {
	s := NewVeteran(3)
	if r := s.LightRadius(); r != skill.LightBase {
		t.Fatalf("default radius %v, want %v", r, skill.LightBase)
	}
	more := skill.NewRig(skill.Points{Optics: 10})
	if more.Light <= skill.LightBase {
		t.Fatal("optics points add nothing")
	}
	lots := skill.NewRig(skill.Points{Optics: 40})
	if (lots.Light-more.Light)/30 >= (more.Light-skill.LightBase)/10 {
		t.Fatal("optics not soft capped")
	}
	s.Rig.ShutdownLeft = 1
	if r := s.LightRadius(); math.Abs(float64(r-skill.LightBase*BrownOut)) > 1e-4 {
		t.Fatalf("locked radius %v, want %v", r, skill.LightBase*BrownOut)
	}
	// The sim charts the start on load.
	if !s.Sight.Seen[s.Floor.Index(s.P().At)] {
		t.Fatal("start not charted")
	}
}

func BenchmarkSightStep(b *testing.B) {
	f := openFloor(100, 74)
	at := center(f)
	var v Sight
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		v.Update(f, skill.Coord{Q: at.Q + i%2, R: at.R}, 16)
	}
}

// foeInView puts a grunt next to the player, where the lamp sees it, for
// tests that need something to cast at.
func foeInView(t *testing.T, s *Sim) skill.EntityID {
	t.Helper()
	p := s.P()
	for _, d := range skill.HexDirs {
		n := p.At.Add(d)
		if s.Floor.At(n).Walkable() && !s.occupied(n) {
			return s.spawn(KindGrunt, 1, n, 1, 1e5)
		}
	}
	t.Fatal("no room next to the player")
	return skill.NoEntity
}

func TestHexesOnlyTargetWhatYouSee(t *testing.T) {
	s := openArena(40, 40)
	p := s.P()
	far := s.spawn(KindGrunt, 1, p.At.Add(skill.Coord{Q: 8, R: 0}), 1, 100)
	var out [8]skill.EntityID
	if n := s.Select(skill.TargetNearest, s.Player, s.Player, 0, out[:]); n != 0 {
		t.Fatalf("picked %v out in the dark", out[:n])
	}
	if s.nearestFoe(p.At, NearestRange) != skill.NoEntity {
		t.Fatal("cast aimed into the dark")
	}
	s.AddOptics(30) // a lamp that reaches past it
	if n := s.Select(skill.TargetNearest, s.Player, s.Player, 0, out[:]); n != 1 || out[0] != far {
		t.Fatalf("lit foe not picked: %v", out[:n])
	}
	// Behind a wall it's hidden again, however bright the lamp.
	s.Floor.Set(p.At.Add(skill.Coord{Q: 1, R: 0}), dungeon.Wall)
	s.Floor.Set(p.At.Add(skill.Coord{Q: 1, R: -1}), dungeon.Wall)
	s.Floor.Set(p.At.Add(skill.Coord{Q: 0, R: 1}), dungeon.Wall)
	s.Sight.r = -1
	s.refreshSight()
	if n := s.Select(skill.TargetNearest, s.Player, s.Player, 0, out[:]); n != 0 {
		t.Fatalf("picked %v through a wall", out[:n])
	}
	// Enemies still see you, dark or not.
	if !s.foe(far, int(s.Player)) {
		t.Fatal("foes lost track of the player")
	}
}

func TestLensesWidenTheLight(t *testing.T) {
	s := openArena(30, 30)
	before := s.LightRadius()
	s.Stash.Lenses = append(s.Stash.Lenses, LensDrop{At: s.P().At, Points: 2})
	s.pickupLenses(s.P().At, 0.1)
	if len(s.Stash.Lenses) != 0 || s.Points.Optics != 2 {
		t.Fatalf("lens not picked up: %d left, optics %d", len(s.Stash.Lenses), s.Points.Optics)
	}
	if s.LightRadius() <= before {
		t.Fatal("light didn't grow")
	}
	b, err := s.Save()
	if err != nil {
		t.Fatal(err)
	}
	r := NewVeteran(9)
	if err := r.LoadSave(b); err != nil {
		t.Fatal(err)
	}
	if r.Points.Optics != 2 || r.LightRadius() != s.LightRadius() {
		t.Fatalf("optics %d, light %v after load", r.Points.Optics, r.LightRadius())
	}
	if LensChance(30, true) > LensDropMax*LensDropElite || LensChance(1, false) != LensDropChance {
		t.Fatal("lens chance off")
	}
}

func TestLensesDrop(t *testing.T) {
	s := NewVeteran(11)
	got := 0
	for v := 0; v < 400 && got == 0; v++ {
		s.Visits = v
		s.Stash.Lenses = s.Stash.Lenses[:0]
		for i := range s.Actors {
			if i != int(s.Player) {
				s.rollLensDrop(skill.EntityID(i), s.P().At, false)
			}
		}
		got = len(s.Stash.Lenses)
	}
	if got == 0 {
		t.Fatal("no lens in 400 floors of kills")
	}
}

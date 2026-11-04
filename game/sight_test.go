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
	s := New(3)
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

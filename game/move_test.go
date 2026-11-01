package game

import (
	"math"
	"testing"

	"hexes/dungeon"
	"hexes/skill"
)

// openArena swaps in a walled rectangle of open floor with the player in
// the middle and nobody else.
func openArena(w, h int) *Sim {
	s := New(1)
	f := &dungeon.Floor{W: w, H: h, Tiles: make([]dungeon.Tile, w*h), Seed: 7}
	for i := range f.Tiles {
		c := f.CoordOf(i)
		col := c.Q + c.R>>1
		if c.R == 0 || c.R == h-1 || col == 0 || col == w-1 {
			f.Tiles[i] = dungeon.Wall
		} else {
			f.Tiles[i] = dungeon.Ground
		}
	}
	f.Start = f.CoordOf(h/2*w + w/2)
	f.Exit = f.CoordOf(w + 1)
	s.Floor = f
	s.occ = make([]skill.EntityID, w*h)
	for i := range s.occ {
		s.occ[i] = skill.NoEntity
	}
	s.dist = make([]int32, w*h)
	s.q = make([]int32, w*h)
	s.Actors = s.Actors[:0]
	s.Player = s.spawn(KindPlayer, 0, f.Start, 1, 1e6)
	p := s.P()
	p.Awake, p.Free = true, true
	p.placeFree(f.Start)
	p.FaceX = 1
	s.safe = f.Start
	s.refreshField()
	return s
}

func run(s *Sim, mx, my float64, seconds float64, each func()) {
	s.Input = Input{MoveX: mx, MoveY: my}
	for i := 0; i < int(seconds*60); i++ {
		s.Step(1.0 / 60)
		if each != nil {
			each()
		}
	}
}

func TestFromPixel(t *testing.T) {
	for q := -6; q <= 6; q++ {
		for r := -6; r <= 6; r++ {
			c := skill.Coord{Q: q, R: r}
			x, y := c.Pixel()
			if got := skill.FromPixel(x, y); got != c {
				t.Fatalf("center of %v maps to %v", c, got)
			}
			// Just inside each corner and each edge stays in c.
			for i := 0; i < 12; i++ {
				a := float64(i) * math.Pi / 6
				reach := 0.97
				if i%2 == 1 { // odd: toward an edge midpoint, which is sqrt(3)/2 out
					reach = 0.85
				}
				px, py := x+math.Cos(a-math.Pi/2)*reach, y+math.Sin(a-math.Pi/2)*reach
				if got := skill.FromPixel(px, py); got != c {
					t.Fatalf("point %.2f out at %d*30deg from %v maps to %v", reach, i, c, got)
				}
			}
			// Just past each edge midpoint is the neighbor.
			for _, d := range skill.HexDirs {
				dx, dy := d.Pixel()
				if got := skill.FromPixel(x+dx*0.52, y+dy*0.52); got != c.Add(d) {
					t.Fatalf("past the edge toward %v from %v maps to %v", d, c, got)
				}
			}
		}
	}
}

func TestMoveAnyDirection(t *testing.T) {
	want := PlayerSpeed * tileSpan * 0.5
	for i := 0; i < 24; i++ {
		s := openArena(30, 30)
		p := s.P()
		x0, y0 := p.X, p.Y
		a := float64(i) * math.Pi / 12
		run(s, math.Cos(a), math.Sin(a), 0.5, nil)
		dx, dy := p.X-x0, p.Y-y0
		if got := math.Hypot(dx, dy); math.Abs(got-want) > want*0.01 {
			t.Fatalf("angle %d*15deg: moved %.3f, want %.3f", i, got, want)
		}
		if da := math.Abs(math.Remainder(math.Atan2(dy, dx)-a, 2*math.Pi)); da > 1e-6 {
			t.Fatalf("angle %d*15deg: went %.4f rad off course", i, da)
		}
		if math.Abs(p.FaceX-math.Cos(a)) > 1e-9 || math.Abs(p.FaceY-math.Sin(a)) > 1e-9 {
			t.Fatalf("angle %d*15deg: facing (%.3f, %.3f)", i, p.FaceX, p.FaceY)
		}
		if p.At != skill.FromPixel(p.X, p.Y) {
			t.Fatalf("tile %v doesn't match position", p.At)
		}
	}
}

func TestDiagonalKeysNotFaster(t *testing.T) {
	s := openArena(30, 30)
	p := s.P()
	x0, y0 := p.X, p.Y
	run(s, 1, 1, 0.5, nil) // D+S held together
	want := PlayerSpeed * tileSpan * 0.5
	if got := math.Hypot(p.X-x0, p.Y-y0); math.Abs(got-want) > want*0.01 {
		t.Fatalf("diagonal moved %.3f, want %.3f", got, want)
	}
	// Half an analog stick is half speed.
	s = openArena(30, 30)
	p = s.P()
	x0 = p.X
	run(s, 0.5, 0, 0.5, nil)
	if got := p.X - x0; math.Abs(got-want/2) > want*0.01 {
		t.Fatalf("half stick moved %.3f, want %.3f", got, want/2)
	}
}

func TestFacingFollowsMovement(t *testing.T) {
	s := openArena(30, 30)
	p := s.P()
	run(s, -1, 0, 0.1, nil)
	if p.Facing != 3 || p.FaceX > -0.999 {
		t.Fatalf("walking west: facing %d (%.2f, %.2f)", p.Facing, p.FaceX, p.FaceY)
	}
	run(s, 0, 0, 0.5, nil) // standing keeps the last facing
	if p.Facing != 3 || p.FaceX > -0.999 {
		t.Fatalf("stopped: facing %d (%.2f, %.2f)", p.Facing, p.FaceX, p.FaceY)
	}
}

// wallClear fails if the player's circle overlaps any wall hex.
func wallClear(t *testing.T, s *Sim, what string) {
	t.Helper()
	p := s.P()
	c := skill.FromPixel(p.X, p.Y)
	if s.Floor.At(c) == dungeon.Wall {
		t.Fatalf("%s: center inside wall %v", what, c)
	}
	for _, d := range append([]skill.Coord{{}}, skill.HexDirs[:]...) {
		h := c.Add(d)
		if s.Floor.At(h) != dungeon.Wall {
			continue
		}
		if dx, dy, hit := hexPush(h, p.X, p.Y, PlayerRadius-1e-6); hit {
			t.Fatalf("%s: overlapping wall %v by (%.4f, %.4f)", what, h, dx, dy)
		}
	}
}

func TestWallSlide(t *testing.T) {
	free := PlayerSpeed * tileSpan
	type wallCase struct {
		name   string
		wall   func(c, start skill.Coord) bool
		nx, ny float64 // into the wall
		ax, ay float64 // along it
	}
	walls := []wallCase{
		// A wall row above: a zigzag edge in pointy-top hexes.
		{"row above", func(c, st skill.Coord) bool { return c.R <= st.R-2 }, 0, -1, 1, 0},
		{"row below", func(c, st skill.Coord) bool { return c.R >= st.R+2 }, 0, 1, -1, 0},
		// A wall column to the east: a stepped vertical edge.
		{"column east", func(c, st skill.Coord) bool { return c.Q+c.R>>1 >= st.Q+st.R>>1+2 }, 1, 0, 0, 1},
		{"column west", func(c, st skill.Coord) bool { return c.Q+c.R>>1 <= st.Q+st.R>>1-2 }, -1, 0, 0, -1},
		// A wall along a hex axis at 60 degrees.
		{"diagonal", func(c, st skill.Coord) bool { return c.Q-st.Q >= 2 }, math.Sqrt(3) / 2, -0.5, -0.5, -math.Sqrt(3) / 2},
	}
	// How hard the stick pushes into the wall while heading along it, and
	// the least share of full speed the slide must keep.
	pushes := []struct{ into, keep float64 }{{0.15, 0.85}, {1, 0.6}}
	type tcase struct {
		wallCase
		mx, my, keep float64
	}
	var cases []tcase
	for _, w := range walls {
		for _, pu := range pushes {
			cases = append(cases, tcase{w, w.ax + w.nx*pu.into, w.ay + w.ny*pu.into, pu.keep})
		}
	}
	for _, tc := range cases {
		s := openArena(40, 40)
		st := s.P().At
		for i := range s.Floor.Tiles {
			if tc.wall(s.Floor.CoordOf(i), st) {
				s.Floor.Tiles[i] = dungeon.Wall
			}
		}
		s.refreshField()
		p := s.P()
		// Walk into the wall first.
		run(s, tc.nx, tc.ny, 0.6, func() { wallClear(t, s, tc.name) })
		x0, y0 := p.X, p.Y
		const secs = 1.5
		run(s, tc.mx, tc.my, secs, func() { wallClear(t, s, tc.name) })
		along := (p.X-x0)*tc.ax + (p.Y-y0)*tc.ay
		// The stick's along-wall share at full speed, sliding perfectly.
		ideal := free * secs * (tc.mx*tc.ax + tc.my*tc.ay) / math.Hypot(tc.mx, tc.my)
		t.Logf("%-11s push %.2f: slid %.2f of %.2f (%.0f%%)", tc.name, math.Hypot(tc.mx-tc.ax, tc.my-tc.ay), along, ideal, 100*along/ideal)
		if along < ideal*tc.keep {
			t.Errorf("%s: snagged, slid %.2f of %.2f", tc.name, along, ideal)
		}
		// Head-on into the wall is a stop. The body may settle into the
		// nearest notch of a stepped wall, but it must not keep sliding.
		run(s, tc.nx, tc.ny, 1, func() { wallClear(t, s, tc.name) })
		x0, y0 = p.X, p.Y
		run(s, tc.nx, tc.ny, 2, func() { wallClear(t, s, tc.name) })
		if drift := math.Hypot(p.X-x0, p.Y-y0); drift > 0.05 {
			t.Errorf("%s: pushing straight in kept drifting, %.2f", tc.name, drift)
		}
	}
}

func TestDoorways(t *testing.T) {
	// A wall row with a one-hex gap; walk straight up at it from off
	// center. The gap's corners should steer the body in, not stop it.
	for _, off := range []float64{0, 0.3, -0.3, 0.6, -0.6, 0.75, -0.75} {
		s := openArena(30, 30)
		p := s.P()
		st := p.At
		gap := skill.Coord{Q: st.Q + 1, R: st.R - 2}
		for i := range s.Floor.Tiles {
			c := s.Floor.CoordOf(i)
			if c.R == gap.R && c != gap {
				s.Floor.Tiles[i] = dungeon.Wall
			}
		}
		gx, _ := gap.Pixel()
		p.X = gx + off
		s.collide(p)
		s.settle(p.X, p.Y)
		run(s, 0, -1, 1, func() { wallClear(t, s, "doorway") })
		_, gy := gap.Pixel()
		if p.Y > gy-1.5 {
			t.Errorf("offset %.1f: stuck at y %.2f, gap row at %.2f", off, p.Y, gy)
		}
	}
}

func TestNeverInsideWalls(t *testing.T) {
	for seed := uint64(1); seed <= 6; seed++ {
		s := New(seed)
		s.load(4, 1)
		p := s.P()
		p.U.MaxHP, p.U.HP = 1e9, 1e9
		var x uint64 = seed
		for frame := 0; frame < 60*40; frame++ {
			if frame%20 == 0 {
				x = hash(x)
				a := float64(x%3600) / 3600 * 2 * math.Pi
				s.Input = Input{MoveX: math.Cos(a), MoveY: math.Sin(a)}
			}
			s.Step(1.0 / 60)
			if s.State != Playing {
				s.load(4, 1)
				p = s.P()
				p.U.MaxHP, p.U.HP = 1e9, 1e9
				continue
			}
			wallClear(t, s, "random walk")
			if s.occ[s.Floor.Index(p.At)] != s.Player {
				t.Fatalf("seed %d frame %d: player's tile is claimed by %d", seed, frame, s.occ[s.Floor.Index(p.At)])
			}
			for i := range s.Actors {
				a := &s.Actors[i]
				if skill.EntityID(i) != s.Player && a.U.Alive && a.At == p.At {
					t.Fatalf("seed %d frame %d: sharing a tile with actor %d", seed, frame, i)
				}
			}
		}
	}
}

func TestEnemyBodyBlocks(t *testing.T) {
	s := openArena(30, 30)
	p := s.P()
	east := p.At.Add(skill.HexDirs[0])
	foe := s.spawn(KindGrunt, 1, east, 1, 1e6)
	s.Actors[foe].U.StunLeft = 10 // hold still
	run(s, 1, 0.05, 1, func() {
		ex, ey := s.Actors[foe].At.Pixel()
		if d := math.Hypot(p.X-ex, p.Y-ey); d < PlayerRadius+BodyRadius-1e-6 {
			t.Fatalf("walked into the grunt: %.3f from its center", d)
		}
		if p.At == s.Actors[foe].At {
			t.Fatal("sharing the grunt's tile")
		}
	})
}

func TestWalkIntoPit(t *testing.T) {
	s := openArena(30, 30)
	p := s.P()
	pit := p.At.Add(skill.HexDirs[0]).Add(skill.HexDirs[0])
	s.Floor.Set(pit, dungeon.Pit)
	hp := p.U.HP
	fell := false
	s.Input = Input{MoveX: 1}
	for i := 0; i < 60 && !fell; i++ {
		s.Step(1.0 / 60)
		s.FX.Each(func(f *FX) {
			if f.Kind == FXFall && f.Hurt && f.At == pit {
				fell = true
			}
		})
	}
	if !fell || !p.U.Alive {
		t.Fatalf("fell %v alive %v", fell, p.U.Alive)
	}
	if want := hp - PitFallFrac*p.U.MaxHP; math.Abs(float64(p.U.HP-want)) > 1 {
		t.Fatalf("hull %.0f after the fall, want %.0f", p.U.HP, want)
	}
	if s.Floor.At(p.At) == dungeon.Pit || p.U.StunLeft <= 0 {
		t.Fatalf("on %v after the fall, stun %.2f", p.At, p.U.StunLeft)
	}
	// The ledge itself is safe: the circle may hang over the pit as long
	// as the center stays out.
	s = openArena(30, 30)
	p = s.P()
	s.Floor.Set(p.At.Add(skill.HexDirs[0]), dungeon.Pit)
	cx := p.X
	p.X = cx + 0.8 // inside the start tile, circle over the pit's edge
	hp = p.U.HP
	run(s, 0, 0, 0.5, nil)
	if p.U.HP != hp {
		t.Fatal("standing at the ledge hurt")
	}
}

func TestHazardUnderCenter(t *testing.T) {
	s := openArena(30, 30)
	p := s.P()
	vent := p.At.Add(skill.HexDirs[0])
	s.Floor.Set(vent, dungeon.Hazard)
	hp := p.U.HP
	p.X += 0.75 // circle over the vent, center still on our tile
	run(s, 0, 0, 1.1, nil)
	if p.U.HP != hp {
		t.Fatal("vent burned before the center was on it")
	}
	run(s, 1, 0, 0.2, nil)
	run(s, 0, 0, 1.1, nil)
	if p.At != vent || p.U.HP >= hp {
		t.Fatalf("on %v, hull %.0f of %.0f: vent didn't burn", p.At, p.U.HP, hp)
	}
}

func TestBehindUsesTrueFacing(t *testing.T) {
	s := openArena(30, 30)
	p := s.P()
	west := s.spawn(KindGrunt, 1, p.At.Add(skill.HexDirs[3]), 1, 100)
	east := s.spawn(KindGrunt, 1, p.At.Add(skill.HexDirs[0]), 1, 100)
	for i := range s.Actors {
		s.Actors[i].U.StunLeft = 10
	}
	run(s, 1, 0, 1.0/60, nil) // face east
	if !s.Behind(west, s.Player) || s.Behind(east, s.Player) {
		t.Fatalf("facing east: west behind %v, east behind %v", s.Behind(west, s.Player), s.Behind(east, s.Player))
	}
}

func TestConeUsesTrueFacing(t *testing.T) {
	s := openArena(30, 30)
	p := s.P()
	// A foe 30 degrees below east. Facing 29 degrees above east is inside
	// the hex east direction's cone but 59 degrees off our real facing.
	foe := s.spawn(KindGrunt, 1, p.At.Add(skill.Coord{Q: 2, R: -1}), 1, 100)
	a := 29 * math.Pi / 180
	p.FaceX, p.FaceY = math.Cos(a), math.Sin(a)
	p.Facing = dirOf(p.FaceX, p.FaceY)
	if p.Facing != 0 {
		t.Fatalf("29deg should round to east, got %d", p.Facing)
	}
	var out [8]skill.EntityID
	if n := s.Select(skill.TargetCone, s.Player, s.Player, 3, out[:]); n != 0 {
		t.Fatalf("cone facing -29deg picked %v", out[:n])
	}
	a = -25 * math.Pi / 180
	p.FaceX, p.FaceY = math.Cos(a), math.Sin(a)
	if n := s.Select(skill.TargetCone, s.Player, s.Player, 3, out[:]); n != 1 || out[0] != foe {
		t.Fatalf("cone facing the foe picked %v", out[:n])
	}
}

func TestStepAllocFree(t *testing.T) {
	s := New(3)
	s.load(10, 1)
	p := s.P()
	p.U.MaxHP, p.U.HP = 1e9, 1e9
	for i := range s.Actors {
		s.Actors[i].Awake = true
	}
	k := 0
	allocs := testing.AllocsPerRun(600, func() {
		k++
		a := float64(k/30) * 1.3
		s.Input = Input{MoveX: math.Cos(a), MoveY: math.Sin(a), Cast: [MaxSlots]bool{k%45 == 0}}
		s.Step(1.0 / 60)
	})
	if allocs != 0 || s.State != Playing {
		t.Fatalf("%.2f allocs per step (state %d)", allocs, s.State)
	}
}

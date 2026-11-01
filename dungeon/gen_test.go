package dungeon

import (
	"hash/fnv"
	"testing"

	"hexes/skill"
)

func hashFloor(f *Floor) uint64 {
	h := fnv.New64a()
	for _, t := range f.Tiles {
		h.Write([]byte{byte(t)})
	}
	for _, s := range f.Spawns {
		h.Write([]byte{byte(s.At.Q), byte(s.At.R), byte(s.Kind), byte(s.Level)})
	}
	return h.Sum64()
}

func TestDeterministic(t *testing.T) {
	var g Generator
	for depth := 1; depth <= 40; depth += 3 {
		a := Generate(42, depth)
		b := g.Generate(42, depth) // reused buffers must not change the result
		c := g.Generate(42, depth)
		if hashFloor(a) != hashFloor(b) || hashFloor(b) != hashFloor(c) {
			t.Fatalf("depth %d: same seed gave different floors", depth)
		}
		if hashFloor(a) == hashFloor(Generate(43, depth)) {
			t.Fatalf("depth %d: seeds 42 and 43 gave the same floor", depth)
		}
	}
}

// TestFloorsAreValid checks the guarantees the game relies on across many
// seeds and depths: walled border, start and stairs connected, every room
// reachable, spawns on reachable open floor away from the start.
func TestFloorsAreValid(t *testing.T) {
	var g Generator
	for depth := 1; depth <= 60; depth++ {
		for seed := uint64(1); seed <= 60; seed++ {
			f := g.Generate(seed*7919, depth)
			check(t, f)
		}
	}
}

func check(t *testing.T, f *Floor) {
	t.Helper()
	where := func() string { return "seed " + itoa(int(f.Seed)) + " depth " + itoa(f.Depth) }
	for i, tile := range f.Tiles {
		c := f.CoordOf(i)
		if !interior(f, c) && tile != Wall {
			t.Fatalf("%s: border tile %v is %s", where(), c, tile)
		}
		if f.Index(c) != i {
			t.Fatalf("%s: index round trip broke at %d", where(), i)
		}
	}
	if f.At(f.Exit) != Stairs {
		t.Fatalf("%s: exit is %s", where(), f.At(f.Exit))
	}
	if f.At(f.Start) != Ground {
		t.Fatalf("%s: start is %s", where(), f.At(f.Start))
	}
	dist := make([]int32, len(f.Tiles))
	q := make([]int32, len(f.Tiles))
	f.DistanceField(f.Start, dist, q)
	if dist[f.Index(f.Exit)] < 0 {
		t.Fatalf("%s: stairs unreachable", where())
	}
	if len(f.Rooms) > 1 && f.Start == f.Exit {
		t.Fatalf("%s: stairs on the start", where())
	}
	for _, r := range f.Rooms {
		if dist[f.Index(r.Center)] < 0 {
			t.Fatalf("%s: room at %v unreachable", where(), r.Center)
		}
	}
	p := ParamsFor(f.Depth)
	seen := map[skill.Coord]bool{}
	for _, s := range f.Spawns {
		if f.At(s.At) != Ground {
			t.Fatalf("%s: spawn on %s", where(), f.At(s.At))
		}
		if d := dist[f.Index(s.At)]; d < int32(p.SafeRadius) {
			t.Fatalf("%s: spawn %d steps from start", where(), d)
		}
		if seen[s.At] {
			t.Fatalf("%s: two spawns on %v", where(), s.At)
		}
		seen[s.At] = true
		if s.Level != p.EnemyLevel && s.Level != p.EnemyLevel+2 {
			t.Fatalf("%s: spawn level %d", where(), s.Level)
		}
	}
}

func TestDepthScales(t *testing.T) {
	avg := func(depth int) (size, enemies, pits float64) {
		for seed := uint64(1); seed <= 30; seed++ {
			f := Generate(seed, depth)
			size += float64(len(f.Tiles))
			enemies += float64(len(f.Spawns))
			pits += float64(f.Count(Pit))
		}
		return size / 30, enemies / 30, pits / 30
	}
	s1, e1, p1 := avg(1)
	s20, e20, p20 := avg(20)
	if !(s20 > s1 && e20 > 2*e1 && p20 > p1) {
		t.Fatalf("depth 20 should be bigger and busier: size %.0f->%.0f enemies %.1f->%.1f pits %.1f->%.1f",
			s1, s20, e1, e20, p1, p20)
	}
	if Generate(1, 1).Count(Hazard) != 0 {
		t.Fatal("depth 1 should have no hazards")
	}
	if Generate(1, 12).Count(Hazard) == 0 {
		t.Fatal("depth 12 should have hazards")
	}
}

// Pits matter in combat because they sit next to where foes stand: most
// pits should border open floor.
func TestPitsAreReachable(t *testing.T) {
	f := Generate(9, 10)
	edge := 0
	total := 0
	for i, tile := range f.Tiles {
		if tile != Pit {
			continue
		}
		total++
		c := f.CoordOf(i)
		for _, d := range skill.HexDirs {
			if f.At(c.Add(d)).Walkable() {
				edge++
				break
			}
		}
	}
	if total == 0 || edge*10 < total*9 {
		t.Fatalf("%d of %d pits border walkable ground", edge, total)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

func BenchmarkGenerate(b *testing.B) {
	for _, depth := range []int{1, 10, 50, 200} {
		b.Run("depth"+itoa(depth), func(b *testing.B) {
			var g Generator
			g.Generate(1, depth) // warm the scratch buffers
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				g.Generate(uint64(i), depth)
			}
		})
	}
}

func BenchmarkDistanceField(b *testing.B) {
	f := Generate(1, 20)
	dist := make([]int32, len(f.Tiles))
	q := make([]int32, len(f.Tiles))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		f.DistanceField(f.Start, dist, q)
	}
}

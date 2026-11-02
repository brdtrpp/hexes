package skill

import (
	"encoding/json"
	"testing"
)

// genHex puts a generated (or hand-built) graph on a hex with wild
// sockets and the trigger socket at its entry.
func genHex(t testing.TB, g Graph, parts map[int]Part) *Hex {
	t.Helper()
	h := NewHex("Gen", [RingSlots]SocketKind{W, W, W, W, W, W})
	for i := range h.Sockets {
		h.Sockets[i] = W
	}
	h.Shape = Shape{Kind: ShapeGen, Net: g}
	h.Sockets[g.Entry] = T
	for i, p := range parts {
		if err := h.Set(i, p); err != nil {
			t.Fatal(err)
		}
	}
	return h
}

// link builds a graph from cells and links by hand, with two-way links
// where the generator would put them.
func link(cells []Cell, links ...[2]int) Graph {
	var g Graph
	g.N = uint8(len(cells))
	copy(g.At[:], cells)
	for _, l := range links {
		g.Out[l[0]] |= bit(l[1])
	}
	for a := 0; a < int(g.N); a++ {
		if g.Outs(a) == 1 {
			for b := 0; b < int(g.N); b++ {
				if g.Out[a]&bit(b) != 0 && g.Ins(b) == 1 {
					g.Two[a] = g.Out[a]
				}
			}
		}
	}
	return g
}

func TestGenGraphsAreValidAtEveryDepth(t *testing.T) {
	r := splitmix{42}
	for depth := 1; depth <= 80; depth++ {
		for i := 0; i < 200; i++ {
			g := GenGraph(r.next, depth)
			if !g.Valid() {
				t.Fatalf("depth %d: invalid graph %+v", depth, g)
			}
			if int(g.N) < GenMinSlots || int(g.N) > GenMaxSlots(depth) {
				t.Fatalf("depth %d: %d slots, want %d..%d", depth, g.N, GenMinSlots, GenMaxSlots(depth))
			}
			// Slots are numbered in the order they run.
			order, n := g.Flow(0)
			for k := 0; k < n; k++ {
				if order[k] != k {
					t.Fatalf("depth %d: flow %v not in slot order", depth, order[:n])
				}
			}
		}
	}
}

func TestDeeperShapesAreBiggerAndStranger(t *testing.T) {
	stats := func(depth int) (slots, splits, merges, loops float64) {
		r := splitmix{uint64(depth)}
		const n = 2000
		for i := 0; i < n; i++ {
			g := GenGraph(r.next, depth)
			slots += float64(g.N)
			splits += float64(g.Splits())
			merges += float64(g.Merges())
			if g.Loop > 0 {
				loops++
			}
		}
		return slots / n, splits / n, merges / n, loops / n
	}
	s5, sp5, m5, l5 := stats(5)
	s40, sp40, m40, l40 := stats(40)
	if s40 <= s5+1.5 || sp40 <= sp5 || m40 <= m5 || l40 <= l5 {
		t.Fatalf("depth 5: %.2f slots %.2f splits %.2f merges %.2f loops; depth 40: %.2f %.2f %.2f %.2f",
			s5, sp5, m5, l5, s40, sp40, m40, l40)
	}
	if GenMaxSlots(60) != Slots {
		t.Fatal("deep shapes never reach the full nine slots")
	}
}

func TestGenIsDeterministic(t *testing.T) {
	a, b := splitmix{9}, splitmix{9}
	for i := 0; i < 50; i++ {
		if GenGraph(a.next, 20) != GenGraph(b.next, 20) {
			t.Fatal("same seed, different graph")
		}
	}
}

func TestGenHeatComesFromStructure(t *testing.T) {
	chain6 := Shape{Kind: ShapeGen, Net: link([]Cell{{0, 0}, {1, 0}, {2, -1}, {2, 0}, {1, 1}, {0, 1}},
		[2]int{0, 1}, [2]int{1, 2}, [2]int{2, 3}, [2]int{3, 4}, [2]int{4, 5})}
	if !chain6.Valid() {
		t.Fatal("chain invalid")
	}
	approx(t, "six-slot chain heat", chain6.HeatScale(), ShapeRing.HeatScale())
	short := Shape{Kind: ShapeGen, Net: link([]Cell{{0, 0}, {1, 0}, {1, 1}}, [2]int{0, 1}, [2]int{1, 2})}
	approx(t, "three-slot chain heat", short.HeatScale(), GenHeatBase+3*GenHeatSlot)
	// Bigger and busier always costs more: no free lunch.
	r := splitmix{3}
	for i := 0; i < 500; i++ {
		s := Shape{Kind: ShapeGen, Net: GenGraph(r.next, 40)}
		g := &s.Net
		want := GenHeatBase + GenHeatSlot*float32(g.N) + GenHeatSplit*float32(g.Splits()) + GenHeatMerge*float32(g.Merges())
		approx(t, "heat", s.HeatScale(), want)
		if g.N >= 6 && s.HeatScale() < 1 {
			t.Fatalf("%v: %d slots run cooler than a Ring", s, g.N)
		}
	}
}

func TestGenNames(t *testing.T) {
	cells := []Cell{{0, 0}, {1, 0}, {1, 1}, {0, 1}}
	for _, tc := range []struct {
		g    Graph
		want string
	}{
		{link(cells[:3], [2]int{0, 1}, [2]int{1, 2}), "Short Chain"},
		{link(cells, [2]int{0, 1}, [2]int{1, 2}, [2]int{0, 3}), "Fork"},
		{link(cells, [2]int{0, 1}, [2]int{1, 2}, [2]int{0, 3}, [2]int{3, 2}), "Hub"},
	} {
		if got := (Shape{Kind: ShapeGen, Net: tc.g}).Name(); got != tc.want {
			t.Errorf("name %q, want %q", got, tc.want)
		}
	}
	loop := link(cells, [2]int{0, 1}, [2]int{1, 2}, [2]int{2, 3})
	loop.Loop = 4
	if got := (Shape{Kind: ShapeGen, Net: loop}).Name(); got != "Looping Chain" {
		t.Errorf("loop name %q", got)
	}
}

// A generated merge pools both branches' targets, once each, and a
// multiplier there boosts both branches.
func TestGenMergePools(t *testing.T) {
	a, c := world()
	f1 := a.Spawn(1, Coord{3, 6}, 10, 1000)
	f2 := a.Spawn(1, Coord{9, 6}, 10, 1000)
	//  0 -> 1 -> 3
	//  0 -> 2 -> 3   (3 is the merge)
	g := link([]Cell{{0, 0}, {1, -1}, {0, 1}, {1, 0}}, [2]int{0, 1}, [2]int{0, 2}, [2]int{1, 3}, [2]int{2, 3})
	if !g.Valid() {
		t.Fatal("merge graph invalid")
	}
	h := genHex(t, g, map[int]Part{
		0: NewTrigger(OnCast),
		1: NewTarget(TargetAdjacent, 1, 0), // f1
		2: NewTarget(TargetNearest, 0, 2),  // f1, f2
		3: NewDamage(id("kinetic"), 10),
	})
	p := compile(t, h, 0)
	res := run(t, p, a, c, NoEntity, nil, nil)
	if res.Hits != 2 || a.Unit(f1).HP == 1000 || a.Unit(f2).HP == 1000 {
		t.Fatalf("merge hit %d\n%s", res.Hits, p)
	}
	if what, ok := h.Use(3); !ok || what != "hits everyone slots 2 and 3 target, once each" {
		t.Fatalf("use %q %v", what, ok)
	}
	amp := genHex(t, g, map[int]Part{
		0: NewTrigger(OnCast),
		1: NewDamage(id("kinetic"), 10),
		2: NewDamage(id("kinetic"), 20),
		3: NewMult(VerbDamage, 2),
	})
	ds := steps(compile(t, amp, 0), OpDamage)
	if len(ds) != 2 || ds[0].Value != 20 || ds[1].Value != 40 {
		t.Fatalf("merge multiplier: %+v", ds)
	}
	if r := amp.Routes(0); r[3].Gate >= 0 {
		t.Fatal("merge routed by a gate")
	}
}

// Two branches out of a split run one after the other and don't read
// each other's targets.
func TestGenSplitKeepsBranchesApart(t *testing.T) {
	a, c := world()
	near := a.Spawn(1, Coord{3, 6}, 10, 1000)
	far := a.Spawn(1, Coord{9, 6}, 10, 1000)
	//  0 -> 1 -> 2
	//  0 -> 3
	g := link([]Cell{{0, 0}, {1, -1}, {2, -1}, {0, 1}}, [2]int{0, 1}, [2]int{1, 2}, [2]int{0, 3})
	h := genHex(t, g, map[int]Part{
		0: NewTrigger(OnCast),
		1: NewDamage(id("plasma"), 10),     // reads slot 2 back along its chain
		2: NewTarget(TargetAdjacent, 1, 0), // the near foe
		3: NewDamage(id("cryo"), 10),       // other branch: the subject only
	})
	p := compile(t, h, 0)
	res := run(t, p, a, c, far, nil, nil)
	if res.Hits != 2 || a.Unit(near).HP == 1000 || a.Unit(far).HP == 1000 {
		t.Fatalf("split hits %d near %.0f far %.0f\n%s", res.Hits, a.Unit(near).HP, a.Unit(far).HP, p)
	}
	if h.Feeds(1, 3) || h.Feeds(3, 1) || h.Feeds(2, 3) || !h.Feeds(2, 1) || h.Feeds(1, 0) {
		t.Fatal("split wiring")
	}
}

func TestGenLoopRunsAgain(t *testing.T) {
	cells := []Cell{{0, 0}, {1, 0}, {1, 1}}
	g := link(cells, [2]int{0, 1}, [2]int{1, 2})
	g.Loop = 3
	h := genHex(t, g, map[int]Part{
		0: NewTrigger(OnCast),
		1: NewCharge(0, 1),
		2: NewSpendGate(0, 1),
	})
	p := compile(t, h, 0)
	if !p.Loops || !p.CallsHex(CallSelf) {
		t.Fatalf("loop not wired\n%s", p)
	}
}

func TestGenShapeRoundTrips(t *testing.T) {
	r := splitmix{5}
	s := RollShape(r.next, 99, 30)
	h := genHex(t, s.Net, map[int]Part{0: NewTrigger(OnCast)})
	h.Shape = s
	b, err := json.Marshal(h)
	if err != nil {
		t.Fatal(err)
	}
	var back Hex
	if err := json.Unmarshal(b, &back); err != nil || back.Shape != s {
		t.Fatalf("round trip: %v\n%+v\n%+v", err, back.Shape, s)
	}
	// An original shape doesn't carry a graph in its save.
	b, _ = json.Marshal(Shape{Kind: ShapeHub, ID: 3})
	if string(b) != `{"Kind":2,"ID":3}` {
		t.Fatalf("hub saves as %s", b)
	}
}

func BenchmarkGenGraph(b *testing.B) {
	r := splitmix{1}
	for b.Loop() {
		GenGraph(r.next, 40)
	}
}

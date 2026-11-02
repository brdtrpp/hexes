package skill

import "testing"

// module is a hex with no trigger: wild sockets everywhere.
func module(t testing.TB, name string, sh Shape, parts map[int]Part) *Hex {
	t.Helper()
	h := NewHex(name, [RingSlots]SocketKind{W, W, W, W, W, W})
	for i := range h.Sockets {
		h.Sockets[i] = W
	}
	h.Shape = sh
	for i, p := range parts {
		if err := h.Set(i, p); err != nil {
			t.Fatal(err)
		}
	}
	return h
}

func TestRingPorts(t *testing.T) {
	ps, n := Shape{}.Ports()
	want := []Port{{0, 0, true}, {5, 5, false}, {4, 4, false}}
	if n != len(want) {
		t.Fatalf("ring ports %v", ps[:n])
	}
	for i, p := range want {
		if ps[i] != p {
			t.Fatalf("ring ports %v, want %v", ps[:n], want)
		}
	}
}

// Every shape, original or generated, has one way in, a way out or two,
// each on its own open side.
func TestEveryShapeHasPorts(t *testing.T) {
	check := func(s Shape) {
		t.Helper()
		ps, n := s.Ports()
		g := s.Graph()
		ins, outs := 0, 0
		sides := 0
		for _, p := range ps[:n] {
			if p.In {
				ins++
			} else {
				outs++
				if int(p.Slot) == int(g.Entry) {
					t.Fatalf("%v: a way out on the start", s)
				}
			}
			if sides&(1<<p.Side) != 0 {
				t.Fatalf("%v: two ports face %s", s, SideName(int(p.Side)))
			}
			sides |= 1 << p.Side
			c := g.At[p.Slot].Add(CellDirs[p.Side])
			for i := 0; i < int(g.N); i++ {
				if g.At[i] == c {
					t.Fatalf("%v: port on slot %d faces its own slot %d", s, p.Slot+1, i+1)
				}
			}
		}
		if ins != 1 || outs < 1 || (g.N >= 5 && outs < 2) {
			t.Fatalf("%v: %d in, %d out: %v", s, ins, outs, ps[:n])
		}
	}
	for k := ShapeRing; k < ShapeGen; k++ {
		check(Shape{Kind: k})
	}
	r := splitmix{7}
	for i := 0; i < 2000; i++ {
		check(RollShape(r.next, 1+r.next()%1000, 5+i%60))
	}
}

// A docked hex runs on whoever the port slot carries, and its parts pay
// their own Heat at their own shape's scale.
func TestDockFlowsAcross(t *testing.T) {
	a, c := world()
	f1 := a.Spawn(1, Coord{3, 6}, 10, 1000)
	f2 := a.Spawn(1, Coord{2, 5}, 10, 1000)
	far := a.Spawn(1, Coord{9, 6}, 10, 1000)
	ring := shaped(t, ShapeRing, map[int]Part{
		0: NewTrigger(OnCast),
		5: NewTarget(TargetAdjacent, 1, 0), // the way out: f1 and f2
	})
	tri := module(t, "Module A", Shape{Kind: ShapeTriangle}, map[int]Part{0: NewDamage(id("plasma"), 10)})
	alone := compile(t, ring, 0)
	if r := run(t, alone, a, c, far, nil, nil); r.Hits != 0 {
		t.Fatal("undocked ring hit someone")
	}
	if what, _ := ring.Use(5); what != "picks targets: put an effect next to it" {
		t.Fatalf("undocked port use %q", what)
	}
	ring.Docks[5] = tri
	p := compile(t, ring, 0)
	res := run(t, p, a, c, far, nil, nil)
	if res.Hits != 2 || a.Unit(f1).HP == 1000 || a.Unit(f2).HP == 1000 || a.Unit(far).HP != 1000 {
		t.Fatalf("docked hits %d\n%s", res.Hits, p)
	}
	if p.Docked != 1 {
		t.Fatalf("docked %d", p.Docked)
	}
	if what, ok := ring.Use(5); !ok || what != "its targets flow on into Module A" {
		t.Fatalf("port use %q %v", what, ok)
	}
	if what, _ := tri.Use(0); what != "hits whoever docks in (no target next to it)" {
		t.Fatalf("module use %q", what)
	}
	m, err := CompileModule(tri, reg, 0)
	if err != nil {
		t.Fatal(err)
	}
	if m.Heat <= 0 {
		t.Fatal("module costs no heat")
	}
	approx(t, "docked heat", p.Heat, alone.Heat+m.Heat)
	approx(t, "docked power", p.Power, alone.Power+m.Power)
}

// A dock only follows a way out: a dock on any other slot does nothing.
func TestDockNeedsAWayOut(t *testing.T) {
	ring := shaped(t, ShapeRing, map[int]Part{0: NewTrigger(OnCast)})
	ring.Docks[2] = module(t, "M", Shape{Kind: ShapeTriangle}, map[int]Part{0: NewDamage(id("plasma"), 10)})
	if p := compile(t, ring, 0); p.Docked != 0 || len(steps(p, OpDamage)) != 0 {
		t.Fatal("docked off a slot with no port")
	}
}

// Chains stop where they come back round, never loop a docked hex, and
// run at most MaxDocked hexes.
func TestDockChainsStop(t *testing.T) {
	a := shaped(t, ShapeRing, map[int]Part{0: NewTrigger(OnCast), 1: NewDamage(id("plasma"), 10)})
	b := module(t, "B", Shape{Kind: ShapeLoop}, map[int]Part{
		0: NewDamage(id("kinetic"), 10),
		5: NewTarget(TargetAdjacent, 1, 0),
	})
	a.Docks[5] = b
	b.Docks[5] = a // back round
	p := compile(t, a, 0)
	if p.Docked != 1 || p.Loops || len(steps(p, OpDamage)) != 2 {
		t.Fatalf("docked %d loops %v\n%s", p.Docked, p.Loops, p)
	}
	// A long chain of modules.
	prev := a
	prev.Docks[5] = nil
	for i := 0; i < MaxDocked+4; i++ {
		m := module(t, "M", Shape{}, map[int]Part{0: NewDamage(id("plasma"), 1)})
		prev.Docks[5] = m
		prev = m
	}
	if p := compile(t, a, 0); int(p.Docked) != MaxDocked || len(steps(p, OpDamage)) != 1+MaxDocked {
		t.Fatalf("long chain ran %d", p.Docked)
	}
}

func TestModuleRefusesATrigger(t *testing.T) {
	m := module(t, "M", Shape{Kind: ShapeTriangle}, nil)
	m.Sockets[0] = SocketTrigger
	_ = m.Set(0, NewTrigger(OnCast))
	if _, err := CompileModule(m, reg, 0); err == nil {
		t.Fatal("module with a trigger compiled")
	}
}

func BenchmarkCompileDocked(b *testing.B) {
	h := shaped(b, ShapeRing, map[int]Part{0: NewTrigger(OnCast), 5: NewTarget(TargetAdjacent, 1, 0)})
	prev := h
	for i := 0; i < 6; i++ {
		m := module(b, "M", Shape{}, map[int]Part{0: NewDamage(id("plasma"), 10), 1: NewMult(VerbDamage, 2), 5: NewTarget(TargetNearest, 0, 3)})
		prev.Docks[5] = m
		prev = m
	}
	for b.Loop() {
		if _, err := Compile(h, reg, 0); err != nil {
			b.Fatal(err)
		}
	}
}

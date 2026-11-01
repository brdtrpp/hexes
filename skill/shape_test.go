package skill

import (
	"encoding/json"
	"strings"
	"testing"
)

// shaped makes a hex of a shape with all-wild sockets (the trigger socket
// where the shape wants it, the center taking effects) and puts parts in
// the given slots.
func shaped(t testing.TB, k ShapeKind, parts map[int]Part) *Hex {
	t.Helper()
	h := NewHex(k.String(), [RingSlots]SocketKind{W, W, W, W, W, W})
	h.Shape = Shape{Kind: k}
	if k == ShapeFork {
		h.Sockets[Center] = SocketTrigger
	} else {
		h.Sockets[0] = SocketTrigger
	}
	for i, p := range parts {
		if err := h.Set(i, p); err != nil {
			t.Fatal(err)
		}
	}
	return h
}

func run(t testing.TB, p *Program, a *Arena, c, subject EntityID, bank *Bank, rig *Rig) Result {
	t.Helper()
	return NewEvaluator(reg, DefaultRules()).Run(p, a, Ctx{Caster: c, Subject: subject, Level: 10, Bank: bank, Rig: rig})
}

func TestShapeTopology(t *testing.T) {
	for _, tc := range []struct {
		k          ShapeKind
		len, edges int
		center     bool
	}{
		{ShapeRing, 6, 6, false},
		{ShapeTriangle, 3, 2, false},
		{ShapeHub, 7, 6, true},
		{ShapeFork, 7, 4, true},
		{ShapeLoop, 6, 6, false},
	} {
		edges := 0
		for e := 0; e < RingSlots; e++ {
			if tc.k.HasEdge(e) {
				edges++
			}
		}
		if tc.k.Len() != tc.len || edges != tc.edges || tc.k.HasCenter() != tc.center {
			t.Errorf("%s: len %d edges %d center %v, want %d %d %v",
				tc.k, tc.k.Len(), edges, tc.k.HasCenter(), tc.len, tc.edges, tc.center)
		}
	}
}

func TestTriangleIsALine(t *testing.T) {
	a, c := world()
	near := a.Spawn(1, Coord{3, 6}, 10, 1000)
	far := a.Spawn(1, Coord{8, 6}, 10, 1000)
	// Target in slot 1, damage in slot 2: the damage reads its neighbor.
	h := shaped(t, ShapeTriangle, map[int]Part{
		0: NewTrigger(OnCast),
		1: NewTarget(TargetAdjacent, 1, 0),
		2: NewDamage(id("plasma"), 10),
	})
	p := compile(t, h, 0)
	res := run(t, p, a, c, far, nil, nil)
	if res.Hits != 1 || a.Unit(near).HP == 1000 || a.Unit(far).HP != 1000 {
		t.Fatalf("triangle hit %d, near %.0f far %.0f\n%s", res.Hits, a.Unit(near).HP, a.Unit(far).HP, p)
	}
	// Slot 2 doesn't touch slot 0 across the missing edge, so damage in
	// slot 1 with a target in slot 2 still lands on the target (its cw
	// neighbor) but a target can never sit "before" the trigger.
	if h.Feeds(2, 0) || h.Feeds(0, 2) {
		t.Fatal("triangle closes its line")
	}
	if err := h.Set(3, NewDamage(id("plasma"), 1)); err != nil {
		t.Fatal(err)
	}
	if _, err := Compile(h, reg, 0); err == nil || !strings.Contains(err.Error(), "isn't part of a Triangle") {
		t.Fatalf("part outside a triangle compiled: %v", err)
	}
}

func TestTriangleRunsCool(t *testing.T) {
	parts := map[int]Part{0: NewTrigger(OnCast), 1: NewDamage(id("plasma"), 10)}
	ring := compile(t, shaped(t, ShapeRing, parts), 0)
	tri := compile(t, shaped(t, ShapeTriangle, parts), 0)
	approx(t, "triangle heat", tri.Heat, ring.Heat*0.75)
	if tri.Power != ring.Power {
		t.Fatalf("triangle power %.2f, ring %.2f", tri.Power, ring.Power)
	}
}

func TestHubPoolsTheRing(t *testing.T) {
	a, c := world()
	// Two foes next to the caster, one far off by itself.
	f1 := a.Spawn(1, Coord{3, 6}, 10, 1000)
	f2 := a.Spawn(1, Coord{2, 5}, 10, 1000)
	f3 := a.Spawn(1, Coord{9, 6}, 10, 1000)
	h := shaped(t, ShapeHub, map[int]Part{
		0:      NewTrigger(OnCast),
		1:      NewTarget(TargetAdjacent, 1, 0), // f1, f2
		3:      NewTarget(TargetNearest, 0, 3),  // f1, f2, f3
		Center: NewDamage(id("kinetic"), 10),
	})
	p := compile(t, h, 0)
	res := run(t, p, a, c, NoEntity, nil, nil)
	if res.Hits != 3 {
		t.Fatalf("center hit %d, want each of 3 foes once\n%s", res.Hits, p)
	}
	for _, f := range []EntityID{f1, f2, f3} {
		if hp := a.Unit(f).HP; hp >= 1000 || hp < 980 {
			t.Fatalf("foe %d at %.0f: pooled hit should land once", f, hp)
		}
	}
	// A multiplier in the center amplifies the whole ring.
	amp := shaped(t, ShapeHub, map[int]Part{
		0:      NewTrigger(OnCast),
		1:      NewDamage(id("kinetic"), 10),
		4:      NewDamage(id("kinetic"), 20),
		Center: NewMult(VerbDamage, 2),
	})
	ds := steps(compile(t, amp, 0), OpDamage)
	if len(ds) != 2 || ds[0].Value != 20 || ds[1].Value != 40 {
		t.Fatalf("center multiplier: %+v", ds)
	}
	// The center is never a gate's branch.
	if r := amp.Routes(0); r[Center].Gate >= 0 {
		t.Fatal("center routed by a gate")
	}
}

func TestHubCenterWithNoTargetsHitsTheSubject(t *testing.T) {
	a, c := world()
	foe := a.Spawn(1, Coord{6, 6}, 10, 1000)
	h := shaped(t, ShapeHub, map[int]Part{0: NewTrigger(OnCast), Center: NewDamage(id("plasma"), 10)})
	res := run(t, compile(t, h, 0), a, c, foe, nil, nil)
	if res.Hits != 1 || a.Unit(foe).HP == 1000 {
		t.Fatalf("hits %d", res.Hits)
	}
}

func TestForkRunsTwoChains(t *testing.T) {
	a, c := world()
	near := a.Spawn(1, Coord{3, 6}, 10, 1000)
	far := a.Spawn(1, Coord{9, 6}, 10, 1000)
	h := shaped(t, ShapeFork, map[int]Part{
		Center: NewTrigger(OnCast),
		0:      NewDamage(id("plasma"), 10), // chain A: the foe next to us
		1:      NewTarget(TargetAdjacent, 1, 0),
		2:      NewTarget(TargetSelf, 0, 0),
		3:      NewDamage(id("cryo"), 10), // chain B head: chain A's slot 2 doesn't feed it
	})
	p := compile(t, h, 0)
	res := run(t, p, a, c, far, nil, nil)
	if res.Hits != 2 || a.Unit(near).HP == 1000 || a.Unit(far).HP == 1000 {
		t.Fatalf("fork hits %d near %.0f far %.0f\n%s", res.Hits, a.Unit(near).HP, a.Unit(far).HP, p)
	}
	if a.Unit(c).HP != 1000 {
		t.Fatal("chain B read chain A's self target across the gap")
	}
	if !h.Feeds(Center, 0) || !h.Feeds(Center, 3) || h.Feeds(2, 3) || h.Feeds(5, 0) {
		t.Fatal("fork wiring")
	}
	// A fork's trigger lives in its center.
	bad := shaped(t, ShapeFork, map[int]Part{0: NewDamage(id("plasma"), 1)})
	bad.Slots[0] = Slot{Kind: SlotPart, Part: NewTrigger(OnCast)}
	if _, err := Compile(bad, reg, 0); err == nil {
		t.Fatal("fork with a ring trigger compiled")
	}
}

func TestForkGatesStayInTheirChain(t *testing.T) {
	h := shaped(t, ShapeFork, map[int]Part{
		Center: NewTrigger(OnCast),
		2:      NewGate(GateHull, 0.5), // last of chain A: nowhere to route
		3:      NewDamage(id("plasma"), 10),
	})
	r := h.Routes(Center)
	if r[3].Gate >= 0 {
		t.Fatal("gate routed across the fork")
	}
	h.Clear(2)
	set(t, h, 1, NewGate(GateHull, 0.5))
	set(t, h, 2, NewDamage(id("plasma"), 5))
	if r = h.Routes(Center); r[2].Gate != 1 || r[2].Branch != BranchYes || r[3].Gate >= 0 {
		t.Fatalf("routes %+v", r)
	}
}

// countLoop: hit the subject, then a spend gate in the last slot. While
// bank A has a coin the gate passes the subject, which the last slot
// carries back round for another pass.
func countLoop(t testing.TB) *Hex {
	return shaped(t, ShapeLoop, map[int]Part{
		0: NewTrigger(OnCast),
		1: NewDamage(id("plasma"), 10),
		5: NewSpendGate(A, 1),
	})
}

func TestLoopRunsWhileItsLastSlotCarries(t *testing.T) {
	a, c := world()
	foe := a.Spawn(1, Coord{5, 6}, 10, 1e9)
	p := compile(t, countLoop(t), 0)
	if !p.Loops || !p.CallsHex(CallSelf) {
		t.Fatalf("loop has no back edge:\n%s", p)
	}
	bank := Bank{A: 3}
	res := run(t, p, a, c, foe, &bank, nil)
	if res.Hits != 4 || res.Calls != 3 || bank[A] != 0 || res.Halted {
		t.Fatalf("hits %d calls %d A=%d halted %v, want 4 hits 3 passes\n%s", res.Hits, res.Calls, bank[A], res.Halted, p)
	}
}

func TestLoopStopsAtHeat(t *testing.T) {
	a, c := world()
	foe := a.Spawn(1, Coord{5, 6}, 10, 1e9)
	h := shaped(t, ShapeLoop, map[int]Part{
		0: NewTrigger(OnCast),
		1: NewDamage(id("plasma"), 10),
		5: NewTarget(TargetNearest, 0, 1), // always someone: only Heat stops it
	})
	p := compile(t, h, 0)
	rig := NewRig(Points{})
	res := run(t, p, a, c, foe, nil, &rig)
	if !res.Halted || !rig.Locked() || res.Calls == 0 || res.Calls >= MaxCalls {
		t.Fatalf("calls %d halted %v locked %v", res.Calls, res.Halted, rig.Locked())
	}
	// No rig: MaxCalls ends it.
	res = run(t, p, a, c, foe, nil, nil)
	if res.Calls != MaxCalls || !res.Halted {
		t.Fatalf("no rig: calls %d", res.Calls)
	}
}

func TestOpenLoopRunsOnce(t *testing.T) {
	for name, last := range map[string]Part{
		"mult":   NewMult(VerbDamage, 2),
		"charge": NewCharge(A, 1), // outside a branch it carries no one
	} {
		h := countLoop(t)
		h.Clear(5)
		set(t, h, 5, last)
		if p := compile(t, h, 0); p.Loops {
			t.Errorf("%s in the last slot looped", name)
		}
	}
	h := countLoop(t)
	h.Linked[5] = false // the back edge is cut
	if p := compile(t, h, 0); p.Loops {
		t.Error("loop with its back edge cut looped")
	}
	h = countLoop(t)
	h.Clear(5)
	if p := compile(t, h, 0); p.Loops {
		t.Error("empty last slot looped")
	}
}

func TestTwinFiresTwice(t *testing.T) {
	parts := map[int]Part{0: NewTrigger(OnCast), 1: NewDamage(id("plasma"), 10), 2: NewMult(VerbDamage, 2)}
	plain := compile(t, shaped(t, ShapeRing, parts), 0)
	h := shaped(t, ShapeRing, parts)
	h.Shape = Shape{Kind: ShapeRing, Quirk: ShapeTwin, At: 1, ID: 9}
	p := compile(t, h, 0)
	ds := steps(p, OpDamage)
	if len(ds) != 2 || ds[0].Value != 20 || ds[1].Value != 20 {
		t.Fatalf("twin damage: %+v (the multiplier after should scale both)", ds)
	}
	if p.Heat <= plain.Heat || p.Power <= plain.Power {
		t.Fatalf("twin should cost more: %.1f/%.1f vs %.1f/%.1f", p.Power, p.Heat, plain.Power, plain.Heat)
	}
	// A multiplier on the twin slot applies twice.
	h.Shape.At = 2
	if ds = steps(compile(t, h, 0), OpDamage); len(ds) != 1 || ds[0].Value != 40 {
		t.Fatalf("twin multiplier: %+v", ds)
	}
}

func TestSurgeBoostsItsSlot(t *testing.T) {
	h := shaped(t, ShapeRing, map[int]Part{0: NewTrigger(OnCast), 1: NewDamage(id("plasma"), 10), 2: NewDamage(id("plasma"), 10)})
	h.Shape = Shape{Kind: ShapeRing, Quirk: ShapeSurge, At: 2, ID: 3}
	ds := steps(compile(t, h, 0), OpDamage)
	if ds[0].Value != 10 || ds[1].Value != 15 {
		t.Fatalf("surge: %+v", ds)
	}
	m := shaped(t, ShapeRing, map[int]Part{0: NewTrigger(OnCast), 1: NewDamage(id("plasma"), 10), 2: NewMult(VerbDamage, 2)})
	m.Shape = Shape{Kind: ShapeRing, Quirk: ShapeSurge, At: 2, ID: 3}
	if ds = steps(compile(t, m, 0), OpDamage); ds[0].Value != 25 {
		t.Fatalf("surged x2 multiplier should give x2.5: %+v", ds)
	}
}

func TestOneWayEdge(t *testing.T) {
	a, c := world()
	foe := a.Spawn(1, Coord{3, 6}, 10, 1000)
	far := a.Spawn(1, Coord{9, 6}, 10, 1000)
	// Damage in slot 1 with its target in slot 2 (clockwise of it).
	h := shaped(t, ShapeRing, map[int]Part{
		0: NewTrigger(OnCast),
		1: NewDamage(id("plasma"), 10),
		2: NewTarget(TargetAdjacent, 1, 0),
	})
	h.Shape = Shape{Kind: ShapeRing, Quirk: ShapeOneWay, At: 1, ID: 5}
	if !h.Feeds(1, 2) || h.Feeds(2, 1) {
		t.Fatal("one-way edge feeds backwards")
	}
	run(t, compile(t, h, 0), a, c, far, nil, nil)
	if a.Unit(foe).HP != 1000 || a.Unit(far).HP == 1000 {
		t.Fatal("damage read the target across a one-way edge; it should fall back to the subject")
	}
}

func TestLeanRunsCooler(t *testing.T) {
	parts := map[int]Part{0: NewTrigger(OnCast), 1: NewDamage(id("plasma"), 10)}
	plain := compile(t, shaped(t, ShapeHub, parts), 0)
	h := shaped(t, ShapeHub, parts)
	h.Shape = Shape{Kind: ShapeHub, Quirk: ShapeLean, ID: 7}
	approx(t, "lean heat", compile(t, h, 0).Heat, plain.Heat*LeanHeat)
}

func TestRollShapeIsValid(t *testing.T) {
	r := splitmix{1}
	seen := map[ShapeKind]bool{}
	quirks := map[ShapeQuirk]bool{}
	for i := uint64(1); i < 2000; i++ {
		s := RollShape(r.next, i)
		if !s.Valid() || s.Quirk == ShapeQuirkNone || s.ID == 0 {
			t.Fatalf("rolled %+v", s)
		}
		seen[s.Kind] = true
		quirks[s.Quirk] = true
		// Every rolled shape must compile with just a trigger in it.
		h := shaped(t, s.Kind, map[int]Part{s.Kind.TriggerSlot(): NewTrigger(OnCast)})
		h.Shape = s
		if _, err := Compile(h, reg, 0); err != nil {
			t.Fatalf("%v: %v", s, err)
		}
	}
	if len(seen) != int(NumShapes) || len(quirks) != int(NumShapeQuirks)-1 {
		t.Fatalf("rolled kinds %v quirks %v", seen, quirks)
	}
}

func TestOldHexLoadsAsRing(t *testing.T) {
	// A hex saved before shapes: six-slot arrays, no Shape field.
	old := `{"Name":"Old","Seed":0,"Sockets":[0,3,3,3,3,3],"Linked":[true,true,true,true,true,true],
	"Slots":[{"Kind":1,"Part":{"Kind":0,"Trigger":0,"Value":1}},{"Kind":0},{"Kind":0},{"Kind":0},{"Kind":0},{"Kind":0}]}`
	var h Hex
	if err := json.Unmarshal([]byte(old), &h); err != nil {
		t.Fatal(err)
	}
	if h.Shape.Kind != ShapeRing || h.Shape.Quirk != ShapeQuirkNone {
		t.Fatalf("old hex shape %+v", h.Shape)
	}
	if h.Slots[Center].Kind != SlotEmpty {
		t.Fatal("old hex has something in the center")
	}
	h.Slots[0].Part = NewTrigger(OnCast)
	set(t, &h, 1, NewDamage(id("plasma"), 10))
	if _, err := Compile(&h, reg, 0); err != nil {
		t.Fatal(err)
	}
	// And a shaped hex round-trips.
	h.Shape = Shape{Kind: ShapeLoop, Quirk: ShapeTwin, At: 4, ID: 77}
	b, _ := json.Marshal(&h)
	var back Hex
	if err := json.Unmarshal(b, &back); err != nil || back.Shape != h.Shape {
		t.Fatalf("round trip: %v %+v", err, back.Shape)
	}
}

func TestShapesRunWithoutAllocating(t *testing.T) {
	for _, bh := range shapeBenchHexes {
		p := compile(t, bh.build(t), 0)
		a, c := benchArena()
		ev := NewEvaluator(reg, DefaultRules())
		ctx := Ctx{Caster: c, Subject: NoEntity, Level: 10}
		ev.Run(p, a, ctx)
		if n := testing.AllocsPerRun(100, func() { ev.Run(p, a, ctx) }); n != 0 {
			t.Errorf("%s allocates %.0f per run", bh.name, n)
		}
	}
}

// Shape hexes for the benchmarks: each shape doing the kind of thing it's
// for, built from parts a player could slot.
var shapeBenchHexes = []struct {
	name  string
	build func(testing.TB) *Hex
}{
	{"triangle", func(t testing.TB) *Hex {
		return shaped(t, ShapeTriangle, map[int]Part{
			0: NewTrigger(OnCast), 1: NewTarget(TargetChain, 3, 3), 2: NewDamage(id("plasma"), 20)})
	}},
	{"hub", func(t testing.TB) *Hex {
		return shaped(t, ShapeHub, map[int]Part{
			0: NewTrigger(OnCast), 1: NewTarget(TargetChain, 3, 3), 2: NewDamage(id("cryo"), 10),
			3: NewTarget(TargetAdjacent, 2, 0), 4: NewStatus(id("cryo"), 2), 5: NewTarget(TargetCone, 3, 0),
			Center: NewDamage(id("plasma"), 25)})
	}},
	{"fork", func(t testing.TB) *Hex {
		return shaped(t, ShapeFork, map[int]Part{
			Center: NewTrigger(OnCast),
			0:      NewTarget(TargetChain, 3, 3), 1: NewDamage(id("plasma"), 15), 2: NewMult(VerbDamage, 1.5),
			3: NewTarget(TargetAdjacent, 2, 0), 4: NewCrowdGate(1, 2), 5: NewStatus(id("cryo"), 2)})
	}},
	{"loop", func(t testing.TB) *Hex {
		// Hits the nearest foe and loops on it while bank A has coins.
		return shaped(t, ShapeLoop, map[int]Part{
			0: NewTrigger(OnCast), 1: NewDamage(id("plasma"), 10), 2: NewTarget(TargetNearest, 0, 2),
			3: NewStatus(id("cryo"), 1), 5: NewSpendGate(A, 1)})
	}},
}

func BenchmarkShapeCompile(b *testing.B) {
	for _, bh := range shapeBenchHexes {
		h := bh.build(b)
		b.Run(bh.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := Compile(h, reg, 4); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkShapeEvaluate(b *testing.B) {
	for _, bh := range shapeBenchHexes {
		p, err := Compile(bh.build(b), reg, 4)
		if err != nil {
			b.Fatal(err)
		}
		b.Run(bh.name, func(b *testing.B) {
			a, c := benchArena()
			ev := NewEvaluator(reg, DefaultRules())
			var bank Bank
			ctx := Ctx{Caster: c, Subject: NoEntity, Level: 10, Bank: &bank}
			b.ReportAllocs()
			b.ResetTimer()
			var res Result
			for i := 0; i < b.N; i++ {
				bank[A] = 8 // the loop makes 8 extra passes every run
				res = ev.Run(p, a, ctx)
			}
			b.ReportMetric(float64(res.Hits+res.Statuses), "effects/run")
			b.ReportMetric(float64(res.Calls), "passes/run")
		})
	}
}

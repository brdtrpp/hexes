package skill

import (
	"strings"
	"testing"
)

// frostbite is the examples' library hex: a Triangle with only a cryo
// chill in slot 3.
func frostbite(t testing.TB) *Hex {
	return module(t, "Frostbite", Shape{Kind: ShapeTriangle}, map[int]Part{2: NewStatus(id("cryo"), 2.5)})
}

// pack is the examples' finder: foes within 3 that have company within 1.
func pack(t testing.TB) *Hex {
	return module(t, "Pack", Shape{Kind: ShapeTriangle}, map[int]Part{
		1: NewTarget(TargetAdjacent, 3, 0),
		2: NewCrowdGate(1, 1),
	})
}

// A run-now call runs the library inline on the call's group, its
// options scale the library's numbers, and the library pays Heat only.
func TestLibCallRunsInline(t *testing.T) {
	fb := frostbite(t)
	call := NewLibCall("Frostbite")
	call, ok := call.WithOpt(NewMult(VerbDuration, 1.5))
	if !ok {
		t.Fatal("option refused")
	}
	snap := shaped(t, ShapeRing, map[int]Part{
		0: NewTrigger(OnCast),
		1: NewTarget(TargetCone, 3, 0),
		2: call,
	})
	snap.Libs[2] = fb
	p := compile(t, snap, 0)
	st := steps(p, OpStatus)
	if len(st) != 1 {
		t.Fatalf("status steps %d\n%s", len(st), p)
	}
	approx(t, "chill", st[0].Value, 2.5*1.5)
	sel := steps(p, OpSelect)
	if len(sel) != 1 || st[0].Src != sel[0].Dst {
		t.Fatalf("chill doesn't land on the cone\n%s", p)
	}
	lib, err := CompileModule(fb, reg, 0)
	if err != nil {
		t.Fatal(err)
	}
	snap.Slots[2] = Slot{}
	bare := compile(t, snap, 0)
	mult := NewMult(VerbDuration, 1.5)
	approx(t, "power", p.Power, bare.Power+call.Power+mult.Power)
	approx(t, "heat", p.Heat, bare.Heat+(call.Heat+mult.Heat)*ShapeRing.HeatScale()+lib.Heat)
}

// A library hex with a missing name does nothing and passes its group on.
func TestLibCallMissingIsHarmless(t *testing.T) {
	h := shaped(t, ShapeRing, map[int]Part{
		0: NewTrigger(OnCast),
		1: NewTarget(TargetAdjacent, 2, 0),
		2: NewLibCall("Nobody"),
		3: NewDamage(id("plasma"), 10),
	})
	p := compile(t, h, 0)
	d := steps(p, OpDamage)
	if len(d) != 1 || d[0].Src != steps(p, OpSelect)[0].Dst {
		t.Fatalf("damage should land on slot 2's targets\n%s", p)
	}
	if what, ok := h.Use(2); ok || !strings.Contains(what, "no library hex") {
		t.Fatalf("use %q %v", what, ok)
	}
}

// A finder returns a group: the slot after the call acts on who Pack
// found, and a gate with nothing after it is fine in a library.
func TestLibCallReturnsAGroup(t *testing.T) {
	a, c := world()
	f1 := a.Spawn(1, Coord{4, 6}, 10, 1000) // f1 and f2 stand together
	f2 := a.Spawn(1, Coord{4, 5}, 10, 1000)
	lone := a.Spawn(1, Coord{2, 4}, 10, 1000) // within 3, but alone
	pk := pack(t)
	if what, ok := pk.Use(2); !ok || !strings.Contains(what, "returns") {
		t.Fatalf("finder gate use %q %v", what, ok)
	}
	gren := shaped(t, ShapeRing, map[int]Part{
		0: NewTrigger(OnCast),
		1: NewLibCall("Pack"),
		2: NewDamage(id("kinetic"), 25),
	})
	gren.Libs[1] = pk
	p := compile(t, gren, 0)
	run(t, p, a, c, lone, nil, nil)
	if a.Unit(f1).HP == 1000 || a.Unit(f2).HP == 1000 || a.Unit(lone).HP != 1000 {
		t.Fatalf("grenade hit f1 %v f2 %v lone %v\n%s", a.Unit(f1).HP, a.Unit(f2).HP, a.Unit(lone).HP, p)
	}
	if what, ok := gren.Use(2); !ok || !strings.Contains(what, "slot 2's targets") {
		t.Fatalf("effect after a finder: %q", what)
	}
}

// A library can't reach a hex that's already running.
func TestLibCallNoRecursion(t *testing.T) {
	a := module(t, "A", Shape{Kind: ShapeTriangle}, map[int]Part{0: NewLibCall("B")})
	b := module(t, "B", Shape{Kind: ShapeTriangle}, map[int]Part{0: NewLibCall("A")})
	a.Libs[0], b.Libs[0] = b, a
	h := shaped(t, ShapeRing, map[int]Part{0: NewTrigger(OnCast), 1: NewLibCall("A")})
	h.Libs[1] = a
	if _, err := Compile(h, reg, 0); err == nil {
		t.Fatal("recursive library compiled")
	}
}

// A gate tests what the slot before it carries: here the group a call
// landed on, not the trigger's subject.
func TestGateReadsCarry(t *testing.T) {
	h := shaped(t, ShapeRing, map[int]Part{
		0: NewTrigger(OnCast),
		1: NewTarget(TargetAdjacent, 2, 0),
		2: NewDamage(id("kinetic"), 10),
		3: NewGate(GateHull, 0.5),
		4: NewDamage(id("kinetic"), 30),
	})
	p := compile(t, h, 0)
	g := steps(p, OpGate)
	if len(g) != 1 || g[0].Src != steps(p, OpSelect)[0].Dst {
		t.Fatalf("gate should split slot 2's targets\n%s", p)
	}
}

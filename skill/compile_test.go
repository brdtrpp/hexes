package skill

import (
	"strings"
	"testing"
)

func TestCompileNeedsTrigger(t *testing.T) {
	h := NewHex("x", [6]SocketKind{T, G, E, E, W, G})
	set(t, h, 2, NewDamage(id("cryo"), 10))
	if _, err := Compile(h, reg, 0); err == nil || !strings.Contains(err.Error(), "no trigger") {
		t.Fatalf("want no-trigger error, got %v", err)
	}
}

func TestSocketsRejectWrongParts(t *testing.T) {
	h := NewHex("x", [6]SocketKind{T, G, E, E, W, G})
	if err := h.Set(1, NewDamage(id("cryo"), 10)); err == nil {
		t.Fatal("target socket took an effect")
	}
	if err := h.Set(4, NewTrigger(OnHit)); err == nil {
		t.Fatal("wild socket took a trigger")
	}
	if err := h.Nest(2, NewHex("n", [6]SocketKind{G, E, E, E, E, E})); err == nil {
		t.Fatal("effect socket took a nested hex")
	}
}

func TestDepthLimitFromProcessing(t *testing.T) {
	inner := NewHex("inner", [6]SocketKind{G, E, E, E, E, E})
	set(t, inner, 0, NewTarget(TargetAdjacent, 1, 0))
	set(t, inner, 1, NewDamage(id("cryo"), 10))
	h := NewHex("outer", [6]SocketKind{T, G, W, E, E, G})
	set(t, h, 0, NewTrigger(OnCast))
	set(t, h, 1, NewTarget(TargetNearest, 0, 1))
	if err := h.Nest(2, inner); err != nil {
		t.Fatal(err)
	}
	if _, err := Compile(h, reg, 0); err == nil {
		t.Fatal("depth 1 hex compiled with processing depth 0")
	}
	p := compile(t, h, 1)
	if p.Depth != 1 {
		t.Fatalf("depth %d", p.Depth)
	}
}

func TestNestedTriggerRejected(t *testing.T) {
	inner := NewHex("inner", [6]SocketKind{T, E, E, E, E, E})
	set(t, inner, 0, NewTrigger(OnHit))
	h := NewHex("outer", [6]SocketKind{T, G, W, E, E, G})
	set(t, h, 0, NewTrigger(OnCast))
	_ = h.Nest(2, inner)
	if _, err := Compile(h, reg, 3); err == nil {
		t.Fatal("nested trigger compiled")
	}
}

func TestNestingCycleRejected(t *testing.T) {
	a := NewHex("a", [6]SocketKind{T, W, E, E, E, E})
	b := NewHex("b", [6]SocketKind{W, E, E, E, E, E})
	set(t, a, 0, NewTrigger(OnCast))
	_ = a.Nest(1, b)
	_ = b.Nest(0, a)
	if _, err := Compile(a, reg, 100); err == nil {
		t.Fatal("cyclic hex compiled")
	}
}

// baseHex: trigger 0, nearest foe 1, then effect sockets 2 and 3, wild 4.
func baseHex(t *testing.T) *Hex {
	h := NewHex("base", [6]SocketKind{T, G, E, E, W, G})
	set(t, h, 0, NewTrigger(OnCast))
	set(t, h, 1, NewTarget(TargetNearest, 0, 1))
	return h
}

func TestMultiplierScalesPartBeforeIt(t *testing.T) {
	h := baseHex(t)
	set(t, h, 2, NewDamage(id("cryo"), 10))
	set(t, h, 3, NewMult(VerbDamage, 2))
	d := steps(compile(t, h, 0), OpDamage)
	approx(t, "damage", d[0].Value, 20)
}

func TestMultiplierBeforeEffectIsWasted(t *testing.T) {
	h := baseHex(t)
	set(t, h, 2, NewMult(VerbDamage, 2))
	set(t, h, 3, NewDamage(id("cryo"), 10))
	d := steps(compile(t, h, 0), OpDamage)
	approx(t, "damage", d[0].Value, 10)
}

func TestBrokenEdgeStopsMultiplier(t *testing.T) {
	h := baseHex(t)
	set(t, h, 2, NewDamage(id("cryo"), 10))
	set(t, h, 3, NewMult(VerbDamage, 2))
	h.Linked[2] = false // cut between slot 2 and 3
	d := steps(compile(t, h, 0), OpDamage)
	approx(t, "damage", d[0].Value, 10)
}

func TestMultiplierRunStacks(t *testing.T) {
	h := baseHex(t)
	set(t, h, 2, NewDamage(id("cryo"), 10))
	set(t, h, 3, NewMult(VerbDamage, 2))
	set(t, h, 4, NewMult(VerbDamage, 1.5))
	d := steps(compile(t, h, 0), OpDamage)
	approx(t, "damage", d[0].Value, 30)
}

func TestMultiplierOnlyScalesItsVerb(t *testing.T) {
	h := baseHex(t)
	set(t, h, 2, NewPush(1))
	set(t, h, 3, NewMult(VerbDamage, 3))
	p := compile(t, h, 0)
	approx(t, "push", steps(p, OpPush)[0].Value, 1)

	set(t, h, 3, NewMult(VerbPush, 3))
	p = compile(t, h, 0)
	approx(t, "push", steps(p, OpPush)[0].Value, 3)

	set(t, h, 2, NewStatus(id("cryo"), 2))
	set(t, h, 3, NewMult(VerbAll, 2))
	p = compile(t, h, 0)
	approx(t, "duration", steps(p, OpStatus)[0].Value, 4)
}

func TestMultiplierScalesTargetCount(t *testing.T) {
	h := NewHex("x", [6]SocketKind{T, G, E, E, E, E})
	set(t, h, 0, NewTrigger(OnCast))
	set(t, h, 1, NewTarget(TargetChain, 3, 2))
	set(t, h, 2, NewMult(VerbCount, 2))
	set(t, h, 3, NewDamage(id("cryo"), 10))
	s := steps(compile(t, h, 0), OpSelect)
	if s[0].Count != 4 {
		t.Fatalf("chain count %d, want 4", s[0].Count)
	}
}

func TestRarityScalesValues(t *testing.T) {
	h := baseHex(t)
	set(t, h, 2, NewDamage(id("cryo"), 10).WithRarity(Rare))
	set(t, h, 3, NewMult(VerbDamage, 2).WithRarity(Exotic))
	d := steps(compile(t, h, 0), OpDamage)
	approx(t, "damage", d[0].Value, 10*1.35*(1+1*1.6))
}

func TestEffectBetweenTwoTargetsHitsBoth(t *testing.T) {
	h := NewHex("x", [6]SocketKind{T, G, E, G, E, E})
	set(t, h, 0, NewTrigger(OnCast))
	set(t, h, 1, NewTarget(TargetNearest, 0, 1))
	set(t, h, 2, NewDamage(id("cryo"), 10))
	set(t, h, 3, NewTarget(TargetAdjacent, 1, 0))
	p := compile(t, h, 0)
	u := steps(p, OpUnion)
	if len(u) != 1 {
		t.Fatalf("want one union, got %d\n%s", len(u), p)
	}
	if d := steps(p, OpDamage)[0]; d.Src != u[0].Dst {
		t.Fatalf("damage reads r%d, union wrote r%d", d.Src, u[0].Dst)
	}
}

func TestResolvesClockwiseFromTrigger(t *testing.T) {
	h := NewHex("x", [6]SocketKind{E, E, E, T, E, E})
	set(t, h, 3, NewTrigger(OnHit))
	set(t, h, 0, NewDamage(id("cryo"), 2)) // walk position 3
	set(t, h, 4, NewDamage(id("cryo"), 1)) // walk position 1
	d := steps(compile(t, h, 0), OpDamage)
	if d[0].Value != 1 || d[1].Value != 2 {
		t.Fatalf("order %v then %v", d[0].Value, d[1].Value)
	}
}

func TestSurpriseTriggerBoostsCloak(t *testing.T) {
	h := baseHex(t)
	set(t, h, 0, NewTrigger(FromStealth))
	set(t, h, 2, NewDamage(id("cloak"), 10))
	set(t, h, 3, NewDamage(id("cryo"), 10))
	d := steps(compile(t, h, 0), OpDamage)
	approx(t, "cloak", d[0].Value, 40) // x2 stealth trigger, x2 cloak
	approx(t, "cryo", d[1].Value, 20)  // trigger only
}

func TestPowerAndHeat(t *testing.T) {
	h := baseHex(t)
	set(t, h, 2, NewDamage(id("plasma"), 10)) // power 2, heat 1 + plasma 2
	set(t, h, 3, NewDamage(id("cryo"), 10))   // power 2, heat 1 - cryo 3
	p := compile(t, h, 0)
	approx(t, "power", p.Power, 2+1+2+2) // trigger + nearest + two damages
	approx(t, "heat", p.Heat, 3-2)

	inner := NewHex("inner", [6]SocketKind{E, E, E, E, E, E})
	set(t, inner, 0, NewDamage(id("kinetic"), 10)) // power 2, heat 1, at depth 1: x1.5
	if err := h.Nest(4, inner); err != nil {
		t.Fatal(err)
	}
	p = compile(t, h, 1)
	approx(t, "nested power", p.Power, 7+3)
	approx(t, "nested heat", p.Heat, 1+1.5)
}

func TestMultiplierAfterNestedScalesInside(t *testing.T) {
	inner := NewHex("inner", [6]SocketKind{G, E, E, E, E, E})
	set(t, inner, 0, NewTarget(TargetAdjacent, 1, 0))
	set(t, inner, 1, NewDamage(id("cryo"), 10))
	set(t, inner, 2, NewDamage(id("cryo"), 5))
	h := NewHex("outer", [6]SocketKind{T, G, W, E, E, G})
	set(t, h, 0, NewTrigger(OnCast))
	set(t, h, 1, NewTarget(TargetNearest, 0, 1))
	_ = h.Nest(2, inner)
	set(t, h, 3, NewMult(VerbDamage, 3))
	p := compile(t, h, 1)
	d := steps(p, OpDamage)
	approx(t, "inner 1", d[0].Value, 30)
	approx(t, "inner 2", d[1].Value, 15)
	if fe := steps(p, OpForEach); len(fe) != 1 || fe[0].Span != 3 {
		t.Fatalf("foreach %+v\n%s", fe, p)
	}
}

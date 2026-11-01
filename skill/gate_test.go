package skill

import (
	"strings"
	"testing"
)

// gateHex: on cast, pick the nearest three foes, gate them, hit the ones
// that pass with cryo and mark the rest.
func gateHex(t *testing.T, g Part) *Hex {
	h := NewHex("gated", [6]SocketKind{T, G, W, W, W, W})
	set(t, h, 0, NewTrigger(OnCast))
	set(t, h, 1, NewTarget(TargetNearest, 0, 3))
	set(t, h, 2, g)
	set(t, h, 3, NewDamage(id("cryo"), 40))
	set(t, h, 4, NewStatus(id("drone"), 3))
	return h
}

// threeFoes spawns three foes far enough apart not to touch.
func threeFoes(a *Arena) [3]EntityID {
	return [3]EntityID{
		a.Spawn(1, Coord{4, 6}, 10, 1000),
		a.Spawn(1, Coord{4, 3}, 10, 1000),
		a.Spawn(1, Coord{4, 9}, 10, 1000),
	}
}

func hitAndMarked(a *Arena, id EntityID) (hit, marked bool) {
	u := a.Unit(id)
	return u.HP < u.MaxHP-0.01, u.MarkLeft > 0
}

func TestGateHullSplitsTheGroup(t *testing.T) {
	a, c := world()
	f := threeFoes(a)
	a.Unit(f[1]).HP = 200 // 20% hull
	p := compile(t, gateHex(t, NewGate(GateHull, 0)), 0)
	NewEvaluator(reg, DefaultRules()).Run(p, a, Ctx{Caster: c, Subject: NoEntity, Level: 10})
	for i, id := range f {
		hit, marked := hitAndMarked(a, id)
		want := i == 1
		if hit != want || marked == want {
			t.Fatalf("foe %d: hit=%v marked=%v, want hit=%v\n%s", i, hit, marked, want, p)
		}
	}
	approx(t, "weak foe hp", a.Unit(f[1]).HP, 160)
}

func TestGateHeatSendsEveryoneOneWay(t *testing.T) {
	for _, hot := range []bool{true, false} {
		a, c := world()
		f := threeFoes(a)
		rig := NewRig(Points{})
		if hot {
			rig.Heat = 60
		} else {
			rig.Heat = 40
		}
		p := compile(t, gateHex(t, NewGate(GateHeat, 0)), 0)
		NewEvaluator(reg, DefaultRules()).Run(p, a, Ctx{Caster: c, Subject: NoEntity, Level: 10, Rig: &rig})
		for i, id := range f {
			hit, marked := hitAndMarked(a, id)
			if hit != hot || marked == hot {
				t.Fatalf("hot=%v foe %d: hit=%v marked=%v", hot, i, hit, marked)
			}
		}
	}
}

func TestGateWithoutRigReadsCold(t *testing.T) {
	a, c := world()
	f := threeFoes(a)
	p := compile(t, gateHex(t, NewGate(GatePower, 0)), 0)
	NewEvaluator(reg, DefaultRules()).Run(p, a, Ctx{Caster: c, Subject: NoEntity, Level: 10})
	for _, id := range f {
		if hit, marked := hitAndMarked(a, id); hit || !marked {
			t.Fatalf("no rig should fail a power gate: hit=%v marked=%v", hit, marked)
		}
	}
}

func TestGateStatus(t *testing.T) {
	a, c := world()
	f := threeFoes(a)
	a.Unit(f[2]).SlowLeft = 1 // chilled
	p := compile(t, gateHex(t, NewStatusGate(id("cryo"))), 0)
	NewEvaluator(reg, DefaultRules()).Run(p, a, Ctx{Caster: c, Subject: NoEntity, Level: 10})
	for i, id := range f {
		hit, _ := hitAndMarked(a, id)
		if hit != (i == 2) {
			t.Fatalf("foe %d hit=%v", i, hit)
		}
	}
}

func TestGateCrowd(t *testing.T) {
	a, c := world()
	lone := a.Spawn(1, Coord{4, 6}, 10, 1000)
	packed := a.Spawn(1, Coord{7, 2}, 10, 1000)
	a.Spawn(1, Coord{8, 2}, 10, 1000)
	a.Spawn(1, Coord{7, 3}, 10, 1000)
	h := NewHex("crowd", [6]SocketKind{T, G, W, W, W, W})
	set(t, h, 0, NewTrigger(OnCast))
	set(t, h, 1, NewTarget(TargetNearest, 0, 2))
	set(t, h, 2, NewCrowdGate(1, 2))
	set(t, h, 3, NewStatus(id("drone"), 3))
	p := compile(t, h, 0)
	// Nearest two: the lone foe and the corner of the pack.
	NewEvaluator(reg, DefaultRules()).Run(p, a, Ctx{Caster: c, Subject: NoEntity, Level: 10})
	if a.Unit(lone).MarkLeft > 0 {
		t.Fatal("lone foe passed a crowd gate")
	}
	if a.Unit(packed).MarkLeft == 0 {
		var near [MaxTargets]EntityID
		t.Fatalf("packed foe failed a crowd gate (%d near)\n%s", a.Select(TargetAdjacent, packed, c, 1, near[:]), p)
	}
}

func TestGateBranchesIgnoreTargetsBesideThem(t *testing.T) {
	// The no slot sits next to a target, but it still lands only on the
	// ones that failed the gate.
	a, c := world()
	f := threeFoes(a)
	h := gateHex(t, NewGate(GateHull, 0))
	set(t, h, 5, NewTarget(TargetNearest, 0, 3))
	p := compile(t, h, 0)
	NewEvaluator(reg, DefaultRules()).Run(p, a, Ctx{Caster: c, Subject: NoEntity, Level: 10})
	for _, id := range f {
		if hit, marked := hitAndMarked(a, id); hit || !marked {
			t.Fatalf("hit=%v marked=%v\n%s", hit, marked, p)
		}
	}
}

func TestGateCloakOnlyInTakenBranch(t *testing.T) {
	a, c := world()
	threeFoes(a)
	h := NewHex("vanish when weak", [6]SocketKind{T, G, W, W, W, W})
	set(t, h, 0, NewTrigger(OnCast))
	set(t, h, 1, NewTarget(TargetNearest, 0, 3))
	set(t, h, 2, NewGate(GateHull, 0))
	set(t, h, 3, NewStatus(id("cloak"), 3))
	p := compile(t, h, 0)
	NewEvaluator(reg, DefaultRules()).Run(p, a, Ctx{Caster: c, Subject: NoEntity, Level: 10})
	if a.Unit(c).Stealth > 0 {
		t.Fatal("cloak in an untaken yes branch fired")
	}
	a.Unit(2).HP = 100
	NewEvaluator(reg, DefaultRules()).Run(p, a, Ctx{Caster: c, Subject: NoEntity, Level: 10})
	if a.Unit(c).Stealth == 0 {
		t.Fatal("cloak in a taken yes branch didn't fire")
	}
}

func TestGateNestedBranchRunsPerPasser(t *testing.T) {
	a, c := world()
	f := threeFoes(a)
	a.Unit(f[0]).HP = 100
	inner := NewHex("mark around", [6]SocketKind{G, E, E, E, E, E})
	set(t, inner, 0, NewTarget(TargetSelf, 0, 0))
	set(t, inner, 1, NewDamage(id("cryo"), 10))
	h := NewHex("x", [6]SocketKind{T, G, W, W, W, W})
	set(t, h, 0, NewTrigger(OnCast))
	set(t, h, 1, NewTarget(TargetNearest, 0, 3))
	set(t, h, 2, NewGate(GateHull, 0))
	if err := h.Nest(3, inner); err != nil {
		t.Fatal(err)
	}
	p := compile(t, h, 1)
	ev := NewEvaluator(reg, DefaultRules())
	// Inner hex hits the caster (self) once per foe that passed: one.
	ev.Run(p, a, Ctx{Caster: c, Subject: NoEntity, Level: 10})
	if n := 1000 - a.Unit(c).HP; n < 9.9 || n > 10.1 {
		t.Fatalf("caster took %.1f, want one 10 hit\n%s", n, p)
	}
}

func TestGateInsideNestedHexTestsThatOne(t *testing.T) {
	a, c := world()
	f := threeFoes(a)
	a.Unit(f[2]).HP = 50
	inner := NewHex("execute", [6]SocketKind{W, W, W, E, E, E})
	set(t, inner, 0, NewGate(GateHull, 0))
	set(t, inner, 1, NewDamage(id("cryo"), 40))
	set(t, inner, 2, NewStatus(id("drone"), 3))
	h := NewHex("x", [6]SocketKind{T, G, W, W, W, W})
	set(t, h, 0, NewTrigger(OnCast))
	set(t, h, 1, NewTarget(TargetNearest, 0, 3))
	if err := h.Nest(2, inner); err != nil {
		t.Fatal(err)
	}
	p := compile(t, h, 1)
	NewEvaluator(reg, DefaultRules()).Run(p, a, Ctx{Caster: c, Subject: NoEntity, Level: 10})
	for i, id := range f {
		hit, marked := hitAndMarked(a, id)
		if hit != (i == 2) || marked == (i == 2) {
			t.Fatalf("foe %d hit=%v marked=%v\n%s", i, hit, marked, p)
		}
	}
}

func TestGateRoutes(t *testing.T) {
	g := NewGate(GateHull, 0)
	d := NewDamage(id("cryo"), 10)
	type want struct {
		slot   int
		gate   int8
		branch Branch
	}
	for _, tc := range []struct {
		name  string
		parts [6]Part
		cut   int // edge to cut, -1 for none
		want  []want
	}{
		{"plain", [6]Part{NewTrigger(OnCast), g, d, d, d, {}}, -1,
			[]want{{2, 1, BranchYes}, {3, 1, BranchNo}, {4, -1, BranchNone}}},
		{"cut before no", [6]Part{NewTrigger(OnCast), g, d, d, {}, {}}, 2,
			[]want{{2, 1, BranchYes}, {3, -1, BranchNone}}},
		{"cut before yes", [6]Part{NewTrigger(OnCast), g, d, d, {}, {}}, 1,
			[]want{{2, -1, BranchNone}, {3, -1, BranchNone}}},
		{"and", [6]Part{NewTrigger(OnCast), g, g, d, d, {}}, -1,
			[]want{{2, 1, BranchYes}, {3, 2, BranchYes}, {4, 2, BranchNo}}},
		{"else if", [6]Part{NewTrigger(OnCast), g, d, g, d, d}, -1,
			[]want{{2, 1, BranchYes}, {3, 1, BranchNo}, {4, 3, BranchYes}, {5, 3, BranchNo}}},
		{"no wrap", [6]Part{NewTrigger(OnCast), d, d, d, d, g}, -1,
			[]want{{0, -1, BranchNone}, {1, -1, BranchNone}}},
		{"last but one", [6]Part{NewTrigger(OnCast), d, d, d, g, d}, -1,
			[]want{{5, 4, BranchYes}, {0, -1, BranchNone}}},
	} {
		h := NewHex(tc.name, LoadoutLike)
		for i, p := range tc.parts {
			if p.Kind != KindNone {
				set(t, h, i, p)
			}
		}
		if tc.cut >= 0 {
			h.Linked[tc.cut] = false
		}
		r := h.Routes(0)
		for _, w := range tc.want {
			if got := r[w.slot]; got.Gate != w.gate || got.Branch != w.branch {
				t.Errorf("%s: slot %d = %+v, want gate %d branch %d", tc.name, w.slot, got, w.gate, w.branch)
			}
		}
		if _, err := Compile(h, reg, 0); err != nil {
			t.Errorf("%s: %v", tc.name, err)
		}
	}
}

// LoadoutLike is the game's socket layout: trigger on top, wild elsewhere.
var LoadoutLike = [6]SocketKind{T, W, W, W, W, W}

func TestGateElseIf(t *testing.T) {
	// if hull < 30% cryo hit, else if chilled mark, else push.
	a, c := world()
	f := threeFoes(a)
	a.Unit(f[0]).HP = 100
	a.Unit(f[1]).SlowLeft = 2
	h := NewHex("else if", LoadoutLike)
	set(t, h, 0, NewTrigger(OnCast))
	set(t, h, 1, NewGate(GateHull, 0))
	set(t, h, 2, NewDamage(id("cryo"), 40))
	set(t, h, 3, NewStatusGate(id("cryo")))
	set(t, h, 4, NewStatus(id("drone"), 3))
	set(t, h, 5, NewStatus(id("kinetic"), 1))
	// The first gate has no target before it, so it gates the subject:
	// make the subject each foe in turn, as on-hit would.
	p := compile(t, h, 0)
	ev := NewEvaluator(reg, DefaultRules())
	for _, id := range f {
		ev.Run(p, a, Ctx{Caster: c, Subject: id, Level: 10})
	}
	if u := a.Unit(f[0]); u.HP > 99 || u.MarkLeft > 0 || u.StunLeft > 0 {
		t.Fatalf("weak foe: %+v", u)
	}
	if u := a.Unit(f[1]); u.HP < 1000 || u.MarkLeft == 0 || u.StunLeft > 0 {
		t.Fatalf("chilled foe: hp %.0f mark %.1f stun %.1f", u.HP, u.MarkLeft, u.StunLeft)
	}
	if u := a.Unit(f[2]); u.HP < 1000 || u.MarkLeft > 0 || u.StunLeft == 0 {
		t.Fatalf("plain foe: hp %.0f mark %.1f stun %.1f", u.HP, u.MarkLeft, u.StunLeft)
	}
}

func TestGateMultiplierAfterBranch(t *testing.T) {
	// A multiplier after the no slot scales the no branch only.
	h := gateHex(t, NewGate(GateHull, 0))
	set(t, h, 5, NewMult(VerbDuration, 2))
	p := compile(t, h, 0)
	st := steps(p, OpStatus)
	approx(t, "no branch duration", st[0].Value, 6)
	approx(t, "yes branch damage", steps(p, OpDamage)[0].Value, 40)
}

func TestGateCostAndDisassembly(t *testing.T) {
	h := gateHex(t, NewGate(GateHull, 0))
	p := compile(t, h, 0)
	h.Clear(2)
	q := compile(t, h, 0)
	approx(t, "gate power", p.Power-q.Power, GatePowerCost)
	h2 := gateHex(t, NewGate(GateHull, 0).WithRarity(Exotic))
	approx(t, "exotic gate power", compile(t, h2, 0).Power-q.Power, GatePowerCost/Exotic.Scale())
	if s := p.String(); !strings.Contains(s, "gate hull") {
		t.Fatalf("disassembly lacks the gate:\n%s", s)
	}
}

func TestGateRunDoesNotAllocate(t *testing.T) {
	a, c := world()
	threeFoes(a)
	rig := NewRig(Points{})
	for _, g := range []Part{NewGate(GateHull, 0), NewGate(GateHeat, 0), NewCrowdGate(1, 1), NewStatusGate(id("plasma"))} {
		p := compile(t, gateHex(t, g), 0)
		ev := NewEvaluator(reg, DefaultRules())
		ctx := Ctx{Caster: c, Subject: NoEntity, Level: 10, Rig: &rig}
		if n := testing.AllocsPerRun(500, func() { ev.Run(p, a, ctx) }); n != 0 {
			t.Fatalf("%s: %v allocs per run", g, n)
		}
	}
}

func TestDispatcherHandsGatesTheRig(t *testing.T) {
	a, c := world()
	f := threeFoes(a)
	rig := NewRig(Points{})
	rig.Heat = 90
	d := NewDispatcher(NewEvaluator(reg, DefaultRules()), a)
	d.Equip(c, &rig, compile(t, gateHex(t, NewGate(GateHeat, 0)), 0))
	if o := d.Cast(c, 0, NoEntity); o != Fired {
		t.Fatalf("cast %s", o)
	}
	for _, id := range f {
		if hit, _ := hitAndMarked(a, id); !hit {
			t.Fatal("hot rig should pass a heat gate through the dispatcher")
		}
	}
}

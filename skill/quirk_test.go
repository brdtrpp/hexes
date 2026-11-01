package skill

import "testing"

func TestRadiantScalesWholeRing(t *testing.T) {
	h := NewHex("x", [6]SocketKind{T, G, E, E, E, E})
	set(t, h, 0, NewTrigger(OnCast))
	set(t, h, 1, NewTarget(TargetNearest, 0, 1))
	set(t, h, 2, NewDamage(id("cryo"), 10))
	set(t, h, 3, NewDamage(id("plasma"), 20))
	// Slot 4 empty: a plain multiplier in slot 5 would be wasted.
	m := NewMult(VerbDamage, 2)
	set(t, h, 5, m)
	if d := steps(compile(t, h, 0), OpDamage); d[0].Value != 10 || d[1].Value != 20 {
		t.Fatalf("plain mult after an empty slot should be wasted: %+v", d)
	}
	m.Quirk = QuirkRadiant
	set(t, h, 5, m)
	p := compile(t, h, 0)
	d := steps(p, OpDamage)
	approx(t, "first", d[0].Value, 20)
	approx(t, "second", d[1].Value, 40)
	if plain := compile(t, func() *Hex { set(t, h, 5, NewMult(VerbDamage, 2)); return h }(), 0); p.Heat-plain.Heat != RadiantHeatCost {
		t.Fatalf("radiant heat %.2f vs %.2f", p.Heat, plain.Heat)
	}
}

func TestRadiantCountReachesTargets(t *testing.T) {
	h := NewHex("x", [6]SocketKind{T, G, E, E, E, E})
	set(t, h, 0, NewTrigger(OnCast))
	set(t, h, 1, NewTarget(TargetNearest, 0, 2))
	set(t, h, 2, NewDamage(id("cryo"), 10))
	m := NewMult(VerbCount, 2)
	m.Quirk = QuirkRadiant
	set(t, h, 4, m)
	if s := steps(compile(t, h, 0), OpSelect); s[0].Count != 4 {
		t.Fatalf("count %d, want 4", s[0].Count)
	}
}

func TestRelayArcsToNeighbors(t *testing.T) {
	a, c := world()
	near := a.Spawn(1, Coord{4, 6}, 10, 1000)
	b := a.Spawn(1, Coord{5, 6}, 10, 1000)
	d := a.Spawn(1, Coord{4, 7}, 10, 1000)
	far := a.Spawn(1, Coord{10, 1}, 10, 1000)
	h := baseHex(t)
	dmg := NewDamage(id("cryo"), 40)
	dmg.Quirk = QuirkRelay
	set(t, h, 2, dmg)
	p := compile(t, h, 0)
	res := NewEvaluator(reg, DefaultRules()).Run(p, a, Ctx{Caster: c, Subject: NoEntity, Level: 10})
	if res.Hits != 3 {
		t.Fatalf("hits %d, want 3\n%s", res.Hits, p)
	}
	approx(t, "target", a.Unit(near).HP, 960)
	for _, e := range []EntityID{b, d} {
		approx(t, "arc", a.Unit(e).HP, 1000-40*RelayFrac)
	}
	approx(t, "far", a.Unit(far).HP, 1000)

	plain := baseHex(t)
	set(t, plain, 2, NewDamage(id("cryo"), 40))
	pp := compile(t, plain, 0)
	if p.Power <= pp.Power {
		t.Fatalf("relay should cost more: %.1f vs %.1f", p.Power, pp.Power)
	}
}

func TestRelayArcIsScaledByMultiplier(t *testing.T) {
	h := baseHex(t)
	dmg := NewDamage(id("cryo"), 40)
	dmg.Quirk = QuirkRelay
	set(t, h, 2, dmg)
	set(t, h, 3, NewMult(VerbDamage, 2))
	d := steps(compile(t, h, 0), OpDamage)
	if len(d) != 2 {
		t.Fatalf("damage steps %d", len(d))
	}
	approx(t, "main", d[0].Value, 80)
	approx(t, "arc", d[1].Value, 40)
}

func TestEchoRunsTwice(t *testing.T) {
	a, c := world()
	a.Spawn(1, Coord{4, 6}, 10, 1000)
	a.Spawn(1, Coord{8, 2}, 10, 1000)
	h := baseHex(t)
	set(t, h, 2, NewDamage(id("cryo"), 40))
	plain := compile(t, h, 0)
	trig := NewTrigger(OnCast)
	trig.Quirk = QuirkEcho
	set(t, h, 0, trig)
	p := compile(t, h, 0)
	if len(p.Steps) != 2*len(plain.Steps) {
		t.Fatalf("steps %d vs %d", len(p.Steps), len(plain.Steps))
	}
	approx(t, "power", p.Power, plain.Power*(1+EchoCost))
	res := NewEvaluator(reg, DefaultRules()).Run(p, a, Ctx{Caster: c, Subject: NoEntity, Level: 10})
	if res.Hits != 2 {
		t.Fatalf("hits %d, want 2", res.Hits)
	}
}

func TestQuirkFits(t *testing.T) {
	for _, tc := range []struct {
		q    Quirk
		p    Part
		want bool
	}{
		{QuirkRadiant, NewMult(VerbAll, 1.2), true},
		{QuirkRadiant, NewDamage(0, 1), false},
		{QuirkRelay, NewStatus(0, 1), true},
		{QuirkRelay, NewPush(1), false},
		{QuirkEcho, NewTrigger(OnHit), true},
		{QuirkEcho, NewTarget(TargetSelf, 0, 0), false},
	} {
		if got := tc.q.Fits(tc.p); got != tc.want {
			t.Errorf("%s fits %s = %v", tc.q, tc.p, got)
		}
	}
}

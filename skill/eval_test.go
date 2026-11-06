package skill

import "testing"

// targetsTargetHex: on cast, pick the nearest foe, then hit everyone
// adjacent to it with a nested hex.
func targetsTargetHex(t testing.TB, dmgType TypeID, amount float32) *Hex {
	inner := NewHex("adjacent burst", [6]SocketKind{G, E, E, E, E, E})
	inner.Slots[0] = Slot{Kind: SlotPart, Part: NewTarget(TargetAdjacent, 1, 0)}
	inner.Slots[1] = Slot{Kind: SlotPart, Part: NewDamage(dmgType, amount)}
	h := NewHex("target's target", [6]SocketKind{T, G, W, E, E, G})
	h.Slots[0] = Slot{Kind: SlotPart, Part: NewTrigger(OnCast)}
	h.Slots[1] = Slot{Kind: SlotPart, Part: NewTarget(TargetNearest, 0, 1)}
	if err := h.Nest(2, inner); err != nil {
		t.Fatal(err)
	}
	return h
}

func TestTargetsTarget(t *testing.T) {
	a, c := world()
	near := a.Spawn(1, Coord{4, 6}, 10, 1000)
	b := a.Spawn(1, Coord{5, 6}, 10, 1000)
	d := a.Spawn(1, Coord{4, 7}, 10, 1000)
	far := a.Spawn(1, Coord{10, 1}, 10, 1000)
	p := compile(t, targetsTargetHex(t, id("cryo"), 50), 1)
	res := NewEvaluator(reg, DefaultRules()).Run(p, a, Ctx{Caster: c, Subject: NoEntity, Level: 10})
	if res.Hits != 2 {
		t.Fatalf("hits %d, want 2\n%s", res.Hits, p)
	}
	for _, e := range []EntityID{b, d} {
		approx(t, "adjacent hp", a.Unit(e).HP, 950)
	}
	for _, e := range []EntityID{near, far} {
		approx(t, "untouched hp", a.Unit(e).HP, 1000)
	}
}

// freezeHex chills three times then hits, or hits first if hitFirst.
func freezeHex(t *testing.T, hitFirst bool) *Program {
	h := NewHex("x", [6]SocketKind{T, G, E, E, E, E})
	set(t, h, 0, NewTrigger(OnCast))
	set(t, h, 1, NewTarget(TargetNearest, 0, 1))
	hit, chill := 5, 2
	if hitFirst {
		hit, chill = 2, 3
	}
	for i := 0; i < 3; i++ {
		set(t, h, chill+i, NewStatus(id("cryo"), 2))
	}
	set(t, h, hit, NewDamage(id("cryo"), 40))
	return compile(t, h, 0)
}

func TestClockwiseOrderMatters(t *testing.T) {
	for _, tc := range []struct {
		hitFirst bool
		hp       float32
		frozen   bool
	}{
		{false, 1000 - 40*2.5, false}, // freeze, then the hit shatters it
		{true, 1000 - 40, true},       // hit lands before the freeze
	} {
		a, c := world()
		f := a.Spawn(1, Coord{4, 6}, 10, 1000)
		NewEvaluator(reg, DefaultRules()).Run(freezeHex(t, tc.hitFirst), a, Ctx{Caster: c, Subject: NoEntity, Level: 10})
		approx(t, "hp", a.Unit(f).HP, tc.hp)
		if a.Unit(f).Frozen != tc.frozen {
			t.Fatalf("hitFirst=%v frozen=%v", tc.hitFirst, a.Unit(f).Frozen)
		}
	}
}

func bigHit(t *testing.T, typ string, amount float32) *Program {
	h := baseHex(t)
	set(t, h, 2, NewDamage(id(typ), amount))
	return compile(t, h, 0)
}

func TestPerHitCapByLevelGap(t *testing.T) {
	for _, tc := range []struct {
		level int
		dealt float32
	}{
		{10, 250},  // even: a quarter
		{15, 500},  // 5 above: doubles
		{20, 1000}, // 10 above: one-shot
		{5, 125},   // 5 below: halves
	} {
		a, c := world()
		a.Spawn(1, Coord{4, 6}, 10, 1000)
		res := NewEvaluator(reg, DefaultRules()).Run(bigHit(t, "cryo", 5000), a, Ctx{Caster: c, Subject: NoEntity, Level: tc.level})
		approx(t, "dealt", res.Damage, tc.dealt)
		if res.Capped != 1 {
			t.Fatalf("level %d: capped %d", tc.level, res.Capped)
		}
	}
}

func TestPitKillIgnoresCap(t *testing.T) {
	a, c := world()
	f := a.Spawn(1, Coord{3, 6}, 10, 1000)
	a.SetTile(Coord{4, 6}, TerrainPit)
	h := baseHex(t)
	set(t, h, 2, NewPush(1))
	res := NewEvaluator(reg, DefaultRules()).Run(compile(t, h, 0), a, Ctx{Caster: c, Subject: NoEntity, Level: 1})
	if a.Unit(f).Alive || res.Kills != 1 {
		t.Fatalf("foe survived the pit: %+v", res)
	}
	approx(t, "damage", res.Damage, 1000)
}

func TestKineticWallSlam(t *testing.T) {
	a, c := world()
	f := a.Spawn(1, Coord{11, 6}, 10, 1000) // east edge: the next hex is wall
	res := NewEvaluator(reg, DefaultRules()).Run(bigHit(t, "kinetic", 10), a, Ctx{Caster: c, Subject: NoEntity, Level: 10})
	approx(t, "damage", res.Damage, 10+250) // hit + (0.10 + 0.15) of max health, uncapped
	approx(t, "hp", a.Unit(f).HP, 740)
}

func TestKineticPushMoves(t *testing.T) {
	a, c := world()
	f := a.Spawn(1, Coord{4, 6}, 10, 1000)
	NewEvaluator(reg, DefaultRules()).Run(bigHit(t, "kinetic", 10), a, Ctx{Caster: c, Subject: NoEntity, Level: 10})
	if a.Pos[f] != (Coord{5, 6}) {
		t.Fatalf("foe at %v", a.Pos[f])
	}
}

func TestAdaptiveResistance(t *testing.T) {
	a, c := world()
	f := a.Spawn(1, Coord{4, 6}, 10, 100000)
	ev := NewEvaluator(reg, DefaultRules())
	cryo, drone := bigHit(t, "cryo", 100), bigHit(t, "drone", 100)
	ctx := Ctx{Caster: c, Subject: NoEntity, Level: 10}
	last := float32(1e9)
	for i := 0; i < 6; i++ {
		d := ev.Run(cryo, a, ctx).Damage
		if d >= last {
			t.Fatalf("hit %d dealt %.2f, not less than %.2f", i, d, last)
		}
		last = d
	}
	if last > 70 {
		t.Fatalf("six cryo hits only resisted down to %.1f", last)
	}
	if d := ev.Run(drone, a, ctx).Damage; d != 100 {
		t.Fatalf("fresh type dealt %.2f, want 100", d)
	}
	if a.Unit(f).Resist[id("cryo")] <= 0 {
		t.Fatal("no cryo resistance built")
	}
}

func TestNaniteTicksSlideUnderCap(t *testing.T) {
	a, c := world()
	f := a.Spawn(1, Coord{4, 6}, 10, 1000)
	res := NewEvaluator(reg, DefaultRules()).Run(bigHit(t, "nanite", 4000), a, Ctx{Caster: c, Subject: NoEntity, Level: 10})
	if res.Damage != 0 || res.Dots != 1 {
		t.Fatalf("nanite should queue, got %+v", res)
	}
	var total float32
	for i := 0; i < 8; i++ {
		total += a.Step(0.5)
	}
	// One 4000 hit would be capped to 250. Eight ticks of 500 each cap at
	// 250 separately, which is enough to kill.
	if a.Unit(f).Alive {
		t.Fatalf("ticks dealt %.1f and the foe lived", total)
	}
	approx(t, "total", total, 1000)
}

func TestDroneRaisesWhatItKills(t *testing.T) {
	a, c := world()
	f := a.Spawn(1, Coord{4, 6}, 1, 50)
	res := NewEvaluator(reg, DefaultRules()).Run(bigHit(t, "drone", 100), a, Ctx{Caster: c, Subject: NoEntity, Level: 20})
	u := a.Unit(f)
	if res.Kills != 1 || res.Raised != 1 || !u.Alive || u.Team != 0 {
		t.Fatalf("res %+v unit %+v", res, u)
	}
	approx(t, "minion hp", u.HP, 25)
}

func TestPlasmaSpreads(t *testing.T) {
	a, c := world()
	a.Spawn(1, Coord{4, 6}, 10, 1000)
	n := a.Spawn(1, Coord{5, 6}, 10, 1000)
	NewEvaluator(reg, DefaultRules()).Run(bigHit(t, "plasma", 100), a, Ctx{Caster: c, Subject: NoEntity, Level: 10})
	approx(t, "neighbor hp", a.Unit(n).HP, 1000-35)
}

func TestOnKillUsesCorpse(t *testing.T) {
	a, c := world()
	corpse := a.Spawn(1, Coord{6, 6}, 10, 1000)
	a.Units[corpse].Alive = false
	n1 := a.Spawn(1, Coord{7, 6}, 10, 1000)
	n2 := a.Spawn(1, Coord{6, 7}, 10, 1000)
	h := NewHex("x", [6]SocketKind{T, G, E, E, E, E})
	set(t, h, 0, NewTrigger(OnKill))
	set(t, h, 1, NewTarget(TargetAdjacent, 1, 0))
	set(t, h, 2, NewDamage(id("cryo"), 30))
	res := NewEvaluator(reg, DefaultRules()).Run(compile(t, h, 0), a, Ctx{Caster: c, Subject: corpse, Level: 10})
	if res.Hits != 2 || a.Unit(n1).HP != 970 || a.Unit(n2).HP != 970 {
		t.Fatalf("res %+v", res)
	}
}

func TestCloakStatusStealthsCaster(t *testing.T) {
	a, c := world()
	a.Spawn(1, Coord{4, 6}, 10, 1000)
	h := baseHex(t)
	set(t, h, 2, NewStatus(id("cloak"), 3))
	NewEvaluator(reg, DefaultRules()).Run(compile(t, h, 0), a, Ctx{Caster: c, Subject: NoEntity, Level: 10})
	approx(t, "stealth", a.Unit(c).Stealth, 3)
}

func TestOldCloakDamageOnlyHides(t *testing.T) {
	a, c := world()
	foe := a.Spawn(1, Coord{4, 6}, 10, 1000)
	h := baseHex(t)
	set(t, h, 2, NewDamage(id("cloak"), 16))
	res := NewEvaluator(reg, DefaultRules()).Run(compile(t, h, 0), a, Ctx{Caster: c, Subject: NoEntity, Level: 10})
	if res.Damage != 0 || a.Unit(foe).HP != 1000 {
		t.Fatalf("cloak hurt someone: %+v, foe hp %v", res, a.Unit(foe).HP)
	}
	approx(t, "stealth", a.Unit(c).Stealth, 3)
}

func TestChainAndCone(t *testing.T) {
	a, c := world()
	a.Facing[c] = 0 // east
	e1 := a.Spawn(1, Coord{4, 6}, 10, 1000)
	e2 := a.Spawn(1, Coord{6, 6}, 10, 1000)
	e3 := a.Spawn(1, Coord{8, 6}, 10, 1000)
	behind := a.Spawn(1, Coord{0, 6}, 10, 1000)
	var out [MaxTargets]EntityID
	n := a.Select(TargetChain, c, c, 3, out[:3])
	if n != 3 || out[0] != e1 || out[1] != e2 || out[2] != e3 {
		t.Fatalf("chain %v", out[:n])
	}
	n = a.Select(TargetCone, c, c, 4, out[:])
	for _, got := range out[:n] {
		if got == behind || got == e3 {
			t.Fatalf("cone picked %d: %v", got, out[:n])
		}
	}
	if n != 2 {
		t.Fatalf("cone %v", out[:n])
	}
}

func TestRunDoesNotAllocate(t *testing.T) {
	a, c := world()
	for i := 0; i < 6; i++ {
		a.Spawn(1, Coord{4 + i%3, 5 + i/3}, 10, 1e12)
	}
	ev := NewEvaluator(reg, DefaultRules())
	p := compile(t, targetsTargetHex(t, id("plasma"), 10), 1)
	ctx := Ctx{Caster: c, Subject: NoEntity, Level: 10}
	if n := testing.AllocsPerRun(1000, func() { ev.Run(p, a, ctx) }); n != 0 {
		t.Fatalf("Run allocated %.1f times per call", n)
	}
	if n := testing.AllocsPerRun(1000, func() { a.Step(0.5) }); n != 0 {
		t.Fatalf("Step allocated %.1f times per call", n)
	}
}

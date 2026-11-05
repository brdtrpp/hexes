package skill

import (
	"strings"
	"testing"
)

// sustainHex is a ring hex: trigger, then parts in slots 1 on.
func sustainHex(t testing.TB, trig Part, parts ...Part) *Hex {
	h := NewHex("sustain", [6]SocketKind{T, W, W, W, W, W})
	if err := h.Set(0, trig); err != nil {
		t.Fatal(err)
	}
	for i, p := range parts {
		if err := h.Set(1+i, p); err != nil {
			t.Fatal(err)
		}
	}
	return h
}

func TestMendOnKillIsLifesteal(t *testing.T) {
	a, c := world()
	corpse := a.Spawn(1, Coord{4, 6}, 10, 100)
	a.Unit(corpse).Alive, a.Unit(corpse).HP = false, 0
	a.Unit(c).HP = 500
	p := compile(t, sustainHex(t, NewTrigger(OnKill), NewMend(0.1)), 0)
	res := NewEvaluator(reg, DefaultRules()).Run(p, a, Ctx{Caster: c, Subject: corpse, Level: 10})
	approx(t, "caster hp", a.Unit(c).HP, 600)
	approx(t, "mended", res.Mended, 100)
}

func TestMendBesideATargetMendsTheTargets(t *testing.T) {
	a, c := world()
	f := a.Spawn(1, Coord{4, 6}, 10, 1000)
	a.Unit(f).HP = 100
	a.Unit(c).HP = 500
	p := compile(t, sustainHex(t, NewTrigger(OnCast), NewTarget(TargetNearest, 0, 1), NewMend(0.1)), 0)
	NewEvaluator(reg, DefaultRules()).Run(p, a, Ctx{Caster: c, Subject: NoEntity, Level: 10})
	approx(t, "foe hp", a.Unit(f).HP, 200)
	approx(t, "caster hp", a.Unit(c).HP, 500)
}

func TestMendHasDiminishingReturns(t *testing.T) {
	a, c := world()
	a.Unit(c).HP = 100
	p := compile(t, sustainHex(t, NewTrigger(OnCast), NewMend(0.1)), 0)
	ev := NewEvaluator(reg, DefaultRules())
	ctx := Ctx{Caster: c, Subject: NoEntity, Level: 10}
	ev.Run(p, a, ctx)
	approx(t, "first", a.Unit(c).HP, 200)
	ev.Run(p, a, ctx) // strain 1: half
	approx(t, "second", a.Unit(c).HP, 250)
	a.Step(StrainHalfLife) // strain 2 halves to 1
	ev.Run(p, a, ctx)
	approx(t, "after a rest", a.Unit(c).HP, 300)
}

// However fast a Mend fires, its hull a second stays under its amount.
func TestFastMendCantOutheal(t *testing.T) {
	for _, every := range []float32{0.05, 0.25, 1} {
		a, c := world()
		a.Unit(c).HP = 1
		a.Unit(c).MaxHP = 1e9 // never full
		p := compile(t, sustainHex(t, NewTrigger(OnCast), NewMend(1e-6)), 0)
		ev := NewEvaluator(reg, DefaultRules())
		ctx := Ctx{Caster: c, Subject: NoEntity, Level: 10}
		amount := float32(1e-6 * 1e9)
		const secs = 60
		for t := float32(0); t < secs; t += every {
			ev.Run(p, a, ctx)
			a.Step(every)
		}
		rate := (a.Unit(c).HP - 1) / secs
		if rate > 0.75*amount {
			t.Fatalf("firing every %gs mended %.0f%% of its amount a second", every, 100*rate/amount)
		}
		if every >= 1 && rate < 0.4*amount {
			t.Fatalf("a once-a-second mend only mended %.0f%% a second", 100*rate/amount)
		}
	}
}

func TestShieldSoaksThenFades(t *testing.T) {
	a, c := world()
	u := a.Unit(c)
	p := compile(t, sustainHex(t, NewTrigger(OnCast), NewShield(0.15)), 0)
	ev := NewEvaluator(reg, DefaultRules())
	res := ev.Run(p, a, Ctx{Caster: c, Subject: NoEntity, Level: 10})
	approx(t, "shield", u.Shield, 150)
	approx(t, "shielded", res.Shielded, 150)
	r := DefaultRules()
	d, _ := r.Resolve(u, 100, id("kinetic"), 10, false)
	approx(t, "dealt through shield", d, 0)
	approx(t, "shield left", u.Shield, 50)
	u.Resist = [MaxTypes]float32{} // the first hit built kinetic resistance
	d, _ = r.Resolve(u, 100, id("kinetic"), 10, false)
	approx(t, "dealt past shield", d, 50)
	approx(t, "hp", u.HP, 950)
	// Walls and pits go straight to hull.
	ev.Run(p, a, Ctx{Caster: c, Subject: NoEntity, Level: 10})
	before := u.Shield
	r.Resolve(u, 10, 0, 0, true)
	approx(t, "terrain skips the shield", u.Shield, before)
	a.Step(ShieldTime + 0.1)
	approx(t, "faded", u.Shield, 0)
}

func TestShieldCapsAndStrains(t *testing.T) {
	a, c := world()
	u := a.Unit(c)
	p := compile(t, sustainHex(t, NewTrigger(OnCast), NewShield(0.4)), 0)
	ev := NewEvaluator(reg, DefaultRules())
	ctx := Ctx{Caster: c, Subject: NoEntity, Level: 10}
	ev.Run(p, a, ctx)
	approx(t, "first", u.Shield, 400)
	ev.Run(p, a, ctx) // half of 400 = 200, capped at 600
	approx(t, "capped", u.Shield, ShieldCap*1000)
}

func TestPurgeCoolsAndCostsPower(t *testing.T) {
	a, c := world()
	h := sustainHex(t, NewTrigger(OnCast), NewPurge(20))
	p := compile(t, h, 0)
	if p.Heat != 0 {
		t.Fatalf("purge hex makes %.1f heat", p.Heat)
	}
	if want := float32(2 + 21); p.Power != want {
		t.Fatalf("purge hex costs %.1f power, want %.1f", p.Power, want)
	}
	rig := NewRig(Points{})
	rig.Heat = 50
	res := NewEvaluator(reg, DefaultRules()).Run(p, a, Ctx{Caster: c, Subject: NoEntity, Level: 10, Rig: &rig})
	approx(t, "heat", rig.Heat, 30)
	approx(t, "purged", res.Purged, 20)
	rig.Heat = 5
	NewEvaluator(reg, DefaultRules()).Run(p, a, Ctx{Caster: c, Subject: NoEntity, Level: 10, Rig: &rig})
	approx(t, "never below zero", rig.Heat, 0)
	// Purge runs with no foe around: it ignores targets.
	if steps(p, OpPurge)[0].Src != RegCaster {
		t.Fatalf("purge reads r%d\n%s", steps(p, OpPurge)[0].Src, p)
	}
}

func TestSustainInAGateBranch(t *testing.T) {
	// Hull gate on the nearest foe: mend in YES, shield in NO.
	for _, weak := range []bool{true, false} {
		a, c := world()
		f := a.Spawn(1, Coord{4, 6}, 10, 1000)
		if weak {
			a.Unit(f).HP = 100
		}
		a.Unit(c).HP = 500
		h := sustainHex(t, NewTrigger(OnCast), NewTarget(TargetNearest, 0, 1), NewGate(GateHull, 0.3), NewMend(0.1), NewShield(0.1))
		p := compile(t, h, 0)
		NewEvaluator(reg, DefaultRules()).Run(p, a, Ctx{Caster: c, Subject: NoEntity, Level: 10})
		cu, fu := a.Unit(c), a.Unit(f)
		mended := cu.HP > 500
		if mended != weak || (cu.Shield > 0) == weak {
			t.Fatalf("weak=%v: caster hp %.0f shield %.0f\n%s", weak, cu.HP, cu.Shield, p)
		}
		if fu.Shield > 0 || (weak && fu.HP != 100) {
			t.Fatal("a branch's sustain landed on the foe")
		}
		if why, ok := h.Use(3); !ok || !strings.Contains(why, "mends you, when someone passes") {
			t.Fatalf("explain: %q", why)
		}
	}
}

func TestDamageMultiplierScalesSustain(t *testing.T) {
	for _, mk := range []func(float32) Part{NewMend, NewShield, NewPurge} {
		base := mk(0)
		h := sustainHex(t, NewTrigger(OnCast), base, NewMult(VerbDamage, 2))
		p := compile(t, h, 0)
		if got := p.Steps[len(p.Steps)-1].Value; got != 2*base.Value {
			t.Fatalf("%s scaled to %g, want %g\n%s", base.Effect, got, 2*base.Value, p)
		}
		h = sustainHex(t, NewTrigger(OnCast), base, NewMult(VerbPush, 2))
		if why, ok := h.Use(2); ok {
			t.Fatalf("push booster on %s: %q", base.Effect, why)
		}
	}
}

func TestSustainTakesNoRelayOrType(t *testing.T) {
	for _, p := range []Part{NewMend(0), NewShield(0), NewPurge(0)} {
		if QuirkRelay.Fits(p) {
			t.Fatalf("%s fits relay", p.Effect)
		}
		if p.Effect.Typed() || !p.Effect.Sustain() {
			t.Fatalf("%s kinds", p.Effect)
		}
	}
}

func TestSustainHexIsNotInert(t *testing.T) {
	for _, s := range []Part{NewMend(0), NewShield(0), NewPurge(0)} {
		if p := compile(t, sustainHex(t, NewTrigger(OnCast), s), 0); p.Inert {
			t.Fatalf("%s hex is inert", s.Effect)
		}
	}
}

func TestSustainRunDoesNotAllocate(t *testing.T) {
	a, c := world()
	for i := 0; i < 6; i++ {
		a.Spawn(1, Coord{4 + i%3, 5 + i/3}, 10, 1e12)
	}
	h := sustainHex(t, NewTrigger(OnCast), NewTarget(TargetAdjacent, 3, 0), NewMend(0.1), NewShield(0.1), NewPurge(5), NewDamage(id("plasma"), 10))
	p := compile(t, h, 0)
	rig := NewRig(Points{})
	ev := NewEvaluator(reg, DefaultRules())
	ev.Obs = sustainSink{}
	ctx := Ctx{Caster: c, Subject: NoEntity, Level: 10, Rig: &rig}
	if n := testing.AllocsPerRun(1000, func() { ev.Run(p, a, ctx) }); n != 0 {
		t.Fatalf("Run allocated %.1f times per call", n)
	}
	if n := testing.AllocsPerRun(1000, func() { a.Step(0.1) }); n != 0 {
		t.Fatalf("Step allocated %.1f times per call", n)
	}
}

type sustainSink struct{}

func (sustainSink) Hit(_, _ EntityID, _ float32, _ TypeID, _ bool)   {}
func (sustainSink) Killed(_, _ EntityID)                             {}
func (sustainSink) Sustained(_, _ EntityID, _ EffectKind, _ float32) {}

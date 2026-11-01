package skill

import (
	"strings"
	"testing"
)

// novaHex is the preset Cryo Bloom shape: every kill charges A and chills
// what's around the corpse; at five, the spend gate empties A and the yes
// slot hits everything around with a big cryo burst.
func novaHex(t *testing.T) *Hex {
	h := NewHex("nova", [6]SocketKind{T, W, W, W, W, W})
	set(t, h, 0, NewTrigger(OnKill))
	set(t, h, 1, NewCharge(0, 1))
	set(t, h, 2, NewStatus(id("cryo"), 2))
	set(t, h, 3, NewTarget(TargetAdjacent, 2, 0))
	set(t, h, 4, NewSpendGate(0, 5))
	set(t, h, 5, NewDamage(id("plasma"), 50))
	return h
}

func TestChargeFiresOnEveryFifthKill(t *testing.T) {
	a, c := world()
	corpse := a.Spawn(1, Coord{4, 6}, 10, 100)
	a.Unit(corpse).Alive = false
	foe := a.Spawn(1, Coord{5, 6}, 10, 1e6)
	p := compile(t, novaHex(t), 0)
	ev := NewEvaluator(reg, DefaultRules())
	var bank Bank
	var fired []int
	for kill := 1; kill <= 12; kill++ {
		hp := a.Unit(foe).HP
		ev.Run(p, a, Ctx{Caster: c, Subject: corpse, Level: 10, Bank: &bank})
		if a.Unit(foe).HP < hp {
			fired = append(fired, kill)
		}
	}
	if len(fired) != 2 || fired[0] != 5 || fired[1] != 10 {
		t.Fatalf("nova went off on kills %v, want [5 10]\n%s", fired, p)
	}
	if bank[0] != 2 {
		t.Fatalf("bank A = %d after 12 kills, want 2", bank[0])
	}
	if p.Banks != 1 || p.Spend[0] != 5 {
		t.Fatalf("program banks %b spend %v", p.Banks, p.Spend)
	}
}

// A full spend gate with nobody to hit holds its charge until it has
// somewhere to go.
func TestSpendHoldsWithEmptyGroup(t *testing.T) {
	a, c := world()
	corpse := a.Spawn(1, Coord{4, 6}, 10, 100)
	a.Unit(corpse).Alive = false
	p := compile(t, novaHex(t), 0)
	ev := NewEvaluator(reg, DefaultRules())
	bank := Bank{9}
	ev.Run(p, a, Ctx{Caster: c, Subject: corpse, Level: 10, Bank: &bank})
	if bank[0] != 10 {
		t.Fatalf("bank A = %d, want 10 (counted, not spent)", bank[0])
	}
	foe := a.Spawn(1, Coord{5, 6}, 10, 1e6)
	ev.Run(p, a, Ctx{Caster: c, Subject: corpse, Level: 10, Bank: &bank})
	if bank[0] != 6 || a.Unit(foe).HP >= a.Unit(foe).MaxHP {
		t.Fatalf("bank A = %d, foe hp %.0f: the held charge should spend now", bank[0], a.Unit(foe).HP)
	}
}

func TestDrainStopsAtZeroAndBanksAreSeparate(t *testing.T) {
	a, c := world()
	h := NewHex("two", [6]SocketKind{T, W, W, W, W, W})
	set(t, h, 0, NewTrigger(OnCast))
	set(t, h, 1, NewCharge(0, 3))
	set(t, h, 2, NewDrain(1, 2))
	set(t, h, 3, NewCharge(1, 1))
	p := compile(t, h, 0)
	ev := NewEvaluator(reg, DefaultRules())
	var bank Bank
	for i := 0; i < 4; i++ {
		ev.Run(p, a, Ctx{Caster: c, Subject: NoEntity, Level: 10, Bank: &bank})
	}
	// A: +3 a run. B: drain 2 then add 1, from zero, every run: 0-2 is 0,
	// +1 is 1; then 1-2 is 0, +1 is 1 again.
	if bank[0] != 12 || bank[1] != 1 {
		t.Fatalf("banks %v, want A=12 B=1\n%s", bank, p)
	}
	if !p.UsesBank(0) || !p.UsesBank(1) || p.UsesBank(2) {
		t.Fatalf("bank mask %b", p.Banks)
	}
}

func TestChargeCapsAtMax(t *testing.T) {
	a, c := world()
	h := NewHex("big", [6]SocketKind{T, W, W, W, W, W})
	set(t, h, 0, NewTrigger(OnCast))
	set(t, h, 1, NewCharge(0, 1))
	p := compile(t, h, 0)
	bank := Bank{MaxCharge}
	NewEvaluator(reg, DefaultRules()).Run(p, a, Ctx{Caster: c, Subject: NoEntity, Bank: &bank})
	if bank[0] != MaxCharge {
		t.Fatalf("bank A = %d", bank[0])
	}
}

// A charge in a gate branch counts only when its half holds someone.
func TestChargeInBranchCountsOnlyWhenTaken(t *testing.T) {
	a, c := world()
	f := threeFoes(a)
	h := NewHex("branch", [6]SocketKind{T, G, W, W, W, W})
	set(t, h, 0, NewTrigger(OnCast))
	set(t, h, 1, NewTarget(TargetNearest, 0, 3))
	set(t, h, 2, NewGate(GateHull, 0.5))
	set(t, h, 3, NewCharge(0, 1)) // yes: someone under half hull
	set(t, h, 4, NewCharge(1, 1)) // no: someone healthy
	p := compile(t, h, 0)
	ev := NewEvaluator(reg, DefaultRules())
	var bank Bank
	ev.Run(p, a, Ctx{Caster: c, Subject: NoEntity, Level: 10, Bank: &bank})
	if bank != (Bank{0, 1}) {
		t.Fatalf("all healthy: banks %v", bank)
	}
	for _, id := range f {
		a.Unit(id).HP = 100
	}
	ev.Run(p, a, Ctx{Caster: c, Subject: NoEntity, Level: 10, Bank: &bank})
	if bank != (Bank{1, 1}) {
		t.Fatalf("all weak: banks %v", bank)
	}
}

func TestEmptyGate(t *testing.T) {
	for _, full := range []bool{false, true} {
		a, c := world()
		f := threeFoes(a)
		h := gateHex(t, NewEmptyGate(2))
		p := compile(t, h, 0)
		bank := Bank{}
		if full {
			bank[2] = 4
		}
		NewEvaluator(reg, DefaultRules()).Run(p, a, Ctx{Caster: c, Subject: NoEntity, Level: 10, Bank: &bank})
		for i, id := range f {
			hit, marked := hitAndMarked(a, id)
			if hit == full || marked != full {
				t.Fatalf("full=%v foe %d: hit=%v marked=%v", full, i, hit, marked)
			}
		}
		if full && bank[2] != 4 {
			t.Fatalf("empty gate changed the bank: %v", bank)
		}
	}
}

// The counter machine pieces: spend 1 is decrement-or-branch-on-zero and
// a charge is increment. Run as a loop by hand (self-call will run it for
// real), this moves A into both B and C through a nested hex, which shares
// the outer hex's banks.
func TestCounterMachineMove(t *testing.T) {
	a, c := world()
	a.Spawn(1, Coord{4, 6}, 10, 1e6)
	inner := NewHex("inc", [6]SocketKind{W, W, W, W, W, W})
	set(t, inner, 0, NewCharge(1, 1))
	set(t, inner, 1, NewCharge(2, 1))
	h := NewHex("move", [6]SocketKind{T, G, W, W, W, W})
	set(t, h, 0, NewTrigger(OnCast))
	set(t, h, 1, NewTarget(TargetNearest, 0, 1))
	set(t, h, 2, NewSpendGate(0, 1))
	if err := h.Nest(3, inner); err != nil {
		t.Fatal(err)
	}
	set(t, h, 4, NewCharge(3, 1)) // no: A was empty, flag done in D
	p := compile(t, h, 1)
	ev := NewEvaluator(reg, DefaultRules())
	bank := Bank{7}
	runs := 0
	for bank[3] == 0 && runs < 100 {
		ev.Run(p, a, Ctx{Caster: c, Subject: NoEntity, Level: 10, Bank: &bank})
		runs++
	}
	if bank != (Bank{0, 7, 7, 1}) || runs != 8 {
		t.Fatalf("banks %v after %d runs, want [0 7 7 1] after 8\n%s", bank, runs, p)
	}
}

// With no bank in the context (a basic strike), a run gets a scratch bank
// that starts empty every time.
func TestNilBankIsScratch(t *testing.T) {
	a, c := world()
	h := NewHex("s", [6]SocketKind{T, W, W, W, W, W})
	set(t, h, 0, NewTrigger(OnCast))
	set(t, h, 1, NewCharge(0, 1))
	p := compile(t, h, 0)
	ev := NewEvaluator(reg, DefaultRules())
	ev.Run(p, a, Ctx{Caster: c, Subject: NoEntity})
	ev.Run(p, a, Ctx{Caster: c, Subject: NoEntity})
	if ev.bank[0] != 1 {
		t.Fatalf("scratch bank A = %d, want 1 (reset each run)", ev.bank[0])
	}
}

// The dispatcher keeps a slot's banks between firings, in the caller's
// storage when given, so they survive a new dispatcher (a new floor).
func TestDispatcherKeepsBanks(t *testing.T) {
	a, c := world()
	a.Spawn(1, Coord{4, 6}, 10, 1e6)
	h := NewHex("tally", [6]SocketKind{T, W, W, W, W, W})
	set(t, h, 0, NewTrigger(OnCast))
	set(t, h, 1, NewCharge(1, 1))
	p := compile(t, h, 0)
	var keep [MaxEquip]Bank
	for floor := 0; floor < 2; floor++ {
		rig := NewRig(Points{})
		d := NewDispatcher(NewEvaluator(reg, DefaultRules()), a)
		d.Equip(c, &rig, nil, p).Banks = &keep
		for i := 0; i < 3; i++ {
			if o := d.Cast(c, 1, NoEntity); o != Fired {
				t.Fatalf("cast %s", o)
			}
		}
	}
	if keep[1][1] != 6 || keep[0] != (Bank{}) {
		t.Fatalf("kept banks %v", keep[:2])
	}
	// Without caller storage the loadout keeps its own.
	rig := NewRig(Points{})
	d := NewDispatcher(NewEvaluator(reg, DefaultRules()), a)
	l := d.Equip(c, &rig, p)
	d.Cast(c, 0, NoEntity)
	d.Cast(c, 0, NoEntity)
	if l.Bank(0)[1] != 2 {
		t.Fatalf("own bank %v", *l.Bank(0))
	}
}

func TestChargeCompileDetails(t *testing.T) {
	p := compile(t, novaHex(t), 0)
	s := p.String()
	for _, want := range []string{"bank A +1", "gate spend", "bank=A n=5"} {
		if !strings.Contains(s, want) {
			t.Fatalf("disassembly lacks %q:\n%s", want, s)
		}
	}
	h := novaHex(t)
	h.Clear(1)
	q := compile(t, h, 0)
	approx(t, "charge power", p.Power-q.Power, ChargePowerCost)
	// Charges go in effect and wild sockets, not target or trigger ones.
	g := NewHex("g", [6]SocketKind{T, G, E, W, W, W})
	if g.Set(1, NewCharge(0, 1)) == nil || g.Set(0, NewCharge(0, 1)) == nil {
		t.Fatal("charge fit a target or trigger socket")
	}
	if g.Set(2, NewCharge(0, 1)) != nil {
		t.Fatal("charge should fit an effect socket")
	}
	// Bank letters wrap and only bank parts take them.
	if NewCharge(3, 1).WithBank(4).Bank != 0 || NewDamage(0, 5).WithBank(2).Bank != 0 {
		t.Fatal("WithBank")
	}
	// An echo hex counts twice.
	e := NewHex("e", [6]SocketKind{T, W, W, W, W, W})
	trig := NewTrigger(OnCast)
	trig.Quirk = QuirkEcho
	set(t, e, 0, trig)
	set(t, e, 1, NewCharge(0, 1))
	a, c := world()
	var bank Bank
	NewEvaluator(reg, DefaultRules()).Run(compile(t, e, 0), a, Ctx{Caster: c, Subject: NoEntity, Bank: &bank})
	if bank[0] != 2 {
		t.Fatalf("echo counted %d", bank[0])
	}
}

func TestChargeRunDoesNotAllocate(t *testing.T) {
	a, c := world()
	corpse := a.Spawn(1, Coord{4, 6}, 10, 100)
	a.Unit(corpse).Alive = false
	a.Spawn(1, Coord{5, 6}, 10, 1e12)
	p := compile(t, novaHex(t), 0)
	ev := NewEvaluator(reg, DefaultRules())
	var bank Bank
	ctx := Ctx{Caster: c, Subject: corpse, Level: 10, Bank: &bank}
	if n := testing.AllocsPerRun(1000, func() { ev.Run(p, a, ctx) }); n != 0 {
		t.Fatalf("%v allocs per run", n)
	}
	ctx.Bank = nil
	if n := testing.AllocsPerRun(1000, func() { ev.Run(p, a, ctx) }); n != 0 {
		t.Fatalf("scratch bank: %v allocs per run", n)
	}
}

func BenchmarkChargeNova(b *testing.B) {
	a, c := benchArena()
	corpse := a.Spawn(1, Coord{4, 4}, 10, 100)
	a.Unit(corpse).Alive = false
	h := NewHex("nova", [6]SocketKind{T, W, W, W, W, W})
	for i, p := range []Part{NewTrigger(OnKill), NewCharge(0, 1), NewStatus(id("cryo"), 2),
		NewTarget(TargetAdjacent, 2, 0), NewSpendGate(0, 5), NewDamage(id("plasma"), 50)} {
		if err := h.Set(i, p); err != nil {
			b.Fatal(err)
		}
	}
	p, err := Compile(h, reg, 0)
	if err != nil {
		b.Fatal(err)
	}
	ev := NewEvaluator(reg, DefaultRules())
	var bank Bank
	ctx := Ctx{Caster: c, Subject: corpse, Level: 10, Bank: &bank}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ev.Run(p, a, ctx)
	}
}

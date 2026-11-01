package skill

import (
	"strings"
	"testing"
)

// A counter machine (a Minsky machine) is a list of numbered states, each
// either "add 1 to counter r, go to next" or "if counter r is zero go to
// alt, else take 1 off it and go to next". Two counters are enough for
// it to compute anything a Turing machine can. The hexes below build one
// state per hex out of parts a player can slot:
//
//	inc:   trigger · CHARGE r +1 · CALL next
//	decjz: trigger · SPEND r at 1 · CALL next (yes) · CALL alt (no)
//
// so hexes with Gates, Charges and Calls are Turing complete, with Heat
// and MaxCalls as the step budget.

type instr struct {
	inc       bool
	reg       uint8
	next, alt int // state to go to, -1 halts
}

func callTo(state int) uint8 { return uint8(state + 1) }

func machine(t testing.TB, prog []instr) *[MaxEquip]*Program {
	t.Helper()
	if len(prog) > MaxEquip {
		t.Fatalf("%d states, room for %d", len(prog), MaxEquip)
	}
	var out [MaxEquip]*Program
	for i, in := range prog {
		h := NewHex("state", [6]SocketKind{T, W, W, W, W, W})
		put := func(slot int, p Part) {
			if err := h.Set(slot, p); err != nil {
				t.Fatal(err)
			}
		}
		put(0, NewTrigger(OnCast))
		if in.inc {
			put(1, NewCharge(in.reg, 1))
			if in.next >= 0 {
				put(2, NewCall(callTo(in.next)))
			}
		} else {
			put(1, NewSpendGate(in.reg, 1))
			if in.next >= 0 {
				put(2, NewCall(callTo(in.next)))
			}
			if in.alt >= 0 {
				put(3, NewCall(callTo(in.alt)))
			}
		}
		p, err := Compile(h, reg, 0)
		if err != nil {
			t.Fatal(err)
		}
		out[i] = p
	}
	return &out
}

// runMachine starts state 0 with a foe standing by as every state's
// subject (spend gates need someone in their group).
func runMachine(t testing.TB, progs *[MaxEquip]*Program, bank *Bank) Result {
	a, c := world()
	foe := a.Spawn(1, Coord{5, 6}, 10, 1e9)
	ev := NewEvaluator(reg, DefaultRules())
	return ev.Run(progs[0], a, Ctx{Caster: c, Subject: foe, Level: 10, Bank: bank, Calls: progs})
}

const (
	A = iota
	B
	C
	D
)

func TestCallAddsAIntoB(t *testing.T) {
	progs := machine(t, []instr{
		{reg: A, next: 1, alt: -1},   // 0: A? A-- -> 1 : halt
		{inc: true, reg: B, next: 0}, // 1: B++ -> 0
	})
	bank := Bank{A: 5, B: 2}
	res := runMachine(t, progs, &bank)
	if bank[A] != 0 || bank[B] != 7 {
		t.Fatalf("A=%d B=%d, want A=0 B=7", bank[A], bank[B])
	}
	if res.Calls != 10 || res.Halted {
		t.Fatalf("calls %d halted %v, want 10 calls, ran to the end", res.Calls, res.Halted)
	}
}

// Multiplication needs a loop inside a loop and a temporary counter:
// C = A*B, with B put back from D after each pass.
var multiply = []instr{
	{reg: A, next: 1, alt: -1},   // 0: A? A-- -> 1 : halt
	{reg: B, next: 2, alt: 4},    // 1: B? B-- -> 2 : 4
	{inc: true, reg: C, next: 3}, // 2: C++ -> 3
	{inc: true, reg: D, next: 1}, // 3: D++ -> 1
	{reg: D, next: 5, alt: 0},    // 4: D? D-- -> 5 : 0
	{inc: true, reg: B, next: 4}, // 5: B++ -> 4
}

func TestCallMachineMultiplies(t *testing.T) {
	progs := machine(t, multiply)
	for _, c := range []struct{ a, b uint32 }{{3, 4}, {0, 5}, {5, 0}, {2, 9}} {
		bank := Bank{A: c.a, B: c.b}
		res := runMachine(t, progs, &bank)
		if bank[C] != c.a*c.b || bank[B] != c.b || bank[A] != 0 || bank[D] != 0 {
			t.Fatalf("%d x %d: bank %v, want C=%d, B=%d", c.a, c.b, bank, c.a*c.b, c.b)
		}
		if res.Halted {
			t.Fatalf("%d x %d: halted after %d calls", c.a, c.b, res.Calls)
		}
	}
}

// The budget is what keeps it a game: past MaxCalls the machine stops
// where it is, mid-sum, and says so.
func TestCallMachineStopsAtBudget(t *testing.T) {
	progs := machine(t, multiply)
	bank := Bank{A: 30, B: 30}
	res := runMachine(t, progs, &bank)
	if !res.Halted || res.Calls != MaxCalls {
		t.Fatalf("calls %d halted %v, want stopped at %d", res.Calls, res.Halted, MaxCalls)
	}
	if bank[C] == 0 || bank[C] >= 900 {
		t.Fatalf("C = %d, want a partial product", bank[C])
	}
}

func TestCallMachineDoesNotAllocate(t *testing.T) {
	progs := machine(t, multiply)
	a, c := world()
	foe := a.Spawn(1, Coord{5, 6}, 10, 1e9)
	ev := NewEvaluator(reg, DefaultRules())
	var bank Bank
	allocs := testing.AllocsPerRun(50, func() {
		bank = Bank{A: 3, B: 4}
		ev.Run(progs[0], a, Ctx{Caster: c, Subject: foe, Level: 10, Bank: &bank, Calls: progs})
	})
	if allocs != 0 {
		t.Fatalf("%v allocations per run", allocs)
	}
	if bank[C] != 12 {
		t.Fatalf("C = %d, want 12", bank[C])
	}
}

// One hex that counts itself down: spend 1 from A, and while that works,
// call itself again.
func countdownHex(t *testing.T) *Hex {
	h := NewHex("countdown", [6]SocketKind{T, W, W, W, W, W})
	set(t, h, 0, NewTrigger(OnCast))
	set(t, h, 1, NewSpendGate(A, 1))
	set(t, h, 2, NewCall(CallSelf))
	set(t, h, 4, NewDamage(id("plasma"), 10))
	return h
}

func TestSelfCallLoops(t *testing.T) {
	a, c := world()
	foe := a.Spawn(1, Coord{5, 6}, 10, 1e9)
	p := compile(t, countdownHex(t), 0)
	if !p.CallsHex(CallSelf) || !strings.Contains(p.String(), "call self") {
		t.Fatalf("program doesn't show its self call:\n%s", p)
	}
	bank := Bank{A: 7}
	res := NewEvaluator(reg, DefaultRules()).Run(p, a, Ctx{Caster: c, Subject: foe, Level: 10, Bank: &bank})
	if bank[A] != 0 || res.Calls != 7 || res.Hits != 8 {
		t.Fatalf("A=%d calls=%d hits=%d, want 0, 7 calls, 8 hits (first run and 7 loops)\n%s",
			bank[A], res.Calls, res.Hits, p)
	}
}

func runawayHex(t *testing.T) *Program {
	h := NewHex("runaway", [6]SocketKind{T, W, W, W, W, W})
	set(t, h, 0, NewTrigger(OnCast))
	set(t, h, 1, NewCall(CallSelf))
	return compile(t, h, 0)
}

// A loop with no way out runs until it overheats the rig, which locks
// like any overheat. That's the halting budget.
func TestRunawayLoopOverheats(t *testing.T) {
	a, c := world()
	foe := a.Spawn(1, Coord{5, 6}, 10, 1e9)
	rig := NewRig(Points{})
	rig.Heat = 40
	res := NewEvaluator(reg, DefaultRules()).Run(runawayHex(t), a, Ctx{Caster: c, Subject: foe, Level: 10, Rig: &rig})
	if !res.Halted || !rig.Locked() || rig.Heat != rig.MaxHeat {
		t.Fatalf("halted %v locked %v heat %.1f, want an overheat", res.Halted, rig.Locked(), rig.Heat)
	}
	if res.Calls != 60 {
		t.Fatalf("calls %d, want 60 (40 heat to 100 at 1 a call)", res.Calls)
	}
	// Locked, the next firing's calls don't run at all.
	res = NewEvaluator(reg, DefaultRules()).Run(runawayHex(t), a, Ctx{Caster: c, Subject: foe, Level: 10, Rig: &rig})
	if res.Calls != 0 || !res.Halted {
		t.Fatalf("locked rig ran %d calls", res.Calls)
	}
}

// With no rig at all, MaxCalls still ends it inside the frame.
func TestRunawayLoopStopsWithoutRig(t *testing.T) {
	a, c := world()
	foe := a.Spawn(1, Coord{5, 6}, 10, 1e9)
	res := NewEvaluator(reg, DefaultRules()).Run(runawayHex(t), a, Ctx{Caster: c, Subject: foe, Level: 10})
	if !res.Halted || res.Calls != MaxCalls {
		t.Fatalf("calls %d halted %v", res.Calls, res.Halted)
	}
}

// Rarer calls run cooler.
func TestRareCallRunsCooler(t *testing.T) {
	heatAfter := func(r Rarity) float32 {
		a, c := world()
		foe := a.Spawn(1, Coord{5, 6}, 10, 1e9)
		h := NewHex("once", [6]SocketKind{T, W, W, W, W, W})
		set(t, h, 0, NewTrigger(OnCast))
		set(t, h, 1, NewSpendGate(A, 1))
		set(t, h, 2, NewCall(CallSelf).WithRarity(r))
		bank := Bank{A: 1}
		rig := NewRig(Points{})
		NewEvaluator(reg, DefaultRules()).Run(compile(t, h, 0), a, Ctx{Caster: c, Subject: foe, Level: 10, Rig: &rig, Bank: &bank})
		return rig.Heat
	}
	common, exotic := heatAfter(Common), heatAfter(Exotic)
	if common <= 0 || exotic >= common {
		t.Fatalf("heat common %.2f exotic %.2f, want exotic cooler", common, exotic)
	}
}

// Calls queue in clockwise order. Hex 2 adds to B only while A is empty;
// hex 3 adds to A. Calling 2 then 3 adds to B; 3 then 2 doesn't.
func TestCallOrderIsClockwise(t *testing.T) {
	var progs [MaxEquip]*Program
	h2 := NewHex("b if a empty", [6]SocketKind{T, W, W, W, W, W})
	set(t, h2, 0, NewTrigger(OnCast))
	set(t, h2, 1, NewEmptyGate(A))
	set(t, h2, 2, NewCharge(B, 1))
	progs[1] = compile(t, h2, 0)
	h3 := NewHex("add a", [6]SocketKind{T, W, W, W, W, W})
	set(t, h3, 0, NewTrigger(OnCast))
	set(t, h3, 1, NewCharge(A, 1))
	progs[2] = compile(t, h3, 0)

	run := func(first, second uint8) Bank {
		h := NewHex("caller", [6]SocketKind{T, W, W, W, W, W})
		set(t, h, 0, NewTrigger(OnCast))
		set(t, h, 2, NewCall(first))
		set(t, h, 4, NewCall(second))
		progs[0] = compile(t, h, 0)
		a, c := world()
		foe := a.Spawn(1, Coord{5, 6}, 10, 1e9)
		var bank Bank
		res := NewEvaluator(reg, DefaultRules()).Run(progs[0], a, Ctx{Caster: c, Subject: foe, Level: 10, Bank: &bank, Calls: &progs})
		if res.Called != 0b110 {
			t.Fatalf("called %b, want hexes 2 and 3", res.Called)
		}
		return bank
	}
	if b := run(2, 3); b[B] != 1 {
		t.Fatalf("2 then 3: B=%d, want 1", b[B])
	}
	if b := run(3, 2); b[B] != 0 {
		t.Fatalf("3 then 2: B=%d, want 0", b[B])
	}
}

// A call beside a target runs once per foe it holds, each as the called
// hex's subject.
func TestCallRunsOncePerTarget(t *testing.T) {
	a, c := world()
	var foes []EntityID
	for i := 0; i < 3; i++ {
		foes = append(foes, a.Spawn(1, Coord{4 + i, 6}, 10, 1e6))
	}
	var progs [MaxEquip]*Program
	hit := NewHex("hit it", [6]SocketKind{T, W, W, W, W, W})
	set(t, hit, 0, NewTrigger(OnHit))
	set(t, hit, 1, NewDamage(id("plasma"), 10))
	progs[1] = compile(t, hit, 0)
	h := NewHex("fan out", [6]SocketKind{T, W, W, W, W, W})
	set(t, h, 0, NewTrigger(OnCast))
	set(t, h, 1, NewTarget(TargetNearest, 0, 3))
	set(t, h, 2, NewCall(2))
	progs[0] = compile(t, h, 0)
	res := NewEvaluator(reg, DefaultRules()).Run(progs[0], a, Ctx{Caster: c, Subject: NoEntity, Level: 10, Calls: &progs})
	if res.Calls != 3 || res.Hits != 3 {
		t.Fatalf("calls %d hits %d, want 3 and 3", res.Calls, res.Hits)
	}
	for _, f := range foes {
		if u := a.Unit(f); u.HP >= u.MaxHP {
			t.Fatalf("foe %d untouched", f)
		}
	}
}

// A call to a hex that isn't there, or with no table at all, does nothing.
func TestCallToNothing(t *testing.T) {
	a, c := world()
	foe := a.Spawn(1, Coord{5, 6}, 10, 1e9)
	h := NewHex("lonely", [6]SocketKind{T, E, W, W, W, W})
	set(t, h, 0, NewTrigger(OnCast))
	set(t, h, 1, NewCall(5)) // effect sockets take calls too
	p := compile(t, h, 0)
	var progs [MaxEquip]*Program
	for _, ctx := range []Ctx{{Caster: c, Subject: foe}, {Caster: c, Subject: foe, Calls: &progs}} {
		if res := NewEvaluator(reg, DefaultRules()).Run(p, a, ctx); res.Calls != 0 || res.Halted {
			t.Fatalf("%+v", res)
		}
	}
}

// Through the dispatcher: casting hex 1 runs hex 2 as well, its Heat
// lands on the rig.
func TestDispatcherCallsEquippedHex(t *testing.T) {
	a, c := world()
	foe := a.Spawn(1, Coord{5, 6}, 10, 30)
	h := NewHex("caller", [6]SocketKind{T, W, W, W, W, W})
	set(t, h, 0, NewTrigger(OnCast))
	set(t, h, 1, NewCall(2))
	hit := NewHex("callee", [6]SocketKind{T, W, W, W, W, W})
	set(t, hit, 0, NewTrigger(OnKill))
	set(t, hit, 1, NewDamage(id("plasma"), 200))
	callee := compile(t, hit, 0)
	ev := NewEvaluator(reg, DefaultRules())
	d := NewDispatcher(ev, a)
	rig := NewRig(Points{})
	d.Equip(c, &rig, compile(t, h, 0), callee)
	if o := d.Cast(c, 0, foe); o != Fired {
		t.Fatalf("cast %s", o)
	}
	if u := a.Unit(foe); u.HP >= u.MaxHP {
		t.Fatal("the called hex didn't land")
	}
	want := CallHeatCost + max(callee.Heat, CallMinHeat)
	approx(t, "rig heat", rig.Heat, want)
}

func BenchmarkCallMachine(b *testing.B) {
	progs := machine(b, multiply)
	a, c := world()
	foe := a.Spawn(1, Coord{5, 6}, 10, 1e9)
	ev := NewEvaluator(reg, DefaultRules())
	var bank Bank
	var res Result
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		bank = Bank{A: 3, B: 4}
		res = ev.Run(progs[0], a, Ctx{Caster: c, Subject: foe, Level: 10, Bank: &bank, Calls: progs})
	}
	b.ReportMetric(float64(res.Calls), "calls/run")
}

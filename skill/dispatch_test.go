package skill

import "testing"

// mk builds a hex from a trigger and parts laid clockwise from slot 0 on an
// all-wild ring (trigger socket in slot 0), every edge linked.
func mk(t testing.TB, name string, trig Part, parts ...Part) *Program {
	t.Helper()
	h := NewHex(name, [6]SocketKind{T, W, W, W, W, W})
	h.Slots[0] = Slot{Kind: SlotPart, Part: trig}
	for i, p := range parts {
		h.Slots[i+1] = Slot{Kind: SlotPart, Part: p}
	}
	p, err := Compile(h, reg, 4)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

type rec struct {
	hits, kills int
	fired       map[int]int
	outcomes    []Outcome
}

func (r *rec) Hit(caster, target EntityID, dealt float32, t TypeID, terrain bool) { r.hits++ }
func (r *rec) Killed(caster, target EntityID)                                     { r.kills++ }
func (r *rec) Fired(owner EntityID, slot int, p *Program, o Outcome, res Result) {
	if r.fired == nil {
		r.fired = map[int]int{}
	}
	if o == Fired {
		r.fired[slot]++
	}
	r.outcomes = append(r.outcomes, o)
}

func setup(t testing.TB) (*Arena, EntityID, *Dispatcher, *Rig, *rec) {
	a, c := world()
	d := NewDispatcher(NewEvaluator(reg, DefaultRules()), a)
	r := &rec{}
	d.Listener = r
	rig := NewRig(Points{Capacity: 10, Recharge: 10, Cooling: 10})
	return a, c, d, &rig, r
}

func TestCastSpendsAndFires(t *testing.T) {
	a, c, d, rig, r := setup(t)
	foe := a.Spawn(1, Coord{4, 6}, 10, 1000)
	bolt := mk(t, "bolt", NewTrigger(OnCast), NewTarget(TargetNearest, 0, 1), NewDamage(id("kinetic"), 20))
	d.Equip(c, rig, bolt)

	p0 := rig.Power
	if o := d.Cast(c, 0, NoEntity); o != Fired {
		t.Fatalf("cast: %s", o)
	}
	approx(t, "power spent", p0-rig.Power, bolt.Power)
	approx(t, "heat", rig.Heat, bolt.Heat)
	if a.Units[foe].HP >= 1000 || r.fired[0] != 1 {
		t.Fatalf("foe hp %.0f fired %v", a.Units[foe].HP, r.fired)
	}
	if o := d.Cast(c, 1, NoEntity); o != Empty {
		t.Fatalf("empty slot: %s", o)
	}
	rig.Power = 0
	if o := d.Cast(c, 0, NoEntity); o != NoPower {
		t.Fatalf("no power: %s", o)
	}
}

func TestOverheatLocksAndVent(t *testing.T) {
	a, c, d, rig, _ := setup(t)
	a.Spawn(1, Coord{4, 6}, 10, 1e6)
	hot := mk(t, "hot", NewTrigger(OnCast), NewTarget(TargetNearest, 0, 1),
		NewDamage(id("plasma"), 40), NewMult(VerbDamage, 3))
	d.Equip(c, rig, hot)
	if hot.Heat < 10 {
		t.Fatalf("test hex too cool: %.1f", hot.Heat)
	}
	casts := 0
	for d.Cast(c, 0, NoEntity) == Fired {
		rig.Power = rig.MaxPower
		casts++
		if casts > 50 {
			t.Fatal("never overheated")
		}
	}
	if !rig.Locked() || rig.Heat != rig.MaxHeat {
		t.Fatalf("after %d casts heat %.0f locked %v", casts, rig.Heat, rig.Locked())
	}
	if o := d.Cast(c, 0, NoEntity); o != Locked {
		t.Fatalf("locked rig cast: %s", o)
	}
	d.Update(OverheatShutdown + 0.01)
	if rig.Locked() {
		t.Fatal("still locked after shutdown time")
	}
	if !rig.Vent() || rig.Heat != 0 || !rig.Locked() {
		t.Fatalf("vent: heat %.0f locked %v", rig.Heat, rig.Locked())
	}
	if rig.Vent() {
		t.Fatal("vented twice")
	}
}

func TestOnHitChainsAndStops(t *testing.T) {
	a, c, d, rig, r := setup(t)
	foe := a.Spawn(1, Coord{4, 6}, 10, 1e6)
	bolt := mk(t, "bolt", NewTrigger(OnCast), NewTarget(TargetNearest, 0, 1), NewDamage(id("cryo"), 10))
	echo := mk(t, "echo", NewTrigger(OnHit), NewDamage(id("cryo"), 5))
	d.Equip(c, rig, bolt, echo)

	// With the default proc cooldown, one cast's hit fires echo once; echo's
	// own hit lands while echo is cooling down.
	d.Cast(c, 0, NoEntity)
	if r.fired[1] != 1 || d.Stats.OnCooldown != 1 {
		t.Fatalf("echo fired %d, cooldown refusals %d", r.fired[1], d.Stats.OnCooldown)
	}
	// Without a cooldown, MaxChain stops it: hits at chain 1, 2, 3 fire
	// echo, the chain-4 hit is cut.
	d.ProcCooldown[OnHit] = 0
	d.Loadout(c).Cool = [MaxEquip]float32{}
	r.fired = nil
	d.Stats = Stats{}
	d.Cast(c, 0, NoEntity)
	if r.fired[1] != int(d.MaxChain) || d.Stats.ChainCut != 1 {
		t.Fatalf("echo fired %d times, chain cuts %d", r.fired[1], d.Stats.ChainCut)
	}
	if a.Units[foe].Alive == false {
		t.Fatal("foe died")
	}
}

func TestOnKillFiresPerCorpse(t *testing.T) {
	a, c, d, rig, r := setup(t)
	for q := 3; q <= 5; q++ {
		a.Spawn(1, Coord{q, 6}, 0, 10) // level gap 10: one-shot
	}
	far := a.Spawn(1, Coord{10, 10}, 10, 1000)
	nova := mk(t, "nova", NewTrigger(OnCast), NewTarget(TargetAdjacent, 4, 0), NewDamage(id("kinetic"), 100))
	// On kill: hit the nearest foe to the corpse.
	reap := mk(t, "reap", NewTrigger(OnKill), NewTarget(TargetNearest, 0, 1), NewDamage(id("cryo"), 5))
	d.Equip(c, rig, nova, reap)
	d.Cast(c, 0, NoEntity)
	if r.kills != 3 || r.fired[1] != 3 {
		t.Fatalf("kills %d, reap fired %d", r.kills, r.fired[1])
	}
	if a.Units[far].HP >= 1000 {
		t.Fatal("reap never reached the far foe")
	}
}

func TestPeriodic(t *testing.T) {
	a, c, d, rig, r := setup(t)
	a.Spawn(1, Coord{3, 6}, 10, 1e6)
	pulse := mk(t, "pulse", NewPeriodic(2), NewTarget(TargetAdjacent, 2, 0), NewDamage(id("nanite"), 8))
	d.Equip(c, rig, pulse)
	for i := 0; i < 70; i++ { // 7 seconds in 0.1 s frames
		d.Update(0.1)
	}
	if r.fired[0] != 3 {
		t.Fatalf("periodic fired %d times in 7 s, want 3", r.fired[0])
	}
}

func TestSurpriseTriggers(t *testing.T) {
	a, c, d, rig, r := setup(t)
	foe := a.Spawn(1, Coord{3, 6}, 10, 1e6)
	stab := mk(t, "stab", NewTrigger(FromStealth), NewDamage(id("kinetic"), 10))
	back := mk(t, "back", NewTrigger(FromBehind), NewDamage(id("kinetic"), 10))
	d.Equip(c, rig, stab, back)

	// Foe at (3,6) faces east (away from the caster at (2,6)): behind.
	a.Units[c].Stealth = 2
	d.Strike(c, nil, foe)
	if r.fired[0] != 1 || r.fired[1] != 1 {
		t.Fatalf("fired %v", r.fired)
	}
	if a.Units[c].Stealth != 0 {
		t.Fatal("strike didn't break stealth")
	}
	// Turn the foe to face the caster: neither fires.
	a.Facing[foe] = 3
	d.Strike(c, nil, foe)
	if r.fired[0] != 1 || r.fired[1] != 1 {
		t.Fatalf("fired %v facing the caster", r.fired)
	}
}

func TestOnHurt(t *testing.T) {
	a, c, d, rig, r := setup(t)
	foe := a.Spawn(1, Coord{3, 6}, 10, 1e6)
	thorns := mk(t, "thorns", NewTrigger(OnHurt), NewDamage(id("kinetic"), 10))
	d.Equip(c, rig, thorns)
	d.Hurt(c, foe)
	d.Hurt(c, foe) // inside the 0.5 s cooldown
	if r.fired[0] != 1 || a.Units[foe].HP >= 1e6 {
		t.Fatalf("thorns fired %d", r.fired[0])
	}
}

func TestStrikeIsFreeAndTriggers(t *testing.T) {
	a, c, d, rig, r := setup(t)
	foe := a.Spawn(1, Coord{3, 6}, 10, 1e6)
	swing := mk(t, "swing", NewTrigger(OnCast), NewTarget(TargetNearest, 0, 1), NewDamage(id("kinetic"), 10))
	echo := mk(t, "echo", NewTrigger(OnHit), NewDamage(id("cryo"), 5))
	d.Equip(c, rig, echo)
	p0 := rig.Power
	d.Strike(c, swing, foe)
	if r.fired[0] != 1 {
		t.Fatalf("echo fired %d", r.fired[0])
	}
	approx(t, "power", p0-rig.Power, echo.Power) // only echo cost anything
}

// The whole cast, hit, proc, kill path must not allocate.
func TestDispatchNoAllocs(t *testing.T) {
	a, c, d, rig, _ := setup(t)
	d.Listener = nil
	a.Spawn(1, Coord{4, 6}, 10, 1e9)
	a.Spawn(1, Coord{5, 6}, 10, 1e9)
	bolt := mk(t, "bolt", NewTrigger(OnCast), NewTarget(TargetChain, 3, 3), NewDamage(id("plasma"), 10))
	echo := mk(t, "echo", NewTrigger(OnHit), NewTarget(TargetAdjacent, 2, 0), NewDamage(id("cryo"), 5))
	pulse := mk(t, "pulse", NewPeriodic(0.05), NewTarget(TargetAdjacent, 3, 0), NewDamage(id("nanite"), 5))
	d.Equip(c, rig, bolt, echo, pulse)
	allocs := testing.AllocsPerRun(200, func() {
		rig.Power, rig.Heat, rig.ShutdownLeft = rig.MaxPower, 0, 0
		d.Cast(c, 0, NoEntity)
		d.Update(0.1)
	})
	if allocs != 0 {
		t.Fatalf("%.1f allocs per cast", allocs)
	}
	if d.Stats.Fired == 0 {
		t.Fatal("nothing fired")
	}
}

func BenchmarkDispatch(b *testing.B) {
	a, c, d, rig, _ := setup(b)
	d.Listener = nil
	for q := 4; q < 10; q++ {
		a.Spawn(1, Coord{q, 6}, 10, 1e9)
	}
	bolt := mk(b, "bolt", NewTrigger(OnCast), NewTarget(TargetChain, 3, 3), NewDamage(id("plasma"), 10))
	echo := mk(b, "echo", NewTrigger(OnHit), NewTarget(TargetAdjacent, 1, 0), NewDamage(id("cryo"), 5))
	d.Equip(c, rig, bolt, echo)
	d.ProcCooldown[OnHit] = 0
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rig.Power, rig.Heat = rig.MaxPower, 0
		d.Cast(c, 0, NoEntity)
	}
	b.ReportMetric(float64(d.Stats.Fired)/float64(b.N), "firings/op")
}

// A cast out of stealth breaks it, but its own cloak gate still passes:
// "if hidden" reads "this cast came out of hiding". Once stealth ends a
// cloak can't hide you again until the rest runs out.
func TestCloakGateAndRest(t *testing.T) {
	a, c, d, rig, r := setup(t)
	foe := a.Spawn(1, Coord{3, 6}, 10, 1e6)
	h := NewHex("ambush", [6]SocketKind{T, W, W, W, W, W})
	set(t, h, 0, NewTrigger(OnCast))
	set(t, h, 1, NewTarget(TargetNearest, 0, 1))
	set(t, h, 2, NewGate(GateStatus, 0))
	h.Slots[2].Part.Type = id("cloak")
	set(t, h, 3, NewDamage(id("kinetic"), 10))
	d.Equip(c, rig, compile(t, h, 0))
	a.Units[c].Stealth = 2
	before := a.Unit(foe).HP
	d.Cast(c, 0, foe)
	if r.fired[0] != 1 || a.Unit(foe).HP >= before {
		t.Fatalf("the ambush gate didn't pass: fired %v, hp %v", r.fired, a.Unit(foe).HP)
	}
	u := a.Unit(c)
	if u.Stealth != 0 || u.Hidden || u.StealthRest <= 0 {
		t.Fatalf("after the cast: stealth %v hidden %v rest %v", u.Stealth, u.Hidden, u.StealthRest)
	}
	hide := baseHex(t)
	set(t, hide, 2, NewStatus(id("cloak"), 3))
	NewEvaluator(reg, DefaultRules()).Run(compile(t, hide, 0), a, Ctx{Caster: c, Subject: NoEntity, Level: 10})
	if u.Stealth != 0 {
		t.Fatal("a cloak hid you during the rest")
	}
	u.TickStealth(StealthRest)
	NewEvaluator(reg, DefaultRules()).Run(compile(t, hide, 0), a, Ctx{Caster: c, Subject: NoEntity, Level: 10})
	if u.Stealth == 0 {
		t.Fatal("a cloak didn't hide you after the rest")
	}
}

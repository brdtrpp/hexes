package skill

import "testing"

// board lays parts loose on the open board, each on its own cell.
func openBoard(t *testing.T, parts ...Part) *Hex {
	t.Helper()
	h := wild()
	for i, p := range parts {
		set(t, h, i, p)
	}
	h.Lay = Layout{Room: RoomBits(19), Wired: true}
	for i := range parts {
		c, ok := h.freeCell(Cell{})
		if !ok {
			t.Fatal("no room")
		}
		h.Lay.Pos[i] = Place{At: c, On: true}
	}
	return h
}

func wire(t *testing.T, h *Hex, a, b int, arm uint8) {
	t.Helper()
	if ok, why := h.WireWhy(a, b); !ok || !h.Wire(a, b, arm) {
		t.Fatalf("can't wire %d to %d: %s", a, b, why)
	}
}

func TestOverloadRungs(t *testing.T) {
	h := openBoard(t, NewTrigger(Overload), NewTarget(TargetNearest, 3, 1), NewDamage(id("kinetic"), 10),
		NewTarget(TargetCone, 3, 0), NewDamage(id("kinetic"), 30))
	wire(t, h, 0, 1, 1)
	wire(t, h, 1, 2, 0)
	wire(t, h, 0, 3, 3)
	wire(t, h, 3, 4, 0)
	if r := h.RungOf(); r[1] != 0 || r[2] != 0 || r[3] != 3 || r[4] != 3 {
		t.Fatalf("rungs %v", r)
	}
	p := compile(t, h, 0)
	if len(steps(p, OpRung)) == 0 {
		t.Fatalf("no rung check:\n%s", p)
	}
	hp := func(rung uint8) float32 {
		a, c := world()
		f := a.Spawn(1, Coord{4, 6}, 10, 1000)
		NewEvaluator(reg, DefaultRules()).Run(p, a, Ctx{Caster: c, Subject: NoEntity, Level: 10, Rung: rung})
		return a.Unit(f).HP
	}
	low, high := hp(1), hp(3)
	if low != 990 || high >= low {
		t.Fatalf("rung I leaves %v, rung III %v\n%s", low, high, p)
	}
	if hp(2) != low {
		t.Fatal("rung II reached the rung III cone")
	}
}

func TestGuardStoresAndReleases(t *testing.T) {
	a, c, d, rig, _ := setup(t)
	foe := a.Spawn(1, Coord{4, 6}, 10, 1000)
	g := mk(t, "guard", NewTrigger(Guard), NewTarget(TargetNearest, 0, 1), NewDamage(id("kinetic"), 20))
	l := d.Equip(c, rig, g)
	if d.Press(c, 0, NoEntity) != Fired || !l.Guarding() {
		t.Fatal("guard didn't go up")
	}
	if got := l.Soaked(100); got != 50 {
		t.Fatalf("a guarded hit of 100 lets %v through", got)
	}
	if o := d.Release(c, NoEntity); o != Fired || l.Guarding() {
		t.Fatalf("release: %s", o)
	}
	// 50 stored on a 1000 hull: x1.3.
	approx(t, "guard release", 1000-a.Units[foe].HP, 20*1.3)
}

func TestOverloadHeldLonger(t *testing.T) {
	h := openBoard(t, NewTrigger(Overload), NewTarget(TargetNearest, 3, 1), NewDamage(id("kinetic"), 10),
		NewDamage(id("kinetic"), 30))
	wire(t, h, 0, 1, 1)
	wire(t, h, 1, 2, 0)
	compile(t, h, 0)
	a, c, d, rig, _ := setup(t)
	foe := a.Spawn(1, Coord{4, 6}, 10, 1000)
	h2 := openBoard(t, NewTrigger(Overload), NewTarget(TargetNearest, 3, 1), NewDamage(id("kinetic"), 10),
		NewTarget(TargetNearest, 3, 1), NewDamage(id("kinetic"), 30))
	wire(t, h2, 0, 1, 1)
	wire(t, h2, 1, 2, 0)
	wire(t, h2, 0, 3, 3)
	wire(t, h2, 3, 4, 0)
	d.Equip(c, rig, compile(t, h2, 0))
	d.Press(c, 0, NoEntity)
	d.Update(0.2)
	d.Release(c, NoEntity)
	approx(t, "tap", 1000-a.Units[foe].HP, 10)
	d.Update(1) // cool off
	d.Press(c, 0, NoEntity)
	for i := 0; i < 20; i++ {
		d.Update(0.1)
	}
	d.Release(c, NoEntity)
	if got := 1000 - a.Units[foe].HP; got < 10+30 {
		t.Fatalf("held to rung III dealt %v in all", got)
	}
}

func TestRageRungsAndSpend(t *testing.T) {
	_, c, d, rig, _ := setup(t)
	l := d.Equip(c, rig, mk(t, "rage", NewTrigger(Rage), NewTarget(TargetNearest, 0, 1), NewDamage(id("kinetic"), 20)))
	l.Rage = 80
	if RageRung(l.Rage) != 3 {
		t.Fatal("80 rage isn't rung III")
	}
	d.Press(c, 0, NoEntity)
	if l.Rage > 20 {
		t.Fatalf("rung III left %v rage", l.Rage)
	}
}

func TestFollowUpLandsOnLastFoe(t *testing.T) {
	a, c, d, rig, _ := setup(t)
	near := a.Spawn(1, Coord{4, 6}, 10, 1000)
	far := a.Spawn(1, Coord{7, 6}, 10, 1000)
	first := mk(t, "jab", NewTrigger(OnCast), NewDamage(id("kinetic"), 10))
	// The follow-up's own reach would pick the far foe if it ran fresh.
	fu := mk(t, "fu", NewTrigger(FollowUp), NewDamage(id("kinetic"), 20))
	l := d.Equip(c, rig, first, fu)
	d.Cast(c, 0, far)
	if foe, _, ok := l.Following(); !ok || foe != far {
		t.Fatalf("no follow-up window on the foe hit (%v %v)", foe, ok)
	}
	d.Press(c, 1, NoEntity)
	if got := 1000 - a.Units[far].HP; got < 10+20*1.2 {
		t.Fatalf("follow-up dealt %v in all, not scaled up", got)
	}
	if a.Units[near].HP != 1000 {
		t.Fatal("follow-up hit the wrong foe")
	}
}

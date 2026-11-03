package skill

import "testing"

// A channel hex runs at once on the press, then every ChannelEvery while
// CAST is held, and stops when it's let go.
func TestChannelRunsWhileHeld(t *testing.T) {
	a, c, d, rig, r := setup(t)
	a.Spawn(1, Coord{4, 6}, 10, 100000)
	beam := mk(t, "beam", NewTrigger(Channel), NewTarget(TargetNearest, 0, 1), NewDamage(id("kinetic"), 5))
	d.Equip(c, rig, beam)
	if d.Cast(c, 0, NoEntity) != Empty {
		t.Fatal("a channel hex answered a tap")
	}
	d.Channel(c, true)
	for i := 0; i < 10; i++ { // 1 s held: the press, then every 0.25 s
		d.Update(0.1)
	}
	if n := r.fired[0]; n != 4 && n != 5 {
		t.Fatalf("fired %d times in a second", n)
	}
	d.Channel(c, false)
	before := r.fired[0]
	for i := 0; i < 10; i++ {
		d.Update(0.1)
	}
	if r.fired[0] != before {
		t.Fatal("kept firing after letting go")
	}
}

// Culler and Mercy: a watch on a finder fires when it finds anyone, hits
// who it found, pays WatchHeat on every check and rests after firing.
func TestWatchLibrary(t *testing.T) {
	a, c, d, rig, r := setup(t)
	weak := a.Spawn(1, Coord{4, 6}, 10, 1000)
	strong := a.Spawn(1, Coord{3, 6}, 10, 1000)
	culler := module(t, "Culler", Shape{Kind: ShapeTriangle}, map[int]Part{
		1: NewTarget(TargetNearest, 0, 5),
		2: NewGate(GateHull, 0.2),
	})
	w := NewTrigger(Watch)
	w.Lib = "Culler"
	h := shaped(t, ShapeTriangle, map[int]Part{0: w, 2: NewDamage(id("kinetic"), 40)})
	h.Libs[0] = culler
	if use, ok := h.Use(0); !ok {
		t.Fatalf("watch use: %s", use)
	}
	p := compile(t, h, 0)
	if p.WatchSteps == 0 {
		t.Fatalf("no check\n%s", p)
	}
	d.Equip(c, rig, p)
	for i := 0; i < 8; i++ {
		d.Update(0.25)
	}
	if r.fired[0] != 0 || a.Unit(weak).HP != 1000 {
		t.Fatal("fired with nobody under 20%")
	}
	if d.Stats.Checks < 6 {
		t.Fatalf("only %d checks in 2 s", d.Stats.Checks)
	}
	a.Unit(weak).HP = 100
	for i := 0; i < 2; i++ {
		d.Update(0.25)
	}
	if r.fired[0] != 1 || a.Unit(weak).HP >= 100 || a.Unit(strong).HP != 1000 {
		t.Fatalf("fired %d, weak %v strong %v", r.fired[0], a.Unit(weak).HP, a.Unit(strong).HP)
	}
	// A watched library that acts is refused.
	bad := module(t, "Bad", Shape{Kind: ShapeTriangle}, map[int]Part{2: NewDamage(id("kinetic"), 5)})
	h.Libs[0] = bad
	if _, err := Compile(h, reg, 4); err == nil {
		t.Fatal("watched a library that acts")
	}
	// Watching nothing compiles and never fires.
	h.Libs[0] = nil
	if p := compile(t, h, 0); p.WatchSteps != 0 {
		t.Fatal("a missing library has a check")
	}
}

// A condition watch tests the foes near the caster; one that reads only
// the rig fires on the nearest foe.
func TestWatchCondition(t *testing.T) {
	w := NewTrigger(Watch)
	w.Cond = cond(Tile{K: TileHull}, Tile{K: TileLess, V: 0.5})
	h := shaped(t, ShapeTriangle, map[int]Part{0: w, 2: NewDamage(id("kinetic"), 40)})
	p := compile(t, h, 0)
	a, c, d, rig, r := setup(t)
	near := a.Spawn(1, Coord{4, 6}, 10, 1000)
	far := a.Spawn(1, Coord{9, 6}, 10, 1000)
	a.Unit(far).HP = 100
	d.Equip(c, rig, p)
	d.Update(0.25)
	if r.fired[0] != 0 {
		t.Fatal("fired on a hurt foe out of range")
	}
	a.Unit(near).HP = 400
	d.Update(0.25)
	if r.fired[0] != 1 || a.Unit(near).HP >= 400 || a.Unit(far).HP != 100 {
		t.Fatalf("fired %d near %v far %v", r.fired[0], a.Unit(near).HP, a.Unit(far).HP)
	}

	w.Cond = cond(Tile{K: TileHeat}, Tile{K: TileMore, V: 0.5})
	h = shaped(t, ShapeTriangle, map[int]Part{0: w, 2: NewDamage(id("kinetic"), 40)})
	p = compile(t, h, 0)
	a, c, d, rig, r = setup(t)
	foe := a.Spawn(1, Coord{9, 6}, 10, 1000)
	d.Equip(c, rig, p)
	d.Update(0.25)
	if r.fired[0] != 0 {
		t.Fatal("fired on a cool rig")
	}
	rig.Heat = rig.MaxHeat * 0.8
	rig.CoolRate = 0
	d.Update(0.25)
	if r.fired[0] != 1 || a.Unit(foe).HP == 1000 {
		t.Fatalf("hot rig: fired %d, foe %v", r.fired[0], a.Unit(foe).HP)
	}
}

// Board banks X and Y are shared: one hex charges X, another spends it.
func TestBoardBanks(t *testing.T) {
	a, c, d, rig, _ := setup(t)
	foe := a.Spawn(1, Coord{4, 6}, 10, 100000)
	x := uint8(MaxBanks)
	if BankName(x) != "X" || BankName(x+1) != "Y" {
		t.Fatal("bank names")
	}
	counter := mk(t, "counter", NewTrigger(OnCast), NewCharge(x, 1))
	spender := mk(t, "spender", NewTrigger(OnCast), NewTarget(TargetNearest, 0, 1),
		NewSpendGate(x, 3), NewDamage(id("kinetic"), 50))
	var board BoardBank
	d.Equip(c, rig, counter, spender).Board = &board
	for i := 0; i < 3; i++ {
		rig.Power, rig.Heat = rig.MaxPower, 0
		d.Cast(c, 0, NoEntity)
	}
	if board[0] != 3 {
		t.Fatalf("X = %d", board[0])
	}
	rig.Power, rig.Heat = rig.MaxPower, 0
	d.Cast(c, 1, NoEntity)
	if board[0] != 0 || a.Unit(foe).HP == 100000 {
		t.Fatalf("spend: X = %d, foe %v", board[0], a.Unit(foe).HP)
	}
	if !spender.UsesBank(int(x)) {
		t.Fatal("spender doesn't list X")
	}
}

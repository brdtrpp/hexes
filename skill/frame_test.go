package skill

import (
	"encoding/json"
	"strings"
	"testing"
)

// framed is the base hex (cast, nearest 1) with a part in slot 2 sitting
// in frame f.
func framed(t *testing.T, f Frame, p Part) *Hex {
	t.Helper()
	h := baseHex(t)
	h.Frames[2] = f
	set(t, h, 2, p)
	return h
}

func plainOf(t *testing.T, p Part) *Program {
	t.Helper()
	h := baseHex(t)
	set(t, h, 2, p)
	return compile(t, h, 0)
}

func TestDoublerFiresTwiceHeatTwicePowerOnce(t *testing.T) {
	dmg := NewDamage(id("plasma"), 20)
	plain := plainOf(t, dmg)
	p := compile(t, framed(t, NewFrame(BendDoubler), dmg), 0)
	if d := steps(p, OpDamage); len(d) != 2 {
		t.Fatalf("damage steps %d, want 2\n%s", len(d), p)
	}
	approx(t, "power", p.Power, plain.Power)
	base := baseHex(t)
	bp := compile(t, base, 0)
	approx(t, "heat", p.Heat-bp.Heat, 2*(plain.Heat-bp.Heat))
}

func TestFrameRefusesWhatItDoesntTake(t *testing.T) {
	h := baseHex(t)
	f := NewFrame(BendDoubler)
	f.Typed, f.Type = true, id("cryo")
	h.Frames[2] = f
	if err := h.Set(2, NewDamage(id("plasma"), 10)); err == nil {
		t.Fatal("a cryo Doubler took plasma")
	}
	if err := h.Set(2, NewMult(VerbDamage, 2)); err == nil {
		t.Fatal("a Doubler took a multiplier")
	}
	set(t, h, 2, NewDamage(id("cryo"), 10))
	if ok, why := h.Fits(2, NewDamage(id("plasma"), 10), reg); ok || !strings.Contains(why, "cryo") {
		t.Fatalf("why = %q", why)
	}
	// A trigger slot takes a plain frame or a Hair trigger, nothing else.
	if f := NewFrame(BendSiphon); f.AcceptsTrigger() {
		t.Fatal("a Siphon sat under a trigger")
	}
	if !NewFrame(BendHair).AcceptsTrigger() || !(Frame{}).AcceptsTrigger() {
		t.Fatal("hair and plain take triggers")
	}
	h.Frames[3] = Frame{Ranked: true, Rank: Rare}
	if ok, _ := h.Fits(3, NewMult(VerbDamage, 2), reg); ok {
		t.Fatal("a rare-only frame took a common part")
	}
	if ok, _ := h.Fits(3, NewMult(VerbDamage, 2).WithRarity(Rare), reg); !ok {
		t.Fatal("a rare-only frame refused a rare part")
	}
}

func TestKillLockFiresAgainOnlyAfterAKill(t *testing.T) {
	run := func(weakHP float32) (strongHP float32) {
		a, c := world()
		weak := a.Spawn(1, Coord{3, 6}, 10, 1000)
		a.Unit(weak).HP = weakHP
		strong := a.Spawn(1, Coord{4, 6}, 10, 1000)
		h := baseHex(t)
		h.Slots[1] = Slot{Kind: SlotPart, Part: NewTarget(TargetNearest, 0, 2)}
		h.Frames[2] = NewFrame(BendKillLock)
		set(t, h, 2, NewDamage(id("cryo"), 30))
		p := compile(t, h, 0)
		if len(steps(p, OpIfKill)) != 1 {
			t.Fatalf("no kill check\n%s", p)
		}
		NewEvaluator(reg, DefaultRules()).Run(p, a, Ctx{Caster: c, Subject: NoEntity, Level: 10})
		return a.Unit(strong).HP
	}
	kill, none := 1000-run(5), 1000-run(1000)
	approx(t, "no kill", none, 30)
	// The second hit lands a little softer (resist builds), but it lands.
	if kill < 1.8*none {
		t.Fatalf("after a kill dealt %.1f, want about twice %.1f", kill, none)
	}
}

func TestPrismTakesTheTypeBeforeIt(t *testing.T) {
	h := baseHex(t)
	set(t, h, 2, NewDamage(id("cryo"), 10))
	h.Frames[3] = NewFrame(BendPrism)
	set(t, h, 3, NewDamage(id("plasma"), 10))
	p := compile(t, h, 0)
	d := steps(p, OpDamage)
	if d[1].Type != id("cryo") {
		t.Fatalf("prism type %d, want cryo\n%s", d[1].Type, p)
	}
	// With nothing typed before it, it keeps its own type.
	h2 := framed(t, NewFrame(BendPrism), NewDamage(id("plasma"), 10))
	if d := steps(compile(t, h2, 0), OpDamage); d[0].Type != id("plasma") {
		t.Fatal("a lone prism changed type")
	}
}

func TestLeechPaysPowerBack(t *testing.T) {
	a, c := world()
	foe := a.Spawn(1, Coord{4, 6}, 10, 1000)
	p := compile(t, framed(t, NewFrame(BendLeech), NewDamage(id("cryo"), 100)), 0)
	if d := steps(p, OpDamage); d[0].Flag&FlagLeech == 0 {
		t.Fatal("no leech flag")
	}
	rig := Rig{MaxPower: 100, Power: 10, MaxHeat: 100}
	res := NewEvaluator(reg, DefaultRules()).Run(p, a, Ctx{Caster: c, Subject: NoEntity, Level: 10, Rig: &rig})
	dealt := 1000 - a.Unit(foe).HP
	if dealt <= 0 || res.Damage <= 0 {
		t.Fatal("no damage")
	}
	approx(t, "power back", rig.Power, 10+dealt*LeechFrac)
	plain := steps(plainOf(t, NewDamage(id("cryo"), 100)), OpDamage)[0].Value
	approx(t, "leech cut", steps(p, OpDamage)[0].Value, plain*(1-LeechCut))
}

func TestSiphonTradesPowerForHeat(t *testing.T) {
	dmg := NewDamage(id("cryo"), 40)
	plain := plainOf(t, dmg)
	bp := compile(t, baseHex(t), 0)
	p := compile(t, framed(t, NewFrame(BendSiphon), dmg), 0)
	approx(t, "power", p.Power, bp.Power)
	approx(t, "heat", p.Heat, plain.Heat+SiphonHeat)
	f := NewFrame(BendSiphon)
	f.Grade = Exotic
	q := compile(t, framed(t, f, dmg), 0)
	if q.Heat >= p.Heat {
		t.Fatal("an exotic siphon should run cooler")
	}
}

func TestHairTriggerFiresFasterAndHotter(t *testing.T) {
	mk := func(f Frame) *Program {
		h := NewHex("x", [6]SocketKind{T, G, E, E, W, G})
		h.Frames[0] = f
		set(t, h, 0, NewPeriodic(4))
		set(t, h, 1, NewTarget(TargetNearest, 0, 1))
		set(t, h, 2, NewDamage(id("plasma"), 20))
		return compile(t, h, 0)
	}
	plain, hair := mk(Frame{}), mk(NewFrame(BendHair))
	approx(t, "period", hair.Period, plain.Period/HairHaste)
	approx(t, "heat", hair.Heat, plain.Heat*HairHeat)
	approx(t, "haste", hair.Haste, 1/HairHaste)
	h := NewHex("x", [6]SocketKind{T, G, E, E, W, G})
	h.Frames[0] = NewFrame(BendDoubler)
	if err := h.Set(0, NewTrigger(OnCast)); err == nil {
		t.Fatal("a doubler took a trigger")
	}
}

func TestOverflowGrowsWithHeatAndLocksLonger(t *testing.T) {
	h := baseHex(t)
	set(t, h, 2, NewDamage(id("cryo"), 50))
	h.Frames[3] = NewFrame(BendOverflow)
	set(t, h, 3, NewMult(VerbDamage, 2))
	p := compile(t, h, 0)
	if p.LockExtra != OverflowLock || steps(p, OpDamage)[0].Flag&FlagOverflow == 0 {
		t.Fatalf("overflow not set\n%s", p)
	}
	hit := func(heat float32) float32 {
		a, c := world()
		foe := a.Spawn(1, Coord{4, 6}, 10, 10000)
		rig := Rig{MaxPower: 100, Power: 100, MaxHeat: 100, Heat: heat}
		NewEvaluator(reg, DefaultRules()).Run(p, a, Ctx{Caster: c, Subject: NoEntity, Level: 30, Rig: &rig})
		return 10000 - a.Unit(foe).HP
	}
	cool, hot := hit(50), hit(100)
	approx(t, "hot/cool", hot/cool, 1+OverflowPer*20)
	rig := Rig{MaxPower: 100, Power: 100, MaxHeat: 100, Heat: 99}
	pp := *p
	pp.Heat = 5
	rig.TryFire(&pp)
	approx(t, "lock", rig.ShutdownLeft, OverheatShutdown+OverflowLock)
}

func TestCrowdPicksMoreAndSkipsElites(t *testing.T) {
	a, c := world()
	for _, at := range []Coord{{3, 6}, {4, 6}, {4, 5}, {5, 6}} {
		a.Spawn(1, at, 10, 1000)
	}
	elite := a.Spawn(1, Coord{3, 5}, 12, 1000)
	h := baseHex(t)
	h.Frames[1] = NewFrame(BendCrowd)
	h.Slots[1] = Slot{Kind: SlotPart, Part: NewTarget(TargetNearest, 0, 1)}
	set(t, h, 2, NewDamage(id("cryo"), 10))
	p := compile(t, h, 0)
	res := NewEvaluator(reg, DefaultRules()).Run(p, a, Ctx{Caster: c, Subject: NoEntity, Level: 10})
	if res.Hits != 4 {
		t.Fatalf("hits %d, want 4 (1 + foes nearby, no elite)\n%s", res.Hits, p)
	}
	approx(t, "elite", a.Unit(elite).HP, 1000)
	if ok, _ := h.Fits(1, NewTarget(TargetAdjacent, 2, 0), reg); ok {
		t.Fatal("crowd took an around target")
	}
}

func TestLonelyHitsHardAloneSoftInACrowd(t *testing.T) {
	run := func(foes int) float32 {
		a, c := world()
		var first EntityID
		for i := 0; i < foes; i++ {
			id := a.Spawn(1, Coord{3 + i, 6}, 10, 10000)
			if i == 0 {
				first = id
			}
		}
		h := baseHex(t)
		h.Slots[1] = Slot{Kind: SlotPart, Part: NewTarget(TargetNearest, 0, 3)}
		h.Frames[2] = NewFrame(BendLonely)
		set(t, h, 2, NewDamage(id("cryo"), 20))
		NewEvaluator(reg, DefaultRules()).Run(compile(t, h, 0), a, Ctx{Caster: c, Subject: NoEntity, Level: 30})
		return 10000 - a.Unit(first).HP
	}
	one, three := run(1), run(3)
	approx(t, "one/three", one/three, LonelyHit/LonelyMiss)
}

func TestEchoFrameCopiesTheSlotBefore(t *testing.T) {
	h := baseHex(t)
	set(t, h, 2, NewDamage(id("plasma"), 20))
	plain := compile(t, h, 0)
	h.Frames[3] = NewFrame(BendEcho)
	if err := h.Set(3, NewMult(VerbDamage, 2)); err == nil {
		t.Fatal("an echo slot took a part")
	}
	p := compile(t, h, 0)
	d := steps(p, OpDamage)
	if len(d) != 2 || d[1].Value != d[0].Value {
		t.Fatalf("echo steps %+v\n%s", d, p)
	}
	bp := compile(t, baseHex(t), 0)
	approx(t, "power", p.Power-bp.Power, 2*(plain.Power-bp.Power))
	if h.EchoOf(3) != 2 {
		t.Fatalf("echo of %d", h.EchoOf(3))
	}
	if h.Slots[3].Kind != SlotEmpty {
		t.Fatal("compiling filled the real slot")
	}
}

func TestShapeTwinAndSurgeAreFrames(t *testing.T) {
	h := NewHex("x", [6]SocketKind{T, W, W, W, W, W})
	h.Shape = Shape{Kind: ShapeHub, Quirk: ShapeTwin, At: 2, ID: 9}
	if f := h.Frame(2); f.Bend != BendTwin || !f.Shaped {
		t.Fatalf("twin frame %+v", f)
	}
	h.Shape = Shape{Kind: ShapeFork, Quirk: ShapeSurge, At: 1, ID: 9}
	if f := h.Frame(1); f.Bend != BendSurge || !f.Shaped {
		t.Fatalf("surge frame %+v", f)
	}
	if !strings.Contains(h.Frame(1).Text(reg), "comes with the shape") {
		t.Fatal(h.Frame(1).Text(reg))
	}
}

func TestRollFrameIsValid(t *testing.T) {
	r := splitmix{7}
	seen := map[Bend]bool{}
	narrowed := 0
	for i := 0; i < 4000; i++ {
		f := RollFrame(r.next, Rarity(i%int(NumRarities)), reg.Len(), uint64(i))
		if !f.Valid() || f.Bend == BendNone || f.ID == 0 {
			t.Fatalf("bad frame %+v", f)
		}
		if f.Takes != bendTakes[f.Bend] {
			t.Fatalf("takes %v for %v", f.Takes, f.Bend)
		}
		if f.Typed || f.Ranked {
			narrowed++
		}
		if f.Bend == BendHair && !f.AcceptsTrigger() {
			t.Fatal("a narrowed hair trigger")
		}
		seen[f.Bend] = true
		if f.Text(reg) == "" || f.Tag() == "" {
			t.Fatalf("no text for %+v", f)
		}
	}
	if len(seen) != int(NumBends)-1 {
		t.Fatalf("bends rolled: %d", len(seen))
	}
	if narrowed < 600 || narrowed > 1600 {
		t.Fatalf("narrowed %d of 4000", narrowed)
	}
}

func TestPlainHexesSaveWithoutFrames(t *testing.T) {
	h := baseHex(t)
	b, err := json.Marshal(h)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "Frames") {
		t.Fatal("a plain hex saved frames")
	}
	h.Frames[2] = NewFrame(BendLeech)
	b, _ = json.Marshal(h)
	var back Hex
	if err := json.Unmarshal(b, &back); err != nil || back.Frames[2] != h.Frames[2] {
		t.Fatalf("round trip: %v %+v", err, back.Frames[2])
	}
}

func TestFramedRunsAllocateNothing(t *testing.T) {
	a, c := world()
	for _, at := range []Coord{{3, 6}, {4, 6}, {4, 5}, {5, 6}} {
		a.Spawn(1, at, 10, 1e9)
	}
	h := NewHex("x", [6]SocketKind{T, G, E, E, W, W})
	set(t, h, 0, NewTrigger(OnCast))
	h.Frames[1] = NewFrame(BendCrowd)
	set(t, h, 1, NewTarget(TargetNearest, 0, 1))
	h.Frames[2] = NewFrame(BendKillLock)
	set(t, h, 2, NewDamage(id("cryo"), 5))
	h.Frames[3] = NewFrame(BendOverflow)
	set(t, h, 3, NewMult(VerbDamage, 1.5))
	h.Frames[4] = NewFrame(BendLeech)
	set(t, h, 4, NewDamage(id("plasma"), 5))
	h.Frames[5] = NewFrame(BendLonely)
	set(t, h, 5, NewStatus(id("cryo"), 1))
	p := compile(t, h, 0)
	ev := NewEvaluator(reg, DefaultRules())
	rig := Rig{MaxPower: 100, Power: 50, MaxHeat: 100, Heat: 95}
	allocs := testing.AllocsPerRun(200, func() {
		ev.Run(p, a, Ctx{Caster: c, Subject: NoEntity, Level: 10, Rig: &rig})
	})
	if allocs != 0 {
		t.Fatalf("%.1f allocs per run", allocs)
	}
}

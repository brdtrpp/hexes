package game

import (
	"math"
	"testing"

	"hexes/loot"
	"hexes/skill"
)

// A fresh save is the bottom of the ladder: one Triangle with a cast
// trigger, plasma and an empty slot, three hexes offline, the casting
// card up, and no bench where you arrive.
func TestFreshSaveStartsWithOneHex(t *testing.T) {
	s := New(11)
	if s.Taught != 1 || s.HexesOpen() != 1 || s.Unlocks() != 0 {
		t.Fatalf("taught %d, %d hexes, unlocks %b", s.Taught, s.HexesOpen(), s.Unlocks())
	}
	h := s.Load.Hexes[0]
	if h.Shape.Kind != skill.ShapeTriangle || h.Shape.Len() != 3 {
		t.Fatalf("starter shape %v", h.Shape)
	}
	order, n := h.Flow(h.TriggerSlot())
	if n != 3 || h.Slots[order[0]].Part.Trigger != skill.OnCast || h.Slots[order[1]].Part.Kind != skill.KindEffect ||
		h.Slots[order[2]].Kind != skill.SlotEmpty {
		t.Fatalf("starter hex slots %+v", h.Slots)
	}
	if s.Load.Keys[0] != 1 || !s.Load.Castable(0) {
		t.Fatal("starter hex isn't on button 1")
	}
	for i := 1; i < MaxSlots; i++ {
		if !s.Load.Progs[i].Inert || s.Load.Names[i] != OfflineName || s.Load.Castable(i) {
			t.Fatalf("hex %d is online before the ladder gives it", i+1)
		}
	}
	if s.CardT <= 0 || s.Card != 0 {
		t.Fatal("no casting card on a fresh save")
	}
	if s.BenchOn(0) || !s.BenchOn(1) {
		t.Fatal("benches before the workbench lesson")
	}
	d := s.Stash.Lesson
	if !d.On || d.Idx != 1 || skill.Dist(d.At, s.Benches[1]) != 1 {
		t.Fatalf("first lesson %+v, stairs bench at %v", d, s.Benches[1])
	}
}

// takeLesson walks the player onto the lesson pickup.
func takeLesson(t *testing.T, s *Sim) int {
	t.Helper()
	d := s.Stash.Lesson
	if !d.On {
		t.Fatalf("depth %d has no lesson", s.Depth)
	}
	p := s.P()
	s.occ[s.Floor.Index(p.At)] = skill.NoEntity
	p.placeFree(d.At)
	s.pickup(0)
	if s.Stash.Lesson.On {
		t.Fatalf("lesson %s not taken on depth %d", Ladder[d.Idx].Name, s.Depth)
	}
	return d.Idx
}

// Loot only rolls what's been taught.
func checkLoot(t *testing.T, s *Sim) {
	t.Helper()
	tb := loot.NewTable(s.Reg)
	tb.Allow = s.lootAllow()
	u := s.Unlocks()
	for i := 0; i < 600; i++ {
		r := loot.NewRNG(uint64(i)*7919 + uint64(s.Depth))
		p := tb.Roll(&r, s.Depth)
		bad := ""
		switch {
		case p.Kind == skill.KindTarget && u&UTargets == 0:
			bad = "target"
		case p.Kind == skill.KindTrigger && u&UTriggers == 0:
			bad = "trigger"
		case p.Kind == skill.KindMult && u&UMults == 0:
			bad = "multiplier"
		case IsSustain(p) && u&USustain == 0:
			bad = "sustain"
		case p.Kind == skill.KindGate && u&UGates == 0:
			bad = "gate"
		case p.Kind == skill.KindGate && (p.Gate == skill.GateSpend || p.Gate == skill.GateEmpty) && u&UCharges == 0:
			bad = "spend gate"
		case p.Kind == skill.KindCharge && u&UCharges == 0:
			bad = "charge"
		case p.Kind == skill.KindCall && u&UCalls == 0:
			bad = "call"
		case (p.Quirk != skill.QuirkNone || p.Rarity == skill.Exotic) && u&UQuirks == 0:
			bad = "quirk or exotic"
		case p.Kind == skill.KindTrigger && (p.Trigger == skill.Channel || p.Trigger == skill.Watch) && u&UQuirks == 0:
			bad = "channel or watch"
		}
		if bad != "" {
			t.Fatalf("depth %d (taught %d) rolled a %s: %+v", s.Depth, s.Taught, bad, p)
		}
	}
}

// A headless fresh run down the floors sees each lesson arrive on its
// own floor, unlocks it for good, and never rolls what it hasn't taught.
func TestLadderArrivesFloorByFloor(t *testing.T) {
	s := New(5)
	seen := 1
	for depth := 1; depth <= 16; depth++ {
		if depth > 1 {
			s.load(depth, 1)
		}
		checkLoot(t, s)
		for s.Stash.Lesson.On {
			idx := s.Stash.Lesson.Idx
			if want := Ladder[idx].Depth; want != depth {
				t.Fatalf("lesson %s on depth %d, want %d", Ladder[idx].Name, depth, want)
			}
			if idx != seen {
				t.Fatalf("lesson %d out of order (want %d)", idx, seen)
			}
			before := s.Stash.Inv.Count()
			takeLesson(t, s)
			seen++
			l := Ladder[idx]
			if s.Taught != idx+1 || !s.Has(l.Gives) || s.HexesOpen() != l.Hexes {
				t.Fatalf("after %s: taught %d, unlocks %b, %d hexes", l.Name, s.Taught, s.Unlocks(), s.HexesOpen())
			}
			if s.Card != idx || s.CardT <= 0 {
				t.Fatalf("no card for %s", l.Name)
			}
			_, np := s.lessonParts(idx)
			if s.Stash.Inv.Count() != before+np {
				t.Fatalf("%s put %d parts in the pack, want %d", l.Name, s.Stash.Inv.Count()-before, np)
			}
			for i := 0; i < s.HexesOpen(); i++ {
				if s.Load.Names[i] == OfflineName {
					t.Fatalf("hex %d still offline after %s", i+1, l.Name)
				}
			}
			for i := s.HexesOpen(); i < MaxSlots; i++ {
				if !s.Load.Progs[i].Inert {
					t.Fatalf("hex %d fires before it's online", i+1)
				}
			}
			checkLoot(t, s)
			s.load(depth, 1) // the floor again: the lesson doesn't come back, the next one may
			if s.Stash.Lesson.On && s.Stash.Lesson.Idx == idx {
				t.Fatalf("%s placed twice", l.Name)
			}
		}
	}
	if !s.Learned() || s.Unlocks() != UAll || seen != len(Ladder) {
		t.Fatalf("ladder not done: taught %d of %d, unlocks %b", s.Taught, len(Ladder), s.Unlocks())
	}
	if s.Stash.Lesson.On {
		t.Fatal("a lesson after the ladder is done")
	}
	if len(s.Frames) != 1 || s.Frames[0].Bend != skill.BendDoubler {
		t.Fatalf("frames lesson racked %+v", s.Frames)
	}
	if len(s.Shapes) != 1 || s.Shapes[0].Kind != skill.ShapeFork {
		t.Fatalf("shapes lesson learned %+v", s.Shapes)
	}
	for i := 0; i < 2; i++ {
		if s.Load.Hexes[i].Shape.Kind != skill.ShapeRing {
			t.Fatalf("hex %d didn't grow to a Ring", i+1)
		}
	}
}

// A lesson you walk past waits on the next floor instead of being
// skipped, and only one shows per floor.
func TestSkippedLessonWaits(t *testing.T) {
	s := New(8)
	s.load(2, 1)
	if d := s.Stash.Lesson; !d.On || d.Idx != 1 {
		t.Fatalf("depth 2 should still hold the workbench lesson, has %+v", d)
	}
	takeLesson(t, s)
	s.load(3, 1)
	if d := s.Stash.Lesson; !d.On || d.Idx != 2 {
		t.Fatalf("depth 3 should hold targets, has %+v", d)
	}
}

// Dying drops you back floors but keeps what you've been taught, and a
// save keeps it too.
func TestUnlocksSurviveDeathAndSaves(t *testing.T) {
	s := New(9)
	for d := 1; d <= 4; d++ {
		if d > 1 {
			s.load(d, 1)
		}
		for s.Stash.Lesson.On {
			takeLesson(t, s)
		}
	}
	taught := s.Taught
	s.P().U.HP = 0
	s.P().U.Alive = false
	s.Step(1.0 / 60)
	if s.State != Dead {
		t.Fatal("didn't die")
	}
	s.respawn()
	if s.Taught != taught || s.Depth >= 4 {
		t.Fatalf("death took unlocks: taught %d (was %d) at depth %d", s.Taught, taught, s.Depth)
	}
	b, err := s.Save()
	if err != nil {
		t.Fatal(err)
	}
	r := New(1)
	if err := r.LoadSave(b); err != nil {
		t.Fatal(err)
	}
	if r.Taught != taught || r.HexesOpen() != s.HexesOpen() || r.CardT != 0 {
		t.Fatalf("save lost the ladder: taught %d, %d hexes", r.Taught, r.HexesOpen())
	}
	if r.LoadSave([]byte(`{"Version":1,"Depth":3,"Hexes":[{},{},{},{}]}`)) == nil {
		t.Fatal("a save from before the ladder loaded")
	}
}

// A lesson with a trial pulls the nearest foes into one pack.
func TestTrialPackGathers(t *testing.T) {
	s := New(4)
	s.Taught = 2
	s.load(2, 1)
	var near []skill.Coord
	for i := range s.Actors {
		a := &s.Actors[i]
		if skill.EntityID(i) != s.Player && a.U.Alive {
			near = append(near, a.At)
		}
	}
	for _, a := range near {
		c := 0
		for _, b := range near {
			if a != b && skill.Dist(a, b) == 1 {
				c++
			}
		}
		if c >= TrialPack-1 {
			return
		}
	}
	t.Fatalf("no pack of %d around one foe", TrialPack)
}

// TEST unlocks everything at once, and turning it off puts the ladder
// back where it was.
func TestTestSwitchUnlocksTheLadder(t *testing.T) {
	s := New(3)
	if err := s.SetTestShapes(true); err != nil {
		t.Fatal(err)
	}
	if s.Unlocks() != UAll || s.HexesOpen() != MaxSlots || !s.BenchOn(0) || s.lootAllow() != (loot.Allow{}) {
		t.Fatal("TEST didn't unlock everything")
	}
	if err := s.SetTestShapes(false); err != nil {
		t.Fatal(err)
	}
	if s.Unlocks() != 0 || s.HexesOpen() != 1 {
		t.Fatal("TEST off didn't put the ladder back")
	}
}

// Hex 2 arrives with an on-hit trigger and fires on its own.
func TestSecondHexFiresOnHit(t *testing.T) {
	s := New(12)
	for d := 1; d <= 3; d++ {
		if d > 1 {
			s.load(d, 1)
		}
		for s.Stash.Lesson.On {
			takeLesson(t, s)
		}
	}
	if s.HexesOpen() != 2 || s.Load.Progs[1].Trigger != skill.OnHit || s.Load.Progs[1].Inert {
		t.Fatalf("hex 2 is %q, %v", s.Load.Names[1], s.Load.Progs[1].Trigger)
	}
	p := s.P()
	p.U.MaxHP, p.U.HP = 1e9, 1e9
	for k := 0; k < 60*40 && s.SlotFlash[1] == 0; k++ {
		foe, best := -1, math.MaxInt
		for i := range s.Actors {
			a := &s.Actors[i]
			if a.U.Alive && a.U.Team != p.U.Team && skill.Dist(a.At, p.At) < best {
				foe, best = i, skill.Dist(a.At, p.At)
			}
		}
		if foe < 0 {
			break
		}
		fx, fy := s.Actors[foe].Pos()
		s.Input = Input{MoveX: fx - p.X, MoveY: fy - p.Y, Cast: [MaxSlots]bool{k%20 == 0}}
		s.Step(1.0 / 60)
	}
	if s.SlotFlash[1] == 0 {
		t.Fatal("hex 2 never fired")
	}
}

// A fresh run's game step, lesson on the floor and card up, allocates
// nothing.
func TestLadderStepAllocFree(t *testing.T) {
	s := New(3)
	p := s.P()
	p.U.MaxHP, p.U.HP = 1e9, 1e9
	k := 0
	allocs := testing.AllocsPerRun(600, func() {
		k++
		a := float64(k/30) * 1.3
		s.Input = Input{MoveX: math.Cos(a), MoveY: math.Sin(a), Cast: [MaxSlots]bool{k%45 == 0}}
		s.Step(1.0 / 60)
	})
	if allocs != 0 || s.State != Playing {
		t.Fatalf("%.2f allocs per step (state %d)", allocs, s.State)
	}
}

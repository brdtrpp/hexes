package game

import (
	"encoding/json"
	"strings"
	"testing"

	"hexes/loot"
	"hexes/skill"
)

func found(s *Sim, k skill.ShapeKind, q skill.ShapeQuirk, at uint8) skill.Shape {
	sh := skill.Shape{Kind: k, Quirk: q, At: at, ID: uint64(len(s.Shapes)*2 + 101)}
	s.Shapes = append(s.Shapes, sh)
	return sh
}

func TestHubKeepsTheBuild(t *testing.T) {
	s := New(5)
	before := *s.Load.Hexes[0]
	_, ringHeat := s.HexPower(0)
	hub := found(s, skill.ShapeHub, skill.ShapeLean, 0)
	if err := s.SetShape(0, hub); err != nil {
		t.Fatal(err)
	}
	h := s.Load.Hexes[0]
	for i := 0; i < skill.RingSlots; i++ {
		if h.Slots[i] != before.Slots[i] {
			t.Fatalf("slot %d moved going to a Hub", i)
		}
	}
	if h.Shape != hub || s.Load.Progs[0].Shape != skill.ShapeHub || !s.Dirty {
		t.Fatal("hub not applied")
	}
	if _, ht := s.HexPower(0); ht >= ringHeat*1.2 || ht <= 0 {
		t.Fatalf("lean hub heat %.1f vs ring %.1f", ht, ringHeat)
	}
	if s.Disp.Loadout(s.Player).Progs[0] != s.Load.Progs[0] {
		t.Fatal("reshaped hex not equipped")
	}
	// The center takes an effect, not a target.
	cell := s.Stash.Inv.Add(skill.NewTarget(skill.TargetAdjacent, 1, 0))
	if err := s.SwapIn(0, skill.Center, cell); err == nil || !strings.Contains(err.Error(), "center") {
		t.Fatalf("target in the center: %v", err)
	}
	cell = s.Stash.Inv.Add(skill.NewDamage(s.Reg.MustID("plasma"), 10))
	if err := s.SwapIn(0, skill.Center, cell); err != nil {
		t.Fatal(err)
	}
}

func TestTriangleSendsTheRestToThePack(t *testing.T) {
	s := New(5)
	// Arc Lance: trigger, chain, damage, multiplier, empty, call.
	if err := s.SetShape(0, skill.Shape{Kind: skill.ShapeTriangle}); err != nil {
		t.Fatal(err)
	}
	h := s.Load.Hexes[0]
	if h.Slots[3].Kind != skill.SlotEmpty || h.Slots[5].Kind != skill.SlotEmpty {
		t.Fatal("parts left outside the triangle")
	}
	if s.Stash.Inv.Count() != 2 || s.Unsecured() != 0 {
		t.Fatalf("pack %d unsecured %d, want the multiplier and the call, safe", s.Stash.Inv.Count(), s.Unsecured())
	}
	// Back to a Ring: the slots are empty, the parts stay in the pack.
	if err := s.SetShape(0, skill.Shape{}); err != nil {
		t.Fatal(err)
	}
	// With a full pack a triangle is refused and nothing changes.
	s2 := New(5)
	for s2.Stash.Inv.Count() < loot.InventorySize {
		s2.Stash.Inv.Add(skill.NewPush(1))
	}
	was := *s2.Load.Hexes[0]
	if err := s2.SetShape(0, skill.Shape{Kind: skill.ShapeTriangle}); err == nil {
		t.Fatal("triangle with a full pack")
	}
	if *s2.Load.Hexes[0] != was {
		t.Fatal("refused shape changed the hex")
	}
}

func TestForkMovesTheTriggerToTheCenter(t *testing.T) {
	s := New(5)
	fork := found(s, skill.ShapeFork, skill.ShapeTwin, 1)
	if err := s.SetShape(1, fork); err != nil {
		t.Fatal(err)
	}
	h := s.Load.Hexes[1]
	if h.Slots[skill.Center].Part.Kind != skill.KindTrigger || h.Slots[0].Kind != skill.SlotEmpty {
		t.Fatalf("fork trigger not in the center: %+v", h.Slots)
	}
	if h.TriggerSlot() != skill.Center {
		t.Fatal("trigger slot")
	}
	// Taking the trigger out is refused with the fork's wording.
	if err := s.SwapIn(1, skill.Center, 11); err != ErrNeedsTrigger {
		t.Fatalf("pulled the trigger: %v", err)
	}
	cell := s.Stash.Inv.Add(skill.NewTrigger(skill.OnCast))
	if err := s.SwapIn(1, 0, cell); err == nil || !strings.Contains(err.Error(), "center") {
		t.Fatalf("trigger on a fork's ring: %v", err)
	}
	// Back to a Ring puts the trigger back on top.
	if err := s.SetShape(1, skill.Shape{}); err != nil {
		t.Fatal(err)
	}
	if s.Load.Hexes[1].Slots[0].Part.Kind != skill.KindTrigger {
		t.Fatal("trigger not back in the top slot")
	}
}

func TestCycleShape(t *testing.T) {
	s := New(5)
	loop := found(s, skill.ShapeLoop, skill.ShapeOneWay, 2)
	hub := found(s, skill.ShapeHub, skill.ShapeSurge, 3)
	// Hex 4 (Nanite Swarm) has 3 parts, so every shape fits.
	var got []skill.ShapeKind
	for i := 0; i < 5; i++ {
		sh, err := s.CycleShape(3)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, sh.Kind)
	}
	want := []skill.ShapeKind{skill.ShapeTriangle, skill.ShapeLoop, skill.ShapeHub, skill.ShapeRing, skill.ShapeTriangle}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("cycle %v, want %v", got, want)
		}
	}
	// A copy on one hex isn't offered to another.
	if err := s.SetShape(3, loop); err != nil {
		t.Fatal(err)
	}
	for _, sh := range s.ShapeChoices(2) {
		if sh == loop {
			t.Fatal("loop on hex 4 offered to hex 3")
		}
	}
	if len(s.ShapeChoices(2)) != 3 || s.ShapeChoices(2)[2] != hub {
		t.Fatalf("choices %+v", s.ShapeChoices(2))
	}
	// A shape that was never found can't be set.
	if err := s.SetShape(2, skill.Shape{Kind: skill.ShapeFork, ID: 99}); err == nil {
		t.Fatal("set an unfound shape")
	}
}

func TestShapesSurviveSaveAndDeath(t *testing.T) {
	s := New(8)
	loop := found(s, skill.ShapeLoop, skill.ShapeTwin, 3)
	found(s, skill.ShapeFork, skill.ShapeLean, 0)
	if err := s.SetShape(3, loop); err != nil {
		t.Fatal(err)
	}
	kill(s)
	b, err := s.Save()
	if err != nil {
		t.Fatal(err)
	}
	r := New(1)
	if err := r.LoadSave(b); err != nil {
		t.Fatal(err)
	}
	if len(r.Shapes) != 2 || r.Load.Hexes[3].Shape != loop || r.Load.Progs[3].Shape != skill.ShapeLoop || !r.ShapeGiven {
		t.Fatalf("shapes %+v hex %+v", r.Shapes, r.Load.Hexes[3].Shape)
	}
}

func TestOldSaveLoadsAsRings(t *testing.T) {
	s := New(4)
	b, _ := s.Save()
	// Strip everything shapes added: the save fields, each hex's shape
	// and its seventh slot, as a save from before shapes looked.
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	delete(m, "Shapes")
	delete(m, "ShapeGiven")
	for _, h := range m["Hexes"].([]any) {
		hx := h.(map[string]any)
		delete(hx, "Shape")
		for _, k := range []string{"Sockets", "Linked", "Slots"} {
			hx[k] = hx[k].([]any)[:6]
		}
	}
	old, _ := json.Marshal(m)
	if strings.Contains(string(old), "Shape") {
		t.Fatal("test didn't strip shapes")
	}
	r := New(1)
	if err := r.LoadSave(old); err != nil {
		t.Fatal(err)
	}
	for i, h := range r.Load.Hexes {
		if h.Shape.Kind != skill.ShapeRing || r.Load.Progs[i] == nil {
			t.Fatalf("hex %d shape %v", i, h.Shape)
		}
	}
	// And the old hexes can be reshaped.
	if _, err := r.CycleShape(3); err != nil {
		t.Fatal(err)
	}
}

func TestFirstShapeWaitsAtDepthFive(t *testing.T) {
	s := New(6)
	if len(s.Stash.Shapes) != 0 || ShapeDropChance(1) != 0 {
		t.Fatal("shapes on depth 1")
	}
	s.load(ShapeMinDepth, 1)
	if len(s.Stash.Shapes) != 1 {
		t.Fatalf("depth %d has %d shapes lying about, want the first one", s.Depth, len(s.Stash.Shapes))
	}
	d := s.Stash.Shapes[0]
	if !d.Shape.Valid() || d.Shape.Quirk == skill.ShapeQuirkNone || d.Shape.Kind == skill.ShapeRing || d.Shape.Kind == skill.ShapeTriangle {
		t.Fatalf("first shape %+v", d.Shape)
	}
	if !s.Floor.At(d.At).Walkable() {
		t.Fatal("first shape on an unwalkable tile")
	}
	s.Dirty = false
	s.pickupShapes(d.At, 0)
	if len(s.Shapes) != 1 || s.Shapes[0] != d.Shape || !s.ShapeGiven || !s.Dirty || len(s.Stash.Shapes) != 0 {
		t.Fatal("first shape not learned")
	}
	shown := false
	s.FX.Each(func(f *FX) { shown = shown || f.Kind == FXShape })
	if !shown {
		t.Fatal("no fx for a learned shape")
	}
	s.load(ShapeMinDepth+1, 1)
	if len(s.Stash.Shapes) != 0 {
		t.Fatal("first shape placed twice")
	}
}

func TestShapeDropsAreRareAndDeterministic(t *testing.T) {
	if c := ShapeDropChance(ShapeMinDepth); c <= 0 || c > 0.01 {
		t.Fatalf("chance at depth %d is %.3f", ShapeMinDepth, c)
	}
	if ShapeDropChance(500) != ShapeDropMax {
		t.Fatal("no cap on the drop chance")
	}
	roll := func() []skill.Shape {
		s := New(9)
		s.ShapeGiven = true
		s.load(40, 1)
		var out []skill.Shape
		for id := 1; id < 400; id++ {
			s.rollShapeDrop(skill.EntityID(id), s.Floor.Start, false)
			if len(s.Stash.Shapes) == MaxShapeDrops {
				break
			}
		}
		for _, d := range s.Stash.Shapes {
			out = append(out, d.Shape)
		}
		return out
	}
	a, b := roll(), roll()
	if len(a) == 0 || len(a) != len(b) {
		t.Fatalf("drops %d then %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] || !a[i].Valid() {
			t.Fatalf("drop %d: %+v vs %+v", i, a[i], b[i])
		}
	}
}

// A Loop hex fired through the dispatcher: the back edge runs through the
// call queue, so its passes pay Heat on the real rig.
func TestLoopHexFiresInTheGame(t *testing.T) {
	s := New(5)
	loop := found(s, skill.ShapeLoop, skill.ShapeLean, 0)
	// Arc Lance's slot 5 call carries its subject back round.
	if err := s.SetShape(0, loop); err != nil {
		t.Fatal(err)
	}
	if !s.Load.Progs[0].Loops {
		t.Fatalf("lance as a loop has no back edge:\n%s", s.Load.Progs[0])
	}
	s.Rig.Heat = 0
	if o := s.Disp.Cast(s.Player, 0, skill.NoEntity); o != skill.Fired {
		t.Fatalf("cast %s", o)
	}
	lance, swarm := s.Load.Progs[0], s.Load.Progs[StarterCallTarget-1]
	once := lance.Heat + max(swarm.Heat, skill.CallMinHeat)
	if s.Rig.Heat < once+lance.Heat {
		t.Fatalf("heat %.1f: loop passes should add to one lance and swarm (%.1f)", s.Rig.Heat, once)
	}
}

func TestTestSwitchUnlocksEveryShape(t *testing.T) {
	s := New(5)
	if len(s.ShapeChoices(0)) != 2 {
		t.Fatal("a new save should only have Ring and Triangle")
	}
	for _, sh := range TestShapeSet {
		if !sh.Valid() || !IsTestShape(sh) {
			t.Fatalf("test shape %v broken", sh)
		}
		if err := s.SetShape(1, sh); err == nil {
			t.Fatalf("%v usable with the switch off", sh)
		}
	}
	if err := s.SetTestShapes(true); err != nil || !s.Dirty {
		t.Fatal(err)
	}
	ch := s.ShapeChoices(0)
	kinds := map[skill.ShapeKind]bool{}
	quirks := map[skill.ShapeQuirk]bool{}
	for _, sh := range ch {
		kinds[sh.Kind], quirks[sh.Quirk] = true, true
	}
	if len(kinds) != int(skill.NumShapes) || len(quirks) != int(skill.NumShapeQuirks) {
		t.Fatalf("switch on: %d kinds, %d quirks", len(kinds), len(quirks))
	}
	// Cycling reaches every shape, and a copy on one hex isn't offered to
	// another.
	seen := map[skill.ShapeKind]bool{}
	for i := 0; i < len(ch); i++ {
		sh, err := s.CycleShape(2)
		if err != nil {
			t.Fatal(err)
		}
		seen[sh.Kind] = true
		if sh.Kind == skill.ShapeLoop {
			break
		}
	}
	if !seen[skill.ShapeHub] || !seen[skill.ShapeFork] || !seen[skill.ShapeLoop] {
		t.Fatalf("cycling reached %v", seen)
	}
	if err := s.SetShape(0, TestShapeSet[0]); err != nil {
		t.Fatal(err)
	}
	for _, sh := range s.ShapeChoices(1) {
		if sh == TestShapeSet[0] {
			t.Fatal("hub on hex 1 offered to hex 2")
		}
	}
	// The switch survives a save and the copies never count as found.
	b, err := s.Save()
	if err != nil {
		t.Fatal(err)
	}
	r := New(9)
	if err := r.LoadSave(b); err != nil {
		t.Fatal(err)
	}
	if !r.TestShapes || len(r.Shapes) != 0 || r.Load.Hexes[0].Shape != TestShapeSet[0] {
		t.Fatalf("after load: switch %v, found %d, hex 1 %v", r.TestShapes, len(r.Shapes), r.Load.Hexes[0].Shape)
	}
	// Off puts test-shaped hexes back to Rings and keeps their parts.
	parts := func(s *Sim) int {
		n := s.Stash.Inv.Count()
		for _, h := range s.Load.Hexes {
			for _, sl := range h.Slots {
				if sl.Kind == skill.SlotPart {
					n++
				}
			}
		}
		return n
	}
	before := parts(r)
	if err := r.SetTestShapes(false); err != nil {
		t.Fatal(err)
	}
	for i, h := range r.Load.Hexes {
		if IsTestShape(h.Shape) {
			t.Fatalf("hex %d kept a test shape", i+1)
		}
	}
	if parts(r) != before || len(r.ShapeChoices(0)) != 2 {
		t.Fatal("switching off lost parts or left shapes unlocked")
	}
}

func TestPartsOnlyGoWhereTheyDoSomething(t *testing.T) {
	s := New(5)
	// Every preset part does something where it sits.
	for i, h := range s.Load.Hexes {
		for j := range h.Slots {
			if h.Slots[j].Kind != skill.SlotPart {
				continue
			}
			if what, ok := h.Use(j); !ok {
				t.Errorf("%s slot %d: %s", s.Load.Names[i], j+1, what)
			}
		}
	}
	// Arc Lance: trigger, chain, damage, x2 damage, empty, call. A damage
	// booster lights after the damage or the open slot 5, not after the
	// trigger or the call.
	s.Stash.Inv = loot.Inventory{}
	cell := s.Stash.Inv.Add(skill.NewMult(skill.VerbDamage, 1.5))
	lit := map[int]bool{}
	for slot := 0; slot < skill.RingSlots; slot++ {
		_, ok := s.PreviewIn(0, slot, cell)
		lit[slot] = ok
	}
	if lit[0] || !lit[3] || !lit[4] {
		t.Fatalf("damage booster lit %v", lit)
	}
	if err := s.SwapIn(0, 1, cell); err == nil || !strings.Contains(err.Error(), "do nothing") {
		t.Fatalf("booster over the chain target: %v", err)
	}
	// Moving the x2 booster next to the trigger is refused; the new one
	// goes in slot 5, stacking on the x2.
	if err := s.SwapSlots(0, 3, 1); err == nil {
		t.Fatal("booster moved onto the chain target's slot")
	}
	if err := s.SwapIn(0, 4, cell); err != nil {
		t.Fatal(err)
	}
}

// Drops are generated now: deep ones are bigger, and every drop goes on a
// hex with its parts kept where the slots line up.
func TestDropsAreGenerated(t *testing.T) {
	big := 0
	for _, depth := range []int{ShapeMinDepth, 45} {
		s := New(11)
		s.ShapeGiven = true
		s.load(depth, 1)
		for id := 1; id < 4000 && len(s.Stash.Shapes) < MaxShapeDrops; id++ {
			s.rollShapeDrop(skill.EntityID(id), s.Floor.Start, true)
		}
		if len(s.Stash.Shapes) == 0 {
			t.Fatalf("no drops at depth %d", depth)
		}
		for _, d := range s.Stash.Shapes {
			sh := d.Shape
			if sh.Kind != skill.ShapeGen || !sh.Valid() || sh.Len() > skill.GenMaxSlots(depth) {
				t.Fatalf("depth %d drop %+v", depth, sh)
			}
			if depth > ShapeMinDepth && sh.Len() > big {
				big = sh.Len()
			}
			s.Shapes = append(s.Shapes, sh)
			if err := s.SetShape(1, sh); err != nil {
				t.Fatalf("%v: %v", sh, err)
			}
			if p := s.Load.Progs[1]; p == nil || p.Shape != skill.ShapeGen {
				t.Fatalf("%v not compiled", sh)
			}
		}
	}
	if big < 7 {
		t.Fatalf("biggest deep drop has %d slots", big)
	}
}

func TestTestSwitchCoversGeneratedFamilies(t *testing.T) {
	fam := map[skill.ShapeKind]bool{}
	most := 0
	for _, sh := range TestShapeSet {
		if sh.Kind != skill.ShapeGen {
			continue
		}
		fam[sh.Family()] = true
		most = max(most, sh.Len())
	}
	if len(fam) != int(skill.ShapeGen) || most != skill.Slots {
		t.Fatalf("test set families %v, biggest %d", fam, most)
	}
	s := New(5)
	if err := s.SetTestShapes(true); err != nil {
		t.Fatal(err)
	}
	for _, sh := range TestShapeSet {
		if err := s.SetShape(0, sh); err != nil {
			t.Fatalf("%v: %v", sh, err)
		}
		if h := s.Load.Hexes[0]; h.Slots[sh.Entry()].Part.Kind != skill.KindTrigger {
			t.Fatalf("%v: trigger not at slot %d", sh, sh.Entry()+1)
		}
	}
}

// A save holding found shapes of the old fixed kinds and generated ones
// loads with every shape intact.
func TestOldAndGeneratedShapesSave(t *testing.T) {
	s := New(5)
	old := found(s, skill.ShapeFork, skill.ShapeSurge, 1)
	r := loot.NewRNG(3)
	gen := skill.RollShape(r.Next, 555, 30)
	s.Shapes = append(s.Shapes, gen)
	if err := s.SetShape(0, old); err != nil {
		t.Fatal(err)
	}
	if err := s.SetShape(1, gen); err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(s.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	var sv Save
	if err := json.Unmarshal(b, &sv); err != nil {
		t.Fatal(err)
	}
	back := New(5)
	if err := back.Restore(sv); err != nil {
		t.Fatal(err)
	}
	if len(back.Shapes) != 2 || back.Shapes[0] != old || back.Shapes[1] != gen {
		t.Fatalf("shapes after load %+v", back.Shapes)
	}
	if back.Load.Hexes[0].Shape != old || back.Load.Hexes[1].Shape != gen {
		t.Fatal("hex shapes lost")
	}
}

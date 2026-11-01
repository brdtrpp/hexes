package game

import (
	"testing"

	"hexes/loot"
	"hexes/skill"
)

// killAll kills every enemy in the given order through the sim's own
// death path, so drops roll the way they do in play.
func killAll(s *Sim, order []int) {
	for _, i := range order {
		a := &s.Actors[i]
		if skill.EntityID(i) == s.Player || !a.U.Alive {
			continue
		}
		a.U.HP, a.U.Alive = 0, false
		s.died(skill.EntityID(i))
	}
}

func enemyOrder(s *Sim, reverse bool) []int {
	var out []int
	for i := range s.Actors {
		out = append(out, i)
	}
	if reverse {
		for l, r := 0, len(out)-1; l < r; l, r = l+1, r-1 {
			out[l], out[r] = out[r], out[l]
		}
	}
	return out
}

func TestDropsAreDeterministic(t *testing.T) {
	collect := func(reverse bool) map[skill.Coord][]skill.Part {
		s := New(42)
		s.load(8, 1)
		killAll(s, enemyOrder(s, reverse))
		m := map[skill.Coord][]skill.Part{}
		for _, d := range s.Stash.Drops {
			m[d.At] = append(m[d.At], d.Part)
		}
		return m
	}
	a, b := collect(false), collect(true)
	if len(a) == 0 {
		t.Fatal("a whole depth-8 floor dropped nothing")
	}
	if len(a) != len(b) {
		t.Fatalf("kill order changed the drops: %d vs %d tiles", len(a), len(b))
	}
	for at, ps := range a {
		count := map[skill.Part]int{}
		for _, p := range ps {
			count[p]++
		}
		for _, p := range b[at] {
			count[p]--
		}
		for p, n := range count {
			if n != 0 {
				t.Fatalf("tile %v: %v differs between kill orders", at, p)
			}
		}
	}
}

func TestDropsLandOnWalkableTiles(t *testing.T) {
	for seed := uint64(1); seed <= 20; seed++ {
		s := New(seed)
		s.load(1+int(seed%9), 1)
		killAll(s, enemyOrder(s, false))
		for _, d := range s.Stash.Drops {
			if !s.Floor.At(d.At).Walkable() {
				t.Fatalf("seed %d: drop on %v tile", seed, s.Floor.At(d.At))
			}
		}
	}
}

func TestElitesDropMore(t *testing.T) {
	var elite, eliteN, plain, plainN int
	for seed := uint64(1); seed <= 60; seed++ {
		s := New(seed)
		s.load(12, 1)
		for i := range s.Actors {
			if skill.EntityID(i) == s.Player {
				continue
			}
			before := len(s.Stash.Drops)
			killAll(s, []int{i})
			got := len(s.Stash.Drops) - before
			if s.Actors[i].U.Level > s.Depth {
				elite += got
				eliteN++
			} else if s.Actors[i].Kind == KindGrunt {
				plain += got
				plainN++
			}
		}
	}
	e, p := float64(elite)/float64(eliteN), float64(plain)/float64(plainN)
	t.Logf("elite drop rate %.2f (%d), grunt %.2f (%d)", e, eliteN, p, plainN)
	if eliteN < 20 || e < p+0.25 {
		t.Fatalf("elites should drop much more often: %.2f vs %.2f", e, p)
	}
}

// clearFloor kills every enemy without drops, so a test can place its own.
func clearFloor(s *Sim) {
	for i := range s.Actors {
		if skill.EntityID(i) != s.Player {
			s.Actors[i].U.Alive = false
			s.occ[s.Floor.Index(s.Actors[i].At)] = skill.NoEntity
		}
	}
	s.Stash.Drops = s.Stash.Drops[:0]
}

func TestPickupOnWalkOver(t *testing.T) {
	s := New(7)
	clearFloor(s)
	p := s.P()
	part := skill.NewMult(skill.VerbDamage, 1.5).WithRarity(skill.Rare)
	s.Stash.Drops = append(s.Stash.Drops, Drop{At: p.At, Part: part}, Drop{At: p.At.Add(skill.HexDirs[0]), Part: part})
	s.Step(1.0 / 60)
	if s.Stash.Inv.Count() != 1 || s.Stash.Inv.Parts[0] != part || len(s.Stash.Drops) != 1 {
		t.Fatalf("inventory %d, drops %d", s.Stash.Inv.Count(), len(s.Stash.Drops))
	}
	var loots int
	s.FX.Each(func(f *FX) {
		if f.Kind == FXLoot && f.Slot == int(skill.Rare) {
			loots++
		}
	})
	if loots != 1 {
		t.Fatalf("pickup fx %d", loots)
	}
}

func TestFullInventoryLeavesTheDrop(t *testing.T) {
	s := New(7)
	clearFloor(s)
	for i := 0; i < loot.InventorySize; i++ {
		s.Stash.Inv.Add(skill.NewPush(1))
	}
	at := s.P().At
	s.Stash.Drops = append(s.Stash.Drops, Drop{At: at, Part: skill.NewPush(3)})
	for i := 0; i < 30; i++ {
		s.Step(1.0 / 60)
	}
	full := 0
	s.FX.Each(func(f *FX) {
		if f.Kind == FXFull {
			full++
		}
	})
	if len(s.Stash.Drops) != 1 || full != 1 {
		t.Fatalf("drops %d, full warnings %d (want 1, 1)", len(s.Stash.Drops), full)
	}
	s.Scrap(0)
	s.Step(1.0 / 60)
	if len(s.Stash.Drops) != 0 || s.Stash.Inv.Parts[0].Value != 3 {
		t.Fatal("scrapping a part should make room for the drop")
	}
}

func TestSwapRecompilesAndRewiresDispatch(t *testing.T) {
	s := New(3)
	before := s.Load.Progs[0]
	dmg := func(p *skill.Program) (sum float32) {
		for _, st := range p.Steps {
			if st.Op == skill.OpDamage {
				sum += st.Value
			}
		}
		return
	}
	// Arc Lance: slot 4 is empty. A radiant x2 there doubles the whole
	// ring, stacking with the x2 already beside the damage.
	m := skill.NewMult(skill.VerbDamage, 2)
	m.Quirk = skill.QuirkRadiant
	cell := s.Stash.Inv.Add(m)
	if err := s.SwapIn(0, 4, cell); err != nil {
		t.Fatal(err)
	}
	after := s.Load.Progs[0]
	if after == before || dmg(after) != 2*dmg(before) {
		t.Fatalf("damage %.1f -> %.1f\n%s", dmg(before), dmg(after), after)
	}
	if s.Disp.Loadout(s.Player).Progs[0] != after {
		t.Fatal("dispatcher still runs the old program")
	}
	if s.Stash.Inv.Has[cell] || !s.Load.Edited[0] || s.Load.Blurb[0] == "plasma chains 3, calls Swarm" {
		t.Fatal("swap into an empty slot should empty the cell and mark the hex edited")
	}
	// Take it back out into an empty cell.
	if err := s.SwapIn(0, 4, cell); err != nil {
		t.Fatal(err)
	}
	if dmg(s.Load.Progs[0]) != dmg(before) || s.Stash.Inv.Parts[cell] != m {
		t.Fatal("removing the part should restore the hex")
	}
}

func TestSwapRefusals(t *testing.T) {
	s := New(3)
	prog := s.Load.Progs[1]
	trig := s.Stash.Inv.Add(skill.NewTrigger(skill.OnKill))
	if err := s.SwapIn(1, 2, trig); err == nil {
		t.Fatal("a trigger went into a wild slot")
	}
	push := s.Stash.Inv.Add(skill.NewPush(1))
	if err := s.SwapIn(1, 0, push); err == nil {
		t.Fatal("a push went into the trigger slot")
	}
	empty := s.Stash.Inv.Add(skill.NewPush(1))
	s.Scrap(empty)
	if err := s.SwapIn(1, 0, empty); err != ErrNeedsTrigger {
		t.Fatalf("emptying the trigger slot: %v", err)
	}
	if err := s.SwapSlots(1, 0, 1); err == nil {
		t.Fatal("swapped the trigger out of its slot")
	}
	if s.Load.Progs[1] != prog || s.Load.Edited[1] {
		t.Fatal("refused edits changed the hex")
	}
	if s.Stash.Inv.Count() != 2 {
		t.Fatalf("refused edits changed the inventory: %d", s.Stash.Inv.Count())
	}
}

func TestSwappedTriggerChangesHowItFires(t *testing.T) {
	s := New(3)
	cell := s.Stash.Inv.Add(skill.NewTrigger(skill.OnCast))
	if err := s.SwapIn(1, 0, cell); err != nil {
		t.Fatal(err)
	}
	if s.Load.Progs[1].Trigger != skill.OnCast || s.Stash.Inv.Parts[cell].Trigger != skill.OnHit {
		t.Fatal("trigger swap")
	}
	s.Rig.Power = s.Rig.MaxPower
	foe := s.nearestFoe(s.P().At, 99)
	if o := s.Disp.Cast(s.Player, 1, foe); o != skill.Fired {
		t.Fatalf("kinetic ram should now answer the button: %s", o)
	}
	// And a periodic trigger starts its timer.
	cell2 := s.Stash.Inv.Add(skill.NewPeriodic(2))
	if err := s.SwapIn(0, 0, cell2); err != nil {
		t.Fatal(err)
	}
	if l := s.Disp.Loadout(s.Player); l.Timer[0] != 2 {
		t.Fatalf("timer %.2f", l.Timer[0])
	}
}

func TestSwapSlotsRewires(t *testing.T) {
	s := New(3)
	// Arc Lance: chain, plasma, x2. Move the x2 two slots on, behind an
	// empty slot where it has nothing to scale, and the damage halves.
	before := s.Load.Progs[0]
	if err := s.SwapSlots(0, 3, 5); err != nil {
		t.Fatal(err)
	}
	var b, a float32
	for _, st := range before.Steps {
		if st.Op == skill.OpDamage {
			b = st.Value
		}
	}
	for _, st := range s.Load.Progs[0].Steps {
		if st.Op == skill.OpDamage {
			a = st.Value
		}
	}
	if a*2 != b {
		t.Fatalf("damage %.1f -> %.1f", b, a)
	}
}

func TestEditsLastTheRun(t *testing.T) {
	s := New(3)
	cell := s.Stash.Inv.Add(skill.NewDamage(s.Reg.MustID("cryo"), 20))
	if err := s.SwapIn(3, 4, cell); err != nil {
		t.Fatal(err)
	}
	s.Stash.Inv.Add(skill.NewPush(1))
	s.load(2, 1)
	if s.Load.Hexes[3].Slots[4].Kind != skill.SlotPart || !s.Load.Edited[3] || s.Stash.Inv.Count() != 1 {
		t.Fatal("next floor lost the edit or the inventory")
	}
	if s.Disp.Loadout(s.Player).Progs[3] != s.Load.Progs[3] {
		t.Fatal("next floor equips stale programs")
	}
	s.NewRun(4)
	if s.Load.Edited[3] || s.Stash.Inv.Count() != 0 || s.Load.Hexes[3].Slots[4].Kind != skill.SlotEmpty {
		t.Fatal("a new run should start from the preset with nothing carried")
	}
}

func TestDropAndPickupDoNotAllocate(t *testing.T) {
	s := New(9)
	s.load(20, 1)
	at := s.P().At
	ids := []skill.EntityID{}
	for i := range s.Actors {
		if skill.EntityID(i) != s.Player {
			ids = append(ids, skill.EntityID(i))
		}
	}
	k := 0
	n := testing.AllocsPerRun(200, func() {
		id := ids[k%len(ids)]
		k++
		s.dropLoot(id)
		if len(s.Stash.Drops) > 0 {
			s.Stash.Drops[0].At = at
		}
		s.pickup(1.0 / 60)
		if s.Stash.Inv.Full() {
			s.Stash.Inv.Clear()
		}
		if len(s.Stash.Drops) == cap(s.Stash.Drops) {
			s.Stash.Drops = s.Stash.Drops[:0]
		}
	})
	if n != 0 {
		t.Fatalf("%v allocs per drop and pickup", n)
	}
}

// BenchmarkSwapRecompile is one editor tap: trade a part into a hex,
// recompile it and hand it to the dispatcher.
func BenchmarkSwapRecompile(b *testing.B) {
	s := New(3)
	m := skill.NewMult(skill.VerbAll, 1.4)
	m.Quirk = skill.QuirkRadiant
	cell := s.Stash.Inv.Add(m)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if err := s.SwapIn(2, 5, cell); err != nil {
			b.Fatal(err)
		}
	}
}

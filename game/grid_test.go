package game

import (
	"encoding/json"
	"reflect"
	"testing"

	"hexes/skill"
)

func TestGridIsDeterministicAndGrowsStable(t *testing.T) {
	reg := skill.DefaultRegistry()
	for seed := uint64(1); seed <= 20; seed++ {
		a := GenGrid(reg, seed, GridRings)
		b := GenGrid(reg, seed, GridRings)
		if !reflect.DeepEqual(a.Nodes, b.Nodes) || a.Types != b.Types {
			t.Fatalf("seed %d: same seed, different grid", seed)
		}
		if len(a.Nodes) != 631 {
			t.Fatalf("a %d ring grid has %d cells, want 631", GridRings, len(a.Nodes))
		}
		// More rings keep the inner ones exactly.
		big := GenGrid(reg, seed, GridRings+2)
		if !reflect.DeepEqual(a.Nodes, big.Nodes[:len(a.Nodes)]) {
			t.Fatalf("seed %d: growing the grid changed its inner rings", seed)
		}
		other := GenGrid(reg, seed+1000, GridRings)
		if reflect.DeepEqual(a.Nodes, other.Nodes) {
			t.Fatalf("seed %d: two seeds rolled the same grid", seed)
		}
	}
}

func TestGridReachableAndGuarantees(t *testing.T) {
	reg := skill.DefaultRegistry()
	cryo := reg.MustID("cryo")
	for seed := uint64(1); seed <= 200; seed++ {
		g := GenGrid(reg, seed, GridRings+1)
		if !Reachable(g, g.Rings) {
			t.Fatalf("seed %d: a cell can't be reached", seed)
		}
		if !GridCheck(g, reg) {
			t.Fatalf("seed %d: grid breaks a promise", seed)
		}
		voids, outer := 0, 0
		keys := 0
		for i, n := range g.Nodes {
			if g.Index(n.At) != i {
				t.Fatalf("seed %d: node %d not indexed", seed, i)
			}
			if n.Ring < GridVoidFrom && n.Kind == NodeVoid {
				t.Fatalf("seed %d: void on ring %d", seed, n.Ring)
			}
			if n.Ring >= GridVoidFrom {
				outer++
				if n.Kind == NodeVoid {
					voids++
				}
			}
			if n.Kind == NodeBoard {
				keys++
				if n.Ring != 10 {
					t.Fatalf("seed %d: board keystone on ring %d", seed, n.Ring)
				}
			}
			if n.Kind == NodePart && n.Part.Kind == skill.KindNone {
				t.Fatalf("seed %d: node %d holds a bad part", seed, i)
			}
			if n.Kind == NodePart && n.Ring <= 10 && n.Part.Rarity == skill.Exotic {
				t.Fatalf("seed %d: exotic on ring %d", seed, n.Ring)
			}
		}
		if keys != BoardKeys {
			t.Fatalf("seed %d: %d board keystones, want %d", seed, keys, BoardKeys)
		}
		if f := float64(voids) / float64(outer); f < 0.05 || f > 0.14 {
			t.Fatalf("seed %d: voids are %.2f of the outer rings", seed, f)
		}
		// Ring 1 is all next to the start: the cryo sits there.
		cryoNext := false
		for i := RingStart(1); i < RingStart(2); i++ {
			p := g.Nodes[i].Part
			cryoNext = cryoNext || (p.Kind == skill.KindEffect && p.Effect == skill.EffectDamage && p.Type == cryo)
		}
		if !cryoNext {
			t.Fatalf("seed %d: no cryo beside the start", seed)
		}
		// Each ring leans on its idea: 40% of it, at least one per pair
		// of wedges.
		for k := 1; k <= g.Rings; k++ {
			open, teach := 0, 0
			var pair [3]bool
			for i := RingStart(k); i < RingStart(k+1); i++ {
				n := g.Nodes[i]
				if n.Kind == NodeVoid {
					continue
				}
				open++
				if n.Teach {
					teach++
					pair[n.Wedge/2] = true
				}
			}
			if float64(teach) < GridTeachFrac*float64(open)-0.001 || pair != [3]bool{true, true, true} {
				t.Fatalf("seed %d ring %d: %d of %d teach, pairs %v", seed, k, teach, open, pair)
			}
		}
	}
}

func TestGridRingIdeas(t *testing.T) {
	reg := skill.DefaultRegistry()
	plasma := reg.MustID("plasma")
	g := GenGrid(reg, 5, GridRings)
	for _, n := range g.Nodes {
		if !n.Teach || n.Kind == NodeVoid {
			continue
		}
		p := n.Part
		bad := false
		switch n.Ring {
		case 1:
			bad = p.Kind != skill.KindEffect || p.Effect != skill.EffectDamage || p.Type == plasma
		case 2:
			bad = p.Kind != skill.KindTarget
		case 3:
			bad = p.Kind != skill.KindTrigger || p.Trigger == skill.OnCast
		case 4:
			bad = p.Kind != skill.KindMult
		case 5:
			bad = !IsSustain(p)
		case 6:
			bad = n.Kind != NodeFrame
		case 7:
			bad = n.Kind != NodeShape
		case 8:
			bad = p.Kind != skill.KindGate
		case 10:
			bad = n.Kind != NodeBoard && p.Kind != skill.KindCall
		}
		if bad {
			t.Fatalf("ring %d teach node holds %+v", n.Ring, n)
		}
	}
	// Shapes are unique.
	ids := map[uint64]bool{}
	for _, n := range g.Nodes {
		if n.Kind == NodeShape {
			if ids[n.Shape.ID] {
				t.Fatal("two shape nodes share an id")
			}
			ids[n.Shape.ID] = true
		}
	}
}

func TestRingDepthsFollowTheLadder(t *testing.T) {
	want := []int{1, 1, 2, 3, 4, 5, 6, 7, 9, 11, 13, 15, 20, 25, 30, 40, 50}
	for k, d := range want {
		if got := RingDepth(k); got != d {
			t.Errorf("ring %d opens at %d, want %d", k, got, d)
		}
	}
	if RingsFor(1) != 14 || RingsFor(39) != 14 || RingsFor(40) != 15 || RingsFor(61) != 17 {
		t.Fatal("rings grow wrong")
	}
	if c := GridCells(14); c != 631 {
		t.Fatalf("%d cells", c)
	}
	g := &Grid{Nodes: []Node{{}, {Ring: 1}, {Ring: 7, Rarity: skill.Rare}, {Ring: 14}, {Ring: 3, Kind: NodeBoard}}}
	for i := range g.Nodes[1:4] {
		g.Nodes[i+1].Kind = NodePart
	}
	if g.Cost(1) != 15 || g.Cost(2) != 101 || g.Cost(3) != 119 || g.Cost(4) != CostBoard {
		t.Fatalf("costs %d %d %d %d", g.Cost(1), g.Cost(2), g.Cost(3), g.Cost(4))
	}
}

// atBench puts the player on the stairs bench.
func atBench(s *Sim) {
	s.P().placeFree(s.Benches[1])
}

// ownRing1 is a ring 1 node next to the start.
func firstOpen(s *Sim, ring int) int {
	for i := RingStart(ring); i < RingStart(ring+1); i++ {
		if s.Grid.Nodes[i].Kind != NodeVoid && !s.Own[i] && s.GridDist(i) == 1 {
			return i
		}
	}
	return -1
}

func TestFreshSaveBuysOneRing1Node(t *testing.T) {
	s := New(11)
	if s.DataTotal() != DataStart || s.Owned() != 1 {
		t.Fatalf("fresh save: %d Data, %d owned", s.DataTotal(), s.Owned())
	}
	i := firstOpen(s, 1)
	atBench(s)
	if _, err := s.CanBuy(i); err == nil {
		t.Fatal("ring 1 bought before the ladder opened it")
	}
	s.teach(1, nil) // EFFECTS: workbench and ring 1
	far := firstOpen(s, 2)
	if far >= 0 {
		t.Fatal("ring 2 is next to the start")
	}
	s.P().placeFree(s.Floor.Start.Add(skill.Coord{Q: 40}))
	if _, err := s.CanBuy(i); err == nil {
		t.Fatal("bought away from a workbench")
	}
	atBench(s)
	before := len(s.Locker)
	if err := s.Buy(i); err != nil {
		t.Fatal(err)
	}
	if s.DataTotal() != 0 || len(s.Locker) != before+1 || !s.Own[i] {
		t.Fatalf("after buy: %d Data, locker %d", s.DataTotal(), len(s.Locker))
	}
	if s.Buy(i) == nil {
		t.Fatal("bought a node twice")
	}
	// Its neighbors out on ring 2 are seen but sealed until TARGETS.
	sealed := false
	s.Grid.Near(i, func(j int) {
		if s.Grid.Nodes[j].Ring == 2 && s.CellView(j) == ViewSealed {
			sealed = true
		}
	})
	if !sealed {
		t.Fatal("ring 2 should show sealed next to a ring 1 node")
	}
}

func TestFogAndGlow(t *testing.T) {
	s := NewVeteran(4)
	for k := 1; k <= 2; k++ {
		s.Data += 1000
	}
	atBench(s)
	if err := s.Buy(firstOpen(s, 1)); err != nil {
		t.Fatal(err)
	}
	for i, n := range s.Grid.Nodes {
		v := s.CellView(i)
		d := s.GridDist(i)
		switch {
		case s.Own[i]:
			if v != ViewOwned {
				t.Fatalf("owned node shows %v", v)
			}
		case n.Kind == NodeVoid:
		case d == 1:
			if v != ViewCard && v != ViewSealed {
				t.Fatalf("a node next to you shows %v", v)
			}
		case d <= GlowSteps[n.Rarity] && n.Rarity > skill.Common:
			if v != ViewGlow {
				t.Fatalf("a %v node %d steps out shows %v, want glow", n.Rarity, d, v)
			}
		default:
			if v != ViewDark {
				t.Fatalf("a %v node %d steps out shows %v, want dark", n.Rarity, d, v)
			}
		}
	}
	// The test switch lifts the fog.
	if err := s.SetTestShapes(true); err != nil {
		t.Fatal(err)
	}
	for i, n := range s.Grid.Nodes {
		if v := s.CellView(i); n.Kind != NodeVoid && v != ViewOwned && v != ViewCard && v != ViewReclaim {
			t.Fatalf("test switch: node shows %v", v)
		}
	}
	if s.RingsOpen() != s.Grid.Rings {
		t.Fatal("test switch should open every ring")
	}
}

func TestSignalLightsKinds(t *testing.T) {
	s := NewVeteran(4)
	far := len(s.Grid.Nodes) - 1
	if s.UseSignal(far) == nil {
		t.Fatal("used a signal you don't have")
	}
	s.Signals = 1
	if err := s.UseSignal(far); err != nil {
		t.Fatal(err)
	}
	if s.Signals != 0 || s.CellView(far) != ViewKind && s.Grid.Nodes[far].Kind != NodeVoid {
		t.Fatalf("signal: %d left, view %v", s.Signals, s.CellView(far))
	}
}

// ownPath buys a line of nodes out to ring n (ladder done, rich).
func ownOut(t *testing.T, s *Sim, n int) {
	t.Helper()
	s.Data += 100000
	atBench(s)
	for k := 1; k <= n; k++ {
		bought := 0
		for i := RingStart(k); i < RingStart(k+1) && bought < 3; i++ {
			if _, err := s.CanBuy(i); err == nil {
				if err := s.Buy(i); err != nil {
					t.Fatal(err)
				}
				bought++
			}
		}
		if bought == 0 {
			t.Fatalf("nothing to buy on ring %d", k)
		}
	}
}

func connected(s *Sim) bool {
	seen := make([]bool, len(s.Own))
	q := []int{0}
	seen[0] = true
	n := 1
	for len(q) > 0 {
		i := q[0]
		q = q[1:]
		s.Grid.Near(i, func(j int) {
			if s.Own[j] && !seen[j] {
				seen[j] = true
				n++
				q = append(q, j)
			}
		})
	}
	return n == s.Owned()
}

func TestShrinkCount(t *testing.T) {
	cases := []struct {
		owned int
		mercy bool
		want  int
	}{{1, false, 2}, {10, false, 2}, {50, false, 4}, {80, false, 6}, {200, false, 10}, {50, true, 2}, {200, true, 5}, {10, true, 1}}
	for _, c := range cases {
		if got := ShrinkCount(c.owned, c.mercy); got != c.want {
			t.Errorf("ShrinkCount(%d, %v) = %d, want %d", c.owned, c.mercy, got, c.want)
		}
	}
}

func TestDeathShrinksTheGridOutsideIn(t *testing.T) {
	s := NewVeteran(9)
	ownOut(t, s, 9)
	s.load(20, 1)
	atBench(s)
	owned := s.Owned()
	locker := len(s.Locker)
	frames, shapes := len(s.Frames), len(s.Shapes)
	maxRing := 0
	for i, o := range s.Own {
		if o {
			maxRing = max(maxRing, s.Grid.Nodes[i].Ring)
		}
	}
	s.Unbanked = 37
	kill(s)
	want := ShrinkCount(owned, false)
	if s.LastDeath.LetGo != want || s.Owned() != owned-want {
		t.Fatalf("let go %d (owned %d -> %d), want %d", s.LastDeath.LetGo, owned, s.Owned(), want)
	}
	if !s.Own[0] || !connected(s) {
		t.Fatal("the shrink cut the start or split the region")
	}
	if len(s.Locker) != locker || len(s.Frames) != frames || len(s.Shapes) != shapes {
		t.Fatal("the shrink took a component")
	}
	// The cells that went were the farthest out.
	var gone []int
	for i, r := range s.Reclaim {
		if r {
			gone = append(gone, i)
		}
	}
	if len(gone) != want {
		t.Fatalf("%d reclaim cells, want %d", len(gone), want)
	}
	for _, i := range gone {
		if r := s.Grid.Nodes[i].Ring; r < maxRing-2 {
			t.Fatalf("let go a ring %d cell while owning ring %d", r, maxRing)
		}
	}
	// Unbanked Data went in the crate.
	if s.Unbanked != 0 || s.Cache.Data != 37 || !s.Cache.Active || s.LastDeath.Data != 37 {
		t.Fatalf("unbanked %d, crate %d", s.Unbanked, s.Cache.Data)
	}
	// Reclaim: half price, no second copy, from the new frontier.
	s.Step(1)
	s.respawn()
	atBench(s)
	for _, i := range gone {
		if s.GridDist(i) != 1 {
			continue
		}
		cost, err := s.CanBuy(i)
		if err != nil {
			t.Fatal(err)
		}
		if full := s.Grid.Cost(i); cost != (full+1)/2 {
			t.Fatalf("reclaim costs %d, want half of %d", cost, full)
		}
		l := len(s.Locker)
		if err := s.Buy(i); err != nil {
			t.Fatal(err)
		}
		if len(s.Locker) != l || s.Reclaim[i] {
			t.Fatal("buying back a reclaim cell gave a second copy")
		}
		return
	}
	t.Fatal("no reclaim cell on the new frontier")
}

func TestMercyHalvesTheShrinkAndFallIsOneFloor(t *testing.T) {
	s := NewVeteran(9)
	ownOut(t, s, 9)
	s.load(6, 1)
	owned := s.Owned()
	kill(s)
	if s.LastDeath.To != 5 {
		t.Fatalf("death at 6 fell to %d, want 5", s.LastDeath.To)
	}
	first := s.LastDeath.LetGo
	s.Step(1)
	s.respawn()
	s.Time += 10
	kill(s)
	if !s.LastDeath.Mercy || s.LastDeath.LetGo != ShrinkCount(owned-first, true) {
		t.Fatalf("mercy death let go %d, want %d", s.LastDeath.LetGo, ShrinkCount(owned-first, true))
	}
}

func TestDataEconomy(t *testing.T) {
	s := NewVeteran(3)
	s.load(4, 1)
	base := s.DataTotal()
	s.load(5, 1) // first time at 5, a checkpoint: pays and banks
	if s.Unbanked != 0 || s.Data != base+DataDepthBase+DataDepthStep*5 {
		t.Fatalf("depth bonus: data %d unbanked %d (was %d)", s.Data, s.Unbanked, base)
	}
	s.load(6, 1)
	if s.Unbanked != DataDepthBase+DataDepthStep*6 {
		t.Fatalf("depth 6 pays %d unbanked", s.Unbanked)
	}
	// Coming back to a floor pays nothing new.
	u := s.Unbanked
	s.load(6, 1)
	if s.Unbanked != u {
		t.Fatal("a revisit paid the depth bonus again")
	}
	if s.KillData(KindGrunt, false) != DataSmall || s.KillData(KindBrute, false) != DataBrute || s.KillData(KindGrunt, true) != DataElite {
		t.Fatal("kill pay")
	}
	s.Best = 20
	s.Depth = 10
	if s.KillData(KindBrute, false) != DataBrute/2 {
		t.Fatal("old floors should pay half")
	}
	// Spending takes unbanked first.
	s.Best, s.Depth = 6, 6
	s.Unbanked, s.Data = 10, 100
	atBench(s)
	i := firstOpen(s, 1)
	cost, err := s.CanBuy(i)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Buy(i); err != nil {
		t.Fatal(err)
	}
	if s.Unbanked != max(0, 10-cost) || s.Data != 100-max(0, cost-10) {
		t.Fatalf("spend: unbanked %d data %d after %d", s.Unbanked, s.Data, cost)
	}
}

func TestSalvageReturnsData(t *testing.T) {
	s := NewVeteran(7)
	s.load(12, 1)
	s.Unbanked = 20
	kill(s)
	s.Step(1)
	s.respawn()
	if s.Cache.Depth != 12 {
		t.Fatal("crate not left where you died")
	}
	s.load(12, 1)
	before := s.Unbanked
	s.salvage(s.Cache.At)
	if s.Unbanked != before+20+DataSalvage || s.Cache.Active {
		t.Fatalf("salvage: unbanked %d (was %d)", s.Unbanked, before)
	}
}

func TestGridSurvivesASave(t *testing.T) {
	s := NewVeteran(21)
	ownOut(t, s, 6)
	s.Signals, s.Unbanked = 2, 9
	s.Glimpse[len(s.Glimpse)-1] = true
	b, err := s.Save()
	if err != nil {
		t.Fatal(err)
	}
	r := NewVeteran(99)
	if err := r.LoadSave(b); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r.Own, s.Own) || !reflect.DeepEqual(r.Seen, s.Seen) || !reflect.DeepEqual(r.Glimpse, s.Glimpse) ||
		r.Data != s.Data || r.Unbanked != s.Unbanked || r.Signals != 2 || len(r.Locker) != len(s.Locker) ||
		!reflect.DeepEqual(r.Grid.Nodes, s.Grid.Nodes) {
		t.Fatal("the grid didn't come back the same")
	}
	// A save from before the grid doesn't load.
	var sv map[string]any
	_ = json.Unmarshal(b, &sv)
	sv["Version"] = 2
	old, _ := json.Marshal(sv)
	if New(1).LoadSave(old) == nil {
		t.Fatal("a version 2 save loaded")
	}
}

func TestDropsAreKindsYouOwn(t *testing.T) {
	s := NewVeteran(8)
	// Own only the start: drops are effects and triggers.
	for seed := uint64(1); seed <= 30; seed++ {
		s.Visits = int(seed)
		s.load(8, 1)
		killAll(s, enemyOrder(s, false))
	}
	if len(s.Stash.Drops) == 0 {
		t.Fatal("nothing dropped")
	}
	for _, d := range s.Stash.Drops {
		if k := d.Part.Kind; k != skill.KindEffect && k != skill.KindTrigger {
			t.Fatalf("dropped a %v with only the start owned", k)
		}
		if IsSustain(d.Part) {
			t.Fatal("dropped a sustain part you don't own")
		}
	}
	if len(s.Stash.Shapes) != 0 || len(s.Stash.Frames) != 0 {
		t.Fatal("shapes and frames don't drop any more")
	}
}

func TestLockerEdits(t *testing.T) {
	s := NewVeteran(2)
	ty := s.Reg.MustID("cryo")
	s.Locker = append(s.Locker, skill.NewDamage(ty, 30))
	h := s.Load.Hex(0)
	// Find a slot holding a damage effect.
	slot := -1
	for i, sl := range h.Slots {
		if h.Shape.Has(i) && sl.Kind == skill.SlotPart && sl.Part.Kind == skill.KindEffect {
			slot = i
			break
		}
	}
	if slot < 0 {
		t.Skip("no effect slot")
	}
	old := h.Slots[slot].Part
	if err := s.LockerIn(0, slot, 0); err != nil {
		t.Fatal(err)
	}
	if h.Slots[slot].Part.Type != ty || len(s.Locker) != 1 || s.Locker[0] != old {
		t.Fatal("locker swap didn't trade places")
	}
	if err := s.LockerOut(0, h.Shape.Entry()); err != ErrNeedsTrigger {
		t.Fatalf("took the trigger out: %v", err)
	}
}

func TestGridStepAllocFree(t *testing.T) {
	s := NewVeteran(5)
	s.load(6, 1)
	allocs := testing.AllocsPerRun(300, func() {
		s.Input.MoveX = 1
		s.Step(1.0 / 60)
		if s.State != Playing {
			s.load(6, 1)
		}
	})
	if allocs > 0 {
		t.Fatalf("step allocates %.1f times", allocs)
	}
}

// Class parts fill part of every ring from ClassRing out, the rings
// inside are untouched, and a save from before them keeps what it owns.
func TestGridClassParts(t *testing.T) {
	reg := skill.DefaultRegistry()
	plain := GenGrid(reg, 77, GridRings)
	g := GenGridKeep(reg, 77, GridRings, nil)
	for i := 0; i < RingStart(ClassRing); i++ {
		if g.Nodes[i].Part != plain.Nodes[i].Part {
			t.Fatalf("node %d inside ring %d changed", i, ClassRing)
		}
	}
	seen := map[skill.PartKind]int{}
	changed := -1
	for k := ClassRing; k <= GridRings; k++ {
		n := 0
		for i := RingStart(k); i < RingStart(k+1); i++ {
			if g.Nodes[i].Part != plain.Nodes[i].Part {
				n++
				seen[g.Nodes[i].Part.Kind]++
				changed = i
			}
		}
		if n == 0 {
			t.Fatalf("ring %d has no class parts", k)
		}
	}
	if seen[skill.KindForm] == 0 {
		t.Fatalf("no forms on the grid: %v", seen)
	}
	kept := GenGridKeep(reg, 77, GridRings, []int{changed})
	if kept.Nodes[changed].Part != plain.Nodes[changed].Part {
		t.Fatal("a kept cell took a class part")
	}

	// An older save owning that cell keeps it through a load.
	s := NewVeteran(77)
	s.Best = 30
	s.growGrid()
	sv := s.Snapshot()
	sv.PartsV, sv.Kept = 0, nil
	sv.Own = append(sv.Own, changed)
	s2 := NewVeteran(77)
	if err := s2.Restore(sv); err != nil {
		t.Fatal(err)
	}
	if s2.Grid.Nodes[changed].Part != plain.Nodes[changed].Part {
		t.Fatal("an old save's owned cell changed")
	}
	if s3 := s2.Snapshot(); s3.PartsV != GridPartsV || len(s3.Kept) == 0 {
		t.Fatalf("kept cells not saved: %d %v", s3.PartsV, s3.Kept)
	}
}

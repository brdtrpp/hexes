package game

import (
	"testing"

	"hexes/dungeon"
	"hexes/loot"
	"hexes/skill"
)

// toStairs puts the player on the stairs and steps once.
func toStairs(t *testing.T, s *Sim) {
	t.Helper()
	f := s.Floor
	if f.At(f.Exit) != dungeon.Stairs {
		t.Fatal("the exit isn't stairs")
	}
	p := s.P()
	s.occ[f.Index(p.At)] = skill.NoEntity
	p.placeFree(f.Exit)
	s.occ[f.Index(f.Exit)] = s.Player
	s.refreshField()
	s.Input = Input{}
	s.Step(1.0 / 60)
}

// freshWithBench is a fresh save with the workbench lesson taken, so the
// draft can offer grid cells and parts.
func freshWithBench(seed uint64) *Sim {
	s := New(seed)
	s.teach(1, []skill.Part{s.lessonEffect()})
	return s
}

func TestDraftAtTheStairs(t *testing.T) {
	s := freshWithBench(3)
	toStairs(t, s)
	if s.State != Drafting || !s.Draft.On || s.Draft.N != 3 || s.Draft.Depth != 1 {
		t.Fatalf("state %d, draft %+v", s.State, s.Draft)
	}
	for i := 0; i < 30; i++ {
		s.Step(1.0 / 60)
	}
	if s.State != Drafting || s.Depth != 1 {
		t.Fatal("the game moved on while the draft waited")
	}
	data, locker := s.DataTotal(), len(s.Locker)
	owned := s.Owned()
	pick := s.Draft.Picks[0]
	if err := s.TakeDraft(0); err != nil {
		t.Fatal(err)
	}
	switch pick.Kind {
	case DraftNode:
		if s.Owned() != owned+1 || !s.Own[pick.Node] || s.DataTotal() != data {
			t.Fatalf("a drafted grid cell: owned %d to %d, Data %d to %d", owned, s.Owned(), data, s.DataTotal())
		}
	case DraftPart:
		if len(s.Locker) != locker+1 {
			t.Fatal("a drafted part didn't reach the locker")
		}
	case DraftData:
		if s.DataTotal() != data+pick.Data {
			t.Fatal("drafted Data didn't arrive")
		}
	}
	if s.State != Descending || s.DraftedTo != 1 || s.Draft.On {
		t.Fatalf("after the pick: state %d, drafted to %d", s.State, s.DraftedTo)
	}
	for i := 0; i < 90 && s.Depth == 1; i++ {
		s.Step(1.0 / 60)
	}
	if s.Depth != 2 {
		t.Fatal("the stairs didn't go down after the pick")
	}
	// The same floor walked again (after a death, say) doesn't draft.
	s.load(1, 1)
	toStairs(t, s)
	if s.State != Descending {
		t.Fatalf("a floor drafted twice: state %d", s.State)
	}
}

func TestDraftSkipPaysData(t *testing.T) {
	s := freshWithBench(4)
	toStairs(t, s)
	before, skip := s.Data, s.Draft.Skip
	if err := s.TakeDraft(-1); err != nil {
		t.Fatal(err)
	}
	if skip <= 0 || s.Data != before+skip || s.State != Descending {
		t.Fatalf("skip paid %d of %d", s.Data-before, skip)
	}
	if err := s.TakeDraft(0); err == nil {
		t.Fatal("picked from a draft that was gone")
	}
}

func TestCheckpointDraftHasFour(t *testing.T) {
	s := freshWithBench(5)
	d := s.rollDraft(5)
	if d.N != 4 || d.Picks[3].Lean != LeanBig {
		t.Fatalf("depth 5 draft: %d offers", d.N)
	}
	if d := s.rollDraft(4); d.N != 3 {
		t.Fatalf("depth 4 draft: %d offers", d.N)
	}
}

// Drafts only ever offer what the ladder has taught, and grid offers are
// cells next to yours on open rings.
func TestDraftKeepsToTheLadder(t *testing.T) {
	for seed := uint64(1); seed <= 20; seed++ {
		s := New(seed)
		for idx := 0; idx < len(Ladder); idx++ {
			if idx > 0 {
				parts, n := s.lessonParts(idx)
				s.teach(idx, parts[:n])
			}
			u := s.Unlocks()
			allow := s.lootAllow()
			for depth := Ladder[idx].Depth; depth < Ladder[idx].Depth+3; depth++ {
				d := s.rollDraft(depth)
				if d.N < 3 {
					t.Fatalf("seed %d: %d offers", seed, d.N)
				}
				for _, p := range d.Picks[:d.N] {
					switch p.Kind {
					case DraftNone, DraftKit:
						t.Fatalf("seed %d depth %d: empty offer %+v", seed, depth, p)
					case DraftNode:
						n := &s.Grid.Nodes[p.Node]
						if s.Own[p.Node] || s.GridDist(p.Node) != 1 || !s.RingOpen(n.Ring) || n.Kind == NodeVoid {
							t.Fatalf("seed %d: grid offer %d isn't a free cell next to yours", seed, p.Node)
						}
					case DraftPart:
						fam := loot.FamOf(p.Part)
						if fam >= 0 && allow.New&(1<<fam) == 0 {
							t.Fatalf("seed %d after %s: offered a %s part early", seed, Ladder[idx].Name, loot.FamName(fam))
						}
						if u&UBench == 0 || (fam < 0 && allow.Limit && allow.Kinds&(1<<p.Part.Kind) == 0) {
							t.Fatalf("seed %d after %s: offered a %v", seed, Ladder[idx].Name, p.Part.Kind)
						}
						if p.Part.Rarity == skill.Exotic && u&UQuirks == 0 {
							t.Fatalf("seed %d: exotic before quirks", seed)
						}
					case DraftFrame:
						if u&UFrames == 0 {
							t.Fatalf("seed %d after %s: a frame before frames", seed, Ladder[idx].Name)
						}
					case DraftShape:
						if u&UShapes == 0 {
							t.Fatalf("seed %d after %s: a shape before shapes", seed, Ladder[idx].Name)
						}
					}
					if title, _, _ := s.Describe(p); title == "" {
						t.Fatalf("seed %d: offer %+v has no words", seed, p)
					}
				}
			}
		}
	}
}

// The draft's floor survives a save, and saves from before drafts don't
// re-draft floors they've already been down from.
func TestDraftSaves(t *testing.T) {
	s := freshWithBench(6)
	s.DraftedTo = 4
	r := New(1)
	if err := r.Restore(s.Snapshot()); err != nil {
		t.Fatal(err)
	}
	if r.DraftedTo != 4 {
		t.Fatalf("drafted to %d after a reload", r.DraftedTo)
	}
	sv := s.Snapshot()
	sv.DraftedTo, sv.Best, sv.Depth = 0, 9, 3
	if err := r.Restore(sv); err != nil {
		t.Fatal(err)
	}
	if r.DraftedTo != 8 {
		t.Fatalf("an old save at best 9 counts drafts to %d", r.DraftedTo)
	}
}

func TestKitsDealDifferentStarts(t *testing.T) {
	starts := map[uint8]bool{}
	for seed := uint64(1); seed <= 40; seed++ {
		k := DealKits(seed)
		seen := map[uint8]bool{}
		rigs := map[uint8]bool{}
		for _, kt := range k {
			if seen[kt.Types[0]] || rigs[kt.Rig] {
				t.Fatalf("seed %d deals two kits alike: %+v", seed, k)
			}
			seen[kt.Types[0]], rigs[kt.Rig] = true, true
			types := map[uint8]bool{}
			for _, ty := range kt.Types {
				types[ty] = true
			}
			if len(types) != 4 {
				t.Fatalf("seed %d: kit repeats a type %+v", seed, kt)
			}
		}
		starts[k[0].Types[0]] = true
	}
	if len(starts) < 3 {
		t.Fatalf("first kits only ever start with %d types", len(starts))
	}
}

func TestKitPick(t *testing.T) {
	s := New(9)
	if !s.OfferKits() || s.State != Drafting || !s.Draft.Kits || s.Draft.N != KitDeal {
		t.Fatalf("no kit pick on a fresh save: state %d", s.State)
	}
	kt := DealKits(9)[1]
	if err := s.TakeDraft(1); err != nil {
		t.Fatal(err)
	}
	if s.State != Playing || s.Kit != 2 || s.Points != RigProfiles[kt.Rig].Points || s.Rig.MaxPower != skill.NewRig(RigProfiles[kt.Rig].Points).MaxPower {
		t.Fatalf("after the pick: state %d, kit %d, points %+v", s.State, s.Kit, s.Points)
	}
	h := s.Load.Hexes[0]
	order, n := h.Flow(h.TriggerSlot())
	eff := h.Slots[order[1]].Part
	if h.Shape.Kind != skill.ShapeTriangle || n != 2 || h.Slots[order[0]].Part.Trigger != skill.OnCast ||
		eff.Type != s.Reg.MustID(kt.StartType()) || h.Slots[2].Kind != skill.SlotEmpty {
		t.Fatalf("kit starter hex %+v", h.Slots)
	}
	if s.Load.Names[0] != kt.KitName() || s.CardT <= 0 || s.Card != 0 || !s.Load.Castable(0) || s.Load.Keys[0] != 1 {
		t.Fatalf("starter %q, card %v, key %d", s.Load.Names[0], s.CardT, s.Load.Keys[0])
	}
	if s.lessonEffect().Type != s.Reg.MustID(kt.LessonType()) {
		t.Fatal("the EFFECTS lesson isn't the kit's type")
	}
	if s.OfferKits() {
		t.Fatal("a kit pick offered twice")
	}
	// The kit and the hexes it hands over later survive a save.
	r := New(1)
	if err := r.Restore(s.Snapshot()); err != nil {
		t.Fatal(err)
	}
	if r.Kit != 2 {
		t.Fatalf("kit %d after a reload", r.Kit)
	}
	_, name, _ := r.ladderHex(1)
	if want, _, _ := kitHex(r.Reg, kt, 1); name != want.Name {
		t.Fatalf("hex 2 would be %q, the kit says %q", name, want.Name)
	}
}

// A save from before kits keeps the old opening.
func TestNoKitIsTheOldOpening(t *testing.T) {
	s := New(2)
	if s.Kit != 0 || s.Load.Names[0] != "Spark" {
		t.Fatalf("a save with no kit opens with %q", s.Load.Names[0])
	}
	if _, name, _ := s.ladderHex(1); name != "Jolt" {
		t.Fatalf("hex 2 is %q", name)
	}
	s.Kills = 1
	if s.CanPickKit() {
		t.Fatal("kit pick after a kill")
	}
}

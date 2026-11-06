package game

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"hexes/loot"
	"hexes/skill"
)

// Shapes in the sim. A hex's shape is its control flow (see skill.Shape).
// Everyone has the plain Ring and the plain Triangle. The rest are found:
// rare drops deep down, each one a generated graph (skill.GenGraph) that
// grows bigger and stranger with depth, with one rolled quirk. Shapes
// found before generation (the fixed Hub, Fork, Loop and Triangle) keep
// working as they were. A found shape is
// learned the moment you walk over it and kept for good, like the rest of
// progress; a death never takes it. Each found copy goes on one hex at a
// time, and the editor's SHAPE button switches a hex between the ones you
// have.

// Shape drop tuning. Placeholder balance, all constants.
const (
	ShapeMinDepth  = 5     // no shapes drop above this depth
	ShapeDropBase  = 0.006 // per kill at ShapeMinDepth
	ShapeDropStep  = 0.002 // more per depth below it
	ShapeDropMax   = 0.04
	ShapeDropElite = 3 // times more likely from a foe above the floor's level
	MaxShapes      = 32
	MaxShapeDrops  = 8 // shapes that can lie on one floor
)

// ShapeDrop is a shape lying on the floor.
type ShapeDrop struct {
	At    skill.Coord
	Shape skill.Shape
	Age   float32
}

// ShapeDropChance is the chance a kill at depth drops a shape.
func ShapeDropChance(depth int) float32 {
	if depth < ShapeMinDepth {
		return 0
	}
	return min(ShapeDropMax, ShapeDropBase+ShapeDropStep*float32(depth-ShapeMinDepth))
}

// ShapeSockets is the socket layout of a game hex of a shape: the trigger
// where the shape wants it, wild sockets everywhere else, except an
// original Hub's center, which takes effects, multipliers, charges and
// calls (no targets or gates: the center pools, it doesn't pick or split).
func ShapeSockets(sh skill.Shape) [skill.Slots]skill.SocketKind {
	var s [skill.Slots]skill.SocketKind
	for i := range s {
		s[i] = skill.SocketWild
	}
	if sh.Kind.Legacy() {
		s[skill.Center] = skill.SocketEffect
	}
	s[sh.Entry()] = skill.SocketTrigger
	return s
}

// rollShapeDrop maybe drops a shape from a dying foe. It uses its own
// random stream so it never changes which parts drop.
func (s *Sim) rollShapeDrop(id skill.EntityID, at skill.Coord, elite bool) {
	chance := ShapeDropChance(s.Depth)
	if chance <= 0 || len(s.Stash.Shapes) >= MaxShapeDrops || !s.Has(UShapes) {
		return
	}
	if elite {
		chance *= ShapeDropElite
	}
	r := loot.NewRNG(loot.Seed(s.Floor.Seed^hash(uint64(s.Visits))^0x5ba9e, int(id)))
	if r.Float() >= chance {
		return
	}
	s.Stash.Shapes = append(s.Stash.Shapes, ShapeDrop{At: at, Shape: skill.RollShape(r.Next, r.Next(), s.Depth)})
}

// pickupShapes learns any shape under the player.
func (s *Sim) pickupShapes(at skill.Coord, dt float32) {
	d := s.Stash.Shapes
	for i := 0; i < len(d); {
		d[i].Age += dt
		if d[i].At != at || len(s.Shapes) >= MaxShapes {
			i++
			continue
		}
		s.Shapes = append(s.Shapes, d[i].Shape)
		s.ShapeGiven = true
		s.Dirty = true
		s.LastShape = d[i].Shape
		s.addFX(FX{Kind: FXShape, At: at, Slot: int(d[i].Shape.Kind), Life: 2.2})
		d[i] = d[len(d)-1]
		d = d[:len(d)-1]
	}
	s.Stash.Shapes = d
}

// ShapeChoices is every shape hex could take, in the order the SHAPE
// button steps through them: the plain Ring and Triangle, then each found
// copy that isn't on another hex.
func (s *Sim) ShapeChoices(hex int) []skill.Shape {
	out := []skill.Shape{{Kind: skill.ShapeRing}, {Kind: skill.ShapeTriangle}}
	have := s.Shapes
	if s.TestShapes {
		have = append(append([]skill.Shape(nil), have...), TestShapeSet...)
	}
	for _, sh := range have {
		used := false
		for j := 0; j < s.Load.Count(); j++ {
			h := s.Load.Hex(j)
			used = used || (j != hex && h != nil && h.Shape.ID == sh.ID)
		}
		if !used {
			out = append(out, sh)
		}
	}
	return out
}

// CycleShape switches a hex to the next shape in ShapeChoices and returns
// it. On error the hex is unchanged.
func (s *Sim) CycleShape(hex int) (skill.Shape, error) {
	if !s.Load.Has(hex) {
		return skill.Shape{}, errors.New("no such hex")
	}
	cur := s.Load.Hex(hex).Shape
	ch := s.ShapeChoices(hex)
	at := 0
	for i, sh := range ch {
		if sh == cur {
			at = i
			break
		}
	}
	// Skip any shape the hex can't take right now (a full pack) and say
	// why only if none fits.
	var first error
	for k := 1; k < len(ch); k++ {
		next := ch[(at+k)%len(ch)]
		err := s.SetShape(hex, next)
		if err == nil {
			return next, nil
		}
		if first == nil {
			first = err
		}
	}
	if first == nil {
		first = errors.New("no other shapes yet: they drop deeper down")
	}
	return cur, first
}

// SetShape gives a hex a new shape. Parts stay in the slots they were in
// where the new shape has that slot and it takes them; the trigger moves
// to where the new shape wants it; anything left over goes in the first
// free slot that takes it, in flow order, and then into the pack. If the
// pack can't take the leftovers the shape isn't changed.
func (s *Sim) SetShape(hex int, sh skill.Shape) error {
	if !s.Load.Has(hex) {
		return errors.New("no such hex")
	}
	if !sh.Valid() {
		return errors.New("broken shape")
	}
	if sh.ID != 0 && !s.hasShape(sh) {
		return errors.New("you haven't found that shape")
	}
	h := s.Load.Hex(hex)
	old := *h
	// The parts move the way a reshape always moved them, then the new
	// room lays them out to run that way.
	nh := old
	nh.Lay = skill.Layout{}
	nh.Shape = sh
	nh.Sockets = ShapeSockets(sh)
	if s.Load.IsMod(hex) {
		nh.Sockets = ModuleSockets()
	}
	nh.Slots = [skill.Slots]skill.Slot{}
	// Frames stay on their slots where they can; the rest go back on the
	// rack. Parts then only go where the frames take them.
	back := settleFrames(&old, &nh)
	takes := func(i int, p skill.Part) bool { ok, _ := nh.Fits(i, p, s.Reg); return ok }
	var extra []skill.Part
	trig := sh.Entry()
	for i, sl := range old.Slots {
		switch sl.Kind {
		case skill.SlotEmpty:
			continue
		case skill.SlotNested:
			return errors.New("that hex holds a nested hex")
		}
		p := sl.Part
		switch {
		case p.Kind == skill.KindTrigger:
			nh.Slots[trig] = sl
		case sh.Has(i) && i != trig && takes(i, p):
			nh.Slots[i] = sl
		default:
			extra = append(extra, p)
		}
	}
	order, n := nh.Flow(trig)
	var pack []skill.Part
	for _, p := range extra {
		placed := false
		for _, i := range order[:n] {
			if nh.Slots[i].Kind == skill.SlotEmpty && takes(i, p) {
				nh.Slots[i] = skill.Slot{Kind: skill.SlotPart, Part: p}
				placed = true
				break
			}
		}
		if !placed {
			pack = append(pack, p)
		}
	}
	inv := &s.Stash.Inv
	if free := loot.InventorySize - inv.Count(); free < len(pack) {
		return fmt.Errorf("a %s has room for fewer parts: free %d pack cells first", sh.Name(), len(pack)-free)
	}
	if room := MaxFrames - len(s.Frames); room < len(back) {
		return fmt.Errorf("a %s has fewer slots for frames: scrap %d from the rack first", sh.Name(), len(back)-room)
	}
	nh.LayOut(skill.RoomFor(sh))
	*h = nh
	if err := s.recompile(hex); err != nil {
		*h = old
		return err
	}
	for _, p := range pack {
		inv.Add(p) // came out of a hex, so already safe from death
	}
	if len(back) > 0 {
		s.Frames = append(s.Frames, back...)
		s.Dirty = true
	}
	return nil
}

func (s *Sim) hasShape(sh skill.Shape) bool {
	if s.TestShapes && IsTestShape(sh) {
		for _, t := range TestShapeSet {
			if t == sh {
				return true
			}
		}
	}
	for _, have := range s.Shapes {
		if have == sh {
			return true
		}
	}
	return false
}

// The test switch. With it on, every kind of shape (and every frame, see
// frames.go) is unlocked without
// finding it: one test copy of each original found shape, each with a
// different quirk so all four show, then a spread of generated ones from
// small to the full nine slots, covering every family. Test copies are
// never added to the shapes you've found, and turning the switch off puts
// any hex wearing one back to a Ring.
const testShapeBit = 1 << 63

// TestShapeSet is the copies the test switch unlocks.
var TestShapeSet = testShapes()

func testShapes() []skill.Shape {
	out := []skill.Shape{
		{Kind: skill.ShapeHub, Quirk: skill.ShapeTwin, At: 2, ID: testShapeBit | 1},
		{Kind: skill.ShapeFork, Quirk: skill.ShapeSurge, At: 1, ID: testShapeBit | 2},
		{Kind: skill.ShapeLoop, Quirk: skill.ShapeOneWay, At: 2, ID: testShapeBit | 3},
		{Kind: skill.ShapeTriangle, Quirk: skill.ShapeLean, ID: testShapeBit | 4},
	}
	// Generated: walk a fixed seed and keep the first shape of each
	// family and size, smallest first, so the set is the same every run.
	type key struct {
		f skill.ShapeKind
		n int
	}
	seen := map[key]bool{}
	r := loot.NewRNG(0x7e57)
	var gen []skill.Shape
	for i := 0; i < 4000 && len(gen) < TestGenShapes; i++ {
		sh := skill.RollShape(r.Next, 0, 5+i%50)
		k := key{sh.Family(), sh.Len()}
		if seen[k] || (k.n < 5 && k.f != skill.ShapeTriangle) {
			continue
		}
		seen[k] = true
		gen = append(gen, sh)
	}
	sort.SliceStable(gen, func(i, j int) bool {
		if gen[i].Len() != gen[j].Len() {
			return gen[i].Len() < gen[j].Len()
		}
		return gen[i].Family() < gen[j].Family()
	})
	for i, sh := range gen {
		sh.ID = testShapeBit | uint64(16+i)
		out = append(out, sh)
	}
	return out
}

// TestGenShapes is how many generated shapes the test switch unlocks.
const TestGenShapes = 16

// IsTestShape reports whether sh is one of the test switch's copies.
func IsTestShape(sh skill.Shape) bool { return sh.ID&testShapeBit != 0 }

// SetTestShapes turns the test switch on or off. Turning it off puts every
// hex wearing a test shape back to a Ring; if a hex's leftover parts don't
// fit in the pack the switch stays on and says why.
func (s *Sim) SetTestShapes(on bool) error {
	if on == s.TestShapes {
		return nil
	}
	if !on {
		if err := s.fixBoard(BoardOpen(s.Best)); err != nil {
			return err
		}
		for i := 0; i < s.Load.Count(); i++ {
			h := s.Load.Hex(i)
			if h == nil || !IsTestShape(h.Shape) {
				continue
			}
			if err := s.SetShape(i, skill.Shape{Kind: skill.ShapeRing}); err != nil {
				return fmt.Errorf("%s: %w", s.Load.Name(i), err)
			}
		}
		// Test frames go too; the parts under them stay.
		s.stripTestFrames()
		if err := s.rebuildAll(-1); err != nil {
			return err
		}
	}
	s.TestShapes = on
	s.Dirty = true
	// The board docks only once it's taught, and TEST teaches it.
	return s.rebuildAll(-1)
}

// ShapeText says what a shape does, in a sentence or two, for the editor.
func ShapeText(sh skill.Shape) string {
	var t string
	switch sh.Kind {
	case skill.ShapeRing:
		t = "Ring: six slots, clockwise, each feeding the next."
	case skill.ShapeTriangle:
		t = "Triangle: three slots in a line. Runs on 25% less heat."
	case skill.ShapeHub:
		t = "Hub: the ring all feeds the center. An effect there lands once on everything the ring targeted; a multiplier there scales the whole ring."
	case skill.ShapeFork:
		t = "Fork: the trigger in the center runs two chains of three, side by side."
	case skill.ShapeLoop:
		t = "Loop: the last slot feeds back to the first. Whoever it carries gets another pass, each costing heat."
	case skill.ShapeGen:
		t = genText(sh)
	}
	switch sh.Quirk {
	case skill.ShapeTwin:
		t += fmt.Sprintf(" Twin: slot %d fires twice.", sh.At+1)
	case skill.ShapeSurge:
		t += fmt.Sprintf(" Surge: slot %d hits 50%% harder.", sh.At+1)
	case skill.ShapeOneWay:
		t += fmt.Sprintf(" One-way: edge %d-%d only feeds forward.", sh.At+1, sh.Graph().FirstTwo(int(sh.At))+1)
	case skill.ShapeLean:
		t += " Lean: 25% less heat."
	}
	return t
}

// genText describes a generated shape from its features: how many slots,
// where it splits and merges, whether it loops, and its heat.
func genText(sh skill.Shape) string {
	g := sh.Graph()
	var splits, merges []string
	for i := 0; i < int(g.N); i++ {
		if g.Outs(i) >= 2 {
			splits = append(splits, fmt.Sprint(i+1))
		}
		if g.Ins(i) >= 2 {
			merges = append(merges, fmt.Sprint(i+1))
		}
	}
	t := fmt.Sprintf("%s: %d slots", sh.Name(), g.N)
	if len(splits) > 0 {
		t += ", splits after " + strings.Join(splits, " and ")
	}
	if len(merges) > 0 {
		t += ", merges at " + strings.Join(merges, " and ")
	}
	t += "."
	if l := g.LoopFrom(); l >= 0 {
		t += fmt.Sprintf(" Whoever slot %d carries loops back to 1.", l+1)
	}
	if len(merges) > 0 {
		t += " A merge pools everything linked into it."
	}
	return t + fmt.Sprintf(" Heat x%.2f.", sh.HeatScale())
}

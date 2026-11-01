package game

import (
	"errors"
	"fmt"

	"hexes/loot"
	"hexes/skill"
)

// Shapes in the sim. A hex's shape is its control flow (see skill.Shape).
// Everyone has the plain Ring and the plain Triangle. The rest are found:
// rare drops deep down, each copy with one rolled quirk. A found shape is
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
// where the shape wants it, wild sockets on the rest of the ring, and a
// Hub's center taking effects, multipliers, charges and calls (no targets
// or gates: the center pools, it doesn't pick or split).
func ShapeSockets(k skill.ShapeKind) [skill.Slots]skill.SocketKind {
	var s [skill.Slots]skill.SocketKind
	for i := range s {
		s[i] = skill.SocketWild
	}
	s[skill.Center] = skill.SocketEffect
	s[k.TriggerSlot()] = skill.SocketTrigger
	return s
}

// rollShapeDrop maybe drops a shape from a dying foe. It uses its own
// random stream so it never changes which parts drop.
func (s *Sim) rollShapeDrop(id skill.EntityID, at skill.Coord, elite bool) {
	chance := ShapeDropChance(s.Depth)
	if chance <= 0 || len(s.Stash.Shapes) >= MaxShapeDrops {
		return
	}
	if elite {
		chance *= ShapeDropElite
	}
	r := loot.NewRNG(loot.Seed(s.Floor.Seed^hash(uint64(s.Visits))^0x5ba9e, int(id)))
	if r.Float() >= chance {
		return
	}
	s.Stash.Shapes = append(s.Stash.Shapes, ShapeDrop{At: at, Shape: skill.RollShape(r.Next, r.Next())})
}

// placeFirstShape lays a shape next to the stairs down the first time a
// save reaches ShapeMinDepth without having found one, so everyone gets
// to try shapes. It comes back every visit until it's picked up.
func (s *Sim) placeFirstShape() {
	if s.ShapeGiven || s.Depth < ShapeMinDepth {
		return
	}
	r := loot.NewRNG(s.RunSeed ^ 0xf1257)
	at := s.Floor.Start
	for _, d := range skill.HexDirs {
		if n := at.Add(d); s.Floor.At(n).Walkable() {
			at = n
			break
		}
	}
	sh := skill.RollShape(r.Next, r.Next())
	if sh.Kind == skill.ShapeRing || sh.Kind == skill.ShapeTriangle {
		sh.Kind = skill.ShapeHub // the first find shows something new
		if !sh.Valid() {
			sh.Quirk, sh.At = skill.ShapeLean, 0
		}
	}
	s.Stash.Shapes = append(s.Stash.Shapes, ShapeDrop{At: at, Shape: sh})
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
		have = append(append([]skill.Shape(nil), have...), TestShapeSet[:]...)
	}
	for _, sh := range have {
		used := false
		for j, h := range s.Load.Hexes {
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
	if hex < 0 || hex >= MaxSlots {
		return skill.Shape{}, errors.New("no such hex")
	}
	cur := s.Load.Hexes[hex].Shape
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
	if hex < 0 || hex >= MaxSlots {
		return errors.New("no such hex")
	}
	if !sh.Valid() {
		return errors.New("broken shape")
	}
	if sh.ID != 0 && !s.hasShape(sh) {
		return errors.New("you haven't found that shape")
	}
	h := s.Load.Hexes[hex]
	old := *h
	nh := old
	nh.Shape = sh
	nh.Sockets = ShapeSockets(sh.Kind)
	nh.Slots = [skill.Slots]skill.Slot{}
	var extra []skill.Part
	trig := sh.Kind.TriggerSlot()
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
		case sh.Kind.Has(i) && i != trig && nh.Sockets[i].Accepts(p.Kind):
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
			if nh.Slots[i].Kind == skill.SlotEmpty && nh.Sockets[i].Accepts(p.Kind) {
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
		return fmt.Errorf("a %s has room for fewer parts: free %d pack cells first", sh.Kind, len(pack)-free)
	}
	*h = nh
	if err := s.recompile(hex); err != nil {
		*h = old
		return err
	}
	for _, p := range pack {
		inv.Add(p) // came out of a hex, so already safe from death
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

// The test switch. With it on, every shape is unlocked without finding
// it: one test copy of each found shape, each with a different quirk so all
// four show. Test copies are never added to the shapes you've found, and
// turning the switch off puts any hex wearing one back to a Ring.
const testShapeBit = 1 << 63

// TestShapeSet is the copies the test switch unlocks.
var TestShapeSet = [...]skill.Shape{
	{Kind: skill.ShapeHub, Quirk: skill.ShapeTwin, At: 2, ID: testShapeBit | 1},
	{Kind: skill.ShapeFork, Quirk: skill.ShapeSurge, At: 1, ID: testShapeBit | 2},
	{Kind: skill.ShapeLoop, Quirk: skill.ShapeOneWay, At: 2, ID: testShapeBit | 3},
	{Kind: skill.ShapeTriangle, Quirk: skill.ShapeLean, ID: testShapeBit | 4},
}

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
		for i, h := range s.Load.Hexes {
			if h == nil || !IsTestShape(h.Shape) {
				continue
			}
			if err := s.SetShape(i, skill.Shape{Kind: skill.ShapeRing}); err != nil {
				return fmt.Errorf("hex %d: %w", i+1, err)
			}
		}
	}
	s.TestShapes = on
	s.Dirty = true
	return nil
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
	}
	switch sh.Quirk {
	case skill.ShapeTwin:
		t += fmt.Sprintf(" Twin: slot %d fires twice.", sh.At+1)
	case skill.ShapeSurge:
		t += fmt.Sprintf(" Surge: slot %d hits 50%% harder.", sh.At+1)
	case skill.ShapeOneWay:
		t += fmt.Sprintf(" One-way: edge %d-%d only feeds clockwise.", sh.At+1, (int(sh.At)+1)%sh.Kind.RingLen()+1)
	case skill.ShapeLean:
		t += " Lean: 25% less heat."
	}
	return t
}

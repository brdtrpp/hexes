package skill

import "fmt"

// A hex's shape is its control flow. The parts are the same in every
// shape; the shape decides which slots feed which and in what order they
// run:
//
//	Triangle  three slots in a line, trigger first. Runs on less Heat.
//	Ring      six slots clockwise, each feeding the next (the original).
//	Hub       a ring plus a center slot. Every ring slot feeds the center,
//	          which runs last: an effect there lands once on everything
//	          the ring targeted, pooled; a multiplier there scales the
//	          whole ring.
//	Fork      the trigger sits in the center and feeds two chains of
//	          three (slots 0-2 and 3-5) that run one after the other in
//	          the same firing, neither feeding the other.
//	Loop      a ring whose last slot feeds back into the first: after the
//	          ring runs, it runs again once for each entity the last slot
//	          carries (its targets, the ones its effect landed on, a
//	          gate's yes half). Each pass costs Heat like a self Call, so
//	          Heat and MaxCalls cap it; a last slot that carries nobody
//	          ends it.
//
// Shapes other than the plain Ring and Triangle are found deep in the
// dungeon, and every found copy carries one ShapeQuirk on a rolled slot
// or edge, so two players' Hubs differ.

// ShapeKind is a hex's control flow.
type ShapeKind uint8

const (
	ShapeRing ShapeKind = iota
	ShapeTriangle
	ShapeHub
	ShapeFork
	ShapeLoop
	NumShapes
)

var shapeNames = [...]string{"Ring", "Triangle", "Hub", "Fork", "Loop"}

func (k ShapeKind) String() string {
	if k >= NumShapes {
		return fmt.Sprintf("shape%d", k)
	}
	return shapeNames[k]
}

// RingLen is how many slots the shape has around its ring (3 or 6).
func (k ShapeKind) RingLen() int {
	if k == ShapeTriangle {
		return 3
	}
	return RingSlots
}

// HasCenter reports whether the shape uses the center slot.
func (k ShapeKind) HasCenter() bool { return k == ShapeHub || k == ShapeFork }

// Has reports whether slot i is part of the shape.
func (k ShapeKind) Has(i int) bool {
	return (i >= 0 && i < k.RingLen()) || (i == Center && k.HasCenter())
}

// Len is how many slots the shape has.
func (k ShapeKind) Len() int {
	n := k.RingLen()
	if k.HasCenter() {
		n++
	}
	return n
}

// HasEdge reports whether ring edge e (between slot e and the next one
// clockwise) exists. A Triangle is a line, so it has no edge from its last
// slot back to its first; a Fork's two chains have no edge between them.
func (k ShapeKind) HasEdge(e int) bool {
	if e < 0 || e >= k.RingLen() {
		return false
	}
	switch k {
	case ShapeTriangle:
		return e != 2
	case ShapeFork:
		return e != 2 && e != 5
	}
	return true
}

// TriggerSlot is where the shape's trigger goes: the center of a Fork,
// slot 0 of anything else.
func (k ShapeKind) TriggerSlot() int {
	if k == ShapeFork {
		return Center
	}
	return 0
}

// Shape tuning. Placeholder balance, all constants.
var shapeHeat = [NumShapes]float32{
	ShapeRing:     1,
	ShapeTriangle: 0.75, // the cheap starter
	ShapeHub:      1.2,  // pooling is strong
	ShapeFork:     1.1,
	ShapeLoop:     1, // every extra pass pays its own Heat
}

// HeatScale is what the shape multiplies a hex's Heat by.
func (k ShapeKind) HeatScale() float32 {
	if k >= NumShapes {
		return 1
	}
	return shapeHeat[k]
}

// ShapeQuirk is the one rule a found copy of a shape bends, on one slot
// or edge (Shape.At).
type ShapeQuirk uint8

const (
	ShapeQuirkNone ShapeQuirk = iota
	// ShapeTwin: the part in slot At fires twice (and costs twice). A
	// multiplier there applies twice; a target just picks once.
	ShapeTwin
	// ShapeSurge: every number of the part in slot At is SurgeScale
	// times bigger: damage, push, duration, radius and count, or a
	// multiplier's bonus.
	ShapeSurge
	// ShapeOneWay: ring edge At only feeds clockwise. The slot after it
	// still reads the slot before; the slot before no longer sees the
	// targets after it.
	ShapeOneWay
	// ShapeLean: the whole hex runs on LeanHeat of its Heat.
	ShapeLean
	NumShapeQuirks
)

var shapeQuirkNames = [...]string{"", "twin", "surge", "one-way", "lean"}

func (q ShapeQuirk) String() string {
	if q >= NumShapeQuirks {
		return fmt.Sprintf("quirk%d", q)
	}
	return shapeQuirkNames[q]
}

// OnSlot reports whether the quirk sits on a slot (as opposed to an edge
// or the whole hex).
func (q ShapeQuirk) OnSlot() bool { return q == ShapeTwin || q == ShapeSurge }

// Shape quirk tuning.
const (
	SurgeScale = 1.5
	LeanHeat   = 0.75
)

// Shape is a hex's shape: its kind and, for a found copy, its quirk and
// the copy's ID. The plain Ring and Triangle have ID 0 and no quirk.
type Shape struct {
	Kind  ShapeKind  `json:",omitempty"`
	Quirk ShapeQuirk `json:",omitempty"`
	At    uint8      `json:",omitempty"`
	ID    uint64     `json:",omitempty"`
}

// Valid reports whether the shape's kind, quirk and quirk position make
// sense together.
func (s Shape) Valid() bool {
	if s.Kind >= NumShapes || s.Quirk >= NumShapeQuirks {
		return false
	}
	switch {
	case s.Quirk.OnSlot():
		return s.Kind.Has(int(s.At)) && int(s.At) != s.Kind.TriggerSlot()
	case s.Quirk == ShapeOneWay:
		return s.Kind.HasEdge(int(s.At))
	}
	return true
}

// Twin is the slot that fires twice, or -1.
func (s Shape) Twin() int {
	if s.Quirk == ShapeTwin {
		return int(s.At)
	}
	return -1
}

// Surge is the slot whose numbers are boosted, or -1.
func (s Shape) Surge() int {
	if s.Quirk == ShapeSurge {
		return int(s.At)
	}
	return -1
}

// OneWay is the edge that only feeds clockwise, or -1.
func (s Shape) OneWay() int {
	if s.Quirk == ShapeOneWay {
		return int(s.At)
	}
	return -1
}

// String names the shape and its quirk: "Hub", "Loop, twin slot 3",
// "Fork, one-way edge 1-2".
func (s Shape) String() string {
	name := s.Kind.String()
	switch {
	case s.Quirk.OnSlot():
		return fmt.Sprintf("%s, %s slot %d", name, s.Quirk, s.At+1)
	case s.Quirk == ShapeOneWay:
		n := s.Kind.RingLen()
		return fmt.Sprintf("%s, one-way edge %d-%d", name, s.At+1, (int(s.At)+1)%n+1)
	case s.Quirk == ShapeLean:
		return name + ", lean"
	}
	return name
}

// RollShape rolls a found shape: a kind other than the plain Ring by
// weight, one quirk, and where it sits. next is a random source (any
// uniform uint64 generator); id should be unique per copy and nonzero.
func RollShape(next func() uint64, id uint64) Shape {
	kinds := [...]struct {
		k ShapeKind
		w uint64
	}{{ShapeTriangle, 3}, {ShapeHub, 3}, {ShapeFork, 3}, {ShapeLoop, 3}, {ShapeRing, 1}}
	var total uint64
	for _, k := range kinds {
		total += k.w
	}
	r := next() % total
	s := Shape{ID: id | 1}
	for _, k := range kinds {
		if r < k.w {
			s.Kind = k.k
			break
		}
		r -= k.w
	}
	s.Quirk = ShapeQuirk(1 + next()%uint64(NumShapeQuirks-1))
	switch {
	case s.Quirk.OnSlot():
		var free [Slots]uint8
		n := 0
		for i := 0; i < Slots; i++ {
			if s.Kind.Has(i) && i != s.Kind.TriggerSlot() {
				free[n] = uint8(i)
				n++
			}
		}
		s.At = free[next()%uint64(n)]
	case s.Quirk == ShapeOneWay:
		var edges [RingSlots]uint8
		n := 0
		for e := 0; e < s.Kind.RingLen(); e++ {
			if s.Kind.HasEdge(e) {
				edges[n] = uint8(e)
				n++
			}
		}
		s.At = edges[next()%uint64(n)]
	}
	return s
}

// Feeds reports whether what slot a produces reaches slot b: a's targets
// are there for an effect in b to land on, a multiplier in b scales a, a
// gate in a can route into b. Around a ring, neighbors feed each other
// across a linked edge, both ways unless the edge is one-way. A Hub's
// ring slots all feed its center; a Fork's center feeds the head of each
// chain.
func (h *Hex) Feeds(a, b int) bool {
	k := h.Shape.Kind
	if a == b || !k.Has(a) || !k.Has(b) {
		return false
	}
	if a == Center || b == Center {
		switch k {
		case ShapeHub:
			return b == Center
		case ShapeFork:
			return a == Center && (b == 0 || b == 3)
		}
		return false
	}
	n := k.RingLen()
	switch {
	case b == (a+1)%n:
		return k.HasEdge(a) && h.Linked[a]
	case a == (b+1)%n:
		return k.HasEdge(b) && h.Linked[b] && h.Shape.OneWay() != b
	}
	return false
}

// TriggerSlot is where this hex's trigger sits: the slot holding one, or
// where its shape puts one.
func (h *Hex) TriggerSlot() int {
	for i, s := range h.Slots {
		if s.Kind == SlotPart && s.Part.Kind == KindTrigger && h.Shape.Kind.Has(i) {
			return i
		}
	}
	return h.Shape.Kind.TriggerSlot()
}

// Flow is the order the hex runs in from slot start, and how many slots
// that is: around the ring from start, then the center last for a Hub.
// A Fork starts at its center, then runs chain 0-2, then chain 3-5.
func (h *Hex) Flow(start int) (order [Slots]int, n int) {
	k := h.Shape.Kind
	if k == ShapeFork {
		order[0] = Center
		for i := 0; i < RingSlots; i++ {
			order[i+1] = i
		}
		return order, Slots
	}
	r := k.RingLen()
	if start < 0 || start >= r {
		start = 0
	}
	for i := 0; i < r; i++ {
		order[i] = (start + i) % r
	}
	n = r
	if k == ShapeHub {
		order[n] = Center
		n++
	}
	return order, n
}

// ringFlow is Flow without the center slot: the slots gates can route.
func (h *Hex) ringFlow(start int) (order [Slots]int, n int) {
	all, m := h.Flow(start)
	for _, s := range all[:m] {
		if s != Center {
			order[n] = s
			n++
		}
	}
	return order, n
}

// entry is where a hex with no trigger (a nested one) starts.
func (h *Hex) entry() int { return h.Shape.Kind.TriggerSlot() }

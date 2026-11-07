package skill

import (
	"fmt"
	"strings"
)

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
	// ShapeGen is a generated shape: its graph travels with it (Shape.Net).
	ShapeGen
	NumShapes
)

var shapeNames = [...]string{"Ring", "Triangle", "Hub", "Fork", "Loop", "Shape"}

func (k ShapeKind) String() string {
	if k >= NumShapes {
		return fmt.Sprintf("shape%d", k)
	}
	return shapeNames[k]
}

// Legacy reports whether k is one of the five original fixed shapes.
func (k ShapeKind) Legacy() bool { return k < ShapeGen }

// RingLen is how many slots one of the original shapes has around its
// ring (3 or 6).
func (k ShapeKind) RingLen() int {
	if k == ShapeTriangle {
		return 3
	}
	return RingSlots
}

// HasCenter reports whether one of the original shapes uses the center
// slot.
func (k ShapeKind) HasCenter() bool { return k == ShapeHub || k == ShapeFork }

// Has reports whether slot i is part of one of the original shapes.
func (k ShapeKind) Has(i int) bool {
	if k >= ShapeGen {
		return false
	}
	return legacyGraphs[k].Has(i)
}

// Len is how many slots one of the original shapes has.
func (k ShapeKind) Len() int {
	if k >= ShapeGen {
		return 0
	}
	return int(legacyGraphs[k].N)
}

// HasEdge reports whether ring edge e (between slot e and the next one
// clockwise) exists in one of the original shapes. A Triangle is a line,
// so it has no edge from its last slot back to its first; a Fork's two
// chains have no edge between them.
func (k ShapeKind) HasEdge(e int) bool {
	if e < 0 || e >= k.RingLen() || k >= ShapeGen {
		return false
	}
	return legacyGraphs[k].Out[e]&bit((e+1)%k.RingLen()) != 0
}

// TriggerSlot is where one of the original shapes puts its trigger: the
// center of a Fork, slot 0 of anything else (including generated ones).
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
	ShapeGen:      1,
}

// HeatScale is what one of the original shapes multiplies a hex's Heat
// by. A generated shape's comes from its structure (Shape.HeatScale).
func (k ShapeKind) HeatScale() float32 {
	if k >= NumShapes {
		return 1
	}
	return shapeHeat[k]
}

// Structural Heat for generated shapes: a base, a share per slot, and
// extra for every split and merge, so a six-slot chain costs what a Ring
// does and bigger, busier graphs cost more. A loop costs nothing up front
// because every extra pass pays its own Heat.
const (
	GenHeatBase  = 0.55
	GenHeatSlot  = 0.075
	GenHeatSplit = 0.05
	GenHeatMerge = 0.1
)

// ShapeQuirk is the one rule a found copy of a shape bends, on one slot
// or link (Shape.At).
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
	// ShapeOneWay: the two-way link out of slot At only feeds forward.
	// The slot after it still reads slot At; slot At no longer sees the
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

// OnSlot reports whether the quirk sits on a slot (as opposed to a link
// or the whole hex).
func (q ShapeQuirk) OnSlot() bool { return q == ShapeTwin || q == ShapeSurge }

// Shape quirk tuning.
const (
	SurgeScale = 1.5
	LeanHeat   = 0.75
)

// Shape is a hex's shape: its kind, its graph if it was generated, and,
// for a found copy, its quirk and the copy's ID. The plain Ring and
// Triangle have ID 0 and no quirk.
type Shape struct {
	Kind  ShapeKind  `json:",omitempty"`
	Quirk ShapeQuirk `json:",omitempty"`
	At    uint8      `json:",omitempty"`
	ID    uint64     `json:",omitempty"`
	Net   Graph      `json:",omitzero"`
}

// Graph is the shape's graph: the fixed one for an original shape, its
// own for a generated one.
func (s *Shape) Graph() *Graph {
	if s.Kind < ShapeGen {
		return &legacyGraphs[s.Kind]
	}
	return &s.Net
}

// Has reports whether slot i is part of the shape.
func (s *Shape) Has(i int) bool { return s.Graph().Has(i) }

// Len is how many slots the shape has.
func (s *Shape) Len() int { return int(s.Graph().N) }

// Entry is where the shape puts its trigger.
func (s *Shape) Entry() int { return int(s.Graph().Entry) }

// HeatScale is what the shape multiplies a hex's Heat by.
func (s *Shape) HeatScale() float32 {
	if s.Kind != ShapeGen {
		return s.Kind.HeatScale()
	}
	g := &s.Net
	return GenHeatBase + GenHeatSlot*float32(g.N) + GenHeatSplit*float32(g.Splits()) + GenHeatMerge*float32(g.Merges())
}

// Valid reports whether the shape's kind, graph, quirk and quirk position
// make sense together.
func (s Shape) Valid() bool {
	if s.Kind >= NumShapes || s.Quirk >= NumShapeQuirks {
		return false
	}
	g := s.Graph()
	switch {
	case s.Kind == ShapeGen && !g.Valid():
		return false
	case s.Kind != ShapeGen && s.Net != (Graph{}):
		return false
	}
	switch {
	case s.Quirk.OnSlot():
		return g.Has(int(s.At)) && int(s.At) != int(g.Entry)
	case s.Quirk == ShapeOneWay:
		return g.FirstTwo(int(s.At)) >= 0
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

// OneWay is the slot whose two-way link only feeds forward, or -1. For
// the original ring shapes that's the ring edge from that slot.
func (s Shape) OneWay() int {
	if s.Quirk == ShapeOneWay {
		return int(s.At)
	}
	return -1
}

// Name is what the shape is called: an original shape's name, or a
// generated one's from its features, like "Forked Hub" or "Long Looping
// Chain". Chains run straight through, Forks split, Hubs merge.
func (s Shape) Name() string {
	if s.Kind != ShapeGen {
		return s.Kind.String()
	}
	g := &s.Net
	sp, mg := g.Splits(), g.Merges()
	noun := "Chain"
	switch {
	case mg > 0:
		noun = "Hub"
	case sp > 0:
		noun = "Fork"
	}
	var size, style string
	switch {
	case g.N <= 3:
		size = "Short"
	case g.N >= 8 && noun == "Chain":
		size = "Long"
	case g.N >= 8:
		size = "Grand"
	}
	switch {
	case noun == "Hub" && mg >= 2:
		style = "Knotted"
	case noun == "Hub" && sp >= 3:
		style = "Forked" // every merge needs a split before it
	case noun == "Fork" && sp >= 2:
		style = "Branching"
	}
	var words []string
	for _, w := range []string{size, map[bool]string{true: "Looping"}[g.Loop > 0], style} {
		if w != "" {
			words = append(words, w)
		}
	}
	if len(words) > 2 {
		words = words[1:] // three words at most
	}
	return strings.Join(append(words, noun), " ")
}

// Family is the original shape a generated one is closest to: Triangle
// or Ring for a chain (by length), Fork, Hub, or Loop when it loops.
func (s Shape) Family() ShapeKind {
	if s.Kind != ShapeGen {
		return s.Kind
	}
	g := &s.Net
	switch {
	case g.Loop > 0:
		return ShapeLoop
	case g.Merges() > 0:
		return ShapeHub
	case g.Splits() > 0:
		return ShapeFork
	case g.N <= 4:
		return ShapeTriangle
	}
	return ShapeRing
}

// String names the shape and its quirk: "Hub", "Loop, twin slot 3",
// "Fork, one-way edge 1-2".
func (s Shape) String() string {
	name := s.Name()
	switch {
	case s.Quirk.OnSlot():
		return fmt.Sprintf("%s, %s slot %d", name, s.Quirk, s.At+1)
	case s.Quirk == ShapeOneWay:
		to := s.Graph().FirstTwo(int(s.At))
		return fmt.Sprintf("%s, one-way edge %d-%d", name, s.At+1, to+1)
	case s.Quirk == ShapeLean:
		return name + ", lean"
	}
	return name
}

// RollShape rolls a found shape for a depth: a generated graph (bigger
// and stranger the deeper it drops, see GenGraph), one quirk, and where
// it sits. next is a random source (any uniform uint64 generator); id
// should be unique per copy and nonzero.
func RollShape(next func() uint64, id uint64, depth int) Shape {
	s := Shape{Kind: ShapeGen, ID: id | 1, Net: GenGraph(next, depth)}
	rollQuirk(&s, next)
	return s
}

func rollQuirk(s *Shape, next func() uint64) {
	g := s.Graph()
	s.Quirk = ShapeQuirk(1 + next()%uint64(NumShapeQuirks-1))
	var at [Slots]uint8
	n := 0
	for i := 0; i < int(g.N); i++ {
		switch {
		case s.Quirk.OnSlot() && i != int(g.Entry):
		case s.Quirk == ShapeOneWay && g.FirstTwo(i) >= 0:
		default:
			continue
		}
		at[n] = uint8(i)
		n++
	}
	switch {
	case n > 0:
		s.At = at[next()%uint64(n)]
	case s.Quirk != ShapeLean:
		s.Quirk, s.At = ShapeLean, 0 // nowhere for it to sit
	}
}

// Feeds reports whether what slot a produces reaches slot b: a's targets
// are there for an effect in b to land on, a multiplier in b scales a, a
// gate in a can route into b. That's a link from a to b, or a two-way
// link from b to a. On the original ring shapes a cut ring edge (Linked)
// feeds neither way, and a one-way quirk stops the backward half.
func (h *Hex) Feeds(a, b int) bool {
	if h.Laid() {
		g := h.Graph()
		return a != b && a >= 0 && b >= 0 && a < Slots && b < Slots && g.Out[a]&bit(b) != 0
	}
	g := h.Shape.Graph()
	if a == b || !g.Has(a) || !g.Has(b) {
		return false
	}
	if g.Out[a]&bit(b) != 0 {
		return h.linkOn(a, b)
	}
	if g.Two[b]&bit(a) != 0 {
		return h.linkOn(b, a) && !(h.Shape.OneWay() == b && g.FirstTwo(b) == a)
	}
	return false
}

// linkOn reports whether the link from a to b is there: always, except a
// ring edge of an original shape that Learn cut.
func (h *Hex) linkOn(a, b int) bool {
	if h.Shape.Kind.Legacy() && a < RingSlots && b < RingSlots {
		return h.Linked[a]
	}
	return true
}

// FedBy is the slots that feed s, in flow order.
func (h *Hex) FedBy(s int, order []int) (in [Slots]int, n int) {
	for _, a := range order {
		if h.Feeds(a, s) {
			in[n] = a
			n++
		}
	}
	return in, n
}

// inFeed is the slot whose output s takes as "the slot before it": the
// latest slot before s in flow order with a link into s, or -1.
func (h *Hex) inFeed(s int, order []int) int {
	g := h.Graph()
	best := -1
	for _, a := range order {
		if a == s {
			break
		}
		if g.Out[a]&bit(s) != 0 && h.Feeds(a, s) {
			best = a
		}
	}
	return best
}

// merge reports whether s takes links from two or more slots: a Hub's
// center, or any generated merge. A merge pools what feeds it and is
// never one of a gate's branches.
func (h *Hex) merge(s int) bool { return h.Graph().Ins(s) >= 2 }

// loopSlot is the slot whose carry runs the hex again, or -1: the last
// slot in flow of an original Loop if it links back to the first, or a
// generated shape's loop-back.
func (h *Hex) loopSlot(start int) int {
	if h.Laid() && h.Lay.Touch {
		// The first LOOP pad whose piece runs carries the loop.
		for _, p := range h.Lay.Pads {
			if p.Kind != PadLoop {
				continue
			}
			if s := h.PieceAt(p.At); s >= 0 && s != start && h.Runs(s) && h.padOn(s, PadLoop) {
				return s
			}
		}
		return -1
	}
	if h.Laid() {
		// The piece on the shape's loop pad carries the loop.
		s := h.pad(h.Shape.Graph().LoopFrom())
		if s < 0 || s == start || !h.Runs(s) {
			return -1
		}
		return s
	}
	switch h.Shape.Kind {
	case ShapeLoop:
		order, n := h.Flow(start)
		if n > 1 && h.Feeds(order[n-1], order[0]) {
			return order[n-1]
		}
		return -1
	case ShapeGen:
		return h.Shape.Net.LoopFrom()
	}
	return -1
}

// TriggerSlot is where this hex's trigger sits: the slot holding one, or
// where its shape puts one.
func (h *Hex) TriggerSlot() int {
	for i, s := range h.Slots {
		if s.Kind == SlotPart && s.Part.Kind == KindTrigger && h.HasSlot(i) && (!h.Laid() || h.Lay.Pos[i].On) {
			return i
		}
	}
	return h.entry()
}

// Flow is the order the hex runs in from slot start, and how many slots
// that is (see Graph.Flow): around a ring from start, then a Hub's center
// last; a Fork's center, then chain 0-2, then chain 3-5.
func (h *Hex) Flow(start int) (order [Slots]int, n int) {
	if h.Laid() {
		// Only the pieces that run, in the order the plugs make.
		w := h.wired()
		all, m := w.g.Flow(start)
		for _, s := range all[:m] {
			if w.runs&bit(s) != 0 {
				order[n] = s
				n++
			}
		}
		return order, n
	}
	return h.Shape.Graph().Flow(start)
}

// entry is where a hex with no trigger (a nested one) starts.
func (h *Hex) entry() int {
	if h.Laid() {
		return int(h.wired().g.Entry)
	}
	return h.Shape.Entry()
}

// SlotName is a slot as the editor numbers it: from 1, or C for an
// original Hub's or Fork's center.
func (h *Hex) SlotName(s int) string {
	if s == Center && h.Shape.Kind.HasCenter() {
		return "C"
	}
	return fmt.Sprint(s + 1)
}

// LoopSlot is the slot whose carry runs the hex again, or -1.
func (h *Hex) LoopSlot() int { return h.loopSlot(h.TriggerSlot()) }

// Merge reports whether slot s pools two or more links (see merge).
func (h *Hex) Merge(s int) bool { return h.merge(s) }

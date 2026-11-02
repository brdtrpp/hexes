package skill

import "math/bits"

// Every shape is a small graph laid out on a hex grid: a few slots, the
// links between neighbors that say which slot feeds which, where the
// trigger sits, and at most one loop-back. The five original shapes are
// fixed graphs (see legacyGraphs); found shapes are generated (see
// GenGraph), so no two need be alike.
//
// Links point the way the hex runs. A link from a to b means a's targets
// are there for an effect in b, a multiplier in b scales a, and a gate in
// a can route into b. A two-way link also lets b's targets reach an effect
// in a, the way a ring's neighbors read both sides. Links into a merge or
// out of a split are one-way.

// Cell is a slot's place on the grid a shape is drawn on, in axial
// coordinates. The layout is flat-top: (0,-1) is straight up and the six
// neighbors go clockwise from there, the same order as a ring's slots.
type Cell struct{ Q, R int8 }

// CellDirs are the six neighbor offsets, clockwise from straight up.
var CellDirs = [6]Cell{{0, -1}, {1, -1}, {1, 0}, {0, 1}, {-1, 1}, {-1, 0}}

// Add is c moved by d.
func (c Cell) Add(d Cell) Cell { return Cell{c.Q + d.Q, c.R + d.R} }

// Ring is how many steps c is from the origin.
func (c Cell) Ring() int {
	q, r := int(c.Q), int(c.R)
	return (abs(q) + abs(r) + abs(q+r)) / 2
}

// Next reports whether a and b are neighbors.
func (a Cell) Next(b Cell) bool {
	d := Cell{b.Q - a.Q, b.R - a.R}
	for _, n := range CellDirs {
		if d == n {
			return true
		}
	}
	return false
}

// XY is the cell's center in units of the distance between neighbors,
// flat-top, y down.
func (c Cell) XY() (x, y float64) {
	return 0.8660254037844386 * float64(c.Q), float64(c.R) + float64(c.Q)/2
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// Graph is a shape's slots and how they feed each other.
type Graph struct {
	N  uint8       // slots 0..N-1
	At [Slots]Cell // where each slot sits
	// Out[a] has bit b set when a feeds b. Two[a] is the part of Out[a]
	// that also feeds back from b to a.
	Out [Slots]uint16
	Two [Slots]uint16
	// Entry is where the trigger goes.
	Entry uint8
	// Loop is 1 + the slot whose carry runs the hex again (a loop-back
	// to the trigger), or 0 for none.
	Loop uint8
}

func bit(i int) uint16 { return 1 << uint(i) }

// Has reports whether slot i is part of the graph.
func (g *Graph) Has(i int) bool { return i >= 0 && i < int(g.N) }

// Links reports whether a feeds b along a link, ignoring direction.
func (g *Graph) Links(a, b int) bool {
	return g.Has(a) && g.Has(b) && (g.Out[a]&bit(b) != 0 || g.Out[b]&bit(a) != 0)
}

// Ins is how many slots link into b.
func (g *Graph) Ins(b int) int {
	n := 0
	for a := 0; a < int(g.N); a++ {
		if g.Out[a]&bit(b) != 0 {
			n++
		}
	}
	return n
}

// Outs is how many slots a links into.
func (g *Graph) Outs(a int) int { return bits.OnesCount16(g.Out[a]) }

// FirstTwo is the first slot a two-way link from a goes to, or -1.
func (g *Graph) FirstTwo(a int) int {
	if !g.Has(a) || g.Two[a] == 0 {
		return -1
	}
	return bits.TrailingZeros16(g.Two[a])
}

// Splits, Merges are how many slots link out to two or more, or take
// links from two or more.
func (g *Graph) Splits() int {
	n := 0
	for a := 0; a < int(g.N); a++ {
		if g.Outs(a) >= 2 {
			n++
		}
	}
	return n
}

func (g *Graph) Merges() int {
	n := 0
	for b := 0; b < int(g.N); b++ {
		if g.Ins(b) >= 2 {
			n++
		}
	}
	return n
}

// LoopFrom is the slot that loops back, or -1.
func (g *Graph) LoopFrom() int { return int(g.Loop) - 1 }

// Flow is the order the hex runs in from slot start: each slot after
// every slot linking into it, following one chain to its end before the
// next. Links into start are ignored (a ring starts wherever its trigger
// is). Slots nothing reaches come last, in number order.
func (g *Graph) Flow(start int) (order [Slots]int, n int) {
	N := int(g.N)
	if start < 0 || start >= N {
		start = int(g.Entry)
	}
	var need [Slots]int
	for a := 0; a < N; a++ {
		for m := g.Out[a] &^ bit(start); m != 0; m &= m - 1 {
			need[bits.TrailingZeros16(m)]++
		}
	}
	var done [Slots]bool
	var stack [Slots + 1]int
	sp := 0
	stack[sp] = start
	sp++
	for n < N {
		if sp == 0 {
			// Stalled: the next unreached slot, preferring one with
			// nothing left waiting on it.
			pick := -1
			for i := 0; i < N && pick < 0; i++ {
				if !done[i] && need[i] == 0 {
					pick = i
				}
			}
			for i := 0; i < N && pick < 0; i++ {
				if !done[i] {
					pick = i
				}
			}
			stack[sp] = pick
			sp++
		}
		sp--
		s := stack[sp]
		if done[s] {
			continue
		}
		done[s] = true
		order[n] = s
		n++
		// Children whose last link just arrived, highest first so the
		// lowest runs next.
		for b := N - 1; b >= 0; b-- {
			if g.Out[s]&bit(b) == 0 || b == start || done[b] {
				continue
			}
			need[b]--
			if need[b] == 0 && sp < len(stack) {
				stack[sp] = b
				sp++
			}
		}
	}
	return order, n
}

// GenRadius bounds where a generated shape's cells may sit, so a single
// hex stays readable on a phone: within two steps of the trigger and no
// more than a row and a half above or below it.
const GenRadius = 2

func genCellOK(c Cell) bool {
	if c.Ring() > GenRadius {
		return false
	}
	_, y := c.XY()
	return y >= -1.5 && y <= 1.5
}

// Valid reports whether a generated graph is well formed: 2 to Slots
// slots on distinct cells inside the bounds, links only between
// neighbors, every slot reachable from the entry, no cycle except the one
// loop-back, two-way links only where one is allowed, and the loop-back
// (if any) from a slot with no links out.
func (g *Graph) Valid() bool {
	N := int(g.N)
	if N < 2 || N > Slots || int(g.Entry) >= N || int(g.Loop) > N {
		return false
	}
	for i := 0; i < Slots; i++ {
		if i >= N {
			if g.Out[i] != 0 || g.Two[i] != 0 || g.At[i] != (Cell{}) {
				return false
			}
			continue
		}
		if !genCellOK(g.At[i]) || g.Out[i]>>uint(N) != 0 || g.Two[i]&^g.Out[i] != 0 || g.Out[i]&bit(i) != 0 {
			return false
		}
		for j := 0; j < i; j++ {
			if g.At[j] == g.At[i] {
				return false
			}
		}
		for m := g.Out[i]; m != 0; m &= m - 1 {
			b := bits.TrailingZeros16(m)
			if !g.At[i].Next(g.At[b]) || g.Out[b]&bit(i) != 0 || b == int(g.Entry) {
				return false
			}
			if g.Two[i]&bit(b) != 0 && (g.Outs(i) != 1 || g.Ins(b) != 1) {
				return false
			}
		}
	}
	// Reachable and acyclic: Flow places every slot after all its
	// inputs only if there is no cycle.
	order, n := g.Flow(int(g.Entry))
	var pos [Slots]int
	for k, s := range order[:n] {
		pos[s] = k
	}
	for a := 0; a < N; a++ {
		for m := g.Out[a]; m != 0; m &= m - 1 {
			if pos[bits.TrailingZeros16(m)] <= pos[a] {
				return false
			}
		}
		if a != int(g.Entry) && g.Ins(a) == 0 {
			return false
		}
	}
	if l := g.LoopFrom(); l >= 0 && (g.Out[l] != 0 || l == int(g.Entry)) {
		return false
	}
	return true
}

// The five original shapes as graphs. A ring's slot i sits in direction
// i from the middle, so the editor draws them where it always has; a
// Hub's or Fork's center slot is the middle.
var legacyGraphs [NumShapes]Graph

func init() {
	ring := func(center bool) Graph {
		var g Graph
		for i := 0; i < RingSlots; i++ {
			g.At[i] = CellDirs[i]
			g.Out[i] = bit((i + 1) % RingSlots)
		}
		g.N = RingSlots
		if center {
			g.N = RingSlots + 1 // the center slot sits in the middle
		}
		return g
	}
	r := ring(false)
	for i := 0; i < RingSlots; i++ {
		r.Two[i] = r.Out[i]
	}
	legacyGraphs[ShapeRing] = r
	legacyGraphs[ShapeLoop] = r
	legacyGraphs[ShapeLoop].Loop = RingSlots // the last ring slot, see Hex.loopSlot

	tri := Graph{N: 3}
	tri.At[0], tri.At[1], tri.At[2] = Cell{0, 0}, Cell{1, 0}, Cell{0, 1}
	tri.Out[0], tri.Out[1] = bit(1), bit(2)
	tri.Two = tri.Out
	legacyGraphs[ShapeTriangle] = tri

	hub := ring(true)
	for i := 0; i < RingSlots; i++ {
		hub.Two[i] = hub.Out[i]
		hub.Out[i] |= bit(Center)
	}
	legacyGraphs[ShapeHub] = hub

	fork := ring(true)
	fork.Out[2], fork.Out[5] = 0, 0
	fork.Two = fork.Out
	fork.Out[Center] = bit(0) | bit(3)
	fork.Entry = Center
	legacyGraphs[ShapeFork] = fork
}

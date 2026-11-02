package skill

import "math/bits"

// GenGraph generates a shape's graph for a depth. It grows a tree out from
// the trigger cell, mostly extending the newest chain and sometimes
// splitting off an earlier slot, then may wire merges (a slot fed from two
// sides) and one loop-back. Deeper drops roll bigger graphs with more
// splits, merges and loops. Slots are numbered in the order they run, so
// slot 1 is always the trigger.
//
// Every graph it returns passes Graph.Valid: connected, every slot
// reachable from the trigger, no cycle but the loop-back, and inside the
// bounds that keep a hex readable on a phone.
func GenGraph(next func() uint64, depth int) Graph {
	for try := 0; try < 64; try++ {
		if g, ok := genTry(next, depth); ok {
			return g
		}
	}
	// Never seen, but a short chain is always valid.
	g := Graph{N: 3}
	g.At[1], g.At[2] = Cell{0, 1}, Cell{1, 0}
	g.Out[0], g.Out[1] = bit(1), bit(2)
	g.Two = g.Out
	return g
}

// Generator tuning. Placeholder balance, all constants. "deep" is how far
// below GenDepth a drop is.
const (
	GenDepth       = 5    // where shapes start to drop
	GenMinSlots    = 3    // smallest generated shape
	GenBaseSlots   = 5    // largest at GenDepth
	GenSlotEvery   = 6    // one more possible slot per this many depths
	GenSplitBase   = 0.2  // chance a new slot branches off an earlier one
	GenSplitStep   = 0.01 // more per depth
	GenSplitMax    = 0.5
	GenMergeBase   = 0.15 // chance of wiring a merge
	GenMergeStep   = 0.015
	GenMergeMax    = 0.6
	GenMergesEvery = 15 // one more possible merge per this many depths
	GenLoopBase    = 0.12
	GenLoopStep    = 0.008
	GenLoopMax     = 0.35
)

func unit(next func() uint64) float64 { return float64(next()>>11) / (1 << 53) }

// GenMaxSlots is the most slots a shape dropping at depth can have.
func GenMaxSlots(depth int) int {
	return min(Slots, GenBaseSlots+max(0, depth-GenDepth)/GenSlotEvery)
}

func genTry(next func() uint64, depth int) (Graph, bool) {
	deep := float64(max(0, depth-GenDepth))
	hi := GenMaxSlots(depth)
	span := uint64(hi - GenMinSlots + 1)
	n := GenMinSlots + int(max(next()%span, next()%span)) // leans big
	pSplit := min(GenSplitMax, GenSplitBase+GenSplitStep*deep)
	pMerge := min(GenMergeMax, GenMergeBase+GenMergeStep*deep)
	pLoop := min(GenLoopMax, GenLoopBase+GenLoopStep*deep)

	var g Graph
	g.N = 1
	used := func(c Cell) bool {
		for i := 0; i < int(g.N); i++ {
			if g.At[i] == c {
				return true
			}
		}
		return false
	}
	// Free neighbor cells of slot p, weighted toward cells that touch
	// more of the shape so it stays compact.
	grow := func(p int) bool {
		var cand [6]Cell
		var w [6]int
		k, total := 0, 0
		for _, d := range CellDirs {
			c := g.At[p].Add(d)
			if !genCellOK(c) || used(c) {
				continue
			}
			touch := 0
			for i := 0; i < int(g.N); i++ {
				if g.At[i].Next(c) {
					touch++
				}
			}
			cand[k], w[k] = c, 1+touch
			total += w[k]
			k++
		}
		if k == 0 {
			return false
		}
		r := int(next() % uint64(total))
		pick := 0
		for pick < k-1 && r >= w[pick] {
			r -= w[pick]
			pick++
		}
		i := int(g.N)
		g.At[i] = cand[pick]
		g.Out[p] |= bit(i)
		g.N++
		return true
	}
	for int(g.N) < n {
		last := int(g.N) - 1
		p := last
		if g.N >= 2 && unit(next) < pSplit {
			p = int(next() % uint64(last)) // an earlier slot: a split
		}
		if g.Outs(p) < 2 && grow(p) {
			continue
		}
		// That slot is boxed in: try the others from a random start.
		ok := false
		off := int(next() % uint64(g.N))
		for j := 0; j < int(g.N) && !ok; j++ {
			q := (off + j) % int(g.N)
			ok = g.Outs(q) < 2 && grow(q)
		}
		if !ok {
			return g, false
		}
	}

	// Merges: a link between two neighbors that aren't linked yet, into
	// a slot that can't reach back to where the link starts.
	merges := 1 + int(deep)/GenMergesEvery
	for m := 0; m < merges && unit(next) < pMerge; m++ {
		type pair struct{ a, b int }
		var cand [Slots * 6]pair
		k := 0
		for a := 0; a < int(g.N); a++ {
			if g.Outs(a) >= 2 {
				continue
			}
			for b := 1; b < int(g.N); b++ {
				if a == b || g.Links(a, b) || !g.At[a].Next(g.At[b]) || g.Ins(b) >= 3 || reaches(&g, b, a) {
					continue
				}
				cand[k] = pair{a, b}
				k++
			}
		}
		if k == 0 {
			break
		}
		pk := cand[next()%uint64(k)]
		g.Out[pk.a] |= bit(pk.b)
	}

	g = renumber(g)
	if unit(next) < pLoop {
		// Loop back from the slot that runs last, if it ends a chain.
		order, cnt := g.Flow(0)
		if last := order[cnt-1]; g.Out[last] == 0 && last != 0 {
			g.Loop = uint8(last + 1)
		}
	}
	for a := 0; a < int(g.N); a++ {
		if g.Outs(a) != 1 {
			continue
		}
		if b := bits.TrailingZeros16(g.Out[a]); g.Ins(b) == 1 {
			g.Two[a] = g.Out[a]
		}
	}
	return g, g.Valid()
}

// reaches reports whether slot a leads to slot b along links.
func reaches(g *Graph, a, b int) bool {
	var seen uint16
	stack := []int{a}
	for len(stack) > 0 {
		s := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if s == b {
			return true
		}
		if seen&bit(s) != 0 {
			continue
		}
		seen |= bit(s)
		for m := g.Out[s]; m != 0; m &= m - 1 {
			stack = append(stack, bits.TrailingZeros16(m))
		}
	}
	return false
}

// renumber puts a graph's slots in flow order from slot 0, so the numbers
// the editor shows follow the way the hex runs. Links, cells and the loop
// move with their slots.
func renumber(g Graph) Graph {
	order, n := g.Flow(0)
	var to [Slots]int
	for k, s := range order[:n] {
		to[s] = k
	}
	out := Graph{N: g.N}
	for s := 0; s < int(g.N); s++ {
		t := to[s]
		out.At[t] = g.At[s]
		for m := g.Out[s]; m != 0; m &= m - 1 {
			out.Out[t] |= bit(to[bits.TrailingZeros16(m)])
		}
		for m := g.Two[s]; m != 0; m &= m - 1 {
			out.Two[t] |= bit(to[bits.TrailingZeros16(m)])
		}
	}
	if g.Loop > 0 {
		out.Loop = uint8(to[g.Loop-1] + 1)
	}
	return out
}

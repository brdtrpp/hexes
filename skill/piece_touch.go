package skill

// Touching. On the open board each piece keeps its silhouette, a few
// hexagons that never turn, and pieces only join where they touch: a
// wire runs between two pieces only while a cell of one sits next to a
// cell of the other. Laying a piece joins it to everything it touches
// that it can join (the sender the one nearer the trigger); moving it
// away lets those go. A gate's YES and NO and an overload's rungs still
// step with a tap on the neighbour.

// cellsAt are the cells piece s would cover anchored on at.
func (h *Hex) cellsAt(s int, at Cell) []Cell {
	pc := PieceOf(h.Slots[s])
	if pc == nil || len(pc.Cells) == 0 {
		return []Cell{at}
	}
	out := make([]Cell, len(pc.Cells))
	for i, c := range pc.Cells {
		out[i] = at.Add(c)
	}
	return out
}

// fitsOn reports whether piece s fits anchored on at against board b:
// every cell on the room and free, or its own.
func (h *Hex) fitsOn(b *board, s int, at Cell) bool {
	for _, c := range h.cellsAt(s, at) {
		if !h.Lay.Room.Has(c) {
			return false
		}
		if o := b.at(c); o >= 0 && o != s {
			return false
		}
	}
	return true
}

// FitsAt reports whether piece s fits anchored on at.
func (h *Hex) FitsAt(s int, at Cell) bool {
	var b board
	h.fill(&b)
	return h.fitsOn(&b, s, at)
}

// touchOn is how many of the pieces in want piece s would touch anchored
// on at, against board b.
func (h *Hex) touchOn(b *board, s int, at Cell, want uint16) int {
	var got uint16
	for _, c := range h.cellsAt(s, at) {
		for _, d := range CellDirs {
			if o := b.at(c.Add(d)); o >= 0 && o != s && want&bit(o) != 0 {
				got |= bit(o)
			}
		}
	}
	return countBits(got)
}

// Touch reports whether pieces a and b, both laid, touch.
func (h *Hex) Touch(a, b int) bool {
	if a == b || !h.Lay.Pos[a].On || !h.Lay.Pos[b].On {
		return false
	}
	var bd board
	h.fill(&bd)
	return h.touchOn(&bd, a, h.Lay.Pos[a].At, bit(b)) > 0
}

// fitNear is the anchor where piece s fits nearest near, touching as
// many of want as it can (laid pieces only).
func (h *Hex) fitNear(s int, near Cell, want uint16) (Cell, bool) {
	// Pieces laid for the player keep off the pads, so a pad's quirk is
	// only ever had on purpose; with no room elsewhere, anywhere does.
	if pads := h.padCells(); len(pads) > 0 {
		if c, ok := h.fitWith(s, near, want, pads, nil); ok {
			return c, true
		}
	}
	return h.fitWith(s, near, want, nil, nil)
}

// padCells are the board's pad tiles.
func (h *Hex) padCells() map[Cell]bool {
	var out map[Cell]bool
	for _, p := range h.BoardPads() {
		if out == nil {
			out = map[Cell]bool{}
		}
		out[p.At] = true
	}
	return out
}

// fitWith is fitNear keeping off the cells in keepOff, and, with must,
// covering that cell.
func (h *Hex) fitWith(s int, near Cell, want uint16, keepOff map[Cell]bool, must *Cell) (Cell, bool) {
	var b board
	h.fill(&b)
	best, bestScore := Cell{}, 1<<30
	cands := h.Lay.Room.Cells()
	if must != nil {
		cands = cands[:0:0]
		pc := PieceOf(h.Slots[s])
		if pc == nil || len(pc.Cells) == 0 {
			cands = append(cands, *must)
		} else {
			for _, c := range pc.Cells {
				cands = append(cands, must.Sub(c))
			}
		}
	}
	for _, c := range cands {
		if !h.fitsOn(&b, s, c) {
			continue
		}
		off := false
		for _, x := range h.cellsAt(s, c) {
			off = off || keepOff[x] && (must == nil || x != *must)
		}
		if off {
			continue
		}
		score := c.Sub(near).Ring()*8 + c.Ring()
		if want != 0 {
			score -= h.touchOn(&b, s, c, want) * 100000
		}
		if score < bestScore {
			best, bestScore = c, score
		}
	}
	return best, bestScore < 1<<30
}

// FitNear is the anchor nearest near where piece s fits, if any.
func (h *Hex) FitNear(s int, near Cell) (Cell, bool) { return h.fitNear(s, near, 0) }

// WireWhyTouch is WireWhy on the open board: the two have to touch.
func (h *Hex) WireWhyTouch(a, b int) (bool, string) {
	if a == b || a < 0 || b < 0 || a >= Slots || b >= Slots {
		return false, ""
	}
	if h.Lay.Wired && h.Lay.Pos[a].On && h.Lay.Pos[b].On && !h.Touch(a, b) {
		return false, "they have to touch: move one next to the other"
	}
	return h.WireWhy(a, b)
}

// CanWireTouch is CanWire for pieces that touch.
func (h *Hex) CanWireTouch(a, b int) (from, to int, ok bool) {
	if ok, _ := h.WireWhyTouch(a, b); ok {
		return a, b, true
	}
	if ok, _ := h.WireWhyTouch(b, a); ok {
		return b, a, true
	}
	return -1, -1, false
}

// reached is the pieces the trigger reaches along the wires now.
func (h *Hex) reached() uint16 {
	t := h.TriggerSlot()
	if t < 0 || t >= Slots || !h.Lay.Pos[t].On {
		return 0
	}
	var seen uint16
	var walk func(x int)
	walk = func(x int) {
		if seen&bit(x) != 0 {
			return
		}
		seen |= bit(x)
		for y := 0; y < Slots; y++ {
			if h.graphEdge(x, y) {
				walk(y)
			}
		}
	}
	walk(t)
	return seen
}

// rank is where a piece usually sits in a run: the trigger first, then
// what picks and shapes the group, then gates, calls and effects.
func (h *Hex) rank(s int) int {
	sl := h.Slots[s]
	if sl.Kind != SlotPart {
		return 3
	}
	switch sl.Part.Kind {
	case KindTrigger:
		return 0
	case KindTarget, KindForm:
		return 1
	case KindGate:
		return 2
	case KindCall:
		return 3
	case KindEffect:
		return 4
	}
	return 5
}

// feeder is the piece that sends piece s its spark or group now, or -1.
func (h *Hex) feeder(s int) int {
	for a := 0; a < Slots; a++ {
		if a != s && h.Lay.Out[a]&bit(s) != 0 && h.Sends(a) != PlugBoost {
			return a
		}
	}
	return -1
}

// pref is how much piece a should be the one that feeds b: one the
// trigger reaches, then one that comes before b in a run, then the
// latest such.
func (h *Hex) pref(a, b int, reach uint16) int {
	p := h.rank(a)
	if reach&bit(a) != 0 {
		p += 100
	}
	if h.rank(a) < h.rank(b) {
		p += 50
	}
	return p
}

// Join lets go of piece s's wires to pieces it no longer touches and
// joins it to what it touches: it takes its spark or group from the
// best neighbour that can send it one (the trigger's side, then the
// piece latest before it in a run), hands its own on to neighbours that
// come after it, and a multiplier boosts what it touches that has a
// socket for it.
func (h *Hex) Join(s int) {
	if !h.Lay.Wired || s < 0 || s >= Slots {
		return
	}
	for t := 0; t < Slots; t++ {
		if t != s && (h.Lay.Out[s]&bit(t) != 0 || h.Lay.Out[t]&bit(s) != 0) && !h.Touch(s, t) {
			h.Unwire(s, t)
			h.Unwire(t, s)
		}
	}
	if !h.Lay.Pos[s].On || h.Slots[s].Kind == SlotEmpty {
		return
	}
	var near []int
	for t := 0; t < Slots; t++ {
		if t != s && h.Lay.Pos[t].On && h.Slots[t].Kind != SlotEmpty && h.Touch(s, t) {
			near = append(near, t)
		}
	}
	wire := func(a, b int) bool {
		if ok, _ := h.WireWhy(a, b); !ok {
			return false
		}
		h.Lay.Out[a] |= bit(b)
		h.setArm(a, b, 1)
		return true
	}
	if h.Sends(s) == PlugBoost {
		for _, t := range near {
			if h.WireArm(s, t) == 0 {
				wire(s, t)
			}
		}
		return
	}
	for _, t := range near {
		if h.Sends(t) == PlugBoost && h.WireArm(t, s) == 0 {
			wire(t, s)
		}
	}
	// What feeds s.
	reach := h.reached()
	if f := h.feeder(s); f < 0 {
		best, bp := -1, -1
		for _, t := range near {
			if h.Sends(t) == PlugBoost {
				continue
			}
			if ok, _ := h.WireWhy(t, s); ok && h.pref(t, s, reach) > bp {
				best, bp = t, h.pref(t, s, reach)
			}
		}
		if best >= 0 {
			wire(best, s)
		}
	}
	// What s feeds: neighbours after it with no feeder, or a weaker one.
	reach = h.reached()
	for _, t := range near {
		if h.Sends(t) == PlugBoost || h.WireArm(s, t) != 0 || h.WireArm(t, s) != 0 {
			continue
		}
		f := h.feeder(t)
		if f >= 0 && h.pref(f, t, reach) >= h.pref(s, t, reach) {
			continue
		}
		if h.rank(s) > h.rank(t) && (f >= 0 || reach&bit(s) == 0 || reach&bit(t) != 0) {
			continue // later in a run feeds earlier only to reach the unreached
		}
		if ok, _ := h.WireWhy(s, t); !ok {
			continue
		}
		if f >= 0 {
			h.Unwire(f, t)
		}
		wire(s, t)
	}
}

// relayTouch lays a wired hex out again so its wires join pieces that
// touch: the trigger in the middle, then each piece in run order next to
// the pieces it's wired to. Wires that still can't touch let go. Hexes
// from before pieces had to touch come over this way, once.
func (h *Hex) relayTouch() {
	// The pads first, while pieces still sit one to a tile: a piece on a
	// loop, twin, surge or docked way out stays on it, and no other piece
	// covers a pad, so nothing gains or loses a pad's role.
	pin := h.pins(h.Lay.Room)
	pads := map[Cell]bool{}
	g := h.Shape.Graph()
	for _, i := range []int{g.LoopFrom(), h.Shape.Twin(), h.Shape.Surge()} {
		if i >= 0 && g.Has(i) {
			pads[g.At[i]] = true
		}
	}
	for i := 0; i < int(g.N); i++ {
		if _, ok := h.Shape.OutPort(i); ok && h.Docks[i] != nil && g.Has(i) {
			pads[g.At[i]] = true
		}
	}
	h.Lay.Touch = true
	var on []int
	for s := 0; s < Slots; s++ {
		if h.Lay.Pos[s].On && h.Slots[s].Kind != SlotEmpty {
			on = append(on, s)
		} else {
			h.Lay.Pos[s].On = false
		}
	}
	// Run order: the trigger, then breadth first along the wires either
	// way, then anything left.
	var order []int
	var seen uint16
	add := func(s int) {
		if seen&bit(s) == 0 {
			seen |= bit(s)
			order = append(order, s)
		}
	}
	if t := h.TriggerSlot(); t >= 0 && t < Slots && h.Lay.Pos[t].On {
		add(t)
	}
	for i := 0; i < len(order) || len(order) < len(on); i++ {
		if i >= len(order) {
			for _, s := range on {
				if seen&bit(s) == 0 {
					add(s)
					break
				}
			}
		}
		x := order[i]
		for _, y := range on {
			if h.Lay.Out[x]&bit(y) != 0 || h.Lay.Out[y]&bit(x) != 0 {
				add(y)
			}
		}
	}
	for _, s := range on {
		h.Lay.Pos[s].On = false
	}
	var placed uint16
	// Pinned pieces go on their pads first.
	for _, s := range order {
		if pin[s] < 0 {
			continue
		}
		pad := roomCells[pin[s]]
		if c, ok := h.fitWith(s, pad, 0, pads, &pad); ok {
			h.Lay.Pos[s] = Place{At: c, On: true}
			placed |= bit(s)
		}
	}
	for _, s := range order {
		if placed&bit(s) != 0 {
			continue
		}
		var want uint16
		near := Cell{}
		for _, t := range on {
			if placed&bit(t) != 0 && (h.Lay.Out[s]&bit(t) != 0 || h.Lay.Out[t]&bit(s) != 0) {
				if want == 0 {
					near = h.Lay.Pos[t].At
				}
				want |= bit(t)
			}
		}
		c, ok := h.fitWith(s, near, want, pads, nil)
		if !ok {
			h.Lay.Kept |= bit(s) // no room: it waits in the tray
			continue
		}
		h.Lay.Pos[s] = Place{At: c, On: true}
		placed |= bit(s)
	}
	for a := 0; a < Slots; a++ {
		for b := 0; b < Slots; b++ {
			if h.Lay.Out[a]&bit(b) != 0 && !h.Touch(a, b) {
				h.Unwire(a, b)
			}
		}
	}
	h.RollPads()
}

// FillPads puts a piece on each of the shape's pads that has none: the
// piece in the pad's own slot, if its shape fits over the pad. It then
// joins what it touches there.
func (h *Hex) FillPads() {
	if !h.Lay.Touch {
		return
	}
	g := h.Shape.Graph()
	for _, i := range []int{g.LoopFrom(), h.Shape.Twin(), h.Shape.Surge()} {
		if i < 0 || i >= Slots || !g.Has(i) || !h.Lay.Pos[i].On || h.Slots[i].Kind == SlotEmpty {
			continue
		}
		pad := g.At[i]
		if h.PieceAt(pad) >= 0 {
			continue
		}
		if c, ok := h.fitWith(i, pad, 0, nil, &pad); ok {
			h.Lay.Pos[i].At = c
			h.Join(i)
		}
	}
}

// CellsAt are the cells piece s would cover anchored on at.
func (h *Hex) CellsAt(s int, at Cell) []Cell { return h.cellsAt(s, at) }

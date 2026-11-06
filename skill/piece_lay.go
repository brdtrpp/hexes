package skill

import "math/bits"

// Laying pieces down for the player: settling a hex after a part comes or
// goes, and laying a whole hex out from how it ran before (an old save,
// a new shape).

// layKey is what a laid hex's wiring depends on.
type layKey struct {
	room Room
	pos  [Slots]Place
	pc   [Slots]*Piece
}

// layCache keeps a laid hex's last wiring, keyed by what it was built
// from, so drawing and explaining don't rebuild it every call.
type layCache struct {
	key layKey
	w   *lay
}

func (h *Hex) layKey() layKey {
	k := layKey{room: h.Lay.Room, pos: h.Lay.Pos}
	for s := 0; s < Slots; s++ {
		k.pc[s] = PieceOf(h.Slots[s])
	}
	return k
}

// wired is wire, cached.
func (h *Hex) wired() *lay {
	k := h.layKey()
	if h.cache != nil && h.cache.w != nil && h.cache.key == k {
		return h.cache.w
	}
	w := h.wire()
	h.cache = &layCache{key: k, w: w}
	return w
}

// Settle tidies a laid hex after its parts changed: empty slots lose
// their spot, a piece that no longer fits where it was (its part changed
// shape) is picked up, and every piece not on the room is laid where it
// best joins in, if there's anywhere it fits.
func (h *Hex) Settle() {
	if !h.Laid() {
		return
	}
	for s := 0; s < Slots; s++ {
		switch {
		case h.Slots[s].Kind == SlotEmpty:
			h.Lift(s)
			h.Lay.Kept &^= bit(s)
		case h.Lay.Pos[s].On && !h.CanPlace(s, h.Lay.Pos[s]):
			h.Lift(s)
		}
	}
	var loose []int
	for _, s := range h.Loose() {
		if h.Lay.Kept&bit(s) == 0 {
			loose = append(loose, s)
		}
	}
	// The trigger first, so everything else can join it.
	for _, s := range loose {
		if sl := h.Slots[s]; sl.Kind == SlotPart && sl.Part.Kind == KindTrigger {
			h.AutoPlace(s, -1, ArmNone)
		}
	}
	for _, s := range loose {
		if !h.Lay.Pos[s].On {
			h.AutoPlace(s, -1, ArmNone)
		}
	}
	// A new piece with nowhere to join where the rest lie: lay the whole
	// hex out again, if that runs it without stopping anything else.
	stuck := false
	for _, s := range loose {
		stuck = stuck || !h.Runs(s)
	}
	if !stuck {
		return
	}
	was, old := h.Lay, *h.wired()
	h.LayOut(h.Lay.Room)
	now := h.wired()
	keep := now.runs&old.runs == old.runs && bits.OnesCount16(now.runs) > bits.OnesCount16(old.runs)
	for a := 0; a < Slots && keep; a++ {
		// Everything that ran still takes from what it took from.
		if old.runs&bit(a) != 0 && now.g.Out[a]&old.g.Out[a]&old.runs != old.g.Out[a]&old.runs {
			keep = false
		}
	}
	if !keep {
		h.Lay = was
	}
}

// AutoPlace lays loose piece s where it does most: running (the trigger
// reaches it) first, then joined to want (on arm, for a gate's branch) if
// want isn't -1, then the most joins and room left to grow. It reports
// whether it found a spot.
func (h *Hex) AutoPlace(s, want int, arm uint8) bool {
	c := h.spots(s, want, arm, 1, ^uint16(0), -1)
	if len(c) == 0 {
		return false
	}
	h.Lay.Pos[s] = c[0].p
	return true
}

// boosts reports whether s is a multiplier (whose graph edges come in
// from what it scales).
func boosts(h *Hex, s int) bool {
	sl := h.Slots[s]
	return sl.Kind == SlotPart && sl.Part.Kind == KindMult
}

type spot struct {
	p  Place
	sc int
}

// spots are the best k places for piece s, best first, at most one per
// anchor cell, with s lifted.
//
// allow is the pieces s may feed; joining any other costs. pin is the
// room index of a pad s should cover, or -1.
func (h *Hex) spots(s, want int, arm uint8, k int, allow uint16, pin int) []spot {
	if s < 0 || s >= Slots || PieceOf(h.Slots[s]) == nil {
		return nil
	}
	h.Lift(s)
	pc := PieceOf(h.Slots[s])
	trig := h.Slots[s].Kind == SlotPart && h.Slots[s].Part.Kind == KindTrigger
	var base board
	h.fill(&base)
	var best []spot
	var b board
	var w lay
	for _, c := range h.Lay.Room.Cells() {
		cell := spot{sc: -1}
		for r := uint8(0); r < Orients; r++ {
			p := Place{At: c, Rot: r, On: true}
			if !h.canPlace(&base, s, p) {
				continue
			}
			// The board with s laid there.
			b = base
			for _, pcell := range pc.Cells {
				b.occ[roomIndex(p.At.Add(Orient(pcell, int(r))))] = int8(s)
			}
			b.np[s] = 0
			for _, pl := range pc.Plugs {
				if b.np[s] < maxPlugs {
					b.plugs[s][b.np[s]] = PlugAt{p.At.Add(Orient(pc.Cells[pl.C], int(r))), OrientSide(int(pl.S), int(r)), pl.K, pl.Arm}
					b.np[s]++
				}
			}
			h.Lay.Pos[s] = p
			h.wireOn(&b, &w)
			sc := h.placeScore(&b, &w, s, want, arm, allow)
			h.Lay.Pos[s] = Place{}
			if pin >= 0 && b.occ[pin] == int8(s) {
				sc += 150 // on the pad that gives it its role
			}
			if trig {
				sc += 8 * c.Ring() // a trigger on the rim leaves room to grow
			} else {
				sc -= 3 * c.Ring()
			}
			if sc > cell.sc {
				cell = spot{p, sc}
			}
		}
		if cell.sc < 0 {
			continue
		}
		// Keep the k best cells, best first.
		i := len(best)
		for i > 0 && best[i-1].sc < cell.sc {
			i--
		}
		if i < k {
			best = append(best, spot{})
			copy(best[i+1:], best[i:])
			best[i] = cell
			if len(best) > k {
				best = best[:k]
			}
		}
	}
	return best
}

// placeScore is how good piece s's spot is on board b, wired as w (see
// AutoPlace).
func (h *Hex) placeScore(b *board, w *lay, s, want int, arm uint8, allow uint16) int {
	sc := 10
	if w.runs&bit(s) != 0 {
		sc += 1000
	}
	if want >= 0 && w.g.Out[want]&bit(s) != 0 {
		sc += 400
		if arm != ArmNone && w.arm[s].Gate == int8(want) && uint8(w.arm[s].Branch) == arm {
			sc += 200
		}
	}
	sc += 20 * int(w.joins[s])
	if extra := w.g.Out[s] &^ allow &^ bit(s); extra != 0 && !boosts(h, s) {
		// Feeding a piece that should take from elsewhere changes it.
		sc -= 60 * bits.OnesCount16(extra)
	}
	if want >= 0 {
		// Taking from more than it should pools what it lands on.
		for a := 0; a < Slots; a++ {
			if a != want && a != s && w.g.Out[a]&bit(s) != 0 && !boosts(h, s) {
				sc -= 60
			}
		}
	}
	// Room to grow: plugs that face an empty cell of the room, so the
	// next piece has somewhere to join.
	var armOpen [3]bool
	// Multipliers that should sit in this piece's sockets.
	need := 0
	for t := 0; t < Slots; t++ {
		if allow&bit(t) != 0 && t != s && boosts(h, t) && !h.Lay.Pos[t].On {
			need++
		}
	}
	for _, p := range b.plugs[s][:b.np[s]] {
		c := p.Cell.Add(CellDirs[p.Side])
		if !h.Lay.Room.Has(c) {
			continue
		}
		if o := b.at(c); o >= 0 {
			if o != s && p.K.Out() {
				armOpen[p.Arm] = true // already joined, maybe
			}
			continue
		}
		open := 0 // empty room around that cell: space to build into
		for _, d := range CellDirs {
			if n := c.Add(d); h.Lay.Room.Has(n) && b.at(n) < 0 {
				open++
			}
		}
		switch {
		case p.K == PlugGroup || p.K == PlugSpark:
			sc += 6 + 3*open
			armOpen[p.Arm] = true
		case p.K == PlugSocket:
			sc += 2 + open
			if need > 0 {
				sc += 60
				need--
			}
		}
	}
	// A gate with an arm that faces nowhere can't branch.
	if sl := h.Slots[s]; sl.Kind == SlotPart && sl.Part.Kind == KindGate {
		for _, a := range []uint8{ArmYes, ArmNo} {
			if !armOpen[a] {
				sc -= 150
			}
		}
	}
	return sc
}

// wish is where a piece wants to join when a hex is laid out: the slot
// it should take from, and the gate arm if it hangs off a gate.
type wish struct {
	from int
	arm  uint8
}

// wishes reads how h runs now (laid or not) as who should feed whom.
func (h *Hex) wishes() (order []int, w [Slots]wish) {
	for s := range w {
		w[s].from = -1
	}
	trig := h.TriggerSlot()
	o, n := h.Flow(trig)
	routes := h.Routes(trig)
	for _, s := range o[:n] {
		sl := h.Slots[s]
		if sl.Kind == SlotEmpty {
			continue
		}
		order = append(order, s)
		if s == trig {
			continue
		}
		if r := routes[s]; r.Gate >= 0 {
			w[s] = wish{int(r.Gate), uint8(r.Branch)}
			continue
		}
		// A multiplier feeds nothing on a laid hex, so look past them to
		// what fed them.
		pred := h.inFeed(s, o[:n])
		for k := 0; pred >= 0 && boosts(h, pred) && k < Slots; k++ {
			pred = h.inFeed(pred, o[:n])
		}
		switch {
		case sl.Kind == SlotPart && (sl.Part.Kind == KindTarget || sl.Part.Kind == KindForm):
			w[s].from = trig
		case sl.Kind == SlotPart && sl.Part.Kind == KindMult:
			w[s].from = pred
		default:
			from := -1
			for _, a := range o[:n] {
				if a != s && h.Feeds(a, s) && h.isTarget(a) {
					from = a
					break
				}
			}
			if from < 0 {
				from = pred
			}
			if from < 0 {
				from = trig
			}
			w[s].from = from
		}
	}
	// Each piece after the one it joins, so a layout can build outward.
	for changed := true; changed; {
		changed = false
		for i, s := range order {
			f := w[s].from
			if f < 0 {
				continue
			}
			for j := i + 1; j < len(order); j++ {
				if order[j] == f {
					copy(order[i:j], order[i+1:j+1])
					order[j] = s
					changed = true
					break
				}
			}
			if changed {
				break
			}
		}
	}
	// Anything the flow didn't reach goes last, wishing to join where its
	// kind usually does: a reach on the trigger, a multiplier in an
	// effect's socket, anything else on what a reach picked.
	ran := len(order)
	for s := 0; s < Slots; s++ {
		if h.Slots[s].Kind == SlotEmpty {
			continue
		}
		seen := false
		for _, o := range order {
			seen = seen || o == s
		}
		if seen {
			continue
		}
		order = append(order, s)
		sl := h.Slots[s]
		if sl.Kind != SlotPart || sl.Part.Kind == KindTrigger || trig < 0 {
			continue
		}
		w[s].from = trig
		switch sl.Part.Kind {
		case KindTarget, KindForm:
		case KindMult:
			w[s].from = -1
			for _, a := range order[:ran] {
				if as := h.Slots[a]; as.Kind == SlotPart && as.Part.Kind == KindEffect && (as.Part.Effect.Verb() == sl.Part.Verb || sl.Part.Verb == VerbAll) {
					w[s].from = a
					break
				}
			}
		default:
			for _, a := range order[:ran] {
				if h.isTarget(a) {
					w[s].from = a
					break
				}
			}
		}
	}
	return order, w
}

// LayOut lays h out on room: every piece placed to run the way h runs
// now (or as close as the room allows), the rest left loose. A hex that
// wasn't laid becomes laid. It searches a few of the best spots for each
// piece in turn, backing up when a later piece finds nowhere to join, and
// keeps the best layout it saw.
func (h *Hex) LayOut(room Room) {
	order, w := h.wishes()
	var allow [Slots]uint16
	for t := 0; t < Slots; t++ {
		if f := w[t].from; f >= 0 {
			allow[f] |= bit(t)
		}
	}
	pin := h.pins(room)
	kept := h.Lay.Kept
	if h.Lay.Room != room {
		kept = 0
	}
	h.Lay = Layout{Room: room, Kept: kept}
	var best Layout
	bestScore := -1
	budget := 4000 // spots tried, so a crowded room gives up quickly
	// Joining where it should counts for more than running at all: a
	// piece left loose is plain to see, one that runs off the wrong piece
	// quietly does something else.
	perfect := 0
	for _, s := range order {
		if kept&bit(s) != 0 {
			continue
		}
		perfect += 100
		if pin[s] >= 0 {
			perfect += 40
		}
		if w[s].from >= 0 {
			perfect += 150
			if w[s].arm != ArmNone {
				perfect += 50
			}
		}
	}
	score := func() int {
		lw := h.wire()
		sc := 0
		for _, s := range order {
			if lw.runs&bit(s) != 0 {
				sc += 100
			}
			if pin[s] >= 0 && h.PieceAt(roomCells[pin[s]]) == s {
				sc += 40
			}
			if f := w[s].from; f >= 0 && lw.g.Out[f]&bit(s) != 0 {
				sc += 150
				if a := w[s].arm; a != ArmNone && lw.arm[s].Gate == int8(f) && uint8(lw.arm[s].Branch) == a {
					sc += 50
				}
			}
		}
		return sc
	}
	var dfs func(k int)
	dfs = func(k int) {
		if k == len(order) {
			if sc := score(); sc > bestScore {
				best, bestScore = h.Lay, sc
			}
			return
		}
		if budget <= 0 || bestScore == perfect {
			return
		}
		s := order[k]
		if kept&bit(s) != 0 {
			dfs(k + 1) // set aside: it stays off
			return
		}
		top := 4
		if h.Slots[s].Kind == SlotPart && h.Slots[s].Part.Kind == KindTrigger {
			top = 8 // where the trigger goes decides the most
		}
		cands := h.spots(s, w[s].from, w[s].arm, top, allow[s], pin[s])
		for _, c := range cands {
			budget--
			h.Lay.Pos[s] = c.p
			// A piece that doesn't join where it should is a dead end
			// unless nothing better came up.
			ok := h.Runs(s)
			if f := w[s].from; f >= 0 {
				ok = ok && h.Feeds(f, s)
			}
			if ok || c.sc == cands[0].sc {
				dfs(k + 1)
			}
			h.Lay.Pos[s] = Place{}
			if budget <= 0 {
				break
			}
		}
		if len(cands) == 0 {
			dfs(k + 1) // no room for it: it stays loose
		}
	}
	dfs(0)
	h.Lay = best
	if bestScore < 0 {
		h.Lay = Layout{Room: room, Kept: kept}
	}
}

// pins are the pads that give a piece its role (the loop's way back, a
// twin or surge, a way out with a module docked on it), by piece: the room index
// of the pad it should cover, or -1. A laid hex keeps them on whatever
// covers them now; an old one on the slot the pad belongs to.
func (h *Hex) pins(room Room) (pin [Slots]int) {
	for s := range pin {
		pin[s] = -1
	}
	sh := h.Shape
	g := sh.Graph()
	special := []int{g.LoopFrom(), sh.Twin(), sh.Surge()}
	for i := 0; i < int(g.N); i++ {
		if _, ok := sh.OutPort(i); ok && h.Docks[i] != nil {
			special = append(special, i)
		}
	}
	for _, i := range special {
		if i < 0 || !g.Has(i) || !room.Has(g.At[i]) {
			continue
		}
		p := h.pad(i)
		if p >= 0 && p < Slots && h.Slots[p].Kind != SlotEmpty && pin[p] < 0 {
			pin[p] = roomIndex(g.At[i])
		}
	}
	return pin
}

// EnsureLaid makes a hex laid on its shape's room, if it isn't already.
func (h *Hex) EnsureLaid() {
	if !h.Laid() {
		h.LayOut(RoomFor(h.Shape))
	}
}

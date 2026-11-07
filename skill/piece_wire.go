package skill

// The open board. A wired hex's pieces each take one cell anywhere on the
// room and never turn: what joins is what the player wires together. A
// wire runs from a piece that sends (a trigger's spark, a group out of a
// target, form, gate or effect, a multiplier's boost) to one whose plugs
// take it (a "who" plug for a spark or a group, a socket for a boost). A
// piece's silhouette is still what it is, and its plugs still say what it
// sends and takes and how many: an effect takes as many multipliers as it
// has sockets, a fork nub boosts two things, a gate sends YES and NO.

// Sends is what piece s sends down its wires: a spark (a trigger), a
// boost (a multiplier), a group (anything with a group plug), or nothing.
func (h *Hex) Sends(s int) PlugKind {
	pc := PieceOf(h.Slots[s])
	if pc == nil {
		return PlugNone
	}
	out := PlugNone
	for _, p := range pc.Plugs {
		switch p.K {
		case PlugSpark, PlugBoost:
			return p.K
		case PlugGroup:
			out = PlugGroup
		}
	}
	return out
}

// Takes reports whether piece s has a plug that takes k.
func (h *Hex) Takes(s int, k PlugKind) bool {
	pc := PieceOf(h.Slots[s])
	if pc == nil {
		return false
	}
	for _, p := range pc.Plugs {
		if !p.K.Out() && p.K.Takes(k) {
			return true
		}
	}
	return false
}

// count is how many of piece s's plugs are of kind k.
func (h *Hex) count(s int, k PlugKind) int {
	pc := PieceOf(h.Slots[s])
	if pc == nil {
		return 0
	}
	n := 0
	for _, p := range pc.Plugs {
		if p.K == k {
			n++
		}
	}
	return n
}

// SocketCount is how many multipliers piece s takes.
func (h *Hex) SocketCount(s int) int { return h.count(s, PlugSocket) }

// Boosts is how many pieces multiplier s can boost.
func (h *Hex) Boosts(s int) int { return h.count(s, PlugBoost) }

// Arms is how many ways piece s sends: a gate's YES and NO, the three
// rungs of an overload or rage trigger, or one.
func (h *Hex) Arms(s int) int {
	sl := h.Slots[s]
	if sl.Kind != SlotPart {
		return 1
	}
	switch {
	case sl.Part.Kind == KindGate:
		return 2
	case sl.Part.Kind == KindTrigger && sl.Part.Trigger.Rungs():
		return 3
	}
	return 1
}

// WireArm is the arm of the wire from a to b (1 for the first, YES, or
// rung I), or 0 with no wire.
func (h *Hex) WireArm(a, b int) uint8 {
	if a < 0 || b < 0 || a >= Slots || b >= Slots || h.Lay.Out[a]&bit(b) == 0 {
		return 0
	}
	arm := uint8(1)
	if h.Lay.Arm0[a]&bit(b) != 0 {
		arm++
	}
	if h.Lay.Arm1[a]&bit(b) != 0 {
		arm += 2
	}
	return arm
}

func (h *Hex) setArm(a, b int, arm uint8) {
	h.Lay.Arm0[a] &^= bit(b)
	h.Lay.Arm1[a] &^= bit(b)
	if arm == 0 {
		return
	}
	if (arm-1)&1 != 0 {
		h.Lay.Arm0[a] |= bit(b)
	}
	if (arm-1)&2 != 0 {
		h.Lay.Arm1[a] |= bit(b)
	}
}

// Unwire takes the wire from a to b off.
func (h *Hex) Unwire(a, b int) {
	if a < 0 || b < 0 || a >= Slots || b >= Slots {
		return
	}
	h.Lay.Out[a] &^= bit(b)
	h.setArm(a, b, 0)
}

// unwireAll takes every wire to and from s off.
func (h *Hex) unwireAll(s int) {
	h.Lay.Out[s], h.Lay.Arm0[s], h.Lay.Arm1[s] = 0, 0, 0
	for a := 0; a < Slots; a++ {
		h.Unwire(a, s)
	}
}

// reaches reports whether a reaches b along the wires (as the graph runs:
// a boost runs from what it scales to the multiplier).
func (h *Hex) reaches(a, b int) bool {
	var seen uint16
	var walk func(x int) bool
	walk = func(x int) bool {
		if x == b {
			return true
		}
		if seen&bit(x) != 0 {
			return false
		}
		seen |= bit(x)
		for y := 0; y < Slots; y++ {
			if h.graphEdge(x, y) && walk(y) {
				return true
			}
		}
		return false
	}
	return walk(a)
}

// graphEdge reports whether the wires make an edge x→y in the run graph.
func (h *Hex) graphEdge(x, y int) bool {
	if h.Lay.Out[x]&bit(y) != 0 && h.Sends(x) != PlugBoost {
		return true
	}
	return h.Lay.Out[y]&bit(x) != 0 && h.Sends(y) == PlugBoost
}

// WireWhy says whether a wire from a to b can go on, and why not.
func (h *Hex) WireWhy(a, b int) (bool, string) {
	if a == b || a < 0 || b < 0 || a >= Slots || b >= Slots {
		return false, ""
	}
	if !h.Lay.Pos[a].On || !h.Lay.Pos[b].On {
		return false, "both pieces have to be on the board"
	}
	k := h.Sends(a)
	if k == PlugNone {
		return false, "it sends nothing on"
	}
	if !h.Takes(b, k) {
		if k == PlugBoost {
			return false, "a multiplier only plugs into something with a socket"
		}
		return false, "it has no way in for that"
	}
	if k == PlugBoost {
		n := 0
		for x := 0; x < Slots; x++ {
			if x != a && h.Lay.Out[x]&bit(b) != 0 && h.Sends(x) == PlugBoost {
				n++
			}
		}
		if n >= h.SocketCount(b) {
			return false, "its sockets are full"
		}
		if have := countBits(h.Lay.Out[a] &^ bit(b)); have >= h.Boosts(a) {
			return false, "this multiplier boosts as many as it can"
		}
		if h.reaches(a, b) {
			return false, "that would run in a circle"
		}
		return true, ""
	}
	if h.reaches(b, a) {
		return false, "that would run in a circle"
	}
	return true, ""
}

func countBits(x uint16) int {
	n := 0
	for ; x != 0; x &= x - 1 {
		n++
	}
	return n
}

// CanWire reports whether a wire can join a and b one way or the other,
// and which way: the sender and the taker.
func (h *Hex) CanWire(a, b int) (from, to int, ok bool) {
	if ok, _ := h.WireWhy(a, b); ok {
		return a, b, true
	}
	if ok, _ := h.WireWhy(b, a); ok {
		return b, a, true
	}
	return -1, -1, false
}

// Wire puts a wire from a to b on (arm 1 for the first way), if it can go.
func (h *Hex) Wire(a, b int, arm uint8) bool {
	if h.WireArm(a, b) != 0 {
		h.setArm(a, b, max(arm, 1))
		return true
	}
	if ok, _ := h.WireWhy(a, b); !ok {
		return false
	}
	h.Lay.Out[a] |= bit(b)
	h.setArm(a, b, max(arm, 1))
	return true
}

// ToggleWire is a tap from a on b: no wire puts one on; a wire steps to
// the sender's next arm (YES, then NO; rung I, II, III); the last arm
// takes it off. It reports what's there now: the arm, 0 for none, or -1
// when no wire can join them.
func (h *Hex) ToggleWire(a, b int) int {
	from, to := a, b
	if h.WireArm(a, b) == 0 && h.WireArm(b, a) != 0 {
		from, to = b, a
	}
	if arm := h.WireArm(from, to); arm != 0 {
		if int(arm) < h.Arms(from) {
			h.setArm(from, to, arm+1)
			return int(arm + 1)
		}
		h.Unwire(from, to)
		return 0
	}
	f, t, ok := h.CanWire(a, b)
	if !ok {
		return -1
	}
	h.Wire(f, t, 1)
	return 1
}

// wireLinks is every wire as a link, senders in slot order. A wire that
// no longer fits (a part changed under it) is skipped.
func (h *Hex) wireLinks(each func(Link)) {
	for a := 0; a < Slots; a++ {
		if h.Lay.Out[a] == 0 || !h.Lay.Pos[a].On {
			continue
		}
		k := h.Sends(a)
		gate := h.Slots[a].Kind == SlotPart && h.Slots[a].Part.Kind == KindGate
		for b := 0; b < Slots; b++ {
			if h.Lay.Out[a]&bit(b) == 0 || !h.Lay.Pos[b].On || k == PlugNone || !h.Takes(b, k) {
				continue
			}
			l := Link{From: a, To: b, Kind: k, Cell: h.Lay.Pos[a].At}
			if gate {
				l.Arm = h.WireArm(a, b)
			}
			each(l)
		}
	}
}

// Rung is the rung piece s hangs off when an overload or rage trigger
// feeds it straight: 1, 2 or 3, or 0 when it doesn't.
func (h *Hex) Rung(s int) int {
	if !h.Lay.Wired {
		return 0
	}
	t := h.TriggerSlot()
	if t < 0 || t >= Slots || h.Arms(t) != 3 {
		return 0
	}
	return int(h.WireArm(t, s))
}

// Budget is how many pieces the hex holds: its shape's slots.
func (h *Hex) Budget() int { return h.Shape.Len() }

// Held is how many pieces are on the board now.
func (h *Hex) Held() int {
	n := 0
	for s, sl := range h.Slots {
		if sl.Kind != SlotEmpty && h.Lay.Pos[s].On {
			n++
		}
	}
	return n
}

// freeCell is the room's free cell nearest near (the middle when near is
// off the room), or false.
func (h *Hex) freeCell(near Cell) (Cell, bool) {
	var b board
	h.fill(&b)
	best, bestD := Cell{}, 1<<30
	for _, c := range h.Lay.Room.Cells() {
		if b.at(c) >= 0 {
			continue
		}
		d := c.Sub(near).Ring()*8 + c.Ring()
		if d < bestD {
			best, bestD = c, d
		}
	}
	return best, bestD < 1<<30
}

// wireFromSlots lays an unlaid hex on the open board, running the way
// its shape ran it: each part on its own slot's cell, a wire for every
// link that did something.
func (h *Hex) wireFromSlots() {
	order, w := h.wishes()
	g := h.Shape.Graph()
	room := Board
	h.Lay = Layout{Room: room, Wired: true}
	var loose []int
	for s := 0; s < Slots; s++ {
		if h.Slots[s].Kind == SlotEmpty {
			continue
		}
		if g.Has(s) && room.Has(g.At[s]) {
			h.Lay.Pos[s] = Place{At: g.At[s], On: true}
		} else {
			loose = append(loose, s)
		}
	}
	for _, s := range loose {
		if c, ok := h.freeCell(Cell{}); ok {
			h.Lay.Pos[s] = Place{At: c, On: true}
		}
	}
	trig := h.TriggerSlot()
	for _, s := range order {
		if !h.Lay.Pos[s].On {
			continue
		}
		// What it wished for, else what that joins, back to the trigger:
		// a piece that sends nothing on (a stance, a charge) passes on what
		// reached it.
		arm := max(w[s].arm, 1)
		for f, k := w[s].from, 0; k <= Slots; k++ {
			if f < 0 {
				if trig < 0 || trig == s || k == Slots {
					break
				}
				f, k = trig, Slots-1
			}
			if h.Lay.Pos[f].On {
				if h.Sends(s) == PlugBoost {
					if h.Wire(s, f, 1) { // a multiplier plugs into what it scales
						break
					}
				} else if h.Wire(f, s, arm) {
					break
				}
			}
			f, arm = w[f].from, max(w[f].arm, 1)
		}
	}
}

// passes reports whether piece a hands on a group the slots used: a
// trigger, a target, a form, a gate or a call that runs now. An effect's
// group plug is new with pieces.
func (h *Hex) passes(a int) bool {
	sl := h.Slots[a]
	if sl.Kind == SlotNested {
		return false
	}
	switch sl.Part.Kind {
	case KindTrigger, KindTarget, KindForm, KindGate:
		return true
	case KindCall:
		return sl.Part.RunsNow()
	}
	return false
}

// wireFromEdges turns a layout from before wires into the open board:
// each piece keeps its first cell, every place two edges met becomes a
// wire, and a piece covering a pad (a loop, a twin, a surge, a docked
// way out) moves onto it.
func (h *Hex) wireFromEdges() {
	var ls []Link
	var b board
	h.fill(&b)
	links(&b, func(l Link) { ls = append(ls, l) })
	pin := h.pins(h.Lay.Room)
	lay := Layout{Room: h.Lay.Room, Kept: h.Lay.Kept, Wired: true}
	for s := 0; s < Slots; s++ {
		if p := h.Lay.Pos[s]; p.On {
			lay.Pos[s] = Place{At: p.At, On: true}
		}
	}
	for s := 0; s < Slots; s++ {
		if pin[s] < 0 || !lay.Pos[s].On {
			continue
		}
		c := roomCells[pin[s]]
		for o := 0; o < Slots; o++ {
			if o != s && lay.Pos[o].On && lay.Pos[o].At == c {
				lay.Pos[o].At = lay.Pos[s].At
			}
		}
		lay.Pos[s].At = c
	}
	h.Lay = lay
	for _, l := range ls {
		h.Lay.Out[l.From] |= bit(l.To)
		if l.Arm != ArmNone {
			h.setArm(l.From, l.To, l.Arm)
		}
	}
}

// AutoWire lays loose piece s on the free cell nearest where it joins,
// with the wires its kind usually takes: a reach off the trigger, a
// multiplier into the first socket that takes its verb, an effect off
// the last reach, anything else off the trigger. It reports whether it
// found room.
func (h *Hex) AutoWire(s int) bool {
	if h.Slots[s].Kind == SlotEmpty {
		return false
	}
	from := h.autoFrom(s)
	near := Cell{}
	if from >= 0 && h.Lay.Pos[from].On {
		near = h.Lay.Pos[from].At
	}
	c, ok := h.freeCell(near)
	if !ok {
		return false
	}
	h.Lay.Pos[s] = Place{At: c, On: true}
	h.Lay.Kept &^= bit(s)
	h.wireIn(s, from)
	return true
}

// autoFrom is the piece s usually takes its wire from: a multiplier the
// first thing in flow it scales, a reach or gate the trigger, an effect
// the last reach that takes it. -1 for a trigger.
func (h *Hex) autoFrom(s int) int {
	sl := h.Slots[s]
	trig := h.TriggerSlot()
	if trig == s {
		trig = -1
	}
	from := -1
	isPart := sl.Kind == SlotPart
	switch {
	case isPart && sl.Part.Kind == KindTrigger:
	case isPart && sl.Part.Kind == KindMult:
		for _, a := range h.flowOrder() {
			as := h.Slots[a]
			if as.Kind != SlotPart || a == s {
				continue
			}
			v := sl.Part.Verb
			fit := as.Part.Kind == KindEffect && (as.Part.Effect.Verb() == v || v == VerbAll || v == VerbMarked && as.Part.Effect == EffectDamage)
			fit = fit || (as.Part.Kind == KindTarget || as.Part.Kind == KindForm) && (v == VerbCount || v == VerbRadius)
			if fit {
				if ok, _ := h.WireWhyAt(s, a); ok {
					from = a
					break
				}
			}
		}
	case isPart && (sl.Part.Kind == KindTarget || sl.Part.Kind == KindForm || sl.Part.Kind == KindGate):
		from = trig
	default:
		// An effect lands on the last reach that will take it, else the
		// last effect, else the trigger.
		from = -1
		order := h.flowOrder()
		for pass := 0; pass < 2 && from < 0; pass++ {
			for i := len(order) - 1; i >= 0; i-- {
				a := order[i]
				as := h.Slots[a]
				if a == s || as.Kind != SlotPart {
					continue
				}
				reach := as.Part.Kind == KindTarget || as.Part.Kind == KindForm
				if reach == (pass == 0) && as.Part.Kind != KindTrigger && as.Part.Kind != KindMult {
					if ok, _ := h.WireWhyAt(a, s); ok {
						from = a
						break
					}
				}
			}
		}
		if from < 0 {
			from = trig
		}
	}
	return from
}

// WireIn wires piece s, laid where it is, the way AutoWire would.
func (h *Hex) WireIn(s int) {
	if s >= 0 && s < Slots && h.Lay.Pos[s].On && h.Slots[s].Kind != SlotEmpty {
		h.Lay.Kept &^= bit(s)
		h.wireIn(s, h.autoFrom(s))
	}
}

// FreeCell is the free tile nearest near, if the board has one.
func (h *Hex) FreeCell(near Cell) (Cell, bool) { return h.freeCell(near) }

// wireIn puts on piece s's usual wires: from the piece it joins, or, for
// a trigger, to everything waiting for a start.
func (h *Hex) wireIn(s, from int) {
	sl := h.Slots[s]
	isPart := sl.Kind == SlotPart
	switch {
	case isPart && sl.Part.Kind == KindTrigger:
		// Everything that waits for a start takes it.
		for b := 0; b < Slots; b++ {
			if b == s || !h.Lay.Pos[b].On || h.Slots[b].Kind == SlotEmpty {
				continue
			}
			fed := false
			for a := 0; a < Slots; a++ {
				fed = fed || h.graphEdge(a, b)
			}
			if !fed && h.Sends(b) != PlugBoost {
				h.Wire(s, b, 1)
			}
		}
	case from >= 0:
		if h.Sends(s) == PlugBoost {
			h.Wire(s, from, 1)
		} else {
			h.Wire(from, s, 1)
		}
	}
}

// WireWhyAt is WireWhy with b treated as laid even if it isn't yet.
func (h *Hex) WireWhyAt(a, b int) (bool, string) {
	pa, pb := h.Lay.Pos[a], h.Lay.Pos[b]
	h.Lay.Pos[a].On, h.Lay.Pos[b].On = true, true
	ok, why := h.WireWhy(a, b)
	h.Lay.Pos[a], h.Lay.Pos[b] = pa, pb
	return ok, why
}

// flowOrder is the laid pieces that run, in run order.
func (h *Hex) flowOrder() []int {
	t := h.TriggerSlot()
	if t < 0 || t >= Slots {
		return nil
	}
	o, n := h.Flow(t)
	out := make([]int, n)
	copy(out, o[:n])
	return out
}

// Rebox moves a wired hex onto room (a new shape's): pieces keep their
// cells where the room has them and move to the nearest free cell where
// it doesn't. Wires stay.
func (h *Hex) Rebox(room Room) {
	if !h.Lay.Wired {
		return
	}
	h.Lay.Room = room
	var move []int
	var seen Room
	for s := 0; s < Slots; s++ {
		p := h.Lay.Pos[s]
		if !p.On {
			continue
		}
		if !room.Has(p.At) || seen.Has(p.At) {
			move = append(move, s)
			h.Lay.Pos[s].On = false
			continue
		}
		seen = seen.with(p.At)
	}
	for _, s := range move {
		at := h.Lay.Pos[s].At
		if c, ok := h.freeCell(at); ok {
			h.Lay.Pos[s] = Place{At: c, On: true}
		} else {
			h.Lay.Pos[s] = Place{}
		}
	}
}

// RungOf is the rung each piece runs from: on a hex an overload or rage
// trigger starts, the lowest rung any path from the trigger reaches it
// on (a piece wired to rung II and everything after it runs from rung
// II up). 0 for pieces that always run.
func (h *Hex) RungOf() (r [Slots]uint8) {
	if !h.Lay.Wired {
		return r
	}
	t := h.TriggerSlot()
	if t < 0 || t >= Slots || h.Arms(t) != 3 {
		return r
	}
	w := h.wired()
	for s := range r {
		r[s] = 4
	}
	r[t] = 0
	for changed := true; changed; {
		changed = false
		for a := 0; a < Slots; a++ {
			if w.runs&bit(a) == 0 || (a != t && r[a] == 4) {
				continue
			}
			for b := 0; b < Slots; b++ {
				if b == t || w.g.Out[a]&bit(b) == 0 {
					continue
				}
				k := r[a]
				if a == t {
					k = h.WireArm(t, b)
					if k == 0 {
						k = 1 // a boost hanging off the trigger
					}
				}
				if k < r[b] {
					r[b] = k
					changed = true
				}
			}
		}
	}
	for s := range r {
		if r[s] == 4 || r[s] < 2 {
			r[s] = 0
		}
	}
	return r
}

// Remap is lay with its pieces moved to new slots (where[old] is the new
// slot, or -1 for a piece that left the hex), wires and all.
func (lay Layout) Remap(where [Slots]int) Layout {
	out := Layout{Room: lay.Room, Wired: lay.Wired}
	for a := 0; a < Slots; a++ {
		na := where[a]
		if na < 0 || na >= Slots {
			continue
		}
		out.Pos[na] = lay.Pos[a]
		if lay.Kept&bit(a) != 0 {
			out.Kept |= bit(na)
		}
		for b := 0; b < Slots; b++ {
			nb := where[b]
			if nb < 0 || nb >= Slots || lay.Out[a]&bit(b) == 0 {
				continue
			}
			out.Out[na] |= bit(nb)
			if lay.Arm0[a]&bit(b) != 0 {
				out.Arm0[na] |= bit(nb)
			}
			if lay.Arm1[a]&bit(b) != 0 {
				out.Arm1[na] |= bit(nb)
			}
		}
	}
	return out
}

// Trim sets aside the pieces past the board's budget, last in flow
// first, into the tray.
func (h *Hex) Trim() {
	for h.Held() > h.Budget() {
		order := h.flowOrder()
		drop := -1
		for s := Slots - 1; s >= 0 && drop < 0; s-- {
			// Pieces that don't run go first.
			if h.Lay.Pos[s].On && h.Slots[s].Kind != SlotEmpty && !h.Runs(s) && s != h.TriggerSlot() {
				drop = s
			}
		}
		for i := len(order) - 1; i >= 0 && drop < 0; i-- {
			if order[i] != h.TriggerSlot() {
				drop = order[i]
			}
		}
		if drop < 0 {
			return
		}
		h.SetAside(drop)
	}
}

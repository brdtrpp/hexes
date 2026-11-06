package skill

import (
	"errors"
	"fmt"
)

// RingSlots is the number of slots around an original hex ring. Slots is
// the most any shape has (a generated one can use them all, see GenGraph).
// Center is the original shapes' center slot index; only Hubs and Forks
// use it.
const (
	RingSlots = 6
	Slots     = 9
	Center    = RingSlots
)

// SocketKind is what a slot will accept. A hex's sockets are fixed by its
// template; where each socket sits on the ring is rolled when it's learned.
type SocketKind uint8

const (
	SocketTrigger SocketKind = iota // triggers only
	SocketTarget                    // targets or gates
	SocketEffect                    // effects, multipliers, charges or calls
	SocketWild                      // anything but a trigger, or a nested hex
)

func (s SocketKind) String() string {
	return [...]string{"T", "G", "E", "W"}[s]
}

// Accepts reports whether a part kind fits the socket.
func (s SocketKind) Accepts(k PartKind) bool {
	switch s {
	case SocketTrigger:
		return k == KindTrigger
	case SocketTarget:
		return k == KindTarget || k == KindGate || k == KindForm
	case SocketEffect:
		return k == KindEffect || k == KindMult || k == KindCharge || k == KindCall || k == KindCost || k == KindStance
	case SocketWild:
		return k != KindNone && k != KindTrigger
	}
	return false
}

// Template is a hex as the game defines it, before anyone learns it.
type Template struct {
	Name    string
	Sockets [RingSlots]SocketKind
	// Broken is how many of the six ring edges are cut when learned. Parts
	// across a cut edge don't feed each other.
	Broken int
}

// SlotKind says what a slot holds.
type SlotKind uint8

const (
	SlotEmpty SlotKind = iota
	SlotPart
	SlotNested
)

// Slot holds a part or a nested hex.
type Slot struct {
	Kind   SlotKind
	Part   Part
	Nested *Hex
}

// Hex is a learned hex: sockets placed on the ring, edges linked or cut,
// and whatever parts the player has slotted. Shape is how it runs; the
// zero Shape is the plain six-slot Ring, so hexes saved before shapes
// existed load as Rings.
type Hex struct {
	Name    string
	Seed    uint64
	Sockets [Slots]SocketKind
	// Linked[i] is the edge between ring slot i and the next one
	// clockwise. A shape can also leave an edge out entirely (see
	// ShapeKind.HasEdge).
	Linked [Slots]bool
	Slots  [Slots]Slot
	Shape  Shape `json:",omitempty"`
	// Frames are the slots' frames (see Frame). Zero is plain, so hexes
	// saved before frames load with every slot plain.
	Frames [Slots]Frame `json:",omitzero"`
	// Docks[s] is the hex docked to the way out on slot s, if any. The
	// game sets it from its board before compiling; it isn't saved.
	Docks [Slots]*Hex `json:"-"`
	// Libs[s] is the library hex a run-now Call in slot s names, if the
	// board has one by that name. The game sets it with Docks; a call
	// whose library is missing does nothing and passes its group on.
	Libs [Slots]*Hex `json:"-"`
	// Lay is where a laid hex's pieces sit (see Piece). The zero Layout
	// is a hex that runs on its shape's slots, as hexes always did.
	Lay   Layout `json:",omitzero"`
	cache *layCache
}

// NewHex builds a Ring hex with sockets exactly as given and every edge
// linked. The center socket takes effects (it only matters once the hex
// is reshaped into a Hub). Learn is how the game makes hexes; this is for
// tests and tools.
func NewHex(name string, sockets [RingSlots]SocketKind) *Hex {
	h := &Hex{Name: name}
	copy(h.Sockets[:], sockets[:])
	h.Sockets[Center] = SocketEffect
	for i := Center + 1; i < Slots; i++ {
		h.Sockets[i] = SocketWild
	}
	for i := range h.Linked {
		h.Linked[i] = true
	}
	return h
}

// Learn rolls a template's layout from a seed: sockets are shuffled around
// the ring and Broken edges are cut. The same template and seed always give
// the same layout; different seeds give different ones.
func Learn(t Template, seed uint64) *Hex {
	rng := splitmix{seed ^ fnv64(t.Name)}
	h := NewHex(t.Name, t.Sockets)
	h.Seed = seed
	for i := RingSlots - 1; i > 0; i-- {
		j := rng.intn(i + 1)
		h.Sockets[i], h.Sockets[j] = h.Sockets[j], h.Sockets[i]
	}
	broken := t.Broken
	if broken > RingSlots {
		broken = RingSlots
	}
	for broken > 0 {
		e := rng.intn(RingSlots)
		if h.Linked[e] {
			h.Linked[e] = false
			broken--
		}
	}
	return h
}

// Set puts a part in a slot, checking the socket.
func (h *Hex) Set(i int, p Part) error {
	if i < 0 || i >= Slots {
		return fmt.Errorf("slot %d out of range", i)
	}
	if !h.Sockets[i].Accepts(p.Kind) {
		return fmt.Errorf("slot %d (%s socket) can't hold a %s part", i, h.Sockets[i], p.Kind)
	}
	if ok, why := h.Fits(i, p, nil); !ok {
		return fmt.Errorf("slot %d: %s", i, why)
	}
	h.Slots[i] = Slot{Kind: SlotPart, Part: p}
	return nil
}

// Nest puts a hex inside a wild slot. The nested hex resolves once per
// target fed to that slot, with that target as its origin: a target's target.
func (h *Hex) Nest(i int, n *Hex) error {
	if i < 0 || i >= Slots {
		return fmt.Errorf("slot %d out of range", i)
	}
	if h.Sockets[i] != SocketWild {
		return fmt.Errorf("slot %d (%s socket) can't hold a nested hex", i, h.Sockets[i])
	}
	if n == nil || n == h {
		return errors.New("bad nested hex")
	}
	h.Slots[i] = Slot{Kind: SlotNested, Nested: n}
	return nil
}

// Clear empties a slot.
func (h *Hex) Clear(i int) { h.Slots[i] = Slot{} }

// Depth is how many nesting levels the hex uses (0 if none).
// A hex that (wrongly) contains itself reports a huge depth instead of
// recursing forever, so Compile rejects it.
func (h *Hex) Depth() int { return h.depth(0) }

const depthGuard = 64

func (h *Hex) depth(level int) int {
	if level >= depthGuard {
		return depthGuard
	}
	d := 0
	for _, s := range h.Slots {
		if s.Kind == SlotNested {
			if nd := 1 + s.Nested.depth(level+1); nd > d {
				d = nd
			}
		}
	}
	return d
}

// splitmix is SplitMix64: tiny, fast and identical on every platform and Go
// version, which math/rand does not promise.
type splitmix struct{ s uint64 }

func (r *splitmix) next() uint64 {
	r.s += 0x9e3779b97f4a7c15
	z := r.s
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}

func (r *splitmix) intn(n int) int { return int(r.next() % uint64(n)) }

func fnv64(s string) uint64 {
	h := uint64(0xcbf29ce484222325)
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 0x100000001b3
	}
	return h
}

// Branch says which way of a gate a slot sits on.
type Branch uint8

const (
	BranchNone Branch = iota
	BranchYes         // runs on whoever passed the gate
	BranchNo          // runs on whoever failed it
)

// Route is a slot's place in a gate: which slot holds the gate it hangs
// off and which way. Gate is -1 for slots no gate routes.
type Route struct {
	Gate   int8
	Branch Branch
}

// Routes works out which slots hang off which gates, reading the ring in
// flow order from slot start (the trigger's slot, or the entry slot for a
// nested hex). A merge (a Hub's center) is never in a gate's branch. The rules, which
// Compile follows:
//   - A gate's yes slot is the next slot in flow order, its no slot the
//     one after that. Each must be fed by the slot before it, and neither
//     can wrap past the end of the ring back to the start.
//   - A gate in a yes slot tests only the ones that passed (both must
//     hold) and takes the next two slots as its own, so the outer gate
//     gets no no slot.
//   - A gate in a no slot tests the ones that failed: an else-if.
func (h *Hex) Routes(start int) [Slots]Route {
	if h.Laid() {
		w := h.wired()
		r := w.arm
		for s := range r {
			if w.runs&bit(s) == 0 || (r[s].Gate >= 0 && w.runs&bit(int(r[s].Gate)) == 0) {
				r[s] = Route{Gate: -1}
			}
		}
		return r
	}
	var r [Slots]Route
	for i := range r {
		r[i].Gate = -1
	}
	order, n := h.Flow(start)
	g := h.Shape.Graph()
	// A merge pools what feeds it, so it's never a branch.
	free := func(s int) bool { return r[s].Gate < 0 && g.Ins(s) < 2 }
	isGate := func(s int) bool {
		return h.Slots[s].Kind == SlotPart && h.Slots[s].Part.Kind == KindGate
	}
	for k := 0; k < n; k++ {
		s := order[k]
		if !isGate(s) {
			continue
		}
		yes := k + 1
		if yes >= n || !free(order[yes]) || !h.Feeds(s, order[yes]) {
			continue
		}
		r[order[yes]] = Route{int8(s), BranchYes}
		if isGate(order[yes]) {
			continue
		}
		no := k + 2
		if no >= n || !free(order[no]) || !h.Feeds(order[yes], order[no]) {
			continue
		}
		r[order[no]] = Route{int8(s), BranchNo}
	}
	return r
}

package skill

import (
	"errors"
	"fmt"
)

// Slots is the number of slots on a hex ring.
const Slots = 6

// SocketKind is what a slot will accept. A hex's sockets are fixed by its
// template; where each socket sits on the ring is rolled when it's learned.
type SocketKind uint8

const (
	SocketTrigger SocketKind = iota // triggers only
	SocketTarget                    // targets or gates
	SocketEffect                    // effects, multipliers or charges
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
		return k == KindTarget || k == KindGate
	case SocketEffect:
		return k == KindEffect || k == KindMult || k == KindCharge
	case SocketWild:
		return k == KindTarget || k == KindEffect || k == KindMult || k == KindGate || k == KindCharge
	}
	return false
}

// Template is a hex as the game defines it, before anyone learns it.
type Template struct {
	Name    string
	Sockets [Slots]SocketKind
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
// and whatever parts the player has slotted.
type Hex struct {
	Name    string
	Seed    uint64
	Sockets [Slots]SocketKind
	// Linked[i] is the edge between slot i and slot i+1 (clockwise).
	Linked [Slots]bool
	Slots  [Slots]Slot
}

// NewHex builds a hex with sockets exactly as given and every edge linked.
// Learn is how the game makes hexes; this is for tests and tools.
func NewHex(name string, sockets [Slots]SocketKind) *Hex {
	h := &Hex{Name: name, Sockets: sockets}
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
	for i := Slots - 1; i > 0; i-- {
		j := rng.intn(i + 1)
		h.Sockets[i], h.Sockets[j] = h.Sockets[j], h.Sockets[i]
	}
	broken := t.Broken
	if broken > Slots {
		broken = Slots
	}
	for broken > 0 {
		e := rng.intn(Slots)
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

// adjacent reports whether slots i and j touch across a linked edge.
func (h *Hex) adjacent(i, j int) bool {
	switch {
	case j == (i+1)%Slots:
		return h.Linked[i]
	case i == (j+1)%Slots:
		return h.Linked[j]
	}
	return false
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

// Routes works out which slots hang off which gates, reading clockwise
// from slot start (the trigger's slot, or 0 for a nested hex). The rules,
// which Compile follows:
//   - A gate's yes slot is the next slot clockwise, its no slot the one
//     after that. Each must be linked to the slot before it, and neither
//     can wrap past the end of the ring back to the start.
//   - A gate in a yes slot tests only the ones that passed (both must
//     hold) and takes the next two slots as its own, so the outer gate
//     gets no no slot.
//   - A gate in a no slot tests the ones that failed: an else-if.
func (h *Hex) Routes(start int) [Slots]Route {
	var r [Slots]Route
	for i := range r {
		r[i].Gate = -1
	}
	var order [Slots]int
	for k := range order {
		order[k] = (start + k) % Slots
	}
	isGate := func(s int) bool {
		return h.Slots[s].Kind == SlotPart && h.Slots[s].Part.Kind == KindGate
	}
	for k, s := range order {
		if !isGate(s) {
			continue
		}
		yes := k + 1
		if yes >= Slots || r[order[yes]].Gate >= 0 || !h.adjacent(s, order[yes]) {
			continue
		}
		r[order[yes]] = Route{int8(s), BranchYes}
		if isGate(order[yes]) {
			continue
		}
		no := k + 2
		if no >= Slots || r[order[no]].Gate >= 0 || !h.adjacent(order[yes], order[no]) {
			continue
		}
		r[order[no]] = Route{int8(s), BranchNo}
	}
	return r
}

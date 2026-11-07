package skill

// Pads. The open board carries bonus tiles scattered over it: a LOOP pad
// runs the hex again from the piece on it, a TWIN pad makes its piece
// fire twice, a SURGE pad boosts its piece's numbers. A board keeps the
// pads its shape comes with where they were, and gets one more in each
// of the six directions out from the middle, at a random distance and of
// a random kind, rolled once when the board is laid.

import (
	"fmt"
	"hash/fnv"
	"math/rand/v2"
)

// MaxPads is how many pads a board holds at most.
const MaxPads = 12

// PadKind is what a pad does to the piece on it.
type PadKind uint8

const (
	PadNone PadKind = iota
	PadLoop
	PadTwin
	PadSurge
)

// Name is the pad's label on the board.
func (k PadKind) Name() string {
	switch k {
	case PadLoop:
		return "LOOP"
	case PadTwin:
		return "TWIN"
	case PadSurge:
		return "SURGE"
	}
	return ""
}

// BoardPad is one pad: its tile and kind. Kind PadNone is no pad.
type BoardPad struct {
	At   Cell
	Kind PadKind `json:",omitempty"`
}

// BoardPads are the board's pads, in order: the shape's own first.
func (h *Hex) BoardPads() []BoardPad {
	var out []BoardPad
	for _, p := range h.Lay.Pads {
		if p.Kind != PadNone {
			out = append(out, p)
		}
	}
	return out
}

// PadAt is the kind of pad on tile c, or PadNone.
func (h *Hex) PadAt(c Cell) PadKind {
	for _, p := range h.Lay.Pads {
		if p.Kind != PadNone && p.At == c {
			return p.Kind
		}
	}
	return PadNone
}

// padOn is the first pad of kind k that piece s covers, or false. A
// trigger takes no pad: it only starts the hex.
func (h *Hex) padOn(s int, k PadKind) bool {
	if s < 0 || s >= Slots || !h.Lay.Pos[s].On {
		return false
	}
	if sl := h.Slots[s]; sl.Kind == SlotPart && sl.Part.Kind == KindTrigger {
		return false
	}
	for _, p := range h.Lay.Pads {
		if p.Kind == k && h.PieceAt(p.At) == s {
			return true
		}
	}
	return false
}

// padSeed is the board's roll: from the hex's name and shape, so the same
// hex rolls the same pads.
func (h *Hex) padSeed() uint64 {
	f := fnv.New64a()
	fmt.Fprintf(f, "%s|%v|%v|%v", h.Name, h.Shape.Kind, h.Shape.Quirk, h.Shape.At)
	return f.Sum64()
}

// RollPads lays the board's pads afresh: the shape's own where its slots
// sit, then one in each direction out from the middle, on a free tile,
// two of each kind in a random order.
func (h *Hex) RollPads() {
	h.Lay.Pads = [MaxPads]BoardPad{}
	n := 0
	add := func(c Cell, k PadKind) bool {
		if n >= MaxPads || !h.Lay.Room.Has(c) || h.PadAt(c) != PadNone {
			return false
		}
		h.Lay.Pads[n] = BoardPad{At: c, Kind: k}
		n++
		return true
	}
	g := h.Shape.Graph()
	for _, sp := range []struct {
		i int
		k PadKind
	}{{g.LoopFrom(), PadLoop}, {h.Shape.Twin(), PadTwin}, {h.Shape.Surge(), PadSurge}} {
		if sp.i >= 0 && g.Has(sp.i) {
			add(g.At[sp.i], sp.k)
		}
	}
	seed := h.padSeed()
	rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
	kinds := []PadKind{PadLoop, PadLoop, PadTwin, PadTwin, PadSurge, PadSurge}
	rng.Shuffle(len(kinds), func(i, j int) { kinds[i], kinds[j] = kinds[j], kinds[i] })
	var b board
	h.fill(&b)
	for d := 0; d < 6; d++ {
		// Tiles in the wedge between direction d and the next one round.
		for try := 0; try < 24; try++ {
			r := 3 + rng.IntN(RoomRadius-3)
			k := rng.IntN(r)
			c := Cell{}
			for i := 0; i < r; i++ {
				c = c.Add(CellDirs[d])
			}
			for i := 0; i < k; i++ {
				c = c.Add(CellDirs[(d+2)%6])
			}
			if b.at(c) >= 0 {
				continue // under a piece already
			}
			if add(c, kinds[d]) {
				break
			}
		}
	}
}

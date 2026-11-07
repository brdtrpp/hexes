package skill

import (
	"encoding/json"
	"math/bits"
	"strconv"
	"strings"
)

// Pieces. A laid hex isn't a ring of identical slots: every part is a
// piece with its own silhouette, laid on the hex's room (a patch of cells
// the shape sets), turned whichever way you like. Pieces talk through
// plugs on their edges, and only plugs of the right kinds join:
//
//	spark  (when)      a trigger's plugs. They start the hex.
//	group  (who)       out of a target, a gate's arms, or an effect (who
//	                   it landed on). Into a "who" plug.
//	boost  (how much)  a multiplier's nub. Into an effect's or target's
//	                   socket, so each effect takes only as many
//	                   multipliers as it has sockets.
//
// A "who" plug takes a spark or a group: an effect fed straight by the
// trigger lands on the trigger's subject. Charges, calls, costs and
// stances take anything on any side.
//
// The silhouette is the part: a cone is a triangle that fans out, a line a
// bar, adjacent an arc around a hole, a gate a V whose two arms are YES
// and NO. Two plugs join when they sit on the same edge facing each
// other. Wiring them turns the layout into the same graph the compiler
// always ran, so gates, charges, calls and loops work as before; a piece
// nothing reaches from the trigger doesn't run.

// PlugKind is what a plug carries or takes.
type PlugKind uint8

const (
	PlugNone   PlugKind = iota
	PlugSpark           // out: the trigger firing
	PlugGroup           // out: a group of entities
	PlugBoost           // out: a multiplier's nub
	PlugWho             // in: a spark or a group
	PlugSocket          // in: a boost
	PlugAny             // in: anything
)

// Out reports whether the plug sends.
func (k PlugKind) Out() bool { return k >= PlugSpark && k <= PlugBoost }

// Takes reports whether in plug k takes what out plug o sends.
func (k PlugKind) Takes(o PlugKind) bool {
	switch k {
	case PlugWho:
		return o == PlugSpark || o == PlugGroup
	case PlugSocket:
		return o == PlugBoost
	case PlugAny:
		return o == PlugSpark || o == PlugGroup
	}
	return false
}

// Arm marks a gate's out plugs: which way of the gate they send.
const (
	ArmNone = iota
	ArmYes
	ArmNo
)

// Plug is one plug on a piece: on its cell C, facing side S (CellDirs
// order, before the piece is turned).
type Plug struct {
	C, S uint8
	K    PlugKind
	Arm  uint8
}

// Piece is a part's silhouette: its cells (around its anchor, before it
// is turned) and its plugs.
type Piece struct {
	Name  string
	Cells []Cell
	Plugs []Plug
}

// Size is how many cells the piece covers.
func (p *Piece) Size() int { return len(p.Cells) }

// Turn turns cell offset c clockwise by r sixths.
func Turn(c Cell, r int) Cell {
	q, s := int(c.Q), int(c.R)
	for i := 0; i < ((r%6)+6)%6; i++ {
		q, s = -s, q+s
	}
	return Cell{int8(q), int8(s)}
}

// Orients is how many ways a piece can lie: six turns, then the same six
// flipped.
const Orients = 12

// Orient is cell offset c with a piece lying orient o: flipped first (left
// for right, across the up-down line) when o is 6 or more, then turned.
func Orient(c Cell, o int) Cell {
	o = ((o % Orients) + Orients) % Orients
	if o >= 6 {
		c = Cell{-c.Q, c.Q + c.R}
		o -= 6
	}
	return Turn(c, o)
}

// OrientSide is side d of a piece lying orient o.
func OrientSide(d, o int) int {
	o = ((o % Orients) + Orients) % Orients
	if o >= 6 {
		d = (6 - d) % 6
		o -= 6
	}
	return (d + o) % 6
}

// pieces is every silhouette, by name of part, then variant (Part.Fit).
var (
	all6 = []Plug{{0, 0, PlugAny, 0}, {0, 1, PlugAny, 0}, {0, 2, PlugAny, 0},
		{0, 3, PlugAny, 0}, {0, 4, PlugAny, 0}, {0, 5, PlugAny, 0}}
	one    = []Cell{{0, 0}}
	domino = []Cell{{0, 0}, {0, -1}}
	wedge  = []Cell{{0, 0}, {0, -1}, {1, -1}} // three cells that all touch
	bar    = []Cell{{0, 0}, {0, -1}, {0, -2}}
	dogleg = []Cell{{0, 0}, {0, -1}, {1, -2}}
	vee    = []Cell{{0, 0}, {-1, 0}, {1, -1}}
)

var trigPieces = []Piece{
	{"spark", one, []Plug{{0, 0, PlugSpark, 0}, {0, 2, PlugSpark, 0}, {0, 4, PlugSpark, 0}}},
	{"twin spark", one, []Plug{{0, 0, PlugSpark, 0}, {0, 3, PlugSpark, 0}}},
	{"long spark", domino, []Plug{{1, 0, PlugSpark, 0}, {1, 1, PlugSpark, 0}, {1, 5, PlugSpark, 0}}},
}

var targetPieces = [...][]Piece{
	TargetSelf: {
		{"you", one, []Plug{{0, 3, PlugWho, 0}, {0, 0, PlugGroup, 0}, {0, 1, PlugGroup, 0}, {0, 5, PlugSocket, 0}}},
	},
	TargetNearest: {
		{"point", one, []Plug{{0, 3, PlugWho, 0}, {0, 0, PlugGroup, 0}, {0, 1, PlugGroup, 0}, {0, 5, PlugSocket, 0}}},
		{"point", one, []Plug{{0, 3, PlugWho, 0}, {0, 0, PlugGroup, 0}, {0, 2, PlugSocket, 0}, {0, 4, PlugSocket, 0}}},
	},
	TargetCone: {
		{"fan", wedge, []Plug{{0, 3, PlugWho, 0}, {1, 0, PlugGroup, 0}, {2, 1, PlugGroup, 0}, {0, 5, PlugSocket, 0}}},
		{"fan", wedge, []Plug{{0, 3, PlugWho, 0}, {1, 5, PlugGroup, 0}, {2, 2, PlugGroup, 0}, {1, 0, PlugSocket, 0}}},
	},
	TargetAdjacent: {
		// An arc around a hole at (-1,0): whatever sits in the hole
		// touches all three cells.
		{"arc", []Cell{{0, 0}, {0, -1}, {-1, 1}}, []Plug{{0, 2, PlugWho, 0}, {0, 5, PlugGroup, 0}, {1, 0, PlugGroup, 0}, {2, 3, PlugGroup, 0}, {1, 1, PlugSocket, 0}}},
	},
	TargetChain: {
		{"zigzag", dogleg, []Plug{{0, 3, PlugWho, 0}, {2, 0, PlugGroup, 0}, {2, 1, PlugGroup, 0}, {1, 5, PlugSocket, 0}}},
	},
	TargetWreck: {
		{"wreck", domino, []Plug{{0, 3, PlugWho, 0}, {1, 0, PlugGroup, 0}, {1, 1, PlugSocket, 0}}},
	},
	TargetWeakest: {
		{"pin", one, []Plug{{0, 3, PlugWho, 0}, {0, 0, PlugGroup, 0}, {0, 5, PlugSocket, 0}}},
	},
	TargetMarked: {
		{"pin", one, []Plug{{0, 3, PlugWho, 0}, {0, 0, PlugGroup, 0}, {0, 5, PlugGroup, 0}, {0, 1, PlugSocket, 0}}},
	},
	TargetLine: {
		{"bar", bar, []Plug{{0, 3, PlugWho, 0}, {2, 0, PlugGroup, 0}, {1, 2, PlugSocket, 0}}},
		{"bar", bar, []Plug{{0, 3, PlugWho, 0}, {2, 0, PlugGroup, 0}, {2, 5, PlugGroup, 0}, {1, 2, PlugSocket, 0}}},
	},
	TargetBoth: {
		{"long bar", []Cell{{0, 0}, {0, -1}, {0, 1}}, []Plug{{0, 2, PlugWho, 0}, {1, 0, PlugGroup, 0}, {2, 3, PlugGroup, 0}, {0, 5, PlugSocket, 0}}},
	},
	TargetAllies: {
		{"band", one, []Plug{{0, 3, PlugWho, 0}, {0, 0, PlugGroup, 0}, {0, 1, PlugGroup, 0}, {0, 5, PlugGroup, 0}}},
	},
	TargetAura: {
		// An arc around a hole at (1,-1).
		{"arc", []Cell{{0, 0}, {0, -1}, {1, 0}}, []Plug{{0, 4, PlugWho, 0}, {0, 1, PlugGroup, 0}, {1, 0, PlugGroup, 0}, {2, 2, PlugGroup, 0}, {1, 5, PlugSocket, 0}}},
	},
}

var formPieces = [...]Piece{
	FormBolt:     {"bolt", domino, []Plug{{0, 3, PlugWho, 0}, {1, 0, PlugGroup, 0}, {0, 5, PlugSocket, 0}}},
	FormBeam:     {"beam", bar, []Plug{{0, 3, PlugWho, 0}, {2, 0, PlugGroup, 0}, {2, 1, PlugGroup, 0}, {1, 5, PlugSocket, 0}}},
	FormAura:     {"ring", []Cell{{0, -1}, {1, -1}, {1, 0}, {0, 1}, {-1, 1}, {-1, 0}}, []Plug{{3, 3, PlugWho, 0}, {0, 3, PlugGroup, 0}, {0, 0, PlugGroup, 0}, {2, 2, PlugGroup, 0}, {4, 4, PlugSocket, 0}}},
	FormNova:     {"flower", []Cell{{0, 0}, {0, -1}, {1, 0}, {-1, 1}}, []Plug{{0, 3, PlugWho, 0}, {1, 0, PlugGroup, 0}, {2, 2, PlugGroup, 0}, {3, 4, PlugGroup, 0}, {0, 1, PlugSocket, 0}}},
	FormDash:     {"dash", bar, []Plug{{0, 3, PlugWho, 0}, {2, 0, PlugGroup, 0}, {1, 5, PlugSocket, 0}}},
	FormDetonate: {"burst", wedge, []Plug{{0, 3, PlugWho, 0}, {1, 0, PlugGroup, 0}, {2, 1, PlugGroup, 0}, {2, 2, PlugGroup, 0}, {0, 5, PlugSocket, 0}}},
	FormZone:     {"field", wedge, []Plug{{0, 3, PlugWho, 0}, {1, 5, PlugGroup, 0}, {2, 2, PlugGroup, 0}, {1, 0, PlugSocket, 0}}},
	FormTurret:   {"turret", domino, []Plug{{0, 3, PlugWho, 0}, {1, 0, PlugGroup, 0}, {1, 1, PlugGroup, 0}, {0, 5, PlugSocket, 0}}},
	FormTrap:     {"trap", one, []Plug{{0, 3, PlugWho, 0}, {0, 0, PlugGroup, 0}, {0, 1, PlugSocket, 0}}},
	FormDrone:    {"drone", dogleg, []Plug{{0, 3, PlugWho, 0}, {2, 0, PlugGroup, 0}, {1, 5, PlugSocket, 0}}},
}

// Effects: a damage piece is a domino, so it costs room; statuses and
// pushes are one cell. Each has one or two sockets for multipliers.
var damagePieces = []Piece{
	{"slab", domino, []Plug{{0, 3, PlugWho, 0}, {1, 0, PlugSocket, 0}, {0, 5, PlugSocket, 0}, {1, 1, PlugGroup, 0}}},
	{"slab", domino, []Plug{{0, 3, PlugWho, 0}, {1, 5, PlugSocket, 0}, {1, 1, PlugSocket, 0}, {1, 0, PlugGroup, 0}}},
	{"chip", one, []Plug{{0, 3, PlugWho, 0}, {0, 0, PlugSocket, 0}, {0, 1, PlugGroup, 0}}},
	{"hook", dogleg, []Plug{{0, 3, PlugWho, 0}, {1, 5, PlugSocket, 0}, {2, 0, PlugSocket, 0}, {2, 2, PlugSocket, 0}, {2, 1, PlugGroup, 0}}},
}

var smallEffect = []Piece{
	{"stud", one, []Plug{{0, 3, PlugWho, 0}, {0, 0, PlugSocket, 0}, {0, 1, PlugGroup, 0}}},
	{"stud", domino, []Plug{{0, 3, PlugWho, 0}, {1, 0, PlugSocket, 0}, {1, 5, PlugSocket, 0}, {1, 1, PlugGroup, 0}}},
}

var sustainPieces = []Piece{
	{"cap", one, []Plug{{0, 3, PlugWho, 0}, {0, 0, PlugSocket, 0}}},
	{"cap", one, []Plug{{0, 3, PlugWho, 0}, {0, 1, PlugSocket, 0}, {0, 5, PlugSocket, 0}}},
}

var multPieces = []Piece{
	{"nub", one, []Plug{{0, 0, PlugBoost, 0}}},
	{"fork nub", one, []Plug{{0, 0, PlugBoost, 0}, {0, 2, PlugBoost, 0}}},
}

var gatePieces = []Piece{
	{"vee", vee, []Plug{{0, 3, PlugWho, 0}, {1, 5, PlugGroup, ArmYes}, {1, 0, PlugGroup, ArmYes}, {2, 1, PlugGroup, ArmNo}, {2, 0, PlugGroup, ArmNo}}},
	{"wide vee", []Cell{{0, 0}, {-1, 0}, {1, -1}, {0, 1}}, []Plug{{3, 3, PlugWho, 0}, {1, 5, PlugGroup, ArmYes}, {1, 4, PlugGroup, ArmYes}, {2, 1, PlugGroup, ArmNo}, {2, 2, PlugGroup, ArmNo}}},
}

var (
	chargePiece = Piece{"node", one, all6}
	callPiece   = Piece{"jack", one, all6}
	libPiece    = Piece{"socket", one, []Plug{{0, 3, PlugWho, 0}, {0, 0, PlugGroup, 0}, {0, 1, PlugGroup, 0}}}
	passPiece   = Piece{"seal", one, all6}
	nestPiece   = Piece{"nest", one, []Plug{{0, 3, PlugWho, 0}, {0, 0, PlugGroup, 0}}}
)

// A who plug is a mouth three sides wide: each piece's who plug also
// takes from the sides either side of it, where those are free (not
// facing the piece's own cells or another plug). It keeps a piece's
// silhouette the thing that matters, not which exact side it faces.
func init() {
	widen := func(p *Piece) {
		var plugs []Plug
		for _, pl := range p.Plugs {
			plugs = append(plugs, pl)
			if pl.K != PlugWho {
				continue
			}
			for _, d := range []int{5, 1} {
				side := uint8((int(pl.S) + d) % 6)
				c := p.Cells[pl.C].Add(CellDirs[side])
				free := true
				for _, own := range p.Cells {
					free = free && own != c
				}
				for _, q := range p.Plugs {
					free = free && !(q.C == pl.C && q.S == side)
				}
				if free {
					plugs = append(plugs, Plug{pl.C, side, PlugWho, 0})
				}
			}
		}
		p.Plugs = plugs
	}
	for _, list := range [][]Piece{damagePieces, smallEffect, sustainPieces, gatePieces} {
		for i := range list {
			widen(&list[i])
		}
	}
	for t := range targetPieces {
		for i := range targetPieces[t] {
			widen(&targetPieces[t][i])
		}
	}
	for i := range formPieces {
		widen(&formPieces[i])
	}
	widen(&libPiece)
	widen(&nestPiece)
}

// pick is variant fit of a list.
func pick(ps []Piece, fit uint8) *Piece { return &ps[int(fit)%len(ps)] }

// PieceOf is a slot's piece, or nil for an empty slot.
func PieceOf(sl Slot) *Piece {
	switch sl.Kind {
	case SlotNested:
		return &nestPiece
	case SlotPart:
		return PartPiece(sl.Part)
	}
	return nil
}

// PartPiece is a part's piece.
func PartPiece(p Part) *Piece {
	switch p.Kind {
	case KindTrigger:
		return pick(trigPieces, p.Fit)
	case KindTarget:
		if int(p.Target) < len(targetPieces) && len(targetPieces[p.Target]) > 0 {
			return pick(targetPieces[p.Target], p.Fit)
		}
		return pick(targetPieces[TargetNearest], p.Fit)
	case KindForm:
		if int(p.Form) < len(formPieces) {
			return &formPieces[p.Form]
		}
		return &formPieces[FormBolt]
	case KindEffect:
		switch {
		case p.Effect == EffectDamage:
			return pick(damagePieces, p.Fit)
		case p.Effect.Sustain():
			return pick(sustainPieces, p.Fit)
		}
		return pick(smallEffect, p.Fit)
	case KindMult:
		return pick(multPieces, p.Fit)
	case KindGate:
		return pick(gatePieces, p.Fit)
	case KindCharge:
		return &chargePiece
	case KindCall:
		if p.RunsNow() {
			return &libPiece
		}
		return &callPiece
	}
	return &passPiece // costs and stances
}

// Fits is how many silhouettes part kind p can roll as (Part.Fit).
func Fits(p Part) int {
	switch p.Kind {
	case KindTrigger:
		return len(trigPieces)
	case KindTarget:
		if int(p.Target) < len(targetPieces) {
			return max(1, len(targetPieces[p.Target]))
		}
	case KindEffect:
		switch {
		case p.Effect == EffectDamage:
			return len(damagePieces)
		case p.Effect.Sustain():
			return len(sustainPieces)
		}
		return len(smallEffect)
	case KindMult:
		return len(multPieces)
	case KindGate:
		return len(gatePieces)
	}
	return 1
}

// Rooms. A room is a set of cells within RoomRadius steps of the
// middle: 169 at most. Bit i is roomCells[i], numbered middle out, so the
// first 19 (two steps out, where shapes' cells sit) and the first 61 (the
// board before it grew) keep the bits they had.

// RoomRadius is how far out a room reaches; Board is that whole disc,
// the open board every wired hex is laid on.
const RoomRadius = 7

const roomN = 1 + 3*RoomRadius*(RoomRadius+1)

// Room is the cells a laid hex has to lay pieces on: a bit set over
// roomCells.
type Room [(roomN + 63) / 64]uint64

var roomCells [roomN]Cell

// Board is the open board: every cell within RoomRadius.
var Board Room

func init() {
	n := 0
	for ring := 0; ring <= RoomRadius; ring++ {
		for q := -RoomRadius; q <= RoomRadius; q++ {
			for r := -RoomRadius; r <= RoomRadius; r++ {
				c := Cell{int8(q), int8(r)}
				if c.Ring() == ring {
					roomCells[n] = c
					n++
				}
			}
		}
	}
	for i := 0; i < roomN; i++ {
		Board = Board.set(i)
	}
}

// RoomBits is the room of the first n cells, middle out.
func RoomBits(n int) Room {
	var m Room
	for i := 0; i < n && i < roomN; i++ {
		m = m.set(i)
	}
	return m
}

func (m Room) bit(i int) bool { return m[i>>6]&(1<<uint(i&63)) != 0 }

func (m Room) set(i int) Room {
	m[i>>6] |= 1 << uint(i&63)
	return m
}

// Empty reports whether the room has no cells.
func (m Room) Empty() bool { return m == Room{} }

// MarshalJSON writes a room as its words in hex, low word first, as a
// string: a number this wide doesn't survive a trip through a float.
func (m Room) MarshalJSON() ([]byte, error) {
	parts := make([]string, len(m))
	for i, w := range m {
		parts[i] = strconv.FormatUint(w, 16)
	}
	return json.Marshal(strings.Join(parts, "."))
}

// UnmarshalJSON reads that, or the plain number older saves held.
func (m *Room) UnmarshalJSON(b []byte) error {
	*m = Room{}
	if len(b) > 0 && b[0] == '"' {
		var str string
		if err := json.Unmarshal(b, &str); err != nil {
			return err
		}
		for i, part := range strings.Split(str, ".") {
			if i >= len(m) {
				break
			}
			w, err := strconv.ParseUint(part, 16, 64)
			if err != nil {
				return err
			}
			m[i] = w
		}
		return nil
	}
	var w uint64
	if err := json.Unmarshal(b, &w); err != nil {
		return err
	}
	m[0] = w
	return nil
}

// roomIndex is c's bit in a Room, or -1 outside.
func roomIndex(c Cell) int {
	q, r := int(c.Q)+RoomRadius, int(c.R)+RoomRadius
	if q < 0 || q > 2*RoomRadius || r < 0 || r > 2*RoomRadius {
		return -1
	}
	return int(roomAt[q][r])
}

var roomAt [2*RoomRadius + 1][2*RoomRadius + 1]int16

func init() {
	for q := range roomAt {
		for r := range roomAt[q] {
			roomAt[q][r] = -1
		}
	}
	for i, c := range roomCells {
		roomAt[c.Q+RoomRadius][c.R+RoomRadius] = int16(i)
	}
}

// Has reports whether the room holds c.
func (m Room) Has(c Cell) bool {
	i := roomIndex(c)
	return i >= 0 && m.bit(i)
}

// Len is how many cells the room has.
func (m Room) Len() int {
	n := 0
	for _, w := range m {
		n += bits.OnesCount64(w)
	}
	return n
}

// Cells are the room's cells, middle out.
func (m Room) Cells() []Cell {
	var out []Cell
	for i, c := range roomCells {
		if m.bit(i) {
			out = append(out, c)
		}
	}
	return out
}

func (m Room) with(c Cell) Room { return m.set(roomIndex(c)) }

// RoomSize is how many cells a shape's room has: a Triangle 7, a Ring 15,
// growing with the shape's slots up to 19.
func RoomSize(s Shape) int {
	n := s.Len()
	if n <= 3 {
		return 7
	}
	return min(19, 2*n+3)
}

// RoomFor is a shape's room: its own cells (where its pads sit), grown
// outward to RoomSize, nearest the middle first, the order rolled from the
// shape so two shapes of one size don't look alike.
func RoomFor(s Shape) Room {
	g := s.Graph()
	var m Room
	for i := 0; i < int(g.N); i++ {
		if roomIndex(g.At[i]) >= 0 {
			m = m.with(g.At[i])
		}
	}
	if m.Empty() {
		m = m.with(Cell{})
	}
	rng := splitmix{s.ID ^ uint64(s.Kind)*0x9e37 ^ 0x51ed}
	want := RoomSize(s)
	for m.Len() < want {
		// Cells next to the room, nearest the middle first, then rolled.
		best, bestScore := -1, int64(-1)
		for i, c := range roomCells {
			if m.bit(i) || c.Ring() > 2 {
				continue // shapes' rooms stay within two steps
			}
			next := false
			for _, d := range CellDirs {
				next = next || m.Has(c.Add(d))
			}
			if !next {
				continue
			}
			score := int64(3-c.Ring())<<40 | int64(rng.next()>>25)
			if score > bestScore {
				best, bestScore = i, score
			}
		}
		if best < 0 {
			break
		}
		m = m.set(best)
	}
	return m
}

// Layout is where a laid hex's pieces sit. The zero Layout means the hex
// isn't laid: it runs on its shape's slots the way hexes always did.
type Layout struct {
	Room Room
	Pos  [Slots]Place
	// Kept is the pieces the player set aside on purpose: Settle leaves
	// them off the room.
	Kept uint16 `json:",omitempty"`
	// Wired is the open board: each piece takes one cell anywhere on the
	// room, never turns, and joins what the player wires it to (Out),
	// not what its edges touch. A Layout from before wires (Wired false)
	// is wired from how its edges met the first time it loads.
	Wired bool `json:",omitempty"`
	// Touch is set once the open board's pieces keep their silhouettes
	// and wires only join pieces that touch (piece_touch.go).
	Touch bool `json:",omitempty"`
	// Pads are the open board's bonus tiles (piece_pads.go): whatever
	// piece covers one gets its quirk. Rolled once per board.
	Pads [MaxPads]BoardPad `json:",omitzero"`
	// Out is the wires: bit b of Out[s] is a wire from piece s to piece
	// b (from a multiplier, it boosts b). Arm0 and Arm1 are each wire's
	// arm, 1 + bit + 2*bit: a gate's YES (1) or NO (2), or the rung an
	// overload or rage trigger sends on (1, 2 or 3).
	Out  [Slots]uint16 `json:",omitempty"`
	Arm0 [Slots]uint16 `json:",omitempty"`
	Arm1 [Slots]uint16 `json:",omitempty"`
}

// Place is one piece's spot: its anchor cell and how it lies (Orient).
// On is false for a piece the hex holds but hasn't laid down yet (it
// doesn't run).
type Place struct {
	At  Cell
	Rot uint8
	On  bool
}

// Laid reports whether the hex lays its parts as pieces.
func (h *Hex) Laid() bool { return !h.Lay.Room.Empty() }

// HasSlot reports whether slot i can hold a part: any slot of a laid hex,
// or one of its shape's slots.
func (h *Hex) HasSlot(i int) bool {
	if h.Laid() {
		return i >= 0 && i < Slots
	}
	return h.Shape.Has(i)
}

// PieceCells are the cells piece s covers where it's laid (nil if it
// isn't).
func (h *Hex) PieceCells(s int) []Cell {
	pc := PieceOf(h.Slots[s])
	if pc == nil || !h.Lay.Pos[s].On {
		return nil
	}
	if h.Lay.Wired {
		if !h.Lay.Touch {
			return []Cell{h.Lay.Pos[s].At}
		}
		return h.cellsAt(s, h.Lay.Pos[s].At)
	}
	out := make([]Cell, len(pc.Cells))
	for i, c := range pc.Cells {
		out[i] = h.Lay.Pos[s].At.Add(Orient(c, int(h.Lay.Pos[s].Rot)))
	}
	return out
}

// PlugAt is a plug where its piece is laid: the cell it's on, the side it
// faces and what it is.
type PlugAt struct {
	Cell Cell
	Side int
	K    PlugKind
	Arm  uint8
}

// Plugs are piece s's plugs where it's laid.
func (h *Hex) Plugs(s int) []PlugAt {
	pc := PieceOf(h.Slots[s])
	if pc == nil || !h.Lay.Pos[s].On {
		return nil
	}
	at := h.Lay.Pos[s]
	out := make([]PlugAt, len(pc.Plugs))
	for i, p := range pc.Plugs {
		out[i] = PlugAt{at.At.Add(Orient(pc.Cells[p.C], int(at.Rot))), OrientSide(int(p.S), int(at.Rot)), p.K, p.Arm}
	}
	return out
}

// board is a laid hex's room as a grid, for working out joins without
// allocating: which slot covers each cell, and every laid plug.
type board struct {
	occ   [roomN]int8
	plugs [Slots][maxPlugs]PlugAt
	np    [Slots]uint8
}

const maxPlugs = 16

func (h *Hex) fill(b *board) {
	for i := range b.occ {
		b.occ[i] = -1
	}
	for s := 0; s < Slots; s++ {
		b.np[s] = 0
		pc := PieceOf(h.Slots[s])
		at := h.Lay.Pos[s]
		if pc == nil || !at.On {
			continue
		}
		if h.Lay.Wired && !h.Lay.Touch {
			if i := roomIndex(at.At); i >= 0 {
				b.occ[i] = int8(s)
			}
			continue
		}
		if h.Lay.Wired {
			// The silhouette as it is: pieces on the open board never turn.
			for _, c := range pc.Cells {
				if i := roomIndex(at.At.Add(c)); i >= 0 {
					b.occ[i] = int8(s)
				}
			}
			continue
		}
		for _, c := range pc.Cells {
			if i := roomIndex(at.At.Add(Orient(c, int(at.Rot)))); i >= 0 {
				b.occ[i] = int8(s)
			}
		}
		for _, p := range pc.Plugs {
			if int(b.np[s]) < maxPlugs {
				b.plugs[s][b.np[s]] = PlugAt{at.At.Add(Orient(pc.Cells[p.C], int(at.Rot))), OrientSide(int(p.S), int(at.Rot)), p.K, p.Arm}
				b.np[s]++
			}
		}
	}
}

func (b *board) at(c Cell) int {
	if i := roomIndex(c); i >= 0 {
		return int(b.occ[i])
	}
	return -1
}

// PieceAt is the slot whose piece covers c, or -1.
func (h *Hex) PieceAt(c Cell) int {
	var b board
	h.fill(&b)
	return b.at(c)
}

// CanPlace reports whether piece s would fit at p: inside the room, on
// no other piece.
func (h *Hex) CanPlace(s int, p Place) bool {
	var b board
	h.fill(&b)
	return h.canPlace(&b, s, p)
}

func (h *Hex) canPlace(b *board, s int, p Place) bool {
	pc := PieceOf(h.Slots[s])
	if pc == nil {
		return false
	}
	if h.Lay.Wired {
		o := b.at(p.At)
		return h.Lay.Room.Has(p.At) && (o < 0 || o == s)
	}
	for _, c := range pc.Cells {
		c = p.At.Add(Orient(c, int(p.Rot)))
		if !h.Lay.Room.Has(c) {
			return false
		}
		if o := b.at(c); o >= 0 && o != s {
			return false
		}
	}
	return true
}

// Place lays piece s at p if it fits.
func (h *Hex) Place(s int, p Place) bool {
	p.On = true
	p.Rot %= Orients
	if s < 0 || s >= Slots || !h.CanPlace(s, p) {
		return false
	}
	h.Lay.Pos[s] = p
	h.Lay.Kept &^= bit(s)
	return true
}

// Lift takes piece s off the room (the hex keeps the part), with its
// wires.
func (h *Hex) Lift(s int) {
	h.Lay.Pos[s] = Place{}
	if h.Lay.Wired {
		h.unwireAll(s)
	}
}

// SetAside takes piece s off the room and keeps it off: the hex holds
// the part, and it stays out until it's laid again.
func (h *Hex) SetAside(s int) {
	if s < 0 || s >= Slots {
		return
	}
	h.Lift(s)
	if h.Slots[s].Kind != SlotEmpty {
		h.Lay.Kept |= bit(s)
	}
}

// TurnPiece turns piece s a sixth clockwise in place (after six turns,
// it flips), or on to the next way that fits. It reports whether it
// turned.
func (h *Hex) TurnPiece(s int) bool {
	p := h.Lay.Pos[s]
	if !p.On || h.Lay.Wired {
		return false
	}
	for k := 1; k < Orients; k++ {
		q := p
		q.Rot = (p.Rot + uint8(k)) % Orients
		if h.CanPlace(s, q) {
			h.Lay.Pos[s] = q
			return true
		}
	}
	return false
}

// Link is one join between two laid pieces.
type Link struct {
	From, To int // the sending piece and the taking one
	Kind     PlugKind
	Arm      uint8
	Cell     Cell // the sending plug's cell
	Side     int  // the side it faces
}

// Links are every join on the room, in a fixed order: senders in slot
// order, their plugs in order. A join that would close a loop is left
// out, so the hex always runs.
func (h *Hex) Links() []Link {
	var b board
	h.fill(&b)
	var out []Link
	h.eachLink(&b, func(l Link) { out = append(out, l) })
	return out
}

// eachLink is every join: the wires on the open board, or where edges
// meet on a layout from before wires.
func (h *Hex) eachLink(b *board, each func(Link)) {
	if h.Lay.Wired {
		h.wireLinks(each)
		return
	}
	links(b, each)
}

func links(b *board, each func(Link)) {
	var reach [Slots]uint16 // bit b: s reaches b along links so far
	for s := range reach {
		reach[s] = bit(s)
	}
	for a := 0; a < Slots; a++ {
		for _, p := range b.plugs[a][:b.np[a]] {
			if !p.K.Out() {
				continue
			}
			to := p.Cell.Add(CellDirs[p.Side])
			o := b.at(to)
			if o < 0 || o == a {
				continue
			}
			back := (p.Side + 3) % 6
			ok := false
			for _, q := range b.plugs[o][:b.np[o]] {
				if q.Cell == to && q.Side == back && q.K.Takes(p.K) {
					ok = true
					break
				}
			}
			if !ok {
				continue
			}
			// The graph edge: a feeds o, except a boost, where the part
			// it scales feeds the multiplier.
			from, into := a, o
			if p.K == PlugBoost {
				from, into = o, a
			}
			if reach[into]&bit(from) != 0 {
				continue // would loop
			}
			for s := range reach {
				if reach[s]&bit(from) != 0 {
					reach[s] |= reach[into]
				}
			}
			each(Link{a, o, p.K, p.Arm, p.Cell, p.Side})
		}
	}
}

// lay is a laid hex's wiring, as a graph with every slot in it, plus
// which pieces hang off which gate arms and which pieces run.
type lay struct {
	g    Graph
	arm  [Slots]Route
	runs uint16
	// joins counts each piece's links, for laying pieces out.
	joins [Slots]uint8
}

func (h *Hex) wire() *lay {
	w := &lay{}
	var b board
	h.fill(&b)
	h.wireOn(&b, w)
	return w
}

func (h *Hex) wireOn(b *board, w *lay) {
	*w = lay{}
	w.g.N = Slots
	for s := range w.arm {
		w.arm[s].Gate = -1
	}
	for s := 0; s < Slots; s++ {
		w.g.At[s] = h.Lay.Pos[s].At
	}
	trig := -1
	for s := 0; s < Slots; s++ {
		if sl := h.Slots[s]; sl.Kind == SlotPart && sl.Part.Kind == KindTrigger && h.Lay.Pos[s].On {
			trig = s
			break
		}
	}
	h.eachLink(b, func(l Link) {
		from, into := l.From, l.To
		if l.Kind == PlugBoost {
			from, into = l.To, l.From
		}
		w.g.Out[from] |= bit(into)
		w.joins[l.From]++
		w.joins[l.To]++
		if l.Arm != ArmNone && w.arm[into].Gate < 0 {
			w.arm[into] = Route{int8(l.From), Branch(l.Arm)}
		}
	})
	// What runs: everything the trigger reaches. A hex with no trigger
	// (one nested, docked or called) runs every laid piece, the ones
	// nothing feeds on what it's handed.
	if trig >= 0 {
		w.g.Entry = uint8(trig)
		w.runs = bit(trig)
		for changed := true; changed; {
			changed = false
			for a := 0; a < Slots; a++ {
				if w.runs&bit(a) != 0 && w.g.Out[a]&^w.runs != 0 {
					w.runs |= w.g.Out[a]
					changed = true
				}
			}
		}
	} else {
		for s := 0; s < Slots; s++ {
			if h.Lay.Pos[s].On && h.Slots[s].Kind != SlotEmpty {
				w.runs |= bit(s)
			}
		}
		for s := 0; s < Slots; s++ {
			if w.runs&bit(s) != 0 && w.g.Ins(s) == 0 {
				w.g.Entry = uint8(s)
				break
			}
		}
	}
}

// Runs reports whether laid piece s runs: the trigger reaches it.
func (h *Hex) Runs(s int) bool {
	if !h.Laid() {
		return h.Shape.Has(s)
	}
	return h.wired().runs&bit(s) != 0
}

// Graph is how the hex's slots feed each other: its shape's graph, or for
// a laid hex the one its plugs make.
func (h *Hex) Graph() *Graph {
	if !h.Laid() {
		return h.Shape.Graph()
	}
	return &h.wired().g
}

// pad is the piece covering the cell where the shape puts slot i (a
// port, a loop-back, a quirk), or -1: on a laid hex the shape's slots are
// pads on the room.
func (h *Hex) pad(i int) int {
	if i < 0 {
		return -1
	}
	if !h.Laid() {
		return i
	}
	g := h.Shape.Graph()
	if !g.Has(i) {
		return -1
	}
	return h.PieceAt(g.At[i])
}

// Pad is where on the room the shape's slot i sits (its pad cell).
func (h *Hex) Pad(i int) Cell { return h.Shape.Graph().At[i] }

// Loose are the laid hex's parts not on the room.
func (h *Hex) Loose() []int {
	var out []int
	for s := 0; s < Slots; s++ {
		if h.Slots[s].Kind != SlotEmpty && !h.Lay.Pos[s].On {
			out = append(out, s)
		}
	}
	return out
}

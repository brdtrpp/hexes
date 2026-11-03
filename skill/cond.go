package skill

import (
	"errors"
	"fmt"
	"strings"
)

// Conditions. A gate (or a watch trigger) can hold a condition the
// player builds from tiles instead of one fixed test:
//
//	Cond    = Term { "or" Term } .
//	Term    = Factor { "and" Factor } .
//	Factor  = [ "not" ] ( Sensor Compare | "has" Type | "(" Cond ")" ) .
//	Compare = ( "<" | ">" | "==" ) Number .
//	Sensor  = "hull" | "heat" | "power" | "distance" | "crowd" "(" Radius ")" | "bank" Bank .
//
// "and" binds before "or", as in Go. A compare tile holds its number. If
// any sensor reads a foe (hull, distance, crowd, has), the gate tests
// each foe and splits the group; if every one reads the rig or a bank, it
// tests once and the whole group goes one way.
//
// Tiles are learned once and kept (the game tracks which); a condition
// is just which tiles in which order, so it costs nothing in a save that
// doesn't use one.

// TileKind is one tile of a condition.
type TileKind uint8

const (
	TileNone TileKind = iota
	// Sensors: each reads one number.
	TileHull  // the foe's hull, as a share of its max
	TileHeat  // the rig's Heat, as a share of max
	TilePower // the rig's Power, as a share of max
	TileDist  // hexes from the caster to the foe
	TileCrowd // foes within V hexes of the foe, counting it
	TileHas   // the foe suffers type T's status (true or false, no compare)
	TileBank  // what bank B holds
	// Compares: each holds the number it compares against in V.
	TileLess
	TileMore
	TileEq
	// Operators.
	TileAnd
	TileOr
	TileNot
	TileOpen
	TileClose
	NumTiles
)

var tileNames = [...]string{"", "hull", "heat", "power", "distance", "crowd", "has", "bank",
	"<", ">", "==", "and", "or", "not", "(", ")"}

func (k TileKind) String() string {
	if int(k) < len(tileNames) {
		return tileNames[k]
	}
	return "?"
}

// Sensor reports whether the tile reads a value.
func (k TileKind) Sensor() bool { return k >= TileHull && k <= TileBank }

// Compare reports whether the tile is a comparison.
func (k TileKind) Compare() bool { return k >= TileLess && k <= TileEq }

// Foe reports whether a sensor reads the foe rather than the rig.
func (k TileKind) Foe() bool {
	return k == TileHull || k == TileDist || k == TileCrowd || k == TileHas
}

// Fraction reports whether the sensor reads a share of a max (0 to 1).
func (k TileKind) Fraction() bool { return k == TileHull || k == TileHeat || k == TilePower }

// Tier is how rare the tile is as loot.
func (k TileKind) Tier() Rarity {
	switch k {
	case TileHas, TileNot:
		return Uncommon
	case TileOr, TileEq, TileBank:
		return Rare
	case TileOpen, TileClose:
		return Exotic
	}
	return Common
}

// TileBit is a tile's bit in a set of known tiles.
func TileBit(k TileKind) uint32 { return 1 << k }

// StarterTiles is what a new rig knows: hull and crowd, < and >, and.
const StarterTiles = 1<<TileHull | 1<<TileCrowd | 1<<TileLess | 1<<TileMore | 1<<TileAnd

// AllTiles is every tile.
const AllTiles = (1<<NumTiles - 1) &^ 1

// Tile is one tile in a condition. V is a compare's number or crowd's
// radius; T is has's type; B is bank's bank.
type Tile struct {
	K TileKind
	V float32 `json:",omitempty"`
	T TypeID  `json:",omitempty"`
	B uint8   `json:",omitempty"`
}

// MaxCondTiles is the most tiles one condition holds; MaxSensors the
// most sensors.
const (
	MaxCondTiles = 16
	MaxSensors   = 4
)

// Condition costs, in Heat per firing.
const (
	SensorHeat   = 0.5
	OperatorHeat = 0.25
)

// Cond is a condition's tiles, from the front; the first empty one ends it.
type Cond [MaxCondTiles]Tile

// Len is how many tiles it has.
func (c Cond) Len() int {
	for i, t := range c {
		if t.K == TileNone {
			return i
		}
	}
	return MaxCondTiles
}

// Tiles is the condition's tiles.
func (c *Cond) Tiles() []Tile { return c[:c.Len()] }

// Uses is the set of tiles the condition needs.
func (c Cond) Uses() uint32 {
	var u uint32
	for _, t := range c.Tiles() {
		u |= TileBit(t.K)
	}
	return u
}

// Insert puts t at i (0 to Len), and reports false when full.
func (c *Cond) Insert(i int, t Tile) bool {
	n := c.Len()
	if n == MaxCondTiles || i < 0 || i > n || t.K == TileNone {
		return false
	}
	copy(c[i+1:n+1], c[i:n])
	c[i] = t
	return true
}

// Delete takes out tile i.
func (c *Cond) Delete(i int) {
	n := c.Len()
	if i < 0 || i >= n {
		return
	}
	copy(c[i:], c[i+1:n])
	c[n-1] = Tile{}
}

// CondOp is one step of a compiled condition, in postfix: a leaf (a
// sensor and its compare, or has) or an operator.
type CondOp struct {
	K   TileKind // the sensor, TileHas, or TileAnd, TileOr, TileNot
	Cmp TileKind // a sensor's compare
	V   float32  // crowd's radius
	X   float32  // the number compared against
	T   TypeID
	B   uint8
}

// CondProg is a compiled condition.
type CondProg struct {
	Ops [MaxCondTiles]CondOp
	N   uint8
	// Foe is set when a sensor reads the foe, so the gate tests each one.
	Foe bool
	// Sensors and Opers count leaves and and/or/not, for the cost.
	Sensors, Opers uint8
}

// Heat is what testing the condition costs per firing.
func (p *CondProg) Heat() float32 {
	return SensorHeat*float32(p.Sensors) + OperatorHeat*float32(p.Opers)
}

// Compile parses the tiles into postfix, or says in plain words what's
// wrong with them.
func (c Cond) Compile() (CondProg, error) {
	ps := condParser{t: c.Tiles()}
	if len(ps.t) == 0 {
		return CondProg{}, errors.New("the condition is empty")
	}
	ps.cond()
	if ps.err == nil && ps.i < len(ps.t) {
		ps.fail("there's a %s with nothing to join it to", ps.t[ps.i].K)
	}
	if ps.err == nil && ps.p.Sensors > MaxSensors {
		ps.err = fmt.Errorf("a condition holds %d sensors at most", MaxSensors)
	}
	return ps.p, ps.err
}

type condParser struct {
	t   []Tile
	i   int
	p   CondProg
	err error
}

func (ps *condParser) fail(f string, a ...any) {
	if ps.err == nil {
		ps.err = fmt.Errorf(f, a...)
	}
}

func (ps *condParser) peek() TileKind {
	if ps.i < len(ps.t) {
		return ps.t[ps.i].K
	}
	return TileNone
}

func (ps *condParser) emit(op CondOp) {
	if int(ps.p.N) < MaxCondTiles {
		ps.p.Ops[ps.p.N] = op
		ps.p.N++
	}
	switch op.K {
	case TileAnd, TileOr, TileNot:
		ps.p.Opers++
	default:
		ps.p.Sensors++
		ps.p.Foe = ps.p.Foe || op.K.Foe()
	}
}

func (ps *condParser) cond() {
	ps.term()
	for ps.err == nil && ps.peek() == TileOr {
		ps.i++
		ps.term()
		ps.emit(CondOp{K: TileOr})
	}
}

func (ps *condParser) term() {
	ps.factor()
	for ps.err == nil && ps.peek() == TileAnd {
		ps.i++
		ps.factor()
		ps.emit(CondOp{K: TileAnd})
	}
}

func (ps *condParser) factor() {
	neg := false
	if ps.peek() == TileNot {
		ps.i++
		neg = true
	}
	k := ps.peek()
	switch {
	case k == TileOpen:
		ps.i++
		ps.cond()
		if ps.err == nil && ps.peek() != TileClose {
			ps.fail("a ( is never closed")
		}
		ps.i++
	case k == TileHas:
		t := ps.t[ps.i]
		ps.i++
		if ps.peek().Compare() {
			ps.fail("has is already true or false, so it takes no compare")
		}
		ps.emit(CondOp{K: TileHas, T: t.T})
	case k.Sensor():
		s := ps.t[ps.i]
		ps.i++
		if !ps.peek().Compare() {
			ps.fail("%s needs a <, > or == after it", k)
			return
		}
		c := ps.t[ps.i]
		ps.i++
		ps.emit(CondOp{K: k, Cmp: c.K, V: s.V, X: c.V, B: s.B})
	case k == TileNone:
		ps.fail("the condition ends too soon")
	default:
		ps.fail("a %s needs a sensor before it", k)
	}
	if neg {
		ps.emit(CondOp{K: TileNot})
	}
}

// Text is the condition written out, for the editor and the manual.
func (c Cond) Text(reg *Registry) string {
	var b strings.Builder
	for i, t := range c.Tiles() {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(t.Text(reg, c.sensorBefore(i)))
	}
	return b.String()
}

// sensorBefore is the sensor a compare at i reads, for how to print its
// number.
func (c Cond) sensorBefore(i int) TileKind {
	if i > 0 {
		return c[i-1].K
	}
	return TileNone
}

// Text is one tile in a few characters. sensor is the tile before a
// compare, which says whether its number is a share or a count.
func (t Tile) Text(reg *Registry, sensor TileKind) string {
	switch t.K {
	case TileCrowd:
		return fmt.Sprintf("crowd(%g)", t.V)
	case TileHas:
		if reg != nil {
			return "has " + reg.Get(t.T).Name
		}
		return "has"
	case TileBank:
		return "bank " + BankName(t.B)
	case TileLess, TileMore, TileEq:
		if sensor.Fraction() {
			return fmt.Sprintf("%s %.0f%%", t.K, t.V*100)
		}
		return fmt.Sprintf("%s %g", t.K, t.V)
	}
	return t.K.String()
}

// PresetCond is the condition a fixed gate kind means, so the editor can
// open an old gate as tiles. ok is false for gates that aren't
// conditions (spend, empty).
func PresetCond(p Part) (c Cond, ok bool) {
	switch p.Gate {
	case GateHull:
		c[0], c[1] = Tile{K: TileHull}, Tile{K: TileLess, V: p.Value}
	case GateHeat:
		c[0], c[1] = Tile{K: TileHeat}, Tile{K: TileMore, V: p.Value}
	case GatePower:
		c[0], c[1] = Tile{K: TilePower}, Tile{K: TileMore, V: p.Value}
	case GateStatus:
		c[0] = Tile{K: TileHas, T: p.Type}
	case GateCrowd:
		// A crowd gate wants Count others: crowd counts the foe itself.
		c[0], c[1] = Tile{K: TileCrowd, V: p.Value}, Tile{K: TileMore, V: float32(p.Count)}
	case GateCond:
		return p.Cond, true
	default:
		return c, false
	}
	return c, true
}

// WithCond returns the gate holding condition c.
func (p Part) WithCond(c Cond) Part {
	p.Gate, p.Cond = GateCond, c
	return p
}

// condValue reads one sensor. ok is false when it can't be read (a foe
// sensor with no live foe).
func (e *Evaluator) condValue(w World, ctx *Ctx, op *CondOp, id EntityID) (v float32, ok bool) {
	r := ctx.Rig
	switch op.K {
	case TileHeat:
		if r == nil || r.MaxHeat <= 0 {
			return 0, true
		}
		return r.Heat / r.MaxHeat, true
	case TilePower:
		if r == nil || r.MaxPower <= 0 {
			return 0, true
		}
		return r.Power / r.MaxPower, true
	case TileBank:
		return float32(e.bankRead(ctx, op.B)), true
	}
	u := w.Unit(id)
	if u == nil || !u.Alive {
		return 0, false
	}
	switch op.K {
	case TileHull:
		if u.MaxHP <= 0 {
			return 0, false
		}
		return u.HP / u.MaxHP, true
	case TileCrowd:
		return float32(w.Select(TargetAdjacent, id, ctx.Caster, op.V, e.scratch[:]) + 1), true
	case TileDist:
		if d, ok := w.(Distancer); ok {
			return float32(d.Distance(ctx.Caster, id)), true
		}
	}
	return 0, true
}

// Distancer is a World that can say how far apart two entities are, in
// hexes. The distance sensor needs it.
type Distancer interface {
	Distance(a, b EntityID) int
}

// condLeaf tests one leaf of a condition.
func (e *Evaluator) condLeaf(w World, ctx *Ctx, op *CondOp, id EntityID) bool {
	if op.K == TileHas {
		if e.Reg.Get(op.T).Status == StatusCloak {
			u := w.Unit(ctx.Caster)
			return u != nil && u.Stealth > 0
		}
		u := w.Unit(id)
		return u != nil && u.Alive && e.hasStatus(u, op.T)
	}
	v, ok := e.condValue(w, ctx, op, id)
	if !ok {
		return false
	}
	switch op.Cmp {
	case TileLess:
		return v < op.X
	case TileMore:
		return v > op.X
	case TileEq:
		tol := float32(0.5)
		if op.K.Fraction() {
			tol = 0.025
		}
		d := v - op.X
		return d >= -tol && d <= tol
	}
	return false
}

// testCond runs a compiled condition for one foe (or for the rig, with
// id NoEntity).
func (e *Evaluator) testCond(w World, ctx *Ctx, cp *CondProg, id EntityID) bool {
	var st [MaxCondTiles]bool
	n := 0
	for i := 0; i < int(cp.N); i++ {
		op := &cp.Ops[i]
		switch op.K {
		case TileAnd:
			if n >= 2 {
				n--
				st[n-1] = st[n-1] && st[n]
			}
		case TileOr:
			if n >= 2 {
				n--
				st[n-1] = st[n-1] || st[n]
			}
		case TileNot:
			if n >= 1 {
				st[n-1] = !st[n-1]
			}
		default:
			if n < MaxCondTiles {
				st[n] = e.condLeaf(w, ctx, op, id)
				n++
			}
		}
	}
	return n > 0 && st[n-1]
}

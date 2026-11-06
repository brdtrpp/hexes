package game

import (
	"math"

	"hexes/loot"
	"hexes/skill"
)

// The skill grid: every component in the game on one big honeycomb, rolled
// from the save's seed, that you buy your way across with Data. The middle
// cell is the starter hex. Each ring out opens at a depth (with the
// teaching ladder for the first eleven) and leans on one new idea, the
// rest being copies of what came before at better rarity. The six wedges
// each lean mildly toward one damage type. The grid only changes on a buy,
// a death or a new ring, never during play, so the game step stays
// allocation free.

// NodeKind is what a grid cell holds.
type NodeKind uint8

const (
	NodeVoid  NodeKind = iota // nothing: bends paths
	NodeStart                 // the starter hex in the middle
	NodePart                  // one part for the locker
	NodeFrame                 // one frame for the rack
	NodeShape                 // one generated shape
	NodeTile                  // one logic tile
	NodeBoard                 // one more board cell
)

// Node is one cell of the grid.
type Node struct {
	At     skill.Coord
	Ring   int
	Wedge  int
	Kind   NodeKind
	Rarity skill.Rarity
	// Teach marks a copy of its ring's new idea.
	Teach bool
	Part  skill.Part
	Frame skill.Frame
	Shape skill.Shape
	Tile  skill.TileKind
}

// Grid tuning. Every number is a starting value, one constant to change.
const (
	GridRings     = 14   // rings on a new save
	GridGrowFrom  = 30   // past this depth...
	GridGrowEvery = 10   // ...one more ring every this many depths
	GridVoidFrac  = 0.12 // cells cut as voids, ring GridVoidFrom and out
	GridVoidFrom  = 3
	GridTeachFrac = 0.4 // at least this share of a ring is its new idea
	GridLean      = 0.4 // share of a wedge's typed effects that are its type
	GridTries     = 24  // void re-rolls before a ring goes without voids

	DataStart     = 15 // a new save's Data: one ring 1 node
	DataSmall     = 2  // grunt or skitter kill
	DataBrute     = 5  // brute kill
	DataElite     = 12 // a foe above the floor's level
	DataDepthBase = 30 // first time reaching a depth: this...
	DataDepthStep = 5  // ...plus this per depth
	DataSalvage   = 10 // recovering a salvage crate
	DataScrap     = 3  // scrapping a pack part or a frame
	DataOldFloor  = 3  // kills more than this many floors above your best pay half

	CostBase  = 15 // a ring 1 node
	CostRing  = 8  // more per ring past 1
	CostBoard = 40 // a board cell keystone
	BoardKeys = 3  // board keystones on ring 10, 120 degrees apart

	ShrinkFrac = 0.08 // a death lets go of this share of owned cells...
	ShrinkMin  = 2    // ...at least this many
	ShrinkMax  = 10   // ...at most this many
)

// CostRarity is the price multiplier for each rarity.
var CostRarity = [skill.NumRarities]float64{1, 1.25, 1.6, 2}

// ladderRings is how many rings follow the ladder: ring k (1 to 11) opens
// when lesson k is taught.
const ladderRings = 11

// ringDepths is where rings 12 to 14 open.
var ringDepths = [...]int{20, 25, 30}

// RingDepth is the depth that opens ring k: the ladder's depth for the
// first eleven, then 20, 25, 30 and one more every ten past that.
func RingDepth(k int) int {
	switch {
	case k <= 0:
		return 1
	case k <= ladderRings:
		return Ladder[k].Depth
	case k-ladderRings-1 < len(ringDepths):
		return ringDepths[k-ladderRings-1]
	}
	return GridGrowFrom + GridGrowEvery*(k-GridRings)
}

// RingsFor is how many rings a save whose deepest floor is best has.
func RingsFor(best int) int {
	return GridRings + max(0, (best-GridGrowFrom)/GridGrowEvery)
}

// RingIdea is the one line a ring's new idea gets when it opens.
func RingIdea(k int) string {
	switch k {
	case 0:
		return "your starter hex"
	case 1:
		return "effects: the other five damage types"
	case 2:
		return "targets: around, nearest, cone and chain"
	case 3:
		return "triggers: on hit, on kill, on hurt and periodic"
	case 4:
		return "multipliers"
	case 5:
		return "Mend, Purge and Shield"
	case 6:
		return "frames"
	case 7:
		return "shapes"
	case 8:
		return "gates"
	case 9:
		return "charges, drains, spend and empty gates"
	case 10:
		return "calls and board cells"
	}
	return "everything: quirks, exotics and logic tiles"
}

// Grid is a save's whole grid.
type Grid struct {
	Seed  uint64
	Rings int
	Nodes []Node
	// Types is each wedge's damage type lean.
	Types [6]skill.TypeID
	index map[skill.Coord]int
}

// RingStart is the index of ring k's first cell: rings are stored inside
// out, each from its west corner clockwise, so growing the grid never
// moves a cell.
func RingStart(k int) int {
	if k <= 0 {
		return 0
	}
	return 3*k*(k-1) + 1
}

// GridCells is how many cells a grid of r rings has.
func GridCells(r int) int { return RingStart(r + 1) }

// Index is the node at c, or -1 off the grid.
func (g *Grid) Index(c skill.Coord) int {
	if i, ok := g.index[c]; ok {
		return i
	}
	return -1
}

// Near calls f for each grid neighbor of node i.
func (g *Grid) Near(i int, f func(j int)) {
	c := g.Nodes[i].At
	for _, d := range skill.HexDirs {
		if j := g.Index(c.Add(d)); j >= 0 {
			f(j)
		}
	}
}

// Cost is what node i costs to buy.
func (g *Grid) Cost(i int) int {
	n := &g.Nodes[i]
	switch n.Kind {
	case NodeVoid, NodeStart:
		return 0
	case NodeBoard:
		return CostBoard
	}
	base := CostBase + CostRing*max(0, n.Ring-1)
	return int(math.Round(float64(base) * CostRarity[n.Rarity]))
}

// ringRarity is the rarity weights on ring k: common only for the first
// three, then better the farther out.
func ringRarity(k int) [skill.NumRarities]float32 {
	switch {
	case k <= 3:
		return [skill.NumRarities]float32{1, 0, 0, 0}
	case k <= 6:
		u := 0.3 + 0.05*float32(k-4)
		return [skill.NumRarities]float32{1 - u, u, 0, 0}
	case k <= 10:
		r := 0.18 + 0.04*float32(k-7)
		return [skill.NumRarities]float32{0.82 - 0.32 - r, 0.32, r, 0}
	}
	x := min(0.2, 0.12+0.01*float32(k-11))
	return [skill.NumRarities]float32{0.46 - x, 0.32, 0.22, x}
}

// Idea is a kind of thing a ring can hold.
type idea uint8

const (
	ideaEffect idea = iota
	ideaTarget
	ideaTrigger
	ideaMult
	ideaSustain
	ideaFrame
	ideaShape
	ideaGate
	ideaCharge
	ideaCall
	ideaWild // ring 11 and out: a quirked part, an exotic, a logic tile
	numIdeas
)

// ideaRing is the ring each idea is new on; copies only show up farther
// out.
var ideaRing = [numIdeas]int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}

// copyWeights is how often each taught idea is copied onto a later ring.
var copyWeights = [numIdeas]float32{30, 18, 6, 16, 8, 6, 3, 10, 8, 6, 8}

// newIdea is ring k's new idea.
func newIdea(k int) idea {
	if k >= ideaRing[ideaWild] {
		return ideaWild
	}
	for i, r := range ideaRing {
		if r == k {
			return idea(i)
		}
	}
	return ideaEffect
}

// gridGen is the state of one generation.
type gridGen struct {
	g     *Grid
	reg   *skill.Registry
	tab   *loot.Table
	plasm skill.TypeID
}

// GenGrid rolls the grid for a save seed with r rings. Each ring rolls
// from its own stream, inside out, so a grid with more rings has the
// same inner rings as one with fewer.
func GenGrid(reg *skill.Registry, seed uint64, r int) *Grid {
	r = max(1, r)
	g := &Grid{Seed: seed, Rings: r, Nodes: make([]Node, GridCells(r)), index: make(map[skill.Coord]int, GridCells(r))}
	gen := gridGen{g: g, reg: reg, tab: loot.NewTable(reg), plasm: reg.MustID("plasma")}
	// The wedges' lean: the damage types shuffled.
	tr := loot.NewRNG(hash(seed ^ 0x9e1d7))
	var types [skill.MaxTypes]skill.TypeID
	nt := reg.Len()
	for i := 0; i < nt; i++ {
		types[i] = skill.TypeID(i)
	}
	for i := nt - 1; i > 0; i-- {
		j := tr.Intn(i + 1)
		types[i], types[j] = types[j], types[i]
	}
	for w := range g.Types {
		g.Types[w] = types[w%nt]
	}
	// Lay out the cells.
	g.Nodes[0] = Node{Kind: NodeStart}
	g.index[skill.Coord{}] = 0
	for k := 1; k <= r; k++ {
		c := skill.HexDirs[4]
		c = skill.Coord{Q: c.Q * k, R: c.R * k}
		i := RingStart(k)
		for side := 0; side < 6; side++ {
			for j := 0; j < k; j++ {
				g.Nodes[i] = Node{At: c, Ring: k, Wedge: side, Kind: NodePart}
				g.index[c] = i
				i++
				c = c.Add(skill.HexDirs[side])
			}
		}
	}
	for k := 1; k <= r; k++ {
		gen.ring(k)
	}
	return g
}

// ring rolls ring k: voids that keep every cell reachable, then the new
// idea's copies (at least one per pair of wedges), then the rest, then
// the guarantees.
func (gen *gridGen) ring(k int) {
	g := gen.g
	lo, hi := RingStart(k), RingStart(k+1)
	gen.voids(k)
	r := loot.NewRNG(hash(g.Seed ^ uint64(k)*0x51ed27f3 ^ 0xc0ffee))
	open := make([]int, 0, hi-lo)
	for i := lo; i < hi; i++ {
		if g.Nodes[i].Kind != NodeVoid {
			open = append(open, i)
		}
	}
	// The new idea: one per pair of wedges first, then random cells up
	// to its share.
	teach := make([]bool, hi-lo)
	need := int(math.Ceil(GridTeachFrac * float64(len(open))))
	got := 0
	for p := 0; p < 3; p++ {
		var in []int
		for _, i := range open {
			if w := g.Nodes[i].Wedge; w == 2*p || w == 2*p+1 {
				in = append(in, i)
			}
		}
		if len(in) > 0 {
			teach[in[r.Intn(len(in))]-lo] = true
			got++
		}
	}
	for got < need {
		i := open[r.Intn(len(open))]
		if !teach[i-lo] {
			teach[i-lo] = true
			got++
		}
	}
	nw := newIdea(k)
	for _, i := range open {
		n := &g.Nodes[i]
		id := nw
		if !teach[i-lo] {
			id = gen.copyIdea(&r, k)
		}
		n.Teach = teach[i-lo]
		gen.fill(&r, n, id, teach[i-lo])
	}
	gen.guarantees(&r, k)
}

// copyIdea picks a taught idea to copy onto ring k.
func (gen *gridGen) copyIdea(r *loot.RNG, k int) idea {
	var w [numIdeas]float32
	any := false
	for i := range w {
		if ideaRing[i] < k {
			w[i] = copyWeights[i]
			any = true
		}
	}
	if !any {
		return newIdea(k)
	}
	return idea(pickW(r, w[:]))
}

// voids cuts ring k's voids. Any cut that would leave a cell of rings 0
// to k unreachable from the middle without going farther out is rolled
// again; after GridTries the ring goes without.
func (gen *gridGen) voids(k int) {
	if k < GridVoidFrom {
		return
	}
	g := gen.g
	lo, hi := RingStart(k), RingStart(k+1)
	n := int(math.Round(GridVoidFrac * float64(hi-lo)))
	for try := 0; try < GridTries; try++ {
		r := loot.NewRNG(hash(g.Seed ^ uint64(k)*0x2545f491 ^ uint64(try)*0x9e37))
		for i := lo; i < hi; i++ {
			g.Nodes[i].Kind = NodePart
		}
		for cut := 0; cut < n; {
			i := lo + r.Intn(hi-lo)
			if g.Nodes[i].Kind != NodeVoid {
				g.Nodes[i].Kind = NodeVoid
				cut++
			}
		}
		if gen.reachable(k) && gen.wedgesOpen(k) {
			return
		}
	}
	for i := lo; i < hi; i++ {
		g.Nodes[i].Kind = NodePart
	}
}

// wedgesOpen reports whether every wedge of ring k has a cell left, so a
// guarantee always has somewhere to go.
func (gen *gridGen) wedgesOpen(k int) bool {
	var has [6]bool
	for i := RingStart(k); i < RingStart(k+1); i++ {
		if gen.g.Nodes[i].Kind != NodeVoid {
			has[gen.g.Nodes[i].Wedge] = true
		}
	}
	return has == [6]bool{true, true, true, true, true, true}
}

// reachable reports whether every cell of rings 0 to k that isn't a void
// connects to the middle through rings 0 to k.
func (gen *gridGen) reachable(k int) bool {
	return Reachable(gen.g, k)
}

// Reachable reports whether every non-void cell of rings 0 to k connects
// to the middle through cells of rings 0 to k.
func Reachable(g *Grid, k int) bool {
	hi := RingStart(min(k, g.Rings) + 1)
	seen := make([]bool, hi)
	q := make([]int, 0, hi)
	seen[0] = true
	q = append(q, 0)
	for len(q) > 0 {
		i := q[0]
		q = q[1:]
		g.Near(i, func(j int) {
			if j < hi && !seen[j] && g.Nodes[j].Kind != NodeVoid {
				seen[j] = true
				q = append(q, j)
			}
		})
	}
	for i := 0; i < hi; i++ {
		if !seen[i] && g.Nodes[i].Kind != NodeVoid {
			return false
		}
	}
	return true
}

// rarity rolls a rarity for ring k.
func rollRarityW(r *loot.RNG, w [skill.NumRarities]float32) skill.Rarity {
	return skill.Rarity(pickW(r, w[:]))
}

func pickW(r *loot.RNG, w []float32) int {
	var sum float32
	for _, x := range w {
		sum += x
	}
	if sum <= 0 {
		return 0
	}
	x := r.Float() * sum
	for i, v := range w {
		if x < v {
			return i
		}
		x -= v
	}
	return len(w) - 1
}

// fill rolls node n as idea id.
func (gen *gridGen) fill(r *loot.RNG, n *Node, id idea, teach bool) {
	k := n.Ring
	rar := rollRarityW(r, ringRarity(k))
	n.Rarity = rar
	switch id {
	case ideaFrame:
		n.Kind = NodeFrame
		f := skill.RollFrame(r.Next, rar, gen.reg.Len(), r.Next()&^testFrameBit)
		if f.Typed {
			f.Type = gen.g.Types[n.Wedge]
		}
		n.Frame = f
	case ideaShape:
		n.Kind = NodeShape
		n.Shape = skill.RollShape(r.Next, r.Next()&^testShapeBit, max(ShapeMinDepth, RingDepth(k)))
	case ideaWild:
		if r.Float() < 0.18 {
			n.Kind, n.Rarity = NodeTile, skill.Rare
			n.Tile = gen.tile(r)
			if n.Tile == skill.TileNone {
				n.Kind, n.Rarity = NodePart, rar
				n.Part = gen.part(r, n, ideaEffect, rar, true)
			} else {
				n.Rarity = n.Tile.Tier()
			}
			return
		}
		n.Kind = NodePart
		pick := [...]idea{ideaEffect, ideaEffect, ideaMult, ideaTrigger, ideaEffect, ideaMult}
		if rar < skill.Rare {
			rar = skill.Rare
			n.Rarity = rar
		}
		n.Part = gen.part(r, n, pick[r.Intn(len(pick))], rar, true)
		n.Rarity = n.Part.Rarity
	default:
		n.Kind = NodePart
		n.Part = gen.part(r, n, id, rar, teach)
		n.Rarity = n.Part.Rarity
	}
}

// tile rolls a logic tile outside the starter set, or none if there's
// nothing to find.
func (gen *gridGen) tile(r *loot.RNG) skill.TileKind {
	var can [skill.NumTiles]skill.TileKind
	n := 0
	for k := skill.TileKind(1); k < skill.NumTiles; k++ {
		if skill.StarterTiles&skill.TileBit(k) == 0 && skill.AllTiles&skill.TileBit(k) != 0 {
			can[n] = k
			n++
		}
	}
	if n == 0 {
		return skill.TileNone
	}
	return can[r.Intn(n)]
}

var newTargets = [...]skill.TargetKind{skill.TargetAdjacent, skill.TargetNearest, skill.TargetCone, skill.TargetChain}
var newTriggers = [...]skill.TriggerKind{skill.OnHit, skill.OnKill, skill.OnHurt, skill.Periodic}

// part rolls a part of idea id at rar on node n. teach makes it the
// ring's new idea exactly (ring 1's non-plasma damage, ring 2's four
// targets, ring 3's four triggers).
func (gen *gridGen) part(r *loot.RNG, n *Node, id idea, rar skill.Rarity, teach bool) skill.Part {
	k := n.Ring
	t := gen.tab
	wild := k >= ideaRing[ideaWild]
	t.Allow = loot.Allow{Limit: true, Sustain: false, Spend: k >= ideaRing[ideaCharge], Quirks: wild}
	var kind skill.PartKind
	ok := func(skill.Part) bool { return true }
	switch id {
	case ideaEffect:
		kind = skill.KindEffect
		if teach && k == 1 {
			ok = func(p skill.Part) bool { return p.Effect == skill.EffectDamage && p.Type != gen.plasm }
		} else {
			ok = func(p skill.Part) bool { return !p.Effect.Sustain() }
		}
	case ideaSustain:
		kind = skill.KindEffect
		t.Allow.Sustain = true
		rar = max(rar, skill.Uncommon) // sustain never rolls common
		ok = func(p skill.Part) bool { return p.Effect.Sustain() }
	case ideaTarget:
		kind = skill.KindTarget
		if teach && k == 2 {
			want := newTargets[r.Intn(len(newTargets))]
			ok = func(p skill.Part) bool { return p.Target == want }
		}
	case ideaTrigger:
		kind = skill.KindTrigger
		if teach && k == 3 {
			want := newTriggers[r.Intn(len(newTriggers))]
			rar = want.Tier()
			ok = func(p skill.Part) bool { return p.Trigger == want }
		} else if rar == skill.Common || (rar == skill.Exotic && !wild) {
			rar = skill.Uncommon // a common trigger is only the cast trigger
		}
	case ideaMult:
		kind = skill.KindMult
	case ideaGate:
		kind = skill.KindGate
		if k == ideaRing[ideaGate] {
			ok = func(p skill.Part) bool { return p.Gate != skill.GateSpend && p.Gate != skill.GateEmpty }
		}
	case ideaCharge:
		kind = skill.KindCharge
		if r.Float() < 0.4 {
			kind = skill.KindGate
			ok = func(p skill.Part) bool { return p.Gate == skill.GateSpend || p.Gate == skill.GateEmpty }
		}
	case ideaCall:
		kind = skill.KindCall
	}
	// Exotics come only as effects, multipliers and triggers.
	if rar == skill.Exotic && kind != skill.KindEffect && kind != skill.KindMult && kind != skill.KindTrigger {
		rar = skill.Rare
	}
	if rar == skill.Exotic && !wild {
		rar = skill.Rare
	}
	t.Allow.Kinds = loot.KindBit(kind)
	var p skill.Part
	for try := 0; try < GridTries; try++ {
		p = t.RollAt(r, rar)
		if p.Kind == kind && ok(p) {
			break
		}
	}
	if !ok(p) || p.Kind != kind {
		p = gen.fallback(id, k, rar)
	}
	// The wedge's lean: a typed effect or status gate takes the wedge's
	// type GridLean of the time.
	if (p.Kind == skill.KindEffect && (p.Effect == skill.EffectDamage || p.Effect == skill.EffectStatus)) ||
		(p.Kind == skill.KindGate && p.Gate == skill.GateStatus) {
		if r.Float() < GridLean && !(teach && k == 1 && gen.g.Types[n.Wedge] == gen.plasm) {
			ty := gen.g.Types[n.Wedge]
			if p.Kind == skill.KindGate && gen.reg.Get(ty).Status == skill.StatusCloak {
				return p
			}
			p.Type = ty
		}
	}
	return p
}

// fallback is a plain part of idea id when the table never rolled one.
func (gen *gridGen) fallback(id idea, k int, rar skill.Rarity) skill.Part {
	var p skill.Part
	switch id {
	case ideaTarget:
		p = skill.NewTarget(skill.TargetAdjacent, 2, 0)
	case ideaTrigger:
		p = skill.NewTrigger(skill.OnHit)
		return p.WithRarity(skill.OnHit.Tier())
	case ideaMult:
		p = skill.NewMult(skill.VerbDamage, 1.5)
	case ideaSustain:
		p = skill.NewMend(0)
	case ideaGate:
		p = skill.NewGate(skill.GateHull, 0.4)
	case ideaCharge:
		p = skill.NewCharge(0, 1)
	case ideaCall:
		p = skill.NewCall(skill.CallSelf)
	default:
		p = skill.NewDamage(gen.reg.MustID("cryo"), 15)
	}
	return p.WithRarity(min(rar, skill.Rare))
}

// guarantees places what every grid must have: a cryo effect beside the
// start, an around target next to ring 1, a trigger in every wedge of
// ring 3, and the board keystones on ring 10, 120 degrees apart.
func (gen *gridGen) guarantees(r *loot.RNG, k int) {
	g := gen.g
	lo, hi := RingStart(k), RingStart(k+1)
	switch k {
	case 1:
		i := lo + r.Intn(hi-lo)
		n := &g.Nodes[i]
		n.Kind, n.Rarity, n.Teach = NodePart, skill.Common, true
		n.Part = skill.NewDamage(gen.reg.MustID("cryo"), 15)
	case 2:
		i := lo + r.Intn(hi-lo)
		n := &g.Nodes[i]
		n.Kind, n.Rarity, n.Teach = NodePart, skill.Common, true
		n.Part = skill.NewTarget(skill.TargetAdjacent, 2, 0)
	case 3:
		for w := 0; w < 6; w++ {
			has := false
			var cand []int
			for i := lo; i < hi; i++ {
				n := &g.Nodes[i]
				if n.Wedge != w || n.Kind == NodeVoid {
					continue
				}
				cand = append(cand, i)
				has = has || (n.Kind == NodePart && n.Part.Kind == skill.KindTrigger)
			}
			if !has && len(cand) > 0 {
				n := &g.Nodes[cand[r.Intn(len(cand))]]
				n.Kind, n.Teach = NodePart, true
				n.Part = gen.part(r, n, ideaTrigger, skill.Common, true)
				n.Rarity = n.Part.Rarity
			}
		}
	case ideaRing[ideaCall]:
		off := r.Intn(k)
		for b := 0; b < BoardKeys; b++ {
			i := lo + (2*b)*k + off
			g.Nodes[i] = Node{At: g.Nodes[i].At, Ring: k, Wedge: g.Nodes[i].Wedge, Kind: NodeBoard, Rarity: skill.Rare, Teach: true}
		}
	}
}

// GridCheck reports whether a grid meets its promises: every cell
// reachable, ring 1 next to the start holds a cryo effect, ring 2 an
// around target, and each wedge's first three rings an effect and a
// trigger.
func GridCheck(g *Grid, reg *skill.Registry) bool {
	if !Reachable(g, g.Rings) {
		return false
	}
	cryo := reg.MustID("cryo")
	hasCryo, hasAround := false, false
	var eff, trig [6]bool
	for i := 1; i < RingStart(min(3, g.Rings)+1); i++ {
		n := &g.Nodes[i]
		if n.Kind != NodePart {
			continue
		}
		p := n.Part
		switch {
		case n.Ring == 1 && p.Kind == skill.KindEffect && p.Effect == skill.EffectDamage && p.Type == cryo:
			hasCryo = true
		case n.Ring == 2 && p.Kind == skill.KindTarget && p.Target == skill.TargetAdjacent:
			hasAround = true
		}
		eff[n.Wedge] = eff[n.Wedge] || p.Kind == skill.KindEffect
		trig[n.Wedge] = trig[n.Wedge] || p.Kind == skill.KindTrigger
	}
	all := [6]bool{true, true, true, true, true, true}
	return hasCryo && hasAround && eff == all && trig == all
}

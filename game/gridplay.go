package game

import (
	"errors"
	"fmt"

	"hexes/loot"
	"hexes/skill"
)

// The grid in play: what you own, what you can see, Data in and out,
// buying, the locker, signal fragments and the shrink a death costs.

// newGrid rolls a fresh save's grid and owns the middle.
func (s *Sim) newGrid() {
	s.GridKept = s.GridKept[:0]
	s.Grid = s.genGrid(RingsFor(0))
	n := len(s.Grid.Nodes)
	s.Own, s.Seen, s.Glimpse, s.Reclaim = make([]bool, n), make([]bool, n), make([]bool, n), make([]bool, n)
	s.Own[0], s.Seen[0] = true, true
	s.Data, s.Unbanked = DataStart, 0
	s.Locker = s.Locker[:0]
	s.Signals, s.BoardKeys, s.GridNewest, s.GridRingsSeen = 0, 0, 0, 0
	s.refreshGrid()
}

// growGrid adds rings when the deepest floor calls for more. The inner
// rings come out the same, so nothing owned moves.
func (s *Sim) growGrid() {
	if s.Grid == nil {
		s.newGrid()
		return
	}
	want := RingsFor(s.Best)
	if want <= s.Grid.Rings {
		return
	}
	s.Grid = s.genGrid(want)
	n := len(s.Grid.Nodes)
	grow := func(b []bool) []bool { return append(b, make([]bool, n-len(b))...) }
	s.Own, s.Seen, s.Glimpse, s.Reclaim = grow(s.Own), grow(s.Seen), grow(s.Glimpse), grow(s.Reclaim)
	s.refreshGrid()
}

// RingOpen reports whether ring k can be bought on: the first eleven open
// with their ladder lesson, the rest at their depth. The test switch
// opens every ring.
func (s *Sim) RingOpen(k int) bool {
	switch {
	case k <= 0 || s.TestShapes:
		return true
	case k <= ladderRings:
		return s.taught() > k
	}
	return s.Best >= RingDepth(k)
}

// RingsOpen is how many rings are open, counting out from the middle.
func (s *Sim) RingsOpen() int {
	k := 0
	for k < s.Grid.Rings && s.RingOpen(k+1) {
		k++
	}
	return k
}

// NextRingDepth is the depth the next sealed ring opens at, or 0 when
// every ring is open.
func (s *Sim) NextRingDepth() int {
	k := s.RingsOpen()
	if k >= s.Grid.Rings {
		return 0
	}
	return RingDepth(k + 1)
}

// refreshGrid works out each node's steps from what you own and marks the
// open cells next to you as seen for good.
func (s *Sim) refreshGrid() {
	g := s.Grid
	n := len(g.Nodes)
	if cap(s.gridDist) < n {
		s.gridDist = make([]int16, n)
	}
	d := s.gridDist[:n]
	q := make([]int, 0, n)
	for i := range d {
		d[i] = -1
		if s.Own[i] {
			d[i] = 0
			q = append(q, i)
		}
	}
	for len(q) > 0 {
		i := q[0]
		q = q[1:]
		g.Near(i, func(j int) {
			if d[j] < 0 {
				d[j] = d[i] + 1
				q = append(q, j)
			}
		})
	}
	s.gridDist = d
	for i := range d {
		if d[i] == 1 && s.RingOpen(g.Nodes[i].Ring) {
			s.Seen[i] = true
		}
	}
}

// GridDist is node i's steps from the nearest node you own.
func (s *Sim) GridDist(i int) int { return int(s.gridDist[i]) }

// View is how a grid cell shows.
type View uint8

const (
	ViewDark    View = iota // fog: nothing shows
	ViewGlow                // only its rarity glows through the fog
	ViewKind                // a signal fragment showed what kind it is
	ViewCard                // seen: the full card
	ViewSealed              // next to you on a ring that isn't open
	ViewVoid                // an empty cell you've come next to
	ViewOwned               // yours
	ViewReclaim             // let go in a death: half price to take back
)

// GlowSteps is how far out each rarity glows through the fog: never for
// common, 2 steps for uncommon, 3 for rare, 4 for exotic.
var GlowSteps = [skill.NumRarities]int{1, 2, 3, 4}

// CellView is how node i shows now.
func (s *Sim) CellView(i int) View {
	n := &s.Grid.Nodes[i]
	d := s.GridDist(i)
	switch {
	case s.Own[i]:
		return ViewOwned
	case n.Kind == NodeVoid:
		if s.Seen[i] || d == 1 || s.TestShapes {
			return ViewVoid
		}
		return ViewDark
	case d == 1 && !s.RingOpen(n.Ring):
		return ViewSealed
	case s.Reclaim[i]:
		return ViewReclaim
	case s.Seen[i] || s.TestShapes:
		return ViewCard
	case s.Glimpse[i]:
		return ViewKind
	case d > 0 && d <= GlowSteps[n.Rarity]:
		return ViewGlow
	}
	return ViewDark
}

// DataTotal is all the Data you can spend.
func (s *Sim) DataTotal() int { return s.Data + s.Unbanked }

// earn adds n Data, unbanked until the next checkpoint.
func (s *Sim) earn(n int) {
	if n > 0 {
		s.Unbanked += n
		s.Dirty = true
	}
}

// bank secures every unbanked Data, at a checkpoint.
func (s *Sim) bank() {
	s.Data += s.Unbanked
	s.Unbanked = 0
}

// KillData is what a kill pays: by the foe, half on a floor more than
// DataOldFloor above your best.
func (s *Sim) KillData(k Kind, elite bool) int {
	n := DataSmall
	switch {
	case elite:
		n = DataElite
	case k == KindBrute:
		n = DataBrute
	}
	if s.Depth < s.Best-DataOldFloor {
		n = max(1, n/2)
	}
	return n
}

// Signal fragment tuning.
const (
	SignalChance      = 0.01 // per kill
	SignalElite       = 0.10 // from a foe above the floor's level
	SignalRadius      = 2    // cells a fragment lights around where you aim it
	ShardMin, ShardMx = 1, 3 // Data a shard pays, where a part used to drop
)

// earnKill pays for a kill and maybe finds a signal fragment. It runs in
// the step, so it doesn't allocate.
func (s *Sim) earnKill(id skill.EntityID) {
	a := &s.Actors[id]
	elite := a.U.Level > s.Depth
	n := s.KillData(a.Kind, elite)
	s.earn(n)
	s.addFX(FX{Kind: FXData, At: a.At, Value: float32(n), Life: 1.0})
	r := loot.NewRNG(loot.Seed(s.Floor.Seed^hash(uint64(s.Visits))^0x5191a1, int(id)))
	c := float32(SignalChance)
	if elite {
		c = SignalElite
	}
	if r.Float() < c {
		s.Signals++
		s.addFX(FX{Kind: FXSignal, At: a.At, Life: 1.8})
	}
}

// CanBuy is what node i costs now and whether you can buy it here: it
// must be next to what you own, on an open ring, and you must be at a
// workbench with the Data.
func (s *Sim) CanBuy(i int) (int, error) {
	if s.Grid == nil || i < 0 || i >= len(s.Grid.Nodes) {
		return 0, errors.New("no such cell")
	}
	n := &s.Grid.Nodes[i]
	cost := s.Grid.Cost(i)
	if s.Reclaim[i] {
		cost = (cost + 1) / 2
	}
	switch {
	case s.Own[i]:
		return cost, errors.New("you own it")
	case n.Kind == NodeVoid:
		return cost, errors.New("nothing there")
	case s.GridDist(i) != 1:
		return cost, errors.New("buy next to what you own")
	case !s.RingOpen(n.Ring):
		return cost, fmt.Errorf("ring %d opens at depth %d", n.Ring, RingDepth(n.Ring))
	case !s.AtBench():
		return cost, errors.New("buy at a workbench")
	case s.DataTotal() < cost:
		return cost, fmt.Errorf("needs %d Data, you have %d", cost, s.DataTotal())
	case !s.Reclaim[i] && n.Kind == NodeFrame && len(s.Frames) >= MaxFrames:
		return cost, errors.New("your frame rack is full: fit or scrap a frame first")
	case !s.Reclaim[i] && n.Kind == NodeShape && len(s.Shapes) >= MaxShapes:
		return cost, errors.New("you can't hold more shapes")
	}
	return cost, nil
}

// Buy buys node i: the Data is spent (unbanked first, so spending banks
// it), the cell is yours for good, and what it holds lands in the locker,
// the rack, the shapes or the board. A reclaimed cell gives nothing new:
// you kept its part when you let it go.
func (s *Sim) Buy(i int) error {
	cost, err := s.CanBuy(i)
	if err != nil {
		return err
	}
	u := min(cost, s.Unbanked)
	s.Unbanked -= u
	s.Data -= cost - u
	s.Own[i] = true
	s.GridNewest = i
	if s.Reclaim[i] {
		s.Reclaim[i] = false
	} else {
		s.grant(&s.Grid.Nodes[i])
	}
	s.refreshGrid()
	s.Dirty = true
	return nil
}

// grant hands over what node n holds.
func (s *Sim) grant(n *Node) {
	switch n.Kind {
	case NodePart:
		s.Locker = append(s.Locker, n.Part)
		if IsSustain(n.Part) {
			s.SustainGiven = true
		}
	case NodeFrame:
		s.Frames = append(s.Frames, n.Frame)
		s.FrameGiven, s.LastFrame = true, n.Frame
	case NodeShape:
		s.Shapes = append(s.Shapes, n.Shape)
		s.ShapeGiven, s.LastShape = true, n.Shape
	case NodeTile:
		s.Tiles |= skill.TileBit(n.Tile)
		s.LastTile = n.Tile
	case NodeBoard:
		s.BoardKeys++
	}
}

// UseSignal spends a signal fragment on node i: every cell within
// SignalRadius of it shows its kind, for good.
func (s *Sim) UseSignal(i int) error {
	if s.Signals <= 0 {
		return errors.New("no signal fragments")
	}
	if i < 0 || i >= len(s.Grid.Nodes) {
		return errors.New("no such cell")
	}
	c := s.Grid.Nodes[i].At
	lit := 0
	for j := range s.Grid.Nodes {
		if skill.Dist(s.Grid.Nodes[j].At, c) <= SignalRadius && !s.Seen[j] && !s.Own[j] && !s.Glimpse[j] {
			s.Glimpse[j] = true
			lit++
		}
	}
	if lit == 0 {
		return errors.New("nothing new to light there")
	}
	s.Signals--
	s.Dirty = true
	return nil
}

// Owned counts the nodes you own.
func (s *Sim) Owned() int {
	n := 0
	for _, o := range s.Own {
		if o {
			n++
		}
	}
	return n
}

// ShrinkCount is how many cells a death lets go of when you own owned:
// ShrinkFrac of them, within ShrinkMin and ShrinkMax, half inside the
// mercy window.
func ShrinkCount(owned int, mercy bool) int {
	n := int(float64(owned)*ShrinkFrac + 0.5)
	n = max(ShrinkMin, min(ShrinkMax, n))
	if mercy {
		n = max(1, n/2)
	}
	return n
}

// shrinkGrid lets go of cells after a death, outside in: farthest from
// the middle first, ties to the cell with the most unowned neighbors.
// The middle never goes, nor a cell whose loss would split what you own.
// Each let-go cell becomes a reclaim cell; what it gave stays yours. It
// returns how many went.
func (s *Sim) shrinkGrid(mercy bool) int {
	want := ShrinkCount(s.Owned(), mercy)
	g := s.Grid
	gone := 0
	for gone < want {
		best, br, bp := -1, -1, -1
		for i := 1; i < len(g.Nodes); i++ {
			if !s.Own[i] {
				continue
			}
			r := g.Nodes[i].Ring
			p := 0
			g.Near(i, func(j int) {
				if !s.Own[j] {
					p++
				}
			})
			p += 6 - nearCount(g, i) // the grid's edge counts as perimeter
			if r < br || (r == br && p <= bp) {
				continue
			}
			if !s.canLetGo(i) {
				continue
			}
			best, br, bp = i, r, p
		}
		if best < 0 {
			break
		}
		s.Own[best] = false
		s.Reclaim[best] = true
		gone++
	}
	if gone > 0 {
		s.refreshGrid()
	}
	return gone
}

func nearCount(g *Grid, i int) int {
	n := 0
	g.Near(i, func(int) { n++ })
	return n
}

// canLetGo reports whether letting go of owned node i leaves what you own
// in one piece.
func (s *Sim) canLetGo(i int) bool {
	g := s.Grid
	s.Own[i] = false
	defer func() { s.Own[i] = true }()
	seen := make([]bool, len(g.Nodes))
	q := []int{0}
	seen[0] = true
	reach := 1
	for len(q) > 0 {
		j := q[0]
		q = q[1:]
		g.Near(j, func(k int) {
			if s.Own[k] && !seen[k] {
				seen[k] = true
				reach++
				q = append(q, k)
			}
		})
	}
	return reach == s.Owned()
}

// gridKinds is what the grid has given you, for drops: the part kinds
// you own a node of (the starter hex counts its cast trigger and plasma),
// whether one is a sustain part, and your best rarity.
func (s *Sim) gridKinds() (kinds uint16, sustain bool, best skill.Rarity) {
	kinds = loot.KindBit(skill.KindEffect) | loot.KindBit(skill.KindTrigger)
	for i, o := range s.Own {
		n := &s.Grid.Nodes[i]
		if !o || n.Kind != NodePart {
			continue
		}
		kinds |= loot.KindBit(n.Part.Kind)
		sustain = sustain || IsSustain(n.Part)
		best = max(best, n.Part.Rarity)
	}
	return kinds, sustain, best
}

// Locker edits. The locker is unlimited and secured: grid parts live
// there, and the workbench pulls from it like the pack.

// PreviewPart says what part p would do in a hex slot, and whether it
// would do anything there.
func (s *Sim) PreviewPart(hex, slot int, p skill.Part) (what string, ok bool) {
	if !s.Load.Has(hex) {
		return "", false
	}
	h := s.Load.Hex(hex)
	if !h.Shape.Has(slot) || h.Slots[slot].Kind == skill.SlotNested {
		return "no slot there", false
	}
	if err := fits(h, slot, p, s.Load.IsMod(hex), s.Reg); err != nil {
		return err.Error(), false
	}
	cp := *h
	cp.Slots[slot] = skill.Slot{Kind: skill.SlotPart, Part: p}
	return cp.Use(slot)
}

// LockerIn puts locker part idx in a hex slot; the slot's old part takes
// its place in the locker.
func (s *Sim) LockerIn(hex, slot, idx int) error {
	if !s.Load.Has(hex) || idx < 0 || idx >= len(s.Locker) || !s.Load.Hex(hex).Shape.Has(slot) {
		return errors.New("no such slot")
	}
	h := s.Load.Hex(hex)
	old := h.Slots[slot]
	if old.Kind == skill.SlotNested {
		return errors.New("that slot holds a nested hex")
	}
	in := s.Locker[idx]
	if err := fits(h, slot, in, s.Load.IsMod(hex), s.Reg); err != nil {
		return err
	}
	if what, ok := s.PreviewPart(hex, slot, in); !ok {
		return useless(h, slot, what)
	}
	h.Slots[slot] = skill.Slot{Kind: skill.SlotPart, Part: in}
	if err := s.recompile(hex); err != nil {
		h.Slots[slot] = old
		return err
	}
	if old.Kind == skill.SlotPart {
		s.Locker[idx] = old.Part
	} else {
		s.Locker = append(s.Locker[:idx], s.Locker[idx+1:]...)
	}
	s.Dirty = true
	return nil
}

// LockerOut takes the part in a hex slot out into the locker.
func (s *Sim) LockerOut(hex, slot int) error {
	if !s.Load.Has(hex) || !s.Load.Hex(hex).Shape.Has(slot) {
		return errors.New("no such slot")
	}
	h := s.Load.Hex(hex)
	old := h.Slots[slot]
	switch {
	case old.Kind == skill.SlotNested:
		return errors.New("that slot holds a nested hex")
	case old.Kind != skill.SlotPart:
		return errors.New("that slot is empty")
	case old.Part.Kind == skill.KindTrigger:
		return ErrNeedsTrigger
	}
	h.Clear(slot)
	if err := s.recompile(hex); err != nil {
		h.Slots[slot] = old
		return err
	}
	s.Locker = append(s.Locker, old.Part)
	s.Dirty = true
	return nil
}

// PackToLocker moves a secured pack part into the locker. A part picked
// up since the last checkpoint has to be secured first.
func (s *Sim) PackToLocker(cell int) error {
	inv := &s.Stash.Inv
	if cell < 0 || cell >= loot.InventorySize || !inv.Has[cell] {
		return errors.New("that cell is empty")
	}
	if s.Stash.Fresh[cell] {
		return fmt.Errorf("it's new: slot it or reach depth %d before it can go in the locker", s.NextCheckpoint())
	}
	p, _ := inv.Take(cell)
	s.Locker = append(s.Locker, p)
	s.Dirty = true
	return nil
}

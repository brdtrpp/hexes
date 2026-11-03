package game

import (
	"errors"
	"fmt"

	"hexes/skill"
)

// The docking board. Every hex you have sits on a cell of a small
// honeycomb: the four equipped ones, which fire on their triggers, and
// modules, hexes with no trigger that only run when something docks into
// them. A hex's ports face its sides (see skill.Shape.Ports); turn it in
// 60 degree steps to point them. When a hex's way out faces the side of
// its neighbor that holds the neighbor's way in, they dock and the chain
// flows across (see skill.Hex.Docks).
//
// The board opens up as you go deeper: a few cells at first, one more
// every few floors of your deepest dive, so a death never takes cells
// away. The test switch opens all of it.

// Board tuning. Placeholder balance, all constants.
const (
	BoardRadius = 2  // cells out from the middle
	BoardCells  = 19 // every cell within BoardRadius
	BoardBase   = 5  // cells open on a new run: room for one module
	BoardEvery  = 3  // one more cell per this many floors of your deepest
	MaxMods     = BoardCells - MaxSlots
)

// BoardOrder is the board's cells in the order they open: the middle,
// then the ring round it from straight up clockwise, then the outer ring.
var BoardOrder = boardOrder()

func boardOrder() (o [BoardCells]skill.Cell) {
	n := 1
	for r := 1; r <= BoardRadius; r++ {
		// Start straight up r steps and walk the ring clockwise.
		c := skill.Cell{Q: 0, R: int8(-r)}
		for side := 0; side < 6; side++ {
			d := skill.CellDirs[(side+2)%6]
			for k := 0; k < r; k++ {
				o[n] = c
				n++
				c = c.Add(d)
			}
		}
	}
	return o
}

// BoardOpen is how many cells are open to a save whose deepest floor is
// best.
func BoardOpen(best int) int {
	return min(BoardCells, BoardBase+max(0, best-1)/BoardEvery)
}

// BoardIndex is where c comes in BoardOrder, or -1 off the board.
func BoardIndex(c skill.Cell) int {
	for i, b := range BoardOrder {
		if b == c {
			return i
		}
	}
	return -1
}

// Spot is where a hex sits on the board and how far it's turned
// clockwise, in sixths.
type Spot struct {
	At  skill.Cell
	Rot uint8 `json:",omitempty"`
}

// Dock is one docked pair: hex From's way out on slot Slot runs hex To.
type Dock struct{ From, Slot, To int }

// Count is how many hexes are on the board: the equipped ones, then the
// modules.
func (l *Loadout) Count() int { return MaxSlots + len(l.Mods) }

// Has reports whether i is a hex on the board.
func (l *Loadout) Has(i int) bool { return i >= 0 && i < l.Count() }

// IsMod reports whether hex i is a module.
func (l *Loadout) IsMod(i int) bool { return i >= MaxSlots && i < l.Count() }

// Hex is hex i: an equipped one or a module.
func (l *Loadout) Hex(i int) *skill.Hex {
	if i < MaxSlots {
		return l.Hexes[i]
	}
	return l.Mods[i-MaxSlots]
}

// Name is hex i's name.
func (l *Loadout) Name(i int) string {
	if i < MaxSlots {
		return l.Names[i]
	}
	return l.Mods[i-MaxSlots].Name
}

// At is the hex sitting on cell c, or -1.
func (l *Loadout) At(c skill.Cell) int {
	for i, s := range l.Spots {
		if s.At == c {
			return i
		}
	}
	return -1
}

// PortSide is the board side a port of hex i faces, with its turn.
func (l *Loadout) PortSide(i int, p skill.Port) int {
	return (int(p.Side) + int(l.Spots[i].Rot)) % 6
}

// Dock works out which hexes dock from where they sit and sets every
// hex's Docks, and returns the pairs.
func (l *Loadout) Dock() []Dock {
	var out []Dock
	for i := 0; i < l.Count(); i++ {
		l.Hex(i).Docks = [skill.Slots]*skill.Hex{}
	}
	l.Link()
	if len(l.Spots) != l.Count() {
		return nil
	}
	for i := 0; i < l.Count(); i++ {
		h := l.Hex(i)
		ps, n := h.Shape.Ports()
		for _, p := range ps[:n] {
			if p.In {
				continue
			}
			side := l.PortSide(i, p)
			j := l.At(l.Spots[i].At.Add(skill.CellDirs[side]))
			if j < 0 {
				continue
			}
			in, ok := l.Hex(j).Shape.InPort()
			if !ok || l.PortSide(j, in) != (side+3)%6 {
				continue
			}
			h.Docks[p.Slot] = l.Hex(j)
			out = append(out, Dock{From: i, Slot: int(p.Slot), To: j})
		}
	}
	return out
}

// Link points every run-now Call, in every hex and every hex nested in
// one, at the module of that name. A call naming no module is left
// pointing at nothing: it compiles and does nothing.
func (l *Loadout) Link() {
	var link func(h *skill.Hex, depth int)
	link = func(h *skill.Hex, depth int) {
		if h == nil || depth > 8 {
			return
		}
		h.Libs = [skill.Slots]*skill.Hex{}
		for s, sl := range h.Slots {
			switch {
			case sl.Kind == skill.SlotNested:
				link(sl.Nested, depth+1)
			case sl.Kind == skill.SlotPart && sl.Part.NeedsLib():
				h.Libs[s] = l.Lib(sl.Part.Lib)
			}
		}
	}
	for i := 0; i < l.Count(); i++ {
		link(l.Hex(i), 0)
	}
}

// Lib is the module called name, or nil.
func (l *Loadout) Lib(name string) *skill.Hex {
	for _, m := range l.Mods {
		if m.Name == name {
			return m
		}
	}
	return nil
}

// LibNames is every module's name, in board order: what a run-now Call
// can name.
func (l *Loadout) LibNames() []string {
	out := make([]string, len(l.Mods))
	for i, m := range l.Mods {
		out[i] = m.Name
	}
	return out
}

// FixSpots makes sure every hex has its own cell on the board. A save
// from before the board, or one whose board doesn't fit its hexes, gets
// the hexes laid out on the first cells, each turned so that nothing
// docks: it plays exactly as it did.
func (l *Loadout) FixSpots() {
	ok := len(l.Spots) == l.Count()
	for i := 0; ok && i < len(l.Spots); i++ {
		ok = BoardIndex(l.Spots[i].At) >= 0 && l.Spots[i].Rot < 6 && l.At(l.Spots[i].At) == i
	}
	if ok {
		return
	}
	l.Spots = make([]Spot, 0, l.Count())
	for i := 0; i < l.Count(); i++ {
		l.Spots = append(l.Spots, Spot{At: BoardOrder[i%BoardCells]})
		best, fewest := 0, 1<<30
		for r := 0; r < 6; r++ {
			l.Spots[i].Rot = uint8(r)
			if n := len(l.Dock()); n < fewest {
				best, fewest = r, n
			}
		}
		l.Spots[i].Rot = uint8(best)
	}
	l.Dock()
}

// Docks is every docked pair on the board right now.
func (s *Sim) Docks() []Dock { return s.Load.Dock() }

// BoardOpen is how many board cells are open now.
func (s *Sim) BoardOpen() int {
	if s.TestShapes {
		return BoardCells
	}
	return BoardOpen(s.Best)
}

// CellOpen reports whether a hex can sit on c now.
func (s *Sim) CellOpen(c skill.Cell) bool {
	i := BoardIndex(c)
	return i >= 0 && i < s.BoardOpen()
}

// NextCellAt is the deepest floor that opens the next board cell, or 0
// when the whole board is open.
func (s *Sim) NextCellAt() int {
	open := BoardOpen(s.Best)
	if open >= BoardCells {
		return 0
	}
	return 1 + (open-BoardBase+1)*BoardEvery
}

// MoveHex puts hex i on cell to, trading places with any hex there.
func (s *Sim) MoveHex(i int, to skill.Cell) error {
	if !s.Load.Has(i) {
		return errors.New("no such hex")
	}
	if !s.CellOpen(to) {
		return fmt.Errorf("that cell opens at depth %d", s.cellOpensAt(to))
	}
	from := s.Load.Spots[i].At
	if from == to {
		return nil
	}
	j := s.Load.At(to)
	old := append([]Spot(nil), s.Load.Spots...)
	s.Load.Spots[i].At = to
	if j >= 0 {
		s.Load.Spots[j].At = from
	}
	return s.boardChanged(old)
}

// RotateHex turns hex i a sixth clockwise.
func (s *Sim) RotateHex(i int) error {
	if !s.Load.Has(i) {
		return errors.New("no such hex")
	}
	old := append([]Spot(nil), s.Load.Spots...)
	s.Load.Spots[i].Rot = (s.Load.Spots[i].Rot + 1) % 6
	return s.boardChanged(old)
}

// cellOpensAt is the deepest floor that opens cell c.
func (s *Sim) cellOpensAt(c skill.Cell) int {
	i := BoardIndex(c)
	if i < 0 {
		return 0
	}
	return 1 + max(0, i-BoardBase+1)*BoardEvery
}

// boardChanged recompiles everything after a move or turn, or puts the
// board back as it was if something won't build.
func (s *Sim) boardChanged(old []Spot) error {
	if err := s.rebuildAll(-1); err != nil {
		s.Load.Spots = old
		s.Load.Dock()
		return err
	}
	return nil
}

// ModuleSockets is the socket layout of a module: wild everywhere, since
// a module has no trigger.
func ModuleSockets() [skill.Slots]skill.SocketKind {
	var s [skill.Slots]skill.SocketKind
	for i := range s {
		s[i] = skill.SocketWild
	}
	return s
}

// AddModule builds an empty module on cell at, a plain Triangle, and
// returns its hex number.
func (s *Sim) AddModule(at skill.Cell) (int, error) {
	switch {
	case !s.CellOpen(at):
		return -1, fmt.Errorf("that cell opens at depth %d", s.cellOpensAt(at))
	case s.Load.At(at) >= 0:
		return -1, errors.New("a hex already sits there")
	case len(s.Load.Mods) >= MaxMods:
		return -1, errors.New("the board is full")
	}
	h := &skill.Hex{Name: s.modName(), Sockets: ModuleSockets(), Shape: skill.Shape{Kind: skill.ShapeTriangle}}
	for i := range h.Linked {
		h.Linked[i] = true
	}
	s.Load.Mods = append(s.Load.Mods, h)
	s.Load.Spots = append(s.Load.Spots, Spot{At: at})
	i := s.Load.Count() - 1
	// Turn it so it docks with whatever is next to it, if any turn does.
	for r := 0; r < 6; r++ {
		s.Load.Spots[i].Rot = uint8(r)
		docked := false
		for _, d := range s.Load.Dock() {
			docked = docked || d.From == i || d.To == i
		}
		if docked {
			break
		}
		if r == 5 {
			s.Load.Spots[i].Rot = 0
		}
	}
	if err := s.rebuildAll(-1); err != nil {
		s.Load.Mods = s.Load.Mods[:len(s.Load.Mods)-1]
		s.Load.Spots = s.Load.Spots[:len(s.Load.Spots)-1]
		s.Load.Dock()
		return -1, err
	}
	return i, nil
}

// modName is the first free module name: Module A, Module B, and so on.
func (s *Sim) modName() string {
	for k := 0; ; k++ {
		name := fmt.Sprintf("Module %c", 'A'+k%26)
		if k >= 26 {
			name = fmt.Sprintf("Module %d", k+1)
		}
		used := false
		for _, m := range s.Load.Mods {
			used = used || m.Name == name
		}
		if !used {
			return name
		}
	}
}

// RemoveModule takes module i off the board and puts its parts in the
// pack. If the pack can't take them, nothing changes.
func (s *Sim) RemoveModule(i int) error {
	if !s.Load.IsMod(i) {
		return errors.New("only a module can come off the board")
	}
	h := s.Load.Hex(i)
	var parts []skill.Part
	for _, sl := range h.Slots {
		if sl.Kind == skill.SlotPart {
			parts = append(parts, sl.Part)
		}
	}
	inv := &s.Stash.Inv
	if free := len(inv.Has) - inv.Count(); free < len(parts) {
		return fmt.Errorf("free %d pack cells first: its parts go back in your pack", len(parts)-free)
	}
	oldMods := append([]*skill.Hex(nil), s.Load.Mods...)
	oldSpots := append([]Spot(nil), s.Load.Spots...)
	j := i - MaxSlots
	s.Load.Mods = append(s.Load.Mods[:j:j], s.Load.Mods[j+1:]...)
	s.Load.Spots = append(s.Load.Spots[:i:i], s.Load.Spots[i+1:]...)
	if err := s.rebuildAll(-1); err != nil {
		s.Load.Mods, s.Load.Spots = oldMods, oldSpots
		s.Load.Dock()
		return err
	}
	for _, p := range parts {
		inv.Add(p) // came out of a hex, so already safe from death
	}
	return nil
}

// rebuildAll recompiles every hex (docks reach across hexes, so one
// change can change any of them) and hands the new programs to the
// dispatcher. A periodic hex that was edited starts a full period from
// now; the others keep their timers. On error nothing changes.
func (s *Sim) rebuildAll(edited int) error {
	if err := s.Load.Rebuild(s.Reg, s.Depth, s.Rig.MaxDepth, edited); err != nil {
		return err
	}
	s.Dirty = true
	l := s.Disp.Loadout(s.Player)
	if l == nil {
		return nil
	}
	for i := 0; i < MaxSlots; i++ {
		p := s.Load.Progs[i]
		l.Progs[i] = p
		if i == edited {
			l.Cool[i] = 0
			l.StartTimer(i)
		}
	}
	return nil
}

// fixBoard puts any hex sitting on a cell that isn't open (after the test
// switch goes off) on a free open cell. If there aren't enough, it says
// how many modules to take off first.
func (s *Sim) fixBoard(open int) error {
	isOpen := func(c skill.Cell) bool { i := BoardIndex(c); return i >= 0 && i < open }
	var out []int
	for i, sp := range s.Load.Spots {
		if !isOpen(sp.At) {
			out = append(out, i)
		}
	}
	if len(out) == 0 {
		return nil
	}
	free := 0
	for _, c := range BoardOrder[:open] {
		if s.Load.At(c) < 0 {
			free++
		}
	}
	if free < len(out) {
		return fmt.Errorf("take %d modules off the board first: only %d cells stay open", len(out)-free, open)
	}
	old := append([]Spot(nil), s.Load.Spots...)
	for _, i := range out {
		for _, c := range BoardOrder[:open] {
			if s.Load.At(c) < 0 {
				s.Load.Spots[i].At = c
				break
			}
		}
	}
	return s.boardChanged(old)
}

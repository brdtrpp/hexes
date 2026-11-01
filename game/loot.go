package game

import (
	"errors"

	"hexes/loot"
	"hexes/skill"
)

// Loot in the sim: enemies drop parts when they die, the parts lie on the
// floor until the player walks over them, and the player swaps them into
// hexes from the editor. The drop and pickup paths run inside Step and
// don't allocate; the edit calls run while the game is paused and
// recompile the hex they touch.

// Drop tuning. Placeholder balance, all constants.
const (
	DropGrunt   = 0.30 // chance a grunt drops a part
	DropSkitter = 0.22
	DropBrute   = 0.55
	DropElite   = 0.45 // added for foes above the floor's level
	MaxDrops    = 96   // parts that can lie on one floor; more are lost
)

var dropChance = [...]float32{KindGrunt: DropGrunt, KindSkitter: DropSkitter, KindBrute: DropBrute}

// Drop is a part lying on the floor.
type Drop struct {
	At   skill.Coord
	Part skill.Part
	Age  float32 // seconds on the floor, for the pop-in
}

// Stash is the run's loot.
type Stash struct {
	Inv   loot.Inventory
	Drops []Drop
	// Picked counts parts picked up this run.
	Picked int

	table    *loot.Table
	fullAt   skill.Coord // tile where the "full" warning last showed
	fullShow bool
}

func (s *Sim) resetStash() {
	if s.Stash.table == nil {
		s.Stash.table = loot.NewTable(s.Reg)
		s.Stash.Drops = make([]Drop, 0, MaxDrops)
	}
	s.Stash.Inv.Clear()
	s.Stash.Drops = s.Stash.Drops[:0]
	s.Stash.Picked = 0
	s.Stash.fullShow = false
}

// loadLoot runs when a floor loads: parts left on the last floor are gone,
// and the run's hexes (preset on the first floor) compile for the new
// level.
func (s *Sim) loadLoot(level int) {
	s.Stash.Drops = s.Stash.Drops[:0]
	s.Stash.fullShow = false
	if s.Load.Hexes[0] == nil {
		s.Load = BuildLoadout(s.Reg, level, s.Rig.MaxDepth)
		return
	}
	if err := s.Load.Rebuild(s.Reg, level, s.Rig.MaxDepth); err != nil {
		// Every hex compiled when it was last edited; only a lower
		// Processing limit could break one, and the slice's rig is fixed.
		panic(err)
	}
}

// dropLoot rolls a dying enemy's drop. The roll is seeded by the floor and
// the enemy, so a run drops the same parts whatever order things die in.
func (s *Sim) dropLoot(id skill.EntityID) {
	a := &s.Actors[id]
	if int(a.Kind) >= len(dropChance) || len(s.Stash.Drops) == cap(s.Stash.Drops) {
		return
	}
	chance := dropChance[a.Kind]
	if a.U.Level > s.Depth {
		chance += DropElite
	}
	r := loot.NewRNG(loot.Seed(s.Floor.Seed, int(id)))
	p, ok := s.Stash.table.Drop(&r, s.Depth, chance)
	if !ok {
		return
	}
	at, ok := s.dropSpot(a.At)
	if !ok {
		return
	}
	s.Stash.Drops = append(s.Stash.Drops, Drop{At: at, Part: p})
}

// dropSpot is where a part lands: where the foe died, or next to it if
// that tile can't be walked on (a foe that fell into a pit drops on the
// rim).
func (s *Sim) dropSpot(c skill.Coord) (skill.Coord, bool) {
	if s.Floor.At(c).Walkable() {
		return c, true
	}
	for _, d := range skill.HexDirs {
		if n := c.Add(d); s.Floor.At(n).Walkable() {
			return n, true
		}
	}
	return c, false
}

// pickup moves parts under the player into the inventory.
func (s *Sim) pickup(dt float32) {
	p := s.P()
	at := p.At
	if s.Stash.fullShow && s.Stash.fullAt != at {
		s.Stash.fullShow = false
	}
	d := s.Stash.Drops
	for i := 0; i < len(d); {
		d[i].Age += dt
		if d[i].At != at || !p.U.Alive {
			i++
			continue
		}
		if s.Stash.Inv.Add(d[i].Part) < 0 {
			if !s.Stash.fullShow {
				s.Stash.fullShow, s.Stash.fullAt = true, at
				s.addFX(FX{Kind: FXFull, At: at, Life: 1.2})
			}
			i++
			continue
		}
		s.Stash.Picked++
		s.addFX(FX{Kind: FXLoot, At: at, Slot: int(d[i].Part.Rarity), Life: 1.0})
		d[i] = d[len(d)-1]
		d = d[:len(d)-1]
	}
	s.Stash.Drops = d
}

// Edits. Each one checks the sockets, applies the change, recompiles the
// hex and swaps the new program into the dispatcher. A change that won't
// compile is undone and its error says why in plain words.

// ErrNeedsTrigger is returned for an edit that would leave a hex without
// its trigger.
var ErrNeedsTrigger = errors.New("a hex needs a trigger in its top slot")

// SwapIn trades the part in a hex slot with an inventory cell. Either side
// may be empty: an empty cell takes the slot's part out, an empty slot
// takes the cell's part in.
func (s *Sim) SwapIn(hex, slot, cell int) error {
	if hex < 0 || hex >= MaxSlots || slot < 0 || slot >= skill.Slots || cell < 0 || cell >= loot.InventorySize {
		return errors.New("no such slot")
	}
	h := s.Load.Hexes[hex]
	inv := &s.Stash.Inv
	old := h.Slots[slot]
	if old.Kind == skill.SlotNested {
		return errors.New("that slot holds a nested hex")
	}
	in, has := inv.Parts[cell], inv.Has[cell]
	if !has && old.Kind == skill.SlotEmpty {
		return nil
	}
	if has {
		if err := fits(h, slot, in); err != nil {
			return err
		}
		h.Slots[slot] = skill.Slot{Kind: skill.SlotPart, Part: in}
	} else {
		if old.Part.Kind == skill.KindTrigger {
			return ErrNeedsTrigger
		}
		h.Clear(slot)
	}
	if old.Kind == skill.SlotPart {
		inv.Put(cell, old.Part)
	} else {
		inv.Take(cell)
	}
	if err := s.recompile(hex); err != nil {
		h.Slots[slot] = old
		inv.Parts[cell], inv.Has[cell] = in, has
		return err
	}
	return nil
}

// SwapSlots trades two slots of one hex, which rewires it: order and
// neighbors are the whole game.
func (s *Sim) SwapSlots(hex, a, b int) error {
	if hex < 0 || hex >= MaxSlots || a < 0 || a >= skill.Slots || b < 0 || b >= skill.Slots {
		return errors.New("no such slot")
	}
	if a == b {
		return nil
	}
	h := s.Load.Hexes[hex]
	sa, sb := h.Slots[a], h.Slots[b]
	if sa.Kind == skill.SlotNested || sb.Kind == skill.SlotNested {
		return errors.New("that slot holds a nested hex")
	}
	if sa.Kind == skill.SlotPart {
		if err := fits(h, b, sa.Part); err != nil {
			return err
		}
	}
	if sb.Kind == skill.SlotPart {
		if err := fits(h, a, sb.Part); err != nil {
			return err
		}
	}
	h.Slots[a], h.Slots[b] = sb, sa
	if err := s.recompile(hex); err != nil {
		h.Slots[a], h.Slots[b] = sa, sb
		return err
	}
	return nil
}

// MoveCell trades two inventory cells.
func (s *Sim) MoveCell(a, b int) {
	if a < 0 || b < 0 || a >= loot.InventorySize || b >= loot.InventorySize {
		return
	}
	v := &s.Stash.Inv
	v.Parts[a], v.Parts[b] = v.Parts[b], v.Parts[a]
	v.Has[a], v.Has[b] = v.Has[b], v.Has[a]
}

// Scrap throws away the part in an inventory cell.
func (s *Sim) Scrap(cell int) bool {
	_, ok := s.Stash.Inv.Take(cell)
	return ok
}

func fits(h *skill.Hex, slot int, p skill.Part) error {
	if h.Sockets[slot].Accepts(p.Kind) {
		return nil
	}
	switch {
	case p.Kind == skill.KindTrigger:
		return errors.New("triggers only go in the top slot")
	case h.Sockets[slot] == skill.SocketTrigger:
		return errors.New("the top slot only takes triggers")
	}
	return errors.New("that part doesn't fit this slot")
}

// recompile rebuilds one hex at the current level and hands the program to
// the dispatcher. A new periodic trigger starts a full period from now.
func (s *Sim) recompile(hex int) error {
	was := s.Load.Edited[hex]
	s.Load.Edited[hex] = true
	if err := s.Load.Compile(hex, s.Reg, s.Depth, s.Rig.MaxDepth); err != nil {
		s.Load.Edited[hex] = was
		if s.Load.Hexes[hex].Slots[0].Kind == skill.SlotEmpty {
			return ErrNeedsTrigger
		}
		return err
	}
	p := s.Load.Progs[hex]
	if l := s.Disp.Loadout(s.Player); l != nil {
		l.Progs[hex] = p
		l.Cool[hex] = 0
		l.Timer[hex] = 0
		if p.Trigger == skill.Periodic {
			l.Timer[hex] = p.Period
		}
	}
	return nil
}

// HexPower is what a hex costs per firing, for the editor.
func (s *Sim) HexPower(hex int) (power, heat float32) {
	p := s.Load.Progs[hex]
	return p.Power, p.Heat
}

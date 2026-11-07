package game

import (
	"errors"
	"fmt"

	"hexes/skill"
)

// Laying pieces by hand in the editor: move a piece to a cell, turn it,
// or set it aside. Each edit recompiles the hex; a refused one changes
// nothing.

func (s *Sim) laidPiece(hex, slot int) (*skill.Hex, error) {
	if !s.Load.Has(hex) {
		return nil, errors.New("no such hex")
	}
	h := s.Load.Hex(hex)
	if !h.Laid() {
		return nil, errors.New("this hex has no room to lay pieces on")
	}
	if slot < 0 || slot >= skill.Slots || h.Slots[slot].Kind != skill.SlotPart {
		return nil, errors.New("no piece there")
	}
	return h, nil
}

// MovePiece moves piece slot onto tile at. A piece already there swaps
// onto slot's old tile; a piece from the tray comes back wired the way
// its kind usually is. Wires go with the piece.
func (s *Sim) MovePiece(hex, slot int, at skill.Cell) error {
	h, err := s.laidPiece(hex, slot)
	if err != nil {
		return err
	}
	if !h.Lay.Room.Has(at) {
		return errors.New("that tile isn't on this hex's board")
	}
	was := h.Lay
	from := h.Lay.Pos[slot]
	if o := h.PieceAt(at); o >= 0 && o != slot {
		if !from.On {
			return errors.New("a piece is there already: move it first")
		}
		h.Lay.Pos[o].At = from.At
	}
	back := !from.On
	if back && h.Held() >= h.Budget() {
		return fmt.Errorf("the board is full: this shape holds %d pieces", h.Budget())
	}
	h.Lay.Pos[slot] = skill.Place{At: at, On: true}
	h.Lay.Kept &^= 1 << uint(slot)
	if back {
		h.WireIn(slot)
	}
	if err := s.recompile(hex); err != nil {
		h.Lay = was
		return err
	}
	return nil
}

// TurnPiece is gone with the open board: pieces don't turn.
func (s *Sim) TurnPiece(hex, slot int) error {
	return errors.New("pieces don't turn: wire them instead")
}

// WirePiece is a tap from piece a on piece b: it puts a wire on, steps
// it to the sender's next arm (YES, NO; rung I, II, III) or takes it off.
// It reports what's there now: the arm, or 0 for no wire.
func (s *Sim) WirePiece(hex, a, b int) (int, error) {
	h, err := s.laidPiece(hex, a)
	if err != nil {
		return 0, err
	}
	if _, err := s.laidPiece(hex, b); err != nil {
		return 0, err
	}
	was := h.Lay
	arm := h.ToggleWire(a, b)
	if arm < 0 {
		_, why := h.WireWhy(a, b)
		if _, why2 := h.WireWhy(b, a); why == "" {
			why = why2
		}
		if why == "" {
			why = "those two don't wire together"
		}
		return 0, errors.New(why)
	}
	if err := s.recompile(hex); err != nil {
		h.Lay = was
		return 0, err
	}
	return arm, nil
}

// SetPieceAside takes a piece off the room into the hex's tray, where it
// stays until it's laid again. It doesn't run there.
func (s *Sim) SetPieceAside(hex, slot int) error {
	h, err := s.laidPiece(hex, slot)
	if err != nil {
		return err
	}
	if h.Slots[slot].Part.Kind == skill.KindTrigger {
		return errors.New("the trigger starts the hex: it has to stay on the room")
	}
	was := h.Lay
	h.SetAside(slot)
	if err := s.recompile(hex); err != nil {
		h.Lay = was
		return err
	}
	return nil
}

// FreeSlot is the first slot of hex with no part, or -1.
func (s *Sim) FreeSlot(hex int) int {
	if !s.Load.Has(hex) {
		return -1
	}
	h := s.Load.Hex(hex)
	for i := range h.Slots {
		if h.Slots[i].Kind == skill.SlotEmpty && h.HasSlot(i) {
			return i
		}
	}
	return -1
}

// PlaceIn lays the part in pack cell onto the hex with its first cell on
// at, in the hex's first free slot. A part that doesn't fit there stays
// in the pack.
func (s *Sim) PlaceIn(hex, cell int, at skill.Cell) error {
	if !s.Load.Has(hex) || cell < 0 || cell >= len(s.Stash.Inv.Parts) || !s.Stash.Inv.Has[cell] {
		return errors.New("nothing in that cell")
	}
	h := s.Load.Hex(hex)
	if !h.Laid() || !h.Lay.Room.Has(at) {
		return errors.New("that tile isn't on this hex's board")
	}
	slot := s.FreeSlot(hex)
	if slot < 0 {
		return errors.New("this hex holds as many pieces as it can: take one out first")
	}
	inv := &s.Stash.Inv
	in, fresh := inv.Parts[cell], s.Stash.Fresh[cell]
	if err := fits(h, slot, in, s.Load.IsMod(hex), s.Reg); err != nil {
		return err
	}
	if h.Held() >= h.Budget() {
		return fmt.Errorf("the board is full: this shape holds %d pieces", h.Budget())
	}
	old, was := h.Slots[slot], h.Lay
	h.Slots[slot] = skill.Slot{Kind: skill.SlotPart, Part: in}
	if o := h.PieceAt(at); o >= 0 {
		// Dropped on a piece: take the nearest free tile.
		c, ok := h.FreeCell(at)
		if !ok {
			h.Slots[slot], h.Lay = old, was
			return errors.New("no free tile on the board")
		}
		at = c
	}
	h.Lay.Pos[slot] = skill.Place{At: at, On: true}
	h.WireIn(slot)
	inv.Take(cell)
	s.Stash.Fresh[cell] = false
	if err := s.recompile(hex); err != nil {
		h.Slots[slot], h.Lay = old, was
		inv.Parts[cell], inv.Has[cell] = in, true
		s.Stash.Fresh[cell] = fresh
		return err
	}
	return nil
}

package game

import (
	"errors"

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

// MovePiece lays piece slot with its first cell on at, turned the way it
// lies now if it fits that way, or the next way that does.
func (s *Sim) MovePiece(hex, slot int, at skill.Cell) error {
	h, err := s.laidPiece(hex, slot)
	if err != nil {
		return err
	}
	if !h.Lay.Room.Has(at) {
		return errors.New("that cell isn't part of this hex's room")
	}
	was := h.Lay
	rot := h.Lay.Pos[slot].Rot
	for k := uint8(0); k < skill.Orients; k++ {
		if h.Place(slot, skill.Place{At: at, Rot: (rot + k) % skill.Orients}) {
			if err := s.recompile(hex); err != nil {
				h.Lay = was
				return err
			}
			return nil
		}
	}
	return errors.New("it doesn't fit there: try a cell with more room around it")
}

// TurnPiece turns a laid piece a sixth clockwise (on the seventh turn it
// flips over), skipping ways that don't fit.
func (s *Sim) TurnPiece(hex, slot int) error {
	h, err := s.laidPiece(hex, slot)
	if err != nil {
		return err
	}
	was := h.Lay
	if !h.TurnPiece(slot) {
		return errors.New("no room to turn it here")
	}
	if err := s.recompile(hex); err != nil {
		h.Lay = was
		return err
	}
	return nil
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
		return errors.New("that cell isn't part of this hex's room")
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
	old, was := h.Slots[slot], h.Lay
	h.Slots[slot] = skill.Slot{Kind: skill.SlotPart, Part: in}
	placed := false
	for k := uint8(0); k < skill.Orients && !placed; k++ {
		placed = h.Place(slot, skill.Place{At: at, Rot: k})
	}
	if !placed {
		h.Slots[slot], h.Lay = old, was
		return errors.New("it doesn't fit there: try a cell with more room around it")
	}
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

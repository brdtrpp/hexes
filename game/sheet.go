package game

import (
	"errors"
	"strings"

	"hexes/loot"
	"hexes/skill"
)

// PartSheet is a part's item card: its school, its one plain line and
// its numbers.
func PartSheet(p skill.Part, reg *skill.Registry) loot.Sheet {
	sh := loot.PartSheet(p, reg, firstSentence(PlainHelp(p)))
	sh.Sub += "  ·  " + SchoolOf(p, reg).Names()
	return sh
}

// HexSheet is a hex's item card at a level, compiled with its working.
func HexSheet(h *skill.Hex, reg *skill.Registry, level, maxDepth int, blurb string) (loot.Sheet, error) {
	p, w, err := ExplainHex(h, reg, level, maxDepth)
	if err != nil {
		return loot.Sheet{}, err
	}
	if blurb == "" {
		blurb = loot.Summary(h, reg)
	}
	return loot.HexSheet(h, p, w, reg, blurb), nil
}

func firstSentence(s string) string {
	if i := strings.Index(s, ". "); i > 0 {
		return s[:i+1]
	}
	return s
}

// SlotSheet is the card for equipped hex i, at the floor you're on.
func (s *Sim) SlotSheet(i int) (loot.Sheet, error) {
	if i < 0 || i >= MaxSlots || s.Load.Hexes[i] == nil {
		return loot.Sheet{}, errNoHex
	}
	sh, err := HexSheet(s.Load.Hexes[i], s.Reg, s.Depth, s.Rig.MaxDepth, s.Load.Blurb[i])
	if err != nil {
		return sh, err
	}
	sh.Name = s.Load.Names[i]
	return sh, nil
}

// HexCard is the card for hex i, an equipped one or a module.
func (s *Sim) HexCard(i int) (loot.Sheet, error) {
	if i < MaxSlots {
		return s.SlotSheet(i)
	}
	h := s.Load.Hex(i)
	if h == nil {
		return loot.Sheet{}, errNoHex
	}
	sh, err := HexSheet(h, s.Reg, s.Depth, s.Rig.MaxDepth, "")
	if err != nil {
		return sh, err
	}
	sh.Name = s.Load.Name(i)
	return sh, nil
}

var errNoHex = errors.New("no hex there")

// DraftSheet is the card for a floor-end pick that offers a part, from
// the grid or for the locker.
func (s *Sim) DraftSheet(p DraftPick) (loot.Sheet, bool) {
	switch p.Kind {
	case DraftPart:
		return PartSheet(p.Part, s.Reg), true
	case DraftNode:
		if n := &s.Grid.Nodes[p.Node]; n.Kind == NodePart {
			return PartSheet(n.Part, s.Reg), true
		}
	}
	return loot.Sheet{}, false
}

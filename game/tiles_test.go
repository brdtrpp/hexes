package game

import (
	"encoding/json"
	"strings"
	"testing"

	"hexes/loot"
	"hexes/skill"
)

func TestConditionNeedsKnownTiles(t *testing.T) {
	s := New(3)
	// Arc Lance has no gate; put one in a module-free hex slot via the pack.
	cell := s.Stash.Inv.Add(skill.NewGate(skill.GateHull, 0.4))
	slot := -1
	h := s.Load.Hexes[1]
	for i := range h.Slots {
		if h.Shape.Has(i) && h.Slots[i].Kind == skill.SlotEmpty && h.Sockets[i].Accepts(skill.KindGate) {
			h.Slots[i] = skill.Slot{Kind: skill.SlotPart, Part: skill.NewGate(skill.GateHull, 0.4)}
			s.Stash.Inv.Take(cell)
			if err := s.rebuildAll(1); err != nil {
				t.Fatal(err)
			}
			slot = i
			break
		}
	}
	if slot < 0 {
		t.Skip("no slot takes a gate in hex 2")
	}
	var heat skill.Cond
	heat[0], heat[1] = skill.Tile{K: skill.TileHeat}, skill.Tile{K: skill.TileLess, V: 0.3}
	if err := s.SetCond(1, slot, heat); err == nil || !strings.Contains(err.Error(), "heat") {
		t.Fatalf("unknown tile: %v", err)
	}
	var hull skill.Cond
	hull[0], hull[1] = skill.Tile{K: skill.TileHull}, skill.Tile{K: skill.TileLess, V: 0.3}
	if err := s.SetCond(1, slot, hull); err != nil {
		t.Fatal(err)
	}
	s.Tiles |= skill.TileBit(skill.TileHeat)
	if err := s.SetCond(1, slot, heat); err != nil {
		t.Fatal(err)
	}
	b, _ := s.Save()
	var sv Save
	_ = json.Unmarshal(b, &sv)
	o := New(9)
	if err := o.LoadSave(b); err != nil {
		t.Fatal(err)
	}
	if !o.Knows(skill.TileHeat) || o.Load.Hexes[1].Slots[slot].Part.Gate != skill.GateCond {
		t.Fatal("tiles or condition not saved")
	}
}

func TestTileDropsAreNew(t *testing.T) {
	s := New(3)
	for i := 0; i < 200; i++ {
		r := newTestRNG(uint64(i))
		k, ok := s.pickTile(&r)
		if !ok {
			t.Fatal("no tile to drop")
		}
		if s.KnownTiles()&skill.TileBit(k) != 0 && !s.TestShapes {
			t.Fatalf("dropped known tile %s", k)
		}
	}
	s.Tiles = skill.AllTiles
	r := newTestRNG(1)
	if _, ok := s.pickTile(&r); ok {
		t.Fatal("dropped a tile when all are known")
	}
}

func newTestRNG(seed uint64) loot.RNG { return loot.NewRNG(seed) }

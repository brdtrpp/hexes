package game

import (
	"encoding/json"
	"testing"

	"hexes/loot"
	"hexes/skill"
)

func emptyCell(h *skill.Hex) (skill.Cell, bool) {
	for _, c := range h.Lay.Room.Cells() {
		if h.PieceAt(c) < 0 {
			return c, true
		}
	}
	return skill.Cell{}, false
}

func TestLayPiecesByHand(t *testing.T) {
	s := NewVeteran(3)
	h := s.Load.Hexes[0]
	if !h.Laid() {
		t.Fatal("an equipped hex isn't laid")
	}
	calls := func() int {
		n := 0
		for _, st := range s.Load.Progs[0].Steps {
			if st.Op == skill.OpCall {
				n++
			}
		}
		return n
	}
	if calls() != 1 {
		t.Fatal("Arc Lance should call")
	}
	// Set the call aside: it stays off and stops calling.
	if err := s.SetPieceAside(0, 5); err != nil {
		t.Fatal(err)
	}
	if h.Lay.Pos[5].On || calls() != 0 {
		t.Fatal("a piece set aside still runs")
	}
	if err := s.SetPieceAside(0, 0); err == nil {
		t.Fatal("set the trigger aside")
	}
	// Lay it beside the trigger again.
	at := h.Lay.Pos[0].At
	moved := false
	for _, d := range skill.CellDirs {
		if s.MovePiece(0, 5, at.Add(d)) == nil && h.Runs(5) {
			moved = true
			break
		}
	}
	if !moved || calls() != 1 {
		t.Fatal("couldn't lay the call back beside the trigger")
	}
	// Its wire comes off with a tap and goes back on with another.
	from := -1
	for a := 0; a < skill.Slots; a++ {
		if h.WireArm(a, 5) != 0 {
			from = a
		}
	}
	if from < 0 {
		t.Fatal("the call came back with no wire in")
	}
	for i := 0; i < 3 && h.WireArm(from, 5) != 0; i++ {
		if _, err := s.WirePiece(0, from, 5); err != nil {
			t.Fatal(err)
		}
	}
	if calls() != 0 {
		t.Fatal("unwired, the call still runs")
	}
	if arm, err := s.WirePiece(0, from, 5); err != nil || arm == 0 || calls() != 1 {
		t.Fatalf("wiring it back: %d %v", arm, err)
	}
	// A pack part dropped on an empty cell lands there.
	s.Stash.Inv = loot.Inventory{}
	cell := s.Stash.Inv.Add(skill.NewDamage(s.Reg.MustID("kinetic"), 5))
	c, ok := emptyCell(h)
	if !ok {
		t.Fatal("no empty cell")
	}
	if err := s.PlaceIn(0, cell, c); err != nil {
		t.Fatal(err)
	}
	if s.Stash.Inv.Has[cell] || h.PieceAt(c) < 0 {
		t.Fatal("the part didn't land on the cell")
	}
}

// A save from before pieces has hexes with no layout: they come back
// laid out, running as they did.
func TestOldSaveWiresUp(t *testing.T) {
	s := NewVeteran(3)
	want := s.Load.Progs[0].String()
	sv := s.Snapshot()
	b, err := json.Marshal(sv)
	if err != nil {
		t.Fatal(err)
	}
	var old Save
	if err := json.Unmarshal(b, &old); err != nil {
		t.Fatal(err)
	}
	for _, h := range old.Hexes {
		if h != nil {
			h.Lay = skill.Layout{}
		}
	}
	r := New(1)
	if err := r.Restore(old); err != nil {
		t.Fatal(err)
	}
	if !r.Load.Hexes[0].Laid() {
		t.Fatal("an old hex didn't lay out")
	}
	if got := r.Load.Progs[0].String(); got != want {
		t.Fatalf("old save runs\n%s\nnow\n%s", want, got)
	}
}

package main

import (
	"testing"

	"hexes/game"
	"hexes/skill"
)

// On the open board a piece moves by dragging it, goes to the tray when
// dropped there, and a tap on the bar between two touching pieces steps
// their join.
func TestDragPiecesAndTapJoins(t *testing.T) {
	sim := game.New(1)
	h := sim.Load.Hex(0)
	h.EnsureLaid()
	if !h.Lay.Touch {
		t.Fatal("hex 0 isn't on the touch board")
	}
	g := &Game{sim: sim, w: 390 * 3, h: 844 * 3, k: 3}
	g.ed.hex = 0
	l := g.edLayout()
	trig := h.TriggerSlot()
	s := -1
	for i := 0; i < skill.Slots; i++ {
		if i != trig && h.Lay.Pos[i].On && h.Slots[i].Kind == skill.SlotPart {
			s = i
			break
		}
	}
	if s < 0 {
		t.Fatal("no piece to drag")
	}
	// Grab it by its first cell.
	grab := h.PieceCells(s)[0]
	gx, gy := l.cellXY(grab)
	d := g.edGrab(&l, gx, gy)
	if d == nil || d.slot != s {
		t.Fatalf("press on piece %d grabbed %+v", s, d)
	}
	// Somewhere free, away from where it is.
	var to skill.Cell
	found := false
	for _, c := range h.Lay.Room.Cells() {
		if c.Ring() >= 4 && h.FitsAt(s, c) && h.PadAt(c) == skill.PadNone {
			to, found = c, true
			break
		}
	}
	if !found {
		t.Fatal("nowhere free to drop it")
	}
	tx, ty := l.cellXY(to.Add(d.off))
	d.live, d.x, d.y = true, tx, ty
	g.ed.bv.drag = d
	if cells, fits, _, _, ok := g.edGhost(&l, h); !ok || !fits || len(cells) == 0 {
		t.Errorf("ghost over a free spot: ok %v fits %v cells %v", ok, fits, cells)
	}
	g.edDrop(d, tx, ty)
	g.ed.bv.drag = nil
	if !h.Lay.Pos[s].On || h.Lay.Pos[s].At != to {
		t.Fatalf("dropped at %v, piece at %+v (%s)", to, h.Lay.Pos[s], g.ed.msg)
	}
	// Onto the tray: set aside.
	l = g.edLayout()
	px, py := l.cellXY(h.PieceCells(s)[0])
	d = g.edGrab(&l, px, py)
	if d == nil {
		t.Fatal("couldn't grab it again")
	}
	d.live = true
	g.edDrop(d, l.tray.x+l.tray.w/2, l.tray.y+l.tray.h/2)
	if h.Lay.Pos[s].On {
		t.Fatalf("dropped on the tray, still on the board (%s)", g.ed.msg)
	}
	// A tap on a join bar steps it.
	sim = game.New(1)
	g.sim = sim
	h = sim.Load.Hex(0)
	h.EnsureLaid()
	l = g.edLayout()
	spots := g.joinSpots(&l, h)
	var j joinSpot
	for _, sp := range spots {
		if sp.on {
			j = sp
			break
		}
	}
	if !j.on {
		t.Fatalf("no join on the board to tap among %d spots", len(spots))
	}
	before := h.WireArm(j.a, j.b) + 4*h.WireArm(j.b, j.a)
	if !g.edPieceTap(&l, j.x, j.y, -1) {
		t.Fatal("tap on a join bar wasn't taken")
	}
	if after := h.WireArm(j.a, j.b) + 4*h.WireArm(j.b, j.a); after == before {
		t.Errorf("tap on the join between %d and %d left it at %d (%s)", j.a, j.b, after, g.ed.msg)
	}
	if g.edGrab(&l, j.x, j.y) != nil {
		t.Error("a press on a join bar grabs a piece instead of tapping it")
	}
}

package main

// Dragging on the open board. Press a piece and drag it to move it (its
// whole shape has to fit where it lands); drag it onto the tray to set it
// aside; drag a tray piece or a pack part onto the board to lay it, or
// onto a piece to swap it in. A press that never moves is still a tap,
// and a press on empty board drags the board itself (edview.go). Tap the
// bar where two pieces touch to step their join.

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"hexes/skill"
)

// edDrag is what a press carries.
type edDrag struct {
	slot int        // the board or tray piece carried, or -1
	cell int        // the pack cell carried, or -1
	off  skill.Cell // the cell under the pointer, from the piece's anchor
	live bool       // it has moved: it's being carried
	x, y float64    // where the pointer is now
}

// midOff is the cell of a silhouette nearest its middle, so a part
// picked up off the board hangs centred under the finger.
func midOff(cells []skill.Cell) skill.Cell {
	if len(cells) == 0 {
		return skill.Cell{}
	}
	mx, my := 0.0, 0.0
	for _, c := range cells {
		x, y := c.XY()
		mx, my = mx+x, my+y
	}
	mx, my = mx/float64(len(cells)), my/float64(len(cells))
	best, bd := cells[0], math.Inf(1)
	for _, c := range cells {
		x, y := c.XY()
		if d := math.Hypot(x-mx, y-my); d < bd {
			best, bd = c, d
		}
	}
	return best
}

// edTouchBoard is the hex on the open board where pieces join by
// touching, if that's what the editor shows.
func (g *Game) edTouchBoard(l *edLayout) (*skill.Hex, bool) {
	e, sim := &g.ed, g.sim
	if !l.laid || !sim.Load.Has(e.hex) {
		return nil, false
	}
	h := sim.Load.Hex(e.hex)
	return h, h.Lay.Wired && h.Lay.Touch
}

// edGrab is what a press at x, y would carry if it drags: a piece on the
// board or in the tray, or a part in the pack. Nil for anything else.
func (g *Game) edGrab(l *edLayout, x, y float64) *edDrag {
	e, sim := &g.ed, g.sim
	h, ok := g.edTouchBoard(l)
	if !ok || e.pick == pickFrame {
		return nil
	}
	if l.board.hit(x, y) {
		if _, ok := g.edJoinAt(l, h, x, y); ok {
			return nil // a join bar is a tap
		}
		c, ok := l.cellAt(h, x, y)
		if !ok {
			return nil
		}
		s := h.PieceAt(c)
		if s < 0 {
			return nil
		}
		return &edDrag{slot: s, cell: -1, off: c.Sub(h.Lay.Pos[s].At), x: x, y: y}
	}
	if l.tray.hit(x, y) {
		loose := h.Loose()
		for i, t := range l.trayTiles(len(loose)) {
			if t.hit(x, y) {
				s := loose[i]
				return &edDrag{slot: s, cell: -1, off: midOff(h.CellsAt(s, skill.Cell{})), x: x, y: y}
			}
		}
		return nil
	}
	if e.rack || e.locker {
		return nil
	}
	inv := &sim.Stash.Inv
	for i, r := range l.cells {
		if !r.hit(x, y) || !inv.Has[i] {
			continue
		}
		off := skill.Cell{}
		if pc := skill.PieceOf(skill.Slot{Kind: skill.SlotPart, Part: inv.Parts[i]}); pc != nil {
			off = midOff(pc.Cells)
		}
		return &edDrag{slot: -1, cell: i, off: off, x: x, y: y}
	}
	return nil
}

// edDrop puts down what a drag carried, where the pointer let go.
func (g *Game) edDrop(d *edDrag, x, y float64) {
	l := g.edLayout()
	e, sim := &g.ed, g.sim
	h, ok := g.edTouchBoard(&l)
	if !ok {
		return
	}
	c, onBoard := l.cellAt(h, x, y)
	if d.slot >= 0 {
		if l.tray.hit(x, y) {
			if !h.Lay.Pos[d.slot].On {
				return // back where it was
			}
			if err := sim.SetPieceAside(e.hex, d.slot); err != nil {
				e.msg, e.bad = upFirst(err.Error())+".", true
				return
			}
			g.edResult(nil)
			e.msg = "Set aside: it waits in the tray and doesn't run. " + e.msg
			return
		}
		if !onBoard {
			e.pick, e.at = pickSlot, d.slot
			e.msg, e.bad = "Dropped off the board: it stays where it was.", false
			return
		}
		e.pick, e.at = pickSlot, d.slot
		err := sim.MovePiece(e.hex, d.slot, c.Sub(d.off))
		g.edResult(err)
		if err == nil {
			msg := e.msg
			e.pick, e.at = pickSlot, d.slot
			e.msg = "Moved: it joins whatever it touches there. " + msg
		}
		return
	}
	if d.cell < 0 || !onBoard {
		return // the part stays in the pack
	}
	e.pick, e.at = pickCell, d.cell
	if h.PieceAt(c) >= 0 {
		// Onto a piece: the same as tapping it with the part held.
		g.edCellTap(&l, c, -1)
		return
	}
	slot := sim.FreeSlot(e.hex)
	err := sim.PlaceIn(e.hex, d.cell, c.Sub(d.off))
	g.edResult(err)
	if err == nil && slot >= 0 {
		e.pick, e.at = pickSlot, slot
	}
}

// edGhost is where a carried piece would land: its cells, whether it
// fits there, and the piece it would swap with (or -1).
func (g *Game) edGhost(l *edLayout, h *skill.Hex) (cells []skill.Cell, fits bool, swap int, p skill.Part, ok bool) {
	d := g.ed.bv.drag
	swap = -1
	if d == nil || !d.live {
		return nil, false, -1, p, false
	}
	c, on := l.cellAt(h, d.x, d.y)
	if !on {
		return nil, false, -1, p, false
	}
	at := c.Sub(d.off)
	if d.slot >= 0 {
		p = h.Slots[d.slot].Part
		fits = h.FitsAt(d.slot, at) && (h.Lay.Pos[d.slot].On || h.Held() < h.Budget())
		return h.CellsAt(d.slot, at), fits, -1, p, true
	}
	inv := &g.sim.Stash.Inv
	if d.cell < 0 || !inv.Has[d.cell] {
		return nil, false, -1, p, false
	}
	p = inv.Parts[d.cell]
	if t := h.PieceAt(c); t >= 0 {
		return nil, true, t, p, true
	}
	s := g.sim.FreeSlot(g.ed.hex)
	if s < 0 {
		return []skill.Cell{c}, false, -1, p, true
	}
	hc := *h
	hc.Slots[s] = skill.Slot{Kind: skill.SlotPart, Part: p}
	hc.Lift(s)
	return hc.CellsAt(s, at), hc.FitsAt(s, at) && h.Held() < h.Budget(), -1, p, true
}

// drawGhost draws where a carried piece would land on the board: its
// silhouette, green where it fits and red where it doesn't, or the piece
// it would swap with ringed.
func (g *Game) drawGhost(dst *ebiten.Image, l *edLayout, h *skill.Hex) {
	cells, fits, swap, p, ok := g.edGhost(l, h)
	if !ok {
		return
	}
	r := l.slotR
	if swap >= 0 {
		for _, c := range h.PieceCells(swap) {
			cx, cy := l.cellXY(c)
			drawHex(dst, cx, cy, r*0.86, true, color.RGBA{}, colCharge, g.u(3))
		}
		return
	}
	edge := colYes
	if !fits {
		edge = colHot
	}
	in := map[skill.Cell]bool{}
	for _, c := range cells {
		in[c] = true
	}
	for _, c := range cells {
		cx, cy := l.cellXY(c)
		drawHex(dst, cx, cy, r*0.97, true, fade(partCol(p), 0.28), color.RGBA{}, 0)
		for d, dir := range skill.CellDirs {
			if in[c.Add(dir)] {
				continue
			}
			ax, ay, bx, by := sideCorners(cx, cy, r*0.97, d)
			vector.StrokeLine(dst, float32(ax), float32(ay), float32(bx), float32(by), g.u(3), edge, true)
		}
	}
	mx, my := 0.0, 0.0
	for _, c := range cells {
		x, y := l.cellXY(c)
		mx, my = mx+x/float64(len(cells)), my+y/float64(len(cells))
	}
	g.partGlyph(dst, mx, my, r*0.42, p)
}

// drawEdDragTile draws the carried part under the pointer while it's off
// the board, over everything else.
func (g *Game) drawEdDragTile(dst *ebiten.Image, l *edLayout) {
	d := g.ed.bv.drag
	h, ok := g.edTouchBoard(l)
	if d == nil || !d.live || !ok {
		return
	}
	if _, on := l.cellAt(h, d.x, d.y); on {
		return // the ghost on the board shows it
	}
	var p skill.Part
	switch {
	case d.slot >= 0:
		p = h.Slots[d.slot].Part
	case d.cell >= 0 && g.sim.Stash.Inv.Has[d.cell]:
		p = g.sim.Stash.Inv.Parts[d.cell]
	default:
		return
	}
	r := 26 * g.k
	g.partTile(dst, d.x, d.y-r*0.9, r, true, p, true, true)
}

// joinSpot is the middle of an edge two touching pieces share, where a
// tap steps their join.
type joinSpot struct {
	a, b int
	x, y float64
	on   bool // joined now
}

// joinSpots are the edges where touching pieces join or could.
func (g *Game) joinSpots(l *edLayout, h *skill.Hex) []joinSpot {
	var out []joinSpot
	for a := 0; a < skill.Slots; a++ {
		if !h.Lay.Pos[a].On {
			continue
		}
		for _, c := range h.PieceCells(a) {
			for _, dir := range skill.CellDirs {
				b := h.PieceAt(c.Add(dir))
				if b <= a {
					continue // each edge once, from its lower piece
				}
				on := h.WireArm(a, b) != 0 || h.WireArm(b, a) != 0
				if !on {
					if _, _, ok := h.CanWireTouch(a, b); !ok {
						if _, _, ok := h.CanWireTouch(b, a); !ok {
							continue
						}
					}
				}
				ax, ay := l.cellXY(c)
				bx, by := l.cellXY(c.Add(dir))
				out = append(out, joinSpot{a: a, b: b, x: (ax + bx) / 2, y: (ay + by) / 2, on: on})
			}
		}
	}
	return out
}

// edJoinAt is the join bar under x, y, if any.
func (g *Game) edJoinAt(l *edLayout, h *skill.Hex, x, y float64) (joinSpot, bool) {
	ox, oy := l.cellXY(skill.Cell{})
	nx, ny := l.cellXY(skill.CellDirs[0])
	reach := math.Hypot(nx-ox, ny-oy) * 0.3
	best, bd := joinSpot{}, reach
	found := false
	for _, j := range g.joinSpots(l, h) {
		if d := math.Hypot(x-j.x, y-j.y); d < bd {
			best, bd, found = j, d, true
		}
	}
	return best, found
}

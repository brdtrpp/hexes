package main

// The open board: a wired hex's pieces each keep their silhouette, a few
// hexagons that never turn, anywhere on a big board, and pieces only join
// where they touch. Drag a piece to move it or onto the tray to set it
// aside (eddrag.go); tap the bar where two pieces touch to step their
// join (a gate's YES and NO, an overload's rungs, then off). Pads
// scattered over the board give the piece on them a bonus. The board
// drags and zooms (edview.go).

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"hexes/loot"
	"hexes/skill"
)

var rungNames = [...]string{"", "I", "II", "III"}

func wireCol(k skill.PlugKind, arm uint8, rung bool) color.RGBA {
	if rung {
		return colCharge
	}
	return plugCol(k, arm)
}

// drawWired draws a wired hex: the board, its pieces as their
// silhouettes, the joins where they touch, and the tray.
func (g *Game) drawWired(dst, full *ebiten.Image, l *edLayout, h *skill.Hex) {
	e := &g.ed
	r := l.slotR
	picked := -1
	if e.pick == pickSlot && e.at >= 0 && e.at < skill.Slots && h.Lay.Pos[e.at].On {
		picked = e.at
	}
	fit, _ := g.edFits(h)
	drag := -1
	if d := g.ed.bv.drag; d != nil && d.live {
		drag = d.slot
		fit = nil
	}
	// The board. Its pads are marked: whatever piece covers one gets its
	// quirk. A way out with a hex docked on it is a pad too.
	pads := g.boardPads(h)
	b := l.board
	for _, c := range h.Lay.Room.Cells() {
		cx, cy := l.cellXY(c)
		if cx < b.x-r || cy < b.y-r || cx > b.x+b.w+r || cy > b.y+b.h+r {
			continue // out of the window
		}
		edge := colCellEdge
		if _, ok := pads[c]; ok {
			edge = fade(colShape, 0.7)
		}
		drawHex(dst, cx, cy, r*0.96, true, colCell, edge, g.u(1.5))
		if fit[c] && h.PieceAt(c) < 0 {
			// Where the picked piece's first cell can go.
			drawHex(dst, cx, cy, r*0.8, true, fade(colYes, 0.08), fade(colYes, 0.5), g.u(1.5))
		}
	}
	// The pieces: each cell of the silhouette a hexagon in the piece's
	// colour, the outline drawn only where it meets something else.
	run := func(s int) bool { return h.Runs(s) }
	for s := 0; s < skill.Slots; s++ {
		if !h.Lay.Pos[s].On || h.Slots[s].Kind == skill.SlotEmpty {
			continue
		}
		p := h.Slots[s].Part
		col := partCol(p)
		fill, edge := fade(col, 0.32), col
		if !run(s) {
			fill, edge = fade(col, 0.1), fade(colDim, 0.7)
		}
		if _, ok := h.Use(s); !ok {
			edge = colHot
		}
		w := g.u(2.5)
		if s == picked {
			fill, edge, w = fade(col, 0.5), colText, g.u(4)
		}
		if s == drag {
			fill, edge = fade(col, 0.08), fade(colDim, 0.5)
		}
		cells := h.PieceCells(s)
		for _, c := range cells {
			cx, cy := l.cellXY(c)
			drawHex(dst, cx, cy, r*0.97, true, fill, color.RGBA{}, 0)
		}
		for _, c := range cells {
			cx, cy := l.cellXY(c)
			for d, dir := range skill.CellDirs {
				if h.PieceAt(c.Add(dir)) == s {
					continue // inside the piece
				}
				ax, ay, bx, by := sideCorners(cx, cy, r*0.97, d)
				vector.StrokeLine(dst, float32(ax), float32(ay), float32(bx), float32(by), w, edge, true)
			}
		}
	}
	// The joins: a bar across each edge two joined pieces share, in the
	// sender's colour, with YES, NO or the rung on it.
	trig := h.TriggerSlot()
	rungs := trig >= 0 && h.Arms(trig) == 3
	for _, k := range h.Links() {
		if !h.Lay.Pos[k.From].On || !h.Lay.Pos[k.To].On {
			continue
		}
		isRung := rungs && k.From == trig
		col := wireCol(k.Kind, k.Arm, isRung)
		if !h.Runs(k.From) {
			col = fade(col, 0.4)
		}
		tag := ""
		switch {
		case isRung:
			tag = rungNames[min(int(k.Arm), 3)]
		case k.Arm == skill.ArmYes && h.Arms(k.From) == 2:
			tag = "YES"
		case k.Arm == skill.ArmNo:
			tag = "NO"
		}
		tagged := false
		for _, c := range h.PieceCells(k.From) {
			for d, dir := range skill.CellDirs {
				if h.PieceAt(c.Add(dir)) != k.To {
					continue
				}
				cx, cy := l.cellXY(c)
				ax, ay, bx, by := sideCorners(cx, cy, r, d)
				mx, my := (ax+bx)/2, (ay+by)/2
				ux, uy := (bx-ax)*0.36, (by-ay)*0.36
				w := g.u(6)
				if k.From == picked || k.To == picked {
					w = g.u(8)
				}
				vector.StrokeLine(dst, float32(mx-ux), float32(my-uy), float32(mx+ux), float32(my+uy), w, col, true)
				if tag != "" && !tagged {
					tagged = true
					drawHex(dst, mx, my, r*0.26, true, colPanel, col, g.u(1.5))
					g.label(dst, g.small, tag, mx, my, col, true)
				}
			}
		}
	}
	// Where two touching pieces could join but don't: a faint bar to tap.
	for _, j := range g.joinSpots(l, h) {
		if j.on {
			continue
		}
		for _, c := range h.PieceCells(j.a) {
			for d, dir := range skill.CellDirs {
				if h.PieceAt(c.Add(dir)) != j.b {
					continue
				}
				cx, cy := l.cellXY(c)
				ax, ay, bx, by := sideCorners(cx, cy, r, d)
				mx, my := (ax+bx)/2, (ay+by)/2
				if math.Hypot(mx-j.x, my-j.y) > r*0.2 {
					continue // another edge of the same pair
				}
				ux, uy := (bx-ax)*0.3, (by-ay)*0.3
				vector.StrokeLine(dst, float32(mx-ux), float32(my-uy), float32(mx+ux), float32(my+uy), g.u(3), fade(colDim, 0.55), true)
			}
		}
	}
	// Each piece's icon and name in its middle.
	e2 := e
	for s := 0; s < skill.Slots; s++ {
		if !h.Lay.Pos[s].On || h.Slots[s].Kind == skill.SlotEmpty {
			continue
		}
		p := h.Slots[s].Part
		// The middle of the silhouette.
		cells := h.PieceCells(s)
		cx, cy := 0.0, 0.0
		for _, c := range cells {
			x, y := l.cellXY(c)
			cx, cy = cx+x/float64(len(cells)), cy+y/float64(len(cells))
		}
		g.partGlyph(dst, cx, cy-r*0.12, r*0.42, p)
		tc := colText
		if !run(s) {
			tc = fade(colDim, 0.8)
		}
		title, _ := loot.Label(p, g.sim.Reg)
		if p.Kind == skill.KindCall {
			title = "to " + g.callName(e2.hex, p, true)
		}
		g.label(dst, g.small, title, cx, cy+r*0.36, tc, true)
		if tag := h.Frame(s).Tag(); tag != "" {
			g.label(dst, g.small, tag, cx, cy-r*0.66, frameCol(h.Frame(s)), true)
		}
		if p.UsesBank() && p.Bank < skill.MaxBanks {
			g.label(dst, g.small, skill.BankName(p.Bank), cx+r*0.55, cy-r*0.45, colCharge, true)
		}
		if rg := h.RungOf()[s]; rg >= 2 {
			g.label(dst, g.small, rungNames[rg], cx-r*0.55, cy-r*0.45, colCharge, true)
		}
	}
	for c, tag := range pads {
		cx, cy := l.cellXY(c)
		if h.PieceAt(c) < 0 {
			g.label(dst, g.small, tag, cx, cy, fade(colShape, 0.8), true)
		} else {
			// Under a piece: a ring, so the pad still shows.
			drawHex(dst, cx, cy, r*0.3, true, color.RGBA{}, fade(colShape, 0.8), g.u(1.5))
		}
	}
	// Ways in and out: IN on the trigger when a hex docks into this one,
	// OUT on the piece that carries a way out to a docked hex.
	if trig := h.TriggerSlot(); trig >= 0 && h.Lay.Pos[trig].On && dockedInto(g.sim, g.ed.hex) {
		g.pieceBadge(dst, l, h, trig, "IN", colDock)
	}
	for _, o := range g.outPads(h) {
		if s := h.PieceAt(o); s >= 0 {
			g.pieceBadge(dst, l, h, s, "OUT", colDock)
		}
	}
	g.drawGhost(dst, l, h)
	g.drawTray(full, l, h, picked)
}

// drawTray draws the tray of pieces set aside.
func (g *Game) drawTray(dst *ebiten.Image, l *edLayout, h *skill.Hex, picked int) {
	loose := h.Loose()
	g.drawRect(dst, l.tray, colPanel, fade(colDim, 0.35))
	if len(loose) == 0 {
		msg := "TRAY: drag a piece here to set it aside"
		if picked >= 0 && h.Lay.Pos[picked].On {
			msg = "drag the piece here, or tap, to set it aside"
		}
		if l.tray.h > l.tray.w {
			msg = "TRAY"
			if picked >= 0 && h.Lay.Pos[picked].On {
				msg = "SET ASIDE"
			}
		}
		g.label(dst, g.small, msg, l.tray.x+l.tray.w/2, l.tray.y+l.tray.h/2, fade(colDim, 0.8), true)
		return
	}
	for i, t := range l.trayTiles(len(loose)) {
		s := loose[i]
		g.partTile(dst, t.x+t.w/2, t.y+t.h/2, t.w*0.5, true, h.Slots[s].Part, true, s == picked)
	}
}

// edWireTap handles a tap on a wired hex's board when a piece is picked
// and the tap lands on another piece: it wires them if a wire can join
// them, and reports whether it did anything.
func (g *Game) edWireTap(h *skill.Hex, a, t int) bool {
	e, sim := &g.ed, g.sim
	if a < 0 || a >= skill.Slots || !h.Lay.Pos[a].On || t == a {
		return false
	}
	_, _, can := h.CanWireTouch(a, t)
	if !can && h.WireArm(a, t) == 0 && h.WireArm(t, a) == 0 {
		return false
	}
	arm, err := sim.WirePiece(e.hex, a, t)
	if err != nil {
		e.msg, e.bad = upFirst(err.Error())+".", true
		return true
	}
	from := a
	if h.WireArm(t, a) != 0 {
		from = t
	}
	g.edResult(nil)
	e.pick, e.at = pickSlot, a
	switch {
	case arm == 0:
		e.msg = "Join off: they still touch, but nothing passes. Tap again to join them."
	case h.Arms(from) == 2 && arm == skill.ArmYes:
		e.msg = "Joined on YES: tap again for NO."
	case h.Arms(from) == 2:
		e.msg = "Joined on NO: tap again to take it off."
	case h.Arms(from) == 3:
		e.msg = "Joined on rung " + rungNames[min(arm, 3)] + ": tap again for the next rung."
	default:
		e.msg = "Joined: tap again to take it off."
	}
	e.bad = false
	return true
}

// boardPads are the board's pads by tile, with their labels: the rolled
// pads, and each way out that has a hex docked on it.
func (g *Game) boardPads(h *skill.Hex) map[skill.Cell]string {
	pads := map[skill.Cell]string{}
	for _, p := range h.BoardPads() {
		pads[p.At] = p.Kind.Name()
	}
	for _, c := range g.outPads(h) {
		pads[c] = "OUT"
	}
	return pads
}

// outPads are the tiles of the hex's ways out that have a hex docked on
// them: the piece covering one passes its targets on.
func (g *Game) outPads(h *skill.Hex) []skill.Cell {
	var out []skill.Cell
	gr := h.Shape.Graph()
	ps, n := h.Shape.Ports()
	for _, p := range ps[:n] {
		if !p.In && h.Docks[p.Slot] != nil && gr.Has(int(p.Slot)) {
			out = append(out, gr.At[p.Slot])
		}
	}
	return out
}

// pieceBadge puts a small tag on piece s's top corner.
func (g *Game) pieceBadge(dst *ebiten.Image, l *edLayout, h *skill.Hex, s int, tag string, col color.RGBA) {
	cells := h.PieceCells(s)
	if len(cells) == 0 {
		return
	}
	// The cell highest up, then leftmost.
	tx, ty := l.cellXY(cells[0])
	for _, c := range cells[1:] {
		x, y := l.cellXY(c)
		if y < ty-1 || (math.Abs(y-ty) <= 1 && x < tx) {
			tx, ty = x, y
		}
	}
	r := l.slotR
	bx, by := tx-r*0.5, ty-r*0.62
	drawHex(dst, bx, by, r*0.3, true, colPanel, col, g.u(1.5))
	g.label(dst, g.small, tag, bx, by, col, true)
}

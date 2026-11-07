package main

// The open board: a wired hex's pieces each keep their silhouette, a few
// hexagons that never turn, anywhere on a big board, and pieces only join
// where they touch. Tap a piece to pick it; tap an empty tile to move it
// there (its whole shape has to fit), tap a piece it touches to step
// their join (a gate's YES and NO, an overload's rungs, then off), tap
// the tray to set it aside. The board drags and zooms (edview.go).

import (
	"image/color"

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
	// The board. The shape's own pads are marked: whatever piece covers
	// one gets its quirk.
	pads := map[skill.Cell]string{}
	gr := h.Shape.Graph()
	if i := h.Shape.Twin(); i >= 0 && gr.Has(i) {
		pads[gr.At[i]] = "TWIN"
	}
	if i := h.Shape.Surge(); i >= 0 && gr.Has(i) {
		pads[gr.At[i]] = "SURGE"
	}
	if i := gr.LoopFrom(); i >= 0 && gr.Has(i) {
		pads[gr.At[i]] = "LOOP"
	}
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
	// What the picked piece touches and can join.
	if picked >= 0 {
		for s := 0; s < skill.Slots; s++ {
			if s == picked || !h.Lay.Pos[s].On {
				continue
			}
			if _, _, ok := h.CanWireTouch(picked, s); ok || h.WireArm(picked, s) != 0 || h.WireArm(s, picked) != 0 {
				for _, c := range h.PieceCells(s) {
					cx, cy := l.cellXY(c)
					drawHex(dst, cx, cy, r*0.82, true, color.RGBA{}, fade(colYes, 0.8), g.u(2))
				}
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
		if h.PieceAt(c) < 0 {
			cx, cy := l.cellXY(c)
			g.label(dst, g.small, tag, cx, cy, fade(colShape, 0.8), true)
		}
	}
	g.drawTray(full, l, h, picked)
}

// drawTray draws the tray of pieces set aside.
func (g *Game) drawTray(dst *ebiten.Image, l *edLayout, h *skill.Hex, picked int) {
	loose := h.Loose()
	g.drawRect(dst, l.tray, colPanel, fade(colDim, 0.35))
	if len(loose) == 0 {
		msg := "TRAY: pieces set aside wait here"
		if picked >= 0 && h.Lay.Pos[picked].On {
			msg = "tap here to set the piece aside"
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
func (g *Game) edWireTap(h *skill.Hex, t int) bool {
	e, sim := &g.ed, g.sim
	a := e.at
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

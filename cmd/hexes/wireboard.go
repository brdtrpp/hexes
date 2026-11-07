package main

// The open board: a wired hex's pieces each sit on one tile, anywhere,
// and wires join them. Tap a piece to pick it; with one picked, tap
// another to wire them (again to step a gate's YES and NO or an
// overload's rungs, and on past the last to take the wire off), tap an
// empty tile to move it there, tap the tray to set it aside. Each tile
// shows the piece's silhouette, which is its kind.

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"hexes/loot"
	"hexes/skill"
)

var rungNames = [...]string{"", "I", "II", "III"}

// silhouette draws piece p's shape small, centered on x, y and size
// across: the same outline it has on every card and drop.
func (g *Game) silhouette(dst *ebiten.Image, x, y, size float64, p skill.Part, col color.RGBA) {
	pc := skill.PartPiece(p)
	if pc == nil || len(pc.Cells) == 0 {
		return
	}
	minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, c := range pc.Cells {
		cx, cy := c.XY()
		minX, minY = math.Min(minX, cx), math.Min(minY, cy)
		maxX, maxY = math.Max(maxX, cx), math.Max(maxY, cy)
	}
	span := math.Max(maxX-minX+1, maxY-minY+1)
	r := size / span / 1.05
	ox, oy := (minX+maxX)/2, (minY+maxY)/2
	for _, c := range pc.Cells {
		cx, cy := c.XY()
		drawHex(dst, x+(cx-ox)*r, y+(cy-oy)*r, r*0.56, true, fade(col, 0.55), col, g.u(1))
	}
}

func wireCol(k skill.PlugKind, arm uint8, rung bool) color.RGBA {
	if rung {
		return colCharge
	}
	return plugCol(k, arm)
}

// drawWired draws a wired hex: its tiles, its pieces, the wires and the
// tray.
func (g *Game) drawWired(dst *ebiten.Image, l *edLayout, h *skill.Hex) {
	e := &g.ed
	r := l.slotR
	picked := -1
	if e.pick == pickSlot && e.at >= 0 && e.at < skill.Slots && h.Lay.Pos[e.at].On {
		picked = e.at
	}
	_, held := g.edFits(h)
	room := h.Held() < h.Budget()
	// The board. The shape's own pads are marked: whatever piece sits on
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
	for _, c := range h.Lay.Room.Cells() {
		cx, cy := l.cellXY(c)
		edge := colCellEdge
		if _, ok := pads[c]; ok {
			edge = fade(colShape, 0.7)
		}
		drawHex(dst, cx, cy, r*0.96, true, colCell, edge, g.u(1.5))
		if h.PieceAt(c) < 0 && (picked >= 0 || held && room) {
			drawHex(dst, cx, cy, r*0.8, true, fade(colYes, 0.08), fade(colYes, 0.5), g.u(1.5))
		}
	}
	// The wires, under the pieces: a line from sender to taker with an
	// arrow at the taker's edge.
	trig := h.TriggerSlot()
	rungs := trig >= 0 && h.Arms(trig) == 3
	for _, k := range h.Links() {
		if !h.Lay.Pos[k.From].On || !h.Lay.Pos[k.To].On {
			continue
		}
		ax, ay := l.cellXY(h.Lay.Pos[k.From].At)
		bx, by := l.cellXY(h.Lay.Pos[k.To].At)
		dx, dy := bx-ax, by-ay
		d := math.Hypot(dx, dy)
		if d < 1 {
			continue
		}
		ux, uy := dx/d, dy/d
		sx, sy := ax+ux*r*0.66, ay+uy*r*0.66
		tx, ty := bx-ux*r*0.7, by-uy*r*0.7
		isRung := rungs && k.From == trig
		col := wireCol(k.Kind, k.Arm, isRung)
		if !h.Runs(k.From) {
			col = fade(col, 0.4)
		}
		w := g.u(3)
		if k.From == picked || k.To == picked {
			w = g.u(4.5)
		}
		vector.StrokeLine(dst, float32(sx), float32(sy), float32(tx), float32(ty), w, col, true)
		a := r * 0.22
		var path vector.Path
		path.MoveTo(float32(tx), float32(ty))
		path.LineTo(float32(tx-ux*a-uy*a*0.6), float32(ty-uy*a+ux*a*0.6))
		path.LineTo(float32(tx-ux*a+uy*a*0.6), float32(ty-uy*a-ux*a*0.6))
		path.Close()
		vs, is := path.AppendVerticesAndIndicesForFilling(nil, nil)
		paint(vs, col)
		dst.DrawTriangles(vs, is, whiteSub(), &ebiten.DrawTrianglesOptions{AntiAlias: true})
		tag := ""
		switch {
		case isRung:
			tag = rungNames[min(int(k.Arm), 3)]
		case k.Arm == skill.ArmYes && h.Arms(k.From) == 2:
			tag = "YES"
		case k.Arm == skill.ArmNo:
			tag = "NO"
		}
		if tag != "" {
			mx, my := (sx+tx)/2, (sy+ty)/2
			drawHex(dst, mx, my, r*0.24, true, colPanel, col, g.u(1.5))
			g.label(dst, g.small, tag, mx, my, col, true)
		}
	}
	// What the picked piece can wire to.
	if picked >= 0 {
		for s := 0; s < skill.Slots; s++ {
			if s == picked || !h.Lay.Pos[s].On {
				continue
			}
			if _, _, ok := h.CanWire(picked, s); ok || h.WireArm(picked, s) != 0 || h.WireArm(s, picked) != 0 {
				cx, cy := l.cellXY(h.Lay.Pos[s].At)
				drawHex(dst, cx, cy, r*0.9, true, color.RGBA{}, fade(colYes, 0.8), g.u(2))
			}
		}
	}
	// The pieces: one tile each, the silhouette on top.
	for s := 0; s < skill.Slots; s++ {
		if !h.Lay.Pos[s].On || h.Slots[s].Kind == skill.SlotEmpty {
			continue
		}
		p := h.Slots[s].Part
		cx, cy := l.cellXY(h.Lay.Pos[s].At)
		run := h.Runs(s)
		col := partCol(p)
		fill, edge := fade(col, 0.32), col
		if !run {
			fill, edge = fade(col, 0.1), fade(colDim, 0.7)
		}
		if _, ok := h.Use(s); !ok {
			edge = colHot
		}
		w := g.u(2.5)
		if s == picked {
			fill, edge, w = fade(col, 0.5), colText, g.u(4)
		}
		drawHex(dst, cx, cy, r*0.8, true, fill, edge, w)
		g.silhouette(dst, cx, cy-r*0.26, r*0.5, p, edge)
		tc := colText
		if !run {
			tc = fade(colDim, 0.8)
		}
		title, _ := loot.Label(p, g.sim.Reg)
		if p.Kind == skill.KindCall {
			title = "to " + g.callName(e.hex, p, true)
		}
		g.label(dst, g.small, title, cx, cy+r*0.28, tc, true)
		if tag := h.Frame(s).Tag(); tag != "" {
			g.label(dst, g.small, tag, cx, cy-r*0.72, frameCol(h.Frame(s)), true)
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
	g.drawTray(dst, l, h, picked)
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
	_, _, can := h.CanWire(a, t)
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
		e.msg = "Wire off."
	case h.Arms(from) == 2 && arm == skill.ArmYes:
		e.msg = "Wired on YES: tap again for NO."
	case h.Arms(from) == 2:
		e.msg = "Wired on NO: tap again to take it off."
	case h.Arms(from) == 3:
		e.msg = "Wired on rung " + rungNames[min(arm, 3)] + ": tap again for the next rung."
	default:
		e.msg = "Wired: tap again to take it off."
	}
	e.bad = false
	return true
}

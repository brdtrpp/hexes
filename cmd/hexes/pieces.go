package main

// The editor for a hex of pieces laid by edges (saves from before the
// open board, until they're wired; wireboard.go draws the open board):
// the room's cells, each piece drawn as
// its own silhouette with the plugs on its edges, and a tray under the
// room for the pieces set aside. Tap a piece to pick it, tap it again to
// turn it (seven turns flip it over), tap an empty cell to move it there,
// tap the tray to set it aside. A pack part tapped onto an empty cell is
// laid there; onto a piece, it takes that piece's place. Plugs that meet
// join, and only the pieces the trigger reaches through joins run.

import (
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"hexes/game"
	"hexes/loot"
	"hexes/skill"
)

var (
	colCell     = color.RGBA{0x12, 0x16, 0x22, 0xff}
	colCellEdge = color.RGBA{0x26, 0x2c, 0x3c, 0xff}
	colBoost    = color.RGBA{0xff, 0x8a, 0x4a, 0xff}
)

// cellXY is where a room cell's center sits on screen.
func (l *edLayout) cellXY(c skill.Cell) (float64, float64) {
	x, y := c.XY()
	return l.gridX + l.R*x, l.gridY + l.R*y
}

// cellAt is the room cell under a tap.
func (l *edLayout) cellAt(h *skill.Hex, x, y float64) (skill.Cell, bool) {
	if l.board.w > 0 && !l.board.hit(x, y) {
		return skill.Cell{}, false // off the board's window
	}
	for _, c := range h.Lay.Room.Cells() {
		cx, cy := l.cellXY(c)
		if math.Hypot(x-cx, y-cy) < l.R*0.52 {
			return c, true
		}
	}
	return skill.Cell{}, false
}

// trayTiles are where the pieces set aside sit in the tray.
func (l *edLayout) trayTiles(n int) []rect {
	if n == 0 {
		return nil
	}
	out := make([]rect, n)
	if l.tray.h > l.tray.w {
		// A column: tiles down it.
		s := math.Min(l.tray.w*0.92, l.tray.h/float64(max(n, 5)))
		y := l.tray.y + (l.tray.h-s*float64(n))/2
		for i := range out {
			out[i] = rect{l.tray.x + (l.tray.w-s)/2, y + float64(i)*s, s, s}
		}
		return out
	}
	s := math.Min(l.tray.h*0.92, l.tray.w/float64(max(n, 6)))
	x := l.tray.x + (l.tray.w-s*float64(n))/2
	for i := range out {
		out[i] = rect{x + float64(i)*s, l.tray.y + (l.tray.h-s)/2, s, s}
	}
	return out
}

// sideCorners are the two corners of a flat-top cell's side d, at radius
// r from its center.
func sideCorners(cx, cy, r float64, d int) (ax, ay, bx, by float64) {
	a := (-120 + 60*float64(d)) * math.Pi / 180
	b := a + math.Pi/3
	return cx + r*math.Cos(a), cy + r*math.Sin(a), cx + r*math.Cos(b), cy + r*math.Sin(b)
}

func plugCol(k skill.PlugKind, arm uint8) color.RGBA {
	switch {
	case arm == skill.ArmYes:
		return colYes
	case arm == skill.ArmNo:
		return colHot
	case k == skill.PlugSpark:
		return color.RGBA{0xff, 0xe0, 0x60, 0xff}
	case k == skill.PlugGroup:
		return color.RGBA{0x7d, 0xf0, 0xe0, 0xff}
	case k == skill.PlugBoost, k == skill.PlugSocket:
		return colBoost
	}
	return colDim
}

// edFits are the empty cells the held part or picked piece could be laid
// on, and whether anything is held.
func (g *Game) edFits(h *skill.Hex) (fit map[skill.Cell]bool, held bool) {
	e, sim := &g.ed, g.sim
	hc := *h
	s := -1
	switch {
	case e.pick == pickSlot && e.at >= 0 && e.at < skill.Slots && h.Slots[e.at].Kind == skill.SlotPart:
		s = e.at
	case e.pick == pickCell && sim.Stash.Inv.Has[e.at], e.pick == pickLocker && e.at < len(sim.Locker):
		s = sim.FreeSlot(e.hex)
		if s < 0 {
			return nil, true
		}
		p := sim.Stash.Inv.Parts[e.at]
		if e.pick == pickLocker {
			p = sim.Locker[e.at]
		}
		hc.Slots[s] = skill.Slot{Kind: skill.SlotPart, Part: p}
		hc.Lift(s)
	default:
		return nil, false
	}
	fit = map[skill.Cell]bool{}
	if h.Lay.Wired {
		// Every tile its first cell can go on with its whole shape free,
		// while the board has room for one more.
		if !h.Lay.Pos[s].On && h.Held() >= h.Budget() {
			return fit, true
		}
		for _, c := range h.Lay.Room.Cells() {
			if hc.FitsAt(s, c) {
				fit[c] = true
			}
		}
		return fit, true
	}
	for _, c := range h.Lay.Room.Cells() {
		if o := h.PieceAt(c); o >= 0 && o != s {
			continue
		}
		for r := uint8(0); r < skill.Orients; r++ {
			if hc.CanPlace(s, skill.Place{At: c, Rot: r}) {
				fit[c] = true
				break
			}
		}
	}
	return fit, true
}

// drawPieces draws a laid hex: its room, its pieces and their plugs, the
// joins between them, and the tray.
func (g *Game) drawPieces(dst *ebiten.Image, l *edLayout, h *skill.Hex) {
	full := dst
	if l.board.w > 0 {
		// The board draws only inside its window; the tray sits outside.
		dst = dst.SubImage(image.Rect(int(l.board.x), int(l.board.y), int(math.Ceil(l.board.x+l.board.w)), int(math.Ceil(l.board.y+l.board.h)))).(*ebiten.Image)
	}
	if h.Lay.Wired {
		g.drawWired(dst, full, l, h)
		return
	}
	e := &g.ed
	r := l.slotR
	fit, held := g.edFits(h)
	picked := -1
	if e.pick == pickSlot {
		picked = e.at
	}
	// The room. The shape's own pads (a twin, a surge, the loop's way
	// back) are marked: whatever piece covers one gets its quirk.
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
		if held && fit[c] && h.PieceAt(c) < 0 {
			drawHex(dst, cx, cy, r*0.82, true, fade(colYes, 0.12), colYes, g.u(2))
		}
	}
	// The pieces.
	for s := 0; s < skill.Slots; s++ {
		cells := h.PieceCells(s)
		if len(cells) == 0 {
			continue
		}
		p := h.Slots[s].Part
		run := h.Runs(s)
		col := partCol(p)
		fill, edge := fade(col, 0.4), col
		if !run {
			fill, edge = fade(col, 0.1), fade(colDim, 0.7)
		}
		if _, ok := h.Use(s); !ok {
			edge = colHot
		}
		w := g.u(2.5)
		if s == picked {
			fill, edge, w = fade(col, 0.45), colText, g.u(4)
		}
		in := func(c skill.Cell) bool {
			for _, o := range cells {
				if o == c {
					return true
				}
			}
			return false
		}
		for _, c := range cells {
			cx, cy := l.cellXY(c)
			drawHex(dst, cx, cy, r, true, fill, color.RGBA{}, 0)
		}
		for _, c := range cells {
			cx, cy := l.cellXY(c)
			for d := range skill.CellDirs {
				if in(c.Add(skill.CellDirs[d])) {
					continue
				}
				ax, ay, bx, by := sideCorners(cx, cy, r*0.97, d)
				vector.StrokeLine(dst, float32(ax), float32(ay), float32(bx), float32(by), w, edge, true)
			}
		}
		// The glyph on its first cell, the name across the middle.
		cx, cy := l.cellXY(cells[0])
		tc := colText
		if !run {
			tc = fade(colDim, 0.8)
		}
		g.partGlyph(dst, cx, cy-r*0.12, r*0.5, p)
		title, _ := loot.Label(p, g.sim.Reg)
		if p.Kind == skill.KindCall {
			title = "to " + g.callName(e.hex, p, true)
		}
		mx, my := 0.0, 0.0
		for _, c := range cells {
			x, y := l.cellXY(c)
			mx, my = mx+x, my+y
		}
		mx, my = mx/float64(len(cells)), my/float64(len(cells))
		if len(cells) == 1 {
			my = cy + r*0.5
		}
		g.label(dst, g.small, title, mx, my, tc, true)
		if tag := h.Frame(s).Tag(); tag != "" {
			g.label(dst, g.small, tag, cx, cy-r*0.68, frameCol(h.Frame(s)), true)
		}
		if p.UsesBank() && e.hex < game.MaxSlots && p.Bank < skill.MaxBanks {
			g.label(dst, g.small, skill.BankName(p.Bank), cx+r*0.55, cy-r*0.45, colCharge, true)
		}
	}
	for c, tag := range pads {
		if h.PieceAt(c) < 0 {
			cx, cy := l.cellXY(c)
			g.label(dst, g.small, tag, cx, cy, fade(colShape, 0.8), true)
		}
	}
	// Plugs that meet nothing stick out as nubs (a way out) or show as
	// open sockets (a way in); joins show as a bar across the edge.
	links := h.Links()
	type edge struct {
		c skill.Cell
		d int
	}
	joined := map[edge]bool{}
	for _, k := range links {
		joined[edge{k.Cell, k.Side}] = true
		joined[edge{k.Cell.Add(skill.CellDirs[k.Side]), (k.Side + 3) % 6}] = true
	}
	for s := 0; s < skill.Slots; s++ {
		if !h.Lay.Pos[s].On {
			continue
		}
		dim := !h.Runs(s)
		for _, p := range h.Plugs(s) {
			if joined[edge{p.Cell, p.Side}] {
				continue
			}
			cx, cy := l.cellXY(p.Cell)
			dx, dy := skill.CellDirs[p.Side].XY()
			mx, my := cx+dx*l.R*0.5, cy+dy*l.R*0.5
			c := plugCol(p.K, p.Arm)
			if dim {
				c = fade(c, 0.45)
			}
			sz := 6 * g.k
			if p.K.Out() {
				var path vector.Path
				path.MoveTo(float32(mx+dx*sz), float32(my+dy*sz))
				path.LineTo(float32(mx-dy*sz*0.8), float32(my+dx*sz*0.8))
				path.LineTo(float32(mx+dy*sz*0.8), float32(my-dx*sz*0.8))
				path.Close()
				vs, is := path.AppendVerticesAndIndicesForFilling(nil, nil)
				paint(vs, c)
				dst.DrawTriangles(vs, is, whiteSub(), &ebiten.DrawTrianglesOptions{AntiAlias: true})
			} else {
				vector.StrokeCircle(dst, float32(mx-dx*sz*0.6), float32(my-dy*sz*0.6), float32(sz*0.5), g.u(1.5), c, true)
			}
		}
	}
	for _, k := range links {
		cx, cy := l.cellXY(k.Cell)
		ax, ay, bx, by := sideCorners(cx, cy, r, k.Side)
		// A short bar across the shared edge, in the sending plug's color.
		mx, my := (ax+bx)/2, (ay+by)/2
		ux, uy := (bx-ax)*0.32, (by-ay)*0.32
		vector.StrokeLine(dst, float32(mx-ux), float32(my-uy), float32(mx+ux), float32(my+uy), g.u(5), plugCol(k.Kind, k.Arm), true)
	}
	g.drawTray(full, l, h, picked)
}

// edPieceTap handles a tap on a laid hex's room or tray, and reports
// whether it was one.
func (g *Game) edPieceTap(l *edLayout, x, y float64, confirm int) bool {
	e, sim := &g.ed, g.sim
	h := sim.Load.Hex(e.hex)
	inv := &sim.Stash.Inv
	if l.tray.hit(x, y) {
		loose := h.Loose()
		for i, t := range l.trayTiles(len(loose)) {
			if !t.hit(x, y) {
				continue
			}
			s := loose[i]
			switch {
			case e.pick == pickFrame:
				g.edFitFrame(s, confirm == s)
			case e.pick == pickCell && inv.Has[e.at]:
				g.edResult(sim.SwapIn(e.hex, s, e.at))
			case e.pick == pickSlot && e.at == s:
				e.pick, e.msg = pickNone, ""
			default:
				e.pick, e.at, e.msg = pickSlot, s, ""
			}
			return true
		}
		if e.pick == pickSlot && h.Lay.Pos[e.at].On {
			if err := sim.SetPieceAside(e.hex, e.at); err != nil {
				e.msg, e.bad = upFirst(err.Error())+".", true
				return true
			}
			g.edResult(nil)
			e.msg = "Set aside: it waits in the tray and doesn't run. " + e.msg
			return true
		}
		e.pick, e.msg = pickNone, ""
		return true
	}
	c, ok := l.cellAt(h, x, y)
	if !ok {
		return false
	}
	t := h.PieceAt(c)
	keep := func(err error) {
		// A move keeps the piece picked, so it can be turned next.
		s := e.at
		g.edResult(err)
		if err == nil {
			e.pick, e.at = pickSlot, s
		}
	}
	switch {
	case e.pick == pickFrame:
		if t >= 0 {
			g.edFitFrame(t, confirm == t)
		} else {
			e.msg, e.bad = "Frames go on pieces: tap one.", true
		}
	case e.pick == pickLocker:
		slot := t
		if slot < 0 {
			slot = sim.FreeSlot(e.hex)
		}
		if err := sim.LockerIn(e.hex, slot, e.at); err != nil || t >= 0 {
			g.edResult(err)
			return true
		}
		g.edResult(nil)
		_ = sim.MovePiece(e.hex, slot, c) // where it was tapped, if it fits
	case e.pick == pickCell && inv.Has[e.at] && t >= 0 && inv.Parts[e.at].Kind == skill.KindMult &&
		h.Slots[t].Kind == skill.SlotPart && h.Slots[t].Part.RunsNow():
		if err := sim.AddOpt(e.hex, t, e.at); err != nil {
			e.msg, e.bad = err.Error()+".", true
			return true
		}
		e.pick, e.at = pickSlot, t
		e.msg, e.bad = "Option on: it scales "+h.Slots[t].Part.Lib+" inside the call. Tap it above to take it off.", false
	case e.pick == pickCell && inv.Has[e.at] && t >= 0:
		g.edResult(sim.SwapIn(e.hex, t, e.at))
	case e.pick == pickCell && inv.Has[e.at]:
		g.edResult(sim.PlaceIn(e.hex, e.at, c))
	case e.pick == pickSlot && t == e.at:
		// Again on the picked piece: a charge or call switches what it
		// does; on the open board anything else lets go, on a room it
		// turns.
		sl := h.Slots[t]
		if sl.Kind == skill.SlotPart && (sl.Part.UsesBank() || sl.Part.Kind == skill.KindCall) && g.edCycle() {
			return true
		}
		if h.Lay.Wired {
			e.pick, e.msg = pickNone, ""
			return true
		}
		keep(sim.TurnPiece(e.hex, t))
	case e.pick == pickSlot && t >= 0 && h.Lay.Wired && g.edWireTap(h, t):
	case t >= 0:
		e.pick, e.at, e.msg = pickSlot, t, ""
		if h.Lay.Wired {
			e.msg = "Tap an empty tile to move it (green: its shape fits), or a piece it touches to step their join."
		}
	case e.pick == pickSlot && h.Slots[e.at].Kind == skill.SlotPart:
		keep(sim.MovePiece(e.hex, e.at, c))
	default:
		e.pick, e.msg = pickNone, ""
	}
	return true
}

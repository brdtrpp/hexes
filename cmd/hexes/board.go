package main

// The docking board view in the editor: every hex as a big cell of a
// small honeycomb, drawn as a little map of its shape, with its ports on
// its sides. Tap a hex to pick it, tap it again (or ROTATE) to turn it,
// tap an open cell to move it there. + MODULE builds a module on a picked
// empty cell, REMOVE takes a module off, EDIT opens the picked hex. Drag
// to pan and pinch to zoom on a phone; on a keyboard, arrows or WASD pan,
// + and - zoom, R turns, Enter edits.

import (
	"fmt"
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"hexes/game"
	"hexes/skill"
)

var (
	colMod  = color.RGBA{0x9a, 0xc4, 0xd8, 0xff} // modules
	colIn   = color.RGBA{0x5c, 0xe0, 0x7a, 0xff} // a way in
	colOut  = color.RGBA{0xff, 0xa8, 0x4a, 0xff} // a way out
	colDock = color.RGBA{0x4d, 0xe3, 0xff, 0xff} // a docked link
)

// hexCol is a hex's color: its card color, or the module color.
func hexCol(i int) color.RGBA {
	if i >= 0 && i < game.MaxSlots {
		return slotCol[i]
	}
	return colMod
}

// boardView is the board's own state: what's picked, the pan and zoom,
// and the pointers in flight.
type boardView struct {
	sel     int        // picked hex, or -1
	cell    skill.Cell // picked empty cell, when empty is set
	empty   bool
	panX    float64 // in board units
	panY    float64
	zoom    float64
	ptr     map[ebiten.TouchID]*pointer
	mouse   *pointer
	pinch   float64 // last distance between two fingers
	pinchOn bool
}

type pointer struct {
	x0, y0, x, y float64
	moved        bool
	// drag is the piece or part this press carries, in the hex editor
	// (eddrag.go); nil for a press that pans the board or taps.
	drag *edDrag
}

func (g *Game) setBoard(on bool) {
	e := &g.ed
	e.board, e.pick, e.shape = on, pickNone, false
	if on {
		e.grid = false
	}
	if !on {
		return
	}
	if e.bd.zoom == 0 {
		e.bd.zoom = 1
	}
	e.bd.sel, e.bd.empty = -1, false
	if g.sim.Load.IsMod(e.hex) || e.hex < game.MaxSlots {
		e.bd.sel = e.hex
	}
	e.msg, e.bad = g.boardHelp(), false
}

func (g *Game) boardHelp() string {
	open := g.sim.BoardOpen()
	t := fmt.Sprintf("Board: %d of %d cells open.", open, game.BoardCells)
	if at := g.sim.NextCellAt(); at > 0 && !g.sim.TestShapes {
		t += fmt.Sprintf(" Next at depth %d.", at)
	}
	return t + " A hex's orange way out docks into the green way in of the hex it faces: the chain flows across."
}

// boardLayout is where the board view's pieces sit.
type boardLayout struct {
	area                rect // where the board is drawn
	rotate, edit, test  rect
	cx, cy, size, tileR float64 // board middle on screen, cell spacing, tile radius
}

func (g *Game) boardLayout(l *edLayout) boardLayout {
	var b boardLayout
	k := g.k
	top := l.shape.y
	bot := math.Min(l.info.y, l.scrap.y) - 10*k
	x0 := l.tabs[0].x
	w := l.tabs[len(l.tabs)-1].x + l.tabs[len(l.tabs)-1].w - x0
	if l.side {
		bot, w = l.scrap.y-10*k, l.info.x-10*k-x0
	}
	capH := l.shape.h
	bw := math.Min(96*k, w*0.28)
	b.rotate = rect{x0, top, bw, capH}
	b.edit = rect{x0 + bw + 6*k, top, bw, capH}
	b.test = l.test
	b.test.x = x0 + w - b.test.w
	b.area = rect{x0, top + capH + 6*k, w, bot - top - capH - 6*k}
	if w > 1.6*(bot-top) {
		// Wide and short (a phone on its side): buttons down the left,
		// the board gets the full height.
		bw = math.Min(110*k, w*0.2)
		gap := 6 * k
		b.rotate = rect{x0, top, bw, capH}
		b.edit = rect{x0, top + capH + gap, bw, capH}
		b.test = rect{x0, top + 2*(capH+gap), bw, capH}
		b.area = rect{x0 + bw + 10*k, top, w - bw - 10*k, bot - top}
	}
	// Fit the whole board: radius 2 is 4 rows of spacing plus a tile.
	fit := math.Min(b.area.w/(4*0.866+1.2), b.area.h/(4+1.2))
	b.size = fit * g.ed.bd.zoom
	b.tileR = b.size * 0.56
	b.cx = b.area.x + b.area.w/2 - g.ed.bd.panX*b.size
	b.cy = b.area.y + b.area.h/2 - g.ed.bd.panY*b.size
	return b
}

func (b *boardLayout) at(c skill.Cell) (x, y float64) {
	cx, cy := c.XY()
	return b.cx + cx*b.size, b.cy + cy*b.size
}

// cellAt is the board cell under a point.
func (b *boardLayout) cellAt(x, y float64) (skill.Cell, bool) {
	best, bd := skill.Cell{}, math.Inf(1)
	for _, c := range game.BoardOrder {
		cx, cy := b.at(c)
		if d := math.Hypot(x-cx, y-cy); d < bd {
			best, bd = c, d
		}
	}
	return best, bd < b.tileR
}

// updateBoard runs the board's input for a frame.
func (g *Game) updateBoard() {
	bd := &g.ed.bd
	if bd.ptr == nil {
		bd.ptr = map[ebiten.TouchID]*pointer{}
	}
	l := g.edLayout()
	b := g.boardLayout(&l)
	// Keys.
	step := 0.08 / bd.zoom * 3
	if ebiten.IsKeyPressed(ebiten.KeyArrowLeft) || ebiten.IsKeyPressed(ebiten.KeyA) {
		bd.panX -= step
	}
	if ebiten.IsKeyPressed(ebiten.KeyArrowRight) || ebiten.IsKeyPressed(ebiten.KeyD) {
		bd.panX += step
	}
	if ebiten.IsKeyPressed(ebiten.KeyArrowUp) || ebiten.IsKeyPressed(ebiten.KeyW) {
		bd.panY -= step
	}
	if ebiten.IsKeyPressed(ebiten.KeyArrowDown) || ebiten.IsKeyPressed(ebiten.KeyS) {
		bd.panY += step
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEqual) || inpututil.IsKeyJustPressed(ebiten.KeyKPAdd) {
		g.zoomBoard(1.25, b.area.x+b.area.w/2, b.area.y+b.area.h/2)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyMinus) || inpututil.IsKeyJustPressed(ebiten.KeyKPSubtract) {
		g.zoomBoard(0.8, b.area.x+b.area.w/2, b.area.y+b.area.h/2)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyR) {
		g.boardRotate()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) {
		g.boardEdit()
	}
	if _, wy := ebiten.Wheel(); wy != 0 {
		x, y := ebiten.CursorPosition()
		g.zoomBoard(math.Pow(1.1, wy), float64(x), float64(y))
	}
	// Fingers: one drags, two pinch, a touch that never moved is a tap.
	for _, id := range inpututil.AppendJustPressedTouchIDs(nil) {
		g.touched = true
		x, y := ebiten.TouchPosition(id)
		bd.ptr[id] = &pointer{x0: float64(x), y0: float64(y), x: float64(x), y: float64(y)}
		bd.pinchOn = false
	}
	ids := ebiten.AppendTouchIDs(nil)
	if len(ids) == 2 {
		p, q := bd.ptr[ids[0]], bd.ptr[ids[1]]
		if p != nil && q != nil {
			x1, y1 := ebiten.TouchPosition(ids[0])
			x2, y2 := ebiten.TouchPosition(ids[1])
			d := math.Hypot(float64(x2-x1), float64(y2-y1))
			if bd.pinchOn && bd.pinch > 0 && d > 0 {
				g.zoomBoard(d/bd.pinch, float64(x1+x2)/2, float64(y1+y2)/2)
			}
			bd.pinch, bd.pinchOn = d, true
			p.moved, q.moved = true, true
			p.x, p.y, q.x, q.y = float64(x1), float64(y1), float64(x2), float64(y2)
		}
	} else if len(ids) == 1 {
		if p := bd.ptr[ids[0]]; p != nil {
			x, y := ebiten.TouchPosition(ids[0])
			g.boardDrag(p, float64(x), float64(y), &b)
		}
	}
	for _, id := range inpututil.AppendJustReleasedTouchIDs(nil) {
		p := bd.ptr[id]
		delete(bd.ptr, id)
		if p != nil && !p.moved && len(bd.ptr) == 0 {
			g.boardTap(p.x0, p.y0)
		}
	}
	if len(ids) < 2 {
		bd.pinchOn = false
	}
	// The mouse: drag to pan, click to tap.
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		x, y := ebiten.CursorPosition()
		bd.mouse = &pointer{x0: float64(x), y0: float64(y), x: float64(x), y: float64(y)}
	}
	if bd.mouse != nil && ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
		x, y := ebiten.CursorPosition()
		g.boardDrag(bd.mouse, float64(x), float64(y), &b)
	}
	if bd.mouse != nil && inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft) {
		if !bd.mouse.moved {
			g.boardTap(bd.mouse.x0, bd.mouse.y0)
		}
		bd.mouse = nil
	}
	bd.panX = math.Max(-3, math.Min(3, bd.panX))
	bd.panY = math.Max(-3, math.Min(3, bd.panY))
}

// boardDrag pans while a pointer that started on the board moves.
func (g *Game) boardDrag(p *pointer, x, y float64, b *boardLayout) {
	if !b.area.hit(p.x0, p.y0) {
		return
	}
	if !p.moved && math.Hypot(x-p.x0, y-p.y0) > 10*g.k {
		p.moved = true
	}
	if p.moved {
		g.ed.bd.panX -= (x - p.x) / b.size
		g.ed.bd.panY -= (y - p.y) / b.size
	}
	p.x, p.y = x, y
}

// zoomBoard zooms by f around a screen point, keeping it still.
func (g *Game) zoomBoard(f, x, y float64) {
	bd := &g.ed.bd
	l := g.edLayout()
	b := g.boardLayout(&l)
	nz := math.Max(0.7, math.Min(3.5, bd.zoom*f))
	// The board point under (x, y) stays under it.
	px := (x - b.cx) / b.size
	py := (y - b.cy) / b.size
	bd.zoom = nz
	nb := g.boardLayout(&l)
	bd.panX += px - (x-nb.cx)/nb.size
	bd.panY += py - (y-nb.cy)/nb.size
}

func (g *Game) boardTap(x, y float64) {
	e, sim, bd := &g.ed, g.sim, &g.ed.bd
	l := g.edLayout()
	b := g.boardLayout(&l)
	for i, t := range l.tabs {
		if t.hit(x, y) && g.edTabOn(i) {
			switch i {
			case game.MaxSlots:
				return // already here
			case game.MaxSlots + 1:
				g.setGrid(true)
				return
			}
			e.hex, e.board, e.pick, e.msg = i, false, pickNone, ""
			return
		}
	}
	switch {
	case l.close.hit(x, y):
		e.on = false
		return
	case l.scrap.hit(x, y):
		g.boardModule()
		return
	case b.rotate.hit(x, y):
		g.boardRotate()
		return
	case b.edit.hit(x, y):
		g.boardEdit()
		return
	case b.test.hit(x, y):
		if !sim.TestShapes {
			if err := sim.SetTestShapes(true); err != nil {
				e.msg, e.bad = err.Error()+".", true
				return
			}
			e.msg, e.bad = "Test switch on: the whole board is open and every shape unlocked.", false
		}
		g.openTestMenu()
		return
	}
	if !b.area.hit(x, y) {
		return
	}
	c, ok := b.cellAt(x, y)
	if !ok {
		bd.sel, bd.empty, e.msg, e.bad = -1, false, g.boardHelp(), false
		return
	}
	at := sim.Load.At(c)
	switch {
	case at >= 0 && at == bd.sel:
		g.boardRotate()
	case at >= 0:
		bd.sel, bd.empty = at, false
		e.msg, e.bad = g.hexDocks(at), false
	case !sim.CellOpen(c):
		e.msg, e.bad = fmt.Sprintf("That cell opens once you reach depth %d.", cellOpens(c)), true
	case bd.sel >= 0:
		if err := sim.MoveHex(bd.sel, c); err != nil {
			e.msg, e.bad = err.Error()+".", true
			return
		}
		e.msg, e.bad = "Moved. "+g.hexDocks(bd.sel), false
	default:
		bd.cell, bd.empty = c, true
		e.msg, e.bad = "Empty cell. Tap + MODULE to build a module here: a hex with no trigger that runs when something docks into it.", false
	}
}

func cellOpens(c skill.Cell) int {
	i := game.BoardIndex(c)
	return 1 + max(0, i-game.BoardBase+1)*game.BoardEvery
}

func (g *Game) boardRotate() {
	e, bd := &g.ed, &g.ed.bd
	if bd.sel < 0 {
		e.msg, e.bad = "Tap a hex first, then tap it again or ROTATE to turn it.", true
		return
	}
	if err := g.sim.RotateHex(bd.sel); err != nil {
		e.msg, e.bad = err.Error()+".", true
		return
	}
	e.msg, e.bad = "Turned. "+g.hexDocks(bd.sel), false
}

func (g *Game) boardEdit() {
	e, bd := &g.ed, &g.ed.bd
	if bd.sel < 0 {
		e.msg, e.bad = "Tap a hex first, then EDIT to change its parts.", true
		return
	}
	e.hex, e.board, e.pick, e.msg = bd.sel, false, pickNone, ""
}

// boardModule is the bottom-left button: REMOVE on a picked module,
// otherwise + MODULE on the picked empty cell (or the first free one).
func (g *Game) boardModule() {
	e, sim, bd := &g.ed, g.sim, &g.ed.bd
	if sim.Load.IsMod(bd.sel) {
		name := sim.Load.Name(bd.sel)
		if err := sim.RemoveModule(bd.sel); err != nil {
			e.msg, e.bad = err.Error()+".", true
			return
		}
		if e.hex >= sim.Load.Count() || e.hex == bd.sel {
			e.hex = 0
		} else if e.hex > bd.sel {
			e.hex--
		}
		bd.sel = -1
		e.msg, e.bad = name+" is off the board. Its parts are in your pack.", false
		return
	}
	c, ok := bd.cell, bd.empty
	if !ok {
		for _, cc := range game.BoardOrder[:sim.BoardOpen()] {
			if sim.Load.At(cc) < 0 {
				c, ok = cc, true
				break
			}
		}
	}
	if !ok {
		msg := "No free cell."
		if at := sim.NextCellAt(); at > 0 {
			msg += fmt.Sprintf(" The next opens at depth %d.", at)
		}
		e.msg, e.bad = msg, true
		return
	}
	i, err := sim.AddModule(c)
	if err != nil {
		e.msg, e.bad = err.Error()+".", true
		return
	}
	bd.sel, bd.empty = i, false
	e.msg, e.bad = sim.Load.Name(i)+" built. Tap EDIT to put parts in it, then turn it so a way out faces its green way in. "+g.hexDocks(i), false
}

// hexDocks says what a hex docks with, in a sentence.
func (g *Game) hexDocks(i int) string {
	sim := g.sim
	name := sim.Load.Name(i)
	var out, in []string
	for _, d := range sim.Docks() {
		if d.From == i {
			out = append(out, sim.Load.Name(d.To))
		}
		if d.To == i {
			in = append(in, sim.Load.Name(d.From))
		}
	}
	t := name
	switch {
	case len(out) > 0 && len(in) > 0:
		t += fmt.Sprintf(" runs %s and is run by %s.", list(out), list(in))
	case len(out) > 0:
		t += fmt.Sprintf(" flows on into %s.", list(out))
	case len(in) > 0:
		t += fmt.Sprintf(" runs when %s fires.", list(in))
	default:
		t += " docks with nothing: turn it or move it so an orange way out faces a green way in."
	}
	if i < game.MaxSlots {
		if p := sim.Load.Progs[i]; p.Docked > 0 {
			t += fmt.Sprintf(" %.0f power, %.0f heat per firing with everything docked.", p.Power, p.Heat)
		}
	}
	return t
}

func list(s []string) string {
	switch len(s) {
	case 1:
		return s[0]
	case 2:
		return s[0] + " and " + s[1]
	}
	out := ""
	for i, x := range s {
		switch {
		case i == len(s)-1:
			out += "and " + x
		default:
			out += x + ", "
		}
	}
	return out
}

func (g *Game) drawBoardTab(dst *ebiten.Image, t rect) {
	e, sim := &g.ed, g.sim
	on := e.board || sim.Load.IsMod(e.hex)
	fill, edge := colPanel, fade(colDock, 0.45)
	if on {
		fill, edge = fade(colDock, 0.22), colDock
	}
	g.drawRect(dst, t, fill, edge)
	name, sub := "BOARD", fmt.Sprintf("%d docked", len(sim.Docks()))
	if !e.board && sim.Load.IsMod(e.hex) {
		name, sub = "MOD "+shortName(sim.Load.Name(e.hex)), "board"
	}
	g.label(dst, g.small, name, t.x+t.w/2, t.y+t.h*0.33, colDock, true)
	g.label(dst, g.small, sub, t.x+t.w/2, t.y+t.h*0.72, colDim, true)
}

func (g *Game) drawBoard(dst *ebiten.Image) {
	sim, bd := g.sim, &g.ed.bd
	l := g.edLayout()
	b := g.boardLayout(&l)
	// Buttons.
	g.drawRect(dst, b.rotate, fade(colDock, 0.15), colDock)
	g.label(dst, g.small, "ROTATE", b.rotate.x+b.rotate.w/2, b.rotate.y+b.rotate.h/2, colDock, true)
	g.drawRect(dst, b.edit, fade(colDock, 0.15), colDock)
	g.label(dst, g.small, "EDIT", b.edit.x+b.edit.w/2, b.edit.y+b.edit.h/2, colDock, true)
	if sim.TestShapes {
		g.drawRect(dst, b.test, fade(colShape, 0.55), colShape)
		g.label(dst, g.small, "TEST ON", b.test.x+b.test.w/2, b.test.y+b.test.h/2, colText, true)
	} else {
		g.drawRect(dst, b.test, colPanel, fade(colDim, 0.6))
		g.label(dst, g.small, "TEST", b.test.x+b.test.w/2, b.test.y+b.test.h/2, colDim, true)
	}
	// The board, clipped to its area.
	sub := dst.SubImage(rectImage(b.area)).(*ebiten.Image)
	docks := sim.Docks()
	for _, c := range game.BoardOrder {
		x, y := b.at(c)
		switch {
		case !sim.CellOpen(c):
			drawHex(sub, x, y, b.tileR*0.96, true, color.RGBA{0x0c, 0x0e, 0x15, 0xff}, color.RGBA{0x22, 0x26, 0x32, 0xff}, g.u(1))
			if b.tileR > 26*g.k {
				g.label(sub, g.small, fmt.Sprintf("depth %d", cellOpens(c)), x, y, fade(colDim, 0.4), true)
			}
		case sim.Load.At(c) < 0:
			edge := color.RGBA{0x2e, 0x34, 0x46, 0xff}
			if bd.empty && bd.cell == c {
				edge = colDock
			}
			drawHex(sub, x, y, b.tileR*0.96, true, color.RGBA{0x16, 0x1a, 0x26, 0xff}, edge, g.u(1.5))
			g.label(sub, g.small, "+", x, y, fade(colDim, 0.5), true)
		}
	}
	for i := 0; i < sim.Load.Count(); i++ {
		g.drawBoardHex(sub, &b, i, i == bd.sel)
	}
	// Docked links on top: a bright bar across the shared side.
	for _, d := range docks {
		ax, ay := b.at(sim.Load.Spots[d.From].At)
		bx, by := b.at(sim.Load.Spots[d.To].At)
		mx, my := (ax+bx)/2, (ay+by)/2
		ux, uy := (bx-ax)/b.size, (by-ay)/b.size
		r := b.tileR * 0.35
		vector.StrokeLine(sub, float32(mx-ux*r), float32(my-uy*r), float32(mx+ux*r), float32(my+uy*r), g.u(5), colDock, true)
		g.edgeArrow(sub, [2]float64{mx - ux*r, my - uy*r}, [2]float64{mx + ux*r, my + uy*r}, colText)
	}
	g.drawBoardButtons(dst, &l)
}

// drawBoardHex draws hex i in its cell: a tile in its color, a little
// map of its shape, its name, and its ports on their sides.
func (g *Game) drawBoardHex(dst *ebiten.Image, b *boardLayout, i int, picked bool) {
	sim := g.sim
	h := sim.Load.Hex(i)
	sp := sim.Load.Spots[i]
	x, y := b.at(sp.At)
	col := hexCol(i)
	w := g.u(2)
	if picked {
		w = g.u(4)
	}
	fill := color.RGBA{0x1a, 0x1f, 0x2e, 0xff}
	if picked {
		fill = fade(col, 0.22)
	}
	drawHex(dst, x, y, b.tileR*0.96, true, fill, col, w)
	// The shape's map, turned with the hex.
	gr := h.Shape.Graph()
	rot := float64(sp.Rot) * math.Pi / 3
	cos, sin := math.Cos(rot), math.Sin(rot)
	var minX, maxX, minY, maxY = math.Inf(1), math.Inf(-1), math.Inf(1), math.Inf(-1)
	pts := make([][2]float64, gr.N)
	for s := 0; s < int(gr.N); s++ {
		cx, cy := gr.At[s].XY()
		rx, ry := cx*cos-cy*sin, cx*sin+cy*cos
		pts[s] = [2]float64{rx, ry}
		minX, maxX = math.Min(minX, rx), math.Max(maxX, rx)
		minY, maxY = math.Min(minY, ry), math.Max(maxY, ry)
	}
	span := math.Max(maxX-minX, maxY-minY) + 1
	sc := b.tileR * 1.05 / span
	ox, oy := x-sc*(minX+maxX)/2, y-sc*(minY+maxY)/2-b.tileR*0.08
	for a := 0; a < int(gr.N); a++ {
		for c := 0; c < int(gr.N); c++ {
			if gr.Out[a]&(1<<c) == 0 {
				continue
			}
			pa, pc := pts[a], pts[c]
			vector.StrokeLine(dst, float32(ox+sc*pa[0]), float32(oy+sc*pa[1]), float32(ox+sc*pc[0]), float32(oy+sc*pc[1]), g.u(1.2), fade(colDim, 0.6), true)
		}
	}
	for s := 0; s < int(gr.N); s++ {
		px, py := ox+sc*pts[s][0], oy+sc*pts[s][1]
		sl := h.Slots[s]
		dot := fade(colDim, 0.35)
		if sl.Kind == skill.SlotPart {
			dot = rarityCol[sl.Part.Rarity]
			if sl.Part.Kind == skill.KindTrigger {
				dot = color.RGBA{0xff, 0xe0, 0x60, 0xff}
			}
		}
		vector.DrawFilledCircle(dst, float32(px), float32(py), float32(math.Max(1.5*g.k, sc*0.22)), dot, true)
	}
	name := shortName(sim.Load.Name(i))
	if sim.Load.IsMod(i) {
		name = "MOD " + name
	}
	if b.tileR > 22*g.k {
		g.label(dst, g.small, name, x, y+b.tileR*0.62, col, true)
	}
	// Ports: green triangles pointing in, orange pointing out.
	ps, n := h.Shape.Ports()
	for _, p := range ps[:n] {
		side := sim.Load.PortSide(i, p)
		dx, dy := skill.CellDirs[side].XY()
		mx, my := x+dx*b.tileR*0.83, y+dy*b.tileR*0.83
		c := colOut
		dir := 1.0
		if p.In {
			c, dir = colIn, -1
		}
		if h.Docks[p.Slot] != nil || (p.In && dockedInto(sim, i)) {
			c = colDock
		}
		s := math.Max(5*g.k, b.tileR*0.13)
		tx, ty := mx+dx*s*dir, my+dy*s*dir
		var path vector.Path
		path.MoveTo(float32(tx), float32(ty))
		path.LineTo(float32(mx-dx*s*0.4*dir-dy*s), float32(my-dy*s*0.4*dir+dx*s))
		path.LineTo(float32(mx-dx*s*0.4*dir+dy*s), float32(my-dy*s*0.4*dir-dx*s))
		path.Close()
		vs, is := path.AppendVerticesAndIndicesForFilling(nil, nil)
		paint(vs, c)
		dst.DrawTriangles(vs, is, whiteSub(), &ebiten.DrawTrianglesOptions{AntiAlias: true})
	}
}

func dockedInto(sim *game.Sim, i int) bool {
	for _, d := range sim.Docks() {
		if d.To == i {
			return true
		}
	}
	return false
}

func (g *Game) drawBoardButtons(dst *ebiten.Image, l *edLayout) {
	e, sim := &g.ed, g.sim
	label := "+ MODULE"
	if sim.Load.IsMod(e.bd.sel) {
		label = "REMOVE"
	}
	g.drawRect(dst, l.scrap, colPanel, colDock)
	g.label(dst, g.face, label, l.scrap.x+l.scrap.w/2, l.scrap.y+l.scrap.h/2, colDock, true)
	g.drawRect(dst, l.info, colPanel, color.RGBA{0x3a, 0x42, 0x58, 0xff})
	ic := colText
	if e.bad {
		ic = colHot
	}
	msg := e.msg
	if msg == "" {
		msg = g.boardHelp()
	}
	g.wrapLabel(dst, msg, l.info, ic)
	g.drawRect(dst, l.close, fade(colStairs, 0.25), colStairs)
	g.label(dst, g.face, "RESUME", l.close.x+l.close.w/2, l.close.y+l.close.h/2, colText, true)
}

// drawPorts marks a hex's ports in the hex view: a small triangle off
// the port slot's open side, green for the way in, orange for a way out,
// blue where it's docked. The board turns hexes; here they're unturned.
func (g *Game) drawPorts(dst *ebiten.Image, l *edLayout, hex int) {
	h := g.sim.Load.Hex(hex)
	ps, n := h.Shape.Ports()
	for _, p := range ps[:n] {
		c := l.slots[p.Slot]
		dx, dy := skill.CellDirs[p.Side].XY()
		mx, my := c[0]+dx*l.slotR*1.02, c[1]+dy*l.slotR*1.02
		col, dir := colOut, 1.0
		if p.In {
			col, dir = colIn, -1
		}
		if h.Docks[p.Slot] != nil || (p.In && dockedInto(g.sim, hex)) {
			col = colDock
		}
		s := 7 * g.k
		tx, ty := mx+dx*s*dir, my+dy*s*dir
		var path vector.Path
		path.MoveTo(float32(tx), float32(ty))
		path.LineTo(float32(mx-dx*s*0.4*dir-dy*s), float32(my-dy*s*0.4*dir+dx*s))
		path.LineTo(float32(mx-dx*s*0.4*dir+dy*s), float32(my-dy*s*0.4*dir-dx*s))
		path.Close()
		vs, is := path.AppendVerticesAndIndicesForFilling(nil, nil)
		paint(vs, col)
		dst.DrawTriangles(vs, is, whiteSub(), &ebiten.DrawTrianglesOptions{AntiAlias: true})
	}
}

func rectImage(r rect) image.Rectangle {
	return image.Rect(int(r.x), int(r.y), int(math.Ceil(r.x+r.w)), int(math.Ceil(r.y+r.h)))
}

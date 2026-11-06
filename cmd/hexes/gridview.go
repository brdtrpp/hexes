package main

// The skill grid, a tab of the workbench: every node of the save's grid
// as a cell of a big honeycomb around the starter hex. What you own is
// lit, the cells next to it show their cards and prices, rarer nodes
// glow through the fog from further out, and rings the ladder hasn't
// opened show a lock. Tap a cell to read it, hold BUY to buy it. Drag to
// pan and pinch to zoom on a phone, double-tap to center; on a keyboard,
// arrows or WASD pan, + and - zoom, Enter held buys.

import (
	"fmt"
	"image/color"
	"math"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"hexes/game"
	"hexes/loot"
	"hexes/skill"
)

var (
	colData   = color.RGBA{0x6c, 0xf2, 0xff, 0xff} // Data and the grid
	colSignal = color.RGBA{0xc8, 0x9c, 0xff, 0xff} // signal fragments
	colFog    = color.RGBA{0x0c, 0x0e, 0x15, 0xff}
	colFogEdg = color.RGBA{0x1c, 0x20, 0x2c, 0xff}
)

// holdFrames is how long BUY is held to buy: 0.4 s at 60 frames a second.
const holdFrames = 24

// gridView is the grid tab's own state.
type gridView struct {
	sel      int // picked cell, or -1
	panX     float64
	panY     float64 // in grid units, centered here
	zoom     float64
	ptr      map[ebiten.TouchID]*pointer
	mouse    *pointer
	pinch    float64
	pinchOn  bool
	holdFrom int  // frame BUY went down, -1 when it's up
	held     bool // this hold already bought
	aim      bool // a tap spends a signal fragment
	tapAt    int  // frame of the last tap, for a double tap
	tapCell  int
}

// setGrid shows or hides the grid tab.
func (g *Game) setGrid(on bool) {
	e := &g.ed
	e.grid, e.board, e.pick, e.shape = on, false, pickNone, false
	g.sim.GridPeek = false // the fog comes back whenever you leave or open the grid
	if !on {
		return
	}
	gv := &e.gv
	if gv.zoom == 0 {
		gv.zoom = 1
	}
	gv.holdFrom, gv.held, gv.aim = -1, false, false
	gv.sel = -1
	g.gridCenter(g.sim.GridNewest)
	g.sim.GridRingsSeen = max(g.sim.GridRingsSeen, g.sim.RingsOpen())
	e.msg, e.bad = g.gridHelp(), false
}

func (g *Game) gridCenter(i int) {
	if i < 0 || i >= len(g.sim.Grid.Nodes) {
		i = 0
	}
	g.ed.gv.panX, g.ed.gv.panY = g.sim.Grid.Nodes[i].At.Pixel()
}

func (g *Game) gridHelp() string {
	sim := g.sim
	t := "Tap a cell to read it. Hold BUY to buy one next to what you own."
	if sim.Owned() <= 1 {
		t = "Your grid. Spend Data on the cells next to the middle: tap one, then hold BUY. Kills, new depths and salvage pay Data."
	}
	return t
}

// gridLayout is where the grid tab's pieces sit.
type gridLayout struct {
	area   rect // where the grid is drawn
	status rect // Data and the next ring, over the grid
	test   rect
	buy    rect // the hold-to-buy button (SCRAP's place)
	signal rect // SIGNAL (the cast button's place)
	cx, cy float64
	size   float64 // one grid unit on screen: a cell's radius
}

func (g *Game) gridLayout(l *edLayout) gridLayout {
	var b gridLayout
	k := g.k
	top := l.shape.y
	bot := math.Min(l.info.y, l.scrap.y) - 10*k
	x0 := l.tabs[0].x
	w := l.tabs[len(l.tabs)-1].x + l.tabs[len(l.tabs)-1].w - x0
	capH := l.shape.h
	b.test = l.test
	b.test.x = x0 + w - b.test.w
	b.status = rect{x0, top, b.test.x - 8*k - x0, capH}
	b.area = rect{x0, top + capH + 6*k, w, bot - top - capH - 6*k}
	b.buy, b.signal = l.scrap, l.bind
	// About three rings either way of the middle at the starting zoom.
	// On a short screen (a phone on its side) cells stay big enough to
	// tap and fewer rings show.
	fit := math.Min(b.area.w/(2*(3.2*math.Sqrt(3)+1)), b.area.h/(2*(4*1.5+1.2)))
	fit = math.Max(fit, 16*k)
	b.size = fit * g.ed.gv.zoom
	b.cx = b.area.x + b.area.w/2 - g.ed.gv.panX*b.size
	b.cy = b.area.y + b.area.h/2 - g.ed.gv.panY*b.size
	return b
}

func (b *gridLayout) at(c skill.Coord) (x, y float64) {
	px, py := c.Pixel()
	return b.cx + px*b.size, b.cy + py*b.size
}

// cellAt is the grid cell under a point, or -1.
func (g *Game) gridCellAt(b *gridLayout, x, y float64) int {
	// Back from the screen to axial coordinates, then round to a cell.
	px, py := (x-b.cx)/b.size, (y-b.cy)/b.size
	r := py / 1.5
	q := px/math.Sqrt(3) - r/2
	s := -q - r
	rq, rr, rs := math.Round(q), math.Round(r), math.Round(s)
	dq, dr, ds := math.Abs(rq-q), math.Abs(rr-r), math.Abs(rs-s)
	switch {
	case dq > dr && dq > ds:
		rq = -rr - rs
	case dr > ds:
		rr = -rq - rs
	}
	return g.sim.Grid.Index(skill.Coord{Q: int(rq), R: int(rr)})
}

// updateGrid runs the grid tab's input for a frame.
func (g *Game) updateGrid() {
	gv := &g.ed.gv
	if gv.ptr == nil {
		gv.ptr = map[ebiten.TouchID]*pointer{}
	}
	l := g.edLayout()
	b := g.gridLayout(&l)
	step := 0.25 / gv.zoom
	if ebiten.IsKeyPressed(ebiten.KeyArrowLeft) || ebiten.IsKeyPressed(ebiten.KeyA) {
		gv.panX -= step
	}
	if ebiten.IsKeyPressed(ebiten.KeyArrowRight) || ebiten.IsKeyPressed(ebiten.KeyD) {
		gv.panX += step
	}
	if ebiten.IsKeyPressed(ebiten.KeyArrowUp) || ebiten.IsKeyPressed(ebiten.KeyW) {
		gv.panY -= step
	}
	if ebiten.IsKeyPressed(ebiten.KeyArrowDown) || ebiten.IsKeyPressed(ebiten.KeyS) {
		gv.panY += step
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEqual) || inpututil.IsKeyJustPressed(ebiten.KeyKPAdd) {
		g.zoomGrid(1.25, b.area.x+b.area.w/2, b.area.y+b.area.h/2)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyMinus) || inpututil.IsKeyJustPressed(ebiten.KeyKPSubtract) {
		g.zoomGrid(0.8, b.area.x+b.area.w/2, b.area.y+b.area.h/2)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyC) {
		g.gridCenter(g.sim.GridNewest)
	}
	if _, wy := ebiten.Wheel(); wy != 0 {
		x, y := ebiten.CursorPosition()
		g.zoomGrid(math.Pow(1.1, wy), float64(x), float64(y))
	}
	holding := ebiten.IsKeyPressed(ebiten.KeyEnter)
	for _, id := range inpututil.AppendJustPressedTouchIDs(nil) {
		g.touched = true
		x, y := ebiten.TouchPosition(id)
		gv.ptr[id] = &pointer{x0: float64(x), y0: float64(y), x: float64(x), y: float64(y)}
		gv.pinchOn = false
	}
	ids := ebiten.AppendTouchIDs(nil)
	for _, id := range ids {
		if p := gv.ptr[id]; p != nil && b.buy.hit(p.x0, p.y0) {
			holding = true
		}
	}
	if len(ids) == 2 {
		p, q := gv.ptr[ids[0]], gv.ptr[ids[1]]
		if p != nil && q != nil {
			x1, y1 := ebiten.TouchPosition(ids[0])
			x2, y2 := ebiten.TouchPosition(ids[1])
			d := math.Hypot(float64(x2-x1), float64(y2-y1))
			if gv.pinchOn && gv.pinch > 0 && d > 0 {
				g.zoomGrid(d/gv.pinch, float64(x1+x2)/2, float64(y1+y2)/2)
			}
			gv.pinch, gv.pinchOn = d, true
			p.moved, q.moved = true, true
			p.x, p.y, q.x, q.y = float64(x1), float64(y1), float64(x2), float64(y2)
		}
	} else if len(ids) == 1 {
		if p := gv.ptr[ids[0]]; p != nil {
			x, y := ebiten.TouchPosition(ids[0])
			g.gridDrag(p, float64(x), float64(y), &b)
		}
	}
	for _, id := range inpututil.AppendJustReleasedTouchIDs(nil) {
		p := gv.ptr[id]
		delete(gv.ptr, id)
		if p != nil && !p.moved && len(gv.ptr) == 0 {
			g.gridTap(p.x0, p.y0)
		}
	}
	if len(ids) < 2 {
		gv.pinchOn = false
	}
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		x, y := ebiten.CursorPosition()
		gv.mouse = &pointer{x0: float64(x), y0: float64(y), x: float64(x), y: float64(y)}
	}
	if gv.mouse != nil && ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
		x, y := ebiten.CursorPosition()
		g.gridDrag(gv.mouse, float64(x), float64(y), &b)
		if b.buy.hit(gv.mouse.x0, gv.mouse.y0) {
			holding = true
		}
	}
	if gv.mouse != nil && inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft) {
		if !gv.mouse.moved {
			g.gridTap(gv.mouse.x0, gv.mouse.y0)
		}
		gv.mouse = nil
	}
	// BUY: held for 0.4 s, it buys once per hold.
	switch {
	case !holding:
		gv.holdFrom, gv.held = -1, false
	case gv.holdFrom < 0:
		gv.holdFrom = g.frame
		if _, err := g.sim.CanBuy(gv.sel); err != nil {
			g.gridRefuse(err)
			gv.held = true
		}
	case !gv.held && g.frame-gv.holdFrom >= holdFrames:
		gv.held = true
		g.gridBuy()
	}
	R := float64(g.sim.Grid.Rings)*1.6 + 2
	gv.panX = math.Max(-R, math.Min(R, gv.panX))
	gv.panY = math.Max(-R, math.Min(R, gv.panY))
}

// gridDrag pans while a pointer that started on the grid moves.
func (g *Game) gridDrag(p *pointer, x, y float64, b *gridLayout) {
	if !b.area.hit(p.x0, p.y0) {
		return
	}
	if !p.moved && math.Hypot(x-p.x0, y-p.y0) > 10*g.k {
		p.moved = true
	}
	if p.moved {
		g.ed.gv.panX -= (x - p.x) / b.size
		g.ed.gv.panY -= (y - p.y) / b.size
	}
	p.x, p.y = x, y
}

// zoomGrid zooms by f around a screen point, keeping it still.
func (g *Game) zoomGrid(f, x, y float64) {
	gv := &g.ed.gv
	l := g.edLayout()
	b := g.gridLayout(&l)
	px, py := (x-b.cx)/b.size, (y-b.cy)/b.size
	gv.zoom = math.Max(0.35, math.Min(3, gv.zoom*f))
	nb := g.gridLayout(&l)
	gv.panX += px - (x-nb.cx)/nb.size
	gv.panY += py - (y-nb.cy)/nb.size
}

func (g *Game) gridTap(x, y float64) {
	e, sim, gv := &g.ed, g.sim, &g.ed.gv
	l := g.edLayout()
	b := g.gridLayout(&l)
	for i, t := range l.tabs {
		if t.hit(x, y) && g.edTabOn(i) {
			switch i {
			case game.MaxSlots + 1:
				return // already here
			case game.MaxSlots:
				g.setBoard(true)
			default:
				e.hex, e.grid, e.pick, e.msg = i, false, pickNone, ""
			}
			return
		}
	}
	switch {
	case l.close.hit(x, y):
		e.on = false
		return
	case l.help.hit(x, y):
		e.msg, e.bad = "Data buys cells next to what you own, at a workbench. Rarer cells glow from further out. A death lets go of a few outer cells; buy them back at half price. Unbanked Data is safe at each checkpoint.", false
		return
	case b.buy.hit(x, y):
		return // BUY works by holding, in updateGrid
	case b.signal.hit(x, y) && sim.Signals > 0:
		gv.aim = !gv.aim
		if gv.aim {
			e.msg, e.bad = fmt.Sprintf("Tap a cell: the fog within %d steps of it shows what kind each cell is.", game.SignalRadius), false
		} else {
			e.msg, e.bad = "", false
		}
		return
	case b.test.hit(x, y):
		sim.GridPeek = !sim.GridPeek
		if sim.GridPeek {
			e.msg, e.bad = "Test look: the fog is lifted while TEST is lit. Nothing you see here counts as found, and sealed rings stay sealed. Tap TEST again, or leave the grid, and the fog is back.", false
		} else {
			e.msg, e.bad = "Fog's back.", false
		}
		return
	}
	if !b.area.hit(x, y) {
		return
	}
	i := g.gridCellAt(&b, x, y)
	if i < 0 {
		gv.sel, e.msg, e.bad = -1, g.gridHelp(), false
		return
	}
	if g.frame-gv.tapAt < 20 && gv.tapCell == i {
		g.gridCenter(i)
	}
	gv.tapAt, gv.tapCell = g.frame, i
	if gv.aim {
		gv.aim = false
		if err := sim.UseSignal(i); err != nil {
			e.msg, e.bad = upFirst(err.Error())+".", true
			return
		}
		g.click(true)
		e.msg, e.bad = "The fog around it shows what kind each cell is now.", false
		return
	}
	gv.sel = i
	e.msg, e.bad = "", false
	g.click(false)
}

func (g *Game) gridRefuse(err error) {
	g.ed.msg, g.ed.bad = upFirst(err.Error())+".", true
	if g.ed.gv.sel < 0 {
		g.ed.msg = "Tap a cell next to what you own first, then hold BUY."
	}
}

func (g *Game) gridBuy() {
	e, sim, gv := &g.ed, g.sim, &g.ed.gv
	i := gv.sel
	n := sim.Grid.Nodes[i]
	reclaim := sim.Reclaim[i]
	if err := sim.Buy(i); err != nil {
		g.gridRefuse(err)
		return
	}
	if s := &g.snd; s.on() && s.ctx.IsReady() {
		s.dir.Chime(int(n.Rarity))
	}
	switch {
	case reclaim:
		e.msg = "Taken back. You kept what it gave you."
	case n.Kind == game.NodePart:
		e.msg = "Bought: " + g.nodeTitle(&n) + ". It's in your LOCKER: open a hex to slot it."
	case n.Kind == game.NodeFrame:
		e.msg = "Bought the " + n.Frame.Name() + " frame. It's on your FRAMES rack."
	case n.Kind == game.NodeShape:
		e.msg = "Learned the " + n.Shape.Name() + " shape. SHAPE in a hex steps to it."
	case n.Kind == game.NodeTile:
		e.msg = "Logic tile: " + n.Tile.String() + ". Tap an IF gate twice to build with it."
	case n.Kind == game.NodeBoard:
		e.msg = "One more board cell is open."
	}
	e.bad = false
}

func upFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// nodeTitle is a node's short name: what you'd call it on a card.
func (g *Game) nodeTitle(n *game.Node) string {
	switch n.Kind {
	case game.NodeStart:
		return "your starter hex"
	case game.NodePart:
		t, d := loot.Label(n.Part, g.sim.Reg)
		return strings.TrimSpace(t + " " + d)
	case game.NodeFrame:
		return n.Frame.Name() + " frame"
	case game.NodeShape:
		return n.Shape.Name() + " shape"
	case game.NodeTile:
		return "logic tile " + n.Tile.String()
	case game.NodeBoard:
		return "board cell"
	}
	return ""
}

// nodeLabel is what fits on a cell: two short lines.
func (g *Game) nodeLabel(n *game.Node) (string, string, color.RGBA) {
	switch n.Kind {
	case game.NodeStart:
		return "START", "", colText
	case game.NodePart:
		t, d := loot.Label(n.Part, g.sim.Reg)
		return t, d, partCol(n.Part)
	case game.NodeFrame:
		return "FRAME", n.Frame.Name(), frameCol(n.Frame)
	case game.NodeShape:
		return "SHAPE", shapeTag(n.Shape), colShape
	case game.NodeTile:
		return "TILE", n.Tile.String(), colGate
	case game.NodeBoard:
		return "BOARD", "+1 cell", colDock
	}
	return "", "", colDim
}

// kindWord is what a signal fragment shows of a node.
func kindWord(n *game.Node) string {
	switch n.Kind {
	case game.NodePart:
		if game.IsSustain(n.Part) {
			return "SUSTAIN"
		}
		return strings.ToUpper(n.Part.Kind.String())
	case game.NodeFrame:
		return "FRAME"
	case game.NodeShape:
		return "SHAPE"
	case game.NodeTile:
		return "TILE"
	case game.NodeBoard:
		return "BOARD"
	}
	return ""
}

// partCol is a part's label color, as on its tile.
func partCol(p skill.Part) color.RGBA {
	switch p.Kind {
	case skill.KindTrigger:
		return color.RGBA{0xff, 0xe0, 0x60, 0xff}
	case skill.KindTarget:
		return color.RGBA{0x7d, 0xf0, 0xe0, 0xff}
	case skill.KindGate:
		return colGate
	case skill.KindCharge:
		return colCharge
	case skill.KindCall:
		return colCall
	case skill.KindEffect:
		switch {
		case p.Effect.Typed():
			return typeColor(p.Type)
		case p.Effect == skill.EffectMend:
			return colMend
		case p.Effect == skill.EffectShield:
			return colShield
		case p.Effect == skill.EffectPurge:
			return colPurge
		}
	}
	return colText
}

func (g *Game) drawGridTab(dst *ebiten.Image, t rect) {
	sim := g.sim
	fill, edge := colPanel, fade(colData, 0.45)
	if g.ed.grid {
		fill, edge = fade(colData, 0.22), colData
	}
	g.drawRect(dst, t, fill, edge)
	g.label(dst, g.small, "GRID", t.x+t.w/2, t.y+t.h*0.33, colData, true)
	g.label(dst, g.small, fmt.Sprintf("%d data", sim.DataTotal()), t.x+t.w/2, t.y+t.h*0.72, colDim, true)
	if sim.RingsOpen() > sim.GridRingsSeen && g.frame/20%2 == 0 {
		// A ring opened since you last looked.
		vector.StrokeRect(dst, float32(t.x-3*g.k), float32(t.y-3*g.k), float32(t.w+6*g.k), float32(t.h+6*g.k), g.u(2), colLesson, false)
	}
}

func (g *Game) drawGrid(dst *ebiten.Image) {
	sim, e, gv := g.sim, &g.ed, &g.ed.gv
	l := g.edLayout()
	b := g.gridLayout(&l)
	k := g.k
	// The status row: Data, unbanked, the next ring.
	x := b.status.x
	cy := b.status.y + b.status.h/2
	s := fmt.Sprintf("DATA %d", sim.Data)
	g.label(dst, g.face, s, x, cy, colData, false)
	x += float64(len(s))*9*k + 14*k
	if sim.Unbanked > 0 {
		s = fmt.Sprintf("+%d unbanked", sim.Unbanked)
		g.label(dst, g.small, s, x, cy, colSalvage, false)
		x += float64(len(s))*7*k + 14*k
	}
	if at := sim.NextRingDepth(); at > 0 && x < b.status.x+b.status.w-100*k {
		g.label(dst, g.small, fmt.Sprintf("ring %d opens at depth %d", sim.RingsOpen()+1, at), x, cy, colDim, false)
	}
	if sim.GridPeek {
		g.drawRect(dst, b.test, fade(colShape, 0.55), colShape)
		g.label(dst, g.small, "TEST ON", b.test.x+b.test.w/2, b.test.y+b.test.h/2, colText, true)
	} else {
		g.drawRect(dst, b.test, colPanel, fade(colDim, 0.6))
		g.label(dst, g.small, "TEST", b.test.x+b.test.w/2, b.test.y+b.test.h/2, colDim, true)
	}

	// The cells, clipped to the area.
	sub := dst.SubImage(rectImage(b.area)).(*ebiten.Image)
	vector.StrokeRect(dst, float32(b.area.x), float32(b.area.y), float32(b.area.w), float32(b.area.h), g.u(1), colFogEdg, false)
	tile := b.size * 0.93
	words := tile > 21*k
	pulse := float32(0.55 + 0.45*math.Sin(float64(g.frame)*0.12))
	for i := range sim.Grid.Nodes {
		n := &sim.Grid.Nodes[i]
		cx, cy := b.at(n.At)
		if cx < b.area.x-tile || cx > b.area.x+b.area.w+tile || cy < b.area.y-tile || cy > b.area.y+b.area.h+tile {
			continue
		}
		v := sim.CellView(i)
		switch v {
		case game.ViewDark:
			drawHex(sub, cx, cy, tile, false, colFog, colFogEdg, g.u(1))
		case game.ViewVoid:
			// Nothing there: just the gap.
			drawHex(sub, cx, cy, tile*0.35, false, color.RGBA{}, fade(colFogEdg, 0.8), g.u(1))
		case game.ViewGlow:
			drawHex(sub, cx, cy, tile, false, colFog, colFogEdg, g.u(1))
			c := rarityCol[n.Rarity]
			drawHex(sub, cx, cy, tile*0.55, false, fade(c, 0.25*pulse), fade(c, 0.7*pulse), g.u(2))
		case game.ViewKind:
			drawHex(sub, cx, cy, tile, false, colFog, fade(colSignal, 0.6), g.u(1.5))
			if words {
				g.label(sub, g.small, kindWord(n), cx, cy, fade(colSignal, 0.9), true)
			}
		case game.ViewSealed:
			drawHex(sub, cx, cy, tile, false, color.RGBA{0x10, 0x12, 0x1a, 0xff}, color.RGBA{0x30, 0x34, 0x42, 0xff}, g.u(1.5))
			g.drawLock(sub, cx, cy-tile*0.15, tile*0.22)
			if words {
				g.label(sub, g.small, fmt.Sprintf("depth %d", game.RingDepth(n.Ring)), cx, cy+tile*0.4, fade(colDim, 0.7), true)
			}
		case game.ViewOwned:
			t1, _, c := g.nodeLabel(n)
			drawHex(sub, cx, cy, tile, false, fade(c, 0.5), c, g.u(2.5))
			drawHex(sub, cx, cy, tile*0.78, false, color.RGBA{}, fade(colText, 0.35), g.u(1))
			if n.Kind == game.NodePart {
				g.partGlyph(sub, cx, cy, tile*0.6, n.Part)
			}
			if words {
				g.label(sub, g.small, t1, cx, cy, colText, true)
			}
		case game.ViewCard, game.ViewReclaim:
			t1, t2, c := g.nodeLabel(n)
			cost, err := sim.CanBuy(i)
			edge := rarityCol[n.Rarity]
			fill := color.RGBA{0x1a, 0x1f, 0x2d, 0xff}
			w := g.u(2)
			if v == game.ViewReclaim {
				edge, fill = fade(colData, 0.5), color.RGBA{0x12, 0x18, 0x22, 0xff}
			}
			near := sim.GridDist(i) == 1
			if near && err == nil {
				// Buyable now: a pulsing edge.
				edge, w = fade(edge, 0.5+0.5*pulse), g.u(3)
			} else if near && sim.DataTotal() < cost {
				edge = fade(edge, 0.45)
			}
			drawHex(sub, cx, cy, tile, false, fill, edge, w)
			if n.Kind == game.NodePart {
				g.partGlyph(sub, cx, cy, tile*0.6, n.Part)
			}
			if words {
				tc := c
				if v == game.ViewReclaim {
					tc = fade(c, 0.6)
				}
				g.label(sub, g.small, t1, cx, cy-tile*0.3, tc, true)
				if tile > 38*k {
					g.label(sub, g.small, t2, cx, cy+tile*0.05, colDim, true)
				}
				if near {
					pc := colData
					if sim.DataTotal() < cost {
						pc = fade(colDim, 0.7)
					}
					p := fmt.Sprintf("%d", cost)
					if v == game.ViewReclaim {
						p = fmt.Sprintf("%d back", cost)
					}
					g.label(sub, g.small, p, cx, cy+tile*0.45, pc, true)
				}
			}
		}
		if i == gv.sel {
			drawHex(sub, cx, cy, tile*1.08, false, color.RGBA{}, colText, g.u(3))
		}
	}

	// Info bar: the picked cell.
	g.drawRect(dst, l.info, colPanel, color.RGBA{0x3a, 0x42, 0x58, 0xff})
	info, ic := g.gridInfo()
	g.wrapLabel(dst, info, l.info, ic)
	g.drawRect(dst, l.help, colPanel, fade(colDim, 0.7))
	g.label(dst, g.face, "?", l.help.x+l.help.w/2, l.help.y+l.help.h/2, colText, true)

	// BUY fills as it's held.
	_, err := sim.CanBuy(gv.sel)
	bc := colData
	if err != nil {
		bc = fade(colDim, 0.6)
	}
	g.drawRect(dst, b.buy, colPanel, bc)
	if gv.holdFrom >= 0 && !gv.held {
		f := math.Min(1, float64(g.frame-gv.holdFrom)/holdFrames)
		vector.DrawFilledRect(dst, float32(b.buy.x), float32(b.buy.y), float32(b.buy.w*f), float32(b.buy.h), fade(colData, 0.4), false)
	}
	g.label(dst, g.face, "HOLD BUY", b.buy.x+b.buy.w/2, b.buy.y+b.buy.h/2, bc, true)
	if sim.Signals > 0 {
		fill := colPanel
		if gv.aim {
			fill = fade(colSignal, 0.35)
		}
		g.drawRect(dst, b.signal, fill, colSignal)
		g.label(dst, g.small, fmt.Sprintf("SIGNAL x%d", sim.Signals), b.signal.x+b.signal.w/2, b.signal.y+b.signal.h/2, colSignal, true)
	}
	g.drawRect(dst, l.close, fade(colStairs, 0.25), colStairs)
	g.label(dst, g.face, "RESUME", l.close.x+l.close.w/2, l.close.y+l.close.h/2, colText, true)
	_ = e
}

// gridInfo is the info bar's line for the picked cell.
func (g *Game) gridInfo() (string, color.RGBA) {
	e, sim, gv := &g.ed, g.sim, &g.ed.gv
	if e.msg != "" {
		if e.bad {
			return e.msg, colHot
		}
		return e.msg, colText
	}
	i := gv.sel
	if i < 0 || i >= len(sim.Grid.Nodes) {
		return g.gridHelp(), colDim
	}
	n := &sim.Grid.Nodes[i]
	where := fmt.Sprintf("ring %d", n.Ring)
	if n.Ring > 0 {
		where += ", " + game.RingIdea(n.Ring)
	}
	switch sim.CellView(i) {
	case game.ViewDark:
		return "Fog. Buy a cell next to it to see what's there.", colDim
	case game.ViewVoid:
		return "Empty: nothing to buy here, paths bend round it.", colDim
	case game.ViewGlow:
		return fmt.Sprintf("Something %s glows through the fog here, %d steps out.", strings.ToLower(n.Rarity.String()), sim.GridDist(i)), rarityCol[n.Rarity]
	case game.ViewKind:
		return "A signal showed a " + strings.ToLower(kindWord(n)) + " here. Buy your way next to it to see more.", colSignal
	case game.ViewSealed:
		return fmt.Sprintf("Ring %d opens at depth %d: %s.", n.Ring, game.RingDepth(n.Ring), game.RingIdea(n.Ring)), colDim
	case game.ViewOwned:
		return "Yours: " + g.nodeTitle(n) + ". " + g.nodeText(n), colText
	}
	cost, err := sim.CanBuy(i)
	lead := fmt.Sprintf("%d Data: ", cost)
	if n.Kind != game.NodePart {
		lead = fmt.Sprintf("%d Data, %s %s. ", cost, strings.ToLower(n.Rarity.String()), g.nodeTitle(n))
	}
	if sim.Reclaim[i] {
		lead = fmt.Sprintf("Let go in a death: %d Data to take it back (you kept what it gave). ", cost)
	}
	text := lead + g.nodeText(n) + " (" + where + ")"
	if err != nil && sim.GridDist(i) == 1 {
		return upFirst(err.Error()) + ". " + text, rarityCol[n.Rarity]
	}
	return text, rarityCol[n.Rarity]
}

// nodeText says what a node gives.
func (g *Game) nodeText(n *game.Node) string {
	switch n.Kind {
	case game.NodePart:
		return loot.Describe(n.Part, g.sim.Reg)
	case game.NodeFrame:
		return n.Frame.Text(g.sim.Reg)
	case game.NodeShape:
		return game.ShapeText(n.Shape)
	case game.NodeTile:
		return "A logic tile for IF gates."
	case game.NodeBoard:
		return "One more board cell: room for another hex or module."
	case game.NodeStart:
		return "Where every path starts."
	}
	return ""
}

// drawLock draws a small padlock centered at x, y.
func (g *Game) drawLock(dst *ebiten.Image, x, y, r float64) {
	c := color.RGBA{0x5a, 0x60, 0x74, 0xff}
	vector.DrawFilledRect(dst, float32(x-r), float32(y-r*0.2), float32(2*r), float32(r*1.3), c, false)
	vector.StrokeCircle(dst, float32(x), float32(y-r*0.25), float32(r*0.6), g.u(1.5), c, true)
}

package main

// The open board's window in the hex editor. The board is bigger than a
// phone's screen, so the editor shows part of it: drag to look around,
// pinch, roll the wheel or tap + and - to zoom. A press on the board
// that never moves is a tap, handled when it lifts; anywhere else a press
// taps at once, as before.

import (
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// edZoomMax is how far in the board zooms, over the scale that shows it
// whole.
const edZoomMax = 12.0

// edViewInput runs the editor's pointers while an open board shows.
func (g *Game) edViewInput(l *edLayout) {
	v := &g.ed.bv
	if v.ptr == nil {
		v.ptr = map[ebiten.TouchID]*pointer{}
	}
	if _, wy := ebiten.Wheel(); wy != 0 {
		x, y := ebiten.CursorPosition()
		if l.board.hit(float64(x), float64(y)) {
			g.edZoom(math.Pow(1.12, wy), float64(x), float64(y))
		}
	}
	mid := func() (float64, float64) { return l.board.x + l.board.w/2, l.board.y + l.board.h/2 }
	if inpututil.IsKeyJustPressed(ebiten.KeyEqual) || inpututil.IsKeyJustPressed(ebiten.KeyKPAdd) {
		x, y := mid()
		g.edZoom(1.25, x, y)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyMinus) || inpututil.IsKeyJustPressed(ebiten.KeyKPSubtract) {
		x, y := mid()
		g.edZoom(0.8, x, y)
	}
	// press starts a pointer: on + or -, it zooms; on the board, it waits
	// to see whether it drags; elsewhere it taps now.
	press := func(x, y float64) *pointer {
		switch {
		case l.zin.hit(x, y):
			mx, my := mid()
			g.edZoom(1.25, mx, my)
			return nil
		case l.zout.hit(x, y):
			mx, my := mid()
			g.edZoom(0.8, mx, my)
			return nil
		case l.board.hit(x, y):
			return &pointer{x0: x, y0: y, x: x, y: y}
		}
		g.edTap(x, y)
		return nil
	}
	for _, id := range inpututil.AppendJustPressedTouchIDs(nil) {
		g.touched = true
		x, y := ebiten.TouchPosition(id)
		if p := press(float64(x), float64(y)); p != nil {
			v.ptr[id] = p
			v.pinchOn = false
		}
	}
	ids := ebiten.AppendTouchIDs(nil)
	if len(ids) == 2 && v.ptr[ids[0]] != nil && v.ptr[ids[1]] != nil {
		p, q := v.ptr[ids[0]], v.ptr[ids[1]]
		x1, y1 := ebiten.TouchPosition(ids[0])
		x2, y2 := ebiten.TouchPosition(ids[1])
		d := math.Hypot(float64(x2-x1), float64(y2-y1))
		if v.pinchOn && v.pinch > 0 && d > 0 {
			g.edZoom(d/v.pinch, float64(x1+x2)/2, float64(y1+y2)/2)
		}
		v.pinch, v.pinchOn = d, true
		p.moved, q.moved = true, true
		p.x, p.y, q.x, q.y = float64(x1), float64(y1), float64(x2), float64(y2)
	} else if len(ids) == 1 {
		if p := v.ptr[ids[0]]; p != nil {
			x, y := ebiten.TouchPosition(ids[0])
			g.edDrag(p, float64(x), float64(y), l)
		}
	}
	if len(ids) < 2 {
		v.pinchOn = false
	}
	for _, id := range inpututil.AppendJustReleasedTouchIDs(nil) {
		p := v.ptr[id]
		delete(v.ptr, id)
		if p != nil && !p.moved && len(v.ptr) == 0 {
			g.edTap(p.x0, p.y0)
		}
	}
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		x, y := ebiten.CursorPosition()
		v.mouse = press(float64(x), float64(y))
	}
	if v.mouse != nil && ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
		x, y := ebiten.CursorPosition()
		g.edDrag(v.mouse, float64(x), float64(y), l)
	}
	if v.mouse != nil && inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft) {
		if !v.mouse.moved {
			g.edTap(v.mouse.x0, v.mouse.y0)
		}
		v.mouse = nil
	}
}

// edHold keeps the window where it is now, so a drag or zoom starts from
// what's on screen.
func (g *Game) edHold(l *edLayout) {
	v := &g.ed.bv
	if !v.set || v.hex != g.ed.hex {
		v.zoom = l.R / l.fitR
	}
	v.cx, v.cy, v.set, v.hex = l.viewX, l.viewY, true, g.ed.hex
}

// edDrag moves the window while a pointer that began on the board moves.
func (g *Game) edDrag(p *pointer, x, y float64, l *edLayout) {
	if !p.moved && math.Hypot(x-p.x0, y-p.y0) > 10*g.k {
		p.moved = true
		g.edHold(l)
	}
	if p.moved {
		v := &g.ed.bv
		cur := g.edLayout() // the window clamps at the board's edge
		v.cx = cur.viewX - (x-p.x)/cur.R
		v.cy = cur.viewY - (y-p.y)/cur.R
	}
	p.x, p.y = x, y
}

// edZoom zooms the window by f around a screen point, keeping the board
// point under it still.
func (g *Game) edZoom(f, x, y float64) {
	l := g.edLayout()
	if l.board.w <= 0 {
		return // no open board showing
	}
	g.edHold(&l)
	v := &g.ed.bv
	bx := l.viewX + (x-(l.board.x+l.board.w/2))/l.R
	by := l.viewY + (y-(l.board.y+l.board.h/2))/l.R
	v.zoom = math.Max(1, math.Min(edZoomMax, v.zoom*f))
	r := l.fitR * v.zoom
	v.cx = bx - (x-(l.board.x+l.board.w/2))/r
	v.cy = by - (y-(l.board.y+l.board.h/2))/r
}

// drawEdViewControls draws + and - over the board's corner, and a line
// saying the board drags while some of it is out of view.
func (g *Game) drawEdViewControls(dst *ebiten.Image, l *edLayout) {
	for i, r := range []rect{l.zout, l.zin} {
		g.drawRect(dst, r, colPanel, fade(colDim, 0.8))
		cx, cy := float32(r.x+r.w/2), float32(r.y+r.h/2)
		s := float32(r.w * 0.28)
		vector.StrokeLine(dst, cx-s, cy, cx+s, cy, g.u(2), colText, true)
		if i == 1 {
			vector.StrokeLine(dst, cx, cy-s, cx, cy+s, g.u(2), colText, true)
		}
	}
	if l.zoomed {
		g.label(dst, g.small, "drag to see more of the board", l.board.x+6*g.k, l.zin.y+l.zin.h/2, fade(colDim, 0.85), false)
	}
}

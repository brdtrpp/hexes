package main

// Dungeon zoom. Pinch with two fingers on a phone, roll the mouse wheel
// on a desktop, or press + and - (0 goes back to the start). The camera
// keeps its fixed 36° tilt and stays on the player; zoom only changes how
// big a hex draws, between zoomMin and zoomMax of the size the window
// picks. It eases to where you asked so a wheel notch glides rather than
// jumps, and it's saved once it settles.
//
// Fingers: a pinch is two touches that aren't on a button. Fingers that
// took part in a pinch don't steer or hold the stick until they lift, so
// letting go of one finger never walks you anywhere. In stick mode a
// second finger landing just after the first (within pinchGrace) turns
// the new stick into a pinch, so a pinch on the left half works too; a
// stick that's been held longer than that stays a stick.
//
// The floor and body sprites are drawn for one hex size. While the zoom
// moves they're scaled (spriteK) and only redrawn once it settles or has
// drifted far, so a pinch stays smooth on a phone.

import (
	"math"
	"strconv"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

const (
	zoomPref = "hexes.zoom"
	zoomMin  = 0.7
	zoomMax  = 2.0
	// zoomStep is one wheel notch or key press.
	zoomStep = 1.12
	// zoomEase is the share of the way to the target covered each frame.
	zoomEase = 0.22
	// zoomSettle is how many still frames count as settled: tiles are
	// redrawn and the zoom saved.
	zoomSettle = 20
	// pinchGrace is how many frames after a stick plants a second finger
	// can still turn it into a pinch.
	pinchGrace = 15
)

type zoomState struct {
	z, want float64 // the zoom drawn, and where it's heading (1 is the start)
	base    float64 // the window's own hex size, pixels
	still   int     // frames since the zoom last moved
	saved   float64 // the zoom last written to the pref

	pinch  bool
	ids    [2]int  // the two fingers pinching
	d0, z0 float64 // their spread and the zoom when the pinch began
	locked map[int]bool
	// stickAt is the frame the stick last planted.
	stickAt int
}

func clampZoom(z float64) float64 { return math.Max(zoomMin, math.Min(zoomMax, z)) }

// init loads the saved zoom; anything unreadable is the start.
func (zs *zoomState) init(pref string) {
	z, err := strconv.ParseFloat(pref, 64)
	if err != nil || math.IsNaN(z) || z <= 0 {
		z = 1
	}
	zs.z = clampZoom(z)
	zs.want, zs.saved = zs.z, zs.z
	zs.still = zoomSettle
	zs.locked = map[int]bool{}
}

// nudge multiplies the target zoom by f.
func (zs *zoomState) nudge(f float64) { zs.want = clampZoom(zs.want * f) }

// reset heads back to the start.
func (zs *zoomState) reset() { zs.want = 1 }

// beginPinch starts a pinch between fingers a and b, d apart.
func (zs *zoomState) beginPinch(a, b int, d float64) {
	zs.pinch, zs.ids = true, [2]int{a, b}
	zs.d0, zs.z0 = math.Max(d, 1), zs.want
	zs.locked[a], zs.locked[b] = true, true
}

// pinchTo follows the pinch to spread d: the zoom grows as the fingers
// part, in proportion.
func (zs *zoomState) pinchTo(d float64) { zs.want = clampZoom(zs.z0 * d / zs.d0) }

// step eases the zoom toward its target, and reports whether it moved.
func (zs *zoomState) step() bool {
	if zs.z == zs.want {
		zs.still++
		return false
	}
	zs.z += (zs.want - zs.z) * zoomEase
	if math.Abs(zs.want-zs.z) < 0.002 {
		zs.z = zs.want
	}
	zs.still = 0
	return true
}

// settled reports whether the zoom has stopped and no pinch holds it.
func (zs *zoomState) settled() bool { return !zs.pinch && zs.still >= zoomSettle }

// release forgets fingers that lifted: a pinch ends when either of its
// fingers lifts, and a locked finger steers again only once it's down
// fresh.
func (zs *zoomState) release(down map[int]bool) {
	if zs.pinch && (!down[zs.ids[0]] || !down[zs.ids[1]]) {
		zs.pinch = false
	}
	for id := range zs.locked {
		if !down[id] {
			delete(zs.locked, id)
		}
	}
}

// size is the hex size to draw at.
func (zs *zoomState) size() float64 { return zs.base * zs.z }

// spriteK is how much the prebuilt sprites scale to match the hex size.
func (g *Game) spriteK() float64 {
	if g.tileSize <= 0 {
		return 1
	}
	return g.size / g.tileSize
}

// needTiles reports whether the floor and body sprites want redrawing at
// the current size: once the zoom settles, or mid-zoom when they've been
// stretched far enough to look soft.
func (g *Game) needTiles() bool {
	if g.tiles == nil {
		return true
	}
	k := g.spriteK()
	if k == 1 {
		return false
	}
	return g.zm.settled() || k < 0.8 || k > 1.25
}

// freeTouch is one touch that isn't on a button.
type freeTouch struct {
	id   int
	x, y float64
}

// updateZoom reads the wheel, the zoom keys and pinches. free is every
// touch this frame that isn't on a button; stick fingers are in it too.
func (g *Game) updateZoom(free []freeTouch, down map[int]bool) {
	zs := &g.zm
	zs.release(down)
	if _, wy := ebiten.Wheel(); wy != 0 {
		zs.nudge(math.Pow(zoomStep, math.Max(-3, math.Min(3, wy))))
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEqual) || inpututil.IsKeyJustPressed(ebiten.KeyKPAdd) {
		zs.nudge(zoomStep)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyMinus) || inpututil.IsKeyJustPressed(ebiten.KeyKPSubtract) {
		zs.nudge(1 / zoomStep)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyDigit0) || inpututil.IsKeyJustPressed(ebiten.KeyKP0) {
		zs.reset()
	}
	g.pinchFrom(free)
	g.applyZoom()
}

// pinchFrom starts, follows or ignores a pinch among the free touches.
func (g *Game) pinchFrom(free []freeTouch) {
	zs := &g.zm
	at := func(id int) (freeTouch, bool) {
		for _, p := range free {
			if p.id == id {
				return p, true
			}
		}
		return freeTouch{}, false
	}
	if zs.pinch {
		a, okA := at(zs.ids[0])
		b, okB := at(zs.ids[1])
		if okA && okB {
			zs.pinchTo(math.Hypot(a.x-b.x, a.y-b.y))
		}
		return
	}
	// Fingers that can pinch: not locked, and not a stick that's been
	// held past the grace.
	var can []freeTouch
	for _, p := range free {
		if zs.locked[p.id] {
			continue
		}
		if g.stick.On && g.stick.ID == p.id && g.frame-zs.stickAt > pinchGrace {
			continue
		}
		can = append(can, p)
	}
	if len(can) < 2 {
		return
	}
	a, b := can[0], can[1]
	if g.stick.On && (g.stick.ID == a.id || g.stick.ID == b.id) {
		g.stick.End()
	}
	zs.beginPinch(a.id, b.id, math.Hypot(a.x-b.x, a.y-b.y))
}

// applyZoom eases the zoom, resizes the hex, and saves once it settles.
func (g *Game) applyZoom() {
	zs := &g.zm
	if zs.base <= 0 {
		return
	}
	zs.step()
	g.size = zs.size()
	if zs.settled() && zs.z != zs.saved {
		zs.saved = zs.z
		savePref(zoomPref, strconv.FormatFloat(zs.z, 'f', 3, 64))
	}
}

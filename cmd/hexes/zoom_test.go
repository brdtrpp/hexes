package main

import (
	"math"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"hexes/dungeon"
	"hexes/game"
)

func zoomGame() *Game {
	g := &Game{sim: game.New(1), w: 420, h: 860, k: 1}
	g.zm.init("")
	g.zm.base = 20
	g.size = g.zm.size()
	return g
}

func settle(g *Game) {
	for i := 0; i < 200; i++ {
		g.applyZoom()
	}
}

// The saved zoom loads, junk loads as the start, and both ends clamp.
func TestZoomPref(t *testing.T) {
	var zs zoomState
	for in, want := range map[string]float64{"": 1, "junk": 1, "1.4": 1.4, "9": zoomMax, "0.01": zoomMin, "-2": 1, "NaN": 1} {
		zs.init(in)
		if zs.z != want || zs.want != want {
			t.Errorf("pref %q loads as %.2f, want %.2f", in, zs.z, want)
		}
	}
}

// A wheel notch glides: the zoom moves part way each frame, lands where
// asked, and never leaves the range however hard you push.
func TestZoomEasesAndClamps(t *testing.T) {
	g := zoomGame()
	g.zm.nudge(zoomStep)
	g.applyZoom()
	if g.zm.z <= 1 || g.zm.z >= zoomStep {
		t.Fatalf("first frame jumped to %.3f", g.zm.z)
	}
	settle(g)
	if math.Abs(g.zm.z-zoomStep) > 1e-9 || math.Abs(g.size-20*zoomStep) > 1e-6 {
		t.Fatalf("settled at %.3f (size %.2f)", g.zm.z, g.size)
	}
	for i := 0; i < 50; i++ {
		g.zm.nudge(zoomStep)
	}
	settle(g)
	if g.zm.z != zoomMax {
		t.Fatalf("zoomed in to %.2f, max is %.2f", g.zm.z, zoomMax)
	}
	for i := 0; i < 50; i++ {
		g.zm.nudge(1 / zoomStep)
	}
	settle(g)
	if g.zm.z != zoomMin {
		t.Fatalf("zoomed out to %.2f, min is %.2f", g.zm.z, zoomMin)
	}
	g.zm.reset()
	settle(g)
	if g.zm.z != 1 {
		t.Fatal("0 didn't go back to the start")
	}
}

// Two free fingers pinch: spreading them zooms in by the spread, and
// both stay locked out of steering until they lift.
func TestPinch(t *testing.T) {
	g := zoomGame()
	two := []freeTouch{{1, 100, 400}, {2, 200, 400}}
	down := map[int]bool{1: true, 2: true}
	g.zm.release(down)
	g.pinchFrom(two)
	if !g.zm.pinch || !g.zm.locked[1] || !g.zm.locked[2] {
		t.Fatal("two fingers didn't start a pinch")
	}
	g.pinchFrom([]freeTouch{{1, 75, 400}, {2, 225, 400}}) // 100 apart to 150
	if math.Abs(g.zm.want-1.5) > 1e-9 {
		t.Fatalf("spreading 1.5x asked for %.3f", g.zm.want)
	}
	// One finger lifts: the pinch ends, the other stays locked.
	g.zm.release(map[int]bool{2: true})
	if g.zm.pinch || g.zm.locked[1] || !g.zm.locked[2] {
		t.Fatalf("after a lift: pinch %v locked %v", g.zm.pinch, g.zm.locked)
	}
	g.pinchFrom([]freeTouch{{2, 225, 400}})
	if g.zm.pinch {
		t.Fatal("one finger pinched")
	}
	g.zm.release(map[int]bool{})
	if len(g.zm.locked) != 0 {
		t.Fatal("lifted fingers stay locked")
	}
}

// In stick mode a stick that's been held is safe from a second finger,
// but a second finger landing right after the first turns it into a
// pinch.
func TestPinchAndStick(t *testing.T) {
	g := zoomGame()
	g.stickMode = true
	g.stick.Radius = 50
	g.frame = 100
	g.stick.Begin(1, 100, 600)
	g.zm.stickAt = 100
	g.frame = 100 + pinchGrace + 5
	g.pinchFrom([]freeTouch{{1, 100, 600}, {2, 300, 500}})
	if g.zm.pinch || !g.stick.On {
		t.Fatal("a held stick got pinched away by a tap")
	}
	g.stick.End()
	g.stick.Begin(3, 100, 600)
	g.zm.stickAt = g.frame
	g.frame += 3
	g.pinchFrom([]freeTouch{{3, 100, 600}, {4, 160, 560}})
	if !g.zm.pinch || g.stick.On {
		t.Fatal("two quick fingers didn't pinch")
	}
}

// Sprites are reused while the zoom moves a little and redrawn once it
// settles or drifts far; taps still land under the finger at both ends
// of the range.
func TestZoomTilesAndTaps(t *testing.T) {
	g := zoomGame()
	g.tiles = map[dungeon.Tile]*ebiten.Image{}
	g.tileSize = g.size
	if g.needTiles() {
		t.Fatal("redraw with nothing changed")
	}
	g.zm.nudge(1.1)
	g.applyZoom()
	if g.needTiles() {
		t.Fatal("redraw mid-glide on a small step")
	}
	settle(g)
	if !g.needTiles() {
		t.Fatal("no redraw once settled")
	}
	g.tileSize = g.size
	g.zm.pinch = true
	g.zm.want = zoomMax
	for i := 0; i < 10; i++ {
		g.applyZoom()
	}
	if !g.needTiles() {
		t.Fatalf("no redraw at %.2fx stretch", g.spriteK())
	}
	for _, z := range []float64{zoomMin, zoomMax} {
		g.size = 20 * z
		g.camX, g.camY = 12.3, 7.9
		for sy := 0.0; sy <= 860; sy += 860.0 / 9 {
			for sx := 0.0; sx <= 420; sx += 420.0 / 7 {
				x, y := g.unproject(sx, sy)
				bx, by, _ := g.project(x, y)
				if math.Hypot(bx-sx, by-sy) > 0.01 {
					t.Fatalf("zoom %.1f: (%.0f, %.0f) came back as (%.2f, %.2f)", z, sx, sy, bx, by)
				}
			}
		}
	}
}

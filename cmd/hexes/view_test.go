package main

import (
	"math"
	"testing"

	"hexes/game"
)

// A tap maps back to the floor point drawn under it, on phone and
// desktop, across the whole screen.
func TestProjectRoundTrip(t *testing.T) {
	for _, sc := range []struct{ w, h int }{{420, 860}, {1280, 800}} {
		g := &Game{sim: game.New(1), w: sc.w, h: sc.h, k: 1, size: 34}
		g.camX, g.camY = 12.3, 7.9
		for sy := 0.0; sy <= float64(sc.h); sy += float64(sc.h) / 9 {
			for sx := 0.0; sx <= float64(sc.w); sx += float64(sc.w) / 7 {
				x, y := g.unproject(sx, sy)
				bx, by, _ := g.project(x, y)
				if math.Hypot(bx-sx, by-sy) > 0.01 {
					t.Fatalf("%dx%d: (%.0f, %.0f) came back as (%.2f, %.2f)", sc.w, sc.h, sx, sy, bx, by)
				}
			}
		}
		// Further up the screen is further away: smaller.
		if g.scaleAt(g.camY-5) >= g.scaleAt(g.camY+5) {
			t.Fatal("no perspective: far rows draw as big as near ones")
		}
	}
}

package main

// The camera. It stays fixed (no rotation, no player control) but is
// pitched back from straight down, with a touch of perspective: the floor
// squashes toward the top of the screen and things further up draw a
// little smaller, so the station reads as a place with depth rather than
// a map. Everything in the world goes through project; taps come back
// through unproject.

import "math"

const (
	// viewPitch is how far the camera leans back from straight down.
	viewPitch = 36 * math.Pi / 180
	// viewDist is the camera's distance as a multiple of the screen
	// height: smaller is stronger perspective.
	viewDist = 2.4
	// viewDrop moves the player this share of the screen height below
	// the middle, so more of the floor ahead of you shows.
	viewDrop = 0.06
	// bodyLift is how high a body's top sits, in hex units: bodies are
	// short machines standing on the floor, not decals on it.
	bodyLift = 0.42
	// chestLift is where hits land and bolts fly, in hex units.
	chestLift = 0.34
)

var (
	viewCos = math.Cos(viewPitch) // how much the floor squashes
	viewSin = math.Sin(viewPitch) // how much of an upright height shows
)

// viewD is the camera distance in pixels.
func (g *Game) viewD() float64 { return viewDist * float64(g.h) }

// project maps a point on the floor in hex units to the screen, and
// returns the perspective scale there (1 at the player's row).
func (g *Game) project(x, y float64) (sx, sy, p float64) {
	d := g.viewD()
	v := (y - g.camY) * g.size
	den := d - v*viewSin
	if den < d*0.25 {
		den = d * 0.25
	}
	p = d / den
	sx = float64(g.w)/2 + (x-g.camX)*g.size*p + g.look.shx
	sy = float64(g.h)*(0.5+viewDrop) + v*viewCos*p + g.look.shy
	return
}

// unproject maps a screen point back to the floor, in hex units.
func (g *Game) unproject(sx, sy float64) (x, y float64) {
	d := g.viewD()
	Y := sy - float64(g.h)*(0.5+viewDrop) - g.look.shy
	den := viewCos*d + Y*viewSin
	if den < viewCos*d*0.2 {
		den = viewCos * d * 0.2
	}
	v := Y * d / den
	p := d / math.Max(d-v*viewSin, d*0.25)
	return g.camX + (sx-float64(g.w)/2-g.look.shx)/(g.size*p), g.camY + v/g.size
}

// scaleAt is the perspective scale at floor row y (hex units).
func (g *Game) scaleAt(y float64) float64 {
	d := g.viewD()
	return d / math.Max(d-(y-g.camY)*g.size*viewSin, d*0.25)
}

// lifted is where a point h hex units above the floor at x, y draws, and
// the perspective scale there.
func (g *Game) lifted(x, y, h float64) (sx, sy, p float64) {
	sx, sy, p = g.project(x, y)
	sy -= h * g.size * viewSin * p
	return
}

// upright is how many pixels a height of h hex units stands at scale p.
func (g *Game) upright(h, p float64) float64 { return h * g.size * viewSin * p }

// viewBounds is the floor area the screen can show, in hex units, with
// margin hexes to spare on every side (more below, where walls and bodies
// stand up into view).
func (g *Game) viewBounds(margin float64) (x0, y0, x1, y1 float64) {
	w, h := float64(g.w), float64(g.h)
	ax, ay := g.unproject(0, 0)
	bx, _ := g.unproject(w, 0)
	cx, cy := g.unproject(0, h)
	dx, _ := g.unproject(w, h)
	x0 = math.Min(ax, cx) - margin
	x1 = math.Max(bx, dx) + margin
	y0 = ay - margin
	y1 = cy + margin + 1.5
	return
}

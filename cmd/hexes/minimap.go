package main

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"hexes/dungeon"
	"hexes/skill"
)

// The minimap is a small chart of the floor in the top right corner that
// fills in as the player explores, so the bigger deep floors stay easy to
// read. Each hex is a 2x2 pixel block with odd rows nudged half a block,
// the same offset layout the floor uses. What counts as explored is the
// sim's Sight (game/sight.go), the same record the fog draws from, so the
// chart shows exactly the ground the lamp has lit. The chart image is only
// redrawn when that record gains a hex.

const (
	mapMaxW = 128.0
	mapMaxH = 96.0
)

type minimap struct {
	floor *dungeon.Floor
	gen   int // Sight.Gen the chart was drawn at
	img   *ebiten.Image
	px    []byte
}

var (
	mapGround = color.RGBA{0x45, 0x5a, 0x78, 0xff}
	mapWall   = color.RGBA{0x22, 0x28, 0x38, 0xff}
	mapPit    = color.RGBA{0x8a, 0x2b, 0x3f, 0xff}
	mapHazard = color.RGBA{0xc2, 0x4e, 0x12, 0xff}
)

// updateMinimap redraws the chart when the explored record has grown. It
// resets on a new floor.
func (g *Game) updateMinimap() {
	m := &g.mm
	f := g.sim.Floor
	sight := &g.sim.Sight
	if f == nil || len(sight.Seen) != f.W*f.H {
		return
	}
	if m.floor != f {
		m.floor = f
		m.gen = -1
		iw, ih := 2*f.W+1, 2*f.H
		if m.img == nil || m.img.Bounds().Dx() != iw || m.img.Bounds().Dy() != ih {
			if m.img != nil {
				m.img.Deallocate()
			}
			m.img = ebiten.NewImage(iw, ih)
			m.px = make([]byte, 4*iw*ih)
		}
	}
	if m.gen == sight.Gen {
		return
	}
	m.gen = sight.Gen
	clear(m.px)
	iw := 2*f.W + 1
	for i, ok := range sight.Seen {
		if !ok {
			continue
		}
		var c color.RGBA
		switch t := f.Tiles[i]; t {
		case dungeon.Ground:
			c = mapGround
		case dungeon.Pit:
			c = mapPit
		case dungeon.Hazard:
			c = mapHazard
		case dungeon.Stairs:
			c = colStairs
		default:
			if !g.nearOpen(f.CoordOf(i)) {
				continue
			}
			c = mapWall
		}
		r, col := i/f.W, i%f.W
		x, y := 2*col+r&1, 2*r
		for _, o := range [4]int{0, 1, iw, iw + 1} {
			j := 4 * (y*iw + x + o)
			m.px[j], m.px[j+1], m.px[j+2], m.px[j+3] = c.R, c.G, c.B, c.A
		}
	}
	m.img.WritePixels(m.px)
}

// mapPos is where hex c sits on the chart, in chart pixels.
func mapPos(c skill.Coord) (float32, float32) {
	col := c.Q + c.R>>1
	return float32(2*col+c.R&1) + 1, float32(2*c.R) + 1
}

// drawMinimap draws the chart under the top right buttons (under the hex
// slots on a narrow phone screen) with the player, the stairs once found
// and the salvage cache when it's on this floor.
func (g *Game) drawMinimap(screen *ebiten.Image) {
	g.updateMinimap()
	m := &g.mm
	if m.img == nil || g.ed.on || g.menu != menuClosed {
		return
	}
	f := m.floor
	iw, ih := float64(2*f.W+1), float64(2*f.H)
	sc := min(float64(g.u(mapMaxW))/iw, float64(g.u(mapMaxH))/ih)
	dw, dh := float32(iw*sc), float32(ih*sc)
	pad := g.u(4)
	x0 := float32(g.w) - g.u(14) - dw
	y0 := g.u(44)
	if float32(g.w) < g.u(700) { // narrow: the bars and slots fill the top
		y0 = g.u(162)
	}
	vector.DrawFilledRect(screen, x0-pad, y0-pad, dw+2*pad, dh+2*pad, colPanel, false)
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(sc, sc)
	op.GeoM.Translate(float64(x0), float64(y0))
	op.ColorScale.ScaleAlpha(0.9)
	screen.DrawImage(m.img, op)

	at := func(c skill.Coord) (float32, float32) {
		x, y := mapPos(c)
		return x0 + x*float32(sc), y0 + y*float32(sc)
	}
	dot := func(c skill.Coord, r float32, col color.RGBA) {
		x, y := at(c)
		vector.DrawFilledCircle(screen, x, y, r, col, true)
	}
	if g.sim.Sight.Seen[f.Index(f.Exit)] { // a breathing ring so the stairs stand out
		x, y := at(f.Exit)
		pulse := 0.6 + 0.4*float32(g.frame%40)/40
		vector.StrokeCircle(screen, x, y, g.u(4)*pulse+g.u(1), g.u(1.5), colStairs, true)
	}
	if c := &g.sim.Cache; c.Active && c.Depth == g.sim.Depth {
		dot(c.At, g.u(2.5), colSalvage)
	}
	dot(g.sim.P().At, g.u(2.5), colPlayer)
}

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
// the same offset layout the floor uses. The chart image is only redrawn
// when the player steps onto a new tile.

const (
	mapReveal = 8 // hexes around the player that get charted
	mapMaxW   = 128.0
	mapMaxH   = 96.0
)

type minimap struct {
	floor *dungeon.Floor
	seen  []bool
	last  skill.Coord
	img   *ebiten.Image
	px    []byte
	dirty bool
}

var (
	mapGround = color.RGBA{0x45, 0x5a, 0x78, 0xff}
	mapWall   = color.RGBA{0x22, 0x28, 0x38, 0xff}
	mapPit    = color.RGBA{0x8a, 0x2b, 0x3f, 0xff}
	mapHazard = color.RGBA{0xc2, 0x4e, 0x12, 0xff}
)

// updateMinimap charts the tiles around the player. It resets on a new
// floor and does nothing while the player stays on one tile.
func (g *Game) updateMinimap() {
	m := &g.mm
	f := g.sim.Floor
	if f == nil {
		return
	}
	p := g.sim.P().At
	if m.floor != f {
		m.floor = f
		n := f.W * f.H
		if cap(m.seen) < n {
			m.seen = make([]bool, n)
		}
		m.seen = m.seen[:n]
		clear(m.seen)
		iw, ih := 2*f.W+1, 2*f.H
		if m.img == nil || m.img.Bounds().Dx() != iw || m.img.Bounds().Dy() != ih {
			if m.img != nil {
				m.img.Deallocate()
			}
			m.img = ebiten.NewImage(iw, ih)
			m.px = make([]byte, 4*iw*ih)
		}
		m.last = skill.Coord{Q: -1 << 20}
	}
	if p == m.last {
		return
	}
	m.last = p
	for dr := -mapReveal; dr <= mapReveal; dr++ {
		for dq := max(-mapReveal, -dr-mapReveal); dq <= min(mapReveal, -dr+mapReveal); dq++ {
			c := skill.Coord{Q: p.Q + dq, R: p.R + dr}
			if f.In(c) {
				if i := f.Index(c); !m.seen[i] {
					m.seen[i] = true
					m.dirty = true
				}
			}
		}
	}
	if !m.dirty {
		return
	}
	m.dirty = false
	clear(m.px)
	iw := 2*f.W + 1
	for i, ok := range m.seen {
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
	if m.seen[f.Index(f.Exit)] { // a breathing ring so the stairs stand out
		x, y := at(f.Exit)
		pulse := 0.6 + 0.4*float32(g.frame%40)/40
		vector.StrokeCircle(screen, x, y, g.u(4)*pulse+g.u(1), g.u(1.5), colStairs, true)
	}
	if c := &g.sim.Cache; c.Active && c.Depth == g.sim.Depth {
		dot(c.At, g.u(2.5), colSalvage)
	}
	dot(g.sim.P().At, g.u(2.5), colPlayer)
}

package main

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"

	"hexes/skill"
)

// silhouette draws piece p's shape small, centered on x, y and size
// across: the same outline it has on every card and drop.
func (g *Game) silhouette(dst *ebiten.Image, x, y, size float64, p skill.Part, col color.RGBA) {
	pc := skill.PartPiece(p)
	if pc == nil || len(pc.Cells) == 0 {
		return
	}
	minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, c := range pc.Cells {
		cx, cy := c.XY()
		minX, minY = math.Min(minX, cx), math.Min(minY, cy)
		maxX, maxY = math.Max(maxX, cx), math.Max(maxY, cy)
	}
	span := math.Max(maxX-minX+1, maxY-minY+1)
	r := size / span / 1.05
	ox, oy := (minX+maxX)/2, (minY+maxY)/2
	for _, c := range pc.Cells {
		cx, cy := c.XY()
		drawHex(dst, x+(cx-ox)*r, y+(cy-oy)*r, r*0.56, true, fade(col, 0.55), col, g.u(1))
	}
}

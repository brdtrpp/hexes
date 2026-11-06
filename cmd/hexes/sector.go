package main

import (
	"github.com/hajimehoshi/ebiten/v2"

	"hexes/dungeon"
	"hexes/game"
)

// The floor's kind (dungeon.Layout) is named when you arrive, with the one
// thing that changes how you fight there, and stays next to the depth in
// the HUD.

var sectorNames = [dungeon.NumLayouts]string{"CAVERNS", "PILLAR HALLS", "WARRENS", "REACTOR CORE", "THE RIFT"}

// sectorTags are the short names next to the depth in the HUD, short
// enough for a phone's top bar.
var sectorTags = [dungeon.NumLayouts]string{"CAVERNS", "HALLS", "WARRENS", "REACTOR", "RIFT"}

var sectorHints = [dungeon.NumLayouts]string{
	"twisting rooms, mixed corridors",
	"big rooms; the pillars block gunfire",
	"tight tunnels; fight them at the chokes",
	"a pit rings the core; cross at the bridges",
	"a chasm splits the floor; push them in",
}

// arriveShow is how long, in frames, the arrival name stays up.
const arriveShow = 150

func sectorTag(l dungeon.Layout) string {
	if int(l) < len(sectorTags) {
		return sectorTags[l]
	}
	return ""
}

// watchSector starts the arrival name whenever a new floor loads. The
// first floor is always caverns, so it isn't announced.
func (g *Game) watchSector() {
	f := g.sim.Floor
	if f == nil || f.Seed == g.arriveSeed {
		if g.arriveT > 0 {
			g.arriveT--
		}
		return
	}
	g.arriveSeed = f.Seed
	g.arriveT = 0
	if f.Depth > 1 {
		g.arriveT = arriveShow
	}
}

func (g *Game) drawSector(screen *ebiten.Image) {
	if g.arriveT <= 0 || g.sim.State != game.Playing || g.ed.on {
		return
	}
	l := g.sim.Floor.Layout
	if int(l) >= len(sectorNames) {
		return
	}
	a := float32(min(1, float64(g.arriveT)/30))
	if t := arriveShow - g.arriveT; t < 15 {
		a = min(a, float32(t)/15)
	}
	W, H := float64(g.w), float64(g.h)
	// Between the HUD cards up top and the player in the middle.
	y := H * 0.36
	g.outlined(screen, g.big, sectorNames[l], W/2, y, colText, a)
	g.outlined(screen, g.face, sectorHints[l], W/2, y+float64(g.u(30)), colDim, a)
}

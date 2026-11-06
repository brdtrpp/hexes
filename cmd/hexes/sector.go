package main

import (
	"github.com/hajimehoshi/ebiten/v2"

	"hexes/dungeon"
	"hexes/game"
	"hexes/skill"
)

// The floor's kind (dungeon.Layout) is named when you arrive, with the one
// thing that changes how you fight there, and stays next to the depth in
// the HUD. A mixed floor names each kind again as you cross into it, and
// the HUD reads FROM > TO while you're on the crossing.

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

// sectorLower is a kind's name in a sentence.
var sectorLower = [dungeon.NumLayouts]string{"caverns", "pillar halls", "warrens", "a reactor core", "the rift"}

// watchSector starts the arrival name whenever a new floor loads, and on
// a mixed floor again each time you cross into the next kind. The first
// floor is always caverns, so it isn't announced. The region you're in
// changes only most of the way across a crossing, so dithering on the
// border doesn't flash names.
func (g *Game) watchSector() {
	f := g.sim.Floor
	if f == nil {
		return
	}
	if f.Seed != g.arriveSeed {
		g.arriveSeed = f.Seed
		g.region = 0
		g.arriveT, g.arriveKind, g.arriveNext = 0, f.Layout, ""
		if f.Mixed() {
			for i, r := range f.Regions[1:] {
				if i == 0 {
					g.arriveNext = "gives way to " + sectorLower[r.Layout]
				} else {
					g.arriveNext += ", then " + sectorLower[r.Layout]
				}
			}
		}
		if f.Depth > 1 {
			g.arriveT = arriveShow
		}
		return
	}
	if g.arriveT > 0 {
		g.arriveT--
	}
	if !f.Mixed() || g.sim.State != game.Playing {
		return
	}
	a, b, t := f.Mix(g.sim.P().At)
	was := g.region
	switch {
	case a == b:
		g.region = a
	case g.region == a && t > 0.7:
		g.region = b
	case g.region == b && t < 0.3:
		g.region = a
	case g.region != a && g.region != b:
		g.region = a
		if t >= 0.5 {
			g.region = b
		}
	}
	if g.region != was {
		g.arriveT, g.arriveKind = arriveShow, f.Regions[g.region].Layout
		g.arriveNext = "from " + sectorLower[f.Regions[was].Layout]
	}
}

// sectorHUD is the floor's kind for the HUD line: on a mixed floor the
// kind you're in, or "FROM > TO" while you're crossing.
func (g *Game) sectorHUD() string {
	f := g.sim.Floor
	if !f.Mixed() {
		return sectorTag(f.Layout)
	}
	a, b, t := f.Mix(g.sim.P().At)
	if a != b && t > 0.15 && t < 0.85 {
		from, to := a, b
		if g.region == b {
			from, to = b, a
		}
		return sectorTag(f.Regions[from].Layout) + " > " + sectorTag(f.Regions[to].Layout)
	}
	return sectorTag(f.Regions[g.region].Layout)
}

func (g *Game) drawSector(screen *ebiten.Image) {
	if g.arriveT <= 0 || g.sim.State != game.Playing || g.ed.on {
		return
	}
	l := g.arriveKind
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
	if g.arriveNext != "" {
		g.outlined(screen, g.small, g.arriveNext, W/2, y+float64(g.u(50)), fade(colDim, 0.85), a)
	}
}

// sectorTints give each kind its own cast over the depth's zone color,
// blended across a crossing so the change reads before the rooms do.
var sectorTints = [dungeon.NumLayouts][3]float32{
	{1, 1, 1},          // caverns
	{0.92, 1.0, 1.12},  // halls: cold steel
	{1.12, 1.0, 0.86},  // warrens: rust
	{0.88, 1.12, 1.0},  // reactor: coolant green
	{1.06, 0.88, 1.12}, // rift: violet
}

// tintAt is the floor's tint at c: the zone's, times its kind's.
func (g *Game) tintAt(c skill.Coord, zr, zg, zb float32) (float32, float32, float32) {
	f := g.sim.Floor
	if len(f.Regions) == 0 {
		return zr, zg, zb
	}
	a, b, t := f.Mix(c)
	ta, tb := sectorTints[f.Regions[a].Layout], sectorTints[f.Regions[b].Layout]
	w := float32(t)
	return zr * (ta[0] + (tb[0]-ta[0])*w), zg * (ta[1] + (tb[1]-ta[1])*w), zb * (ta[2] + (tb[2]-ta[2])*w)
}

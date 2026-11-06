package main

// Repair pads on the floor (game/repair.go): a mint plate with a plus at
// its heart and a ring round it that empties as the pad gives out hull.
// Standing on one that's patching throws a column of light up off it;
// jammed it flickers amber, spent it goes grey. Once seen, a pad stays
// on the floor dimly, like the stairs, and on the minimap as a plus.

import (
	"fmt"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"hexes/fx"
	"hexes/game"
	"hexes/skill"
)

var (
	colPad      = colMend
	colPadSpent = color.RGBA{0x5a, 0x66, 0x78, 0xff}
	colPadJam   = color.RGBA{0xff, 0xb0, 0x3b, 0xff}
)

// padNear is how many hexes off a pad its name shows.
const padNear = 3.5

// drawPads draws the floor's repair pads.
func (g *Game) drawPads(screen *ebiten.Image) {
	sim := g.sim
	f := sim.Floor
	s := g.size
	t := float64(g.frame) / 60
	for i := range sim.Pads {
		pd := &sim.Pads[i]
		x, y := pd.At.Pixel()
		sx, sy := g.toScreen(x, y)
		if !g.onScreen(sx, sy, s*3) || !sim.Sight.Seen[f.Index(pd.At)] {
			continue
		}
		a := float32(math.Max(0.4, math.Min(1, float64(g.seeAt(x, y))*1.5)))
		frac := float64(pd.Left / game.RepairStore)
		on := sim.OnPad() == i
		col := colPad
		switch {
		case pd.Left <= 0:
			col = colPadSpent
		case on && sim.PadJammed() > 0:
			if g.frame/4%2 == 0 {
				col = colPadJam
			}
		}
		cr, cg, cb := colorF(col)
		breathe := float32(0.75 + 0.25*math.Sin(t*2.4+float64(i)))
		if pd.Left > 0 {
			g.glow(screen, sx, sy, s*1.8, cr, cg, cb, 0.2*a*breathe)
		}
		// The plate, an inner edge, and its rim.
		g.ringPath(sx, sy, s*0.94, fx.RingStyle{Sides: 6}, t, 0, 0, 2*math.Pi, true)
		g.fillPath(screen, col, 0.2*a)
		g.strokePath(screen, col, 0.85*a, g.u(2.5))
		g.ringPath(sx, sy, s*0.6, fx.RingStyle{Sides: 6}, t, 0, 0, 2*math.Pi, true)
		g.strokePath(screen, col, 0.4*a, g.u(1.5))
		// What's left, as a ring that empties clockwise.
		g.ringPath(sx, sy, s*1.22, fx.RingStyle{Sides: 48}, 0, 0, 0, 2*math.Pi, true)
		g.strokePath(screen, col, 0.15*a, g.u(4))
		if frac > 0.01 {
			g.ringPath(sx, sy, s*1.22, fx.RingStyle{Sides: 48}, 0, 0, -math.Pi/2, -math.Pi/2+2*math.Pi*frac, false)
			g.strokePath(screen, col, 0.95*a, g.u(4))
		}
		// The plus, laid on the floor.
		l := 0.3
		for _, d := range [][4]float64{{-l, 0, l, 0}, {0, -l, 0, l}} {
			ax, ay := g.toScreen(x+d[0], y+d[1])
			bx, by := g.toScreen(x+d[2], y+d[3])
			vector.StrokeLine(screen, float32(ax), float32(ay), float32(bx), float32(by), g.u(4), fade(col, a), true)
		}
		if on && sim.Repairing() {
			// A column of light and rings climbing it.
			for j := 0; j < 7; j++ {
				lx, ly, _ := g.lifted(x, y, float64(j)*0.4)
				g.glow(screen, lx, ly, s*(1.1-0.1*float64(j)), cr, cg, cb, 0.34*(1-float32(j)/8))
			}
			for j := 0; j < 3; j++ {
				k := math.Mod(t*1.1+float64(j)/3, 1)
				_, ly, _ := g.lifted(x, y, k*2.4)
				g.ringPath(sx, ly, s*(0.95-0.45*k), fx.RingStyle{Sides: 6}, t, 0, 0, 2*math.Pi, true)
				g.strokePath(screen, col, float32(1-k)*0.9, g.u(2.5))
			}
		}
	}
}

// drawPadLabels names the pads near the player, over the walls and
// bodies, and says how the one underfoot is doing.
func (g *Game) drawPadLabels(screen *ebiten.Image) {
	sim := g.sim
	f := sim.Floor
	s := g.size
	px, py := sim.P().Pos()
	for i := range sim.Pads {
		pd := &sim.Pads[i]
		x, y := pd.At.Pixel()
		sx, sy := g.toScreen(x, y)
		if !g.onScreen(sx, sy, s*3) || !sim.Sight.Seen[f.Index(pd.At)] {
			continue
		}
		a := float32(math.Max(0.4, math.Min(1, float64(g.seeAt(x, y))*1.5)))
		on := sim.OnPad() == i
		col := colPad
		if pd.Left <= 0 {
			col = colPadSpent
		}
		if math.Hypot(px-x, py-y) > padNear*math.Sqrt(3) {
			continue
		}
		ly := sy - s*1.55
		if on { // above the +N the patching throws up
			ly = sy - s*2.4 - float64(g.u(14))
		}
		g.outlined(screen, g.small, "REPAIR PAD", sx, ly, fade(col, a), a)
		if on {
			g.outlined(screen, g.small, g.padStatus(pd), sx, ly+float64(g.u(14)), colText, 1)
		}
	}
}

// padStatus is the line under the pad you're standing on.
func (g *Game) padStatus(pd *game.Pad) string {
	p := g.sim.P()
	left := fmt.Sprintf("%.0f%% left", 100*pd.Left/game.RepairStore)
	switch {
	case pd.Left <= 0:
		return "spent for this floor"
	case g.sim.PadJammed() > 0:
		return "jammed by the hit, " + left
	case p.U.HP >= p.U.MaxHP:
		return "hull full, " + left
	}
	return "patching hull, " + left
}

// watchPads shows the pad's how-to the first time a hurt player steps
// on one in a session.
func (g *Game) watchPads() {
	if g.padTaught || g.sim.OnPad() < 0 || g.sim.State != game.Playing {
		return
	}
	p := g.sim.P()
	if p.U.HP >= p.U.MaxHP {
		return
	}
	g.padTaught = true
	g.showHint(fmt.Sprintf("Repair pad: stand on it to patch %.0f%% hull a second\nHits jam it. It holds %.0f%% of your hull per floor", 100*game.RepairRate, 100*game.RepairStore))
}

// drawPadMarks puts the seen pads on the minimap as small pluses.
func (g *Game) drawPadMarks(screen *ebiten.Image, at func(c skill.Coord) (float32, float32)) {
	f := g.sim.Floor
	for i := range g.sim.Pads {
		pd := &g.sim.Pads[i]
		if !g.sim.Sight.Seen[f.Index(pd.At)] {
			continue
		}
		x, y := at(pd.At)
		c := colPad
		if pd.Left <= 0 {
			c = colPadSpent
		}
		r, w := g.u(3), g.u(1.6)
		vector.StrokeLine(screen, x-r, y, x+r, y, w, c, false)
		vector.StrokeLine(screen, x, y-r, x, y+r, w, c, false)
	}
}

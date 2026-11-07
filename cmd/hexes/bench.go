package main

// Cast buttons and workbenches on screen.
//
// Up to four cast buttons sit in an arc around the bottom right corner,
// where a right thumb rests: button 1 big in the corner, 2 to its left,
// 3 on the diagonal, 4 above. Only buttons with a hex on them show (1
// always does). Each shows its number and the hex's name, in the hex's
// color. Keys 1 to 4 press them at a keyboard; Space is button 1.
//
// Workbenches are consoles on the floor (game/bench.go). The HEX button
// shows only beside one. On a PC the key bar stands in for all of these
// (hud.go).

import (
	"fmt"
	"image/color"
	"math"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"hexes/fx"
	"hexes/skill"
)

var colBench = color.RGBA{0xe0, 0x5c, 0xff, 0xff}

// layoutButtons places the cast buttons, VENT and HEX for a w by h
// screen; r is button 1's radius.
func (g *Game) layoutButtons(w, h int, r float64) {
	k := g.k
	b1 := button{float64(w) - r - 24*k, float64(h) - r - 30*k, r, "1"}
	rs := r * 0.62
	d := r * 1.9 // the small buttons clear button 1 and each other
	g.casts[0] = b1
	for i, a := range [...]float64{math.Pi, math.Pi * 0.75, math.Pi * 0.5} {
		g.casts[i+1] = button{b1.x + d*math.Cos(a), b1.y - d*math.Sin(a), rs, fmt.Sprint(i + 2)}
	}
	rv := r * 0.6
	g.vent = button{g.casts[1].x - rs - rv - 12*k, float64(h) - rv - 24*k, rv, "VENT"}
	g.hexBtn = button{b1.x, g.casts[3].y - rs - rv - 12*k, rv, "HEX"}
}

// castButton is which cast button (1 to Buttons) is under x, y, or 0.
// Buttons with nothing on them don't answer, except button 1.
func (g *Game) castButton(x, y float64) int {
	if g.pcBar() {
		if b := g.keyAt(x, y); b <= len(g.casts) {
			return b
		}
		return 0
	}
	for i, b := range g.casts {
		if (i == 0 || g.sim.Load.ButtonUsed(i+1)) && b.hit(x, y) {
			return i + 1
		}
	}
	return 0
}

// keyButtons are the keyboard's cast keys: 1 to 4, with Space and J
// also on button 1.
var keyButtons = [...][]ebiten.Key{
	{ebiten.KeyDigit1, ebiten.KeySpace, ebiten.KeyJ},
	{ebiten.KeyDigit2},
	{ebiten.KeyDigit3},
	{ebiten.KeyDigit4},
}

// showHint puts a line of help over the play field for a few seconds.
func (g *Game) showHint(s string) {
	g.hint, g.hintT = s, 3.5
}

// drawCastButtons draws the cast buttons: number, hex name, heat ring.
func (g *Game) drawCastButtons(screen *ebiten.Image) {
	load := &g.sim.Load
	rig := &g.sim.Rig
	u := g.u
	for i, b := range g.casts {
		n := i + 1
		first := load.FirstOn(n)
		if first < 0 && i > 0 {
			continue
		}
		c, name := colDim, "CAST"
		if first >= 0 {
			c, name = slotCol[first], shortName(load.Names[first])
			if fl := g.sim.SlotFlash[first]; fl > 0 && g.sim.SlotLast[first] != skill.Fired {
				c = colHot
			}
		}
		g.roundButton(screen, b, c)
		if i == 0 {
			// Heat wraps button 1, so you see it where your thumb is.
			hf := float64(rig.Heat / rig.MaxHeat)
			if hf > 0.01 {
				hc := colHeat
				if hf > 0.75 {
					hc = colHot
				}
				g.ringPath(b.x, b.y, b.r+float64(u(5)), fx.RingStyle{Sides: 48}, 0, 0, -math.Pi/2, -math.Pi/2+2*math.Pi*hf, false)
				g.strokePath(screen, hc, 0.9, u(3.5))
			}
		}
		more := 0
		for j := range load.Keys {
			if j != first && load.OnButton(j, n) {
				more++
			}
		}
		if more > 0 {
			name = fmt.Sprintf("%s+%d", name, more)
		}
		g.drawSchoolRound(screen, b, g.schoolOf(n), c)
		g.outlined(screen, g.face, b.label, b.x, b.y-b.r*0.22, colText, 1)
		g.outlined(screen, g.small, fitWord(g, []string{name, name[:min(len(name), 5)]}, b.r*1.8), b.x, b.y+b.r*0.34, c, 1)
	}
}

// roundButton draws one round touch button in color c.
func (g *Game) roundButton(screen *ebiten.Image, b button, c color.RGBA) {
	u := g.u
	cr, cg, cb := colorF(c)
	g.glow(screen, b.x, b.y, b.r*1.5, cr, cg, cb, 0.22)
	vector.DrawFilledCircle(screen, float32(b.x), float32(b.y), float32(b.r), color.RGBA{0x0c, 0x0f, 0x18, 0xe8}, true)
	vector.DrawFilledCircle(screen, float32(b.x), float32(b.y), float32(b.r), fade(c, 0.2), true)
	vector.DrawFilledCircle(screen, float32(b.x), float32(b.y-b.r*0.12), float32(b.r*0.82), fade(c, 0.1), true)
	vector.StrokeCircle(screen, float32(b.x), float32(b.y), float32(b.r*0.86), u(1), fade(c, 0.45), true)
	vector.StrokeCircle(screen, float32(b.x), float32(b.y), float32(b.r), u(2.5), c, true)
}

// drawHexButton draws HEX, lit and pulsing, while you stand at a bench.
func (g *Game) drawHexButton(screen *ebiten.Image) {
	if !g.hexOn() {
		return
	}
	b := g.hexBtn
	pulse := float32(0.75 + 0.25*math.Sin(float64(g.frame)*0.12))
	g.roundButton(screen, b, fade(colBench, pulse))
	g.outlined(screen, g.face, "HEX", b.x, b.y, colText, 1)
}

// drawBenches draws the floor's workbenches where the lamp shows them: a
// console with a glowing hex screen. The one you're at says how to use it.
func (g *Game) drawBenches(screen *ebiten.Image) {
	s := g.size
	at, near := g.sim.NearBench()
	for i, b := range g.sim.Benches {
		if !g.sim.BenchOn(i) {
			continue // the arrival bench is taught on the first floor
		}
		x, y := b.Pixel()
		sx, sy := g.toScreen(x, y)
		if sx < -s || sy < -s || sx > float64(g.w)+s || sy > float64(g.h)+s {
			continue
		}
		lit := g.seeAt(x, y)
		if lit < 0.15 {
			continue
		}
		a := float32(math.Min(1, float64(lit)*1.5))
		on := near && at == b
		glow := float32(0.12)
		if on {
			glow = 0.22 + 0.08*float32(math.Sin(float64(g.frame)*0.12))
		}
		cr, cg, cb := colorF(colBench)
		g.glow(screen, sx, sy, s*1.3, cr, cg, cb, glow*a)
		w, h := s*1.1, s*0.76
		vector.DrawFilledRect(screen, float32(sx-w/2), float32(sy-h/2), float32(w), float32(h), fade(color.RGBA{0x1a, 0x14, 0x26, 0xff}, a), true)
		vector.StrokeRect(screen, float32(sx-w/2), float32(sy-h/2), float32(w), float32(h), g.u(2), fade(colBench, a), true)
		drawHex(screen, sx, sy, h*0.36, false, fade(colBench, 0.35*a), fade(colBench, a), g.u(1.5))
		if on {
			how := "TAB: edit hexes"
			if !g.pcBar() {
				how = "tap HEX to edit"
			}
			g.outlined(screen, g.small, "WORKBENCH", sx, sy-s*1.05, colBench, 1)
			if g.sim.Depth <= 3 { // the early floors say how
				g.outlined(screen, g.small, how, sx, sy+s*0.95, colText, 1)
			}
		}
	}
}

// drawHint draws the current help line, fading out.
func (g *Game) drawHint(screen *ebiten.Image) {
	if g.hintT <= 0 {
		return
	}
	a := float32(math.Min(1, g.hintT))
	y := float64(g.h) * 0.6
	for _, ln := range strings.Split(g.hint, "\n") {
		g.outlined(screen, g.face, ln, float64(g.w)/2, y, fade(colBench, a), a)
		y += float64(g.u(20))
	}
}

// hexOn reports whether the HEX button shows: only standing at a bench.
func (g *Game) hexOn() bool { return g.sim.AtBench() }

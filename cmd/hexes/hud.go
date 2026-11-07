package main

// The play screen's controls, kept to what a fight needs. The top shows
// depth and hull; Power and Heat ride as arcs round the rig (look.go) and
// heat round button 1. Everything else lives in MENU.
//
// On a phone the cast buttons sit in an arc under the right thumb (the
// left thumb moves). On a PC a slim bar along the bottom shows the keys,
// 1 to 4 and R, each with the hex on it, what it costs and how hot the
// rig runs; a click on a cell works like the key. HEX joins the bar, or
// the arc, only beside a workbench.
//
// Each button also shows its school's state: an overload's charge pips,
// a raised guard and what it has stored, the rage meter, and a follow-up
// lit while it would land on the foe just hit.

import (
	"fmt"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"hexes/fx"
	"hexes/skill"
)

// pcBar reports whether the screen shows the PC key bar rather than the
// thumb buttons: a wide window that has never been touched, on a screen
// that doesn't take touches.
func (g *Game) pcBar() bool {
	return !g.touched && !touchDevice() && float64(g.w) >= 700*g.k
}

// keyCell is one cell of the PC key bar: b is the button (1 to 4), or
// keyVent or keyHex.
type keyCell struct {
	r rect
	b int
}

const (
	keyVent = 5
	keyHex  = 6
)

// keyBar lays out the PC key bar along the bottom centre: the cast keys
// in use, VENT, and HEX beside a workbench.
func (g *Game) keyBar() []keyCell {
	k := g.k
	bs := []int{}
	for b := 1; b <= len(g.casts); b++ {
		if b == 1 || g.sim.Load.ButtonUsed(b) {
			bs = append(bs, b)
		}
	}
	bs = append(bs, keyVent)
	if g.sim.AtBench() {
		bs = append(bs, keyHex)
	}
	cw, ch, gap := 150*k, 44*k, 8*k
	small := 92 * k // VENT and HEX need less room
	total := -gap
	for _, b := range bs {
		w := cw
		if b > 4 {
			w = small
		}
		total += w + gap
	}
	x := (float64(g.w) - total) / 2
	y := float64(g.h) - ch - 14*k
	out := make([]keyCell, 0, len(bs))
	for _, b := range bs {
		w := cw
		if b > 4 {
			w = small
		}
		out = append(out, keyCell{rect{x, y, w, ch}, b})
		x += w + gap
	}
	return out
}

// keyAt is the key bar cell under x, y, or 0.
func (g *Game) keyAt(x, y float64) int {
	for _, c := range g.keyBar() {
		if c.r.hit(x, y) {
			return c.b
		}
	}
	return 0
}

// ventHit and hexHit are whether x, y is on VENT or on HEX, wherever the
// screen shows them.
func (g *Game) ventHit(x, y float64) bool {
	if g.pcBar() {
		return g.keyAt(x, y) == keyVent
	}
	return g.vent.hit(x, y)
}

func (g *Game) hexHit(x, y float64) bool {
	if g.pcBar() {
		return g.keyAt(x, y) == keyHex
	}
	return g.hexOn() && g.hexBtn.hit(x, y)
}

// school is what a button's school shows right now.
type school struct {
	trig skill.TriggerKind
	// rung is the overload's charge or the rage meter's rung (0 none);
	// frac how far the meter or the charge is toward the top.
	rung uint8
	frac float32
	// guard is a raised guard, and scale what its release would deal.
	guard bool
	scale float32
	// follow is a follow-up that lands on the foe just hit, at step.
	follow bool
	step   uint8
}

// schoolOf is button b's school state, from its first hex.
func (g *Game) schoolOf(b int) school {
	load := &g.sim.Load
	i := load.FirstOn(b)
	if i < 0 || load.Progs[i] == nil {
		return school{}
	}
	sc := school{trig: load.Progs[i].Trigger}
	l := g.sim.Disp.Loadout(g.sim.Player)
	if l == nil {
		return sc
	}
	switch sc.trig {
	case skill.Overload:
		if int(l.Held) == i {
			sc.rung = skill.HoldRung(l.HeldFor)
			sc.frac = min(1, l.HeldFor/skill.OverloadRung3)
		}
	case skill.Guard:
		if int(l.Held) == i {
			sc.guard = true
			hp := g.sim.P().U.MaxHP
			sc.scale = 1
			if hp > 0 {
				sc.scale = min(skill.GuardMax, 1+skill.GuardScale*l.Soak/hp)
			}
		}
	case skill.Rage:
		sc.rung = skill.RageRung(l.Rage)
		sc.frac = l.Rage / skill.RageMax
	case skill.FollowUp:
		_, step, ok := l.Following()
		sc.follow, sc.step = ok, step
	}
	return sc
}

// tag is a few characters for the school's state, or "".
func (sc school) tag() string {
	switch {
	case sc.trig == skill.Overload && sc.rung > 0:
		return "charge " + rungName(sc.rung)
	case sc.guard:
		return fmt.Sprintf("guard x%.1f", sc.scale)
	case sc.trig == skill.Rage:
		return "rage " + rungName(sc.rung)
	case sc.follow:
		return fmt.Sprintf("follow x%.2f", 1+skill.ComboBonus*float32(min(sc.step+1, skill.ComboSteps)))
	}
	return ""
}

func rungName(r uint8) string {
	switch r {
	case 3:
		return "III"
	case 2:
		return "II"
	}
	return "I"
}

// pips draws three rung pips centred on x, y, n of them lit, in c.
func (g *Game) pips(screen *ebiten.Image, x, y float64, n uint8, c color.RGBA) {
	r, gap := float64(g.u(3.5)), float64(g.u(11))
	for i := 0; i < 3; i++ {
		px := x + float64(i-1)*gap
		if uint8(i) < n {
			vector.DrawFilledCircle(screen, float32(px), float32(y), float32(r), c, true)
		} else {
			vector.StrokeCircle(screen, float32(px), float32(y), float32(r), g.u(1.2), fade(c, 0.5), true)
		}
	}
}

// drawSchoolRound draws a school's state on round button b in color c.
func (g *Game) drawSchoolRound(screen *ebiten.Image, b button, sc school, c color.RGBA) {
	u := g.u
	switch {
	case sc.trig == skill.Overload && sc.rung > 0:
		g.ringPath(b.x, b.y, b.r+float64(u(9)), fx.RingStyle{Sides: 48}, 0, 0, -math.Pi/2, -math.Pi/2+2*math.Pi*float64(sc.frac), false)
		g.strokePath(screen, c, 1, u(3))
		g.pips(screen, b.x, b.y-b.r-float64(u(16)), sc.rung, c)
	case sc.guard:
		drawHex(screen, b.x, b.y, b.r+float64(u(8)), false, fade(colShield, 0.12), colShield, u(2.5))
		g.outlined(screen, g.small, fmt.Sprintf("x%.1f", sc.scale), b.x, b.y-b.r-float64(u(18)), colShield, 1)
	case sc.trig == skill.Rage:
		g.ringPath(b.x, b.y, b.r+float64(u(9)), fx.RingStyle{Sides: 48}, 0, 0, -math.Pi/2, -math.Pi/2+2*math.Pi*float64(sc.frac), false)
		g.strokePath(screen, colHot, 0.9, u(3))
		g.pips(screen, b.x, b.y-b.r-float64(u(16)), sc.rung, colHot)
	case sc.follow:
		pulse := 0.6 + 0.4*math.Sin(float64(g.frame)*0.3)
		cr, cg, cb := colorF(c)
		g.glow(screen, b.x, b.y, b.r*2.1, cr, cg, cb, float32(0.35*pulse))
		vector.StrokeCircle(screen, float32(b.x), float32(b.y), float32(b.r+float64(u(7))), u(3), fade(c, float32(pulse)), true)
		g.outlined(screen, g.small, "NOW", b.x, b.y-b.r-float64(u(18)), c, 1)
	}
}

// drawKeyBar draws the PC key bar.
func (g *Game) drawKeyBar(screen *ebiten.Image) {
	sim := g.sim
	load := &sim.Load
	rig := &sim.Rig
	u := g.u
	for _, cell := range g.keyBar() {
		r := cell.r
		x, y, w, h := float32(r.x), float32(r.y), float32(r.w), float32(r.h)
		key, name, sub, c := "", "", "", colDim
		var sc school
		switch cell.b {
		case keyVent:
			key, name, c = "R", "VENT", color.RGBA{0xd8, 0xf4, 0xff, 0xff}
			if rig.Heat > rig.MaxHeat*0.75 && g.frame/10%2 == 0 {
				c = colHot
			}
			sub = fmt.Sprintf("heat %.0f%%", 100*rig.Heat/max(1, rig.MaxHeat))
			if rig.Locked() {
				sub = fmt.Sprintf("locked %.1fs", rig.ShutdownLeft)
			}
		case keyHex:
			pulse := float32(0.75 + 0.25*math.Sin(float64(g.frame)*0.12))
			key, name, sub, c = "TAB", "HEX", "workbench", fade(colBench, pulse)
		default:
			key = fmt.Sprint(cell.b)
			first := load.FirstOn(cell.b)
			name = "CAST"
			if first >= 0 {
				c, name = slotCol[first], load.Names[first]
				if pr := load.Progs[first]; pr != nil {
					sub = costTag(pr.Power, pr.Heat)
				}
				if fl := sim.SlotFlash[first]; fl > 0 && sim.SlotLast[first] != skill.Fired {
					c, sub = colHot, sim.SlotLast[first].String()
				}
				sc = g.schoolOf(cell.b)
				if t := sc.tag(); t != "" {
					sub = t
				}
			}
		}
		vector.DrawFilledRect(screen, x, y, w, h, color.RGBA{0x0c, 0x0f, 0x18, 0xe0}, false)
		vector.DrawFilledRect(screen, x, y, w, h, fade(c, 0.1), false)
		if cell.b <= 4 {
			if i := load.FirstOn(cell.b); i >= 0 {
				if fl := sim.SlotFlash[i]; fl > 0 {
					vector.DrawFilledRect(screen, x, y, w, h, fade(c, 0.3*min(1, float32(fl)*3)), false)
				}
			}
		}
		// A meter along the bottom: heat on VENT, the school's charge or
		// rage on a cast key.
		switch {
		case cell.b == keyVent:
			vector.DrawFilledRect(screen, x, y+h-u(3), w*min(1, rig.Heat/max(1, rig.MaxHeat)), u(3), c, false)
		case sc.frac > 0:
			mc := c
			if sc.trig == skill.Rage {
				mc = colHot
			}
			vector.DrawFilledRect(screen, x, y+h-u(3), w*min(1, sc.frac), u(3), mc, false)
		}
		edge := fade(c, 0.6)
		if sc.follow || sc.guard {
			pulse := float32(0.6 + 0.4*math.Sin(float64(g.frame)*0.3))
			edge = fade(c, pulse)
			if sc.guard {
				edge = colShield
			}
			vector.StrokeRect(screen, x-u(3), y-u(3), w+u(6), h+u(6), u(2), edge, false)
		}
		vector.StrokeRect(screen, x, y, w, h, u(1.5), edge, false)
		// The key, in a small box at the left.
		kw := u(26)
		if len(key) > 1 {
			kw = u(34)
		}
		vector.DrawFilledRect(screen, x+u(6), y+u(8), kw, h-u(16), fade(c, 0.22), false)
		vector.StrokeRect(screen, x+u(6), y+u(8), kw, h-u(16), u(1), c, false)
		g.label(screen, g.small, key, float64(x+u(6)+kw/2), float64(y+h/2), colText, true)
		tx := float64(x + u(6) + kw + u(8))
		room := float64(w) - (tx - float64(x)) - float64(u(6))
		g.label(screen, g.small, fitWord(g, []string{name, shortName(name)}, room), tx, float64(y+h*0.33), c, false)
		g.label(screen, g.small, sub, tx, float64(y+h*0.7), colDim, false)
		if sc.rung > 0 {
			pc := c
			if sc.trig == skill.Rage {
				pc = colHot
			}
			g.pips(screen, float64(x+w-u(22)), float64(y+h*0.33), sc.rung, pc)
		}
	}
}

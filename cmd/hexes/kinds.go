package main

// What each kind of skill looks like. Color alone said little, so every
// damage kind leads with its own shape and motion (fx.Arch): plasma is a
// hot core that writhes and throws sparks, cryo a crystal spear that
// shatters into shards and leaves frost, kinetic a hard slug and a heavy
// shockwave that cracks the deck, nanites a swarm of motes, drones a
// laser with lock-on brackets, cloak a shimmer. A hex's Grade (how much
// Power it cost and how hard it landed) adds structure as well as size:
// more strands, extra rings, forks and spikes, and a palette that runs
// hotter within its hue. The same marks label parts in the grid and the
// locker. Everything is drawn from paths: no image assets.

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"hexes/fx"
	"hexes/game"
	"hexes/skill"
)

// How ringPath maps its points.
const (
	ringFlat   uint8 = iota // screen space: UI and the HUD
	ringGround              // lying on the floor round a ground point
	ringLocal               // squashed like the floor, round any point (hits in the air)
)

// ringPt maps an offset dx, dy (pixels at the player's row) round centre
// cx, cy by the current ring mode. wx, wy is the centre on the floor, for
// ground mode; p the perspective scale there, for local mode.
func (g *Game) ringPt(cx, cy, wx, wy, p, dx, dy float64) (float64, float64) {
	switch g.ringMode {
	case ringGround:
		sx, sy, _ := g.project(wx+dx/g.size, wy+dy/g.size)
		return sx, sy
	case ringLocal:
		return cx + dx*p, cy + dy*viewCos*p*p
	}
	return cx + dx, cy + dy
}

// ringFrame works out what ringPt needs for a centre on screen.
func (g *Game) ringFrame(cx, cy float64) (wx, wy, p float64) {
	if g.ringMode == ringFlat {
		return 0, 0, 1
	}
	wx, wy = g.unproject(cx, cy)
	return wx, wy, g.scaleAt(wy)
}

// local runs fn with rings squashed round a point in the air.
func (g *Game) local(fn func()) {
	m := g.ringMode
	if m != ringFlat {
		g.ringMode = ringLocal
	}
	fn()
	g.ringMode = m
}

// spokes builds n lines from radius r0 to r1 round cx, cy, starting at
// angle a0, into the path.
func (g *Game) spokes(cx, cy float64, n int, a0, r0, r1 float64, fresh bool) {
	p := &g.fxs.path
	if fresh {
		*p = vector.Path{}
	}
	wx, wy, sc := g.ringFrame(cx, cy)
	for i := 0; i < n; i++ {
		a := a0 + float64(i)*2*math.Pi/float64(n)
		c, s := math.Cos(a), math.Sin(a)
		x0, y0 := g.ringPt(cx, cy, wx, wy, sc, c*r0, s*r0)
		x1, y1 := g.ringPt(cx, cy, wx, wy, sc, c*r1, s*r1)
		p.MoveTo(float32(x0), float32(y0))
		p.LineTo(float32(x1), float32(y1))
	}
}

// dots builds n small diamonds round a ring of radius r, turned by a0.
func (g *Game) dots(cx, cy float64, n int, a0, r, d float64) {
	p := &g.fxs.path
	*p = vector.Path{}
	wx, wy, sc := g.ringFrame(cx, cy)
	for i := 0; i < n; i++ {
		a := a0 + float64(i)*2*math.Pi/float64(n)
		x, y := g.ringPt(cx, cy, wx, wy, sc, math.Cos(a)*r, math.Sin(a)*r)
		p.MoveTo(float32(x), float32(y-d))
		p.LineTo(float32(x+d), float32(y))
		p.LineTo(float32(x), float32(y+d))
		p.LineTo(float32(x-d), float32(y))
		p.Close()
	}
}

// diamond builds a long diamond from a to b, w wide: a crystal or a slug.
func (g *Game) diamond(ax, ay, bx, by, w, waist float64) {
	p := &g.fxs.path
	*p = vector.Path{}
	dx, dy := bx-ax, by-ay
	l := math.Hypot(dx, dy)
	if l < 1e-6 {
		return
	}
	nx, ny := -dy/l*w/2, dx/l*w/2
	mx, my := ax+dx*waist, ay+dy*waist
	p.MoveTo(float32(ax), float32(ay))
	p.LineTo(float32(mx+nx), float32(my+ny))
	p.LineTo(float32(bx), float32(by))
	p.LineTo(float32(mx-nx), float32(my-ny))
	p.Close()
}

// wavePath builds a sine strand from a to b, amp pixels to each side,
// waves times along it, sliding with phase.
func (g *Game) wavePath(ax, ay, bx, by, amp, waves, phase float64, fresh bool) {
	p := &g.fxs.path
	if fresh {
		*p = vector.Path{}
	}
	dx, dy := bx-ax, by-ay
	l := math.Hypot(dx, dy)
	if l < 1e-6 {
		return
	}
	nx, ny := -dy/l, dx/l
	n := max(6, min(24, int(l/(g.size*0.25))))
	for i := 0; i <= n; i++ {
		k := float64(i) / float64(n)
		o := amp * math.Sin(k*waves*2*math.Pi+phase) * math.Sin(k*math.Pi)
		x, y := ax+dx*k+nx*o, ay+dy*k+ny*o
		if i == 0 {
			p.MoveTo(float32(x), float32(y))
		} else {
			p.LineTo(float32(x), float32(y))
		}
	}
}

// brackets builds four lock-on corners round cx, cy at radius r, turned
// by a0.
func (g *Game) brackets(cx, cy, r, a0 float64, fresh bool) {
	p := &g.fxs.path
	if fresh {
		*p = vector.Path{}
	}
	wx, wy, sc := g.ringFrame(cx, cy)
	arm := 0.42
	for i := 0; i < 4; i++ {
		a := a0 + math.Pi/4 + float64(i)*math.Pi/2
		x0, y0 := g.ringPt(cx, cy, wx, wy, sc, math.Cos(a-arm)*r, math.Sin(a-arm)*r)
		x1, y1 := g.ringPt(cx, cy, wx, wy, sc, math.Cos(a)*r*1.12, math.Sin(a)*r*1.12)
		x2, y2 := g.ringPt(cx, cy, wx, wy, sc, math.Cos(a+arm)*r, math.Sin(a+arm)*r)
		p.MoveTo(float32(x0), float32(y0))
		p.LineTo(float32(x1), float32(y1))
		p.LineTo(float32(x2), float32(y2))
	}
}

// gradeCols is a type's color at a grade: the core runs lighter and the
// rim deeper and richer as the grade climbs, each turned a little round
// the hue, so a big hit is a different shade of the same kind and not
// just a bigger one.
func gradeCols(c color.RGBA, grade uint8) (core, rim color.RGBA) {
	if grade == 0 {
		return c, c
	}
	h, s, v := rgbToHSV(c)
	gf := float64(grade)
	core = hsvToRGB(h+6*gf, s*(1-0.22*gf), math.Min(1, v+0.1*gf))
	rim = hsvToRGB(h-9*gf, math.Min(1, s+0.15*gf+0.1), v*(1-0.06*gf))
	return
}

func rgbToHSV(c color.RGBA) (h, s, v float64) {
	r, g, b := float64(c.R)/255, float64(c.G)/255, float64(c.B)/255
	mx, mn := math.Max(r, math.Max(g, b)), math.Min(r, math.Min(g, b))
	v = mx
	d := mx - mn
	if mx > 0 {
		s = d / mx
	}
	if d == 0 {
		return 0, s, v
	}
	switch mx {
	case r:
		h = math.Mod((g-b)/d, 6)
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	h *= 60
	if h < 0 {
		h += 360
	}
	return
}

func hsvToRGB(h, s, v float64) color.RGBA {
	h = math.Mod(h, 360)
	if h < 0 {
		h += 360
	}
	s = math.Max(0, math.Min(1, s))
	c := v * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := v - c
	var r, g, b float64
	switch {
	case h < 60:
		r, g = c, x
	case h < 120:
		r, g = x, c
	case h < 180:
		g, b = c, x
	case h < 240:
		g, b = x, c
	case h < 300:
		r, b = x, c
	default:
		r, b = c, x
	}
	return color.RGBA{uint8((r + m) * 255), uint8((g + m) * 255), uint8((b + m) * 255), 0xff}
}

var colWhite = color.RGBA{0xff, 0xff, 0xff, 0xff}

// drawBolt draws a hex's hit thrown from the caster: the projectile in
// flight, by kind and grade. a..b is the bit of the line that's lit.
func (g *Game) drawBolt(screen *ebiten.Image, arch fx.Arch, grade uint8, c color.RGBA, ax, ay, bx, by, alpha float32, m, seed float64, sig fx.Sig) {
	s := g.size
	t := float64(g.frame) / 60
	core, rim := gradeCols(c, grade)
	gf := float64(grade)
	w := 0.7 + 0.3*m + 0.15*gf
	fax, fay, fbx, fby := float64(ax), float64(ay), float64(bx), float64(by)
	dx, dy := fbx-fax, fby-fay
	l := math.Hypot(dx, dy) + 1e-9
	ux, uy := dx/l, dy/l
	nx, ny := -uy, ux
	switch arch {
	case fx.ArchHot:
		// A writhing flame line with a hot core at its head; more
		// strands twist round it as the grade climbs.
		for j := 0; j <= int(min(grade, 2)); j++ {
			g.jagPath(fax, fay, fbx, fby, float64(sig.Jag())*(1+0.3*float64(j)), seed+float64(j)*5.3)
			g.strokePath(screen, rim, alpha*0.22, g.u(14*w))
			g.strokePath(screen, core, alpha*0.6, g.u(4.5*w/float64(j+1)))
		}
		cr, cg, cb := colorF(core)
		g.glow(screen, fbx, fby, s*(0.45+0.2*gf)*w, cr, cg, cb, alpha)
		g.glow(screen, fbx, fby, s*(0.18+0.06*gf), 1, 1, 1, alpha)
		if grade >= 2 {
			// Forked tongues flicking back off the head.
			p := &g.fxs.path
			*p = vector.Path{}
			for _, sgn := range []float64{-1, 1} {
				fl := s * (0.7 + 0.25*math.Sin(t*23+seed))
				p.MoveTo(float32(fbx), float32(fby))
				p.LineTo(float32(fbx-ux*fl+nx*fl*0.55*sgn), float32(fby-uy*fl+ny*fl*0.55*sgn))
			}
			g.strokePath(screen, core, alpha*0.8, g.u(2.2))
		}
	case fx.ArchCold:
		// A straight, thin line and a crystal spear at its head, with
		// frost facets ticking off the shaft.
		g.jagPath(fax, fay, fbx, fby, 0, seed)
		g.strokePath(screen, rim, alpha*0.25, g.u(7*w))
		g.strokePath(screen, colWhite, alpha*0.7, g.u(1.2))
		sl := s * (0.75 + 0.22*gf)
		g.diamond(fbx-ux*sl, fby-uy*sl, fbx+ux*s*0.15, fby+uy*s*0.15, s*(0.22+0.05*gf), 0.62)
		g.fillPath(screen, core, alpha*0.9)
		g.strokePath(screen, colWhite, alpha, g.u(1))
		if grade >= 1 {
			p := &g.fxs.path
			*p = vector.Path{}
			n := 2 + 2*int(grade)
			for i := 1; i <= n; i++ {
				k := float64(i) / float64(n+1)
				px, py := fax+dx*k, fay+dy*k
				tk := s * 0.22 * (1 - k*0.5)
				p.MoveTo(float32(px-ux*tk+nx*tk), float32(py-uy*tk+ny*tk))
				p.LineTo(float32(px), float32(py))
				p.LineTo(float32(px-ux*tk-nx*tk), float32(py-uy*tk-ny*tk))
			}
			g.strokePath(screen, core, alpha*0.8, g.u(1.3))
		}
	case fx.ArchForce:
		// A hard slug with speed lines behind it; more lines per grade.
		sl := s * (0.55 + 0.15*gf)
		g.diamond(fbx-ux*sl, fby-uy*sl, fbx+ux*s*0.1, fby+uy*s*0.1, s*(0.3+0.06*gf), 0.75)
		g.fillPath(screen, core, alpha)
		p := &g.fxs.path
		*p = vector.Path{}
		n := 2 + int(grade)
		for i := 0; i < n; i++ {
			o := (float64(i) - float64(n-1)/2) * s * 0.16
			ll := s * (1.0 + 0.3*float64((i+int(seed))%2))
			p.MoveTo(float32(fbx-ux*sl+nx*o), float32(fby-uy*sl+ny*o))
			p.LineTo(float32(fbx-ux*(sl+ll)+nx*o), float32(fby-uy*(sl+ll)+ny*o))
		}
		g.strokePath(screen, rim, alpha*0.7, g.u(1.6))
		g.jagPath(fax, fay, fbx, fby, 0, seed)
		g.strokePath(screen, rim, alpha*0.12, g.u(10*w))
	case fx.ArchSwarm:
		// Motes streaming in sinuous strands: one more per grade.
		n := 1 + int(grade)
		for j := 0; j < n; j++ {
			ph := t*10 + float64(j)*2*math.Pi/float64(n) + seed
			g.wavePath(fax, fay, fbx, fby, s*(0.22+0.05*gf), 2.5+0.5*float64(j), ph, true)
			g.strokePath(screen, rim, alpha*0.35, g.u(3*w))
			g.strokePath(screen, core, alpha*0.6, g.u(1))
		}
		g.local(func() { g.dots(fbx, fby, 5+2*int(grade), t*5, s*0.3, s*0.07) })
		g.fillPath(screen, core, alpha)
	case fx.ArchMark:
		// A dead straight laser, thin and steady.
		g.jagPath(fax, fay, fbx, fby, 0, seed)
		g.strokePath(screen, rim, alpha*0.18, g.u(6*w))
		g.strokePath(screen, core, alpha*0.9, g.u(1.4+0.4*gf))
		if grade >= 1 {
			// A second beam rides beside it.
			o := s * 0.12
			p := &g.fxs.path
			*p = vector.Path{}
			p.MoveTo(float32(fax+nx*o), float32(fay+ny*o))
			p.LineTo(float32(fbx+nx*o), float32(fby+ny*o))
			p.MoveTo(float32(fax-nx*o), float32(fay-ny*o))
			p.LineTo(float32(fbx-nx*o), float32(fby-ny*o))
			g.strokePath(screen, core, alpha*0.45, g.u(1))
		}
	case fx.ArchVeil:
		// A faint wide wash with ripples running along it.
		g.jagPath(fax, fay, fbx, fby, 0.02, seed)
		g.strokePath(screen, rim, alpha*0.1, g.u(18*w))
		for j := 0; j < 1+int(grade); j++ {
			g.wavePath(fax, fay, fbx, fby, s*0.12, 1.5, t*4+float64(j)*2, true)
			g.strokePath(screen, core, alpha*0.3, g.u(1.2))
		}
	default:
		g.jagPath(fax, fay, fbx, fby, float64(sig.Jag()), seed)
		g.strokePath(screen, c, alpha*0.18, g.u(16*w))
		g.strokePath(screen, c, alpha*0.5, g.u(6*w))
		g.strokePath(screen, colWhite, alpha*0.8, g.u(1.6*w))
	}
}

// drawHit draws a hit landing at x, y (in the air), by kind and grade.
// kk runs 0 to 1 over the hit's short life.
func (g *Game) drawHit(screen *ebiten.Image, arch fx.Arch, grade uint8, c color.RGBA, x, y, kk, m, seed float64, sig fx.Sig) {
	s := g.size
	t := float64(g.frame) / 60
	a := float32(1 - kk)
	core, rim := gradeCols(c, grade)
	gf := float64(grade)
	cr, cg, cb := colorF(rim)
	g.glow(screen, x, y, s*(0.9+0.6*m)*(1-0.4*kk), cr, cg, cb, a*0.8)
	g.glow(screen, x, y, s*(0.35+0.2*m)*(1-kk), 1, 1, 1, a)
	r := s * (0.3 + (0.9+0.4*m+0.15*gf)*easeOut(kk))
	g.local(func() {
		switch arch {
		case fx.ArchHot:
			g.ringPath(x, y, r, sig.Ring(), t, seed, 0, 2*math.Pi, true)
			g.strokePath(screen, rim, a*0.9, g.u(3*(1-kk)*m+1))
			if grade >= 1 {
				// Tongues of flame licking out.
				n := 5 + 2*int(grade)
				p := &g.fxs.path
				*p = vector.Path{}
				wx, wy, sc := g.ringFrame(x, y)
				for i := 0; i < n; i++ {
					an := seed + float64(i)*2*math.Pi/float64(n)
					fl := r * (1.25 + 0.3*math.Sin(t*19+float64(i)*2.1))
					x0, y0 := g.ringPt(x, y, wx, wy, sc, math.Cos(an)*r*0.8, math.Sin(an)*r*0.8)
					x1, y1 := g.ringPt(x, y, wx, wy, sc, math.Cos(an+0.15)*fl, math.Sin(an+0.15)*fl)
					p.MoveTo(float32(x0), float32(y0))
					p.LineTo(float32(x1), float32(y1))
				}
				g.strokePath(screen, core, a*0.85, g.u(2))
			}
		case fx.ArchCold:
			// A hex ring and shards snapping out of its corners.
			g.ringPath(x, y, r*0.8, fx.RingStyle{Sides: 6}, 0, seed, 0, 2*math.Pi, true)
			g.strokePath(screen, rim, a*0.9, g.u(2+gf))
			g.spokes(x, y, 6, -math.Pi/2, r*0.8, r*(1.35+0.25*gf), true)
			if grade >= 2 {
				g.spokes(x, y, 6, -math.Pi/2+math.Pi/6, r*0.3, r*(0.95+0.15*gf), false)
			}
			g.strokePath(screen, core, a, g.u(1.8))
		case fx.ArchForce:
			// A thick shockwave with echoes behind it.
			g.ringPath(x, y, r, fx.RingStyle{Sides: 40}, t, seed, 0, 2*math.Pi, true)
			g.strokePath(screen, rim, a, g.u((4+2*gf)*float64(a)+1.5))
			for j := 1; j <= 1+int(grade); j++ {
				rj := r * (1 - 0.17*float64(j))
				g.ringPath(x, y, rj, fx.RingStyle{Sides: 40}, t, seed, 0, 2*math.Pi, true)
				g.strokePath(screen, core, a*(0.6-0.12*float32(j)), g.u(1.5))
			}
		case fx.ArchSwarm:
			// The swarm wheels round where it hit and spreads.
			g.ringPath(x, y, r*0.9, fx.RingStyle{Sides: 40, Dash: 0.7, Spin: 0.8}, t, seed, 0, 2*math.Pi, true)
			g.strokePath(screen, rim, a*0.7, g.u(1.6))
			g.dots(x, y, 4+3*int(grade), t*4+seed, r, s*(0.06+0.015*gf))
			g.fillPath(screen, core, a)
		case fx.ArchMark:
			// Lock-on brackets closing in and turning.
			br := s * (1.2 - 0.6*math.Min(1, kk/0.4)) * (1 + 0.1*gf)
			g.brackets(x, y, br, t*1.5*gf, true)
			if grade >= 2 {
				g.brackets(x, y, br*0.6, -t*2+math.Pi/4, false)
			}
			g.strokePath(screen, core, a, g.u(2))
		case fx.ArchVeil:
			for j := 0; j <= int(grade); j++ {
				kj := math.Max(0, kk-0.12*float64(j))
				g.ringPath(x, y, s*(0.2+1.2*easeOut(kj)), fx.RingStyle{Sides: 40, Wobble: 0.05}, t, seed+float64(j), 0, 2*math.Pi, true)
				g.strokePath(screen, core, a*0.4, g.u(1.2))
			}
		default:
			g.ringPath(x, y, r, sig.Ring(), t, seed, 0, 2*math.Pi, true)
			g.strokePath(screen, c, a*0.9, g.u(3*(1-kk)*m+1))
		}
	})
}

// drawAreaExtra adds a kind's structure to an area's ring of radius r
// round x, y (on the floor): spikes, echoes, tongues, a wheeling swarm.
// arc0, arc1 limit it to a cone's arc.
func (g *Game) drawAreaExtra(screen *ebiten.Image, arch fx.Arch, grade uint8, c color.RGBA, x, y, r, k float64, seed, arc0, arc1 float64) {
	s := g.size
	t := float64(g.frame) / 60
	a := float32(1 - k)
	core, rim := gradeCols(c, grade)
	gf := float64(grade)
	full := arc1-arc0 >= 2*math.Pi-1e-6
	wx, wy, sc := g.ringFrame(x, y)
	p := &g.fxs.path
	switch arch {
	case fx.ArchHot:
		n := int((10 + 4*gf) * (arc1 - arc0) / (2 * math.Pi))
		*p = vector.Path{}
		for i := 0; i <= n; i++ {
			an := arc0 + (arc1-arc0)*float64(i)/float64(max(n, 1))
			fl := r * (1.08 + (0.1+0.04*gf)*(1+math.Sin(t*17+float64(i)*1.7+seed)))
			x0, y0 := g.ringPt(x, y, wx, wy, sc, math.Cos(an)*r, math.Sin(an)*r)
			x1, y1 := g.ringPt(x, y, wx, wy, sc, math.Cos(an)*fl, math.Sin(an)*fl)
			p.MoveTo(float32(x0), float32(y0))
			p.LineTo(float32(x1), float32(y1))
		}
		g.strokePath(screen, core, a*0.8, g.u(2+0.5*gf))
	case fx.ArchCold:
		if full {
			g.spokes(x, y, 6, -math.Pi/2, r*0.92, r*(1.15+0.08*gf), true)
			if grade >= 1 {
				g.spokes(x, y, 6, -math.Pi/2+math.Pi/6, r*0.55, r*0.8, false)
			}
		} else {
			*p = vector.Path{}
			n := 3 + int(grade)
			for i := 0; i <= n; i++ {
				an := arc0 + (arc1-arc0)*float64(i)/float64(n)
				x0, y0 := g.ringPt(x, y, wx, wy, sc, math.Cos(an)*r*0.9, math.Sin(an)*r*0.9)
				x1, y1 := g.ringPt(x, y, wx, wy, sc, math.Cos(an)*r*1.15, math.Sin(an)*r*1.15)
				p.MoveTo(float32(x0), float32(y0))
				p.LineTo(float32(x1), float32(y1))
			}
		}
		g.strokePath(screen, core, a, g.u(1.6))
		if full && grade >= 2 {
			g.ringPath(x, y, r*0.6, fx.RingStyle{Sides: 6, Spin: 0.2}, t, seed, 0, 2*math.Pi, true)
			g.strokePath(screen, rim, a*0.6, g.u(1.2))
		}
	case fx.ArchForce:
		for j := 1; j <= 1+int(grade); j++ {
			rj := r * (1 - 0.13*float64(j))
			if rj <= 0 {
				break
			}
			g.ringPath(x, y, rj, fx.RingStyle{Sides: 40}, t, seed, arc0, arc1, full)
			g.strokePath(screen, rim, a*(0.55-0.1*float32(j)), g.u(2.5/float64(j)+0.8))
		}
	case fx.ArchSwarm:
		n := 8 + 4*int(grade)
		if !full {
			n = n/2 + 1
		}
		*p = vector.Path{}
		for i := 0; i < n; i++ {
			an := arc0 + (arc1-arc0)*(float64(i)+0.5)/float64(n) + t*1.6*boolF(full)
			rr := r * (0.85 + 0.12*math.Sin(t*6+float64(i)*2.3))
			px, py := g.ringPt(x, y, wx, wy, sc, math.Cos(an)*rr, math.Sin(an)*rr)
			d := s * (0.06 + 0.015*gf)
			p.MoveTo(float32(px), float32(py-d))
			p.LineTo(float32(px+d), float32(py))
			p.LineTo(float32(px), float32(py+d))
			p.LineTo(float32(px-d), float32(py))
			p.Close()
		}
		g.fillPath(screen, core, a)
	case fx.ArchMark:
		*p = vector.Path{}
		n := 4 + 2*int(grade)
		if !full {
			n = 3
		}
		for i := 0; i < n; i++ {
			an := arc0 + (arc1-arc0)*(float64(i)+boolF(!full)*0.0)/float64(max(n-boolI(!full), 1))
			if full {
				an += t * 0.8
			}
			x0, y0 := g.ringPt(x, y, wx, wy, sc, math.Cos(an)*r*0.82, math.Sin(an)*r*0.82)
			x1, y1 := g.ringPt(x, y, wx, wy, sc, math.Cos(an)*r*1.08, math.Sin(an)*r*1.08)
			p.MoveTo(float32(x0), float32(y0))
			p.LineTo(float32(x1), float32(y1))
		}
		g.strokePath(screen, core, a, g.u(2.2))
	case fx.ArchVeil:
		for j := 1; j <= 1+int(grade); j++ {
			g.ringPath(x, y, r*(1-0.2*float64(j)), fx.RingStyle{Sides: 40, Wobble: 0.04}, t*0.6, seed+float64(j), arc0, arc1, full)
			g.strokePath(screen, core, a*0.3, g.u(1.1))
		}
	}
}

func boolF(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

func boolI(b bool) int {
	if b {
		return 1
	}
	return 0
}

// archOf is the look a damage type leads with.
func (g *Game) archOf(t skill.TypeID) fx.Arch { return g.sigOf(t).Arch() }

// Decals are marks a hit leaves on the deck for a moment: frost from
// cryo, cracks from a heavy kinetic hit, scorch from big plasma.
type decal struct {
	x, y, r   float64 // hex units
	age, life float32
	arch      fx.Arch
	grade     uint8
	col       color.RGBA
	seed      float64
}

const maxDecals = 40

type decalState struct {
	d    [maxDecals]decal
	head int
	n    int
}

func (s *decalState) add(d decal) {
	if s.n == maxDecals {
		s.head = (s.head + 1) % maxDecals
		s.n--
	}
	s.d[(s.head+s.n)%maxDecals] = d
	s.n++
}

func (s *decalState) update(dt float32) {
	for i := 0; i < s.n; i++ {
		s.d[(s.head+i)%maxDecals].age += dt
	}
	for s.n > 0 && s.d[s.head].age >= s.d[s.head].life {
		s.head = (s.head + 1) % maxDecals
		s.n--
	}
}

func (s *decalState) clear() { s.head, s.n = 0, 0 }

// leaveMark puts a kind's mark on the deck where an effect landed.
func (g *Game) leaveMark(e *game.FX, x, y, r float64) {
	arch := g.archOf(e.Type)
	d := decal{x: x, y: y, r: r, arch: arch, grade: e.Grade, col: typeColor(e.Type), seed: x*7.3 + y*3.1}
	switch arch {
	case fx.ArchCold:
		d.life = 2 + 0.4*float32(e.Grade)
	case fx.ArchForce:
		if e.Mag < 0.8 && e.Grade == 0 && e.Kind == game.FXBolt {
			return
		}
		d.life = 1.6
	case fx.ArchHot:
		if e.Grade == 0 && e.Kind == game.FXBolt {
			return
		}
		d.life = 1.4
	default:
		return
	}
	g.decals.add(d)
}

// drawDecals draws the marks on the floor, under everything standing.
func (g *Game) drawDecals(screen *ebiten.Image) {
	ds := &g.decals
	s := g.size
	p := &g.fxs.path
	for i := 0; i < ds.n; i++ {
		d := &ds.d[(ds.head+i)%maxDecals]
		k := float64(d.age / d.life)
		a := float32(1 - k*k)
		if g.seeAt(d.x, d.y) < 0.15 {
			continue
		}
		cx, cy := g.toScreen(d.x, d.y)
		if cx < -s*4 || cy < -s*4 || cx > float64(g.w)+s*4 || cy > float64(g.h)+s*4 {
			continue
		}
		R := d.r * s
		core, _ := gradeCols(d.col, d.grade)
		switch d.arch {
		case fx.ArchCold:
			// Frost: a pale crystal of six arms with side ticks, fading.
			g.ringPath(cx, cy, R*0.75, fx.RingStyle{Sides: 6}, 0, d.seed, 0, 2*math.Pi, true)
			g.fillPath(screen, core, a*0.08)
			g.spokes(cx, cy, 6, -math.Pi/2+d.seed, 0, R, true)
			wx, wy, sc := g.ringFrame(cx, cy)
			for j := 0; j < 6; j++ {
				an := -math.Pi/2 + d.seed + float64(j)*math.Pi/3
				for _, f := range []float64{0.45, 0.7} {
					mx, my := math.Cos(an)*R*f, math.Sin(an)*R*f
					for _, sg := range []float64{-1, 1} {
						b := an + sg*0.7
						x0, y0 := g.ringPt(cx, cy, wx, wy, sc, mx, my)
						x1, y1 := g.ringPt(cx, cy, wx, wy, sc, mx+math.Cos(b)*R*0.18, my+math.Sin(b)*R*0.18)
						p.MoveTo(float32(x0), float32(y0))
						p.LineTo(float32(x1), float32(y1))
					}
				}
			}
			g.strokePath(screen, core, a*0.45, g.u(1.2))
		case fx.ArchForce:
			// Cracks radiating from the point of impact.
			wx, wy, sc := g.ringFrame(cx, cy)
			*p = vector.Path{}
			n := 4 + int(d.grade)
			for j := 0; j < n; j++ {
				an := d.seed + float64(j)*2*math.Pi/float64(n) + 0.4*noise(d.seed, float64(j))
				px, py := 0.0, 0.0
				x0, y0 := g.ringPt(cx, cy, wx, wy, sc, 0, 0)
				p.MoveTo(float32(x0), float32(y0))
				for q := 1; q <= 3; q++ {
					step := R * 0.38
					an += 0.5 * noise(d.seed+float64(j), float64(q))
					px += math.Cos(an) * step
					py += math.Sin(an) * step
					x1, y1 := g.ringPt(cx, cy, wx, wy, sc, px, py)
					p.LineTo(float32(x1), float32(y1))
				}
			}
			g.strokeDark(screen, color.RGBA{0x05, 0x06, 0x0a, 0xff}, a*0.75, g.u(2.4))
			g.strokePath(screen, core, a*0.18, g.u(1))
		case fx.ArchHot:
			// A scorch with a dull glowing rim.
			g.ringPath(cx, cy, R*0.7, fx.RingStyle{Sides: 40, Wobble: 0.12}, 0, d.seed, 0, 2*math.Pi, true)
			g.fillDark(screen, color.RGBA{0x08, 0x05, 0x04, 0xff}, a*0.55)
			g.ringPath(cx, cy, R*0.7, fx.RingStyle{Sides: 40, Wobble: 0.12}, 0, d.seed, 0, 2*math.Pi, true)
			g.strokePath(screen, core, a*0.3*(0.7+0.3*float32(math.Sin(float64(g.frame)*0.3+d.seed))), g.u(1.6))
		}
	}
}

// strokeDark and fillDark draw the current path with normal blending,
// for marks darker than the floor.
func (g *Game) strokeDark(dst *ebiten.Image, c color.RGBA, a, width float32) {
	f := &g.fxs
	vs, is := f.path.AppendVerticesAndIndicesForStroke(f.vs[:0], f.is[:0], &vector.StrokeOptions{Width: width, LineJoin: vector.LineJoinRound})
	g.tintBlend(dst, vs, is, c, a, ebiten.BlendSourceOver)
}

func (g *Game) fillDark(dst *ebiten.Image, c color.RGBA, a float32) {
	f := &g.fxs
	vs, is := f.path.AppendVerticesAndIndicesForFilling(f.vs[:0], f.is[:0])
	g.tintBlend(dst, vs, is, c, a, ebiten.BlendSourceOver)
}

func (g *Game) tintBlend(dst *ebiten.Image, vs []ebiten.Vertex, is []uint16, c color.RGBA, a float32, b ebiten.Blend) {
	r, gg, bb := colorF(c)
	for i := range vs {
		vs[i].SrcX, vs[i].SrcY = 1, 1
		vs[i].ColorR, vs[i].ColorG, vs[i].ColorB, vs[i].ColorA = r*a, gg*a, bb*a, a
	}
	if len(is) > 0 {
		dst.DrawTriangles(vs, is, whiteSub(), &ebiten.DrawTrianglesOptions{AntiAlias: true, Blend: b})
	}
	g.fxs.vs, g.fxs.is = vs[:0], is[:0]
}

// colShot is a foe's round: hot and pink, nothing like any of yours.
var colShot = color.RGBA{0xff, 0x4f, 0x7a, 0xff}

// drawShots draws gunners' rounds in flight, at chest height.
func (g *Game) drawShots(screen *ebiten.Image) {
	sim := g.sim
	s := g.size
	cr, cg, cb := colorF(colShot)
	for i := 0; i < sim.NShots; i++ {
		sh := &sim.Shots[i]
		hx, hy, p := g.lifted(sh.X, sh.Y, chestLift)
		tx, ty, _ := g.lifted(sh.X-sh.VX*0.07, sh.Y-sh.VY*0.07, chestLift)
		g.glow(screen, hx, hy, s*0.55*p, cr, cg, cb, 0.7)
		vector.StrokeLine(screen, float32(tx), float32(ty), float32(hx), float32(hy), g.u(4*p), fade(colShot, 0.8), true)
		vector.StrokeLine(screen, float32(tx+(hx-tx)*0.5), float32(ty+(hy-ty)*0.5), float32(hx), float32(hy), g.u(1.6*p), colWhite, true)
	}
}

// drawTell draws what a foe is about to do: the mark when it spots you
// and calls its pack, and a gunner's aim line as it lines up a shot.
func (g *Game) drawTell(screen *ebiten.Image, a *game.Actor, l *actorLook) {
	s := g.size
	if a.Alert > 0 {
		k := float64(a.Alert / game.AlertShow)
		pop := easeOut(math.Min(1, (1-k)/0.25))
		x := l.tx
		y := l.ty - l.R*0.6 - s*0.55*pop*l.p
		h := s * 0.32 * l.p * pop
		al := float32(math.Min(1, k*2.5)) * l.alpha
		c := color.RGBA{0xff, 0xd0, 0x40, 0xff}
		vector.StrokeLine(screen, float32(x), float32(y-h), float32(x), float32(y+h*0.25), g.u(3.2*l.p), fade(c, al), true)
		vector.DrawFilledCircle(screen, float32(x), float32(y+h*0.7), g.u(2*l.p), fade(c, al), true)
	}
	if a.Kind == game.KindGunner && a.Aim > 0 {
		k := 1 - float64(a.Aim/game.GunAim) // 0 to 1 as it lines up
		ax, ay := a.Pos()
		px, py := g.sim.P().Pos()
		sx, sy, _ := g.lifted(ax, ay, chestLift)
		tx, ty, tp := g.lifted(px, py, chestLift)
		al := float32(0.25+0.6*k) * l.alpha
		// A dashed sight line that firms up, and a reticle closing on you.
		dx, dy := tx-sx, ty-sy
		n := 14
		for j := 0; j < n; j += 2 {
			k0, k1 := float64(j)/float64(n), float64(j+1)/float64(n)
			vector.StrokeLine(screen, float32(sx+dx*k0), float32(sy+dy*k0), float32(sx+dx*k1), float32(sy+dy*k1), g.u(1+1.5*k), fade(colShot, al), true)
		}
		r := s * (1.1 - 0.6*k) * tp
		g.local(func() { g.brackets(tx, ty, r, k*math.Pi/2, true) })
		g.strokePath(screen, colShot, al, g.u(2))
	}
}

// partGlyph marks a part with its kind's sign, faintly, behind its label:
// the same shapes its effect makes in a fight. Effects show their damage
// kind; targets show their delivery.
func (g *Game) partGlyph(dst *ebiten.Image, x, y, r float64, p skill.Part) {
	m := g.ringMode
	g.ringMode = ringFlat
	defer func() { g.ringMode = m }()
	if g.classGlyph(dst, x, y, r, p) {
		return
	}
	switch {
	case p.Kind == skill.KindEffect && p.Effect.Typed():
		g.typeGlyph(dst, x, y, r, p.Type, 0.6)
	case p.Kind == skill.KindTarget:
		g.targetGlyph(dst, x, y, r, p.Target, color.RGBA{0x7d, 0xf0, 0xe0, 0xff}, 0.5)
	}
}

// typeGlyph draws a damage kind's sign at x, y, about r across.
func (g *Game) typeGlyph(dst *ebiten.Image, x, y, r float64, t skill.TypeID, a float32) {
	c := typeColor(t)
	path := &g.fxs.path
	*path = vector.Path{}
	switch g.archOf(t) {
	case fx.ArchHot:
		// A flame: a teardrop with an inner tongue.
		path.MoveTo(float32(x), float32(y-r*0.85))
		path.QuadTo(float32(x+r*0.7), float32(y), float32(x), float32(y+r*0.6))
		path.QuadTo(float32(x-r*0.7), float32(y), float32(x), float32(y-r*0.85))
		path.MoveTo(float32(x), float32(y-r*0.25))
		path.QuadTo(float32(x+r*0.3), float32(y+r*0.2), float32(x), float32(y+r*0.45))
		path.QuadTo(float32(x-r*0.3), float32(y+r*0.2), float32(x), float32(y-r*0.25))
		g.strokeUI(dst, c, a, g.u(2))
	case fx.ArchCold:
		// A six-armed crystal with ticks.
		for j := 0; j < 6; j++ {
			an := float64(j)*math.Pi/3 - math.Pi/2
			ca, sa := math.Cos(an), math.Sin(an)
			path.MoveTo(float32(x), float32(y))
			path.LineTo(float32(x+ca*r*0.8), float32(y+sa*r*0.8))
			mx, my := x+ca*r*0.5, y+sa*r*0.5
			for _, sg := range []float64{-1, 1} {
				b := an + sg*0.75
				path.MoveTo(float32(mx), float32(my))
				path.LineTo(float32(mx+math.Cos(b)*r*0.22), float32(my+math.Sin(b)*r*0.22))
			}
		}
		g.strokeUI(dst, c, a, g.u(1.8))
	case fx.ArchForce:
		// A slug and the shockwave arcs ahead of it.
		path.MoveTo(float32(x-r*0.75), float32(y))
		path.LineTo(float32(x-r*0.25), float32(y-r*0.16))
		path.LineTo(float32(x-r*0.1), float32(y))
		path.LineTo(float32(x-r*0.25), float32(y+r*0.16))
		path.Close()
		g.fillUI(dst, c, a)
		for j := 0; j < 3; j++ {
			rr := r * (0.35 + 0.22*float64(j))
			path.MoveTo(float32(x-r*0.3+math.Cos(-0.8)*rr), float32(y+math.Sin(-0.8)*rr))
			path.Arc(float32(x-r*0.3), float32(y), float32(rr), -0.8, 0.8, vector.Clockwise)
		}
		g.strokeUI(dst, c, a, g.u(2))
	case fx.ArchSwarm:
		// A cloud of motes.
		for j := 0; j < 9; j++ {
			an := float64(j) * 2.39996
			rr := r * 0.75 * math.Sqrt(float64(j)/9+0.04)
			px, py := x+math.Cos(an)*rr, y+math.Sin(an)*rr
			d := r * 0.09
			path.MoveTo(float32(px), float32(py-d))
			path.LineTo(float32(px+d), float32(py))
			path.LineTo(float32(px), float32(py+d))
			path.LineTo(float32(px-d), float32(py))
			path.Close()
		}
		g.fillUI(dst, c, a*1.4)
	case fx.ArchMark:
		// Lock-on brackets and a dot.
		g.brackets(x, y, r*0.7, 0, false)
		g.strokeUI(dst, c, a, g.u(2))
		vector.DrawFilledCircle(dst, float32(x), float32(y), float32(r*0.12), fade(c, a), true)
	case fx.ArchVeil:
		// Ripples.
		for j := 0; j < 3; j++ {
			oy := (float64(j) - 1) * r * 0.35
			path.MoveTo(float32(x-r*0.7), float32(y+oy))
			path.QuadTo(float32(x-r*0.35), float32(y+oy-r*0.2), float32(x), float32(y+oy))
			path.QuadTo(float32(x+r*0.35), float32(y+oy+r*0.2), float32(x+r*0.7), float32(y+oy))
		}
		g.strokeUI(dst, c, a, g.u(1.8))
	default:
		vector.DrawFilledCircle(dst, float32(x), float32(y), float32(r*0.4), fade(c, a), true)
	}
}

// targetGlyph draws a delivery's sign: a bolt's arrow, a chain's hops, a
// ring round you, a cone's wedge, a dot for self.
func (g *Game) targetGlyph(dst *ebiten.Image, x, y, r float64, k skill.TargetKind, c color.RGBA, a float32) {
	path := &g.fxs.path
	*path = vector.Path{}
	switch k {
	case skill.TargetNearest:
		path.MoveTo(float32(x-r*0.7), float32(y))
		path.LineTo(float32(x+r*0.6), float32(y))
		path.MoveTo(float32(x+r*0.3), float32(y-r*0.28))
		path.LineTo(float32(x+r*0.65), float32(y))
		path.LineTo(float32(x+r*0.3), float32(y+r*0.28))
	case skill.TargetChain:
		pts := [...][2]float64{{-0.7, 0.3}, {-0.25, -0.35}, {0.2, 0.3}, {0.65, -0.3}}
		for i, q := range pts {
			if i == 0 {
				path.MoveTo(float32(x+q[0]*r), float32(y+q[1]*r))
			} else {
				path.LineTo(float32(x+q[0]*r), float32(y+q[1]*r))
			}
		}
		for _, q := range pts {
			vector.DrawFilledCircle(dst, float32(x+q[0]*r), float32(y+q[1]*r), float32(r*0.1), fade(c, a), true)
		}
	case skill.TargetAdjacent:
		vector.DrawFilledCircle(dst, float32(x), float32(y), float32(r*0.14), fade(c, a), true)
		for j := 0; j < 6; j++ {
			an := float64(j)*math.Pi/3 + math.Pi/6
			px, py := x+math.Cos(an)*r*0.62, y+math.Sin(an)*r*0.62
			path.MoveTo(float32(px+r*0.12), float32(py))
			path.Arc(float32(px), float32(py), float32(r*0.12), 0, 2*math.Pi, vector.Clockwise)
		}
	case skill.TargetCone:
		path.MoveTo(float32(x-r*0.6), float32(y))
		path.LineTo(float32(x+r*0.55), float32(y-r*0.55))
		path.MoveTo(float32(x-r*0.6), float32(y))
		path.LineTo(float32(x+r*0.55), float32(y+r*0.55))
		path.MoveTo(float32(x+r*0.55), float32(y-r*0.55))
		path.Arc(float32(x-r*0.6), float32(y), float32(r*1.15*0.7), -0.76, 0.76, vector.Clockwise)
	default:
		path.MoveTo(float32(x+r*0.35), float32(y))
		path.Arc(float32(x), float32(y), float32(r*0.35), 0, 2*math.Pi, vector.Clockwise)
	}
	g.strokeUI(dst, c, a, g.u(2))
}

// strokeUI and fillUI draw the current path with normal blending, for
// panels and icons.
func (g *Game) strokeUI(dst *ebiten.Image, c color.RGBA, a, width float32) {
	g.strokeDark(dst, c, a, width)
}

func (g *Game) fillUI(dst *ebiten.Image, c color.RGBA, a float32) {
	g.fillDark(dst, c, a)
}

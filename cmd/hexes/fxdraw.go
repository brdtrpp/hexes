package main

// Procedural effects. The sim records what happened (game.FX); this file
// turns each record into motion worked out from its numbers: the damage
// type's signature (fx.SigOf) sets how particles and edges move, the hit's
// magnitude sets size and count, the area the hex touched sets the form,
// and the Power and Heat it cost show at the cast. Particles draw in one
// batched, additive DrawTriangles call from a tiny sprite atlas.

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"hexes/fx"
	"hexes/game"
	"hexes/skill"
)

const sqrt3 = 1.7320508

// fxState is the renderer's side of the effects.
type fxState struct {
	parts *fx.Pool
	atlas *ebiten.Image
	sigs  [skill.MaxTypes]fx.Sig
	vs    []ebiten.Vertex
	is    []uint16
	path  vector.Path
}

// spriteCell is the size of one atlas cell; sprites sit inside a 1px
// transparent border so linear filtering doesn't bleed.
const spriteCell = 32

func (g *Game) initFX() {
	f := &g.fxs
	f.parts = fx.NewPool(uint32(g.frame*2654435761) | 1)
	f.atlas = buildAtlas()
	for i := 0; i < g.sim.Reg.Len() && i < skill.MaxTypes; i++ {
		c := typeColor(skill.TypeID(i))
		f.sigs[i] = fx.SigOf(*g.sim.Reg.Get(skill.TypeID(i)), float32(c.R)/255, float32(c.G)/255, float32(c.B)/255)
	}
	f.vs = make([]ebiten.Vertex, 0, fx.Max*4)
	f.is = make([]uint16, 0, fx.Max*6)
}

// buildAtlas draws the four particle sprites procedurally into one image.
func buildAtlas() *ebiten.Image {
	const n = spriteCell
	pix := make([]byte, int(fx.NumSprites)*n*n*4)
	w := int(fx.NumSprites) * n
	for s := 0; s < int(fx.NumSprites); s++ {
		for y := 0; y < n; y++ {
			for x := 0; x < n; x++ {
				// -1 to 1 across the cell, inside the border.
				u := (float64(x) + 0.5 - n/2) / (n/2 - 1.5)
				v := (float64(y) + 0.5 - n/2) / (n/2 - 1.5)
				var a float64
				switch fx.Sprite(s) {
				case fx.SpriteGlow, fx.SpriteStreak:
					d := math.Hypot(u, v)
					a = math.Max(0, 1-d)
					a = a * a * (1.2 - 0.2*a)
				case fx.SpriteShard:
					d := math.Abs(u)/0.98 + math.Abs(v)/0.32
					a = math.Pow(math.Max(0, 1-d), 0.6)
				case fx.SpriteSquare:
					d := math.Max(math.Abs(u), math.Abs(v))
					a = math.Min(1, math.Max(0, (0.85-d)*8))
				}
				a = math.Min(1, a)
				i := (y*w + s*n + x) * 4
				b := byte(255 * a)
				pix[i], pix[i+1], pix[i+2], pix[i+3] = b, b, b, b
			}
		}
	}
	img := ebiten.NewImage(w, n)
	img.WritePixels(pix)
	return img
}

func (g *Game) sigOf(t skill.TypeID) fx.Sig {
	if int(t) < len(g.fxs.sigs) && g.fxs.sigs[t] != (fx.Sig{}) {
		return g.fxs.sigs[t]
	}
	return fx.Neutral
}

func colorF(c color.RGBA) (float32, float32, float32) {
	return float32(c.R) / 255, float32(c.G) / 255, float32(c.B) / 255
}

// updateFX spawns particles for new effects and statuses and moves them
// all on a fixed step. It runs after the sim steps.
func (g *Game) updateFX(dt float32) {
	f := &g.fxs
	sim := g.sim
	sim.FX.Each(func(e *game.FX) {
		if e.Seen {
			return
		}
		e.Seen = true
		g.spawn(e)
	})
	// Lingering statuses, only for actors on screen.
	for i := range sim.Actors {
		a := &sim.Actors[i]
		if !a.U.Alive {
			continue
		}
		x, y := a.Pos()
		if sx, sy := g.toScreen(x, y); sx < -g.size*2 || sy < -g.size*2 || sx > float64(g.w)+g.size*2 || sy > float64(g.h)+g.size*2 {
			continue
		}
		if a.Kind != game.KindPlayer && g.seeAt(x, y) < 0.2 {
			continue // a glow in the dark would give a hidden foe away
		}
		g.auras(a, float32(x), float32(y), dt)
	}
	f.parts.Update(dt)
}

func (g *Game) auras(a *game.Actor, x, y, dt float32) {
	p := g.fxs.parts
	u := &a.U
	rad := float32(0.55)
	burn, toxin := false, false
	var burnT, toxT skill.TypeID
	for _, d := range u.Dots {
		if d.Left == 0 {
			continue
		}
		switch g.sim.Reg.Get(d.Type).Status {
		case skill.StatusToxin:
			toxin, toxT = true, d.Type
		default:
			burn, burnT = true, d.Type
		}
	}
	if burn {
		r, gg, b := colorF(typeColor(burnT))
		p.Aura(fx.AuraBurn, x, y, rad, r, gg, b, dt)
	}
	if toxin {
		r, gg, b := colorF(typeColor(toxT))
		p.Aura(fx.AuraToxin, x, y, rad, r, gg, b, dt)
	}
	cr, cg, cb := colorF(typeColor(u.FrozenBy))
	switch {
	case u.Frozen:
		p.Aura(fx.AuraFrozen, x, y, rad, cr, cg, cb, dt)
	case u.SlowLeft > 0:
		p.Aura(fx.AuraChill, x, y, rad, cr, cg, cb, dt)
	}
	if u.MarkLeft > 0 {
		r, gg, b := colorF(typeCol[min(4, len(typeCol)-1)])
		p.Aura(fx.AuraMark, x, y, rad, r, gg, b, dt)
	}
	if u.StunLeft > 0 && !u.Frozen {
		p.Aura(fx.AuraStun, x, y, rad, 1, 1, 1, dt)
	}
	if u.Stealth > 0 {
		p.Aura(fx.AuraCloak, x, y, rad, 0.75, 0.8, 0.9, dt)
	}
	if u.Shield > 0 && u.MaxHP > 0 {
		p.ShieldAura(x, y, rad, u.Shield/u.MaxHP, dt)
	}
}

// spawn bursts a new effect's particles.
func (g *Game) spawn(e *game.FX) {
	p := g.fxs.parts
	x, y := float32(e.X), float32(e.Y)
	fx0, fy0 := float32(e.FromX), float32(e.FromY)
	shot := fx.Shot{Sig: g.sigOf(e.Type), Mag: e.Mag, Heat: e.Heat}
	g.kick(e)
	switch e.Kind {
	case game.FXBolt:
		dx, dy := x-fx0, y-fy0
		if !e.Impact {
			p.Trail(fx0, fy0, x, y, shot)
		}
		p.Impact(x, y, dx, dy, shot)
		p.Add(flash(x, y, shot, 0.45))
	case game.FXWave:
		r := (e.Value + 0.5) * sqrt3
		p.Wave(x, y, r, shot)
		p.Add(flash(x, y, shot, 0.8))
	case game.FXCone:
		l := (e.Value + 0.5) * sqrt3
		p.Wedge(x, y, e.DX, e.DY, 35*math.Pi/180, l, shot)
	case game.FXChain:
		for i := 1; i < int(e.NPts); i++ {
			a, b := e.Pts[i-1], e.Pts[i]
			s := shot
			s.Mag *= 0.6
			p.Trail(a[0], a[1], b[0], b[1], s)
		}
	case game.FXRing:
		p.Cast(x, y, e.Power, e.Heat)
	case game.FXKill:
		c := colText
		if e.Slot < len(killCol) {
			c = killCol[e.Slot]
		}
		r, gg, b := colorF(c)
		p.Kill(x, y, r, gg, b, fx.Shot{Sig: g.sigOf(e.Type), Mag: 1.5})
	case game.FXSlam:
		p.Impact((x+fx0)/2, (y+fy0)/2, fx0-x, fy0-y, fx.Shot{Sig: fx.Terrain, Mag: 1.5})
	case game.FXPush:
		p.Trail(fx0, fy0, x, y, fx.Shot{Sig: fx.Sig{R: 0.9, G: 0.9, B: 0.9, Force: 0.6}, Mag: 0.5})
	case game.FXClaw:
		r, gg, b := colorF(colHot)
		p.Impact(x, y, x-fx0, y-fy0, fx.Shot{Sig: fx.Sig{R: r, G: gg, B: b, Force: 0.5}, Mag: 0.6})
	case game.FXVent:
		p.Cast(x, y, 0, -60)
	case game.FXFall:
		p.Wave(x, y, -0.8, fx.Shot{Sig: fx.Sig{R: 0.54, G: 0.17, B: 0.25}, Mag: 1})
	case game.FXMend:
		p.Mend(x, y, e.Mag)
	case game.FXShield:
		p.Shield(x, y, e.Mag)
	case game.FXSoak:
		p.Soak(x, y, x-fx0, y-fy0, e.Mag)
	case game.FXPurge:
		p.Purge(x, y, e.Value)
	}
}

// killCol is the debris color for each actor kind.
var killCol = func() []color.RGBA {
	c := make([]color.RGBA, 8)
	for k, v := range enemyCol {
		if int(k) < len(c) {
			c[k] = v
		}
	}
	c[game.KindPlayer] = colPlayer
	return c
}()

// flash is a big, short glow at a hit or the heart of an area.
func flash(x, y float32, s fx.Shot, size float32) fx.P {
	m := float32(math.Sqrt(float64(s.Mag)))
	sig := s.Sig
	size /= fx.DrawScale
	return fx.P{X: x, Y: y, Life: 0.14 + 0.05*m, Size0: size * (0.6 + 0.3*m), Size1: size * (1 + 0.5*m),
		R0: 1, G0: 1, B0: 1, R1: sig.R, G1: sig.G, B1: sig.B, A: 0.8}
}

// drawParticles draws every particle in one additive triangle batch.
func (g *Game) drawParticles(screen *ebiten.Image) {
	f := &g.fxs
	p := f.parts
	if p.N == 0 {
		return
	}
	vs, is := f.vs[:0], f.is[:0]
	s := float32(g.size)
	ox0, oy0 := g.toScreen(0, 0)
	ox, oy := float32(ox0), float32(oy0)
	W, H := float32(g.w), float32(g.h)
	for i := 0; i < p.N; i++ {
		q := &p.P[i]
		l := q.Look()
		if l.A < 0.01 {
			continue
		}
		cx, cy := q.X*s+ox, q.Y*s+oy
		r := max(l.Size*s, 1)
		if cx < -r*4 || cy < -r*4 || cx > W+r*4 || cy > H+r*4 {
			continue
		}
		// Half-extent axes: ux, uy along the particle, vx, vy across it.
		ux, uy, vx, vy := r, float32(0), float32(0), r
		if l.Len > 0 {
			half := r + l.Len*s*0.5
			ux, uy = l.DX*half, l.DY*half
			w := r
			if q.Sprite == fx.SpriteShard {
				w = r * 0.9
			}
			vx, vy = -l.DY*w, l.DX*w
		}
		sx0 := float32(int(q.Sprite) * spriteCell)
		sx1 := sx0 + spriteCell
		cr, cg, cb, ca := l.R*l.A, l.G*l.A, l.B*l.A, l.A
		base := uint16(len(vs))
		vs = append(vs,
			ebiten.Vertex{DstX: cx - ux - vx, DstY: cy - uy - vy, SrcX: sx0, SrcY: 0, ColorR: cr, ColorG: cg, ColorB: cb, ColorA: ca},
			ebiten.Vertex{DstX: cx + ux - vx, DstY: cy + uy - vy, SrcX: sx1, SrcY: 0, ColorR: cr, ColorG: cg, ColorB: cb, ColorA: ca},
			ebiten.Vertex{DstX: cx + ux + vx, DstY: cy + uy + vy, SrcX: sx1, SrcY: spriteCell, ColorR: cr, ColorG: cg, ColorB: cb, ColorA: ca},
			ebiten.Vertex{DstX: cx - ux + vx, DstY: cy - uy + vy, SrcX: sx0, SrcY: spriteCell, ColorR: cr, ColorG: cg, ColorB: cb, ColorA: ca},
		)
		is = append(is, base, base+1, base+2, base, base+2, base+3)
		// A wide faint halo round every bright bit, so sparks bloom.
		if q.Sprite != fx.SpriteSquare && l.A > 0.15 {
			hr := r*2.6 + l.Len*s*0.3
			ha := ca * 0.16
			hcr, hcg, hcb := l.R*ha, l.G*ha, l.B*ha
			base = uint16(len(vs))
			vs = append(vs,
				ebiten.Vertex{DstX: cx - hr, DstY: cy - hr, SrcX: 0, SrcY: 0, ColorR: hcr, ColorG: hcg, ColorB: hcb, ColorA: ha},
				ebiten.Vertex{DstX: cx + hr, DstY: cy - hr, SrcX: spriteCell, SrcY: 0, ColorR: hcr, ColorG: hcg, ColorB: hcb, ColorA: ha},
				ebiten.Vertex{DstX: cx + hr, DstY: cy + hr, SrcX: spriteCell, SrcY: spriteCell, ColorR: hcr, ColorG: hcg, ColorB: hcb, ColorA: ha},
				ebiten.Vertex{DstX: cx - hr, DstY: cy + hr, SrcX: 0, SrcY: spriteCell, ColorR: hcr, ColorG: hcg, ColorB: hcb, ColorA: ha},
			)
			is = append(is, base, base+1, base+2, base, base+2, base+3)
		}
	}
	if len(is) > 0 {
		screen.DrawTriangles(vs, is, f.atlas, &ebiten.DrawTrianglesOptions{Blend: ebiten.BlendLighter, Filter: ebiten.FilterLinear})
	}
	f.vs, f.is = vs, is
}

// noise is a cheap smooth-ish value in -1 to 1 for jagged lines.
func noise(a, b float64) float64 {
	return math.Sin(a*12.9898+b*78.233) * math.Cos(a*4.1+b*1.7)
}

func easeOut(k float64) float64 { return 1 - (1-k)*(1-k)*(1-k) }

// fill and stroke draw the current path in one color.
func (g *Game) fillPath(dst *ebiten.Image, c color.RGBA, a float32) {
	f := &g.fxs
	vs, is := f.path.AppendVerticesAndIndicesForFilling(f.vs[:0], f.is[:0])
	g.tint(dst, vs, is, c, a)
}

func (g *Game) strokePath(dst *ebiten.Image, c color.RGBA, a, width float32) {
	f := &g.fxs
	vs, is := f.path.AppendVerticesAndIndicesForStroke(f.vs[:0], f.is[:0], &vector.StrokeOptions{Width: width, LineJoin: vector.LineJoinRound})
	g.tint(dst, vs, is, c, a)
}

func (g *Game) tint(dst *ebiten.Image, vs []ebiten.Vertex, is []uint16, c color.RGBA, a float32) {
	r, gg, b := colorF(c)
	for i := range vs {
		vs[i].SrcX, vs[i].SrcY = 1, 1
		vs[i].ColorR, vs[i].ColorG, vs[i].ColorB, vs[i].ColorA = r*a, gg*a, b*a, a
	}
	if len(is) > 0 {
		dst.DrawTriangles(vs, is, whiteSub(), &ebiten.DrawTrianglesOptions{AntiAlias: true, Blend: ebiten.BlendLighter})
	}
	g.fxs.vs, g.fxs.is = vs[:0], is[:0]
}

// ringPath builds a ring of radius r round cx, cy in the signature's
// style: hex-sided for crystalline types, wobbling for hot ones, broken
// into segments for swarms and markers. from and to limit it to an arc
// (radians), for cones.
func (g *Game) ringPath(cx, cy, r float64, st fx.RingStyle, t, seed float64, from, to float64, closed bool) {
	p := &g.fxs.path
	*p = vector.Path{}
	sides := st.Sides
	arc := to - from
	n := sides
	if sides > 6 || !closed {
		n = max(8, int(float64(sides)*math.Abs(arc)/(2*math.Pi)+0.5))
	}
	spin := t * float64(st.Spin) * 2 * math.Pi
	dashOn := 1 - 0.55*float64(st.Dash)
	pen := false
	for i := 0; i <= n; i++ {
		k := float64(i) / float64(n)
		a := from + arc*k
		rr := r
		if sides == 6 && closed {
			a = -math.Pi/2 + float64(i)*math.Pi/3 + spin*0.2
		}
		if st.Wobble > 0 {
			rr *= 1 + float64(st.Wobble)*(0.6*math.Sin(5*a+t*9+seed)+0.4*math.Sin(9*a-t*13+seed*2))
		}
		x, y := cx+math.Cos(a)*rr, cy+math.Sin(a)*rr
		on := true
		if st.Dash > 0 {
			ph := math.Mod(k*float64(n)/2+spin/(2*math.Pi)*float64(n)/4, 1)
			if ph < 0 {
				ph++
			}
			on = ph < dashOn
		}
		switch {
		case on && !pen:
			p.MoveTo(float32(x), float32(y))
			pen = true
		case on:
			p.LineTo(float32(x), float32(y))
		default:
			pen = false
		}
	}
}

// jagPath builds a line from a to b that wanders off true by jag (a
// fraction of its length), re-rolled every couple of frames so arcs crackle.
func (g *Game) jagPath(ax, ay, bx, by float64, jag float64, seed float64) {
	p := &g.fxs.path
	*p = vector.Path{}
	dx, dy := bx-ax, by-ay
	l := math.Hypot(dx, dy)
	if l < 1e-6 {
		return
	}
	nx, ny := -dy/l, dx/l
	segs := max(3, min(10, int(l/(g.size*0.9))+2))
	tick := float64(g.frame / 2)
	p.MoveTo(float32(ax), float32(ay))
	for i := 1; i < segs; i++ {
		k := float64(i) / float64(segs)
		off := jag * l * noise(seed+float64(i), tick) * math.Sin(k*math.Pi)
		p.LineTo(float32(ax+dx*k+nx*off), float32(ay+dy*k+ny*off))
	}
	p.LineTo(float32(bx), float32(by))
}

// drawForms draws the shapes of effects: bolts, arcs, rings, cones and
// cast rings. Particles go on top in drawParticles; numbers and labels
// after that in drawLabels.
func (g *Game) drawForms(screen *ebiten.Image) {
	s := g.size
	t := float64(g.frame) / 60
	g.sim.FX.Each(func(e *game.FX) {
		k := float64(e.Age / e.Life)
		a := float32(1 - k)
		x, y := g.toScreen(e.X, e.Y)
		seed := float64(int(e.X*7) ^ int(e.Y*13))
		switch e.Kind {
		case game.FXBolt:
			sig := g.sigOf(e.Type)
			c := typeColor(e.Type)
			m := math.Sqrt(float64(e.Mag))
			// The hit itself: a hot glow and a ring kicked out from it.
			if k < 0.5 {
				kk := k / 0.5
				cr, cg, cb := colorF(c)
				g.glow(screen, x, y, s*(0.9+0.6*m)*(1-0.4*kk), cr, cg, cb, float32(1-kk)*0.9)
				g.glow(screen, x, y, s*(0.35+0.2*m)*(1-kk), 1, 1, 1, float32(1-kk))
				g.ringPath(x, y, s*(0.3+(0.9+0.4*m)*easeOut(kk)), sig.Ring(), t, seed, 0, 2*math.Pi, true)
				g.strokePath(screen, c, float32(1-kk)*0.9, g.u(3*(1-kk)*m+1))
			}
			if e.Impact {
				return
			}
			fx0, fy0 := g.toScreen(e.FromX, e.FromY)
			head := math.Min(1, k/0.3)
			tail := math.Max(0, (k-0.1)/0.6)
			if tail >= head {
				return
			}
			ax, ay := fx0+(x-fx0)*tail, fy0+(y-fy0)*tail
			bx, by := fx0+(x-fx0)*head, fy0+(y-fy0)*head
			g.jagPath(ax, ay, bx, by, float64(sig.Jag()), seed)
			g.strokePath(screen, c, a*0.18, g.u(16*(0.7+0.3*m)))
			g.strokePath(screen, c, a*0.5, g.u(6*(0.7+0.3*m)))
			g.strokePath(screen, color.RGBA{0xff, 0xff, 0xff, 0xff}, a*0.8, g.u(1.6*(0.7+0.3*m)))
		case game.FXChain:
			sig := g.sigOf(e.Type)
			c := typeColor(e.Type)
			m := math.Sqrt(float64(e.Mag))
			for i := 1; i < int(e.NPts); i++ {
				// Each hop lights a beat after the one before it.
				local := (k - float64(i-1)*0.08) / (1 - float64(i-1)*0.08)
				if local <= 0 || local >= 1 {
					continue
				}
				la := float32(1 - local)
				ax, ay := g.toScreen(float64(e.Pts[i-1][0]), float64(e.Pts[i-1][1]))
				bx, by := g.toScreen(float64(e.Pts[i][0]), float64(e.Pts[i][1]))
				g.jagPath(ax, ay, bx, by, float64(sig.Jag())*1.4, seed+float64(i)*3.1)
				g.strokePath(screen, c, la*0.16, g.u(18*(0.7+0.3*m)))
				g.strokePath(screen, c, la*0.45, g.u(7*(0.7+0.3*m)))
				cr, cg, cb := colorF(c)
				g.glow(screen, bx, by, s*0.9*m, cr, cg, cb, la*0.8)
				g.strokePath(screen, color.RGBA{0xff, 0xff, 0xff, 0xff}, la*0.85, g.u(1.8))
			}
		case game.FXWave:
			sig := g.sigOf(e.Type)
			st := sig.Ring()
			c := typeColor(e.Type)
			m := math.Sqrt(float64(e.Mag))
			R := (float64(e.Value) + 0.5) * sqrt3 * s
			e1 := easeOut(k)
			r := R * (0.2 + 0.8*e1)
			heat := math.Max(0, math.Min(1.5, float64(e.Heat)/25))
			if k < 0.35 {
				cr, cg, cb := colorF(c)
				g.glow(screen, x, y, R*0.9, cr, cg, cb, float32(1-k/0.35)*0.7)
			}
			g.ringPath(x, y, r, st, t, seed, 0, 2*math.Pi, true)
			g.fillPath(screen, c, a*a*0.16)
			g.ringPath(x, y, r, st, t, seed, 0, 2*math.Pi, true)
			g.strokePath(screen, c, a*0.2, g.u((6+6*m)*float64(a)+2))
			g.ringPath(x, y, r, st, t, seed, 0, 2*math.Pi, true)
			g.strokePath(screen, c, float32(float64(a)*(0.65+0.2*heat)), g.u((1.5+3*m)*float64(a)+1))
			if st.Echo && k > 0.1 {
				r2 := R * (0.2 + 0.8*easeOut(k-0.1)) * 0.82
				g.ringPath(x, y, r2, st, t, seed+1, 0, 2*math.Pi, true)
				g.strokePath(screen, color.RGBA{0xff, 0xff, 0xff, 0xff}, a*0.4, g.u(1.5))
			}
		case game.FXCone:
			sig := g.sigOf(e.Type)
			st := sig.Ring()
			st.Sides = 40 // cones are arcs, never hex rings
			c := typeColor(e.Type)
			m := math.Sqrt(float64(e.Mag))
			L := (float64(e.Value) + 0.5) * sqrt3 * s
			dir := math.Atan2(float64(e.DY), float64(e.DX))
			half := 35 * math.Pi / 180
			e1 := easeOut(k)
			// Wedge fill out to the front.
			p := &g.fxs.path
			*p = vector.Path{}
			p.MoveTo(float32(x), float32(y))
			p.Arc(float32(x), float32(y), float32(L*e1), float32(dir-half), float32(dir+half), vector.Clockwise)
			p.Close()
			g.fillPath(screen, c, a*a*0.14)
			// The front and two ripples behind it.
			for j := 0; j < 3; j++ {
				r := L * e1 * (1 - 0.22*float64(j))
				if r <= 0 {
					continue
				}
				g.ringPath(x, y, r, st, t, seed+float64(j), dir-half, dir+half, false)
				w := (1.2 + 2.5*m) * float64(a) / float64(j+1)
				g.strokePath(screen, c, a*(0.8-0.22*float32(j)), g.u(w+0.8))
			}
		case game.FXRing:
			c := slotCol[e.Slot%game.MaxSlots]
			// Power spent sets the ring's weight, Heat added how far it
			// flares and how hot it tints.
			pw := math.Min(2, float64(e.Power)/15)
			ht := math.Max(-1, math.Min(1.5, float64(e.Heat)/20))
			r := s * (0.7 + (1.8+0.6*ht)*easeOut(k))
			st := fx.RingStyle{Sides: 40, Wobble: float32(math.Max(0, ht) * 0.08)}
			g.ringPath(x, y, r, st, t, seed, 0, 2*math.Pi, true)
			g.strokePath(screen, c, a, g.u((1.5+2.5*pw)*float64(a)+0.8))
		case game.FXSlam:
			wx, wy := g.toScreen(e.FromX, e.FromY)
			mx, my := (x+wx)/2, (y+wy)/2
			r := s * (0.3 + 0.9*easeOut(k))
			g.ringPath(mx, my, r, fx.RingStyle{Sides: 6}, t, seed, 0, 2*math.Pi, true)
			g.strokePath(screen, color.RGBA{0xff, 0xe0, 0x60, 0xff}, a, g.u(3))
		case game.FXFall:
			vector.DrawFilledCircle(screen, float32(x), float32(y), float32(s*0.9*float64(a)), fade(colPitRim, a), true)
		case game.FXKill:
			if k < 0.4 {
				g.glow(screen, x, y, s*1.6*(1-k), 1, 0.95, 0.85, float32(1-k/0.4)*0.8)
			}
			r := s * (0.4 + 1.3*easeOut(k))
			g.ringPath(x, y, r, fx.RingStyle{Sides: 6}, t, seed, 0, 2*math.Pi, true)
			g.strokePath(screen, colText, a*0.6, g.u(2*float64(a)+0.6))
		case game.FXClaw:
			fx0, fy0 := g.toScreen(e.FromX, e.FromY)
			// Three raking slashes across the line of the attack.
			dx, dy := x-fx0, y-fy0
			l := math.Hypot(dx, dy) + 1e-9
			nx, ny := -dy/l, dx/l
			ux, uy := dx/l, dy/l
			sweep := easeOut(math.Min(1, k/0.6))
			for j := -1; j <= 1; j++ {
				o := float64(j) * s * 0.28
				ax, ay := x+nx*o-ux*s*0.5-nx*s*0.5, y+ny*o-uy*s*0.5-ny*s*0.5
				bx, by := ax+(ux+nx)*s*sweep, ay+(uy+ny)*s*sweep
				vector.StrokeLine(screen, float32(ax), float32(ay), float32(bx), float32(by), g.u(2.5), fade(colHot, a), true)
			}
		case game.FXVent:
			r := s * (1 + 4*easeOut(k))
			g.ringPath(x, y, r, fx.RingStyle{Sides: 6}, t, seed, 0, 2*math.Pi, true)
			g.strokePath(screen, color.RGBA{0xd8, 0xf4, 0xff, 0xff}, a, g.u(5*float64(a)+1))
		case game.FXPush:
			fx0, fy0 := g.toScreen(e.FromX, e.FromY)
			vector.StrokeLine(screen, float32(fx0), float32(fy0), float32(x), float32(y), float32(s*0.5*float64(a)), fade(color.RGBA{0xf0, 0xf0, 0xf0, 0xff}, a*0.3), true)
		case game.FXMend:
			// A green glow and a ring that pulls in tight round the unit,
			// with a stitch of crossed lines at its heart.
			m := math.Sqrt(float64(e.Mag))
			g.glow(screen, x, y, s*(0.9+0.5*m), fx.MendR, fx.MendG, fx.MendB, a*0.6)
			r := s * (1.3 - 0.8*easeOut(k))
			g.ringPath(x, y, r, fx.RingStyle{Sides: 6, Dash: 0.5, Spin: 0.6}, t, seed, 0, 2*math.Pi, true)
			g.strokePath(screen, colMend, a, g.u(2+1.5*m))
			if k < 0.6 {
				l := float32(s * 0.22 * (1 - k/0.6*0.4))
				ca := fade(colMend, float32(1-k/0.6))
				vector.StrokeLine(screen, float32(x)-l, float32(y), float32(x)+l, float32(y), g.u(2.5), ca, true)
				vector.StrokeLine(screen, float32(x), float32(y)-l, float32(x), float32(y)+l, g.u(2.5), ca, true)
			}
		case game.FXShield:
			// The field snaps out to a hex shell and settles.
			m := math.Sqrt(float64(e.Mag))
			r := s * (0.4 + 0.5*easeOut(math.Min(1, k/0.35)))
			if k < 0.4 {
				g.glow(screen, x, y, s*1.1, fx.ShieldR, fx.ShieldG, fx.ShieldB, float32(1-k/0.4)*0.6)
			}
			g.ringPath(x, y, r, fx.RingStyle{Sides: 6}, t, seed, 0, 2*math.Pi, true)
			g.fillPath(screen, colShield, a*0.15)
			g.ringPath(x, y, r, fx.RingStyle{Sides: 6}, t, seed, 0, 2*math.Pi, true)
			g.strokePath(screen, colShield, a, g.u(1.5+2*m))
		case game.FXSoak:
			// The shell lights up where the hit broke on it.
			r := s * 0.9
			g.ringPath(x, y, r, fx.RingStyle{Sides: 6}, t, seed, 0, 2*math.Pi, true)
			g.strokePath(screen, colShield, a*0.9, g.u(3*float64(a)+1))
			g.glow(screen, x, y, s*0.9, fx.ShieldR, fx.ShieldG, fx.ShieldB, a*0.35)
		case game.FXPurge:
			// A cold flash and a low, flat ring blowing out at deck level.
			if k < 0.3 {
				g.glow(screen, x, y, s*1.3, fx.PurgeR, fx.PurgeG, fx.PurgeB, float32(1-k/0.3)*0.7)
			}
			r := s * (0.6 + 2.2*easeOut(k))
			g.ringPath(x, y, r, fx.RingStyle{Sides: 40, Wobble: 0.06}, t, seed, 0, 2*math.Pi, true)
			g.strokePath(screen, colPurge, a*0.8, g.u(4*float64(a)+0.8))
		}
	})
}

// kick shakes the screen for heavy moments and reddens the edge when the
// player takes a real hit.
func (g *Game) kick(e *game.FX) {
	l := &g.look
	add := func(v float64) { l.shake = math.Min(1, l.shake+v) }
	switch e.Kind {
	case game.FXNumber:
		if e.Hurt && !e.Small {
			l.hurt = min(1, l.hurt+0.35+0.4*min(1, e.Value/20))
			add(0.35)
		}
	case game.FXBolt:
		if e.Mag >= 1.5 {
			add(0.12 * float64(min(e.Mag, 4)))
		}
	case game.FXSlam:
		add(0.4)
	case game.FXKill:
		if e.Slot == int(game.KindBrute) {
			add(0.5)
		} else {
			add(0.15)
		}
	case game.FXFall:
		add(0.3)
	}
}

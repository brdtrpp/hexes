package main

// How the class parts look in the world and on their tiles: devices
// (turrets, drones, zones, traps), raised minions, wrecks on the floor,
// beams, taunts, thorns, haste and stances. Like everything else it's
// drawn from paths, and a rarer form adds structure (an outer ring, then
// spokes) rather than just getting bigger.

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"hexes/fx"
	"hexes/game"
	"hexes/skill"
)

var (
	colDevice = color.RGBA{0x6c, 0xf2, 0xc2, 0xff} // your devices: mint
	colMinion = color.RGBA{0x9c, 0xff, 0x6a, 0xff} // raised bodies: grave green
	colEmber  = color.RGBA{0xff, 0x8a, 0x3a, 0xff} // a wreck's last heat
	colForm   = color.RGBA{0xff, 0xb4, 0x6a, 0xff}
	colCost   = color.RGBA{0xff, 0x5a, 0x6e, 0xff}
	colStance = color.RGBA{0xa8, 0xb8, 0xff, 0xff}
	colTaunt  = color.RGBA{0xff, 0x7a, 0x3a, 0xff}
	colThorns = color.RGBA{0xc8, 0xe8, 0x4a, 0xff}
	colHaste  = color.RGBA{0xff, 0xf0, 0x8a, 0xff}
	colBlink  = color.RGBA{0xd8, 0xb8, 0xff, 0xff}
	colDark   = color.RGBA{0x12, 0x18, 0x22, 0xff}

	stanceCol = [skill.NumStances]color.RGBA{
		{0x6b, 0xc7, 0xff, 0xff}, // bulwark
		{0xff, 0xd0, 0x5a, 0xff}, // hunter
		{0xff, 0x6a, 0x2a, 0xff}, // furnace
	}
)

// ruleCol is the accent a form's rule adds: rare blue, exotic violet.
func ruleCol(rule uint8) color.RGBA {
	switch {
	case rule >= 2:
		return rarityCol[skill.Exotic]
	case rule == 1:
		return rarityCol[skill.Rare]
	}
	return colDevice
}

func sigCol(c color.RGBA) fx.Sig {
	r, g, b := colorF(c)
	return fx.Sig{R: r, G: g, B: b}
}

// drawGroundExtras draws what lies flat on the floor: wrecks, zones and
// traps. It runs before the standing pass so bodies stand over them.
func (g *Game) drawGroundExtras(screen *ebiten.Image) {
	sim := g.sim
	for i := range sim.Actors {
		a := &sim.Actors[i]
		switch {
		case a.Kind == game.KindDevice && a.U.Alive && a.Dev == skill.FormZone:
			g.drawZone(screen, a)
		case a.Kind == game.KindDevice && a.U.Alive && a.Dev == skill.FormTrap:
			g.drawTrap(screen, a)
		case !a.U.Alive && a.DeadFor > 0.6 && sim.IsWreck(i):
			g.drawWreck(screen, i)
		}
	}
}

// onScreen reports whether world point x, y is near enough to draw.
func (g *Game) onScreen(sx, sy, pad float64) bool {
	return sx > -pad && sy > -pad && sx < float64(g.w)+pad && sy < float64(g.h)+pad
}

// drawWreck draws a fallen foe's body lying charred on the floor, with a
// faint ember while it can still be used.
func (g *Game) drawWreck(screen *ebiten.Image, i int) {
	a := &g.sim.Actors[i]
	x, y := a.Pos()
	gx, gy, p := g.project(x, y)
	if !g.onScreen(gx, gy, g.size*2) {
		return
	}
	alpha := float32(math.Min(1, float64(game.WreckLife-a.DeadFor)/4)) * g.seeAt(x, y)
	if alpha < 0.02 {
		return
	}
	img := g.look.body[a.Kind]
	b := img.Bounds()
	fx0, fy0 := a.FaceVec()
	ang := math.Atan2(fy0, fx0) + 0.7 // it lies toppled, turned off true
	sq := viewCos * p * p
	op := &ebiten.DrawImageOptions{}
	op.Filter = ebiten.FilterLinear
	op.GeoM.Translate(-float64(b.Dx())/2, -float64(b.Dy())/2)
	op.GeoM.Scale(0.95, 0.7)
	op.GeoM.Rotate(ang)
	op.GeoM.Scale(p, sq)
	op.GeoM.Translate(gx, gy)
	op.ColorScale.Scale(0.3*alpha, 0.25*alpha, 0.22*alpha, alpha)
	screen.DrawImage(img, op)
	t := float64(g.frame) / 60
	R := g.look.bodyR[a.Kind]
	flick := float32(0.6 + 0.4*math.Sin(t*2.3+float64(i)*1.9))
	er, eg, eb := colorF(colEmber)
	g.glow(screen, gx, gy, R*p*0.5, er, eg, eb, alpha*0.3*flick)
}

// fadeOut is how far a device or minion has faded as its life runs out.
func fadeOut(life float32) float32 { return min(1, max(0, life/0.8)) }

// drawZone draws a field lying on the floor: a dashed edge, a hex that
// closes as it builds to its next run, and drift or frost marks by rule.
func (g *Game) drawZone(screen *ebiten.Image, a *game.Actor) {
	x, y := a.Pos()
	sx, sy := g.toScreen(x, y)
	s := g.size
	R := skill.ZoneRadius * sqrt3 * s
	if !g.onScreen(sx, sy, R*1.5) {
		return
	}
	t := float64(g.frame) / 60
	fa := fadeOut(a.Life)
	c := ruleCol(a.Rule)
	seed := float64(a.U.ID)
	br := R * (1 + 0.04*math.Sin(t*3+seed))
	g.ringPath(sx, sy, br, fx.RingStyle{Sides: 40}, t, seed, 0, 2*math.Pi, true)
	g.fillPath(screen, c, 0.07*fa)
	g.ringPath(sx, sy, br, fx.RingStyle{Sides: 40, Dash: 0.5, Spin: 0.12}, t, seed, 0, 2*math.Pi, true)
	g.strokePath(screen, c, 0.6*fa, g.u(2))
	k := 1 - float64(a.Clock/max(a.Every, 0.01))
	k = math.Max(0, math.Min(1, k))
	g.ringPath(sx, sy, R*(0.85-0.6*k), fx.RingStyle{Sides: 6}, t, seed, 0, 2*math.Pi, true)
	g.strokePath(screen, c, (0.2+0.4*float32(k))*fa, g.u(1.5))
	if a.Rule >= 1 {
		g.spokes(sx, sy, 3, t*0.8+seed, R*0.9, R*1.08, true)
		g.strokePath(screen, c, 0.5*fa, g.u(2))
	}
	if a.Rule >= 2 { // it chills: a frost star
		g.spokes(sx, sy, 6, -t*0.3, R*0.15, R*0.4, true)
		g.strokePath(screen, color.RGBA{0x8f, 0xdc, 0xff, 0xff}, 0.5*fa, g.u(1.5))
	}
}

// drawTrap draws a plate with teeth and a dot for each arm it has left.
func (g *Game) drawTrap(screen *ebiten.Image, a *game.Actor) {
	x, y := a.Pos()
	sx, sy := g.toScreen(x, y)
	s := g.size
	if !g.onScreen(sx, sy, s*3) {
		return
	}
	t := float64(g.frame) / 60
	fa := fadeOut(a.Life)
	seed := float64(a.U.ID)
	R0 := s * 0.42
	c := ruleCol(a.Rule)
	armed := a.Clock <= 0
	g.ringPath(sx, sy, skill.TrapRadius*sqrt3*s*0.8, fx.RingStyle{Sides: 40, Dash: 0.6, Spin: 0.05}, t, seed, 0, 2*math.Pi, true)
	g.strokePath(screen, c, 0.18*fa, g.u(1.2))
	g.ringPath(sx, sy, R0, fx.RingStyle{Sides: 6}, t, seed, 0, 2*math.Pi, true)
	g.fillDark(screen, colDark, 0.85*fa)
	g.ringPath(sx, sy, R0, fx.RingStyle{Sides: 6}, t, seed, 0, 2*math.Pi, true)
	g.strokePath(screen, c, 0.75*fa, g.u(1.8))
	tc := fade(colTaunt, 0.5)
	if armed {
		tc = fade(colTaunt, float32(0.65+0.35*math.Sin(t*6+seed)))
	}
	g.spokes(sx, sy, 6, 0, R0*0.3, R0*0.85, true)
	g.strokePath(screen, tc, fa, g.u(2))
	if a.Arms > 0 {
		g.dots(sx, sy, int(a.Arms), t*0.6, R0*1.3, s*0.07)
		g.fillPath(screen, c, 0.9*fa)
	}
	if a.Rule >= 2 { // it stuns: a second ring
		g.ringPath(sx, sy, R0*1.55, fx.RingStyle{Sides: 40}, t, seed, 0, 2*math.Pi, true)
		g.strokePath(screen, c, 0.35*fa, g.u(1.2))
	}
}

// drawDevice draws a standing device (a turret or a drone) in the
// standing pass, where drawBody would draw a body.
func (g *Game) drawDevice(screen *ebiten.Image, a *game.Actor, l actorLook) {
	s := g.size
	t := float64(g.frame) / 60
	fa := fadeOut(a.Life) * l.alpha
	if a.Life < 2 && math.Sin(t*20) < 0 { // about to go: it blinks
		fa *= 0.55
	}
	c := ruleCol(a.Rule)
	p := l.p
	sq := viewCos * p * p
	seed := float64(a.U.ID)
	charge := 1 - float64(a.Clock/max(a.Every, 0.01))
	charge = math.Max(0, math.Min(1, charge))
	fresh := a.Every > 0 && a.Clock > a.Every-0.15 // it just ran its hex
	cr, cg, cb := colorF(c)
	switch a.Dev {
	case skill.FormTurret:
		g.drawEllipse(screen, l.gx, l.gy, s*0.5*p, s*0.5*sq*0.8, 0, 0, 0, 0.5*fa)
		g.ringPath(l.gx, l.gy, s*0.48, fx.RingStyle{Sides: 6}, t, seed, 0, 2*math.Pi, true)
		g.fillDark(screen, colDark, fa)
		g.ringPath(l.gx, l.gy, s*0.48, fx.RingStyle{Sides: 6}, t, seed, 0, 2*math.Pi, true)
		g.strokePath(screen, c, 0.6*fa, g.u(1.6))
		hx, hy := l.gx, l.gy-g.upright(0.55, p)
		vector.StrokeLine(screen, float32(l.gx), float32(l.gy), float32(hx), float32(hy), g.u(5), fade(colDark, fa), true)
		vector.StrokeLine(screen, float32(l.gx), float32(l.gy), float32(hx), float32(hy), g.u(1.5), fade(c, 0.6*fa), true)
		dx, dy := a.FaceX, a.FaceY
		if dx == 0 && dy == 0 {
			dx = 1
		}
		bx, by := hx+dx*s*0.55*p, hy+dy*s*0.55*sq
		vector.StrokeLine(screen, float32(hx), float32(hy), float32(bx), float32(by), g.u(5), fade(colDark, fa), true)
		vector.StrokeLine(screen, float32(hx), float32(hy), float32(bx), float32(by), g.u(2), fade(c, fa), true)
		g.local(func() { g.ringPath(hx, hy, s*0.26, fx.RingStyle{Sides: 40}, t, seed, 0, 2*math.Pi, true) })
		g.fillDark(screen, colDark, fa)
		g.local(func() { g.ringPath(hx, hy, s*0.26, fx.RingStyle{Sides: 40}, t, seed, 0, 2*math.Pi, true) })
		g.strokePath(screen, c, fa, g.u(2))
		// How close it is to its next shot, as an arc round the head.
		g.local(func() {
			g.ringPath(hx, hy, s*0.38, fx.RingStyle{Sides: 40}, t, seed, -math.Pi/2, -math.Pi/2+2*math.Pi*charge, false)
		})
		g.strokePath(screen, c, 0.55*fa, g.u(1.5))
		if a.Rule >= 1 {
			g.local(func() { g.ringPath(hx, hy, s*0.48, fx.RingStyle{Sides: 6, Spin: 0.3}, t, seed, 0, 2*math.Pi, true) })
			g.strokePath(screen, c, 0.3*fa, g.u(1.2))
		}
		if a.Rule >= 2 {
			g.local(func() { g.spokes(hx, hy, 6, t, s*0.5, s*0.62, true) })
			g.strokePath(screen, c, 0.45*fa, g.u(1.4))
		}
		g.glow(screen, hx, hy, s*0.3*p, cr, cg, cb, 0.5*fa)
		if fresh {
			g.glow(screen, bx, by, s*0.45*p, 1, 1, 1, 0.8*fa)
		}
	case skill.FormDrone:
		h := 0.95 + 0.07*math.Sin(t*4+seed)
		g.drawEllipse(screen, l.gx, l.gy, s*0.3*p, s*0.3*sq*0.8, 0, 0, 0, 0.35*fa)
		hx, hy := l.gx, l.gy-g.upright(h, p)
		dx, dy := a.FaceX, a.FaceY
		ang := math.Atan2(dy, dx)
		pt := func(u, v float64) (float32, float32) {
			ca, sa := math.Cos(ang), math.Sin(ang)
			return float32(hx + (u*ca-v*sa)*s*p), float32(hy + (u*sa+v*ca)*s*sq)
		}
		path := &g.fxs.path
		*path = vector.Path{}
		x0, y0 := pt(0.32, 0)
		x1, y1 := pt(0, -0.18)
		x2, y2 := pt(-0.22, 0)
		x3, y3 := pt(0, 0.18)
		path.MoveTo(x0, y0)
		path.LineTo(x1, y1)
		path.LineTo(x2, y2)
		path.LineTo(x3, y3)
		path.Close()
		g.fillDark(screen, colDark, fa)
		*path = vector.Path{}
		path.MoveTo(x0, y0)
		path.LineTo(x1, y1)
		path.LineTo(x2, y2)
		path.LineTo(x3, y3)
		path.Close()
		g.strokePath(screen, c, fa, g.u(1.8))
		rotors := 4
		if a.Rule >= 1 {
			rotors = 6
		}
		for j := 0; j < rotors; j++ {
			an := float64(j)*2*math.Pi/float64(rotors) + math.Pi/4
			rx, ry := pt(math.Cos(an)*0.34, math.Sin(an)*0.34)
			g.local(func() { g.spokes(float64(rx), float64(ry), 2, t*30+float64(j), 0, s*0.12, true) })
			g.strokePath(screen, c, 0.7*fa, g.u(1.2))
		}
		core := 0.4 + 0.3*charge
		if a.Rule >= 2 { // it bursts at the end: a core that swells as life runs out
			core += 0.4 * (1 - math.Min(1, float64(a.Life)/4))
		}
		g.glow(screen, hx, hy, s*0.22*p, cr, cg, cb, float32(core)*fa)
		if fresh {
			g.glow(screen, hx, hy, s*0.35*p, 1, 1, 1, 0.7*fa)
		}
	}
}

// drawStanceRing marks the stances you hold with a slow ring at your feet,
// one per stance.
func (g *Game) drawStanceRing(screen *ebiten.Image) {
	sim := g.sim
	if sim.Stance == 0 {
		return
	}
	p := sim.P()
	if !p.U.Alive {
		return
	}
	x, y := p.Pos()
	sx, sy := g.toScreen(x, y)
	t := float64(g.frame) / 60
	s := g.size
	n := 0
	for k := 0; k < skill.NumStances; k++ {
		if sim.Stance&(1<<k) == 0 {
			continue
		}
		c := stanceCol[k]
		r := s * (0.95 + 0.22*float64(n))
		st := fx.RingStyle{Sides: 6, Spin: 0.15}
		switch skill.StanceKind(k) {
		case skill.StanceHunter:
			st = fx.RingStyle{Sides: 40, Dash: 0.5, Spin: 0.4}
		case skill.StanceFurnace:
			st = fx.RingStyle{Sides: 40, Wobble: 0.05}
		}
		g.ringPath(sx, sy, r, st, t, float64(k), 0, 2*math.Pi, true)
		g.strokePath(screen, c, float32(0.45+0.15*math.Sin(t*2+float64(k))), g.u(2))
		n++
	}
}

// minionTint lays a raised body's grave-green over its plate.
func minionTint(op *ebiten.DrawImageOptions, screen, img *ebiten.Image, alpha float32) {
	r, g, b := colorF(colMinion)
	a := 0.45 * alpha
	op.ColorScale.Reset()
	op.ColorScale.Scale(r*a, g*a, b*a, a)
	op.Blend = ebiten.BlendLighter
	screen.DrawImage(img, op)
	op.Blend = ebiten.Blend{}
}

// classAuras adds the lingering looks of taunt, thorns and haste.
func (g *Game) classAuras(a *game.Actor, x, y, rad, dt float32) {
	p := g.fxs.parts
	u := &a.U
	if u.TauntLeft > 0 {
		r, gg, b := colorF(colTaunt)
		p.Aura(fx.AuraMark, x, y, rad, r, gg, b, dt)
	}
	if u.ThornsLeft > 0 {
		r, gg, b := colorF(colThorns)
		p.Aura(fx.AuraChill, x, y, rad*1.1, r, gg, b, dt)
	}
	if u.HasteLeft > 0 {
		r, gg, b := colorF(colHaste)
		p.Aura(fx.AuraToxin, x, y, rad, r, gg, b, dt)
	}
	if a.Minion {
		r, gg, b := colorF(colMinion)
		p.Aura(fx.AuraToxin, x, y, rad*0.8, r*0.6, gg*0.6, b*0.6, dt*0.5)
	}
}

// spawnClass bursts the particles of the class parts' effects. It reports
// whether e was one of them.
func (g *Game) spawnClass(e *game.FX, shot fx.Shot) bool {
	p := g.fxs.parts
	x, y := float32(e.X), float32(e.Y)
	fx0, fy0 := float32(e.FromX), float32(e.FromY)
	switch e.Kind {
	case game.FXBeam:
		p.Z = chestLift
		l := float32(math.Hypot(float64(e.DX), float64(e.DY)))
		if l < 1e-6 {
			return true
		}
		dx, dy := e.DX/l, e.DY/l
		L := e.Value * sqrt3
		p.Trail(x, y, x+dx*L, y+dy*L, shot)
		if e.Slot == 1 {
			p.Trail(x, y, x-dx*L, y-dy*L, shot)
		}
		p.Add(flash(x, y, shot, 0.4))
	case game.FXWreck:
		r, gg, b := colorF(colEmber)
		p.Kill(x, y, r, gg, b, fx.Shot{Sig: fx.Terrain, Mag: 1.3})
	case game.FXRaise:
		p.Mend(x, y, 1.4)
		p.Wave(x, y, 0.5, fx.Shot{Sig: sigCol(colMinion), Mag: 0.7})
	case game.FXDeploy:
		c := colDevice
		p.Wave(x, y, 0.5, fx.Shot{Sig: sigCol(c), Mag: 0.6})
	case game.FXDeviceFire:
		p.Z = chestLift
		p.Add(flash(x, y, fx.Shot{Sig: sigCol(colDevice), Mag: 0.5}, 0.3))
	case game.FXExpire:
		c := colDevice
		if e.Value > 0 {
			c = colMinion
		}
		r, gg, b := colorF(c)
		p.Kill(x, y, r, gg, b, fx.Shot{Sig: fx.Neutral, Mag: 0.6})
	case game.FXThorns:
		p.Z = chestLift
		sg := sigCol(colThorns)
		sg.Cold = 0.6
		p.Impact(x, y, x-fx0, y-fy0, fx.Shot{Sig: sg, Mag: 0.8})
	case game.FXTaunt:
		p.Z = chestLift
		p.Trail(fx0, fy0, x, y, fx.Shot{Sig: sigCol(colTaunt), Mag: 0.4})
	case game.FXBuff:
		c := colThorns
		if skill.EffectKind(e.Slot) == skill.EffectHaste {
			c = colHaste
		}
		p.Wave(x, y, 0.4, fx.Shot{Sig: sigCol(c), Mag: 0.5})
	case game.FXAmbush:
		p.Z = chestLift
		sg := sigCol(colBlink)
		sg.Force = 0.8
		p.Impact(x, y, x-fx0, y-fy0, fx.Shot{Sig: sg, Mag: 1.3})
		p.Add(flash(x, y, fx.Shot{Sig: sg, Mag: 1.5}, 0.6))
	default:
		return false
	}
	return true
}

// drawClassForm draws the shapes of the class parts' effects. It reports
// whether e was one of them.
func (g *Game) drawClassForm(screen *ebiten.Image, e *game.FX, k float64, x, y, seed float64) bool {
	s := g.size
	t := float64(g.frame) / 60
	a := float32(1 - k)
	switch e.Kind {
	case game.FXBeam:
		l := math.Hypot(float64(e.DX), float64(e.DY))
		if l < 1e-6 {
			return true
		}
		dx, dy := float64(e.DX)/l, float64(e.DY)/l
		L := float64(e.Value) * sqrt3
		sig := g.sigOf(e.Type)
		c := typeColor(e.Type)
		m := math.Sqrt(float64(e.Mag))
		ax, ay, _ := g.lifted(e.X, e.Y, chestLift)
		head := math.Min(1, k/0.2)
		tail := math.Max(0, (k-0.3)/0.6)
		ends := 1
		if e.Slot == 1 {
			ends = 2
		}
		for j := 0; j < ends; j++ {
			sg := 1.0
			if j == 1 {
				sg = -1
			}
			bx, by, _ := g.lifted(e.X+dx*L*sg, e.Y+dy*L*sg, chestLift)
			if tail >= head {
				continue
			}
			px, py := ax+(bx-ax)*tail, ay+(by-ay)*tail
			qx, qy := ax+(bx-ax)*head, ay+(by-ay)*head
			g.drawBolt(screen, sig.Arch(), e.Grade, c, float32(px), float32(py), float32(qx), float32(qy), a, m*1.2, seed+float64(j), sig)
		}
	case game.FXWreck:
		r := s * (0.3 + 1.4*easeOut(k))
		g.ringPath(x, y, r, fx.RingStyle{Sides: 6, Wobble: 0.08}, t, seed, 0, 2*math.Pi, true)
		g.strokePath(screen, colEmber, a, g.u(3*float64(a)+0.6))
		g.spokes(x, y, 7, seed, r*0.5, r*1.1, true)
		g.strokePath(screen, colEmber, a*0.7, g.u(1.5))
	case game.FXRaise:
		r := s * (1.3 - 0.9*easeOut(k))
		g.ringPath(x, y, r, fx.RingStyle{Sides: 6, Spin: 0.5}, t, seed, 0, 2*math.Pi, true)
		g.strokePath(screen, colMinion, a, g.u(2.5))
		_, _, p := g.project(e.X, e.Y)
		top := y - g.upright(1.4*easeOut(k), p)
		vector.StrokeLine(screen, float32(x), float32(y), float32(x), float32(top), g.u(3*float64(a)+0.5), fade(colMinion, a*0.7), true)
	case game.FXDeploy:
		c := colDevice
		if e.Slot < skill.NumForms {
			c = ruleCol(0)
		}
		r := s * (1.5 - 1.0*easeOut(k))
		g.ringPath(x, y, r, fx.RingStyle{Sides: 6}, t, seed, 0, 2*math.Pi, true)
		g.strokePath(screen, c, a, g.u(2.5))
		g.spokes(x, y, 3, -math.Pi/2, r*0.9, r*1.2, true)
		g.strokePath(screen, c, a*0.8, g.u(2))
	case game.FXDeviceFire:
		_, _, p := g.project(e.X, e.Y)
		h := 0.55
		if skill.FormKind(e.Slot) == skill.FormDrone {
			h = 0.95
		}
		if f := skill.FormKind(e.Slot); f == skill.FormZone || f == skill.FormTrap {
			h = 0
		}
		hy := y - g.upright(h, p)
		g.local(func() {
			g.ringPath(x, hy, s*(0.25+0.6*easeOut(k)), fx.RingStyle{Sides: 6}, t, seed, 0, 2*math.Pi, true)
		})
		g.strokePath(screen, colDevice, a, g.u(2))
	case game.FXExpire:
		c := colDevice
		if e.Value > 0 {
			c = colMinion
		}
		r := s * (0.5 + 0.8*easeOut(k))
		g.ringPath(x, y, r, fx.RingStyle{Sides: 40, Dash: 0.6}, t, seed, 0, 2*math.Pi, true)
		g.strokePath(screen, c, a*0.7, g.u(1.5))
	case game.FXThorns:
		hx, hy, _ := g.lifted(e.X, e.Y, chestLift)
		g.local(func() { g.spokes(hx, hy, 8, seed, s*0.15, s*(0.5+0.4*easeOut(k)), true) })
		g.strokePath(screen, colThorns, a, g.u(2))
	case game.FXTaunt:
		fx0, fy0, _ := g.lifted(e.FromX, e.FromY, chestLift)
		hx, hy, _ := g.lifted(e.X, e.Y, chestLift)
		head := math.Min(1, k/0.3)
		vector.StrokeLine(screen, float32(fx0), float32(fy0), float32(fx0+(hx-fx0)*head), float32(fy0+(hy-fy0)*head), g.u(1.5), fade(colTaunt, a*0.6), true)
		g.local(func() { g.brackets(hx, hy, s*(0.9-0.3*easeOut(k)), t, true) })
		g.strokePath(screen, colTaunt, a, g.u(2.2))
	case game.FXBuff:
		c := colThorns
		if skill.EffectKind(e.Slot) == skill.EffectHaste {
			c = colHaste
		}
		r := s * (0.5 + 0.7*easeOut(k))
		g.ringPath(x, y, r, fx.RingStyle{Sides: 6, Spin: 0.6}, t, seed, 0, 2*math.Pi, true)
		g.strokePath(screen, c, a, g.u(2))
	case game.FXAmbush:
		hx, hy, p := g.lifted(e.X, e.Y, chestLift)
		cr, cg, cb := colorF(colBlink)
		if k < 0.4 {
			g.glow(screen, hx, hy, s*1.4*p, cr, cg, cb, float32(1-k/0.4)*0.8)
		}
		g.local(func() { g.brackets(hx, hy, s*(0.5+0.5*easeOut(k)), t*2, true) })
		g.strokePath(screen, colBlink, a, g.u(2.5))
	default:
		return false
	}
	return true
}

// classGlyph draws the signs of the class parts on their tiles: forms,
// costs, stances and the new effects and targets. It reports whether it
// drew one.
func (g *Game) classGlyph(dst *ebiten.Image, x, y, r float64, p skill.Part) bool {
	path := &g.fxs.path
	*path = vector.Path{}
	mv := func(u, v float64) { path.MoveTo(float32(x+u*r), float32(y+v*r)) }
	ln := func(u, v float64) { path.LineTo(float32(x+u*r), float32(y+v*r)) }
	circ := func(u, v, rr float64) {
		path.MoveTo(float32(x+u*r+rr*r), float32(y+v*r))
		path.Arc(float32(x+u*r), float32(y+v*r), float32(rr*r), 0, 2*math.Pi, vector.Clockwise)
	}
	hexAt := func(u, v, rr float64) {
		for j := 0; j <= 6; j++ {
			an := float64(j)*math.Pi/3 - math.Pi/6
			if j == 0 {
				mv(u+math.Cos(an)*rr, v+math.Sin(an)*rr)
			} else {
				ln(u+math.Cos(an)*rr, v+math.Sin(an)*rr)
			}
		}
	}
	c := partCol(p)
	a := float32(0.55)
	switch p.Kind {
	case skill.KindForm:
		switch p.Form {
		case skill.FormBolt:
			mv(-0.7, 0)
			ln(0.4, 0)
			mv(0.15, -0.3)
			ln(0.7, 0)
			ln(0.15, 0.3)
			ln(0.32, 0)
			ln(0.15, -0.3)
		case skill.FormBeam:
			mv(-0.75, -0.1)
			ln(0.75, -0.1)
			ln(0.75, 0.1)
			ln(-0.75, 0.1)
			ln(-0.75, -0.1)
			mv(-0.75, -0.35)
			ln(-0.75, 0.35)
			mv(0.75, -0.35)
			ln(0.75, 0.35)
		case skill.FormAura:
			circ(0, 0, 0.12)
			circ(0, 0, 0.42)
			circ(0, 0, 0.72)
		case skill.FormNova:
			for j := 0; j < 8; j++ {
				an := float64(j) * math.Pi / 4
				mv(math.Cos(an)*0.2, math.Sin(an)*0.2)
				ln(math.Cos(an)*0.75, math.Sin(an)*0.75)
			}
		case skill.FormDash:
			for _, o := range []float64{-0.45, 0, 0.45} {
				mv(o-0.2, -0.4)
				ln(o+0.15, 0)
				ln(o-0.2, 0.4)
			}
		case skill.FormDetonate:
			hexAt(0, 0.1, 0.45)
			for j := 0; j < 5; j++ {
				an := -math.Pi/2 + float64(j-2)*0.45
				mv(math.Cos(an)*0.5, 0.1+math.Sin(an)*0.5)
				ln(math.Cos(an)*0.85, 0.1+math.Sin(an)*0.85)
			}
		case skill.FormZone:
			for j := 0; j < 8; j++ {
				a0 := float64(j) * math.Pi / 4
				path.MoveTo(float32(x+math.Cos(a0)*r*0.7), float32(y+math.Sin(a0)*r*0.7))
				path.Arc(float32(x), float32(y), float32(r*0.7), float32(a0), float32(a0+0.45), vector.Clockwise)
			}
			hexAt(0, 0, 0.3)
		case skill.FormTurret:
			hexAt(0, 0.35, 0.35)
			mv(0, 0.35)
			ln(0, -0.15)
			circ(0, -0.25, 0.2)
			mv(0.15, -0.25)
			ln(0.75, -0.25)
		case skill.FormTrap:
			hexAt(0, 0, 0.55)
			for j := 0; j < 6; j++ {
				an := float64(j)*math.Pi/3 + math.Pi/6
				mv(math.Cos(an)*0.15, math.Sin(an)*0.15)
				ln(math.Cos(an)*0.45, math.Sin(an)*0.45)
			}
		case skill.FormDrone:
			mv(0.4, 0)
			ln(0, -0.22)
			ln(-0.3, 0)
			ln(0, 0.22)
			ln(0.4, 0)
			for _, q := range [][2]float64{{-0.5, -0.5}, {0.5, -0.5}, {-0.5, 0.5}, {0.5, 0.5}} {
				circ(q[0], q[1], 0.16)
			}
		}
		// Structure by rarity: a rare form adds an outer hex, exotic spokes.
		if rule := p.Rule(); rule >= 1 {
			g.strokeUI(dst, c, a, g.u(2))
			*path = vector.Path{}
			hexAt(0, 0, 0.95)
			if rule >= 2 {
				for j := 0; j < 6; j++ {
					an := float64(j)*math.Pi/3 - math.Pi/6
					mv(math.Cos(an)*0.95, math.Sin(an)*0.95)
					ln(math.Cos(an)*1.15, math.Sin(an)*1.15)
				}
			}
			g.strokeUI(dst, ruleCol(rule), a*0.8, g.u(1.5))
			return true
		}
	case skill.KindCost:
		switch p.Cost {
		case skill.CostBlood:
			path.MoveTo(float32(x), float32(y-r*0.75))
			path.QuadTo(float32(x+r*0.6), float32(y+r*0.05), float32(x), float32(y+r*0.55))
			path.QuadTo(float32(x-r*0.6), float32(y+r*0.05), float32(x), float32(y-r*0.75))
		case skill.CostWrecks:
			hexAt(-0.2, 0.1, 0.4)
			mv(-0.2, -0.3)
			ln(-0.05, 0.05)
			ln(-0.3, 0.2)
			mv(0.3, -0.45)
			ln(0.6, -0.2)
			ln(0.4, 0)
		default:
			mv(0.1, -0.7)
			ln(-0.3, 0.1)
			ln(0.05, 0.1)
			ln(-0.1, 0.7)
			ln(0.3, -0.1)
			ln(-0.05, -0.1)
			ln(0.1, -0.7)
		}
	case skill.KindStance:
		hexAt(0, 0, 0.75)
		switch p.Stance {
		case skill.StanceBulwark:
			mv(-0.35, -0.35)
			ln(0.35, -0.35)
			ln(0.35, 0.05)
			ln(0, 0.4)
			ln(-0.35, 0.05)
			ln(-0.35, -0.35)
		case skill.StanceHunter:
			circ(0, 0, 0.3)
			for j := 0; j < 4; j++ {
				an := float64(j) * math.Pi / 2
				mv(math.Cos(an)*0.15, math.Sin(an)*0.15)
				ln(math.Cos(an)*0.5, math.Sin(an)*0.5)
			}
		case skill.StanceFurnace:
			mv(0, -0.45)
			path.QuadTo(float32(x+r*0.4), float32(y), float32(x), float32(y+r*0.35))
			path.QuadTo(float32(x-r*0.4), float32(y), float32(x), float32(y-r*0.45))
		}
		c = stanceCol[min(int(p.Stance), skill.NumStances-1)]
	case skill.KindEffect:
		switch p.Effect {
		case skill.EffectTaunt:
			circ(0, 0, 0.12)
			for j := 0; j < 4; j++ {
				an := float64(j)*math.Pi/2 + math.Pi/4
				path.MoveTo(float32(x+math.Cos(an-0.45)*r*0.65), float32(y+math.Sin(an-0.45)*r*0.65))
				path.Arc(float32(x), float32(y), float32(r*0.65), float32(an-0.45), float32(an+0.45), vector.Clockwise)
			}
		case skill.EffectThorns:
			circ(0, 0, 0.35)
			for j := 0; j < 8; j++ {
				an := float64(j) * math.Pi / 4
				mv(math.Cos(an-0.2)*0.35, math.Sin(an-0.2)*0.35)
				ln(math.Cos(an)*0.75, math.Sin(an)*0.75)
				ln(math.Cos(an+0.2)*0.35, math.Sin(an+0.2)*0.35)
			}
		case skill.EffectReanimate:
			mv(-0.6, 0.55)
			ln(0.6, 0.55)
			mv(0, 0.4)
			ln(0, -0.6)
			mv(-0.3, -0.3)
			ln(0, -0.6)
			ln(0.3, -0.3)
		case skill.EffectHaste:
			for _, o := range []float64{-0.3, 0.15} {
				mv(o-0.15, -0.4)
				ln(o+0.25, 0)
				ln(o-0.15, 0.4)
			}
			mv(-0.75, -0.2)
			ln(-0.5, -0.2)
			mv(-0.8, 0.2)
			ln(-0.55, 0.2)
		case skill.EffectBlink:
			circ(-0.45, 0.25, 0.18)
			circ(0.45, -0.25, 0.18)
			for j := 1; j < 4; j++ {
				k := float64(j) / 4
				circ(-0.45+0.9*k, 0.25-0.5*k, 0.04)
			}
		default:
			return false
		}
	case skill.KindTarget:
		switch p.Target {
		case skill.TargetWreck:
			hexAt(0, 0.15, 0.4)
			mv(-0.15, -0.25)
			ln(0.05, 0.1)
			ln(-0.2, 0.3)
			mv(-0.7, -0.6)
			ln(-0.4, -0.3)
		case skill.TargetWeakest:
			mv(-0.6, -0.15)
			ln(0.6, -0.15)
			ln(0.6, 0.15)
			ln(-0.6, 0.15)
			ln(-0.6, -0.15)
			mv(-0.6, 0)
			ln(-0.25, 0)
		case skill.TargetMarked:
			g.brackets(x, y, r*0.6, 0, true)
			circ(0, 0, 0.1)
		case skill.TargetLine, skill.TargetBoth:
			mv(-0.75, 0)
			ln(0.75, 0)
			mv(0.45, -0.25)
			ln(0.75, 0)
			ln(0.45, 0.25)
			if p.Target == skill.TargetBoth {
				mv(-0.45, -0.25)
				ln(-0.75, 0)
				ln(-0.45, 0.25)
			}
		case skill.TargetAllies, skill.TargetAura:
			circ(0, 0, 0.18)
			circ(-0.5, 0.35, 0.14)
			circ(0.5, 0.35, 0.14)
			circ(0, -0.55, 0.14)
		default:
			return false
		}
	default:
		return false
	}
	g.strokeUI(dst, c, a, g.u(2))
	return true
}

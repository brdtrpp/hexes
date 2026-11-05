package fx

import "math"

// Shot is what an effect knows about the hex behind it.
type Shot struct {
	Sig Sig
	// Mag is how hard it landed: about 0.15 for a scratch, 1 for a quarter
	// of the foe's health, up to 3 for a one-shot. See MagOf.
	Mag float32
	// Heat is the Heat the hex added when it fired (negative for cryo
	// builds that vent). Hotter hexes burn brighter and throw more sparks.
	Heat float32
}

// MagOf turns damage dealt against a foe's max health into a magnitude.
func MagOf(dealt, maxHP float32) float32 {
	if maxHP <= 0 {
		maxHP = 60
	}
	return min(max(dealt/maxHP*4, 0.15), 3)
}

func (s Shot) m() float32 { return float32(math.Sqrt(float64(max(s.Mag, 0)))) }

// heat01 is the shot's Heat on a rough -1 to 1.5 scale.
func (s Shot) heat01() float32 { return min(max(s.Heat/25, -1), 1.5) }

// pick chooses a facet for one particle by the signature's weights.
func (p *Pool) pick(s Sig) Facet {
	w := s.weights()
	var sum float32
	for _, v := range w {
		sum += v
	}
	r := p.rand() * sum
	for f, v := range w {
		if r < v {
			return Facet(f)
		}
		r -= v
	}
	return FacetPlain
}

// particle makes one particle of facet f at x, y heading along angle a.
// speed scales the facet's natural speed (1 is normal).
func (p *Pool) particle(f Facet, s Shot, x, y, a, speed float32) P {
	m := s.m()
	sig := s.Sig
	q := P{X: x, Y: y, Seed: p.rand(), A: 0.95}
	var v float32
	switch f {
	case FacetHot:
		v = p.rr(1.5, 3.5)
		q.Drag, q.Grav = 3, -3.2
		q.Life = p.rr(0.45, 0.9)
		q.Size0, q.Size1 = p.rr(0.12, 0.18), 0.03
		q.R0, q.G0, q.B0 = 1, 0.95, 0.72
		q.R1, q.G1, q.B1 = sig.R*0.9, sig.G*0.35, sig.B*0.2
		q.Flicker, q.Wobble = 0.8, 0.8
	case FacetCold:
		v = p.rr(2.5, 4.5)
		q.Drag, q.Grav = 8, 2.4
		q.Life = p.rr(0.6, 1.0)
		q.Size0, q.Size1 = p.rr(0.08, 0.11), 0.06
		q.R0, q.G0, q.B0 = 0.6+0.4*sig.R, 0.6+0.4*sig.G, 0.6+0.4*sig.B
		q.R1, q.G1, q.B1 = sig.R, sig.G, sig.B
		q.Flicker, q.Stretch = 0.5, 0.025
		q.Sprite = SpriteShard
	case FacetForce:
		v = p.rr(7, 13)
		q.Drag = 9
		q.Life = p.rr(0.15, 0.3)
		q.Size0, q.Size1 = p.rr(0.06, 0.09), 0.02
		q.R0, q.G0, q.B0 = 1, 1, 1
		q.R1, q.G1, q.B1 = sig.R, sig.G, sig.B
		q.Stretch = 0.05
		q.Sprite = SpriteStreak
	case FacetSwarm:
		v = p.rr(1, 2.5)
		q.Drag = 1.5
		q.Swirl = p.rr(4, 8)
		if p.rand() < 0.5 {
			q.Swirl = -q.Swirl
		}
		q.Wobble = 1.5
		q.Life = p.rr(0.9, 1.6)
		q.Size0, q.Size1 = p.rr(0.05, 0.075), 0.045
		q.R0, q.G0, q.B0 = sig.R, sig.G, sig.B
		q.R1, q.G1, q.B1 = sig.R*0.6, sig.G*0.6, sig.B*0.6
		q.A = 0.85
	case FacetMark:
		v = p.rr(3, 5)
		q.Drag = 7
		q.Life = p.rr(0.4, 0.7)
		q.Size0, q.Size1 = p.rr(0.06, 0.08), 0.05
		q.R0, q.G0, q.B0 = sig.R, sig.G, sig.B
		q.R1, q.G1, q.B1 = 1, 1, 1
		q.Flicker = 0.25
		q.Sprite = SpriteSquare
	case FacetVeil:
		v = p.rr(0.5, 1.2)
		q.Drag = 2
		q.Life = p.rr(0.6, 1.0)
		q.Size0, q.Size1 = 0.15, p.rr(0.35, 0.5)
		q.R0, q.G0, q.B0 = sig.R, sig.G, sig.B
		q.R1, q.G1, q.B1 = sig.R, sig.G, sig.B
		q.A = 0.25
	default:
		v = p.rr(2, 4)
		q.Drag = 5
		q.Life = p.rr(0.25, 0.45)
		q.Size0, q.Size1 = p.rr(0.09, 0.14), 0.02
		q.R0, q.G0, q.B0 = 1, 1, 1
		q.R1, q.G1, q.B1 = sig.R, sig.G, sig.B
	}
	v *= (0.8 + 0.3*m) * speed
	q.Size0 *= 0.85 + 0.25*m
	q.Size1 *= 0.85 + 0.25*m
	q.Life *= 0.9 + 0.15*m
	if h := s.heat01(); h > 0 {
		q.A = min(1, q.A*(1+0.3*h))
	}
	sn, cs := sincos(a)
	q.VX, q.VY = cs*v, sn*v
	return q
}

// spray spawns n particles from x, y, centred on angle a and spread
// either side of it.
func (p *Pool) spray(s Shot, x, y, a, spread float32, n int, speed float32) {
	for i := 0; i < n; i++ {
		f := p.pick(s.Sig)
		p.Add(p.particle(f, s, x, y, a+p.rr(-spread, spread), speed))
	}
}

func angle(dx, dy float32) float32 { return float32(math.Atan2(float64(dy), float64(dx))) }

// Impact is a hit landing at x, y, travelling along dx, dy (from caster
// to target; zero for no direction). Most of it sprays on through the
// target; the rest bursts round it.
func (p *Pool) Impact(x, y, dx, dy float32, s Shot) {
	m := s.m()
	n := p.Count(10*(0.6+0.9*m) + 4*max(s.heat01(), 0))
	a := angle(dx, dy)
	if dx == 0 && dy == 0 {
		p.spray(s, x, y, 0, math.Pi, n, 1)
	} else {
		fwd := n * 2 / 3
		p.spray(s, x, y, a, 1.2, fwd, 1)
		p.spray(s, x, y, a, math.Pi, n-fwd, 0.6)
	}
	if s.Sig.Splash > 0 {
		k := p.Count(6 * s.Sig.Splash)
		p.spray(s, x, y, 0, math.Pi, k, 0.45)
	}
}

// Trail lays particles along a bolt's path from x0, y0 to x1, y1.
func (p *Pool) Trail(x0, y0, x1, y1 float32, s Shot) {
	dx, dy := x1-x0, y1-y0
	l := float32(math.Hypot(float64(dx), float64(dy)))
	if l < 1e-3 {
		return
	}
	n := p.Count(l * 1.6 * (0.7 + 0.4*s.m()))
	a := angle(dx, dy)
	for i := 0; i < n; i++ {
		t := p.rand()
		f := p.pick(s.Sig)
		q := p.particle(f, s, x0+dx*t, y0+dy*t, a+p.rr(-0.5, 0.5), 0.35)
		q.Life *= 0.6
		p.Add(q)
	}
}

// Wave fills a round area of the given radius (hex units) with particles
// that fly out from the centre and stop near the edge.
func (p *Pool) Wave(x, y, radius float32, s Shot) {
	m := s.m()
	n := p.Count(14 + 8*radius*(0.6+0.6*m))
	for i := 0; i < n; i++ {
		a := p.rr(-math.Pi, math.Pi)
		f := p.pick(s.Sig)
		q := p.particle(f, s, x, y, a, 1)
		sn, cs := sincos(a)
		r0 := radius * 0.15
		q.X, q.Y = x+cs*r0, y+sn*r0
		// With linear drag a particle coasts v/drag before it stops.
		drag := max(q.Drag, 3)
		q.Drag = drag
		v := radius * drag * p.rr(0.6, 1.0)
		if f == FacetCold {
			v *= 0.7 // shards stall short and hang inside the ring
		}
		q.VX, q.VY = cs*v, sn*v
		q.Life = max(q.Life, 0.45)
		p.Add(q)
	}
}

// Wedge fills a cone from x, y along dx, dy, half radians either side,
// length long.
func (p *Pool) Wedge(x, y, dx, dy, half, length float32, s Shot) {
	m := s.m()
	n := p.Count(14 + 7*length*(0.6+0.6*m))
	a0 := angle(dx, dy)
	for i := 0; i < n; i++ {
		a := a0 + p.rr(-half, half)
		f := p.pick(s.Sig)
		q := p.particle(f, s, x, y, a, 1)
		sn, cs := sincos(a)
		drag := max(q.Drag, 3)
		q.Drag = drag
		v := length * drag * p.rr(0.35, 1.0)
		q.VX, q.VY = cs*v, sn*v
		q.Life = max(q.Life, 0.45)
		p.Add(q)
	}
}

// Kill bursts a dying foe into debris of its own color (r, g, b) and the
// killing type's facets.
func (p *Pool) Kill(x, y, r, g, b float32, s Shot) {
	n := p.Count(10 * (0.7 + 0.4*s.m()))
	for i := 0; i < n; i++ {
		q := p.particle(FacetPlain, s, x, y, p.rr(-math.Pi, math.Pi), 1.1)
		q.R0, q.G0, q.B0 = r, g, b
		q.R1, q.G1, q.B1 = r*0.4, g*0.4, b*0.4
		q.Sprite = SpriteSquare
		q.Grav = 3
		q.Life *= 1.8
		p.Add(q)
	}
	p.spray(s, x, y, 0, math.Pi, p.Count(8), 0.8)
}

// Rig colors for Power and Heat, matching the HUD bars.
var (
	PowerR, PowerG, PowerB = float32(0.3), float32(0.89), float32(1)
	HeatR, HeatG, HeatB    = float32(1), float32(0.54), float32(0.17)
)

// Cast is a hex firing at x, y. The Power it spent streams in to the
// caster and the Heat it added vents out as sparks (or, below zero, falls
// away as frost). Bigger costs, bigger show.
func (p *Pool) Cast(x, y, power, heat float32) {
	n := p.Count(min(max(power/2.5, 3), 18))
	for i := 0; i < n; i++ {
		a := p.rr(-math.Pi, math.Pi)
		sn, cs := sincos(a)
		r := p.rr(1.4, 2.0)
		t := p.rr(0.2, 0.3)
		q := P{X: x + cs*r, Y: y + sn*r, VX: -cs * r / t, VY: -sn * r / t, Life: t,
			Size0: 0.05, Size1: 0.11, A: 0.9, Seed: p.rand(), Stretch: 0.03, Sprite: SpriteStreak,
			R0: PowerR, G0: PowerG, B0: PowerB, R1: 1, G1: 1, B1: 1}
		p.Add(q)
	}
	switch {
	case heat > 0:
		hot := Shot{Sig: Sig{R: HeatR, G: HeatG, B: HeatB, Hot: 1}, Mag: 0.5, Heat: heat}
		p.spray(hot, x, y, -math.Pi/2, 1.4, p.Count(min(heat/1.5, 20)), 1)
	case heat < 0:
		k := p.Count(min(-heat/2, 12))
		for i := 0; i < k; i++ {
			a := p.rr(-math.Pi, math.Pi)
			sn, cs := sincos(a)
			p.Add(P{X: x + cs*0.4, Y: y + sn*0.4, VX: cs * 0.8, VY: sn*0.8 + 0.4, Life: p.rr(0.7, 1.1),
				Size0: 0.12, Size1: 0.3, A: 0.35, Drag: 1.5, Grav: 0.8, Seed: p.rand(),
				R0: 0.85, G0: 0.97, B0: 1, R1: 0.56, G1: 0.86, B1: 1})
		}
	}
}

// AuraKind is a lingering status shown on an actor.
type AuraKind uint8

const (
	AuraBurn AuraKind = iota
	AuraToxin
	AuraChill
	AuraFrozen
	AuraMark
	AuraStun
	AuraCloak
	NumAuras
)

// auraRate is particles per second for each aura.
var auraRate = [NumAuras]float32{AuraBurn: 12, AuraToxin: 7, AuraChill: 4, AuraFrozen: 6, AuraMark: 2.5, AuraStun: 5, AuraCloak: 3}

// Aura spawns this frame's share of a status's particles round an actor
// at x, y of radius rad, colored r, g, b. Call it every frame the status
// holds, with that frame's dt.
func (p *Pool) Aura(k AuraKind, x, y, rad, r, g, b, dt float32) {
	n := p.Count(auraRate[k] * dt)
	for i := 0; i < n; i++ {
		a := p.rr(-math.Pi, math.Pi)
		sn, cs := sincos(a)
		px, py := x+cs*rad*p.rr(0.3, 1), y+sn*rad*p.rr(0.3, 1)
		q := P{X: px, Y: py, Seed: p.rand(), A: 0.85, R0: r, G0: g, B0: b, R1: r, G1: g, B1: b}
		switch k {
		case AuraBurn:
			q.VX, q.VY = p.rr(-0.3, 0.3), p.rr(-1.6, -0.8)
			q.Grav, q.Drag, q.Wobble, q.Flicker = -1.5, 1, 0.6, 0.8
			q.Life, q.Size0, q.Size1 = p.rr(0.4, 0.7), 0.09, 0.02
			q.R0, q.G0, q.B0 = 1, 0.9, 0.6
			q.R1, q.G1, q.B1 = r*0.9, g*0.3, b*0.2
		case AuraToxin:
			q.VX, q.VY = -sn*1.2, cs*1.2
			q.Swirl, q.Wobble = 3, 0.6
			q.Life, q.Size0, q.Size1 = p.rr(0.8, 1.2), 0.05, 0.04
		case AuraChill, AuraFrozen:
			q.Life, q.Size0, q.Size1 = p.rr(0.3, 0.5), 0.02, 0.09
			q.Flicker, q.Sprite = 0.5, SpriteShard
			q.VX, q.VY, q.Stretch = cs*0.3, sn*0.3+0.2, 0.2
			q.R0, q.G0, q.B0 = 0.95, 1, 1
			if k == AuraFrozen {
				q.Size1 = 0.12
			}
		case AuraMark:
			q.X, q.Y = x+cs*rad*1.3, y+sn*rad*1.3
			q.VX, q.VY = -cs*1.0, -sn*1.0
			q.Life, q.Size0, q.Size1, q.Sprite = 0.5, 0.06, 0.04, SpriteSquare
		case AuraStun:
			q.X, q.Y = x+cs*rad*0.8, y-rad*1.1+sn*rad*0.25
			q.VX, q.VY = -sn*2.5, cs*0.6
			q.Swirl, q.Life, q.Size0, q.Size1 = 6, 0.4, 0.06, 0.03
			q.R0, q.G0, q.B0 = 1, 0.95, 0.6
		case AuraCloak:
			q.VX, q.VY = cs*0.4, sn*0.4
			q.Life, q.Size0, q.Size1, q.A = 0.8, 0.1, 0.3, 0.2
		}
		p.Add(q)
	}
}

// Sustain colors: nanite green for Mend, field blue for Shield, coolant
// white-cyan for Purge.
var (
	MendR, MendG, MendB       = float32(0.35), float32(1), float32(0.62)
	ShieldR, ShieldG, ShieldB = float32(0.42), float32(0.78), float32(1)
	PurgeR, PurgeG, PurgeB    = float32(0.78), float32(0.97), float32(1)
)

// Mend is hull knitting back at x, y: green nanite motes that gather in
// from round the unit, then rise off it like steam. mag is fx.MagOf's
// 0.15 to 3 on the amount restored.
func (p *Pool) Mend(x, y, mag float32) {
	m := float32(math.Sqrt(float64(max(mag, 0.15))))
	n := p.Count(6 + 8*m)
	for i := 0; i < n; i++ {
		a := p.rr(-math.Pi, math.Pi)
		sn, cs := sincos(a)
		r := p.rr(0.7, 1.1)
		t := p.rr(0.25, 0.4)
		p.Add(P{X: x + cs*r, Y: y + sn*r, VX: -cs * r / t, VY: -sn * r / t, Life: t, Swirl: 2,
			Size0: 0.04, Size1: 0.08, A: 0.9, Seed: p.rand(),
			R0: MendR, G0: MendG, B0: MendB, R1: 0.85, G1: 1, B1: 0.9})
	}
	k := p.Count(4 + 5*m)
	for i := 0; i < k; i++ {
		p.Add(P{X: x + p.rr(-0.3, 0.3), Y: y + p.rr(-0.2, 0.2), VX: p.rr(-0.2, 0.2), VY: p.rr(-1.3, -0.7),
			Life: p.rr(0.6, 1), Drag: 0.8, Wobble: 0.4, Size0: 0.07, Size1: 0.02, A: 0.8, Seed: p.rand(),
			Sprite: SpriteSquare, R0: 0.85, G0: 1, B0: 0.9, R1: MendR, G1: MendG, B1: MendB})
	}
}

// Shield is a buffer field snapping on round x, y: hard blue bits that fly
// out to the field's edge and hang there.
func (p *Pool) Shield(x, y, mag float32) {
	m := float32(math.Sqrt(float64(max(mag, 0.15))))
	n := p.Count(8 + 6*m)
	for i := 0; i < n; i++ {
		a := p.rr(-math.Pi, math.Pi)
		sn, cs := sincos(a)
		const drag = 6
		v := 0.75 * drag * p.rr(0.85, 1)
		p.Add(P{X: x + cs*0.2, Y: y + sn*0.2, VX: cs * v, VY: sn * v, Drag: drag, Life: p.rr(0.45, 0.7),
			Size0: 0.05, Size1: 0.04, A: 0.9, Flicker: 0.4, Seed: p.rand(), Sprite: SpriteSquare,
			R0: 1, G0: 1, B0: 1, R1: ShieldR, G1: ShieldG, B1: ShieldB})
	}
}

// Soak is a hit breaking on a shield at x, y, coming along dx, dy: blue
// chips glance off the field's face.
func (p *Pool) Soak(x, y, dx, dy, mag float32) {
	a0 := angle(-dx, -dy)
	if dx == 0 && dy == 0 {
		a0 = -math.Pi / 2
	}
	n := p.Count(5 + 4*float32(math.Sqrt(float64(max(mag, 0.15)))))
	for i := 0; i < n; i++ {
		a := a0 + p.rr(-1.1, 1.1)
		sn, cs := sincos(a)
		v := p.rr(2, 3.5)
		p.Add(P{X: x - cs*0.45, Y: y - sn*0.45, VX: cs * v, VY: sn * v, Drag: 5, Life: p.rr(0.2, 0.35),
			Size0: 0.03, Size1: 0.08, A: 0.9, Stretch: 0.05, Seed: p.rand(), Sprite: SpriteShard,
			R0: 1, G0: 1, B0: 1, R1: ShieldR, G1: ShieldG, B1: ShieldB})
	}
}

// Purge is the rig flushing coolant at x, y: white-cyan plumes jet out
// sideways from the vents and roll upward, more for more Heat purged.
func (p *Pool) Purge(x, y, heat float32) {
	n := p.Count(min(max(heat/1.5, 6), 18))
	for i := 0; i < n; i++ {
		side := float32(1)
		if i%2 == 1 {
			side = -1
		}
		v := p.rr(1.6, 2.6)
		p.Add(P{X: x + side*0.25, Y: y + p.rr(-0.1, 0.15), VX: side * v, VY: p.rr(-0.6, 0.1), Drag: 2.2, Grav: -1.2,
			Life: p.rr(0.6, 1), Size0: 0.1, Size1: 0.32, A: 0.4, Wobble: 0.3, Seed: p.rand(),
			R0: 1, G0: 1, B0: 1, R1: PurgeR, G1: PurgeG, B1: PurgeB})
	}
	// A few cold glints spat out with it.
	k := p.Count(min(heat/4, 6) + 2)
	for i := 0; i < k; i++ {
		a := p.rr(-math.Pi, 0)
		sn, cs := sincos(a)
		p.Add(P{X: x, Y: y, VX: cs * 2.5, VY: sn * 2.5, Drag: 3, Grav: 2, Life: p.rr(0.35, 0.55),
			Size0: 0.03, Size1: 0.07, A: 0.9, Stretch: 0.1, Seed: p.rand(), Sprite: SpriteShard,
			R0: 1, G0: 1, B0: 1, R1: PurgeR, G1: PurgeG, B1: PurgeB})
	}
}

// ShieldAura is a held shield on a unit at x, y: now and then a blue bit
// drifts round the field's edge. share is the shield as a share of max
// hull (0 to 0.6); a fuller shield sheds more.
func (p *Pool) ShieldAura(x, y, rad, share, dt float32) {
	n := p.Count((2 + 10*share) * dt)
	for i := 0; i < n; i++ {
		a := p.rr(-math.Pi, math.Pi)
		sn, cs := sincos(a)
		p.Add(P{X: x + cs*rad*1.25, Y: y + sn*rad*1.25, VX: -sn * 0.9, VY: cs * 0.9, Swirl: 1.6,
			Life: p.rr(0.4, 0.6), Size0: 0.04, Size1: 0.02, A: 0.7, Seed: p.rand(), Sprite: SpriteSquare,
			R0: ShieldR, G0: ShieldG, B0: ShieldB, R1: ShieldR, G1: ShieldG, B1: ShieldB})
	}
}

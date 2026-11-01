package game

import (
	"math"

	"hexes/dungeon"
	"hexes/skill"
)

// Free movement. The floor is still hexes, but the player walks in any
// direction: X, Y is a float position in hex units and At is whichever hex
// that point falls in. Enemies still step tile to tile.
//
// The player is a circle. It slides along wall hexes (circle against the
// hex polygon) and is kept clear of enemy bodies by a second circle on
// each enemy's tile, sized so the player's center can never share a hex
// with an enemy. Pits and hazards go off when the center enters the hex.

// Movement tuning. All placeholder.
const (
	PlayerRadius = 0.38 // player body, hex units (hex center to corner is 1)
	BodyRadius   = 0.64 // how close the player's center gets to an enemy tile center, less PlayerRadius
	PitFallFrac  = 0.25 // hull lost walking into a pit, share of max
	PitStun      = 0.5  // seconds frozen after climbing out of a pit
	maxSubstep   = 0.15 // longest move between collision checks, hex units
	slideMin     = 0.25 // along-wall share of a step below which it doesn't slide
	keepPlain    = 0.7  // share of a step kept by plain push-out that needs no slide help
)

// tileSpan is the distance between neighboring hex centers.
var tileSpan = math.Sqrt(3)

// hexCorners are a size-1 pointy-top hex's corners, clockwise from the top.
var hexCorners = func() (c [6][2]float64) {
	for i := range c {
		a := math.Pi/180*float64(60*i) - math.Pi/2
		c[i] = [2]float64{math.Cos(a), math.Sin(a)}
	}
	return
}()

// FaceVec is the direction the actor faces in hex units. The player's is
// its last movement direction; everyone else faces a hex direction.
func (a *Actor) FaceVec() (float64, float64) {
	if a.Free && (a.FaceX != 0 || a.FaceY != 0) {
		return a.FaceX, a.FaceY
	}
	dx, dy := skill.HexDirs[a.Facing].Pixel()
	l := math.Hypot(dx, dy)
	return dx / l, dy / l
}

// placeFree puts a free actor at the center of hex c.
func (a *Actor) placeFree(c skill.Coord) {
	a.At, a.From = c, c
	a.X, a.Y = c.Pixel()
}

// dirOf is the hex direction closest to (x, y).
func dirOf(x, y float64) uint8 {
	best, bestDot := 0, -math.MaxFloat64
	for i, d := range skill.HexDirs {
		dx, dy := d.Pixel()
		if dot := dx*x + dy*y; dot > bestDot {
			best, bestDot = i, dot
		}
	}
	return uint8(best)
}

// movePlayer walks the player along s.Input's direction. Input longer
// than 1 (diagonal keys, a finger far from the player) is cut to 1; an
// analog stick held halfway walks at half speed.
func (s *Sim) movePlayer(dt float32) {
	p := s.P()
	mx, my := s.Input.MoveX, s.Input.MoveY
	l := math.Hypot(mx, my)
	if l < 1e-9 || p.U.StunLeft > 0 || !p.U.Alive {
		return
	}
	p.FaceX, p.FaceY = mx/l, my/l
	p.Facing = dirOf(mx, my)
	if l > 1 {
		mx, my = mx/l, my/l
	}
	speed := PlayerSpeed * tileSpan
	if p.U.SlowLeft > 0 {
		speed *= float64(1 - p.U.SlowFrac)
	}
	dist := speed * float64(dt) * math.Hypot(mx, my)
	steps := int(math.Ceil(dist / maxSubstep))
	if steps < 1 {
		steps = 1
	}
	vx, vy := mx*speed*float64(dt)/float64(steps), my*speed*float64(dt)/float64(steps)
	ox, oy := p.X, p.Y
	for i := 0; i < steps; i++ {
		sx, sy := p.X, p.Y
		p.X += vx
		p.Y += vy
		if !s.collide(p) {
			continue
		}
		// Touching a wall. When the plain push-out kept most of the step
		// (gliding round a corner, into a doorway), keep it. Hex walls are
		// jagged, though, and pushing out of each hex alone can catch the
		// body in a notch between two of them. Then slide along the wall's
		// overall line instead: drop the part of the step that heads into
		// it and take the step again.
		v2 := vx*vx + vy*vy
		prog := (p.X-sx)*vx + (p.Y-sy)*vy
		if prog >= keepPlain*v2 {
			continue
		}
		nx, ny, ok := s.wallNormal(p.X, p.Y)
		into := vx*nx + vy*ny
		tx, ty := vx-into*nx, vy-into*ny
		// Pushing almost straight in is a stop: the body rests in its
		// notch instead of creeping along.
		if !ok || into >= 0 || tx*tx+ty*ty <= slideMin*slideMin*v2 {
			continue
		}
		p.X, p.Y = sx+tx, sy+ty
		s.collide(p)
	}
	s.settle(ox, oy)
}

// collide pushes the player out of enemy bodies and wall hexes, and
// reports whether a wall pushed. A few passes settle corners where more
// than one thing pushes at once.
func (s *Sim) collide(p *Actor) bool {
	walled := false
	for pass := 0; pass < 4; pass++ {
		moved := false
		c := skill.FromPixel(p.X, p.Y)
		for k := -1; k < 6; k++ {
			h := c
			if k >= 0 {
				h = c.Add(skill.HexDirs[k])
			}
			if s.Floor.At(h) == dungeon.Wall {
				if dx, dy, hit := hexPush(h, p.X, p.Y, PlayerRadius); hit {
					p.X += dx
					p.Y += dy
					moved, walled = true, true
				}
				continue
			}
			if !s.Floor.In(h) {
				continue
			}
			o := s.occ[s.Floor.Index(h)]
			if o == skill.NoEntity || o == s.Player || !s.Actors[o].U.Alive {
				continue
			}
			ex, ey := h.Pixel()
			dx, dy := p.X-ex, p.Y-ey
			d := math.Hypot(dx, dy)
			min := PlayerRadius + BodyRadius
			if d >= min {
				continue
			}
			if d < 1e-9 {
				dx, dy, d = p.FaceX, p.FaceY, 1
				if dx == 0 && dy == 0 {
					dx = 1
				}
			}
			p.X += dx / d * (min - d)
			p.Y += dy / d * (min - d)
			moved = true
		}
		if !moved {
			break
		}
	}
	return walled
}

// near2 is every offset within two hexes.
var near2 = func() (out [19]skill.Coord) {
	n := 0
	for q := -2; q <= 2; q++ {
		for r := -2; r <= 2; r++ {
			if c := (skill.Coord{Q: q, R: r}); skill.Dist(c, skill.Coord{}) <= 2 {
				out[n] = c
				n++
			}
		}
	}
	return
}()

// wallProbe is how far wallNormal looks for wall hex centers. It spans
// two rows of hexes, enough to see past the notch the body sits in.
const wallProbe = 2.6

// wallNormal is the direction away from the walls around (x, y), averaged
// over the nearby wall hexes so a jagged hex wall reads as one smooth
// line. ok is false when no wall is close.
func (s *Sim) wallNormal(x, y float64) (nx, ny float64, ok bool) {
	c := skill.FromPixel(x, y)
	for _, o := range near2 {
		h := c.Add(o)
		if s.Floor.At(h) != dungeon.Wall {
			continue
		}
		hx, hy := h.Pixel()
		dx, dy := x-hx, y-hy
		d := math.Hypot(dx, dy)
		if d >= wallProbe || d < 1e-9 {
			continue
		}
		w := (wallProbe - d) / d
		nx += dx * w
		ny += dy * w
	}
	l := math.Hypot(nx, ny)
	if l < 1e-9 {
		return 0, 0, false
	}
	return nx / l, ny / l, true
}

// hexPush is how far to move a circle at (x, y) of radius r so it no
// longer overlaps hex c, and whether it overlapped at all.
func hexPush(c skill.Coord, x, y, r float64) (float64, float64, bool) {
	cx, cy := c.Pixel()
	vx, vy := x-cx, y-cy
	if vx*vx+vy*vy >= (1+r)*(1+r) {
		return 0, 0, false
	}
	// Closest point on the hex outline.
	bestD := math.MaxFloat64
	var qx, qy float64
	for i := 0; i < 6; i++ {
		ax, ay := hexCorners[i][0], hexCorners[i][1]
		bx, by := hexCorners[(i+1)%6][0], hexCorners[(i+1)%6][1]
		ex, ey := bx-ax, by-ay
		t := ((vx-ax)*ex + (vy-ay)*ey) / (ex*ex + ey*ey)
		t = math.Max(0, math.Min(1, t))
		px, py := ax+ex*t, ay+ey*t
		if d := (vx-px)*(vx-px) + (vy-py)*(vy-py); d < bestD {
			bestD, qx, qy = d, px, py
		}
	}
	d := math.Sqrt(bestD)
	if insideHex(vx, vy) {
		// Center is in the wall: out through the nearest edge, then r more.
		if d < 1e-9 {
			l := math.Hypot(vx, vy)
			if l < 1e-9 {
				return r + 1, 0, true
			}
			return vx / l * (1 - l + r), vy / l * (1 - l + r), true
		}
		ox, oy := (qx-vx)/d, (qy-vy)/d
		return ox * (d + r), oy * (d + r), true
	}
	if d >= r {
		return 0, 0, false
	}
	if d < 1e-12 {
		l := math.Hypot(vx, vy)
		return vx / l * r, vy / l * r, true
	}
	return (vx - qx) / d * (r - d), (vy - qy) / d * (r - d), true
}

// insideHex reports whether (x, y), relative to a hex's center, is inside
// a size-1 pointy-top hex.
func insideHex(x, y float64) bool {
	const apo = 0.8660254037844386 // sqrt(3)/2, center to edge
	ax, ay := math.Abs(x), math.Abs(y)
	return ax <= apo && ax*0.5+ay*apo <= apo
}

// settle moves the player's tile to wherever its center ended up and runs
// what the new tile does. If the center landed somewhere it can't be (a
// wall, or a tile another body claimed this frame), the move is undone.
func (s *Sim) settle(ox, oy float64) {
	p := s.P()
	c := skill.FromPixel(p.X, p.Y)
	if c == p.At {
		return
	}
	if !s.Floor.In(c) || s.Floor.At(c) == dungeon.Wall || s.occupied(c) {
		p.X, p.Y = ox, oy
		return
	}
	s.occ[s.Floor.Index(p.At)] = skill.NoEntity
	s.occ[s.Floor.Index(c)] = s.Player
	p.At, p.From = c, c
	if s.Floor.At(c) == dungeon.Pit {
		s.fallInPit()
		return
	}
	s.safe = c
	s.refreshField()
}

// fallInPit is the player walking off a ledge: lose a chunk of hull and
// climb back out at the last tile you stood on, stunned for a moment.
// Pushes into a pit still kill outright, for anyone.
func (s *Sim) fallInPit() {
	p := s.P()
	pit := p.At
	s.addFX(FX{Kind: FXFall, At: pit, Hurt: true, Life: 0.7})
	loss := PitFallFrac * p.U.MaxHP
	p.U.HP -= loss
	p.Flash = 0.3
	s.occ[s.Floor.Index(pit)] = skill.NoEntity
	back, ok := s.freeNear(s.safe)
	if p.U.HP <= 0 || !ok {
		p.U.HP, p.U.Alive, p.Fell = 0, false, true
		s.died(s.Player)
		return
	}
	p.placeFree(back)
	s.occ[s.Floor.Index(back)] = s.Player
	p.U.StunLeft = max(p.U.StunLeft, PitStun)
	s.addFX(FX{Kind: FXNumber, At: back, Value: loss, Hurt: true, Terrain: true, Life: 0.8})
	s.refreshField()
}

// freeNear is c if the player can stand there, else an open neighbor.
func (s *Sim) freeNear(c skill.Coord) (skill.Coord, bool) {
	ok := func(h skill.Coord) bool {
		t := s.Floor.At(h)
		return t != dungeon.Wall && t != dungeon.Pit && !s.occupied(h)
	}
	if ok(c) {
		return c, true
	}
	for _, d := range skill.HexDirs {
		if n := c.Add(d); ok(n) {
			return n, true
		}
	}
	return c, false
}

// posOf is where an effect at tile c should draw: the player's true
// position if it's the player's tile, else the tile's center.
func (s *Sim) posOf(c skill.Coord) (float64, float64) {
	if p := s.P(); p.Free && c == p.At {
		return p.X, p.Y
	}
	return c.Pixel()
}

// addFX records an effect with its draw positions filled in.
func (s *Sim) addFX(f FX) {
	f.X, f.Y = s.posOf(f.At)
	f.FromX, f.FromY = s.posOf(f.From)
	s.FX.Add(f)
}

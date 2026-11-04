package fx

import "math"

// Sprite is which small image a particle draws as.
type Sprite uint8

const (
	SpriteGlow   Sprite = iota // soft round dot
	SpriteShard                // thin diamond, drawn along its velocity
	SpriteSquare               // hard square bit
	SpriteStreak               // glow stretched along its velocity
	NumSprites
)

// P is one particle. Positions are in hex units, like the game's.
type P struct {
	X, Y, VX, VY float32
	Age, Life    float32
	Size0, Size1 float32 // radius at birth and at death, hex units
	// R0..B0 is the color at birth, R1..B1 at death; A is peak alpha.
	R0, G0, B0, R1, G1, B1, A float32
	Drag                      float32 // velocity lost per second, as a rate
	Grav                      float32 // hex units per second squared, down is positive
	Swirl                     float32 // radians per second the velocity turns
	Wobble                    float32 // sideways noise, hex units per second
	Flicker                   float32 // 0 steady, 1 strong flicker
	Stretch                   float32 // seconds of travel the streak or shard covers
	Seed                      float32
	Sprite                    Sprite
}

// Max is how many particles can live at once. Spawns past it are dropped,
// and spawns start thinning out well before it (see LOD).
const Max = 1200

// Pool holds every live particle in a fixed array. Nothing allocates.
type Pool struct {
	P   [Max]P
	N   int
	rng uint32
}

// NewPool returns a pool seeded with seed (any value but zero works the
// same; zero is replaced).
func NewPool(seed uint32) *Pool {
	if seed == 0 {
		seed = 0x9e3779b9
	}
	return &Pool{rng: seed}
}

// rand returns a float in [0, 1). xorshift32: fast and good enough for
// sparks.
func (p *Pool) rand() float32 {
	x := p.rng
	x ^= x << 13
	x ^= x >> 17
	x ^= x << 5
	p.rng = x
	return float32(x>>8) / (1 << 24)
}

// rr is a random float in [lo, hi).
func (p *Pool) rr(lo, hi float32) float32 { return lo + (hi-lo)*p.rand() }

// LOD is how much of a requested burst to actually spawn: all of it while
// the pool is under half full, thinning to a fifth as it fills. Busy
// fights stay fast on phones and still read.
func (p *Pool) LOD() float32 {
	load := float32(p.N) / Max
	if load <= 0.5 {
		return 1
	}
	return max(0.2, 1-1.6*(load-0.5))
}

// Count turns a wanted number of particles into what to spawn, after LOD.
func (p *Pool) Count(want float32) int {
	n := want * p.LOD()
	whole := int(n)
	if p.rand() < n-float32(whole) {
		whole++
	}
	return whole
}

// Add spawns a particle. It reports false when the pool is full.
func (p *Pool) Add(q P) bool {
	if p.N == Max || q.Life <= 0 {
		return false
	}
	p.P[p.N] = q
	p.N++
	return true
}

// Update moves every particle forward dt seconds and drops the dead ones.
func (p *Pool) Update(dt float32) {
	if dt <= 0 {
		return
	}
	for i := 0; i < p.N; {
		q := &p.P[i]
		q.Age += dt
		if q.Age >= q.Life {
			p.N--
			p.P[i] = p.P[p.N]
			continue
		}
		damp := 1 - q.Drag*dt
		if damp < 0 {
			damp = 0
		}
		q.VX *= damp
		q.VY *= damp
		q.VY += q.Grav * dt
		if q.Swirl != 0 {
			s, c := sincos(q.Swirl * dt)
			q.VX, q.VY = q.VX*c-q.VY*s, q.VX*s+q.VY*c
		}
		q.X += q.VX * dt
		q.Y += q.VY * dt
		if q.Wobble != 0 {
			ph := q.Age*7 + q.Seed*20
			q.X += q.Wobble * float32(math.Sin(float64(ph))) * dt
			q.Y += q.Wobble * float32(math.Cos(float64(ph*1.3))) * dt
		}
		i++
	}
}

// Clear drops every particle.
func (p *Pool) Clear() { p.N = 0 }

func sincos(a float32) (float32, float32) {
	s, c := math.Sincos(float64(a))
	return float32(s), float32(c)
}

// Look is how a particle draws right now.
type Look struct {
	Size       float32 // radius, hex units
	R, G, B, A float32 // premultiply by A yourself
	// Len is the streak or shard length in hex units (0 for dots), along
	// the unit vector DX, DY.
	Len, DX, DY float32
}

// DrawScale multiplies every particle's radius when drawn. Sizes in P are
// the bright core; the soft sprite's glow needs about twice that to read.
const DrawScale = 2.2

// Look works out a particle's size, color and alpha at its age.
func (q *P) Look() Look {
	k := q.Age / q.Life
	e := 1 - (1-k)*(1-k) // ease out
	l := Look{Size: (q.Size0 + (q.Size1-q.Size0)*e) * DrawScale}
	l.R = q.R0 + (q.R1-q.R0)*k
	l.G = q.G0 + (q.G1-q.G0)*k
	l.B = q.B0 + (q.B1-q.B0)*k
	// Pop in over the first 8% of life, fade out as k squared.
	in := min(1, k/0.08)
	a := q.A * in * (1 - k*k)
	if q.Flicker > 0 {
		f := 0.5 + 0.5*float32(math.Sin(float64(q.Age*38+q.Seed*50)))
		a *= 1 - q.Flicker*0.6*f
	}
	l.A = clamp01(a)
	if q.Stretch > 0 {
		sp := float32(math.Hypot(float64(q.VX), float64(q.VY)))
		if sp > 1e-4 {
			l.DX, l.DY = q.VX/sp, q.VY/sp
			l.Len = sp * q.Stretch
		}
	}
	return l
}

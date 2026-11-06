// Package fx is the procedural animation behind skill effects. Nothing in
// it is canned: every effect is worked out from numbers the game already
// has (a damage type's JSON row, how hard the hit landed, what the hex
// cost in Power and Heat, the shape of the area it touched). Adding a
// damage type to the JSON gives it its own look with no code.
//
// The package is plain Go with no renderer. The game's renderer spawns
// particles here and draws them as batched sprites.
package fx

import "hexes/skill"

// Sig is a damage type's motion signature. Each field is 0 to 1 and says
// how much of the effect behaves that way; a type can mix several.
type Sig struct {
	R, G, B float32 // base color
	Hot     float32 // rising, flickering embers that cool from white to the base color
	Cold    float32 // shards that snap out, stall and fall, with a glint
	Force   float32 // fast streaks and a hard shockwave
	Swarm   float32 // small motes that orbit and drift and linger
	Splash  float32 // a second spray off the target
	Mark    float32 // square tracer bits and lock-on brackets
	Veil    float32 // faint, slow, growing shimmer
}

func clamp01(x float32) float32 { return min(max(x, 0), 1) }

// SigOf works a signature out of a damage type's numbers. col is the
// type's base color as 0 to 1 floats.
func SigOf(t skill.DamageType, r, g, b float32) Sig {
	s := Sig{R: r, G: g, B: b}
	s.Hot = clamp01(t.HeatOffset / 3)
	s.Cold = clamp01(-t.HeatOffset/3 + t.Slow + boolf(t.FreezeStacks > 0)*0.3)
	s.Force = clamp01(t.PushPerHit*0.8 + t.WallImpactFrac*2)
	s.Swarm = clamp01(float32(t.Ticks) / 8)
	s.Splash = clamp01(t.SpreadFrac * 2)
	s.Mark = clamp01(boolf(t.RaiseOnKill) * 0.9)
	if t.StealthMult > 1 {
		s.Veil = clamp01(t.StealthMult - 1)
	}
	switch t.Status {
	case skill.StatusBurn:
		s.Hot = clamp01(s.Hot + 0.4)
	case skill.StatusChill:
		s.Cold = clamp01(s.Cold + 0.3)
	case skill.StatusStagger:
		s.Force = clamp01(s.Force + 0.3)
	case skill.StatusToxin:
		s.Swarm = clamp01(s.Swarm + 0.3)
	case skill.StatusMark:
		s.Mark = clamp01(s.Mark + 0.3)
	case skill.StatusCloak:
		s.Veil = clamp01(s.Veil + 0.5)
	}
	return s
}

func boolf(b bool) float32 {
	if b {
		return 1
	}
	return 0
}

// Terrain is the signature of a wall slam or a pit: pure force, dusty.
var Terrain = Sig{R: 1, G: 0.88, B: 0.38, Force: 1}

// Neutral is used for effects with no damage type.
var Neutral = Sig{R: 0.94, G: 0.94, B: 0.94}

// Facet is one way a particle can behave.
type Facet uint8

const (
	FacetPlain Facet = iota
	FacetHot
	FacetCold
	FacetForce
	FacetSwarm
	FacetMark
	FacetVeil
	numFacets
)

// weights is how likely each facet is for one particle of this signature.
// Plain keeps a little weight so pure types still have a body.
func (s Sig) weights() [numFacets]float32 {
	return [numFacets]float32{0.15, s.Hot, s.Cold, s.Force, s.Swarm, s.Mark, s.Veil}
}

// Ring style: how a signature draws the edge of an area it touches. The
// renderer reads these instead of switching on type names.
type RingStyle struct {
	Sides  int     // 6 for crystalline hex rings, more for round ones
	Wobble float32 // radial noise as a fraction of the radius
	Dash   float32 // 0 solid, up to 1 for broken, segmented rings
	Echo   bool    // a second, lagging ring (shockwaves)
	Spin   float32 // turns per second the pattern rotates
}

// Ring is the ring style for this signature.
func (s Sig) Ring() RingStyle {
	r := RingStyle{Sides: 40}
	if s.Cold >= 0.5 && s.Cold >= s.Hot {
		r.Sides = 6
	}
	r.Wobble = 0.12 * s.Hot
	r.Dash = clamp01(s.Swarm*0.8 + s.Mark*0.9)
	r.Echo = s.Force >= 0.5
	r.Spin = 0.6*s.Swarm + 0.3*s.Mark
	return r
}

// Jag is how much a bolt or chain arc wanders off its straight line, as a
// fraction of its length: force flies straight, heat writhes.
func (s Sig) Jag() float32 {
	return 0.05 + 0.18*s.Hot + 0.12*s.Plainness() + 0.08*s.Swarm - 0.05*s.Force
}

// Plainness is how much of the signature none of the facets claim.
func (s Sig) Plainness() float32 {
	return clamp01(1 - (s.Hot + s.Cold + s.Force + s.Swarm + s.Mark + s.Veil))
}

// Arch is the one look a signature leads with, so each damage kind reads
// by shape and motion as well as color: plasma burns, cryo shatters into
// crystal, kinetic hits as a hard ring, nanites swarm, drones lock on,
// cloak shimmers.
type Arch uint8

const (
	ArchPlain Arch = iota
	ArchHot
	ArchCold
	ArchForce
	ArchSwarm
	ArchMark
	ArchVeil
	NumArch
)

var archNames = [...]string{"plain", "hot", "cold", "force", "swarm", "mark", "veil"}

func (a Arch) String() string { return archNames[a] }

// Arch is the facet the signature has most of, or Plain when none has
// much.
func (s Sig) Arch() Arch {
	best, v := ArchPlain, float32(0.3)
	for i, w := range [...]float32{s.Hot, s.Cold, s.Force, s.Swarm, s.Mark, s.Veil} {
		if w > v {
			best, v = Arch(i+1), w
		}
	}
	return best
}

package game

import (
	"math"

	"hexes/dungeon"
	"hexes/skill"
)

// Sight is the fog of war: what the player's lamp lights now and what
// they have ever seen on this floor. The lamp reaches Sim.LightRadius
// hexes and walls block it, so the next room stays dark until you look
// in. Seen is the one record of explored ground: the floor draws it dimly
// remembered and the minimap charts it. It is pure data, so it runs in
// tests, and it only recomputes when the player steps onto a new hex or
// the radius changes, so standing still costs nothing.
type Sight struct {
	floor *dungeon.Floor
	// Vis marks hexes in the lamp's line of sight right now.
	Vis []bool
	// Seen marks hexes ever lit on this floor. Never cleared until the
	// floor changes.
	Seen []bool
	// Gen goes up whenever Seen gains a hex, so a chart can redraw only
	// when there's something new.
	Gen int

	at   skill.Coord
	r    float32
	list []int32 // indices set in Vis, to clear them fast
}

// Light radius tuning.
const (
	// BrownOut is the share of its reach the lamp keeps while the rig is
	// locked by an overheat or a vent: losing your light is part of the
	// cost of running hot.
	BrownOut = 0.6
	// MinLight is the shortest the lamp ever reaches, in hexes.
	MinLight = 2
	// RememberIn is how far inside the radius a lit hex must be to be
	// remembered: the faint rim of the light shows ground without
	// charting it.
	RememberIn = 1
)

// LightRadius is how many hexes the player's lamp reaches now.
func (s *Sim) LightRadius() float32 {
	r := s.Rig.Light
	if s.Rig.Locked() {
		r *= BrownOut
	}
	return max(MinLight, r)
}

// refreshSight relights the floor around the player.
func (s *Sim) refreshSight() {
	if s.Floor == nil || int(s.Player) >= len(s.Actors) {
		return
	}
	s.Sight.Update(s.Floor, s.P().At, s.LightRadius())
}

// Update recomputes what the lamp at hex at with reach r lights. A new
// floor wipes the record.
func (v *Sight) Update(f *dungeon.Floor, at skill.Coord, r float32) {
	if v.floor != f {
		v.floor = f
		n := f.W * f.H
		v.Vis = grow(v.Vis, n)
		v.Seen = grow(v.Seen, n)
		v.list = v.list[:0]
		v.Gen++
		v.r = -1
	}
	if at == v.at && r == v.r {
		return
	}
	v.at, v.r = at, r
	for _, i := range v.list {
		v.Vis[i] = false
	}
	v.list = v.list[:0]
	if !f.In(at) {
		return
	}
	reach := int(math.Ceil(float64(r)))
	keep := r - RememberIn
	for dr := -reach; dr <= reach; dr++ {
		for dq := max(-reach, -dr-reach); dq <= min(reach, -dr+reach); dq++ {
			c := skill.Coord{Q: at.Q + dq, R: at.R + dr}
			if !f.In(c) || !clearLine(f, at, c) {
				continue
			}
			i := f.Index(c)
			v.Vis[i] = true
			v.list = append(v.list, int32(i))
			if !v.Seen[i] && float32(skill.Dist(at, c)) <= keep {
				v.Seen[i] = true
				v.Gen++
			}
		}
	}
}

// Lit reports whether hex c is in the lamp's sight now.
func (v *Sight) Lit(f *dungeon.Floor, c skill.Coord) bool {
	return v.floor == f && f.In(c) && v.Vis[f.Index(c)]
}

func grow(b []bool, n int) []bool {
	if cap(b) < n {
		return make([]bool, n)
	}
	b = b[:n]
	clear(b)
	return b
}

// clearLine reports whether light gets from a to b: no wall on the hexes
// between them (b itself may be a wall, so walls get lit). It walks the
// hex line twice, nudged to either side, and passes if either is clear,
// so light slips past a corner the line only grazes.
func clearLine(f *dungeon.Floor, a, b skill.Coord) bool {
	n := skill.Dist(a, b)
	if n <= 1 {
		return true
	}
	return walk(f, a, b, n, 1e-4) || walk(f, a, b, n, -1e-4)
}

func walk(f *dungeon.Floor, a, b skill.Coord, n int, e float64) bool {
	aq, ar := float64(a.Q)+e, float64(a.R)+e
	bq, br := float64(b.Q)+e, float64(b.R)+e
	inv := 1 / float64(n)
	for i := 1; i < n; i++ {
		t := float64(i) * inv
		if f.At(cubeRound(aq+(bq-aq)*t, ar+(br-ar)*t)) == dungeon.Wall {
			return false
		}
	}
	return true
}

func cubeRound(fq, fr float64) skill.Coord {
	fs := -fq - fr
	q, r, s := math.Round(fq), math.Round(fr), math.Round(fs)
	dq, dr, ds := math.Abs(q-fq), math.Abs(r-fr), math.Abs(s-fs)
	switch {
	case dq > dr && dq > ds:
		q = -r - s
	case dr > ds:
		r = -q - s
	}
	return skill.Coord{Q: int(q), R: int(r)}
}

// LightAt is how lit a point is by a lamp at (lx, ly) reaching r hexes,
// from 1 (full) to 0 (dark), in the same pixel space as Coord.Pixel. It
// is full out to half the radius (at least two hexes short of it), then
// falls off smoothly to nothing at the radius, so the edge of the light
// is soft.
func LightAt(lx, ly, x, y float64, r float32) float32 {
	d := float32(math.Hypot(x-lx, y-ly) / math.Sqrt(3)) // in hexes
	if d >= r {
		return 0
	}
	soft := max(2, r*0.5)
	in := r - soft
	if d <= in {
		return 1
	}
	t := (d - in) / soft
	return 1 - t*t*(3-2*t)
}

package game

import (
	"hexes/loot"
	"hexes/skill"
)

// Lenses: loot that makes the rig's lamp reach further. A lens is an
// Optics upgrade, learned for good the moment you walk over it like a
// shape or a logic tile: it adds Optics points, which the soft cap turns
// into light radius (skill.LightFor), so the first few lenses widen your
// light a lot and later ones less. Deeper floors and elites drop them
// more often and drop bigger ones.

// Lens drop tuning. Placeholder balance.
const (
	LensDropChance   = 0.03  // per kill at depth 1
	LensDropPerDepth = 0.002 // added per floor down
	LensDropMax      = 0.08  // per-kill chance never goes past this
	LensDropElite    = 3     // times for an elite
	LensBigChance    = 0.25  // a +2 lens, from depth LensBigDepth
	LensBigDepth     = 5
	MaxLensDrops     = 3 // lying on one floor at once
)

// LensDrop is a lens lying on the floor.
type LensDrop struct {
	At     skill.Coord
	Points int // Optics points it gives
	Age    float32
}

// LensChance is the per-kill chance of a lens at depth.
func LensChance(depth int, elite bool) float32 {
	c := min(LensDropMax, LensDropChance+LensDropPerDepth*float32(max(0, depth-1)))
	if elite {
		c *= LensDropElite
	}
	return c
}

// rollLensDrop maybe drops a lens where a foe died.
func (s *Sim) rollLensDrop(id skill.EntityID, at skill.Coord, elite bool) {
	if len(s.Stash.Lenses) >= MaxLensDrops {
		return
	}
	r := loot.NewRNG(loot.Seed(s.Floor.Seed^hash(uint64(s.Visits))^0x1e75, int(id)))
	if r.Float() >= LensChance(s.Depth, elite) {
		return
	}
	n := 1
	if s.Depth >= LensBigDepth && r.Float() < LensBigChance {
		n = 2
	}
	s.Stash.Lenses = append(s.Stash.Lenses, LensDrop{At: at, Points: n})
}

// pickupLenses fits any lens under the player.
func (s *Sim) pickupLenses(at skill.Coord, dt float32) {
	d := s.Stash.Lenses
	for i := 0; i < len(d); {
		d[i].Age += dt
		if d[i].At != at {
			i++
			continue
		}
		s.AddOptics(d[i].Points)
		s.addFX(FX{Kind: FXLens, At: at, Slot: d[i].Points, Value: s.Rig.Light, Life: 2.2})
		d[i] = d[len(d)-1]
		d = d[:len(d)-1]
	}
	s.Stash.Lenses = d
}

// AddOptics adds Optics points for good and widens the lamp at once.
func (s *Sim) AddOptics(n int) {
	s.Points.Optics = max(0, s.Points.Optics+n)
	s.Rig.Light = skill.LightFor(s.Points)
	s.refreshSight()
	s.Dirty = true
}

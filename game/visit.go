package game

import (
	"math"

	"hexes/dungeon"
	"hexes/skill"
)

// Visits: the test menu can drop the player onto a deep mixed floor to
// see it without playing down to it. A visit is a look, not progress: it
// earns no depth Data, doesn't move Best or grow the grid, skips the
// floor-end draft, and a death there costs nothing. The stairs or a
// death bring you back to the floor you left, at full hull, and a save
// made during the visit is a save at home.

// VisitDepth is where a visit to a floor of n kinds goes.
func VisitDepth(n int) int {
	if n >= 3 {
		return dungeon.TripleDepth
	}
	return dungeon.MixDepth
}

// VisitMixed drops the player onto a floor holding n kinds and reports
// its depth. Each visit rolls a new floor.
func (s *Sim) VisitMixed(n int) int {
	d := VisitDepth(n)
	if s.Visit == 0 {
		s.Visit = s.Depth
	}
	seed := s.floorSeed(d)
	for k := uint64(1); k < 4096; k++ {
		seed = hash(s.floorSeed(d) + k*0x2545f4914f6cdd1d + s.visitSeed)
		if len(dungeon.KindsFor(seed, d)) == n {
			break
		}
	}
	s.visitSeed = seed
	s.CheckpointT = 0
	s.enter(d, 1, seed)
	return d
}

// Visiting reports whether the player is on a visited floor.
func (s *Sim) Visiting() bool { return s.Visit > 0 }

// DevToCrossing moves the player into crossing k (between regions k-1
// and k) of a mixed floor, at a reachable tile nearest its middle, with
// every enemy gone, for the screenshot autopilot. t picks how far across:
// 0.5 is the middle.
func (s *Sim) DevToCrossing(k int, t float64) bool {
	f := s.Floor
	if k < 1 || k >= len(f.Regions) {
		return false
	}
	for i := range s.Actors {
		if a := &s.Actors[i]; skill.EntityID(i) != s.Player {
			a.U.Alive, a.U.HP = false, 0
		}
	}
	best, bestD := f.Start, 1e9
	for i, tile := range f.Tiles {
		if !tile.Walkable() || s.dist[i] <= 0 {
			continue
		}
		c := f.CoordOf(i)
		a, b, tt := f.Mix(c)
		if a != k-1 || b != k {
			continue
		}
		d := math.Abs(tt-t)*40 + math.Abs(float64(c.R-f.H/2))*0.2
		if d < bestD {
			best, bestD = c, d
		}
	}
	s.P().placeFree(best)
	s.refreshSight()
	return bestD < 1e9
}

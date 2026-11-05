package game

import "hexes/skill"

// Sustain parts in the sim (see skill.EffectMend, EffectPurge and
// EffectShield). They drop like any effect part, uncommon or rare, so no
// build can count on one. None of the preset hexes has one: the first
// waits by the stairs.

// SustainFirstDepth is where the first sustain part, a Mend, waits by
// the stairs until it's picked up.
const SustainFirstDepth = 1

// FirstSustain is the part that waits by the stairs.
func FirstSustain() skill.Part {
	return skill.NewMend(skill.DefaultMend).WithRarity(skill.Uncommon)
}

// placeFirstSustain lays a Mend next to where you arrive the first time a
// save reaches SustainFirstDepth without having picked up a sustain part.
// It comes back every visit until it's picked up, and never sits on the
// tile the first shape or frame or a workbench took.
func (s *Sim) placeFirstSustain() {
	if s.SustainGiven || s.Depth < SustainFirstDepth || len(s.Stash.Drops) == cap(s.Stash.Drops) {
		return
	}
	at, ok := s.Floor.Start, false
	for k := range skill.HexDirs {
		n := s.Floor.Start.Add(skill.HexDirs[(k+2)%len(skill.HexDirs)])
		if s.Floor.At(n).Walkable() && !s.takenBySpecial(n) {
			at, ok = n, true
			break
		}
	}
	if !ok {
		return
	}
	s.Stash.Drops = append(s.Stash.Drops, Drop{At: at, Part: FirstSustain()})
}

// takenBySpecial reports whether a shape, frame or workbench is at c.
func (s *Sim) takenBySpecial(c skill.Coord) bool {
	for _, b := range s.Benches {
		if b == c {
			return true
		}
	}
	for _, d := range s.Stash.Shapes {
		if d.At == c {
			return true
		}
	}
	for _, d := range s.Stash.Frames {
		if d.At == c {
			return true
		}
	}
	return false
}

// IsSustain reports whether p is a Mend, Purge or Shield.
func IsSustain(p skill.Part) bool { return p.Kind == skill.KindEffect && p.Effect.Sustain() }

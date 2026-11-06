package game

import "hexes/skill"

// Sustain parts in the sim (see skill.EffectMend, EffectPurge and
// EffectShield). They drop like any effect part, uncommon or rare, once
// the teaching ladder has taught them, so no build can count on one.
// None of the preset hexes has one: the first is the ladder's depth 5
// pickup (teach.go).

// FirstSustain is the Mend the ladder hands over.
func FirstSustain() skill.Part {
	return skill.NewMend(skill.DefaultMend).WithRarity(skill.Uncommon)
}

// takenBySpecial reports whether a shape, frame, workbench or repair pad
// is at c.
func (s *Sim) takenBySpecial(c skill.Coord) bool {
	if s.PadAt(c) >= 0 {
		return true
	}
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

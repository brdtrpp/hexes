package game

import "hexes/skill"

// A form is one area a hex picked targets in during a firing: a cone, a
// ring round a point, a chain of hops, or a nearest pick. The sim notes
// each pick, marks the ones a hit lands in, and when the firing ends
// turns the landed ones into effects. The renderer then animates the
// area itself instead of a line per foe.
type form struct {
	kind       skill.TargetKind
	caster     skill.EntityID
	x, y       float64 // origin, hex units
	dx, dy     float64 // cone direction
	radius     float32 // hexes
	ids        [maxFormIDs]skill.EntityID
	n          int
	landed     bool
	dealt, mag float32
	typ        skill.TypeID
}

const (
	maxForms   = 8
	maxFormIDs = 12
)

// magOf is how hard a hit landed against the foe's max health, from about
// 0.15 for a scratch to 3 for a one-shot (the same scale as fx.MagOf).
func magOf(dealt, maxHP float32) float32 {
	if maxHP <= 0 {
		maxHP = 60
	}
	return min(max(dealt/maxHP*4, 0.15), 3)
}

// gradeOf is an effect's Grade from the Power its hex cost and how hard
// it landed: a cheap tap is 0, a big build hitting hard is 3.
func gradeOf(power, mag float32) uint8 {
	g := power/9 + mag*0.9
	switch {
	case g >= 3:
		return 3
	case g >= 2:
		return 2
	case g >= 1:
		return 1
	}
	return 0
}

// noteForm records a pick. Self picks aren't areas and aren't noted.
func (s *Sim) noteForm(kind skill.TargetKind, origin, caster skill.EntityID, radius float32, dx, dy float64, ids []skill.EntityID) {
	if kind == skill.TargetSelf || origin < 0 || int(origin) >= len(s.Actors) {
		return
	}
	if s.nforms == maxForms {
		copy(s.forms[:], s.forms[1:])
		s.nforms--
	}
	f := &s.forms[s.nforms]
	s.nforms++
	*f = form{kind: kind, caster: caster, radius: radius, dx: dx, dy: dy}
	f.x, f.y = s.Actors[origin].Pos()
	f.n = copy(f.ids[:], ids)
}

// landForm marks the newest form of caster's that holds target as hit,
// and reports whether that form is an area the renderer draws itself
// (so the hit needs no line back to the caster).
func (s *Sim) landForm(caster, target skill.EntityID, dealt, mag float32, t skill.TypeID) bool {
	for i := s.nforms - 1; i >= 0; i-- {
		f := &s.forms[i]
		if f.caster != caster {
			continue
		}
		for _, id := range f.ids[:f.n] {
			if id != target {
				continue
			}
			f.landed = true
			f.dealt += dealt
			f.mag = max(f.mag, mag)
			f.typ = t
			return f.kind != skill.TargetNearest
		}
	}
	return false
}

// flushForms turns owner's landed forms into effects, carrying what the
// hex cost, and forgets them.
func (s *Sim) flushForms(owner skill.EntityID, p *skill.Program, o skill.Outcome) {
	var heat, power float32
	if p != nil && o == skill.Fired {
		heat, power = p.Heat, p.Power
	}
	keep := 0
	for i := 0; i < s.nforms; i++ {
		f := s.forms[i]
		if f.caster != owner {
			s.forms[keep] = f
			keep++
			continue
		}
		if !f.landed {
			continue
		}
		fx := FX{Type: f.typ, Mag: f.mag, Heat: heat, Power: power, Value: f.radius, X: f.x, Y: f.y, Life: 0.55, Grade: gradeOf(power, f.mag)}
		switch f.kind {
		case skill.TargetAdjacent:
			fx.Kind = FXWave
		case skill.TargetCone:
			fx.Kind = FXCone
			fx.DX, fx.DY = float32(f.dx), float32(f.dy)
		case skill.TargetLine, skill.TargetBoth:
			fx.Kind = FXBeam
			fx.DX, fx.DY = float32(f.dx), float32(f.dy)
			if f.kind == skill.TargetBoth {
				fx.Slot = 1
			}
		case skill.TargetChain:
			fx.Kind, fx.Life = FXChain, 0.5
			fx.Pts[0] = [2]float32{float32(f.x), float32(f.y)}
			n := 1
			for _, id := range f.ids[:f.n] {
				if n == MaxChainPts {
					break
				}
				x, y := s.Actors[id].Pos()
				fx.Pts[n] = [2]float32{float32(x), float32(y)}
				n++
			}
			fx.NPts = uint8(n)
		default:
			continue // nearest picks draw as bolts per hit
		}
		s.FX.Add(fx)
	}
	s.nforms = keep
}

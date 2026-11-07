package game

import (
	"hexes/dungeon"
	"hexes/skill"
)

// The Necromancer's army. A foe your drone damage finishes gets up and
// fights for you, and so does anything your raised bodies kill, until
// you keep skill.MinionMax; past that, bodies stay down as wrecks for
// your other necro parts to spend. A raised body stays until it's
// killed: it follows you down the stairs and comes back with a reload,
// and only your own death scatters it.

// Raised is one body of the army as it's carried between floors.
type Raised struct {
	Kind  Kind
	Level int
	// Hull is its max hull as a share of a fresh foe of its kind and
	// level; HP is how much of that it has left.
	Hull, HP float32
}

// necroType is the Necromancer school's damage type.
func (s *Sim) necroType() skill.TypeID { return SchoolType(s.Reg, SchoolNecro) }

// minions counts the raised bodies fighting for owner.
func (s *Sim) minions(owner skill.EntityID) int {
	n := 0
	for i := range s.Actors {
		if a := &s.Actors[i]; a.Minion && a.U.Alive && a.Owner == owner {
			n++
		}
	}
	return n
}

// rise raises body id for you as it falls, while your army has room.
// It never pushes an older body out: a full army leaves the wreck.
func (s *Sim) rise(id skill.EntityID) bool {
	p := s.P()
	if !p.U.Alive || s.minions(s.Player) >= skill.MinionMax {
		return false
	}
	return s.Reanimate(id, s.Player, skill.ReanimateBase)
}

// armyNow is the army standing right now, to carry or save.
func (s *Sim) armyNow() []Raised {
	var out []Raised
	for i := range s.Actors {
		a := &s.Actors[i]
		if !a.Minion || !a.U.Alive || a.Owner != s.Player || a.Kind < KindGrunt || a.Kind >= KindDevice {
			continue
		}
		full := enemyTable[a.Kind].hp * scale(a.U.Level)
		if full <= 0 || a.U.MaxHP <= 0 {
			continue
		}
		out = append(out, Raised{Kind: a.Kind, Level: a.U.Level, Hull: a.U.MaxHP / full, HP: a.U.HP / a.U.MaxHP})
		if len(out) == skill.MinionMax {
			break
		}
	}
	return out
}

// marchIn sets the carried army down around you at the start of a floor.
// Bodies keep up with you: each comes in at least a level under yours.
func (s *Sim) marchIn() {
	army := s.Army
	s.Army = nil
	if len(army) == 0 {
		return
	}
	p := s.P()
	spots := s.openAround(p.At, len(army))
	for k, r := range army {
		if k >= len(spots) {
			break
		}
		if r.Kind < KindGrunt || r.Kind >= KindDevice {
			continue
		}
		lvl := max(r.Level, s.Depth-1, 1)
		hull := min(max(r.Hull, 0.2), 1)
		hp := enemyTable[r.Kind].hp * scale(lvl) * hull
		id := s.spawn(r.Kind, p.U.Team, spots[k], lvl, hp)
		a := &s.Actors[id]
		s.occ[s.Floor.Index(a.At)] = skill.NoEntity // raised bodies walk free
		a.U.HP = hp * min(max(r.HP, 0.1), 1)
		a.Minion, a.Owner, a.Keeps, a.Seq, a.Life = true, s.Player, true, s.nextSeq(), 1
		a.Free, a.Awake = true, true
		a.X, a.Y = a.At.Pixel()
		a.Clock = MinionEvery / 2
		s.addFX(FX{Kind: FXRaise, At: a.At, Slot: int(a.Kind), Life: 0.7})
	}
}

// openAround is up to n open ground tiles near c, nearest first.
func (s *Sim) openAround(c skill.Coord, n int) []skill.Coord {
	var out []skill.Coord
	seen := map[skill.Coord]bool{c: true}
	q := []skill.Coord{c}
	for len(q) > 0 && len(out) < n {
		at := q[0]
		q = q[1:]
		for _, d := range skill.HexDirs {
			t := at.Add(d)
			if seen[t] || !s.Floor.In(t) || skill.Dist(c, t) > 4 || !s.Floor.At(t).Walkable() {
				continue
			}
			seen[t] = true
			q = append(q, t)
			if s.Floor.At(t) == dungeon.Ground && !s.occupied(t) && len(out) < n {
				out = append(out, t)
			}
		}
	}
	return out
}

// nextSeq numbers devices and minions in the order they came, so the
// oldest makes room first.
func (s *Sim) nextSeq() uint32 {
	s.seq++
	return s.seq
}

// DevArmy is for screenshots: depth d with a full army standing around
// you and a Brace half full.
func (s *Sim) DevArmy(d int) {
	s.Army = nil
	for k := 0; k < skill.MinionMax; k++ {
		s.Army = append(s.Army, Raised{Kind: KindGrunt + Kind(k%int(KindDevice-KindGrunt)), Level: d, Hull: skill.MinionHull, HP: 1})
	}
	s.load(d, 1)
	s.Brace = 0.5 * BraceFull * s.P().U.MaxHP
}

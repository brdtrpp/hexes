package game

import (
	"math"

	"hexes/dungeon"
	"hexes/skill"
)

// The Sim is the skill.World hexes run against. None of these methods
// allocate.

// Unit implements skill.World.
func (s *Sim) Unit(id skill.EntityID) *skill.Unit {
	if id < 0 || int(id) >= len(s.Actors) {
		return nil
	}
	return &s.Actors[id].U
}

func (s *Sim) foe(caster skill.EntityID, i int) bool {
	u := &s.Actors[i].U
	return u.Alive && u.Team != s.Actors[caster].U.Team
}

// Select implements skill.World.
func (s *Sim) Select(kind skill.TargetKind, origin, caster skill.EntityID, radius float32, out []skill.EntityID) int {
	if len(out) == 0 {
		return 0
	}
	if kind == skill.TargetSelf {
		out[0] = caster
		return 1
	}
	o := s.Actors[origin].At
	switch kind {
	case skill.TargetNearest:
		return s.nearest(o, origin, caster, NearestRange, out, nil)
	case skill.TargetAdjacent:
		n := 0
		for i := range s.Actors {
			if n == len(out) {
				break
			}
			if skill.EntityID(i) == origin || !s.foe(caster, i) {
				continue
			}
			if float32(skill.Dist(s.Actors[i].At, o)) <= radius {
				out[n] = skill.EntityID(i)
				n++
			}
		}
		return n
	case skill.TargetCone:
		var dx, dy float64
		if origin == caster {
			dx, dy = s.Actors[caster].FaceVec()
		} else {
			ox, oy := o.Pixel()
			cx, cy := s.Actors[caster].At.Pixel()
			dx, dy = ox-cx, oy-cy
		}
		dl := math.Hypot(dx, dy)
		ox, oy := o.Pixel()
		n := 0
		for i := range s.Actors {
			if n == len(out) {
				break
			}
			if skill.EntityID(i) == origin || !s.foe(caster, i) {
				continue
			}
			d := skill.Dist(s.Actors[i].At, o)
			if d == 0 || float32(d) > radius {
				continue
			}
			px, py := s.Actors[i].At.Pixel()
			vx, vy := px-ox, py-oy
			if (vx*dx+vy*dy)/(math.Hypot(vx, vy)*dl) >= coneCos {
				out[n] = skill.EntityID(i)
				n++
			}
		}
		return n
	case skill.TargetChain:
		cur := o
		n := 0
		for n < len(out) {
			got := s.nearest(cur, origin, caster, int(radius), out[n:n+1], out[:n])
			if got == 0 {
				break
			}
			cur = s.Actors[out[n]].At
			n++
		}
		return n
	}
	return 0
}

var coneCos = math.Cos(35 * math.Pi / 180)

func (s *Sim) nearest(c skill.Coord, origin, caster skill.EntityID, maxDist int, out, skip []skill.EntityID) int {
	var dist [skill.MaxTargets]int
	n := 0
next:
	for i := range s.Actors {
		if skill.EntityID(i) == origin || !s.foe(caster, i) {
			continue
		}
		for _, k := range skip {
			if k == skill.EntityID(i) {
				continue next
			}
		}
		d := skill.Dist(s.Actors[i].At, c)
		if d > maxDist {
			continue
		}
		j := n
		if j == len(out) {
			if d >= dist[j-1] {
				continue
			}
			j--
		} else {
			n++
		}
		for j > 0 && dist[j-1] > d {
			dist[j], out[j] = dist[j-1], out[j-1]
			j--
		}
		dist[j], out[j] = d, skill.EntityID(i)
	}
	return n
}

// Push implements skill.World: shove id away from `from`, one tile at a
// time, stopping at walls and bodies. A pit swallows it.
func (s *Sim) Push(id, from skill.EntityID, hexes int) skill.Terrain {
	a := &s.Actors[id]
	f := &s.Actors[from]
	dir := uint8(f.Facing)
	if a.At != f.At {
		dir = dirTo(f.At, a.At)
	}
	start := a.At
	result := skill.TerrainFloor
	for i := 0; i < hexes; i++ {
		next := a.At.Add(skill.HexDirs[dir])
		t := s.Floor.At(next)
		if t == dungeon.Wall {
			result = skill.TerrainWall
			break
		}
		if s.occupied(next) {
			break
		}
		s.occ[s.Floor.Index(a.At)] = skill.NoEntity
		s.occ[s.Floor.Index(next)] = id
		a.At = next
		if t == dungeon.Pit {
			result = skill.TerrainPit
			a.Fell = true
			s.occ[s.Floor.Index(next)] = skill.NoEntity
			break
		}
	}
	if a.At != start {
		// Snap the slide: start from the old tile and move fast.
		a.From, a.MoveT, a.MoveDur = start, 0, 0.12*float32(skill.Dist(start, a.At))
		if a.Free {
			a.X, a.Y = a.At.Pixel()
		}
		s.addFX(FX{Kind: FXPush, From: start, At: a.At, Life: 0.35})
		if id == s.Player {
			s.refreshField()
		}
	}
	switch result {
	case skill.TerrainWall:
		s.addFX(FX{Kind: FXSlam, At: a.At, From: a.At.Add(skill.HexDirs[dir]), Life: 0.4})
	case skill.TerrainPit:
		s.addFX(FX{Kind: FXFall, At: a.At, Life: 0.7})
	}
	return result
}

// Raise implements skill.World. Minions aren't in the slice yet.
func (s *Sim) Raise(id, owner skill.EntityID) bool { return false }

// Behind implements skill.Senses.
func (s *Sim) Behind(attacker, target skill.EntityID) bool {
	if attacker < 0 || target < 0 || int(attacker) >= len(s.Actors) || int(target) >= len(s.Actors) {
		return false
	}
	t, a := &s.Actors[target], &s.Actors[attacker]
	if !t.Free && !a.Free {
		return skill.BehindOf(t.At, skill.HexDirs[t.Facing], a.At)
	}
	// The player has a true position and facing: use them.
	tx, ty := t.At.Pixel()
	if t.Free {
		tx, ty = t.X, t.Y
	}
	ax, ay := a.At.Pixel()
	if a.Free {
		ax, ay = a.X, a.Y
	}
	fx, fy := t.FaceVec()
	vx, vy := ax-tx, ay-ty
	l := math.Hypot(vx, vy)
	return l > 1e-9 && (vx*fx+vy*fy)/l < -0.5
}

// Hit implements skill.Listener: damage numbers and bolts.
func (s *Sim) Hit(caster, target skill.EntityID, dealt float32, t skill.TypeID, terrain bool) {
	a := &s.Actors[target]
	a.Flash = 0.15
	a.Awake = true
	if dealt > 0 || terrain {
		s.addFX(FX{Kind: FXNumber, At: a.At, Value: dealt, Type: t, Terrain: terrain,
			Hurt: target == s.Player, Life: 0.8})
	}
	if !terrain && caster != target {
		s.addFX(FX{Kind: FXBolt, From: s.Actors[caster].At, At: a.At, Type: t, Life: 0.25})
	}
}

// Killed implements skill.Listener.
func (s *Sim) Killed(caster, target skill.EntityID) {
	s.died(target)
	s.addFX(FX{Kind: FXKill, At: s.Actors[target].At, Life: 0.5})
}

// Fired implements skill.Listener: flash the slot on the HUD and ring the
// caster.
func (s *Sim) Fired(owner skill.EntityID, slot int, p *skill.Program, o skill.Outcome, res skill.Result) {
	if owner != s.Player || slot >= MaxSlots {
		return
	}
	// A proc on cooldown is normal, not news; only flash refusals the
	// player can act on.
	if o == skill.OnCooldown && p.Trigger != skill.OnCast {
		return
	}
	s.SlotFlash[slot], s.SlotLast[slot] = 0.4, o
	if o == skill.Fired {
		s.addFX(FX{Kind: FXRing, At: s.Actors[owner].At, Slot: slot, Life: 0.4})
	}
}

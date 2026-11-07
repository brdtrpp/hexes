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

// Distance implements skill.Distancer, for the distance sensor.
func (s *Sim) Distance(a, b skill.EntityID) int {
	if a < 0 || b < 0 || int(a) >= len(s.Actors) || int(b) >= len(s.Actors) {
		return 0
	}
	return skill.Dist(s.Actors[a].At, s.Actors[b].At)
}

// foe reports whether actor i is a target for caster's hexes: alive, on
// the other side, and, for the player and their minions, in sight. Every
// targeting part goes through here, so the player's hexes never pick a
// foe hidden in the dark (see Sees).
func (s *Sim) foe(caster skill.EntityID, i int) bool {
	u := &s.Actors[i].U
	if !u.Alive || u.Team == s.Actors[caster].U.Team {
		return false
	}
	return s.Actors[caster].U.Team != s.Actors[s.Player].U.Team || s.Sees(i)
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
		n := s.nearest(o, origin, caster, NearestRange, out, nil)
		s.noteForm(kind, origin, caster, 0, 0, 0, out[:n])
		return n
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
		s.noteForm(kind, origin, caster, radius, 0, 0, out[:n])
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
		if dl > 1e-9 {
			s.noteForm(kind, origin, caster, radius, dx/dl, dy/dl, out[:n])
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
		s.noteForm(kind, origin, caster, radius, 0, 0, out[:n])
		return n
	}
	return s.selectMore(kind, origin, caster, radius, out)
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
			if i > 0 {
				// A slam needs the shove to carry it: a foe already pinned
				// to the wall isn't slammed again by every hit.
				result = skill.TerrainWall
			}
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

// Raise implements skill.World: a body gets up for owner (devices.go).
func (s *Sim) Raise(id, owner skill.EntityID) bool {
	return s.Reanimate(id, owner, skill.ReanimateBase)
}

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
	if target == s.Player {
		a.Awake = true
	} else {
		s.wake(target, true) // a hit foe calls its pack
	}
	mag := magOf(dealt, a.U.MaxHP)
	if !terrain {
		a.LastHit = t
		if caster == s.Player && dealt > 0 && t == s.kineticType() {
			s.pull(target) // the Warden drags them in (warden.go)
		}
	}
	if dealt > 0 || terrain {
		s.addFX(FX{Kind: FXNumber, At: a.At, Value: dealt, Type: t, Terrain: terrain,
			Hurt: target == s.Player, Mag: mag, Life: 0.8})
	}
	if !terrain && caster != target && dealt > 0 {
		// Any hit you or your minions land gives you away.
		who := &s.Actors[caster]
		if who.Minion && int(who.Owner) < len(s.Actors) {
			who = &s.Actors[who.Owner]
		}
		if who.U.Team != a.U.Team {
			who.U.Reveal()
		}
	}
	if !terrain && caster != target {
		area := s.landForm(caster, target, dealt, mag, t)
		from := s.Actors[caster].At
		if s.emitting {
			from = s.Actors[s.emitter].At // a device's shot starts from it
		}
		if s.Actors[caster].U.Ambush && caster == s.Player {
			s.addFX(FX{Kind: FXAmbush, At: a.At, From: from, Value: dealt, Life: 0.5})
		}
		s.addFX(FX{Kind: FXBolt, From: from, At: a.At, Type: t, Mag: mag, Impact: area, Life: 0.3})
	}
}

// Statused implements skill.StatusObserver: a status landing marks the
// area it landed in, so status-only hexes still show their reach.
func (s *Sim) Statused(caster, target skill.EntityID, t skill.TypeID) {
	if caster != target {
		s.landForm(caster, target, 0, 0.3, t)
	}
}

// Sustained implements skill.SustainObserver: a Mend, Shield or Purge
// landing shows on whoever it landed on.
func (s *Sim) Sustained(caster, target skill.EntityID, kind skill.EffectKind, amount float32) {
	if target < 0 || int(target) >= len(s.Actors) {
		return
	}
	a := &s.Actors[target]
	mag := magOf(amount, a.U.MaxHP)
	switch kind {
	case skill.EffectMend:
		s.addFX(FX{Kind: FXMend, At: a.At, Value: amount, Mag: mag, Life: 0.9})
	case skill.EffectShield:
		s.addFX(FX{Kind: FXShield, At: a.At, Value: amount, Mag: mag, Life: 0.7})
	case skill.EffectPurge:
		s.addFX(FX{Kind: FXPurge, At: a.At, Value: amount, Life: 0.8})
	case skill.EffectTaunt:
		s.addFX(FX{Kind: FXTaunt, At: a.At, From: s.Actors[caster].At, Life: 0.6})
		a.Awake = true
	case skill.EffectThorns, skill.EffectHaste:
		s.addFX(FX{Kind: FXBuff, At: a.At, Slot: int(kind), Life: 0.7})
	}
}

// Killed implements skill.Listener.
func (s *Sim) Killed(caster, target skill.EntityID) {
	s.died(target)
	a := &s.Actors[target]
	s.addFX(FX{Kind: FXKill, At: a.At, Type: a.LastHit, Slot: int(a.Kind), Life: 0.5})
}

// Fired implements skill.Listener: flash the slot on the HUD and ring the
// caster.
func (s *Sim) Fired(owner skill.EntityID, slot int, p *skill.Program, o skill.Outcome, res skill.Result) {
	s.flushForms(owner, p, o)
	if p != nil && o == skill.Fired {
		// Grade the bolts this firing just threw, newest first, back to
		// the first effect from before this frame.
		from := s.Actors[owner].At
		s.FX.Back(func(f *FX) bool {
			if f.Age > 0 {
				return false
			}
			if f.Kind == FXBolt && f.From == from && f.Grade == 0 {
				f.Grade = gradeOf(p.Power, f.Mag)
				f.Power = p.Power
			}
			return true
		})
	}
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
		s.addFX(FX{Kind: FXRing, At: s.Actors[owner].At, Slot: slot, Power: p.Power, Heat: p.Heat, Life: 0.4, Grade: gradeOf(p.Power, 0.5)})
	}
	// Hexes it called flash as if they fired too.
	for i := 0; i < MaxSlots; i++ {
		if res.Called&(1<<i) != 0 && i != slot {
			s.SlotFlash[i], s.SlotLast[i] = 0.4, skill.Fired
			s.addFX(FX{Kind: FXRing, At: s.Actors[owner].At, Slot: i, Life: 0.4})
		}
	}
}

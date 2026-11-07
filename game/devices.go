package game

import (
	"math"

	"hexes/dungeon"
	"hexes/skill"
)

// Wrecks, devices and minions: what the class parts leave on the floor.
//
// A wreck is a foe's body for WreckLife seconds after it falls: forms,
// reanimate and wreck costs use them up. A device is a placed form (a
// zone, turret, trap or drone) standing on the floor running the hex that
// placed it. A minion is a wreck raised to fight for you for a while.
//
// Devices and minions live in the Actors slice on your team, never on a
// tile, so the player's targeting and the enemies' chasing pass them by.
// load keeps MaxExtra spare slots so placing one never allocates, and a
// spent device's slot is reused.

// Tuning, placeholder like the rest.
const (
	MaxExtra      = 24   // actor slots kept free on each floor for devices
	MaxDevices    = 8    // devices standing at once
	WreckLife     = 25   // seconds a body lies as a wreck
	MinionSpeed   = 3.4  // tiles per second
	MinionEvery   = 0.9  // seconds between a minion's blows
	MinionHit     = 0.7  // a minion's blow, share of its kind's
	MinionReach   = 1.2  // hexes, centre to centre
	DroneSpeed    = 6.0  // tiles per second
	DriftSpeed    = 1.1  // a drifting zone, tiles per second
	DodgeRadius   = 1.1  // a shot passing this close (hex units) is a dodge
	HideTime      = 2.0  // seconds unseen after an exotic dash back
	TrapStun      = 1.0  // an exotic trap's stun
	ZoneChill     = 0.3  // an exotic zone's slow
	lineWidth     = 0.95 // hex units either side of a line's centre
	deviceFirstIn = 0.3  // seconds before a new device first runs
)

// wreck reports whether actor i is a wreck: a foe's body, fresh, whole
// (not in a pit) and not used up.
func (s *Sim) wreck(i int) bool {
	a := &s.Actors[i]
	return !a.U.Alive && a.DeadFor > 0 && a.DeadFor < WreckLife && !a.Fell && !a.Used &&
		!a.Minion && a.Kind != KindPlayer && a.Kind != KindDevice
}

// ally reports whether actor i fights beside caster: the caster, its
// minions and its devices.
func (s *Sim) ally(caster skill.EntityID, i int) bool {
	a := &s.Actors[i]
	if !a.U.Alive || a.U.Team != s.Actors[caster].U.Team {
		return false
	}
	return skill.EntityID(i) == caster || a.Minion || a.Kind == KindDevice
}

// extra reports whether actor i is a device or a minion.
func (a *Actor) extra() bool { return a.Minion || a.Kind == KindDevice }

// distF is the distance between two actors' true positions in hexes.
func (s *Sim) distF(a, b *Actor) float64 {
	ax, ay := a.Pos()
	bx, by := b.Pos()
	return math.Hypot(ax-bx, ay-by) / tileSpan
}

// selectMore is Select for the targets the class parts brought.
func (s *Sim) selectMore(kind skill.TargetKind, origin, caster skill.EntityID, radius float32, out []skill.EntityID) int {
	o := &s.Actors[origin]
	switch kind {
	case skill.TargetWreck:
		if radius <= 0 {
			radius = skill.WreckReach
		}
		return s.closest(out, func(i int) (float64, bool) {
			if skill.EntityID(i) == origin || !s.wreck(i) {
				return 0, false
			}
			d := s.distF(o, &s.Actors[i])
			return d, d <= float64(radius)+0.5
		})
	case skill.TargetWeakest:
		reach := float64(NearestRange)
		if radius > 0 {
			reach = float64(radius) + 0.5 // a radius multiplier reaches further
		}
		n := s.closest(out, func(i int) (float64, bool) {
			if !s.foe(caster, i) || s.distF(o, &s.Actors[i]) > reach {
				return 0, false
			}
			u := &s.Actors[i].U
			return float64(u.HP / max(1, u.MaxHP)), true
		})
		s.noteForm(skill.TargetNearest, origin, caster, 0, 0, 0, out[:n])
		return n
	case skill.TargetMarked:
		if radius <= 0 {
			radius = NearestRange
		}
		n := s.closest(out, func(i int) (float64, bool) {
			if !s.foe(caster, i) || s.Actors[i].U.MarkLeft <= 0 {
				return 0, false
			}
			d := s.distF(o, &s.Actors[i])
			return d, d <= float64(radius)+0.5
		})
		s.noteForm(skill.TargetNearest, origin, caster, 0, 0, 0, out[:n])
		return n
	case skill.TargetLine, skill.TargetBoth:
		dx, dy := s.aim(origin, caster)
		ox, oy := o.Pos()
		reach := float64(radius) * tileSpan
		n := 0
		for i := range s.Actors {
			if n == len(out) {
				break
			}
			if skill.EntityID(i) == origin || !s.foe(caster, i) {
				continue
			}
			px, py := s.Actors[i].Pos()
			vx, vy := px-ox, py-oy
			t := vx*dx + vy*dy
			if kind == skill.TargetBoth {
				t = math.Abs(t)
			}
			if t <= 0 || t > reach+0.5 || math.Abs(vx*dy-vy*dx) > lineWidth {
				continue
			}
			out[n] = skill.EntityID(i)
			n++
		}
		both := 0.0
		if kind == skill.TargetBoth {
			both = 1
		}
		s.noteLine(origin, caster, radius, dx, dy, both, out[:n])
		return n
	case skill.TargetAllies:
		if radius <= 0 {
			radius = skill.AuraRadius
		}
		n := 0
		for i := range s.Actors {
			if n == len(out) {
				break
			}
			if !s.ally(caster, i) || s.Actors[i].Kind == KindDevice && skill.EntityID(i) != origin {
				continue
			}
			if skill.EntityID(i) == caster || s.distF(o, &s.Actors[i]) <= float64(radius)+0.5 {
				out[n] = skill.EntityID(i)
				n++
			}
		}
		return n
	case skill.TargetAura:
		n := 0
		for i := range s.Actors {
			if n == len(out) {
				break
			}
			if !s.foe(caster, i) {
				continue
			}
			f := &s.Actors[i]
			for j := range s.Actors {
				if (skill.EntityID(j) == origin || s.ally(caster, j)) && skill.Dist(f.At, s.Actors[j].At) <= int(radius) {
					out[n] = skill.EntityID(i)
					n++
					break
				}
			}
		}
		s.noteForm(skill.TargetAdjacent, origin, caster, radius, 0, 0, out[:n])
		return n
	}
	return 0
}

// closest fills out with the actors score accepts, lowest score first.
func (s *Sim) closest(out []skill.EntityID, score func(i int) (float64, bool)) int {
	var key [skill.MaxTargets]float64
	n := 0
	for i := range s.Actors {
		d, ok := score(i)
		if !ok {
			continue
		}
		j := n
		if j == len(out) {
			if d >= key[j-1] {
				continue
			}
			j--
		} else {
			n++
		}
		for j > 0 && key[j-1] > d {
			key[j], out[j] = key[j-1], out[j-1]
			j--
		}
		key[j], out[j] = d, skill.EntityID(i)
	}
	return n
}

// aim is the unit direction a line from origin takes: toward the nearest
// foe, else the way the caster faces.
func (s *Sim) aim(origin, caster skill.EntityID) (float64, float64) {
	var near [1]skill.EntityID
	o := &s.Actors[origin]
	if s.nearest(o.At, origin, caster, NearestRange, near[:], nil) == 1 {
		ox, oy := o.Pos()
		px, py := s.Actors[near[0]].Pos()
		if l := math.Hypot(px-ox, py-oy); l > 1e-6 {
			return (px - ox) / l, (py - oy) / l
		}
	}
	return s.Actors[caster].FaceVec()
}

// noteLine notes a line pick for the renderer: a beam along dx, dy.
func (s *Sim) noteLine(origin, caster skill.EntityID, radius float32, dx, dy, both float64, ids []skill.EntityID) {
	s.noteForm(skill.TargetLine, origin, caster, radius, dx, dy, ids)
	if both != 0 && s.nforms > 0 {
		s.forms[s.nforms-1].kind = skill.TargetBoth
	}
}

// --- skill.Actions ---

// Move implements skill.Actions: dashes and blinks for the player.
func (s *Sim) Move(id, at skill.EntityID, hexes float32, mode skill.MoveMode) {
	if id != s.Player {
		return
	}
	p := s.P()
	if !p.U.Alive {
		return
	}
	switch mode {
	case skill.MoveBack, skill.MoveHide:
		s.glide(s.dashFromX, s.dashFromY, 99)
		if mode == skill.MoveHide {
			p.U.Stealth = max(p.U.Stealth, HideTime)
		}
		return
	}
	if at == skill.NoEntity || int(at) >= len(s.Actors) {
		at = s.nearestFoe(p.At, NearestRange)
	}
	dx, dy := p.FaceVec()
	if at != skill.NoEntity {
		tx, ty := s.Actors[at].Pos()
		if l := math.Hypot(tx-p.X, ty-p.Y); l > 1e-6 {
			dx, dy = (tx-p.X)/l, (ty-p.Y)/l
		}
	}
	reach := float64(hexes) * tileSpan
	switch mode {
	case skill.MoveDash:
		s.dashFromX, s.dashFromY = p.X, p.Y
		if at != skill.NoEntity {
			// Through the line, stopping a step short of its far end.
			tx, ty := s.Actors[at].Pos()
			reach = min(reach, math.Hypot(tx-p.X, ty-p.Y)-tileSpan*0.6)
		}
	case skill.MoveToward:
		if at != skill.NoEntity {
			tx, ty := s.Actors[at].Pos()
			reach = min(reach, math.Hypot(tx-p.X, ty-p.Y)-tileSpan*0.9)
		}
	case skill.MoveAway:
		dx, dy = -dx, -dy
	}
	if reach <= 0 {
		return
	}
	from := p.At
	s.glide(p.X+dx*reach, p.Y+dy*reach, reach)
	if p.At != from {
		s.addFX(FX{Kind: FXPush, From: from, At: p.At, Life: 0.3})
	}
}

// glide moves the player straight toward x, y, at most reach hex units,
// as far as open floor goes: walls, pits and bodies stop it.
func (s *Sim) glide(x, y, reach float64) {
	p := s.P()
	vx, vy := x-p.X, y-p.Y
	l := math.Hypot(vx, vy)
	if l < 1e-6 {
		return
	}
	l = min(l, reach)
	steps := int(math.Ceil(l / maxSubstep))
	sx, sy := vx/math.Hypot(vx, vy)*l/float64(steps), vy/math.Hypot(vx, vy)*l/float64(steps)
	bx, by := p.X, p.Y
	for i := 1; i <= steps; i++ {
		nx, ny := p.X+sx*float64(i), p.Y+sy*float64(i)
		c := skill.FromPixel(nx, ny)
		if !s.Floor.In(c) {
			break
		}
		t := s.Floor.At(c)
		if t == dungeon.Wall || t == dungeon.Pit || (c != p.At && s.occupied(c)) {
			break
		}
		bx, by = nx, ny
	}
	c := skill.FromPixel(bx, by)
	s.occ[s.Floor.Index(p.At)] = skill.NoEntity
	p.X, p.Y = bx, by
	p.At, p.From = c, c
	s.occ[s.Floor.Index(c)] = s.Player
	s.collide(p)
	if c2 := skill.FromPixel(p.X, p.Y); c2 != c && s.Floor.In(c2) && s.Floor.At(c2).Walkable() && !s.occupied(c2) {
		s.occ[s.Floor.Index(c)] = skill.NoEntity
		p.At, p.From = c2, c2
		s.occ[s.Floor.Index(c2)] = s.Player
	}
	if s.Floor.At(p.At) != dungeon.Pit {
		s.safe = p.At
	}
	s.refreshField()
}

// Consume implements skill.Actions: use wreck id up.
func (s *Sim) Consume(id skill.EntityID) bool {
	if id < 0 || int(id) >= len(s.Actors) || !s.wreck(int(id)) {
		return false
	}
	s.Actors[id].Used = true
	s.addFX(FX{Kind: FXWreck, At: s.Actors[id].At, Slot: int(s.Actors[id].Kind), Life: 0.5})
	return true
}

// Wrecks implements skill.Actions.
func (s *Sim) Wrecks(id skill.EntityID, n int, take bool) bool {
	var got [skill.MaxTargets]skill.EntityID
	n = min(n, len(got))
	k := s.selectMore(skill.TargetWreck, id, id, skill.WreckReach, got[:n])
	if k < n {
		return false
	}
	if take {
		for _, w := range got[:k] {
			s.Consume(w)
		}
	}
	return true
}

// Reanimate implements skill.Actions: wreck id gets up on owner's side
// and stays until it's killed. value is the Reanimate's own number:
// skill.ReanimateBase raises a body at MinionHull of the hull it had
// alive, more or less in step.
func (s *Sim) Reanimate(id, owner skill.EntityID, value float32) bool {
	if id < 0 || int(id) >= len(s.Actors) || !s.wreck(int(id)) {
		return false
	}
	// Past the cap, the oldest minion falls first.
	n, oldest, oldSeq := 0, -1, uint32(math.MaxUint32)
	for i := range s.Actors {
		if a := &s.Actors[i]; a.Minion && a.U.Alive && a.Owner == owner {
			n++
			if a.Seq < oldSeq {
				oldest, oldSeq = i, a.Seq
			}
		}
	}
	if n >= skill.MinionMax && oldest >= 0 {
		s.expire(oldest)
	}
	a := &s.Actors[id]
	hp := a.U.MaxHP * skill.RaisedHull(value)
	a.U = skill.Unit{ID: id, Team: s.Actors[owner].U.Team, Alive: true, HP: hp, MaxHP: hp,
		Level: a.U.Level, MarkOwner: skill.NoEntity}
	a.Minion, a.Owner, a.Life, a.DeadFor = true, owner, 1, 0
	a.Keeps, a.Seq = true, s.nextSeq()
	a.Free, a.Awake = true, true
	a.X, a.Y = a.At.Pixel()
	a.Clock = MinionEvery / 2
	s.addFX(FX{Kind: FXRaise, At: a.At, Slot: int(a.Kind), Life: 0.7})
	return true
}

// Deploy implements skill.Actions: place a device. It runs from devices,
// never inside the firing that placed it.
func (s *Sim) Deploy(d skill.Deployment) bool {
	if d.Caster < 0 || int(d.Caster) >= len(s.Actors) {
		return false
	}
	// Each form keeps a few standing; the oldest makes room.
	limit := 1
	switch d.Form {
	case skill.FormTurret:
		limit = 1 + int(d.Rule)
	case skill.FormDrone:
		limit = 1 + int(min(d.Rule, 1))
	case skill.FormZone:
		limit = 2
	case skill.FormTrap:
		limit = 3
	}
	n, oldest, oldSeq, total := 0, -1, uint32(math.MaxUint32), 0
	for i := range s.Actors {
		a := &s.Actors[i]
		if a.Kind != KindDevice || !a.U.Alive {
			continue
		}
		total++
		if a.Dev == d.Form && a.Owner == d.Caster {
			n++
			if a.Seq < oldSeq {
				oldest, oldSeq = i, a.Seq
			}
		}
	}
	if n >= limit && oldest >= 0 {
		s.retire(oldest)
		total--
	}
	if total >= MaxDevices {
		return false
	}
	slot := -1
	for i := range s.Actors {
		if a := &s.Actors[i]; a.Kind == KindDevice && !a.U.Alive {
			slot = i
			break
		}
	}
	if slot < 0 {
		if len(s.Actors) == cap(s.Actors) {
			return false // no room this floor without allocating
		}
		s.Actors = s.Actors[:len(s.Actors)+1]
		slot = len(s.Actors) - 1
	}
	c := &s.Actors[d.Caster]
	at := c.At
	x, y := c.Pos()
	if d.At >= 0 && int(d.At) < len(s.Actors) {
		at = s.Actors[d.At].At
		x, y = s.Actors[d.At].Pos()
	}
	if d.Form == skill.FormTurret {
		// Beside you, on open floor.
		for k := range skill.HexDirs {
			nb := c.At.Add(skill.HexDirs[(int(c.Facing)+k)%6])
			if s.Floor.In(nb) && s.Floor.At(nb).Walkable() && s.Floor.At(nb) != dungeon.Pit && !s.occupied(nb) {
				at = nb
				break
			}
		}
		x, y = at.Pixel()
	}
	id := skill.EntityID(slot)
	life, every := float32(0), float32(0)
	var arms uint8 = 1
	switch d.Form {
	case skill.FormZone:
		life, every = skill.ZoneLife, skill.ZoneEvery
	case skill.FormTurret:
		life, every = skill.TurretLife, skill.TurretEvery
	case skill.FormTrap:
		life, arms = skill.TrapLife, skill.TrapArms
		if d.Rule >= 1 {
			arms = skill.TrapArmsRare
		}
	case skill.FormDrone:
		life, every = skill.DroneLife, skill.DroneEvery
	}
	s.Actors[slot] = Actor{
		U:    skill.Unit{ID: id, Team: c.U.Team, Alive: true, HP: 1, MaxHP: 1, Level: c.U.Level, MarkOwner: skill.NoEntity},
		Kind: KindDevice, At: at, From: at, MoveT: 1, Free: true, X: x, Y: y, Awake: true,
		Dev: d.Form, Rule: d.Rule, Life: life, Every: every, Clock: deviceFirstIn, Arms: arms,
		Dep: d, Owner: d.Caster, FaceX: c.FaceX, FaceY: c.FaceY,
		Keeps: skill.Standing(d.Form), Seq: s.nextSeq(),
	}
	s.addFX(FX{Kind: FXDeploy, At: at, Slot: int(d.Form), Life: 0.6})
	return true
}

// retire ends a device that ran its course or made room for a newer
// one: an exotic drone bursts once more on the way out.
func (s *Sim) retire(i int) {
	if a := &s.Actors[i]; a.U.Alive && a.Kind == KindDevice && a.Dev == skill.FormDrone && a.Rule >= 2 {
		s.runDevice(i, skill.NoEntity)
	}
	s.expire(i)
}

// expire ends a device or a minion without a death: no kill, no loot.
func (s *Sim) expire(i int) {
	a := &s.Actors[i]
	if !a.U.Alive {
		return
	}
	a.U.Alive, a.U.HP = false, 0
	a.Used, a.DeadFor = true, WreckLife
	s.addFX(FX{Kind: FXExpire, At: a.At, Slot: int(a.Dev), Value: boolF(a.Minion), Life: 0.5})
}

func boolF(b bool) float32 {
	if b {
		return 1
	}
	return 0
}

// extras runs every device and minion for a frame.
func (s *Sim) extras(dt float32) {
	for i := range s.Actors {
		a := &s.Actors[i]
		if !a.U.Alive || !a.extra() {
			continue
		}
		if !a.Keeps {
			a.Life -= dt
		}
		if a.Life <= 0 || int(a.Owner) >= len(s.Actors) || !s.Actors[a.Owner].U.Alive {
			if a.Life <= 0 {
				s.retire(i)
			} else {
				s.expire(i)
			}
			continue
		}
		if a.Minion {
			s.minion(i, dt)
			continue
		}
		s.device(i, dt)
	}
}

// device runs one device for a frame.
func (s *Sim) device(i int, dt float32) {
	a := &s.Actors[i]
	switch a.Dev {
	case skill.FormDrone:
		// Hang off your shoulder, the side away from where you face.
		p := &s.Actors[a.Owner]
		tx, ty := p.X-p.FaceY*1.1-p.FaceX*0.5, p.Y+p.FaceX*1.1-p.FaceY*0.5
		s.slide(a, tx, ty, DroneSpeed*tileSpan*float64(dt), false)
	case skill.FormZone:
		if a.Rule >= 1 {
			if f := s.nearestFoe(a.At, NearestRange); f != skill.NoEntity {
				tx, ty := s.Actors[f].Pos()
				s.slide(a, tx, ty, DriftSpeed*tileSpan*float64(dt), true)
			}
		}
		if a.Rule >= 2 {
			for j := range s.Actors {
				if s.foe(a.Owner, j) && s.distF(a, &s.Actors[j]) <= skill.ZoneRadius+0.5 {
					u := &s.Actors[j].U
					u.SlowLeft, u.SlowFrac = max(u.SlowLeft, 0.3), max(u.SlowFrac, ZoneChill)
				}
			}
		}
	case skill.FormTrap:
		for j := range s.Actors {
			if f := &s.Actors[j]; !f.U.Alive || f.U.Team == a.U.Team || f.extra() {
				continue
			}
			if skill.Dist(s.Actors[j].At, a.At) > skill.TrapRadius {
				continue
			}
			if a.Clock > 0 {
				a.Clock -= dt
				return
			}
			if a.Rule >= 2 {
				s.Actors[j].U.StunLeft = max(s.Actors[j].U.StunLeft, TrapStun)
			}
			s.runDevice(i, skill.EntityID(j))
			a.Arms--
			a.Clock = 0.8 // rearming
			if a.Arms == 0 {
				s.expire(i)
			}
			return
		}
		return
	}
	a.Clock -= dt
	if a.Clock > 0 {
		return
	}
	a.Clock = a.Every
	reach := NearestRange
	if a.Dev == skill.FormZone {
		reach = 3
	}
	if f := s.nearestFoe(a.At, reach); f != skill.NoEntity {
		a.FaceX, a.FaceY = s.unitTo(a, &s.Actors[f])
		s.runDevice(i, skill.NoEntity)
	}
}

// unitTo is the unit vector from a to b.
func (s *Sim) unitTo(a, b *Actor) (float64, float64) {
	ax, ay := a.Pos()
	bx, by := b.Pos()
	l := math.Hypot(bx-ax, by-ay)
	if l < 1e-9 {
		return a.FaceX, a.FaceY
	}
	return (bx - ax) / l, (by - ay) / l
}

// slide moves a free extra toward x, y by at most step, over open floor.
func (s *Sim) slide(a *Actor, x, y, step float64, stopShort bool) {
	vx, vy := x-a.X, y-a.Y
	l := math.Hypot(vx, vy)
	if stopShort {
		l -= tileSpan * 0.8
	}
	if l <= 1e-3 {
		return
	}
	k := min(step, l) / math.Hypot(vx, vy)
	nx, ny := a.X+vx*k, a.Y+vy*k
	c := skill.FromPixel(nx, ny)
	if a.Dev != skill.FormDrone && (!s.Floor.In(c) || !s.Floor.At(c).Walkable()) {
		return
	}
	if !s.Floor.In(c) {
		return
	}
	a.X, a.Y = nx, ny
	a.At, a.From = c, c
	if l > 1e-3 {
		a.FaceX, a.FaceY = vx/math.Hypot(vx, vy), vy/math.Hypot(vx, vy)
	}
}

// runDevice runs device i's hex from where it stands.
func (s *Sim) runDevice(i int, subject skill.EntityID) {
	a := &s.Actors[i]
	dep := a.Dep
	s.emitter, s.emitting = skill.EntityID(i), true
	s.Disp.RunDevice(dep, skill.EntityID(i), subject)
	// Engineer blueprints: every device runs them too, from where it
	// stands.
	s.Disp.Emit(skill.Event{Trigger: skill.Blueprint, Actor: dep.Caster, Subject: subject, Chain: 1, Slot: -1,
		AtOrigin: true, Origin: skill.EntityID(i)})
	s.emitting = false
	s.flushForms(dep.Caster, dep.Prog, skill.Fired)
	s.addFX(FX{Kind: FXDeviceFire, At: a.At, Slot: int(a.Dev), Life: 0.3})
}

// minion runs one raised minion for a frame: walk to the nearest foe and
// hit it.
func (s *Sim) minion(i int, dt float32) {
	a := &s.Actors[i]
	a.Clock -= dt
	if a.U.StunLeft > 0 {
		return
	}
	f := s.nearestFoe(a.At, NearestRange+2)
	if f == skill.NoEntity {
		// Nothing to fight: fall in around you, each in its own place.
		p := &s.Actors[a.Owner]
		ring := 1 + int(a.Seq/6)%2
		spot := p.At
		for range ring {
			spot = spot.Add(skill.HexDirs[a.Seq%6])
		}
		sx, sy := spot.Pixel()
		ox, oy := p.At.Pixel()
		s.slide(a, p.X+sx-ox, p.Y+sy-oy, MinionSpeed*tileSpan*float64(dt)*0.8, false)
		return
	}
	t := &s.Actors[f]
	if s.distF(a, t) > MinionReach {
		tx, ty := t.Pos()
		s.slide(a, tx, ty, MinionSpeed*tileSpan*float64(dt), true)
		return
	}
	a.FaceX, a.FaceY = s.unitTo(a, t)
	a.Facing = dirOf(a.FaceX, a.FaceY)
	if a.Clock > 0 {
		return
	}
	a.Clock = MinionEvery
	st := enemyTable[a.Kind]
	dmg := st.dmg * scale(a.U.Level) * MinionHit
	dealt, _ := s.Rules.Resolve(&t.U, dmg, 0, a.U.Level, false)
	s.Hit(skill.EntityID(i), f, dealt, 0, false)
	if t.U.Alive {
		// Necromancer brains: the minion runs them on the foe it hit.
		s.emitter, s.emitting = skill.EntityID(i), true
		s.Disp.Emit(skill.Event{Trigger: skill.Brain, Actor: a.Owner, Subject: f, Chain: 1, Slot: -1,
			AtOrigin: true, Origin: skill.EntityID(i)})
		s.emitting = false
	}
	if !t.U.Alive {
		s.died(f)
		s.rise(f) // what the army kills joins it, while there's room
		s.addFX(FX{Kind: FXKill, At: t.At, Type: t.LastHit, Slot: int(t.Kind), Life: 0.5})
		s.Disp.Emit(skill.Event{Trigger: skill.OnKill, Actor: a.Owner, Subject: f, Chain: 1, Slot: -1})
	}
}

// --- triggers the class parts listen for ---

// wrecked posts OnWreck to every hex owner near a body that just fell.
func (s *Sim) wrecked(id skill.EntityID) {
	a := &s.Actors[id]
	if a.Fell || a.extra() || id == s.Player {
		return
	}
	p := s.P()
	if !p.U.Alive || skill.Dist(p.At, a.At) > skill.WreckRange {
		return
	}
	s.Disp.Post(skill.Event{Trigger: skill.OnWreck, Actor: s.Player, Subject: id, Chain: 1, Slot: -1})
}

// dodged is a shot passing close by you without hitting: OnDodge.
func (s *Sim) dodged(sh *Shot) {
	s.Disp.Emit(skill.Event{Trigger: skill.OnDodge, Actor: s.Player, Subject: sh.Owner, Chain: 1, Slot: -1})
}

// thorns hurts attacker for share of what it dealt you, while your
// thorns last.
func (s *Sim) thorns(attacker skill.EntityID, dealt float32) {
	p := s.P()
	if p.U.ThornsLeft <= 0 || attacker < 0 || int(attacker) >= len(s.Actors) || dealt <= 0 {
		return
	}
	a := &s.Actors[attacker]
	if !a.U.Alive {
		return
	}
	back, _ := s.Rules.Resolve(&a.U, dealt*skill.ThornsFrac, 0, p.U.Level, false)
	s.addFX(FX{Kind: FXThorns, At: a.At, From: p.At, Value: back, Life: 0.4})
	if back > 0 {
		s.addFX(FX{Kind: FXNumber, At: a.At, Value: back, Small: true, Life: 0.6})
	}
	if !a.U.Alive {
		s.died(attacker)
		s.Disp.Emit(skill.Event{Trigger: skill.OnKill, Actor: s.Player, Subject: attacker, Chain: 1, Slot: -1})
	}
}

// taunted is what a taunt does to a blow aimed at you.
func (s *Sim) taunted(attacker skill.EntityID, raw float32) float32 {
	if attacker >= 0 && int(attacker) < len(s.Actors) && s.Actors[attacker].U.TauntLeft > 0 {
		return raw * (1 - skill.TauntCut)
	}
	return raw
}

// stances sets the player's stats from the stances on right now, each
// frame: guard, might, speed and the rig's rates.
func (s *Sim) stances() {
	p := s.P()
	var m uint8
	if l := s.Disp.Loadout(s.Player); l != nil {
		m = l.Stances()
	}
	s.Stance = m
	p.U.Might = 1
	s.Rig.ChargeMul, s.Rig.CoolMul = 1, 1
	if m&(1<<skill.StanceHunter) != 0 {
		p.U.Might += skill.HunterMight
	}
	if m&(1<<skill.StanceFurnace) != 0 {
		s.Rig.CoolMul *= skill.FurnaceCool
		s.Rig.ChargeMul *= skill.FurnaceCharge
	}
}

// soak takes a hit on the player's raised guard hex, if one is held:
// half is stopped and stored for the release.
func (s *Sim) soak(raw float32) float32 {
	if l := s.Disp.Loadout(s.Player); l != nil {
		return l.Soaked(raw)
	}
	return raw
}

// guard is the share of a blow your stance lets through.
func (s *Sim) guard() float32 {
	g := float32(1)
	if s.Stance&(1<<skill.StanceBulwark) != 0 {
		g *= 1 - skill.BulwarkGuard
	}
	if s.Stance&(1<<skill.StanceHunter) != 0 {
		g *= 1 + skill.HunterFrail
	}
	return g
}

// speedMul is how fast your haste and stance let you move.
func (s *Sim) speedMul() float64 {
	m := 1.0
	if s.P().U.HasteLeft > 0 {
		m *= 1 + skill.HasteFrac
	}
	if s.Stance&(1<<skill.StanceBulwark) != 0 {
		m *= 1 - skill.BulwarkSlow
	}
	if s.Stance&(1<<skill.StanceHunter) != 0 {
		m *= 1 + skill.HunterSpeed
	}
	return m
}

// IsWreck reports whether actor i lies as a wreck the class parts can use.
func (s *Sim) IsWreck(i int) bool { return s.wreck(i) }

// Extra reports whether a is a device or a raised minion.
func (a *Actor) Extra() bool { return a.extra() }

// StanceOn reports whether slot i's stance is on right now.
func (s *Sim) StanceOn(i int) bool {
	l := s.Disp.Loadout(s.Player)
	return l != nil && i >= 0 && i < len(l.On) && l.On[i]
}

package game

// Squad tactics. Foes that spawn in the same room are a pack. The one
// that spots you calls the rest, who come a beat later. A pack moves
// behind its leader (a brute if it has one), keeps a little room between
// its members, and spreads round you to surround you instead of queueing
// up one behind another; skitters swing wide to your sides and back.
// Gunners keep their distance, line up a shot you can see coming and
// back off when you close in. A hurt foe falls back toward its pack for
// a few seconds, once, and then comes again.
//
// It's all cheap and deterministic: every choice is a score over the six
// tiles next to a foe, read off the walking-distance field the sim
// already keeps, with no random rolls and nothing allocated in the step.

import (
	"math"

	"hexes/dungeon"
	"hexes/skill"
)

// Squad tuning. Placeholder balance, one constant each.
const (
	MaxSquads     = 64
	MaxShots      = 32
	AlertShow     = 0.9  // seconds the spotted mark shows
	CallDelay     = 0.3  // a called packmate's first beat
	CallStagger   = 0.12 // and this much more per packmate
	CallRadius    = 4    // foes this near (hexes) hear a call even outside the pack
	SurroundRange = 4    // steps from you where a pack starts to spread round you
	FollowGap     = 1    // followers stay no more than this many steps ahead of their leader
	FleeTime      = 1.8  // seconds a hurt foe falls back
	GunMin        = 3    // a gunner backs off inside this many hexes
	GunMax        = 6    // and closes in past this many
	GunAim        = 0.5  // seconds a gunner lines up before it fires
	GunAimClose   = 0.9  // and from inside GunMin, when it can't get out
	GunButt       = 0.6  // a cornered gunner's swing, share of its shot
	UnjamDepth    = 4    // packmates down a queue a jammed gunner can shuffle back
	ShotSpeed     = 8.5  // hex units per second
	ShotLife      = 1.6  // seconds a shot flies
	ShotHitRadius = 0.7  // hex units from the player's middle
)

// fleeAt is the health share under which a kind falls back once.
var fleeAt = [NumKinds]float32{KindGrunt: 0.25, KindSkitter: 0.35, KindGunner: 0.35}

type squad struct {
	leader skill.EntityID
	n      uint8
}

// Shot is a gunner's round in the air.
type Shot struct {
	X, Y, VX, VY float64
	Dmg          float32
	Owner        skill.EntityID
	Age          float32
	Type         skill.TypeID
}

// formSquads picks each pack's leader: its brute if it has one, else its
// toughest member.
func (s *Sim) formSquads() {
	s.squads = [MaxSquads]squad{}
	for i := range s.squads {
		s.squads[i].leader = skill.NoEntity
	}
	s.NShots = 0
	for i := range s.Actors {
		a := &s.Actors[i]
		if a.Squad == 0 {
			continue
		}
		q := &s.squads[a.Squad]
		q.n++
		if q.leader == skill.NoEntity || s.leaderScore(i) > s.leaderScore(int(q.leader)) {
			q.leader = skill.EntityID(i)
		}
	}
}

func (s *Sim) leaderScore(i int) float32 {
	a := &s.Actors[i]
	sc := a.U.MaxHP
	if a.Kind == KindBrute {
		sc += 1e4
	}
	if a.Kind == KindGunner || a.Kind == KindSkitter {
		sc -= 1e3 // they don't lead from the front
	}
	return sc
}

// leaderOf is a's pack leader while it lives, or NoEntity.
func (s *Sim) leaderOf(a *Actor) skill.EntityID {
	if a.Squad == 0 {
		return skill.NoEntity
	}
	q := &s.squads[a.Squad]
	if q.leader != skill.NoEntity && !s.Actors[q.leader].U.Alive {
		// The leader fell: the next toughest steps up.
		q.leader = skill.NoEntity
		for i := range s.Actors {
			b := &s.Actors[i]
			if b.Squad == a.Squad && b.U.Alive && (q.leader == skill.NoEntity || s.leaderScore(i) > s.leaderScore(int(q.leader))) {
				q.leader = skill.EntityID(i)
			}
		}
	}
	return q.leader
}

// wake wakes foe id. spotted is whether it saw you itself, which marks it
// and calls its pack and anyone close by.
func (s *Sim) wake(id skill.EntityID, spotted bool) {
	a := &s.Actors[id]
	if id == s.Player || !a.U.Alive {
		return
	}
	was := a.Awake
	a.Awake = true
	if was {
		return
	}
	a.Alert = AlertShow
	if !spotted {
		return
	}
	k := float32(0)
	for i := range s.Actors {
		b := &s.Actors[i]
		if i == int(id) || skill.EntityID(i) == s.Player || !b.U.Alive || b.Awake || b.U.Team != a.U.Team {
			continue
		}
		near := skill.Dist(a.At, b.At) <= CallRadius
		if (a.Squad != 0 && b.Squad == a.Squad) || near {
			k++
			b.Awake = true
			b.Alert = AlertShow
			b.WakeIn = CallDelay + CallStagger*k
			// Turn to the one who called.
			b.Facing = dirTo(b.At, a.At)
		}
	}
}

// enemies runs every foe's turn.
func (s *Sim) enemies(dt float32) {
	p := s.P()
	for i := range s.Actors {
		a := &s.Actors[i]
		if skill.EntityID(i) == s.Player || !a.U.Alive || a.U.Team == p.U.Team {
			continue
		}
		a.AtkCD -= dt
		a.Alert = max(0, a.Alert-dt)
		a.Flee = max(0, a.Flee-dt)
		if a.U.StunLeft > 0 {
			a.Aim = 0 // a stun spoils the shot
			continue
		}
		d := s.dist[s.Floor.Index(a.At)]
		if !a.Awake {
			if (d >= 0 && d <= AggroRange) || a.U.HP < a.U.MaxHP {
				s.wake(skill.EntityID(i), true)
			} else {
				continue
			}
		}
		if a.WakeIn > 0 {
			a.WakeIn -= dt
			continue
		}
		// Hurt badly: fall back once.
		if !a.Fled && a.U.HP < a.U.MaxHP*fleeAt[a.Kind] && fleeAt[a.Kind] > 0 {
			a.Fled, a.Flee = true, FleeTime
			a.Aim = 0
		}
		if a.Kind == KindGunner {
			s.gunner(skill.EntityID(i), d, dt)
			continue
		}
		st := enemyTable[a.Kind]
		adj := skill.Dist(a.At, p.At) == 1
		if adj && a.Arrived() && (a.Flee == 0 || !s.canBackOff(a, d)) {
			// Between swings, edge round you to its own side of the ring.
			if a.Flee == 0 && a.AtkCD > 0 {
				if dir, ok := s.edge(skill.EntityID(i)); ok {
					s.startMove(skill.EntityID(i), dir, st.speed, false)
					continue
				}
			}
			a.Facing = dirTo(a.At, p.At)
			if a.AtkCD <= 0 && p.U.Alive {
				s.claw(skill.EntityID(i), st.dmg*scale(a.U.Level))
				a.AtkCD = st.cd
			}
			continue
		}
		if !a.Arrived() || d < 0 {
			continue
		}
		if dir, ok := s.melee(skill.EntityID(i), d); ok {
			s.startMove(skill.EntityID(i), dir, st.speed, false)
		}
	}
}

// canBackOff reports whether a has a tile further from you to go to.
func (s *Sim) canBackOff(a *Actor, d int32) bool {
	for _, dir := range skill.HexDirs {
		n := a.At.Add(dir)
		if s.Floor.In(n) && !s.occupied(n) {
			if nd := s.dist[s.Floor.Index(n)]; nd > d {
				return true
			}
		}
	}
	return false
}

// crowd is how many of a's living allies stand next to tile c.
func (s *Sim) crowd(c skill.Coord, self skill.EntityID) int32 {
	n := int32(0)
	for _, dir := range skill.HexDirs {
		t := c.Add(dir)
		if !s.Floor.In(t) {
			continue
		}
		o := s.occ[s.Floor.Index(t)]
		if o != skill.NoEntity && o != self && o != s.Player && s.Actors[o].U.Alive {
			n++
		}
	}
	return n
}

// slot is the tile round you that foe id wants: packmates take turns
// round the ring, starting from the side they came from; skitters start
// two turns round, at your flank.
func (s *Sim) slot(id skill.EntityID) skill.Coord {
	a := &s.Actors[id]
	p := s.P()
	from := a.At
	if l := s.leaderOf(a); l != skill.NoEntity {
		from = s.Actors[l].At
	}
	base := int(dirTo(p.At, from))
	rank := 0
	for i := range s.Actors {
		b := &s.Actors[i]
		if i >= int(id) {
			break
		}
		if b.U.Alive && b.Awake && b.Squad == a.Squad && b.Kind != KindGunner && skill.EntityID(i) != s.Player {
			rank++
		}
	}
	if a.Squad == 0 {
		rank = int(id)
	}
	if a.Kind == KindSkitter {
		rank += 3
	}
	// Turns round the ring: 0, +1, -1, +2, -2, 3.
	turn := [...]int{0, 1, -1, 2, -2, 3}[rank%6]
	return p.At.Add(skill.HexDirs[(base+turn+6)%6])
}

// edge is a step round you, staying next to you, toward a foe's slot.
func (s *Sim) edge(id skill.EntityID) (uint8, bool) {
	a := &s.Actors[id]
	p := s.P()
	sl := s.slot(id)
	if sl == a.At || skill.Dist(a.At, sl) != 1 || !s.Floor.In(sl) || !s.Floor.At(sl).Walkable() || s.occupied(sl) {
		return 0, false
	}
	if skill.Dist(sl, p.At) != 1 {
		return 0, false
	}
	return dirTo(a.At, sl), true
}

// melee picks a melee foe's next step, or false to hold.
func (s *Sim) melee(id skill.EntityID, d int32) (uint8, bool) {
	a := &s.Actors[id]
	p := s.P()
	f := s.Floor
	if a.Flee > 0 {
		return s.backOff(id, d)
	}
	// Followers don't run ahead of a leader still on its way in.
	lead := s.leaderOf(a)
	if lead != skill.NoEntity && lead != id && a.Kind != KindSkitter && d > 2 {
		l := &s.Actors[lead]
		if ld := s.dist[f.Index(l.At)]; ld >= 0 && d < ld-FollowGap && skill.Dist(l.At, p.At) > 1 {
			return s.near(id, l.At, d)
		}
	}
	slot, wantSlot := skill.Coord{}, d <= SurroundRange && a.Kind != KindGunner
	if wantSlot {
		slot = s.slot(id)
		if !f.In(slot) || !f.At(slot).Walkable() {
			wantSlot = false
		}
	}
	best, bestScore := -1, int32(math.MaxInt32)
	stay := int32(math.MaxInt32)
	var px, py, want float64
	if wantSlot {
		// Work round you by angle, not straight at the slot: the straight
		// line to the far side runs through you.
		px, py = p.Pos()
		sx, sy := slot.Pixel()
		want = math.Atan2(sy-py, sx-px)
		stay = s.slotScore(a.At, d, px, py, want, id)
	}
	for k, dir := range skill.HexDirs {
		n := a.At.Add(dir)
		if !f.In(n) || s.occupied(n) {
			continue
		}
		nd := s.dist[f.Index(n)]
		if nd < 0 {
			continue
		}
		var sc int32
		if wantSlot {
			// Head for the slot, but never by backing right off.
			if nd > d+1 {
				continue
			}
			sc = s.slotScore(n, nd, px, py, want, id)
		} else {
			// Close in, sidestepping a crowd rather than queueing.
			if nd > d {
				continue
			}
			sc = nd*4 + s.crowd(n, id)*2
			if nd == d {
				sc += 3
			}
		}
		if sc < bestScore {
			best, bestScore = k, sc
		}
	}
	if best < 0 || (wantSlot && bestScore >= stay) {
		return 0, false
	}
	return uint8(best), true
}

// slotScore is how far tile c, nd steps from you, is from a slot at
// angle want round you (px, py): lower is better.
func (s *Sim) slotScore(c skill.Coord, nd int32, px, py, want float64, id skill.EntityID) int32 {
	cx, cy := c.Pixel()
	da := math.Abs(math.Atan2(cy-py, cx-px) - want)
	if da > math.Pi {
		da = 2*math.Pi - da
	}
	return int32(da*8) + nd*3 + s.crowd(c, id)
}

// backOff steps a hurt foe away from you, toward its pack if it has one.
func (s *Sim) backOff(id skill.EntityID, d int32) (uint8, bool) {
	a := &s.Actors[id]
	f := s.Floor
	home := skill.Coord{}
	hasHome := false
	if l := s.leaderOf(a); l != skill.NoEntity && l != id {
		home, hasHome = s.Actors[l].At, true
	}
	best, bestScore := -1, int32(math.MinInt32)
	for k, dir := range skill.HexDirs {
		n := a.At.Add(dir)
		if !f.In(n) || s.occupied(n) || f.At(n) == dungeon.Hazard {
			continue
		}
		nd := s.dist[f.Index(n)]
		if nd <= d {
			continue
		}
		sc := nd * 4
		if hasHome {
			sc -= int32(skill.Dist(n, home))
		}
		if sc > bestScore {
			best, bestScore = k, sc
		}
	}
	if best < 0 {
		return 0, false
	}
	return uint8(best), true
}

// near steps a follower toward its leader without closing on you.
func (s *Sim) near(id skill.EntityID, at skill.Coord, d int32) (uint8, bool) {
	a := &s.Actors[id]
	f := s.Floor
	if skill.Dist(a.At, at) <= 2 {
		return 0, false
	}
	best, bestScore := -1, int32(skill.Dist(a.At, at))*2+s.crowd(a.At, id)
	for k, dir := range skill.HexDirs {
		n := a.At.Add(dir)
		if !f.In(n) || s.occupied(n) {
			continue
		}
		nd := s.dist[f.Index(n)]
		if nd < 0 || nd < d {
			continue
		}
		sc := int32(skill.Dist(n, at))*2 + s.crowd(n, id)
		if sc < bestScore {
			best, bestScore = k, sc
		}
	}
	if best < 0 {
		return 0, false
	}
	return uint8(best), true
}

// gunner runs a gunner's turn: hold at range with a clear line, line up
// a shot and fire; back off if you close in; come closer if it can't see
// you.
func (s *Sim) gunner(id skill.EntityID, d int32, dt float32) {
	a := &s.Actors[id]
	p := s.P()
	f := s.Floor
	st := enemyTable[KindGunner]
	hd := skill.Dist(a.At, p.At)
	line := clearLine(f, a.At, p.At)
	if a.Aim > 0 {
		a.Facing = dirTo(a.At, p.At)
		a.Aim -= dt
		if a.Aim <= 0 {
			a.Aim = 0
			if line && p.U.Alive {
				s.fire(id, st.dmg*scale(a.U.Level))
			}
			a.AtkCD = st.cd
		}
		return
	}
	if !a.Arrived() || d < 0 {
		return
	}
	cornered := false
	switch {
	case a.Flee > 0 || hd < GunMin:
		if dir, ok := s.backOff(id, d); ok {
			s.startMove(id, dir, st.speed*1.15, false)
			return
		}
		// Jammed: packmates fill the way back. Trade places with the one
		// behind, make room by moving the queue back, or sidestep; only
		// with all of that shut does it fight from where it stands.
		if s.unjam(id, d) {
			return
		}
		// Packmates in the way will move soon: hold fire and wait. Only
		// walls all round make it cornered.
		cornered = hd < GunMin && !s.allyBehind(id, d)
		if hd < GunMin && !cornered {
			return
		}
	case hd > GunMax || !line:
		if dir, ok := s.melee(id, d); ok {
			s.startMove(id, dir, st.speed, false)
			return
		}
	}
	if cornered {
		if hd <= 1 {
			// Point blank with nowhere to go: a rifle-butt swing, not a shot.
			a.Facing = dirTo(a.At, p.At)
			if a.AtkCD <= 0 && p.U.Alive {
				s.claw(id, st.dmg*GunButt*scale(a.U.Level))
				a.AtkCD = st.cd
			}
			return
		}
		if line && a.AtkCD <= 0 && p.U.Alive {
			a.Aim = GunAimClose // a slow wind-up, so a close shot can still be dodged
			a.Facing = dirTo(a.At, p.At)
		}
		return
	}
	// Never line up a shot from inside the range band's near edge: a
	// gunner that got there is still trying to get out.
	if line && hd >= GunMin && hd <= GunMax+1 && a.AtkCD <= 0 && p.U.Alive {
		a.Aim = GunAim
		a.Facing = dirTo(a.At, p.At)
		return
	}
	// Between shots, every other one, sidestep along the range band.
	if line && a.AtkCD > 0 && a.AtkCD < st.cd*0.6 && a.Shots%2 == 1 {
		a.Shots++
		for k := range skill.HexDirs {
			dir := (int(a.Facing) + 2 + 2*int(hash(uint64(id)+uint64(a.Shots))%2)*1 + k) % 6
			n := a.At.Add(skill.HexDirs[dir])
			if !f.In(n) || s.occupied(n) {
				continue
			}
			if nd := s.dist[f.Index(n)]; nd >= 0 && skill.Dist(n, p.At) >= GunMin && clearLine(f, n, p.At) {
				s.startMove(id, uint8(dir), st.speed, false)
				break
			}
		}
	}
}

// unjam gets a gunner that's too close and can't back off moving: it
// trades places with an ally that's further from you and wants to come
// in, asks a queue of gunners behind it to step back, or sidesteps to a
// tile no nearer you. It reports whether anything moved.
func (s *Sim) unjam(id skill.EntityID, d int32) bool {
	a := &s.Actors[id]
	f := s.Floor
	st := enemyTable[KindGunner]
	// The packmate behind.
	for _, dir := range skill.HexDirs {
		n := a.At.Add(dir)
		if !f.In(n) {
			continue
		}
		o := s.occ[f.Index(n)]
		if o == skill.NoEntity || o == s.Player || o == id {
			continue
		}
		b := &s.Actors[o]
		if !b.U.Alive || b.U.Team != a.U.Team || !b.Arrived() || b.U.StunLeft > 0 || s.dist[f.Index(n)] <= d {
			continue
		}
		if b.Kind != KindGunner && b.Flee == 0 {
			// A melee ally wants to be where the gunner is.
			s.swap(id, o, st.speed*1.15, enemyTable[b.Kind].speed)
			return true
		}
		if b.Kind == KindGunner && s.shuffleBack(o, UnjamDepth) {
			return true
		}
	}
	// Sidestep: any free tile no nearer you, keeping a line if it can.
	p := s.P()
	best, bestScore := -1, int32(math.MaxInt32)
	hd := skill.Dist(a.At, p.At)
	for k, dir := range skill.HexDirs {
		n := a.At.Add(dir)
		if !f.In(n) || s.occupied(n) || f.At(n) == dungeon.Hazard || !f.At(n).Walkable() {
			continue
		}
		nd := s.dist[f.Index(n)]
		if nd < d || skill.Dist(n, p.At) < hd {
			continue
		}
		sc := s.crowd(n, id) * 2
		if !clearLine(f, n, p.At) {
			sc += 3
		}
		if sc < bestScore {
			best, bestScore = k, sc
		}
	}
	if best >= 0 {
		return s.startMove(id, uint8(best), st.speed, false)
	}
	return false
}

// shuffleBack steps gunner id one tile further from you, first asking the
// gunner behind it to do the same, up to depth gunners down the queue.
func (s *Sim) shuffleBack(id skill.EntityID, depth int) bool {
	a := &s.Actors[id]
	if depth <= 0 || !a.Arrived() || a.U.StunLeft > 0 {
		return false
	}
	f := s.Floor
	d := s.dist[f.Index(a.At)]
	if dir, ok := s.backOff(id, d); ok {
		a.Aim = 0 // making room comes first
		return s.startMove(id, dir, enemyTable[KindGunner].speed*1.15, false)
	}
	for _, dir := range skill.HexDirs {
		n := a.At.Add(dir)
		if !f.In(n) || s.dist[f.Index(n)] <= d {
			continue
		}
		o := s.occ[f.Index(n)]
		if o != skill.NoEntity && o != s.Player && o != id && s.Actors[o].U.Alive && s.Actors[o].Kind == KindGunner &&
			s.Actors[o].U.Team == a.U.Team && s.shuffleBack(o, depth-1) {
			return true
		}
	}
	return false
}

// allyBehind reports whether a living packmate stands on a tile next to
// actor id that's further from you than d.
func (s *Sim) allyBehind(id skill.EntityID, d int32) bool {
	a := &s.Actors[id]
	f := s.Floor
	for _, dir := range skill.HexDirs {
		n := a.At.Add(dir)
		if !f.In(n) || s.dist[f.Index(n)] <= d {
			continue
		}
		if o := s.occ[f.Index(n)]; o != skill.NoEntity && o != s.Player && o != id && s.Actors[o].U.Alive && s.Actors[o].U.Team == a.U.Team {
			return true
		}
	}
	return false
}

// swap trades two actors' tiles, each stepping at its own speed.
func (s *Sim) swap(x, y skill.EntityID, vx, vy float32) {
	a, b := &s.Actors[x], &s.Actors[y]
	f := s.Floor
	ax, bx := a.At, b.At
	s.occ[f.Index(ax)], s.occ[f.Index(bx)] = y, x
	a.From, a.At = ax, bx
	b.From, b.At = bx, ax
	if a.U.SlowLeft > 0 {
		vx *= 1 - a.U.SlowFrac
	}
	if b.U.SlowLeft > 0 {
		vy *= 1 - b.U.SlowFrac
	}
	a.MoveT, a.MoveDur, a.Facing = 0, 1/vx, dirTo(ax, bx)
	b.MoveT, b.MoveDur, b.Facing = 0, 1/vy, dirTo(bx, ax)
}

// fire looses a gunner's shot at where you are now.
func (s *Sim) fire(id skill.EntityID, dmg float32) {
	a := &s.Actors[id]
	if s.NShots == MaxShots {
		return
	}
	p := s.P()
	ax, ay := a.Pos()
	px, py := p.Pos()
	dx, dy := px-ax, py-ay
	l := math.Hypot(dx, dy)
	if l < 1e-6 {
		return
	}
	a.Shots++
	s.Shots[s.NShots] = Shot{X: ax + dx/l*0.5, Y: ay + dy/l*0.5, VX: dx / l * ShotSpeed, VY: dy / l * ShotSpeed, Dmg: dmg, Owner: id}
	s.NShots++
	s.FX.Add(FX{Kind: FXMuzzle, At: a.At, X: ax, Y: ay, DX: float32(dx / l), DY: float32(dy / l), Life: 0.25})
}

// stepShots flies every shot: a wall stops it, you take it.
func (s *Sim) stepShots(dt float32) {
	p := s.P()
	f := s.Floor
	for i := 0; i < s.NShots; {
		sh := &s.Shots[i]
		sh.Age += dt
		sh.X += sh.VX * float64(dt)
		sh.Y += sh.VY * float64(dt)
		c := skill.FromPixel(sh.X, sh.Y)
		gone := sh.Age >= ShotLife
		if !gone && (!f.In(c) || f.At(c) == dungeon.Wall) {
			gone = true
			s.FX.Add(FX{Kind: FXZap, At: c, X: sh.X, Y: sh.Y, DX: float32(sh.VX), DY: float32(sh.VY), Life: 0.3})
		}
		if !gone && p.U.Alive {
			px, py := p.Pos()
			if math.Hypot(px-sh.X, py-sh.Y) < ShotHitRadius {
				gone = true
				s.shotHit(sh)
			}
		}
		if gone {
			s.NShots--
			s.Shots[i] = s.Shots[s.NShots]
			continue
		}
		i++
	}
}

// shotHit is a shot landing on you: like a claw, from range.
func (s *Sim) shotHit(sh *Shot) {
	p := s.P()
	lvl := p.U.Level
	if int(sh.Owner) < len(s.Actors) {
		lvl = s.Actors[sh.Owner].U.Level
	}
	hit := min(sh.Dmg, s.Rules.HitCap(lvl, p.U.Level, p.U.MaxHP))
	left := p.U.Soak(hit)
	dealt := min(left, p.U.HP)
	p.U.HP -= dealt
	p.Flash = 0.2
	if p.U.HP <= 0 {
		p.U.HP, p.U.Alive = 0, false
	}
	if soaked := hit - left; soaked > 0 {
		s.addFX(FX{Kind: FXSoak, At: p.At, From: p.At, Value: soaked, Hurt: true, Mag: magOf(soaked, p.U.MaxHP), Life: 0.45})
	}
	if dealt > 0 {
		s.addFX(FX{Kind: FXNumber, At: p.At, Value: dealt, Hurt: true, Life: 0.8})
	}
	s.FX.Add(FX{Kind: FXZap, At: p.At, X: sh.X, Y: sh.Y, DX: float32(sh.VX), DY: float32(sh.VY), Hurt: true, Life: 0.3})
	if p.U.Alive && int(sh.Owner) < len(s.Actors) {
		s.Disp.Hurt(s.Player, sh.Owner)
	}
}

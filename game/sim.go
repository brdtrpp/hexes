// Package game is the real-time simulation for the playable slice: a
// generated floor, the player's rig, enemies with simple AI, hexes firing
// from triggers, terrain that kills. It has no rendering and no input
// devices, so it runs in tests; cmd/hexes draws it with Ebitengine.
package game

import (
	"math"

	"hexes/dungeon"
	"hexes/skill"
)

// Kind is what an actor is.
type Kind uint8

const (
	KindPlayer Kind = iota
	KindGrunt
	KindSkitter
	KindBrute
)

// Actor is one creature on the floor.
type Actor struct {
	U    skill.Unit
	Kind Kind
	// At is the tile the actor occupies. While moving it is the tile it is
	// moving into; From is where it came from and MoveT runs 0 to 1.
	At, From skill.Coord
	MoveT    float32
	MoveDur  float32
	Facing   uint8 // index into skill.HexDirs
	AtkCD    float32
	Awake    bool
	DeadFor  float32      // seconds since death, for the fade-out
	Fell     bool         // died in a pit
	Flash    float32      // hurt flash, seconds left
	LastHit  skill.TypeID // damage type of the last hit taken, for the death burst

	// Free actors (the player) move continuously instead of tile to tile:
	// X, Y is the true position in hex units and At is the hex under it.
	// FaceX, FaceY is the unit facing, from the last movement.
	Free         bool
	X, Y         float64
	FaceX, FaceY float64
}

// Arrived reports whether the actor has finished its current step.
func (a *Actor) Arrived() bool { return a.MoveT >= 1 }

// Pos is where to draw the actor, in hex units (lerped between tiles).
func (a *Actor) Pos() (float64, float64) {
	if a.Free {
		return a.X, a.Y
	}
	fx, fy := a.From.Pixel()
	tx, ty := a.At.Pixel()
	t := float64(min(1, a.MoveT))
	return fx + (tx-fx)*t, fy + (ty-fy)*t
}

// State is where the run is.
type State uint8

const (
	Playing State = iota
	Descending
	Dead
)

// Input is what the player is asking for this frame. The front end fills
// it from keys, mouse or touch.
type Input struct {
	// MoveX, MoveY is a direction in screen space (y down). Zero means stand.
	MoveX, MoveY float64
	Cast         [MaxSlots]bool
	Vent         bool
	Restart      bool
	// Hold is whether CAST is held down, for channel hexes.
	Hold bool
}

// Tuning for the slice. All of it is placeholder balance.
const (
	PlayerSpeed   = 4.5  // tiles per second
	StrikeEvery   = 0.45 // seconds between basic strikes
	NearestRange  = 9    // nearest-foe targeting reach, hexes
	AggroRange    = 10   // walking steps at which enemies notice you
	HazardTick    = 0.5  // seconds between vent burns
	HazardFrac    = 0.05 // vent burn per tick, share of max health
	DescendTime   = 0.9  // seconds of fade between floors
	LevelScale    = 1.10 // health and damage grow this much per level
	PlayerBaseHP  = 220
	HealOnDescend = 0.3
	RigPoints     = 8 // rig points the slice puts in Capacity and Recharge
)

type enemyStats struct {
	hp, dmg, speed, cd float32
}

var enemyTable = [...]enemyStats{
	KindGrunt:   {hp: 60, dmg: 18, speed: 2.4, cd: 1.0},
	KindSkitter: {hp: 32, dmg: 11, speed: 3.9, cd: 0.7},
	KindBrute:   {hp: 150, dmg: 34, speed: 1.5, cd: 1.6},
}

// Sim is one run of the game.
type Sim struct {
	Reg   *skill.Registry
	Rules skill.Rules
	Eval  *skill.Evaluator
	Disp  *skill.Dispatcher
	Floor *dungeon.Floor
	Gen   dungeon.Generator

	RunSeed uint64
	Depth   int
	Actors  []Actor
	Player  skill.EntityID
	Rig     skill.Rig
	Load    Loadout
	Input   Input
	// Banks are each hex's charge banks. They keep their counts through
	// edits and start empty on every new floor.
	Banks [skill.MaxEquip]skill.Bank
	// BoardBanks are the board's shared banks X and Y, which every hex
	// can charge, spend and read. They start empty on every floor too.
	BoardBanks skill.BoardBank

	State  State
	StateT float32
	Kills  int
	Best   int     // deepest floor ever reached
	Time   float64 // seconds of play, ever
	FX     FXBuf
	// forms are the target areas hexes picked during the current firing
	// (see form.go); Fired turns the ones that landed into effects.
	forms  [maxForms]form
	nforms int

	// Progress that lasts (save.go). Points are the rig's attribute
	// points. Cache is the salvage from the last death. Visits counts
	// floor loads, so a floor seen again drops different loot.
	Points      skill.Points
	Cache       Cache
	Deaths      int
	LastDeathAt float64
	LastDeath   Death
	Visits      int
	// CheckpointT counts down while the "pack secured" notice shows.
	CheckpointT float32
	// Dirty is set when something worth saving changed. The front end
	// saves and Save clears it.
	Dirty bool
	// CallGiven: the save has had its starter Call (see giveCall).
	CallGiven bool
	// Shapes are the found shapes learned so far, kept for good (see
	// shape.go). ShapeGiven is set once the first one is found.
	// LastShape is the latest one learned, for the HUD.
	Shapes     []skill.Shape
	ShapeGiven bool
	LastShape  skill.Shape
	// TestShapes is the test switch: every shape unlocked for trying out
	// (see shape.go), and every frame (frames.go). A setting, so a new
	// run keeps it.
	TestShapes bool
	// Frames is the frame rack: frames found and not yet fitted, kept for
	// good (see frames.go). FrameGiven is set once the first is found.
	// LastFrame is the latest, for the HUD.
	Frames     []skill.Frame
	FrameGiven bool
	LastFrame  skill.Frame
	// Tiles are the logic tiles found so far, one bit each (see
	// tiles.go); the starter set is always known on top. LastTile is
	// the latest, for the HUD.
	Tiles uint32
	// Mine is the player's own build while an example is loaded (see
	// examples.go); Example names the example.
	Mine     *MyBuild
	Example  string
	LastTile skill.TileKind

	// SlotFlash counts down after a slot tries to fire; SlotLast is how.
	SlotFlash [MaxSlots]float32
	SlotLast  [MaxSlots]skill.Outcome

	// Stash is loot: parts on the floor and in the inventory (loot.go).
	Stash Stash
	// Sight is the fog of war: what the lamp lights now and what's been
	// explored on this floor (sight.go).
	Sight Sight

	occ      []skill.EntityID
	dist, q  []int32
	distFrom skill.Coord
	strikeCD float32
	dotClock float32
	hazClock float32
	safe     skill.Coord // last tile the player stood on that wasn't a pit
}

// New starts a run at depth 1.
func New(seed uint64) *Sim {
	s := &Sim{Reg: skill.DefaultRegistry(), Rules: skill.DefaultRules()}
	s.Eval = skill.NewEvaluator(s.Reg, s.Rules)
	s.NewRun(seed)
	return s
}

// DefaultPoints is the rig a new save starts with.
var DefaultPoints = skill.Points{Capacity: RigPoints, Recharge: RigPoints, Cooling: 0, Processing: 5}

// NewRun wipes all progress and starts again at depth 1 with the preset
// hexes: a fresh start. Dying doesn't do this; see save.go.
func (s *Sim) NewRun(seed uint64) {
	s.RunSeed = seed
	s.Kills, s.Best, s.Time, s.Visits = 0, 0, 0, 0
	s.Points = DefaultPoints
	s.Cache, s.Deaths, s.LastDeathAt, s.LastDeath = Cache{}, 0, 0, Death{}
	s.Load = Loadout{}
	s.CallGiven = true // the preset Arc Lance carries it
	s.Shapes, s.ShapeGiven, s.LastShape = nil, false, skill.Shape{}
	s.Frames, s.FrameGiven, s.LastFrame = nil, false, skill.Frame{}
	s.Tiles, s.LastTile = 0, 0
	s.resetStash()
	s.load(1, 1)
	s.Dirty = true
}

func (s *Sim) floorSeed(depth int) uint64 {
	x := s.RunSeed + uint64(depth)*0x9e3779b97f4a7c15
	x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
	return x ^ (x >> 31)
}

// load builds the floor for depth. hpFrac carries the player's health
// share over from the floor above.
func (s *Sim) load(depth int, hpFrac float32) {
	s.Depth = depth
	s.Best = max(s.Best, depth)
	s.Visits++
	s.CheckpointT = 0
	if IsCheckpoint(depth) {
		s.secure()
		s.CheckpointT = CheckpointShow
	}
	s.Floor = s.Gen.Generate(s.floorSeed(depth), depth)
	f := s.Floor
	n := f.W * f.H
	s.occ = make([]skill.EntityID, n)
	for i := range s.occ {
		s.occ[i] = skill.NoEntity
	}
	s.dist = make([]int32, n)
	s.q = make([]int32, n)
	s.Actors = s.Actors[:0]
	s.FX.Clear()

	lvl := depth
	maxHP := PlayerBaseHP * scale(lvl)
	s.Player = s.spawn(KindPlayer, 0, f.Start, lvl, maxHP)
	s.Actors[s.Player].U.HP = maxHP * min(1, max(0.05, hpFrac))
	s.Actors[s.Player].Awake = true
	s.Actors[s.Player].Free = true
	s.Actors[s.Player].placeFree(f.Start)
	s.Actors[s.Player].FaceX = 1
	s.safe = f.Start
	for _, sp := range f.Spawns {
		st := enemyTable[KindGrunt+Kind(sp.Kind)]
		id := s.spawn(KindGrunt+Kind(sp.Kind), 1, sp.At, sp.Level, st.hp*scale(sp.Level))
		s.Actors[id].Facing = uint8(hash(uint64(id)+f.Seed) % 6)
	}

	s.Rig = skill.NewRig(s.Points)
	s.loadLoot(lvl)
	s.Banks = [skill.MaxEquip]skill.Bank{} // every floor starts uncharged
	s.BoardBanks = skill.BoardBank{}
	s.Disp = skill.NewDispatcher(s.Eval, s)
	s.Disp.Listener = s
	s.Disp.ProcCooldown[skill.OnHit] = 1.2
	l := s.Disp.Equip(s.Player, &s.Rig, s.Load.Progs[:]...)
	l.Banks, l.Board = &s.Banks, &s.BoardBanks
	s.State, s.StateT = Playing, 0
	s.strikeCD = 0
	s.refreshField()
	// A cache saved before floors changed shape can sit in rock or out of
	// reach on the regenerated floor; move it next to where you arrive.
	if c := &s.Cache; c.Active && c.Depth == depth && (!f.In(c.At) || s.dist[f.Index(c.At)] <= 0) {
		c.At = f.Start
		for _, d := range skill.HexDirs {
			if n := f.Start.Add(d); f.In(n) && s.dist[f.Index(n)] == 1 {
				c.At = n
				break
			}
		}
	}
	s.refreshSight()
	s.Dirty = true
}

func (s *Sim) spawn(k Kind, team uint8, at skill.Coord, level int, hp float32) skill.EntityID {
	id := skill.EntityID(len(s.Actors))
	s.Actors = append(s.Actors, Actor{
		U:    skill.Unit{ID: id, Team: team, Alive: true, HP: hp, MaxHP: hp, Level: level, MarkOwner: skill.NoEntity},
		Kind: k, At: at, From: at, MoveT: 1,
	})
	s.occ[s.Floor.Index(at)] = id
	return id
}

func scale(level int) float32 { return float32(math.Pow(LevelScale, float64(level-1))) }

func hash(x uint64) uint64 {
	x += 0x9e3779b97f4a7c15
	x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
	x = (x ^ (x >> 27)) * 0x94d049bb133111eb
	return x ^ (x >> 31)
}

// P returns the player.
func (s *Sim) P() *Actor { return &s.Actors[s.Player] }

// Step advances the game by dt seconds using s.Input.
func (s *Sim) Step(dt float32) {
	if dt > 0.1 {
		dt = 0.1 // a dropped frame shouldn't teleport anyone
	}
	s.Time += float64(dt)
	s.nforms = 0 // forms only live within one firing
	s.FX.Update(dt)
	for i := range s.SlotFlash {
		s.SlotFlash[i] = max(0, s.SlotFlash[i]-dt)
	}
	s.StateT += dt
	s.CheckpointT = max(0, s.CheckpointT-dt)
	switch s.State {
	case Dead:
		s.ageCorpses(dt)
		if s.Input.Restart && s.StateT > 0.5 {
			s.respawn()
		}
		return
	case Descending:
		s.ageCorpses(dt)
		if s.StateT >= DescendTime {
			p := s.P()
			s.load(s.Depth+1, p.U.HP/p.U.MaxHP+HealOnDescend)
		}
		return
	}

	s.Disp.Update(dt)
	s.playerInput(dt)
	s.enemies(dt)
	s.advance(dt)
	s.timers(dt)
	s.pickup(dt)
	s.refreshSight()

	p := s.P()
	if !p.U.Alive {
		s.State, s.StateT = Dead, 0
		s.die()
		return
	}
	if p.Arrived() && s.Floor.At(p.At) == dungeon.Stairs {
		s.State, s.StateT = Descending, 0
	}
}

func (s *Sim) playerInput(dt float32) {
	p := s.P()
	in := &s.Input
	if in.Vent {
		if s.Rig.Vent() {
			s.addFX(FX{Kind: FXVent, At: p.At, Life: 0.6})
		}
	}
	s.Disp.Channel(s.Player, in.Hold && p.U.Alive)
	for slot, pressed := range in.Cast {
		if !pressed || s.Load.Progs[slot] == nil {
			continue
		}
		o := s.Disp.Cast(s.Player, slot, s.nearestFoe(p.At, NearestRange))
		if o != skill.Fired {
			s.SlotFlash[slot], s.SlotLast[slot] = 0.4, o
		}
	}
	if !p.U.Alive {
		return
	}
	stunned := p.U.StunLeft > 0
	s.movePlayer(dt)
	if !p.U.Alive {
		return
	}
	s.strikeCD -= dt
	if s.strikeCD <= 0 && !stunned {
		if foe := s.adjacentFoe(s.Player); foe != skill.NoEntity {
			p.Facing = dirTo(p.At, s.Actors[foe].At)
			if s.Input.MoveX == 0 && s.Input.MoveY == 0 {
				// Standing still: turn to face what we hit.
				fx, fy := s.Actors[foe].Pos()
				fx, fy = fx-p.X, fy-p.Y
				if l := math.Hypot(fx, fy); l > 0 {
					p.FaceX, p.FaceY = fx/l, fy/l
				}
			}
			s.Disp.Strike(s.Player, s.Load.Strike, foe)
			s.flushForms(s.Player, s.Load.Strike, skill.Fired)
			s.strikeCD = StrikeEvery
		}
	}
}

// startMove begins a one-tile step. Enemies and the player both refuse to
// walk into pits; only pushes put things in pits.
func (s *Sim) startMove(id skill.EntityID, dir uint8, speed float32, _ bool) bool {
	a := &s.Actors[id]
	to := a.At.Add(skill.HexDirs[dir])
	if !s.Floor.At(to).Walkable() || s.occupied(to) {
		return false
	}
	if a.U.SlowLeft > 0 {
		speed *= 1 - a.U.SlowFrac
	}
	s.occ[s.Floor.Index(a.At)] = skill.NoEntity
	s.occ[s.Floor.Index(to)] = id
	a.From, a.At = a.At, to
	a.MoveT, a.MoveDur = 0, 1/speed
	a.Facing = dir
	if id == s.Player {
		s.refreshField()
	}
	return true
}

func (s *Sim) occupied(c skill.Coord) bool {
	if !s.Floor.In(c) {
		return true
	}
	o := s.occ[s.Floor.Index(c)]
	return o != skill.NoEntity && s.Actors[o].U.Alive
}

func (s *Sim) advance(dt float32) {
	for i := range s.Actors {
		a := &s.Actors[i]
		if a.MoveT < 1 {
			a.MoveT = min(1, a.MoveT+dt/a.MoveDur)
		}
	}
}

// refreshField recomputes walking distance from the player, which every
// enemy reads to chase. It only runs when the player changes tile.
func (s *Sim) refreshField() {
	p := s.P()
	s.distFrom = p.At
	s.Floor.DistanceField(p.At, s.dist, s.q)
}

func (s *Sim) enemies(dt float32) {
	p := s.P()
	for i := range s.Actors {
		a := &s.Actors[i]
		if skill.EntityID(i) == s.Player || !a.U.Alive || a.U.Team == p.U.Team {
			continue
		}
		a.AtkCD -= dt
		if a.U.StunLeft > 0 {
			continue
		}
		d := s.dist[s.Floor.Index(a.At)]
		if !a.Awake {
			if (d >= 0 && d <= AggroRange) || a.U.HP < a.U.MaxHP {
				a.Awake = true
			} else {
				continue
			}
		}
		st := enemyTable[a.Kind]
		if skill.Dist(a.At, p.At) == 1 && a.Arrived() {
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
		// Step to the neighbor closest to the player on foot.
		best, bestD := -1, d
		for k, dir := range skill.HexDirs {
			n := a.At.Add(dir)
			if !s.Floor.In(n) {
				continue
			}
			nd := s.dist[s.Floor.Index(n)]
			if nd >= 0 && nd < bestD && !s.occupied(n) {
				best, bestD = k, nd
			}
		}
		if best >= 0 {
			s.startMove(skill.EntityID(i), uint8(best), st.speed, false)
		}
	}
}

// claw is an enemy melee hit on the player: capped by level gap like any
// hit, but it doesn't build the player's resistance.
func (s *Sim) claw(attacker skill.EntityID, raw float32) {
	p := s.P()
	a := &s.Actors[attacker]
	dealt := min(raw, s.Rules.HitCap(a.U.Level, p.U.Level, p.U.MaxHP), p.U.HP)
	p.U.HP -= dealt
	p.Flash = 0.2
	if p.U.HP <= 0 {
		p.U.HP, p.U.Alive = 0, false
	}
	s.addFX(FX{Kind: FXNumber, At: p.At, Value: dealt, Hurt: true, Life: 0.8})
	s.addFX(FX{Kind: FXClaw, At: p.At, From: a.At, Life: 0.25})
	if p.U.Alive {
		s.Disp.Hurt(s.Player, attacker)
	}
}

// timers runs statuses, resistance decay, damage over time and hazard
// vents for everyone.
func (s *Sim) timers(dt float32) {
	s.ageCorpses(dt)
	for i := range s.Actors {
		a := &s.Actors[i]
		a.Flash = max(0, a.Flash-dt)
		u := &a.U
		if !u.Alive {
			continue
		}
		u.SlowLeft = max(0, u.SlowLeft-dt)
		u.StunLeft = max(0, u.StunLeft-dt)
		u.MarkLeft = max(0, u.MarkLeft-dt)
		u.Stealth = max(0, u.Stealth-dt)
		if u.StunLeft == 0 && u.Frozen {
			u.Frozen, u.Chill = false, 0
		}
		s.Rules.Decay(u, dt)
	}
	s.dotClock += dt
	for s.dotClock >= s.Rules.DotInterval {
		s.dotClock -= s.Rules.DotInterval
		for i := range s.Actors {
			a := &s.Actors[i]
			for k := range a.U.Dots {
				d := &a.U.Dots[k]
				if d.Left == 0 || !a.U.Alive {
					continue
				}
				dealt, _ := s.Rules.ResolveTick(&a.U, d.PerTick, d.Type, int(d.AttackerLevel))
				d.Left--
				if dealt > 0 {
					s.addFX(FX{Kind: FXNumber, At: a.At, Value: dealt, Type: d.Type, Small: true, Life: 0.6})
				}
				if !a.U.Alive {
					s.died(skill.EntityID(i))
					s.Disp.Emit(skill.Event{Trigger: skill.OnKill, Actor: d.Owner, Subject: skill.EntityID(i), Chain: 1, Slot: -1})
				}
			}
		}
	}
	s.hazClock += dt
	for s.hazClock >= HazardTick {
		s.hazClock -= HazardTick
		for i := range s.Actors {
			a := &s.Actors[i]
			if !a.U.Alive || !a.Arrived() || s.Floor.At(a.At) != dungeon.Hazard {
				continue
			}
			dealt, _ := s.Rules.Resolve(&a.U, HazardFrac*a.U.MaxHP, 0, 0, true)
			a.Flash = 0.15
			s.addFX(FX{Kind: FXNumber, At: a.At, Value: dealt, Hurt: skill.EntityID(i) == s.Player, Small: true, Life: 0.6})
			if !a.U.Alive {
				s.died(skill.EntityID(i))
			}
		}
	}
}

func (s *Sim) ageCorpses(dt float32) {
	for i := range s.Actors {
		if !s.Actors[i].U.Alive {
			s.Actors[i].DeadFor += dt
		}
	}
}

// died does the bookkeeping for any death: free the tile, count kills.
func (s *Sim) died(id skill.EntityID) {
	a := &s.Actors[id]
	if a.DeadFor > 0 {
		return
	}
	a.DeadFor = 1e-6
	if i := s.Floor.Index(a.At); s.occ[i] == id {
		s.occ[i] = skill.NoEntity
	}
	if id != s.Player {
		s.Kills++
		s.dropLoot(id)
	}
}

func (s *Sim) adjacentFoe(id skill.EntityID) skill.EntityID {
	a := &s.Actors[id]
	best := skill.NoEntity
	bestScore := -10
	for k, d := range skill.HexDirs {
		n := a.At.Add(d)
		if !s.Floor.In(n) {
			continue
		}
		o := s.occ[s.Floor.Index(n)]
		if o == skill.NoEntity || !s.Actors[o].U.Alive || s.Actors[o].U.Team == a.U.Team {
			continue
		}
		// Prefer whatever is in front of us.
		score := 3 - hexTurn(uint8(k), a.Facing)
		if score > bestScore {
			best, bestScore = o, score
		}
	}
	return best
}

func (s *Sim) nearestFoe(c skill.Coord, reach int) skill.EntityID {
	best, bestD := skill.NoEntity, reach+1
	for i := range s.Actors {
		a := &s.Actors[i]
		if !s.foe(s.Player, i) {
			continue
		}
		if d := skill.Dist(a.At, c); d < bestD {
			best, bestD = skill.EntityID(i), d
		}
	}
	return best
}

// hexTurn is how many 60-degree turns separate two directions (0 to 3).
func hexTurn(a, b uint8) int {
	d := int(a) - int(b)
	if d < 0 {
		d = -d
	}
	return min(d, 6-d)
}

// dirTo is the hex direction that best points from a toward b.
func dirTo(a, b skill.Coord) uint8 {
	ax, ay := a.Pixel()
	bx, by := b.Pixel()
	vx, vy := bx-ax, by-ay
	best, bestDot := 0, -math.MaxFloat64
	for i, d := range skill.HexDirs {
		dx, dy := d.Pixel()
		if dot := dx*vx + dy*vy; dot > bestDot {
			best, bestDot = i, dot
		}
	}
	return uint8(best)
}

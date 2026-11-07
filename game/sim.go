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
	KindGunner
	// KindDevice is a placed form: a zone, turret, trap or drone
	// (devices.go). Dev says which.
	KindDevice
	// NumKinds is how many actor kinds exist.
	NumKinds
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

	// Squad tactics (squad.go). Squad is the pack it spawned with (0 for
	// none). Alert counts down from spotting you or being called by a
	// packmate; WakeIn is the beat before a called foe moves. Flee is
	// seconds left of falling back hurt, and Fled marks that it has.
	// Aim counts down while a gunner lines up a shot.
	Squad  uint8
	Alert  float32
	WakeIn float32
	Flee   float32
	Fled   bool
	Aim    float32
	Shots  uint16

	// Devices, minions and wrecks (devices.go). Owner is whose hex placed
	// or raised it, Life its seconds left, Clock the countdown to its next
	// run, Every how often it runs and Arms a trap's triggers left. Dep is
	// the hex a device runs. Minion marks a raised body (its Kind stays
	// what it was alive); Used marks a body used up.
	Dev    skill.FormKind
	Rule   uint8
	Owner  skill.EntityID
	Life   float32
	Clock  float32
	Every  float32
	Arms   uint8
	Dep    skill.Deployment
	Minion bool
	Used   bool
	// Keeps marks a device or minion that stays until it's killed or
	// the floor ends, its Life left alone; Seq is the order extras came
	// in, so the oldest makes room first.
	Keeps bool
	Seq   uint32
	// Prey is the foe a drone is hunting, or skill.NoEntity.
	Prey skill.EntityID
	// PulledAt is when a Warden's kinetic hit last pulled it, HeldAt
	// when one last landed, which keeps that hit from shoving it back
	// out (warden.go).
	PulledAt, HeldAt float64
	// Exposed is the Rogue's stacks on it, ExposedT their seconds left
	// (rogue.go).
	Exposed  uint8
	ExposedT float32
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
	// Drafting waits on a draft pick (draft.go): the floor-end draft at
	// the stairs, or a fresh save's kit pick. Nothing moves meanwhile.
	Drafting
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
	// Held is which slots' buttons are held down, for overload and guard
	// hexes: letting go fires them.
	Held [MaxSlots]bool
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

var enemyTable = [NumKinds]enemyStats{
	KindGrunt:   {hp: 60, dmg: 18, speed: 2.4, cd: 1.0},
	KindSkitter: {hp: 32, dmg: 11, speed: 3.9, cd: 0.7},
	KindBrute:   {hp: 150, dmg: 34, speed: 1.5, cd: 1.6},
	KindGunner:  {hp: 44, dmg: 13, speed: 2.6, cd: 1.7},
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
	// Army is the raised bodies carried down to the next floor (army.go);
	// seq numbers extras as they come.
	Army []Raised
	seq  uint32
	// Brace is the Warden's stored block (warden.go), braceT the seconds
	// since it last grew; bursting is set while it goes off.
	Brace    float32
	braceT   float32
	bursting bool
	// The Berserker's streak of nanite hits and when the last landed
	// (berserker.go); berserk and mage are set while your hexes carry
	// those schools' types; cashing is set while a Rogue cash-in lands.
	furyStreak    int
	furyAt        float64
	berserk, mage bool
	devStance     bool // screenshots: both stances on whatever you carry
	cashing       bool
	Best          int     // deepest floor ever reached
	Time          float64 // seconds of play, ever
	FX            FXBuf
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
	// Visit is the depth to go back to while the player tries a floor
	// from the test menu (visit.go); zero on an ordinary floor.
	Visit     int
	visitSeed uint64
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
	// LastLoot is the latest part picked up off the floor, and LootN
	// counts pickups, so the HUD can show each one's card.
	LastLoot skill.Part
	LootN    int
	// SustainGiven is set once a sustain part (Mend, Purge, Shield) has
	// been picked up.
	SustainGiven bool
	// Taught is how many rungs of the teaching ladder are done (see
	// teach.go), kept for good. Card is the rung whose card shows, for
	// CardT more seconds.
	Taught int
	Card   int
	CardT  float32
	// veteran starts new runs with the preset hexes and the ladder done
	// (NewVeteran).
	veteran bool
	// Tiles are the logic tiles found so far, one bit each (see
	// tiles.go); the starter set is always known on top. LastTile is
	// the latest, for the HUD.
	Tiles uint32
	// Mine is the player's own build while an example is loaded (see
	// examples.go); Example names the example.
	Mine     *MyBuild
	Example  string
	LastTile skill.TileKind

	// The skill grid (grid.go, gridplay.go). Grid is rolled from RunSeed;
	// the rest is what this save did with it. Data is banked, Unbanked is
	// Data earned since the last checkpoint, which a death drops in the
	// salvage crate. Own, Seen, Glimpse and Reclaim are per grid node.
	// Locker holds every part bought on the grid. Signals are signal
	// fragments not yet used, BoardKeys the board keystones bought,
	// GridNewest the last node bought and GridRingsSeen how many rings
	// were open the last time the GRID tab was looked at.
	Grid          *Grid
	Data          int
	Unbanked      int
	Own           []bool
	Seen          []bool
	Glimpse       []bool
	Reclaim       []bool
	Locker        []skill.Part
	Signals       int
	BoardKeys     int
	GridNewest    int
	GridRingsSeen int
	// GridKeep are cells that keep what they held on an older grid, from
	// before schools (see genGrid).
	GridKeep []KeptCell
	// GridPeek lifts the grid's fog while the GRID tab's TEST is lit. It's
	// a look only: never saved, and it marks nothing seen.
	GridPeek bool
	// gridDist is each node's steps from the nearest owned node,
	// recomputed when what you own changes.
	gridDist []int16

	// Kit is the starter kit picked (kit.go): 1 to KitDeal, 0 for the old
	// fixed opening. Draft is the draft on screen, and DraftedTo the
	// deepest floor whose draft has been taken (draft.go).
	Kit       int
	Draft     Draft
	DraftedTo int

	// SlotFlash counts down after a slot tries to fire; SlotLast is how.
	SlotFlash [MaxSlots]float32
	SlotLast  [MaxSlots]skill.Outcome

	// Stash is loot: parts on the floor and in the inventory (loot.go).
	Stash Stash
	// Sight is the fog of war: what the lamp lights now and what's been
	// explored on this floor (sight.go).
	Sight Sight
	// Benches are this floor's workbenches, the only places hexes can be
	// edited (bench.go).
	Benches [NumBenches]skill.Coord
	// Pads are the floor's repair pads (repair.go).
	Pads []Pad

	// Squads and the gunners' shots in the air (squad.go).
	squads [MaxSquads]squad
	Shots  [MaxShots]Shot
	NShots int

	// Stance is the stances on, one bit per skill.StanceKind (set each
	// frame from the loadout).
	Stance uint8
	// emitter is the device whose hex is running, so its bolts start from
	// it; dashFromX, dashFromY is where the last dash began.
	emitter  skill.EntityID
	emitting bool
	// lastHit is the foe you last hit yourself (not a device), at
	// lastHitAt, so a rare drone can go for another one.
	lastHit   skill.EntityID
	lastHitAt float64
	// kits are the drone guns compiled so far (see droneKit).
	kits                 map[kitKey]*skill.Program
	dashFromX, dashFromY float64

	occ      []skill.EntityID
	dist, q  []int32
	distFrom skill.Coord
	strikeCD float32
	dotClock float32
	hazClock float32
	safe     skill.Coord // last tile the player stood on that wasn't a pit
	pad      padState    // the repair pad being stood on (repair.go)
}

// New starts a run at depth 1, at the bottom of the teaching ladder.
func New(seed uint64) *Sim {
	s := &Sim{Reg: skill.DefaultRegistry(), Rules: skill.DefaultRules()}
	s.Eval = skill.NewEvaluator(s.Reg, s.Rules)
	s.NewRun(seed)
	return s
}

// NewVeteran starts a run at depth 1 with the ladder already done and
// the four preset hexes equipped, as runs began before the ladder. Tests
// of the deeper systems use it.
func NewVeteran(seed uint64) *Sim {
	s := &Sim{Reg: skill.DefaultRegistry(), Rules: skill.DefaultRules(), veteran: true}
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
	s.Army = nil
	s.Visit = 0
	s.Load = Loadout{}
	s.CallGiven = true // the preset Arc Lance carries it; the ladder hands its own
	s.Shapes, s.ShapeGiven, s.LastShape = nil, false, skill.Shape{}
	s.Frames, s.FrameGiven, s.LastFrame = nil, false, skill.Frame{}
	s.SustainGiven = false
	s.Tiles, s.LastTile = 0, 0
	s.Taught, s.Card, s.CardT = 1, 0, CardShow
	s.Kit, s.Draft, s.DraftedTo = 0, Draft{}, 0
	s.newGrid()
	if s.veteran {
		s.Taught, s.CardT = len(Ladder), 0
		s.SustainGiven, s.FrameGiven, s.ShapeGiven = true, true, true
	}
	s.resetStash()
	s.load(1, 1)
	s.Dirty = true
}

// floorSeed picks a depth's floor. It mixes in the death count, so a death
// rolls every floor below the checkpoint again: going back down is new
// ground, not a replay you can learn by heart. Reloading a save, or
// walking back down without dying, keeps the same floors.
func (s *Sim) floorSeed(depth int) uint64 {
	x := s.RunSeed + uint64(depth)*0x9e3779b97f4a7c15 + uint64(s.Deaths)*0xd1b54a32d192ed03
	x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
	return x ^ (x >> 31)
}

// nearestReachable is the walkable tile nearest c (as the crow flies)
// that can be reached from the start; if c is nowhere near the floor, a
// tile next to the start.
func (s *Sim) nearestReachable(c skill.Coord) skill.Coord {
	f := s.Floor
	for r := 0; r < f.W+f.H; r++ {
		best, bestD := f.Start, int32(-1)
		at := c
		for k := 0; k < r; k++ {
			at = at.Add(skill.HexDirs[4])
		}
		for side := 0; side < 6; side++ {
			for k := 0; k < max(r, 1); k++ {
				if f.In(at) {
					if d := s.dist[f.Index(at)]; d > 0 && (bestD < 0 || d < bestD) {
						best, bestD = at, d
					}
				}
				if r == 0 {
					break
				}
				at = at.Add(skill.HexDirs[side])
			}
			if r == 0 {
				break
			}
		}
		if bestD > 0 {
			return best
		}
	}
	// Nowhere near the floor at all: next to where you arrive.
	for _, d := range skill.HexDirs {
		if n := f.Start.Add(d); f.In(n) && s.dist[f.Index(n)] == 1 {
			return n
		}
	}
	return f.Start
}

// load builds the floor for depth. hpFrac carries the player's health
// share over from the floor above.
func (s *Sim) load(depth int, hpFrac float32) {
	if s.Visit > 0 {
		// Any way off a visited floor, stairs or a death, goes home.
		depth, hpFrac, s.Visit = s.Visit, 1, 0
	}
	s.Depth = depth
	if depth > s.Best && s.Best > 0 {
		s.earn(DataDepthBase + DataDepthStep*depth)
	}
	s.Best = max(s.Best, depth)
	s.growGrid()
	s.Visits++
	s.CheckpointT = 0
	if IsCheckpoint(depth) {
		s.secure()
		s.CheckpointT = CheckpointShow
	}
	s.enter(depth, hpFrac, s.floorSeed(depth))
}

// enter builds and starts the floor at depth from seed, with the player at
// hpFrac of full hull. load does the bookkeeping around it; a visit from
// the test menu skips that (visit.go).
func (s *Sim) enter(depth int, hpFrac float32, seed uint64) {
	s.Depth = depth
	s.Floor = s.Gen.Generate(seed, depth)
	f := s.Floor
	s.placeBenches()
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
	s.placePads()
	for _, sp := range f.Spawns {
		st := enemyTable[KindGrunt+Kind(sp.Kind)]
		id := s.spawn(KindGrunt+Kind(sp.Kind), 1, sp.At, sp.Level, st.hp*scale(sp.Level))
		s.Actors[id].Facing = uint8(hash(uint64(id)+f.Seed) % 6)
		s.Actors[id].Squad = uint8(sp.Room%(MaxSquads-1) + 1)
	}
	s.formSquads()
	// Room for devices without allocating mid-floor (devices.go).
	if cap(s.Actors)-len(s.Actors) < MaxExtra {
		grown := make([]Actor, len(s.Actors), len(s.Actors)+MaxExtra)
		copy(grown, s.Actors)
		s.Actors = grown
	}
	s.marchIn()
	s.Stance = 0
	s.Brace, s.braceT = 0, 0
	s.furyStreak = 0

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
	s.placeLesson()
	// The floor you died on is rolled again when you come back to it
	// (floorSeed), so the cache can sit in rock or out of reach on the
	// new one; move it to the nearest open floor to where you fell, so
	// it's still about as far in.
	if c := &s.Cache; c.Active && c.Depth == depth && (!f.In(c.At) || s.dist[f.Index(c.At)] <= 0) {
		c.At = s.nearestReachable(c.At)
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
	s.CardT = max(0, s.CardT-dt)
	switch s.State {
	case Dead:
		s.ageCorpses(dt)
		if s.Input.Restart && s.StateT > 0.5 {
			s.respawn()
		}
		return
	case Drafting:
		return // the world waits on the pick
	case Descending:
		s.ageCorpses(dt)
		if s.StateT >= DescendTime {
			p := s.P()
			s.Army = s.armyNow()
			s.load(s.Depth+1, p.U.HP/p.U.MaxHP+HealOnDescend)
		}
		return
	}

	s.stances()
	s.Disp.Update(dt)
	s.playerInput(dt)
	s.enemies(dt)
	s.extras(dt)
	s.stepShots(dt)
	s.advance(dt)
	s.timers(dt)
	s.pickup(dt)
	s.repair(dt)
	s.refreshSight()

	p := s.P()
	if !p.U.Alive {
		s.State, s.StateT = Dead, 0
		s.die()
		return
	}
	if p.Arrived() && s.Floor.At(p.At) == dungeon.Stairs {
		s.State, s.StateT = Descending, 0
		if s.Visit == 0 && s.openDraft() {
			s.State = Drafting
		}
	}
}

func (s *Sim) playerInput(dt float32) {
	p := s.P()
	in := &s.Input
	if in.Vent {
		heat := s.Rig.Heat
		if s.Rig.Vent() {
			s.ventNova(heat) // the Mage lets it go (mage.go)
			s.addFX(FX{Kind: FXVent, At: p.At, Life: 0.6})
			s.Disp.Emit(skill.Event{Trigger: skill.OnVent, Actor: s.Player, Subject: skill.NoEntity, Chain: 1, Slot: -1})
		}
	}
	s.Disp.Channel(s.Player, in.Hold && p.U.Alive)
	for slot, pressed := range in.Cast {
		if !pressed || s.Load.Progs[slot] == nil {
			continue
		}
		o := s.Disp.Press(s.Player, slot, s.nearestFoe(p.At, NearestRange))
		if o != skill.Fired {
			s.SlotFlash[slot], s.SlotLast[slot] = 0.4, o
		}
	}
	if l := s.Disp.Loadout(s.Player); l != nil && l.Held >= 0 && (!in.Held[l.Held] || !p.U.Alive) {
		slot := int(l.Held)
		if o := s.Disp.Release(s.Player, s.nearestFoe(p.At, NearestRange)); o != skill.Fired && slot < MaxSlots {
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

// claw is an enemy melee hit on the player: capped by level gap like any
// hit, but it doesn't build the player's resistance.
func (s *Sim) claw(attacker skill.EntityID, raw float32) {
	p := s.P()
	a := &s.Actors[attacker]
	turned := s.taunted(attacker, raw) * s.guard()
	block := max(0, raw-turned) // what a taunt and the bulwark turned aside
	raw = s.soak(turned)
	hit := min(raw, s.Rules.HitCap(a.U.Level, p.U.Level, p.U.MaxHP))
	left := p.U.Soak(hit) // a shield takes it first
	dealt := min(left, p.U.HP)
	p.U.HP -= dealt
	p.Flash = 0.2
	if p.U.HP <= 0 {
		p.U.HP, p.U.Alive = 0, false
	}
	if soaked := hit - left; soaked > 0 {
		s.addFX(FX{Kind: FXSoak, At: p.At, From: a.At, Value: soaked, Hurt: true, Mag: magOf(soaked, p.U.MaxHP), Life: 0.45})
	}
	if dealt > 0 {
		s.addFX(FX{Kind: FXNumber, At: p.At, Value: dealt, Hurt: true, Life: 0.8})
	}
	s.addFX(FX{Kind: FXClaw, At: p.At, From: a.At, Life: 0.25})
	if p.U.Alive {
		s.blocked(block + hit - left)
		s.thorns(attacker, hit)
		s.Disp.Hurt(s.Player, attacker)
	}
}

// timers runs statuses, resistance decay, damage over time and hazard
// vents for everyone.
func (s *Sim) timers(dt float32) {
	s.ageCorpses(dt)
	s.fadeBrace(dt)
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
		u.TickStealth(dt)
		u.TauntLeft = max(0, u.TauntLeft-dt)
		u.ThornsLeft = max(0, u.ThornsLeft-dt)
		u.HasteLeft = max(0, u.HasteLeft-dt)
		if a.ExposedT = max(0, a.ExposedT-dt); a.ExposedT == 0 {
			a.Exposed = 0
		}
		u.TickSustain(dt)
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
				if d.Owner == s.Player && d.Type == s.naniteType() && skill.EntityID(i) != s.Player {
					s.fury(dealt) // the Berserker's nanite feeds the rage (berserker.go)
				}
				if dealt > 0 && int(d.Owner) < len(s.Actors) && a.U.Team != s.Actors[d.Owner].U.Team {
					s.Actors[d.Owner].U.Reveal() // a burn or toxin ticking gives you away too
				}
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
	if a.extra() {
		// A minion or device going down is no kill and drops nothing.
		a.Used = true
		return
	}
	if id != s.Player {
		s.Kills++
		s.earnKill(id)
		s.dropLoot(id)
		s.wrecked(id)
		s.exposedDied(a)
		if !a.Fell && a.LastHit == s.necroType() {
			s.rise(id) // the Necromancer's dead get up (army.go)
		}
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
		if o == skill.NoEntity || !s.Actors[o].U.Alive || s.Actors[o].U.Team == a.U.Team || s.Actors[o].extra() {
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

// DevDepth drops straight onto a fresh floor at depth d, for screenshot
// tools. Play never calls it.
func (s *Sim) DevDepth(d int) { s.load(d, 1) }

// DevLayout loads depth d in layout l, counting deaths up until the floor
// rolls that layout, for the screenshot autopilot.
func (s *Sim) DevLayout(d int, l dungeon.Layout) {
	for k := 0; k < 200 && dungeon.LayoutFor(s.floorSeed(d), d) != l; k++ {
		s.Deaths++
	}
	s.load(d, 1)
}

// DevDraft opens the floor-end draft for depth d on that floor, for the
// screenshot autopilot.
func (s *Sim) DevDraft(d int) {
	s.load(d, 1)
	s.Draft = s.rollDraft(d)
	s.State, s.StateT = Drafting, 1
}

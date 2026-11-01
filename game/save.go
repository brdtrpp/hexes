package game

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"hexes/loot"
	"hexes/skill"
)

// Hexes is not a roguelike: progress is permanent. The rig, the depth
// reached, the pack and every hex edit live in a save that survives
// reloads. Floors stay temporary: each one is generated fresh when you
// arrive, and charge banks start empty on every floor.
//
// Death still costs something. You drop back a few floors (more the
// deeper you are, less if you died a moment ago), and the parts you picked
// up since your last checkpoint fall out of the pack into a salvage cache
// where you died. Walk back down to it and they're yours again; die before
// you get there and they're gone for good.
//
// The sim never touches storage. It sets Dirty when something worth
// keeping changes, the front end calls Save and writes the bytes where it
// keeps things (localStorage in the browser, a file on desktop).

// Death and checkpoint tuning. Placeholder balance, all constants.
const (
	SaveVersion     = 1
	CheckpointEvery = 5   // every 5th depth secures the pack
	FallMin         = 2   // floors lost on death, at least, before mercy
	FallFrac        = 0.1 // deep down you lose this share of your depth
	MercyWindow     = 300 // seconds of play after a death when the next costs less
	MercyMin        = 0.5 // a death right after another costs this share
	CheckpointShow  = 3   // seconds the checkpoint notice stays up
)

// Cache is the salvage a death leaves behind: the parts that fell out of
// the pack, lying on the floor where the player died.
type Cache struct {
	Active bool
	Depth  int
	At     skill.Coord
	N      int
	Parts  [loot.InventorySize]skill.Part
}

// Death is what the last death cost, for the death screen.
type Death struct {
	From, To int  // depth died on, depth you come back on
	Lost     int  // parts dropped in the new salvage cache
	Gone     int  // parts in an old cache that was never reached, now lost
	Mercy    bool // the cost was eased because the last death was recent
}

// FallFloors is how many floors a death at depth costs, sinceLast seconds
// of play after the previous one (negative for no previous death). Early
// on it's FallMin; deep down a tenth of the depth. A death inside the
// mercy window costs less, down to half for one right after another, and
// never less than a floor.
func FallFloors(depth int, sinceLast float64) (floors int, mercy bool) {
	base := max(FallMin, int(math.Round(float64(depth)*FallFrac)))
	f := 1.0
	if sinceLast >= 0 && sinceLast < MercyWindow {
		f = MercyMin + (1-MercyMin)*sinceLast/MercyWindow
		mercy = true
	}
	return max(1, int(math.Round(float64(base)*f))), mercy
}

// IsCheckpoint reports whether arriving at depth secures the pack.
func IsCheckpoint(depth int) bool { return depth%CheckpointEvery == 0 }

// die runs once, on the frame the player's hull hits zero: work out the
// fall, and move unsecured parts from the pack into a salvage cache.
func (s *Sim) die() {
	since := -1.0
	if s.Deaths > 0 {
		since = s.Time - s.LastDeathAt
	}
	fall, mercy := FallFloors(s.Depth, since)
	d := Death{From: s.Depth, To: max(1, s.Depth-fall), Mercy: mercy}
	if s.Cache.Active {
		d.Gone = s.Cache.N
	}
	s.Cache = Cache{}
	inv := &s.Stash.Inv
	for i := range inv.Has {
		if inv.Has[i] && s.Stash.Fresh[i] {
			s.Cache.Parts[s.Cache.N], _ = inv.Take(i)
			s.Cache.N++
		}
		s.Stash.Fresh[i] = false
	}
	if s.Cache.N > 0 {
		at, ok := s.dropSpot(s.P().At)
		if !ok {
			at = s.Floor.Start
		}
		s.Cache.Active, s.Cache.Depth, s.Cache.At = true, s.Depth, at
	}
	d.Lost = s.Cache.N
	s.LastDeath = d
	s.Deaths++
	s.LastDeathAt = s.Time
	s.Dirty = true
}

// respawn brings the player back after a death, on the depth die chose,
// at full hull.
func (s *Sim) respawn() {
	s.load(s.LastDeath.To, 1)
}

// secure marks everything in the pack as safe from the next death.
func (s *Sim) secure() {
	s.Stash.Fresh = [loot.InventorySize]bool{}
}

// Unsecured counts pack parts a death would drop into a salvage cache.
func (s *Sim) Unsecured() int {
	n := 0
	for i, f := range s.Stash.Fresh {
		if f && s.Stash.Inv.Has[i] {
			n++
		}
	}
	return n
}

// NextCheckpoint is the next depth that secures the pack.
func (s *Sim) NextCheckpoint() int {
	return (s.Depth/CheckpointEvery + 1) * CheckpointEvery
}

// salvage checks whether the player is standing on the salvage cache and,
// if so, puts its parts back in the pack. Parts that don't fit stay on the
// floor as ordinary drops. Recovered parts are still unsecured until the
// next checkpoint.
func (s *Sim) salvage(at skill.Coord) {
	c := &s.Cache
	if !c.Active || c.Depth != s.Depth || c.At != at {
		return
	}
	for _, p := range c.Parts[:c.N] {
		if i := s.Stash.Inv.Add(p); i >= 0 {
			s.Stash.Fresh[i] = true
			continue
		}
		if len(s.Stash.Drops) < cap(s.Stash.Drops) {
			s.Stash.Drops = append(s.Stash.Drops, Drop{At: at, Part: p, Age: 1})
		}
	}
	s.addFX(FX{Kind: FXSalvage, At: at, Value: float32(c.N), Life: 1.6})
	*c = Cache{}
	s.Dirty = true
}

// Save is everything that lasts between sessions. Floors, enemies, loot
// lying on the floor, charge banks and the rig's Power and Heat are not
// in it: they belong to one visit to one floor.
type Save struct {
	Version int
	RunSeed uint64
	Depth   int
	Best    int
	HPFrac  float32
	Kills   int
	Picked  int
	Visits  int
	Points  skill.Points

	Hexes  [MaxSlots]*skill.Hex
	Names  [MaxSlots]string
	Blurb  [MaxSlots]string
	Edited [MaxSlots]bool
	Inv    loot.Inventory
	Fresh  [loot.InventorySize]bool

	Cache       Cache
	Deaths      int
	LastDeathAt float64
	PlayTime    float64
	// CallGiven is set once the save has had its starter Call: in the
	// preset Arc Lance for a new run, in the pack for a save from before
	// Calls existed.
	CallGiven bool
}

// Snapshot is the current progress. While dead it already counts the
// death, so closing the game on the death screen can't dodge the fall.
func (s *Sim) Snapshot() Save {
	sv := Save{
		Version: SaveVersion, RunSeed: s.RunSeed, Depth: s.Depth, Best: s.Best,
		HPFrac: 1, Kills: s.Kills, Picked: s.Stash.Picked, Visits: s.Visits, Points: s.Points,
		Hexes: s.Load.Hexes, Names: s.Load.Names, Blurb: s.Load.Blurb, Edited: s.Load.Edited,
		Inv: s.Stash.Inv, Fresh: s.Stash.Fresh,
		Cache: s.Cache, Deaths: s.Deaths, LastDeathAt: s.LastDeathAt, PlayTime: s.Time,
		CallGiven: s.CallGiven,
	}
	switch s.State {
	case Dead:
		sv.Depth = s.LastDeath.To
	case Playing, Descending:
		p := s.P()
		sv.HPFrac = p.U.HP / p.U.MaxHP
	}
	return sv
}

// Save encodes the progress and clears Dirty.
func (s *Sim) Save() ([]byte, error) {
	b, err := json.Marshal(s.Snapshot())
	if err == nil {
		s.Dirty = false
	}
	return b, err
}

// LoadSave decodes a save and starts its floor. A save that won't decode or
// whose hexes don't compile leaves the sim as it was and returns an error.
func (s *Sim) LoadSave(b []byte) error {
	var sv Save
	if err := json.Unmarshal(b, &sv); err != nil {
		return err
	}
	return s.Restore(sv)
}

// Restore starts from a snapshot: the saved depth on a freshly generated
// floor, with the saved hull share, rig, pack and hexes.
func (s *Sim) Restore(sv Save) error {
	if sv.Version != SaveVersion {
		return fmt.Errorf("save version %d, want %d", sv.Version, SaveVersion)
	}
	if sv.Depth < 1 {
		return errors.New("save has no depth")
	}
	l := Loadout{Hexes: sv.Hexes, Names: sv.Names, Blurb: sv.Blurb, Edited: sv.Edited}
	for i, h := range l.Hexes {
		if h == nil {
			return fmt.Errorf("save is missing hex %d", i+1)
		}
	}
	rig := skill.NewRig(sv.Points)
	if err := l.Rebuild(s.Reg, sv.Depth, rig.MaxDepth); err != nil {
		return err
	}
	if sv.Cache.N < 0 || sv.Cache.N > loot.InventorySize {
		sv.Cache = Cache{}
	}
	s.RunSeed = sv.RunSeed
	s.Kills, s.Best, s.Visits, s.Points = sv.Kills, sv.Best, sv.Visits, sv.Points
	s.Load = l
	s.resetStash()
	s.Stash.Inv, s.Stash.Fresh, s.Stash.Picked = sv.Inv, sv.Fresh, sv.Picked
	s.Cache, s.Deaths, s.LastDeathAt, s.Time = sv.Cache, sv.Deaths, sv.LastDeathAt, sv.PlayTime
	s.LastDeath = Death{}
	s.CallGiven = sv.CallGiven
	dirty := s.giveCall()
	s.load(sv.Depth, sv.HPFrac)
	s.Dirty = dirty
	return nil
}

// giveCall puts a starter Call (at Nanite Swarm, hex 4) in the pack of a
// save made before Calls existed, once, unless one is already slotted or
// packed. It's safe from death like anything slotted. It reports whether
// the save changed.
func (s *Sim) giveCall() bool {
	if s.CallGiven {
		return false
	}
	have := false
	for _, h := range s.Load.Hexes {
		for _, sl := range h.Slots {
			have = have || (sl.Kind == skill.SlotPart && sl.Part.Kind == skill.KindCall)
		}
	}
	for i, p := range s.Stash.Inv.Parts {
		have = have || (s.Stash.Inv.Has[i] && p.Kind == skill.KindCall)
	}
	if !have {
		i := s.Stash.Inv.Add(skill.NewCall(StarterCallTarget))
		if i < 0 {
			return false // pack full: try again next load
		}
		s.Stash.Fresh[i] = false
	}
	s.CallGiven = true
	return true
}

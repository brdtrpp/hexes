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
	// SaveVersion 3 is the skill grid. (2 was the teaching ladder.) A
	// bump needs a migration step: see migrate.go.
	SaveVersion     = 3
	CheckpointEvery = 5   // every 5th depth secures the pack and banks Data
	FallMin         = 1   // floors lost on death, at least, before mercy
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
	// Data is the unbanked Data that fell out with the parts.
	Data int `json:",omitempty"`
}

// Death is what the last death cost, for the death screen.
type Death struct {
	From, To int  // depth died on, depth you come back on
	Lost     int  // parts dropped in the new salvage cache
	Gone     int  // parts in an old cache that was never reached, now lost
	Mercy    bool // the cost was eased because the last death was recent
	Data     int  // unbanked Data dropped in the new salvage cache
	DataGone int  // Data in an old cache that was never reached, now lost
	LetGo    int  // grid cells let go (they turn into reclaim cells)
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
	if s.Visit > 0 {
		// A death on a visit costs nothing and goes home.
		s.LastDeath = Death{From: s.Depth, To: s.Visit}
		s.Dirty = true
		return
	}
	since := -1.0
	if s.Deaths > 0 {
		since = s.Time - s.LastDeathAt
	}
	fall, mercy := FallFloors(s.Depth, since)
	d := Death{From: s.Depth, To: max(1, s.Depth-fall), Mercy: mercy}
	if s.Cache.Active {
		d.Gone, d.DataGone = s.Cache.N, s.Cache.Data
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
	s.Cache.Data, s.Unbanked = s.Unbanked, 0
	if s.Cache.N > 0 || s.Cache.Data > 0 {
		at, ok := s.dropSpot(s.P().At)
		if !ok {
			at = s.Floor.Start
		}
		s.Cache.Active, s.Cache.Depth, s.Cache.At = true, s.Depth, at
	}
	d.Lost, d.Data = s.Cache.N, s.Cache.Data
	d.LetGo = s.shrinkGrid(mercy)
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

// secure marks everything in the pack as safe from the next death, and
// banks the Data earned since the last checkpoint.
func (s *Sim) secure() {
	s.Stash.Fresh = [loot.InventorySize]bool{}
	s.bank()
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
	s.earn(c.Data + DataSalvage)
	s.addFX(FX{Kind: FXSalvage, At: at, Value: float32(c.N), Life: 1.6})
	s.addFX(FX{Kind: FXData, At: at, Value: float32(c.Data + DataSalvage), Life: 1.6})
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
	// Keys are the cast buttons (bench.go). Saves from before buttons
	// have none and hand them out in slot order.
	Keys  [MaxSlots]uint8 `json:",omitempty"`
	Inv   loot.Inventory
	Fresh [loot.InventorySize]bool

	Cache       Cache
	Deaths      int
	LastDeathAt float64
	PlayTime    float64
	// CallGiven is set once the save has had its starter Call: in the
	// preset Arc Lance for a new run, in the pack for a save from before
	// Calls existed.
	CallGiven bool
	// Shapes are the found shapes learned so far; ShapeGiven is set once
	// the first one is found. Saves from before shapes have neither and
	// load with every hex a Ring.
	Shapes     []skill.Shape `json:",omitempty"`
	ShapeGiven bool          `json:",omitempty"`
	// TestShapes is the test switch that unlocks every shape and frame.
	TestShapes bool `json:",omitempty"`
	// Frames is the frame rack; FrameGiven is set once the first frame is
	// found. Saves from before frames have neither, and every slot plain.
	Frames     []skill.Frame `json:",omitempty"`
	FrameGiven bool          `json:",omitempty"`
	// SustainGiven is set once a sustain part has been picked up. Saves
	// from before sustain parts don't have it, so the first Mend waits by
	// the stairs on their next floor.
	SustainGiven bool `json:",omitempty"`
	// Taught is how many rungs of the teaching ladder are done.
	Taught int
	// Mods and Board are the docking board's modules and where every hex
	// sits. Saves from before the board have neither and load with the
	// hexes laid out so nothing docks.
	Mods  []*skill.Hex `json:",omitempty"`
	Board []Spot       `json:",omitempty"`
	// Army is the Necromancer's raised bodies standing when it was saved
	// (army.go). Saves from before the army have none.
	Army []Raised `json:",omitempty"`
	// Tiles are the logic tiles found (see tiles.go).
	Tiles uint32 `json:",omitempty"`
	// Mine is the player's own build while an example is loaded, and
	// Example the example's name.
	Mine    *MyBuild `json:",omitempty"`
	Example string   `json:",omitempty"`

	// The skill grid. The grid itself rolls again from RunSeed; these
	// are what this save did with it, as node indexes.
	Data          int
	Unbanked      int          `json:",omitempty"`
	Own           []int        `json:",omitempty"`
	Seen          []int        `json:",omitempty"`
	Glimpse       []int        `json:",omitempty"`
	Reclaim       []int        `json:",omitempty"`
	Locker        []skill.Part `json:",omitempty"`
	Signals       int          `json:",omitempty"`
	BoardKeys     int          `json:",omitempty"`
	GridNewest    int          `json:",omitempty"`
	GridRingsSeen int          `json:",omitempty"`

	// Kit is the starter kit (kit.go); saves from before kits have none
	// and keep the old opening. DraftedTo is the deepest floor whose
	// draft was taken; saves from before drafts count every floor above
	// their best as drafted.
	Kit       int `json:",omitempty"`
	DraftedTo int `json:",omitempty"`
	// PartsV is the grid's part set when saved: GridPartsV once the class
	// parts are on it. Kept are the cells that keep what they held before
	// (owned when an older save first loaded).
	PartsV int   `json:",omitempty"`
	Kept   []int `json:",omitempty"`
	// Cells are the owned cells an older grid's save keeps as they were,
	// once the wedges became schools (PartsV 2).
	Cells []KeptCell `json:",omitempty"`
	// Pads is what the repair pads on floor PadsAt still hold, so a
	// reload can't refill one (repair.go).
	Pads   []float32 `json:",omitempty"`
	PadsAt int       `json:",omitempty"`
}

// marks lists the set entries of b.
func marks(b []bool) []int {
	var out []int
	for i, v := range b {
		if v {
			out = append(out, i)
		}
	}
	return out
}

// setMarks is b with the listed entries set; indexes off the end are
// dropped.
func setMarks(n int, idx []int) []bool {
	b := make([]bool, n)
	for _, i := range idx {
		if i >= 0 && i < n {
			b[i] = true
		}
	}
	return b
}

// Snapshot is the current progress. While dead it already counts the
// death, so closing the game on the death screen can't dodge the fall.
func (s *Sim) Snapshot() Save {
	sv := Save{
		Version: SaveVersion, RunSeed: s.RunSeed, Depth: s.Depth, Best: s.Best,
		HPFrac: 1, Kills: s.Kills, Picked: s.Stash.Picked, Visits: s.Visits, Points: s.Points,
		Hexes: s.Load.Hexes, Names: s.Load.Names, Blurb: s.Load.Blurb, Edited: s.Load.Edited, Keys: s.Load.Keys,
		Inv: s.Stash.Inv, Fresh: s.Stash.Fresh,
		Cache: s.Cache, Deaths: s.Deaths, LastDeathAt: s.LastDeathAt, PlayTime: s.Time,
		CallGiven: s.CallGiven,
		Shapes:    append([]skill.Shape(nil), s.Shapes...), ShapeGiven: s.ShapeGiven,
		TestShapes: s.TestShapes,
		Frames:     append([]skill.Frame(nil), s.Frames...), FrameGiven: s.FrameGiven,
		SustainGiven: s.SustainGiven,
		Taught:       s.Taught,
		Mods:         append([]*skill.Hex(nil), s.Load.Mods...),
		Board:        append([]Spot(nil), s.Load.Spots...),
		Tiles:        s.Tiles,
		Mine:         s.Mine, Example: s.Example,
		Data: s.Data, Unbanked: s.Unbanked,
		Own: marks(s.Own), Seen: marks(s.Seen), Glimpse: marks(s.Glimpse), Reclaim: marks(s.Reclaim),
		Locker:  append([]skill.Part(nil), s.Locker...),
		Signals: s.Signals, BoardKeys: s.BoardKeys, GridNewest: s.GridNewest, GridRingsSeen: s.GridRingsSeen,
		Kit: s.Kit, DraftedTo: s.DraftedTo,
		PartsV: GridPartsV, Cells: append([]KeptCell(nil), s.GridKeep...),
	}
	if s.Visit > 0 {
		// A visit is never saved: a reload is back home, at full hull.
		sv.Depth = s.Visit
		return sv
	}
	switch s.State {
	case Dead:
		sv.Depth = s.LastDeath.To
	case Playing, Descending, Drafting:
		p := s.P()
		sv.HPFrac = p.U.HP / p.U.MaxHP
		if s.State == Playing {
			sv.Pads, sv.PadsAt = s.padsLeft(), s.Depth
		}
		sv.Army = s.armyNow()
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
	b, err := Upgrade(b)
	if err != nil {
		return err
	}
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
	shapes := sv.Shapes[:0:0]
	for _, sh := range sv.Shapes {
		if sh.Valid() && sh.ID != 0 && len(shapes) < MaxShapes {
			shapes = append(shapes, sh)
		}
	}
	frames := sv.Frames[:0:0]
	for _, f := range sv.Frames {
		if f.Valid() && !f.Plain() && f.Takes == skill.NewFrame(f.Bend).Takes && !IsTestFrame(f) && len(frames) < MaxFrames {
			frames = append(frames, f)
		}
	}
	// Cloak is a self effect now: an old cloak damage part becomes the
	// cloak it reads as, so nothing is lost.
	migrated := selfOnly(s.Reg, &sv)
	for _, h := range sv.Hexes {
		cleanFrames(h, sv.TestShapes)
	}
	for _, m := range sv.Mods {
		cleanFrames(m, sv.TestShapes)
	}
	if sv.Mine != nil {
		for _, h := range sv.Mine.Hexes {
			cleanFrames(h, sv.TestShapes)
		}
		for _, m := range sv.Mine.Mods {
			cleanFrames(m, sv.TestShapes)
		}
	}
	l := Loadout{Hexes: sv.Hexes, Names: sv.Names, Blurb: sv.Blurb, Edited: sv.Edited, Spots: sv.Board, Keys: sv.Keys}
	for i, h := range l.Hexes {
		if h == nil {
			return fmt.Errorf("save is missing hex %d", i+1)
		}
	}
	for _, m := range sv.Mods {
		if m != nil && m.Shape.Valid() && len(l.Mods) < MaxMods {
			l.Mods = append(l.Mods, m)
		}
	}
	l.FixSpots()
	rig := skill.NewRig(sv.Points)
	l.Undocked = !sv.TestShapes && ladderUnlocks(sv.Taught)&UBoard == 0
	if err := l.Rebuild(s.Reg, sv.Depth, rig.MaxDepth, -1); err != nil {
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
	s.Visit = 0
	s.CallGiven = sv.CallGiven
	s.Shapes, s.ShapeGiven = shapes, sv.ShapeGiven || len(shapes) > 0
	s.TestShapes = sv.TestShapes
	s.Frames, s.FrameGiven = frames, sv.FrameGiven || len(frames) > 0
	s.SustainGiven = sv.SustainGiven
	s.Taught = min(len(Ladder), max(1, sv.Taught))
	s.Card, s.CardT = 0, 0
	s.Tiles = sv.Tiles & skill.AllTiles
	s.Mine, s.Example = nil, ""
	if sv.Mine != nil {
		if m, err := copyBuild(sv.Mine); err == nil {
			s.Mine, s.Example = m, sv.Example
		}
	}
	// A save from before schools keeps every cell it owns as it was on
	// its old grid, so what you bought still reads the same.
	s.GridKeep = append([]KeptCell(nil), sv.Cells...)
	if sv.PartsV < GridPartsV {
		kept := sv.Kept
		if sv.PartsV < 1 { // from before the class parts too
			kept = nil
			for _, i := range sv.Own {
				if i >= RingStart(ClassRing) {
					kept = append(kept, i)
				}
			}
		}
		old := genGridV1(s.Reg, s.RunSeed, RingsFor(s.Best), kept)
		s.GridKeep = nil
		for _, i := range sv.Own {
			if i > 0 && i < len(old.Nodes) && old.Nodes[i].Kind != NodeVoid {
				s.GridKeep = append(s.GridKeep, KeptCell{I: i, N: old.Nodes[i]})
			}
		}
	}
	s.Grid = s.genGrid(RingsFor(s.Best))
	n := len(s.Grid.Nodes)
	s.Own, s.Seen = setMarks(n, sv.Own), setMarks(n, sv.Seen)
	s.Glimpse, s.Reclaim = setMarks(n, sv.Glimpse), setMarks(n, sv.Reclaim)
	s.Own[0], s.Seen[0] = true, true
	for i := range s.Own {
		if s.Own[i] || s.Grid.Nodes[i].Kind == NodeVoid {
			s.Reclaim[i] = false
		}
	}
	s.Data, s.Unbanked = max(0, sv.Data), max(0, sv.Unbanked)
	s.Locker = append(s.Locker[:0], sv.Locker...)
	s.Signals, s.BoardKeys = max(0, sv.Signals), max(0, min(BoardCells, sv.BoardKeys))
	s.GridNewest, s.GridRingsSeen = sv.GridNewest, sv.GridRingsSeen
	if s.GridNewest < 0 || s.GridNewest >= n {
		s.GridNewest = 0
	}
	if sv.Cache.Data < 0 {
		sv.Cache.Data = 0
	}
	// TEST used to open every ring, so a save may hold cells marked seen
	// on rings that aren't open yet. Those go back under the fog; cells you
	// own stay yours.
	for i := range s.Seen {
		if s.Seen[i] && !s.Own[i] && !s.RingOpen(s.Grid.Nodes[i].Ring) {
			s.Seen[i] = false
		}
	}
	s.refreshGrid()
	// A save made while TEST opened every ring counted them all as seen;
	// count only the rings that are really open, so a new one still pulses.
	s.GridRingsSeen = min(max(0, s.GridRingsSeen), s.RingsOpen())
	s.GridPeek = false
	s.Kit = sv.Kit
	if s.Kit < 0 || s.Kit > KitDeal {
		s.Kit = 0
	}
	s.DraftedTo = max(sv.DraftedTo, sv.Best-1)
	s.Draft = Draft{}
	dirty := s.giveCall() || migrated
	s.Army = sv.Army
	if len(s.Army) > skill.MinionMax {
		s.Army = s.Army[:skill.MinionMax]
	}
	s.load(sv.Depth, sv.HPFrac)
	s.restorePads(sv.PadsAt, sv.Pads)
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

// selfOnly turns every cloak damage part a save holds into the cloak it
// is now (skill.Registry.SelfOnly): in the hexes and modules, your own
// build under an example, the pack, a death's salvage and the locker.
// Nothing is dropped. It also gives a drone part from before drones
// carried their own guns a gun of its own (skill.DroneKit), so two old
// drones needn't shoot alike. It reports whether anything changed.
func selfOnly(reg *skill.Registry, sv *Save) bool {
	changed := false
	n := uint32(0)
	part := func(p *skill.Part) {
		if q, ok := reg.SelfOnly(*p); ok {
			*p, changed = q, true
		}
		if p.Kind == skill.KindForm && p.Form == skill.FormDrone && p.Seed == 0 {
			n++
			p.Seed, changed = skill.DroneSeed(uint64(sv.RunSeed)+uint64(n)*0x9e3779b97f4a7c15), true
		}
	}
	seen := map[*skill.Hex]bool{}
	var hex func(h *skill.Hex)
	hex = func(h *skill.Hex) {
		if h == nil || seen[h] {
			return
		}
		seen[h] = true
		for i := range h.Slots {
			part(&h.Slots[i].Part)
			hex(h.Slots[i].Nested)
		}
	}
	for _, h := range sv.Hexes {
		hex(h)
	}
	for _, m := range sv.Mods {
		hex(m)
	}
	if sv.Mine != nil {
		for _, h := range sv.Mine.Hexes {
			hex(h)
		}
		for _, m := range sv.Mine.Mods {
			hex(m)
		}
	}
	for i := range sv.Inv.Parts {
		part(&sv.Inv.Parts[i])
	}
	for i := range sv.Cache.Parts {
		part(&sv.Cache.Parts[i])
	}
	for i := range sv.Locker {
		part(&sv.Locker[i])
	}
	return changed
}

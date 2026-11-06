package game

import (
	"errors"
	"fmt"

	"hexes/loot"
	"hexes/skill"
)

// Loot in the sim: enemies drop parts when they die, the parts lie on the
// floor until the player walks over them, and the player swaps them into
// hexes from the editor. The drop and pickup paths run inside Step and
// don't allocate; the edit calls run while the game is paused and
// recompile the hex they touch.

// Drop tuning. Placeholder balance, all constants. The grid decides what
// you can build, so drops are rare extras: always a kind the grid has
// given you, sometimes a rarity above your best. Where a part used to drop
// and now doesn't, a Data shard does.
const (
	DropGrunt   = 0.06 // chance a grunt drops a part
	DropSkitter = 0.04
	DropBrute   = 0.11
	DropGunner  = 0.06
	DropElite   = 0.30 // a foe above the floor's level
	DropBump    = 0.15 // chance a drop is a rarity above your best grid part
	MaxDrops    = 96   // parts that can lie on one floor; more are lost

	// The old chances: a roll between the new chance and these is a shard.
	ShardGrunt   = 0.30
	ShardSkitter = 0.22
	ShardBrute   = 0.55
	ShardGunner  = 0.32
	ShardElite   = 0.75
)

var dropChance = [NumKinds]float32{KindGrunt: DropGrunt, KindSkitter: DropSkitter, KindBrute: DropBrute, KindGunner: DropGunner}
var shardChance = [NumKinds]float32{KindGrunt: ShardGrunt, KindSkitter: ShardSkitter, KindBrute: ShardBrute, KindGunner: ShardGunner}

// Drop is a part lying on the floor.
type Drop struct {
	At   skill.Coord
	Part skill.Part
	Age  float32 // seconds on the floor, for the pop-in
}

// Stash is the run's loot.
type Stash struct {
	Inv   loot.Inventory
	Drops []Drop
	// Shapes are shapes lying on the floor (shape.go).
	Shapes []ShapeDrop
	// Tiles are logic tiles lying on the floor (tiles.go).
	Tiles []TileDrop
	// Lenses are Optics upgrades lying on the floor (lens.go).
	Lenses []LensDrop
	// Frames are slot frames lying on the floor (frames.go).
	Frames []FrameDrop
	// Lesson is the teaching ladder's pickup, when one waits here
	// (teach.go).
	Lesson LessonDrop
	// Picked counts parts ever picked up.
	Picked int
	// Fresh marks pack cells holding a part picked up since the last
	// checkpoint. A death drops those into the salvage cache. Parts
	// slotted into a hex are always safe.
	Fresh [loot.InventorySize]bool

	table    *loot.Table
	fullAt   skill.Coord // tile where the "full" warning last showed
	fullShow bool
}

func (s *Sim) resetStash() {
	if s.Stash.table == nil {
		s.Stash.table = loot.NewTable(s.Reg)
		s.Stash.Drops = make([]Drop, 0, MaxDrops)
		s.Stash.Shapes = make([]ShapeDrop, 0, MaxShapeDrops)
		s.Stash.Tiles = make([]TileDrop, 0, MaxTileDrops)
		s.Stash.Lenses = make([]LensDrop, 0, MaxLensDrops)
		s.Stash.Frames = make([]FrameDrop, 0, MaxFrameDrops+1)
	}
	s.Stash.Lenses = s.Stash.Lenses[:0]
	s.Stash.Frames = s.Stash.Frames[:0]
	s.Stash.Shapes = s.Stash.Shapes[:0]
	s.Stash.Tiles = s.Stash.Tiles[:0]
	s.Stash.Inv.Clear()
	s.Stash.Fresh = [loot.InventorySize]bool{}
	s.Stash.Drops = s.Stash.Drops[:0]
	s.Stash.Picked = 0
	s.Stash.fullShow = false
}

// loadLoot runs when a floor loads: parts left on the last floor are gone,
// and the run's hexes (preset on the first floor) compile for the new
// level.
func (s *Sim) loadLoot(level int) {
	s.Stash.Drops = s.Stash.Drops[:0]
	s.Stash.Shapes = s.Stash.Shapes[:0]
	s.Stash.Tiles = s.Stash.Tiles[:0]
	s.Stash.Lenses = s.Stash.Lenses[:0]
	s.Stash.Frames = s.Stash.Frames[:0]
	s.Stash.fullShow = false
	s.Stash.Lesson = LessonDrop{}
	if s.Load.Hexes[0] == nil {
		if s.Learned() {
			s.Load = BuildLoadout(s.Reg, level, s.Rig.MaxDepth)
		} else {
			s.Load = LadderLoadout(s.Reg, s.curKit(), level, s.Rig.MaxDepth)
		}
		return
	}
	if err := s.Load.Rebuild(s.Reg, level, s.Rig.MaxDepth, -1); err != nil {
		// Every hex compiled when it was last edited; only a lower
		// Processing limit could break one, and the slice's rig is fixed.
		panic(err)
	}
}

// dropLoot rolls a dying enemy's drop. The roll is seeded by the floor,
// the visit and the enemy, so one visit drops the same parts whatever
// order things die in, and coming back to a floor rolls new ones. Shapes
// and frames come from the grid now, so they no longer drop.
func (s *Sim) dropLoot(id skill.EntityID) {
	a := &s.Actors[id]
	if int(a.Kind) >= len(dropChance) {
		return
	}
	chance, shard := dropChance[a.Kind], shardChance[a.Kind]
	elite := a.U.Level > s.Depth
	if elite {
		chance, shard = DropElite, ShardElite
	}
	if at, ok := s.dropSpot(a.At); ok {
		s.rollTileDrop(id, at, elite)
		s.rollLensDrop(id, at, elite)
	}
	r := loot.NewRNG(loot.Seed(s.Floor.Seed^hash(uint64(s.Visits)), int(id)))
	x := r.Float()
	if x >= chance {
		if x < shard {
			n := ShardMin + r.Intn(ShardMx-ShardMin+1)
			s.earn(n)
			s.addFX(FX{Kind: FXData, At: a.At, Value: float32(n), Life: 1.0, Small: true})
		}
		return
	}
	if len(s.Stash.Drops) == cap(s.Stash.Drops) {
		return
	}
	allow := s.lootAllow()
	kinds, sustain, best := s.gridKinds()
	if !allow.Limit {
		allow = loot.Allow{Limit: true, Kinds: 0xffff, Sustain: true, Spend: true, Quirks: true, New: allow.New}
	}
	allow.Kinds &= kinds
	allow.Sustain = allow.Sustain && sustain
	s.Stash.table.Allow = allow
	rar := best
	if r.Float() < DropBump {
		rar = min(skill.Exotic, rar+1)
	}
	if rar == skill.Exotic && !allow.Quirks {
		rar = skill.Rare
	}
	p := s.schoolRoll(&r, s.OpenSchools(), func() skill.Part { return s.Stash.table.RollTaught(&r, rar) }, nil)
	at, ok := s.dropSpot(a.At)
	if !ok {
		return
	}
	s.Stash.Drops = append(s.Stash.Drops, Drop{At: at, Part: p})
}

// dropSpot is where a part lands: where the foe died, or next to it if
// that tile can't be walked on (a foe that fell into a pit drops on the
// rim).
func (s *Sim) dropSpot(c skill.Coord) (skill.Coord, bool) {
	if s.Floor.At(c).Walkable() {
		return c, true
	}
	for _, d := range skill.HexDirs {
		if n := c.Add(d); s.Floor.At(n).Walkable() {
			return n, true
		}
	}
	return c, false
}

// pickup moves parts under the player into the inventory.
func (s *Sim) pickup(dt float32) {
	p := s.P()
	at := p.At
	if s.Stash.fullShow && s.Stash.fullAt != at {
		s.Stash.fullShow = false
	}
	d := s.Stash.Drops
	for i := 0; i < len(d); {
		d[i].Age += dt
		if d[i].At != at || !p.U.Alive {
			i++
			continue
		}
		cell := s.Stash.Inv.Add(d[i].Part)
		if cell < 0 {
			if !s.Stash.fullShow {
				s.Stash.fullShow, s.Stash.fullAt = true, at
				s.addFX(FX{Kind: FXFull, At: at, Life: 1.2})
			}
			i++
			continue
		}
		s.Stash.Picked++
		s.Stash.Fresh[cell] = true
		s.LastLoot, s.LootN = d[i].Part, s.LootN+1
		if IsSustain(d[i].Part) {
			s.SustainGiven = true
		}
		s.Dirty = true
		s.addFX(FX{Kind: FXLoot, At: at, Slot: int(d[i].Part.Rarity), Life: 1.0})
		d[i] = d[len(d)-1]
		d = d[:len(d)-1]
	}
	s.Stash.Drops = d
	if p.U.Alive {
		s.salvage(at)
		s.pickupShapes(at, dt)
		s.pickupTiles(at, dt)
		s.pickupLenses(at, dt)
		s.pickupFrames(at, dt)
		s.pickupLesson(at, dt)
	}
}

// Edits. Each one checks the sockets, applies the change, recompiles the
// hex and swaps the new program into the dispatcher. A change that won't
// compile is undone and its error says why in plain words.

// ErrNeedsTrigger is returned for an edit that would leave a hex without
// its trigger.
var ErrNeedsTrigger = errors.New("a hex needs a trigger in its top slot")

// SwapIn trades the part in a hex slot with an inventory cell. Either side
// may be empty: an empty cell takes the slot's part out, an empty slot
// takes the cell's part in.
func (s *Sim) SwapIn(hex, slot, cell int) error {
	if !s.Load.Has(hex) || cell < 0 || cell >= loot.InventorySize || !s.Load.Hex(hex).Shape.Has(slot) {
		return errors.New("no such slot")
	}
	h := s.Load.Hex(hex)
	inv := &s.Stash.Inv
	old := h.Slots[slot]
	if old.Kind == skill.SlotNested {
		return errors.New("that slot holds a nested hex")
	}
	in, has, fresh := inv.Parts[cell], inv.Has[cell], s.Stash.Fresh[cell]
	if !has && old.Kind == skill.SlotEmpty {
		return nil
	}
	if has {
		if err := fits(h, slot, in, s.Load.IsMod(hex), s.Reg); err != nil {
			return err
		}
		if what, ok := s.PreviewIn(hex, slot, cell); !ok {
			return useless(h, slot, what)
		}
		h.Slots[slot] = skill.Slot{Kind: skill.SlotPart, Part: in}
	} else {
		if old.Part.Kind == skill.KindTrigger {
			return ErrNeedsTrigger
		}
		h.Clear(slot)
	}
	if old.Kind == skill.SlotPart {
		inv.Put(cell, old.Part)
	} else {
		inv.Take(cell)
	}
	// Whatever comes out of a hex into the pack was already safe, and a
	// part that goes into a hex is secured by slotting it.
	s.Stash.Fresh[cell] = false
	if err := s.recompile(hex); err != nil {
		h.Slots[slot] = old
		inv.Parts[cell], inv.Has[cell] = in, has
		s.Stash.Fresh[cell] = fresh
		return err
	}
	return nil
}

// SwapSlots trades two slots of one hex, which rewires it: order and
// neighbors are the whole game.
func (s *Sim) SwapSlots(hex, a, b int) error {
	if !s.Load.Has(hex) || !s.Load.Hex(hex).Shape.Has(a) || !s.Load.Hex(hex).Shape.Has(b) {
		return errors.New("no such slot")
	}
	if a == b {
		return nil
	}
	h := s.Load.Hex(hex)
	sa, sb := h.Slots[a], h.Slots[b]
	if sa.Kind == skill.SlotNested || sb.Kind == skill.SlotNested {
		return errors.New("that slot holds a nested hex")
	}
	if sa.Kind == skill.SlotPart {
		if err := fits(h, b, sa.Part, s.Load.IsMod(hex), s.Reg); err != nil {
			return err
		}
	}
	if sb.Kind == skill.SlotPart {
		if err := fits(h, a, sb.Part, s.Load.IsMod(hex), s.Reg); err != nil {
			return err
		}
	}
	if what, ok := s.PreviewSwap(hex, a, b); !ok {
		to := b
		if sa.Kind != skill.SlotPart {
			to = a
		}
		return useless(h, to, what)
	}
	h.Slots[a], h.Slots[b] = sb, sa
	if err := s.recompile(hex); err != nil {
		h.Slots[a], h.Slots[b] = sa, sb
		return err
	}
	return nil
}

// PreviewIn says what the part in pack cell would do in a hex slot, and
// whether it would do anything there (see skill.Hex.Use). A part only goes
// where it does something, so the editor lights those slots.
func (s *Sim) PreviewIn(hex, slot, cell int) (what string, ok bool) {
	if !s.Load.Has(hex) || cell < 0 || cell >= loot.InventorySize || !s.Stash.Inv.Has[cell] {
		return "", false
	}
	h := s.Load.Hex(hex)
	p := s.Stash.Inv.Parts[cell]
	if !h.Shape.Has(slot) || h.Slots[slot].Kind == skill.SlotNested {
		return "no slot there", false
	}
	if err := fits(h, slot, p, s.Load.IsMod(hex), s.Reg); err != nil {
		return err.Error(), false
	}
	cp := *h
	cp.Slots[slot] = skill.Slot{Kind: skill.SlotPart, Part: p}
	return cp.Use(slot)
}

// PreviewSwap says what the part in slot a would do moved to slot b (the
// part in b going to a), or, with a empty, what b's part would do in a.
func (s *Sim) PreviewSwap(hex, a, b int) (what string, ok bool) {
	if !s.Load.Has(hex) {
		return "", false
	}
	h := s.Load.Hex(hex)
	if !h.Shape.Has(a) || !h.Shape.Has(b) || h.Slots[a].Kind == skill.SlotNested || h.Slots[b].Kind == skill.SlotNested {
		return "no slot there", false
	}
	sa, sb := h.Slots[a], h.Slots[b]
	if sa.Kind == skill.SlotPart {
		if err := fits(h, b, sa.Part, s.Load.IsMod(hex), s.Reg); err != nil {
			return err.Error(), false
		}
	}
	if sb.Kind == skill.SlotPart {
		if err := fits(h, a, sb.Part, s.Load.IsMod(hex), s.Reg); err != nil {
			return err.Error(), false
		}
	}
	cp := *h
	cp.Slots[a], cp.Slots[b] = sb, sa
	if sa.Kind == skill.SlotPart {
		return cp.Use(b)
	}
	return cp.Use(a)
}

func useless(h *skill.Hex, slot int, what string) error {
	name := "slot " + h.SlotName(slot)
	if name == "slot C" {
		name = "the center"
	}
	return fmt.Errorf("it would do nothing in %s: %s", name, what)
}

// CycleBank moves a charge part or charge gate in a hex slot to the next
// bank letter (A, B, C, D, then A again). It reports false when the slot
// holds no such part. The counts in the banks stay where they are.
func (s *Sim) CycleBank(hex, slot int) bool {
	if !s.Load.Has(hex) || slot < 0 || slot >= skill.Slots {
		return false
	}
	h := s.Load.Hex(hex)
	sl := &h.Slots[slot]
	if sl.Kind != skill.SlotPart || !sl.Part.UsesBank() {
		return false
	}
	old := sl.Part
	sl.Part = old.WithBank(old.Bank + 1)
	if err := s.recompile(hex); err != nil {
		sl.Part = old
		return false
	}
	return true
}

// CycleCall points a Call part in a hex slot at the next hex: itself,
// then each of the other equipped hexes (run later), then each library
// hex on the board by name (run now). Targets that wouldn't compile, like
// a library that leads back to this hex, are skipped. It reports false
// when the slot holds no Call or nothing else fits.
func (s *Sim) CycleCall(hex, slot int) bool {
	if !s.Load.Has(hex) || slot < 0 || slot >= skill.Slots {
		return false
	}
	h := s.Load.Hex(hex)
	sl := &h.Slots[slot]
	if sl.Kind != skill.SlotPart || sl.Part.Kind != skill.KindCall {
		return false
	}
	old := sl.Part
	opts := callTargets(s.Load, hex)
	cur := 0
	for i, o := range opts {
		if o.Lib == old.Lib && (o.Lib != "" || o.Call == old.Call) {
			cur = i
		}
	}
	for k := 1; k < len(opts); k++ {
		o := opts[(cur+k)%len(opts)]
		sl.Part.Call, sl.Part.Lib = o.Call, o.Lib
		if err := s.recompile(hex); err == nil {
			return true
		}
	}
	sl.Part = old
	_ = s.recompile(hex)
	return false
}

// CycleWatch points a watch trigger at the next thing it can watch: its
// own condition (Lib ""), then each library hex on the board that only
// picks and tests foes. It reports whether the slot held a watch and what
// it watches now.
func (s *Sim) CycleWatch(hex, slot int) (lib string, ok bool) {
	if !s.Load.Has(hex) || slot < 0 || slot >= skill.Slots {
		return "", false
	}
	sl := &s.Load.Hex(hex).Slots[slot]
	if sl.Kind != skill.SlotPart || sl.Part.Kind != skill.KindTrigger || sl.Part.Trigger != skill.Watch {
		return "", false
	}
	old := sl.Part
	opts := append([]string{""}, s.Load.LibNames()...)
	cur := 0
	for i, o := range opts {
		if o == old.Lib {
			cur = i
		}
	}
	for k := 1; k <= len(opts); k++ {
		sl.Part.Lib = opts[(cur+k)%len(opts)]
		if s.recompile(hex) == nil {
			return sl.Part.Lib, true
		}
	}
	sl.Part = old
	_ = s.recompile(hex)
	return old.Lib, true
}

// callTarget is one place a Call can point: an equipped hex by number
// (run later) or a library hex by name (run now).
type callTarget struct {
	Call uint8
	Lib  string
}

func callTargets(l Loadout, hex int) []callTarget {
	out := []callTarget{{Call: skill.CallSelf}}
	for n := 1; n <= MaxSlots; n++ {
		if n != hex+1 {
			out = append(out, callTarget{Call: uint8(n)})
		}
	}
	for i, name := range l.LibNames() {
		if MaxSlots+i != hex {
			out = append(out, callTarget{Lib: name})
		}
	}
	return out
}

// CallsTo is the hex a Call part points at: its own index for a self
// call, or another slot. ok is false for a target past the loadout.
func CallsTo(hex int, p skill.Part) (target int, ok bool) {
	if p.Call == skill.CallSelf {
		return hex, true
	}
	t := int(p.Call) - 1
	return t, t < MaxSlots
}

// MoveCell trades two inventory cells.
func (s *Sim) MoveCell(a, b int) {
	if a < 0 || b < 0 || a >= loot.InventorySize || b >= loot.InventorySize {
		return
	}
	v := &s.Stash.Inv
	v.Parts[a], v.Parts[b] = v.Parts[b], v.Parts[a]
	v.Has[a], v.Has[b] = v.Has[b], v.Has[a]
	f := &s.Stash.Fresh
	f[a], f[b] = f[b], f[a]
	s.Dirty = true
}

// Scrap throws away the part in an inventory cell for DataScrap Data.
func (s *Sim) Scrap(cell int) bool {
	_, ok := s.Stash.Inv.Take(cell)
	if ok {
		s.Stash.Fresh[cell] = false
		s.earn(DataScrap)
		s.Dirty = true
	}
	return ok
}

func fits(h *skill.Hex, slot int, p skill.Part, mod bool, reg *skill.Registry) error {
	if mod && p.Kind == skill.KindTrigger {
		return errors.New("a module has no trigger: it runs when a hex docks into it")
	}
	if h.Sockets[slot].Accepts(p.Kind) {
		// The slot's frame has its say too (frames.go).
		if ok, why := h.Fits(slot, p, reg); !ok {
			return errors.New("that slot's" + why[len("its"):])
		}
		return nil
	}
	where := "top"
	switch {
	case h.Shape.Kind == skill.ShapeFork:
		where = "center"
	case h.Shape.Kind == skill.ShapeGen:
		where = "first"
	}
	switch {
	case p.Kind == skill.KindTrigger:
		return fmt.Errorf("triggers only go in the %s slot", where)
	case h.Sockets[slot] == skill.SocketTrigger:
		return fmt.Errorf("the %s slot only takes triggers", where)
	case slot == skill.Center && h.Shape.Kind == skill.ShapeHub && (p.Kind == skill.KindTarget || p.Kind == skill.KindGate):
		return errors.New("the center pools the ring: no targets or gates there")
	}
	return errors.New("that part doesn't fit this slot")
}

// recompile rebuilds every hex after an edit to hex (docks reach across
// hexes) and hands the programs to the dispatcher. A new periodic trigger
// starts a full period from now.
func (s *Sim) recompile(hex int) error {
	mod := s.Load.IsMod(hex)
	was := false
	if !mod {
		was = s.Load.Edited[hex]
		s.Load.Edited[hex] = true
	}
	if err := s.rebuildAll(hex); err != nil {
		if !mod {
			s.Load.Edited[hex] = was
			if h := s.Load.Hexes[hex]; h.Slots[h.Shape.Entry()].Kind == skill.SlotEmpty {
				return ErrNeedsTrigger
			}
		}
		return err
	}
	return nil
}

// HexPower is what a hex costs per firing, for the editor.
// A module's is its own parts', as if a plain cast fired it.
func (s *Sim) HexPower(hex int) (power, heat float32) {
	if s.Load.IsMod(hex) {
		p, err := skill.CompileModule(s.Load.Hex(hex), s.Reg, s.Rig.MaxDepth)
		if err != nil {
			return 0, 0
		}
		return p.Power, p.Heat
	}
	p := s.Load.Progs[hex]
	return p.Power, p.Heat
}

// AddOpt moves the multiplier in pack cell onto the run-now Call in a
// hex slot as an option. It fails, changing nothing, when the slot holds
// no run-now call, the cell holds no multiplier, or the call is full.
func (s *Sim) AddOpt(hex, slot, cell int) error {
	if !s.Load.Has(hex) || slot < 0 || slot >= skill.Slots || cell < 0 || cell >= loot.InventorySize {
		return errors.New("no such slot")
	}
	sl := &s.Load.Hex(hex).Slots[slot]
	inv := &s.Stash.Inv
	if sl.Kind != skill.SlotPart || !sl.Part.RunsNow() {
		return errors.New("options go on a call to a library hex")
	}
	if !inv.Has[cell] || inv.Parts[cell].Kind != skill.KindMult {
		return errors.New("only a multiplier rides on a call")
	}
	old := sl.Part
	p, ok := old.WithOpt(inv.Parts[cell])
	if !ok {
		return fmt.Errorf("a call takes %d options at most", skill.MaxOpts)
	}
	m, fresh := inv.Parts[cell], s.Stash.Fresh[cell]
	sl.Part = p
	inv.Take(cell)
	s.Stash.Fresh[cell] = false
	if err := s.recompile(hex); err != nil {
		sl.Part = old
		inv.Put(cell, m)
		s.Stash.Fresh[cell] = fresh
		return err
	}
	return nil
}

// TakeOpt puts option i of the call in a hex slot back in the pack.
func (s *Sim) TakeOpt(hex, slot, i int) error {
	if !s.Load.Has(hex) || slot < 0 || slot >= skill.Slots {
		return errors.New("no such slot")
	}
	sl := &s.Load.Hex(hex).Slots[slot]
	if sl.Kind != skill.SlotPart || sl.Part.Kind != skill.KindCall {
		return errors.New("no call there")
	}
	inv := &s.Stash.Inv
	if inv.Full() {
		return errors.New("your pack is full")
	}
	old := sl.Part
	p, m, ok := old.WithoutOpt(i)
	if !ok {
		return errors.New("no such option")
	}
	sl.Part = p
	if err := s.recompile(hex); err != nil {
		sl.Part = old
		return err
	}
	inv.Add(m)
	return nil
}

package game

import "hexes/skill"

// Repair pads: where the hull gets patched between fights.
//
// A floor gets a pad for every few hundred tiles of walkable floor, one at
// least and six at most (dungeon.PadsFor), so the count follows the
// floor's size and not its depth. They sit in
// the rooms farthest on foot from both the start and the stairs, so
// reaching one is a detour into the floor. Stand on the pad and it
// patches RepairRate of your max hull a second, out of a store of
// RepairStore that has to last the whole floor; it only drains while it's
// actually patching, so stepping on at full hull wastes nothing. Any hull
// you lose jams it for RepairJam seconds, which is why it's a place to
// heal after a fight and not during one: a Mend still heals mid-fight
// anywhere, and a Shield soaking the hits keeps the pad running. A spent
// pad stays spent on that floor; reloading the save doesn't refill it.
// Dying sends you back up to a newly rolled floor, with fresh pads.
const (
	RepairRate  = 0.15 // share of max hull patched per second
	RepairStore = 0.6  // share of max hull a pad holds per floor
	RepairJam   = 1.5  // seconds a pad jams after you lose hull
	// repairPulse is how often the patching shows as a +N, in seconds.
	repairPulse = 0.5
)

// Pad is one repair pad on the floor.
type Pad struct {
	At skill.Coord
	// Left is what the pad still holds, as a share of max hull.
	Left float32
}

// padState is the pad patching in progress.
type padState struct {
	jam    float32 // seconds left jammed
	lastHP float32 // hull at the end of last step, to see hull lost
	acc    float32 // hull patched since the last +N
	tick   float32
	on     int // the pad being stood on, -1 for none
}

// placePads puts the floor's pads down, each full.
func (s *Sim) placePads() {
	s.Pads = s.Pads[:0]
	for _, at := range s.Floor.Pads {
		s.Pads = append(s.Pads, Pad{At: at, Left: RepairStore})
	}
	s.pad = padState{on: -1, lastHP: s.P().U.HP}
}

// PadAt is the pad at c, or -1.
func (s *Sim) PadAt(c skill.Coord) int {
	for i := range s.Pads {
		if s.Pads[i].At == c {
			return i
		}
	}
	return -1
}

// OnPad is the pad the player stands on, or -1.
func (s *Sim) OnPad() int { return s.pad.on }

// PadJammed reports how many seconds the pad stays jammed.
func (s *Sim) PadJammed() float32 { return s.pad.jam }

// Repairing reports whether the pad under the player is patching now.
func (s *Sim) Repairing() bool {
	i := s.pad.on
	if i < 0 || s.pad.jam > 0 || s.Pads[i].Left <= 0 {
		return false
	}
	p := s.P()
	return p.U.Alive && p.U.HP < p.U.MaxHP
}

// repair runs the pad under the player for dt seconds.
func (s *Sim) repair(dt float32) {
	ps := &s.pad
	p := s.P()
	if !p.U.Alive {
		return
	}
	if p.U.HP < ps.lastHP-0.01 {
		ps.jam = RepairJam
	}
	ps.jam = max(0, ps.jam-dt)
	ps.on = s.PadAt(p.At)
	patching := false
	if s.Repairing() {
		pad := &s.Pads[ps.on]
		want := min(RepairRate*dt, pad.Left, (p.U.MaxHP-p.U.HP)/p.U.MaxHP)
		p.U.HP = min(p.U.MaxHP, p.U.HP+want*p.U.MaxHP)
		pad.Left = max(0, pad.Left-want)
		ps.acc += want * p.U.MaxHP
		patching = true
		if pad.Left <= 0 {
			s.addFX(FX{Kind: FXPadSpent, At: pad.At, Life: 0.9})
		}
	}
	ps.tick += dt
	if ps.acc > 0 && (ps.tick >= repairPulse || !patching) {
		s.addFX(FX{Kind: FXMend, At: p.At, Value: ps.acc, Mag: magOf(ps.acc, p.U.MaxHP), Life: 0.9})
		ps.acc, ps.tick = 0, 0
		s.Dirty = true // so a reload keeps what the pad gave
	}
	if !patching {
		ps.tick = 0
	}
	ps.lastHP = p.U.HP
}

// padsLeft is what each pad holds, for the save.
func (s *Sim) padsLeft() []float32 {
	out := make([]float32, len(s.Pads))
	for i := range s.Pads {
		out[i] = s.Pads[i].Left
	}
	return out
}

// restorePads puts back what a saved floor's pads held. A save from
// another floor, or one whose pads don't line up, leaves them full.
func (s *Sim) restorePads(depth int, left []float32) {
	if depth != s.Depth || len(left) != len(s.Pads) {
		return
	}
	for i, l := range left {
		s.Pads[i].Left = min(RepairStore, max(0, l))
	}
}

// DevToPad clears the floor of foes and stands the player on the first
// pad at hp of max hull, or off hexes beside it, for the screenshot
// autopilot.
func (s *Sim) DevToPad(hp float32, off int) bool {
	if len(s.Pads) == 0 {
		return false
	}
	for i := range s.Actors {
		if a := &s.Actors[i]; skill.EntityID(i) != s.Player {
			a.U.Alive, a.U.HP = false, 0
		}
	}
	p := s.P()
	at := s.Pads[0].At
	for k := 0; k < off; k++ {
		for _, d := range skill.HexDirs[3:] {
			if n := at.Add(d); s.Floor.At(n).Walkable() {
				at = n
				break
			}
		}
	}
	p.placeFree(at)
	p.U.HP = p.U.MaxHP * hp
	s.pad.lastHP = p.U.HP
	s.refreshSight()
	return true
}

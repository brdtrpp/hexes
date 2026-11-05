package game

import (
	"hexes/dungeon"
	"hexes/skill"
)

// Workbenches and cast buttons.
//
// Hexes are only rewired at a workbench. Every floor has two: one next to
// where you arrive and one next to the stairs, so you set up on the way
// in and rework with the floor's loot on the way out. Standing on the
// bench or any hex touching it counts (BenchReach). Away from a bench the
// editor stays shut, so a fight is fought with the build you walked in
// with.
//
// Cast buttons: every hex whose trigger is on cast or hold has a button,
// 1 to Buttons. The player picks which at a bench; a new castable hex
// takes the lowest free one. Two hexes on the same button fire together,
// in slot order, so a combo is one press. Hexes with any other trigger
// keep their button number for when a cast trigger comes back, but a
// press skips them: the trigger part decides whether a hex is cast by
// hand, the button only decides where.

// Buttons is how many cast buttons there are.
const Buttons = 4

// BenchReach is how close to a bench (in hexes) the editor opens.
const BenchReach = 1

// NumBenches is how many benches a floor has: by the start, by the stairs.
const NumBenches = 2

// placeBenches puts the floor's benches on open ground beside the start
// and beside the stairs. It picks the first open neighbor in a fixed
// direction order, so a floor's benches never move. A spot with no open
// neighbor (it doesn't happen on generated floors) puts the bench on the
// spot itself.
func (s *Sim) placeBenches() {
	f := s.Floor
	s.Benches[0] = benchBeside(f, f.Start, f.Exit)
	s.Benches[1] = benchBeside(f, f.Exit, f.Start)
}

func benchBeside(f *dungeon.Floor, at, avoid skill.Coord) skill.Coord {
	for _, d := range skill.HexDirs {
		n := at.Add(d)
		if f.In(n) && f.At(n) == dungeon.Ground && n != avoid {
			return n
		}
	}
	return at
}

// AtBench reports whether the player is close enough to a bench to edit.
func (s *Sim) AtBench() bool {
	_, ok := s.NearBench()
	return ok
}

// NearBench is the bench the player is at, if any.
func (s *Sim) NearBench() (skill.Coord, bool) {
	if s.Floor == nil || s.State != Playing {
		return skill.Coord{}, false
	}
	at := s.P().At
	for _, b := range s.Benches {
		if skill.Dist(at, b) <= BenchReach {
			return b, true
		}
	}
	return skill.Coord{}, false
}

// Castable reports whether hex i fires from a button: its trigger is on
// cast or hold.
func (l *Loadout) Castable(i int) bool {
	p := l.Progs[i]
	return p != nil && (p.Trigger == skill.OnCast || p.Trigger == skill.Channel)
}

// FixKeys gives every castable hex without a button the lowest button no
// other castable hex is on (or button 1 when all four are taken).
func (l *Loadout) FixKeys() {
	for i := range l.Keys {
		if int(l.Keys[i]) > Buttons {
			l.Keys[i] = 0
		}
	}
	for i := range l.Keys {
		if l.Keys[i] != 0 || !l.Castable(i) {
			continue
		}
		key := uint8(1)
		for b := uint8(1); b <= Buttons; b++ {
			if !l.keyTaken(b) {
				key = b
				break
			}
		}
		l.Keys[i] = key
	}
}

func (l *Loadout) keyTaken(b uint8) bool {
	for j := range l.Keys {
		if l.Keys[j] == b && l.Castable(j) {
			return true
		}
	}
	return false
}

// CycleKey moves hex i to the next button (4 wraps to 1) and returns it.
// It refuses a hex that isn't cast by hand.
func (l *Loadout) CycleKey(i int) (int, bool) {
	if i < 0 || i >= MaxSlots || !l.Castable(i) {
		return 0, false
	}
	l.Keys[i] = l.Keys[i]%Buttons + 1
	return int(l.Keys[i]), true
}

// OnButton reports whether button b (1 to Buttons) fires hex i.
func (l *Loadout) OnButton(i, b int) bool {
	return l.Castable(i) && int(l.Keys[i]) == b
}

// ButtonUsed reports whether any hex fires from button b.
func (l *Loadout) ButtonUsed(b int) bool {
	for i := range l.Keys {
		if l.OnButton(i, b) {
			return true
		}
	}
	return false
}

// FirstOn is the first hex on button b, or -1.
func (l *Loadout) FirstOn(b int) int {
	for i := range l.Keys {
		if l.OnButton(i, b) {
			return i
		}
	}
	return -1
}

// Press asks for button b this frame: every on-cast hex on it casts.
// It reports whether a hold hex is on it, so the caller can count the
// button as held for channeling.
func (l *Loadout) Press(b int, in *Input) (hold bool) {
	for i := range l.Keys {
		if !l.OnButton(i, b) {
			continue
		}
		if l.Progs[i].Trigger == skill.Channel {
			hold = true
		} else {
			in.Cast[i] = true
		}
	}
	return hold
}

// Holds reports whether button b has a hold hex on it.
func (l *Loadout) Holds(b int) bool {
	for i := range l.Keys {
		if l.OnButton(i, b) && l.Progs[i].Trigger == skill.Channel {
			return true
		}
	}
	return false
}

// CycleKey is Loadout.CycleKey for the player's build, marking the save.
func (s *Sim) CycleKey(i int) (int, bool) {
	b, ok := s.Load.CycleKey(i)
	if ok {
		s.Dirty = true
	}
	return b, ok
}

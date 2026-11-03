package game

import (
	"testing"

	"hexes/skill"
)

// A watch trigger takes a condition from the editor, cycles on to a
// finder module, refuses a module that acts, and survives a save.
func TestWatchInTheGame(t *testing.T) {
	s := New(3)
	h := s.Load.Hexes[0]
	entry := h.Shape.Entry()
	h.Slots[entry].Part = skill.NewTrigger(skill.Watch)
	if err := s.rebuildAll(0); err != nil {
		t.Fatal(err)
	}
	if s.Load.Progs[0].WatchSteps != 0 {
		t.Fatal("a fresh watch has a check")
	}
	var c skill.Cond
	c[0], c[1] = skill.Tile{K: skill.TileHull}, skill.Tile{K: skill.TileLess, V: 0.3}
	if err := s.SetCond(0, entry, c); err != nil {
		t.Fatal(err)
	}
	if s.Load.Progs[0].WatchSteps == 0 {
		t.Fatal("the condition gave no check")
	}
	// A module that acts can't be watched; a finder can.
	acts, err := s.AddModule(BoardOrder[4])
	if err != nil {
		t.Fatal(err)
	}
	cell := s.Stash.Inv.Add(skill.NewDamage(s.Reg.MustID("kinetic"), 10))
	if err := s.SwapIn(acts, 2, cell); err != nil {
		t.Fatal(err)
	}
	if lib, ok := s.CycleWatch(0, entry); !ok || lib != "" {
		t.Fatalf("watched %q, a module that acts", lib)
	}
	m := s.Load.Hex(acts)
	m.Slots[2] = skill.Slot{}
	m.Slots[1] = skill.Slot{Kind: skill.SlotPart, Part: skill.NewTarget(skill.TargetNearest, 0, 3)}
	if err := s.rebuildAll(acts); err != nil {
		t.Fatal(err)
	}
	lib, ok := s.CycleWatch(0, entry)
	if !ok || lib != m.Name {
		t.Fatalf("watched %q, want %q", lib, m.Name)
	}
	b, err := s.Save()
	if err != nil {
		t.Fatal(err)
	}
	o := New(9)
	if err := o.LoadSave(b); err != nil {
		t.Fatal(err)
	}
	if p := o.Load.Hexes[0].Slots[entry].Part; p.Trigger != skill.Watch || p.Lib != m.Name || p.Cond.Len() != 2 {
		t.Fatalf("watch not saved: %+v", p)
	}
	if o.Load.Progs[0].WatchSteps == 0 {
		t.Fatal("loaded watch has no check")
	}
}

// Board banks X and Y start empty on each floor and a charge on X shows
// in the board's banks, not the hex's.
func TestBoardBanksInTheGame(t *testing.T) {
	s := New(3)
	if s.Disp.Loadout(s.Player).Board != &s.BoardBanks {
		t.Fatal("dispatcher not on the board's banks")
	}
}

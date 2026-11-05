package game

import (
	"testing"

	"hexes/skill"
)

// A Call cycles on from the equipped hexes to the library hexes on the
// board; a run-now call takes options from the pack and gives them back,
// and all of it survives a save.
func TestLibraryCallInTheGame(t *testing.T) {
	s := NewVeteran(3)
	m, err := s.AddModule(BoardOrder[4])
	if err != nil {
		t.Fatal(err)
	}
	cell := s.Stash.Inv.Add(skill.NewStatus(s.Reg.MustID("cryo"), 2.5))
	if err := s.SwapIn(m, 2, cell); err != nil {
		t.Fatal(err)
	}
	// Arc Lance's slot 5 calls Nanite Swarm; step until it names the module.
	found := false
	for i := 0; i < 8 && !found; i++ {
		if !s.CycleCall(0, 5) {
			t.Fatal("cycle refused")
		}
		found = s.Load.Hexes[0].Slots[5].Part.Lib == "Module A"
	}
	if !found {
		t.Fatal("never reached the library hex")
	}
	if s.Load.Hexes[0].Libs[5] != s.Load.Hex(m) {
		t.Fatal("call not linked")
	}
	before := s.Load.Progs[0]
	if len(before.Steps) == 0 || before.CallsHex(StarterCallTarget) {
		t.Fatalf("still queues the swarm:\n%s", before)
	}
	cell = s.Stash.Inv.Add(skill.NewMult(skill.VerbDuration, 1.5))
	if err := s.AddOpt(0, 5, cell); err != nil {
		t.Fatal(err)
	}
	if s.Stash.Inv.Has[cell] || s.Load.Hexes[0].Slots[5].Part.Opts.Len() != 1 {
		t.Fatal("option didn't move onto the call")
	}
	b, err := s.Save()
	if err != nil {
		t.Fatal(err)
	}
	o := NewVeteran(9)
	if err := o.LoadSave(b); err != nil {
		t.Fatal(err)
	}
	p := o.Load.Hexes[0].Slots[5].Part
	if p.Lib != "Module A" || p.Opts.Len() != 1 || o.Load.Hexes[0].Libs[5] == nil {
		t.Fatalf("loaded call %+v", p)
	}
	if err := o.TakeOpt(0, 5, 0); err != nil {
		t.Fatal(err)
	}
	if o.Load.Hexes[0].Slots[5].Part.Opts.Len() != 0 {
		t.Fatal("option still on")
	}
}

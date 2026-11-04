package game

import (
	"testing"

	"hexes/skill"
)

// The preset Arc Lance ends in a Call at Nanite Swarm: one press fires
// both, the swarm's slot flashes and its Heat lands on the rig too.
func TestStarterCallRunsSwarm(t *testing.T) {
	s := openArena(30, 30) // one foe in view, so the swarm is called once
	if p := s.Load.Progs[0]; !p.CallsHex(StarterCallTarget) {
		t.Fatalf("Arc Lance has no call at hex %d:\n%s", StarterCallTarget, p)
	}
	s.Rig.Heat = 0
	s.spawn(KindGrunt, 1, s.P().At.Add(skill.Coord{Q: 4, R: 0}), 1, 1e5) // lit, out of the ram's reach
	if o := s.Disp.Cast(s.Player, 0, skill.NoEntity); o != skill.Fired {
		t.Fatalf("cast %s", o)
	}
	if s.SlotFlash[StarterCallTarget-1] <= 0 || s.SlotLast[StarterCallTarget-1] != skill.Fired {
		t.Fatal("Nanite Swarm didn't flash")
	}
	lance, swarm := s.Load.Progs[0], s.Load.Progs[StarterCallTarget-1]
	want := lance.Heat + max(swarm.Heat, skill.CallMinHeat)
	if s.SlotFlash[1] > 0 { // the lance's hit set off Kinetic Ram on its own
		want += s.Load.Progs[1].Heat
	}
	if d := s.Rig.Heat - want; d < -0.01 || d > 0.01 {
		t.Fatalf("heat %.2f, want %.2f (lance plus swarm)", s.Rig.Heat, want)
	}
}

func TestCycleCallSkipsItself(t *testing.T) {
	s := New(3)
	var got []uint8
	for i := 0; i < 5; i++ {
		if !s.CycleCall(0, 5) {
			t.Fatal("no call in Arc Lance slot 5")
		}
		got = append(got, s.Load.Hexes[0].Slots[5].Part.Call)
	}
	want := []uint8{skill.CallSelf, 2, 3, 4, skill.CallSelf}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("cycle went %v, want %v", got, want)
		}
	}
	if s.CycleCall(0, 1) {
		t.Fatal("cycled a target part")
	}
	if to, ok := CallsTo(0, s.Load.Hexes[0].Slots[5].Part); !ok || to != 0 {
		t.Fatalf("self call points at %d", to)
	}
}

func TestCallSurvivesSave(t *testing.T) {
	s := New(11)
	s.CycleCall(0, 5) // self
	s.CycleCall(0, 5) // hex 2
	b, err := s.Save()
	if err != nil {
		t.Fatal(err)
	}
	r := New(1)
	if err := r.LoadSave(b); err != nil {
		t.Fatal(err)
	}
	p := r.Load.Hexes[0].Slots[5].Part
	if p.Kind != skill.KindCall || p.Call != 2 || !r.Load.Progs[0].CallsHex(2) {
		t.Fatalf("call came back as %+v", p)
	}
	if r.Stash.Inv.Count() != 0 {
		t.Fatal("a save that had its call got another")
	}
}

// A save from before Calls gets one starter Call in the pack, once.
func TestOldSaveGetsOneCall(t *testing.T) {
	s := New(11)
	s.Load.Hexes[0].Clear(5)
	if err := s.recompile(0); err != nil {
		t.Fatal(err)
	}
	sv := s.Snapshot()
	sv.CallGiven = false
	r := New(1)
	if err := r.Restore(sv); err != nil {
		t.Fatal(err)
	}
	if r.Stash.Inv.Count() != 1 || r.Stash.Inv.Parts[0].Kind != skill.KindCall || r.Unsecured() != 0 {
		t.Fatalf("pack %d %+v", r.Stash.Inv.Count(), r.Stash.Inv.Parts[0])
	}
	if !r.Dirty || !r.CallGiven {
		t.Fatal("the gift should be saved")
	}
	b, _ := r.Save()
	r2 := New(2)
	if err := r2.LoadSave(b); err != nil {
		t.Fatal(err)
	}
	if r2.Stash.Inv.Count() != 1 {
		t.Fatalf("pack %d after a second load, want 1", r2.Stash.Inv.Count())
	}
}

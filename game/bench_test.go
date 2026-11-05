package game

import (
	"encoding/json"
	"testing"

	"hexes/dungeon"
	"hexes/skill"
)

func TestBenchesSitOnGroundByStartAndStairs(t *testing.T) {
	for seed := uint64(1); seed <= 20; seed++ {
		s := NewVeteran(seed)
		for depth := 1; depth <= 12; depth += 3 {
			s.load(depth, 1)
			f := s.Floor
			for i, b := range s.Benches {
				if f.At(b) != dungeon.Ground {
					t.Fatalf("seed %d depth %d bench %d on %s", seed, depth, i, f.At(b))
				}
			}
			if skill.Dist(s.Benches[0], f.Start) != 1 || skill.Dist(s.Benches[1], f.Exit) != 1 {
				t.Fatalf("seed %d depth %d: benches %v not beside start %v and stairs %v", seed, depth, s.Benches, f.Start, f.Exit)
			}
			// The same floor always gets the same benches.
			was := s.Benches
			s.load(depth, 1)
			if s.Benches != was {
				t.Fatalf("seed %d depth %d: benches moved", seed, depth)
			}
		}
	}
}

func TestEditingOnlyAtABench(t *testing.T) {
	s := NewVeteran(4)
	if !s.AtBench() {
		t.Fatal("you arrive next to a bench")
	}
	p := s.P()
	far := s.Floor.Start
	for i := range s.Floor.Tiles {
		c := s.Floor.CoordOf(i)
		if s.Floor.At(c) == dungeon.Ground && skill.Dist(c, s.Benches[0]) > 3 && skill.Dist(c, s.Benches[1]) > 3 {
			far = c
			break
		}
	}
	p.placeFree(far)
	if s.AtBench() {
		t.Fatal("editing open away from both benches")
	}
	p.placeFree(s.Benches[1])
	if !s.AtBench() {
		t.Fatal("standing on the stairs bench should open the editor")
	}
	s.State = Dead
	if s.AtBench() {
		t.Fatal("no editing while dead")
	}
}

func TestCastButtons(t *testing.T) {
	s := NewVeteran(3)
	l := &s.Load
	if l.Keys != [MaxSlots]uint8{1, 0, 0, 0} {
		t.Fatalf("preset keys %v: only the lance casts, on button 1", l.Keys)
	}
	// A second on-cast hex gets its own button, not a share of the first.
	cell := s.Stash.Inv.Add(skill.NewTrigger(skill.OnCast))
	if err := s.SwapIn(1, 0, cell); err != nil {
		t.Fatal(err)
	}
	if l.Keys[1] != 2 {
		t.Fatalf("second cast hex on button %d, want 2", l.Keys[1])
	}
	var in Input
	if l.Press(2, &in); in.Cast != [MaxSlots]bool{false, true} {
		t.Fatalf("button 2 cast %v", in.Cast)
	}
	in = Input{}
	if l.Press(1, &in); in.Cast != [MaxSlots]bool{true} {
		t.Fatalf("button 1 cast %v", in.Cast)
	}
	// Moving it onto button 1 makes a combo: one press, both hexes.
	for l.Keys[1] != 1 {
		if _, ok := s.CycleKey(1); !ok {
			t.Fatal("cycle refused a cast hex")
		}
	}
	in = Input{}
	if l.Press(1, &in); in.Cast != [MaxSlots]bool{true, true} || l.ButtonUsed(2) {
		t.Fatalf("combo on button 1: %v", in.Cast)
	}
	// Hexes that fire on their own don't take a button.
	if _, ok := s.CycleKey(2); ok {
		t.Fatal("an on-kill hex took a button")
	}
	if l.Keys[2] != 0 || l.OnButton(2, 0) {
		t.Fatal("on-kill hex has a button")
	}
	// The sim casts what the button asked for.
	s.Rig.Power = s.Rig.MaxPower
	s.Input = Input{}
	l.Press(1, &s.Input)
	s.Step(1.0 / 60)
	if s.Rig.Power >= s.Rig.MaxPower {
		t.Fatal("pressing button 1 cast nothing")
	}
}

func TestHoldHexHoldsItsButton(t *testing.T) {
	s := NewVeteran(5)
	l := &s.Load
	cell := s.Stash.Inv.Add(skill.NewTrigger(skill.Channel))
	if err := s.SwapIn(3, 0, cell); err != nil {
		t.Fatal(err)
	}
	b := int(l.Keys[3])
	if b != 2 || !l.Holds(b) || l.Holds(1) {
		t.Fatalf("hold hex on button %d", b)
	}
	var in Input
	if !l.Press(b, &in) || in.Cast[3] {
		t.Fatal("a hold hex channels, it doesn't cast")
	}
}

func TestCastButtonsSave(t *testing.T) {
	s := NewVeteran(6)
	cell := s.Stash.Inv.Add(skill.NewTrigger(skill.OnCast))
	if err := s.SwapIn(2, 0, cell); err != nil {
		t.Fatal(err)
	}
	s.CycleKey(2) // 2 -> 3
	b, err := s.Save()
	if err != nil {
		t.Fatal(err)
	}
	r := NewVeteran(99)
	if err := r.LoadSave(b); err != nil {
		t.Fatal(err)
	}
	if r.Load.Keys != s.Load.Keys || r.Load.Keys[2] != 3 {
		t.Fatalf("keys %v after load, want %v", r.Load.Keys, s.Load.Keys)
	}
	// A save from before buttons hands them out in slot order.
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	delete(raw, "Keys")
	old, _ := json.Marshal(raw)
	r = NewVeteran(99)
	if err := r.LoadSave(old); err != nil {
		t.Fatal(err)
	}
	if r.Load.Keys != [MaxSlots]uint8{1, 0, 2, 0} {
		t.Fatalf("old save keys %v", r.Load.Keys)
	}
}

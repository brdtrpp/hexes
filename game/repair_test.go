package game

import (
	"encoding/json"
	"testing"
)

// standOnPad puts the player on pad i of a fresh floor at depth, hurt to
// hp of max, with every foe gone so nothing jams the pad.
func standOnPad(t *testing.T, depth int, hp float32) (*Sim, *Pad) {
	t.Helper()
	s := NewVeteran(31)
	s.load(depth, hp)
	if len(s.Pads) == 0 {
		t.Fatalf("depth %d: no pad", depth)
	}
	for i := range s.Actors {
		if i != int(s.Player) {
			s.Actors[i].U.Alive = false
		}
	}
	p := s.P()
	p.placeFree(s.Pads[0].At)
	s.pad.lastHP = p.U.HP
	return s, &s.Pads[0]
}

func TestEveryFloorHasPads(t *testing.T) {
	s := NewVeteran(5)
	for depth := 1; depth <= 30; depth += 4 {
		s.load(depth, 1)
		if len(s.Pads) == 0 {
			t.Fatalf("depth %d: no repair pad", depth)
		}
		for _, pd := range s.Pads {
			if pd.Left != RepairStore {
				t.Fatalf("depth %d: a new pad holds %.2f", depth, pd.Left)
			}
			if pd.At == s.Benches[0] || pd.At == s.Benches[1] {
				t.Fatalf("depth %d: pad on a bench", depth)
			}
		}
	}
}

// A pad patches at its rate, stops at full hull, and only spends what it
// patched.
func TestPadPatchesAtItsRate(t *testing.T) {
	s, pad := standOnPad(t, 3, 0.5)
	p := s.P()
	for i := 0; i < 60; i++ { // one second
		s.repair(1.0 / 60)
	}
	got := p.U.HP/p.U.MaxHP - 0.5
	if got < RepairRate*0.95 || got > RepairRate*1.05 {
		t.Fatalf("one second patched %.3f of max hull, want %.2f", got, RepairRate)
	}
	if d := RepairStore - pad.Left; d < got-0.001 || d > got+0.001 {
		t.Fatalf("pad spent %.3f for %.3f patched", d, got)
	}
	mended := false
	s.FX.Each(func(f *FX) { mended = mended || f.Kind == FXMend })
	if !mended {
		t.Fatal("patching shows no +N")
	}
	for i := 0; i < 60*10; i++ {
		s.repair(1.0 / 60)
	}
	if p.U.HP != p.U.MaxHP {
		t.Fatalf("ten more seconds left hull at %.2f", p.U.HP/p.U.MaxHP)
	}
	left := pad.Left
	for i := 0; i < 60; i++ {
		s.repair(1.0 / 60)
	}
	if pad.Left != left || s.Repairing() {
		t.Fatal("a pad drains while the hull is already full")
	}
}

// The store runs out: from near zero hull, one pad gives RepairStore and
// no more, then says it's spent.
func TestPadRunsDry(t *testing.T) {
	s, pad := standOnPad(t, 3, 0.05)
	p := s.P()
	for i := 0; i < 60*20; i++ {
		s.repair(1.0 / 60)
	}
	if pad.Left != 0 {
		t.Fatalf("pad still holds %.2f", pad.Left)
	}
	if got := p.U.HP / p.U.MaxHP; got < 0.05+RepairStore-0.01 || got > 0.05+RepairStore+0.01 {
		t.Fatalf("a full pad gave %.2f of max hull, want %.2f", got-0.05, RepairStore)
	}
	spent := false
	s.FX.Each(func(f *FX) { spent = spent || f.Kind == FXPadSpent })
	if !spent {
		t.Fatal("no spent cue")
	}
}

// Losing hull jams the pad for RepairJam seconds; off the pad nothing
// happens at all.
func TestPadJamsWhenHit(t *testing.T) {
	s, _ := standOnPad(t, 3, 0.5)
	p := s.P()
	s.repair(1.0 / 60)
	p.U.HP -= 5
	s.repair(1.0 / 60)
	if s.PadJammed() <= 0 || s.Repairing() {
		t.Fatal("a hit didn't jam the pad")
	}
	hp := p.U.HP
	for i := 0; i < int(RepairJam*60)-5; i++ {
		s.repair(1.0 / 60)
	}
	if p.U.HP != hp {
		t.Fatal("a jammed pad still patched")
	}
	for i := 0; i < 20; i++ {
		s.repair(1.0 / 60)
	}
	if p.U.HP <= hp {
		t.Fatal("the pad never came back after the jam")
	}

	s2, pad2 := standOnPad(t, 3, 0.5)
	n := s2.P().At
	s2.P().placeFree(s2.Floor.Start)
	for i := 0; i < 120; i++ {
		s2.repair(1.0 / 60)
	}
	if pad2.Left != RepairStore || s2.OnPad() >= 0 {
		t.Fatalf("a pad at %v patched from the start", n)
	}
}

// What a pad gave stays given across a save and reload of the same floor;
// a new floor's pads are full again.
func TestPadSurvivesReload(t *testing.T) {
	s, pad := standOnPad(t, 4, 0.2)
	for i := 0; i < 120; i++ {
		s.repair(1.0 / 60)
	}
	left := pad.Left
	b, err := s.Save()
	if err != nil {
		t.Fatal(err)
	}
	var sv Save
	if err := json.Unmarshal(b, &sv); err != nil {
		t.Fatal(err)
	}
	s2 := NewVeteran(1)
	if err := s2.Restore(sv); err != nil {
		t.Fatal(err)
	}
	if got := s2.Pads[0].Left; got < left-0.001 || got > left+0.001 {
		t.Fatalf("reloaded pad holds %.3f, want %.3f", got, left)
	}
	s2.load(5, 1)
	for _, pd := range s2.Pads {
		if pd.Left != RepairStore {
			t.Fatal("the next floor's pad isn't full")
		}
	}
}

package game

import (
	"strings"
	"testing"

	"hexes/skill"
)

// sustainDrop is the first sustain part lying on the floor, if any.
func sustainDrop(s *Sim) (Drop, bool) {
	for _, d := range s.Stash.Drops {
		if IsSustain(d.Part) {
			return d, true
		}
	}
	return Drop{}, false
}

func TestFirstMendWaitsByTheStairs(t *testing.T) {
	s := New(6)
	d, ok := sustainDrop(s)
	if !ok || d.Part != FirstSustain() || d.Part.Effect != skill.EffectMend {
		t.Fatalf("no first mend on depth 1: %+v", s.Stash.Drops)
	}
	if !s.Floor.At(d.At).Walkable() || skill.Dist(d.At, s.Floor.Start) != 1 {
		t.Fatalf("first mend at %v, start %v", d.At, s.Floor.Start)
	}
	// Not picked up: it comes back on the next floor.
	s.load(2, 1)
	if _, ok := sustainDrop(s); !ok {
		t.Fatal("first mend gone before it was picked up")
	}
	for _, f := range s.Stash.Frames {
		if f.At == d.At {
			t.Fatal("first mend on the first frame's tile")
		}
	}
	d, _ = sustainDrop(s)
	s.P().At = d.At
	s.pickup(0)
	if !s.SustainGiven {
		t.Fatal("picking up a mend didn't count")
	}
	found := false
	for i, p := range s.Stash.Inv.Parts {
		found = found || (s.Stash.Inv.Has[i] && p == FirstSustain())
	}
	if !found {
		t.Fatal("mend not in the pack")
	}
	b, err := s.Save()
	if err != nil {
		t.Fatal(err)
	}
	r := New(1)
	if err := r.LoadSave(b); err != nil {
		t.Fatal(err)
	}
	if !r.SustainGiven {
		t.Fatal("save lost SustainGiven")
	}
	if _, ok := sustainDrop(r); ok {
		t.Fatal("first mend placed again after it was found")
	}
}

func TestOldSaveGetsTheFirstMend(t *testing.T) {
	s := New(3)
	b, _ := s.Save()
	if strings.Contains(string(b), "SustainGiven") {
		t.Fatal("an unfound mend wrote SustainGiven")
	}
	r := New(1)
	if err := r.LoadSave(b); err != nil {
		t.Fatal(err)
	}
	if _, ok := sustainDrop(r); !ok {
		t.Fatal("no mend waiting for an old save")
	}
}

func TestPresetHexesHaveNoSustain(t *testing.T) {
	s := New(2)
	for i, h := range s.Load.Hexes {
		if h == nil {
			continue
		}
		for _, sl := range h.Slots {
			if sl.Kind == skill.SlotPart && IsSustain(sl.Part) {
				t.Fatalf("preset hex %d has a %s", i, sl.Part.Effect)
			}
		}
	}
}

func TestShieldSoaksClaws(t *testing.T) {
	s := New(9)
	p := s.P()
	var foe skill.EntityID = -1
	for i := range s.Actors {
		if skill.EntityID(i) != s.Player {
			foe = skill.EntityID(i)
			break
		}
	}
	if foe < 0 {
		t.Skip("no foe on this floor")
	}
	p.U.AddShield(0.1)
	shield, hp := p.U.Shield, p.U.HP
	s.claw(foe, shield/2)
	if p.U.HP != hp || p.U.Shield >= shield {
		t.Fatalf("claw went past a shield: hp %.1f->%.1f shield %.1f->%.1f", hp, p.U.HP, shield, p.U.Shield)
	}
	soaked := false
	s.FX.Each(func(f *FX) { soaked = soaked || f.Kind == FXSoak })
	if !soaked {
		t.Fatal("no soak fx")
	}
	s.timers(skill.ShieldTime + 0.1)
	if p.U.Shield != 0 {
		t.Fatal("shield never faded")
	}
}

func TestSustainFiresInTheSim(t *testing.T) {
	s := New(4)
	p := s.P()
	p.U.HP = p.U.MaxHP / 2
	s.Rig.Heat = 40
	h := &skill.Hex{Name: "Medic", Shape: skill.Shape{Kind: skill.ShapeRing}}
	h.Sockets = ShapeSockets(h.Shape)
	for i := range h.Linked {
		h.Linked[i] = true
	}
	for i, part := range []skill.Part{skill.NewTrigger(skill.OnCast), skill.NewMend(0.1), skill.NewShield(0.1), skill.NewPurge(10)} {
		h.Slots[i] = skill.Slot{Kind: skill.SlotPart, Part: part}
	}
	prog, err := skill.Compile(h, s.Reg, 1)
	if err != nil {
		t.Fatal(err)
	}
	s.Disp.Loadout(s.Player).Progs[0] = prog
	hp := p.U.HP
	s.Input.Cast[0] = true
	s.Step(1.0 / 60)
	if p.U.HP <= hp || p.U.Shield <= 0 {
		t.Fatalf("hp %.0f->%.0f shield %.0f", hp, p.U.HP, p.U.Shield)
	}
	kinds := map[FXKind]bool{}
	s.FX.Each(func(f *FX) { kinds[f.Kind] = true })
	for _, k := range []FXKind{FXMend, FXShield, FXPurge} {
		if !kinds[k] {
			t.Fatalf("no fx %d", k)
		}
	}
}

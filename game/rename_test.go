package game

import (
	"strings"
	"testing"

	"hexes/skill"
)

func TestRenameHex(t *testing.T) {
	s := NewVeteran(3)
	before := s.Load.Names[0]
	if got, err := s.RenameHex(0, "  My   Big\tBolt  "); err != nil || got != "My Big Bolt" {
		t.Fatalf("rename %q %v", got, err)
	}
	if s.Load.Names[0] != "My Big Bolt" || !s.Dirty {
		t.Fatalf("names %q dirty %v", s.Load.Names[0], s.Dirty)
	}
	if got, _ := s.RenameHex(0, "   "); got != "My Big Bolt" {
		t.Fatalf("blank renamed to %q", got)
	}
	if got, _ := s.RenameHex(0, strings.Repeat("x", 40)); len(got) != MaxNameLen {
		t.Fatalf("long name kept %d letters", len(got))
	}
	// The name survives a save and a load.
	s.RenameHex(0, "Keeper")
	b, err := s.Save()
	if err != nil {
		t.Fatal(err)
	}
	s2 := NewVeteran(3)
	if err := s2.LoadSave(b); err != nil {
		t.Fatal(err)
	}
	if s2.Load.Names[0] != "Keeper" {
		t.Fatalf("after load %q (was %q)", s2.Load.Names[0], before)
	}
	if sh, err := s2.SlotSheet(0); err != nil || sh.Name != "Keeper" {
		t.Fatalf("card name %q %v", sh.Name, err)
	}
}

// A module's callers follow it to its new name.
func TestRenameModuleKeepsCalls(t *testing.T) {
	s := NewVeteran(5)
	m, err := s.AddModule(BoardOrder[4])
	if err != nil {
		t.Fatal(err)
	}
	s.Load.Hexes[0].Slots[5] = skill.Slot{Kind: skill.SlotPart, Part: skill.NewLibCall(s.Load.Name(m))}
	if _, err := s.RenameHex(m, "Finder"); err != nil {
		t.Fatal(err)
	}
	if got := s.Load.Hexes[0].Slots[5].Part.Lib; got != "Finder" {
		t.Fatalf("call still names %q", got)
	}
	m2, _ := s.AddModule(BoardOrder[5])
	if _, err := s.RenameHex(m2, "finder"); err == nil {
		t.Fatal("two modules share a name")
	}
}

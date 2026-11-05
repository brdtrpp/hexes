package game

import (
	"encoding/json"
	"testing"

	"hexes/skill"
)

func TestExamplesLoad(t *testing.T) {
	s := NewVeteran(5)
	if err := s.LoadExample(0); err != ErrNeedsTest {
		t.Fatalf("loaded with the test switch off: %v", err)
	}
	if err := s.SetTestShapes(true); err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(s.Load.Hexes)
	names := s.Load.Names
	for i, e := range Examples(s.Reg) {
		if err := s.LoadExample(i); err != nil {
			t.Fatalf("%s: %v", e.Name, err)
		}
		for k := range e.Hexes {
			p := s.Load.Progs[k]
			if p == nil || p.Inert {
				t.Fatalf("%s: hex %d is inert or missing", e.Name, k)
			}
		}
		if s.Example != e.Name {
			t.Fatalf("example name %q", s.Example)
		}
	}
	// A reload keeps the stashed build.
	sv := s.Snapshot()
	raw, _ := json.Marshal(sv)
	var back Save
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	s2 := NewVeteran(5)
	if err := s2.Restore(back); err != nil {
		t.Fatal(err)
	}
	ex := Examples(s.Reg)
	if s2.Mine == nil || s2.Example != ex[len(ex)-1].Name {
		t.Fatalf("stash lost on reload: %v %q", s2.Mine != nil, s2.Example)
	}
	if err := s2.RestoreBuild(); err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(s2.Load.Hexes)
	if string(after) != string(before) || s2.Load.Names != names {
		t.Fatal("restore didn't bring the build back")
	}
	if s2.Mine != nil || s2.RestoreBuild() == nil {
		t.Fatal("stash should be gone")
	}
}

func TestFieldMedicSustains(t *testing.T) {
	s := NewVeteran(5)
	if err := s.SetTestShapes(true); err != nil {
		t.Fatal(err)
	}
	ex := Examples(s.Reg)
	i := len(ex) - 1
	if ex[i].Name != "Field Medic" {
		t.Fatalf("last example %q", ex[i].Name)
	}
	if err := s.LoadExample(i); err != nil {
		t.Fatal(err)
	}
	ops := map[skill.Op]bool{}
	for k := 0; k < 3; k++ {
		for _, st := range s.Load.Progs[k].Steps {
			ops[st.Op] = true
		}
	}
	for _, op := range []skill.Op{skill.OpMend, skill.OpShield, skill.OpPurge} {
		if !ops[op] {
			t.Fatalf("Field Medic has no %s", op)
		}
	}
}

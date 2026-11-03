package game

import (
	"encoding/json"
	"testing"
)

func TestExamplesLoad(t *testing.T) {
	s := New(5)
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
	s2 := New(5)
	if err := s2.Restore(back); err != nil {
		t.Fatal(err)
	}
	if s2.Mine == nil || s2.Example != "Deep Freeze" {
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

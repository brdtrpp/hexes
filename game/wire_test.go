package game

import (
	"encoding/json"
	"testing"

	"hexes/skill"
)

// A hex from before the open board runs just as it did once it's wired.
func TestWiringKeepsPrograms(t *testing.T) {
	reg := skill.DefaultRegistry()
	moved := map[string]bool{"Snare": true, "Tag": true, "Pin": true, "Bastion": true, "Cryo Plan": true}
	for _, ex := range Examples(reg) {
		for _, eh := range ex.Hexes {
			h := eh.Hex
			before, err := skill.Compile(h, reg, 4)
			if err != nil {
				t.Fatalf("%s: %v", eh.Name, err)
			}
			b, _ := json.Marshal(h)
			var w skill.Hex
			if err := json.Unmarshal(b, &w); err != nil {
				t.Fatal(err)
			}
			w.EnsureLaid()
			after, err := skill.Compile(&w, reg, 4)
			if err != nil {
				t.Fatalf("%s wired: %v", eh.Name, err)
			}
			// On the ring, a damage after a status on a reach landed on the
			// cast's subject; wired, it lands on the reach the status did.
			if moved[eh.Name] {
				continue
			}
			if before.String() != after.String() {
				t.Errorf("%s / %s runs differently wired:\nbefore\n%s\nafter\n%s", ex.Name, eh.Name, before, after)
			}
		}
	}
}

// A hex laid by edges (a save from the shaped pieces) keeps every join as
// a wire.
func TestEdgesBecomeWires(t *testing.T) {
	reg := skill.DefaultRegistry()
	for _, ex := range Examples(reg) {
		for _, eh := range ex.Hexes {
			b, _ := json.Marshal(eh.Hex)
			var h skill.Hex
			if err := json.Unmarshal(b, &h); err != nil {
				t.Fatal(err)
			}
			h.LayOut(skill.RoomFor(h.Shape))
			before, err := skill.Compile(&h, reg, 4)
			if err != nil {
				t.Fatalf("%s: %v", eh.Name, err)
			}
			h.EnsureLaid()
			if !h.Lay.Wired {
				t.Fatalf("%s isn't wired", eh.Name)
			}
			after, err := skill.Compile(&h, reg, 4)
			if err != nil {
				t.Fatalf("%s wired: %v", eh.Name, err)
			}
			if before.String() != after.String() {
				t.Errorf("%s / %s runs differently wired:\nbefore\n%s\nafter\n%s", ex.Name, eh.Name, before, after)
			}
		}
	}
}

package loot

import (
	"strings"
	"testing"

	"hexes/skill"
)

// Dropped drones carry guns of their own, rolled as they drop, and their
// cards say what the gun does.
func TestDroppedDronesCarryTheirOwnGuns(t *testing.T) {
	reg := skill.DefaultRegistry()
	tb := NewTable(reg)
	r := NewRNG(7)
	seeds := map[uint32]bool{}
	titles := map[string]bool{}
	for k := 0; k < 400 && len(seeds) < 12; k++ {
		p := tb.RollFam(&r, 3, skill.Common)
		if p.Form != skill.FormDrone {
			continue
		}
		if p.Seed == 0 {
			t.Fatal("dropped drone has no gun")
		}
		seeds[p.Seed] = true
		title, detail := Label(p, reg)
		if title != "DRONE" || detail != p.DroneKit().Title() {
			t.Fatalf("label %q %q", title, detail)
		}
		titles[detail] = true
		if d := Describe(p, reg); !strings.Contains(d, p.DroneKit().Text()) {
			t.Fatalf("card %q doesn't say what its gun does", d)
		}
	}
	if len(seeds) < 12 || len(titles) < 4 {
		t.Fatalf("drones too alike: %d seeds, %d guns", len(seeds), len(titles))
	}
}

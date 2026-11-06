package game

import (
	"strings"
	"testing"

	"hexes/loot"
	"hexes/skill"
)

func TestSheetsRead(t *testing.T) {
	reg := skill.DefaultRegistry()
	for _, ex := range Examples(reg) {
		for _, h := range ex.Hexes {
			s, err := HexSheet(h.Hex, reg, 5, 2, h.Blurb)
			if err != nil {
				t.Fatal(err)
			}
			if s.Name == "" || len(s.Lines) == 0 {
				t.Fatalf("%s has an empty sheet", h.Name)
			}
			if testing.Verbose() {
				t.Log("\n" + s.Text())
			}
			for _, sl := range h.Hex.Slots {
				if sl.Kind != skill.SlotPart {
					continue
				}
				ps := PartSheet(sl.Part, reg)
				if ps.Name == "" || ps.Desc == "" || strings.Contains(ps.Text(), "%!") {
					t.Fatalf("part sheet %q", ps.Text())
				}
				if testing.Verbose() {
					t.Log("\n" + ps.Text())
				}
			}
		}
	}
	tab := loot.NewTable(reg)
	r := loot.NewRNG(3)
	for i := 0; i < 2000; i++ {
		p := tab.RollNew(&r, skill.Rarity(r.Intn(int(skill.NumRarities))))
		ps := PartSheet(p, reg)
		if ps.Name == "" || strings.Contains(ps.Text(), "%!") {
			t.Fatalf("part sheet %q", ps.Text())
		}
	}
}

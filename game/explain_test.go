package game

import (
	"math"
	"testing"

	"hexes/loot"
	"hexes/skill"
)

// The working on a hex's sheet adds up: every effect's number is its base
// times the factors shown, and the cost lines sum to its Power and Heat.
func TestWorkingAddsUp(t *testing.T) {
	reg := skill.DefaultRegistry()
	var hexes []*skill.Hex
	for _, ex := range Examples(reg) {
		for _, h := range ex.Hexes {
			hexes = append(hexes, h.Hex)
		}
		hexes = append(hexes, ex.Mods...)
	}
	pre, _, _ := PresetHexes(reg)
	for _, h := range pre {
		if h != nil {
			hexes = append(hexes, h)
		}
	}
	for _, lvl := range []int{1, 12} {
		for _, h := range hexes {
			p, w, err := ExplainHex(h, reg, lvl, 2)
			if err != nil {
				continue // library hexes that don't compile alone
			}
			q, _ := CompileHex(h, reg, lvl, 2)
			if len(q.Steps) != len(p.Steps) || q.Power != p.Power || q.Heat != p.Heat {
				t.Fatalf("%s: explaining it changed the program", h.Name)
			}
			for _, e := range w.Effects {
				if e.Runs == 0 {
					continue
				}
				want := float64(e.Base)
				for _, f := range e.Factors {
					want *= float64(f.X)
				}
				if math.Abs(want-float64(e.Final)) > 0.01*math.Max(1, want) {
					t.Errorf("%s level %d: %s shows %.2f from its factors %+v, runs at %.2f", h.Name, lvl, e.Name, want, e.Factors, e.Final)
				}
			}
			pw, ht := w.Sum()
			if math.Abs(float64(pw-p.Power)) > 0.01 || math.Abs(float64(ht-p.Heat)) > 0.01 {
				t.Errorf("%s: cost lines add to %.2f power %.2f heat, it costs %.2f and %.2f\n%+v\n%+v", h.Name, pw, ht, p.Power, p.Heat, w.Costs, w.Totals)
			}
		}
	}
}

// Random hexes of rolled parts, frames and shapes add up too.
func TestWorkingAddsUpOnLoot(t *testing.T) {
	reg := skill.DefaultRegistry()
	tab := loot.NewTable(reg)
	tab.Allow.New = 0xff
	r := loot.NewRNG(7)
	built := 0
	for n := 0; n < 3000; n++ {
		h := skill.NewHex("fuzz", LoadoutSockets)
		h.Shape = skill.Shape{Kind: skill.ShapeKind(r.Intn(int(skill.NumShapes)))}
		if !h.Shape.Valid() {
			h.Shape = skill.Shape{}
		}
		for i := range h.Sockets {
			h.Sockets[i] = skill.SocketWild
		}
		h.Sockets[h.Shape.Entry()] = skill.SocketTrigger
		if err := h.Set(h.Shape.Entry(), skill.NewTrigger(skill.OnCast)); err != nil {
			t.Fatal(err)
		}
		for s := 0; s < skill.Slots; s++ {
			if s == h.TriggerSlot() || !h.Shape.Has(s) {
				continue
			}
			p := tab.RollTaught(&r, skill.Rarity(r.Intn(int(skill.NumRarities))))
			if p.Kind == skill.KindTrigger {
				continue
			}
			_ = h.Set(s, p)
			if r.Float() < 0.3 {
				h.Frames[s] = skill.NewFrame(skill.Bend(r.Intn(int(skill.NumBends))))
			}
		}
		p, w, err := ExplainHex(h, reg, 1+r.Intn(20), 2)
		if err != nil {
			if n < 5 {
				t.Log(err)
			}
			continue
		}
		built++
		for _, e := range w.Effects {
			if e.Runs == 0 {
				continue
			}
			want := float64(e.Base)
			for _, f := range e.Factors {
				want *= float64(f.X)
			}
			if math.Abs(want-float64(e.Final)) > 0.01*math.Max(1, want) {
				t.Fatalf("hex %d: %s shows %.2f from %+v, runs at %.2f", n, e.Name, want, e.Factors, e.Final)
			}
		}
		pw, ht := w.Sum()
		if math.Abs(float64(pw-p.Power)) > 0.02 || math.Abs(float64(ht-p.Heat)) > 0.02 {
			t.Fatalf("hex %d: lines add to %.2f/%.2f, costs %.2f/%.2f\n%+v\n%+v\n%+v", n, pw, ht, p.Power, p.Heat, w.Costs, w.Totals, h.Slots)
		}
	}
	if built < 300 {
		t.Fatalf("only %d random hexes compiled", built)
	}
}

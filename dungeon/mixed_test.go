package dungeon

import (
	"testing"

	"hexes/skill"
)

// TestMixedFloors checks the deep floors that hold more than one kind:
// they start at MixDepth, three kinds wait for TripleDepth, every pair of
// kinds turns up, rooms of both kinds sit in each crossing, you arrive in
// the first region and leave from the last, and the floor is still one
// piece with its pads.
func TestMixedFloors(t *testing.T) {
	var g Generator
	n := MaxW * MaxH
	dist, queue := make([]int32, n), make([]int32, n)
	kinds := map[int]int{}
	pairs := map[[2]Layout]bool{}
	second := map[Layout]int{}
	floors := 0
	for depth := MixDepth - 4; depth <= 40; depth++ {
		for seed := uint64(1); seed <= 40; seed++ {
			s := seed * 0x9e3779b97f4a7c15
			ks := KindsFor(s, depth)
			if ks[0] != LayoutFor(s, depth) {
				t.Fatalf("depth %d: a mixed floor starts as %s, not its own layout %s", depth, ks[0], LayoutFor(s, depth))
			}
			if depth < MixDepth && len(ks) != 1 {
				t.Fatalf("depth %d holds %d kinds, before mixing starts", depth, len(ks))
			}
			if depth < TripleDepth && len(ks) > 2 {
				t.Fatalf("depth %d holds three kinds, before TripleDepth", depth)
			}
			f := g.Generate(s, depth)
			if depth < MixDepth {
				continue
			}
			floors++
			kinds[len(ks)]++
			if len(f.Regions) != len(ks) {
				t.Fatalf("depth %d: %d regions for %d kinds", depth, len(f.Regions), len(ks))
			}
			for i, r := range f.Regions {
				if r.Layout != ks[i] {
					t.Fatalf("depth %d: region %d is %s, want %s", depth, i, r.Layout, ks[i])
				}
				for j := 0; j < i; j++ {
					if f.Regions[j].Layout == r.Layout {
						t.Fatalf("depth %d: %s twice on one floor", depth, r.Layout)
					}
				}
				if i > 0 {
					pairs[[2]Layout{f.Regions[i-1].Layout, r.Layout}] = true
					if i == 1 {
						second[r.Layout]++
					}
				}
			}
			f.DistanceField(f.Start, dist, queue)
			for i, r := range f.Rooms {
				if dist[f.Index(r.Center)] < 0 {
					t.Fatalf("depth %d seed %d %v: room %d (%s) cut off", depth, seed, ks, i, r.Kind)
				}
			}
			if dist[f.Index(f.Exit)] < 0 || f.At(f.Exit) != Stairs {
				t.Fatalf("depth %d seed %d %v: stairs missing or cut off", depth, seed, ks)
			}
			if want := PadsFor(f.Open()); len(f.Pads) < want-1 {
				t.Fatalf("depth %d seed %d %v: %d pads, want about %d", depth, seed, ks, len(f.Pads), want)
			}
			if !f.Mixed() {
				continue
			}
			if got := f.RegionAt(f.Start); got != 0 {
				t.Fatalf("depth %d seed %d %v: you arrive in region %d, want the first", depth, seed, ks, got)
			}
			if got := f.RegionAt(f.Exit); got != len(ks)-1 {
				t.Fatalf("depth %d seed %d %v: stairs in region %d, want the last", depth, seed, ks, got)
			}
			// The crossing is a blend, not a seam: tiles there weigh in
			// for both kinds.
			for k := 1; k < len(ks); k++ {
				row := f.H / 2
				col := f.Regions[k].From
				c := skill.Coord{Q: col - row>>1, R: row}
				wa, wb := f.Weight(c, ks[k-1]), f.Weight(c, ks[k])
				if wa <= 0.05 || wb <= 0.05 || wa+wb < 0.99 || wa+wb > 1.01 {
					t.Fatalf("depth %d %v: at crossing %d weights are %.2f and %.2f", depth, ks, k, wa, wb)
				}
			}
		}
	}
	mixed := kinds[2] + kinds[3]
	if f := float64(mixed) / float64(floors); f < 0.6 || f > 0.9 {
		t.Errorf("%.0f%% of deep floors are mixed, want about 75%%", 100*f)
	}
	if kinds[3] == 0 {
		t.Error("no floor held three kinds")
	}
	for l := Layout(0); l < NumLayouts; l++ {
		if s := float64(second[l]) / float64(mixed); s < 0.08 || s > 0.35 {
			t.Errorf("%s is the second kind on %.0f%% of mixed floors", l, 100*s)
		}
	}
	if len(pairs) < 14 {
		t.Errorf("only %d of the 20 pairs of kinds turned up", len(pairs))
	}
}

// TestMixedCrossingRooms checks that a crossing really mixes: across many
// floors, rooms in the crossing come in both of its kinds.
func TestMixedCrossingRooms(t *testing.T) {
	var g Generator
	both, crossings := 0, 0
	for seed := uint64(1); seed <= 60; seed++ {
		f := g.Generate(seed*0x2545f4914f6cdd1d, 24)
		if !f.Mixed() {
			continue
		}
		crossings++
		var a, b bool
		for _, r := range f.Rooms {
			_, _, tt := f.Mix(r.Center)
			if tt <= 0 || tt >= 1 {
				continue
			}
			a = a || r.Kind == f.Regions[0].Layout
			b = b || r.Kind == f.Regions[1].Layout
		}
		if a && b {
			both++
		}
	}
	if crossings == 0 || float64(both)/float64(crossings) < 0.5 {
		t.Errorf("only %d of %d crossings hold rooms of both kinds", both, crossings)
	}
}

func TestMixedDeterministic(t *testing.T) {
	var g, h Generator
	for seed := uint64(1); seed <= 20; seed++ {
		a := hashFloor(g.Generate(seed*7919, 32))
		h.Generate(seed*31, 5) // a different floor between, on the other generator
		if b := hashFloor(h.Generate(seed*7919, 32)); a != b {
			t.Fatalf("seed %d: the same mixed floor came out two ways", seed)
		}
	}
}

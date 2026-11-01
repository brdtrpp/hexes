package main

import (
	"math"
	"testing"

	"hexes/game"
	"hexes/skill"
)

// The editor's layout for every shape on phone and desktop screens: the
// shape's tiles stay on screen, never overlap each other, the SHAPE row,
// the pack or the info bar, and are big enough to tap. Runs under
// GOOS=js (see README) since the desktop build needs X11 headers.
func TestEditorLayoutFitsEveryShape(t *testing.T) {
	screens := []struct {
		name    string
		w, h, k float64
	}{
		{"phone portrait", 390, 844, 3},
		{"small phone", 360, 640, 2},
		{"phone landscape", 844, 390, 3},
		{"desktop", 1280, 800, 1},
	}
	sim := game.New(1)
	for _, sc := range screens {
		for k := skill.ShapeKind(0); k < skill.NumShapes; k++ {
			g := &Game{sim: sim, w: int(sc.w * sc.k), h: int(sc.h * sc.k), k: sc.k}
			sim.Load.Hexes[0].Shape = skill.Shape{Kind: k}
			l := g.edLayout()
			W, H := float64(g.w), float64(g.h)
			inr := l.slotR * math.Sqrt(3) / 2 // a flat-top tile's half height
			if l.slotR/sc.k < 26 {
				t.Errorf("%s %s: tiles %.0f css px across, too small to tap", sc.name, k, 2*l.slotR/sc.k)
			}
			top := l.shape.y + l.shape.h
			if l.test.x < l.shape.x+l.shape.w || l.test.x+l.test.w > W || l.test.y != l.shape.y {
				t.Errorf("%s %s: TEST button misplaced", sc.name, k)
			}
			if l.test.w/sc.k < 60 || l.capW/sc.k < 60 {
				t.Errorf("%s %s: TEST %.0f and caption %.0f css px wide", sc.name, k, l.test.w/sc.k, l.capW/sc.k)
			}
			for i, c := range l.slots {
				if !k.Has(i) {
					continue
				}
				if c[0]-l.slotR < 0 || c[0]+l.slotR > W || c[1]-inr < top || c[1]+inr > H {
					t.Errorf("%s %s: slot %d off its area at %.0f,%.0f", sc.name, k, i, c[0], c[1])
				}
				for j, d := range l.slots {
					if j > i && k.Has(j) && math.Hypot(c[0]-d[0], c[1]-d[1]) < 2*inr*0.98 {
						t.Errorf("%s %s: slots %d and %d overlap", sc.name, k, i, j)
					}
				}
				for _, r := range append([]rect{l.info, l.scrap, l.close, l.cells[0], l.cells[len(l.cells)-1]}, l.tabs[:]...) {
					if c[0]+l.slotR > r.x && c[0]-l.slotR < r.x+r.w && c[1]+inr > r.y && c[1]-inr < r.y+r.h {
						t.Errorf("%s %s: slot %d overlaps a control", sc.name, k, i)
					}
				}
			}
		}
	}
	sim.Load.Hexes[0].Shape = skill.Shape{}
}

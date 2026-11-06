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
	shapes := []skill.Shape{}
	for k := skill.ShapeKind(0); k < skill.ShapeGen; k++ {
		shapes = append(shapes, skill.Shape{Kind: k})
	}
	shapes = append(shapes, game.TestShapeSet...)
	// And a spread of deep drops, the biggest and widest there are.
	r := uint64(1)
	next := func() uint64 { r = r*6364136223846793005 + 1442695040888963407; return r ^ r>>29 }
	for i := 0; i < 200; i++ {
		shapes = append(shapes, skill.RollShape(next, 1, 60))
	}
	for _, sc := range screens {
		for _, sh := range shapes {
			k := sh
			g := &Game{sim: sim, w: int(sc.w * sc.k), h: int(sc.h * sc.k), k: sc.k}
			sim.Load.Hexes[0].Shape = sh
			l := g.edLayout()
			W, H := float64(g.w), float64(g.h)
			inr := l.slotR * math.Sqrt(3) / 2 // a flat-top tile's half height
			if l.slotR/sc.k < 26 {
				t.Errorf("%s %s: tiles %.0f css px across, too small to tap", sc.name, k, 2*l.slotR/sc.k)
			}
			top := l.shape.y + l.shape.h
			// The PACK and FRAMES tabs sit over the grid, clear of it.
			for _, r := range []rect{l.pack, l.locker, l.frames} {
				if r.w/sc.k < 80 || r.h/sc.k < 28 || overlaps(r, l.cells[0]) || overlaps(r, l.cells[3]) || overlaps(r, l.info) || overlaps(r, l.shape) || overlaps(r, l.test) {
					t.Errorf("%s %s: grid tab misplaced %+v", sc.name, k, r)
				}
			}
			if overlaps(l.pack, l.frames) || overlaps(l.pack, l.locker) || overlaps(l.locker, l.frames) {
				t.Errorf("%s %s: grid tabs overlap", sc.name, k)
			}
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
				for _, r := range append([]rect{l.info, l.scrap, l.close, l.bind, l.pack, l.frames, l.cells[0], l.cells[len(l.cells)-1]}, l.tabs[:]...) {
					if c[0]+l.slotR > r.x && c[0]-l.slotR < r.x+r.w && c[1]+inr > r.y && c[1]-inr < r.y+r.h {
						t.Errorf("%s %s: slot %d overlaps a control", sc.name, k, i)
					}
				}
			}
		}
	}
	sim.Load.Hexes[0].Shape = skill.Shape{}
}

// The board view: at its starting zoom the whole board shows inside its
// area, cells big enough to tap, and the buttons don't overlap.
func TestBoardLayoutFits(t *testing.T) {
	for _, sc := range []struct {
		name    string
		w, h, k float64
	}{
		{"phone portrait", 390, 844, 3},
		{"small phone", 360, 640, 2},
		{"phone landscape", 844, 390, 3},
		{"desktop", 1280, 800, 1},
	} {
		g := &Game{sim: game.New(1), w: int(sc.w * sc.k), h: int(sc.h * sc.k), k: sc.k}
		g.ed.bd.zoom = 1
		l := g.edLayout()
		b := g.boardLayout(&l)
		if b.tileR/sc.k < 24 {
			t.Errorf("%s: board cells %.0f css px across", sc.name, 2*b.tileR/sc.k)
		}
		for _, c := range game.BoardOrder {
			x, y := b.at(c)
			if x-b.tileR < b.area.x-1 || x+b.tileR > b.area.x+b.area.w+1 || y-b.tileR < b.area.y-1 || y+b.tileR > b.area.y+b.area.h+1 {
				t.Errorf("%s: cell %v off the board area", sc.name, c)
			}
			if got, ok := b.cellAt(x, y); !ok || got != c {
				t.Errorf("%s: tapping cell %v finds %v", sc.name, c, got)
			}
		}
		btn := []rect{b.rotate, b.edit, b.test}
		for i, r := range btn {
			if r.x < 0 || r.x+r.w > float64(g.w) || overlaps(r, b.area) {
				t.Errorf("%s: button %d misplaced", sc.name, i)
			}
			for _, q := range btn[i+1:] {
				if overlaps(r, q) {
					t.Errorf("%s: buttons overlap", sc.name)
				}
			}
		}
	}
}

// The grid tab: about four rings show at the starting zoom, cells are
// big enough to tap, tapping a cell finds it, and the controls stay clear
// of the grid.
func TestGridLayoutFits(t *testing.T) {
	for _, sc := range []struct {
		name    string
		w, h, k float64
	}{
		{"phone portrait", 390, 844, 3},
		{"small phone", 360, 640, 2},
		{"phone landscape", 844, 390, 3},
		{"desktop", 1280, 800, 1},
	} {
		g := &Game{sim: game.New(1), w: int(sc.w * sc.k), h: int(sc.h * sc.k), k: sc.k}
		g.ed.gv.zoom = 1
		l := g.edLayout()
		b := g.gridLayout(&l)
		if b.size/sc.k < 16 {
			t.Errorf("%s: grid cells %.0f css px across", sc.name, 2*b.size/sc.k)
		}
		for i, n := range g.sim.Grid.Nodes {
			x, y := b.at(n.At)
			if n.Ring <= 3 && (x < b.area.x || x > b.area.x+b.area.w || y < b.area.y || y > b.area.y+b.area.h) {
				t.Errorf("%s: ring %d cell off the grid area at the start", sc.name, n.Ring)
			}
			if got := g.gridCellAt(&b, x+b.size*0.3, y-b.size*0.3); got != i {
				t.Fatalf("%s: tapping cell %d finds %d", sc.name, i, got)
			}
		}
		btn := []rect{b.status, b.test, b.buy, b.signal, l.close, l.info}
		for i, r := range btn {
			if r.x < 0 || r.x+r.w > float64(g.w) || overlaps(r, b.area) {
				t.Errorf("%s: control %d misplaced", sc.name, i)
			}
			for _, q := range btn[i+1:] {
				if overlaps(r, q) {
					t.Errorf("%s: controls %d overlap", sc.name, i)
				}
			}
		}
		for _, tb := range l.tabs {
			if tb.x < 0 || tb.x+tb.w > float64(g.w) || overlaps(tb, b.area) || tb.w/sc.k < 50 {
				t.Errorf("%s: tab misplaced or %.0f css px wide", sc.name, tb.w/sc.k)
			}
		}
	}
}

func overlaps(r, q rect) bool {
	return r.x < q.x+q.w && q.x < r.x+r.w && r.y < q.y+q.h && q.y < r.y+r.h
}

// The condition editor fits phone and desktop screens: every button on
// screen, none overlapping another, all big enough to tap.
func TestCondLayoutFits(t *testing.T) {
	for _, sc := range []struct {
		name    string
		w, h, k float64
	}{
		{"phone portrait", 390, 844, 3},
		{"small phone", 360, 640, 2},
		{"phone landscape", 844, 390, 3},
		{"desktop", 1280, 800, 1},
	} {
		g := &Game{sim: game.New(1), w: int(sc.w * sc.k), h: int(sc.h * sc.k), k: sc.k}
		l := g.condLayout()
		rs := []rect{l.minus, l.plus, l.value, l.cycle, l.del, l.cancel, l.clear, l.done}
		for _, k := range palette {
			rs = append(rs, l.pal[k])
		}
		rs = append(rs, l.row[:]...)
		W, H := float64(g.w), float64(g.h)
		for i, a := range rs {
			if a.x < 0 || a.y < 0 || a.x+a.w > W+0.5 || a.y+a.h > H+0.5 {
				t.Errorf("%s: piece %d off screen %+v", sc.name, i, a)
			}
			if a.w/sc.k < 28 || a.h/sc.k < 28 {
				t.Errorf("%s: piece %d too small %+v", sc.name, i, a)
			}
			for j, b := range rs[:i] {
				if a.x < b.x+b.w-0.5 && b.x < a.x+a.w-0.5 && a.y < b.y+b.h-0.5 && b.y < a.y+a.h-0.5 {
					t.Errorf("%s: pieces %d and %d overlap", sc.name, i, j)
				}
			}
		}
		if l.info.h/sc.k < 30 {
			t.Errorf("%s: info line %.0f css px tall", sc.name, l.info.h/sc.k)
		}
	}
}

func TestTestMenuFits(t *testing.T) {
	for _, sc := range []struct {
		name    string
		w, h, k float64
	}{
		{"phone portrait", 390, 844, 3},
		{"small phone", 360, 640, 2},
		{"phone landscape", 844, 390, 3},
		{"desktop", 1280, 800, 1},
	} {
		g := &Game{sim: game.New(1), w: int(sc.w * sc.k), h: int(sc.h * sc.k), k: sc.k}
		n := len(game.Examples(g.sim.Reg))
		l := g.testLayout(n)
		// Every example is on some page.
		seen := map[int]bool{}
		for pg := 0; pg < max(1, l.pages); pg++ {
			g.tm.page = pg
			pl := g.testLayout(n)
			for j := range pl.rows {
				seen[pl.first+j] = true
			}
		}
		if len(seen) != n {
			t.Errorf("%s: pages show %d of %d examples", sc.name, len(seen), n)
		}
		g.tm.page = 0
		rs := append([]rect{l.mine, l.off, l.closeBtn}, l.rows...)
		if l.more.w > 0 {
			rs = append(rs, l.more)
		}
		W, H := float64(g.w), float64(g.h)
		for i, a := range rs {
			if a.x < 0 || a.y < 0 || a.x+a.w > W+0.5 || a.y+a.h > H+0.5 {
				t.Errorf("%s: piece %d off screen %+v", sc.name, i, a)
			}
			if a.w/sc.k < 28 || a.h/sc.k < 36 {
				t.Errorf("%s: piece %d too small %+v", sc.name, i, a)
			}
			for j, b := range rs[:i] {
				if a.x < b.x+b.w-0.5 && b.x < a.x+a.w-0.5 && a.y < b.y+b.h-0.5 && b.y < a.y+a.h-0.5 {
					t.Errorf("%s: pieces %d and %d overlap", sc.name, i, j)
				}
			}
		}
		// A blurb fits its row: about 7 css px a character at the small size.
		for _, e := range game.Examples(g.sim.Reg) {
			if need := float64(len(e.Blurb))*7*sc.k + 20*sc.k; need > l.rows[0].w {
				t.Errorf("%s: %s blurb too wide", sc.name, e.Name)
			}
		}
	}
}

// The BUTTON switch in the editor sits on screen beside SCRAP and RESUME
// without touching them or the info bar, wide enough for its label.
func TestBindSwitchFits(t *testing.T) {
	for _, sc := range []struct {
		name    string
		w, h, k float64
	}{
		{"phone portrait", 390, 844, 3},
		{"small phone", 360, 640, 2},
		{"phone landscape", 844, 390, 3},
		{"desktop", 1280, 800, 1},
	} {
		g := &Game{sim: game.New(1), w: int(sc.w * sc.k), h: int(sc.h * sc.k), k: sc.k}
		l := g.edLayout()
		if l.bind.w/sc.k < 96 || l.bind.h/sc.k < 44 {
			t.Errorf("%s: BUTTON switch %.0fx%.0f css px", sc.name, l.bind.w/sc.k, l.bind.h/sc.k)
		}
		if l.bind.x < 0 || l.bind.x+l.bind.w > float64(g.w) || l.bind.y+l.bind.h > float64(g.h) {
			t.Errorf("%s: BUTTON switch off screen", sc.name)
		}
		for _, r := range []rect{l.scrap, l.close, l.info, l.cells[len(l.cells)-1]} {
			if overlaps(l.bind, r) {
				t.Errorf("%s: BUTTON switch overlaps %+v", sc.name, r)
			}
		}
	}
}

// The four cast buttons, VENT and HEX sit on screen in the bottom right,
// none overlapping (with the thumb slop hit() allows), the small ones
// still big enough for a thumb, and clear of the minimap.
func TestCastButtonsFit(t *testing.T) {
	for _, sc := range []struct {
		name    string
		w, h, k float64
	}{
		{"phone portrait", 390, 844, 3},
		{"small phone", 360, 640, 2},
		{"phone landscape", 844, 390, 3},
		{"desktop", 1280, 800, 1},
	} {
		g := &Game{sim: game.New(1)}
		g.w, g.h, g.k = int(sc.w*sc.k), int(sc.h*sc.k), sc.k
		short := math.Min(sc.w, sc.h)
		r := sc.k * math.Max(34, math.Min(56, short/9))
		g.layoutButtons(g.w, g.h, r)
		bs := append(g.casts[:], g.vent, g.hexBtn)
		for i, b := range bs {
			if b.x-b.r < 0 || b.x+b.r > float64(g.w) || b.y-b.r < 0 || b.y+b.r > float64(g.h) {
				t.Errorf("%s: button %s off screen", sc.name, b.label)
			}
			if b.r/sc.k < 21 {
				t.Errorf("%s: button %s only %.0f css px across", sc.name, b.label, 2*b.r/sc.k)
			}
			if b.x < float64(g.w)*0.4 {
				t.Errorf("%s: button %s is out of right-thumb reach", sc.name, b.label)
			}
			for _, q := range bs[i+1:] {
				if math.Hypot(b.x-q.x, b.y-q.y) < (b.r+q.r)*1.1 {
					t.Errorf("%s: buttons %s and %s overlap", sc.name, b.label, q.label)
				}
			}
		}
	}
}

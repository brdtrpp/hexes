package main

import (
	"fmt"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"

	"hexes/skill"
)

// The condition editor: tap an IF gate in a hex, tap it again, and it
// opens here as a row of tiles. Tap a tile in the palette to add it after
// the picked one, tap a tile in the row to pick it, then nudge its number
// or take it out. DONE puts the condition on the gate if it reads right.
// It also builds a watch trigger's condition (see watch).

type condEd struct {
	on   bool
	hex  int
	slot int
	c    skill.Cond
	sel  int // picked tile in the row, or -1
	msg  string
	bad  bool
	// watch is set when the condition belongs to a watch trigger.
	watch bool
}

// condLayout is where the condition editor's pieces sit.
type condLayout struct {
	panel       rect
	row         [skill.MaxCondTiles]rect
	minus, plus rect
	value       rect
	cycle, del  rect
	pal         [skill.NumTiles]rect // by tile kind; TileNone unused
	info        rect
	cancel      rect
	clear, done rect
}

// palette is the order tiles sit in the palette.
var palette = []skill.TileKind{
	skill.TileHull, skill.TileHeat, skill.TilePower, skill.TileDist, skill.TileCrowd, skill.TileHas, skill.TileBank,
	skill.TileLess, skill.TileMore, skill.TileEq, skill.TileAnd, skill.TileOr, skill.TileNot, skill.TileOpen, skill.TileClose,
}

func (g *Game) condLayout() condLayout {
	var l condLayout
	W, H := float64(g.w), float64(g.h)
	k := g.k
	pad := 14 * k
	pw := math.Min(W-2*pad, 760*k)
	px := (W - pw) / 2
	l.panel = rect{px, pad + 22*k, pw, H - 2*pad - 22*k}
	gap := 6 * k
	// On a wide, short screen the row and its controls go left, the
	// palette and the buttons right; otherwise everything stacks.
	lx, lw := px, pw
	rx, rw := px, pw
	wide := W > H*1.3
	if wide {
		lw = pw*0.55 - gap
		rx, rw = px+pw*0.55+gap, pw*0.45-gap
	}
	y := l.panel.y + 40*k
	// The row: tiles wrap in the column's width.
	tw, th := 58*k, 40*k
	perRow := max(1, int((lw+gap)/(tw+gap)))
	tw = (lw - float64(perRow-1)*gap) / float64(perRow)
	for i := range l.row {
		c, r := i%perRow, i/perRow
		l.row[i] = rect{lx + float64(c)*(tw+gap), y + float64(r)*(th+gap), tw, th}
	}
	rows := (skill.MaxCondTiles + perRow - 1) / perRow
	y += float64(rows)*(th+gap) + 8*k
	// Controls for the picked tile: - value + switch remove.
	cw := (lw - 4*gap) / 5.4
	bh := 44 * k
	l.minus = rect{lx, y, cw, bh}
	l.value = rect{lx + cw + gap, y, cw * 1.4, bh}
	l.plus = rect{l.value.x + l.value.w + gap, y, cw, bh}
	l.cycle = rect{l.plus.x + cw + gap, y, cw, bh}
	l.del = rect{l.cycle.x + cw + gap, y, cw, bh}
	y += bh + 14*k
	if wide {
		y = l.panel.y + 40*k
	}
	// The palette: 5 across, 3 down.
	cols := 5
	pcw := (rw - float64(cols-1)*gap) / float64(cols)
	ch := 44 * k
	for i, t := range palette {
		c, r := i%cols, i/cols
		l.pal[t] = rect{rx + float64(c)*(pcw+gap), y + float64(r)*(ch+gap), pcw, ch}
	}
	y += 3*(ch+gap) + 6*k
	// The bottom: info, then CANCEL, CLEAR, DONE.
	fh := 50 * k
	by := l.panel.y + l.panel.h - fh
	l.info = rect{rx, y, rw, by - 8*k - y}
	w3 := (rw - 2*gap) / 3
	l.cancel = rect{rx, by, w3, fh}
	l.clear = rect{rx + w3 + gap, by, w3, fh}
	l.done = rect{rx + 2*(w3+gap), by, w3, fh}
	return l
}

// openCond opens the condition editor on the gate in hex's slot.
func (g *Game) openCond(hex, slot int) bool {
	sl := g.sim.Load.Hex(hex).Slots[slot]
	if sl.Kind != skill.SlotPart {
		return false
	}
	var c skill.Cond
	watch := sl.Part.Kind == skill.KindTrigger && sl.Part.Trigger == skill.Watch
	switch {
	case watch:
		c = sl.Part.Cond
		if c.Len() == 0 {
			c = skill.Cond{{K: skill.TileHull}, {K: skill.TileLess, V: 0.5}}
		}
	case sl.Part.Kind == skill.KindGate:
		var ok bool
		if c, ok = skill.PresetCond(sl.Part); !ok {
			return false
		}
	default:
		return false
	}
	g.cd = condEd{on: true, hex: hex, slot: slot, c: c, sel: c.Len() - 1, watch: watch}
	g.condCheck()
	return true
}

// condCheck says whether the condition reads right, and what it costs.
func (g *Game) condCheck() {
	cd := &g.cd
	p, err := cd.c.Compile()
	if err != nil {
		cd.msg, cd.bad = "Not yet: "+err.Error()+".", true
		return
	}
	if cd.watch {
		who := fmt.Sprintf("Tests each foe within %g hexes and lands on the ones that pass.", skill.WatchRange)
		if !p.Foe {
			who = "Reads only the rig, and lands on the nearest foe."
		}
		cd.msg, cd.bad = fmt.Sprintf("WATCH %s. %s Adds %g heat every %g s check.", cd.c.Text(g.sim.Reg), who, skill.WatchHeat, skill.WatchEvery), false
		return
	}
	how := "Tests each foe and splits the group."
	if !p.Foe {
		how = "Reads only the rig, so the whole group goes one way."
	}
	cd.msg, cd.bad = fmt.Sprintf("IF %s. %s Adds %.2g heat per firing.", cd.c.Text(g.sim.Reg), how, p.Heat()), false
}

func (g *Game) condTap(x, y float64) {
	l := g.condLayout()
	cd := &g.cd
	sim := g.sim
	switch {
	case l.cancel.hit(x, y):
		cd.on = false
		g.ed.msg, g.ed.bad = "Condition unchanged.", false
		return
	case l.clear.hit(x, y):
		cd.c, cd.sel = skill.Cond{}, -1
		g.condCheck()
		return
	case l.done.hit(x, y):
		if err := sim.SetCond(cd.hex, cd.slot, cd.c); err != nil {
			cd.msg, cd.bad = err.Error()+".", true
			return
		}
		cd.on = false
		g.edResult(nil)
		return
	case l.del.hit(x, y):
		if cd.sel >= 0 {
			cd.c.Delete(cd.sel)
			cd.sel = min(cd.sel, cd.c.Len()-1)
			g.condCheck()
		}
		return
	case l.minus.hit(x, y), l.plus.hit(x, y):
		if cd.sel >= 0 {
			d := 1.0
			if l.minus.hit(x, y) {
				d = -1
			}
			g.condNudge(cd.sel, d)
			g.condCheck()
		}
		return
	case l.cycle.hit(x, y):
		if cd.sel >= 0 {
			g.condCycle(cd.sel)
			g.condCheck()
		}
		return
	}
	n := cd.c.Len()
	for i := 0; i < n; i++ {
		if l.row[i].hit(x, y) {
			if cd.sel == i {
				cd.sel = -1
			} else {
				cd.sel = i
			}
			return
		}
	}
	for _, k := range palette {
		if !l.pal[k].hit(x, y) {
			continue
		}
		if !sim.Knows(k) {
			cd.msg, cd.bad = fmt.Sprintf("You haven't found the %s tile yet. It drops as %s loot.", k, k.Tier()), true
			return
		}
		at := cd.sel + 1
		if cd.sel < 0 {
			at = n
		}
		if !cd.c.Insert(at, defaultTile(k, sim.Reg)) {
			cd.msg, cd.bad = fmt.Sprintf("A condition holds %d tiles at most.", skill.MaxCondTiles), true
			return
		}
		cd.sel = at
		// A sensor that needs a compare gets one, so it reads at once.
		if k.Sensor() && k != skill.TileHas && sim.Knows(skill.TileLess) {
			cmp := skill.Tile{K: skill.TileLess, V: defaultNumber(k)}
			if k == skill.TileCrowd || k == skill.TileDist || k == skill.TileBank {
				cmp.K = skill.TileMore
			}
			if cd.c.Insert(at+1, cmp) {
				cd.sel = at + 1
			}
		}
		g.condCheck()
		return
	}
}

// defaultTile is a fresh tile of kind k.
func defaultTile(k skill.TileKind, reg *skill.Registry) skill.Tile {
	t := skill.Tile{K: k}
	switch k {
	case skill.TileCrowd:
		t.V = 1
	case skill.TileHas:
		t.T = statusTypes(reg)[0]
	case skill.TileLess, skill.TileMore, skill.TileEq:
		t.V = 0.5
	}
	return t
}

// defaultNumber is what a compare after sensor k starts at.
func defaultNumber(k skill.TileKind) float32 {
	switch k {
	case skill.TileCrowd:
		return 1
	case skill.TileDist:
		return 2
	case skill.TileBank:
		return 4
	}
	return 0.5
}

// condNudge steps tile i's number: a share by 5%, a count by 1.
func (g *Game) condNudge(i int, d float64) {
	t := &g.cd.c[i]
	switch {
	case t.K == skill.TileCrowd:
		t.V = float32(math.Max(1, math.Min(4, float64(t.V)+d)))
	case t.K.Compare():
		s := g.cd.c[max(0, i-1)].K
		if i == 0 {
			s = skill.TileNone
		}
		if s.Fraction() || s == skill.TileNone {
			t.V = float32(math.Max(0, math.Min(1, math.Round((float64(t.V)+0.05*d)*20)/20)))
		} else {
			t.V = float32(math.Max(0, math.Min(24, math.Round(float64(t.V)+d))))
		}
	}
}

// condCycle steps tile i's type (has), bank (bank) or compare.
func (g *Game) condCycle(i int) {
	t := &g.cd.c[i]
	switch {
	case t.K == skill.TileHas:
		ts := statusTypes(g.sim.Reg)
		for j, id := range ts {
			if id == t.T {
				t.T = ts[(j+1)%len(ts)]
				return
			}
		}
		t.T = ts[0]
	case t.K == skill.TileBank:
		t.B = (t.B + 1) % skill.NumBanks
	case t.K.Compare():
		for range 3 {
			t.K = skill.TileLess + (t.K-skill.TileLess+1)%3
			if g.sim.Knows(t.K) {
				return
			}
		}
	}
}

// statusTypes is every damage type with a status, for has.
func statusTypes(reg *skill.Registry) []skill.TypeID {
	var out []skill.TypeID
	for i := 0; i < reg.Len(); i++ {
		if reg.Get(skill.TypeID(i)).Status != skill.StatusNone {
			out = append(out, skill.TypeID(i))
		}
	}
	if len(out) == 0 {
		out = append(out, 0)
	}
	return out
}

func tileCol(k skill.TileKind) color.RGBA {
	switch {
	case k.Sensor():
		return color.RGBA{0x7d, 0xf0, 0xe0, 0xff}
	case k.Compare():
		return color.RGBA{0xff, 0xe0, 0x60, 0xff}
	}
	return colGate
}

func (g *Game) drawCond(screen *ebiten.Image) {
	l := g.condLayout()
	cd := &g.cd
	sim := g.sim
	g.drawRect(screen, rect{0, 0, float64(g.w), float64(g.h)}, fade(colPanel, 1), fade(colPanel, 1))
	title := "IF  ·  build the gate's condition"
	if g.cd.watch {
		title = "WATCH  ·  build what the watch looks for"
	}
	g.label(screen, g.face, title, l.panel.x, l.panel.y+12*g.k, colGate, false)
	n := cd.c.Len()
	for i := 0; i < skill.MaxCondTiles; i++ {
		r := l.row[i]
		if i >= n {
			if i == n {
				g.drawRect(screen, r, colPanel, fade(colDim, 0.35))
				g.label(screen, g.small, "+", r.x+r.w/2, r.y+r.h/2, fade(colDim, 0.6), true)
			}
			continue
		}
		t := cd.c[i]
		c := tileCol(t.K)
		fill := fade(c, 0.14)
		if i == cd.sel {
			fill = fade(c, 0.45)
		}
		g.drawRect(screen, r, fill, c)
		s := skill.TileNone
		if i > 0 {
			s = cd.c[i-1].K
		}
		g.label(screen, g.small, t.Text(sim.Reg, s), r.x+r.w/2, r.y+r.h/2, colText, true)
	}
	// Controls for the picked tile.
	var sel skill.Tile
	if cd.sel >= 0 && cd.sel < n {
		sel = cd.c[cd.sel]
	}
	num := sel.K == skill.TileCrowd || sel.K.Compare()
	cyc := sel.K == skill.TileHas || sel.K == skill.TileBank || sel.K.Compare()
	button := func(r rect, s string, on bool) {
		if on {
			g.drawRect(screen, r, fade(colGate, 0.2), colGate)
			g.label(screen, g.small, s, r.x+r.w/2, r.y+r.h/2, colText, true)
		} else {
			g.drawRect(screen, r, colPanel, fade(colDim, 0.3))
			g.label(screen, g.small, s, r.x+r.w/2, r.y+r.h/2, fade(colDim, 0.5), true)
		}
	}
	button(l.minus, "-", num)
	button(l.plus, "+", num)
	val := "pick a tile"
	if sel.K != skill.TileNone {
		s := skill.TileNone
		if cd.sel > 0 {
			s = cd.c[cd.sel-1].K
		}
		val = sel.Text(sim.Reg, s)
	}
	g.label(screen, g.small, val, l.value.x+l.value.w/2, l.value.y+l.value.h/2, colText, true)
	cl := "SWITCH"
	switch {
	case sel.K == skill.TileHas:
		cl = "TYPE"
	case sel.K == skill.TileBank:
		cl = "BANK"
	case sel.K.Compare():
		cl = "< > =="
	}
	button(l.cycle, cl, cyc)
	button(l.del, "REMOVE", sel.K != skill.TileNone)
	// The palette.
	for _, k := range palette {
		r := l.pal[k]
		c := tileCol(k)
		name := k.String()
		switch k {
		case skill.TileDist:
			name = "dist"
		case skill.TileCrowd:
			name = "crowd"
		}
		if sim.Knows(k) {
			g.drawRect(screen, r, fade(c, 0.12), c)
			g.label(screen, g.small, name, r.x+r.w/2, r.y+r.h/2, c, true)
		} else {
			g.drawRect(screen, r, colPanel, fade(colDim, 0.25))
			g.label(screen, g.small, name+" ?", r.x+r.w/2, r.y+r.h/2, fade(colDim, 0.45), true)
		}
	}
	ic := colText
	if cd.bad {
		ic = colHot
	}
	g.wrapLabel(screen, cd.msg, l.info, ic)
	button(l.cancel, "CANCEL", true)
	button(l.clear, "CLEAR", true)
	g.drawRect(screen, l.done, fade(colStairs, 0.25), colStairs)
	g.label(screen, g.face, "DONE", l.done.x+l.done.w/2, l.done.y+l.done.h/2, colStairs, true)
}

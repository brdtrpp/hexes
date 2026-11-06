package main

// The hex editor: a paused overlay with one hex's slots laid out in its
// shape (a ring of six, a triangle of three, a ring with a center slot, or
// any generated graph, each slot on its own cell of a small honeycomb) and
// the inventory grid beside it. The SHAPE button switches the hex
// between the shapes you've found. Everything is a tap: tap a part, then tap
// where it goes. Slot to slot rewires the hex, pack to slot swaps loot in,
// slot to an empty cell takes a part out. Every edit recompiles the hex at
// once; a refused one says why in the info bar. No text entry anywhere.
//
// The FRAMES tab over the pack shows the frame rack instead: tap a frame,
// then a slot, to fit it. Fitting scraps the frame that was there, so a
// slot holding a found frame asks for a second tap first.

import (
	"fmt"
	"image/color"
	"math"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"hexes/game"
	"hexes/loot"
	"hexes/skill"
)

var rarityCol = [skill.NumRarities]color.RGBA{
	{0xc8, 0xcd, 0xd8, 0xff}, // common
	{0x5c, 0xe0, 0x7a, 0xff}, // uncommon
	{0x4d, 0xa3, 0xff, 0xff}, // rare
	{0xe0, 0x5c, 0xff, 0xff}, // exotic
}

const (
	pickNone = iota
	pickSlot
	pickCell
	pickFrame  // a frame on the rack: at indexes FrameChoices
	pickLocker // a part in the locker: at indexes Locker
)

type editor struct {
	on   bool
	hex  int // the hex on show
	pick int // what is picked up: pickNone, pickSlot or pickCell
	at   int // which slot or cell
	msg  string
	bad  bool // msg is a refusal
	// shape is set when msg is about the SHAPE button.
	shape bool
	// board is set while the docking board is on show (see board.go).
	board bool
	bd    boardView
	// rack is set while the grid shows the frame rack, page is which
	// twelve, and confirm is the slot that asked for a second tap (a fit
	// that scraps a found frame), -1 for none.
	rack    bool
	page    int
	confirm int
	// help is set while the ? is on: a picked part shows one plain
	// sentence instead of its numbers.
	help bool
	// locker is set while the strip shows the locker, lpage is which
	// twelve. grid is set while the skill grid is on show (gridview.go).
	locker bool
	lpage  int
	grid   bool
	gv     gridView
}

type rect struct{ x, y, w, h float64 }

func (r rect) hit(x, y float64) bool { return x >= r.x && y >= r.y && x < r.x+r.w && y < r.y+r.h }

// edLayout is where everything sits for the current window.
type edLayout struct {
	tabs         [game.MaxSlots + 2]rect // the four hexes, BOARD, then GRID
	ringX, ringY float64
	slotR        float64
	gridX, gridY float64 // where the shape's middle cell (0,0) sits
	plate        bool    // the shape has a name plate in its middle
	slots        [skill.Slots][2]float64
	shape        rect    // the SHAPE button
	test         rect    // the test switch that unlocks every shape
	capW         float64 // room for the shape caption
	capX, capY   float64 // where the shape caption starts
	cells        [loot.InventorySize]rect
	pack, frames rect // the PACK and FRAMES tabs over the grid
	locker       rect // the LOCKER tab between them
	close, scrap rect
	info         rect
	// bind is the hex's cast button: tap it to move the hex to the next.
	bind rect
	// opts are a picked run-now call's options, in the caption row.
	opts [skill.MaxOpts]rect
	// help is the ? at the end of the info bar.
	help rect
}

func (g *Game) edLayout() edLayout {
	var l edLayout
	W, H := float64(g.w), float64(g.h)
	k := g.k
	pad := 14 * k
	wide := W > H*1.1
	short := H/k < 500 // a phone on its side
	// Tabs across the top.
	tw := math.Min(W-2*pad, 760*k)
	tx := (W - tw) / 2
	gap := 6 * k
	cw := (tw - float64(len(l.tabs)-1)*gap) / float64(len(l.tabs))
	tabH := 40 * k
	if short {
		tabH = 34 * k
	}
	for i := range l.tabs {
		l.tabs[i] = rect{tx + float64(i)*(cw+gap), pad + 22*k, cw, tabH}
	}
	// Along the bottom: SCRAP, the info bar and RESUME on one row when
	// there's width, the info bar above the buttons when there isn't.
	bh := 50 * k
	bw := math.Min(150*k, tw/2-gap)
	by := H - pad - bh
	// The BUTTON switch sits beside RESUME on a wide screen and between
	// SCRAP and RESUME on a narrow one.
	if !wide {
		bw = math.Min(150*k, (tw-2*gap)/3)
	}
	l.scrap = rect{tx, by, bw, bh}
	l.close = rect{tx + tw - bw, by, bw, bh}
	if wide {
		kw := bw * 0.85
		l.bind = rect{l.close.x - gap - kw, by, kw, bh}
		l.info = rect{tx + bw + gap, by, l.bind.x - gap - (tx + bw + gap), bh}
	} else {
		l.bind = rect{tx + bw + gap, by, tw - 2*bw - 2*gap, bh}
		l.info = rect{tx, by - 8*k - 64*k, tw, 64 * k}
	}
	hw := math.Min(44*k, l.info.h)
	l.info.w -= hw + gap
	l.help = rect{l.info.x + l.info.w + gap, l.info.y, hw, l.info.h}

	top := l.tabs[0].y + l.tabs[0].h + 12*k
	bot := math.Min(l.info.y, by) - 10*k
	areaH := bot - top
	// The pack is always 4 by 3: cells stay wide enough for their labels.
	cols, rows := 4, 3
	var ring, grid rect
	if wide {
		ring = rect{tx, top, tw * 0.54, areaH}
		grid = rect{tx + tw*0.56, top, tw * 0.44, areaH}
	} else {
		ring = rect{tx, top, tw, areaH * 0.6}
		grid = rect{tx, top + areaH*0.62, tw, areaH * 0.38}
	}
	// A row across the top of the ring area: the SHAPE button and a
	// caption saying what the shape is.
	capH := 32 * k
	l.shape = rect{ring.x, ring.y, math.Min(104*k, ring.w*0.36), capH}
	tw2 := math.Min(92*k, ring.w*0.26)
	l.test = rect{ring.x + ring.w - tw2, ring.y, tw2, capH}
	l.capX, l.capY = l.shape.x+l.shape.w+10*k, l.shape.y+capH/2
	l.capW = l.test.x - 8*k - l.capX
	ow := math.Min(84*k, (l.capW-2*gap)/float64(skill.MaxOpts))
	for i := range l.opts {
		l.opts[i] = rect{l.capX + float64(i)*(ow+gap), ring.y, ow, capH}
	}
	ring.y += capH + 6*k
	ring.h -= capH + 6*k
	l.ringX, l.ringY = ring.x+ring.w/2, ring.y+ring.h/2
	// Flat-top slots tile like a honeycomb: neighbors sit R apart, a tile
	// is just under R/sqrt(3) across. Every shape's slots sit on its own
	// cells (a ring's six around the middle, which is a plate or a Hub's
	// or Fork's center slot), scaled to fill the area and centered.
	sh := &g.sim.Load.Hex(g.ed.hex).Shape
	gr := sh.Graph()
	minX, maxX, minY, maxY := math.Inf(1), math.Inf(-1), math.Inf(1), math.Inf(-1)
	for i := 0; i < int(gr.N); i++ {
		x, y := gr.At[i].XY()
		minX, maxX = math.Min(minX, x), math.Max(maxX, x)
		minY, maxY = math.Min(minY, y), math.Max(maxY, y)
	}
	const tile = 0.54 // a tile's radius over R
	R := math.Min(ring.w/(maxX-minX+2*tile+0.02), ring.h/(maxY-minY+2*tile*0.866+0.01))
	R = math.Min(R, math.Min(ring.w/2.2, ring.h/2.2)) // small shapes don't balloon
	l.gridX, l.gridY = l.ringX-R*(minX+maxX)/2, l.ringY-R*(minY+maxY)/2
	for i := 0; i < int(gr.N); i++ {
		x, y := gr.At[i].XY()
		l.slots[i] = [2]float64{l.gridX + R*x, l.gridY + R*y}
	}
	l.plate = sh.Kind == skill.ShapeRing || sh.Kind == skill.ShapeLoop
	l.slotR = R * 0.54
	// PACK, LOCKER and FRAMES tabs over the grid.
	stripH := 30 * k
	third := (math.Min(grid.w, 4*110*k) - 2*gap) / 3
	sx := grid.x + (grid.w-(3*third+2*gap))/2
	l.pack = rect{sx, grid.y, third, stripH}
	l.locker = rect{sx + third + gap, grid.y, third, stripH}
	l.frames = rect{sx + 2*(third+gap), grid.y, third, stripH}
	grid.y += stripH + 6*k
	grid.h -= stripH + 6*k
	cg := 6 * k
	cs := math.Min(math.Min((grid.w-float64(cols-1)*cg)/float64(cols), (grid.h-float64(rows-1)*cg)/float64(rows)), 90*k)
	gx := grid.x + (grid.w-(float64(cols)*cs+float64(cols-1)*cg))/2
	gy := grid.y + (grid.h-(float64(rows)*cs+float64(rows-1)*cg))/2
	for i := range l.cells {
		c, r := i%cols, i/cols
		l.cells[i] = rect{gx + float64(c)*(cs+cg), gy + float64(r)*(cs+cg), cs, cs}
	}
	return l
}

func (g *Game) openEditor() {
	if g.sim.State != game.Playing {
		return
	}
	if !g.sim.AtBench() {
		g.showHint("Hexes are rewired at a workbench:\none where you arrive, one by the stairs")
		return
	}
	g.ed.on = true
	g.ed.pick = pickNone
	g.ed.confirm = -1
	if !g.edTabOn(g.ed.hex) {
		g.ed.hex = 0
	}
	if g.ed.board && !g.sim.Has(game.UBoard) {
		g.ed.board = false
	}
	if g.ed.rack && !g.sim.Has(game.UFrames) {
		g.ed.rack = false
	}
	if g.ed.grid {
		g.setGrid(true)
	}
	g.cd.on, g.tm.on = false, false
	g.stick.End() // a thumb on the stick shouldn't keep walking after

	g.ed.msg, g.ed.bad = "", false
}

// updateEditor runs the editor while it's open and reports whether it
// took the frame (the game stays paused).
func (g *Game) updateEditor() bool {
	if !g.ed.on {
		if inpututil.IsKeyJustPressed(ebiten.KeyTab) || inpututil.IsKeyJustPressed(ebiten.KeyE) || inpututil.IsKeyJustPressed(ebiten.KeyI) {
			g.openEditor()
			return g.ed.on
		}
		return false
	}
	if (g.cd.on || g.tm.on) && inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		g.cd.on, g.tm.on = false, false
		return true
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyTab) || inpututil.IsKeyJustPressed(ebiten.KeyE) ||
		inpututil.IsKeyJustPressed(ebiten.KeyI) || inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		g.ed.on, g.cd.on, g.tm.on = false, false, false
		return true
	}
	for i, key := range []ebiten.Key{ebiten.KeyDigit1, ebiten.KeyDigit2, ebiten.KeyDigit3, ebiten.KeyDigit4} {
		if inpututil.IsKeyJustPressed(key) && g.edTabOn(i) {
			g.ed.hex, g.ed.pick, g.ed.board, g.ed.grid = i, pickNone, false, false
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyG) && g.edTabOn(game.MaxSlots+1) && !g.cd.on && !g.tm.on {
		g.setGrid(!g.ed.grid)
	}
	if g.ed.grid && !g.cd.on && !g.tm.on {
		g.updateGrid()
		return true
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyB) && g.sim.Has(game.UBoard) {
		g.setBoard(!g.ed.board)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyL) && !g.ed.board && !g.cd.on && !g.tm.on {
		g.edShowLocker()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyF) && g.sim.Has(game.UFrames) && !g.ed.board && !g.cd.on && !g.tm.on {
		g.edShowRack(!g.ed.rack)
	}
	if g.ed.board && !g.cd.on && !g.tm.on {
		g.updateBoard()
		return true
	}
	tap := g.edTap
	switch {
	case g.tm.on:
		tap = g.testTap
	case g.cd.on:
		tap = g.condTap
	}
	for _, id := range inpututil.AppendJustPressedTouchIDs(nil) {
		g.touched = true
		x, y := ebiten.TouchPosition(id)
		tap(float64(x), float64(y))
	}
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		x, y := ebiten.CursorPosition()
		tap(float64(x), float64(y))
	}
	return true
}

func (g *Game) edTap(x, y float64) {
	l := g.edLayout()
	e := &g.ed
	sim := g.sim
	inv := &sim.Stash.Inv
	e.shape = false
	confirm := e.confirm
	e.confirm = -1
	for i, t := range l.tabs {
		if t.hit(x, y) && g.edTabOn(i) {
			switch i {
			case game.MaxSlots:
				g.setBoard(true)
			case game.MaxSlots + 1:
				g.setGrid(true)
			default:
				e.hex, e.pick, e.msg = i, pickNone, ""
			}
			return
		}
	}
	if l.close.hit(x, y) {
		e.on = false
		return
	}
	if l.help.hit(x, y) {
		e.help = !e.help
		e.msg, e.bad = "", false
		return
	}
	if l.bind.hit(x, y) && e.hex < game.MaxSlots && sim.Has(game.UTriggers) {
		g.edBind()
		return
	}
	if l.pack.hit(x, y) {
		g.edShowRack(false)
		return
	}
	if l.locker.hit(x, y) {
		if e.pick == pickCell && inv.Has[e.at] {
			// A picked pack part tapped onto LOCKER moves there.
			if err := sim.PackToLocker(e.at); err != nil {
				e.msg, e.bad = upFirst(err.Error())+".", true
				return
			}
			e.pick, e.msg, e.bad = pickNone, "Moved to the locker.", false
			return
		}
		g.edShowLocker()
		return
	}
	if l.frames.hit(x, y) && sim.Has(game.UFrames) {
		g.edShowRack(true)
		return
	}
	if l.scrap.hit(x, y) && e.rack {
		g.edScrapFrame()
		return
	}
	if l.scrap.hit(x, y) && e.locker {
		e.msg, e.bad = "Locker parts are yours for good: they can't be scrapped.", true
		return
	}
	if l.scrap.hit(x, y) {
		if e.pick == pickCell && inv.Has[e.at] {
			sim.Scrap(e.at)
			e.pick, e.msg, e.bad = pickNone, "Scrapped.", false
		} else {
			e.msg, e.bad = "Pick a part in your pack to scrap it.", true
		}
		return
	}
	if l.test.hit(x, y) {
		e.pick = pickNone
		if !sim.TestShapes {
			if err := sim.SetTestShapes(true); err != nil {
				e.msg, e.bad = err.Error()+".", true
				return
			}
			e.shape = true
			e.msg, e.bad = "Test switch on: every shape, frame and logic tile is unlocked. Tap SHAPE to step through shapes, FRAMES for frames.", false
		}
		g.openTestMenu()
		return
	}
	if p, ok := g.edPickedCall(); ok {
		for i, r := range l.opts {
			if !r.hit(x, y) || p.Opts[i].Value == 0 {
				continue
			}
			if err := sim.TakeOpt(e.hex, e.at, i); err != nil {
				e.msg, e.bad = err.Error()+".", true
				return
			}
			e.msg, e.bad = "Option back in your pack.", false
			return
		}
	}
	if l.shape.hit(x, y) && sim.Has(game.UShapes) {
		sh, err := sim.CycleShape(e.hex)
		e.pick, e.shape = pickNone, true
		if err != nil {
			e.msg, e.bad = err.Error()+".", true
			return
		}
		e.msg, e.bad = game.ShapeText(sh), false
		return
	}
	sh := &sim.Load.Hex(e.hex).Shape
	if l.plate && math.Hypot(x-l.gridX, y-l.gridY) < l.slotR*0.9 {
		// The center plate does what tapping a picked slot again does.
		if e.pick == pickSlot && g.edCycle() {
			return
		}
		e.pick, e.msg = pickNone, ""
		return
	}
	for i, c := range l.slots {
		if !sh.Has(i) || math.Hypot(x-c[0], y-c[1]) > l.slotR*0.95 {
			continue
		}
		hs := sim.Load.Hex(e.hex).Slots[i]
		switch {
		case e.pick == pickFrame:
			g.edFitFrame(i, confirm == i)
		case e.pick == pickLocker:
			g.edResult(sim.LockerIn(e.hex, i, e.at))
		case e.pick == pickCell && inv.Has[e.at] && inv.Parts[e.at].Kind == skill.KindMult &&
			hs.Kind == skill.SlotPart && hs.Part.RunsNow():
			// A multiplier dropped on a library call rides on it as an
			// option and leaves the slot after the call free.
			if err := sim.AddOpt(e.hex, i, e.at); err != nil {
				e.msg, e.bad = err.Error()+".", true
				return
			}
			e.pick, e.at = pickSlot, i
			e.msg, e.bad = "Option on: it scales "+hs.Part.Lib+" inside the call. Tap it above to take it off.", false
		case e.pick == pickCell:
			g.edResult(sim.SwapIn(e.hex, i, e.at))
		case e.pick == pickSlot && e.at != i:
			g.edResult(sim.SwapSlots(e.hex, e.at, i))
		case e.pick == pickSlot:
			// Tapping a picked charge or Call again switches its bank or
			// what it calls; anything else is put down.
			if !g.edCycle() {
				e.pick, e.msg = pickNone, ""
			}
		default:
			e.pick, e.at, e.msg = pickSlot, i, ""
		}
		return
	}
	for i, c := range l.cells {
		if !c.hit(x, y) {
			continue
		}
		if e.rack {
			g.edRackTap(i)
			return
		}
		if e.locker {
			g.edLockerTap(i)
			return
		}
		switch {
		case e.pick == pickSlot:
			g.edResult(sim.SwapIn(e.hex, e.at, i))
		case e.pick == pickCell && e.at != i:
			sim.MoveCell(e.at, i)
			e.pick, e.msg = pickNone, ""
		case e.pick == pickCell:
			e.pick, e.msg = pickNone, ""
		case inv.Has[i]:
			e.pick, e.at, e.msg = pickCell, i, ""
		default:
			e.msg, e.bad = "Empty. Pick a slot first to take its part out here.", false
		}
		return
	}
	e.pick, e.msg = pickNone, ""
}

// edCycle switches the picked slot's charge bank or Call target, and
// reports whether it held one.
func (g *Game) edCycle() bool {
	e, sim := &g.ed, g.sim
	if sim.CycleBank(e.hex, e.at) {
		p := sim.Load.Hex(e.hex).Slots[e.at].Part
		e.msg, e.bad = "Now on bank "+skill.BankName(p.Bank)+".", false
		return true
	}
	if sim.CycleCall(e.hex, e.at) {
		p := sim.Load.Hex(e.hex).Slots[e.at].Part
		e.msg, e.bad = "Now calls "+g.callName(e.hex, p, false)+".", false
		return true
	}
	if lib, ok := sim.CycleWatch(e.hex, e.at); ok {
		if lib != "" {
			e.msg, e.bad = "Now watches "+lib+": fires when it finds anyone.", false
			return true
		}
		return g.openCond(e.hex, e.at)
	}
	return g.openCond(e.hex, e.at)
}

// edPickedCall is the run-now call in the picked slot, if that's what is
// picked.
func (g *Game) edPickedCall() (skill.Part, bool) {
	e := &g.ed
	if e.pick != pickSlot || !g.sim.Load.Has(e.hex) || e.at < 0 || e.at >= skill.Slots {
		return skill.Part{}, false
	}
	sl := g.sim.Load.Hex(e.hex).Slots[e.at]
	return sl.Part, sl.Kind == skill.SlotPart && sl.Part.RunsNow()
}

// edShowRack switches the grid between the pack and the frame rack.
// Tapping FRAMES while it shows turns the page when there are more than
// twelve frames.
func (g *Game) edShowRack(on bool) {
	e := &g.ed
	if on && e.rack {
		if n := len(g.sim.FrameChoices()); n > loot.InventorySize {
			e.page = (e.page + 1) % ((n + loot.InventorySize - 1) / loot.InventorySize)
		}
	} else {
		e.page = 0
	}
	if e.pick == pickCell || e.pick == pickFrame || e.pick == pickLocker {
		e.pick = pickNone
	}
	e.rack, e.locker, e.msg, e.bad = on, false, "", false
	if on && len(g.sim.FrameChoices()) == 0 {
		e.msg = "No frames yet: they drop from foes, elites most. TEST unlocks every one to try."
	}
}

// edFrames is the frame rack page on show: up to twelve frames and the
// index of the first in FrameChoices.
func (g *Game) edFrames() ([]skill.Frame, int) {
	all := g.sim.FrameChoices()
	lo := min(g.ed.page*loot.InventorySize, len(all))
	hi := min(lo+loot.InventorySize, len(all))
	return all[lo:hi], lo
}

// edPickedFrame is the frame picked on the rack, if that's what is picked.
func (g *Game) edPickedFrame() (skill.Frame, bool) {
	e := &g.ed
	all := g.sim.FrameChoices()
	if e.pick != pickFrame || e.at < 0 || e.at >= len(all) {
		return skill.Frame{}, false
	}
	return all[e.at], true
}

// edRackTap picks or puts down a frame on the rack page.
func (g *Game) edRackTap(cell int) {
	e := &g.ed
	page, lo := g.edFrames()
	switch {
	case cell >= len(page):
		e.pick, e.msg, e.bad = pickNone, "", false
		if len(page) == 0 {
			e.msg = "No frames yet: they drop from foes, elites most. TEST unlocks every one to try."
		}
	case e.pick == pickFrame && e.at == lo+cell:
		e.pick, e.msg = pickNone, ""
	default:
		e.pick, e.at, e.msg, e.bad = pickFrame, lo+cell, "", false
	}
}

// edFitFrame fits the picked frame on slot i. A fit that would scrap a
// found frame waits for a second tap on the same slot.
func (g *Game) edFitFrame(i int, sure bool) {
	e, sim := &g.ed, g.sim
	f, ok := g.edPickedFrame()
	if !ok {
		e.pick = pickNone
		return
	}
	if err := sim.PreviewFrame(e.hex, i, f); err != nil {
		e.msg, e.bad = err.Error()+".", true
		return
	}
	old := sim.Load.Hex(e.hex).Frames[i]
	if !old.Plain() && !game.IsTestFrame(old) && !sure {
		e.confirm = i
		e.msg, e.bad = "That scraps the "+old.Name()+" frame on it for good. Tap the slot again to fit.", true
		return
	}
	scrapped, err := sim.FitFrame(e.hex, i, f)
	if err != nil {
		e.msg, e.bad = err.Error()+".", true
		return
	}
	g.edResult(nil)
	if !scrapped.Plain() {
		e.msg = "Scrapped the " + scrapped.Name() + " frame. " + e.msg
	}
	e.msg = f.Name() + " fitted. " + e.msg
	if all := sim.FrameChoices(); e.page*loot.InventorySize >= len(all) {
		e.page = 0
	}
}

// edScrapFrame is SCRAP while the rack shows: it scraps the picked rack
// frame, or the frame on the picked slot.
func (g *Game) edScrapFrame() {
	e, sim := &g.ed, g.sim
	switch {
	case e.pick == pickFrame:
		f, _ := g.edPickedFrame()
		if game.IsTestFrame(f) {
			e.msg, e.bad = "Test frames can't be scrapped: turn TEST off to put them away.", true
			return
		}
		if e.at < len(sim.Frames) && sim.ScrapFrame(e.at) {
			e.pick, e.msg, e.bad = pickNone, "Scrapped the "+f.Name()+" frame.", false
		}
	case e.pick == pickSlot:
		old, err := sim.ScrapSlotFrame(e.hex, e.at)
		if err != nil {
			e.msg, e.bad = err.Error()+".", true
			return
		}
		e.pick, e.msg, e.bad = pickNone, "Scrapped the "+old.Name()+" frame. The slot is plain.", false
	default:
		e.msg, e.bad = "Pick a frame on the rack, or a slot, to scrap its frame.", true
	}
}

func (g *Game) edResult(err error) {
	e := &g.ed
	if err != nil {
		// Keep holding the part so another slot can be tried.
		e.msg, e.bad = err.Error()+".", true
		return
	}
	e.pick = pickNone
	pw, ht := g.sim.HexPower(e.hex)
	e.msg, e.bad = fmt.Sprintf("%s rebuilt: %.0f power, %.0f heat per firing.", g.sim.Load.Name(e.hex), pw, ht), false
	if g.sim.Load.IsMod(e.hex) {
		e.msg = fmt.Sprintf("%s rebuilt: adds %.0f power, %.0f heat to whatever docks into it.", g.sim.Load.Name(e.hex), pw, ht)
	} else if p := g.sim.Load.Progs[e.hex]; p != nil && p.Inert {
		e.msg, e.bad = fmt.Sprintf("%s won't fire yet: an equipped hex needs an effect, a charge or a call.", g.sim.Load.Name(e.hex)), true
	}
}

// drawShapeEdges draws how a hex's slots feed each other: every link with
// a small arrow for the way it runs, bold for a two-way link and thin for
// one into a merge or out of a split, gold for a one-way quirk, pink for a
// loop back to the trigger. Lines go under the tiles; arrows (arrows true)
// go on top, in the gaps between tiles, so a missing arrow reads as a
// break (a Triangle's end, between a Fork's chains).
func (g *Game) drawShapeEdges(dst *ebiten.Image, l *edLayout, h *skill.Hex, arrows bool) {
	gr := h.Shape.Graph()
	on := color.RGBA{0x5a, 0x66, 0x82, 0xff}
	off := color.RGBA{0x2a, 0x2e, 0x3a, 0xff}
	loop := h.LoopSlot()
	trig := h.TriggerSlot()
	for a := 0; a < int(gr.N); a++ {
		for b := 0; b < int(gr.N); b++ {
			if gr.Out[a]&(1<<b) == 0 {
				continue
			}
			pa, pb := l.slots[a], l.slots[b]
			c, w, arrow := on, g.u(3), fade(colDim, 0.8)
			if gr.Two[a]&(1<<b) == 0 {
				c, w = fade(on, 0.8), g.u(2)
			}
			switch {
			case !h.Feeds(a, b):
				c, arrow = off, color.RGBA{}
			case h.Shape.OneWay() == a && gr.FirstTwo(a) == b:
				c, w, arrow = fade(colShape, 0.7), g.u(4), colShape
			case h.Shape.Kind == skill.ShapeLoop && a == loop && b == trig:
				c, w, arrow = fade(colCall, 0.7), g.u(4), colCall
			}
			if !arrows {
				vector.StrokeLine(dst, float32(pa[0]), float32(pa[1]), float32(pb[0]), float32(pb[1]), w, c, true)
			} else if arrow.A > 0 {
				g.edgeArrow(dst, pa, pb, arrow)
			}
		}
	}
	// A generated loop-back runs from its slot to the trigger, wherever
	// they sit: a pink line under the tiles, tagged on the tile.
	if h.Shape.Kind == skill.ShapeGen && loop >= 0 && !arrows {
		pa, pb := l.slots[loop], l.slots[trig]
		vector.StrokeLine(dst, float32(pa[0]), float32(pa[1]), float32(pb[0]), float32(pb[1]), g.u(3), fade(colCall, 0.55), true)
	}
}

// edgeArrow draws a small arrowhead in the gap between two slot tiles,
// pointing from a to b.
func (g *Game) edgeArrow(dst *ebiten.Image, a, b [2]float64, c color.RGBA) {
	dx, dy := b[0]-a[0], b[1]-a[1]
	d := math.Hypot(dx, dy)
	if d == 0 {
		return
	}
	ux, uy := dx/d, dy/d
	mx, my := (a[0]+b[0])/2, (a[1]+b[1])/2
	s := 7 * g.k
	var p vector.Path
	p.MoveTo(float32(mx+ux*s), float32(my+uy*s))
	p.LineTo(float32(mx-ux*s*0.6-uy*s*0.8), float32(my-uy*s*0.6+ux*s*0.8))
	p.LineTo(float32(mx-ux*s*0.6+uy*s*0.8), float32(my-uy*s*0.6-ux*s*0.8))
	p.Close()
	vs, is := p.AppendVerticesAndIndicesForFilling(nil, nil)
	paint(vs, c)
	dst.DrawTriangles(vs, is, whiteSub(), &ebiten.DrawTrianglesOptions{AntiAlias: true})
}

// partTile draws a part as a hex tile with its two-line label. Ring slots
// are flat-top so they tile; pack cells are pointy-top like the floor.
func (g *Game) partTile(dst *ebiten.Image, x, y, r float64, flat bool, p skill.Part, has, picked bool) {
	fill := color.RGBA{0x1c, 0x21, 0x30, 0xff}
	edge := color.RGBA{0x3a, 0x42, 0x58, 0xff}
	if has {
		edge = rarityCol[p.Rarity]
		fill = color.RGBA{0x22, 0x28, 0x3a, 0xff}
	}
	w := g.u(2)
	if picked {
		fill = fade(edge, 0.35)
		w = g.u(4)
	}
	drawHex(dst, x, y, r, flat, fill, edge, w)
	if !has {
		return
	}
	g.partGlyph(dst, x, y, r*0.6, p)
	title, detail := loot.Label(p, g.sim.Reg)
	if p.Kind == skill.KindCall {
		detail = "to " + g.callName(g.ed.hex, p, true)
	}
	tc := partCol(p)
	if p.Kind == skill.KindCall {
		if to, ok := game.CallsTo(g.ed.hex, p); ok && p.Call != skill.CallSelf && to < game.MaxSlots {
			tc = slotCol[to]
		}
	}
	g.label(dst, g.small, title, x, y-r*0.2, tc, true)
	g.label(dst, g.small, detail, x, y+r*0.25, colDim, true)
	if p.Quirk != skill.QuirkNone {
		// A small diamond on top marks a part that bends a rule.
		d := r * 0.16
		cy := y - r*0.6
		var path vector.Path
		path.MoveTo(float32(x), float32(cy-d))
		path.LineTo(float32(x+d), float32(cy))
		path.LineTo(float32(x), float32(cy+d))
		path.LineTo(float32(x-d), float32(cy))
		path.Close()
		vs, is := path.AppendVerticesAndIndicesForFilling(nil, nil)
		paint(vs, rarityCol[skill.Exotic])
		dst.DrawTriangles(vs, is, whiteSub(), &ebiten.DrawTrianglesOptions{AntiAlias: true})
	}
}

// drawHex draws a hexagon, pointy-top or flat-top.
func drawHex(dst *ebiten.Image, x, y, r float64, flat bool, fill, edge color.RGBA, w float32) {
	var p vector.Path
	off := -math.Pi / 6
	if flat {
		off = 0
	}
	for i := 0; i < 6; i++ {
		a := math.Pi/180*float64(60*i) + off
		px, py := float32(x+r*math.Cos(a)), float32(y+r*math.Sin(a))
		if i == 0 {
			p.MoveTo(px, py)
		} else {
			p.LineTo(px, py)
		}
	}
	p.Close()
	vs, is := p.AppendVerticesAndIndicesForFilling(nil, nil)
	paint(vs, fill)
	dst.DrawTriangles(vs, is, whiteSub(), &ebiten.DrawTrianglesOptions{AntiAlias: true})
	if w > 0 {
		vs, is = p.AppendVerticesAndIndicesForStroke(nil, nil, &vector.StrokeOptions{Width: w, LineJoin: vector.LineJoinRound})
		paint(vs, edge)
		dst.DrawTriangles(vs, is, whiteSub(), &ebiten.DrawTrianglesOptions{AntiAlias: true})
	}
}

func (g *Game) drawRect(dst *ebiten.Image, r rect, fill, edge color.RGBA) {
	vector.DrawFilledRect(dst, float32(r.x), float32(r.y), float32(r.w), float32(r.h), fill, false)
	vector.StrokeRect(dst, float32(r.x), float32(r.y), float32(r.w), float32(r.h), g.u(2), edge, false)
}

func (g *Game) drawEditor(screen *ebiten.Image) {
	sim := g.sim
	e := &g.ed
	l := g.edLayout()
	W, H := float32(g.w), float32(g.h)
	vector.DrawFilledRect(screen, 0, 0, W, H, color.RGBA{0x05, 0x06, 0x0b, 0xe8}, false)
	inv := &sim.Stash.Inv
	g.label(screen, g.face, fmt.Sprintf("HEXES  ·  paused  ·  pack %d/%d", inv.Count(), loot.InventorySize),
		l.tabs[0].x, l.tabs[0].y-11*g.k, colDim, false)

	for i, t := range l.tabs {
		if !g.edTabOn(i) {
			continue // not taught yet
		}
		g.newPulse(screen, t, g.teach.edNew(i))
		if i == game.MaxSlots {
			g.drawBoardTab(screen, t)
			continue
		}
		if i == game.MaxSlots+1 {
			g.drawGridTab(screen, t)
			continue
		}
		fill, edge := colPanel, fade(slotCol[i], 0.45)
		if i == e.hex {
			fill, edge = fade(slotCol[i], 0.25), slotCol[i]
		}
		g.drawRect(screen, t, fill, edge)
		name := sim.Load.Names[i]
		if t.w < 150*g.k {
			name = shortName(name)
		}
		g.label(screen, g.small, name, t.x+t.w/2, t.y+t.h*0.33, slotCol[i], true)
		sub := triggerName(sim.Load.Progs[i])
		if sh := sim.Load.Hexes[i].Shape; sh.Kind != skill.ShapeRing && t.w >= 90*g.k {
			sub += " · " + shapeTag(sh)
		}
		g.label(screen, g.small, sub, t.x+t.w/2, t.y+t.h*0.72, colDim, true)
	}

	if g.tm.on {
		g.drawTestMenu(screen)
		return
	}
	if g.cd.on {
		g.drawCond(screen)
		return
	}
	if e.board {
		g.drawBoard(screen)
		return
	}
	if e.grid {
		g.drawGrid(screen)
		return
	}
	// The shape: edges first, then the center plate (shapes without a
	// center slot), then the slots.
	h := sim.Load.Hex(e.hex)
	kind := h.Shape.Kind
	g.drawShapeEdges(screen, &l, h, false)
	pw, ht := sim.HexPower(e.hex)
	if l.plate {
		px, py := l.gridX, l.gridY
		drawHex(screen, px, py, l.slotR, true, colPanel, hexCol(e.hex), g.u(2))
		g.label(screen, g.small, shortName(sim.Load.Name(e.hex)), px, py-l.slotR*0.3, hexCol(e.hex), true)
		g.label(screen, g.small, fmt.Sprintf("%.0f pw %.0f ht", pw, ht), px, py+l.slotR*0.12, colDim, true)
		if l.slotR > 48*g.k {
			flow := "clockwise"
			if kind == skill.ShapeLoop {
				flow = "loops"
			}
			g.label(screen, g.small, flow, px, py+l.slotR*0.48, fade(colDim, 0.6), true)
		}
	}
	// The SHAPE button and what the shape is.
	if sim.Has(game.UShapes) {
		g.newPulse(screen, l.shape, g.teach.edNewU(game.UShapes))
		g.drawRect(screen, l.shape, fade(colShape, 0.18), colShape)
		g.label(screen, g.small, "SHAPE", l.shape.x+l.shape.w/2, l.shape.y+l.shape.h/2, colShape, true)
	}
	caption := fmt.Sprintf("%s  ·  %.0f pw %.0f ht", h.Shape, pw, ht)
	for _, alt := range []string{h.Shape.String(), h.Shape.Name()} {
		if text.Advance(caption, g.small) <= l.capW {
			break
		}
		caption = alt
	}
	if p, ok := g.edPickedCall(); ok {
		// A picked library call shows its options here instead.
		for i, r := range l.opts {
			o := p.Opts[i]
			if o.Value == 0 {
				g.drawRect(screen, r, colPanel, fade(colDim, 0.4))
				g.label(screen, g.small, "+ option", r.x+r.w/2, r.y+r.h/2, fade(colDim, 0.7), true)
				continue
			}
			c := rarityCol[o.Rarity]
			g.drawRect(screen, r, fade(c, 0.2), c)
			g.label(screen, g.small, fmt.Sprintf("x%.2g %s", 1+(o.Value-1)*o.Rarity.Scale(), shortVerb(o.Verb)), r.x+r.w/2, r.y+r.h/2, colText, true)
		}
	} else {
		g.label(screen, g.small, caption, l.capX, l.capY, colShape, false)
	}
	// The test switch: lit while every shape is unlocked.
	if sim.TestShapes {
		g.drawRect(screen, l.test, fade(colShape, 0.55), colShape)
		g.label(screen, g.small, "TEST ON", l.test.x+l.test.w/2, l.test.y+l.test.h/2, colText, true)
	} else {
		g.drawRect(screen, l.test, colPanel, fade(colDim, 0.6))
		g.label(screen, g.small, "TEST", l.test.x+l.test.w/2, l.test.y+l.test.h/2, colDim, true)
	}

	routes := h.Routes(h.TriggerSlot())
	ret := -1 // a library hex's way back to whoever calls it
	if !h.HasTrigger() {
		ret = h.ReturnSlot()
	}
	loop := -1
	if kind == skill.ShapeGen {
		loop = h.LoopSlot()
	}
	for i, c := range l.slots {
		if !h.Shape.Has(i) {
			continue
		}
		s := h.Slots[i]
		fr := h.Frame(i)
		g.partTile(screen, c[0], c[1], l.slotR, true, s.Part, s.Kind == skill.SlotPart, e.pick == pickSlot && e.at == i)
		if s.Kind != skill.SlotPart {
			sock := "any part"
			switch {
			case h.Sockets[i] == skill.SocketTrigger:
				sock = "trigger"
			case h.Merge(i):
				sock = "pool"
			case fr.Bend == skill.BendEcho:
				sock = "echo"
			}
			g.label(screen, g.small, sock, c[0], c[1], fade(colDim, 0.7), true)
		}
		// The top of the tile: the shape's quirk on this slot, what a
		// charge part's bank holds now, a call that loops on its hex.
		var top []string
		col := colCharge
		if tag := fr.Tag(); tag != "" {
			top, col = append(top, tag), frameCol(fr)
		}
		if s.Kind == skill.SlotPart && s.Part.UsesBank() && e.hex < game.MaxSlots {
			b := s.Part.Bank
			v := uint32(0)
			switch {
			case b < skill.MaxBanks:
				v = sim.Banks[e.hex][b]
			case b < skill.NumBanks:
				v = sim.BoardBanks[b-skill.MaxBanks]
			}
			top = append(top, fmt.Sprintf("%s = %d", skill.BankName(b), v))
		}
		if i == loop {
			top = append(top, "LOOPS TO "+h.SlotName(h.TriggerSlot()))
			if col == colCharge {
				col = colCall
			}
		}
		if i == ret {
			top = append(top, "RETURNS")
		}
		if s.Kind == skill.SlotPart && s.Part.Kind == skill.KindCall && s.Part.Call == skill.CallSelf && !s.Part.RunsNow() {
			top = append(top, "LOOP")
			if col == colCharge {
				col = colCall
			}
		}
		if len(top) > 0 {
			g.label(screen, g.small, strings.Join(top, " · "), c[0], c[1]-l.slotR*0.62, col, true)
		}
		if !fr.Plain() {
			drawHex(screen, c[0], c[1], l.slotR*1.08, true, color.RGBA{}, frameCol(fr), g.u(2))
		}
		// Holding a part: slots where it would do something light up,
		// the rest go dark. Otherwise a part that does nothing where it
		// sits (its target was moved away) gets a red edge.
		switch ok, held := g.edLit(i); {
		case held && ok:
			drawHex(screen, c[0], c[1], l.slotR*1.1, true, color.RGBA{}, colYes, g.u(2.5))
		case held:
			drawHex(screen, c[0], c[1], l.slotR, true, fade(color.RGBA{0, 0, 0, 0xff}, 0.62), color.RGBA{}, 0)
		case s.Kind == skill.SlotPart:
			if _, ok := h.Use(i); !ok {
				drawHex(screen, c[0], c[1], l.slotR*1.04, true, color.RGBA{}, colHot, g.u(2))
			}
		}
		// A gate's two ways: the yes slot gets the ones that pass, the no
		// slot the rest. Tagged at the bottom of the tile.
		switch routes[i].Branch {
		case skill.BranchYes:
			g.label(screen, g.small, "YES", c[0], c[1]+l.slotR*0.62, colYes, true)
		case skill.BranchNo:
			g.label(screen, g.small, "NO", c[0], c[1]+l.slotR*0.62, colHot, true)
		}
	}
	g.drawShapeEdges(screen, &l, h, true)
	g.drawPorts(screen, &l, e.hex)

	g.drawGridTabs(screen, &l)
	if e.rack {
		page, lo := g.edFrames()
		for i, c := range l.cells {
			var f skill.Frame
			if i < len(page) {
				f = page[i]
			}
			g.frameTile(screen, c.x+c.w/2, c.y+c.h/2, c.w*0.5, f, i < len(page), e.pick == pickFrame && e.at == lo+i)
		}
	} else if e.locker {
		page, lo := g.edLocker()
		for i, c := range l.cells {
			var p skill.Part
			if i < len(page) {
				p = page[i]
			}
			g.partTile(screen, c.x+c.w/2, c.y+c.h/2, c.w*0.5, false, p, i < len(page), e.pick == pickLocker && e.at == lo+i)
		}
	} else {
		for i, c := range l.cells {
			g.partTile(screen, c.x+c.w/2, c.y+c.h/2, c.w*0.5, false, inv.Parts[i], inv.Has[i], e.pick == pickCell && e.at == i)
			if inv.Has[i] && sim.Stash.Fresh[i] {
				// Unsecured: a death would drop it in the salvage cache.
				g.label(screen, g.small, "NEW", c.x+c.w-14*g.k, c.y+9*g.k, colSalvage, true)
			}
		}
	}

	// Info bar: the picked part, the last result, or how to use it.
	g.drawRect(screen, l.info, colPanel, color.RGBA{0x3a, 0x42, 0x58, 0xff})
	info, ic := "Tap a part, then tap where it goes. Slot to slot rewires the hex.", colDim
	fr := skill.Frame{}
	if e.pick == pickSlot {
		fr = h.Frame(e.at)
	}
	pf, pickedFrame := g.edPickedFrame()
	switch {
	case e.help && e.pick == pickCell && inv.Has[e.at]:
		info, ic = game.PlainHelp(inv.Parts[e.at]), colText
	case e.help && e.pick == pickSlot:
		info, ic = game.PlainHelp(h.Slots[e.at].Part), colText
		if h.Slots[e.at].Kind != skill.SlotPart {
			info = "An empty slot: put a part here."
		}
	case e.help && !pickedFrame:
		info, ic = "Tap any part to see what it does in plain words. Tap ? again for the numbers.", colText
	case pickedFrame && e.msg != "":
		info, ic = e.msg, colText
		if e.bad {
			ic = colHot
		}
	case pickedFrame:
		info, ic = "Green slots take it; fitting scraps the frame there. "+pf.Text(sim.Reg), frameCol(pf)
		if game.IsTestFrame(pf) {
			info += " Test frame: free to try."
		}
	case e.pick == pickLocker && e.at < len(sim.Locker) && e.help:
		info, ic = game.PlainHelp(sim.Locker[e.at]), colText
	case e.pick == pickLocker && e.at < len(sim.Locker) && e.msg != "" && e.bad:
		info, ic = e.msg, colHot
	case e.pick == pickLocker && e.at < len(sim.Locker):
		p := sim.Locker[e.at]
		info, ic = "Green slots are where it does something. "+loot.Describe(p, sim.Reg), rarityCol[p.Rarity]
	case e.pick == pickCell && inv.Has[e.at] && e.msg != "" && e.bad:
		info, ic = e.msg, colHot // why it can't go there
	case e.pick == pickCell && inv.Has[e.at]:
		info, ic = "Green slots are where it does something. "+loot.Describe(inv.Parts[e.at], sim.Reg), rarityCol[inv.Parts[e.at].Rarity]
		if sim.Stash.Fresh[e.at] {
			info += fmt.Sprintf(" New: slot it or reach depth %d to keep it if you die.", sim.NextCheckpoint())
		}
	case e.shape && e.msg != "":
		info, ic = e.msg, colShape // what the new shape does, or why not
		if e.bad {
			ic = colHot
		}
	case e.pick == pickSlot && h.Slots[e.at].Kind == skill.SlotPart && e.msg != "" && e.bad:
		info, ic = e.msg, colHot // why it can't move there
	case e.pick == pickSlot && h.Slots[e.at].Kind == skill.SlotPart:
		p := h.Slots[e.at].Part
		info, ic = loot.Describe(p, sim.Reg), rarityCol[p.Rarity]
		if what, ok := h.Use(e.at); what != "" {
			// What it does where it sits, first, so it's never cut off.
			lead := "Here it " + what + "."
			if !ok {
				lead, ic = "Does nothing here: "+what+".", colHot
			}
			info = lead + " " + info
		}
		if (p.UsesBank() || p.Kind == skill.KindCall) && e.msg != "" && !e.bad {
			info = e.msg + " " + info
		}
		if !fr.Plain() {
			info += " " + fr.Text(sim.Reg)
		}
	case e.pick == pickSlot && e.msg != "" && e.bad:
		info, ic = e.msg, colHot
	case e.pick == pickSlot:
		info = "Empty slot. Tap a part in your pack to put it here."
		if !fr.Plain() {
			info, ic = "Empty slot. "+fr.Text(sim.Reg), frameCol(fr)
		}
	case e.msg != "":
		info, ic = e.msg, colText
		if e.bad {
			ic = colHot
		}
	}
	g.wrapLabel(screen, info, l.info, ic)
	if e.help {
		g.drawRect(screen, l.help, fade(colText, 0.2), colText)
	} else {
		g.drawRect(screen, l.help, colPanel, fade(colDim, 0.7))
	}
	g.label(screen, g.face, "?", l.help.x+l.help.w/2, l.help.y+l.help.h/2, colText, true)
	if sim.Has(game.UTriggers) {
		g.newPulse(screen, l.bind, g.teach.edNewU(game.UTriggers))
		g.drawBind(screen, l.bind)
	}

	g.drawRect(screen, l.scrap, colPanel, colHot)
	g.label(screen, g.face, "SCRAP", l.scrap.x+l.scrap.w/2, l.scrap.y+l.scrap.h/2, colHot, true)
	g.drawRect(screen, l.close, fade(colStairs, 0.25), colStairs)
	g.label(screen, g.face, "RESUME", l.close.x+l.close.w/2, l.close.y+l.close.h/2, colText, true)
}

// edLit reports, while a part is held (picked from the pack or a slot),
// whether it would do something in slot i. held is false when nothing is
// held, and for the held part's own slot.
func (g *Game) edLit(i int) (ok, held bool) {
	e := &g.ed
	sim := g.sim
	if f, picked := g.edPickedFrame(); picked {
		return sim.PreviewFrame(e.hex, i, f) == nil, true
	}
	switch {
	case e.pick == pickCell && sim.Stash.Inv.Has[e.at]:
		_, ok = sim.PreviewIn(e.hex, i, e.at)
		return ok, true
	case e.pick == pickLocker && e.at < len(sim.Locker):
		_, ok = sim.PreviewPart(e.hex, i, sim.Locker[e.at])
		return ok, true
	case e.pick == pickSlot && e.at != i && sim.Load.Hex(e.hex).Slots[e.at].Kind == skill.SlotPart:
		_, ok = sim.PreviewSwap(e.hex, e.at, i)
		return ok, true
	}
	return false, false
}

// wrapLabel fits text into a box, breaking at spaces into as many lines
// as the box is tall, and cuts the rest with an ellipsis.
func (g *Game) wrapLabel(dst *ebiten.Image, s string, r rect, c color.RGBA) {
	maxChars := max(8, int((r.w-16*g.k)/(7*g.k)))
	maxLines := max(1, int(r.h/(15*g.k)))
	var lines []string
	for len(s) > 0 && len(lines) < maxLines {
		if len(s) <= maxChars {
			lines = append(lines, s)
			s = ""
			break
		}
		cut := maxChars
		for cut > 0 && s[cut] != ' ' {
			cut--
		}
		if cut == 0 {
			cut = maxChars
		}
		lines = append(lines, s[:cut])
		s = s[cut:]
		for len(s) > 0 && s[0] == ' ' {
			s = s[1:]
		}
	}
	if s != "" {
		last := lines[len(lines)-1]
		if len(last) > maxChars-1 {
			last = last[:maxChars-1]
		}
		lines[len(lines)-1] = last + "…"
	}
	lh := r.h / float64(len(lines)+1)
	for i, ln := range lines {
		g.label(dst, g.small, ln, r.x+10*g.k, r.y+lh*float64(i+1), c, false)
	}
}

// callName is the hex a Call points at, seen from hex: "self" or its
// name (short for a tile).
func (g *Game) callName(hex int, p skill.Part, short bool) string {
	if p.RunsNow() {
		if short {
			return shortName(p.Lib)
		}
		return p.Lib + " (runs now)"
	}
	to, ok := game.CallsTo(hex, p)
	switch {
	case !ok:
		return skill.CallName(p.Call)
	case p.Call == skill.CallSelf || to == hex || to >= game.MaxSlots:
		return "self"
	case short:
		return shortName(g.sim.Load.Names[to])
	}
	return g.sim.Load.Names[to]
}

func triggerName(p *skill.Program) string {
	switch p.Trigger {
	case skill.OnCast:
		return "CAST"
	case skill.Periodic:
		return fmt.Sprintf("every %g s", p.Period)
	case skill.Channel:
		return "HOLD"
	case skill.Watch:
		return "WATCH"
	}
	return p.Trigger.String()
}

// drawDrops draws parts lying on the floor: a small diamond in the
// part's rarity color that pops in and pulses.
func (g *Game) drawDrops(screen *ebiten.Image) {
	s := g.size
	for i, d := range g.sim.Stash.Drops {
		x, y := d.At.Pixel()
		sx, sy := g.toScreen(x, y)
		if sx < -s || sy < -s || sx > float64(g.w)+s || sy > float64(g.h)+s || g.seeAt(x, y) < 0.2 {
			continue
		}
		sx += float64(i%3-1) * s * 0.18
		pop := math.Min(1, float64(d.Age)*4)
		r := s * 0.32 * pop * (1 + 0.12*math.Sin(float64(g.frame)*0.15+float64(i)))
		c := rarityCol[d.Part.Rarity]
		var p vector.Path
		p.MoveTo(float32(sx), float32(sy-r*1.3))
		p.LineTo(float32(sx+r), float32(sy))
		p.LineTo(float32(sx), float32(sy+r*1.3))
		p.LineTo(float32(sx-r), float32(sy))
		p.Close()
		vs, is := p.AppendVerticesAndIndicesForFilling(nil, nil)
		paint(vs, c)
		screen.DrawTriangles(vs, is, whiteSub(), &ebiten.DrawTrianglesOptions{AntiAlias: true})
		if d.Part.Rarity >= skill.Rare {
			vector.StrokeCircle(screen, float32(sx), float32(sy), float32(r*1.8), g.u(1.5), fade(c, 0.5), true)
		}
	}
	// A shape lying on the floor: a gold hexagon that turns slowly, with
	// a glow, so it reads as something rarer than any part.
	for _, d := range g.sim.Stash.Shapes {
		x, y := d.At.Pixel()
		sx, sy := g.toScreen(x, y)
		if sx < -s || sy < -s || sx > float64(g.w)+s || sy > float64(g.h)+s || g.seeAt(x, y) < 0.2 {
			continue
		}
		pulse := 1 + 0.15*math.Sin(float64(g.frame)*0.1)
		r := s * 0.42 * pulse
		vector.DrawFilledCircle(screen, float32(sx), float32(sy), float32(r*1.6), fade(colShape, 0.18), true)
		var p vector.Path
		spin := float64(g.frame) * 0.02
		for i := 0; i < 6; i++ {
			a := spin + float64(i)*math.Pi/3
			px, py := float32(sx+r*math.Cos(a)), float32(sy+r*math.Sin(a))
			if i == 0 {
				p.MoveTo(px, py)
			} else {
				p.LineTo(px, py)
			}
		}
		p.Close()
		vs, is := p.AppendVerticesAndIndicesForStroke(nil, nil, &vector.StrokeOptions{Width: g.u(3), LineJoin: vector.LineJoinRound})
		paint(vs, colShape)
		screen.DrawTriangles(vs, is, whiteSub(), &ebiten.DrawTrianglesOptions{AntiAlias: true})
		g.label(screen, g.small, "SHAPE", sx, sy-r*1.9, fade(colShape, 0.9), true)
	}
	// A frame on the floor: an empty hexagon outline in its grade's color,
	// a frame with nothing in it yet.
	for _, d := range g.sim.Stash.Frames {
		x, y := d.At.Pixel()
		sx, sy := g.toScreen(x, y)
		if sx < -s || sy < -s || sx > float64(g.w)+s || sy > float64(g.h)+s || g.seeAt(x, y) < 0.2 {
			continue
		}
		c := frameCol(d.Frame)
		pop := math.Min(1, float64(d.Age)*4+0.25)
		r := s * 0.38 * pop * (1 + 0.08*math.Sin(float64(g.frame)*0.12))
		vector.DrawFilledCircle(screen, float32(sx), float32(sy), float32(r*1.5), fade(c, 0.14), true)
		drawHex(screen, sx, sy, r, true, color.RGBA{}, c, g.u(3))
		drawHex(screen, sx, sy, r*0.6, true, color.RGBA{}, fade(c, 0.6), g.u(1.5))
		g.label(screen, g.small, "FRAME", sx, sy-r*1.9, fade(c, 0.9), true)
	}
	// A logic tile on the floor: an orange square that bobs.
	for _, d := range g.sim.Stash.Tiles {
		x, y := d.At.Pixel()
		sx, sy := g.toScreen(x, y)
		if sx < -s || sy < -s || sx > float64(g.w)+s || sy > float64(g.h)+s || g.seeAt(x, y) < 0.2 {
			continue
		}
		r := s * 0.3
		sy += math.Sin(float64(g.frame)*0.08) * r * 0.25
		vector.DrawFilledCircle(screen, float32(sx), float32(sy), float32(r*1.8), fade(colGate, 0.16), true)
		vector.DrawFilledRect(screen, float32(sx-r), float32(sy-r), float32(2*r), float32(2*r), fade(colGate, 0.35), true)
		vector.StrokeRect(screen, float32(sx-r), float32(sy-r), float32(2*r), float32(2*r), g.u(2), colGate, true)
		g.label(screen, g.small, strings.ToUpper(d.Tile.String()), sx, sy-r*2.4, fade(colGate, 0.9), true)
	}
}

// slotKey is the second line of a HUD card: how the hex fires, and on
// which cast button.
func (g *Game) slotKey(i int) string {
	p := g.sim.Load.Progs[i]
	if p.Inert {
		return "NEEDS EFFECT"
	}
	switch p.Trigger {
	case skill.OnCast:
		return g.keyName(int(g.sim.Load.Keys[i]))
	case skill.Channel:
		return "HOLD " + g.keyName(int(g.sim.Load.Keys[i]))
	}
	return triggerName(p)
}

// keyName is a cast button as the player presses it: KEY 1 at a
// keyboard, BUTTON 1 on a phone.
func (g *Game) keyName(b int) string {
	if g.touched || float64(g.w) < 700*g.k {
		return fmt.Sprintf("BUTTON %d", b)
	}
	return fmt.Sprintf("KEY %d", b)
}

// castAll presses every cast button (the screenshot autopilot uses it).
func (g *Game) castAll(in *game.Input) {
	for b := 1; b <= game.Buttons; b++ {
		g.sim.Load.Press(b, in)
	}
}

// edBind moves the hex on show to the next cast button, or says why it
// has none.
func (g *Game) edBind() {
	e, sim := &g.ed, g.sim
	b, ok := sim.CycleKey(e.hex)
	e.pick, e.shape = pickNone, false
	if !ok {
		e.msg, e.bad = "Only on-cast and hold hexes take a button. Swap a CAST or HOLD trigger into this hex to give it one.", true
		return
	}
	e.msg, e.bad = fmt.Sprintf("%s is on %s.", sim.Load.Names[e.hex], g.keyName(b)), false
	for j := range sim.Load.Keys {
		if j != e.hex && sim.Load.OnButton(j, b) {
			e.msg += fmt.Sprintf(" %s is there too: one press fires both.", sim.Load.Names[j])
		}
	}
}

// drawBind draws the BUTTON switch for the hex on show.
func (g *Game) drawBind(dst *ebiten.Image, r rect) {
	sim := g.sim
	i := g.ed.hex
	if i >= game.MaxSlots {
		return
	}
	c := slotCol[i]
	top, sub := g.keyName(int(sim.Load.Keys[i])), "tap: next button"
	if !sim.Load.Castable(i) {
		c, top, sub = colDim, "NO BUTTON", triggerName(sim.Load.Progs[i])
	}
	g.drawRect(dst, r, colPanel, c)
	g.label(dst, g.face, top, r.x+r.w/2, r.y+r.h*0.38, c, true)
	g.label(dst, g.small, sub, r.x+r.w/2, r.y+r.h*0.74, colDim, true)
}

var (
	colGate   = color.RGBA{0xff, 0xa8, 0x4a, 0xff}
	colCharge = color.RGBA{0xb4, 0x9c, 0xff, 0xff}
	colCall   = color.RGBA{0xff, 0x8a, 0xc8, 0xff}
	colYes    = color.RGBA{0x5c, 0xe0, 0x7a, 0xff}
	colShape  = color.RGBA{0xff, 0xd2, 0x5a, 0xff}
)

var legacyTag = [...]string{"RING", "TRI", "HUB", "FORK", "LOOP"}

// shapeTag is a shape in a word or two for the hex tabs: an original
// shape's tag, or a generated one's family and size, like FORK7.
func shapeTag(sh skill.Shape) string {
	if sh.Kind.Legacy() {
		return legacyTag[sh.Kind]
	}
	name := strings.Fields(sh.Name())
	return fmt.Sprintf("%s%d", strings.ToUpper(name[len(name)-1]), sh.Len())
}

var rarityName = [skill.NumRarities]string{"+COMMON", "+UNCOMMON", "+RARE", "+EXOTIC"}

// shortVerb is a multiplier's kind in a few letters, for an option chip.
func shortVerb(v skill.Verb) string {
	switch v {
	case skill.VerbDamage:
		return "dmg"
	case skill.VerbDuration:
		return "dur"
	case skill.VerbPush:
		return "push"
	case skill.VerbRadius:
		return "rad"
	case skill.VerbCount:
		return "cnt"
	}
	return "all"
}

// frameCol is a frame's color: gold for one that comes with the shape,
// otherwise its grade's.
func frameCol(f skill.Frame) color.RGBA {
	if f.Shaped {
		return colShape
	}
	return rarityCol[f.Grade]
}

// frameTile draws a frame on the rack: an outlined hex in its grade's
// color, its name over what it takes.
func (g *Game) frameTile(dst *ebiten.Image, x, y, r float64, f skill.Frame, has, picked bool) {
	if !has {
		drawHex(dst, x, y, r, false, color.RGBA{0x1c, 0x21, 0x30, 0xff}, color.RGBA{0x3a, 0x42, 0x58, 0xff}, g.u(2))
		return
	}
	c := frameCol(f)
	fill := color.RGBA{0x16, 0x1a, 0x26, 0xff}
	w := g.u(3)
	if picked {
		fill, w = fade(c, 0.35), g.u(5)
	}
	drawHex(dst, x, y, r, false, fill, c, w)
	drawHex(dst, x, y, r*0.72, false, color.RGBA{}, fade(c, 0.45), g.u(1.5))
	fit := r * 1.75
	g.label(dst, g.small, fitWord(g, frameWords(f), fit), x, y-r*0.38, c, true)
	g.label(dst, g.small, fitWord(g, takesWords(f, g.sim.Reg), fit), x, y+r*0.06, colDim, true)
	if game.IsTestFrame(f) {
		g.label(dst, g.small, "TEST", x, y+r*0.5, fade(colShape, 0.8), true)
	}
}

// fitWord is the first of words that fits in w at the small size, or the
// last cut to six letters.
func fitWord(g *Game, words []string, w float64) string {
	for _, s := range words {
		if text.Advance(s, g.small) <= w {
			return s
		}
	}
	last := words[len(words)-1]
	if len(last) > 6 {
		last = last[:6]
	}
	return last
}

// frameWords is a frame's name, longest first, for a small tile.
func frameWords(f skill.Frame) []string {
	n := strings.ToUpper(f.Name())
	short := strings.FieldsFunc(n, func(r rune) bool { return r == ' ' || r == '-' })[0]
	return []string{n, f.Tag(), short}
}

// takesWords is what a frame takes, longest first, for a small tile.
func takesWords(f skill.Frame, reg *skill.Registry) []string {
	full := f.TakesText(reg)
	var short string
	switch f.Takes {
	case skill.TakeAny:
		short = "any"
	case skill.TakeEffect, skill.TakeActs:
		short = "effects"
	case skill.TakeDamage:
		short = "damage"
	case skill.TakeTarget:
		short = "targets"
	case skill.TakeCount:
		short = "nearest"
	case skill.TakeMult:
		short = "mults"
	case skill.TakeTrigger:
		short = "trigger"
	case skill.TakeNone:
		short = "no part"
	}
	if f.Typed && int(f.Type) < reg.Len() {
		short = reg.Get(f.Type).Name + " only"
	} else if f.Ranked {
		short = strings.ToLower(f.Rank.String()) + " only"
	}
	return []string{full, short, strings.Fields(short)[0]}
}

// drawGridTabs draws the PACK and FRAMES tabs over the grid.
func (g *Game) drawGridTabs(dst *ebiten.Image, l *edLayout) {
	e := &g.ed
	n := len(g.sim.FrameChoices())
	tabs := []struct {
		r    rect
		name string
		on   bool
	}{
		{l.pack, fmt.Sprintf("PACK %d/%d", g.sim.Stash.Inv.Count(), loot.InventorySize), !e.rack && !e.locker},
		{l.locker, fmt.Sprintf("LOCKER %d", len(g.sim.Locker)), e.locker},
		{l.frames, fmt.Sprintf("FRAMES %d", len(g.sim.Frames)), e.rack},
	}
	if !g.sim.Has(game.UFrames) {
		tabs = tabs[:2]
	} else {
		g.newPulse(dst, l.frames, g.teach.edNewU(game.UFrames))
	}
	if e.rack && n > loot.InventorySize {
		tabs[2].name = fmt.Sprintf("FRAMES %d/%d", e.page+1, (n+loot.InventorySize-1)/loot.InventorySize)
	}
	if m := len(g.sim.Locker); e.locker && m > loot.InventorySize {
		tabs[1].name = fmt.Sprintf("LOCKER %d/%d", e.lpage+1, (m+loot.InventorySize-1)/loot.InventorySize)
	}
	for _, t := range tabs {
		fill, edge, c := colPanel, fade(colDim, 0.5), colDim
		if t.on {
			fill, edge, c = fade(colText, 0.12), colText, colText
		}
		g.drawRect(dst, t.r, fill, edge)
		g.label(dst, g.small, t.name, t.r.x+t.r.w/2, t.r.y+t.r.h/2, c, true)
	}
}

// edShowLocker switches the strip to the locker. Tapping LOCKER while it
// shows turns the page when it holds more than twelve parts.
func (g *Game) edShowLocker() {
	e := &g.ed
	if e.locker {
		if n := len(g.sim.Locker); n > loot.InventorySize {
			e.lpage = (e.lpage + 1) % ((n + loot.InventorySize - 1) / loot.InventorySize)
		}
	} else {
		e.lpage = 0
	}
	if e.pick == pickCell || e.pick == pickFrame || e.pick == pickLocker {
		e.pick = pickNone
	}
	e.locker, e.rack, e.msg, e.bad = true, false, "", false
	if len(g.sim.Locker) == 0 {
		e.msg = "The locker holds what you buy on the GRID. Nothing here yet."
	}
}

// edLocker is the locker page on show: up to twelve parts and the index
// of the first.
func (g *Game) edLocker() ([]skill.Part, int) {
	all := g.sim.Locker
	lo := min(g.ed.lpage*loot.InventorySize, len(all))
	hi := min(lo+loot.InventorySize, len(all))
	return all[lo:hi], lo
}

// edLockerTap picks a locker part, puts one down, or takes the picked
// slot's part out into the locker.
func (g *Game) edLockerTap(cell int) {
	e, sim := &g.ed, g.sim
	page, lo := g.edLocker()
	switch {
	case e.pick == pickSlot:
		if err := sim.LockerOut(e.hex, e.at); err != nil {
			e.msg, e.bad = upFirst(err.Error())+".", true
			return
		}
		g.edResult(nil)
		e.msg = "Into the locker. " + e.msg
	case cell >= len(page):
		e.pick, e.msg, e.bad = pickNone, "", false
	case e.pick == pickLocker && e.at == lo+cell:
		e.pick, e.msg = pickNone, ""
	default:
		e.pick, e.at, e.msg, e.bad = pickLocker, lo+cell, "", false
	}
}

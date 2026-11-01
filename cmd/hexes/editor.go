package main

// The hex editor: a paused overlay with one hex's six slots in a ring and
// the inventory grid beside it. Everything is a tap: tap a part, then tap
// where it goes. Slot to slot rewires the hex, pack to slot swaps loot in,
// slot to an empty cell takes a part out. Every edit recompiles the hex at
// once; a refused one says why in the info bar. No text entry anywhere.

import (
	"fmt"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
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
)

type editor struct {
	on   bool
	hex  int // the hex on show
	pick int // what is picked up: pickNone, pickSlot or pickCell
	at   int // which slot or cell
	msg  string
	bad  bool // msg is a refusal
}

type rect struct{ x, y, w, h float64 }

func (r rect) hit(x, y float64) bool { return x >= r.x && y >= r.y && x < r.x+r.w && y < r.y+r.h }

// edLayout is where everything sits for the current window.
type edLayout struct {
	tabs         [game.MaxSlots]rect
	ringX, ringY float64
	slotR        float64
	slots        [skill.Slots][2]float64
	cells        [loot.InventorySize]rect
	close, scrap rect
	info         rect
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
	cw := (tw - 3*gap) / game.MaxSlots
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
	l.scrap = rect{tx, by, bw, bh}
	l.close = rect{tx + tw - bw, by, bw, bh}
	l.info = rect{tx + bw + gap, by, tw - 2*bw - 2*gap, bh}
	if !wide {
		l.info = rect{tx, by - 8*k - 64*k, tw, 64 * k}
	}

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
		ring = rect{tx, top, tw, areaH * 0.56}
		grid = rect{tx, top + areaH*0.58, tw, areaH * 0.42}
	}
	l.ringX, l.ringY = ring.x+ring.w/2, ring.y+ring.h/2
	// Six flat-top slots around a center plate tile like a honeycomb:
	// neighbors sit R apart, a tile is just under R/sqrt(3) across.
	R := math.Min(ring.w/2.85, ring.h/2.98)
	l.slotR = R * 0.54
	for i := range l.slots {
		a := -math.Pi/2 + float64(i)*math.Pi/3 // slot 0 on top, clockwise
		l.slots[i] = [2]float64{l.ringX + R*math.Cos(a), l.ringY + R*math.Sin(a)}
	}
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
	g.ed.on = true
	g.ed.pick = pickNone
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
	if inpututil.IsKeyJustPressed(ebiten.KeyTab) || inpututil.IsKeyJustPressed(ebiten.KeyE) ||
		inpututil.IsKeyJustPressed(ebiten.KeyI) || inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		g.ed.on = false
		return true
	}
	for i, key := range []ebiten.Key{ebiten.KeyDigit1, ebiten.KeyDigit2, ebiten.KeyDigit3, ebiten.KeyDigit4} {
		if inpututil.IsKeyJustPressed(key) {
			g.ed.hex, g.ed.pick = i, pickNone
		}
	}
	for _, id := range inpututil.AppendJustPressedTouchIDs(nil) {
		g.touched = true
		x, y := ebiten.TouchPosition(id)
		g.edTap(float64(x), float64(y))
	}
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		x, y := ebiten.CursorPosition()
		g.edTap(float64(x), float64(y))
	}
	return true
}

func (g *Game) edTap(x, y float64) {
	l := g.edLayout()
	e := &g.ed
	sim := g.sim
	inv := &sim.Stash.Inv
	for i, t := range l.tabs {
		if t.hit(x, y) {
			e.hex, e.pick, e.msg = i, pickNone, ""
			return
		}
	}
	if l.close.hit(x, y) {
		e.on = false
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
	if math.Hypot(x-l.ringX, y-l.ringY) < l.slotR*0.9 {
		// The center plate switches a picked charge part's bank letter.
		if e.pick == pickSlot && sim.CycleBank(e.hex, e.at) {
			p := sim.Load.Hexes[e.hex].Slots[e.at].Part
			e.msg, e.bad = "Now on bank "+skill.BankName(p.Bank)+".", false
			return
		}
		// It also points a picked Call at the next hex.
		if e.pick == pickSlot && sim.CycleCall(e.hex, e.at) {
			p := sim.Load.Hexes[e.hex].Slots[e.at].Part
			e.msg, e.bad = "Now calls "+g.callName(e.hex, p, false)+".", false
			return
		}
		e.pick, e.msg = pickNone, ""
		return
	}
	for i, c := range l.slots {
		if math.Hypot(x-c[0], y-c[1]) > l.slotR*0.95 {
			continue
		}
		switch {
		case e.pick == pickCell:
			g.edResult(sim.SwapIn(e.hex, i, e.at))
		case e.pick == pickSlot && e.at != i:
			g.edResult(sim.SwapSlots(e.hex, e.at, i))
		case e.pick == pickSlot:
			e.pick, e.msg = pickNone, ""
		default:
			e.pick, e.at, e.msg = pickSlot, i, ""
		}
		return
	}
	for i, c := range l.cells {
		if !c.hit(x, y) {
			continue
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

func (g *Game) edResult(err error) {
	e := &g.ed
	e.pick = pickNone
	if err != nil {
		e.msg, e.bad = err.Error()+".", true
		return
	}
	pw, ht := g.sim.HexPower(e.hex)
	e.msg, e.bad = fmt.Sprintf("%s rebuilt: %.0f power, %.0f heat per firing.", g.sim.Load.Names[e.hex], pw, ht), false
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
	title, detail := loot.Label(p, g.sim.Reg)
	if p.Kind == skill.KindCall {
		detail = "to " + g.callName(g.ed.hex, p, true)
	}
	tc := colText
	switch p.Kind {
	case skill.KindTrigger:
		tc = color.RGBA{0xff, 0xe0, 0x60, 0xff}
	case skill.KindTarget:
		tc = color.RGBA{0x7d, 0xf0, 0xe0, 0xff}
	case skill.KindGate:
		tc = colGate
	case skill.KindCharge:
		tc = colCharge
	case skill.KindCall:
		tc = colCall
		if to, ok := game.CallsTo(g.ed.hex, p); ok && p.Call != skill.CallSelf {
			tc = slotCol[to]
		}
	case skill.KindEffect:
		if p.Effect != skill.EffectPush {
			tc = typeColor(p.Type)
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
		g.label(screen, g.small, triggerName(sim.Load.Progs[i]), t.x+t.w/2, t.y+t.h*0.72, colDim, true)
	}

	// The ring: edges first, then the center plate, then the six slots.
	h := sim.Load.Hexes[e.hex]
	for i := 0; i < skill.Slots; i++ {
		j := (i + 1) % skill.Slots
		c := color.RGBA{0x5a, 0x66, 0x82, 0xff}
		if !h.Linked[i] {
			c = color.RGBA{0x2a, 0x2e, 0x3a, 0xff}
		}
		a, b := l.slots[i], l.slots[j]
		vector.StrokeLine(screen, float32(a[0]), float32(a[1]), float32(b[0]), float32(b[1]), g.u(3), c, true)
	}
	drawHex(screen, l.ringX, l.ringY, l.slotR, true, colPanel, slotCol[e.hex], g.u(2))
	pw, ht := sim.HexPower(e.hex)
	g.label(screen, g.small, shortName(sim.Load.Names[e.hex]), l.ringX, l.ringY-l.slotR*0.3, slotCol[e.hex], true)
	g.label(screen, g.small, fmt.Sprintf("%.0f pw %.0f ht", pw, ht), l.ringX, l.ringY+l.slotR*0.12, colDim, true)
	if l.slotR > 48*g.k {
		g.label(screen, g.small, "clockwise", l.ringX, l.ringY+l.slotR*0.48, fade(colDim, 0.6), true)
	}
	routes := h.Routes(0)
	for i, c := range l.slots {
		s := h.Slots[i]
		g.partTile(screen, c[0], c[1], l.slotR, true, s.Part, s.Kind == skill.SlotPart, e.pick == pickSlot && e.at == i)
		if s.Kind != skill.SlotPart {
			sock := "any part"
			if h.Sockets[i] == skill.SocketTrigger {
				sock = "trigger"
			}
			g.label(screen, g.small, sock, c[0], c[1], fade(colDim, 0.7), true)
		}
		// A charge part or charge gate shows what its bank holds now, at
		// the top of the tile.
		if s.Kind == skill.SlotPart && s.Part.UsesBank() {
			b := s.Part.Bank
			g.label(screen, g.small, fmt.Sprintf("%s = %d", skill.BankName(b), sim.Banks[e.hex][b]),
				c[0], c[1]-l.slotR*0.62, colCharge, true)
		}
		// A call loops back on its own hex or reaches across to another:
		// say which at the top of the tile.
		if s.Kind == skill.SlotPart && s.Part.Kind == skill.KindCall && s.Part.Call == skill.CallSelf {
			g.label(screen, g.small, "LOOP", c[0], c[1]-l.slotR*0.62, colCall, true)
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

	for i, c := range l.cells {
		g.partTile(screen, c.x+c.w/2, c.y+c.h/2, c.w*0.5, false, inv.Parts[i], inv.Has[i], e.pick == pickCell && e.at == i)
		if inv.Has[i] && sim.Stash.Fresh[i] {
			// Unsecured: a death would drop it in the salvage cache.
			g.label(screen, g.small, "NEW", c.x+c.w-14*g.k, c.y+9*g.k, colSalvage, true)
		}
	}

	// Info bar: the picked part, the last result, or how to use it.
	g.drawRect(screen, l.info, colPanel, color.RGBA{0x3a, 0x42, 0x58, 0xff})
	info, ic := "Tap a part, then tap where it goes. Slot to slot rewires the hex.", colDim
	switch {
	case e.pick == pickCell && inv.Has[e.at]:
		info, ic = loot.Describe(inv.Parts[e.at], sim.Reg), rarityCol[inv.Parts[e.at].Rarity]
		if sim.Stash.Fresh[e.at] {
			info += fmt.Sprintf(" New: slot it or reach depth %d to keep it if you die.", sim.NextCheckpoint())
		}
	case e.pick == pickSlot && h.Slots[e.at].Kind == skill.SlotPart:
		p := h.Slots[e.at].Part
		info, ic = loot.Describe(p, sim.Reg), rarityCol[p.Rarity]
		if (p.UsesBank() || p.Kind == skill.KindCall) && e.msg != "" && !e.bad {
			info = e.msg + " " + info
		}
	case e.pick == pickSlot:
		info = "Empty slot. Tap a part in your pack to put it here."
	case e.msg != "":
		info, ic = e.msg, colText
		if e.bad {
			ic = colHot
		}
	}
	g.wrapLabel(screen, info, l.info, ic)

	g.drawRect(screen, l.scrap, colPanel, colHot)
	g.label(screen, g.face, "SCRAP", l.scrap.x+l.scrap.w/2, l.scrap.y+l.scrap.h/2, colHot, true)
	g.drawRect(screen, l.close, fade(colStairs, 0.25), colStairs)
	g.label(screen, g.face, "RESUME", l.close.x+l.close.w/2, l.close.y+l.close.h/2, colText, true)
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
	to, ok := game.CallsTo(hex, p)
	switch {
	case !ok:
		return skill.CallName(p.Call)
	case p.Call == skill.CallSelf || to == hex:
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
		if sx < -s || sy < -s || sx > float64(g.w)+s || sy > float64(g.h)+s {
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
}

// slotKey is the second line of a HUD card: how the hex fires.
func (g *Game) slotKey(i int) string {
	p := g.sim.Load.Progs[i]
	if p.Trigger == skill.OnCast {
		if g.touched || float64(g.w) < 700*g.k {
			return "CAST"
		}
		return "SPACE"
	}
	return triggerName(p)
}

// castAll presses every on-cast hex: the CAST button fires them all, so a
// trigger swapped into any slot has a button.
func (g *Game) castAll(in *game.Input) {
	for i, p := range g.sim.Load.Progs {
		if p != nil && p.Trigger == skill.OnCast {
			in.Cast[i] = true
		}
	}
}

var (
	colGate   = color.RGBA{0xff, 0xa8, 0x4a, 0xff}
	colCharge = color.RGBA{0xb4, 0x9c, 0xff, 0xff}
	colCall   = color.RGBA{0xff, 0x8a, 0xc8, 0xff}
	colYes    = color.RGBA{0x5c, 0xe0, 0x7a, 0xff}
)

var rarityName = [skill.NumRarities]string{"+COMMON", "+UNCOMMON", "+RARE", "+EXOTIC"}

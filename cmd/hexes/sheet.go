package main

// The item card: a part's or a hex's sheet drawn the way an item reads
// in a loot game. Name in its rarity's color, what it is, one plain line,
// then the numbers in a fixed order, each with its math under it in grey,
// and what the next rarity up would read at the bottom. The editor's ?,
// the grid's ? and a held cast button all open the same card.

import (
	"fmt"
	"image/color"
	"math"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text/v2"

	"hexes/game"
	"hexes/loot"
	"hexes/skill"
)

var (
	colRule = color.RGBA{0xe8, 0xc8, 0x6a, 0xff}
	colMath = color.RGBA{0x7a, 0x83, 0x98, 0xff}
	colNext = color.RGBA{0x8f, 0xd8, 0xa0, 0xff}
)

// openSheet shows a card until the next tap.
func (g *Game) openSheet(s loot.Sheet) {
	g.sheet, g.sheetOn, g.sheetHex = s, true, -1
}

// updateSheet closes an open card on any tap or key. It reports whether
// the card had the input.
func (g *Game) updateSheet() bool {
	if !g.sheetOn {
		return false
	}
	tx, ty, tapped := -1.0, -1.0, false
	for _, id := range inpututil.AppendJustPressedTouchIDs(nil) {
		x, y := ebiten.TouchPosition(id)
		tx, ty, tapped = float64(x), float64(y), true
	}
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		x, y := ebiten.CursorPosition()
		tx, ty, tapped = float64(x), float64(y), true
	}
	if tapped || inpututil.IsKeyJustPressed(ebiten.KeyEscape) || inpututil.IsKeyJustPressed(ebiten.KeySlash) {
		g.sheetOn = false
		g.click(false)
		if tapped && g.sheetHex >= 0 && g.sheetName.hit(tx, ty) {
			g.renameHex(g.sheetHex)
		}
	}
	return true
}

// sheetRow is one drawn row of a card.
type sheetRow struct {
	left, right string
	face        *text.GoTextFace
	c, rc       color.RGBA
	gap         bool // a rule above it
}

// sheetRows lays a card out for width w: long lines wrap, and with
// noMath the grey working is left off to fit a short screen.
func (g *Game) sheetRows(s loot.Sheet, w float64, noMath bool) []sheetRow {
	per := max(10, int(w/(7*g.k)))
	var rows []sheetRow
	add := func(t string, face *text.GoTextFace, c color.RGBA, indent string) {
		for i, ln := range wrapText(t, per-len(indent)) {
			if i > 0 || indent != "" {
				ln = indent + ln
			}
			rows = append(rows, sheetRow{left: ln, face: face, c: c})
		}
	}
	rows = append(rows, sheetRow{left: s.Name, face: g.face, c: rarityCol[s.Rarity]})
	add(strings.ToUpper(s.Rarity.String())+"  "+s.Sub, g.small, colDim, "")
	if s.Desc != "" {
		add(s.Desc, g.small, colText, "")
	}
	first := true
	lastTone := loot.Tone(255)
	for _, l := range s.Lines {
		gap := first || l.Tone != lastTone
		first, lastTone = false, l.Tone
		switch {
		case l.Value != "":
			c := colText
			if l.Tone == loot.ToneCost {
				c = colHeat
				if strings.HasPrefix(l.Value, "-") {
					c = colPower // a cost that vents reads cool
				}
			}
			lab := l.Label
			if len(lab)+len(l.Value)+2 > per {
				lab = lab[:max(3, per-len(l.Value)-3)] + "…"
			}
			rows = append(rows, sheetRow{left: lab, right: l.Value, face: g.small, c: colDim, rc: c, gap: gap})
		default:
			c := colRule
			if l.Tone == loot.ToneStat {
				c = colText
			}
			n := len(rows)
			add(l.Label, g.small, c, "")
			rows[n].gap = gap
		}
		if l.Math != "" && !noMath {
			add(l.Math, g.small, colMath, "  ")
		}
	}
	if s.Next != "" {
		n := len(rows)
		add(s.Next, g.small, colNext, "")
		rows[n].gap = true
	}
	return rows
}

// drawSheet draws a card. top puts it at the top of the screen (a held
// cast button's peek, which mustn't sit under the thumb); otherwise it
// sits in the middle with a tap-to-close line.
func (g *Game) drawSheet(dst *ebiten.Image, s loot.Sheet, top bool) {
	W, H := float64(g.w), float64(g.h)
	k := g.k
	pad := 14 * k
	w := math.Min(W-2*pad, 420*k)
	in := 14 * k
	lh := 17 * k
	rows := g.sheetRows(s, w-2*in, false)
	height := func(rs []sheetRow) float64 {
		h := 2 * in
		for i, r := range rs {
			h += lh
			if i == 0 {
				h += 6 * k
			}
			if r.gap {
				h += 8 * k
			}
		}
		if !top {
			h += lh
		}
		return h
	}
	if height(rows) > H-2*pad {
		rows = g.sheetRows(s, w-2*in, true)
	}
	for height(rows) > H-2*pad && len(rows) > 3 {
		rows = rows[:len(rows)-1]
	}
	h := height(rows)
	x := (W - w) / 2
	y := (H - h) / 2
	if top {
		y = pad + 70*k
		if y+h > H-pad {
			y = math.Max(pad, H-pad-h)
		}
	}
	if !top {
		g.drawRect(dst, rect{0, 0, W, H}, color.RGBA{0x04, 0x05, 0x0a, 0x90}, color.RGBA{})
	}
	edge := rarityCol[s.Rarity]
	g.drawRect(dst, rect{x, y, w, h}, color.RGBA{0x0c, 0x0f, 0x18, 0xf4}, fade(edge, 0.8))
	cy := y + in
	for i, r := range rows {
		if r.gap {
			cy += 4 * k
			g.drawRect(dst, rect{x + in, cy, w - 2*in, 1}, fade(colDim, 0.25), color.RGBA{})
			cy += 4 * k
		}
		cy += lh / 2
		if i == 0 {
			g.label(dst, r.face, r.left, x+w/2, cy+2*k, r.c, true)
			g.sheetName = rect{x, cy - lh, w, 2*lh + 6*k}
			cy += 6 * k
		} else if i == 1 {
			g.label(dst, r.face, r.left, x+w/2, cy, r.c, true)
		} else {
			g.label(dst, r.face, r.left, x+in, cy, r.c, false)
		}
		if r.right != "" {
			tw, _ := text.Measure(r.right, g.face, 0)
			g.label(dst, g.face, r.right, x+w-in-tw, cy, r.rc, false)
		}
		cy += lh / 2
	}
	if !top {
		bye := "tap anywhere to close"
		if g.sheetHex >= 0 {
			bye = "tap the name to rename it, anywhere else to close"
		}
		g.label(dst, g.small, bye, x+w/2, y+h-in-lh/2+4*k, fade(colDim, 0.8), true)
	}
}

// wrapText breaks s at spaces into lines of at most n characters.
func wrapText(s string, n int) []string {
	n = max(n, 8)
	var out []string
	for len(s) > n {
		cut := strings.LastIndexByte(s[:n+1], ' ')
		if cut <= 0 {
			cut = n
		}
		out = append(out, s[:cut])
		s = strings.TrimLeft(s[cut:], " ")
	}
	if s != "" || len(out) == 0 {
		out = append(out, s)
	}
	return out
}

// edSheet opens the card for what the editor has picked: a part from the
// pack, the locker or a slot, or the whole hex when nothing is picked.
func (g *Game) edSheet() {
	e, sim := &g.ed, g.sim
	inv := &sim.Stash.Inv
	switch {
	case e.pick == pickCell && inv.Has[e.at]:
		g.openSheet(game.PartSheet(inv.Parts[e.at], sim.Reg))
		return
	case e.pick == pickLocker && e.at < len(sim.Locker):
		g.openSheet(game.PartSheet(sim.Locker[e.at], sim.Reg))
		return
	case e.pick == pickSlot:
		if sl := sim.Load.Hex(e.hex).Slots[e.at]; sl.Kind == skill.SlotPart {
			g.openSheet(game.PartSheet(sl.Part, sim.Reg))
			return
		}
	}
	s, err := sim.HexCard(e.hex)
	if err != nil {
		e.msg, e.bad = upFirst(err.Error())+".", true
		return
	}
	g.openSheet(s)
	g.sheetHex = e.hex
}

// costTag is a cast's cost in a few characters, for the HUD card.
func costTag(power, heat float32) string {
	return fmt.Sprintf("%.0fP %+.0fH", power, heat)
}

// drawCardPanel draws a card docked in r, the standard way a part or a
// hex shows in the editor, the grid and the floor-end pick: the name in
// its rarity's color, what it is, an optional context line (where it
// goes, why it can't), the plain line, then the numbers without their
// math. Rows that don't fit are cut, and a hint says a tap shows the
// whole card. hint names what a tap on the name does, or "".
func (g *Game) drawCardPanel(dst *ebiten.Image, s loot.Sheet, r rect, top string, topC color.RGBA, hint, more string) {
	k := g.k
	in := 9 * k
	lh := 15 * k
	w := r.w - 2*in
	per := max(10, int(w/(7*k)))
	x := r.x + in
	y := r.y + in
	bot := r.y + r.h - in - lh // the last row is kept for the hint
	cut := false
	line := func(t string, face *text.GoTextFace, c color.RGBA, maxLines int) {
		ls := wrapText(t, per)
		for i, ln := range ls {
			if i == maxLines || y+lh > bot {
				cut = true
				return
			}
			if i == maxLines-1 && i < len(ls)-1 {
				ln = strings.TrimRight(ln, " ") + "…"
				cut = true
			}
			g.label(dst, face, ln, x, y+lh/2, c, false)
			y += lh
		}
	}
	// The name, with what a tap on it does at the right.
	name := s.Name
	below := false // a narrow card puts the hint under the name
	if hint != "" {
		hw, _ := text.Measure(hint, g.small, 0)
		nw, _ := text.Measure(name, g.face, 0)
		if nw+hw+8*k > w {
			below = true
		} else {
			g.label(dst, g.small, hint, r.x+r.w-in-hw, y+9*k, fade(colDim, 0.9), false)
		}
	}
	if maxName := max(4, int(w/(9*k))); len(name) > maxName {
		name = name[:maxName-1] + "…"
	}
	g.label(dst, g.face, name, x, y+9*k, rarityCol[s.Rarity], false)
	y += 20 * k
	if below {
		line(hint, g.small, fade(colDim, 0.9), 1)
	}
	line(strings.ToUpper(s.Rarity.String())+"  ·  "+s.Sub, g.small, colDim, 1)
	if top != "" {
		line(top, g.small, topC, 2)
	}
	if s.Desc != "" {
		line(s.Desc, g.small, colText, 2)
	}
	for _, l := range s.Lines {
		if y+lh > bot {
			cut = true
			break
		}
		if l.Math != "" {
			cut = true // the math is on the whole card
		}
		if l.Value == "" {
			c := colRule
			if l.Tone == loot.ToneStat {
				c = colText
			}
			line(l.Label, g.small, c, 1)
			continue
		}
		vc := colText
		if l.Tone == loot.ToneCost {
			vc = colHeat
			if strings.HasPrefix(l.Value, "-") {
				vc = colPower
			}
		}
		vw, _ := text.Measure(l.Value, g.small, 0)
		lab := l.Label
		if room := int((w - vw - 8*k) / (7 * k)); len(lab) > room {
			lab = lab[:max(3, room-1)] + "…"
		}
		g.label(dst, g.small, lab, x, y+lh/2, colDim, false)
		g.label(dst, g.small, l.Value, r.x+r.w-in-vw, y+lh/2, vc, false)
		y += lh
	}
	if s.Next != "" {
		cut = true
	}
	if cut && more != "" {
		mw, _ := text.Measure(more, g.small, 0)
		g.label(dst, g.small, more, r.x+r.w-in-mw, r.y+r.h-in-lh/2, fade(colNext, 0.85), false)
	}
}

// edCardView is what the editor's card panel shows: a card (ok) with a
// context line on top, or only a line, for a frame or a shape.
type edCardView struct {
	s    loot.Sheet
	ok   bool
	top  string
	topC color.RGBA
	hint string // what a tap on the name does
	hex  bool   // the card is the hex's own, so its name renames it
}

// edCard works out the editor's card panel: the part picked in the pack,
// the locker or a slot, or else the hex being edited.
func (g *Game) edCard() edCardView {
	e, sim := &g.ed, g.sim
	inv := &sim.Stash.Inv
	h := sim.Load.Hex(e.hex)
	v := edCardView{topC: colDim}
	msg := func() (string, color.RGBA) {
		if e.bad {
			return e.msg, colHot
		}
		return e.msg, colText
	}
	part := func(p skill.Part, lead string, leadC color.RGBA) edCardView {
		v.s, v.ok = game.PartSheet(p, sim.Reg), true
		v.top, v.topC = lead, leadC
		if e.msg != "" && e.bad {
			v.top, v.topC = msg()
		}
		return v
	}
	var fr skill.Frame
	if e.pick == pickSlot {
		fr = h.Frame(e.at)
	}
	laid := h.Laid()
	wired := laid && h.Lay.Wired
	green := "Green slots are where it does something."
	switch {
	case wired:
		green = fmt.Sprintf("Tap any free tile to lay it (%d of %d on the board), or a piece to swap it in.", h.Held(), h.Budget())
		if fit, held := g.edFits(h); held && len(fit) == 0 {
			green = fmt.Sprintf("The board holds %d: drop it on a piece to swap them, or set one aside first.", h.Budget())
		}
	case laid:
		green = "Green cells are where it fits: tap one to lay it, or a piece to swap it in."
		if fit, held := g.edFits(h); held && len(fit) == 0 {
			green = "No room left for it: drop it on a piece to swap them, or set one aside first."
		}
	}
	pf, pickedFrame := g.edPickedFrame()
	switch {
	case pickedFrame && e.msg != "":
		v.top, v.topC = msg()
		return v
	case pickedFrame:
		v.top, v.topC = "Green slots take it; fitting scraps the frame there. "+pf.Text(sim.Reg), frameCol(pf)
		if game.IsTestFrame(pf) {
			v.top += " Test frame: free to try."
		}
		return v
	case e.pick == pickLocker && e.at < len(sim.Locker):
		return part(sim.Locker[e.at], green, colText)
	case e.pick == pickCell && inv.Has[e.at]:
		lead := green
		if sim.Stash.Fresh[e.at] {
			lead += fmt.Sprintf(" New: slot it or reach depth %d to keep it if you die.", sim.NextCheckpoint())
		}
		return part(inv.Parts[e.at], lead, colText)
	case e.shape && e.msg != "":
		v.top, v.topC = msg()
		if !e.bad {
			v.topC = colShape
		}
		return v
	case e.pick == pickSlot && h.Slots[e.at].Kind == skill.SlotPart:
		p := h.Slots[e.at].Part
		lead, c := "", colText
		if what, ok := h.Use(e.at); laid && !h.Runs(e.at) && what != "" {
			lead, c = upFirst(what)+".", colDim
		} else if what != "" {
			lead = "Here it " + what + "."
			if !ok {
				lead, c = "Does nothing here: "+what+".", colHot
			}
		}
		if (p.UsesBank() || p.Kind == skill.KindCall) && e.msg != "" && !e.bad {
			lead = e.msg + " " + lead
		}
		if !fr.Plain() {
			lead += " " + fr.Text(sim.Reg)
		}
		if wired && e.msg == "" {
			if h.Lay.Pos[e.at].On {
				lead += " Pieces join where they touch: tap a free tile to move it there, a piece it touches to step their join, the tray to set it aside."
			} else {
				lead = "In the tray. Tap a free tile to lay it."
			}
		} else if laid && e.msg == "" {
			if h.Lay.Pos[e.at].On {
				lead += " Tap it again to turn it, a cell to move it, the tray to set it aside."
			} else {
				lead = "In the tray. Tap a cell to lay it."
			}
		}
		return part(p, strings.TrimSpace(lead), c)
	case e.pick == pickSlot && e.msg != "" && e.bad:
		v.top, v.topC = msg()
		return v
	case e.pick == pickSlot:
		v.top = "Empty slot. Tap a part in your pack to put it here."
		if !fr.Plain() {
			v.top, v.topC = "Empty slot. "+fr.Text(sim.Reg), frameCol(fr)
		}
		return v
	}
	// Nothing picked: the hex's own card.
	v.top = "Tap a part, then tap where it goes. Slot to slot rewires the hex."
	switch {
	case wired:
		v.top = "Pieces join where they touch. Only what touches its way back to the trigger runs."
	case laid:
		v.top = "Tap a piece, then a cell. Plugs that meet join; only pieces the trigger reaches run."
	}
	if e.msg != "" {
		v.top, v.topC = msg()
	}
	if s, err := sim.HexCard(e.hex); err == nil {
		v.s, v.ok, v.hex, v.hint = s, true, true, "tap name to rename"
	} else if h != nil {
		v.s, v.ok, v.hex, v.hint = loot.Sheet{Name: sim.Load.Name(e.hex), Sub: "won't build yet"}, true, true, "tap name to rename"
		if e.msg == "" {
			v.top, v.topC = upFirst(err.Error())+".", colHot
		}
	}
	return v
}

// lootCardFrames is how long a picked-up part's card stays up.
const lootCardFrames = 150

// drawLootCard shows the card of a part just picked up off the floor,
// near the top, for a few seconds, while you play.
func (g *Game) drawLootCard(dst *ebiten.Image) {
	if n := g.sim.LootN; n != g.lootSeen {
		g.lootSeen = n
		g.lootSheet = game.PartSheet(g.sim.LastLoot, g.sim.Reg)
		g.lootUntil = g.frame + lootCardFrames
	}
	if g.frame >= g.lootUntil || g.ed.on || g.menu != menuClosed || g.sheetOn || g.peek >= 0 || g.sim.State == game.Drafting {
		return
	}
	k := g.k
	W := float64(g.w)
	w := math.Min(W-28*k, 340*k)
	r := rect{(W - w) / 2, 168 * k, w, 140 * k} // under the bars and cast cards
	edge := rarityCol[g.lootSheet.Rarity]
	g.drawRect(dst, r, color.RGBA{0x0c, 0x0f, 0x18, 0xe8}, fade(edge, 0.8))
	g.drawCardPanel(dst, g.lootSheet, r, "Picked up: in your pack.", colYes, "", "")
}

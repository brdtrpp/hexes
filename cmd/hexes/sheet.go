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

// holdPeek is how long a cast button is held before its card shows.
const holdPeek = 0.45

// openSheet shows a card until the next tap.
func (g *Game) openSheet(s loot.Sheet) {
	g.sheet, g.sheetOn = s, true
}

// updateSheet closes an open card on any tap or key. It reports whether
// the card had the input.
func (g *Game) updateSheet() bool {
	if !g.sheetOn {
		return false
	}
	if len(inpututil.AppendJustPressedTouchIDs(nil)) > 0 || inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) ||
		inpututil.IsKeyJustPressed(ebiten.KeyEscape) || inpututil.IsKeyJustPressed(ebiten.KeySlash) {
		g.sheetOn = false
		g.click(false)
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
		g.label(dst, g.small, "tap anywhere to close", x+w/2, y+h-in-lh/2+4*k, fade(colDim, 0.8), true)
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
	if e.hex < game.MaxSlots {
		s, err := sim.SlotSheet(e.hex)
		if err != nil {
			e.msg, e.bad = upFirst(err.Error())+".", true
			return
		}
		g.openSheet(s)
		return
	}
	e.msg, e.bad = "Pick a part to see its card.", false
}

// peekCast tracks a held cast button and returns the slot whose card to
// show, or -1.
func (g *Game) peekCast() int {
	b := 0
	for _, id := range ebiten.AppendTouchIDs(nil) {
		x, y := ebiten.TouchPosition(id)
		if c := g.castButton(float64(x), float64(y)); c > 0 {
			b = c
		}
	}
	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
		x, y := ebiten.CursorPosition()
		if c := g.castButton(float64(x), float64(y)); c > 0 {
			b = c
		}
	}
	if b == 0 || b != g.peekBtn {
		g.peekBtn, g.peekT = b, 0
		return -1
	}
	g.peekT += 1.0 / float64(ebiten.TPS())
	if g.peekT < holdPeek {
		return -1
	}
	return g.sim.Load.FirstOn(b)
}

// costTag is a cast's cost in a few characters, for the HUD card.
func costTag(power, heat float32) string {
	return fmt.Sprintf("%.0fP %+.0fH", power, heat)
}

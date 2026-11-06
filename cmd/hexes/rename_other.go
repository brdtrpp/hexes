//go:build !js

package main

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"hexes/game"
)

// renameHex starts typing a new name for hex i: Enter keeps it, Escape
// cancels, Backspace rubs out.
func (g *Game) renameHex(i int) {
	if g.renaming >= 0 {
		return
	}
	g.renaming = i
	g.renameBuf = []rune(g.sim.Load.Name(i))
}

func (g *Game) updateRename() bool {
	g.pollRename()
	if g.renaming < 0 {
		return false
	}
	for _, r := range ebiten.AppendInputChars(nil) {
		if len(g.renameBuf) < game.MaxNameLen {
			g.renameBuf = append(g.renameBuf, r)
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) && len(g.renameBuf) > 0 {
		g.renameBuf = g.renameBuf[:len(g.renameBuf)-1]
	}
	switch {
	case inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeyNumpadEnter):
		i := g.renaming
		g.renaming = -1
		g.applyRename(i, string(g.renameBuf))
	case inpututil.IsKeyJustPressed(ebiten.KeyEscape):
		g.renaming = -1
	}
	return true
}

func (g *Game) drawRename(dst *ebiten.Image) {
	if g.renaming < 0 {
		return
	}
	k := g.k
	W, H := float64(g.w), float64(g.h)
	w := math.Min(W-28*k, 360*k)
	h := 96 * k
	x, y := (W-w)/2, H*0.18
	g.drawRect(dst, rect{0, 0, W, H}, color.RGBA{0x04, 0x05, 0x0a, 0xa0}, color.RGBA{})
	g.drawRect(dst, rect{x, y, w, h}, color.RGBA{0x0c, 0x0f, 0x18, 0xf8}, fade(colDim, 0.8))
	g.label(dst, g.small, "NAME THIS HEX", x+w/2, y+18*k, colDim, true)
	caret := ""
	if g.frame/20%2 == 0 {
		caret = "_"
	}
	g.label(dst, g.face, string(g.renameBuf)+caret, x+w/2, y+48*k, colText, true)
	g.label(dst, g.small, "Enter keeps it, Esc cancels", x+w/2, y+78*k, fade(colDim, 0.8), true)
}

//go:build js

package main

import (
	"syscall/js"

	"github.com/hajimehoshi/ebiten/v2"

	"hexes/game"
)

// renameHex opens the page's name box (web/index.html) for hex i.
func (g *Game) renameHex(i int) {
	open := js.Global().Get("hexesRename")
	if open.Type() != js.TypeFunction || g.renaming >= 0 {
		return
	}
	g.renaming = i
	var done js.Func
	done = js.FuncOf(func(this js.Value, args []js.Value) any {
		r := renameResult{i: i}
		if len(args) > 0 && args[0].Type() == js.TypeString {
			r.name, r.ok = args[0].String(), true
		}
		select {
		case g.renameCh <- r:
		default:
		}
		done.Release()
		return nil
	})
	open.Invoke(g.sim.Load.Name(i), game.MaxNameLen, done)
}

// updateRename holds the game still while the name box is up.
func (g *Game) updateRename() bool {
	g.pollRename()
	return g.renaming >= 0
}

func (g *Game) drawRename(*ebiten.Image) {}

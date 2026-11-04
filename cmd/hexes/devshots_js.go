//go:build js

package main

import (
	"github.com/hajimehoshi/ebiten/v2"

	"hexes/game"
)

type devState struct{}

func (g *Game) devInput(in *game.Input)      {}
func (g *Game) devShot(screen *ebiten.Image) {}

func devWindow() (int, int) { return 1100, 720 }

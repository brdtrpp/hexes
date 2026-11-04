//go:build !js

package main

// A headless screenshot autopilot for checking the look without a phone.
// Run the desktop build with HEXES_SHOTS=<dir> (under xvfb-run on a box
// with no display): it wakes every foe, walks toward the nearest, casts
// on a beat, keeps the player alive, saves a PNG every HEXES_EVERY frames
// (default 12) from frame 90, HEXES_COUNT of them (default 24), and quits.
// HEXES_EXAMPLE=<n> loads example build n first. Without HEXES_SHOTS it
// does nothing.

import (
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"strconv"

	"github.com/hajimehoshi/ebiten/v2"

	"hexes/game"
)

type devState struct {
	dir          string
	every, count int
	taken        int
	ready        bool
}

func envInt(k string, d int) int {
	if v, err := strconv.Atoi(os.Getenv(k)); err == nil {
		return v
	}
	return d
}

func (g *Game) devInput(in *game.Input) {
	d := &g.dev
	if !d.ready {
		d.ready = true
		d.dir = os.Getenv("HEXES_SHOTS")
		d.every, d.count = envInt("HEXES_EVERY", 12), envInt("HEXES_COUNT", 24)
		if d.dir != "" {
			if n := envInt("HEXES_EXAMPLE", -1); n >= 0 {
				_ = g.sim.SetTestShapes(true)
				_ = g.sim.LoadExample(n)
			}
		}
	}
	if d.dir == "" {
		return
	}
	s := g.sim
	p := s.P()
	p.U.HP = p.U.MaxHP
	if s.Rig.Heat > s.Rig.MaxHeat*0.8 {
		s.Rig.Heat = s.Rig.MaxHeat * 0.3
	}
	s.Rig.Power = s.Rig.MaxPower
	px, py := p.Pos()
	best, bd := -1, math.Inf(1)
	for i := range s.Actors {
		a := &s.Actors[i]
		if a.Kind == game.KindPlayer || !a.U.Alive {
			continue
		}
		a.Awake = true
		x, y := a.Pos()
		if d := math.Hypot(x-px, y-py); d < bd {
			best, bd = i, d
		}
	}
	if best >= 0 && bd > 4 {
		x, y := s.Actors[best].Pos()
		in.MoveX, in.MoveY = x-px, y-py
	}
	if g.frame%30 == 0 && bd < 12 {
		g.castAll(in)
	}
	in.Restart = true
}

func (g *Game) devShot(screen *ebiten.Image) {
	d := &g.dev
	if d.dir == "" || g.frame < 90 || (g.frame-90)%d.every != 0 {
		return
	}
	b := screen.Bounds()
	img := image.NewRGBA(b)
	screen.ReadPixels(img.Pix)
	f, err := os.Create(fmt.Sprintf("%s/shot%03d.png", d.dir, d.taken))
	if err == nil {
		_ = png.Encode(f, img)
		f.Close()
	}
	d.taken++
	if d.taken >= d.count {
		os.Exit(0)
	}
}

// devWindow is the window size, overridable with HEXES_WIN=WxH so the
// screenshot autopilot can check phone layouts.
func devWindow() (int, int) {
	var w, h int
	if _, err := fmt.Sscanf(os.Getenv("HEXES_WIN"), "%dx%d", &w, &h); err == nil && w > 0 && h > 0 {
		return w, h
	}
	return 1100, 720
}

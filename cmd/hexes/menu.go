package main

// Saving, the menu and the salvage cache. Progress saves on its own: the
// sim flags anything worth keeping and autosave writes it, at most once a
// second while playing and at once on floor changes, deaths and edits
// made with the game paused. The menu is the only way to wipe a save, and
// it asks twice. Every control is a tap; nothing is typed.

import (
	"fmt"
	"image/color"
	"log"
	"math"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"hexes/game"
)

// saveKey is where progress lives: localStorage in the browser, a file
// in the config folder on desktop.
const saveKey = "hexes.save"

type menuState uint8

const (
	menuClosed menuState = iota
	menuOpen
	menuConfirm // asking whether to wipe everything
)

// loadSave picks up where the player left off, if there's a save.
func (g *Game) loadSave() {
	s := loadPref(saveKey)
	if s == "" {
		return
	}
	if err := g.sim.LoadSave([]byte(s)); err != nil {
		log.Printf("hexes: save not loaded: %v", err)
		g.showHint("This version teaches the game a floor at a time,\nso your run starts fresh")
		return
	}
	g.camX, g.camY = g.sim.P().Pos()
}

// autosave writes progress when the sim says something changed. While
// the game runs it waits a second between writes so a burst of pickups is
// one write; otherwise it writes now.
func (g *Game) autosave() {
	s := g.sim
	if !s.Dirty {
		return
	}
	paused := g.ed.on || g.menu != menuClosed
	if s.State == game.Playing && !paused && g.frame-g.savedAt < 60 {
		return
	}
	b, err := s.Save()
	if err != nil {
		log.Printf("hexes: save failed: %v", err)
		return
	}
	savePref(saveKey, string(b))
	g.savedAt = g.frame
}

func (g *Game) openMenu() {
	if g.ed.on {
		return
	}
	g.menu = menuOpen
	g.stick.End()
}

type menuLayout struct {
	panel, left, right rect
	// The sound row: volume down, sound on/off (shows the level), up.
	vdown, vmid, vup rect
}

func (g *Game) menuLayout() menuLayout {
	W, H, k := float64(g.w), float64(g.h), g.k
	pw := math.Min(W-28*k, 440*k)
	ph := math.Min(H-28*k, 350*k)
	px, py := (W-pw)/2, (H-ph)/2
	gap, bh := 12*k, 50*k
	bw := (pw - 3*gap) / 2
	by := py + ph - gap - bh
	rh := 44 * k
	ry := by - gap - rh
	sq := rh * 1.3
	return menuLayout{
		panel: rect{px, py, pw, ph},
		left:  rect{px + gap, by, bw, bh},
		right: rect{px + 2*gap + bw, by, bw, bh},
		vdown: rect{px + gap, ry, sq, rh},
		vmid:  rect{px + 2*gap + sq, ry, pw - 4*gap - 2*sq, rh},
		vup:   rect{px + pw - gap - sq, ry, sq, rh},
	}
}

// updateMenu runs the menu while it's open and reports whether it took
// the frame. Esc or M opens and closes it.
func (g *Game) updateMenu() bool {
	toggle := inpututil.IsKeyJustPressed(ebiten.KeyEscape) || inpututil.IsKeyJustPressed(ebiten.KeyM)
	if g.menu == menuClosed {
		if toggle && !g.ed.on {
			g.openMenu()
			return true
		}
		return false
	}
	if toggle {
		g.menu = menuClosed
		return true
	}
	var taps [][2]float64
	for _, id := range inpututil.AppendJustPressedTouchIDs(nil) {
		x, y := ebiten.TouchPosition(id)
		taps = append(taps, [2]float64{float64(x), float64(y)})
	}
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		x, y := ebiten.CursorPosition()
		taps = append(taps, [2]float64{float64(x), float64(y)})
	}
	l := g.menuLayout()
	for _, t := range taps {
		open := g.menu == menuOpen
		switch {
		case open && l.vdown.hit(t[0], t[1]):
			g.nudgeVolume(-10)
		case open && l.vup.hit(t[0], t[1]):
			g.nudgeVolume(10)
		case open && l.vmid.hit(t[0], t[1]):
			g.toggleMute()
		case l.left.hit(t[0], t[1]) && g.menu == menuOpen:
			g.menu = menuConfirm
		case l.left.hit(t[0], t[1]) && g.menu == menuConfirm:
			g.sim.NewRun(uint64(time.Now().UnixNano()))
			g.resetTeach()
			g.sim.OfferKits()
			g.camX, g.camY = g.sim.P().Pos()
			g.menu = menuClosed
		case l.right.hit(t[0], t[1]) && g.menu == menuConfirm:
			g.menu = menuOpen
		case l.right.hit(t[0], t[1]), !l.panel.hit(t[0], t[1]):
			g.menu = menuClosed
		}
		break // one tap per frame
	}
	return true
}

func (g *Game) drawMenu(screen *ebiten.Image) {
	sim := g.sim
	l := g.menuLayout()
	k := g.k
	vector.DrawFilledRect(screen, 0, 0, float32(g.w), float32(g.h), color.RGBA{0x05, 0x06, 0x0b, 0xd8}, false)
	g.drawRect(screen, l.panel, color.RGBA{0x14, 0x18, 0x24, 0xff}, color.RGBA{0x3a, 0x42, 0x58, 0xff})
	cx := l.panel.x + l.panel.w/2
	ty := l.panel.y + 30*k
	body := rect{l.panel.x + 8*k, ty + 22*k, l.panel.w - 16*k, l.vmid.y - ty - 30*k}
	if g.menu == menuConfirm {
		body.h = l.left.y - ty - 30*k
	}
	if g.menu == menuConfirm {
		g.label(screen, g.big, "START FRESH?", cx, ty, colHot, true)
		g.wrapLabel(screen, "This wipes your rig, level, pack and hexes and starts you over at depth 1. It can't be undone.", body, colText)
		g.drawRect(screen, l.left, fade(colHot, 0.25), colHot)
		g.label(screen, g.face, "WIPE IT", l.left.x+l.left.w/2, l.left.y+l.left.h/2, colText, true)
		g.drawRect(screen, l.right, fade(colStairs, 0.25), colStairs)
		g.label(screen, g.face, "KEEP IT", l.right.x+l.right.w/2, l.right.y+l.right.h/2, colText, true)
		return
	}
	g.label(screen, g.big, "MENU", cx, ty, colText, true)
	info := fmt.Sprintf("Deepest %d · deaths %d · kills %d. Progress saves on this device by itself. Every %dth depth is a checkpoint that secures your pack.",
		sim.Best, sim.Deaths, sim.Kills, game.CheckpointEvery)
	if c := &sim.Cache; c.Active {
		info += fmt.Sprintf(" Your salvage cache (%d parts) waits on depth %d.", c.N, c.Depth)
	}
	g.wrapLabel(screen, info, body, colDim)
	g.drawSoundRow(screen, l)
	g.drawRect(screen, l.left, colPanel, colHot)
	g.label(screen, g.face, "START FRESH", l.left.x+l.left.w/2, l.left.y+l.left.h/2, colHot, true)
	g.drawRect(screen, l.right, fade(colStairs, 0.25), colStairs)
	g.label(screen, g.face, "RESUME", l.right.x+l.right.w/2, l.right.y+l.right.h/2, colText, true)
}

// drawSalvage draws the salvage cache when it's on this floor: a gold
// crate with a ring that breathes, or an arrow at the screen edge pointing
// to it when it's out of view.
func (g *Game) drawSalvage(screen *ebiten.Image) {
	c := &g.sim.Cache
	if !c.Active || c.Depth != g.sim.Depth {
		return
	}
	s := g.size
	x, y := c.At.Pixel()
	sx, sy := g.toScreen(x, y)
	pulse := 0.5 + 0.5*math.Sin(float64(g.frame)*0.1)
	margin := s * 1.5
	if sx >= -s && sy >= -s && sx <= float64(g.w)+s && sy <= float64(g.h)+s {
		vector.StrokeCircle(screen, float32(sx), float32(sy), float32(s*(0.75+0.25*pulse)), g.u(2.5), fade(colSalvage, float32(0.5+0.5*pulse)), true)
		drawHex(screen, sx, sy, s*0.45, false, fade(colSalvage, 0.85), color.RGBA{0xff, 0xf0, 0xc0, 0xff}, g.u(2))
		g.label(screen, g.small, fmt.Sprintf("SALVAGE %d", c.N), sx, sy-s*1.2, colSalvage, true)
		return
	}
	// Off screen: an arrow on the edge, toward the cache.
	cx, cy := float64(g.w)/2, float64(g.h)/2
	dx, dy := sx-cx, sy-cy
	l := math.Hypot(dx, dy)
	if l < 1 {
		return
	}
	dx, dy = dx/l, dy/l
	t := math.Min((cx-margin)/math.Max(1e-6, math.Abs(dx)), (cy-margin)/math.Max(1e-6, math.Abs(dy)))
	ax, ay := cx+dx*t, cy+dy*t
	r := s * 0.55
	var p vector.Path
	p.MoveTo(float32(ax+dx*r), float32(ay+dy*r))
	p.LineTo(float32(ax-dx*r*0.6-dy*r*0.7), float32(ay-dy*r*0.6+dx*r*0.7))
	p.LineTo(float32(ax-dx*r*0.6+dy*r*0.7), float32(ay-dy*r*0.6-dx*r*0.7))
	p.Close()
	vs, is := p.AppendVerticesAndIndicesForFilling(nil, nil)
	paint(vs, fade(colSalvage, float32(0.6+0.4*pulse)))
	screen.DrawTriangles(vs, is, whiteSub(), &ebiten.DrawTrianglesOptions{AntiAlias: true})
	g.label(screen, g.small, "SALVAGE", ax-dx*r*2, ay-dy*r*2, colSalvage, true)
}

// drawSoundRow draws the menu's volume controls.
func (g *Game) drawSoundRow(screen *ebiten.Image, l menuLayout) {
	s := &g.snd
	c := colStairs
	label := fmt.Sprintf("SOUND %d%%", s.vol)
	switch {
	case s.player == nil:
		c, label = colDim, "NO SOUND HERE"
	case !s.on():
		c, label = colDim, "SOUND OFF"
	}
	for _, r := range []rect{l.vdown, l.vup} {
		g.drawRect(screen, r, colPanel, colDim)
	}
	g.label(screen, g.big, "-", l.vdown.x+l.vdown.w/2, l.vdown.y+l.vdown.h/2, colText, true)
	g.label(screen, g.big, "+", l.vup.x+l.vup.w/2, l.vup.y+l.vup.h/2, colText, true)
	g.drawRect(screen, l.vmid, fade(c, 0.18), c)
	if s.on() { // the level as a bar along the bottom
		m := l.vmid
		vector.DrawFilledRect(screen, float32(m.x+4*g.k), float32(m.y+m.h-6*g.k), float32((m.w-8*g.k)*float64(s.vol)/100), float32(3*g.k), c, false)
	}
	g.label(screen, g.face, label, l.vmid.x+l.vmid.w/2, l.vmid.y+l.vmid.h/2-2*g.k, colText, true)
}

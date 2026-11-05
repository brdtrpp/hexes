package main

// The teaching ladder on screen (game/teach.go): the lesson pickup with
// its beacon, the card that says what a lesson gives you, and the one
// pulse that marks something newly unlocked, on the HUD and in the
// editor.

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"hexes/game"
)

var colLesson = color.RGBA{0x8c, 0xff, 0xe8, 0xff}

// pulseFrames is how long the one pulse on something new lasts.
const pulseFrames = 90

// teachState remembers what was unlocked last frame, so a new unlock can
// pulse once on the HUD and once more the first time the editor opens.
type teachState struct {
	ready  bool
	u      game.Unlock
	hexes  int
	hudAt  int // frame the last unlock landed
	hudHex [game.MaxSlots]bool
	edU    game.Unlock
	edHex  [game.MaxSlots]bool
	edAt   int // frame the editor opened with something new in it
	edWas  bool
}

// watchTeach notes new unlocks. A save loading or the test switch flips
// everything at once and doesn't pulse.
func (g *Game) watchTeach() {
	t := &g.teach
	u, n := g.sim.Unlocks(), g.sim.HexesOpen()
	if t.ready && !g.sim.TestShapes && (u != t.u || n != t.hexes) {
		gained := u &^ t.u
		t.hudAt = g.frame
		t.hudHex = [game.MaxSlots]bool{}
		for i := t.hexes; i < n; i++ {
			t.hudHex[i], t.edHex[i] = true, true
		}
		t.edU |= gained
	}
	t.ready, t.u, t.hexes = true, u, n
	if g.ed.on && !t.edWas {
		t.edAt = g.frame
	}
	if !g.ed.on && t.edWas {
		// Seen in the editor once: nothing pulses there again.
		t.edU, t.edHex = 0, [game.MaxSlots]bool{}
	}
	t.edWas = g.ed.on
}

// resetTeach forgets the last unlocks (a fresh run or a loaded save).
func (g *Game) resetTeach() { g.teach = teachState{} }

// edNew reports whether editor tab i (a hex, or BOARD) is new.
func (t *teachState) edNew(i int) int {
	if i < game.MaxSlots && t.edHex[i] || i == game.MaxSlots && t.edU&game.UBoard != 0 {
		return t.edAt
	}
	return -1
}

// edNewU is when what u unlocks started pulsing in the editor, or -1.
func (t *teachState) edNewU(u game.Unlock) int {
	if t.edU&u != 0 {
		return t.edAt
	}
	return -1
}

// edTabOn reports whether editor tab i is taught: a hex that's online,
// or the BOARD.
func (g *Game) edTabOn(i int) bool {
	if i == game.MaxSlots {
		return g.sim.Has(game.UBoard)
	}
	return g.sim.HexOpen(i)
}

// newPulse draws the one pulse on r for something new: a bright edge
// that swells out and fades, starting at frame from (-1 for none).
func (g *Game) newPulse(dst *ebiten.Image, r rect, from int) {
	if from < 0 {
		return
	}
	k := float64(g.frame-from) / pulseFrames
	if k < 0 || k > 1 {
		return
	}
	grow := k * 14 * g.k
	a := float32(1 - k)
	vector.DrawFilledRect(dst, float32(r.x), float32(r.y), float32(r.w), float32(r.h), fade(colLesson, 0.25*a), false)
	vector.StrokeRect(dst, float32(r.x-grow), float32(r.y-grow), float32(r.w+2*grow), float32(r.h+2*grow), g.u(3), fade(colLesson, a), false)
}

// hudPulse is the HUD's pulse on hex card i.
func (g *Game) hudPulse(i int) int {
	if g.teach.hudHex[i] {
		return g.teach.hudAt
	}
	return -1
}

// drawLesson draws the lesson waiting on this floor: a bright diamond
// in a ring with a beam of light over it, seen through the fog, and an
// arrow at the screen's edge while it's out of view.
func (g *Game) drawLesson(screen *ebiten.Image) {
	d := &g.sim.Stash.Lesson
	if !d.On {
		return
	}
	s := g.size
	x, y := d.At.Pixel()
	sx, sy := g.toScreen(x, y)
	t := float64(g.frame) / 60
	cr, cg, cb := colorF(colLesson)
	W, H := float64(g.w), float64(g.h)
	if sx < -s || sy < -s || sx > W+s || sy > H+s {
		g.lessonArrow(screen, sx, sy)
		return
	}
	// The beacon: a tall soft column, so it reads from across the room.
	for k := 0; k < 6; k++ {
		h := float64(k) * s * 0.7
		g.glow(screen, sx, sy-h, s*(0.9-0.1*float64(k)), cr, cg, cb, float32(0.32-0.045*float64(k)))
	}
	pop := math.Min(1, float64(d.Age)*3+0.3)
	r := s * 0.42 * pop * (1 + 0.1*math.Sin(t*4))
	g.glow(screen, sx, sy, r*3, cr, cg, cb, 0.45)
	ring := math.Mod(t*0.8, 1)
	vector.StrokeCircle(screen, float32(sx), float32(sy), float32(r*(1.2+1.2*ring)), g.u(2), fade(colLesson, float32(1-ring)), true)
	var p vector.Path
	p.MoveTo(float32(sx), float32(sy-r*1.35))
	p.LineTo(float32(sx+r), float32(sy))
	p.LineTo(float32(sx), float32(sy+r*1.35))
	p.LineTo(float32(sx-r), float32(sy))
	p.Close()
	vs, is := p.AppendVerticesAndIndicesForFilling(nil, nil)
	paint(vs, colLesson)
	screen.DrawTriangles(vs, is, whiteSub(), &ebiten.DrawTrianglesOptions{AntiAlias: true})
	name := game.Ladder[d.Idx].Name
	g.outlined(screen, g.small, "LEARN "+name, sx, sy-r*2.4, colLesson, 1)
	if d.Full {
		g.outlined(screen, g.small, "make room in your pack", sx, sy+r*2.2, colHot, 1)
	}
}

// lessonArrow points at a lesson off screen, from just inside the edge.
func (g *Game) lessonArrow(screen *ebiten.Image, sx, sy float64) {
	W, H := float64(g.w), float64(g.h)
	cx, cy := W/2, H/2
	dx, dy := sx-cx, sy-cy
	m := 40 * g.k
	k := math.Min((W/2-m)/math.Max(1e-6, math.Abs(dx)), (H/2-m)/math.Max(1e-6, math.Abs(dy)))
	ax, ay := cx+dx*k, cy+dy*k
	a := math.Atan2(dy, dx)
	r := 11 * g.k
	pulse := float32(0.6 + 0.4*math.Sin(float64(g.frame)*0.1))
	var p vector.Path
	p.MoveTo(float32(ax+r*math.Cos(a)), float32(ay+r*math.Sin(a)))
	p.LineTo(float32(ax+r*math.Cos(a+2.5)), float32(ay+r*math.Sin(a+2.5)))
	p.LineTo(float32(ax+r*math.Cos(a-2.5)), float32(ay+r*math.Sin(a-2.5)))
	p.Close()
	vs, is := p.AppendVerticesAndIndicesForFilling(nil, nil)
	paint(vs, fade(colLesson, pulse))
	screen.DrawTriangles(vs, is, whiteSub(), &ebiten.DrawTrianglesOptions{AntiAlias: true})
}

// drawCard draws the card for the lesson just taken: its name and one
// line on what it does for you, fading out at the end.
func (g *Game) drawCard(screen *ebiten.Image, top float64) {
	sim := g.sim
	if sim.CardT <= 0 || sim.State != game.Playing || g.ed.on {
		return
	}
	l := game.Ladder[sim.Card]
	a := float32(math.Min(1, float64(sim.CardT)))
	W := float64(g.w)
	w := math.Min(W-28*g.k, 520*g.k)
	per := max(8, int((w-32*g.k)/(7*g.k)))
	lines := (len(l.Card) + per - 1) / per
	th := float64(lines+1) * 15 * g.k
	r := rect{(W - w) / 2, top, w, 30*g.k + th}
	vector.DrawFilledRect(screen, float32(r.x), float32(r.y), float32(r.w), float32(r.h), fade(colPanel, 0.92*a), false)
	vector.StrokeRect(screen, float32(r.x), float32(r.y), float32(r.w), float32(r.h), g.u(2), fade(colLesson, a), false)
	head := "NEW: " + l.Name
	if sim.Card == 0 {
		head = "HOW TO PLAY"
	}
	g.label(screen, g.face, head, r.x+r.w/2, r.y+15*g.k, fade(colLesson, a), true)
	g.wrapLabel(screen, l.Card, rect{r.x + 8*g.k, r.y + 26*g.k, r.w - 16*g.k, th}, fade(colText, a))
}

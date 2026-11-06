package main

import (
	"fmt"
	"math"

	"github.com/hajimehoshi/ebiten/v2"

	"hexes/game"
)

// The test menu: with the test switch on, TEST opens a list of the
// example builds. Tapping one loads it into your hexes; MY BUILD puts
// your own build back, and TEST OFF does that too before it turns the
// switch off.

type testMenu struct {
	on   bool
	msg  string
	bad  bool
	page int // which page of examples shows, when they don't all fit
}

type testLayout struct {
	panel               rect
	rows                []rect // the examples on this page
	first               int    // the example the first row shows
	mine, off, closeBtn rect
	more                rect // turns the page; zero when everything fits
	page, pages         int
	titleY, msgY        float64
}

// testRowMin is the shortest an example row gets, in css px.
const testRowMin = 40

func (g *Game) testLayout(n int) testLayout {
	var l testLayout
	W, H := float64(g.w), float64(g.h)
	k := g.k
	pad := 14 * k
	gap := 6 * k
	pw := math.Min(W-2*pad, 760*k)
	px := (W - pw) / 2
	l.panel = rect{px, pad + 22*k, pw, H - 2*pad - 22*k}
	l.titleY = l.panel.y + 12*k
	l.msgY = l.titleY + 22*k
	top := l.msgY + 16*k
	bh := 44 * k
	by := H - pad - bh
	cols := 1
	if W > H*1.1 {
		cols = 2
	}
	// Rows stay tall enough to tap: when the examples don't fit at that
	// height, they go on pages and a MORE button turns them.
	avail := by - 8*k - top
	fit := max(1, int((avail+gap)/(testRowMin*k+gap)))
	per := (n + cols - 1) / cols
	shown := n
	if per > fit {
		per, shown = fit, fit*cols
	}
	nb := 3.0
	if shown < n {
		nb = 4
	}
	bw := (pw - (nb-1)*gap) / nb
	l.mine = rect{px, by, bw, bh}
	l.off = rect{px + bw + gap, by, bw, bh}
	l.closeBtn = rect{px + 2*(bw+gap), by, bw, bh}
	if shown < n {
		l.more = rect{px + 3*(bw+gap), by, bw, bh}
		pages := (n + shown - 1) / shown
		l.page, l.pages = g.tm.page%pages, pages
		l.first = l.page * shown
	}
	rh := math.Min(58*k, (avail-float64(per-1)*gap)/float64(per))
	cw := (pw - float64(cols-1)*gap) / float64(cols)
	for i := 0; i < shown && l.first+i < n; i++ {
		c, r := i/per, i%per
		l.rows = append(l.rows, rect{px + float64(c)*(cw+gap), top + float64(r)*(rh+gap), cw, rh})
	}
	return l
}

func (g *Game) openTestMenu() {
	g.tm = testMenu{on: true}
}

func (g *Game) testTap(x, y float64) {
	sim := g.sim
	ex := game.Examples(sim.Reg)
	l := g.testLayout(len(ex))
	tm := &g.tm
	e := &g.ed
	switch {
	case l.closeBtn.hit(x, y):
		tm.on = false
		return
	case l.mine.hit(x, y):
		if sim.Mine == nil {
			tm.msg, tm.bad = "Your own build is already in.", true
			return
		}
		if err := sim.RestoreBuild(); err != nil {
			tm.msg, tm.bad = err.Error()+".", true
			return
		}
		tm.on = false
		e.hex, e.pick, e.board = 0, pickNone, false
		e.msg, e.bad = "Your own build is back.", false
		return
	case l.off.hit(x, y):
		back := sim.Mine != nil
		if back {
			if err := sim.RestoreBuild(); err != nil {
				tm.msg, tm.bad = err.Error()+".", true
				return
			}
		}
		if err := sim.SetTestShapes(false); err != nil {
			tm.msg, tm.bad = err.Error()+".", true
			return
		}
		tm.on = false
		e.hex, e.pick, e.board = 0, pickNone, false
		e.msg = "Test switch off: only shapes you've found. Hexes on a test shape went back to a Ring, and test frames came off."
		if back {
			e.msg = "Test switch off and your own build is back."
		}
		e.bad = false
		return
	}
	if l.more.w > 0 && l.more.hit(x, y) {
		tm.page++
		return
	}
	for j, r := range l.rows {
		if !r.hit(x, y) {
			continue
		}
		i := l.first + j
		if err := sim.LoadExample(i); err != nil {
			tm.msg, tm.bad = err.Error()+".", true
			return
		}
		tm.on = false
		e.hex, e.pick, e.board = 0, pickNone, false
		e.msg, e.bad = "Loaded "+ex[i].Name+": "+ex[i].Blurb+". Your own build waits under TEST, MY BUILD.", false
		return
	}
}

func (g *Game) drawTestMenu(screen *ebiten.Image) {
	sim := g.sim
	ex := game.Examples(sim.Reg)
	l := g.testLayout(len(ex))
	k := g.k
	g.drawRect(screen, rect{0, 0, float64(g.w), float64(g.h)}, fade(colPanel, 1), fade(colPanel, 1))
	g.label(screen, g.face, "TEST  ·  try an example build", l.panel.x, l.titleY, colShape, false)
	msg, c := g.tm.msg, colDim
	switch {
	case g.tm.bad:
		c = colHot
	case msg == "" && sim.Mine != nil:
		msg = "Now playing " + sim.Example + ". Your own build waits under MY BUILD."
	case msg == "":
		msg = "Loading one swaps it into your hexes. Your own build waits until you want it back."
	}
	g.label(screen, g.small, msg, l.panel.x, l.msgY, c, false)
	for j, r := range l.rows {
		e := ex[l.first+j]
		fill, edge := colPanel, fade(colShape, 0.6)
		if e.Name == sim.Example {
			fill, edge = fade(colShape, 0.35), colShape
		}
		g.drawRect(screen, r, fill, edge)
		g.label(screen, g.face, e.Name, r.x+10*k, r.y+r.h*0.32, colText, false)
		g.label(screen, g.small, e.Blurb, r.x+10*k, r.y+r.h*0.72, colDim, false)
	}
	btn := func(r rect, s string, live bool) {
		if live {
			g.drawRect(screen, r, fade(colShape, 0.3), colShape)
			g.label(screen, g.small, s, r.x+r.w/2, r.y+r.h/2, colText, true)
			return
		}
		g.drawRect(screen, r, colPanel, fade(colDim, 0.5))
		g.label(screen, g.small, s, r.x+r.w/2, r.y+r.h/2, colDim, true)
	}
	btn(l.mine, "MY BUILD", sim.Mine != nil)
	btn(l.off, "TEST OFF", true)
	btn(l.closeBtn, "CLOSE", true)
	if l.more.w > 0 {
		btn(l.more, fmt.Sprintf("MORE  %d/%d", l.page+1, l.pages), true)
	}
}

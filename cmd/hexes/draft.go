package main

// The draft screen: the floor-end pick 1 of 3 (4 on a checkpoint), and a
// fresh save's pick of 3 starter kits. One tap on a card keeps it. On a
// phone the cards stack; on a wide screen they sit in a row. Keys 1 to 4
// pick too, and 0 or X takes none.

import (
	"fmt"
	"image/color"
	"math"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"hexes/game"
	"hexes/skill"
)

// draftArm is how long the cards ignore taps after they show, so a tap
// that was already on its way doesn't pick for you.
const draftArm = 0.35

var colDraft = color.RGBA{0xff, 0xd0, 0x5c, 0xff}

// draftHoldFrames is how long a press on a card lasts before it shows
// the full card instead of keeping it.
const draftHoldFrames = 16

// draftHold is a press on a draft card: let go quickly to keep it, hold
// to read its full card.
type draftHold struct {
	on    bool
	card  int
	mouse bool
	id    ebiten.TouchID
	from  int
	shown bool
}

var leanCols = [...]color.RGBA{
	game.LeanIn:   {0x7c, 0xe8, 0x9a, 0xff},
	game.LeanSide: {0x8f, 0xb8, 0xff, 0xff},
	game.LeanWild: {0xe0, 0x8c, 0xff, 0xff},
	game.LeanBig:  {0xff, 0xd0, 0x5c, 0xff},
}

type draftLayout struct {
	head  float64 // y of the title line
	cards [game.MaxDraft]rect
	skip  rect
	wide  bool
}

func (g *Game) draftLayout() draftLayout {
	d := &g.sim.Draft
	W, H, k := float64(g.w), float64(g.h), g.k
	gap := 12 * k
	var l draftLayout
	n := max(1, d.N)
	skipH := 0.0
	if !d.Kits {
		skipH = 46 * k
	}
	l.wide = W >= float64(n)*190*k+float64(n+1)*gap && W > H*0.9
	top := 100 * k
	l.head = top - 44*k
	bottom := H - gap - skipH - gap
	if l.wide {
		cw := math.Min(260*k, (W-float64(n+1)*gap)/float64(n))
		ch := math.Min(230*k, bottom-top)
		x0 := (W - (float64(n)*cw + float64(n-1)*gap)) / 2
		y0 := top + math.Max(0, (bottom-top-ch)/2)
		for i := 0; i < n; i++ {
			l.cards[i] = rect{x0 + float64(i)*(cw+gap), y0, cw, ch}
		}
	} else {
		cw := math.Min(W-2*gap, 460*k)
		ch := math.Min(130*k, (bottom-top-float64(n-1)*gap)/float64(n))
		x0 := (W - cw) / 2
		for i := 0; i < n; i++ {
			l.cards[i] = rect{x0, top + float64(i)*(ch+gap), cw, ch}
		}
	}
	if !d.Kits {
		sw := math.Min(W-2*gap, 320*k)
		l.skip = rect{(W - sw) / 2, H - gap - skipH, sw, skipH}
	}
	return l
}

// updateDraft runs the draft screen while one is up and reports whether
// it took the frame.
func (g *Game) updateDraft() bool {
	s := g.sim
	if s.State != game.Drafting || !s.Draft.On {
		return false
	}
	g.stick.End()
	s.StateT += float32(1 / float64(ebiten.TPS())) // the sim doesn't step while it waits
	pick, picked := g.devDraft()
	if !picked && s.StateT >= draftArm {
		for i := 0; i < s.Draft.N; i++ {
			for _, key := range keyButtons[min(i, len(keyButtons)-1)] {
				if inpututil.IsKeyJustPressed(key) && i < 4 {
					pick, picked = i, true
				}
			}
		}
		if !s.Draft.Kits && (inpututil.IsKeyJustPressed(ebiten.Key0) || inpututil.IsKeyJustPressed(ebiten.KeyX)) {
			pick, picked = -1, true
		}
		type press struct {
			x, y  float64
			mouse bool
			id    ebiten.TouchID
		}
		var taps []press
		for _, id := range inpututil.AppendJustPressedTouchIDs(nil) {
			x, y := ebiten.TouchPosition(id)
			taps = append(taps, press{float64(x), float64(y), false, id})
			g.touched = true
		}
		if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
			x, y := ebiten.CursorPosition()
			taps = append(taps, press{float64(x), float64(y), true, 0})
		}
		l := g.draftLayout()
		h := &g.dhold
		for _, t := range taps {
			if h.on {
				break
			}
			for i := 0; i < s.Draft.N; i++ {
				if l.cards[i].hit(t.x, t.y) {
					*h = draftHold{on: true, card: i, mouse: t.mouse, id: t.id, from: g.frame}
				}
			}
			if !s.Draft.Kits && l.skip.hit(t.x, t.y) {
				pick, picked = -1, true
				break
			}
		}
		if h.on && !picked {
			up := (h.mouse && inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft)) ||
				(!h.mouse && inpututil.IsTouchJustReleased(h.id))
			if !up && g.frame-h.from >= draftHoldFrames {
				if _, ok := s.DraftSheet(s.Draft.Picks[h.card]); ok {
					h.shown = true
				}
			}
			if up {
				if !h.shown {
					pick, picked = h.card, true
				}
				h.on, h.shown = false, false
			}
		}
	}
	if picked {
		g.dhold = draftHold{}
		kits := s.Draft.Kits
		if err := s.TakeDraft(pick); err != nil {
			g.showHint(err.Error())
			g.click(false)
		} else {
			g.click(true)
			if kits {
				g.camX, g.camY = s.P().Pos()
			}
		}
	}
	return true
}

func (g *Game) drawDraft(screen *ebiten.Image) {
	s := g.sim
	d := &s.Draft
	if s.State != game.Drafting || !d.On {
		return
	}
	k := g.k
	l := g.draftLayout()
	vector.DrawFilledRect(screen, 0, 0, float32(g.w), float32(g.h), color.RGBA{0x05, 0x06, 0x0b, 0xec}, false)
	head, sub := fmt.Sprintf("FLOOR %d CLEARED: KEEP ONE", d.Depth), "Tap a card to keep it for good. Hold one to read it."
	if d.N == game.MaxDraft {
		head = fmt.Sprintf("CHECKPOINT %d: KEEP ONE", d.Depth)
	}
	if d.Kits {
		head, sub = "PICK YOUR RIG", "Each one starts you differently. Tap one."
	}
	g.label(screen, g.face, head, float64(g.w)/2, l.head, colDraft, true)
	g.label(screen, g.small, sub, float64(g.w)/2, l.head+20*k, colDim, true)
	armed := float32(math.Min(1, float64(s.StateT/draftArm)))
	for i := 0; i < d.N; i++ {
		g.drawDraftCard(screen, l.cards[i], i, d.Picks[i], armed, l.wide)
	}
	if !d.Kits {
		g.drawRect(screen, l.skip, colPanel, colDim)
		g.label(screen, g.face, fmt.Sprintf("TAKE NONE: +%d DATA", d.Skip), l.skip.x+l.skip.w/2, l.skip.y+l.skip.h/2, colDim, true)
	}
	if h := &g.dhold; h.on && h.shown {
		if sh, ok := s.DraftSheet(d.Picks[h.card]); ok {
			g.drawSheet(screen, sh, true)
		}
	}
}

func (g *Game) drawDraftCard(screen *ebiten.Image, r rect, i int, p game.DraftPick, a float32, wide bool) {
	s := g.sim
	k := g.k
	title, line, rar := s.Describe(p)
	edge := rarityCol[rar]
	tag, tc := p.Lean.String(), leanCols[p.Lean]
	if p.Kind == game.DraftKit {
		kt := game.DealKits(s.RunSeed)[p.Kit]
		edge = typeColor(s.Reg.MustID(kt.StartType()))
		tag, tc = strings.ToUpper(kt.StartType()), edge
	}
	g.drawRect(screen, r, fade(color.RGBA{0x14, 0x18, 0x24, 0xff}, a), fade(edge, a))
	vector.StrokeRect(screen, float32(r.x+2*k), float32(r.y+2*k), float32(r.w-4*k), float32(r.h-4*k), g.u(1), fade(edge, 0.35*a), false)
	pad := 10 * k
	if sh, ok := s.DraftSheet(p); ok {
		// A part: its card, under the lean and where it goes.
		where := "TO YOUR LOCKER"
		if p.Kind == game.DraftNode {
			where = "FREE GRID CELL"
		}
		g.label(screen, g.small, tag+"  ·  "+where, r.x+pad, r.y+pad+6*k, tc, false)
		g.label(screen, g.small, fmt.Sprintf("%d", i+1), r.x+r.w-pad-4*k, r.y+pad+6*k, colDim, true)
		g.drawCardPanel(screen, sh, rect{r.x + 1*k, r.y + 16*k, r.w - 2*k, r.h - 17*k}, "", colText, "", "hold for the full card")
		return
	}
	// The number key, the lean, then the offer.
	g.label(screen, g.small, fmt.Sprintf("%d", i+1), r.x+r.w-pad-4*k, r.y+pad+6*k, fade(colDim, a), true)
	g.label(screen, g.small, tag, r.x+pad, r.y+pad+6*k, fade(tc, a), false)
	ty := r.y + pad + 30*k
	if wide {
		ty = r.y + pad + 36*k
	}
	g.label(screen, g.face, clip(title, int((r.w-2*pad)/(8.5*k))), r.x+pad, ty, fade(colText, a), false)
	// wrapLabel spreads its lines over the box, so size the box to them.
	bw := r.w - 2*pad + 10*k
	per := max(8, int((bw-16*k)/(7*k)))
	n := (len(line)+per-1)/per + 1
	bh := math.Min(r.y+r.h-ty-12*k, float64(n)*15*k)
	body := rect{r.x + pad - 10*k, ty + 8*k, bw, bh}
	g.wrapLabel(screen, line, body, fade(colDim, a))
	if p.Kind == game.DraftNode || p.Kind == game.DraftPart {
		// A rarity pip, bottom right.
		cx, cy := r.x+r.w-pad-4*k, r.y+r.h-pad-4*k
		vector.DrawFilledCircle(screen, float32(cx), float32(cy), g.u(4), fade(rarityCol[rar], a), true)
		_ = skill.Common
	}
}

// clip cuts s to n characters with an ellipsis.
func clip(s string, n int) string {
	if n < 4 || len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

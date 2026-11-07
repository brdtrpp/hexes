package main

// Piece outlines: each kind of part has a shape of its own, the same on
// the open board, in the pack and the locker, so a piece reads at a
// glance before it's laid.
//
//	trigger  circle         (where a hex starts)
//	target   diamond        (who it picks)
//	form     four-point star (how it shows up in the world)
//	effect   square         (what it does)
//	boost    triangle up    (makes the next one stronger)
//	gate     triangle down  (splits YES and NO)
//	charge   battery        (a counter that keeps its value)
//	call     arrow right    (runs another hex)
//	cost     drop           (what it burns)
//	stance   octagon        (on until you tap it off)

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"hexes/skill"
)

// kindNames are what each outline is called, for the editor's caption.
func kindShapeName(k skill.PartKind) string {
	switch k {
	case skill.KindTrigger:
		return "circle: a trigger"
	case skill.KindTarget:
		return "diamond: a target"
	case skill.KindForm:
		return "star: a form"
	case skill.KindEffect:
		return "square: an effect"
	case skill.KindMult:
		return "triangle up: a boost"
	case skill.KindGate:
		return "triangle down: a gate"
	case skill.KindCharge:
		return "battery: a charge"
	case skill.KindCall:
		return "arrow: a call"
	case skill.KindCost:
		return "drop: a cost"
	case skill.KindStance:
		return "octagon: a stance"
	}
	return ""
}

// kindPath builds part kind k's outline, centred on x, y and r out.
func kindPath(k skill.PartKind, x, y, r float64) vector.Path {
	var p vector.Path
	pt := func(a, rr float64) (float32, float32) {
		return float32(x + rr*math.Cos(a)), float32(y + rr*math.Sin(a))
	}
	poly := func(n int, rot, rr float64) {
		for i := 0; i < n; i++ {
			px, py := pt(rot+2*math.Pi*float64(i)/float64(n), rr)
			if i == 0 {
				p.MoveTo(px, py)
			} else {
				p.LineTo(px, py)
			}
		}
		p.Close()
	}
	switch k {
	case skill.KindTrigger:
		p.Arc(float32(x), float32(y), float32(r*0.9), 0, 2*math.Pi, vector.Clockwise)
		p.Close()
	case skill.KindTarget:
		poly(4, -math.Pi/2, r)
	case skill.KindForm:
		// Four points with a pinched waist.
		for i := 0; i < 8; i++ {
			rr := r
			if i%2 == 1 {
				rr = r * 0.42
			}
			px, py := pt(-math.Pi/2+math.Pi/4*float64(i), rr)
			if i == 0 {
				p.MoveTo(px, py)
			} else {
				p.LineTo(px, py)
			}
		}
		p.Close()
	case skill.KindEffect:
		s := float32(r * 0.74)
		p.MoveTo(float32(x)-s, float32(y)-s)
		p.LineTo(float32(x)+s, float32(y)-s)
		p.LineTo(float32(x)+s, float32(y)+s)
		p.LineTo(float32(x)-s, float32(y)+s)
		p.Close()
	case skill.KindMult:
		poly(3, -math.Pi/2, r)
	case skill.KindGate:
		poly(3, math.Pi/2, r)
	case skill.KindCharge:
		w, h := float32(r*0.92), float32(r*0.56)
		n := float32(r * 0.16)
		fx, fy := float32(x), float32(y)
		p.MoveTo(fx-w, fy-h)
		p.LineTo(fx+w-n, fy-h)
		p.LineTo(fx+w-n, fy-h*0.45)
		p.LineTo(fx+w, fy-h*0.45)
		p.LineTo(fx+w, fy+h*0.45)
		p.LineTo(fx+w-n, fy+h*0.45)
		p.LineTo(fx+w-n, fy+h)
		p.LineTo(fx-w, fy+h)
		p.Close()
	case skill.KindCall:
		fx, fy, fr := float32(x), float32(y), float32(r)
		p.MoveTo(fx-fr*0.9, fy-fr*0.4)
		p.LineTo(fx+fr*0.1, fy-fr*0.4)
		p.LineTo(fx+fr*0.1, fy-fr*0.85)
		p.LineTo(fx+fr*0.95, fy)
		p.LineTo(fx+fr*0.1, fy+fr*0.85)
		p.LineTo(fx+fr*0.1, fy+fr*0.4)
		p.LineTo(fx-fr*0.9, fy+fr*0.4)
		p.Close()
	case skill.KindCost:
		fx, fy, fr := float32(x), float32(y), float32(r)
		p.MoveTo(fx, fy-fr)
		p.QuadTo(fx+fr*0.85, fy+fr*0.05, fx+fr*0.6, fy+fr*0.55)
		p.QuadTo(fx, fy+fr*1.1, fx-fr*0.6, fy+fr*0.55)
		p.QuadTo(fx-fr*0.85, fy+fr*0.05, fx, fy-fr)
		p.Close()
	case skill.KindStance:
		poly(8, math.Pi/8, r*0.95)
	default:
		poly(6, 0, r)
	}
	return p
}

// drawKindShape fills and outlines kind k's shape.
func (g *Game) drawKindShape(dst *ebiten.Image, k skill.PartKind, x, y, r float64, fill, edge color.RGBA, w float32) {
	p := kindPath(k, x, y, r)
	if fill.A > 0 {
		vs, is := p.AppendVerticesAndIndicesForFilling(nil, nil)
		paint(vs, fill)
		dst.DrawTriangles(vs, is, whiteSub(), &ebiten.DrawTrianglesOptions{AntiAlias: true})
	}
	if w > 0 {
		vs, is := p.AppendVerticesAndIndicesForStroke(nil, nil, &vector.StrokeOptions{Width: w, LineJoin: vector.LineJoinRound})
		paint(vs, edge)
		dst.DrawTriangles(vs, is, whiteSub(), &ebiten.DrawTrianglesOptions{AntiAlias: true})
	}
}

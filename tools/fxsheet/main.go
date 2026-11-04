// Command fxsheet renders a contact sheet of the procedural effects
// (particles only) to a PNG, for checking motion without a browser.
package main

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"

	"hexes/fx"
	"hexes/skill"
)

var cols = [][3]float32{{1, .48, .18}, {.56, .86, 1}, {.94, .94, .94}, {.55, 1, .35}, {.75, .55, 1}, {.6, .63, .69}}

func main() {
	reg := skill.DefaultRegistry()
	const cell, scale = 220, 22.0
	times := []float32{0.05, 0.15, 0.35, 0.6}
	modes := 4
	img := image.NewRGBA(image.Rect(0, 0, cell*len(times)*modes, cell*reg.Len()))
	for i := range img.Pix {
		if i%4 == 3 {
			img.Pix[i] = 255
		} else {
			img.Pix[i] = 12
		}
	}
	for ti := 0; ti < reg.Len(); ti++ {
		c := cols[ti]
		sig := fx.SigOf(*reg.Get(skill.TypeID(ti)), c[0], c[1], c[2])
		for m := 0; m < modes; m++ {
			for k, tt := range times {
				p := fx.NewPool(uint32(ti*31 + m + 7))
				shot := fx.Shot{Sig: sig, Mag: 1, Heat: 10}
				switch m {
				case 0:
					p.Trail(-3.5, 0, 0, 0, shot)
					p.Impact(0, 0, 1, 0, shot)
				case 1:
					p.Wave(0, 0, 2.5*1.73, shot)
				case 2:
					p.Wedge(-3.5, 0, 1, 0, 35*math.Pi/180, 6, shot)
				case 3:
					p.Cast(0, 0, 20, reg.Get(skill.TypeID(ti)).HeatOffset*4)
					for f := 0; f < 30; f++ {
						p.Aura(fx.AuraKind(ti%int(fx.NumAuras)), 2.5, 2.5, 0.55, c[0], c[1], c[2], 1.0/60)
					}
				}
				for s := float32(0); s < tt; s += 1.0 / 60 {
					p.Update(1.0 / 60)
				}
				ox := float64((m*len(times)+k)*cell + cell/2)
				oy := float64(ti*cell + cell/2)
				for i := 0; i < p.N; i++ {
					q := &p.P[i]
					l := q.Look()
					cx, cy := ox+float64(q.X)*scale, oy+float64(q.Y)*scale
					r := math.Max(1, float64(l.Size)*scale)
					ln := float64(l.Len) * scale / 2
					for y := -int(r+ln) - 1; y <= int(r+ln)+1; y++ {
						for x := -int(r+ln) - 1; x <= int(r+ln)+1; x++ {
							// distance to segment
							px, py := float64(x), float64(y)
							t := px*float64(l.DX) + py*float64(l.DY)
							t = math.Max(-ln, math.Min(ln, t))
							dx, dy := px-t*float64(l.DX), py-t*float64(l.DY)
							d := math.Hypot(dx, dy) / r
							if d >= 1 {
								continue
							}
							a := float64(l.A) * (1 - d) * (1 - d)
							ix, iy := int(cx)+x, int(cy)+y
							if !(image.Pt(ix, iy).In(img.Rect)) {
								continue
							}
							o := img.RGBAAt(ix, iy)
							add := func(v uint8, c float32) uint8 { return uint8(math.Min(255, float64(v)+a*float64(c)*255)) }
							img.SetRGBA(ix, iy, color.RGBA{add(o.R, l.R), add(o.G, l.G), add(o.B, l.B), 255})
						}
					}
				}
			}
		}
	}
	f, _ := os.Create(os.Args[1])
	png.Encode(f, img)
	f.Close()
}

package main

// The world's look: floor tiles with panel detail and raised walls, actor
// bodies drawn as small machines that face, breathe and squash when hit,
// a light falloff round the player, drifting dust, screen shake and a red
// edge when you're hurt. Every sprite is drawn procedurally at the current
// hex size when the size changes, so the floor and the actors still draw
// as batched sprites each frame.

import (
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"hexes/dungeon"
	"hexes/fx"
	"hexes/game"
	"hexes/skill"
)

type lookState struct {
	shake, shx, shy float64
	hurt            float32 // red edge, 0 to 1
	trail           []float32
	vignette        *ebiten.Image
	floor           [4]*ebiten.Image
	wallLift        float64 // how far a wall's top sits above its base, pixels
	body            [4]*ebiten.Image
	bodyR           [4]float64
}

// Palette for the world.
var (
	colFloorA    = color.RGBA{0x1f, 0x25, 0x33, 0xff}
	colFloorHi   = color.RGBA{0x34, 0x3e, 0x55, 0xff}
	colFloorSeam = color.RGBA{0x16, 0x1b, 0x26, 0xff}
	colWallSide  = color.RGBA{0x1a, 0x1f, 0x2e, 0xff}
	colWallFace  = color.RGBA{0x3a, 0x44, 0x5e, 0xff}
	colWallRim   = color.RGBA{0x76, 0x86, 0xac, 0xff}
)

func hexPath(cx, cy, r float64) *vector.Path {
	var p vector.Path
	for i := 0; i < 6; i++ {
		a := math.Pi/180*float64(60*i) - math.Pi/6
		x, y := float32(cx+r*math.Cos(a)), float32(cy+r*math.Sin(a))
		if i == 0 {
			p.MoveTo(x, y)
		} else {
			p.LineTo(x, y)
		}
	}
	p.Close()
	return &p
}

func polyPath(pts ...[2]float64) *vector.Path {
	var p vector.Path
	for i, q := range pts {
		if i == 0 {
			p.MoveTo(float32(q[0]), float32(q[1]))
		} else {
			p.LineTo(float32(q[0]), float32(q[1]))
		}
	}
	p.Close()
	return &p
}

func circlePath(cx, cy, r float64) *vector.Path {
	var p vector.Path
	p.Arc(float32(cx), float32(cy), float32(r), 0, 2*math.Pi, vector.Clockwise)
	p.Close()
	return &p
}

func fillOn(img *ebiten.Image, p *vector.Path, c color.RGBA) {
	vs, is := p.AppendVerticesAndIndicesForFilling(nil, nil)
	paint(vs, c)
	img.DrawTriangles(vs, is, whiteSub(), &ebiten.DrawTrianglesOptions{AntiAlias: true})
}

func strokeOn(img *ebiten.Image, p *vector.Path, c color.RGBA, w float32) {
	vs, is := p.AppendVerticesAndIndicesForStroke(nil, nil, &vector.StrokeOptions{Width: w, LineJoin: vector.LineJoinRound, LineCap: vector.LineCapRound})
	paint(vs, c)
	img.DrawTriangles(vs, is, whiteSub(), &ebiten.DrawTrianglesOptions{AntiAlias: true})
}

func lineOn(img *ebiten.Image, x0, y0, x1, y1 float64, c color.RGBA, w float32) {
	var p vector.Path
	p.MoveTo(float32(x0), float32(y0))
	p.LineTo(float32(x1), float32(y1))
	strokeOn(img, &p, c, w)
}

// buildTiles draws every floor and actor sprite for the current hex size.
func (g *Game) buildTiles() {
	g.tileSize = g.size
	g.tiles = map[dungeon.Tile]*ebiten.Image{}
	s := g.size
	k := g.k
	w, h := int(math.Ceil(math.Sqrt(3)*s))+2, int(math.Ceil(2*s))+2
	cx, cy := float64(w)/2, float64(h)/2
	r := s - k
	// Floor: four panel variants, picked per tile by a hash.
	for v := range g.look.floor {
		img := ebiten.NewImage(w, h)
		fillOn(img, hexPath(cx, cy, r), colFloorA)
		strokeOn(img, hexPath(cx, cy, r-1.2*k), fade(colFloorHi, 0.55), float32(k))
		switch v {
		case 1:
			strokeOn(img, hexPath(cx, cy, r*0.52), colFloorSeam, float32(1.4*k))
			strokeOn(img, hexPath(cx, cy+k, r*0.52), fade(colFloorHi, 0.35), float32(0.8*k))
		case 2:
			for _, sx := range []float64{-1, 1} {
				fillOn(img, circlePath(cx+sx*r*0.42, cy-r*0.28, r*0.07), colFloorSeam)
				fillOn(img, circlePath(cx-sx*r*0.42, cy+r*0.28, r*0.07), colFloorSeam)
			}
		case 3:
			for j := -2; j <= 2; j++ {
				y := cy + float64(j)*r*0.16
				lineOn(img, cx-r*0.45, y, cx+r*0.45, y, colFloorSeam, float32(1.3*k))
			}
		}
		g.look.floor[v] = img
	}
	g.tiles[dungeon.Ground] = g.look.floor[0]

	// Walls are raised blocks: a dark side and a lit top face lifted up.
	lift := s * 0.55
	g.look.wallLift = lift
	{
		img := ebiten.NewImage(w, h+int(math.Ceil(lift)))
		bx, by := cx, cy+lift // base hex centre
		hw := math.Sqrt(3) / 2 * s
		fillOn(img, hexPath(bx, by, s), colWallSide)
		fillOn(img, polyPath([2]float64{bx - hw, cy - s/2}, [2]float64{bx + hw, cy - s/2}, [2]float64{bx + hw, by + s/2}, [2]float64{bx - hw, by + s/2}), colWallSide)
		// A faint lit band where the side meets the top.
		fillOn(img, polyPath([2]float64{bx - hw, cy + s/2}, [2]float64{bx, cy + s}, [2]float64{bx + hw, cy + s/2}, [2]float64{bx + hw, cy + s/2 + lift*0.35}, [2]float64{bx, cy + s + lift*0.35}, [2]float64{bx - hw, cy + s/2 + lift*0.35}), color.RGBA{0x28, 0x2f, 0x43, 0xff})
		fillOn(img, hexPath(cx, cy, s), colWallFace)
		strokeOn(img, hexPath(cx, cy, s-0.8*k), colWallRim, float32(1.6*k))
		strokeOn(img, hexPath(cx, cy, s*0.6), fade(colWallRim, 0.25), float32(k))
		g.tiles[dungeon.Wall] = img
	}
	{
		img := ebiten.NewImage(w, h)
		fillOn(img, hexPath(cx, cy, r), colPit)
		fillOn(img, hexPath(cx, cy+r*0.12, r*0.8), color.RGBA{0x00, 0x00, 0x00, 0xff})
		strokeOn(img, hexPath(cx, cy, r-k), colPitRim, float32(2*k))
		strokeOn(img, hexPath(cx, cy+r*0.12, r*0.6), fade(colPitRim, 0.3), float32(k))
		strokeOn(img, hexPath(cx, cy+r*0.22, r*0.35), fade(colPitRim, 0.15), float32(k))
		g.tiles[dungeon.Pit] = img
	}
	{
		img := ebiten.NewImage(w, h)
		fillOn(img, hexPath(cx, cy, r), color.RGBA{0x4a, 0x1c, 0x0a, 0xff})
		fillOn(img, hexPath(cx, cy, r*0.62), colHazard)
		for j := -1; j <= 1; j++ {
			y := cy + float64(j)*r*0.2
			lineOn(img, cx-r*0.32, y, cx+r*0.32, y, color.RGBA{0x2a, 0x0c, 0x04, 0xff}, float32(2*k))
		}
		strokeOn(img, hexPath(cx, cy, r-k), color.RGBA{0xff, 0xb0, 0x40, 0xff}, float32(1.5*k))
		g.tiles[dungeon.Hazard] = img
	}
	{
		img := ebiten.NewImage(w, h)
		fillOn(img, hexPath(cx, cy, r), color.RGBA{0x0c, 0x3e, 0x3a, 0xff})
		fillOn(img, hexPath(cx, cy, r*0.66), colStairs)
		strokeOn(img, hexPath(cx, cy, r-k), color.RGBA{0xc8, 0xff, 0xf2, 0xff}, float32(2.5*k))
		g.tiles[dungeon.Stairs] = img
	}
	g.buildBodies()
	if g.look.vignette == nil {
		g.look.vignette = buildVignette()
	}
}

// buildVignette is a white disc of alpha that is clear in the middle and
// thick at the edge; drawn black it's the light falloff, drawn red it's
// the hurt edge.
func buildVignette() *ebiten.Image {
	const n = 256
	pix := make([]byte, n*n*4)
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			d := math.Hypot(float64(x)+0.5-n/2, float64(y)+0.5-n/2) / (n / 2)
			t := math.Min(1, math.Max(0, (d-0.22)/(0.62-0.22)))
			a := t * t * (3 - 2*t)
			b := byte(255 * a)
			i := (y*n + x) * 4
			pix[i], pix[i+1], pix[i+2], pix[i+3] = b, b, b, b
		}
	}
	img := ebiten.NewImage(n, n)
	img.WritePixels(pix)
	return img
}

// bodyRadius is an actor kind's radius as a fraction of the hex size.
func bodyRadius(k game.Kind) float64 {
	switch k {
	case game.KindBrute:
		return 0.72
	case game.KindSkitter:
		return 0.45
	}
	return 0.58
}

// buildBodies draws each kind's body facing +x.
func (g *Game) buildBodies() {
	k := g.k
	for kind := game.KindPlayer; kind <= game.KindBrute; kind++ {
		R := g.size * bodyRadius(kind)
		n := int(math.Ceil(R*2.8)) + 4
		img := ebiten.NewImage(n, n)
		c := float64(n) / 2
		pt := func(x, y float64) [2]float64 { return [2]float64{c + x*R, c + y*R} }
		switch kind {
		case game.KindPlayer:
			fillOn(img, polyPath(pt(-1.05, -0.62), pt(-0.7, -0.62), pt(-0.7, -0.3), pt(-1.05, -0.3)), color.RGBA{0x2a, 0x8f, 0xa8, 0xff})
			fillOn(img, polyPath(pt(-1.05, 0.62), pt(-0.7, 0.62), pt(-0.7, 0.3), pt(-1.05, 0.3)), color.RGBA{0x2a, 0x8f, 0xa8, 0xff})
			fillOn(img, circlePath(c, c, R), color.RGBA{0x10, 0x34, 0x44, 0xff})
			strokeOn(img, circlePath(c, c, R-k), colPlayer, float32(2*k))
			strokeOn(img, circlePath(c, c, R*0.62), color.RGBA{0x2a, 0x8f, 0xa8, 0xff}, float32(1.2*k))
			fillOn(img, polyPath(pt(1.08, 0), pt(0.42, -0.42), pt(0.58, 0), pt(0.42, 0.42)), colPlayer)
		case game.KindGrunt:
			col := enemyCol[kind]
			fillOn(img, polyPath(pt(1.25, -0.5), pt(0.55, -0.85), pt(0.7, -0.3)), col)
			fillOn(img, polyPath(pt(1.25, 0.5), pt(0.55, 0.85), pt(0.7, 0.3)), col)
			fillOn(img, hexPath(c, c, R*0.98), color.RGBA{0x4a, 0x14, 0x1a, 0xff})
			strokeOn(img, hexPath(c, c, R*0.98-k), col, float32(2*k))
			strokeOn(img, hexPath(c, c, R*0.55), fade(col, 0.5), float32(1.2*k))
		case game.KindSkitter:
			col := enemyCol[kind]
			for _, sy := range []float64{-1, 1} {
				for j := 0; j < 3; j++ {
					x := 0.3 - float64(j)*0.45
					lineOn(img, c+x*R, c+sy*0.3*R, c+(x-0.25)*R, c+sy*1.15*R, col, float32(1.5*k))
				}
			}
			body := polyPath(pt(1.1, 0), pt(-0.55, -0.7), pt(-0.85, 0), pt(-0.55, 0.7))
			fillOn(img, body, color.RGBA{0x4a, 0x3a, 0x0c, 0xff})
			strokeOn(img, body, col, float32(1.8*k))
		case game.KindBrute:
			col := enemyCol[kind]
			var oct [8][2]float64
			for i := range oct {
				a := math.Pi/8 + float64(i)*math.Pi/4
				oct[i] = pt(math.Cos(a)*0.98, math.Sin(a)*0.98)
			}
			body := polyPath(oct[:]...)
			fillOn(img, body, color.RGBA{0x2c, 0x16, 0x48, 0xff})
			strokeOn(img, body, col, float32(3*k))
			for i := 0; i < 4; i++ {
				a := math.Pi/4 + float64(i)*math.Pi/2
				ca, sa := math.Cos(a), math.Sin(a)
				px, py := ca*0.58, sa*0.58
				q := func(u, v float64) [2]float64 { return pt(px+u*ca-v*sa, py+u*sa+v*ca) }
				fillOn(img, polyPath(q(-0.16, -0.2), q(0.16, -0.2), q(0.16, 0.2), q(-0.16, 0.2)), color.RGBA{0x4e, 0x2c, 0x78, 0xff})
			}
			fillOn(img, polyPath(pt(0.42, -0.42), pt(0.82, -0.3), pt(0.82, 0.3), pt(0.42, 0.42)), col)
		}
		g.look.body[kind] = img
		g.look.bodyR[kind] = R
	}
}

// zoneTint is the floor's cast for a depth: every few floors the station
// changes, from bare steel to rust, moss, violet and ice, and blends into
// the next one across a floor so the change reads as going deeper.
var zoneTints = [][3]float32{
	{1, 1, 1.05},
	{1.22, 0.92, 0.78},
	{0.86, 1.12, 0.92},
	{1.08, 0.86, 1.22},
	{0.82, 1.02, 1.28},
}

func zoneTint(depth int) (float32, float32, float32) {
	const per = 3
	d := max(0, depth-1)
	z := d / per % len(zoneTints)
	a := zoneTints[z]
	if d%per != per-1 {
		return a[0], a[1], a[2]
	}
	b := zoneTints[(z+1)%len(zoneTints)]
	return (a[0] + b[0]) / 2, (a[1] + b[1]) / 2, (a[2] + b[2]) / 2
}

func tileHash(c skill.Coord) uint32 {
	x := uint32(c.Q)*0x9e3779b1 ^ uint32(c.R)*0x85ebca6b
	x ^= x >> 15
	x *= 0x2c1b3c6d
	x ^= x >> 12
	return x
}

func (g *Game) drawFloor(screen *ebiten.Image) {
	f := g.sim.Floor
	s := g.size
	halfW := float64(g.w)/2/s + 2
	halfH := float64(g.h)/2/s + 2
	r0 := max(0, int((g.camY-halfH)/1.5))
	r1 := min(f.H-1, int((g.camY+halfH+1)/1.5)+1)
	t := float64(g.frame) / 60
	pulse := float32(0.75 + 0.25*math.Sin(t*7.2))
	zr, zg, zb := zoneTint(g.sim.Depth)
	g.beginFog()
	reach := g.lampReach()
	// Floors first, then walls in row order, so a wall's lifted top
	// overlaps the floor behind it and its side faces the floor in front.
	for pass := 0; pass < 2; pass++ {
		for r := r0; r <= r1; r++ {
			for col := 0; col < f.W; col++ {
				c := skill.Coord{Q: col - r>>1, R: r}
				x, y := c.Pixel()
				if math.Abs(x-g.camX) > halfW {
					continue
				}
				tile := f.At(c)
				if (tile == dungeon.Wall) != (pass == 1) {
					continue
				}
				if tile == dungeon.Wall && !g.nearOpen(c) {
					continue
				}
				h := tileHash(c)
				img := g.tiles[tile]
				if tile == dungeon.Ground {
					v := h % 16
					if v < 9 {
						v = 0
					} else {
						v = 1 + v%3
					}
					img = g.look.floor[v]
				}
				sx, sy := g.toScreen(x, y)
				b := img.Bounds()
				oy := float64(b.Dy()) / 2
				if tile == dungeon.Wall {
					oy = float64(b.Dy()) - (float64(b.Dy())-g.look.wallLift)/2
				}
				cr, cg, cb := float32(1), float32(1), float32(1)
				switch tile {
				case dungeon.Hazard:
					cr, cg, cb = pulse, pulse, pulse
				case dungeon.Ground, dungeon.Wall:
					v := 0.9 + 0.16*float32(h>>8&0xff)/255
					cr, cg, cb = v*zr, v*zg, v*zb
				}
				i := f.Index(c)
				g.drawLitTile(screen, img, tile, i, x, y, sx, sy, sx-float64(b.Dx())/2, sy-oy, cr, cg, cb, reach)
				if tile == dungeon.Stairs && g.sim.Sight.Seen[i] {
					g.drawStairs(screen, sx, sy, t)
				}
			}
		}
	}
}

func (g *Game) drawStairs(screen *ebiten.Image, sx, sy, t float64) {
	s := g.size
	// A slow beacon: a glow that breathes and a hex ring that rises off it.
	k := math.Mod(t*0.7, 1)
	g.glow(screen, sx, sy, s*(1.4+0.2*math.Sin(t*3)), 0.12, 0.78, 0.66, 0.5)
	g.ringPath(sx, sy-k*s*0.8, s*(0.5+0.5*k), fx.RingStyle{Sides: 6}, t, 0, 0, 2*math.Pi, true)
	g.strokePath(screen, colStairs, float32(1-k)*0.8, g.u(2))
	g.label(screen, g.face, "▼", sx, sy, colBG, true)
}

// glow draws a soft additive light of radius r.
func (g *Game) glow(dst *ebiten.Image, x, y, r float64, cr, cg, cb, a float32) {
	src := g.fxs.atlas.SubImage(image.Rect(0, 0, spriteCell, spriteCell)).(*ebiten.Image)
	op := &ebiten.DrawImageOptions{}
	sc := r / (spriteCell / 2)
	op.GeoM.Translate(-spriteCell/2, -spriteCell/2)
	op.GeoM.Scale(sc, sc)
	op.GeoM.Translate(x, y)
	op.ColorScale.Scale(cr*a, cg*a, cb*a, a)
	op.Blend = ebiten.BlendLighter
	op.Filter = ebiten.FilterLinear
	dst.DrawImage(src, op)
}

func (g *Game) drawActors(screen *ebiten.Image) {
	sim := g.sim
	s := g.size
	t := float64(g.frame) / 60
	if len(g.look.trail) != len(sim.Actors) {
		g.look.trail = make([]float32, len(sim.Actors))
		for i := range g.look.trail {
			g.look.trail[i] = 1
		}
	}
	op := &ebiten.DrawImageOptions{}
	for i := range sim.Actors {
		a := &sim.Actors[i]
		alpha := float32(1)
		R := g.look.bodyR[a.Kind]
		scale := 1.0
		spin := 0.0
		if !a.U.Alive {
			if a.DeadFor > 0.6 {
				continue
			}
			kk := float64(1 - a.DeadFor/0.6)
			alpha = float32(kk)
			scale = 1 + 0.25*(1-kk)
			if a.Fell {
				scale = kk * kk
				spin = (1 - kk) * 4
			}
		}
		x, y := a.Pos()
		sx, sy := g.toScreen(x, y)
		if sx < -s*2 || sy < -s*2 || sx > float64(g.w)+s*2 || sy > float64(g.h)+s*2 {
			continue
		}
		if a.Kind != game.KindPlayer { // out of the light, out of sight (fog.go)
			if alpha *= g.seeAt(x, y); alpha < 0.02 {
				continue
			}
		}
		fx0, fy0 := a.FaceVec()
		ang := math.Atan2(fy0, fx0) + spin
		if a.Kind == game.KindSkitter && a.U.Alive {
			ang += 0.14 * math.Sin(t*16+float64(i))
		}
		// Breathing, and a squash when hit.
		hit := float64(a.Flash / 0.15)
		bx := scale * (1 + 0.03*math.Sin(t*3+float64(i)*1.7) + 0.2*hit)
		by := scale * (1 - 0.03*math.Sin(t*3+float64(i)*1.7) - 0.12*hit)

		// Shadow.
		g.drawEllipse(screen, sx, sy+R*0.6, R*1.05*scale, R*0.42*scale, 0, 0, 0, 0.45*alpha)

		cr, cg, cb := float32(1), float32(1), float32(1)
		if a.U.SlowLeft > 0 {
			cr, cg, cb = 0.65, 0.85, 1.1
		}
		if a.U.Frozen {
			cr, cg, cb = 0.75, 0.95, 1.2
		}
		if a.U.Stealth > 0 {
			alpha *= 0.45
		}
		img := g.look.body[a.Kind]
		b := img.Bounds()
		op.GeoM.Reset()
		op.ColorScale.Reset()
		op.Blend = ebiten.Blend{}
		op.Filter = ebiten.FilterLinear
		op.GeoM.Translate(-float64(b.Dx())/2, -float64(b.Dy())/2)
		op.GeoM.Scale(bx, by)
		op.GeoM.Rotate(ang)
		op.GeoM.Translate(sx, sy)
		op.ColorScale.Scale(min(1, cr)*alpha, min(1, cg)*alpha, min(1, cb)*alpha, alpha)
		screen.DrawImage(img, op)
		if a.Flash > 0 || a.U.Frozen {
			wa := float32(hit) * 0.9
			if a.U.Frozen {
				wa = max(wa, 0.25)
			}
			wa *= alpha
			op.ColorScale.Reset()
			op.ColorScale.Scale(wa, wa, wa, wa)
			op.Blend = ebiten.BlendLighter
			screen.DrawImage(img, op)
		}

		// Eye or core light.
		dx, dy := math.Cos(ang), math.Sin(ang)
		if a.U.Alive {
			switch a.Kind {
			case game.KindPlayer:
				hf := float32(sim.Rig.Heat / sim.Rig.MaxHeat)
				r0, g0, b0 := colorF(colPlayer)
				hr, hg, hb := colorF(colHeat)
				pr, pg, pb := r0+(hr-r0)*hf, g0+(hg-g0)*hf, b0+(hb-b0)*hf
				g.glow(screen, sx, sy, R*(0.7+0.08*math.Sin(t*5)), pr, pg, pb, 0.9)
				g.glow(screen, sx+dx*R*0.75, sy+dy*R*0.75, R*0.35, 1, 1, 1, 0.6)
			default:
				col := enemyCol[a.Kind]
				er, eg, eb := colorF(col)
				fwd := 0.5
				if a.Kind == game.KindBrute {
					fwd = 0.62
				}
				blink := float32(0.75 + 0.25*math.Sin(t*6+float64(i)))
				if a.Awake {
					blink = 1
				}
				g.glow(screen, sx+dx*R*fwd, sy+dy*R*fwd, R*0.45, er, eg, eb, 0.9*blink*alpha)
				g.glow(screen, sx+dx*R*fwd, sy+dy*R*fwd, R*0.16, 1, 1, 1, 0.8*blink*alpha)
			}
		}

		// Elites (above the floor's level) get a turning dashed ring.
		if a.U.Alive && a.Kind != game.KindPlayer && a.U.Level > sim.Depth {
			g.ringPath(sx, sy, R*1.3, fx.RingStyle{Sides: 40, Dash: 0.6, Spin: 0.4}, t, 0, 0, 2*math.Pi, true)
			g.strokePath(screen, colText, 0.7*alpha, g.u(1.6))
		}
		if !a.U.Alive {
			continue
		}
		if a.Kind == game.KindPlayer {
			g.drawRigArcs(screen, sx, sy, R)
			continue
		}
		// Health bar with a trailing chunk for the last hit.
		frac := a.U.HP / a.U.MaxHP
		tr := &g.look.trail[i]
		if frac > *tr {
			*tr = frac
		}
		*tr = max(frac, *tr-1.0/60*0.7)
		if a.U.HP < a.U.MaxHP && alpha > 0.5 {
			bw := float32(s * 1.3)
			bh := g.u(4)
			bx0, by0 := float32(sx)-bw/2, float32(sy-R)-g.u(9)
			vector.DrawFilledRect(screen, bx0-g.u(1), by0-g.u(1), bw+g.u(2), bh+g.u(2), color.RGBA{0, 0, 0, 0xd0}, false)
			vector.DrawFilledRect(screen, bx0, by0, bw**tr, bh, color.RGBA{0xff, 0xe8, 0xb0, 0xe0}, false)
			hc := colHP
			if frac < 0.25 {
				hc = colHot
			} else if frac < 0.5 {
				hc = color.RGBA{0xf2, 0xc1, 0x3d, 0xff}
			}
			vector.DrawFilledRect(screen, bx0, by0, bw*frac, bh, hc, false)
			for q := 1; q < 4; q++ {
				vector.DrawFilledRect(screen, bx0+bw*float32(q)/4, by0, g.u(1), bh, color.RGBA{0, 0, 0, 0x90}, false)
			}
		}
	}
}

// drawRigArcs puts the rig on the player: Power as an arc on the left,
// Heat on the right, so you can read both without looking up.
func (g *Game) drawRigArcs(screen *ebiten.Image, sx, sy, R float64) {
	rig := &g.sim.Rig
	t := float64(g.frame) / 60
	r := R * 1.5
	plain := fx.RingStyle{Sides: 40}
	// Power: left side, filling up from the bottom.
	pf := float64(rig.Power / rig.MaxPower)
	g.ringPath(sx, sy, r, plain, t, 0, math.Pi*0.62, math.Pi*1.38, false)
	g.strokePath(screen, colPower, 0.18, g.u(2.5))
	if pf > 0.01 {
		g.ringPath(sx, sy, r, plain, t, 0, math.Pi*0.62, math.Pi*(0.62+0.76*pf), false)
		g.strokePath(screen, colPower, 0.85, g.u(2.5))
	}
	// Heat: right side, filling up from the bottom, pulsing near the top.
	hf := float64(rig.Heat / rig.MaxHeat)
	hc := colHeat
	ha := float32(0.85)
	if hf > 0.75 || rig.Locked() {
		hc = colHot
		ha = float32(0.6 + 0.4*math.Sin(t*14))
	}
	g.ringPath(sx, sy, r, plain, t, 0, math.Pi*0.38-math.Pi*0.76, math.Pi*0.38, false)
	g.strokePath(screen, colHeat, 0.18, g.u(2.5))
	if hf > 0.01 {
		g.ringPath(sx, sy, r, plain, t, 0, math.Pi*0.38-math.Pi*0.76*hf, math.Pi*0.38, false)
		g.strokePath(screen, hc, ha, g.u(2.5))
	}
}

func (g *Game) drawEllipse(dst *ebiten.Image, x, y, rx, ry float64, cr, cg, cb, ca float32) {
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(-32, -32)
	op.GeoM.Scale(rx/31, ry/31)
	op.GeoM.Translate(x, y)
	op.ColorScale.Scale(cr*ca, cg*ca, cb*ca, ca)
	op.Filter = ebiten.FilterLinear
	dst.DrawImage(g.disc, op)
}

// drawLight darkens the world away from the player and reddens the edge
// when the player is hurt. Effects draw after it, so they glow in the dark.
func (g *Game) drawLight(screen *ebiten.Image) {
	v := g.look.vignette
	if v == nil {
		return
	}
	px, py := g.playerScreen()
	full := math.Max(float64(g.w), float64(g.h)) * 0.95
	sc := full / (0.62 * 128)
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(-128, -128)
	op.GeoM.Scale(sc, sc)
	op.GeoM.Translate(px, py)
	op.Filter = ebiten.FilterLinear
	// The fog (fog.go) does the darkness now; this only shades the far
	// corners of the screen a touch, so the lit room still feels lamp-lit.
	dark := float32(min(0.45, 0.3+0.01*float64(g.sim.Depth)))
	op.ColorScale.Scale(0, 0, 0, dark)
	screen.DrawImage(v, op)
	if h := g.look.hurt; h > 0 {
		op.GeoM.Reset()
		op.GeoM.Translate(-128, -128)
		op.GeoM.Scale(sc*0.62, sc*0.62)
		op.GeoM.Translate(float64(g.w)/2, float64(g.h)/2)
		op.ColorScale.Reset()
		a := h * 0.55
		op.ColorScale.Scale(0.9*a, 0.08*a, 0.08*a, a)
		screen.DrawImage(v, op)
	}
}

// shakeCamera turns the shake level into this frame's offset.
func (g *Game) shakeCamera() {
	l := &g.look
	if l.shake <= 0.001 {
		l.shx, l.shy = 0, 0
		return
	}
	amp := l.shake * l.shake * g.size * 0.45
	t := float64(g.frame)
	l.shx = amp * math.Sin(t*1.9) * math.Cos(t*0.73)
	l.shy = amp * math.Cos(t*2.3) * math.Sin(t*1.1)
}

// updateLook decays shake and hurt and spawns ambient particles: dust in
// the air and embers off hazard vents in view.
func (g *Game) updateLook(dt float32) {
	l := &g.look
	l.shake = math.Max(0, l.shake-float64(dt)*2.2)
	l.hurt = max(0, l.hurt-dt*2.5)
	p := g.fxs.parts
	halfW := float32(float64(g.w) / 2 / g.size)
	halfH := float32(float64(g.h) / 2 / g.size)
	cx, cy := float32(g.camX), float32(g.camY)
	if n := p.Count(10 * dt * 6); n > 0 && p.N < fx.Max/3 {
		for i := 0; i < n; i++ {
			seed := float32(g.frame*7+i*13) * 0.618
			fx0 := float32(math.Mod(float64(seed), 1))*2 - 1
			fy0 := float32(math.Mod(float64(seed*1.37), 1))*2 - 1
			p.Add(fx.P{X: cx + fx0*halfW, Y: cy + fy0*halfH, VX: 0.15 * fy0, VY: -0.12 - 0.1*fx0,
				Life: 4, Size0: 0.025, Size1: 0.02, A: 0.22, Wobble: 0.25, Seed: seed,
				R0: 0.7, G0: 0.85, B0: 1, R1: 0.7, G1: 0.85, B1: 1})
		}
	}
	// Hazard embers, only for tiles in view.
	f := g.sim.Floor
	r0 := max(0, int((cy-halfH-1)/1.5))
	r1 := min(f.H-1, int((cy+halfH+1)/1.5)+1)
	for r := r0; r <= r1; r++ {
		for col := 0; col < f.W; col++ {
			c := skill.Coord{Q: col - r>>1, R: r}
			if f.At(c) != dungeon.Hazard {
				continue
			}
			x, y := c.Pixel()
			if math.Abs(x-float64(cx)) > float64(halfW)+1 {
				continue
			}
			p.Aura(fx.AuraBurn, float32(x), float32(y), 0.5, 1, 0.55, 0.15, dt*0.35)
		}
	}
}

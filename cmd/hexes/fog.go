package main

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"hexes/dungeon"
	"hexes/game"
	"hexes/skill"
)

// Fog of war. The sim's Sight says which hexes the lamp sees now and
// which have been explored; this file turns that into light. Each floor
// hex is drawn as one quad whose four corners are shaded by the light at
// that point, so the edge of the light is a smooth gradient rather than a
// ring of hex steps. Ground you've explored stays dimly remembered in a
// cool tint; ground you've never seen isn't drawn at all. Enemies, drops
// and status glows outside the light are hidden. Hexes fade in and out of
// line of sight over a few frames so turning a corner doesn't pop.

const (
	fogShadeRate = 8.0 // per second: fade in and out of line of sight
	fogRadRate   = 5.0 // hexes per second: the lamp's reach easing
	fogMem       = 0.2 // how bright remembered ground is
	fogMemVent   = 0.4 // remembered vents glow a little brighter
	fogMemStairs = 0.5 // so do the stairs, so you can find your way back
)

// fogTint colors remembered ground: dim and cool against the warm light.
var fogTint = [3]float32{0.62, 0.72, 1}

// The lamp: a touch warm, and brighter than plain in its core so the
// pool of light reads as light on the dark station floor.
var lampTint = [3]float32{1.04, 1, 0.92}

const lampBoost = 0.35

type fogState struct {
	floor  *dungeon.Floor
	shade  []float32 // per hex: 0 out of sight, 1 in it, eased
	faced  []float32 // per wall hex: the light on the ground it faces, eased
	r      float32   // the lamp's reach as drawn
	lx, ly float64   // the lamp: the player, in pixel space
	meshes [2]mesh   // a floor quad and a wall's grid
	// wall is set while a wall draws: face is how lit the ground it
	// faces is, a cap on the wall's own light.
	wall bool
	face float32
}

// beginFog sets up this frame's light. drawFloor calls it first.
func (g *Game) beginFog() {
	fs := &g.fog
	f := g.sim.Floor
	want := g.sim.LightRadius()
	g.sim.Sight.Update(f, g.sim.P().At, want) // a no-op unless the sim hasn't yet
	if fs.floor != f {
		fs.floor = f
		n := f.W * f.H
		if cap(fs.shade) < n {
			fs.shade = make([]float32, n)
		}
		fs.shade = fs.shade[:n]
		clear(fs.shade)
		if cap(fs.faced) < n {
			fs.faced = make([]float32, n)
		}
		fs.faced = fs.faced[:n]
		clear(fs.faced)
		fs.r = want
	}
	step := float32(fogRadRate / 60)
	switch {
	case fs.r < want:
		fs.r = min(want, fs.r+step)
	case fs.r > want:
		fs.r = max(want, fs.r-step)
	}
	fs.lx, fs.ly = g.sim.P().Pos()
}

// lampReach is the reach to shade with this frame: a browned-out lamp
// flickers.
func (g *Game) lampReach() float32 {
	r := g.fog.r
	if g.sim.Rig.Locked() {
		h := hash32(uint32(g.frame / 3))
		r *= 0.94 + 0.06*float32(h&0xff)/255
	}
	return r
}

func hash32(x uint32) uint32 {
	x ^= x >> 16
	x *= 0x7feb352d
	x ^= x >> 15
	x *= 0x846ca68b
	return x ^ x>>16
}

// drawLitTile draws tile sprite img for hex c (index i, center x, y in
// hex units) with the hex's center at (ox, oy) in the sprite, tinted by
// cr, cg, cb, under the lamp. The sprite lies on the floor: each of its
// four corners goes through the camera (view.go) and is shaded by the
// light at that point. It draws nothing for a hex never seen.
func (g *Game) drawLitTile(dst, img *ebiten.Image, t dungeon.Tile, i int, x, y, ox, oy float64, cr, cg, cb, r float32) {
	fs := &g.fog
	sight := &g.sim.Sight
	vis, seen := sight.Vis[i], sight.Seen[i]
	sh := fs.shade[i]
	if vis {
		sh = min(1, sh+fogShadeRate/60)
	} else {
		sh = max(0, sh-fogShadeRate/60)
	}
	fs.shade[i] = sh
	if fs.wall {
		// Ease what the wall faces too, so stepping round a corner fades
		// it rather than popping it.
		fc := fs.faced[i]
		if fs.face > fc {
			fc = min(fs.face, fc+fogShadeRate/60)
		} else {
			fc = max(fs.face, fc-fogShadeRate/60)
		}
		fs.faced[i], fs.face = fc, fc
	}
	if !seen && sh == 0 {
		return
	}
	var mem float32
	if seen {
		switch t {
		case dungeon.Hazard:
			mem = fogMemVent
		case dungeon.Stairs:
			mem = fogMemStairs
		default:
			// A little darker each floor down, to a floor of its own.
			mem = max(fogMem*0.6, fogMem-0.004*float32(g.sim.Depth))
		}
	}
	b := img.Bounds()
	w, h := float64(b.Dx()), float64(b.Dy())
	far := sh == 0 || math.Hypot(x-fs.lx, y-fs.ly) > (float64(r)+1.5)*math.Sqrt(3)
	ts := g.tileSize
	// A floor tile is one quad. A wall is a fine grid, so the light can
	// fade across it: lit where the lamp reaches, dark a hand's width on.
	nx, ny := 2, 2
	if fs.wall && !far {
		nx, ny = wallGridX, wallGridY
	}
	vs, is := fs.grid(nx, ny)
	lift := g.look.wallLift
	for k := range vs {
		u, v := float64(k%nx)/float64(nx-1), float64(k/nx)/float64(ny-1)
		px0, py0 := u*w, v*h
		wx, wy := x+(px0-ox)/ts, y+(py0-oy)/ts
		var l float32
		if !far {
			ly := wy
			if fs.wall {
				// The sprite's top face is drawn a wall's height up the
				// screen from the hex it stands on: light it where it
				// stands. The faces below the top run down to the base.
				ly = y + (min(py0+lift, h)-oy)/ts
			}
			l = sh * game.LightAt(fs.lx, fs.ly, wx, ly, r)
		}
		if fs.wall {
			// A wall is no brighter than the ground in front of it, and
			// the faces at its foot that turn away from the lamp (it's
			// standing behind them, up the screen) stay in shadow.
			l = min(l, sh*fs.face)
			if front := (py0 - (h - lift - ts*0.5)) / (ts * 0.75); front > 0 && fs.ly < y {
				l *= float32(1 - min(1, front)*min(0.7, 0.47*(y-fs.ly)))
			}
		}
		m := mem * (1 - l)
		l *= 1 + lampBoost*l
		px, py, _ := g.project(wx, wy)
		vx := &vs[k]
		vx.DstX, vx.DstY = float32(px), float32(py)
		vx.SrcX, vx.SrcY = float32(float64(b.Min.X)+px0), float32(float64(b.Min.Y)+py0)
		vx.ColorR = cr * (fogTint[0]*m + lampTint[0]*l)
		vx.ColorG = cg * (fogTint[1]*m + lampTint[1]*l)
		vx.ColorB = cb * (fogTint[2]*m + lampTint[2]*l)
		vx.ColorA = 1
	}
	dst.DrawTriangles(vs, is, img, nil)
}

// The grid a wall is lit on: columns and rows of vertices.
const wallGridX, wallGridY = 5, 9

// grid is a vertex grid nx by ny and its triangles, kept from frame to
// frame (one for floor quads, one for walls).
func (fs *fogState) grid(nx, ny int) ([]ebiten.Vertex, []uint16) {
	m := &fs.meshes[0]
	if nx != 2 || ny != 2 {
		m = &fs.meshes[1]
	}
	if m.nx != nx || m.ny != ny {
		m.nx, m.ny = nx, ny
		m.vs = make([]ebiten.Vertex, nx*ny)
		m.is = m.is[:0]
		for j := 0; j < ny-1; j++ {
			for i := 0; i < nx-1; i++ {
				a := uint16(j*nx + i)
				m.is = append(m.is, a, a+1, a+uint16(nx), a+1, a+uint16(nx)+1, a+uint16(nx))
			}
		}
	}
	return m.vs, m.is
}

// mesh is a lit grid's vertices and triangles.
type mesh struct {
	vs     []ebiten.Vertex
	is     []uint16
	nx, ny int
}

// seeAt is how visible a point in pixel space is: 0 in the dark or
// behind a wall, up to 1 in full light. Hidden things aren't drawn.
func (g *Game) seeAt(x, y float64) float32 {
	fs := &g.fog
	f := g.sim.Floor
	if fs.floor != f {
		return 0
	}
	c := skill.FromPixel(x, y)
	if !f.In(c) {
		return 0
	}
	sh := fs.shade[f.Index(c)]
	if sh == 0 {
		return 0
	}
	return sh * game.LightAt(fs.lx, fs.ly, x, y, g.lampReach())
}

// colLens is a lens on the floor: pale lamp light.
var colLens = color.RGBA{0xff, 0xf1, 0xc4, 0xff}

// drawLenses draws lenses lying on the floor: a pale ring with a bright
// core that throws a little light of its own, so it reads as something
// to do with your lamp. Like any loot it's hidden in the dark.
func (g *Game) drawLenses(screen *ebiten.Image) {
	s := g.size
	t := float64(g.frame) / 60
	for i, d := range g.sim.Stash.Lenses {
		x, y := d.At.Pixel()
		sx, sy := g.toScreen(x, y)
		if sx < -s || sy < -s || sx > float64(g.w)+s || sy > float64(g.h)+s || g.seeAt(x, y) < 0.2 {
			continue
		}
		pop := math.Min(1, float64(d.Age)*4)
		r := s * 0.3 * pop * (1 + 0.1*math.Sin(t*4+float64(i)))
		g.glow(screen, sx, sy, r*3.2, 1, 0.92, 0.7, 0.55)
		vector.StrokeCircle(screen, float32(sx), float32(sy), float32(r), g.u(2.5), colLens, true)
		vector.DrawFilledCircle(screen, float32(sx), float32(sy), float32(r*0.4), colLens, true)
		if d.Points > 1 {
			vector.StrokeCircle(screen, float32(sx), float32(sy), float32(r*1.6), g.u(1.5), fade(colLens, 0.6), true)
		}
		g.label(screen, g.small, "LENS", sx, sy-r*2.6, fade(colLens, 0.9), true)
	}
}

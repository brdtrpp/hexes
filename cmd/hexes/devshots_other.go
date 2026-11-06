//go:build !js

package main

// A headless screenshot autopilot for checking the look without a phone.
// Run the desktop build with HEXES_SHOTS=<dir> (under xvfb-run on a box
// with no display): it wakes every foe, walks toward the nearest, casts
// on a beat, keeps the player alive, saves a PNG every HEXES_EVERY frames
// (default 12) from frame 90, HEXES_COUNT of them (default 24), and quits.
// HEXES_EXAMPLE=<n> loads example build n first. HEXES_FRAMES=editor fits
// a few test frames and opens the editor on the frame rack with one
// picked; HEXES_FRAMES=floor lays a frame of each grade by the player;
// HEXES_FRAMES=buttons puts cast triggers in hexes 2 and 4, so three cast
// buttons show; HEXES_FRAMES=bind does that and opens the editor;
// HEXES_FRAMES=bench stands still beside the start bench.
// HEXES_FRAMES=pad stands hurt on a repair pad (HEXES_DEPTH, default 3).
// HEXES_ZOOM=<z> starts the dungeon view at that zoom.
// HEXES_FRAMES=grid opens the skill grid on a save a few floors in;
// HEXES_FRAMES=locker opens the locker on that save.
// HEXES_FRAMES=hexcard opens the editor on the first hex's card,
// rename starts naming it, loot shows a picked-up part's card.
// HEXES_FRAMES=draft opens a checkpoint's floor-end draft on that save
// (HEXES_DEPTH, default 5); HEXES_FRAMES=kits leaves a fresh save's kit
// pick up. Any other mode takes the first card of every draft.
// Without HEXES_SHOTS it does nothing.

import (
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"strconv"

	"github.com/hajimehoshi/ebiten/v2"

	"hexes/game"
	"hexes/skill"
)

type devState struct {
	dir          string
	every, count int
	taken        int
	ready        bool
	still        bool // stand at the start bench, don't fight
	gallery      bool // throw every kind's effects round the player
}

func envInt(k string, d int) int {
	if v, err := strconv.Atoi(os.Getenv(k)); err == nil {
		return v
	}
	return d
}

// devDraft is the autopilot's pick on a draft: none while a draft or
// kit pick is the thing being shot, the first card otherwise.
func (g *Game) devDraft() (int, bool) {
	g.devInit()
	if g.dev.dir == "" {
		return 0, false
	}
	m := os.Getenv("HEXES_FRAMES")
	return 0, m != "draft" && m != "kits"
}

func (g *Game) devInput(in *game.Input) {
	g.devInit()
	g.devPilot(in)
}

func (g *Game) devInit() {
	d := &g.dev
	if !d.ready {
		d.ready = true
		d.dir = os.Getenv("HEXES_SHOTS")
		d.every, d.count = envInt("HEXES_EVERY", 12), envInt("HEXES_COUNT", 24)
		if d.dir != "" {
			if g.sim.Draft.Kits && os.Getenv("HEXES_FRAMES") != "kits" {
				_ = g.sim.TakeDraft(0) // the first kit, then the mode's setup
			}
			if n := envInt("HEXES_EXAMPLE", -1); n >= 0 {
				_ = g.sim.SetTestShapes(true)
				_ = g.sim.LoadExample(n)
			}
			g.devFrames(os.Getenv("HEXES_FRAMES"))
			m := os.Getenv("HEXES_FRAMES")
			if z, err := strconv.ParseFloat(os.Getenv("HEXES_ZOOM"), 64); err == nil {
				g.zm.z, g.zm.want = clampZoom(z), clampZoom(z)
				g.size = g.zm.size()
			}
			d.still = m == "pad" || m == "bench" || m == "grid" || m == "locker" || m == "draft" || m == "kits"
			d.gallery = m == "kinds" || m == "grades"
		}
	}
}

func (g *Game) devPilot(in *game.Input) {
	d := &g.dev
	if d.gallery {
		g.devGallery(os.Getenv("HEXES_FRAMES"))
		return
	}
	if d.dir == "" || d.still {
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

func (g *Game) devFrames(mode string) {
	s := g.sim
	switch mode {
	case "editor":
		_ = s.SetTestShapes(true)
		set := game.TestFrameSet
		_, _ = s.FitFrame(0, 2, set[2]) // Doubler on the damage
		_, _ = s.FitFrame(0, 5, set[3]) // Kill-lock on the call
		_, _ = s.FitFrame(0, 3, set[8]) // Overflow on the multiplier
		s.Frames = append(s.Frames, set[5], set[6])
		s.Frames[0].ID, s.Frames[1].ID = 11, 13
		s.Frames[0].Grade, s.Frames[1].Grade = 2, 1
		g.openEditor()
		g.edShowRack(true)
		g.ed.pick, g.ed.at = pickFrame, len(s.Frames)+4 // the test Prism
	case "buttons", "bind":
		for _, i := range []int{1, 3} {
			cell := s.Stash.Inv.Add(skill.NewTrigger(skill.OnCast))
			_ = s.SwapIn(i, 0, cell)
		}
		if mode == "bind" {
			g.openEditor()
			g.ed.hex = 1
			g.edBind()
		}
	case "hexcard", "rename":
		// The editor with nothing picked: the hex's own card; rename
		// starts typing a new name for it.
		g.openEditor()
		if mode == "rename" {
			g.renameHex(0)
			g.renameBuf = []rune("Ember Lance")
		}
	case "loot":
		// A rare part just picked up: its card up top.
		p := skill.NewDamage(s.Reg.MustID("plasma"), 22)
		p.Rarity = skill.Rare
		s.LastLoot, s.LootN = p, s.LootN+1
	case "card":
		// The hex card for the first hex.
		if sh, err := s.SlotSheet(0); err == nil {
			g.openSheet(sh)
		}
	case "partcard":
		// A rare part's card, with the next rarity at the bottom.
		p := skill.NewDamage(s.Reg.MustID("plasma"), 22)
		p.Rarity = skill.Rare
		p.Quirk = skill.QuirkRelay
		g.openSheet(game.PartSheet(p, s.Reg))
	case "kits":
		// A fresh save: the kit pick is already up.
	case "grid", "locker", "draft":
		// A save a few floors in: the ladder at frames, Data to spend, a
		// path bought out along one wedge, one cell lost to a death.
		s.Taught, s.Best = 7, 6
		s.Data, s.Unbanked, s.Signals = 400, 23, 2
		bought := 0
		for k := 1; k <= 6 && bought < 12; k++ {
			for i := game.RingStart(k); i < game.RingStart(k+1) && bought < 2*k; i++ {
				if _, err := s.CanBuy(i); err == nil {
					_ = s.Buy(i)
					bought++
				}
			}
		}
		s.Data = 140
		if mode == "draft" {
			s.Kit = 1
			s.DevDraft(envInt("HEXES_DEPTH", 5))
			g.camX, g.camY = s.P().Pos()
			return
		}
		g.openEditor()
		if mode == "grid" {
			g.setGrid(true)
			for i := len(s.Grid.Nodes) - 1; i >= 0; i-- {
				if s.GridDist(i) == 1 && s.CellView(i) == game.ViewCard {
					g.ed.gv.sel = i
					g.ed.msg = ""
					break
				}
			}
		} else {
			g.edShowLocker()
			g.ed.pick, g.ed.at = pickLocker, 0
		}
	case "kinds", "grades":
		// An empty room: the gallery throws its own effects.
		for i := range s.Actors {
			if a := &s.Actors[i]; a.Kind != game.KindPlayer {
				a.U.Alive, a.U.HP = false, 0
			}
		}
	case "pad":
		// Standing hurt on a floor's repair pad, foes cleared.
		s.DevDepth(envInt("HEXES_DEPTH", 3))
		s.DevToPad(0.15, envInt("HEXES_PADOFF", 0))
		g.camX, g.camY = s.P().Pos()
	case "pack":
		// A deeper floor, with gunners, walking into the nearest pack.
		s.DevDepth(envInt("HEXES_DEPTH", 5))
	case "floor":
		p := s.P()
		for i, f := range game.TestFrameSet[:4] {
			f.Grade = skill.Rarity(i)
			s.Stash.Frames = append(s.Stash.Frames, game.FrameDrop{At: p.At.Add(skill.HexDirs[i]).Add(skill.HexDirs[i]), Frame: f, Age: 1})
		}
	}
}

// devGallery throws every damage kind's effects round the player on a
// beat: HEXES_FRAMES=kinds shows the six kinds at HEXES_GRADE (default
// 2); HEXES_FRAMES=grades shows kind HEXES_TYPE at grades 0 to 3. The
// delivery (bolt, around, cone, chain) turns over every 45 frames.
func (g *Game) devGallery(mode string) {
	s := g.sim
	p := s.P()
	p.U.HP = p.U.MaxHP
	// Throw them HEXES_LAG frames (default 10) before each shot, so a
	// shot every 45 frames catches each round mid-flight.
	if g.frame%45 != (45-envInt("HEXES_LAG", 10))%45 {
		return
	}
	px, py := p.Pos()
	phase := (g.frame/45 + 1) % 4
	if v := envInt("HEXES_PHASE", -1); v >= 0 {
		phase = v
	}
	n := 6
	if mode == "grades" {
		n = 4
	}
	for i := 0; i < n; i++ {
		t := skill.TypeID(i)
		grade := uint8(envInt("HEXES_GRADE", 2))
		if mode == "grades" {
			t, grade = skill.TypeID(envInt("HEXES_TYPE", 0)), uint8(i)
		}
		an := float64(i)*2*math.Pi/float64(n) - math.Pi/2 + 0.3
		ca, sa := math.Cos(an), math.Sin(an)
		x, y := px+ca*3.4, py+sa*3.4
		mag := float32(0.6 + 0.35*float64(grade))
		e := game.FX{Type: t, Grade: grade, Mag: mag, Power: 8 + 6*float32(grade), Life: 0.55}
		switch phase {
		case 0:
			e.Kind, e.Life = game.FXBolt, 0.5
			e.X, e.Y, e.FromX, e.FromY = x, y, px+ca*0.6, py+sa*0.6
		case 1:
			e.Kind, e.X, e.Y, e.Value = game.FXWave, x, y, 1
		case 2:
			e.Kind, e.X, e.Y, e.Value = game.FXCone, px+ca*1.2, py+sa*1.2, 3
			e.DX, e.DY = float32(ca), float32(sa)
		case 3:
			e.Kind, e.NPts = game.FXChain, 4
			for j := 0; j < 4; j++ {
				r := 1.2 + 1.6*float64(j)
				w := 0.5 * float64(j%2*2-1)
				e.Pts[j] = [2]float32{float32(px + math.Cos(an+w*0.2)*r), float32(py + math.Sin(an+w*0.2)*r)}
			}
		}
		s.FX.Add(e)
	}
}

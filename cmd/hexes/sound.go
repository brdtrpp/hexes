package main

// Sound. Everything you hear is synthesized while it plays (package sfx)
// from the same records the effects draw from, so a hit sounds like its
// damage type and as hard as it landed, a cast climbs with the Power it
// pulled and hisses with the Heat it made. One Ebitengine player pulls
// from one mixer with a fixed voice cap, so a big fight costs no more
// than a small one.
//
// Browsers only start audio after a tap or key press; Ebitengine resumes
// it on the first one, and nothing is queued before then. The sound
// button (top bar) mutes; the menu has the volume. Both are saved.

import (
	"image/color"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"hexes/game"
	"hexes/sfx"
)

// Settings keys: volume 0 to 100, and "1" when muted.
const (
	volumePref = "hexes.volume"
	mutePref   = "hexes.mute"
)

// defaultVolume is the volume before the player picks one.
const defaultVolume = 80

type soundState struct {
	ctx    *audio.Context
	player *audio.Player
	mix    *sfx.Mixer
	dir    *sfx.Director
	vol    int // 0 to 100
	muted  bool
	depth  int // the floor last heard, to cue the stairs
}

func (g *Game) initSound() {
	s := &g.snd
	s.vol = defaultVolume
	if v, err := strconv.Atoi(loadPref(volumePref)); err == nil {
		s.vol = min(max(v, 0), 100)
	}
	s.muted = loadPref(mutePref) == "1"
	s.depth = g.sim.Depth
	s.mix = sfx.NewMixer()
	s.dir = sfx.NewDirector(g.sim.Reg, s.mix)
	if os.Getenv("HEXES_NOSOUND") != "" { // desktop with no audio device
		return
	}
	s.ctx = audio.NewContext(sfx.Rate)
	p, err := s.ctx.NewPlayerF32(s.mix)
	if err != nil {
		log.Printf("hexes: no sound: %v", err)
		return
	}
	// Short enough that a hit sounds with its flash, long enough that a
	// slow phone frame doesn't crackle.
	p.SetBufferSize(80 * time.Millisecond)
	s.player = p
	s.apply()
}

// on reports whether sound should play at all.
func (s *soundState) on() bool { return s.player != nil && !s.muted && s.vol > 0 }

// apply pushes the volume and mute to the player. Muted, the player
// pauses, so the synth costs nothing.
func (s *soundState) apply() {
	if s.player == nil {
		return
	}
	v := float64(s.vol) / 100
	s.player.SetVolume(v * v) // ears hear level roughly by its square
	if s.on() {
		if !s.player.IsPlaying() {
			s.player.Play()
		}
		return
	}
	s.player.Pause()
	s.mix.Stop()
}

func (s *soundState) save() {
	savePref(volumePref, strconv.Itoa(s.vol))
	m := "0"
	if s.muted {
		m = "1"
	}
	savePref(mutePref, m)
}

// toggleMute flips mute (unmuting at zero volume brings it back up).
func (g *Game) toggleMute() {
	s := &g.snd
	s.muted = !s.muted
	if !s.muted && s.vol == 0 {
		s.vol = defaultVolume
	}
	s.apply()
	s.save()
	g.click(true)
}

// nudgeVolume steps the volume by d (out of 100).
func (g *Game) nudgeVolume(d int) {
	s := &g.snd
	s.vol = min(max(s.vol+d, 0), 100)
	if d > 0 {
		s.muted = false
	}
	s.apply()
	s.save()
	g.click(d > 0)
}

// click is a UI tap.
func (g *Game) click(on bool) {
	if s := &g.snd; s.on() && s.ctx.IsReady() {
		s.dir.UI(on)
	}
}

// updateSound plays every effect the sim recorded since last step. It
// runs before updateFX, which marks effects as seen.
func (g *Game) updateSound(dt float32) {
	s := &g.snd
	depth := g.sim.Depth
	newFloor := depth > s.depth
	s.depth = depth
	if !s.on() || !s.ctx.IsReady() {
		return
	}
	s.dir.Step(dt)
	if newFloor {
		s.dir.NewFloor()
	}
	lx, ly := g.sim.P().Pos()
	g.sim.FX.Each(func(e *game.FX) {
		if !e.Seen {
			s.dir.Hear(e, lx, ly)
		}
	})
}

// drawSoundButton draws the mute toggle: a speaker, with waves when on
// and a cross when muted.
func (g *Game) drawSoundButton(screen *ebiten.Image) {
	m := g.sndBtn
	u := g.u
	c := colDim
	if g.snd.on() {
		c = colText
	}
	vector.DrawFilledRect(screen, float32(m.x), float32(m.y), float32(m.w), float32(m.h), colPanel, false)
	vector.StrokeRect(screen, float32(m.x), float32(m.y), float32(m.w), float32(m.h), u(1.5), colDim, false)
	cx, cy := float32(m.x+m.w*0.4), float32(m.y+m.h/2)
	s := float32(m.h) * 0.22
	var p vector.Path
	p.MoveTo(cx-s*1.6, cy-s*0.55)
	p.LineTo(cx-s*0.6, cy-s*0.55)
	p.LineTo(cx+s*0.5, cy-s*1.3)
	p.LineTo(cx+s*0.5, cy+s*1.3)
	p.LineTo(cx-s*0.6, cy+s*0.55)
	p.LineTo(cx-s*1.6, cy+s*0.55)
	p.Close()
	vs, is := p.AppendVerticesAndIndicesForFilling(nil, nil)
	r, gg, b := colorF(c)
	for i := range vs {
		vs[i].SrcX, vs[i].SrcY = 1, 1
		vs[i].ColorR, vs[i].ColorG, vs[i].ColorB, vs[i].ColorA = r, gg, b, 1
	}
	screen.DrawTriangles(vs, is, whiteSub(), &ebiten.DrawTrianglesOptions{AntiAlias: true})
	x := cx + s*1.2
	if g.snd.on() {
		vector.StrokeLine(screen, x, cy-s*0.6, x+s*0.3, cy, u(1.5), c, true)
		vector.StrokeLine(screen, x+s*0.3, cy, x, cy+s*0.6, u(1.5), c, true)
		vector.StrokeLine(screen, x+s*0.8, cy-s*1.1, x+s*1.3, cy, u(1.5), c, true)
		vector.StrokeLine(screen, x+s*1.3, cy, x+s*0.8, cy+s*1.1, u(1.5), c, true)
	} else {
		red := color.RGBA{0xe5, 0x48, 0x4d, 0xff}
		vector.StrokeLine(screen, x, cy-s*0.7, x+s*1.4, cy+s*0.7, u(1.8), red, true)
		vector.StrokeLine(screen, x, cy+s*0.7, x+s*1.4, cy-s*0.7, u(1.8), red, true)
	}
}

package sfx

import (
	"math"

	"hexes/game"
	"hexes/skill"
)

// Director turns the sim's effect records into sounds. It keeps the mix
// readable in a big fight: the same sound can't retrigger faster than a
// short gap, a frame plays only a few new sounds, far-off effects are
// quiet or dropped, and hits inside an area lean on the area's sound.
type Director struct {
	M     *Mixer
	tm    [skill.MaxTypes]Timbre
	ntm   int
	now   float32
	last  [lastSlots]float32
	frame int // sounds started this step
	rng   uint32
}

// MaxPerStep is how many new sounds one game step can start.
const MaxPerStep = 5

// gaps between retriggers of the same kind of sound, in seconds.
const (
	gapHit    = 0.04
	gapImpact = 0.07
	gapTick   = 0.07
	gapOther  = 0.06
)

// one retrigger clock per FX kind and damage type.
const lastSlots = 32 * skill.MaxTypes

// NewDirector works out every damage type's timbre from reg.
func NewDirector(reg *skill.Registry, m *Mixer) *Director {
	d := &Director{M: m, rng: 0x2545f491}
	d.ntm = min(reg.Len(), skill.MaxTypes)
	for i := 0; i < d.ntm; i++ {
		d.tm[i] = TimbreOf(*reg.Get(skill.TypeID(i)))
	}
	for i := range d.last {
		d.last[i] = -1
	}
	return d
}

// Timbre is the sound of damage type t.
func (d *Director) Timbre(t skill.TypeID) Timbre {
	if int(t) < d.ntm {
		return d.tm[t]
	}
	return Neutral
}

// Step moves the director's clock on; call it once per game step.
func (d *Director) Step(dt float32) {
	d.now += dt
	d.frame = 0
}

// detune is a small random pitch wobble so repeats don't sound stamped.
func (d *Director) detune() float32 {
	d.rng ^= d.rng << 13
	d.rng ^= d.rng >> 17
	d.rng ^= d.rng << 5
	return 0.97 + 0.06*float32(d.rng%1000)/1000
}

// ready reports whether a sound keyed by kind and type may play now, and
// claims the slot if so.
func (d *Director) ready(kind game.FXKind, t skill.TypeID, gap float32) bool {
	if d.frame >= MaxPerStep {
		return false
	}
	k := (int(kind)%32)*skill.MaxTypes + int(t)%skill.MaxTypes
	if d.last[k] >= 0 && d.now-d.last[k] < gap {
		return false
	}
	d.last[k] = d.now
	return true
}

// Play starts s panned to x relative to the listener at lx, ly (hex
// units), quieter with distance.
func (d *Director) play(s Sound, x, y, lx, ly float64) {
	dx, dy := x-lx, y-ly
	dist := math.Hypot(dx, dy)
	att := float32(1 / (1 + max(0, dist-6)/6))
	if att < 0.2 {
		return
	}
	s.Gain *= att
	s.Pan = float32(min(max(dx/10, -1), 1)) * 0.7
	d.M.Play(s)
	d.frame++
}

// UI plays a tap; on marks a toggle switched on.
func (d *Director) UI(on bool) { d.M.Play(Click(on)) }

// NewFloor plays the trip down the stairs.
func (d *Director) NewFloor() { d.M.Play(Floor()) }

// Hear plays e if it makes a sound, heard from lx, ly.
func (d *Director) Hear(e *game.FX, lx, ly float64) {
	x, y := e.X, e.Y
	switch e.Kind {
	case game.FXBolt:
		gap := float32(gapHit)
		if e.Impact {
			gap = gapImpact
		}
		if d.ready(e.Kind, e.Type, gap) {
			d.play(Hit(d.Timbre(e.Type), e.Mag, e.Impact, d.detune()), x, y, lx, ly)
		}
	case game.FXWave, game.FXCone:
		if d.ready(e.Kind, e.Type, gapOther) {
			d.play(Area(d.Timbre(e.Type), e.Kind == game.FXCone, e.Value, e.Mag, d.detune()), x, y, lx, ly)
		}
	case game.FXChain:
		if !d.ready(e.Kind, e.Type, gapOther) {
			return
		}
		tm := d.Timbre(e.Type)
		f := d.frame // the hops share one slot of the step
		for i := 1; i < int(e.NPts) && i <= 6; i++ {
			p := e.Pts[i]
			d.play(ChainHop(tm, i-1, d.detune()), float64(p[0]), float64(p[1]), lx, ly)
		}
		d.frame = f + 1
	case game.FXRing:
		if d.ready(e.Kind, skill.TypeID(e.Slot), gapOther) {
			d.play(Cast(e.Power, e.Heat), x, y, lx, ly)
		}
	case game.FXVent:
		if d.ready(e.Kind, 0, 0.3) {
			d.play(Vent(), x, y, lx, ly)
		}
	case game.FXNumber:
		switch {
		case e.Small && e.Hurt:
			if d.ready(e.Kind, 15, gapTick*2) {
				s := Hurt(0.2)
				s.Gain *= 0.35
				d.play(s, x, y, lx, ly)
			}
		case e.Small:
			if d.ready(e.Kind, 14, gapTick) { // ticks share one clock
				d.play(Tick(d.Timbre(e.Type), d.detune()), x, y, lx, ly)
			}
		case e.Hurt:
			if d.ready(e.Kind, 13, gapOther) {
				d.play(Hurt(e.Mag), x, y, lx, ly)
			}
		}
	case game.FXKill:
		player := e.Slot == int(game.KindPlayer)
		if d.ready(e.Kind, e.Type, gapOther) || player {
			d.play(Kill(d.Timbre(e.Type), player), x, y, lx, ly)
		}
	case game.FXClaw:
		if d.ready(e.Kind, 0, gapOther) {
			d.play(Claw(), x, y, lx, ly)
		}
	case game.FXPush:
		if d.ready(e.Kind, 0, gapOther) {
			d.play(Push(), x, y, lx, ly)
		}
	case game.FXSlam:
		if d.ready(e.Kind, 0, gapOther) {
			d.play(Slam(), x, y, lx, ly)
		}
	case game.FXFall:
		if d.ready(e.Kind, 0, gapOther) {
			d.play(Fall(), x, y, lx, ly)
		}
	case game.FXLoot:
		if d.ready(e.Kind, 0, gapOther) {
			d.play(Loot(e.Slot), x, y, lx, ly)
		}
	case game.FXFull:
		if d.ready(e.Kind, 0, 0.5) {
			d.play(Full(), x, y, lx, ly)
		}
	case game.FXSalvage:
		d.play(Salvage(), x, y, lx, ly)
	case game.FXShape, game.FXTile, game.FXFrame:
		if d.ready(e.Kind, 0, gapOther) {
			d.play(Learn(), x, y, lx, ly)
		}
	}
}

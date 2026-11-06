package sfx

import (
	"math"

	"hexes/fx"
	"hexes/skill"
)

// Timbre is a damage type's sound, worked out once from its JSON row. It
// reuses the effects' motion signature (fx.SigOf), so a type that looks
// hot also sounds hot.
type Timbre struct {
	Sig fx.Sig
	// Base is the type's body pitch in Hz.
	Base float32
	// Ticks speeds a swarm's buzz; Splash adds an echo off the target.
	Ticks  float32
	Splash float32
	// Norm evens out loudness: a quiet-natured type (a breath, a chirp)
	// plays as loud as a thump. It's measured, not set by hand.
	Norm float32
}

// pentatonic degrees keep every type's pitch in one key.
var pentatonic = [5]float32{0, 2, 4, 7, 9}

// TimbreOf works out a type's sound from its numbers. Cold and markers
// sit high, force and heat sit low, and the name picks a note of the
// scale so two similar types still sound apart.
func TimbreOf(t skill.DamageType) Timbre {
	s := fx.SigOf(t, 0, 0, 0)
	var h uint32 = 2166136261
	for i := 0; i < len(t.Name); i++ {
		h = (h ^ uint32(t.Name[i])) * 16777619
	}
	semis := 7*s.Cold - 5*s.Hot - 9*s.Force + 12*s.Mark + 4*s.Swarm + 2*s.Veil + pentatonic[h%5]
	tm := Timbre{
		Sig:    s,
		Base:   220 * float32(math.Pow(2, float64(semis)/12)),
		Ticks:  float32(t.Ticks),
		Splash: s.Splash,
		Norm:   1,
	}
	tm.Norm = normOf(tm)
	return tm
}

// normTarget is the loudness every type's plain hit is evened to.
const normTarget = 0.06

// normOf renders a quarter second of a plain hit offline and works out
// the gain that brings it to normTarget. Loudness is the RMS after a
// gentle highpass, since ears care less about deep bass than meters do.
func normOf(tm Timbre) float32 {
	m := &Mixer{rng: 1, Gain: 1}
	m.Play(Hit(tm, 1, false, 1))
	buf := make([]float32, 2*Rate/4)
	for i := range m.v { // no limiter: measure the raw sum
		if m.v[i].on {
			m.v[i].render(buf)
		}
	}
	a := float32(math.Exp(-2 * math.Pi * 150 / Rate))
	var y, px float32
	var sum float64
	for i := 0; i < len(buf); i += 2 {
		x := buf[i] + buf[i+1]
		y = a * (y + x - px)
		px = x
		sum += float64(y * y)
	}
	rms := math.Sqrt(sum / float64(len(buf)/2))
	if rms <= 0 {
		return 1
	}
	return float32(min(max(normTarget/rms, 0.5), 4))
}

// Neutral is the timbre for effects with no damage type.
var Neutral = Timbre{Sig: fx.Neutral, Base: 330, Norm: 1}

// facet is one side of a timbre with its weight.
type facet struct {
	f fx.Facet
	w float32
}

// facets lists the timbre's facets, strongest first, dropping faint ones.
// A type no facet claims gets a plain body.
func (tm Timbre) facets() (out [4]facet, n int) {
	s := tm.Sig
	all := [6]facet{{fx.FacetHot, s.Hot}, {fx.FacetCold, s.Cold}, {fx.FacetForce, s.Force},
		{fx.FacetSwarm, s.Swarm}, {fx.FacetMark, s.Mark}, {fx.FacetVeil, s.Veil}}
	for i := 1; i < len(all); i++ { // insertion sort, strongest first
		for j := i; j > 0 && all[j].w > all[j-1].w; j-- {
			all[j], all[j-1] = all[j-1], all[j]
		}
	}
	for _, f := range all {
		if f.w >= 0.12 && n < 3 {
			out[n] = f
			n++
		}
	}
	if p := s.Plainness(); p > 0.2 || n == 0 {
		if n == 3 {
			n--
		}
		out[n] = facet{fx.FacetPlain, max(p, 0.5)}
		n++
	}
	return
}

// lead is the timbre's strongest facet.
func (tm Timbre) lead() facet {
	f, _ := tm.facets()
	return f[0]
}

// voiceOf is the layers one facet adds to a hit. f is the pitch, m the
// square root of the magnitude, w the facet's weight.
func voiceOf(fc fx.Facet, tm Timbre, f, m, w float32, s *Sound) {
	switch fc {
	case fx.FacetHot: // a crackling roar
		s.Add(Layer{Wave: Noise, Attack: 0.002, Decay: 0.22 + 0.12*m, LP0: 4500, LP1: 700, Res: 0.2, Crackle: 0.6, Gain: 0.55 * w})
		s.Add(Layer{Wave: Saw, F0: f * 0.65, F1: f * 0.5, Glide: 0.08, Attack: 0.004, Decay: 0.18 + 0.1*m,
			LP0: 2000, LP1: 300, Vib: 0.03 + 0.2*tm.Sig.Jag(), VibRate: 13, Gain: 0.35 * w})
	case fx.FacetCold: // a glassy bell and a glint of shatter
		s.Add(Layer{Wave: FM, F0: f * 2.02, F1: f * 2, Glide: 0.05, FMRatio: 3.51, FMIndex: 3, Attack: 0.001, Decay: 0.45 + 0.2*m, Gain: 0.4 * w})
		s.Add(Layer{Wave: Noise, Attack: 0.001, Decay: 0.06, HP: 5000, Gain: 0.3 * w})
	case fx.FacetForce: // a thump that drops in pitch, and a dull crack
		s.Add(Layer{Wave: Sine, F0: 150 + 40*m, F1: 42, Glide: 0.035, Attack: 0.001, Decay: 0.16 + 0.1*m, Gain: 0.9 * w})
		s.Add(Layer{Wave: Noise, Attack: 0.001, Decay: 0.07, LP0: 900, LP1: 200, Gain: 0.45 * w})
	case fx.FacetSwarm: // a buzz that flutters faster the more ticks it has
		s.Add(Layer{Wave: Square, F0: f * 1.5, F1: f * 1.45, Glide: 0.2, LP0: 1600, LP1: 900, Trem: 0.8, TremRate: 22 + 2*tm.Ticks,
			Vib: 0.02, VibRate: 7, Attack: 0.01, Decay: 0.3 + 0.1*m, Gain: 0.28 * w})
	case fx.FacetMark: // a two-note lock-on chirp
		s.Add(Layer{Wave: Square, F0: f * 4, Attack: 0.001, Decay: 0.05, LP0: 3500, LP1: 3500, Gain: 0.25 * w})
		s.Add(Layer{Wave: Square, F0: f * 4 * 1.335, Delay: 0.065, Attack: 0.001, Decay: 0.06, LP0: 3500, LP1: 3500, Gain: 0.25 * w})
	case fx.FacetVeil: // a soft breath that opens up
		s.Add(Layer{Wave: Noise, Attack: 0.05, Decay: 0.3, LP0: 800, LP1: 2600, HP: 400, Gain: 0.3 * w})
	default: // plain: a short tonal blip
		s.Add(Layer{Wave: Tri, F0: f * 1.6, F1: f, Glide: 0.04, Attack: 0.002, Decay: 0.1 + 0.05*m, Gain: 0.45 * w})
	}
}

// Hit is a hex landing on a target. mag is fx.MagOf's 0.15 to 3; impact
// marks a hit inside an area, which the area sound mostly covers.
func Hit(tm Timbre, mag float32, impact bool, detune float32) Sound {
	var s Sound
	m := float32(math.Sqrt(float64(max(mag, 0))))
	f := tm.Base * detune / (1 + 0.1*max(mag-1, 0))
	s.Add(Layer{Wave: Noise, Attack: 0, Decay: 0.012 + 0.012*tm.Sig.Force, HP: 2500, Gain: 0.3}) // the click of contact
	fs, n := tm.facets()
	for i := 0; i < n; i++ {
		voiceOf(fs[i].f, tm, f, m, fs[i].w, &s)
	}
	if tm.Splash > 0.1 && !impact && s.N > 1 { // an echo off the target
		e := s.L[1]
		e.Delay += 0.07
		e.Gain *= 0.45 * tm.Splash
		e.Pan = 0.4
		s.Add(e)
	}
	if mag >= 1.6 { // a big hit gets a low boom under it
		s.Add(Layer{Wave: Sine, F0: 90, F1: 32, Glide: 0.12, Attack: 0.002, Decay: 0.45, Gain: 0.6 * min((mag-1.2)/1.8, 1)})
	}
	s.Gain = (0.35 + 0.4*m) * tm.Norm
	if impact {
		s.Gain *= 0.55
		for i := range s.L[:s.N] {
			s.L[i].Decay *= 0.7
		}
	}
	return s
}

// Area is a hex touching a round area (wave) or a cone, size hexes
// across: a whoosh that grows with the area, under the type's lead voice
// stretched to fit.
func Area(tm Timbre, cone bool, size, mag float32, detune float32) Sound {
	var s Sound
	m := float32(math.Sqrt(float64(max(mag, 0.3))))
	if cone {
		s.Add(Layer{Wave: Noise, Attack: 0.01, Decay: 0.22 + 0.03*size, HP: 800, LP0: 1200, LP1: 5000, Gain: 0.45})
	} else {
		s.Add(Layer{Wave: Noise, Attack: 0.015, Decay: 0.3 + 0.07*size, LP0: 300, LP1: 1800 + 500*min(size, 4), Res: 0.3, Gain: 0.5})
	}
	k := s.N
	voiceOf(tm.lead().f, tm, tm.Base*detune*0.75, m, 0.9, &s)
	for i := k; i < s.N; i++ {
		s.L[i].Decay *= 1 + 0.2*min(size, 5)
	}
	s.Gain = (0.4 + 0.3*m) * min(tm.Norm, 2)
	return s
}

// ChainHop is one hop of a chain, i of them in: a short zap that climbs
// as the chain goes on.
func ChainHop(tm Timbre, i int, detune float32) Sound {
	var s Sound
	f := tm.Base * detune * 2 * (1 + 0.07*float32(i))
	s.Add(Layer{Wave: Saw, F0: f, F1: f * 0.6, Glide: 0.03, Attack: 0.001, Decay: 0.09, LP0: 3000, LP1: 800, Gain: 0.35})
	s.Add(Layer{Wave: Noise, Attack: 0, Decay: 0.015, HP: 2500, Gain: 0.2})
	s.Delay = 0.045 * float32(i)
	s.Gain = 0.8
	return s
}

// Cast is a hex firing: Power pulled in rises in pitch (more Power, a
// longer, higher climb) and Heat vents as hiss (or a cold sigh when the
// hex vents Heat). A hex that cost nothing gets a small blip.
func Cast(power, heat float32) Sound {
	var s Sound
	p := min(max(power, 0), 40)
	if power <= 0 && heat == 0 {
		s.Add(Layer{Wave: Tri, F0: 660, F1: 880, Glide: 0.02, Attack: 0.002, Decay: 0.06, Gain: 0.15})
		s.Gain = 1
		return s
	}
	if power > 0 {
		s.Add(Layer{Wave: FM, F0: 160, F1: 160 * (1.8 + p/25), Glide: 0.06 + p/400, FMRatio: 2, FMIndex: 1.2,
			Attack: 0.02, Decay: 0.18 + p/200, Gain: 0.2 + p/100})
	}
	switch h := min(max(heat, -30), 40); {
	case h > 0:
		s.Add(Layer{Wave: Noise, Attack: 0.01, Decay: 0.15 + h/80, HP: 2500, LP0: 7000, LP1: 3000, Crackle: 0.3, Gain: 0.12 + h/120})
	case h < 0:
		s.Add(Layer{Wave: Noise, Attack: 0.03, Decay: 0.35, LP0: 1500, LP1: 500, Gain: 0.15 - h/150})
	}
	s.Gain = 1
	return s
}

// Vent is the rig dumping its Heat: a long falling hiss and a pressure
// drop.
func Vent() Sound {
	var s Sound
	s.Add(Layer{Wave: Noise, Attack: 0.02, Decay: 0.8, HP: 1800, LP0: 7500, LP1: 1800, Crackle: 0.25, Gain: 0.5})
	s.Add(Layer{Wave: Sine, F0: 200, F1: 90, Glide: 0.3, Attack: 0.01, Decay: 0.5, Gain: 0.15})
	s.Gain = 1
	return s
}

// Tick is one tick of a damage-over-time status, in the type's lead
// voice but tiny.
func Tick(tm Timbre, detune float32) Sound {
	var s Sound
	f := tm.Base * detune
	switch tm.lead().f {
	case fx.FacetHot:
		s.Add(Layer{Wave: Noise, Attack: 0, Decay: 0.035, LP0: 5000, LP1: 1500, Crackle: 0.8, Gain: 0.5})
	case fx.FacetSwarm:
		s.Add(Layer{Wave: Square, F0: f * 2, Attack: 0.002, Decay: 0.045, LP0: 2000, LP1: 1200, Trem: 0.7, TremRate: 30 + 2*tm.Ticks, Gain: 0.3})
	case fx.FacetCold:
		s.Add(Layer{Wave: FM, F0: f * 3, FMRatio: 3.51, FMIndex: 1.5, Attack: 0.001, Decay: 0.06, Gain: 0.25})
	default:
		s.Add(Layer{Wave: Tri, F0: f * 2, Attack: 0.001, Decay: 0.03, Gain: 0.3})
	}
	s.Gain = 0.4 * min(tm.Norm, 2)
	return s
}

// Kill is something dying: a crunch and a falling tone. The player's own
// death is longer and lower.
func Kill(tm Timbre, player bool) Sound {
	var s Sound
	if player {
		s.Add(Layer{Wave: Sine, F0: 300, F1: 40, Glide: 0.4, Attack: 0.005, Decay: 1.2, Gain: 0.6})
		s.Add(Layer{Wave: Noise, Attack: 0.005, Decay: 0.9, LP0: 2500, LP1: 150, Crackle: 0.4, Gain: 0.5})
		s.Gain = 1
		return s
	}
	s.Add(Layer{Wave: Noise, Attack: 0.002, Decay: 0.3, LP0: 3000, LP1: 200, Gain: 0.4})
	s.Add(Layer{Wave: Saw, F0: tm.Base * 0.8, F1: tm.Base * 0.2, Glide: 0.15, Attack: 0.003, Decay: 0.3, LP0: 1500, LP1: 300, Gain: 0.3})
	s.Gain = 0.9
	return s
}

// Hurt is the player taking a hit: a low thud with a sour pair of tones.
func Hurt(mag float32) Sound {
	var s Sound
	s.Add(Layer{Wave: Sine, F0: 110, F1: 55, Glide: 0.05, Attack: 0.001, Decay: 0.15, Gain: 0.55})
	s.Add(Layer{Wave: Square, F0: 233, Attack: 0.002, Decay: 0.12, LP0: 1200, LP1: 500, Gain: 0.12})
	s.Add(Layer{Wave: Square, F0: 247, Attack: 0.002, Decay: 0.12, LP0: 1200, LP1: 500, Gain: 0.12})
	s.Gain = 0.6 + 0.3*min(mag, 1)
	return s
}

// Claw is an enemy's melee swipe.
func Claw() Sound {
	var s Sound
	s.Add(Layer{Wave: Noise, Attack: 0.004, Decay: 0.09, LP0: 3000, LP1: 700, HP: 400, Gain: 0.35})
	s.Gain = 1
	return s
}

// Push is something shoved across the floor.
func Push() Sound {
	var s Sound
	s.Add(Layer{Wave: Noise, Attack: 0.02, Decay: 0.15, LP0: 600, LP1: 2000, Gain: 0.4})
	s.Gain = 1
	return s
}

// Slam is something shoved into a wall.
func Slam() Sound {
	var s Sound
	s.Add(Layer{Wave: Sine, F0: 120, F1: 40, Glide: 0.04, Attack: 0.001, Decay: 0.2, Gain: 0.7})
	s.Add(Layer{Wave: Noise, Attack: 0.001, Decay: 0.12, LP0: 1500, LP1: 300, Crackle: 0.3, Gain: 0.4})
	s.Gain = 1
	return s
}

// Fall is something dropping into a pit: a falling whistle.
func Fall() Sound {
	var s Sound
	s.Add(Layer{Wave: Sine, F0: 900, F1: 110, Glide: 0.35, Attack: 0.01, Decay: 0.7, Vib: 0.01, VibRate: 6, Gain: 0.3})
	s.Gain = 1
	return s
}

// notes is a run of rising notes, the higher the rarer.
func notes(base float32, n int, wave Wave, gap, decay, gain float32) Sound {
	var s Sound
	for i := 0; i < n && i < MaxLayers; i++ {
		f := base * float32(math.Pow(2, float64(pentatonic[i%5]+12*float32(i/5))/12))
		s.Add(Layer{Wave: wave, F0: f, FMRatio: 2, FMIndex: 1, Delay: gap * float32(i), Attack: 0.002, Decay: decay, Gain: gain})
	}
	s.Gain = 1
	return s
}

// Loot is a part picked up; rarity 0 (common) and up climbs higher and
// plays more notes.
func Loot(rarity int) Sound {
	r := min(max(rarity, 0), 4)
	return notes(660*float32(math.Pow(2, float64(2*r)/12)), 2+r/2, Tri, 0.06, 0.16, 0.22)
}

// Full is a part left on the floor because the pack is full.
func Full() Sound {
	var s Sound
	s.Add(Layer{Wave: Square, F0: 150, Attack: 0.002, Decay: 0.08, LP0: 900, LP1: 900, Gain: 0.2})
	s.Add(Layer{Wave: Square, F0: 150, Delay: 0.11, Attack: 0.002, Decay: 0.08, LP0: 900, LP1: 900, Gain: 0.2})
	s.Gain = 1
	return s
}

// Salvage is the salvage cache recovered.
func Salvage() Sound { return notes(440, 4, FM, 0.08, 0.4, 0.22) }

// Learn is a new shape or logic tile.
func Learn() Sound {
	var s Sound
	s.Add(Layer{Wave: FM, F0: 880, FMRatio: 3.51, FMIndex: 2, Attack: 0.002, Decay: 0.8, Gain: 0.25})
	s.Add(Layer{Wave: FM, F0: 1320, FMRatio: 3.51, FMIndex: 2, Delay: 0.1, Attack: 0.002, Decay: 0.9, Gain: 0.22})
	s.Gain = 1
	return s
}

// Floor is going down the stairs to a new floor.
func Floor() Sound {
	var s Sound
	s.Add(Layer{Wave: Noise, Attack: 0.05, Decay: 0.9, LP0: 4000, LP1: 300, Gain: 0.3})
	s.Add(Layer{Wave: Sine, F0: 400, F1: 100, Glide: 0.3, Attack: 0.02, Decay: 0.8, Gain: 0.25})
	s.Gain = 1
	return s
}

// Click is a UI tap; on picks a higher pair (a toggle switched on).
func Click(on bool) Sound {
	var s Sound
	f := float32(900)
	if on {
		f = 1300
	}
	s.Add(Layer{Wave: Tri, F0: f * 1.3, F1: f, Glide: 0.01, Attack: 0.001, Decay: 0.03, Gain: 0.18})
	s.Gain = 1
	return s
}

// Mend is hull knitting back: a soft rising pair of glassy notes over a
// shimmer of nanite chatter, brighter the more it restored.
func Mend(mag float32) Sound {
	var s Sound
	m := float32(math.Sqrt(float64(min(max(mag, 0.15), 3))))
	s.Add(Layer{Wave: FM, F0: 523, F1: 659, Glide: 0.08, FMRatio: 2, FMIndex: 0.6, Attack: 0.02, Decay: 0.45, Gain: 0.18 + 0.06*m})
	s.Add(Layer{Wave: FM, F0: 784, FMRatio: 2, FMIndex: 0.5, Delay: 0.09, Attack: 0.02, Decay: 0.55, Gain: 0.14 + 0.05*m})
	s.Add(Layer{Wave: Noise, Attack: 0.05, Decay: 0.35, HP: 4000, LP0: 7000, LP1: 5000, Trem: 0.8, TremRate: 28, Gain: 0.08})
	s.Gain = 1
	return s
}

// Shield is a field snapping on: a quick upward hum that locks into a
// steady fifth.
func Shield(mag float32) Sound {
	var s Sound
	m := float32(math.Sqrt(float64(min(max(mag, 0.15), 3))))
	s.Add(Layer{Wave: Saw, F0: 110, F1: 220, Glide: 0.05, Attack: 0.005, Decay: 0.4, LP0: 600, LP1: 1800, Res: 0.5, Gain: 0.2 + 0.05*m})
	s.Add(Layer{Wave: Sine, F0: 330, Attack: 0.03, Decay: 0.5, Trem: 0.3, TremRate: 12, Gain: 0.16})
	s.Add(Layer{Wave: Noise, Attack: 0.001, Decay: 0.03, HP: 3000, Gain: 0.15})
	s.Gain = 1
	return s
}

// Soak is a hit breaking on a shield: a hollow ring instead of a thud.
func Soak(mag float32) Sound {
	var s Sound
	s.Add(Layer{Wave: FM, F0: 440, F1: 330, Glide: 0.06, FMRatio: 1.41, FMIndex: 1.6, Attack: 0.001, Decay: 0.22, Gain: 0.22})
	s.Add(Layer{Wave: Noise, Attack: 0.001, Decay: 0.04, HP: 1800, LP0: 5000, LP1: 2000, Gain: 0.2})
	s.Gain = 0.6 + 0.3*min(mag, 1)
	return s
}

// Purge is coolant flushing the rig: a sharp hiss that drops cold, with
// a falling chime. More Heat purged, a longer flush.
func Purge(heat float32) Sound {
	var s Sound
	h := min(max(heat, 0), 40)
	s.Add(Layer{Wave: Noise, Attack: 0.005, Decay: 0.35 + h/80, HP: 2200, LP0: 7000, LP1: 1200, Gain: 0.35})
	s.Add(Layer{Wave: FM, F0: 1760, F1: 880, Glide: 0.15, FMRatio: 3.51, FMIndex: 1.2, Attack: 0.002, Decay: 0.4, Gain: 0.12})
	s.Add(Layer{Wave: Noise, Attack: 0.08, Decay: 0.5, LP0: 1200, LP1: 400, Gain: 0.12})
	s.Gain = 1
	return s
}

// Chime is a grid node bought: two bright bell notes a fifth apart, the
// second a beat later, over a short sparkle. rare lifts it an octave step
// per rarity.
func Chime(rare int) Sound {
	var s Sound
	f := float32(660) * float32(math.Pow(1.122, float64(min(max(rare, 0), 3))))
	s.Add(Layer{Wave: FM, F0: f, FMRatio: 3.5, FMIndex: 0.8, Attack: 0.002, Decay: 0.5, Gain: 0.2})
	s.Add(Layer{Wave: FM, F0: f * 1.5, FMRatio: 3.5, FMIndex: 0.7, Delay: 0.08, Attack: 0.002, Decay: 0.7, Gain: 0.17})
	s.Add(Layer{Wave: Noise, Attack: 0.01, Decay: 0.25, HP: 6000, LP0: 9000, LP1: 6000, Gain: 0.05})
	s.Gain = 1
	return s
}

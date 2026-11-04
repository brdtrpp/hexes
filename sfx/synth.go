// Package sfx is the game's procedural sound. Nothing is recorded: every
// sound is synthesized while it plays from oscillators, noise, envelopes
// and filters, and the numbers that shape it come from what the game
// already knows (a damage type's JSON row, how hard a hit landed, what a
// hex cost in Power and Heat, the shape of the area it touched). A new
// damage type in the JSON gets its own sound with no code.
//
// The package is plain Go with no audio backend. Mixer is an endless
// io.Reader of 32-bit float stereo PCM that any player can pull from;
// the game hands it to Ebitengine's audio player.
package sfx

import (
	"encoding/binary"
	"math"
	"sync"
)

// Rate is the sample rate everything renders at.
const Rate = 44100

// MaxVoices caps how many layers sound at once. A new layer past the cap
// takes over the quietest one, so a big fight costs the same CPU as a
// small one.
const MaxVoices = 20

// MaxLayers is the most layers one Sound can stack.
const MaxLayers = 6

// block is how many samples share one set of control values (pitch,
// cutoff, vibrato). Sweeps update about 1400 times a second.
const block = 32

// Wave is a layer's sound source.
type Wave uint8

const (
	Sine Wave = iota
	Tri
	Saw
	Square
	Noise
	FM // a sine whose phase another sine bends: bells and glassy tones
)

// Layer is one voice of a sound: a source, a pitch sweep, an envelope and
// a filter. Zero values mean "off" or "none".
type Layer struct {
	Wave Wave
	// F0 is the starting pitch in Hz. With Glide set, pitch slides toward
	// F1 with that time constant in seconds.
	F0, F1, Glide float32
	// Delay waits before starting; Attack ramps up; Decay is how long the
	// tail takes to fall 60 dB after the attack.
	Delay, Attack, Decay float32
	Gain                 float32
	Pan                  float32 // -1 left to 1 right
	// FMRatio and FMIndex shape an FM layer: the modulator runs at
	// F*FMRatio, and the index (brightness) fades with the envelope.
	FMRatio, FMIndex float32
	// LP0 to LP1 sweeps a resonant lowpass over the tail (0 leaves it
	// open). HP is a fixed highpass. Res is resonance, 0 to 0.9.
	LP0, LP1, HP, Res float32
	// Vib is pitch vibrato as a fraction of the pitch, VibRate in Hz.
	Vib, VibRate float32
	// Trem is tremolo depth 0 to 1, TremRate in Hz.
	Trem, TremRate float32
	// Crackle breaks a noise layer into random pops (0 to 1).
	Crackle float32
}

// Sound is a stack of layers that start together (each may still have
// its own Delay). Gain, Pan and Delay apply to every layer.
type Sound struct {
	L     [MaxLayers]Layer
	N     int
	Gain  float32
	Pan   float32
	Delay float32
}

// Add stacks a layer, dropping it if the sound is full.
func (s *Sound) Add(l Layer) {
	if s.N < MaxLayers && l.Gain > 0 {
		s.L[s.N] = l
		s.N++
	}
}

// sine is a one-cycle table; nearest lookup is plenty at these sizes.
const tabBits = 12
const tabSize = 1 << tabBits

var sineTab = func() (t [tabSize]float32) {
	for i := range t {
		t[i] = float32(math.Sin(2 * math.Pi * float64(i) / tabSize))
	}
	return
}()

func sin1(ph float32) float32 { // ph in cycles, any value
	return sineTab[int(ph*tabSize)&(tabSize-1)]
}

type voice struct {
	on      bool
	l       Layer
	wait    int // samples left before it starts
	n       int // samples since it started
	attackN int
	env     float32
	decMul  float32
	ph, mph float32 // carrier and modulator phase, in cycles
	vph     float32 // vibrato phase
	tph     float32 // tremolo phase
	low     float32 // filter state
	band    float32
	hpA     float32
	hpY     float32
	hpX     float32
	pop     float32
	gl, gr  float32
	rng     uint32
}

func (v *voice) start(l Layer, seed uint32) {
	*v = voice{on: true, l: l, rng: seed | 1}
	v.wait = int(l.Delay * Rate)
	v.attackN = max(int(l.Attack*Rate), 1)
	dec := max(l.Decay, 0.005)
	v.decMul = float32(math.Exp(-6.9 / float64(dec*Rate)))
	if l.HP > 0 {
		v.hpA = float32(math.Exp(-2 * math.Pi * float64(l.HP) / Rate))
	}
	p := min(max(l.Pan, -1), 1)
	a := float64(p+1) * math.Pi / 4 // equal power
	v.gl, v.gr = float32(math.Cos(a)), float32(math.Sin(a))
}

// level is roughly how loud the voice is right now, for stealing.
func (v *voice) level() float32 {
	if !v.on {
		return -1
	}
	if v.wait > 0 {
		return v.l.Gain
	}
	return v.env * v.l.Gain
}

func (v *voice) rand() float32 {
	x := v.rng
	x ^= x << 13
	x ^= x >> 17
	x ^= x << 5
	v.rng = x
	return float32(x)/float32(1<<31) - 1
}

// render adds frames of this voice into out (stereo, interleaved).
func (v *voice) render(out []float32) {
	frames := len(out) / 2
	l := &v.l
	i := 0
	for i < frames && v.wait > 0 {
		v.wait--
		i++
	}
	for i < frames && v.on {
		// Control values for this block.
		t := float32(v.n) / Rate
		f := l.F0
		if l.Glide > 0 {
			f = l.F1 + (l.F0-l.F1)*float32(math.Exp(float64(-t/l.Glide)))
		}
		if l.Vib > 0 {
			f *= 1 + l.Vib*sin1(v.vph)
			v.vph += l.VibRate * block / Rate
		}
		dph := f / Rate
		mdph := f * l.FMRatio / Rate
		trem := float32(1)
		if l.Trem > 0 {
			trem = 1 - l.Trem*0.5*(1+sin1(v.tph))
			v.tph += l.TremRate * block / Rate
		}
		var fc, damp float32
		filt := l.LP0 > 0
		if filt {
			c := l.LP1 + (l.LP0-l.LP1)*float32(math.Exp(float64(-t/max(l.Decay*0.4, 0.01))))
			c = min(max(c, 30), 7500)
			fc = 2 * float32(math.Sin(math.Pi*float64(c)/Rate))
			damp = 1.4*(1-min(l.Res, 0.9)) + 0.1
		}
		end := min(i+block, frames)
		for ; i < end; i++ {
			// Envelope: linear attack, then exponential tail.
			if v.n < v.attackN {
				v.env = float32(v.n) / float32(v.attackN)
			} else if v.n == v.attackN {
				v.env = 1
			} else {
				v.env *= v.decMul
			}
			var x float32
			switch l.Wave {
			case Sine:
				x = sin1(v.ph)
			case Tri:
				x = 4*abs(v.ph-0.5) - 1
			case Saw:
				x = 2*v.ph - 1
			case Square:
				if v.ph < 0.5 {
					x = 0.7
				} else {
					x = -0.7
				}
			case Noise:
				x = v.rand()
				if l.Crackle > 0 {
					if v.rand() > 1-l.Crackle*0.03 {
						v.pop = 1
					}
					v.pop *= 0.94
					x *= (1 - l.Crackle) + l.Crackle*3*v.pop
				}
			case FM:
				x = sin1(v.ph + l.FMIndex*v.env*0.16*sin1(v.mph))
				v.mph += mdph
				v.mph -= float32(int(v.mph))
			}
			v.ph += dph
			v.ph -= float32(int(v.ph))
			if filt {
				v.low += fc * v.band
				high := x - v.low - damp*v.band
				v.band += fc * high
				x = v.low
			}
			if v.hpA > 0 {
				y := v.hpA * (v.hpY + x - v.hpX)
				v.hpX, v.hpY = x, y
				x = y
			}
			s := x * v.env * trem * l.Gain
			out[2*i] += s * v.gl
			out[2*i+1] += s * v.gr
			v.n++
		}
		if v.n > v.attackN && v.env < 0.001 {
			v.on = false
		}
	}
}

func abs(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

// Mixer plays sounds. It is safe to call Play from the game loop while
// the audio backend calls Read on its own goroutine.
type Mixer struct {
	mu   sync.Mutex
	v    [MaxVoices]voice
	buf  []float32
	pend []byte
	pos  int
	rng  uint32
	// Gain is the master level before the soft limiter.
	Gain float32
}

// NewMixer makes a silent mixer.
func NewMixer() *Mixer {
	return &Mixer{rng: 0x9e3779b9, Gain: 0.9, buf: make([]float32, 2*1024), pend: make([]byte, 0, 8*1024)}
}

func (m *Mixer) seed() uint32 {
	m.rng = m.rng*1664525 + 1013904223
	return m.rng
}

// Play starts a sound. Past MaxVoices it replaces the quietest voices.
func (m *Mixer) Play(s Sound) {
	if s.N == 0 || s.Gain <= 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := 0; i < s.N; i++ {
		l := s.L[i]
		l.Gain *= s.Gain
		l.Pan = min(max(l.Pan+s.Pan, -1), 1)
		l.Delay += s.Delay
		best, bl := -1, float32(math.MaxFloat32)
		for j := range m.v {
			if lv := m.v[j].level(); lv < bl {
				best, bl = j, lv
			}
		}
		if bl >= 0 && bl > l.Gain { // everything playing is louder
			continue
		}
		m.v[best].start(l, m.seed())
	}
}

// Active is how many voices are sounding or waiting to.
func (m *Mixer) Active() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for i := range m.v {
		if m.v[i].on {
			n++
		}
	}
	return n
}

// Stop silences everything at once.
func (m *Mixer) Stop() {
	m.mu.Lock()
	for i := range m.v {
		m.v[i].on = false
	}
	m.pend, m.pos = m.pend[:0], 0
	m.mu.Unlock()
}

// Render mixes stereo interleaved frames into dst, overwriting it. Every
// sample comes out of a soft limiter, so it stays inside -1 to 1.
func (m *Mixer) Render(dst []float32) {
	m.mu.Lock()
	m.render(dst)
	m.mu.Unlock()
}

func (m *Mixer) render(dst []float32) {
	clear(dst)
	for i := range m.v {
		if m.v[i].on {
			m.v[i].render(dst)
		}
	}
	g := m.Gain
	for i, x := range dst {
		dst[i] = soft(x * g)
	}
}

// soft is a cheap tanh-like limiter: transparent when quiet, never past 1.
func soft(x float32) float32 {
	if x > 3 {
		return 1
	}
	if x < -3 {
		return -1
	}
	return x * (27 + x*x) / (27 + 9*x*x)
}

// Read fills p with 32-bit float little-endian stereo PCM. It never runs
// dry: with nothing playing it returns silence.
func (m *Mixer) Read(p []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for n < len(p) {
		if m.pos == len(m.pend) {
			frames := min((len(p)-n+7)/8, len(m.buf)/2)
			buf := m.buf[:frames*2]
			m.render(buf)
			m.pend = m.pend[:len(buf)*4]
			for i, x := range buf {
				binary.LittleEndian.PutUint32(m.pend[i*4:], math.Float32bits(x))
			}
			m.pos = 0
		}
		c := copy(p[n:], m.pend[m.pos:])
		m.pos += c
		n += c
	}
	return n, nil
}

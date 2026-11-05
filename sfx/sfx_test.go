package sfx

import (
	"math"
	"strings"
	"testing"

	"hexes/game"
	"hexes/skill"
)

// stats describes a rendered clip.
type stats struct {
	peak, rms float64
	zcr       float64 // zero crossings per second: a rough brightness
	dur       float64 // seconds until it last rose above -40 dB of its peak
}

func render(s Sound, secs float64) []float32 {
	m := NewMixer()
	m.Play(s)
	out := make([]float32, 2*int(secs*Rate))
	m.Render(out)
	return out
}

func measure(out []float32) stats {
	var st stats
	var sum float64
	cross := 0
	prev := float32(0)
	for i := 0; i < len(out); i += 2 {
		x := (out[i] + out[i+1]) / 2
		a := math.Abs(float64(x))
		st.peak = math.Max(st.peak, a)
		sum += float64(x) * float64(x)
		if (x > 0) != (prev > 0) {
			cross++
		}
		prev = x
	}
	n := float64(len(out) / 2)
	st.rms = math.Sqrt(sum / n)
	st.zcr = float64(cross) / (n / Rate)
	for i := len(out) - 2; i >= 0; i -= 2 {
		if math.Abs(float64(out[i])) > st.peak*0.01 {
			st.dur = float64(i/2) / Rate
			break
		}
	}
	return st
}

func checkClip(t *testing.T, name string, out []float32) {
	t.Helper()
	for i, x := range out {
		if math.IsNaN(float64(x)) || math.IsInf(float64(x), 0) || x > 1 || x < -1 {
			t.Fatalf("%s: sample %d is %v", name, i, x)
		}
	}
}

func TestEverySoundPlays(t *testing.T) {
	reg := skill.DefaultRegistry()
	tm := TimbreOf(*reg.Get(0))
	sounds := map[string]Sound{
		"hit": Hit(tm, 1, false, 1), "impact": Hit(tm, 0.5, true, 1), "big": Hit(tm, 3, false, 1),
		"wave": Area(tm, false, 2, 1, 1), "cone": Area(tm, true, 3, 1, 1), "hop": ChainHop(tm, 2, 1),
		"cast": Cast(12, 8), "coldcast": Cast(5, -6), "freecast": Cast(0, 0), "vent": Vent(),
		"tick": Tick(tm, 1), "kill": Kill(tm, false), "death": Kill(tm, true), "hurt": Hurt(1),
		"claw": Claw(), "push": Push(), "slam": Slam(), "fall": Fall(), "loot": Loot(3), "full": Full(),
		"salvage": Salvage(), "learn": Learn(), "floor": Floor(), "click": Click(true),
		"mend": Mend(1), "shield": Shield(1), "soak": Soak(1), "purge": Purge(20),
	}
	for name, s := range sounds {
		out := render(s, 2.5)
		checkClip(t, name, out)
		st := measure(out)
		if st.peak < 0.02 {
			t.Errorf("%s is silent (peak %.4f)", name, st.peak)
		}
		if st.dur > 2.2 {
			t.Errorf("%s rings for %.2fs; it should die out", name, st.dur)
		}
	}
}

// Every seed type sounds different from every other: the pairs differ in
// brightness, length or loudness by a clear margin.
func TestTypesSoundApart(t *testing.T) {
	reg := skill.DefaultRegistry()
	var st []stats
	var names []string
	for i := 0; i < reg.Len(); i++ {
		dt := reg.Get(skill.TypeID(i))
		out := render(Hit(TimbreOf(*dt), 1, false, 1), 1.5)
		st = append(st, measure(out))
		names = append(names, dt.Name)
	}
	for i := range st {
		for j := i + 1; j < len(st); j++ {
			a, b := st[i], st[j]
			d := math.Abs(math.Log(a.zcr/b.zcr)) + math.Abs(math.Log(a.dur/b.dur)) + math.Abs(math.Log(a.rms/b.rms))
			if d < 0.3 {
				t.Errorf("%s and %s sound alike: %+v vs %+v", names[i], names[j], a, b)
			}
		}
	}
	for i, s := range st {
		t.Logf("%-8s zcr %6.0f/s  dur %.2fs  rms %.3f  peak %.2f", names[i], s.zcr, s.dur, s.rms, s.peak)
	}
}

// A type added to the JSON gets a sound with no code, and it isn't a copy
// of any seed type's.
func TestNewTypeGetsItsOwnSound(t *testing.T) {
	reg, err := skill.LoadRegistry(strings.NewReader(`[
		{"name": "plasma", "status": "burn", "status_power": 4, "spread_frac": 0.35, "heat_offset": 2},
		{"name": "arc", "status": "stagger", "push_per_hit": 0.4, "ticks": 3, "heat_offset": 1}
	]`))
	if err != nil {
		t.Fatal(err)
	}
	arc := TimbreOf(*reg.Get(1))
	out := render(Hit(arc, 1, false, 1), 1.5)
	checkClip(t, "arc", out)
	a := measure(out)
	if a.peak < 0.05 {
		t.Fatalf("arc is silent: %+v", a)
	}
	seed := skill.DefaultRegistry()
	for i := 0; i < seed.Len(); i++ {
		if TimbreOf(*seed.Get(skill.TypeID(i))) == arc {
			t.Errorf("arc's timbre copies %s", seed.Get(skill.TypeID(i)).Name)
		}
	}
}

func TestHarderHitsAreLouderAndLonger(t *testing.T) {
	reg := skill.DefaultRegistry()
	for i := 0; i < reg.Len(); i++ {
		tm := TimbreOf(*reg.Get(skill.TypeID(i)))
		lo := measure(render(Hit(tm, 0.2, false, 1), 2))
		hi := measure(render(Hit(tm, 3, false, 1), 2))
		if hi.rms <= lo.rms {
			t.Errorf("%s: big hit rms %.3f not above scratch %.3f", reg.Get(skill.TypeID(i)).Name, hi.rms, lo.rms)
		}
	}
}

func TestMorePowerClimbsLonger(t *testing.T) {
	lo := measure(render(Cast(3, 0), 2))
	hi := measure(render(Cast(35, 0), 2))
	if hi.dur <= lo.dur || hi.rms <= lo.rms {
		t.Errorf("35 Power %+v should outlast and outweigh 3 Power %+v", hi, lo)
	}
}

func TestVoiceCapAndLimiter(t *testing.T) {
	reg := skill.DefaultRegistry()
	m := NewMixer()
	for i := 0; i < 200; i++ {
		m.Play(Hit(TimbreOf(*reg.Get(skill.TypeID(i % reg.Len()))), 3, false, 1))
	}
	if n := m.Active(); n > MaxVoices {
		t.Fatalf("%d voices active, cap is %d", n, MaxVoices)
	}
	out := make([]float32, 2*Rate)
	m.Render(out)
	checkClip(t, "200 hits", out)
}

func TestReadIsEndlessFloatPCM(t *testing.T) {
	m := NewMixer()
	m.Play(Hit(Neutral, 1, false, 1))
	var got []byte
	for _, n := range []int{7, 1, 4096, 333, 8, 12000} {
		p := make([]byte, n)
		k, err := m.Read(p)
		if err != nil || k != n {
			t.Fatalf("Read(%d) = %d, %v", n, k, err)
		}
		got = append(got, p...)
	}
	// The stream matches a straight render of the same sound.
	ref := NewMixer()
	ref.Play(Hit(Neutral, 1, false, 1))
	want := make([]float32, len(got)/4)
	ref.Render(want[:len(want)/2*2])
	for i := 0; i+4 <= len(got) && i/4 < len(want)/2*2; i += 4 {
		x := math.Float32frombits(uint32(got[i]) | uint32(got[i+1])<<8 | uint32(got[i+2])<<16 | uint32(got[i+3])<<24)
		if math.Abs(float64(x-want[i/4])) > 0.01 { // block edges shift with chunk size
			t.Fatalf("sample %d: stream %v, render %v", i/4, x, want[i/4])
		}
	}
	// Silence once it's done.
	p := make([]byte, 8*Rate)
	m.Read(p)
	m.Read(p)
	for _, b := range p {
		if b != 0 {
			t.Fatal("not silent after the sound ended")
		}
	}
}

// A burst of identical hits in one step makes a few sounds, not dozens.
func TestDirectorThinsBursts(t *testing.T) {
	reg := skill.DefaultRegistry()
	m := NewMixer()
	d := NewDirector(reg, m)
	d.Step(1.0 / 60)
	for i := 0; i < 30; i++ {
		d.Hear(&game.FX{Kind: game.FXBolt, Type: 1, Mag: 1, Impact: true, X: float64(i % 3)}, 0, 0)
	}
	if d.frame != 1 {
		t.Errorf("30 impacts of one type in a step started %d sounds, want 1", d.frame)
	}
	for k := 0; k < 30; k++ {
		d.Hear(&game.FX{Kind: game.FXBolt, Type: skill.TypeID(k % reg.Len()), Mag: 1}, 0, 0)
	}
	if d.frame > MaxPerStep {
		t.Errorf("%d sounds in a step, cap is %d", d.frame, MaxPerStep)
	}
	if m.Active() > MaxVoices {
		t.Errorf("voice cap broken")
	}
	// Far off-screen hits are dropped.
	d.Step(1)
	d.Hear(&game.FX{Kind: game.FXBolt, Type: 0, Mag: 1, X: 80}, 0, 0)
	if d.frame != 0 {
		t.Error("a hit 80 hexes away played")
	}
}

// Every effect kind the sim records goes through the director without
// trouble, and the ones that should make noise do.
func TestDirectorHearsEveryKind(t *testing.T) {
	reg := skill.DefaultRegistry()
	for k := game.FXNumber; k <= game.FXSoak; k++ {
		if k == game.FXSalvage || k == game.FXLens {
			continue // the salvage chime is a test of its own; lenses are silent
		}
		m := NewMixer()
		d := NewDirector(reg, m)
		d.Step(1)
		e := &game.FX{Kind: k, Type: 2, Mag: 1, Value: 2, Power: 10, Heat: 5, Hurt: true, NPts: 3}
		d.Hear(e, 0, 0)
		out := make([]float32, Rate)
		m.Render(out)
		checkClip(t, "kind", out)
		if measure(out).peak < 0.01 {
			t.Errorf("FX kind %d made no sound", k)
		}
	}
}

func BenchmarkMixFullVoices(b *testing.B) {
	reg := skill.DefaultRegistry()
	var tms [4]Timbre
	for j := range tms {
		tms[j] = TimbreOf(*reg.Get(skill.TypeID(j)))
	}
	m := NewMixer()
	out := make([]float32, 2*512)
	b.SetBytes(int64(len(out) * 4))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if m.Active() < MaxVoices {
			for j := range tms {
				m.Play(Hit(tms[j], 2, false, 1))
			}
		}
		m.Render(out)
	}
}

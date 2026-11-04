package fx

import (
	"math"
	"testing"

	"hexes/skill"
)

func defaultSigs(t *testing.T) map[string]Sig {
	t.Helper()
	reg := skill.DefaultRegistry()
	out := map[string]Sig{}
	for _, name := range []string{"plasma", "cryo", "kinetic", "nanite", "drone", "cloak"} {
		id, ok := reg.ID(name)
		if !ok {
			t.Fatalf("no type %q", name)
		}
		out[name] = SigOf(*reg.Get(id), 1, 1, 1)
	}
	return out
}

// Each seed type's strongest facet is the one its numbers say.
func TestSigOfSeedTypes(t *testing.T) {
	want := map[string]Facet{
		"plasma": FacetHot, "cryo": FacetCold, "kinetic": FacetForce,
		"nanite": FacetSwarm, "drone": FacetMark, "cloak": FacetVeil,
	}
	for name, sig := range defaultSigs(t) {
		w := sig.weights()
		best := FacetPlain
		for f := range w {
			if w[f] > w[best] {
				best = Facet(f)
			}
		}
		if best != want[name] {
			t.Errorf("%s: strongest facet %d, want %d (%+v)", name, best, want[name], sig)
		}
	}
	// Cryo rings are hexagons, kinetic rings echo.
	s := defaultSigs(t)
	if s["cryo"].Ring().Sides != 6 {
		t.Errorf("cryo ring sides %d", s["cryo"].Ring().Sides)
	}
	if !s["kinetic"].Ring().Echo {
		t.Error("kinetic ring has no echo")
	}
	if s["kinetic"].Jag() >= s["plasma"].Jag() {
		t.Error("kinetic bolts should fly straighter than plasma")
	}
}

// A new type in the JSON gets a look from its numbers alone.
func TestSigOfNewType(t *testing.T) {
	s := SigOf(skill.DamageType{HeatOffset: 6, PushPerHit: 1}, 1, 1, 1)
	if s.Hot < 1 || s.Force < 0.8 {
		t.Fatalf("hot pusher: %+v", s)
	}
}

func TestPoolCapAndLOD(t *testing.T) {
	p := NewPool(1)
	shot := Shot{Sig: Neutral, Mag: 3}
	for i := 0; i < 2000; i++ {
		p.Impact(0, 0, 1, 0, shot)
	}
	if p.N > Max {
		t.Fatalf("pool overflow %d", p.N)
	}
	if p.LOD() >= 0.5 {
		t.Fatalf("full pool LOD %v", p.LOD())
	}
	p.Update(5)
	if p.N != 0 {
		t.Fatalf("%d particles outlived 5 s", p.N)
	}
	if p.LOD() != 1 {
		t.Fatalf("empty pool LOD %v", p.LOD())
	}
}

func TestBiggerHitsSpawnMore(t *testing.T) {
	small, big := NewPool(7), NewPool(7)
	for i := 0; i < 20; i++ {
		small.Impact(0, 0, 1, 0, Shot{Sig: Neutral, Mag: 0.15})
		big.Impact(0, 0, 1, 0, Shot{Sig: Neutral, Mag: 3})
	}
	if big.N <= small.N*3/2 {
		t.Fatalf("big %d vs small %d", big.N, small.N)
	}
}

func TestDeterministic(t *testing.T) {
	a, b := NewPool(42), NewPool(42)
	s := defaultSigs(t)["plasma"]
	for _, p := range []*Pool{a, b} {
		p.Wave(1, 2, 3, Shot{Sig: s, Mag: 1, Heat: 12})
		p.Update(0.1)
	}
	if a.N != b.N || a.P[0] != b.P[0] {
		t.Fatal("same seed, different particles")
	}
}

// Wave particles coast out and stop inside the area they were given.
func TestWaveStaysInArea(t *testing.T) {
	p := NewPool(3)
	const r = 4
	p.Wave(0, 0, r, Shot{Sig: Neutral, Mag: 1})
	for i := 0; i < 20; i++ {
		p.Update(1.0 / 60)
	}
	for i := 0; i < p.N; i++ {
		q := &p.P[i]
		if d := math.Hypot(float64(q.X), float64(q.Y)); d > r*1.25 {
			t.Fatalf("particle %d at %.2f, past radius %d", i, d, r)
		}
	}
}

// Power motes converge on the caster.
func TestCastPowerConverges(t *testing.T) {
	p := NewPool(5)
	p.Cast(0, 0, 20, 0)
	if p.N == 0 {
		t.Fatal("no motes")
	}
	q := p.P[0]
	d0 := math.Hypot(float64(q.X), float64(q.Y))
	p.Update(q.Life * 0.9)
	q = p.P[0]
	if d1 := math.Hypot(float64(q.X), float64(q.Y)); d1 > d0*0.3 {
		t.Fatalf("mote went %.2f -> %.2f", d0, d1)
	}
}

func TestNoAllocs(t *testing.T) {
	p := NewPool(9)
	s := defaultSigs(t)
	n := testing.AllocsPerRun(50, func() {
		p.Impact(0, 0, 1, 0, Shot{Sig: s["kinetic"], Mag: 1})
		p.Wave(0, 0, 3, Shot{Sig: s["cryo"], Mag: 1})
		p.Wedge(0, 0, 1, 0, 0.6, 4, Shot{Sig: s["nanite"], Mag: 1})
		p.Cast(0, 0, 12, 8)
		p.Aura(AuraBurn, 0, 0, 0.5, 1, 0.5, 0.2, 1.0/60)
		p.Update(1.0 / 60)
		for i := 0; i < p.N; i++ {
			_ = p.P[i].Look()
		}
	})
	if n != 0 {
		t.Fatalf("%v allocs per frame", n)
	}
}

func BenchmarkBusyFrame(b *testing.B) {
	p := NewPool(11)
	s := Shot{Sig: Sig{R: 1, G: 0.5, B: 0.2, Hot: 1, Splash: 0.7}, Mag: 1, Heat: 10}
	for i := 0; i < b.N; i++ {
		p.Impact(0, 0, 1, 0, s)
		p.Update(1.0 / 60)
		for j := 0; j < p.N; j++ {
			_ = p.P[j].Look()
		}
	}
}

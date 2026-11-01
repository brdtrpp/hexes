package skill

import (
	"math"
	"strings"
	"testing"
)

var testTemplate = Template{Name: "Lattice", Sockets: [6]SocketKind{T, G, G, E, E, W}, Broken: 1}

func TestLearnIsDeterministic(t *testing.T) {
	a, b := Learn(testTemplate, 42), Learn(testTemplate, 42)
	if a.Sockets != b.Sockets || a.Linked != b.Linked {
		t.Fatal("same seed rolled different layouts")
	}
}

func TestLearnVariesBySeed(t *testing.T) {
	seen := map[[6]SocketKind]bool{}
	for s := uint64(0); s < 50; s++ {
		h := Learn(testTemplate, s)
		seen[h.Sockets] = true
		var count [4]int
		for _, k := range h.Sockets {
			count[k]++
		}
		if count != [4]int{1, 2, 2, 1} {
			t.Fatalf("seed %d changed the socket mix: %v", s, h.Sockets)
		}
		broken := 0
		for _, l := range h.Linked {
			if !l {
				broken++
			}
		}
		if broken != 1 {
			t.Fatalf("seed %d broke %d edges", s, broken)
		}
	}
	if len(seen) < 20 {
		t.Fatalf("only %d distinct layouts from 50 seeds", len(seen))
	}
}

func TestHitCapCurve(t *testing.T) {
	r := DefaultRules()
	for _, tc := range []struct {
		att, def int
		want     float32
	}{{10, 10, 250}, {15, 10, 500}, {20, 10, 1000}, {40, 10, 1000}, {5, 10, 125}, {0, 100, 10}} {
		approx(t, "cap", r.HitCap(tc.att, tc.def, 1000), tc.want)
	}
}

func TestTerrainSkipsCapAndResist(t *testing.T) {
	r := DefaultRules()
	u := Unit{Alive: true, HP: 1000, MaxHP: 1000, Level: 50}
	u.Resist[0] = 0.6
	d, capped := r.Resolve(&u, 900, 0, 1, true)
	if d != 900 || capped {
		t.Fatalf("terrain dealt %.1f capped=%v", d, capped)
	}
}

func TestSoftCap(t *testing.T) {
	if SoftCap(5, 10) != 5 || SoftCap(10, 10) != 10 {
		t.Fatal("soft cap bites below the knee")
	}
	prev, prevGain := SoftCap(10, 10), 1.0
	for p := 11; p <= 60; p++ {
		v := SoftCap(p, 10)
		gain := v - prev
		if gain <= 0 || gain > prevGain+1e-9 {
			t.Fatalf("point %d gained %.4f after %.4f", p, gain, prevGain)
		}
		prev, prevGain = v, gain
	}
	if math.Abs(SoftCap(20, 10)-16.93) > 0.01 {
		t.Fatalf("20 points -> %.2f", SoftCap(20, 10))
	}
}

func TestRigDepthFromProcessing(t *testing.T) {
	for _, tc := range []struct{ points, depth int }{{0, 0}, {5, 1}, {10, 2}, {20, 3}, {40, 4}} {
		if d := NewRig(Points{Processing: tc.points}).MaxDepth; d != tc.depth {
			t.Fatalf("processing %d -> depth %d, want %d", tc.points, d, tc.depth)
		}
	}
}

func TestRigOverheats(t *testing.T) {
	r := NewRig(Points{})
	p := &Program{Power: 10, Heat: 40}
	if !r.TryFire(p) || !r.TryFire(p) {
		t.Fatal("cool rig refused to fire")
	}
	if !r.TryFire(p) || r.ShutdownLeft != OverheatShutdown {
		t.Fatalf("third fire should overheat: heat %.0f shutdown %.1f", r.Heat, r.ShutdownLeft)
	}
	if r.TryFire(p) {
		t.Fatal("fired while shut down")
	}
	r.Tick(OverheatShutdown)
	vent := &Program{Power: 1, Heat: -500}
	if !r.TryFire(vent) {
		t.Fatal("rig never came back")
	}
	if r.Heat != 0 {
		t.Fatalf("heat went to %.1f", r.Heat)
	}
}

func TestDefaultRegistry(t *testing.T) {
	if reg.Len() != 6 {
		t.Fatalf("%d types", reg.Len())
	}
	for _, n := range []string{"plasma", "cryo", "kinetic", "nanite", "drone", "cloak"} {
		if _, ok := reg.ID(n); !ok {
			t.Fatalf("missing %s", n)
		}
	}
	if reg.Get(id("cryo")).HeatOffset >= 0 {
		t.Fatal("cryo should vent heat")
	}
}

func TestLoadRegistry(t *testing.T) {
	r, err := LoadRegistry(strings.NewReader(`[{"name":"sonic","status":"stagger","push_per_hit":2}]`))
	if err != nil || r.Get(r.MustID("sonic")).PushPerHit != 2 {
		t.Fatalf("load: %v", err)
	}
	if _, err := LoadRegistry(strings.NewReader(`[{"name":"a"},{"name":"a"}]`)); err == nil {
		t.Fatal("duplicate accepted")
	}
	if _, err := LoadRegistry(strings.NewReader(`[{"name":"a","status":"glitter"}]`)); err == nil {
		t.Fatal("unknown status accepted")
	}
}

func TestDisassembly(t *testing.T) {
	p := compile(t, targetsTargetHex(t, id("cryo"), 10), 1)
	s := p.String()
	for _, want := range []string{"foreach", "select nearest", "select adjacent", "damage"} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q in\n%s", want, s)
		}
	}
}

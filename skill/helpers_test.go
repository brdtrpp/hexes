package skill

import "testing"

const (
	T = SocketTrigger
	G = SocketTarget
	E = SocketEffect
	W = SocketWild
)

var reg = DefaultRegistry()

func id(name string) TypeID { return reg.MustID(name) }

func set(t *testing.T, h *Hex, i int, p Part) {
	t.Helper()
	if err := h.Set(i, p); err != nil {
		t.Fatal(err)
	}
}

func compile(t *testing.T, h *Hex, depth int) *Program {
	t.Helper()
	p, err := Compile(h, reg, depth)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// steps returns the program's steps with a given op, in order.
func steps(p *Program, op Op) []Step {
	var out []Step
	for _, s := range p.Steps {
		if s.Op == op {
			out = append(out, s)
		}
	}
	return out
}

// world builds a 12x12 arena with the caster at (2,6), team 0, level 10.
func world() (*Arena, EntityID) {
	a := NewArena(12, 12, reg, DefaultRules())
	c := a.Spawn(0, Coord{2, 6}, 10, 1000)
	return a, c
}

func approx(t *testing.T, what string, got, want float32) {
	t.Helper()
	d := got - want
	if d < -0.01 || d > 0.01 {
		t.Fatalf("%s = %.3f, want %.3f", what, got, want)
	}
}

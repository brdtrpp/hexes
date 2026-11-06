package skill

import (
	"errors"
	"fmt"
)

// Docking. Hexes sit side by side on a board (the game keeps it). Every
// shape has a few ports on its border: one way in and one or two ways
// out, each facing one of the six sides. When a hex's out port faces the
// side of the hex next to it that holds that hex's way in, the two dock:
// once the first hex's chain reaches the port slot, the docked hex runs
// too, from its own start (its trigger, if it has one, is skipped), on
// whoever the port slot carries. Its parts pay their own Heat, so a long
// docked chain is a hot one. Call stays the way to reach a hex that isn't
// next to you.
//
// Ports come from the shape's graph alone, so every shape, old or new,
// has them without anything new in a save.

// Port is one place on a shape's border where a hex can dock.
type Port struct {
	Slot uint8
	Side uint8 // which way it faces, CellDirs order (0 is straight up)
	In   bool  // the way in; otherwise a way out
}

// MaxPorts is the most ports a shape has.
const MaxPorts = 3

// MaxDocked is the most docked hexes one firing runs, all chains counted,
// so a board full of forks can't blow up.
const MaxDocked = 12

// SideName is a side as a compass point: N, NE, SE, S, SW, NW.
func SideName(side int) string {
	return [...]string{"N", "NE", "SE", "S", "SW", "NW"}[((side%6)+6)%6]
}

// Ports is where the shape can dock: the way in on its start slot (or the
// first slot after it with an open side, for a Fork whose center is
// boxed in), then up to two ways out on the slots that run last,
// preferring ones that end a chain. Shapes of five slots or more get a
// second way out. Each port faces a different side, outward from the
// middle of the shape.
func (s Shape) Ports() (ps [MaxPorts]Port, n int) {
	g := s.Graph()
	N := int(g.N)
	var cx, cy float64
	for i := 0; i < N; i++ {
		x, y := g.At[i].XY()
		cx += x
		cy += y
	}
	cx /= float64(N)
	cy /= float64(N)
	used := uint8(0) // sides taken
	// side picks the open side of slot i that faces most outward, or -1.
	side := func(i int) int {
		best, score := -1, -1e9
		x, y := g.At[i].XY()
		for d, dir := range CellDirs {
			if used&(1<<d) != 0 {
				continue
			}
			c := g.At[i].Add(dir)
			open := true
			for j := 0; j < N; j++ {
				if g.At[j] == c {
					open = false
					break
				}
			}
			if !open {
				continue
			}
			dx, dy := dir.XY()
			if sc := dx*(x-cx) + dy*(y-cy); sc > score+1e-9 {
				best, score = d, sc
			}
		}
		return best
	}
	add := func(slot, d int, in bool) {
		ps[n] = Port{Slot: uint8(slot), Side: uint8(d), In: in}
		n++
		used |= 1 << d
	}
	order, cnt := g.Flow(int(g.Entry))
	inSlot := -1
	for _, i := range order[:cnt] {
		if d := side(i); d >= 0 {
			add(i, d, true)
			inSlot = i
			break
		}
	}
	outs := 1
	if N >= 5 {
		outs = 2
	}
	// Ways out: sinks first, latest in the flow first, then any other
	// slot from the end, never the start or the way in.
	for pass := 0; pass < 2 && n < MaxPorts && outs > 0; pass++ {
		for k := cnt - 1; k >= 0 && outs > 0; k-- {
			i := order[k]
			if i == int(g.Entry) || i == inSlot || (pass == 0) != (g.Out[i]&^bit(int(g.Entry)) == 0) {
				continue
			}
			taken := false
			for _, p := range ps[:n] {
				taken = taken || int(p.Slot) == i
			}
			if taken {
				continue
			}
			if d := side(i); d >= 0 {
				add(i, d, false)
				outs--
			}
		}
	}
	return ps, n
}

// InPort is the shape's way in.
func (s Shape) InPort() (Port, bool) {
	ps, n := s.Ports()
	for _, p := range ps[:n] {
		if p.In {
			return p, true
		}
	}
	return Port{}, false
}

// OutPort reports whether slot is one of the shape's ways out, and which
// side it faces.
func (s Shape) OutPort(slot int) (side int, ok bool) {
	ps, n := s.Ports()
	for _, p := range ps[:n] {
		if !p.In && int(p.Slot) == slot {
			return int(p.Side), true
		}
	}
	return -1, false
}

// CompileModule compiles a hex with no trigger (a module that only runs
// docked) as if a plain cast fired it, for its costs and to check it
// builds. Its own parts, its own shape's Heat.
func CompileModule(h *Hex, reg *Registry, maxDepth int) (*Program, error) {
	shape := h.Shape
	if !shape.Valid() {
		return nil, errBrokenShape(shape)
	}
	for i, s := range h.Slots {
		if s.Kind == SlotEmpty {
			continue
		}
		if !h.HasSlot(i) {
			return nil, errNotInShape(i, shape)
		}
		if s.Kind == SlotPart && s.Part.Kind == KindTrigger {
			return nil, errModuleTrigger
		}
	}
	trig := NewTrigger(OnCast)
	trig.Power, trig.Heat = 0, 0
	c := &compiler{reg: reg, trig: trig, next: RegSubject + 1,
		p: &Program{Name: h.Name, Trigger: OnCast, Steps: make([]Step, 0, 32)}}
	c.docking = 1 // no loop, no docks of its own counted here
	if err := c.level(h, h.entry(), 0, RegOrigin, RegSubject, map[*Hex]bool{h: true}); err != nil {
		return nil, err
	}
	if c.p.Heat > 0 {
		c.p.Heat *= shape.HeatScale()
		if shape.Quirk == ShapeLean {
			c.p.Heat *= LeanHeat
		}
	}
	c.p.Heat += c.dockHeat
	c.p.Shape = shape.Kind
	c.p.Regs = int(c.next)
	return c.p, nil
}

var errModuleTrigger = errors.New("a module has no trigger: it runs when a hex docks into it")

func errBrokenShape(s Shape) error { return fmt.Errorf("hex has a broken shape (%v)", s) }

func errNotInShape(i int, s Shape) error {
	return fmt.Errorf("slot %d isn't part of a %s", i+1, s.Name())
}

// dock runs the hexes docked to h's ways out, in flow order, each on
// whoever its port slot carries (or, if the slot carries no one, on
// landing). A chain that comes back round to a hex already running stops
// there.
func (c *compiler) dock(h *Hex, order []int, carry [Slots]int, landing uint8, seen map[*Hex]bool) error {
	for _, s := range order {
		// The way out is on a shape slot; on a laid hex, on the piece
		// covering that slot's pad.
		port := s
		if h.Laid() {
			port = -1
			ps, np := h.Shape.Ports()
			for _, p := range ps[:np] {
				if !p.In && h.pad(int(p.Slot)) == s {
					port = int(p.Slot)
					break
				}
			}
			if port < 0 {
				continue
			}
		}
		nh := h.Docks[port]
		if nh == nil || seen[nh] {
			continue
		}
		if _, ok := h.Shape.OutPort(port); !ok {
			continue
		}
		if c.docked >= MaxDocked {
			return nil
		}
		c.docked++
		src := landing
		if r := carry[s]; r >= 0 {
			src = uint8(r)
		}
		heat := c.p.Heat
		seen[nh] = true
		c.docking++
		err := c.level(nh, nh.entry(), 0, src, src, seen)
		c.docking--
		delete(seen, nh)
		if err != nil {
			return fmt.Errorf("docked %s: %w", nh.Name, err)
		}
		// Its own parts' Heat, on its own shape's scale.
		d := c.p.Heat - heat
		c.p.Heat = heat
		if d > 0 {
			d *= nh.Shape.HeatScale()
			if nh.Shape.Quirk == ShapeLean {
				d *= LeanHeat
			}
		}
		c.dockHeat += d
	}
	return nil
}

package skill

import (
	"errors"
	"fmt"
)

// NestCostPerDepth raises the Power and Heat of parts inside nested hexes:
// a part at depth d costs (1 + NestCostPerDepth*d) times its base.
const NestCostPerDepth = 0.5

// Compile turns a learned hex and everything slotted in it into a flat
// program. maxDepth is the caster's nesting limit from Processing.
//
// The rules it bakes in:
//   - The hex resolves clockwise starting at its trigger slot. Nested hexes
//     have no trigger and resolve clockwise from their slot 0.
//   - Parts only feed slots they touch across a linked edge.
//   - An effect (or nested hex) lands on the targets in the slots on either
//     side of it. Touching two targets, it lands on both (no double hits).
//     Touching none, it lands on the trigger's subject at the top (the foe
//     hit, the corpse, or the nearest foe for cast triggers), or on the
//     entity a nested hex is running on.
//   - Top-level targets select around the subject for on-hit and on-kill
//     triggers, and around the caster for the rest.
//   - A multiplier scales the part in the slot just before it (counter-
//     clockwise, already resolved) if that edge is linked. A run of
//     multipliers stacks onto the same part. A multiplier after a nested hex
//     scales the matching numbers everywhere inside it. A multiplier with
//     nothing linked before it is wasted.
func Compile(h *Hex, reg *Registry, maxDepth int) (*Program, error) {
	trig := -1
	for i, s := range h.Slots {
		if s.Kind == SlotPart && s.Part.Kind == KindTrigger {
			if trig >= 0 {
				return nil, errors.New("hex has more than one trigger")
			}
			trig = i
		}
	}
	if trig < 0 {
		return nil, errors.New("hex has no trigger")
	}
	depth := h.Depth()
	if depth > maxDepth {
		return nil, fmt.Errorf("hex nests %d deep, processing allows %d", depth, maxDepth)
	}
	t := h.Slots[trig].Part
	c := &compiler{
		reg:  reg,
		trig: t,
		next: RegSubject + 1,
		p: &Program{
			Name:    h.Name,
			Trigger: t.Trigger,
			Depth:   depth,
			Steps:   make([]Step, 0, 32),
			Power:   t.Power,
			Heat:    t.Heat,
			Period:  t.Period,
		},
	}
	if t.Trigger == Periodic && c.p.Period <= 0 {
		c.p.Period = DefaultPeriod
	}
	if err := c.level(h, trig, 0, RegOrigin, RegSubject, map[*Hex]bool{h: true}); err != nil {
		return nil, err
	}
	if t.Quirk == QuirkEcho {
		// Run the whole body twice. Spans are relative, so a plain copy
		// works; the second pass selects its targets afresh.
		c.p.Steps = append(c.p.Steps, c.p.Steps...)
		c.p.Power *= 1 + EchoCost
		c.p.Heat *= 1 + EchoCost
	}
	c.p.Regs = int(c.next)
	return c.p, nil
}

type compiler struct {
	reg  *Registry
	trig Part
	p    *Program
	next uint8
}

func (c *compiler) alloc() (uint8, error) {
	if int(c.next) >= MaxRegs {
		return 0, fmt.Errorf("hex needs more than %d registers", MaxRegs)
	}
	r := c.next
	c.next++
	return r, nil
}

func (c *compiler) emit(s Step) int {
	c.p.Steps = append(c.p.Steps, s)
	return len(c.p.Steps) - 1
}

func (c *compiler) cost(p Part, depth int) {
	f := float32(1 + NestCostPerDepth*float64(depth))
	heat := p.Heat
	if p.Kind == KindEffect && p.Effect != EffectPush {
		heat += c.reg.Get(p.Type).HeatOffset
	}
	switch p.Quirk {
	case QuirkRelay:
		f *= 1 + RelayCost
	case QuirkRadiant:
		heat += RadiantHeatCost
	}
	c.p.Power += p.Power * f
	c.p.Heat += heat * f
}

type span struct {
	lo, hi int
	ok     bool
}

// level compiles one hex ring starting at slot start. Targets select
// around the entity in origin; effects with no target beside them land on
// the entities in landing. In a nested hex both are the entity it runs on.
func (c *compiler) level(h *Hex, start, depth int, origin, landing uint8, seen map[*Hex]bool) error {
	var order [Slots]int
	for k := range order {
		order[k] = (start + k) % Slots
	}
	var tgt [Slots]int
	for i := range tgt {
		tgt[i] = -1
	}
	var fed [Slots]span // step range each slot produced, for multipliers
	first := len(c.p.Steps)

	// Pass 1: targets. They depend on nothing but the origin, so they all
	// select up front and any effect can read them.
	for _, s := range order {
		sl := h.Slots[s]
		if sl.Kind != SlotPart || sl.Part.Kind != KindTarget {
			continue
		}
		p := sl.Part
		r, err := c.alloc()
		if err != nil {
			return err
		}
		src := origin
		if p.Target == TargetSelf {
			src = RegCaster
		}
		i := c.emit(Step{Op: OpSelect, Target: p.Target, Src: src, Dst: r,
			Count: clampCount(int(p.Count)), Value: p.Value})
		tgt[s] = int(r)
		fed[s] = span{i, i + 1, true}
		c.cost(p, depth)
	}

	// Pass 2: effects and nested hexes, clockwise.
	for _, s := range order {
		sl := h.Slots[s]
		switch {
		case sl.Kind == SlotPart && sl.Part.Kind == KindTrigger:
			if depth > 0 {
				return errors.New("nested hex can't have its own trigger")
			}
		case sl.Kind == SlotPart && sl.Part.Kind == KindEffect:
			p := sl.Part
			src, err := c.feed(h, s, tgt, landing)
			if err != nil {
				return err
			}
			st := Step{Src: src, Type: p.Type, Value: p.Value * p.Rarity.Scale()}
			switch p.Effect {
			case EffectDamage:
				st.Op = OpDamage
				st.Value *= c.trig.Value
				if c.trig.Trigger.Stealthy() {
					if m := c.reg.Get(p.Type).StealthMult; m > 0 {
						st.Value *= m
					}
				}
			case EffectPush:
				st.Op = OpPush
			case EffectStatus:
				st.Op = OpStatus
			}
			i := c.emit(st)
			if p.Quirk == QuirkRelay && p.Effect != EffectPush {
				if err := c.relay(st); err != nil {
					return err
				}
			}
			fed[s] = span{i, len(c.p.Steps), true}
			c.cost(p, depth)
		case sl.Kind == SlotNested:
			n := sl.Nested
			if seen[n] {
				return errors.New("hex nests itself")
			}
			src, err := c.feed(h, s, tgt, landing)
			if err != nil {
				return err
			}
			r, err := c.alloc()
			if err != nil {
				return err
			}
			at := c.emit(Step{Op: OpForEach, Src: src, Dst: r})
			seen[n] = true
			if err := c.level(n, 0, depth+1, r, r, seen); err != nil {
				return err
			}
			delete(seen, n)
			body := len(c.p.Steps) - at - 1
			if body > 0xffff {
				return errors.New("nested hex too large")
			}
			c.p.Steps[at].Span = uint16(body)
			fed[s] = span{at, len(c.p.Steps), true}
		}
	}

	// Pass 3: multipliers patch the steps of the part before them.
	var mult [Slots]span
	for k, s := range order {
		sl := h.Slots[s]
		if sl.Kind != SlotPart || sl.Part.Kind != KindMult {
			continue
		}
		p := sl.Part
		c.cost(p, depth)
		if p.Quirk == QuirkRadiant {
			// Bends the "part just before it" rule: the whole ring.
			f := span{first, len(c.p.Steps), len(c.p.Steps) > first}
			mult[s] = f
			if f.ok {
				c.scale(f, p.Verb, 1+(p.Value-1)*p.Rarity.Scale())
			}
			continue
		}
		if k == 0 {
			continue
		}
		pred := order[k-1]
		if !h.adjacent(pred, s) {
			continue
		}
		f := fed[pred]
		if ps := h.Slots[pred]; ps.Kind == SlotPart && ps.Part.Kind == KindMult {
			f = mult[pred]
		}
		if !f.ok {
			continue
		}
		mult[s] = f
		c.scale(f, p.Verb, 1+(p.Value-1)*p.Rarity.Scale())
	}
	return nil
}

// relay follows an effect step with its arc: for each entity the effect
// landed on, select what's next to it and apply the effect again at
// RelayFrac. The arc is part of the slot's span, so a multiplier after it
// scales the arc too.
func (c *compiler) relay(st Step) error {
	loop, err := c.alloc()
	if err != nil {
		return err
	}
	near, err := c.alloc()
	if err != nil {
		return err
	}
	c.emit(Step{Op: OpForEach, Src: st.Src, Dst: loop, Span: 2})
	c.emit(Step{Op: OpSelect, Target: TargetAdjacent, Src: loop, Dst: near, Value: 1})
	arc := st
	arc.Src = near
	if arc.Op == OpDamage {
		arc.Value *= RelayFrac
	}
	c.emit(arc)
	return nil
}

// feed returns the register an effect or nested hex in slot s lands on.
func (c *compiler) feed(h *Hex, s int, tgt [Slots]int, landing uint8) (uint8, error) {
	ccw, cw := (s+Slots-1)%Slots, (s+1)%Slots
	a, b := -1, -1
	if h.adjacent(ccw, s) {
		a = tgt[ccw]
	}
	if h.adjacent(s, cw) {
		b = tgt[cw]
	}
	switch {
	case a >= 0 && b >= 0:
		r, err := c.alloc()
		if err != nil {
			return 0, err
		}
		c.emit(Step{Op: OpUnion, Src: uint8(a), Src2: uint8(b), Dst: r})
		return r, nil
	case a >= 0:
		return uint8(a), nil
	case b >= 0:
		return uint8(b), nil
	}
	return landing, nil
}

func (c *compiler) scale(f span, v Verb, x float32) {
	all := v == VerbAll
	for i := f.lo; i < f.hi; i++ {
		st := &c.p.Steps[i]
		switch st.Op {
		case OpDamage:
			if all || v == VerbDamage {
				st.Value *= x
			}
		case OpPush:
			if all || v == VerbPush {
				st.Value *= x
			}
		case OpStatus:
			if all || v == VerbDuration {
				st.Value *= x
			}
		case OpSelect:
			if (all || v == VerbCount) && (st.Target == TargetNearest || st.Target == TargetChain) {
				st.Count = clampCount(int(float32(st.Count)*x + 0.5))
			}
			if (all || v == VerbRadius) && st.Target != TargetSelf && st.Target != TargetNearest {
				st.Value *= x
			}
		}
	}
}

func clampCount(n int) uint8 {
	switch {
	case n < 1:
		return 1
	case n > MaxTargets:
		return MaxTargets
	}
	return uint8(n)
}

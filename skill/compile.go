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
//   - A gate splits a group in two and routes each half one way (see
//     Hex.Routes for which slots). The group is the targets in the slot
//     counter-clockwise before it if linked, else the trigger's subject (or
//     the entity a nested hex runs on); a gate in another gate's branch
//     takes that branch's half instead. The yes slot runs on the ones that
//     passed, the no slot on the rest. Effects and nested hexes in a
//     branch land only on their half, never on the targets beside them.
//     Targets and multipliers in a branch slot work as they would anywhere.
//   - A charge adds to (or drains) its bank once each time the ring runs
//     through its slot, in clockwise order with everything else, so a
//     charge before a spend gate counts before the gate reads. In a gate
//     branch it only counts when its half holds someone. Banks belong to
//     the equipped hex and keep their counts between firings; nested
//     hexes share them by letter. An echo hex counts twice.
//
// The hex's shape sets the order and who feeds whom (see Shape): the
// "clockwise" and "beside it" above follow the shape's flow and its
// Feeds, a Hub's center pools the whole ring, and a Loop runs again on
// what its last slot carries.
func Compile(h *Hex, reg *Registry, maxDepth int) (*Program, error) {
	shape := h.Shape
	if !shape.Valid() {
		return nil, fmt.Errorf("hex has a broken shape (%v)", shape)
	}
	trig := -1
	for i, s := range h.Slots {
		if s.Kind == SlotEmpty {
			continue
		}
		if !shape.Has(i) {
			return nil, fmt.Errorf("slot %d isn't part of a %s", i+1, shape.Name())
		}
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
	switch {
	case shape.Kind == ShapeFork && trig != Center:
		return nil, errors.New("a fork's trigger sits in its center")
	case shape.Kind.Legacy() && shape.Kind != ShapeFork && trig == Center:
		return nil, errors.New("only a fork has its trigger in the center")
	case shape.Kind == ShapeGen && trig != shape.Entry():
		return nil, fmt.Errorf("this %s's trigger sits in slot %d", shape.Name(), shape.Entry()+1)
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
	if c.loop {
		// The Loop's back edge: run the hex again on whoever the last
		// slot carried, after this pass, through the same queue as a self
		// Call (so it pays Heat and counts against MaxCalls).
		c.emit(Step{Op: OpCall, Src: c.loopSrc, Count: CallSelf, Value: 1})
		c.p.Calls |= 1 << CallSelf
		c.p.Loops = true
	}
	if c.p.Heat > 0 {
		c.p.Heat *= shape.HeatScale()
		if shape.Quirk == ShapeLean {
			c.p.Heat *= LeanHeat
		}
	}
	c.p.Shape = shape.Kind
	c.p.Regs = int(c.next)
	return c.p, nil
}

type compiler struct {
	reg  *Registry
	trig Part
	p    *Program
	next uint8
	// loop is set when a Loop's last slot carries someone back to its
	// first: loopSrc is the register holding them.
	loop    bool
	loopSrc uint8
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

// useBank notes a bank the program touches, and the first spend on it.
func (c *compiler) useBank(p Part) {
	b := p.Bank % MaxBanks
	c.p.Banks |= 1 << b
	if p.Kind == KindGate && p.Gate == GateSpend && c.p.Spend[b] == 0 {
		c.p.Spend[b] = p.Count
	}
}

func (c *compiler) cost(p Part, depth int) {
	f := float32(1 + NestCostPerDepth*float64(depth))
	heat := p.Heat
	if p.Kind == KindEffect && p.Effect != EffectPush {
		heat += c.reg.Get(p.Type).HeatOffset
	}
	power := p.Power
	if p.Kind == KindGate || p.Kind == KindCharge {
		power /= p.Rarity.Scale() // rarer gates and charges run leaner
	}
	switch p.Quirk {
	case QuirkRelay:
		f *= 1 + RelayCost
	case QuirkRadiant:
		heat += RadiantHeatCost
	}
	c.p.Power += power * f
	c.p.Heat += heat * f
}

type span struct {
	lo, hi int
	ok     bool
}

// level compiles one hex starting at slot start, in its shape's flow.
// Targets select around the entity in origin; effects with no target
// beside them land on the entities in landing. In a nested hex both are
// the entity it runs on.
func (c *compiler) level(h *Hex, start, depth int, origin, landing uint8, seen map[*Hex]bool) error {
	order, n := h.Flow(start)
	shape := h.Shape
	twin, surge := shape.Twin(), shape.Surge()
	var tgt [Slots]int
	for i := range tgt {
		tgt[i] = -1
	}
	var fed [Slots]span  // step range each slot produced, for multipliers
	var carry [Slots]int // register each slot carries on, for a Loop
	for i := range carry {
		carry[i] = -1
	}
	first := len(c.p.Steps)
	routes := h.Routes(start)
	var pass, fail [Slots]uint8 // each gate's two halves
	// half is the register a slot in a gate's branch works on.
	half := func(r Route) uint8 {
		if r.Branch == BranchYes {
			return pass[r.Gate]
		}
		return fail[r.Gate]
	}
	// land is where an effect or nested hex in slot s lands: its gate's
	// half if it sits in a branch, else the targets in every slot that
	// feeds it (both neighbors on a ring, the whole ring for a Hub's
	// center, every input of a merge).
	land := func(s int) (src, guard uint8, err error) {
		if r := routes[s]; r.Gate >= 0 {
			g := half(r)
			return g, g, nil
		}
		src, err = c.feed(h, s, order[:n], tgt, landing)
		return src, RegCaster, err
	}

	// Pass 1: targets. They depend on nothing but the origin, so they all
	// select up front and any effect can read them.
	for _, s := range order[:n] {
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
		carry[s] = int(r)
		fed[s] = span{i, i + 1, true}
		c.cost(p, depth)
		if s == surge {
			c.scale(fed[s], VerbAll, SurgeScale)
		}
	}

	// Pass 2: effects, gates, charges, calls and nested hexes, in flow.
	for _, s := range order[:n] {
		sl := h.Slots[s]
		at := len(c.p.Steps)
		pw, ht := c.p.Power, c.p.Heat
		switch {
		case sl.Kind == SlotPart && sl.Part.Kind == KindTrigger:
			if depth > 0 {
				return errors.New("nested hex can't have its own trigger")
			}
		case sl.Kind == SlotPart && sl.Part.Kind == KindGate:
			p := sl.Part
			var in uint8
			if r := routes[s]; r.Gate >= 0 {
				in = half(r)
			} else {
				in = landing
				if pred := h.inFeed(s, order[:n]); pred >= 0 && tgt[pred] >= 0 {
					in = uint8(tgt[pred])
				}
			}
			yes, err := c.alloc()
			if err != nil {
				return err
			}
			no, err := c.alloc()
			if err != nil {
				return err
			}
			pass[s], fail[s] = yes, no
			i := c.emit(Step{Op: OpGate, Gate: p.Gate, Src: in, Dst: yes, Dst2: no,
				Type: p.Type, Count: p.Count, Value: p.Value, Bank: p.Bank % MaxBanks})
			fed[s] = span{i, i + 1, true}
			carry[s] = int(yes)
			c.cost(p, depth)
			if p.Gate.Charged() {
				c.useBank(p)
			}
		case sl.Kind == SlotPart && sl.Part.Kind == KindCharge:
			// A charge runs once per pass through its slot. In a branch it
			// runs only when its half holds someone; outside one, always.
			p := sl.Part
			guard := uint8(RegCaster)
			if r := routes[s]; r.Gate >= 0 {
				guard = half(r)
				carry[s] = int(guard)
			}
			v := p.Value
			if v <= 0 {
				v = 1
			}
			if p.Charge == ChargeDrain {
				v = -v
			}
			i := c.emit(Step{Op: OpCharge, Src: guard, Src2: guard, Bank: p.Bank % MaxBanks, Value: v})
			fed[s] = span{i, i + 1, true}
			c.cost(p, depth)
			c.useBank(p)
		case sl.Kind == SlotPart && sl.Part.Kind == KindCall:
			p := sl.Part
			src, _, err := land(s)
			if err != nil {
				return err
			}
			i := c.emit(Step{Op: OpCall, Src: src, Count: p.Call, Value: 1 / p.Rarity.Scale()})
			fed[s] = span{i, i + 1, true}
			carry[s] = int(src)
			c.cost(p, depth)
			if p.Call < 16 {
				c.p.Calls |= 1 << p.Call
			}
		case sl.Kind == SlotPart && sl.Part.Kind == KindEffect:
			p := sl.Part
			src, guard, err := land(s)
			if err != nil {
				return err
			}
			// Src2 guards effects that ignore Src (a cloak): they only run
			// while the guard register holds someone. Outside a branch the
			// guard is the caster, who is always there.
			st := Step{Src: src, Src2: guard, Type: p.Type, Value: p.Value * p.Rarity.Scale()}
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
			carry[s] = int(src)
			c.cost(p, depth)
		case sl.Kind == SlotNested:
			nh := sl.Nested
			if seen[nh] {
				return errors.New("hex nests itself")
			}
			src, _, err := land(s)
			if err != nil {
				return err
			}
			r, err := c.alloc()
			if err != nil {
				return err
			}
			at := c.emit(Step{Op: OpForEach, Src: src, Dst: r})
			seen[nh] = true
			if err := c.level(nh, nh.entry(), depth+1, r, r, seen); err != nil {
				return err
			}
			delete(seen, nh)
			body := len(c.p.Steps) - at - 1
			if body > 0xffff {
				return errors.New("nested hex too large")
			}
			c.p.Steps[at].Span = uint16(body)
			fed[s] = span{at, len(c.p.Steps), true}
			carry[s] = int(src)
		}
		if !fed[s].ok || fed[s].lo < at {
			continue // targets (pass 1) and empty slots
		}
		if s == twin {
			// Fires twice: the slot's steps again, right after, and its
			// cost again. Spans inside are relative, so a copy works.
			lo := fed[s].lo
			c.p.Steps = append(c.p.Steps, c.p.Steps[lo:]...)
			fed[s].hi = len(c.p.Steps)
			c.p.Power += c.p.Power - pw
			c.p.Heat += c.p.Heat - ht
		}
		if s == surge {
			c.scale(fed[s], VerbAll, SurgeScale)
		}
	}

	// Pass 3: multipliers patch the steps of the part before them.
	var mult [Slots]span
	// resolved is the span a slot's output stands for: its own steps, or
	// for a multiplier, the steps it scaled.
	resolved := func(a int) span {
		if ps := h.Slots[a]; ps.Kind == SlotPart && ps.Part.Kind == KindMult {
			return mult[a]
		}
		return fed[a]
	}
	for _, s := range order[:n] {
		sl := h.Slots[s]
		if sl.Kind != SlotPart || sl.Part.Kind != KindMult {
			continue
		}
		p := sl.Part
		times := 1
		if s == twin {
			times = 2
		}
		bonus := (p.Value - 1) * p.Rarity.Scale()
		if s == surge {
			bonus *= SurgeScale
		}
		x := 1 + bonus
		for range times {
			c.cost(p, depth)
		}
		if p.Quirk == QuirkRadiant {
			// Bends the "part just before it" rule: the whole ring.
			f := span{first, len(c.p.Steps), len(c.p.Steps) > first}
			mult[s] = f
			for range times {
				if f.ok {
					c.scale(f, p.Verb, x)
				}
			}
			continue
		}
		if h.merge(s) {
			// A merge is fed by every slot linking into it, so it scales
			// them all (a Hub's center: the whole ring), each step once.
			lo, hi := len(c.p.Steps), 0
			var done []bool
			for _, a := range order[:n] {
				if a == s || h.Shape.Graph().Out[a]&bit(s) == 0 || !h.Feeds(a, s) {
					continue
				}
				f := resolved(a)
				if !f.ok {
					continue
				}
				if done == nil {
					done = make([]bool, len(c.p.Steps))
				}
				for i := f.lo; i < f.hi; i++ {
					if done[i] {
						continue
					}
					done[i] = true
					for range times {
						c.scale(span{i, i + 1, true}, p.Verb, x)
					}
				}
				lo, hi = min(lo, f.lo), max(hi, f.hi)
			}
			mult[s] = span{lo, hi, hi > lo}
			continue
		}
		pred := h.inFeed(s, order[:n])
		if pred < 0 {
			continue
		}
		f := resolved(pred)
		if !f.ok {
			continue
		}
		mult[s] = f
		for range times {
			c.scale(f, p.Verb, x)
		}
	}

	// A Loop's last slot feeds back into its first: note what it carries
	// so Compile can add the back edge. Only the top hex loops.
	if depth == 0 {
		if last := h.loopSlot(start); last >= 0 {
			if r := carry[last]; r >= 0 && r != RegCaster {
				c.loop, c.loopSrc = true, uint8(r)
			}
		}
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

// feed returns the register an effect or nested hex in slot s lands on:
// the targets in every slot that feeds it, gathered with no one twice.
// With none, it's the landing.
func (c *compiler) feed(h *Hex, s int, order []int, tgt [Slots]int, landing uint8) (uint8, error) {
	acc := -1
	for _, a := range order {
		if tgt[a] < 0 || !h.Feeds(a, s) {
			continue
		}
		if acc < 0 {
			acc = tgt[a]
			continue
		}
		r, err := c.alloc()
		if err != nil {
			return 0, err
		}
		c.emit(Step{Op: OpUnion, Src: uint8(acc), Src2: uint8(tgt[a]), Dst: r})
		acc = int(r)
	}
	if acc < 0 {
		return landing, nil
	}
	return uint8(acc), nil
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

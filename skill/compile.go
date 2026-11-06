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
		return nil, errBrokenShape(shape)
	}
	trig := -1
	for i, s := range h.Slots {
		if s.Kind == SlotEmpty {
			continue
		}
		if !shape.Has(i) {
			return nil, errNotInShape(i, shape)
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
	hair := h.Frame(trig).Bend == BendHair
	if hair {
		// A Hair trigger frame: faster procs and periods, a hotter hex.
		c.p.Haste = 1 / HairHaste
		if t.Trigger == Periodic {
			c.p.Period /= HairHaste
		}
	}
	seen := map[*Hex]bool{h: true}
	landing := uint8(RegSubject)
	if t.Trigger == Watch {
		var err error
		if landing, err = c.watch(h, trig, t, seen); err != nil {
			return nil, err
		}
	}
	// A placed form in the hex makes it a device: find it first, so an
	// aura in it centers on the device.
	for _, i := range func() []int { o, n := h.Flow(trig); return o[:n] }() {
		if sl := h.Slots[i]; sl.Kind == SlotPart && sl.Part.Kind == KindForm && sl.Part.Form.Placed() {
			c.device, c.deviceRule = uint8(sl.Part.Form)+1, sl.Part.Rule()
			break
		}
	}
	if err := c.level(h, trig, 0, RegOrigin, landing, seen); err != nil {
		return nil, err
	}
	if c.device != 0 {
		// Everything after the watch check runs from the device, not here.
		at := c.p.WatchSteps
		body := len(c.p.Steps) - at
		if body > 0xffff {
			return nil, errors.New("device hex too large")
		}
		if FormKind(c.device-1) == FormZone {
			c.scale(span{at, len(c.p.Steps), body > 0}, VerbDamage, ZoneScale)
		}
		c.p.Steps = append(c.p.Steps, Step{})
		copy(c.p.Steps[at+1:], c.p.Steps[at:])
		c.p.Steps[at] = Step{Op: OpDeploy, Count: c.device - 1, Bank: c.deviceRule, Span: uint16(body)}
		c.p.Device = c.device
	}
	if t.Quirk == QuirkEcho {
		// Run the whole body twice. Spans are relative, so a plain copy
		// works; the second pass selects its targets afresh.
		c.p.Steps = append(c.p.Steps, c.p.Steps...)
		c.p.Power *= 1 + EchoCost
		c.p.Heat *= 1 + EchoCost
		c.dockHeat *= 1 + EchoCost
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
		if hair {
			c.p.Heat *= HairHeat
		}
	}
	c.p.Heat += c.dockHeat
	if c.overflow {
		c.p.LockExtra = OverflowLock
	}
	if c.pay != nil {
		switch c.pay.Cost {
		case CostBlood:
			if c.p.Heat > 0 {
				c.p.HullCost = c.p.Heat * BloodPerHeat
				c.p.Heat = 0
			}
		case CostWrecks:
			c.p.WreckCost = WreckCostN
			c.p.Heat = min(c.p.Heat, 0)
		}
	}
	if c.stance != nil {
		c.p.Stance = uint8(c.stance.Stance) + 1
		c.p.StanceKeep = c.stance.Rarity >= Rare
		c.p.Pulses = acts(c.p.Steps[c.p.WatchSteps:])
	}
	c.p.Inert = !acts(c.p.Steps[c.p.WatchSteps:]) && c.p.Stance == 0
	c.p.Docked = uint8(c.docked)
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
	// docking is how deep in a docked chain the hex being compiled sits
	// (0 for the hex that fired); docked counts docked hexes run so far.
	// dockHeat is their Heat, already on each one's own shape scale.
	docking  int
	docked   int
	dockHeat float32
	// calling is how deep in run-now calls the hex being compiled sits.
	// ret is what the hex level just compiled carries out (see level).
	calling int
	ret     uint8
	// overflow is set once an Overflow frame is compiled in.
	overflow bool
	// device is 1 + the placed form the hex leaves (zero for none), at
	// rule deviceRule. back is the move a rare dash makes at the end.
	device     uint8
	deviceRule uint8
	back       MoveMode
	backSet    bool
	// pay is the hex's first cost part, stance its first stance part.
	pay    *Part
	stance *Part
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
	b := p.Bank % NumBanks
	c.p.Banks |= 1 << b
	if p.Kind == KindGate && p.Gate == GateSpend && c.p.Spend[b] == 0 {
		c.p.Spend[b] = p.Count
	}
}

func (c *compiler) cost(p Part, depth int) {
	f := float32(1 + NestCostPerDepth*float64(depth))
	heat := p.Heat
	if p.Kind == KindEffect && p.Effect.Typed() {
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
	h = echoes(h, start) // Echo frames run a copy of the slot that feeds them
	order, n := h.Flow(start)
	var frames [Slots]Frame
	for _, s := range order[:n] {
		frames[s] = h.Frame(s)
	}
	ptype, prismed := prism(h, order[:n])
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
	var allies [Slots]int // an aura's allies, for Mend and Shield beside it
	for i := range allies {
		allies[i] = -1
	}
	for _, s := range order[:n] {
		sl := h.Slots[s]
		if sl.Kind != SlotPart || (sl.Part.Kind != KindTarget && sl.Part.Kind != KindForm) {
			continue
		}
		p := sl.Part
		fr := frames[s]
		pw, ht := c.p.Power, c.p.Heat
		if p.Kind == KindForm {
			i := len(c.p.Steps)
			r, al, err := c.form(p, origin, depth)
			if err != nil {
				return err
			}
			tgt[s], carry[s], allies[s] = int(r), int(r), al
			fed[s] = span{i, len(c.p.Steps), true}
			c.cost(p, depth)
			if fr.Bend == BendSurge {
				c.scale(fed[s], VerbAll, SurgeScale)
			}
			c.frameCost(fr, pw, ht)
			continue
		}
		r, err := c.alloc()
		if err != nil {
			return err
		}
		src := origin
		if p.Target == TargetSelf {
			src = RegCaster
		}
		st := Step{Op: OpSelect, Target: p.Target, Src: src, Dst: r,
			Count: clampCount(int(p.Count)), Value: p.Value}
		if fr.Bend == BendCrowd && (p.Target == TargetNearest || p.Target == TargetChain) {
			st.Flag |= FlagCrowd
		}
		i := c.emit(st)
		tgt[s] = int(r)
		carry[s] = int(r)
		fed[s] = span{i, i + 1, true}
		c.cost(p, depth)
		if fr.Bend == BendSurge {
			c.scale(fed[s], VerbAll, SurgeScale)
		}
		c.frameCost(fr, pw, ht)
	}

	// Pass 2: effects, gates, charges, calls and nested hexes, in flow.
	for _, s := range order[:n] {
		sl := h.Slots[s]
		fr := frames[s]
		if prismed[s] {
			sl.Part.Type = ptype[s]
		}
		at := len(c.p.Steps)
		pw, ht, dh := c.p.Power, c.p.Heat, c.dockHeat
		switch {
		case sl.Kind == SlotPart && sl.Part.Kind == KindTrigger:
			if depth > 0 {
				return errors.New("nested hex can't have its own trigger")
			}
		case sl.Kind == SlotPart && sl.Part.Kind == KindGate:
			p := sl.Part
			// A gate tests what the slot before it carries: a target's
			// group, who an effect, call or nested hex landed on, another
			// gate's YES half. With nothing carried, the landing.
			var in uint8
			if r := routes[s]; r.Gate >= 0 {
				in = half(r)
			} else {
				in = landing
				if pred := h.inFeed(s, order[:n]); pred >= 0 && carry[pred] >= 0 {
					in = uint8(carry[pred])
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
			count := p.Count
			if p.Gate == GateCond {
				cp, err := p.Cond.Compile()
				if err != nil {
					return fmt.Errorf("slot %s's condition: %w", h.SlotName(s), err)
				}
				if len(c.p.Conds) >= 255 {
					return errors.New("too many conditions")
				}
				count = uint8(len(c.p.Conds))
				c.p.Conds = append(c.p.Conds, cp)
				c.p.Heat += cp.Heat() * float32(1+NestCostPerDepth*float64(depth))
				for _, t := range p.Cond.Tiles() {
					if t.K == TileBank {
						c.useBank(Part{Kind: KindCharge, Bank: t.B})
					}
				}
			}
			i := c.emit(Step{Op: OpGate, Gate: p.Gate, Src: in, Dst: yes, Dst2: no,
				Type: p.Type, Count: count, Value: p.Value, Bank: p.Bank % NumBanks})
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
			i := c.emit(Step{Op: OpCharge, Src: guard, Src2: guard, Bank: p.Bank % NumBanks, Value: v})
			fed[s] = span{i, i + 1, true}
			c.cost(p, depth)
			c.useBank(p)
		case sl.Kind == SlotPart && sl.Part.RunsNow():
			// A run-now call: the library hex runs here, inline, and the
			// slot then holds what it returns, like a target holds its
			// group, so the slots after it can act on it.
			p := sl.Part
			src, _, err := land(s)
			if err != nil {
				return err
			}
			c.cost(p, depth)
			ret := src
			if lib := h.Libs[s]; lib != nil {
				if ret, err = c.callLib(lib, p, origin, src, depth, seen); err != nil {
					return err
				}
			}
			fed[s] = span{at, len(c.p.Steps), len(c.p.Steps) > at}
			carry[s], tgt[s] = int(ret), int(ret)
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
		case sl.Kind == SlotPart && sl.Part.Kind == KindCost:
			p := sl.Part
			c.cost(p, depth)
			if c.pay == nil {
				c.pay = &p
			}
		case sl.Kind == SlotPart && sl.Part.Kind == KindStance:
			p := sl.Part
			c.cost(p, depth)
			if c.stance == nil {
				c.stance = &p
			}
		case sl.Kind == SlotPart && sl.Part.Kind == KindEffect && sl.Part.Effect > EffectShield:
			p := sl.Part
			src, guard, err := land(s)
			if err != nil {
				return err
			}
			st := Step{Src: src, Src2: guard, Value: p.Value * p.Rarity.Scale()}
			switch p.Effect {
			case EffectTaunt:
				st.Op = OpTaunt
			case EffectThorns, EffectHaste:
				st.Op, st.Src, st.Count = OpBuff, RegCaster, uint8(p.Effect)
			case EffectReanimate:
				st.Op = OpRaise
				if guard == RegCaster && src == landing && (depth > 0 || (c.trig.Trigger != OnKill && c.trig.Trigger != OnWreck)) {
					// Nothing beside it: the nearest wreck.
					r, err := c.alloc()
					if err != nil {
						return err
					}
					c.emit(Step{Op: OpSelect, Target: TargetWreck, Src: origin, Dst: r, Count: 1, Value: WreckReach})
					st.Src = r
				}
			case EffectBlink:
				st.Op, st.Count = OpMove, uint8(MoveToward)
				if guard == RegCaster && src == landing {
					st.Count = uint8(MoveAway)
				}
			}
			i := c.emit(st)
			fed[s] = span{min(i, at), len(c.p.Steps), true}
			carry[s] = int(src)
			c.cost(p, depth)
		case sl.Kind == SlotPart && sl.Part.Kind == KindEffect:
			// Cloak is a self effect: an old cloak damage part runs as the
			// cloak status it reads as now.
			p, _ := c.reg.SelfOnly(sl.Part)
			src, guard, err := land(s)
			if err != nil {
				return err
			}
			// Src2 guards effects that ignore Src (a cloak): they only run
			// while the guard register holds someone. Outside a branch the
			// guard is the caster, who is always there.
			st := Step{Src: src, Src2: guard, Type: p.Type, Value: p.Value * p.Rarity.Scale()}
			if p.Effect.Sustain() {
				// Mend and Shield land on the targets beside them, or the
				// caster with none; in a branch, on the caster when the
				// branch holds anyone. Purge only ever cools the rig.
				if a := c.alliesOf(h, s, order[:n], allies); a >= 0 && guard == RegCaster && p.Effect != EffectPurge {
					st.Src = uint8(a) // beside an aura: you and your allies
				} else if guard != RegCaster || src == landing || p.Effect == EffectPurge {
					st.Src = RegCaster
				}
				st.Type = 0
				switch p.Effect {
				case EffectMend:
					st.Op = OpMend
				case EffectShield:
					st.Op = OpShield
				default:
					st.Op = OpPurge
				}
				i := c.emit(st)
				fed[s] = span{i, i + 1, true}
				carry[s] = int(src)
				c.cost(p, depth)
				break
			}
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
			switch fr.Bend {
			case BendLeech:
				if st.Op == OpDamage {
					st.Value *= 1 - LeechCut
					st.Flag |= FlagLeech
				}
			case BendLonely:
				st.Flag |= FlagLonely
			}
			i := c.emit(st)
			if p.Quirk == QuirkRelay && p.Effect != EffectPush {
				if err := c.relay(st); err != nil {
					return err
				}
			}
			if p.Quirk == QuirkRider {
				c.rider(st)
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
		switch fr.Bend {
		case BendTwin:
			// Fires twice: the slot's steps again, right after, and its
			// cost again. Spans inside are relative, so a copy works.
			lo := fed[s].lo
			c.p.Steps = append(c.p.Steps, c.p.Steps[lo:]...)
			fed[s].hi = len(c.p.Steps)
			c.p.Power += c.p.Power - pw
			c.p.Heat += c.p.Heat - ht
			c.dockHeat += c.dockHeat - dh
		case BendDoubler:
			// Twice, but the Power once: only the Heat is paid again.
			lo := fed[s].lo
			c.p.Steps = append(c.p.Steps, c.p.Steps[lo:]...)
			fed[s].hi = len(c.p.Steps)
			c.p.Heat += c.p.Heat - ht
			c.dockHeat += c.dockHeat - dh
		case BendKillLock:
			// The copy waits behind a check: only a run that has killed
			// by now gets it. No extra cost.
			lo, hi := fed[s].lo, len(c.p.Steps)
			if hi-lo > 0xffff {
				return errors.New("kill-lock part too large")
			}
			c.emit(Step{Op: OpIfKill, Span: uint16(hi - lo)})
			c.p.Steps = append(c.p.Steps, c.p.Steps[lo:hi]...)
			fed[s].hi = len(c.p.Steps)
		case BendSurge:
			c.scale(fed[s], VerbAll, SurgeScale)
		}
		c.frameCost(fr, pw, ht)
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
		fr := frames[s]
		times := 1
		if fr.Bend == BendTwin {
			times = 2
		}
		bonus := (p.Value - 1) * p.Rarity.Scale()
		if fr.Bend == BendSurge {
			bonus *= SurgeScale
		}
		x := 1 + bonus
		pw, ht := c.p.Power, c.p.Heat
		for range times {
			c.cost(p, depth)
		}
		c.frameCost(fr, pw, ht)
		c.overflow = c.overflow || fr.Bend == BendOverflow
		over := fr.Bend == BendOverflow || p.Quirk == QuirkHeated
		if p.Quirk == QuirkRadiant {
			// Bends the "part just before it" rule: the whole ring.
			f := span{first, len(c.p.Steps), len(c.p.Steps) > first}
			mult[s] = f
			for range times {
				if f.ok {
					c.scale(f, p.Verb, x)
				}
			}
			if over && f.ok {
				c.flag(f, p.Verb, FlagOverflow)
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
					if over {
						c.flag(span{i, i + 1, true}, p.Verb, FlagOverflow)
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
		if over {
			c.flag(f, p.Verb, FlagOverflow)
		}
	}

	// A rare dash carries you back once the hex is done.
	if depth == 0 && c.calling == 0 && c.docking == 0 && c.backSet {
		c.emit(Step{Op: OpMove, Src: RegCaster, Src2: RegCaster, Count: uint8(c.back)})
		c.backSet = false
	}

	// Hexes docked to its ways out run next, on what the port carries.
	// Only board hexes dock, never ones nested in a slot.
	if depth == 0 && c.calling == 0 {
		if err := c.dock(h, order[:n], carry, landing, seen); err != nil {
			return err
		}
	}

	// A Loop's last slot feeds back into its first: note what it carries
	// so Compile can add the back edge. Only the hex that fired loops.
	if depth == 0 && c.docking == 0 && c.calling == 0 {
		if last := h.loopSlot(start); last >= 0 {
			if r := carry[last]; r >= 0 && r != RegCaster {
				c.loop, c.loopSrc = true, uint8(r)
			}
		}
	}

	// What the hex returns when called: what the last slot in its flow
	// that carries anyone carries. With none, the group it was handed.
	c.ret = landing
	for k := n - 1; k >= 0; k-- {
		if r := carry[order[k]]; r >= 0 {
			c.ret = uint8(r)
			break
		}
	}
	return nil
}

// callLib compiles a run-now call to library hex lib inline. Its targets
// select around the caller's origin, and its effects with nothing feeding
// them land on src, the group the call landed on. The call's options
// scale its numbers. Its parts pay no Power, only Heat, on its own shape's
// scale, times the call's rarity factor. It returns the register holding
// what the library returns.
func (c *compiler) callLib(lib *Hex, p Part, origin, src uint8, depth int, seen map[*Hex]bool) (uint8, error) {
	if seen[lib] {
		return 0, fmt.Errorf("%s calls a hex that's already running", lib.Name)
	}
	if !lib.Shape.Valid() {
		return 0, errBrokenShape(lib.Shape)
	}
	for i, sl := range lib.Slots {
		if sl.Kind == SlotPart && sl.Part.Kind == KindTrigger && lib.Shape.Has(i) {
			return 0, fmt.Errorf("library %s has a trigger", lib.Name)
		}
	}
	if c.calling >= MaxCallDepth {
		return 0, fmt.Errorf("calls nest more than %d deep", MaxCallDepth)
	}
	pw, ht := c.p.Power, c.p.Heat
	at := len(c.p.Steps)
	seen[lib] = true
	c.calling++
	err := c.level(lib, lib.entry(), depth, origin, src, seen)
	c.calling--
	delete(seen, lib)
	if err != nil {
		return 0, fmt.Errorf("call %s: %w", lib.Name, err)
	}
	ret := c.ret
	if f := (span{at, len(c.p.Steps), len(c.p.Steps) > at}); f.ok {
		for _, o := range p.Opts {
			if o.Value != 0 {
				c.scale(f, o.Verb, 1+(o.Value-1)*o.Rarity.Scale())
			}
		}
	}
	d := c.p.Heat - ht
	c.p.Power, c.p.Heat = pw, ht
	if d > 0 {
		d *= lib.Shape.HeatScale()
		if lib.Shape.Quirk == ShapeLean {
			d *= LeanHeat
		}
	}
	c.dockHeat += d / p.Rarity.Scale()
	for _, o := range p.Opts {
		if o.Value != 0 {
			c.cost(o.Part(), depth)
		}
	}
	return ret, nil
}

// watch compiles a watch trigger's check at the front of the program and
// returns where the hex's effects land when it fires: who the check
// found, or the usual subject when the check only read the rig. The
// check costs nothing here; the dispatcher charges WatchHeat for each.
func (c *compiler) watch(h *Hex, trig int, t Part, seen map[*Hex]bool) (uint8, error) {
	pw, ht, dh := c.p.Power, c.p.Heat, c.dockHeat
	defer func() { c.p.Power, c.p.Heat, c.dockHeat = pw, ht, dh }()
	switch {
	case t.Lib != "":
		lib := h.Libs[trig]
		if lib == nil {
			return RegSubject, nil // watches nothing until the library is on the board
		}
		if why := finderOnly(lib); why != "" {
			return 0, fmt.Errorf("a watched library can only pick and test foes, and %s %s", lib.Name, why)
		}
		ret, err := c.callLib(lib, Part{Kind: KindCall, Lib: t.Lib}, RegOrigin, RegSubject, 0, seen)
		if err != nil {
			return 0, err
		}
		c.p.WatchSteps, c.p.WatchRet = len(c.p.Steps), ret
		return ret, nil
	case t.Cond.Len() > 0:
		cp, err := t.Cond.Compile()
		if err != nil {
			return 0, fmt.Errorf("the watch's condition: %w", err)
		}
		src := uint8(RegCaster)
		if cp.Foe {
			if src, err = c.alloc(); err != nil {
				return 0, err
			}
			c.emit(Step{Op: OpSelect, Target: TargetAdjacent, Src: RegOrigin, Dst: src, Value: WatchRange})
		}
		yes, err := c.alloc()
		if err != nil {
			return 0, err
		}
		no, err := c.alloc()
		if err != nil {
			return 0, err
		}
		for _, tl := range t.Cond.Tiles() {
			if tl.K == TileBank {
				c.useBank(Part{Kind: KindCharge, Bank: tl.B})
			}
		}
		c.emit(Step{Op: OpGate, Gate: GateCond, Src: src, Dst: yes, Dst2: no, Count: uint8(len(c.p.Conds))})
		c.p.Conds = append(c.p.Conds, cp)
		c.p.WatchSteps, c.p.WatchRet = len(c.p.Steps), yes
		if cp.Foe {
			return yes, nil
		}
		return RegSubject, nil
	}
	return RegSubject, nil
}

// acts reports whether steps do anything when they run: hit, push,
// afflict, charge a bank or call a hex. An equipped hex that doesn't act
// is Inert and won't fire, so it never spends Power for nothing.
func acts(steps []Step) bool {
	for _, s := range steps {
		switch s.Op {
		case OpDamage, OpPush, OpStatus, OpCharge, OpCall, OpMend, OpShield, OpPurge,
			OpTaunt, OpBuff, OpRaise, OpMove, OpConsume:
			return true
		}
	}
	return false
}

// finderOnly says what in a library hex does more than pick and test
// foes, or "" if nothing does: a watch runs its library on every check.
func finderOnly(lib *Hex) string {
	for i, sl := range lib.Slots {
		if !lib.Shape.Has(i) {
			continue
		}
		switch {
		case sl.Kind == SlotNested:
			return "holds a nested hex in slot " + lib.SlotName(i)
		case sl.Kind != SlotPart:
		case sl.Part.Kind == KindEffect:
			return "acts in slot " + lib.SlotName(i)
		case sl.Part.Kind == KindCharge, sl.Part.Kind == KindCall, sl.Part.Kind == KindCost, sl.Part.Kind == KindStance:
			return "has a " + sl.Part.Kind.String() + " in slot " + lib.SlotName(i)
		case sl.Part.Kind == KindForm && (sl.Part.Form == FormDash || sl.Part.Form == FormDetonate || sl.Part.Form.Placed()):
			return "acts in slot " + lib.SlotName(i)
		case sl.Part.Kind == KindGate && sl.Part.Gate == GateSpend:
			return "spends a bank in slot " + lib.SlotName(i)
		}
	}
	return ""
}

// MaxCallDepth is how deep run-now calls may nest (a library calling a
// library calling ...). None may reach a hex already running.
const MaxCallDepth = 4

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
		case OpDamage, OpMend, OpShield, OpPurge:
			if all || v == VerbDamage {
				st.Value *= x
			}
			if v == VerbMarked && st.Op == OpDamage {
				if st.Bonus == 0 {
					st.Bonus = 1
				}
				st.Bonus *= x
			}
		case OpPush, OpMove:
			if all || v == VerbPush {
				st.Value *= x
			}
		case OpStatus, OpTaunt, OpBuff, OpRaise:
			if all || v == VerbDuration {
				st.Value *= x
			}
		case OpSelect:
			if (all || v == VerbCount) && st.Target.Counted() && st.Count < MaxTargets {
				st.Count = clampCount(int(float32(st.Count)*x + 0.5))
			}
			if (all || v == VerbRadius) && st.Target.Ranged() {
				st.Value *= x
			}
		}
	}
}

// flag sets a run-time bend on the steps in f a multiplier of verb v
// scales: damage, push and status numbers (not target counts or radii).
func (c *compiler) flag(f span, v Verb, fl uint8) {
	all := v == VerbAll
	for i := f.lo; i < f.hi; i++ {
		st := &c.p.Steps[i]
		switch {
		case st.Op == OpDamage && (all || v == VerbDamage || v == VerbMarked),
			st.Op == OpPush && (all || v == VerbPush),
			st.Op == OpStatus && (all || v == VerbDuration):
			st.Flag |= fl
		}
	}
}

// frameCost adds what a slot's frame costs on top of its part, given the
// program's Power and Heat from before the part was paid for: a Siphon
// takes the part's Power back and adds Heat, a Prism or a loot Surge
// adds a little Heat. The frame's grade cuts the Heat.
func (c *compiler) frameCost(f Frame, pw, ht float32) {
	switch f.Bend {
	case BendSiphon:
		c.p.Power = pw
		c.p.Heat += SiphonHeat * f.Cut()
	case BendPrism:
		c.p.Heat += PrismHeat * f.Cut()
	case BendSurge:
		if !f.Shaped {
			c.p.Heat += SurgeHeat * f.Cut()
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

// form compiles a form part in a target slot: the selects that pick who
// the slots beside it hit, plus what it does on its own (a dash's move,
// a detonation using up its wreck). It returns the register of foes and,
// for an aura, the register of allies (else -1).
func (c *compiler) form(p Part, origin uint8, depth int) (uint8, int, error) {
	rule := p.Rule()
	sel := func(t TargetKind, src uint8, v float32, n int) (uint8, error) {
		r, err := c.alloc()
		if err != nil {
			return 0, err
		}
		c.emit(Step{Op: OpSelect, Target: t, Src: src, Dst: r, Count: clampCount(n), Value: v})
		return r, nil
	}
	union := func(a, b uint8) (uint8, error) {
		r, err := c.alloc()
		if err != nil {
			return 0, err
		}
		c.emit(Step{Op: OpUnion, Src: a, Src2: b, Dst: r})
		return r, nil
	}
	both := func(a, b uint8, err error) (uint8, int, error) {
		if err != nil {
			return 0, -1, err
		}
		r, err := union(a, b)
		return r, -1, err
	}
	one := func(r uint8, err error) (uint8, int, error) { return r, -1, err }
	switch p.Form {
	case FormBolt:
		switch rule {
		case 0:
			return one(sel(TargetNearest, origin, 0, 1))
		case 1:
			return one(sel(TargetLine, origin, BoltReach, MaxTargets))
		}
		line, err := sel(TargetLine, origin, BoltReach, MaxTargets)
		if err != nil {
			return 0, -1, err
		}
		split, err := sel(TargetAdjacent, line, 1.5, MaxTargets)
		return both(line, split, err)
	case FormBeam:
		switch rule {
		case 0:
			return one(sel(TargetLine, origin, BeamLength, MaxTargets))
		case 1:
			return one(sel(TargetBoth, origin, BeamLength, MaxTargets))
		}
		return one(sel(TargetCone, origin, BeamLength, MaxTargets))
	case FormAura:
		home := uint8(RegCaster)
		if c.device != 0 {
			home = RegOrigin // a totem: round the device
		}
		kind := TargetAdjacent
		if rule >= 1 {
			kind = TargetAura
		}
		foes, err := sel(kind, home, AuraRadius, MaxTargets)
		if err != nil {
			return 0, -1, err
		}
		if rule == 2 {
			marked, err := sel(TargetMarked, home, NearestReach, MaxTargets)
			if err != nil {
				return 0, -1, err
			}
			if foes, err = union(foes, marked); err != nil {
				return 0, -1, err
			}
		}
		al, err := sel(TargetAllies, home, AuraRadius, MaxTargets)
		return foes, int(al), err
	case FormNova:
		rad := float32(NovaRadius)
		if rule == 2 {
			rad++
		}
		ring, err := sel(TargetAdjacent, origin, rad, MaxTargets)
		if err != nil || rule == 0 {
			return ring, -1, err
		}
		mine, err := sel(TargetAdjacent, RegCaster, rad, MaxTargets)
		return both(ring, mine, err)
	case FormDash:
		line, err := sel(TargetLine, origin, DashLength, MaxTargets)
		if err != nil {
			return 0, -1, err
		}
		c.emit(Step{Op: OpMove, Src: line, Src2: RegCaster, Count: uint8(MoveDash), Value: DashLength})
		if rule > 0 && depth == 0 {
			c.back, c.backSet = MoveBack, true
			if rule == 2 {
				c.back = MoveHide
			}
		}
		return line, -1, nil
	case FormDetonate:
		rad := float32(BlastRadius)
		if rule == 2 {
			rad++
		}
		w, err := sel(TargetWreck, origin, WreckReach, 1)
		if err != nil {
			return 0, -1, err
		}
		hit, err := sel(TargetAdjacent, w, rad, MaxTargets)
		if err != nil {
			return 0, -1, err
		}
		if rule >= 1 {
			w2, err := sel(TargetWreck, w, WreckReach, 1)
			if err != nil {
				return 0, -1, err
			}
			hit2, err := sel(TargetAdjacent, w2, rad, MaxTargets)
			if err != nil {
				return 0, -1, err
			}
			if hit, err = union(hit, hit2); err != nil {
				return 0, -1, err
			}
			c.emit(Step{Op: OpConsume, Src: w2})
		}
		c.emit(Step{Op: OpConsume, Src: w})
		return hit, -1, nil
	case FormZone:
		return one(sel(TargetAdjacent, origin, ZoneRadius, MaxTargets))
	case FormTrap:
		return one(sel(TargetAdjacent, origin, TrapRadius, MaxTargets))
	}
	// Turret and drone: the foe nearest the device.
	return one(sel(TargetNearest, origin, 0, 1))
}

// NearestReach is how far a marked aura looks for marked foes.
const NearestReach = 9

// alliesOf is the allies register of an aura feeding slot s, or -1.
func (c *compiler) alliesOf(h *Hex, s int, order []int, allies [Slots]int) int {
	for _, a := range order {
		if allies[a] >= 0 && h.Feeds(a, s) {
			return allies[a]
		}
	}
	return -1
}

// rider follows a Rider effect with its second effect: damage leaves its
// type's status, a status deals a little damage of its type.
func (c *compiler) rider(st Step) {
	r := st
	r.Bonus = 0
	r.Flag &^= FlagLeech
	switch st.Op {
	case OpDamage:
		if c.reg.Get(st.Type).Status == StatusNone || c.reg.Get(st.Type).Status == StatusCloak {
			return
		}
		r.Op, r.Value = OpStatus, RiderTime
	case OpStatus:
		if c.reg.Cloak(st.Type) {
			return // cloak hides the caster and never hurts anyone
		}
		r.Op, r.Value = OpDamage, RiderHit
	default:
		return
	}
	c.emit(r)
	c.p.Power += RiderCost * 4
	c.p.Heat += RiderCost * 3
}

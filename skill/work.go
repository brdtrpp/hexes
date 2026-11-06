package skill

import "fmt"

// Working is how a hex's numbers came out: each effect's base number and
// every factor the compiler put on it, and each share of the cost. The
// part and hex sheets read it so a player can see the math, not just the
// answer. Only Explain fills one; Compile leaves it off, so play pays
// nothing for it.
type Working struct {
	Effects []EffectWork
	// Costs is each part's share of Power and Heat, in compile order.
	Costs []CostWork
	// Totals is what changed the whole hex's cost after the parts were
	// added up: an echo, the shape, a frame, a sacrifice.
	Totals []CostWork
	// Warnings are things a hex does that a player likely didn't mean.
	Warnings []string
}

// EffectWork is one effect in a compiled hex.
type EffectWork struct {
	Tag  uint16
	Name string // the part, as its label reads
	// Slot is the hex slot it sits in, from 1; 0 inside a called or
	// nested hex.
	Slot int
	Op   Op
	Type TypeID
	// Base is the part's own number before anything touched it.
	Base    float32
	Rarity  Rarity
	Factors []Factor
	// Final is the number its first step ended with; Runs is how many
	// steps carry it (a Twin frame or an echo runs it again). Bonus is a
	// marked multiplier's factor on marked foes, zero for none.
	Final float32
	Runs  int
	Bonus float32
}

// Factor is one thing that scaled an effect's number.
type Factor struct {
	Why string
	X   float32
}

// CostWork is one share of a hex's cost, or one change to its total.
// For a total, X is a factor on the Heat so far (zero when it adds).
type CostWork struct {
	Why         string
	Power, Heat float32
	X           float32
}

// Explain compiles h like Compile does and also returns its working.
func Explain(h *Hex, reg *Registry, maxDepth int) (*Program, *Working, error) {
	w := &Working{}
	p, err := build(h, reg, maxDepth, w)
	if err != nil {
		return nil, nil, err
	}
	for i := range w.Effects {
		e := &w.Effects[i]
		for _, st := range p.Steps {
			if st.Tag != e.Tag {
				continue
			}
			if e.Runs == 0 {
				e.Final, e.Bonus = st.Value, st.Bonus
				e.Op, e.Type = st.Op, st.Type
			}
			e.Runs++
		}
	}
	return p, w, nil
}

// track gives an effect step its tag and starts its working at the
// part's base, with the rarity factor on it.
func (c *compiler) track(st *Step, p Part, name string, base float32, slot, depth int) {
	if c.work == nil {
		return
	}
	c.tags++
	st.Tag = c.tags
	e := EffectWork{Tag: st.Tag, Name: name, Op: st.Op, Type: st.Type, Base: base, Rarity: p.Rarity}
	if depth == 0 && c.calling == 0 && c.docking == 0 {
		e.Slot = slot + 1
	}
	c.work.Effects = append(c.work.Effects, e)
	if s := p.Rarity.Scale(); s != 1 && st.Value != base {
		c.factor(st, p.Rarity.String(), s)
	}
}

// factor notes x on the effect step st carries.
func (c *compiler) factor(st *Step, why string, x float32) {
	if c.work == nil || st.Tag == 0 || x == 1 {
		return
	}
	for i := len(c.work.Effects) - 1; i >= 0; i-- {
		if c.work.Effects[i].Tag == st.Tag {
			c.work.Effects[i].Factors = append(c.work.Effects[i].Factors, Factor{why, x})
			return
		}
	}
}

// snapTags takes each tracked effect's number as it stands, and
// noteSnap notes how far each has moved since as one factor. A merge
// scales step by step, where a per-call note would count twins twice.
func (c *compiler) snapTags() map[uint16]float32 {
	if c.work == nil {
		return nil
	}
	m := map[uint16]float32{}
	for _, st := range c.p.Steps {
		if _, ok := m[st.Tag]; st.Tag != 0 && !ok {
			m[st.Tag] = st.Value
		}
	}
	return m
}

func (c *compiler) noteSnap(before map[uint16]float32, why string) {
	if c.work == nil {
		return
	}
	done := map[uint16]bool{}
	for i := range c.p.Steps {
		st := &c.p.Steps[i]
		v, ok := before[st.Tag]
		if !ok || done[st.Tag] {
			continue
		}
		done[st.Tag] = true
		if v != 0 && st.Value != v {
			c.factor(st, why, st.Value/v)
		}
	}
}

// costLine notes one part's share of the cost.
func (c *compiler) costLine(why string, power, heat float32) {
	if c.work == nil || (power == 0 && heat == 0) {
		return
	}
	c.work.Costs = append(c.work.Costs, CostWork{Why: why, Power: power, Heat: heat})
}

// total notes a change to the whole hex's cost.
func (c *compiler) total(why string, power, heat, x float32) {
	if c.work == nil {
		return
	}
	c.work.Totals = append(c.work.Totals, CostWork{Why: why, Power: power, Heat: heat, X: x})
}

// costMark and costCut drop the cost lines a called hex's body logged,
// since a call pays only Heat, and that through the docked total.
func (c *compiler) costMark() int {
	if c.work == nil {
		return 0
	}
	return len(c.work.Costs)
}

func (c *compiler) costCut(n int) {
	if c.work != nil && n <= len(c.work.Costs) {
		c.work.Costs = c.work.Costs[:n]
	}
}

// Sum adds the working back up: each part's share, then each change to
// the total in order. It matches the program's Power and Heat.
func (w *Working) Sum() (power, heat float32) {
	for _, l := range w.Costs {
		power += l.Power
		heat += l.Heat
	}
	for _, l := range w.Totals {
		if l.X != 0 {
			heat *= l.X
			continue
		}
		power += l.Power
		heat += l.Heat
	}
	return power, heat
}

// costName is how a part reads in the cost working.
func costName(p Part, reg *Registry) string {
	if p.Kind == KindEffect && p.Effect.Typed() && reg != nil {
		return fmt.Sprintf("%s %s", reg.Get(p.Type).Name, p.Effect)
	}
	return p.String()
}

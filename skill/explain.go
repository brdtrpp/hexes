package skill

import (
	"fmt"
	"strings"
)

// Use says, in a few words for the editor, what the part in slot s does
// where it sits, and whether it does anything there at all. It follows the
// rules Compile bakes in, so what it says is what runs:
//
//   - An effect, nested hex or Call lands on the targets beside it, its
//     gate's half in a branch, or everything the ring targeted in a Hub's
//     center. With no target beside it, the trigger's own target.
//   - A target is only worth something if a slot it feeds reads targets
//     (an effect, Call, nested hex or gate after it), or could once an
//     empty slot next to it is filled, or a Loop carries it round.
//   - A multiplier needs a part linked just before it in the flow whose
//     numbers its kind scales: damage for damage, push for a push,
//     duration for a status, count and radius for targets. A Hub's center
//     and a Radiant multiplier boost the whole ring.
//   - A gate needs a slot after it in the flow to route into.
//
// The editor only lets a part go where Use says it does something.
func (h *Hex) Use(s int) (what string, ok bool) {
	if s < 0 || s >= Slots || !h.Shape.Has(s) {
		return "outside this shape", false
	}
	sl := h.Slots[s]
	if sl.Kind == SlotEmpty {
		return "", true
	}
	start := h.TriggerSlot()
	routes := h.Routes(start)
	if sl.Kind == SlotNested {
		return "runs on " + h.whom(s, routes), true
	}
	p := sl.Part
	switch p.Kind {
	case KindTrigger:
		return triggerUse(h, s, p)
	case KindTarget:
		return h.targetUse(s, routes)
	case KindEffect:
		return fmt.Sprintf("%s %s", effectVerb(p), h.whom(s, routes)), true
	case KindCall:
		if p.RunsNow() {
			if h.Libs[s] == nil {
				return "no library hex named " + p.Lib + " on the board", false
			}
			return fmt.Sprintf("runs %s on %s, then holds who it returns", p.Lib, h.whom(s, routes)), true
		}
		return "runs on " + h.whom(s, routes), true
	case KindCharge:
		w := fmt.Sprintf("adds to bank %s each firing", BankName(p.Bank))
		if p.Charge == ChargeDrain {
			w = fmt.Sprintf("drains bank %s each firing", BankName(p.Bank))
		}
		if r := routes[s]; r.Gate >= 0 {
			w += fmt.Sprintf(", only when someone %s slot %s", branchWord(r.Branch), h.SlotName(int(r.Gate)))
		}
		return w, true
	case KindGate:
		return h.gateUse(s, routes)
	case KindMult:
		return h.multUse(s, p)
	}
	return "", true
}

func (h *Hex) targetUse(s int, routes [Slots]Route) (string, bool) {
	var readers []int
	empty := false
	trig := h.TriggerSlot()
	for b := 0; b < Slots; b++ {
		if !h.Feeds(s, b) {
			continue
		}
		sl := h.Slots[b]
		if sl.Kind == SlotEmpty {
			// An empty slot that could later read these targets.
			empty = empty || b != trig
			continue
		}
		if h.readsTargets(b, s, routes) {
			readers = append(readers, b)
		}
	}
	if len(readers) > 0 {
		return "picks targets for " + h.slotList(readers), true
	}
	if d := h.Docks[s]; d != nil {
		return "its targets flow on into " + d.Name, true
	}
	if h.loopSlot(trig) == s {
		return "each target loops the hex again", true
	}
	if empty {
		return "picks targets: put an effect next to it", true
	}
	return "nothing next to it uses targets", false
}

// readsTargets reports whether slot b uses the targets in slot from.
func (h *Hex) readsTargets(b, from int, routes [Slots]Route) bool {
	sl := h.Slots[b]
	if !h.Feeds(from, b) || routes[b].Gate >= 0 {
		return false // a branch slot works on its gate's half instead
	}
	isPart := func(k PartKind) bool { return sl.Kind == SlotPart && sl.Part.Kind == k }
	switch {
	case sl.Kind == SlotNested, isPart(KindEffect), isPart(KindCall):
		return true // they land on everything that feeds them
	case isPart(KindGate):
		order, n := h.Flow(h.TriggerSlot())
		return from == h.inFeed(b, order[:n])
	}
	return false
}

// whom is who an effect, Call or nested hex in slot s lands on.
func (h *Hex) whom(s int, routes [Slots]Route) string {
	if r := routes[s]; r.Gate >= 0 {
		return fmt.Sprintf("whoever %s slot %s", branchWord(r.Branch), h.SlotName(int(r.Gate)))
	}
	order, n := h.Flow(h.TriggerSlot())
	var tgts []int
	for _, a := range order[:n] {
		if h.Feeds(a, s) && h.isTarget(a) {
			tgts = append(tgts, a)
		}
	}
	switch {
	case len(tgts) > 0 && s == Center && h.Shape.Kind == ShapeHub:
		return "everyone the ring targets, once each"
	case len(tgts) > 1 && h.merge(s):
		return "everyone " + h.slotList(tgts) + " target, once each"
	case len(tgts) > 0:
		return h.slotList(tgts) + "'s targets"
	}
	return h.subject() + " (no target next to it)"
}

func (h *Hex) gateUse(s int, routes [Slots]Route) (string, bool) {
	branch := false
	for _, r := range routes {
		branch = branch || int(r.Gate) == s
	}
	if !branch {
		if !h.HasTrigger() {
			return "its YES half is what this library returns", true
		}
		return "nothing after it to split into", false
	}
	if r := routes[s]; r.Gate >= 0 {
		return fmt.Sprintf("splits whoever %s slot %s", branchWord(r.Branch), h.SlotName(int(r.Gate))), true
	}
	order, n := h.Flow(h.TriggerSlot())
	if pred := h.inFeed(s, order[:n]); pred >= 0 && h.isTarget(pred) {
		return fmt.Sprintf("splits slot %s's targets", h.SlotName(pred)), true
	} else if pred >= 0 && h.carries(pred) {
		return fmt.Sprintf("splits whoever slot %s reached", h.SlotName(pred)), true
	}
	if p := h.Slots[s].Part; p.Gate.Charged() {
		return fmt.Sprintf("checks bank %s", BankName(p.Bank)), true
	}
	return "splits " + h.subject(), true
}

func (h *Hex) multUse(s int, p Part) (string, bool) {
	switch {
	case p.Quirk == QuirkRadiant || (s == Center && h.Shape.Kind == ShapeHub):
		return "boosts the whole ring", true
	case h.merge(s):
		return "boosts everything linked into it", true
	}
	order, n := h.Flow(h.TriggerSlot())
	cur := s
	// Walk back through a run of multipliers to the part they all boost.
	for {
		pred := h.inFeed(cur, order[:n])
		if pred < 0 {
			if cur == order[0] {
				return "nothing before it to boost", false
			}
			return "nothing linked before it to boost", false
		}
		ps := h.Slots[pred]
		switch {
		case ps.Kind == SlotEmpty:
			return fmt.Sprintf("boosts what goes in slot %s", h.SlotName(pred)), true
		case ps.Kind == SlotNested:
			return fmt.Sprintf("boosts the hex in slot %s", h.SlotName(pred)), true
		case ps.Part.Kind == KindMult:
			if h.merge(pred) {
				return fmt.Sprintf("boosts slot %s", h.SlotName(pred)), true
			}
			cur = pred
			continue
		}
		if why := multFits(p.Verb, ps.Part); why != "" {
			return why, false
		}
		return fmt.Sprintf("boosts slot %s", h.SlotName(pred)), true
	}
}

// multFits says why a multiplier of verb v does nothing to part q, or "".
func multFits(v Verb, q Part) string {
	switch q.Kind {
	case KindTrigger:
		return "can't boost a trigger"
	case KindGate, KindCharge, KindCall:
		return fmt.Sprintf("can't boost a %s", q.Kind)
	case KindEffect:
		want := map[EffectKind]Verb{EffectDamage: VerbDamage, EffectPush: VerbPush, EffectStatus: VerbDuration}[q.Effect]
		if v != VerbAll && v != want {
			return fmt.Sprintf("a %s booster does nothing to %s", verbWord(v), q.Effect)
		}
	case KindTarget:
		count := q.Target == TargetNearest || q.Target == TargetChain
		radius := q.Target != TargetSelf && q.Target != TargetNearest
		switch {
		case v == VerbAll && (count || radius):
		case v == VerbCount && count:
		case v == VerbRadius && radius:
		default:
			return fmt.Sprintf("a %s booster does nothing to a %s target", verbWord(v), q.Target)
		}
	}
	return ""
}

// isTarget reports whether slot s holds a group the slots it feeds can
// act on: a target, or a run-now call (which holds who its library
// returned).
func (h *Hex) isTarget(s int) bool {
	sl := h.Slots[s]
	return sl.Kind == SlotPart && (sl.Part.Kind == KindTarget || sl.Part.RunsNow())
}

// carries reports whether slot s passes anyone on to a gate after it:
// what Compile's carry holds.
func (h *Hex) carries(s int) bool {
	sl := h.Slots[s]
	switch {
	case sl.Kind == SlotNested:
		return true
	case sl.Kind != SlotPart:
		return false
	}
	switch sl.Part.Kind {
	case KindTarget, KindEffect, KindCall, KindGate:
		return true
	}
	return false
}

// HasTrigger reports whether the hex has a trigger in its shape. One
// without is a library hex: it runs when docked into or called.
func (h *Hex) HasTrigger() bool {
	for i, sl := range h.Slots {
		if sl.Kind == SlotPart && sl.Part.Kind == KindTrigger && h.Shape.Has(i) {
			return true
		}
	}
	return false
}

// subject is the trigger's own target, in words.
// triggerUse is what a trigger does; a watch says what it watches.
func triggerUse(h *Hex, s int, p Part) (string, bool) {
	switch p.Trigger {
	case Channel:
		return "fires the hex while you hold CAST", true
	case Watch:
		switch {
		case p.Lib != "" && h.Libs[s] == nil:
			return "watches " + p.Lib + ", which isn't on the board", false
		case p.Lib != "":
			if why := finderOnly(h.Libs[s]); why != "" {
				return "can't watch " + p.Lib + ": it " + why, false
			}
			return "fires when " + p.Lib + " finds anyone", true
		case p.Cond.Len() > 0:
			if _, err := p.Cond.Compile(); err != nil {
				return "its condition doesn't read yet: " + err.Error(), false
			}
			return "fires when " + p.Cond.Text(nil) + " holds", true
		}
		return "watches nothing yet: tap it again to pick what it watches", false
	}
	return "fires the hex", true
}

func (h *Hex) subject() string {
	for i, sl := range h.Slots {
		if sl.Kind == SlotPart && sl.Part.Kind == KindTrigger && h.Shape.Has(i) {
			switch sl.Part.Trigger {
			case OnHit:
				return "the foe you hit"
			case OnKill:
				return "the kill"
			case OnHurt:
				return "whoever hit you"
			case Watch:
				if p := sl.Part; p.Lib != "" {
					return "whoever " + p.Lib + " finds"
				} else if cp, err := p.Cond.Compile(); err == nil && cp.Foe {
					return "the foes the watch finds"
				}
			}
			return "the nearest foe"
		}
	}
	return "whoever docks in"
}

func effectVerb(p Part) string {
	switch p.Effect {
	case EffectPush:
		return "pushes"
	case EffectStatus:
		return "afflicts"
	}
	return "hits"
}

func verbWord(v Verb) string {
	return [...]string{"damage", "push", "duration", "count", "radius", "all"}[v]
}

func branchWord(b Branch) string {
	if b == BranchYes {
		return "passes"
	}
	return "fails"
}

func (h *Hex) slotList(s []int) string {
	names := make([]string, len(s))
	for i, x := range s {
		names[i] = h.SlotName(x)
	}
	if len(names) == 1 {
		return "slot " + names[0]
	}
	return "slots " + strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}

func contains(s []int, x int) bool {
	for _, y := range s {
		if y == x {
			return true
		}
	}
	return false
}

// ReturnSlot is the slot whose group a library hex hands back when
// called: the last one in its flow that carries anyone, or -1 (it then
// hands back the group it was called on).
func (h *Hex) ReturnSlot() int {
	order, n := h.Flow(h.TriggerSlot())
	for k := n - 1; k >= 0; k-- {
		s := order[k]
		if h.carries(s) {
			if sl := h.Slots[s]; sl.Kind == SlotPart && sl.Part.Kind == KindCharge {
				continue
			}
			return s
		}
	}
	return -1
}

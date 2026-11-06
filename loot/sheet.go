package loot

import (
	"fmt"
	"math"
	"strings"

	"hexes/skill"
)

// A Sheet is a part or a hex laid out the way an item card reads: its
// name and rarity, a line on what it does, then a fixed list of numbers
// with the math under each one. Only the lines that apply are there, and
// always in the same order, so the eye learns where to look.
type Sheet struct {
	Name   string
	Rarity skill.Rarity
	Sub    string // what it is: "effect, plasma" or "ring hex, on cast"
	Desc   string // one line on what it does
	Lines  []Line
	// Next is what the next rarity up would read, the way a skill tree
	// shows the next level. Empty at the top or for a hex.
	Next string
}

// Line is one number on a sheet: "Plasma damage  23", and under it the
// math that made it, "17 base x1.35 rare".
type Line struct {
	Label, Value string
	Math         string
	Tone         Tone
}

// Tone says how a sheet line reads: a plain stat, a cost, or a rule that
// rarity or a quirk adds.
type Tone uint8

const (
	ToneStat Tone = iota
	ToneCost
	ToneRule
)

func num(v float32) string {
	switch a := math.Abs(float64(v)); {
	case a >= 10 || a == math.Trunc(a):
		return fmt.Sprintf("%.0f", v)
	case a >= 1:
		return fmt.Sprintf("%.1f", v)
	}
	return fmt.Sprintf("%.2f", v)
}

func times(x float32) string {
	return "x" + strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", x), "0"), ".")
}

// rarMath is "17 base x1.35 rare", or "" when rarity adds nothing.
func rarMath(base float32, r skill.Rarity, unit string) string {
	if r.Scale() == 1 {
		return ""
	}
	return fmt.Sprintf("%s%s base %s %s", num(base), unit, times(r.Scale()), r)
}

// PartSheet is a part's sheet. desc is its plain line; the game has it.
func PartSheet(p skill.Part, reg *skill.Registry, desc string) Sheet {
	s := Sheet{Name: partName(p, reg), Rarity: p.Rarity, Sub: partSub(p, reg), Desc: desc}
	s.Lines = partLines(p, reg)
	if p.Rarity+1 < skill.NumRarities {
		q := p
		q.Rarity++
		cur := partLines(p, reg)
		if next := partLines(q, reg); len(next) > 0 && next[0].Tone == ToneStat && len(cur) > 0 && next[0].Value != cur[0].Value {
			s.Next = fmt.Sprintf("At %s: %s %s", q.Rarity, strings.ToLower(next[0].Label), next[0].Value)
			if extra := rarityRule(q); extra != "" && extra != rarityRule(p) {
				s.Next += ". " + extra
			}
		} else if extra := rarityRule(q); extra != "" && extra != rarityRule(p) {
			s.Next = fmt.Sprintf("At %s: %s", q.Rarity, extra)
		}
	}
	return s
}

func partName(p skill.Part, reg *skill.Registry) string {
	if p.Kind == skill.KindMult {
		if p.Verb == skill.VerbMarked {
			return "Marked Multiplier"
		}
		return strings.Title(p.Verb.String()) + " Multiplier"
	}
	t, _ := Label(p, reg)
	return strings.Title(strings.ToLower(t))
}

func partSub(p skill.Part, reg *skill.Registry) string {
	sub := p.Kind.String()
	if p.Kind == skill.KindEffect && p.Effect.Typed() {
		sub += ", " + reg.Get(p.Type).Name
	}
	return sub
}

// partLines is the part's numbers, main stat first, then costs, then the
// rules its type, rarity and quirk add.
func partLines(p skill.Part, reg *skill.Registry) []Line {
	sc := p.Rarity.Scale()
	v := p.Value * sc
	var ls []Line
	stat := func(label, value, m string) { ls = append(ls, Line{Label: label, Value: value, Math: m}) }
	switch p.Kind {
	case skill.KindTrigger:
		if p.Value != 1 && p.Value != 0 {
			stat("Damage", times(p.Value), "on every hit in the hex")
		}
		switch p.Trigger {
		case skill.Periodic:
			stat("Fires every", num(p.Period)+" s", "")
		case skill.Channel:
			stat("Fires every", num(skill.ChannelEvery)+" s", "while you hold CAST")
		case skill.Watch:
			stat("Checks every", num(skill.WatchEvery)+" s", fmt.Sprintf("%s Heat a check, %s s rest after it fires", num(skill.WatchHeat), num(skill.WatchRest)))
		}
		if p.Trigger.Stealthy() {
			stat("From hiding", "cloak "+times(reg.Get(reg.MustID("cloak")).StealthMult), "on cloak damage only")
		}
	case skill.KindTarget:
		switch p.Target {
		case skill.TargetNearest, skill.TargetWeakest, skill.TargetMarked:
			stat("Targets", plural(int(p.Count), "foe"), "")
		case skill.TargetChain:
			stat("Hops", plural(int(p.Count), "hop"), "")
		case skill.TargetCone:
			stat("Reach", hexes(p.Value), "")
		case skill.TargetAdjacent:
			stat("Radius", hexes(p.Value), "")
		case skill.TargetWreck:
			stat("Range", hexes(p.Value), "to the nearest wreck")
			stat("Hits", "foes within "+hexes(skill.WreckBurst), "of that wreck")
		case skill.TargetSelf:
			stat("Targets", "you", "")
		}
	case skill.KindEffect:
		dt := reg.Get(p.Type)
		m := rarMath(p.Value, p.Rarity, "")
		switch p.Effect {
		case skill.EffectDamage:
			stat(strings.Title(dt.Name)+" damage", num(v), m)
		case skill.EffectStatus:
			if reg.Cloak(p.Type) {
				stat("Cloak on you", num(v)+" s", m)
			} else {
				stat(strings.Title(dt.Status.String()), num(v)+" s", m)
			}
		case skill.EffectPush:
			stat("Push", hexes(v), m)
		case skill.EffectMend:
			stat("Mend", num(100*v)+"% hull", rarMath(100*p.Value, p.Rarity, "%"))
		case skill.EffectShield:
			stat("Shield", num(100*v)+"% hull", rarMath(100*p.Value, p.Rarity, "%"))
			stat("Lasts", num(skill.ShieldTime)+" s", fmt.Sprintf("up to %s%% of max hull", num(100*skill.ShieldCap)))
		case skill.EffectPurge:
			stat("Heat off", num(v), m)
		case skill.EffectTaunt, skill.EffectThorns, skill.EffectHaste, skill.EffectReanimate:
			stat(strings.Title(p.Effect.String()), num(v)+" s", m)
		case skill.EffectBlink:
			stat("Blink", hexes(v), m)
		}
		if p.Effect.Typed() {
			for _, t := range typeTraits(dt, p.Effect) {
				ls = append(ls, Line{Label: t, Tone: ToneRule})
			}
		}
	case skill.KindMult:
		label := strings.Title(p.Verb.String())
		if p.Verb == skill.VerbMarked {
			label = "Damage on marked"
		}
		x := 1 + (p.Value-1)*sc
		m := ""
		if sc != 1 {
			m = fmt.Sprintf("1 + %s bonus %s %s", num(p.Value-1), times(sc), p.Rarity)
		}
		stat(label, times(x), m)
	case skill.KindGate:
		_, d := Label(p, reg)
		stat("Passes", d, "")
	case skill.KindCharge:
		_, d := Label(p, reg)
		stat("Bank "+skill.BankName(p.Bank), d, "kept between firings")
	case skill.KindCall:
		if p.RunsNow() {
			stat("Called hex Heat", times(1/sc), "never its Power")
		} else {
			stat("Called hex Heat", times(1/sc), fmt.Sprintf("and %s%% of its Power, each run", num(100*skill.CallPower/sc)))
		}
		for _, o := range p.Opts {
			if o.Value != 0 {
				stat("Option "+o.Verb.String(), times(1+(o.Value-1)*o.Rarity.Scale()), "")
			}
		}
	case skill.KindForm:
		stat("Shape", formDetail(p), "")
	case skill.KindCost:
		if p.Cost == skill.CostWrecks {
			stat("Pays", "1 wreck a cast", fmt.Sprintf("and cuts its Heat by %s%%", num(100*skill.WreckHeatCut)))
		} else {
			stat("Pays", fmt.Sprintf("%s%% hull per Heat", num(100*skill.BloodPerHeat)), "in place of its Heat")
		}
	case skill.KindStance:
		stat("Pulses every", num(skill.StancePulse)+" s", "while on")
		stat("Upkeep", num(skill.StanceUpkeep)+" Heat a pulse", "on a bare stance; with more in the hex, each pulse pays the hex again. An overheat drops it")
	}
	// Costs: the values come from the compiler's own PartCost, the math
	// says how.
	pw, heat := skill.PartCost(p, reg)
	var pm, hm []string
	if (p.Kind == skill.KindGate || p.Kind == skill.KindCharge) && sc != 1 {
		pm = append(pm, fmt.Sprintf("%s base %s %s", num(p.Power), times(1/sc), p.Rarity))
	}
	h0 := p.Heat
	if p.Kind == skill.KindEffect && p.Effect.Typed() {
		off := reg.Get(p.Type).HeatOffset
		if reg.Cloak(p.Type) {
			off += skill.CloakHeat
		}
		if off != 0 {
			hm = append(hm, fmt.Sprintf("%s base %+.0f %s", num(p.Heat), off, reg.Get(p.Type).Name))
			h0 += off
		}
	}
	if h0 > 0 {
		switch {
		case p.Kind == skill.KindMult && sc != 1:
			hm = append(hm, fmt.Sprintf("%s %s", times(sc), p.Rarity))
		case p.Kind == skill.KindEffect && p.Effect != skill.EffectPurge && sc != 1:
			hm = append(hm, fmt.Sprintf("%s %s", times(skill.RarityHeat(p.Rarity)), p.Rarity))
		}
	}
	switch p.Quirk {
	case skill.QuirkRelay:
		pm = append(pm, times(1+skill.RelayCost)+" relay")
		hm = append(hm, times(1+skill.RelayCost)+" relay")
	case skill.QuirkRadiant:
		hm = append(hm, "+"+num(skill.RadiantHeatCost)+" radiant")
	}
	if len(hm) > 0 && !strings.Contains(hm[0], "base") {
		hm = append([]string{num(p.Heat) + " base"}, hm...)
	}
	if len(pm) > 0 && !strings.Contains(pm[0], "base") {
		pm = append([]string{num(p.Power) + " base"}, pm...)
	}
	if pw != 0 {
		ls = append(ls, Line{Label: "Power", Value: num(pw), Math: strings.Join(pm, " "), Tone: ToneCost})
	}
	if heat != 0 {
		ls = append(ls, Line{Label: "Heat", Value: signed(heat, false), Math: strings.Join(hm, " "), Tone: ToneCost})
	}
	if r := rarityRule(p); r != "" {
		ls = append(ls, Line{Label: r, Tone: ToneRule})
	}
	if q := QuirkText(p.Quirk); q != "" {
		ls = append(ls, Line{Label: q, Tone: ToneRule})
	}
	return ls
}

// typeTraits is what a damage type adds to every hit, read off its row.
func typeTraits(dt *skill.DamageType, e skill.EffectKind) []string {
	var out []string
	if e == skill.EffectDamage {
		if dt.StatusPower > 0 && dt.Status != skill.StatusNone {
			out = append(out, fmt.Sprintf("%s statuses tick for %s", strings.Title(dt.Status.String()), num(dt.StatusPower)))
		}
		if dt.SpreadFrac > 0 {
			out = append(out, fmt.Sprintf("%s%% splashes onto foes beside the target", num(100*dt.SpreadFrac)))
		}
		if dt.PushPerHit > 0 {
			out = append(out, fmt.Sprintf("Each hit pushes %s hex, %s%% of max hull on a wall", num(dt.PushPerHit), num(100*dt.WallImpactFrac)))
		}
		if dt.Ticks > 1 {
			out = append(out, fmt.Sprintf("Lands over %d ticks", dt.Ticks))
		}
		if dt.RaiseOnKill {
			out = append(out, "Foes it kills rise to fight for you")
		}
		if dt.StealthMult > 1 {
			out = append(out, fmt.Sprintf("%s from hiding or behind", times(dt.StealthMult)))
		}
	}
	if e == skill.EffectStatus {
		if dt.Slow > 0 {
			out = append(out, fmt.Sprintf("Slows %s%%, %d stacks freeze", num(100*dt.Slow), dt.FreezeStacks))
		}
		if dt.ShatterMult > 0 {
			out = append(out, fmt.Sprintf("The hit that breaks a freeze lands %s", times(dt.ShatterMult)))
		}
		if dt.StatusPower > 0 {
			out = append(out, fmt.Sprintf("Ticks for %s", num(dt.StatusPower)))
		}
	}
	return out
}

// rarityRule is what a part's rarity adds beyond bigger numbers.
func rarityRule(p skill.Part) string {
	r := p.Rarity
	switch p.Kind {
	case skill.KindForm:
		if p.Rule() > 0 {
			return "Rarity rule: " + formDetail(p)
		}
	case skill.KindStance:
		if r >= skill.Rare {
			return "Rarity rule: stays on beside another stance"
		}
	case skill.KindGate, skill.KindCharge:
		if r > skill.Common {
			return fmt.Sprintf("Rarity rule: runs on %s of the Power", times(1/r.Scale()))
		}
	case skill.KindCall:
		if r > skill.Common {
			return fmt.Sprintf("Rarity rule: the called hex runs %s cooler", times(1/r.Scale()))
		}
	}
	return ""
}

// HexSheet is a compiled hex's sheet: each effect with its whole chain of
// factors, then what a cast costs and where each share comes from.
func HexSheet(h *skill.Hex, p *skill.Program, w *skill.Working, reg *skill.Registry, desc string) Sheet {
	s := Sheet{Name: h.Name, Sub: hexSub(h, p), Desc: desc}
	best := skill.Common
	for _, sl := range h.Slots {
		if sl.Kind == skill.SlotPart && sl.Part.Rarity > best {
			best = sl.Part.Rarity
		}
	}
	s.Rarity = best
	if p.Trigger == skill.Periodic {
		s.Lines = append(s.Lines, Line{Label: "Fires every", Value: num(p.Period) + " s"})
	}
	dup := map[string]int{}
	for _, e := range w.Effects {
		if e.Runs > 0 {
			l, _ := effectLine(e, reg)
			dup[l]++
		}
	}
	for _, e := range w.Effects {
		if e.Runs == 0 {
			continue
		}
		label, val := effectLine(e, reg)
		if dup[label] > 1 && e.Slot > 0 {
			label += fmt.Sprintf(", slot %d", e.Slot)
		}
		if e.Runs > 1 {
			val += fmt.Sprintf(" x%d", e.Runs)
		}
		m := num(effBase(e)) + " base"
		for _, f := range e.Factors {
			m += fmt.Sprintf(" %s %s", times(f.X), f.Why)
		}
		if len(e.Factors) == 0 {
			m = ""
		}
		if e.Bonus > 0 {
			m = strings.TrimSpace(m + fmt.Sprintf(", %s on marked foes", times(e.Bonus)))
		}
		s.Lines = append(s.Lines, Line{Label: label, Value: val, Math: m})
	}
	if p.Power != 0 {
		s.Lines = append(s.Lines, Line{Label: "Power a cast", Value: num(p.Power), Math: costMath(w, true), Tone: ToneCost})
	}
	if p.Heat != 0 || len(w.Costs) > 0 {
		s.Lines = append(s.Lines, Line{Label: "Heat a cast", Value: signed(p.Heat, false), Math: costMath(w, false), Tone: ToneCost})
	}
	if p.HullCost > 0 {
		s.Lines = append(s.Lines, Line{Label: "Hull a cast", Value: num(100*p.HullCost) + "%", Math: "the Heat, paid in hull", Tone: ToneCost})
	}
	if p.WreckCost > 0 {
		s.Lines = append(s.Lines, Line{Label: "Wrecks a cast", Value: fmt.Sprint(p.WreckCost), Tone: ToneCost})
	}
	if p.Heat > 0 {
		n := int(100 / p.Heat)
		s.Lines = append(s.Lines, Line{Label: "Casts to overheat", Value: fmt.Sprint(n), Math: fmt.Sprintf("100 Heat / %s a cast, before it cools", num(p.Heat))})
	}
	for _, wn := range w.Warnings {
		s.Lines = append(s.Lines, Line{Label: wn, Tone: ToneRule})
	}
	for i, sl := range h.Slots {
		if sl.Kind == skill.SlotPart && sl.Part.Quirk != skill.QuirkNone {
			s.Lines = append(s.Lines, Line{Label: fmt.Sprintf("Slot %d: %s", i+1, QuirkText(sl.Part.Quirk)), Tone: ToneRule})
		}
	}
	return s
}

func hexSub(h *skill.Hex, p *skill.Program) string {
	return fmt.Sprintf("%s hex, %s", strings.ToLower(h.Shape.Name()), strings.ToLower(triggerWord(p.Trigger)))
}

func triggerWord(t skill.TriggerKind) string {
	p := skill.Part{Kind: skill.KindTrigger, Trigger: t, Value: 1}
	w, _ := Label(p, nil)
	return w
}

// effBase is an effect's base as the sheet shows it: mends and shields as
// a share of hull.
func effBase(e skill.EffectWork) float32 {
	if e.Op == skill.OpMend || e.Op == skill.OpShield {
		return 100 * e.Base
	}
	return e.Base
}

func effectLine(e skill.EffectWork, reg *skill.Registry) (label, val string) {
	dt := reg.Get(e.Type)
	name := strings.Title(e.Name)
	switch e.Op {
	case skill.OpDamage:
		return strings.Title(dt.Name) + " damage", num(e.Final)
	case skill.OpStatus:
		if reg.Cloak(e.Type) {
			return "Cloak on you", num(e.Final) + " s"
		}
		return strings.Title(dt.Status.String()), num(e.Final) + " s"
	case skill.OpMend:
		return "Mend", num(100*e.Final) + "% hull"
	case skill.OpShield:
		return "Shield", num(100*e.Final) + "% hull"
	case skill.OpPurge:
		return "Heat off", num(e.Final)
	case skill.OpPush:
		return "Push", hexes(e.Final)
	case skill.OpMove:
		return "Blink", hexes(e.Final)
	}
	return name, num(e.Final) + " s"
}

// costMath is the shares of a cast's Power or Heat, as a sum.
func costMath(w *skill.Working, power bool) string {
	var parts []string
	for _, l := range w.Costs {
		v := l.Heat
		if power {
			v = l.Power
		}
		if v == 0 {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s %s", l.Why, signed(v, len(parts) == 0)))
	}
	for _, l := range w.Totals {
		switch {
		case l.X != 0 && !power:
			parts = append(parts, fmt.Sprintf("then %s %s", times(l.X), l.Why))
		case l.X == 0 && power && l.Power != 0:
			parts = append(parts, fmt.Sprintf("%s %s", l.Why, signed(l.Power, false)))
		case l.X == 0 && !power && l.Heat != 0:
			parts = append(parts, fmt.Sprintf("%s %s", l.Why, signed(l.Heat, false)))
		}
	}
	return strings.Join(parts, ", ")
}

func signed(v float32, first bool) string {
	switch {
	case first && v >= 0:
		return num(v)
	case v >= 0:
		return "+" + num(v)
	}
	return "-" + num(-v)
}

// Text is a sheet as plain lines, for tests and for anywhere a panel
// can't be drawn.
func (s Sheet) Text() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s (%s %s)\n%s\n", s.Name, s.Rarity, s.Sub, s.Desc)
	for _, l := range s.Lines {
		if l.Value != "" {
			fmt.Fprintf(&b, "%s: %s", l.Label, l.Value)
		} else {
			b.WriteString(l.Label)
		}
		if l.Math != "" {
			fmt.Fprintf(&b, " [%s]", l.Math)
		}
		b.WriteByte('\n')
	}
	if s.Next != "" {
		b.WriteString(s.Next + "\n")
	}
	return b.String()
}

func hexes(v float32) string {
	if num(v) == "1" {
		return "1 hex"
	}
	return num(v) + " hexes"
}

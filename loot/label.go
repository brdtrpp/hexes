package loot

import (
	"fmt"
	"strings"

	"hexes/skill"
)

// The functions here are for the screen, not the simulation: they build
// strings, so they allocate.

// Label is a part as two short lines for a slot or inventory cell: what it
// is, then its main number. Numbers include the rarity boost the compiler
// will apply.
func Label(p skill.Part, reg *skill.Registry) (title, detail string) {
	sc := p.Rarity.Scale()
	switch p.Kind {
	case skill.KindTrigger:
		switch p.Trigger {
		case skill.OnCast:
			title = "ON CAST"
		case skill.OnHit:
			title = "ON HIT"
		case skill.OnKill:
			title = "ON KILL"
		case skill.OnHurt:
			title = "ON HURT"
		case skill.FromStealth:
			title = "STEALTH"
		case skill.FromBehind:
			title = "BEHIND"
		case skill.Periodic:
			return "EVERY", fmt.Sprintf("%gs", p.Period)
		}
		if p.Value != 1 {
			return title, fmt.Sprintf("x%g dmg", p.Value)
		}
		return title, "trigger"
	case skill.KindTarget:
		switch p.Target {
		case skill.TargetSelf:
			return "SELF", "you"
		case skill.TargetNearest:
			return "NEAREST", plural(int(p.Count), "foe")
		case skill.TargetCone:
			return "CONE", fmt.Sprintf("%g long", p.Value)
		case skill.TargetAdjacent:
			return "AROUND", fmt.Sprintf("%g hex", p.Value)
		case skill.TargetChain:
			return "CHAIN", plural(int(p.Count), "hop")
		}
	case skill.KindEffect:
		name := strings.ToUpper(reg.Get(p.Type).Name)
		switch p.Effect {
		case skill.EffectDamage:
			return name, fmt.Sprintf("%.0f dmg", p.Value*sc)
		case skill.EffectPush:
			return "PUSH", fmt.Sprintf("%.1f hex", p.Value*sc)
		case skill.EffectStatus:
			return strings.ToUpper(reg.Get(p.Type).Status.String()), fmt.Sprintf("%.1fs", p.Value*sc)
		}
	case skill.KindMult:
		return fmt.Sprintf("x%.2f", 1+(p.Value-1)*sc), p.Verb.String()
	case skill.KindGate:
		switch p.Gate {
		case skill.GateHull:
			return "IF HULL", fmt.Sprintf("< %.0f%%", p.Value*100)
		case skill.GateHeat:
			return "IF HEAT", fmt.Sprintf("> %.0f%%", p.Value*100)
		case skill.GatePower:
			return "IF POWER", fmt.Sprintf("> %.0f%%", p.Value*100)
		case skill.GateStatus:
			dt := reg.Get(p.Type)
			if dt.Status == skill.StatusCloak {
				return "IF HIDDEN", "stealthed"
			}
			return "IF " + strings.ToUpper(dt.Status.String()), dt.Name
		case skill.GateCrowd:
			return "IF CROWD", fmt.Sprintf("%d+ in %g", p.Count, p.Value)
		case skill.GateSpend:
			return "SPEND " + skill.BankName(p.Bank), fmt.Sprintf("at %d", p.Count)
		case skill.GateEmpty:
			return "IF " + skill.BankName(p.Bank) + " EMPTY", "bank " + skill.BankName(p.Bank)
		}
	case skill.KindCharge:
		if p.Charge == skill.ChargeDrain {
			return "DRAIN " + skill.BankName(p.Bank), fmt.Sprintf("-%g", p.Value)
		}
		return "CHARGE " + skill.BankName(p.Bank), fmt.Sprintf("+%g", p.Value)
	}
	return "EMPTY", ""
}

func plural(n int, s string) string {
	if n == 1 {
		return "1 " + s
	}
	return fmt.Sprintf("%d %ss", n, s)
}

// Describe is one line on a part for the editor's info bar.
func Describe(p skill.Part, reg *skill.Registry) string {
	title, detail := Label(p, reg)
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s: %s %s", p.Rarity, p.Kind, strings.ToLower(title), detail)
	if p.Kind == skill.KindEffect && p.Effect == skill.EffectStatus {
		fmt.Fprintf(&b, " (%s)", reg.Get(p.Type).Name)
	}
	fmt.Fprintf(&b, ", %.0f power", p.Power)
	if p.Heat != 0 {
		fmt.Fprintf(&b, " %+.0f heat", p.Heat)
	}
	switch {
	case p.Kind == skill.KindGate && p.Gate == skill.GateSpend:
		fmt.Fprintf(&b, ". Once bank %s holds %d it spends them and the next slot clockwise fires, else the slot after it",
			skill.BankName(p.Bank), p.Count)
	case p.Kind == skill.KindGate && p.Gate == skill.GateEmpty:
		fmt.Fprintf(&b, ". Next slot clockwise fires while bank %s is at zero, the slot after it otherwise", skill.BankName(p.Bank))
	case p.Kind == skill.KindGate:
		b.WriteString(". Next slot clockwise hits the ones that pass, the slot after it the rest")
	case p.Kind == skill.KindCharge:
		fmt.Fprintf(&b, ". Bank %s keeps its count between firings", skill.BankName(p.Bank))
	}
	if p.UsesBank() {
		b.WriteString(". In a hex, tap the center plate to switch its bank")
	}
	if q := QuirkText(p.Quirk); q != "" {
		b.WriteString(". ")
		b.WriteString(q)
	}
	return b.String()
}

// QuirkText says what a quirk does, in a sentence.
func QuirkText(q skill.Quirk) string {
	switch q {
	case skill.QuirkRadiant:
		return "Radiant: scales every part in the hex, not just the one before it."
	case skill.QuirkRelay:
		return "Relay: arcs on to everything next to each target."
	case skill.QuirkEcho:
		return "Echo: the whole hex runs twice."
	}
	return ""
}

// Summary is a hex's contents in a few words, for the HUD card.
func Summary(h *skill.Hex, reg *skill.Registry) string {
	var parts []string
	for _, s := range h.Slots {
		if s.Kind != skill.SlotPart || s.Part.Kind == skill.KindTrigger {
			continue
		}
		t, _ := Label(s.Part, reg)
		parts = append(parts, strings.ToLower(t))
	}
	if len(parts) == 0 {
		return "empty"
	}
	return strings.Join(parts, " · ")
}

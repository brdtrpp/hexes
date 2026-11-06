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
		case skill.OnWreck:
			title = "ON WRECK"
		case skill.OnDodge:
			title = "ON DODGE"
		case skill.OnVent:
			title = "ON VENT"
		case skill.Periodic:
			return "EVERY", fmt.Sprintf("%gs", p.Period)
		case skill.Channel:
			return "CHANNEL", "hold CAST"
		case skill.Watch:
			switch {
			case p.Lib != "":
				return "WATCH", p.Lib
			case p.Cond.Len() > 0:
				return "WATCH", CondShort(p.Cond, reg)
			}
			return "WATCH", "tap to set"
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
		case skill.TargetWreck:
			return "WRECK", "nearest body"
		case skill.TargetWeakest:
			return "WEAKEST", plural(int(p.Count), "foe")
		case skill.TargetMarked:
			return "MARKED", plural(int(p.Count), "foe")
		}
	case skill.KindEffect:
		name := strings.ToUpper(reg.Get(p.Type).Name)
		switch p.Effect {
		case skill.EffectDamage:
			return name, fmt.Sprintf("%.0f dmg", p.Value*sc)
		case skill.EffectPush:
			return "PUSH", fmt.Sprintf("%.1f hex", p.Value*sc)
		case skill.EffectStatus:
			if reg.Cloak(p.Type) {
				return "CLOAK", fmt.Sprintf("self %.1fs", p.Value*sc)
			}
			return strings.ToUpper(reg.Get(p.Type).Status.String()), fmt.Sprintf("%.1fs", p.Value*sc)
		case skill.EffectMend:
			return "MEND", fmt.Sprintf("%.0f%% hull", 100*p.Value*sc)
		case skill.EffectShield:
			return "SHIELD", fmt.Sprintf("%.0f%% hull", 100*p.Value*sc)
		case skill.EffectPurge:
			return "PURGE", fmt.Sprintf("-%.0f heat", p.Value*sc)
		case skill.EffectTaunt:
			return "TAUNT", fmt.Sprintf("%.1fs", p.Value*sc)
		case skill.EffectThorns:
			return "THORNS", fmt.Sprintf("%.1fs", p.Value*sc)
		case skill.EffectReanimate:
			return "REANIMATE", fmt.Sprintf("%.0fs", p.Value*sc)
		case skill.EffectHaste:
			return "HASTE", fmt.Sprintf("%.1fs", p.Value*sc)
		case skill.EffectBlink:
			return "BLINK", fmt.Sprintf("%.1f hex", p.Value*sc)
		}
	case skill.KindMult:
		if p.Verb == skill.VerbMarked {
			return fmt.Sprintf("x%.2f", 1+(p.Value-1)*sc), "on marked"
		}
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
		case skill.GateBehind:
			return "IF BEHIND", "foe faces away"
		case skill.GateHurt:
			return "IF HURT", fmt.Sprintf("you < %.0f%%", p.Value*100)
		case skill.GateSpend:
			if p.Name == "finisher" {
				return "FINISHER", fmt.Sprintf("%d on %s", p.Count, skill.BankName(p.Bank))
			}
			return "SPEND " + skill.BankName(p.Bank), fmt.Sprintf("at %d", p.Count)
		case skill.GateEmpty:
			return "IF " + skill.BankName(p.Bank) + " EMPTY", "bank " + skill.BankName(p.Bank)
		case skill.GateCond:
			return "IF", CondShort(p.Cond, reg)
		}
	case skill.KindForm:
		return strings.ToUpper(p.Form.String()), formDetail(p)
	case skill.KindCost:
		if p.Cost == skill.CostWrecks {
			return "WRECK COST", "1 wreck"
		}
		return "SACRIFICE", "pays in hull"
	case skill.KindStance:
		return strings.ToUpper(p.Stance.String()), "stance"
	case skill.KindCharge:
		if p.Name == "combo" {
			return "COMBO", "+1 on " + skill.BankName(p.Bank)
		}
		if p.Charge == skill.ChargeDrain {
			return "DRAIN " + skill.BankName(p.Bank), fmt.Sprintf("-%g", p.Value)
		}
		return "CHARGE " + skill.BankName(p.Bank), fmt.Sprintf("+%g", p.Value)
	case skill.KindCall:
		if p.RunsNow() {
			if n := p.Opts.Len(); n > 0 {
				return "CALL", fmt.Sprintf("%s +%d", p.Lib, n)
			}
			return "CALL", p.Lib
		}
		return "CALL", skill.CallName(p.Call)
	}
	return "EMPTY", ""
}

// formDetail is a form's second line: what its tier makes it do.
func formDetail(p skill.Part) string {
	rule := p.Rule()
	switch p.Form {
	case skill.FormBolt:
		return [...]string{"nearest", "pierces", "pierce+split"}[rule]
	case skill.FormBeam:
		return [...]string{"line", "both ways", "fan"}[rule]
	case skill.FormAura:
		return [...]string{"round you", "+ allies", "+ marked"}[rule]
	case skill.FormNova:
		return [...]string{"burst", "twin burst", "wide"}[rule]
	case skill.FormDash:
		return [...]string{"dash in", "in and back", "back unseen"}[rule]
	case skill.FormDetonate:
		return [...]string{"1 wreck", "2 wrecks", "2, wide"}[rule]
	case skill.FormZone:
		return [...]string{"field", "drifts", "drifts, chills"}[rule]
	case skill.FormTurret:
		return [...]string{"1 stands", "2 stand", "3 stand"}[rule]
	case skill.FormTrap:
		return [...]string{"1 trigger", "3 triggers", "3, stuns"}[rule]
	case skill.FormDrone:
		return [...]string{"1 flies", "2 fly", "bursts at end"}[rule]
	}
	return ""
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
	case p.Kind == skill.KindTrigger && p.Trigger == skill.Channel:
		fmt.Fprintf(&b, ". While you hold CAST the hex runs every %g s, paying its cost each run, until you let go or overheat", skill.ChannelEvery)
	case p.Kind == skill.KindTrigger && p.Trigger == skill.Watch:
		fmt.Fprintf(&b, ". Checks every %g s, adding %g Heat each check whatever it finds. It fires when its condition holds for a foe within %g hexes (or for your rig), or when the library hex it watches returns anyone, and lands on who it found. Then it rests %g s. In a hex, tap it again to switch between a condition and a library hex",
			skill.WatchEvery, skill.WatchHeat, skill.WatchRange, skill.WatchRest)
	case p.Kind == skill.KindGate && p.Gate == skill.GateSpend:
		fmt.Fprintf(&b, ". Once bank %s holds %d it spends them and the next slot clockwise fires, else the slot after it",
			skill.BankName(p.Bank), p.Count)
	case p.Kind == skill.KindGate && p.Gate == skill.GateEmpty:
		fmt.Fprintf(&b, ". Next slot clockwise fires while bank %s is at zero, the slot after it otherwise", skill.BankName(p.Bank))
	case p.Kind == skill.KindGate && p.Gate == skill.GateCond:
		fmt.Fprintf(&b, ". Tests: %s. Next slot clockwise hits the ones that pass, the slot after it the rest. Tap it again in a hex to edit the condition", p.Cond.Text(reg))
	case p.Kind == skill.KindGate:
		b.WriteString(". Next slot clockwise hits the ones that pass, the slot after it the rest. Tap it again in a hex to edit it as a condition")
	case p.Kind == skill.KindEffect && p.Effect == skill.EffectMend:
		fmt.Fprintf(&b, ". Restores hull to you, or to the targets beside it. Each Mend that lands adds strain, which halves every %g s, and divides the next one by 1 + strain", float32(skill.StrainHalfLife))
	case p.Kind == skill.KindEffect && p.Effect == skill.EffectShield:
		fmt.Fprintf(&b, ". A buffer on you, or the targets beside it, that soaks hits for %g s, up to %.0f%% of max hull. Strains like Mend", float32(skill.ShieldTime), 100*float32(skill.ShieldCap))
	case p.Kind == skill.KindEffect && p.Effect == skill.EffectStatus && reg.Cloak(p.Type):
		fmt.Fprintf(&b, ". A self effect: hides you, whoever it sits beside, and hurts no one. Your next hit out of it lands at x%g, and that hit gives you away", float32(skill.AmbushMult))
	case p.Kind == skill.KindEffect && p.Effect == skill.EffectPurge:
		b.WriteString(". Takes Heat off your rig, paid in Power, whoever it sits beside")
	case p.Kind == skill.KindCharge:
		fmt.Fprintf(&b, ". Bank %s keeps its count between firings", skill.BankName(p.Bank))
	case p.RunsNow():
		fmt.Fprintf(&b, ". Runs library hex %s right here, on the group this slot lands on, then holds who it returns so the next slot can act on them. Adds its Heat (%.2gx), never its Power", p.Lib, 1/p.Rarity.Scale())
		for _, o := range p.Opts {
			if o.Value != 0 {
				fmt.Fprintf(&b, ". Option x%.2f %s", 1+(o.Value-1)*o.Rarity.Scale(), o.Verb)
			}
		}
	case p.Kind == skill.KindCall && p.Call == skill.CallSelf:
		fmt.Fprintf(&b, ". When this hex finishes it runs again, on whoever reached the call. Each run adds its Heat (%.2gx). Put it behind a gate or it loops until you overheat", 1/p.Rarity.Scale())
	case p.Kind == skill.KindCall:
		fmt.Fprintf(&b, ". When this hex finishes, %s runs too, on whoever reached the call. Each run adds that hex's Heat (%.2gx)", skill.CallName(p.Call), 1/p.Rarity.Scale())
	default:
		if t := ClassText(p); t != "" {
			b.WriteString(". ")
			b.WriteString(t)
		}
	}
	if p.UsesBank() {
		b.WriteString(". In a hex, tap the center plate to switch its bank")
	}
	if p.Kind == skill.KindCall {
		b.WriteString(". In a hex, tap the center plate to point it at another hex or a library hex")
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
	case skill.QuirkRider:
		return "Rider: it also leaves its type's status, or a status also stings."
	case skill.QuirkHeated:
		return "Heated: grows with your Heat, up to double near the top."
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
	out := strings.Join(parts, " · ")
	if len(parts) == 0 {
		out = "empty"
	}
	if h.Shape.Kind != skill.ShapeRing {
		out = strings.ToLower(h.Shape.Name()) + ": " + out
	}
	return out
}

// CondShort is a condition in a dozen characters, for a tile: its first
// test, and "+n" for the rest.
func CondShort(c skill.Cond, reg *skill.Registry) string {
	ts := c.Tiles()
	if len(ts) == 0 {
		return "empty"
	}
	n, first := 0, ""
	for i, t := range ts {
		if t.K.Sensor() {
			if n == 0 {
				first = t.Text(reg, skill.TileNone)
				if i+1 < len(ts) && ts[i+1].K.Compare() {
					first += " " + ts[i+1].Text(reg, t.K)
				}
			}
			n++
		}
	}
	if n > 1 {
		return fmt.Sprintf("%s +%d", first, n-1)
	}
	return first
}

// ClassText says what a class part does, in a sentence or two, or "" for
// a part from the original set.
func ClassText(p skill.Part) string {
	switch p.Kind {
	case skill.KindTrigger:
		switch p.Trigger {
		case skill.OnWreck:
			return fmt.Sprintf("Fires when a foe falls within %d hexes, on its wreck", skill.WreckRange)
		case skill.OnDodge:
			return "Fires when a shot passes close by you without hitting"
		case skill.OnVent:
			return "Fires when you VENT"
		}
	case skill.KindTarget:
		switch p.Target {
		case skill.TargetWreck:
			return fmt.Sprintf("Picks the nearest wreck within %g hexes: a foe's body, for %g s after it falls. Reanimate and detonate use it up", p.Value, float32(25))
		case skill.TargetWeakest:
			return "Picks the foes in reach with the least hull left, by share"
		case skill.TargetMarked:
			return "Picks marked foes in reach, nearest first"
		}
	case skill.KindEffect:
		switch p.Effect {
		case skill.EffectTaunt:
			return fmt.Sprintf("Holds the foes it lands on to you: they come at you, never flee, and hit %.0f%% softer", 100*skill.TauntCut)
		case skill.EffectThorns:
			return fmt.Sprintf("On you, wherever it sits: hands back %.0f%% of every hit you take", 100*skill.ThornsFrac)
		case skill.EffectReanimate:
			return fmt.Sprintf("Raises the wreck it lands on (or the nearest one) to fight for you. Up to %d at once", skill.MinionMax)
		case skill.EffectHaste:
			return fmt.Sprintf("On you, wherever it sits: %.0f%% faster", 100*skill.HasteFrac)
		case skill.EffectBlink:
			return "Jumps you toward the foe it lands on, or away from danger when nothing's beside it"
		}
	case skill.KindMult:
		if p.Verb == skill.VerbMarked {
			return "Multiplies the damage before it, on marked foes only"
		}
	case skill.KindGate:
		switch p.Gate {
		case skill.GateBehind:
			return "Next slot clockwise hits the foes facing away from you, the slot after it the rest"
		case skill.GateHurt:
			return fmt.Sprintf("Next slot clockwise fires while your hull is under %.0f%%, the slot after it otherwise", p.Value*100)
		}
		if p.Name == "finisher" {
			return fmt.Sprintf("Pays out once the combo on %s holds %d, then the next slot clockwise fires", skill.BankName(p.Bank), p.Count)
		}
	case skill.KindCharge:
		if p.Name == "combo" {
			return fmt.Sprintf("Builds a point on %s each time it runs; a finisher spends them", skill.BankName(p.Bank))
		}
	case skill.KindForm:
		return formText(p.Form)
	case skill.KindCost:
		if p.Cost == skill.CostWrecks {
			return "The hex adds no Heat: it uses up a wreck near you instead, and won't fire without one"
		}
		return fmt.Sprintf("The hex adds no Heat: it takes %.1f%% of your max hull per Heat instead, and won't fire on your last", 100*skill.BloodPerHeat)
	case skill.KindStance:
		var what string
		switch p.Stance {
		case skill.StanceBulwark:
			what = fmt.Sprintf("take %.0f%% less, move %.0f%% slower", 100*skill.BulwarkGuard, 100*skill.BulwarkSlow)
		case skill.StanceHunter:
			what = fmt.Sprintf("deal %.0f%% more, move %.0f%% faster, take %.0f%% more", 100*skill.HunterMight, 100*skill.HunterSpeed, 100*skill.HunterFrail)
		case skill.StanceFurnace:
			what = fmt.Sprintf("Heat cools %gx as fast, Power comes back at %gx", float32(skill.FurnaceCool), float32(skill.FurnaceCharge))
		}
		keep := "One stance at a time"
		if p.Rarity >= skill.Rare {
			keep = "Stays on beside another stance"
		}
		return fmt.Sprintf("A stance: cast it to switch it on, again to switch it off. While on: %s, and the rest of the hex runs every %g s. %s", what, float32(skill.StancePulse), keep)
	}
	return ""
}

func formText(f skill.FormKind) string {
	switch f {
	case skill.FormBolt:
		return "Hits the nearest foe. Rare: pierces the whole line. Exotic: pierces and splits"
	case skill.FormBeam:
		return "Hits every foe on a short line. Rare: both ways. Exotic: a wedge"
	case skill.FormAura:
		return "Hits every foe close round you; beside Mend or Shield it lands on you and your allies. Rare: round your allies too. Exotic: and every marked foe"
	case skill.FormNova:
		return "Bursts round where the hex fires from. Rare: round you too. Exotic: wider"
	case skill.FormDash:
		return "Carries you along a line to the nearest foe, hitting all of it. Rare: you dash back. Exotic: back, unseen"
	case skill.FormDetonate:
		return "Blows up the nearest wreck, hitting everything round it, and uses it up. Rare: the next wreck too. Exotic: both, wider"
	case skill.FormZone:
		return fmt.Sprintf("Leaves a field on the nearest foe that runs the hex every %g s for %g s, at %.0f%%. Rare: it drifts after foes. Exotic: and chills", float32(skill.ZoneEvery), float32(skill.ZoneLife), 100*skill.ZoneScale)
	case skill.FormTurret:
		return fmt.Sprintf("Places a turret beside you that runs the hex at the nearest foe every %g s for %g s. With an aura, it's a totem. Rare: two stand. Exotic: three", float32(skill.TurretEvery), float32(skill.TurretLife))
	case skill.FormTrap:
		return fmt.Sprintf("Leaves a trap where you stand that runs the hex on the first foe to step on it, for %g s. Rare: three triggers. Exotic: and it stuns", float32(skill.TrapLife))
	case skill.FormDrone:
		return fmt.Sprintf("Launches a drone that follows you and runs the hex at the nearest foe every %g s for %g s. Rare: two fly. Exotic: one last burst when it ends", float32(skill.DroneEvery), float32(skill.DroneLife))
	}
	return ""
}

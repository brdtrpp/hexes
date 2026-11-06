package loot

import "hexes/skill"

// The class parts: forms, the rogue's, the necromancer's, devices and
// oaths. They roll from their own families so the original tables, and
// every roll made with them, stay as they were.

// Families of class parts, one bit each in Allow.New.
const (
	// FamForms: bolt, beam, aura, nova and dash.
	FamForms uint16 = 1 << iota
	// FamShadow: the rogue's weakest and marked targets, behind gate,
	// marked multiplier, combo and finisher, blink and the dodge trigger.
	FamShadow
	// FamWrecks: the necromancer's wreck target, reanimate, detonate, the
	// wreck trigger and the wreck cost.
	FamWrecks
	// FamDevices: zone, turret, trap and drone.
	FamDevices
	// FamOaths: taunt, thorns, haste, the hurt gate, sacrifice, stances
	// and the vent trigger.
	FamOaths

	// FamAll is every family.
	FamAll uint16 = 1<<iota - 1
)

// NumFams is how many families there are.
const NumFams = 5

var famNames = [NumFams]string{"forms", "shadow", "wrecks", "devices", "oaths"}

// FamName is a family's name.
func FamName(f int) string { return famNames[f] }

// NewChance is how often a drop rolls from the class families when any
// are allowed.
const NewChance = 0.3

// RollNew makes one class part of a tier from the families Allow.New
// lets through (all of them if none).
func (t *Table) RollNew(r *RNG, rar skill.Rarity) skill.Part {
	fams := t.Allow.New
	if fams == 0 {
		fams = FamAll
	}
	var w [NumFams]float32
	for f := range w {
		if fams&(1<<f) != 0 {
			w[f] = 1
		}
	}
	return t.RollFam(r, r.pick(w[:]), rar)
}

// RollTaught is RollAt with the class families mixed in: when Allow.New
// lets any family through, NewChance of rolls come from them. Foe drops
// and the floor-end pick roll through it.
func (t *Table) RollTaught(r *RNG, rar skill.Rarity) skill.Part {
	if t.Allow.New != 0 && r.Float() < NewChance {
		return t.RollNew(r, rar)
	}
	return t.RollAt(r, rar)
}

// RollFam makes one part of family f at a tier.
func (t *Table) RollFam(r *RNG, f int, rar skill.Rarity) skill.Part {
	var p skill.Part
	switch f {
	case 0:
		p = skill.NewForm(skill.FormKind(r.Intn(int(skill.FormDetonate))))
	case 1:
		switch r.Intn(8) {
		case 0:
			p = skill.NewTarget(skill.TargetWeakest, 0, uint8(1+r.Intn(2)))
		case 1:
			p = skill.NewTarget(skill.TargetMarked, float32(skill.NearestReach), uint8(2+r.Intn(3)))
		case 2:
			// The shadow's marked parts need something that marks.
			if ty, ok := t.Reg.StatusOf(skill.StatusMark); ok {
				p = skill.NewStatus(ty, half(r.Range(3, 6)))
			} else {
				p = skill.NewBehindGate()
			}
		case 3:
			p = skill.NewMarkedMult(float32(int(r.Range(1.6, 2.4)*20)) / 20)
		case 4:
			p = skill.NewCombo(rar)
		case 5:
			p = skill.NewFinisher(rar)
		case 6:
			p = skill.NewBlink(float32(2 + r.Intn(3)))
		default:
			if rar >= skill.Rare {
				p = skill.NewTrigger(skill.OnDodge)
			} else {
				p = skill.NewBehindGate()
			}
		}
	case 2:
		switch r.Intn(5) {
		case 0:
			p = skill.NewTarget(skill.TargetWreck, skill.WreckReach, 1)
		case 1:
			p = skill.NewReanimate(half(r.Range(8, 14)))
		case 2:
			p = skill.NewForm(skill.FormDetonate)
		case 3:
			p = skill.NewTrigger(skill.OnWreck)
		default:
			if rar >= skill.Rare {
				p = skill.NewCost(skill.CostWrecks)
			} else {
				p = skill.NewReanimate(half(r.Range(8, 14)))
			}
		}
	case 3:
		p = skill.NewForm(skill.FormZone + skill.FormKind(r.Intn(4)))
	default:
		switch r.Intn(8) {
		case 0:
			p = skill.NewTaunt(half(r.Range(2.5, 4.5)))
		case 1:
			p = skill.NewThorns(half(r.Range(3, 5)))
		case 2:
			p = skill.NewHaste(half(r.Range(2, 4)))
		case 3:
			p = skill.NewHurtGate(twentieth(r.Range(0.35, 0.6)))
		case 4:
			p = skill.NewCost(skill.CostBlood)
		case 5, 6:
			p = skill.NewStance(skill.StanceKind(r.Intn(skill.NumStances)))
		default:
			if rar >= skill.Rare {
				p = skill.NewTrigger(skill.OnVent)
			} else {
				p = skill.NewTaunt(half(r.Range(2.5, 4.5)))
			}
		}
	}
	if p.Kind != skill.KindCharge && !(p.Kind == skill.KindGate && p.Gate == skill.GateSpend) {
		p = p.WithRarity(rar)
	}
	// What rarity adds that isn't a number: a Heat-scaled multiplier, a
	// rider on an effect.
	if rar == skill.Exotic && skill.QuirkHeated.Fits(p) {
		p.Quirk = skill.QuirkHeated
	}
	return p
}

// Rider makes a rare typed effect carrying a second effect: what an
// effect's rarity adds in the class table.
func (t *Table) Rider(r *RNG, rar skill.Rarity) skill.Part {
	ty := skill.TypeID(r.Intn(t.Reg.Len()))
	// Cloak never deals damage, so a cloak rider takes the next type over
	// (no extra draw, so seeded rolls stay as they were).
	for n := 0; n < t.Reg.Len() && t.Reg.Cloak(ty); n++ {
		ty = (ty + 1) % skill.TypeID(t.Reg.Len())
	}
	amt := float32(int(t.Damage[ty]*r.Range(0.8, 1.25) + 0.5))
	p := skill.NewDamage(ty, max(1, amt)).WithRarity(rar)
	p.Quirk = skill.QuirkRider
	return p
}

// FamOf is the class family part p comes from, or -1 for a part of the
// original tables.
func FamOf(p skill.Part) int {
	switch p.Kind {
	case skill.KindForm:
		switch {
		case p.Form == skill.FormDetonate:
			return 2
		case p.Form.Placed():
			return 3
		}
		return 0
	case skill.KindTarget:
		switch p.Target {
		case skill.TargetWeakest, skill.TargetMarked:
			return 1
		case skill.TargetWreck:
			return 2
		}
	case skill.KindTrigger:
		switch p.Trigger {
		case skill.OnDodge:
			return 1
		case skill.OnWreck:
			return 2
		case skill.OnVent:
			return 4
		}
	case skill.KindEffect:
		switch p.Effect {
		case skill.EffectBlink:
			return 1
		case skill.EffectReanimate:
			return 2
		case skill.EffectTaunt, skill.EffectThorns, skill.EffectHaste:
			return 4
		}
	case skill.KindMult:
		if p.Verb == skill.VerbMarked {
			return 1
		}
	case skill.KindGate:
		switch {
		case p.Gate == skill.GateBehind || p.Name == "finisher":
			return 1
		case p.Gate == skill.GateHurt:
			return 4
		}
	case skill.KindCharge:
		if p.Name == "combo" {
			return 1
		}
	case skill.KindCost:
		if p.Cost == skill.CostWrecks {
			return 2
		}
		return 4
	case skill.KindStance:
		return 4
	}
	return -1
}

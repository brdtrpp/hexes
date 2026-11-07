package game

import (
	"strings"

	"hexes/loot"
	"hexes/skill"
)

// Schools: the skill grid's six wedges are six classes, each with its own
// damage type and its own kinds of part. Past the shared middle rings a
// wedge only holds parts of its school, and drops and floor-end picks
// only bring parts of the schools whose wedges you've opened. The logic
// parts (charges, calls, spend and condition gates, the cast trigger, the
// self target, the all multiplier) are shared by every school.

// School is one wedge's class.
type School uint8

const (
	SchoolMage School = iota
	SchoolEngineer
	SchoolWarden
	SchoolBerserker
	SchoolNecro
	SchoolRogue
	NumSchools
)

// SchoolSet is a set of schools, one bit each.
type SchoolSet uint8

// AllSchools is every school: a shared basic.
const AllSchools SchoolSet = 1<<NumSchools - 1

// BasicRings is how many rings out from the middle hold shared basics:
// any part a ring can hold, in any wedge. Damage and status types follow
// the wedge from ring 1 all the same.
const BasicRings = 3

var schoolNames = [NumSchools]string{"MAGE", "ENGINEER", "WARDEN", "BERSERKER", "NECROMANCER", "ROGUE"}

// schoolType is each school's damage type.
var schoolType = [NumSchools]string{"plasma", "cryo", "kinetic", "nanite", "drone", "cloak"}

var schoolIdea = [NumSchools]string{
	"plasma, bolts, beams, novas and auras",
	"cryo, turrets, traps, zones and drones",
	"kinetic, taunt, thorns, shields and the bulwark",
	"nanite, haste, blood costs and the hunter",
	"drone, wrecks, raising and detonating",
	"cloak, marks, blinks, combos and backstabs",
}

func (s School) String() string {
	if s < NumSchools {
		return schoolNames[s]
	}
	return "SHARED"
}

// Idea is the one line a school's wedge is about.
func (s School) Idea() string {
	if s < NumSchools {
		return schoolIdea[s]
	}
	return ""
}

// Bit is the set holding only s.
func (s School) Bit() SchoolSet { return 1 << s }

// Has reports whether s is in the set.
func (ss SchoolSet) Has(s School) bool { return ss&s.Bit() != 0 }

// Names is the set's school names, "Mage / Rogue", or "Shared" for all.
func (ss SchoolSet) Names() string {
	if ss == AllSchools {
		return "Shared"
	}
	var out []string
	for s := School(0); s < NumSchools; s++ {
		if ss.Has(s) {
			n := schoolNames[s]
			out = append(out, n[:1]+strings.ToLower(n[1:]))
		}
	}
	return strings.Join(out, " / ")
}

// TypeSchool is the school a damage type belongs to.
func TypeSchool(reg *skill.Registry, t skill.TypeID) School {
	name := reg.Get(t).Name
	for s, n := range schoolType {
		if n == name {
			return School(s)
		}
	}
	return School(int(t) % int(NumSchools))
}

// SchoolType is a school's damage type.
func SchoolType(reg *skill.Registry, s School) skill.TypeID {
	if id, ok := reg.ID(schoolType[s]); ok {
		return id
	}
	return skill.TypeID(int(s) % reg.Len())
}

// WedgeSchool is the school of grid wedge w.
func (g *Grid) WedgeSchool(reg *skill.Registry, w int) School {
	return TypeSchool(reg, g.Types[w%6])
}

func set(ss ...School) SchoolSet {
	var out SchoolSet
	for _, s := range ss {
		out |= s.Bit()
	}
	return out
}

const (
	mage = SchoolMage
	engi = SchoolEngineer
	ward = SchoolWarden
	bers = SchoolBerserker
	necr = SchoolNecro
	rogu = SchoolRogue
)

// Typed reports whether a part takes a damage type: a damage or status
// effect, or a status gate.
func Typed(p skill.Part) bool {
	return (p.Kind == skill.KindEffect && (p.Effect == skill.EffectDamage || p.Effect == skill.EffectStatus)) ||
		(p.Kind == skill.KindGate && p.Gate == skill.GateStatus)
}

// SchoolOf is the schools a part belongs to.
func SchoolOf(p skill.Part, reg *skill.Registry) SchoolSet {
	if Typed(p) {
		return TypeSchool(reg, p.Type).Bit()
	}
	switch p.Kind {
	case skill.KindTrigger:
		switch p.Trigger {
		case skill.OnHit:
			return set(bers, rogu, ward)
		case skill.OnKill:
			return set(necr, mage, bers)
		case skill.Periodic:
			return set(engi, mage, ward)
		case skill.OnHurt:
			return set(ward, bers)
		case skill.Channel:
			return set(mage, engi)
		case skill.Watch:
			return set(engi, rogu)
		case skill.OnWreck:
			return set(necr)
		case skill.OnDodge, skill.FromStealth, skill.FromBehind:
			return set(rogu)
		case skill.OnVent:
			return set(bers, engi)
		case skill.Overload:
			return set(mage)
		case skill.Guard:
			return set(ward)
		case skill.Rage:
			return set(bers)
		case skill.Blueprint:
			return set(engi)
		case skill.Brain:
			return set(necr)
		case skill.FollowUp:
			return set(rogu)
		}
	case skill.KindTarget:
		switch p.Target {
		case skill.TargetNearest:
			return set(mage, necr, engi, rogu)
		case skill.TargetCone:
			return set(mage, rogu, bers)
		case skill.TargetAdjacent:
			return set(ward, bers, necr)
		case skill.TargetChain:
			return set(mage, engi)
		case skill.TargetWreck:
			return set(necr)
		case skill.TargetWeakest:
			return set(rogu, necr)
		case skill.TargetMarked:
			return set(rogu)
		case skill.TargetLine, skill.TargetBoth:
			return set(mage, engi)
		case skill.TargetAllies:
			return set(ward, necr)
		case skill.TargetAura:
			return set(ward, bers)
		}
	case skill.KindEffect:
		switch p.Effect {
		case skill.EffectPush:
			return set(ward, bers)
		case skill.EffectMend:
			return set(ward, necr, rogu)
		case skill.EffectPurge:
			return set(engi, bers, mage)
		case skill.EffectShield:
			return set(ward, engi)
		case skill.EffectTaunt:
			return set(ward)
		case skill.EffectThorns:
			return set(ward, bers)
		case skill.EffectReanimate:
			return set(necr)
		case skill.EffectHaste:
			return set(bers, rogu)
		case skill.EffectBlink:
			return set(rogu, mage)
		}
	case skill.KindMult:
		switch p.Verb {
		case skill.VerbDamage:
			return set(bers, mage, rogu, necr)
		case skill.VerbPush:
			return set(ward, bers)
		case skill.VerbDuration:
			return set(engi, necr, rogu)
		case skill.VerbCount:
			return set(mage, necr, engi)
		case skill.VerbRadius:
			return set(mage, ward, engi)
		case skill.VerbMarked:
			return set(rogu)
		}
	case skill.KindGate:
		switch p.Gate {
		case skill.GateHull:
			return set(ward, necr, rogu)
		case skill.GateHeat:
			return set(bers, engi)
		case skill.GatePower:
			return set(engi, mage)
		case skill.GateCrowd:
			return set(mage, ward, necr)
		case skill.GateBehind:
			return set(rogu)
		case skill.GateHurt:
			return set(bers)
		}
	case skill.KindForm:
		switch p.Form {
		case skill.FormBolt, skill.FormBeam, skill.FormNova:
			return set(mage)
		case skill.FormAura:
			return set(mage, ward)
		case skill.FormDash:
			return set(bers, rogu)
		case skill.FormDetonate:
			return set(necr)
		}
		return set(engi) // zones, turrets, traps and drones
	case skill.KindCost:
		if p.Cost == skill.CostWrecks {
			return set(necr)
		}
		return set(bers)
	case skill.KindStance:
		switch p.Stance {
		case skill.StanceBulwark:
			return set(ward)
		case skill.StanceHunter:
			return set(bers, rogu)
		}
		return set(engi, mage) // the furnace
	}
	// Charges, calls, spend, empty and condition gates, the cast trigger,
	// the self target and the all multiplier: logic every school uses.
	return AllSchools
}

// SchoolRolls is how many times a drop or a floor-end pick rerolls for a
// part of a school you've opened before it takes the last roll's type
// over to one of yours.
const SchoolRolls = 24

// OpenSchools is the schools a drop or a floor-end pick may bring: those
// whose wedges you own a cell in, and those of the damage types your
// online hexes use. With none of either (the first floors), every school.
func (s *Sim) OpenSchools() SchoolSet {
	var ss SchoolSet
	if s.Grid != nil {
		for i, o := range s.Own {
			if !o || i >= len(s.Grid.Nodes) {
				continue
			}
			if n := &s.Grid.Nodes[i]; n.Ring > 0 && n.Kind != NodeVoid {
				ss |= s.Grid.WedgeSchool(s.Reg, n.Wedge).Bit()
			}
		}
	}
	for i := 0; i < MaxSlots; i++ {
		if !s.HexOpen(i) || s.Load.Hexes[i] == nil {
			continue
		}
		for _, sl := range s.Load.Hexes[i].Slots {
			if sl.Kind == skill.SlotPart && Typed(sl.Part) && int(sl.Part.Type) < s.Reg.Len() {
				ss |= TypeSchool(s.Reg, sl.Part.Type).Bit()
			}
		}
	}
	if ss == 0 {
		return AllSchools
	}
	return ss
}

// The lean: Necromancer, Engineer and Warden hexes are about their
// signature parts (raising, devices, oaths), so when a drop or a pick
// for one of them comes up a plain damage or status effect, LeanChance
// of the time it's one of the school's own parts instead.
const LeanChance = 0.5

// leanFam is each leaning school's family of signature parts, -1 for a
// school that doesn't lean.
var leanFam = [NumSchools]int{-1, 3, 4, -1, 2, -1}

// leaning wraps roll with the lean, drawing signature parts from t at
// rarity rar. A family the ladder hasn't let in yet stays out.
func (s *Sim) leaning(r *loot.RNG, t *loot.Table, rar skill.Rarity, open SchoolSet, roll func() skill.Part) func() skill.Part {
	return func() skill.Part {
		p := roll()
		if !Typed(p) || p.Kind != skill.KindEffect {
			return p
		}
		sc := TypeSchool(s.Reg, p.Type)
		f := -1
		if sc < NumSchools {
			f = leanFam[sc]
		}
		if f < 0 || !open.Has(sc) || t.Allow.New&(1<<f) == 0 || r.Float() >= LeanChance {
			return p
		}
		for k := 0; k < 4; k++ {
			if q := t.RollFam(r, f, rar); SchoolOf(q, s.Reg).Has(sc) {
				return q
			}
		}
		return p
	}
}

// schoolRoll rolls until a part of an open school comes up (and ok likes
// it, when ok isn't nil). When none does in SchoolRolls goes, it keeps a
// roll ok liked (a typed one if there was one) and a typed part takes an
// open school's type. An untyped part off your schools is then the rare
// special find.
func (s *Sim) schoolRoll(r *loot.RNG, open SchoolSet, roll func() skill.Part, ok func(skill.Part) bool) skill.Part {
	var p, liked skill.Part
	got := false
	for i := 0; i < SchoolRolls; i++ {
		p = roll()
		if ok != nil && !ok(p) {
			continue
		}
		if SchoolOf(p, s.Reg)&open != 0 {
			return p
		}
		if !got || Typed(p) {
			liked, got = p, true
		}
	}
	if got {
		p = liked
	}
	if Typed(p) {
		return s.retype(r, p, open)
	}
	return p
}

// retype moves a typed part over to the type of one of the open schools,
// its damage scaled to the new type's.
func (s *Sim) retype(r *loot.RNG, p skill.Part, open SchoolSet) skill.Part {
	var opts [NumSchools]School
	n := 0
	for sc := School(0); sc < NumSchools; sc++ {
		if open.Has(sc) {
			opts[n] = sc
			n++
		}
	}
	if n == 0 {
		return p
	}
	t := SchoolType(s.Reg, opts[r.Intn(n)])
	tab := s.Stash.table
	if p.Kind == skill.KindEffect && p.Effect == skill.EffectDamage && int(p.Type) < len(tab.Damage) && int(t) < len(tab.Damage) && tab.Damage[p.Type] > 0 {
		p.Value = max(1, float32(int(p.Value*tab.Damage[t]/tab.Damage[p.Type]+0.5)))
	}
	p.Type = t
	p, _ = s.Reg.SelfOnly(p)
	return p
}

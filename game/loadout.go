package game

import (
	"fmt"

	"hexes/skill"
)

// MaxSlots is how many hexes the slice equips.
const MaxSlots = 4

// Loadout is the slice's preset build: a free basic strike plus four
// hexes, one per kind of trigger, so every part of dispatch shows up in
// play. Real loot will replace this.
type Loadout struct {
	Strike *skill.Program
	Progs  [MaxSlots]*skill.Program
	Names  [MaxSlots]string
	Blurb  [MaxSlots]string
}

// BuildLoadout compiles the preset hexes for a level. Damage grows with
// level the same way enemy health does (as if loot found deeper rolls
// bigger numbers); Power and Heat costs stay at their base so the rig
// keeps up.
func BuildLoadout(reg *skill.Registry, level, maxDepth int) Loadout {
	ty := reg.MustID
	sc := scale(level)
	var l Loadout
	build := func(name string, parts ...skill.Part) *skill.Program {
		h := skill.NewHex(name, [6]skill.SocketKind{skill.SocketTrigger,
			skill.SocketWild, skill.SocketWild, skill.SocketWild, skill.SocketWild, skill.SocketWild})
		for i, p := range parts {
			if err := h.Set(i, p); err != nil {
				panic(fmt.Sprintf("loadout %s slot %d: %v", name, i, err))
			}
		}
		p, err := skill.Compile(h, reg, maxDepth)
		if err != nil {
			panic(fmt.Sprintf("loadout %s: %v", name, err))
		}
		for i := range p.Steps {
			if p.Steps[i].Op == skill.OpDamage {
				p.Steps[i].Value *= sc
			}
		}
		return p
	}

	// Basic strike: whatever is next to you, plasma, splashes.
	l.Strike = build("Strike",
		skill.NewTrigger(skill.OnCast),
		skill.NewDamage(ty("plasma"), 16))
	l.Strike.Power, l.Strike.Heat = 0, 0

	// Slot 1, on cast: lightning that hops three foes, doubled.
	l.Progs[0] = build("Arc Lance",
		skill.NewTrigger(skill.OnCast),
		skill.NewTarget(skill.TargetChain, 4, 3),
		skill.NewDamage(ty("plasma"), 30),
		skill.NewMult(skill.VerbDamage, 2))
	l.Names[0], l.Blurb[0] = "Arc Lance", "plasma chains 3 foes"

	// Slot 2, on hit: every so often a strike shoves its target three hexes.
	// Pits and walls do the killing.
	l.Progs[1] = build("Kinetic Ram",
		skill.NewTrigger(skill.OnHit),
		skill.NewPush(2),
		skill.NewDamage(ty("kinetic"), 6))
	l.Names[1], l.Blurb[1] = "Kinetic Ram", "shoves 3 hexes, into pits"

	// Slot 3, on kill: the corpse bursts cold, chilling and hurting what's
	// around it. Cryo parts vent heat, so killing cools the rig.
	l.Progs[2] = build("Cryo Bloom",
		skill.NewTrigger(skill.OnKill),
		skill.NewDamage(ty("cryo"), 14),
		skill.NewTarget(skill.TargetAdjacent, 2, 0),
		skill.NewStatus(ty("cryo"), 2.5))
	l.Names[2], l.Blurb[2] = "Cryo Bloom", "chill burst, vents heat"

	// Slot 4, periodic: a nanite cloud eats everything near you.
	l.Progs[3] = build("Nanite Swarm",
		skill.NewPeriodic(4),
		skill.NewTarget(skill.TargetAdjacent, 2, 0),
		skill.NewDamage(ty("nanite"), 30))
	l.Names[3], l.Blurb[3] = "Nanite Swarm", "toxin cloud around you"
	return l
}

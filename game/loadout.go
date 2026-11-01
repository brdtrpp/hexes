package game

import (
	"fmt"

	"hexes/loot"
	"hexes/skill"
)

// MaxSlots is how many hexes the slice equips.
const MaxSlots = 4

// Loadout is the player's build: a free basic strike plus four hexes. The
// hexes start as a preset, one per kind of trigger, and the player swaps
// looted parts into them. Hexes persist for the run; Progs are their
// compiled form at the current level and are rebuilt on every floor and
// every swap.
type Loadout struct {
	Strike *skill.Program
	Hexes  [MaxSlots]*skill.Hex
	Progs  [MaxSlots]*skill.Program
	Names  [MaxSlots]string
	Blurb  [MaxSlots]string
	// Edited marks hexes the player has changed, whose blurb is generated.
	Edited [MaxSlots]bool
}

// LoadoutSockets is the socket layout every slice hex uses: the trigger
// in slot 0, wild sockets everywhere else.
var LoadoutSockets = [skill.Slots]skill.SocketKind{skill.SocketTrigger,
	skill.SocketWild, skill.SocketWild, skill.SocketWild, skill.SocketWild, skill.SocketWild}

// BuildLoadout makes the preset hexes and compiles them for a level.
func BuildLoadout(reg *skill.Registry, level, maxDepth int) Loadout {
	var l Loadout
	l.Hexes, l.Names, l.Blurb = PresetHexes(reg)
	if err := l.Rebuild(reg, level, maxDepth); err != nil {
		panic(err)
	}
	return l
}

// PresetHexes is the starting build, with every edge linked so it plays
// the same every run.
func PresetHexes(reg *skill.Registry) (hexes [MaxSlots]*skill.Hex, names, blurbs [MaxSlots]string) {
	ty := reg.MustID
	mk := func(i int, name, blurb string, parts ...skill.Part) {
		h := skill.NewHex(name, LoadoutSockets)
		for j, p := range parts {
			if err := h.Set(j, p); err != nil {
				panic(fmt.Sprintf("loadout %s slot %d: %v", name, j, err))
			}
		}
		hexes[i], names[i], blurbs[i] = h, name, blurb
	}
	// Slot 1, on cast: lightning that hops three foes, doubled.
	mk(0, "Arc Lance", "plasma chains 3 foes",
		skill.NewTrigger(skill.OnCast),
		skill.NewTarget(skill.TargetChain, 4, 3),
		skill.NewDamage(ty("plasma"), 30),
		skill.NewMult(skill.VerbDamage, 2))
	// Slot 2, on hit: every so often a strike shoves its target three hexes.
	// Pits and walls do the killing.
	mk(1, "Kinetic Ram", "shoves 3 hexes, into pits",
		skill.NewTrigger(skill.OnHit),
		skill.NewPush(2),
		skill.NewDamage(ty("kinetic"), 6))
	// Slot 3, on kill: the corpse bursts cold, chilling and hurting what's
	// around it. Cryo parts vent heat, so killing cools the rig.
	mk(2, "Cryo Bloom", "chill burst, vents heat",
		skill.NewTrigger(skill.OnKill),
		skill.NewDamage(ty("cryo"), 14),
		skill.NewTarget(skill.TargetAdjacent, 2, 0),
		skill.NewStatus(ty("cryo"), 2.5))
	// Slot 4, periodic: a nanite cloud eats everything near you.
	mk(3, "Nanite Swarm", "toxin cloud around you",
		skill.NewPeriodic(4),
		skill.NewTarget(skill.TargetAdjacent, 2, 0),
		skill.NewDamage(ty("nanite"), 30))
	return
}

// CompileHex compiles one hex for a level. Damage grows with level the
// same way enemy health does (as if deeper loot rolls bigger numbers);
// Power and Heat costs stay at their base so the rig keeps up.
func CompileHex(h *skill.Hex, reg *skill.Registry, level, maxDepth int) (*skill.Program, error) {
	p, err := skill.Compile(h, reg, maxDepth)
	if err != nil {
		return nil, err
	}
	sc := scale(level)
	for i := range p.Steps {
		if p.Steps[i].Op == skill.OpDamage {
			p.Steps[i].Value *= sc
		}
	}
	return p, nil
}

// Rebuild recompiles the strike and every hex for a level.
func (l *Loadout) Rebuild(reg *skill.Registry, level, maxDepth int) error {
	strike := skill.NewHex("Strike", LoadoutSockets)
	_ = strike.Set(0, skill.NewTrigger(skill.OnCast))
	_ = strike.Set(1, skill.NewDamage(reg.MustID("plasma"), 16))
	s, err := CompileHex(strike, reg, level, maxDepth)
	if err != nil {
		return err
	}
	s.Power, s.Heat = 0, 0
	l.Strike = s
	for i := range l.Hexes {
		if err := l.Compile(i, reg, level, maxDepth); err != nil {
			return fmt.Errorf("loadout %s: %w", l.Names[i], err)
		}
	}
	return nil
}

// Compile recompiles one slot. On error the old program stays.
func (l *Loadout) Compile(slot int, reg *skill.Registry, level, maxDepth int) error {
	p, err := CompileHex(l.Hexes[slot], reg, level, maxDepth)
	if err != nil {
		return err
	}
	l.Progs[slot] = p
	if l.Edited[slot] {
		l.Blurb[slot] = loot.Summary(l.Hexes[slot], reg)
	}
	return nil
}

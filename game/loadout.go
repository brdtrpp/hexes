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
	// Mods are modules: hexes with no trigger that only run docked (see
	// board.go). Spots is where each hex sits on the board, the equipped
	// ones first, then the modules.
	Mods  []*skill.Hex
	Spots []Spot
	// Keys is each hex's cast button, 1 to Buttons, 0 for none yet (see
	// bench.go). Only on-cast and hold hexes answer a button.
	Keys [MaxSlots]uint8
	// Undocked turns the board off: no hex docks to another, so hexes
	// the teaching ladder hasn't brought online can't run anything.
	Undocked bool
}

// StarterCallTarget is the hex the starting Call points at: Nanite Swarm.
const StarterCallTarget = 4

// LoadoutSockets is the socket layout every slice hex uses: the trigger
// in slot 0, wild sockets everywhere else.
var LoadoutSockets = [skill.RingSlots]skill.SocketKind{skill.SocketTrigger,
	skill.SocketWild, skill.SocketWild, skill.SocketWild, skill.SocketWild, skill.SocketWild}

// BuildLoadout makes the preset hexes and compiles them for a level.
func BuildLoadout(reg *skill.Registry, level, maxDepth int) Loadout {
	var l Loadout
	l.Hexes, l.Names, l.Blurb = PresetHexes(reg)
	l.FixSpots()
	if err := l.Rebuild(reg, level, maxDepth, -1); err != nil {
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
	// Slot 1, on cast: lightning that hops three foes, doubled. Slot 4 is
	// left open, and slot 5 holds a Call: once the lance is done, Nanite
	// Swarm (hex 4) goes off too, and its Heat lands on the rig on top of
	// the lance's.
	mk(0, "Arc Lance", "plasma chains 3, calls Swarm",
		skill.NewTrigger(skill.OnCast),
		skill.NewTarget(skill.TargetChain, 4, 3),
		skill.NewDamage(ty("plasma"), 30),
		skill.NewMult(skill.VerbDamage, 2))
	if err := hexes[0].Set(5, skill.NewCall(StarterCallTarget)); err != nil {
		panic(err)
	}
	// Slot 2, on hit: every so often a strike shoves its target, pits and
	// walls do the killing. Then a gate: a foe under 40% hull takes a heavy
	// kinetic jab (the yes slot), a healthier one a light one (the no slot).
	mk(1, "Kinetic Ram", "shove, then hits weak foes 3x",
		skill.NewTrigger(skill.OnHit),
		skill.NewPush(2),
		skill.NewGate(skill.GateHull, 0.4),
		skill.NewDamage(ty("kinetic"), 18),
		skill.NewDamage(ty("kinetic"), 6))
	// Slot 3, on kill: every kill chills what's around the corpse and
	// charges bank A. On the fifth, the spend gate empties A and its yes
	// slot sets off a big cryo nova on everything around. Cryo parts vent
	// heat, so the nova cools the rig too.
	mk(2, "Cryo Bloom", "chill, every 5th kill a nova",
		skill.NewTrigger(skill.OnKill),
		skill.NewCharge(0, 1),
		skill.NewStatus(ty("cryo"), 2.5),
		skill.NewTarget(skill.TargetAdjacent, 2, 0),
		skill.NewSpendGate(0, 5),
		skill.NewDamage(ty("cryo"), 45))
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

// Rebuild recompiles the strike and every hex for a level, with docks
// from where the hexes sit on the board. It's all or nothing: on error
// the old programs stay. edited (or -1) is the hex just changed: it's
// compiled first, so its own error comes back unwrapped, and its blurb is
// regenerated.
func (l *Loadout) Rebuild(reg *skill.Registry, level, maxDepth, edited int) error {
	strike := skill.NewHex("Strike", LoadoutSockets)
	_ = strike.Set(0, skill.NewTrigger(skill.OnCast))
	_ = strike.Set(1, skill.NewDamage(reg.MustID("plasma"), 16))
	s, err := CompileHex(strike, reg, level, maxDepth)
	if err != nil {
		return err
	}
	s.Power, s.Heat = 0, 0
	l.Dock()
	if l.IsMod(edited) {
		if _, err := skill.CompileModule(l.Hex(edited), reg, maxDepth); err != nil {
			return err
		}
	}
	var progs [MaxSlots]*skill.Program
	order := []int{0, 1, 2, 3}
	if edited >= 0 && edited < MaxSlots {
		order[0], order[edited] = edited, 0
	}
	for _, i := range order {
		p, err := CompileHex(l.Hexes[i], reg, level, maxDepth)
		if err != nil {
			if i == edited {
				return err
			}
			return fmt.Errorf("%s: %w", l.Names[i], err)
		}
		progs[i] = p
	}
	l.Strike, l.Progs = s, progs
	l.FixKeys()
	for i := range l.Hexes {
		if l.Edited[i] {
			l.Blurb[i] = loot.Summary(l.Hexes[i], reg)
		}
	}
	return nil
}

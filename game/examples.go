package game

import (
	"encoding/json"
	"errors"
	"fmt"

	"hexes/skill"
)

// The nine example skills from the examples doc, plus Field Medic for
// the sustain parts, built as real hexes so
// they can be played. Loading one (with the test switch on) puts its
// hexes in the first equipped slots and its library hexes on the board;
// the player's own build waits in Mine until they ask for it back.

// Example is one example skill: its equipped hexes and library hexes.
type Example struct {
	Name, Blurb string
	Hexes       []ExampleHex
	Mods        []*skill.Hex
}

// ExampleHex is one equipped hex of an example.
type ExampleHex struct {
	Hex         *skill.Hex
	Name, Blurb string
}

// MyBuild is the player's own build, kept while an example is loaded.
type MyBuild struct {
	Hexes  [MaxSlots]*skill.Hex
	Names  [MaxSlots]string
	Blurb  [MaxSlots]string
	Edited [MaxSlots]bool
	Mods   []*skill.Hex `json:",omitempty"`
	Spots  []Spot       `json:",omitempty"`
	Keys   [MaxSlots]uint8
}

// exHex builds an example hex: its parts by slot (Center for a Fork's
// middle), every edge linked. mod makes it a library hex.
func exHex(name string, sh skill.Shape, mod bool, parts map[int]skill.Part) *skill.Hex {
	h := &skill.Hex{Name: name, Shape: sh}
	h.Sockets = ShapeSockets(sh)
	if mod {
		h.Sockets = ModuleSockets()
	}
	for i := range h.Linked {
		h.Linked[i] = true
	}
	for i, p := range parts {
		h.Slots[i] = skill.Slot{Kind: skill.SlotPart, Part: p}
	}
	return h
}

func tiles(ts ...skill.Tile) skill.Cond {
	var c skill.Cond
	copy(c[:], ts)
	return c
}

func condGate(ts ...skill.Tile) skill.Part {
	return skill.NewGate(skill.GateHull, 0.5).WithCond(tiles(ts...))
}

// Examples builds the examples for a type registry.
func Examples(reg *skill.Registry) []Example {
	ty := reg.MustID
	ring := skill.Shape{Kind: skill.ShapeRing}
	tri := skill.Shape{Kind: skill.ShapeTriangle}
	X := uint8(skill.MaxBanks)
	T := func(k skill.TileKind, v float32) skill.Tile { return skill.Tile{K: k, V: v} }
	hull, heat, power := T(skill.TileHull, 0), T(skill.TileHeat, 0), T(skill.TilePower, 0)
	less := func(v float32) skill.Tile { return T(skill.TileLess, v) }
	more := func(v float32) skill.Tile { return T(skill.TileMore, v) }
	and, or := T(skill.TileAnd, 0), T(skill.TileOr, 0)
	crowd1 := T(skill.TileCrowd, 1)

	frostbite := func() *skill.Hex {
		return exHex("Frostbite", tri, true, map[int]skill.Part{2: skill.NewStatus(ty("cryo"), 2.5)})
	}
	pack := func() *skill.Hex {
		return exHex("Pack", tri, true, map[int]skill.Part{
			1: skill.NewTarget(skill.TargetAdjacent, 3, 0),
			2: condGate(crowd1, more(1)),
		})
	}
	chill := func() skill.Part {
		c, _ := skill.NewLibCall("Frostbite").WithOpt(skill.NewMult(skill.VerbDuration, 1.5))
		return c
	}
	coldSnap := func(nova bool) *skill.Hex {
		parts := map[int]skill.Part{
			0: skill.NewTrigger(skill.OnCast),
			1: skill.NewTarget(skill.TargetCone, 3, 0),
			2: chill(),
		}
		if nova {
			parts[3] = skill.NewSpendGate(X, 10)
			parts[4] = skill.NewDamage(ty("cryo"), 45)
		}
		return exHex("Cold Snap", ring, false, parts)
	}
	ricochet := func() *skill.Hex {
		return exHex("Ricochet", skill.Shape{Kind: skill.ShapeLoop}, false, map[int]skill.Part{
			0: skill.NewTrigger(skill.OnHit),
			1: skill.NewDamage(ty("plasma"), 18),
			3: skill.NewTarget(skill.TargetNearest, 0, 1),
			4: condGate(heat, less(0.8)),
			5: skill.NewStatus(ty("plasma"), 1),
		})
	}
	tripwire := func() *skill.Hex {
		w := skill.NewTrigger(skill.Watch)
		w.Lib = "Pack"
		return exHex("Tripwire", ring, false, map[int]skill.Part{
			0: w,
			2: skill.NewStatus(ty("cryo"), 2.5),
			3: skill.NewPush(1),
		})
	}
	one := func(h *skill.Hex, blurb string) []ExampleHex { return []ExampleHex{{h, h.Name, blurb}} }

	return []Example{
		{Name: "Brawler", Blurb: "starter parts and a two-sensor condition",
			Hexes: one(exHex("Brawler", ring, false, map[int]skill.Part{
				0: skill.NewTrigger(skill.OnCast),
				1: skill.NewTarget(skill.TargetAdjacent, 2, 0),
				2: condGate(crowd1, more(1), and, hull, less(0.5)),
				3: skill.NewPush(2),
				4: skill.NewDamage(ty("kinetic"), 18),
				5: skill.NewMult(skill.VerbDamage, 2),
			}), "shoves hurt, bunched foes; hits the rest")},
		{Name: "Cold Snap", Blurb: "a library hex called by name, with an option",
			Hexes: one(coldSnap(false), "a cone, chilled by Frostbite"),
			Mods:  []*skill.Hex{frostbite()}},
		{Name: "Grenade", Blurb: "a finder library hex that returns a group",
			Hexes: one(exHex("Grenade", ring, false, map[int]skill.Part{
				0: skill.NewTrigger(skill.OnCast),
				1: skill.NewLibCall("Pack"),
				2: skill.NewDamage(ty("plasma"), 25),
				3: skill.NewMult(skill.VerbDamage, 1.5),
			}), "plasma on whoever Pack finds"),
			Mods: []*skill.Hex{pack()}},
		{Name: "Overcharge", Blurb: "a counter on bank A",
			Hexes: one(exHex("Overcharge", ring, false, map[int]skill.Part{
				0: skill.NewTrigger(skill.OnHit),
				1: skill.NewCharge(0, 1),
				2: skill.NewTarget(skill.TargetChain, 4, 3),
				3: skill.NewSpendGate(0, 8),
				4: skill.NewDamage(ty("plasma"), 50),
			}), "every 8th hit, a chained bolt")},
		{Name: "Overclock", Blurb: "a compound condition on the rig and each foe",
			Hexes: one(exHex("Overclock", ring, false, map[int]skill.Part{
				0: skill.NewTrigger(skill.OnCast),
				1: skill.NewTarget(skill.TargetCone, 3, 0),
				2: condGate(heat, less(0.3), and, power, more(0.6), or, hull, less(0.2)),
				3: skill.NewDamage(ty("plasma"), 45),
				4: skill.NewDamage(ty("plasma"), 12),
			}), "big hits on a cool, charged rig")},
		{Name: "Feint", Blurb: "a fork: two chains, one after the other",
			Hexes: one(exHex("Feint", skill.Shape{Kind: skill.ShapeFork}, false, map[int]skill.Part{
				skill.Center: skill.NewTrigger(skill.OnHurt),
				1:            skill.NewPush(2),
				2:            skill.NewStatus(ty("kinetic"), 1.5),
				3:            skill.NewTarget(skill.TargetSelf, 0, 0),
				4:            skill.NewStatus(ty("cloak"), 3),
			}), "shove the attacker, then vanish")},
		{Name: "Ricochet", Blurb: "a loop that Heat stops",
			Hexes: one(ricochet(), "bounces plasma until 80% Heat")},
		{Name: "Tripwire", Blurb: "a watch trigger on a finder",
			Hexes: one(tripwire(), "chills and shoves packs by itself"),
			Mods:  []*skill.Hex{pack()}},
		{Name: "Deep Freeze", Blurb: "four hexes and two library hexes at once",
			Hexes: []ExampleHex{
				{coldSnap(true), "Cold Snap", "chill a cone; nova at X 10"},
				{exHex("Shatter", ring, false, map[int]skill.Part{
					0: skill.NewTrigger(skill.OnHit),
					1: skill.NewTarget(skill.TargetAdjacent, 2, 0),
					2: condGate(skill.Tile{K: skill.TileHas, T: ty("cryo")}),
					3: skill.NewDamage(ty("kinetic"), 20),
					4: skill.NewLibCall("Frostbite"),
					5: skill.NewCharge(X, 1),
				}), "Shatter", "hit the chilled, chill the rest"},
				{ricochet(), "Ricochet", "bounces plasma until 80% Heat"},
				{tripwire(), "Tripwire", "chills and shoves packs by itself"},
			},
			Mods: []*skill.Hex{frostbite(), pack()}},
		{Name: "Field Medic", Blurb: "Mend, Shield and Purge keeping you going",
			Hexes: []ExampleHex{
				{exHex("Leech", ring, false, map[int]skill.Part{
					0: skill.NewTrigger(skill.OnKill),
					1: skill.NewMend(0.08),
				}), "Leech", "every kill mends you"},
				{exHex("Lifeline", ring, false, map[int]skill.Part{
					0: skill.NewTrigger(skill.OnHurt),
					1: skill.NewTarget(skill.TargetSelf, 0, 0),
					2: skill.NewGate(skill.GateHull, 0.4),
					3: skill.NewMend(0.15),
					4: skill.NewShield(0.15),
				}), "Lifeline", "hurt: mend under 40% hull, else shield"},
				{exHex("Coolant", ring, false, map[int]skill.Part{
					0: skill.NewPeriodic(2),
					1: skill.NewTarget(skill.TargetSelf, 0, 0),
					2: skill.NewGate(skill.GateHeat, 0.5),
					3: skill.NewPurge(20),
				}), "Coolant", "every 2s, purge 20 Heat over half"},
			}},
	}
}

// ErrNeedsTest is why an example won't load with the test switch off.
var ErrNeedsTest = errors.New("examples need the test switch on")

// LoadExample swaps in example i. The first time, the player's own build
// is kept in Mine; loading another example keeps that same build.
func (s *Sim) LoadExample(i int) error {
	ex := Examples(s.Reg)
	if i < 0 || i >= len(ex) {
		return errors.New("no such example")
	}
	if !s.TestShapes {
		return ErrNeedsTest
	}
	e := ex[i]
	backup, err := s.snapshotBuild()
	if err != nil {
		return err
	}
	if s.Mine == nil {
		s.Mine = backup
	}
	old := s.Load
	l := &s.Load
	for k, eh := range e.Hexes {
		l.Hexes[k], l.Names[k], l.Blurb[k], l.Edited[k] = eh.Hex, eh.Name, eh.Blurb, false
	}
	l.Mods, l.Spots, l.Keys = e.Mods, nil, [MaxSlots]uint8{}
	l.FixSpots()
	if err := s.rebuildAll(-1); err != nil {
		s.Load = old
		_ = s.rebuildAll(-1)
		return fmt.Errorf("%s: %w", e.Name, err)
	}
	s.Example = e.Name
	return nil
}

// RestoreBuild puts the player's own build back after an example.
func (s *Sim) RestoreBuild() error {
	if s.Mine == nil {
		return errors.New("your own build is already in")
	}
	m, err := copyBuild(s.Mine)
	if err != nil {
		return err
	}
	old := s.Load
	l := &s.Load
	l.Hexes, l.Names, l.Blurb, l.Edited = m.Hexes, m.Names, m.Blurb, m.Edited
	l.Mods, l.Spots, l.Keys = m.Mods, m.Spots, m.Keys
	l.FixSpots()
	if err := s.rebuildAll(-1); err != nil {
		s.Load = old
		_ = s.rebuildAll(-1)
		return err
	}
	s.Mine, s.Example = nil, ""
	return nil
}

func (s *Sim) snapshotBuild() (*MyBuild, error) {
	l := &s.Load
	return copyBuild(&MyBuild{Hexes: l.Hexes, Names: l.Names, Blurb: l.Blurb, Edited: l.Edited, Mods: l.Mods, Spots: l.Spots, Keys: l.Keys})
}

// copyBuild deep-copies a build through JSON, the same way a save does.
func copyBuild(b *MyBuild) (*MyBuild, error) {
	raw, err := json.Marshal(b)
	if err != nil {
		return nil, err
	}
	var out MyBuild
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	for _, h := range out.Hexes {
		if h == nil {
			return nil, errors.New("the build is missing a hex")
		}
	}
	return &out, nil
}

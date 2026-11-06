package game

import (
	"fmt"

	"hexes/loot"
	"hexes/skill"
)

// Starter kits. A fresh save deals three rolled kits and you keep one, so
// two saves open differently. A kit is the starter hex's damage type, the
// rig's spread of points, and the types of the three hexes the teaching
// ladder hands over later (and of the effect the EFFECTS lesson gives).
// Every kit keeps the ladder's shape: one cast hex on a Triangle with
// its last slot empty, a single idea at a time. The kit is rolled from
// the save's seed, so only its number is saved. Kit 0 is the old fixed
// opening (Spark, Jolt, Frost Ring, Swarm), which saves from before kits
// keep.

// KitDeal is how many kits a fresh save is dealt.
const KitDeal = 3

// kitTypes is the pool kits take their types from: the four that read
// plainly on floor 1. Drone hijack and cloak come later, from the grid.
var kitTypes = [4]string{"plasma", "cryo", "kinetic", "nanite"}

// Per type, in kitTypes order: the starter's damage, the on-hit hex's
// damage, the periodic hex's damage, and the EFFECTS lesson's damage.
var (
	kitStart  = [4]float32{24, 22, 16, 34}
	kitOnHit  = [4]float32{8, 9, 10, 14}
	kitPulse  = [4]float32{14, 14, 10, 20}
	kitLesson = [4]float32{20, 22, 14, 30}
)

var (
	kitStartNames = [4]string{"Spark", "Shard", "Slug", "Needle"}
	kitHitNames   = [4]string{"Flare", "Rime", "Jolt", "Itch"}
	kitRingNames  = [4]string{"Pyre Ring", "Frost Ring", "Shock Ring", "Rot Ring"}
	kitPulseNames = [4]string{"Heat Pulse", "Cold Snap", "Quake", "Swarm"}
	kitRingVerbs  = [4]string{"burns", "chills", "staggers", "poisons"}
)

// RigProfile is one way to spread the starting rig's points.
type RigProfile struct {
	Name, Blurb string
	Points      skill.Points
}

// RigProfiles are the spreads a kit can roll. Each spends the same 16
// points on Capacity, Recharge and Cooling, with Processing at 5.
var RigProfiles = [4]RigProfile{
	{"Balanced", "even Power and refill", DefaultPoints},
	{"Battery", "big Power, slow refill", skill.Points{Capacity: 12, Recharge: 4, Processing: 5}},
	{"Coolant", "runs cool, less Power", skill.Points{Capacity: 6, Recharge: 6, Cooling: 4, Processing: 5}},
	{"Quick", "small Power, fast refill", skill.Points{Capacity: 4, Recharge: 12, Processing: 5}},
}

// Kit is one starting kit.
type Kit struct {
	// Types are indexes into kitTypes: the starter's, then hex 2's, hex
	// 3's and hex 4's. The EFFECTS lesson gives hex 2's type.
	Types [4]uint8
	Rig   uint8
}

// legacyKit is the opening every save had before kits.
var legacyKit = Kit{Types: [4]uint8{0, 2, 1, 3}}

// DealKits is the three kits a save with this seed is dealt: three
// different starter types and three different rig spreads.
func DealKits(seed uint64) [KitDeal]Kit {
	r := loot.NewRNG(seed ^ 0x5717e4a11c)
	order := [4]uint8{0, 1, 2, 3}
	rigs := [4]uint8{0, 1, 2, 3}
	shuffle := func(a *[4]uint8) {
		for i := 3; i > 0; i-- {
			j := r.Intn(i + 1)
			a[i], a[j] = a[j], a[i]
		}
	}
	shuffle(&order)
	shuffle(&rigs)
	var out [KitDeal]Kit
	for k := range out {
		rest := [4]uint8{}
		n := 0
		for _, t := range [4]uint8{0, 1, 2, 3} {
			if t != order[k] {
				rest[n] = t
				n++
			}
		}
		for i := 2; i > 0; i-- { // shuffle the other three
			j := r.Intn(i + 1)
			rest[i], rest[j] = rest[j], rest[i]
		}
		out[k] = Kit{Types: [4]uint8{order[k], rest[0], rest[1], rest[2]}, Rig: rigs[k]}
	}
	return out
}

// curKit is the save's kit.
func (s *Sim) curKit() Kit {
	if s.Kit <= 0 || s.Kit > KitDeal {
		return legacyKit
	}
	return DealKits(s.RunSeed)[s.Kit-1]
}

// KitName is a kit's short name for its card: its starter hex.
func (k Kit) KitName() string { return kitStartNames[k.Types[0]] }

// KitLines is a kit's card text: what the starter does, then the rig.
func (k Kit) KitLines() (what, rig string) {
	return kitTypes[k.Types[0]] + " bolt at the nearest foe", RigProfiles[k.Rig].Name + " rig: " + RigProfiles[k.Rig].Blurb
}

// StartType is the kit's starter damage type name.
func (k Kit) StartType() string { return kitTypes[k.Types[0]] }

// LessonType is the type of the EFFECTS lesson's part.
func (k Kit) LessonType() string { return kitTypes[k.Types[1]] }

// ladderHex is hex i as the ladder hands it over, by the save's kit.
func (s *Sim) ladderHex(i int) (h *skill.Hex, name, blurb string) {
	return kitHex(s.Reg, s.curKit(), i)
}

func kitHex(reg *skill.Registry, k Kit, i int) (h *skill.Hex, name, blurb string) {
	tri, ring := skill.Shape{Kind: skill.ShapeTriangle}, skill.Shape{Kind: skill.ShapeRing}
	t := k.Types[min(i, 3)]
	ty := reg.MustID(kitTypes[t])
	switch i {
	case 0:
		name = kitStartNames[t]
		return starterHex(name, tri, skill.NewTrigger(skill.OnCast), skill.NewDamage(ty, kitStart[t])), name, kitTypes[t] + " at the nearest foe"
	case 1:
		name = kitHitNames[t]
		return starterHex(name, tri, skill.NewTrigger(skill.OnHit), skill.NewDamage(ty, kitOnHit[t])), name, "every hit, a " + kitTypes[t] + " jolt"
	case 2:
		name = kitRingNames[t]
		return starterHex(name, ring, skill.NewTrigger(skill.OnKill), skill.NewTarget(skill.TargetAdjacent, 2, 0),
			skill.NewStatus(ty, 2.5)), name, "a kill " + kitRingVerbs[t] + " foes around it"
	default:
		name = kitPulseNames[t]
		return starterHex(name, ring, skill.NewPeriodic(4), skill.NewTarget(skill.TargetAdjacent, 2, 0),
			skill.NewDamage(ty, kitPulse[t])), name, "every 4 s, " + kitTypes[t] + " around you"
	}
}

// lessonEffect is the damage effect the EFFECTS lesson hands over.
func (s *Sim) lessonEffect() skill.Part {
	t := s.curKit().Types[1]
	return skill.NewDamage(s.Reg.MustID(kitTypes[t]), kitLesson[t])
}

// LessonCard is lesson idx's card text, worded for the save's kit.
func (s *Sim) LessonCard(idx int) string {
	if idx < 0 || idx >= len(Ladder) {
		return ""
	}
	l := Ladder[idx]
	switch l.Name {
	case "CASTING":
		if s.Kit > 0 {
			return fmt.Sprintf("Tap 1 to cast %s. Every cast costs Power and adds Heat; VENT dumps the Heat but locks you a moment.", s.Load.Names[0])
		}
	case "EFFECTS":
		return fmt.Sprintf("Effects are what a hex does. Swap this %s into your hex at the purple workbench.", s.curKit().LessonType())
	}
	return l.Card
}

// CanPickKit reports whether this save can still choose its kit: a fresh
// one, on floor 1, before a kill, a lesson or an edit.
func (s *Sim) CanPickKit() bool {
	return s.Kit == 0 && s.Depth == 1 && s.Best <= 1 && s.Kills == 0 && s.Taught <= 1 &&
		!s.Load.Edited[0] && s.Mine == nil && !s.veteran
}

// OfferKits opens the kit pick on a fresh save: the game waits on it like
// a floor-end draft. It does nothing on a save that's past choosing.
func (s *Sim) OfferKits() bool {
	if !s.CanPickKit() || s.State != Playing {
		return false
	}
	s.Draft = Draft{On: true, Kits: true, Depth: s.Depth, N: KitDeal}
	for k := range KitDeal {
		s.Draft.Picks[k] = DraftPick{Kind: DraftKit, Kit: k}
	}
	s.State, s.StateT = Drafting, 0
	return true
}

// pickKit applies dealt kit k: the rig's points, the starter hex, and the
// ladder's later hexes from then on. The floor reloads so the new rig is
// equipped; nothing has happened on it yet.
func (s *Sim) pickKit(k int) {
	s.Kit = k + 1
	kt := s.curKit()
	s.Points = RigProfiles[kt.Rig].Points
	s.Load.Hexes[0], s.Load.Names[0], s.Load.Blurb[0] = s.ladderHex(0)
	s.Load.Edited[0] = false
	s.Load.FixKeys()
	s.Draft = Draft{}
	s.load(s.Depth, 1)
	s.Card, s.CardT = 0, CardShow
}

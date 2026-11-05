package game

import (
	"hexes/loot"
	"hexes/skill"
)

// The teaching ladder. A fresh save starts with one small hex and earns
// the rest one idea per floor, so the game never shows a thing it hasn't
// taught. Each step is a glowing pickup that waits near where you arrive
// (the first one by the stairs workbench) from its depth on, every visit,
// until you walk over it. Taking it shows a one-line card, hands over
// what it teaches, and unlocks that idea for good: loot starts rolling
// it, and the editor and HUD start showing it. A death that drops you
// back floors never takes an unlock back. The test switch unlocks
// everything at once.

// Unlock is one idea the ladder teaches, one bit each.
type Unlock uint32

const (
	// UBench: the workbench editor, and the bench where you arrive.
	UBench Unlock = 1 << iota
	// UTargets: target parts.
	UTargets
	// UTriggers: trigger parts, a second hex, cast buttons past 1.
	UTriggers
	// UMults: multiplier parts.
	UMults
	// USustain: Mend, Shield and Purge.
	USustain
	// UFrames: slot frames (drops and the FRAMES tab).
	UFrames
	// UShapes: shapes (drops and the SHAPE button), hex 3.
	UShapes
	// UGates: gate parts.
	UGates
	// UCharges: charge parts, spend and empty gates.
	UCharges
	// UCalls: Call parts, hex 4.
	UCalls
	// UBoard: the docking board and its modules.
	UBoard
	// UQuirks: quirks, exotic parts, channel and watch triggers, logic
	// tiles and the condition builder.
	UQuirks

	// UAll is every idea.
	UAll Unlock = 1<<iota - 1
)

// Lesson is one rung of the ladder.
type Lesson struct {
	Depth int    // the first depth its pickup waits on
	Name  string // what the pickup and card are titled
	Card  string // one line, written as what it does for you
	Gives Unlock
	Hexes int // hexes you have once it's taught
	// Trial puts a pack of foes together just past where you arrive, so
	// the new idea gets used at once.
	Trial bool
}

// Ladder is the order ideas are taught in. Lesson 0 is taught when a
// save starts; its card shows then.
var Ladder = [...]Lesson{
	{Depth: 1, Name: "CASTING", Hexes: 1,
		Card: "Tap 1 to cast. Every cast costs Power and adds Heat; VENT dumps the Heat but locks you a moment."},
	{Depth: 1, Name: "EFFECTS", Gives: UBench, Hexes: 1,
		Card: "Effects are what a hex does. Swap this cryo into your hex at the purple workbench."},
	{Depth: 2, Name: "TARGETS", Gives: UTargets, Hexes: 1, Trial: true,
		Card: "Targets pick who your next slot hits. This one hits every foe within 2 of you."},
	{Depth: 3, Name: "TRIGGERS", Gives: UTriggers, Hexes: 2, Trial: true,
		Card: "Hex 2 fires by itself whenever you land a hit. Only cast hexes need a button."},
	{Depth: 4, Name: "MULTIPLIERS", Gives: UMults, Hexes: 2, Trial: true,
		Card: "A multiplier boosts the slot before it. Your hexes grew to six slots to fit it."},
	{Depth: 5, Name: "SUSTAIN", Gives: USustain, Hexes: 2,
		Card: "Mend patches your hull every time its hex fires. Stairs only give back 30%, so build for it."},
	{Depth: 6, Name: "FRAMES", Gives: UFrames, Hexes: 2,
		Card: "Frames bend the slot they sit on. A Doubler fires its effect twice: fit it from HEX, FRAMES."},
	{Depth: 7, Name: "SHAPES", Gives: UShapes, Hexes: 3,
		Card: "A shape is how a hex flows. A Fork runs two paths off one trigger. Hex 3 is online too."},
	{Depth: 9, Name: "GATES", Gives: UGates, Hexes: 3, Trial: true,
		Card: "A gate splits a hex: foes under 40% hull go down YES, the rest down NO."},
	{Depth: 11, Name: "CHARGES", Gives: UCharges, Hexes: 3, Trial: true,
		Card: "A charge counts every firing, and the spend gate pays off on every 4th."},
	{Depth: 13, Name: "CALLS", Gives: UCalls | UBoard, Hexes: 4, Trial: true,
		Card: "A Call runs another hex from inside this one. Hex 4 is online, and HEX, BOARD docks hexes."},
	{Depth: 15, Name: "EVERYTHING", Gives: UQuirks, Hexes: 4,
		Card: "Everything drops now: quirks, exotic parts and logic tiles. Tap a gate twice to build its test."},
}

// Teaching tuning.
const (
	CardShow  = 7 // seconds a lesson card stays up
	TrialPack = 3 // foes gathered into a lesson's trial pack
)

// LessonDrop is the ladder's next pickup lying on the floor.
type LessonDrop struct {
	On   bool
	At   skill.Coord
	Idx  int
	Age  float32
	Full bool // walked over it with no room for what it gives
}

// Unlocks is every idea taught so far (everything with the test switch).
func (s *Sim) Unlocks() Unlock {
	if s.TestShapes {
		return UAll
	}
	return ladderUnlocks(s.taught())
}

// ladderUnlocks is what the first taught rungs of the ladder give.
func ladderUnlocks(taught int) Unlock {
	var u Unlock
	for _, l := range Ladder[:min(len(Ladder), max(1, taught))] {
		u |= l.Gives
	}
	return u
}

// Has reports whether every idea in u is taught.
func (s *Sim) Has(u Unlock) bool { return s.Unlocks()&u == u }

// HexesOpen is how many hexes the player has (all four with the test
// switch). The rest are offline: inert, hidden and never fired.
func (s *Sim) HexesOpen() int {
	if s.TestShapes {
		return MaxSlots
	}
	return Ladder[s.taught()-1].Hexes
}

// HexOpen reports whether equipped hex i is online. Modules count as
// open once the board is.
func (s *Sim) HexOpen(i int) bool {
	if i >= MaxSlots {
		return s.Has(UBoard)
	}
	return i < s.HexesOpen()
}

func (s *Sim) taught() int { return min(len(Ladder), max(1, s.Taught)) }

// Learned reports whether the whole ladder is done.
func (s *Sim) Learned() bool { return s.taught() == len(Ladder) }

// NextLesson is the next rung, or -1 when the ladder is done.
func (s *Sim) NextLesson() int {
	if s.Learned() {
		return -1
	}
	return s.taught()
}

// lootAllow is what drops may roll: only what has been taught.
func (s *Sim) lootAllow() loot.Allow {
	u := s.Unlocks()
	if u == UAll {
		return loot.Allow{}
	}
	a := loot.Allow{Limit: true, Kinds: loot.KindBit(skill.KindEffect)}
	add := func(need Unlock, k skill.PartKind) {
		if u&need != 0 {
			a.Kinds |= loot.KindBit(k)
		}
	}
	add(UTargets, skill.KindTarget)
	add(UTriggers, skill.KindTrigger)
	add(UMults, skill.KindMult)
	add(UGates, skill.KindGate)
	add(UCharges, skill.KindCharge)
	add(UCalls, skill.KindCall)
	a.Sustain = u&USustain != 0
	a.Spend = u&UCharges != 0
	a.Quirks = u&UQuirks != 0
	return a
}

// Starter hexes: the one a save starts with, the ones the ladder hands
// over, and the offline placeholder that holds a hex's place until then.
func starterHex(name string, sh skill.Shape, parts ...skill.Part) *skill.Hex {
	h := &skill.Hex{Name: name, Shape: sh, Sockets: ShapeSockets(sh)}
	for i := range h.Linked {
		h.Linked[i] = true
	}
	order, n := h.Flow(sh.Entry())
	for j, p := range parts {
		if j < n {
			h.Slots[order[j]] = skill.Slot{Kind: skill.SlotPart, Part: p}
		}
	}
	return h
}

// OfflineName is what a hex the ladder hasn't handed over is called.
const OfflineName = "Offline"

func offlineHex() *skill.Hex {
	return starterHex(OfflineName, skill.Shape{Kind: skill.ShapeTriangle}, skill.NewTrigger(skill.OnHit))
}

// ladderHex is hex i as the ladder hands it over.
func ladderHex(reg *skill.Registry, i int) (h *skill.Hex, name, blurb string) {
	ty := reg.MustID
	tri, ring := skill.Shape{Kind: skill.ShapeTriangle}, skill.Shape{Kind: skill.ShapeRing}
	switch i {
	case 0:
		return starterHex("Spark", tri, skill.NewTrigger(skill.OnCast), skill.NewDamage(ty("plasma"), 24)), "Spark", "plasma at the nearest foe"
	case 1:
		return starterHex("Jolt", tri, skill.NewTrigger(skill.OnHit), skill.NewDamage(ty("kinetic"), 10)), "Jolt", "every hit, a kinetic jolt"
	case 2:
		return starterHex("Frost Ring", ring, skill.NewTrigger(skill.OnKill), skill.NewTarget(skill.TargetAdjacent, 2, 0),
			skill.NewStatus(ty("cryo"), 2.5)), "Frost Ring", "a kill chills foes around it"
	default:
		return starterHex("Swarm", ring, skill.NewPeriodic(4), skill.NewTarget(skill.TargetAdjacent, 2, 0),
			skill.NewDamage(ty("nanite"), 20)), "Swarm", "every 4 s, toxin around you"
	}
}

// LadderLoadout is a fresh save's build: Spark on button 1, the other
// three hexes offline.
func LadderLoadout(reg *skill.Registry, level, maxDepth int) Loadout {
	var l Loadout
	l.Hexes[0], l.Names[0], l.Blurb[0] = ladderHex(reg, 0)
	for i := 1; i < MaxSlots; i++ {
		l.Hexes[i], l.Names[i], l.Blurb[i] = offlineHex(), OfflineName, "not online yet"
	}
	l.Undocked = true
	l.FixSpots()
	if err := l.Rebuild(reg, level, maxDepth, -1); err != nil {
		panic(err)
	}
	return l
}

// lessonParts is what each rung puts in the pack: up to two parts, in a
// fixed array so standing on a pickup with a full pack doesn't allocate.
func (s *Sim) lessonParts(idx int) (out [2]skill.Part, n int) {
	ty := s.Reg.MustID
	add := func(p skill.Part) { out[n] = p; n++ }
	switch Ladder[idx].Name {
	case "EFFECTS":
		add(skill.NewDamage(ty("cryo"), 22))
	case "TARGETS":
		add(skill.NewTarget(skill.TargetAdjacent, 2, 0))
	case "MULTIPLIERS":
		add(skill.NewMult(skill.VerbDamage, 1.5))
	case "SUSTAIN":
		add(FirstSustain())
	case "GATES":
		add(skill.NewGate(skill.GateHull, 0.4))
	case "CHARGES":
		add(skill.NewCharge(0, 1))
		add(skill.NewSpendGate(0, 4))
	case "CALLS":
		add(skill.NewCall(1))
	case "EVERYTHING":
		p := skill.NewDamage(ty("plasma"), 18).WithRarity(skill.Rare)
		p.Quirk = skill.QuirkRelay
		add(p)
	}
	return out, n
}

// placeLesson lays the ladder's next pickup on a floor that has reached
// its depth: beside the stairs workbench for the first workbench lesson,
// beside where you arrive for the rest. A lesson with a trial pulls the
// nearest foes into one pack.
func (s *Sim) placeLesson() {
	s.Stash.Lesson = LessonDrop{}
	idx := s.NextLesson()
	if idx < 0 || s.Depth < Ladder[idx].Depth {
		return
	}
	from := s.Floor.Start
	if Ladder[idx].Gives&UBench != 0 {
		from = s.Benches[1]
	}
	at, ok := skill.Coord{}, false
	for k := range skill.HexDirs {
		n := from.Add(skill.HexDirs[(k+4)%len(skill.HexDirs)])
		if s.Floor.At(n).Walkable() && n != s.Floor.Exit && n != s.Floor.Start && !s.takenBySpecial(n) {
			at, ok = n, true
			break
		}
	}
	if !ok {
		at = from
	}
	s.Stash.Lesson = LessonDrop{On: true, At: at, Idx: idx}
	if Ladder[idx].Trial {
		s.gatherTrial()
	}
}

// gatherTrial moves the foes nearest the start (on foot) into one pack
// around the nearest of them, so a lesson's new idea meets a crowd.
func (s *Sim) gatherTrial() {
	var near [TrialPack]skill.EntityID
	n := 0
	for k := 0; k < TrialPack; k++ {
		best, bd := skill.NoEntity, int32(1<<30)
		for i := range s.Actors {
			a := &s.Actors[i]
			id := skill.EntityID(i)
			if id == s.Player || !a.U.Alive || a.U.Team == s.P().U.Team {
				continue
			}
			taken := false
			for _, t := range near[:n] {
				taken = taken || t == id
			}
			d := s.dist[s.Floor.Index(a.At)]
			if taken || d <= 0 || d >= bd {
				continue
			}
			best, bd = id, d
		}
		if best == skill.NoEntity {
			break
		}
		near[n] = best
		n++
	}
	if n < 2 {
		return
	}
	hub := s.Actors[near[0]].At
	for _, id := range near[1:n] {
		for _, d := range skill.HexDirs {
			c := hub.Add(d)
			if !s.Floor.At(c).Walkable() || s.occupied(c) || s.dist[s.Floor.Index(c)] <= 0 || c == s.Floor.Exit {
				continue
			}
			a := &s.Actors[id]
			s.occ[s.Floor.Index(a.At)] = skill.NoEntity
			a.At, a.From = c, c
			s.occ[s.Floor.Index(c)] = id
			break
		}
	}
}

// pickupLesson takes the lesson pickup under the player: what it gives
// goes in the pack (safe from death, like the starter Call), the rack or
// the shapes, the hexes it brings come online, and its card shows. With
// no room for its parts it stays where it is.
func (s *Sim) pickupLesson(at skill.Coord, dt float32) {
	d := &s.Stash.Lesson
	if !d.On {
		return
	}
	d.Age += dt
	if d.At != at {
		d.Full = false
		return
	}
	parts, np := s.lessonParts(d.Idx)
	inv := &s.Stash.Inv
	needFrame := Ladder[d.Idx].Gives&UFrames != 0
	if loot.InventorySize-inv.Count() < np || (needFrame && len(s.Frames) >= MaxFrames) {
		if !d.Full {
			d.Full = true
			s.addFX(FX{Kind: FXFull, At: at, Life: 1.2})
		}
		return
	}
	idx := d.Idx
	*d = LessonDrop{}
	s.teach(idx, parts[:np])
	s.addFX(FX{Kind: FXLesson, At: at, Slot: idx, Life: 2.2})
}

// teach hands over lesson idx: parts into the pack, then what else it
// brings.
func (s *Sim) teach(idx int, parts []skill.Part) {
	inv := &s.Stash.Inv
	for _, p := range parts {
		if i := inv.Add(p); i >= 0 {
			s.Stash.Fresh[i] = false
		}
	}
	s.Taught = idx + 1
	s.Card, s.CardT = idx, CardShow
	switch Ladder[idx].Name {
	case "MULTIPLIERS":
		// The starter hexes grow from Triangles to Rings: room for the
		// multiplier without giving anything up.
		for i := 0; i < s.HexesOpen(); i++ {
			if h := s.Load.Hexes[i]; h.Shape.Kind == skill.ShapeTriangle && h.Shape.ID == 0 {
				_ = s.SetShape(i, skill.Shape{Kind: skill.ShapeRing})
			}
		}
	case "SUSTAIN":
		s.SustainGiven = true
	case "FRAMES":
		r := loot.NewRNG(s.RunSeed ^ 0xf1a3e)
		f := skill.NewFrame(skill.BendDoubler)
		f.ID = r.Next()&^testFrameBit | 1
		s.Frames = append(s.Frames, f)
		s.FrameGiven, s.LastFrame = true, f
	case "SHAPES":
		r := loot.NewRNG(s.RunSeed ^ 0xf1257)
		sh := skill.Shape{Kind: skill.ShapeFork, ID: r.Next()&^testShapeBit | 1}
		if len(s.Shapes) < MaxShapes {
			s.Shapes = append(s.Shapes, sh)
		}
		s.ShapeGiven, s.LastShape = true, sh
	}
	for i := 1; i < Ladder[idx].Hexes; i++ {
		s.bringOnline(i)
	}
	_ = s.rebuildAll(-1)
	s.Load.FixKeys()
	s.Dirty = true
}

// bringOnline swaps the offline placeholder in hex i for the ladder's
// hex, in the player's own build if an example is loaded. A hex the
// player already changed (under the test switch) is left alone.
func (s *Sim) bringOnline(i int) {
	hexes, names, blurbs, edited := &s.Load.Hexes, &s.Load.Names, &s.Load.Blurb, &s.Load.Edited
	if s.Mine != nil {
		hexes, names, blurbs, edited = &s.Mine.Hexes, &s.Mine.Names, &s.Mine.Blurb, &s.Mine.Edited
	}
	if hexes[i] == nil || hexes[i].Name != OfflineName || edited[i] {
		return
	}
	hexes[i], names[i], blurbs[i] = ladderHex(s.Reg, i)
}

// PlainHelp is one plain sentence on what a part does, for the editor's
// ? button: no numbers, no rules, just the idea.
func PlainHelp(p skill.Part) string {
	switch p.Kind {
	case skill.KindTrigger:
		switch p.Trigger {
		case skill.OnCast:
			return "A cast trigger fires the hex when you press its button."
		case skill.OnHit:
			return "An on-hit trigger fires the hex by itself every time you land a hit."
		case skill.OnKill:
			return "An on-kill trigger fires the hex by itself every time you finish a foe."
		case skill.OnHurt:
			return "An on-hurt trigger fires the hex by itself every time something hits you."
		case skill.Periodic:
			return "A periodic trigger fires the hex by itself every few seconds."
		case skill.Channel:
			return "A channel trigger keeps firing the hex for as long as you hold its button."
		case skill.Watch:
			return "A watch trigger fires the hex by itself the moment its test comes out true."
		case skill.FromStealth:
			return "This trigger fires the hex when you cast while hidden."
		case skill.FromBehind:
			return "This trigger fires the hex when you cast at a foe's back."
		}
		return "A trigger decides when the hex fires."
	case skill.KindTarget:
		switch p.Target {
		case skill.TargetSelf:
			return "This target points the slots after it at you."
		case skill.TargetNearest:
			return "This target points the slots after it at the closest foes."
		case skill.TargetCone:
			return "This target points the slots after it at every foe in a wedge in front of you."
		case skill.TargetAdjacent:
			return "This target points the slots after it at every foe close around the last one hit."
		case skill.TargetChain:
			return "This target jumps from foe to foe, hitting each one on the way."
		}
		return "A target picks who the slots after it hit."
	case skill.KindEffect:
		switch p.Effect {
		case skill.EffectDamage:
			return "This effect hurts whatever the hex is pointed at."
		case skill.EffectPush:
			return "This effect shoves whatever the hex is pointed at."
		case skill.EffectStatus:
			return "This effect leaves a lasting condition on whatever the hex is pointed at."
		case skill.EffectMend:
			return "Mend patches your hull every time the hex fires."
		case skill.EffectShield:
			return "Shield puts a soak on you that takes hits before your hull does."
		case skill.EffectPurge:
			return "Purge burns off the bad conditions stuck on you."
		}
		return "An effect is what the hex actually does."
	case skill.KindMult:
		return "A multiplier makes the slot right before it stronger, and costs a little more to fire."
	case skill.KindGate:
		switch p.Gate {
		case skill.GateSpend:
			return "A spend gate pays out its charge: when the count is high enough the hex goes YES and the count drops."
		case skill.GateEmpty:
			return "This gate goes YES only when its charge count is at zero."
		case skill.GateCond:
			return "A condition gate sends foes down YES or NO by a test you build yourself."
		}
		return "A gate splits the hex in two: what passes its test goes down YES, the rest down NO."
	case skill.KindCharge:
		return "A charge keeps a count that stays between firings, so the hex can remember."
	case skill.KindCall:
		if p.RunsNow() {
			return "This call runs a docked hex right here, as if its parts were in this slot."
		}
		return "A call runs another of your hexes once this one finishes."
	}
	return "An empty slot: put a part here."
}

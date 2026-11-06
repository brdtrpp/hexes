package game

import (
	"errors"
	"fmt"

	"hexes/loot"
	"hexes/skill"
)

// The floor-end draft, from the design doc "Hexes: control without a
// plan". Stepping onto the stairs of a floor you've never gone down from
// stops the game on three offers, and you keep one with a tap: one that
// leans into what your hexes already do, one off to the side, one wild.
// Every 5th depth, where the pack locks, adds a fourth, bigger one.
// Taking none pays a little Data instead. Offers only hold what the
// teaching ladder has unlocked, and grid offers are cells next to what
// you own, so the draft is a free step across the grid as often as not.
// A floor's draft rolls from the save's seed and the depth, so a reload
// shows the same three; floors walked again after a death don't draft.

// Draft tuning. Placeholder balance, all constants.
const (
	MaxDraft       = 4  // offers on a big draft
	DraftDataBase  = 12 // a Data offer: this...
	DraftDataStep  = 4  // ...plus this per depth
	DraftSkipShare = 3  // taking none pays a Data offer divided by this
	DraftSignals   = 2  // signal fragments in a signal bundle
)

// DraftKind is what an offer gives.
type DraftKind uint8

const (
	DraftNone   DraftKind = iota
	DraftNode             // a grid cell next to yours, free
	DraftData             // banked Data
	DraftPart             // a part for the locker
	DraftFrame            // a frame for the rack
	DraftShape            // a shape
	DraftSignal           // signal fragments and some Data
	DraftKit              // a starter kit (a fresh save's first pick)
)

// DraftLean is where an offer sits against your build.
type DraftLean uint8

const (
	LeanIn   DraftLean = iota // more of what you run
	LeanSide                  // something you don't run
	LeanWild                  // anything
	LeanBig                   // the checkpoint's extra
)

var leanNames = [...]string{"LEANS IN", "OFF TO THE SIDE", "WILD", "CHECKPOINT"}

func (l DraftLean) String() string { return leanNames[l] }

// DraftPick is one offer.
type DraftPick struct {
	Kind    DraftKind
	Lean    DraftLean
	Node    int
	Part    skill.Part
	Frame   skill.Frame
	Shape   skill.Shape
	Data    int
	Signals int
	Kit     int
}

// Draft is the offer on screen, if any.
type Draft struct {
	On    bool
	Kits  bool // the fresh-save kit pick rather than a floor's end
	Depth int
	N     int
	Picks [MaxDraft]DraftPick
	Skip  int // Data for taking none
}

// DraftDataAt is a Data offer at depth.
func DraftDataAt(depth int) int { return DraftDataBase + DraftDataStep*depth }

// openDraft rolls the draft for the floor you're leaving, the first time
// you go down from it. It reports whether there's one to show.
func (s *Sim) openDraft() bool {
	if s.Depth <= s.DraftedTo {
		return false
	}
	s.Draft = s.rollDraft(s.Depth)
	return s.Draft.On
}

// rollDraft builds the offers for depth.
func (s *Sim) rollDraft(depth int) Draft {
	r := loot.NewRNG(s.RunSeed ^ hash(uint64(depth)^0xd4af7))
	d := Draft{On: true, Depth: depth, N: 3, Skip: max(4, DraftDataAt(depth)/DraftSkipShare)}
	main, has := s.mainType()
	d.Picks[0] = s.offerLean(&r, depth, main, has)
	d.Picks[1] = s.offerSide(&r, depth, main, has, d.Picks[0])
	d.Picks[2] = s.offerWild(&r, depth, d.Picks[:2])
	if IsCheckpoint(depth) {
		d.N = 4
		d.Picks[3] = s.offerBig(&r, depth)
	}
	return d
}

// mainType is the damage type your online hexes use most.
func (s *Sim) mainType() (skill.TypeID, bool) {
	var n [skill.MaxTypes]int
	best, bn := skill.TypeID(0), 0
	for i := 0; i < MaxSlots; i++ {
		if !s.HexOpen(i) || s.Load.Hexes[i] == nil {
			continue
		}
		for _, sl := range s.Load.Hexes[i].Slots {
			p := sl.Part
			if sl.Kind != skill.SlotPart || p.Kind != skill.KindEffect || (p.Effect != skill.EffectDamage && p.Effect != skill.EffectStatus) {
				continue
			}
			if int(p.Type) < len(n) {
				n[p.Type]++
				if n[p.Type] > bn {
					best, bn = p.Type, n[p.Type]
				}
			}
		}
	}
	return best, bn > 0
}

// draftNodes is every grid cell a draft may give: next to what you own,
// on an open ring, not a void or a reclaim, with room for what it holds.
func (s *Sim) draftNodes(out []int) []int {
	if s.Grid == nil || !s.Has(UBench) {
		return out
	}
	for i := range s.Grid.Nodes {
		n := &s.Grid.Nodes[i]
		if s.Own[i] || s.Reclaim[i] || n.Kind == NodeVoid || n.Kind == NodeStart || s.GridDist(i) != 1 || !s.RingOpen(n.Ring) {
			continue
		}
		if (n.Kind == NodeFrame && len(s.Frames) >= MaxFrames) || (n.Kind == NodeShape && len(s.Shapes) >= MaxShapes) {
			continue
		}
		out = append(out, i)
	}
	return out
}

func typed(p skill.Part) bool {
	return p.Kind == skill.KindEffect && (p.Effect == skill.EffectDamage || p.Effect == skill.EffectStatus)
}

// offerLean leans into your build: a free grid cell of your main type if
// one sits next to you, else any free cell, else a part of your type.
func (s *Sim) offerLean(r *loot.RNG, depth int, main skill.TypeID, has bool) DraftPick {
	nodes := s.draftNodes(nil)
	var mine []int
	for _, i := range nodes {
		if p := s.Grid.Nodes[i].Part; has && s.Grid.Nodes[i].Kind == NodePart && typed(p) && p.Type == main {
			mine = append(mine, i)
		}
	}
	switch {
	case len(mine) > 0:
		return DraftPick{Kind: DraftNode, Lean: LeanIn, Node: mine[r.Intn(len(mine))]}
	case len(nodes) > 0:
		return DraftPick{Kind: DraftNode, Lean: LeanIn, Node: nodes[r.Intn(len(nodes))]}
	case s.Has(UBench):
		p := s.draftPart(r, s.bestRarity(), func(p skill.Part) bool { return !has || (typed(p) && p.Type == main) })
		return DraftPick{Kind: DraftPart, Lean: LeanIn, Part: p}
	}
	return DraftPick{Kind: DraftData, Lean: LeanIn, Data: DraftDataAt(depth)}
}

// offerSide is off to the side: a part a step above your best rarity
// that isn't your main type, or isn't an effect at all.
func (s *Sim) offerSide(r *loot.RNG, depth int, main skill.TypeID, has bool, lean DraftPick) DraftPick {
	if !s.Has(UBench) {
		return DraftPick{Kind: DraftSignal, Lean: LeanSide, Signals: DraftSignals, Data: DraftDataAt(depth) / 2}
	}
	rar := min(skill.Rare, s.bestRarity()+1)
	p := s.draftPart(r, rar, func(p skill.Part) bool {
		return !(has && typed(p) && p.Type == main) && !(lean.Kind == DraftPart && p == lean.Part)
	})
	return DraftPick{Kind: DraftPart, Lean: LeanSide, Part: p}
}

// offerWild is anything the ladder has opened: a frame, a shape, an
// exotic part, signal fragments or a lump of Data.
func (s *Sim) offerWild(r *loot.RNG, depth int, taken []DraftPick) DraftPick {
	var opts [5]DraftKind
	n := 0
	add := func(k DraftKind, ok bool) {
		if ok {
			opts[n] = k
			n++
		}
	}
	add(DraftFrame, s.Has(UFrames) && len(s.Frames) < MaxFrames)
	add(DraftShape, s.Has(UShapes) && len(s.Shapes) < MaxShapes)
	add(DraftPart, s.Has(UQuirks))
	add(DraftSignal, true)
	add(DraftData, true)
	k := opts[r.Intn(n)]
	for _, t := range taken {
		if t.Kind == k && n > 1 {
			k = opts[r.Intn(n)] // one more go at something not on offer yet
			break
		}
	}
	{
		switch k {
		case DraftFrame:
			return DraftPick{Kind: DraftFrame, Lean: LeanWild, Frame: s.rollFrame(r, depth+5)}
		case DraftShape:
			return DraftPick{Kind: DraftShape, Lean: LeanWild, Shape: skill.Shape{Kind: s.rollShapeKind(r), ID: r.Next()&^testShapeBit | 1}}
		case DraftPart:
			return DraftPick{Kind: DraftPart, Lean: LeanWild, Part: s.draftPart(r, skill.Exotic, nil)}
		case DraftSignal:
			return DraftPick{Kind: DraftSignal, Lean: LeanWild, Signals: DraftSignals, Data: DraftDataAt(depth) / 2}
		}
		return DraftPick{Kind: DraftData, Lean: LeanWild, Data: DraftDataAt(depth) * 3 / 2}
	}
}

// offerBig is the checkpoint's fourth offer: a rare frame once frames are
// taught, else a rare part, else a big lump of Data.
func (s *Sim) offerBig(r *loot.RNG, depth int) DraftPick {
	switch {
	case s.Has(UFrames) && len(s.Frames) < MaxFrames:
		f := skill.RollFrame(r.Next, skill.Rare, s.Reg.Len(), r.Next()&^testFrameBit)
		return DraftPick{Kind: DraftFrame, Lean: LeanBig, Frame: f}
	case s.Has(UBench):
		return DraftPick{Kind: DraftPart, Lean: LeanBig, Part: s.draftPart(r, skill.Rare, nil)}
	}
	return DraftPick{Kind: DraftData, Lean: LeanBig, Data: DraftDataAt(depth) * 2}
}

// bestRarity is the best rarity of a part you own on the grid.
func (s *Sim) bestRarity() skill.Rarity {
	_, _, best := s.gridKinds()
	return best
}

// draftPart rolls a part of what the ladder has taught at rarity rar
// (exotic only once quirks are), up to a few times until ok likes it.
func (s *Sim) draftPart(r *loot.RNG, rar skill.Rarity, ok func(skill.Part) bool) skill.Part {
	allow := s.lootAllow()
	if rar == skill.Exotic && allow.Limit && !allow.Quirks {
		rar = skill.Rare
	}
	t := s.Stash.table
	old := t.Allow
	t.Allow = allow
	defer func() { t.Allow = old }()
	p := t.RollAt(r, rar)
	for i := 0; i < 6 && ok != nil && !ok(p); i++ {
		p = t.RollAt(r, rar)
	}
	return p
}

// rollShapeKind picks a found shape's kind for a draft.
func (s *Sim) rollShapeKind(r *loot.RNG) skill.ShapeKind {
	kinds := [...]skill.ShapeKind{skill.ShapeHub, skill.ShapeFork, skill.ShapeLoop}
	return kinds[r.Intn(len(kinds))]
}

// ErrNoDraft is returned for a pick with no draft on screen.
var ErrNoDraft = errors.New("no draft to pick from")

// TakeDraft keeps offer i (or takes none, for i < 0) and goes on down the
// stairs. The kit pick takes a kit and goes on playing.
func (s *Sim) TakeDraft(i int) error {
	d := &s.Draft
	if !d.On || s.State != Drafting {
		return ErrNoDraft
	}
	if d.Kits {
		if i < 0 || i >= d.N {
			i = 0 // the kit pick has no skip: the first kit it is
		}
		s.pickKit(d.Picks[i].Kit)
		s.State, s.StateT = Playing, 0
		s.Dirty = true
		return nil
	}
	if i >= d.N {
		return fmt.Errorf("no offer %d", i+1)
	}
	if i < 0 {
		s.Data += d.Skip
	} else if err := s.grantPick(d.Picks[i]); err != nil {
		return err
	}
	s.DraftedTo = max(s.DraftedTo, d.Depth)
	*d = Draft{}
	s.State, s.StateT = Descending, 0
	s.Dirty = true
	return nil
}

// grantPick hands over what an offer gives. Data from a draft is banked:
// a death can't take it back.
func (s *Sim) grantPick(p DraftPick) error {
	switch p.Kind {
	case DraftNode:
		if p.Node <= 0 || p.Node >= len(s.Grid.Nodes) || s.Own[p.Node] {
			return errors.New("that cell is already yours")
		}
		s.Own[p.Node] = true
		s.GridNewest = p.Node
		s.grant(&s.Grid.Nodes[p.Node])
		s.refreshGrid()
	case DraftData:
		s.Data += p.Data
	case DraftPart:
		s.Locker = append(s.Locker, p.Part)
		if IsSustain(p.Part) {
			s.SustainGiven = true
		}
	case DraftFrame:
		if len(s.Frames) >= MaxFrames {
			return errors.New("your frame rack is full")
		}
		s.Frames = append(s.Frames, p.Frame)
		s.FrameGiven, s.LastFrame = true, p.Frame
	case DraftShape:
		if len(s.Shapes) >= MaxShapes {
			return errors.New("you can't hold more shapes")
		}
		s.Shapes = append(s.Shapes, p.Shape)
		s.ShapeGiven, s.LastShape = true, p.Shape
	case DraftSignal:
		s.Signals += p.Signals
		s.Data += p.Data
	}
	return nil
}

// Describe is an offer in words for its card: a title, one line on what
// it is, and a rarity for its color.
func (s *Sim) Describe(p DraftPick) (title, line string, rar skill.Rarity) {
	switch p.Kind {
	case DraftKit:
		k := DealKits(s.RunSeed)[p.Kit]
		what, rig := k.KitLines()
		return k.KitName(), what + ". " + rig, skill.Common
	case DraftNode:
		n := &s.Grid.Nodes[p.Node]
		switch n.Kind {
		case NodePart:
			t, d := loot.Label(n.Part, s.Reg)
			return "GRID: " + t, d + ". A free cell next to yours.", n.Rarity
		case NodeFrame:
			return "GRID: " + n.Frame.Name() + " frame", n.Frame.What() + ". A free cell.", n.Rarity
		case NodeShape:
			return "GRID: " + n.Shape.Name() + " shape", "A free cell next to yours.", n.Rarity
		case NodeTile:
			return "GRID: logic tile", n.Tile.String() + ". A free cell.", n.Rarity
		case NodeBoard:
			return "GRID: board cell", "One more board cell. A free cell.", n.Rarity
		}
	case DraftData:
		return fmt.Sprintf("%d DATA", p.Data), "Banked: a death can't take it.", skill.Common
	case DraftPart:
		t, d := loot.Label(p.Part, s.Reg)
		return t, d + ". Goes to your locker.", p.Part.Rarity
	case DraftFrame:
		return p.Frame.Name() + " frame", p.Frame.What() + ".", p.Frame.Grade
	case DraftShape:
		return p.Shape.Name() + " shape", "A new way for a hex to flow.", skill.Uncommon
	case DraftSignal:
		return fmt.Sprintf("%d SIGNALS", p.Signals), fmt.Sprintf("Light up grid cells through the fog, plus %d Data.", p.Data), skill.Uncommon
	}
	return "", "", skill.Common
}

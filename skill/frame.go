package skill

import (
	"fmt"
	"strings"
)

// Frames. Every slot on a hex holds a frame. A frame says what the slot
// takes and bends what the part in it does. Hexes start with plain frames
// (take anything, bend nothing); the rest drop as loot, so the rules your
// best hex runs on are ones you found, not ones you picked.
//
// A frame has three rolled things:
//   - What it takes: anything, or only effects, damage effects, targets,
//     multipliers or a trigger, sometimes narrowed further to one damage
//     type or one rarity.
//   - A bend: one rule changing what the part in it does (Bend).
//   - A cost: the bend's own Heat, Power or drawback. The frame's grade
//     (its rarity) cuts the Heat it adds, the way rarer gates run leaner.
//
// A shape's twin and surge quirks are frames too: they come with the
// shape on that slot and can't be swapped out (Hex.Frame).

// Bend is the rule a frame bends.
type Bend uint8

const (
	BendNone Bend = iota
	// BendTwin: the part fires twice and costs twice. A multiplier
	// applies twice; a target just picks once.
	BendTwin
	// BendSurge: every number of the part is SurgeScale times bigger.
	BendSurge
	// BendDoubler: an effect fires twice, paying its Heat twice and its
	// Power once.
	BendDoubler
	// BendKillLock: the part fires a second time, but only on a firing
	// that has killed something by the time it gets there.
	BendKillLock
	// BendPrism: a damage effect takes the damage type of the latest
	// effect before it in the flow.
	BendPrism
	// BendLeech: a damage effect deals LeechCut less, and LeechFrac of
	// the damage it deals comes back as Power.
	BendLeech
	// BendSiphon: the part costs no Power, and adds SiphonHeat Heat.
	BendSiphon
	// BendHair: a trigger fires HairHaste times faster (a periodic's
	// period, an on-hit or on-hurt cooldown); the whole hex runs
	// HairHeat hotter.
	BendHair
	// BendOverflow: a multiplier grows with Heat above OverflowAt of
	// max; an overheat this hex causes locks OverflowLock longer.
	BendOverflow
	// BendCrowd: a nearest or chain target picks one more for every foe
	// within CrowdRadius of where it picks from, and never picks an
	// elite.
	BendCrowd
	// BendLonely: an effect is LonelyHit times stronger when it lands on
	// exactly one foe, LonelyMiss when it lands on more.
	BendLonely
	// BendEcho: the slot holds no part of its own. It runs a copy of the
	// part in the slot that feeds it, at full price.
	BendEcho
	NumBends
)

var bendNames = [...]string{"Plain", "Twin", "Surge", "Doubler", "Kill-lock", "Prism", "Leech", "Siphon",
	"Hair trigger", "Overflow", "Crowd", "Lonely", "Echo"}

func (b Bend) String() string {
	if b >= NumBends {
		return fmt.Sprintf("bend%d", b)
	}
	return bendNames[b]
}

// Frame tuning. Placeholder balance, all constants.
const (
	PrismHeat    = 1    // a Prism adds this much Heat
	LeechFrac    = 0.10 // share of damage dealt that comes back as Power
	LeechCut     = 0.20 // a Leech part deals this much less
	SiphonHeat   = 3    // what a Siphon adds instead of the Power it saves
	SurgeHeat    = 1    // a loot Surge adds this; a shape's own surge doesn't
	HairHaste    = 1.4  // a Hair trigger fires this many times as often
	HairHeat     = 1.3  // and the whole hex runs this much hotter
	OverflowAt   = 0.8  // share of max Heat where Overflow starts paying
	OverflowPer  = 0.02 // each point of Heat past it adds this to the factor
	OverflowLock = 1    // seconds an Overflow hex adds to an overheat lock
	CrowdRadius  = 3    // a Crowd target counts foes this close
	LonelyHit    = 2.5
	LonelyMiss   = 0.5
)

// Takes is the kind of part a frame takes.
type Takes uint8

const (
	TakeAny     Takes = iota // any part but a trigger
	TakeEffect               // effects
	TakeDamage               // damage effects
	TakeTarget               // targets
	TakeCount                // nearest or chain targets: the ones with a count
	TakeMult                 // multipliers
	TakeTrigger              // triggers
	TakeActs                 // effects, charges, calls: the parts that do something when run
	TakeNone                 // nothing: the slot runs a copy (Echo)
	NumTakes
)

var takesNames = [...]string{"anything", "effects", "damage effects", "targets", "nearest or chain targets",
	"multipliers", "triggers", "effects, charges or calls", "nothing"}

func (t Takes) String() string {
	if t >= NumTakes {
		return fmt.Sprintf("takes%d", t)
	}
	return takesNames[t]
}

// bendTakes is what each bend takes before any narrowing.
var bendTakes = [NumBends]Takes{
	BendNone:     TakeAny,
	BendTwin:     TakeAny,
	BendSurge:    TakeAny,
	BendDoubler:  TakeEffect,
	BendKillLock: TakeActs,
	BendPrism:    TakeDamage,
	BendLeech:    TakeDamage,
	BendSiphon:   TakeAny,
	BendHair:     TakeTrigger,
	BendOverflow: TakeMult,
	BendCrowd:    TakeCount,
	BendLonely:   TakeEffect,
	BendEcho:     TakeNone,
}

// Frame is one slot's frame. The zero Frame is plain: it takes anything
// and bends nothing, which is what every slot of a hex saved before
// frames has.
type Frame struct {
	Bend  Bend  `json:",omitempty"`
	Takes Takes `json:",omitempty"`
	// Typed narrows it to parts of damage type Type, Ranked to parts of
	// rarity Rank. Either one means effects only when Takes is anything.
	Typed  bool   `json:",omitempty"`
	Type   TypeID `json:",omitempty"`
	Ranked bool   `json:",omitempty"`
	Rank   Rarity `json:",omitempty"`
	// Grade is the frame's own rarity: it cuts the Heat its bend adds.
	Grade Rarity `json:",omitempty"`
	// ID tells two copies apart; nonzero for any frame that dropped.
	ID uint64 `json:",omitempty"`
	// Shaped marks a frame that comes with its hex's shape (a twin or
	// surge quirk). It's never saved: Hex.Frame makes it.
	Shaped bool `json:"-"`
}

// NewFrame is a frame with a bend and what that bend takes.
func NewFrame(b Bend) Frame { return Frame{Bend: b, Takes: bendTakes[b]} }

// Plain reports whether the frame takes anything and bends nothing.
func (f Frame) Plain() bool { return f.Bend == BendNone && !f.Typed && !f.Ranked }

// Valid reports whether the frame's fields make sense together.
func (f Frame) Valid() bool {
	return f.Bend < NumBends && f.Takes < NumTakes && f.Grade < NumRarities && f.Rank < NumRarities
}

// Accepts reports whether the frame takes part p, and if not, why.
func (f Frame) Accepts(p Part, reg *Registry) (bool, string) {
	ok := true
	switch f.Takes {
	case TakeAny:
		ok = p.Kind != KindTrigger
	case TakeEffect:
		ok = p.Kind == KindEffect
	case TakeDamage:
		ok = p.Kind == KindEffect && p.Effect == EffectDamage
	case TakeTarget:
		ok = p.Kind == KindTarget
	case TakeCount:
		ok = p.Kind == KindTarget && (p.Target == TargetNearest || p.Target == TargetChain)
	case TakeMult:
		ok = p.Kind == KindMult
	case TakeTrigger:
		ok = p.Kind == KindTrigger
	case TakeActs:
		ok = p.Kind == KindEffect || p.Kind == KindCharge || p.Kind == KindCall
	case TakeNone:
		ok = false
	}
	if ok && f.Typed {
		ok = p.Kind == KindEffect && p.Effect.Typed() && p.Type == f.Type
	}
	if ok && f.Ranked {
		ok = p.Rarity == f.Rank
	}
	if ok {
		return true, ""
	}
	return false, "its " + f.Name() + " frame takes " + f.TakesText(reg)
}

// AcceptsTrigger reports whether the frame can sit under a trigger: a
// plain frame or a Hair trigger. A trigger slot takes nothing else.
func (f Frame) AcceptsTrigger() bool {
	return f.Plain() || (f.Takes == TakeTrigger && !f.Typed && !f.Ranked)
}

// Name is what the frame is called: its bend, or "Plain".
func (f Frame) Name() string { return f.Bend.String() }

// TakesText says what the frame takes in a few words: "anything",
// "cryo effects", "rare damage effects".
func (f Frame) TakesText(reg *Registry) string {
	t := f.Takes.String()
	if f.Typed {
		name := "typed"
		if reg != nil && int(f.Type) < reg.Len() {
			name = reg.Get(f.Type).Name
		}
		switch f.Takes {
		case TakeAny, TakeActs:
			t = name + " effects"
		default:
			t = name + " " + t
		}
	}
	if f.Ranked {
		if !f.Typed && (f.Takes == TakeAny) {
			t = "parts"
		}
		t = strings.ToLower(f.Rank.String()) + " " + t
	}
	return t
}

// Cut is what the frame's grade does to the Heat its bend adds.
func (f Frame) Cut() float32 { return 1 / f.Grade.Scale() }

// What says what the bend does, in one plain sentence fragment.
func (f Frame) What() string {
	switch f.Bend {
	case BendTwin:
		return "the part fires twice and costs twice"
	case BendSurge:
		if f.Shaped {
			return fmt.Sprintf("every number of the part is x%.1f", SurgeScale)
		}
		return fmt.Sprintf("every number of the part is x%.1f, +%.1f Heat", SurgeScale, SurgeHeat*f.Cut())
	case BendDoubler:
		return "the effect fires twice, paying its Heat twice and its Power once"
	case BendKillLock:
		return "the part fires again, but only on firings that have killed by then"
	case BendPrism:
		return fmt.Sprintf("the damage takes the type of the effect before it, +%.1f Heat", PrismHeat*f.Cut())
	case BendLeech:
		return fmt.Sprintf("%.0f%% of the damage dealt comes back as Power; it deals %.0f%% less", LeechFrac*100, LeechCut*100)
	case BendSiphon:
		return fmt.Sprintf("the part costs no Power, +%.1f Heat instead", SiphonHeat*f.Cut())
	case BendHair:
		return fmt.Sprintf("the trigger fires %.0f%% faster (periodic, on-hit, on-hurt); the hex runs x%.1f Heat", (HairHaste-1)*100, HairHeat)
	case BendOverflow:
		return fmt.Sprintf("Heat over %.0f%% adds to the multiplier (+%.2f a point); an overheat locks %ds longer", OverflowAt*100, OverflowPer, OverflowLock)
	case BendCrowd:
		return fmt.Sprintf("+1 target for each foe within %d; never picks an elite", CrowdRadius)
	case BendLonely:
		return fmt.Sprintf("x%.1f on exactly one foe, x%.1f on more", LonelyHit, LonelyMiss)
	case BendEcho:
		return "holds no part: runs a copy of the part that feeds it, at full price"
	}
	return "takes the part as it is"
}

// Text is the frame in one line for the editor: "Doubler (cryo effects):
// the effect fires twice ...".
func (f Frame) Text(reg *Registry) string {
	if f.Plain() {
		return "Plain frame: takes anything, bends nothing."
	}
	s := fmt.Sprintf("%s frame, takes %s: %s.", f.Name(), f.TakesText(reg), f.What())
	if f.Shaped {
		s += " It comes with the shape."
	}
	return s
}

// Tag is the frame's short label for a slot tile: "x2", "KILL", "PRISM".
func (f Frame) Tag() string {
	switch f.Bend {
	case BendTwin:
		return "TWIN x2"
	case BendSurge:
		return "SURGE"
	case BendDoubler:
		return "DOUBLER"
	case BendKillLock:
		return "KILL-LOCK"
	case BendPrism:
		return "PRISM"
	case BendLeech:
		return "LEECH"
	case BendSiphon:
		return "SIPHON"
	case BendHair:
		return "HAIR"
	case BendOverflow:
		return "OVERFLOW"
	case BendCrowd:
		return "CROWD"
	case BendLonely:
		return "LONELY"
	case BendEcho:
		return "ECHO"
	}
	if f.Typed || f.Ranked {
		return "ONLY"
	}
	return ""
}

// Frame loot tuning. Placeholder balance.
var bendWeights = [NumBends]float32{
	BendTwin:     4,
	BendSurge:    6,
	BendDoubler:  10,
	BendKillLock: 9,
	BendPrism:    8,
	BendLeech:    9,
	BendSiphon:   9,
	BendHair:     7,
	BendOverflow: 6,
	BendCrowd:    7,
	BendLonely:   8,
	BendEcho:     6,
}

// NarrowChance is the chance a frame of each grade is narrowed to one
// damage type or one rarity: commons often are, exotics seldom.
var NarrowChance = [NumRarities]float32{0.5, 0.35, 0.2, 0.1}

// RollFrame rolls a frame of grade g: a bend by weight, what it takes,
// and maybe a narrowing to one of types damage types (or to one rarity).
// next is any uniform uint64 source; id should be unique per copy.
func RollFrame(next func() uint64, g Rarity, types int, id uint64) Frame {
	var total float32
	for _, w := range bendWeights {
		total += w
	}
	x := float32(next()>>40) / (1 << 24) * total
	b := Bend(1)
	for i, w := range bendWeights {
		if w == 0 {
			continue
		}
		b = Bend(i)
		if x < w {
			break
		}
		x -= w
	}
	f := NewFrame(b)
	f.Grade, f.ID = g, id|1
	if float32(next()>>40)/(1<<24) >= NarrowChance[g] {
		return f
	}
	switch f.Takes {
	case TakeTrigger, TakeNone:
		return f // a trigger or an echo has nothing to narrow
	case TakeTarget, TakeCount, TakeMult:
		f.Ranked, f.Rank = true, Rarity(next()%uint64(NumRarities))
		return f
	}
	if types > 0 && next()%3 != 0 {
		f.Typed, f.Type = true, TypeID(next()%uint64(types))
	} else {
		f.Ranked, f.Rank = true, Rarity(next()%uint64(NumRarities))
	}
	return f
}

// Frame is the frame on slot s: the one fitted there, or the one the
// hex's shape comes with (a twin or surge quirk), which can't be swapped.
func (h *Hex) Frame(s int) Frame {
	if s < 0 || s >= Slots {
		return Frame{}
	}
	if s == h.Shape.Twin() {
		f := NewFrame(BendTwin)
		f.Shaped = true
		return f
	}
	if s == h.Shape.Surge() {
		f := NewFrame(BendSurge)
		f.Shaped = true
		return f
	}
	return h.Frames[s]
}

// Fits reports whether part p can go in slot s: the socket takes its
// kind and the frame takes the part. why says what refused it.
func (h *Hex) Fits(s int, p Part, reg *Registry) (ok bool, why string) {
	if s < 0 || s >= Slots {
		return false, "no such slot"
	}
	if !h.Sockets[s].Accepts(p.Kind) {
		return false, fmt.Sprintf("slot %s takes no %s", h.SlotName(s), p.Kind)
	}
	f := h.Frame(s)
	if p.Kind == KindTrigger {
		if f.AcceptsTrigger() {
			return true, ""
		}
		return false, "its " + f.Name() + " frame takes " + f.TakesText(reg)
	}
	if f.Plain() {
		return true, ""
	}
	return f.Accepts(p, reg)
}

// echoes fills every empty Echo-framed slot of a copy of h with the slot
// that feeds it (the latest slot before it in flow with a link in, or
// failing that the one just before it in flow), in flow order so an
// echo can copy an echo. It returns h itself when there's nothing to
// fill, so hexes without echoes compile exactly as before.
func echoes(h *Hex, start int) *Hex {
	order, n := h.Flow(start)
	var cp *Hex
	for k := 0; k < n; k++ {
		s := order[k]
		if h.Frame(s).Bend != BendEcho || h.Slots[s].Kind != SlotEmpty {
			continue
		}
		if cp == nil {
			c := *h
			cp = &c
		}
		src := cp.inFeed(s, order[:n])
		if src < 0 && k > 0 {
			src = order[k-1]
		}
		if src < 0 {
			continue
		}
		sl := cp.Slots[src]
		if sl.Kind == SlotPart && sl.Part.Kind == KindTrigger {
			continue
		}
		cp.Slots[s] = sl
	}
	if cp == nil {
		return h
	}
	return cp
}

// EchoOf is the slot an Echo frame on slot s copies, or -1.
func (h *Hex) EchoOf(s int) int {
	if h.Frame(s).Bend != BendEcho {
		return -1
	}
	order, n := h.Flow(h.TriggerSlot())
	for k := 0; k < n; k++ {
		if order[k] != s {
			continue
		}
		src := h.inFeed(s, order[:n])
		if src < 0 && k > 0 {
			src = order[k-1]
		}
		return src
	}
	return -1
}

// prism works out, in flow order, the damage type each Prism-framed
// damage effect takes: the type of the latest effect before it in the
// flow (itself after its own prism), if any.
func prism(h *Hex, order []int) (types [Slots]TypeID, set [Slots]bool) {
	last, have := TypeID(0), false
	for _, s := range order {
		sl := h.Slots[s]
		if sl.Kind != SlotPart || sl.Part.Kind != KindEffect || !sl.Part.Effect.Typed() {
			continue
		}
		t := sl.Part.Type
		if h.Frame(s).Bend == BendPrism && sl.Part.Effect == EffectDamage && have {
			t = last
			types[s], set[s] = t, true
		}
		last, have = t, true
	}
	return types, set
}

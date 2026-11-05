package game

import (
	"errors"
	"fmt"

	"hexes/loot"
	"hexes/skill"
)

// Frames in the sim (see skill.Frame). Every slot holds a frame: plain
// until you fit one you found. Frames drop as loot, leaning toward elites,
// and go on the frame rack the moment you walk over them. They're kept
// for good like shapes; a death never takes one. Fitting a frame from the
// rack into a slot is free, but whatever frame was there is scrapped, so
// every swap gives something up. A shape's twin or surge slot comes with
// its frame and can't take another.

// Frame drop tuning. Placeholder balance, all constants.
const (
	FrameDropBase   = 0.015  // per kill at depth 1
	FrameDropStep   = 0.0015 // more per depth below it
	FrameDropMax    = 0.06
	FrameDropElite  = 4  // times more likely from a foe above the floor's level
	FrameFirstDepth = 2  // from here the first frame waits by the stairs until found
	MaxFrames       = 12 // the rack
	MaxFrameDrops   = 4  // frames that can lie on one floor
)

// testFrameBit marks the test switch's frames, which nobody owns.
const testFrameBit = 1 << 63

// FrameDrop is a frame lying on the floor.
type FrameDrop struct {
	At    skill.Coord
	Frame skill.Frame
	Age   float32
}

// FrameDropChance is the chance a kill at depth drops a frame.
func FrameDropChance(depth int, elite bool) float32 {
	c := min(FrameDropMax, FrameDropBase+FrameDropStep*float32(max(0, depth-1)))
	if elite {
		c *= FrameDropElite
	}
	return c
}

// rollFrame rolls a frame for depth from r.
func (s *Sim) rollFrame(r *loot.RNG, depth int) skill.Frame {
	g := loot.RollRarity(r, depth)
	return skill.RollFrame(r.Next, g, s.Reg.Len(), r.Next()&^testFrameBit)
}

// rollFrameDrop maybe drops a frame from a dying foe. It uses its own
// random stream so it never changes which parts drop.
func (s *Sim) rollFrameDrop(id skill.EntityID, at skill.Coord, elite bool) {
	if len(s.Stash.Frames) >= MaxFrameDrops {
		return
	}
	r := loot.NewRNG(loot.Seed(s.Floor.Seed^hash(uint64(s.Visits))^0xf4a3e, int(id)))
	if r.Float() >= FrameDropChance(s.Depth, elite) {
		return
	}
	s.Stash.Frames = append(s.Stash.Frames, FrameDrop{At: at, Frame: s.rollFrame(&r, s.Depth)})
}

// placeFirstFrame lays a frame next to the stairs down the first time a
// save reaches FrameFirstDepth without having found one, so everyone gets
// to try frames early. It comes back every visit until it's picked up.
// It's always a bend that works on almost any hex.
func (s *Sim) placeFirstFrame() {
	if s.FrameGiven || s.Depth < FrameFirstDepth {
		return
	}
	at := s.Floor.Start
	for i := len(skill.HexDirs) - 1; i >= 0; i-- {
		if n := at.Add(skill.HexDirs[i]); s.Floor.At(n).Walkable() {
			at = n
			break
		}
	}
	r := loot.NewRNG(s.RunSeed ^ 0xf1a3e)
	f := skill.NewFrame(skill.BendDoubler)
	if r.Next()%2 == 1 {
		f = skill.NewFrame(skill.BendKillLock)
	}
	f.ID = r.Next()&^testFrameBit | 1
	s.Stash.Frames = append(s.Stash.Frames, FrameDrop{At: at, Frame: f})
}

// pickupFrames racks any frame under the player. A full rack leaves it
// lying there.
func (s *Sim) pickupFrames(at skill.Coord, dt float32) {
	d := s.Stash.Frames
	for i := 0; i < len(d); {
		d[i].Age += dt
		if d[i].At != at || len(s.Frames) >= MaxFrames {
			if d[i].At == at && !s.Stash.fullShow {
				s.Stash.fullShow, s.Stash.fullAt = true, at
				s.addFX(FX{Kind: FXFull, At: at, Life: 1.2})
			}
			i++
			continue
		}
		s.Frames = append(s.Frames, d[i].Frame)
		s.FrameGiven = true
		s.LastFrame = d[i].Frame
		s.Dirty = true
		s.addFX(FX{Kind: FXFrame, At: at, Slot: int(d[i].Frame.Bend), Life: 2.2})
		d[i] = d[len(d)-1]
		d = d[:len(d)-1]
	}
	s.Stash.Frames = d
}

// IsTestFrame reports whether f is one of the test switch's frames.
func IsTestFrame(f skill.Frame) bool { return f.ID&testFrameBit != 0 }

// TestFrameSet is the frames the test switch unlocks: one of every bend,
// then a cryo-only Doubler and a rare-only Twin to show narrowed frames.
var TestFrameSet = testFrames()

func testFrames() []skill.Frame {
	var out []skill.Frame
	for b := skill.Bend(1); b < skill.NumBends; b++ {
		f := skill.NewFrame(b)
		f.ID = testFrameBit | uint64(b)
		out = append(out, f)
	}
	reg := skill.DefaultRegistry()
	d := skill.NewFrame(skill.BendDoubler)
	d.Typed, d.Type, d.Grade, d.ID = true, reg.MustID("cryo"), skill.Rare, testFrameBit|64
	t := skill.NewFrame(skill.BendTwin)
	t.Ranked, t.Rank, t.Grade, t.ID = true, skill.Rare, skill.Exotic, testFrameBit|65
	return append(out, d, t)
}

// FrameChoices is every frame you can fit now: the rack, then with the
// test switch on, the test set.
func (s *Sim) FrameChoices() []skill.Frame {
	if !s.TestShapes {
		return s.Frames
	}
	return append(append([]skill.Frame(nil), s.Frames...), TestFrameSet...)
}

// frameFits says whether frame f could sit on slot of h as the slot is
// now, and why not.
func frameFits(h *skill.Hex, slot int, f skill.Frame, reg *skill.Registry) error {
	if !h.Shape.Has(slot) {
		return errors.New("no slot there")
	}
	if cur := h.Frame(slot); cur.Shaped {
		return fmt.Errorf("that slot's %s comes with the shape", cur.Name())
	}
	sl := h.Slots[slot]
	if sl.Kind == skill.SlotNested {
		return errors.New("that slot holds a nested hex")
	}
	trig := h.Sockets[slot] == skill.SocketTrigger
	switch {
	case trig && !f.AcceptsTrigger():
		return errors.New("the trigger slot takes only a plain or Hair trigger frame")
	case !trig && f.Takes == skill.TakeTrigger:
		return errors.New("a Hair trigger frame goes on the trigger slot")
	}
	if sl.Kind == skill.SlotPart && !trig {
		if ok, _ := f.Accepts(sl.Part, reg); !ok {
			return fmt.Errorf("a %s frame takes %s: take the part out first", f.Name(), f.TakesText(reg))
		}
	}
	return nil
}

// PreviewFrame says whether frame f could go on a hex slot, and if not,
// why. The editor lights the slots it could.
func (s *Sim) PreviewFrame(hex, slot int, f skill.Frame) error {
	if !s.Load.Has(hex) || slot < 0 || slot >= skill.Slots {
		return errors.New("no such slot")
	}
	return frameFits(s.Load.Hex(hex), slot, f, s.Reg)
}

// FitFrame puts rack frame f on a hex slot. The frame that was there is
// scrapped, unless it was plain or a test frame. It returns the scrapped
// frame (zero if none). On error nothing changes.
func (s *Sim) FitFrame(hex, slot int, f skill.Frame) (scrapped skill.Frame, err error) {
	if err := s.PreviewFrame(hex, slot, f); err != nil {
		return skill.Frame{}, err
	}
	at := -1
	for i, r := range s.Frames {
		if r.ID == f.ID && r == f {
			at = i
		}
	}
	test := IsTestFrame(f) && s.TestShapes && hasFrame(TestFrameSet, f)
	if at < 0 && !test {
		return skill.Frame{}, errors.New("that frame isn't on your rack")
	}
	h := s.Load.Hex(hex)
	old := h.Frames[slot]
	h.Frames[slot] = f
	if err := s.recompile(hex); err != nil {
		h.Frames[slot] = old
		return skill.Frame{}, err
	}
	if at >= 0 {
		s.Frames = append(s.Frames[:at], s.Frames[at+1:]...)
	}
	s.Dirty = true
	if old.Plain() || IsTestFrame(old) {
		return skill.Frame{}, nil
	}
	return old, nil
}

// ScrapSlotFrame scraps the frame on a hex slot, leaving it plain.
func (s *Sim) ScrapSlotFrame(hex, slot int) (skill.Frame, error) {
	if !s.Load.Has(hex) || slot < 0 || slot >= skill.Slots {
		return skill.Frame{}, errors.New("no such slot")
	}
	h := s.Load.Hex(hex)
	if cur := h.Frame(slot); cur.Shaped {
		return skill.Frame{}, fmt.Errorf("that slot's %s comes with the shape", cur.Name())
	}
	old := h.Frames[slot]
	if old.Plain() {
		return skill.Frame{}, errors.New("that slot's frame is plain")
	}
	h.Frames[slot] = skill.Frame{}
	if err := s.recompile(hex); err != nil {
		h.Frames[slot] = old
		return skill.Frame{}, err
	}
	s.Dirty = true
	return old, nil
}

// ScrapFrame throws away rack frame i.
func (s *Sim) ScrapFrame(i int) bool {
	if i < 0 || i >= len(s.Frames) {
		return false
	}
	s.Frames = append(s.Frames[:i], s.Frames[i+1:]...)
	s.Dirty = true
	return true
}

func hasFrame(set []skill.Frame, f skill.Frame) bool {
	for _, x := range set {
		if x == f {
			return true
		}
	}
	return false
}

// settleFrames works out which of old's frames nh (old reshaped) keeps:
// a frame stays on its slot if the new shape has that slot, doesn't bring
// its own frame there, and the slot still is (or still isn't) the
// trigger's. The rest come back: owned ones to the rack, test ones to
// nowhere. It sets nh.Frames and returns the frames that come back.
func settleFrames(old, nh *skill.Hex) (back []skill.Frame) {
	nh.Frames = [skill.Slots]skill.Frame{}
	trig := nh.Shape.Entry()
	for i, f := range old.Frames {
		if f.Plain() {
			continue
		}
		keep := nh.Shape.Has(i) && !nh.Frame(i).Shaped
		if keep && i == trig {
			keep = f.AcceptsTrigger()
		} else if keep {
			keep = f.Takes != skill.TakeTrigger
		}
		if keep {
			nh.Frames[i] = f
		} else if !IsTestFrame(f) {
			back = append(back, f)
		}
	}
	return back
}

// stripTestFrames takes every test frame off every hex and module (the
// test switch going off). Parts stay: a plain slot takes them all.
func (s *Sim) stripTestFrames() {
	strip := func(h *skill.Hex) {
		if h == nil {
			return
		}
		for i, f := range h.Frames {
			if IsTestFrame(f) {
				h.Frames[i] = skill.Frame{}
			}
		}
	}
	for _, h := range s.Load.Hexes {
		strip(h)
	}
	for _, m := range s.Load.Mods {
		strip(m)
	}
	if s.Mine != nil {
		for _, h := range s.Mine.Hexes {
			strip(h)
		}
		for _, m := range s.Mine.Mods {
			strip(m)
		}
	}
}

// cleanFrames drops frames a save shouldn't hold: broken ones, and test
// frames when the test switch is off.
func cleanFrames(h *skill.Hex, test bool) {
	if h == nil {
		return
	}
	for i, f := range h.Frames {
		if !f.Valid() || f.Takes != skill.NewFrame(f.Bend).Takes || (!test && IsTestFrame(f)) {
			h.Frames[i] = skill.Frame{}
		}
	}
}

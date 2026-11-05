package game

import (
	"encoding/json"
	"strings"
	"testing"

	"hexes/skill"
)

// rack puts a frame of bend b on the rack, as if found, and returns it.
func rack(s *Sim, b skill.Bend) skill.Frame {
	f := skill.NewFrame(b)
	f.ID = uint64(len(s.Frames)*2 + 201)
	s.Frames = append(s.Frames, f)
	return f
}

func TestFitFrameScrapsTheOldOne(t *testing.T) {
	s := NewVeteran(5)
	// Arc Lance: trigger, chain, damage, multiplier, empty, call.
	_, heat := s.HexPower(0)
	dbl := rack(s, skill.BendDoubler)
	leech := rack(s, skill.BendLeech)
	if old, err := s.FitFrame(0, 2, dbl); err != nil || !old.Plain() {
		t.Fatalf("fit: %v, scrapped %+v", err, old)
	}
	if s.Load.Hexes[0].Frames[2] != dbl || len(s.Frames) != 1 {
		t.Fatal("frame not fitted, or still on the rack")
	}
	if _, ht := s.HexPower(0); ht <= heat {
		t.Fatalf("doubler heat %.1f, plain %.1f", ht, heat)
	}
	if s.Disp.Loadout(s.Player).Progs[0] != s.Load.Progs[0] {
		t.Fatal("framed hex not equipped")
	}
	old, err := s.FitFrame(0, 2, leech)
	if err != nil || old != dbl {
		t.Fatalf("swap: %v, scrapped %+v", err, old)
	}
	if len(s.Frames) != 0 {
		t.Fatalf("rack %+v: the doubler should be gone, not back", s.Frames)
	}
	// A frame you don't have can't be fitted.
	if _, err := s.FitFrame(0, 4, dbl); err == nil {
		t.Fatal("fitted a scrapped frame")
	}
}

func TestFrameRefusesAPartItDoesntTake(t *testing.T) {
	s := NewVeteran(5)
	prism := rack(s, skill.BendPrism)
	// Slot 3 holds a multiplier; a Prism takes only damage.
	if _, err := s.FitFrame(0, 3, prism); err == nil || !strings.Contains(err.Error(), "take the part out") {
		t.Fatalf("prism over a multiplier: %v", err)
	}
	// The trigger slot takes only a plain or Hair trigger frame.
	if _, err := s.FitFrame(0, 0, prism); err == nil {
		t.Fatal("prism on the trigger slot")
	}
	hair := rack(s, skill.BendHair)
	if _, err := s.FitFrame(0, 4, hair); err == nil {
		t.Fatal("hair trigger off the trigger slot")
	}
	if _, err := s.FitFrame(0, 0, hair); err != nil {
		t.Fatal(err)
	}
	// An empty slot takes the prism, and then refuses a push.
	if _, err := s.FitFrame(0, 4, prism); err != nil {
		t.Fatal(err)
	}
	cell := s.Stash.Inv.Add(skill.NewPush(1))
	if err := s.SwapIn(0, 4, cell); err == nil || !strings.Contains(err.Error(), "Prism") {
		t.Fatalf("push into a prism: %v", err)
	}
	if what, ok := s.PreviewIn(0, 4, cell); ok {
		t.Fatalf("preview lit a refused slot: %s", what)
	}
	cell = s.Stash.Inv.Add(skill.NewDamage(s.Reg.MustID("cryo"), 10))
	if err := s.SwapIn(0, 4, cell); err != nil {
		t.Fatal(err)
	}
}

func TestScrapFrames(t *testing.T) {
	s := NewVeteran(5)
	f := rack(s, skill.BendSiphon)
	rack(s, skill.BendCrowd)
	if _, err := s.FitFrame(0, 2, f); err != nil {
		t.Fatal(err)
	}
	if old, err := s.ScrapSlotFrame(0, 2); err != nil || old != f || !s.Load.Hexes[0].Frames[2].Plain() {
		t.Fatalf("scrap slot frame: %v %+v", err, old)
	}
	if _, err := s.ScrapSlotFrame(0, 2); err == nil {
		t.Fatal("scrapped a plain frame")
	}
	if !s.ScrapFrame(0) || len(s.Frames) != 0 || s.ScrapFrame(0) {
		t.Fatal("scrap from the rack")
	}
}

func TestShapedSlotsKeepTheirFrame(t *testing.T) {
	s := NewVeteran(5)
	hub := found(s, skill.ShapeHub, skill.ShapeTwin, 2)
	if err := s.SetShape(0, hub); err != nil {
		t.Fatal(err)
	}
	if f := s.Load.Hexes[0].Frame(2); f.Bend != skill.BendTwin || !f.Shaped {
		t.Fatalf("twin slot frame %+v", f)
	}
	d := rack(s, skill.BendDoubler)
	if _, err := s.FitFrame(0, 2, d); err == nil || !strings.Contains(err.Error(), "comes with the shape") {
		t.Fatalf("fitted over a twin: %v", err)
	}
	if _, err := s.ScrapSlotFrame(0, 2); err == nil {
		t.Fatal("scrapped a shape's frame")
	}
}

func TestReshapeSendsFramesBackToTheRack(t *testing.T) {
	s := NewVeteran(5)
	d, k := rack(s, skill.BendDoubler), rack(s, skill.BendKillLock)
	if _, err := s.FitFrame(0, 2, d); err != nil {
		t.Fatal(err)
	}
	if _, err := s.FitFrame(0, 5, k); err != nil {
		t.Fatal(err)
	}
	// A Triangle keeps slots 0-2: the doubler stays, the kill-lock comes back.
	if err := s.SetShape(0, skill.Shape{Kind: skill.ShapeTriangle}); err != nil {
		t.Fatal(err)
	}
	h := s.Load.Hexes[0]
	if h.Frames[2] != d || !h.Frames[5].Plain() || len(s.Frames) != 1 || s.Frames[0] != k {
		t.Fatalf("frames %+v rack %+v", h.Frames, s.Frames)
	}
	// A shape whose surge lands on the doubler's slot sends it back too.
	fork := found(s, skill.ShapeFork, skill.ShapeSurge, 2)
	if err := s.SetShape(0, fork); err != nil {
		t.Fatal(err)
	}
	if !s.Load.Hexes[0].Frames[2].Plain() || len(s.Frames) != 2 {
		t.Fatalf("frames %+v rack %+v", s.Load.Hexes[0].Frames, s.Frames)
	}
	// A full rack refuses a reshape that would hand frames back.
	if _, err := s.FitFrame(0, 3, d); err != nil {
		t.Fatal(err)
	}
	for len(s.Frames) < MaxFrames {
		rack(s, skill.BendEcho)
	}
	was := *s.Load.Hexes[0]
	if err := s.SetShape(0, skill.Shape{Kind: skill.ShapeTriangle}); err == nil || !strings.Contains(err.Error(), "rack") {
		t.Fatalf("reshape with a full rack: %v", err)
	}
	if *s.Load.Hexes[0] != was {
		t.Fatal("refused reshape changed the hex")
	}
}

func TestFramesSurviveSaveAndDeath(t *testing.T) {
	s := NewVeteran(8)
	d := rack(s, skill.BendDoubler)
	l := rack(s, skill.BendLonely)
	if _, err := s.FitFrame(0, 2, d); err != nil {
		t.Fatal(err)
	}
	kill(s)
	b, err := s.Save()
	if err != nil {
		t.Fatal(err)
	}
	r := NewVeteran(1)
	if err := r.LoadSave(b); err != nil {
		t.Fatal(err)
	}
	if len(r.Frames) != 1 || r.Frames[0] != l || r.Load.Hexes[0].Frames[2] != d || !r.FrameGiven {
		t.Fatalf("rack %+v frame %+v", r.Frames, r.Load.Hexes[0].Frames[2])
	}
	if len(steps(r.Load.Progs[0], skill.OpDamage)) < 2 {
		t.Fatalf("loaded doubler didn't compile in\n%s", r.Load.Progs[0])
	}
}

func steps(p *skill.Program, op skill.Op) []skill.Step {
	var out []skill.Step
	for _, s := range p.Steps {
		if s.Op == op {
			out = append(out, s)
		}
	}
	return out
}

func TestOldSaveLoadsWithPlainFrames(t *testing.T) {
	s := NewVeteran(4)
	b, _ := s.Save()
	if strings.Contains(string(b), "Frames") {
		t.Fatal("a save with no frames wrote frames")
	}
	r := NewVeteran(1)
	if err := r.LoadSave(b); err != nil {
		t.Fatal(err)
	}
	for i, h := range r.Load.Hexes {
		for j, f := range h.Frames {
			if !f.Plain() {
				t.Fatalf("hex %d slot %d frame %+v", i, j, f)
			}
		}
	}
	// A broken frame in a save is dropped, not trusted.
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	m["Frames"] = []any{map[string]any{"Bend": 99}, map[string]any{"Bend": 3, "Takes": 1, "ID": 7}}
	bad, _ := json.Marshal(m)
	if err := r.LoadSave(bad); err != nil {
		t.Fatal(err)
	}
	if len(r.Frames) != 1 || r.Frames[0].Bend != skill.BendDoubler {
		t.Fatalf("rack from a bad save: %+v", r.Frames)
	}
}

func TestFrameDropsLeanElite(t *testing.T) {
	if FrameDropChance(1, true) != FrameDropChance(1, false)*FrameDropElite {
		t.Fatal("elites don't lean")
	}
	if FrameDropChance(500, false) != FrameDropMax {
		t.Fatal("no cap")
	}
	roll := func() []skill.Frame {
		s := NewVeteran(9)
		s.FrameGiven = true
		s.load(30, 1)
		for id := 1; id < 600 && len(s.Stash.Frames) < MaxFrameDrops; id++ {
			s.rollFrameDrop(skill.EntityID(id), s.Floor.Start, true)
		}
		var out []skill.Frame
		for _, d := range s.Stash.Frames {
			out = append(out, d.Frame)
		}
		return out
	}
	a, b := roll(), roll()
	if len(a) != MaxFrameDrops || len(a) != len(b) {
		t.Fatalf("drops %d then %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] || !a[i].Valid() || a[i].Plain() || IsTestFrame(a[i]) {
			t.Fatalf("drop %d: %+v vs %+v", i, a[i], b[i])
		}
	}
	// A full rack leaves a frame on the floor.
	s := NewVeteran(9)
	for len(s.Frames) < MaxFrames {
		rack(s, skill.BendEcho)
	}
	s.Stash.Frames = append(s.Stash.Frames, FrameDrop{At: s.P().At, Frame: a[0]})
	s.pickupFrames(s.P().At, 0)
	if len(s.Frames) != MaxFrames || len(s.Stash.Frames) != 1 {
		t.Fatal("racked past the cap")
	}
}

func TestTestSwitchUnlocksEveryFrame(t *testing.T) {
	s := NewVeteran(5)
	if len(s.FrameChoices()) != 0 {
		t.Fatal("frames before finding any")
	}
	if err := s.SetTestShapes(true); err != nil {
		t.Fatal(err)
	}
	bends := map[skill.Bend]bool{}
	for _, f := range s.FrameChoices() {
		bends[f.Bend] = true
		if !IsTestFrame(f) || !f.Valid() {
			t.Fatalf("test frame %+v", f)
		}
	}
	if len(bends) != int(skill.NumBends)-1 {
		t.Fatalf("test frames cover %d bends", len(bends))
	}
	// Every test frame goes somewhere on Arc Lance or an empty slot.
	echo := TestFrameSet[skill.BendEcho-1]
	if _, err := s.FitFrame(0, 4, echo); err != nil {
		t.Fatal(err)
	}
	if _, err := s.FitFrame(1, 4, echo); err != nil && !strings.Contains(err.Error(), "part out") {
		t.Fatalf("a test frame used up: %v", err)
	}
	if len(steps(s.Load.Progs[0], skill.OpCall)) < 1 {
		t.Fatal("echo broke the hex")
	}
	// Off: the test frames come off and nothing lands on the rack.
	if err := s.SetTestShapes(false); err != nil {
		t.Fatal(err)
	}
	if !s.Load.Hexes[0].Frames[4].Plain() || len(s.Frames) != 0 {
		t.Fatalf("frames after the switch: %+v rack %+v", s.Load.Hexes[0].Frames[4], s.Frames)
	}
	if _, err := s.FitFrame(0, 4, echo); err == nil {
		t.Fatal("test frame with the switch off")
	}
}

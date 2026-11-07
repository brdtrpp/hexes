package skill

import (
	"encoding/json"
	"math"
	"testing"
)

// A board rolls a pad in each of the six directions out from the middle,
// the same pads every time for the same hex, and they survive a save.
// The piece on a TWIN pad fires twice; a trigger on one doesn't.
func TestPadsSpreadAndWork(t *testing.T) {
	h := wild()
	h.Name = "Padded"
	set(t, h, 0, NewTrigger(OnCast))
	set(t, h, 1, NewTarget(TargetNearest, 3, 1))
	set(t, h, 2, NewDamage(id("kinetic"), 10))
	h.Lay = Layout{Room: Board, Wired: true, Touch: true}
	h.RollPads()
	pads := h.BoardPads()
	wedge := map[int]bool{}
	for _, p := range pads {
		if p.At.Ring() < 3 {
			continue // the shape's own
		}
		// Which sixth of the board it sits in, counted clockwise from
		// straight up.
		x, y := p.At.XY()
		ux, uy := CellDirs[0].XY()
		a := math.Atan2(y, x) - math.Atan2(uy, ux)
		for a < -1e-9 {
			a += 2 * math.Pi
		}
		wedge[int(a/(math.Pi/3)+1e-6)%6] = true
	}
	if len(wedge) < 6 {
		t.Errorf("pads in %d of six directions: %+v", len(wedge), pads)
	}
	again := *h
	again.RollPads()
	if again.Lay.Pads != h.Lay.Pads {
		t.Error("the same hex rolled different pads")
	}
	b, err := json.Marshal(h.Lay)
	if err != nil {
		t.Fatal(err)
	}
	var back Layout
	if err := json.Unmarshal(b, &back); err != nil || back.Pads != h.Lay.Pads {
		t.Fatalf("pads didn't survive a save: %v", err)
	}
	// Put the damage on a TWIN pad.
	var twin Cell
	ok := false
	for _, p := range pads {
		if p.Kind == PadTwin {
			twin, ok = p.At, true
			break
		}
	}
	if !ok {
		t.Fatal("no TWIN pad")
	}
	h.Lay.Pos[2] = Place{At: twin.Sub(PieceOf(h.Slots[2]).Cells[0]), On: true}
	if h.PieceAt(twin) != 2 {
		t.Fatal("damage isn't on the pad")
	}
	if f := h.Frame(2); f.Bend != BendTwin {
		t.Errorf("damage on a TWIN pad has frame %+v", f)
	}
	h.Lay.Pos[2].On = false
	h.Lay.Pos[0] = Place{At: twin.Sub(PieceOf(h.Slots[0]).Cells[0]), On: true}
	if f := h.Frame(0); f.Bend == BendTwin {
		t.Error("a trigger took a TWIN pad")
	}
}

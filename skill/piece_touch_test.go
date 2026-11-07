package skill

import (
	"encoding/json"
	"testing"
)

func TestRoomJSON(t *testing.T) {
	for _, m := range []Room{{}, RoomBits(19), RoomBits(61), Board} {
		b, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		var back Room
		if err := json.Unmarshal(b, &back); err != nil || back != m {
			t.Fatalf("%s came back %v (%v)", b, back, err)
		}
	}
	// A save from before the board grew holds a plain number.
	var old Room
	if err := json.Unmarshal([]byte("524287"), &old); err != nil || old != RoomBits(19) {
		t.Fatalf("old room %v %v", old, err)
	}
	if Board.Len() != 169 {
		t.Fatalf("board has %d tiles", Board.Len())
	}
}

// Pieces join only where they touch: laying one next to another joins
// them, moving it away lets go, and a wire can't reach across the board.
func TestPiecesJoinWhereTheyTouch(t *testing.T) {
	h := wild()
	set(t, h, 0, NewTrigger(OnCast))
	set(t, h, 1, NewTarget(TargetNearest, 3, 1))
	set(t, h, 2, NewDamage(id("kinetic"), 10))
	h.Lay = Layout{Room: Board, Wired: true, Touch: true}
	h.Lay.Pos[0] = Place{At: Cell{}, On: true}
	h.Join(0)
	c, ok := h.fitNear(1, Cell{}, bit(0))
	if !ok {
		t.Fatal("no room for the target")
	}
	h.Lay.Pos[1] = Place{At: c, On: true}
	h.Join(1)
	if h.WireArm(0, 1) == 0 {
		t.Fatal("the target touches the trigger and isn't joined to it")
	}
	// The damage far away: nothing joins, and no wire can.
	far := Cell{6, 0}
	if !h.FitsAt(2, far) {
		t.Fatal("damage doesn't fit at the edge")
	}
	h.Lay.Pos[2] = Place{At: far, On: true}
	h.Join(2)
	if h.WireArm(1, 2) != 0 || h.WireArm(2, 1) != 0 {
		t.Fatal("joined across the board")
	}
	if ok, _ := h.WireWhyTouch(1, 2); ok {
		t.Fatal("a wire could reach across the board")
	}
	// Next to the target: joined, the target sending.
	c, ok = h.fitNear(2, h.Lay.Pos[1].At, bit(1))
	if !ok || !func() bool { h.Lay.Pos[2].At = c; return h.Touch(1, 2) }() {
		t.Fatal("couldn't lay the damage beside the target")
	}
	h.Join(2)
	if h.WireArm(1, 2) == 0 {
		ok1, w1 := h.WireWhy(1, 2)
		t.Fatalf("the damage touches the target and isn't joined: 0-2 %d, 1-2 %v %s, touch01 %v at %v %v %v", h.WireArm(0, 2), ok1, w1, h.Touch(0, 2), h.Lay.Pos[0].At, h.Lay.Pos[1].At, h.Lay.Pos[2].At)
	}
	if !h.Runs(2) {
		t.Fatal("the damage doesn't run")
	}
	// And away again.
	h.Lay.Pos[2].At = far
	h.Join(2)
	if h.WireArm(1, 2) != 0 || h.Runs(2) {
		t.Fatal("moved away, the damage is still joined")
	}
}

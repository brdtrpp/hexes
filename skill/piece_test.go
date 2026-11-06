package skill

import (
	"encoding/json"
	"strings"
	"testing"
)

func wild() *Hex {
	return NewHex("laid", [RingSlots]SocketKind{T, W, W, W, W, W})
}

// laid is a hex with every cell of the biggest room and its trigger in
// the middle.
func laid(t *testing.T, parts ...Part) *Hex {
	t.Helper()
	h := wild()
	h.Lay.Room = Room(1<<19 - 1)
	for i, p := range parts {
		set(t, h, i, p)
	}
	if !h.AutoPlace(0, -1, ArmNone) {
		t.Fatal("trigger doesn't fit")
	}
	return h
}

func join(t *testing.T, h *Hex, s, from int, arm uint8) {
	t.Helper()
	if !h.AutoPlace(s, from, arm) || !h.Feeds(from, s) {
		t.Fatalf("piece %d didn't join piece %d", s, from)
	}
}

func TestTurn(t *testing.T) {
	for d := range CellDirs {
		if got := Turn(CellDirs[d], 1); got != CellDirs[(d+1)%6] {
			t.Fatalf("turning direction %d gives %v, want %v", d, got, CellDirs[(d+1)%6])
		}
		if got := Turn(CellDirs[d], 6); got != CellDirs[d] {
			t.Fatalf("a full turn moves direction %d", d)
		}
	}
}

func TestFlip(t *testing.T) {
	// Flipped, up-right and up-left trade places and up stays up.
	if Orient(CellDirs[1], 6) != CellDirs[5] || Orient(CellDirs[0], 6) != CellDirs[0] || Orient(CellDirs[2], 6) != CellDirs[4] {
		t.Fatal("flip is wrong")
	}
	for o := 0; o < Orients; o++ {
		for d := range CellDirs {
			if Orient(CellDirs[d], o) != CellDirs[OrientSide(d, o)] {
				t.Fatalf("orient %d: cell and side disagree for direction %d", o, d)
			}
		}
	}
}

func TestRoomSizes(t *testing.T) {
	for _, c := range []struct {
		k    ShapeKind
		want int
	}{{ShapeTriangle, 7}, {ShapeRing, 15}, {ShapeHub, 17}, {ShapeFork, 17}, {ShapeLoop, 15}} {
		sh := Shape{Kind: c.k}
		m := RoomFor(sh)
		if m.Len() != c.want {
			t.Errorf("%v room has %d cells, want %d", c.k, m.Len(), c.want)
		}
		g := sh.Graph()
		for i := 0; i < int(g.N); i++ {
			if !m.Has(g.At[i]) {
				t.Errorf("%v room is missing the pad for slot %d", c.k, i)
			}
		}
	}
}

func TestPiecesDiffer(t *testing.T) {
	cone := PartPiece(NewTarget(TargetCone, 3, 0))
	line := PartPiece(NewTarget(TargetLine, 3, 0))
	near := PartPiece(NewTarget(TargetNearest, 3, 1))
	dmg := PartPiece(NewDamage(id("kinetic"), 10))
	gate := PartPiece(NewGate(GateHurt, 0.5))
	if cone.Size() != 3 || line.Size() != 3 || near.Size() != 1 || dmg.Size() != 2 || gate.Size() != 3 {
		t.Fatalf("sizes: cone %d line %d nearest %d damage %d gate %d", cone.Size(), line.Size(), near.Size(), dmg.Size(), gate.Size())
	}
	// A cone's cells all touch; a line's ends don't.
	if !cone.Cells[1].Next(cone.Cells[2]) || line.Cells[0].Next(line.Cells[2]) {
		t.Fatal("cone and line have the wrong silhouettes")
	}
}

func TestLaidHexRuns(t *testing.T) {
	h := laid(t, NewTrigger(OnCast), NewTarget(TargetCone, 3, 0), NewDamage(id("kinetic"), 20))
	join(t, h, 1, 0, ArmNone)
	join(t, h, 2, 1, ArmNone)
	p := compile(t, h, 0)
	sel, dmg := steps(p, OpSelect), steps(p, OpDamage)
	if len(sel) != 1 || len(dmg) != 1 || sel[0].Target != TargetCone {
		t.Fatalf("want one cone select and one damage, got %v", p.Steps)
	}
	if dmg[0].Src != sel[0].Dst {
		t.Fatal("damage doesn't land on the cone")
	}
}

func TestUnjoinedPieceDoesNotRun(t *testing.T) {
	h := laid(t, NewTrigger(OnCast), NewTarget(TargetNearest, 3, 1), NewDamage(id("kinetic"), 20))
	join(t, h, 1, 0, ArmNone)
	// Damage in a far corner, touching nothing.
	for _, c := range h.Lay.Room.Cells() {
		if c.Ring() == 2 && h.Place(2, Place{At: c, Rot: 0}) && !h.Runs(2) {
			break
		}
	}
	if !h.Lay.Pos[2].On || h.Runs(2) {
		t.Fatal("couldn't lay the damage apart")
	}
	if len(steps(compile(t, h, 0), OpDamage)) != 0 {
		t.Fatal("a piece nothing reaches still ran")
	}
}

func TestSocketsTakeMultipliers(t *testing.T) {
	h := laid(t, NewTrigger(OnCast), NewTarget(TargetNearest, 3, 1), NewDamage(id("kinetic"), 20), NewMult(VerbDamage, 2))
	join(t, h, 1, 0, ArmNone)
	join(t, h, 2, 1, ArmNone)
	before := steps(compile(t, h, 0), OpDamage)
	join(t, h, 3, 2, ArmNone) // the nub in the damage's socket
	after := steps(compile(t, h, 0), OpDamage)
	if len(before) != 1 || len(after) != 1 || after[0].Value < 1.9*before[0].Value {
		t.Fatalf("multiplier in the socket: %v then %v", before, after)
	}
	// A multiplier can't take a spark: it only plugs into a socket.
	if PartPiece(NewMult(VerbDamage, 2)).Plugs[0].K != PlugBoost {
		t.Fatal("a multiplier's plug should be a nub")
	}
}

func TestGateArms(t *testing.T) {
	h := wild()
	for i, p := range []Part{NewTrigger(OnCast), NewTarget(TargetNearest, 3, 3), NewGate(GateHull, 0.5),
		NewDamage(id("kinetic"), 20), NewStatus(id("cryo"), 2)} {
		set(t, h, i, p)
	}
	if r := h.Routes(0); r[3] != (Route{2, BranchYes}) || r[4] != (Route{2, BranchNo}) {
		t.Fatalf("the ring itself routes %v %v", r[3], r[4])
	}
	h.EnsureLaid()
	r := h.Routes(h.TriggerSlot())
	if r[3] != (Route{2, BranchYes}) || r[4] != (Route{2, BranchNo}) {
		t.Fatalf("laid routes %v %v", r[3], r[4])
	}
	p := compile(t, h, 0)
	g, d, st := steps(p, OpGate), steps(p, OpDamage), steps(p, OpStatus)
	if len(g) != 1 || len(d) != 1 || len(st) != 1 {
		t.Fatalf("steps %v", p.Steps)
	}
	if d[0].Src != g[0].Dst || st[0].Src != g[0].Dst2 {
		t.Fatal("arms don't land on their halves")
	}
}

func TestEffectPassesWhoItHit(t *testing.T) {
	h := laid(t, NewTrigger(OnCast), NewTarget(TargetCone, 3, 0), NewDamage(id("kinetic"), 20), NewStatus(id("cryo"), 2))
	join(t, h, 1, 0, ArmNone)
	join(t, h, 2, 1, ArmNone)
	join(t, h, 3, 2, ArmNone)
	p := compile(t, h, 0)
	if st := steps(p, OpStatus); len(st) != 1 || st[0].Src != steps(p, OpSelect)[0].Dst {
		t.Fatal("a status after damage should land on who the damage hit")
	}
}

func TestLayOutKeepsAPreset(t *testing.T) {
	h := wild()
	set(t, h, 0, NewTrigger(OnCast))
	set(t, h, 1, NewTarget(TargetChain, 4, 3))
	set(t, h, 2, NewDamage(id("plasma"), 30))
	set(t, h, 3, NewMult(VerbDamage, 2))
	old := steps(compile(t, h, 0), OpDamage)
	h.EnsureLaid()
	if !h.Laid() || len(h.Loose()) != 0 {
		t.Fatalf("laid %v, loose %v", h.Laid(), h.Loose())
	}
	for s := 0; s < 4; s++ {
		if !h.Runs(s) {
			t.Fatalf("piece %d doesn't run after laying out", s)
		}
	}
	got := steps(compile(t, h, 0), OpDamage)
	if len(got) != 1 || got[0].Value != old[0].Value {
		t.Fatalf("damage %v laid, %v before", got, old)
	}
}

func TestSettleAfterSwap(t *testing.T) {
	h := laid(t, NewTrigger(OnCast), NewTarget(TargetNearest, 3, 1), NewDamage(id("kinetic"), 20))
	join(t, h, 1, 0, ArmNone)
	join(t, h, 2, 1, ArmNone)
	// A cone is bigger than the point it replaces: settling moves it
	// somewhere it fits and runs.
	h.Slots[1] = Slot{Kind: SlotPart, Part: NewTarget(TargetCone, 3, 0)}
	h.Settle()
	if !h.Runs(1) {
		t.Fatal("swapped-in cone doesn't run")
	}
	h.Clear(2)
	h.Settle()
	if h.Lay.Pos[2].On {
		t.Fatal("an empty slot kept its spot")
	}
}

func TestLaidJSON(t *testing.T) {
	h := laid(t, NewTrigger(OnCast), NewTarget(TargetNearest, 3, 1), NewDamage(id("kinetic"), 20))
	join(t, h, 1, 0, ArmNone)
	join(t, h, 2, 1, ArmNone)
	b, err := json.Marshal(h)
	if err != nil {
		t.Fatal(err)
	}
	var back Hex
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if back.Lay != h.Lay || !back.Runs(2) {
		t.Fatal("layout didn't survive a save")
	}
	// An old hex has no layout and saves none.
	b, _ = json.Marshal(wild())
	if strings.Contains(string(b), `"Lay"`) {
		t.Fatal("an unlaid hex saved a layout")
	}
}

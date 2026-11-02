package game

import (
	"encoding/json"
	"strings"
	"testing"

	"hexes/skill"
)

func TestBoardOrder(t *testing.T) {
	seen := map[skill.Cell]bool{}
	for i, c := range BoardOrder {
		if seen[c] || c.Ring() > BoardRadius {
			t.Fatalf("cell %d %v repeats or is off the board", i, c)
		}
		seen[c] = true
		if i >= 1 && i <= 6 && c != skill.CellDirs[i-1] {
			t.Fatalf("inner ring cell %d is %v", i, c)
		}
	}
	if BoardOrder[0] != (skill.Cell{}) {
		t.Fatal("board doesn't start in the middle")
	}
}

func TestBoardGrowsWithYourDeepest(t *testing.T) {
	if BoardOpen(1) != BoardBase || BoardOpen(4) != BoardBase+1 || BoardOpen(200) != BoardCells {
		t.Fatalf("open %d %d %d", BoardOpen(1), BoardOpen(4), BoardOpen(200))
	}
	s := New(5)
	if s.NextCellAt() != 4 {
		t.Fatalf("next cell at %d", s.NextCellAt())
	}
	if err := s.MoveHex(0, BoardOrder[BoardBase]); err == nil || !strings.Contains(err.Error(), "depth 4") {
		t.Fatalf("move onto a shut cell: %v", err)
	}
	s.Best = 4
	if !s.CellOpen(BoardOrder[BoardBase]) || s.NextCellAt() != 7 {
		t.Fatal("depth 4 didn't open a cell")
	}
}

// A new run, and a save from before the board, dock nothing: the hexes
// play as they always have.
func TestNewRunDocksNothing(t *testing.T) {
	s := New(5)
	if d := s.Docks(); len(d) != 0 {
		t.Fatalf("new run docks %v", d)
	}
	for i, p := range s.Load.Progs {
		if p.Docked != 0 {
			t.Fatalf("hex %d docked", i+1)
		}
	}
	sv := s.Snapshot()
	sv.Board, sv.Mods = nil, nil
	b, _ := json.Marshal(sv)
	if strings.Contains(string(b), `"Board"`) {
		t.Fatal("empty board saved")
	}
	o := New(9)
	if err := o.LoadSave(b); err != nil {
		t.Fatal(err)
	}
	if len(o.Load.Spots) != MaxSlots || len(o.Docks()) != 0 {
		t.Fatalf("old save board %v docks %v", o.Load.Spots, o.Docks())
	}
}

// dockInto turns hex a and hex b until a's chain flows into b.
func dockInto(t *testing.T, s *Sim, a, b int) Dock {
	t.Helper()
	for ra := 0; ra < 6; ra++ {
		for rb := 0; rb < 6; rb++ {
			for _, d := range s.Docks() {
				if d.From == a && d.To == b {
					return d
				}
			}
			if err := s.RotateHex(b); err != nil {
				t.Fatal(err)
			}
		}
		if err := s.RotateHex(a); err != nil {
			t.Fatal(err)
		}
	}
	t.Fatalf("hex %d never docks into %d", a, b)
	return Dock{}
}

func TestModuleDocksOntoAHex(t *testing.T) {
	s := New(5)
	m, err := s.AddModule(BoardOrder[4])
	if err != nil {
		t.Fatal(err)
	}
	if m != MaxSlots || s.Load.Name(m) != "Module A" || !s.Load.IsMod(m) {
		t.Fatalf("module %d %q", m, s.Load.Name(m))
	}
	if _, err := s.AddModule(BoardOrder[0]); err == nil {
		t.Fatal("module on top of a hex")
	}
	cell := s.Stash.Inv.Add(skill.NewTrigger(skill.OnCast))
	if err := s.SwapIn(m, 0, cell); err == nil || !strings.Contains(err.Error(), "no trigger") {
		t.Fatalf("trigger in a module: %v", err)
	}
	s.Scrap(cell)
	cell = s.Stash.Inv.Add(skill.NewDamage(s.Reg.MustID("plasma"), 25))
	if err := s.SwapIn(m, 0, cell); err != nil {
		t.Fatal(err)
	}
	if pw, ht := s.HexPower(m); pw <= 0 && ht <= 0 {
		t.Fatal("module costs nothing")
	}
	before := s.Load.Progs[0]
	d := dockInto(t, s, 0, m)
	p := s.Load.Progs[0]
	if p.Docked != 1 || len(p.Steps) <= len(before.Steps) || p.Heat <= before.Heat {
		t.Fatalf("docked arc lance: docked %d, %d steps (was %d), heat %.1f (was %.1f)",
			p.Docked, len(p.Steps), len(before.Steps), p.Heat, before.Heat)
	}
	if s.Disp.Loadout(s.Player).Progs[0] != p {
		t.Fatal("docked program not equipped")
	}
	if s.Load.Hexes[0].Docks[d.Slot] != s.Load.Hex(m) {
		t.Fatal("dock not set on the hex")
	}
	b, err := s.Save()
	if err != nil {
		t.Fatal(err)
	}
	o := New(9)
	if err := o.LoadSave(b); err != nil {
		t.Fatal(err)
	}
	if len(o.Load.Mods) != 1 || o.Load.Progs[0].Docked != 1 || len(o.Docks()) != len(s.Docks()) {
		t.Fatalf("loaded board: %d mods, docked %d", len(o.Load.Mods), o.Load.Progs[0].Docked)
	}
	if err := s.MoveHex(m, BoardOrder[3]); err != nil { // trades places with hex 4
		t.Fatal(err)
	}
	if s.Load.At(BoardOrder[4]) != 3 {
		t.Fatal("move didn't trade places")
	}
	n := s.Stash.Inv.Count()
	if err := s.RemoveModule(m); err != nil {
		t.Fatal(err)
	}
	if len(s.Load.Mods) != 0 || len(s.Load.Spots) != MaxSlots || s.Stash.Inv.Count() != n+1 {
		t.Fatal("module not removed cleanly")
	}
	if err := s.RemoveModule(0); err == nil {
		t.Fatal("removed an equipped hex")
	}
}

// Two equipped hexes can dock too: the first one's firing runs the
// second's parts, and the second still fires on its own trigger.
func TestHexesDockIntoEachOther(t *testing.T) {
	s := New(5)
	dockInto(t, s, 0, 1)
	if s.Load.Progs[0].Docked == 0 || s.Load.Progs[1].Trigger != skill.OnHit {
		t.Fatal("hex to hex dock")
	}
}

func TestTestSwitchOpensTheBoard(t *testing.T) {
	s := New(5)
	if err := s.SetTestShapes(true); err != nil {
		t.Fatal(err)
	}
	if s.BoardOpen() != BoardCells {
		t.Fatal("test switch didn't open the board")
	}
	if _, err := s.AddModule(BoardOrder[BoardCells-1]); err != nil {
		t.Fatal(err)
	}
	if err := s.SetTestShapes(false); err != nil {
		t.Fatal(err)
	}
	if at := BoardIndex(s.Load.Spots[MaxSlots].At); at >= BoardOpen(s.Best) {
		t.Fatalf("module left on shut cell %d", at)
	}
}

func BenchmarkBoardRebuild(b *testing.B) {
	s := New(5)
	s.TestShapes = true
	for i := MaxSlots; i < BoardCells; i++ {
		if _, err := s.AddModule(BoardOrder[i]); err != nil {
			b.Fatal(err)
		}
	}
	for b.Loop() {
		if err := s.RotateHex(0); err != nil {
			b.Fatal(err)
		}
	}
}

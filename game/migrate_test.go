package game

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"hexes/skill"
)

// Every file in testdata/saves is a real save from some version of the
// game. They must all keep loading, forever: that's the promise that an
// update never resets anyone. When SaveVersion goes up, add a migration
// step and freeze a save of the new version with
//
//	HEXES_FREEZE_SAVE=1 go test ./game -run TestFreezeSave
func TestFrozenSavesLoad(t *testing.T) {
	files, _ := filepath.Glob("testdata/saves/*.json")
	if len(files) == 0 {
		t.Fatal("no frozen saves")
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var want struct{ Depth, Best, Kills int }
		if err := json.Unmarshal(b, &want); err != nil {
			t.Fatal(err)
		}
		s := NewVeteran(999)
		if err := s.LoadSave(b); err != nil {
			t.Fatalf("%s no longer loads: %v", f, err)
		}
		if s.Best != want.Best || s.Kills != want.Kills || s.Depth != want.Depth {
			t.Fatalf("%s: depth %d best %d kills %d, saved %d %d %d", f, s.Depth, s.Best, s.Kills, want.Depth, want.Best, want.Kills)
		}
		if SaveBest(b) < want.Best {
			t.Fatalf("%s: SaveBest %d", f, SaveBest(b))
		}
	}
}

// TestCurrentVersionIsFrozen fails when SaveVersion was bumped without a
// frozen save of the new version, or without a step for every version
// in between.
func TestCurrentVersionIsFrozen(t *testing.T) {
	if _, err := os.Stat(frozenPath(SaveVersion)); err != nil {
		t.Fatalf("no frozen save for version %d: run HEXES_FREEZE_SAVE=1 go test ./game -run TestFreezeSave", SaveVersion)
	}
	for v := MinSaveVersion; v < SaveVersion; v++ {
		if migrations[v] == nil {
			t.Fatalf("no migration step from version %d", v)
		}
	}
}

func frozenPath(v int) string { return filepath.Join("testdata", "saves", fmt.Sprintf("v%d.json", v)) }

// TestFreezeSave writes a save of the current version with a bit of
// everything in it. It only runs when asked.
func TestFreezeSave(t *testing.T) {
	if os.Getenv("HEXES_FREEZE_SAVE") == "" {
		t.Skip("set HEXES_FREEZE_SAVE=1 to freeze a save of this version")
	}
	s := NewVeteran(4242)
	cryo := skill.NewDamage(s.Reg.MustID("cryo"), 20)
	cell := s.Stash.Inv.Add(cryo)
	if err := s.SwapIn(3, 4, cell); err != nil {
		t.Fatal(err)
	}
	s.Stash.Inv.Add(skill.NewPush(1))
	found(s, skill.ShapeHub, skill.ShapeLean, 0)
	s.load(7, 0.6)
	s.Kills, s.Data = 57, 12
	s.Load.Names[0] = "Frost Lance"
	b, err := s.Save()
	if err != nil {
		t.Fatal(err)
	}
	var pretty json.RawMessage = b
	out, _ := json.MarshalIndent(pretty, "", " ")
	if err := os.MkdirAll(filepath.Dir(frozenPath(SaveVersion)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(frozenPath(SaveVersion), append(out, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestUpgradeClimbsEachStep(t *testing.T) {
	steps := map[int]func(map[string]json.RawMessage) error{
		3: func(m map[string]json.RawMessage) error { // Kills becomes Tally
			m["Tally"] = m["Kills"]
			delete(m, "Kills")
			return nil
		},
		4: func(m map[string]json.RawMessage) error {
			m["Seen4"] = json.RawMessage("true")
			return nil
		},
	}
	out, err := upgrade([]byte(`{"Version":3,"Kills":9,"Depth":2}`), 5, steps)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Version, Tally, Depth int
		Kills                 *int
		Seen4                 bool
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if got.Version != 5 || got.Tally != 9 || got.Kills != nil || !got.Seen4 || got.Depth != 2 {
		t.Fatalf("%s", out)
	}
	// From the middle, only the later step runs.
	out, err = upgrade([]byte(`{"Version":4,"Kills":9}`), 5, steps)
	if err != nil || string(out) == "" {
		t.Fatal(err)
	}
	if err := json.Unmarshal(out, &got); err != nil || got.Version != 5 {
		t.Fatalf("%s %v", out, err)
	}
	if _, err := upgrade([]byte(`{"Version":3}`), 6, steps); err == nil {
		t.Fatal("a missing step should refuse the save, not guess")
	}
}

func TestUpgradeRefusesNewerAndAncientSaves(t *testing.T) {
	_, err := Upgrade([]byte(fmt.Sprintf(`{"Version":%d,"Depth":3}`, SaveVersion+1)))
	if !errors.Is(err, ErrNewerSave) {
		t.Fatalf("newer save: %v", err)
	}
	_, err = Upgrade([]byte(`{"Version":1,"Depth":3}`))
	if !errors.Is(err, ErrOldSave) {
		t.Fatalf("ancient save: %v", err)
	}
	if SaveBest([]byte(`{"Version":99}`)) != -1 || SaveBest([]byte("nope")) != -1 {
		t.Fatal("SaveBest of an unreadable save should be -1")
	}
}

func TestCurrentSaveIsUntouched(t *testing.T) {
	in := []byte(fmt.Sprintf(`{"Version":%d,"Odd":[1,2]}`, SaveVersion))
	out, err := Upgrade(in)
	if err != nil || string(out) != string(in) {
		t.Fatalf("%s %v", out, err)
	}
}

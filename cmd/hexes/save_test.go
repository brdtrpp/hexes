//go:build js

package main

import (
	"strings"
	"syscall/js"
	"testing"

	"hexes/game"
)

// memStorage swaps in an empty localStorage for one test.
func memStorage(t *testing.T) {
	js.Global().Call("eval", `globalThis.localStorage = (() => {
		const m = new Map();
		return {
			getItem: k => m.has(k) ? m.get(k) : null,
			setItem: (k, v) => m.set(k, String(v)),
			removeItem: k => m.delete(k),
		};
	})()`)
	t.Cleanup(func() { js.Global().Delete("localStorage") })
}

func saveAt(t *testing.T, depth int) string {
	s := game.NewVeteran(7)
	s.DevDepth(depth)
	b, err := s.Save()
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestUnreadableSaveIsSetAside(t *testing.T) {
	memStorage(t)
	newer := `{"Version":999,"Depth":30,"Best":30}`
	savePref(saveKey, newer)
	g := &Game{sim: game.New(1)}
	g.loadSave()
	if loadPref(keptKey) != newer {
		t.Fatal("a save from a newer version wasn't set aside")
	}
	if !strings.Contains(g.hint, "newer version") {
		t.Fatalf("hint %q", g.hint)
	}
	// The stopgap run saves over the main key; the kept one stays.
	savePref(saveKey, saveAt(t, 2))
	g = &Game{sim: game.New(1)}
	g.loadSave()
	if loadPref(keptKey) != newer || g.sim.Depth != 2 {
		t.Fatalf("kept %q, depth %d", loadPref(keptKey), g.sim.Depth)
	}
}

func TestKeptSaveComesBack(t *testing.T) {
	memStorage(t)
	deep := saveAt(t, 4)
	savePref(keptKey, deep)
	savePref(saveKey, saveAt(t, 2))
	g := &Game{sim: game.New(1)}
	g.loadSave()
	if g.sim.Depth != 4 || loadPref(saveKey) != deep || loadPref(keptKey) != "" {
		t.Fatalf("depth %d, kept %q", g.sim.Depth, loadPref(keptKey))
	}
}

func TestDeeperStopgapBeatsKeptSave(t *testing.T) {
	memStorage(t)
	savePref(keptKey, saveAt(t, 2))
	savePref(saveKey, saveAt(t, 4))
	g := &Game{sim: game.New(1)}
	g.loadSave()
	if g.sim.Depth != 4 || loadPref(keptKey) != "" {
		t.Fatalf("depth %d, kept %q", g.sim.Depth, loadPref(keptKey))
	}
}

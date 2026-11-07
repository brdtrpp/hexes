package game

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Saves outlive the build that wrote them, so a change to the save format
// must never cost anyone their progress. The rule:
//
//   - A change Go's JSON decoding already copes with (a new field, a field
//     nobody reads any more) needs nothing: old saves load with the new
//     field at its zero value, and Restore fills in what that means.
//   - Any other change (a field renamed, retyped, split or reinterpreted)
//     bumps SaveVersion and adds a step to migrations that turns the old
//     shape into the new one, working on the raw JSON. Then freeze a save
//     of the new version in testdata (see migrate_test.go).
//
// Old saves climb the steps one version at a time, so a save from any
// version back to MinSaveVersion still loads.

// MinSaveVersion is the oldest save that still loads. Versions 1 and 2
// were dropped before migrations existed and none survive.
const MinSaveVersion = 3

// migrations[v] turns a version v save into a version v+1 save. The map
// holds the save's top-level fields as raw JSON; a step edits them in
// place and needn't touch Version.
var migrations = map[int]func(map[string]json.RawMessage) error{}

var (
	// ErrNewerSave is a save written by a newer build than this one: it
	// can't be read here and must not be overwritten.
	ErrNewerSave = errors.New("save is from a newer version")
	// ErrOldSave is a save older than MinSaveVersion.
	ErrOldSave = errors.New("save is too old to load")
)

// Upgrade brings a save up to SaveVersion, returning it unchanged when it
// already is.
func Upgrade(b []byte) ([]byte, error) {
	return upgrade(b, SaveVersion, migrations)
}

func upgrade(b []byte, to int, steps map[int]func(map[string]json.RawMessage) error) ([]byte, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	if m == nil {
		return nil, errors.New("save is empty")
	}
	v := 0
	if raw, ok := m["Version"]; ok {
		if err := json.Unmarshal(raw, &v); err != nil {
			return nil, fmt.Errorf("save version: %w", err)
		}
	}
	switch {
	case v > to:
		return nil, fmt.Errorf("%w (%d, this build reads %d)", ErrNewerSave, v, to)
	case v < MinSaveVersion:
		return nil, fmt.Errorf("%w (%d)", ErrOldSave, v)
	case v == to:
		return b, nil
	}
	for ; v < to; v++ {
		step := steps[v]
		if step == nil {
			return nil, fmt.Errorf("no migration from save version %d", v)
		}
		if err := step(m); err != nil {
			return nil, fmt.Errorf("migrating save version %d: %w", v, err)
		}
		m["Version"] = json.RawMessage(fmt.Sprint(v + 1))
	}
	return json.Marshal(m)
}

// SaveBest is how deep a save had reached, without loading it: the front
// end uses it to pick between two saves. It's -1 for one that won't read.
func SaveBest(b []byte) int {
	u, err := Upgrade(b)
	if err != nil {
		return -1
	}
	var sv struct{ Best, Depth int }
	if json.Unmarshal(u, &sv) != nil {
		return -1
	}
	return max(sv.Best, sv.Depth)
}

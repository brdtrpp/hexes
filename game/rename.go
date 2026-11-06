package game

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"hexes/skill"
)

// MaxNameLen is the longest name a hex can take, in letters.
const MaxNameLen = 16

// CleanName tidies a typed name: no control characters, spaces
// collapsed, cut to MaxNameLen. It can come back empty.
func CleanName(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) > MaxNameLen {
		s = strings.TrimSpace(string([]rune(s)[:MaxNameLen]))
	}
	return s
}

// RenameHex names hex i: an equipped hex or a module. An empty name
// keeps the one it had. A module's calls and watches follow it to its
// new name, and two modules can't share one. It reports the name the hex
// ends up with.
func (s *Sim) RenameHex(i int, name string) (string, error) {
	l := &s.Load
	if i < 0 || i >= l.Count() || l.Hex(i) == nil {
		return "", errNoHex
	}
	old := l.Name(i)
	name = CleanName(name)
	if name == "" || name == old {
		return old, nil
	}
	if i < MaxSlots {
		l.Names[i] = name
		l.Hexes[i].Name = name
		s.Dirty = true
		return name, nil
	}
	for j, m := range l.Mods {
		if j != i-MaxSlots && strings.EqualFold(m.Name, name) {
			return old, errors.New("another module already has that name")
		}
	}
	l.Mods[i-MaxSlots].Name = name
	for k := 0; k < l.Count(); k++ {
		renameCalls(l.Hex(k), old, name)
	}
	s.Dirty = true
	return name, nil
}

// renameCalls points every call and watch in h that names old at name.
func renameCalls(h *skill.Hex, old, name string) {
	if h == nil {
		return
	}
	for k := range h.Slots {
		if p := &h.Slots[k].Part; h.Slots[k].Kind == skill.SlotPart && p.Lib == old {
			p.Lib = name
		}
	}
}

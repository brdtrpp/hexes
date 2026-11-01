package loot

import "hexes/skill"

// InventorySize is how many loose parts the player can carry.
const InventorySize = 12

// Inventory is a fixed grid of cells. Cells keep their place, so the
// editor can show a stable grid and a tap on an empty cell means "put it
// here".
type Inventory struct {
	Parts [InventorySize]skill.Part
	Has   [InventorySize]bool
}

// Add puts a part in the first empty cell and returns the cell, or -1
// when the inventory is full.
func (v *Inventory) Add(p skill.Part) int {
	for i, h := range v.Has {
		if !h {
			v.Parts[i], v.Has[i] = p, true
			return i
		}
	}
	return -1
}

// Take empties a cell and returns what was in it.
func (v *Inventory) Take(i int) (skill.Part, bool) {
	if i < 0 || i >= InventorySize || !v.Has[i] {
		return skill.Part{}, false
	}
	p := v.Parts[i]
	v.Parts[i], v.Has[i] = skill.Part{}, false
	return p, true
}

// Put fills a cell, replacing whatever was there.
func (v *Inventory) Put(i int, p skill.Part) {
	v.Parts[i], v.Has[i] = p, true
}

// Count is how many cells hold a part.
func (v *Inventory) Count() int {
	n := 0
	for _, h := range v.Has {
		if h {
			n++
		}
	}
	return n
}

// Full reports whether every cell holds a part.
func (v *Inventory) Full() bool { return v.Count() == InventorySize }

// Clear empties every cell.
func (v *Inventory) Clear() { *v = Inventory{} }

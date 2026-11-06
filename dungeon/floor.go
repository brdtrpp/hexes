// Package dungeon generates Hexes dungeon floors on a hex grid.
//
// A floor is a pointy-top hex grid stored as a rectangle of rows (so it
// looks rectangular on screen) but addressed with the same axial
// skill.Coord the combat code uses. Generation is deterministic: the same
// seed and depth always give the same floor, on every platform.
package dungeon

import (
	"math"

	"hexes/skill"
)

// Tile is what a hex on the floor is.
type Tile uint8

const (
	Wall   Tile = iota // solid; pushes stop here and hurt
	Ground             // open ground
	Pit                // a drop; anything pushed in dies
	Hazard             // a plasma vent: walkable, burns whatever stands on it
	Stairs             // the way down to the next depth
)

var tileNames = [...]string{"wall", "floor", "pit", "hazard", "stairs"}

func (t Tile) String() string { return tileNames[t] }

// Walkable reports whether a unit can stand on the tile.
func (t Tile) Walkable() bool { return t == Ground || t == Hazard || t == Stairs }

// Terrain is what combat sees: walls and pits matter, everything else is
// ground.
func (t Tile) Terrain() skill.Terrain {
	switch t {
	case Wall:
		return skill.TerrainWall
	case Pit:
		return skill.TerrainPit
	}
	return skill.TerrainFloor
}

// EnemyKind is which enemy a spawn point makes.
type EnemyKind uint8

const (
	Grunt   EnemyKind = iota // melee, average
	Skitter                  // fast, fragile
	Brute                    // slow, tough, hits hard
	Gunner                   // keeps its distance and shoots
)

var enemyNames = [...]string{"grunt", "skitter", "brute", "gunner"}

func (k EnemyKind) String() string { return enemyNames[k] }

// Layout is the kind of floor: the same seed and depth always give the
// same one. Each builds rooms, corridors and terrain its own way, so two
// floors of the same size can play nothing alike.
type Layout uint8

const (
	Caverns Layout = iota // lobed rooms, mixed corridors: the first floor's kind
	Halls                 // a few big pillared halls joined by wide corridors
	Warrens               // many small rooms, narrow twisting corridors, lots of loops
	Reactor               // one great core chamber ringed by a pit, spokes out to the rooms
	Rift                  // a chasm of pits splits the floor; narrow bridges cross it
	NumLayouts
)

var layoutNames = [...]string{"caverns", "halls", "warrens", "reactor", "rift"}

func (l Layout) String() string {
	if int(l) < len(layoutNames) {
		return layoutNames[l]
	}
	return "floor"
}

// Room is a roughly hexagonal open area.
type Room struct {
	Center skill.Coord
	Radius int
	Kind   Layout // the layout it was built for (a mixed floor has several)
	Core   bool   // a reactor's great chamber
}

// Region is a stretch of a floor's columns, From up to To, built as one
// layout. Most floors are one region; from MixDepth down a floor can be
// two or three, west to east, with a crossing between each pair.
type Region struct {
	Layout   Layout
	From, To int
}

// Spawn is where an enemy starts.
type Spawn struct {
	At    skill.Coord
	Kind  EnemyKind
	Level int
	Room  int
}

// Floor is one generated dungeon level.
type Floor struct {
	Seed  uint64
	Depth int
	// W columns by H rows. Row r, column c is axial (c - r/2, r).
	W, H   int
	Layout Layout // the first region's: where you arrive
	// Regions, west to east. The crossing between two regions is Blend
	// columns either side of their border, and the border bends from row
	// to row by Bend, so it never reads as a straight seam.
	Regions []Region
	Blend   int
	Bend    uint64
	Tiles   []Tile
	Rooms   []Room
	Start   skill.Coord // where the player arrives
	Exit    skill.Coord // the stairs down
	Spawns  []Spawn
	// Pads are the floor's repair pads: open ground in the rooms
	// farthest on foot from both the start and the stairs.
	Pads []skill.Coord
}

// Mixed reports whether the floor holds more than one layout.
func (f *Floor) Mixed() bool { return len(f.Regions) > 1 }

// border is the column where region k gives way to region k+1 on a row.
func (f *Floor) border(k, row int) float64 {
	ph := float64(f.Bend>>uint(8*k)&0xff) / 255 * 2 * math.Pi
	return float64(f.Regions[k].To) + 0.5*float64(f.Blend)*math.Sin(float64(row)*0.21+ph)
}

// Mix says which regions c sits between: a and b, and how far it is into
// b, from 0 to 1 across the crossing (eased, so the middle changes
// fastest). Away from a crossing a == b and t is 0.
func (f *Floor) Mix(c skill.Coord) (a, b int, t float64) {
	n := len(f.Regions)
	if n <= 1 {
		return 0, 0, 0
	}
	col := float64(c.Q + c.R>>1)
	w := float64(max(1, f.Blend))
	for k := 0; k < n-1; k++ {
		bd := f.border(k, c.R)
		if col < bd-w {
			return k, k, 0
		}
		if col < bd+w {
			x := (col - (bd - w)) / (2 * w)
			return k, k + 1, x * x * (3 - 2*x)
		}
	}
	return n - 1, n - 1, 0
}

// RegionAt is the region c mostly belongs to.
func (f *Floor) RegionAt(c skill.Coord) int {
	a, b, t := f.Mix(c)
	if t >= 0.5 {
		return b
	}
	return a
}

// Weight is how much of layout l there is at c: 1 deep inside a region of
// that layout, 0 away from it, in between across a crossing.
func (f *Floor) Weight(c skill.Coord, l Layout) float64 {
	if len(f.Regions) == 0 {
		if f.Layout == l {
			return 1
		}
		return 0
	}
	a, b, t := f.Mix(c)
	w := 0.0
	if f.Regions[a].Layout == l {
		w += 1 - t
	}
	if f.Regions[b].Layout == l {
		w += t
	}
	return min(1, w)
}

// In reports whether c is on the floor.
func (f *Floor) In(c skill.Coord) bool {
	if c.R < 0 || c.R >= f.H {
		return false
	}
	col := c.Q + c.R>>1
	return col >= 0 && col < f.W
}

// Index is c's position in Tiles. c must be on the floor.
func (f *Floor) Index(c skill.Coord) int { return c.R*f.W + c.Q + c.R>>1 }

// CoordOf is the inverse of Index.
func (f *Floor) CoordOf(i int) skill.Coord {
	r := i / f.W
	return skill.Coord{Q: i%f.W - r>>1, R: r}
}

// At returns the tile at c; off the floor is wall.
func (f *Floor) At(c skill.Coord) Tile {
	if !f.In(c) {
		return Wall
	}
	return f.Tiles[f.Index(c)]
}

// Set changes the tile at c if it is on the floor.
func (f *Floor) Set(c skill.Coord, t Tile) {
	if f.In(c) {
		f.Tiles[f.Index(c)] = t
	}
}

// Count returns how many tiles of kind t the floor has.
func (f *Floor) Count(t Tile) int {
	n := 0
	for _, x := range f.Tiles {
		if x == t {
			n++
		}
	}
	return n
}

// DistanceField fills dist (len W*H) with walking steps from `from` to
// every tile, -1 where unreachable. Pits and walls block. queue must have
// room for W*H entries; both buffers are reused so the call does not
// allocate. It returns how many tiles were reached.
func (f *Floor) DistanceField(from skill.Coord, dist, queue []int32) int {
	for i := range dist {
		dist[i] = -1
	}
	if !f.In(from) || !f.At(from).Walkable() {
		return 0
	}
	start := f.Index(from)
	dist[start] = 0
	queue[0] = int32(start)
	head, tail := 0, 1
	for head < tail {
		i := int(queue[head])
		head++
		c := f.CoordOf(i)
		d := dist[i] + 1
		for _, dir := range skill.HexDirs {
			n := c.Add(dir)
			if !f.In(n) {
				continue
			}
			j := f.Index(n)
			if dist[j] >= 0 || !f.Tiles[j].Walkable() {
				continue
			}
			dist[j] = d
			queue[tail] = int32(j)
			tail++
		}
	}
	return tail
}

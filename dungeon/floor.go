// Package dungeon generates Hexes dungeon floors on a hex grid.
//
// A floor is a pointy-top hex grid stored as a rectangle of rows (so it
// looks rectangular on screen) but addressed with the same axial
// skill.Coord the combat code uses. Generation is deterministic: the same
// seed and depth always give the same floor, on every platform.
package dungeon

import "hexes/skill"

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

// Room is a roughly hexagonal open area.
type Room struct {
	Center skill.Coord
	Radius int
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
	Tiles  []Tile
	Rooms  []Room
	Start  skill.Coord // where the player arrives
	Exit   skill.Coord // the stairs down
	Spawns []Spawn
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

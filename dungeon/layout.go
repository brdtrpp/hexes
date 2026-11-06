package dungeon

import "hexes/skill"

// LayoutFor is the layout a floor gets. The first floor is always
// caverns, so the opening stays the same gentle start; below it every
// layout is equally likely, and the reactor (a big open fight) waits
// until depth 3. The pick hashes the seed on its own, so it draws nothing
// from the floor's random stream: a caverns floor is the same floor it
// always was.
func LayoutFor(seed uint64, depth int) Layout {
	if depth <= 1 {
		return Caverns
	}
	x := seed ^ 0x7f4a7c159e3779b9
	x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
	x = (x ^ (x >> 27)) * 0x94d049bb133111eb
	x ^= x >> 31
	pool := []Layout{Caverns, Halls, Warrens, Rift, Reactor}
	if depth < 3 {
		pool = pool[:4]
	}
	return pool[x%uint64(len(pool))]
}

// Tune turns caverns params into a layout's. Floor size, enemy count and
// level stay as depth set them, so every layout is the same fight in a
// different place: halls pack the same foes into fewer, bigger rooms,
// warrens spread them through many small ones.
func Tune(p Params, l Layout) Params {
	p.Layout = l
	switch l {
	case Halls:
		p.Rooms = max(4, p.Rooms/2)
		p.MinRadius = 3
		p.MaxRadius = min(p.MaxRadius+2, 8)
		p.Lobes = 0
		p.WideChance = 1
		p.LoopChance = 0.4
		p.PitClusters /= 2
		p.Hazards = p.Hazards*3/2 + 1
	case Warrens:
		p.Rooms = min(p.Rooms*8/5, MaxRooms+24)
		p.MinRadius = 1
		p.MaxRadius = 2
		if p.EnemyLevel >= 12 {
			p.MaxRadius = 3
		}
		p.Lobes = 1
		p.Gap = 2
		p.WideChance = 0
		p.LoopChance = 0.6
		p.Jogs = 2
		p.PitClusters = p.PitClusters * 2 / 3
		p.PitSize = max(2, p.PitSize-1)
	case Reactor:
		p.Rooms = max(4, p.Rooms*3/4)
		p.PitClusters /= 2
		p.Hazards /= 2
	case Rift:
		p.PitClusters /= 3
	}
	return p
}

// coreRadius is the reactor chamber's size: a fifth of the floor's short
// side, so it grows with the floor.
func coreRadius(p Params) int {
	return max(5, min(11, min(p.W, p.H*2)/5))
}

// placeCore opens the reactor's chamber in the middle of the floor, before
// the other rooms, so they gather around it.
func (g *Generator) placeCore(f *Floor, p Params) {
	rad := coreRadius(p)
	row := p.H / 2
	c := skill.Coord{Q: p.W/2 - row>>1, R: row}
	f.Rooms = append(f.Rooms[:0], Room{Center: c, Radius: rad, Kind: Reactor, Core: true})
	carveHex(f, c, rad)
}

// shapeLayout is each layout's own pass over the rooms and corridors:
// pillars in halls, the reactor's pit ring and spokes, the rift's chasm.
// Anything it cuts off is bridged again, so every floor stays one piece.
func (g *Generator) shapeLayout(f *Floor, rng *rng, p Params) {
	switch p.Layout {
	case Halls:
		for _, r := range f.Rooms {
			pillars(f, r)
		}
	case Reactor:
		g.reactor(f, p)
	case Rift:
		g.rift(f, rng, p)
	default:
		return
	}
	g.bridge(f)
}

// pillars stands six columns in a big room, halfway out from its middle,
// for cover from gunners and something to circle. Rooms of six and up get
// paired columns.
func pillars(f *Floor, r Room) {
	if r.Radius < 4 {
		return
	}
	k := r.Radius / 2
	for i, d := range skill.HexDirs {
		at := r.Center
		for s := 0; s < k; s++ {
			at = at.Add(d)
		}
		f.Set(at, Wall)
		if r.Radius >= 6 {
			f.Set(at.Add(skill.HexDirs[(i+2)%6]), Wall)
		}
	}
}

// reactor rings the core's dais with a pit, leaving six bridges at the
// corners, and runs wide spokes from the core to the three nearest rooms.
// It moves the core off the start, so you arrive at the edge and walk in.
func (g *Generator) reactor(f *Floor, p Params) {
	f.Rooms[0].Core = true
	g.coreWorks(f, 0)
	if len(f.Rooms) > 1 {
		f.Rooms[0], f.Rooms[1] = f.Rooms[1], f.Rooms[0]
	}
}

// coreWorks runs the spokes and cuts the pit ring for the core room ci.
func (g *Generator) coreWorks(f *Floor, ci int) {
	core := f.Rooms[ci]
	// Spokes first: the ring is cut after, so the spokes don't fill it.
	spoke := [3]int{-1, -1, -1}
	for k := range spoke {
		bestD := 1 << 30
		for i := 0; i < len(f.Rooms); i++ {
			if i == ci || i == spoke[0] || i == spoke[1] {
				continue
			}
			if d := skill.Dist(core.Center, f.Rooms[i].Center); d < bestD {
				spoke[k], bestD = i, d
			}
		}
		if spoke[k] < 0 {
			break
		}
		line(f, core.Center, f.Rooms[spoke[k]].Center, true)
	}
	if ring := core.Radius - 2; ring >= 2 {
		at := core.Center
		for s := 0; s < ring; s++ {
			at = at.Add(skill.HexDirs[4])
		}
		for side := 0; side < 6; side++ {
			for s := 0; s < ring; s++ {
				if s != 0 && f.At(at) == Ground {
					f.Set(at, Pit)
				}
				at = at.Add(skill.HexDirs[side])
			}
		}
	}
}

// rift cuts a winding chasm of pits across the floor from side to side.
// Narrow corridors are left standing as bridges; wide floor falls away.
func (g *Generator) rift(f *Floor, rng *rng, p Params) {
	width := 2
	if f.Depth >= 8 {
		width = 3
	}
	row := f.H/3 + rng.intn(max(1, f.H/3))
	for col := 1; col < f.W-1; col++ {
		if rng.intn(3) == 0 {
			row += rng.intn(3) - 1
			row = max(2, min(f.H-2-width, row))
		}
		for r := row; r < row+width; r++ {
			c := skill.Coord{Q: col - r>>1, R: r}
			if f.At(c) != Ground || g.isRoomCenter(f, c) || openAround(f, c) < 4 {
				continue
			}
			// On a mixed floor the chasm only runs through the rift's
			// region, crumbling away across the crossing.
			if w := f.Weight(c, Rift); w < 1 && float64(coordHash(c)&0xffff)/0xffff >= w {
				continue
			}
			f.Set(c, Pit)
		}
	}
}

// openAround counts the walkable tiles next to c.
func openAround(f *Floor, c skill.Coord) int {
	n := 0
	for _, d := range skill.HexDirs {
		if f.At(c.Add(d)).Walkable() || f.At(c.Add(d)) == Pit {
			n++
		}
	}
	return n
}

// bridge joins back any room a layout pass cut off from the first room,
// across the fewest pits (or, failing that, walls). It takes no random
// draws, so it never changes the rest of the floor.
func (g *Generator) bridge(f *Floor) {
	n := len(f.Tiles)
	if cap(g.from) < n {
		g.from = make([]int32, n)
		g.cost = make([]int32, n)
		g.deque = make([]int32, 12*n+2)
		g.done = make([]bool, n)
	}
	for guard := 0; guard < 2*len(f.Rooms)+8; guard++ {
		f.DistanceField(f.Rooms[0].Center, g.dist, g.queue)
		lost := -1
		for i, r := range f.Rooms {
			if g.dist[f.Index(r.Center)] < 0 {
				lost = i
				break
			}
		}
		if lost < 0 {
			return
		}
		if !g.cross(f, f.Rooms[lost].Center, false) && !g.cross(f, f.Rooms[lost].Center, true) {
			return
		}
	}
}

// cross finds the way from the lost room to the reachable floor (g.dist)
// that opens the fewest tiles, walking free over ground and paying one for
// each pit (and wall, when walls is set), then opens it.
func (g *Generator) cross(f *Floor, lost skill.Coord, walls bool) bool {
	n := len(f.Tiles)
	// Each tile is pushed at most once per side it's reached from, so six
	// times n either way is room enough.
	from, cost, dq, done := g.from[:n], g.cost[:n], g.deque[:12*n+2], g.done[:n]
	for i := range cost {
		cost[i], from[i], done[i] = -1, -1, false
	}
	head, tail := 6*n+1, 6*n+1
	s := f.Index(lost)
	cost[s] = 0
	dq[tail] = int32(s)
	tail++
	for head < tail {
		i := int(dq[head])
		head++
		if done[i] {
			continue
		}
		done[i] = true
		if g.dist[i] >= 0 {
			for k := i; k >= 0; k = int(from[k]) {
				if !f.Tiles[k].Walkable() {
					f.Tiles[k] = Ground
				}
			}
			return true
		}
		c := f.CoordOf(i)
		for _, d := range skill.HexDirs {
			nc := c.Add(d)
			if !interior(f, nc) {
				continue
			}
			j := f.Index(nc)
			t := f.Tiles[j]
			step := int32(0)
			switch {
			case t.Walkable():
			case t == Pit, walls && t == Wall:
				step = 1
			default:
				continue
			}
			if c := cost[i] + step; !done[j] && (cost[j] < 0 || c < cost[j]) {
				cost[j], from[j] = c, int32(i)
				if step == 0 {
					head--
					dq[head] = int32(j)
				} else {
					dq[tail] = int32(j)
					tail++
				}
			}
		}
	}
	return false
}

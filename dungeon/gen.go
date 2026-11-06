package dungeon

import (
	"math"

	"hexes/skill"
)

// Params is everything depth decides about a floor. Generate derives it
// from depth with ParamsFor; tests and tools can tweak it and call
// GenerateWith.
type Params struct {
	W, H        int
	Rooms       int     // rooms to try to place
	MinRadius   int     // room size range
	MaxRadius   int     //
	LoopChance  float64 // chance each extra nearby room pair gets its own corridor
	WideChance  float64 // chance a corridor is two hexes wide
	PitClusters int
	PitSize     int // max pits per cluster
	Hazards     int // hazard clusters
	HazardSize  int
	Enemies     int
	EnemyLevel  int     // base level of enemies; also the expected player level
	EliteChance float64 // chance an enemy is two levels above base
	SafeRadius  int     // no enemy spawns this close to the start
}

// Floor growth. Depth 1 is BaseW x BaseH; every floor down adds GrowW
// columns and GrowH rows until MaxW x MaxH (reached around depth 17), so
// each descent is a visibly bigger floor while the deepest ones still
// generate in a few milliseconds and walk across in well under a minute.
const (
	BaseW, BaseH = 36, 26
	GrowW, GrowH = 4, 3
	MaxW, MaxH   = 100, 74
	MaxRooms     = 56
	MaxEnemies   = 100
)

// ParamsFor returns the tuning for a depth (1 is the first floor). The
// floor grows every depth up to a cap; rooms, pits and hazards scale with
// the floor's area so deeper floors are bigger, not emptier, and enemies
// per room climb with depth, which is the progression.
func ParamsFor(depth int) Params {
	if depth < 1 {
		depth = 1
	}
	w := min(BaseW+GrowW*(depth-1), MaxW)
	h := min(BaseH+GrowH*(depth-1), MaxH)
	area := float64(w*h) / float64(BaseW*BaseH) // 1 at depth 1, about 7.9 at the cap
	rooms := min(int(7*area), MaxRooms)
	p := Params{
		W:           w,
		H:           h,
		Rooms:       rooms,
		MinRadius:   2,
		MaxRadius:   min(3+depth/6, 6),
		LoopChance:  0.25,
		WideChance:  0.35,
		PitClusters: min(int(float64(2+depth/2)*area), 48),
		PitSize:     min(3+depth/4, 9),
		Hazards:     min(int(float64(depth/2)*area), 40),
		HazardSize:  min(2+depth/5, 7),
		Enemies:     min(int(math.Round(float64(rooms)*(0.8+0.05*float64(depth)))), MaxEnemies),
		EnemyLevel:  depth,
		EliteChance: 0,
		SafeRadius:  6,
	}
	if depth >= 4 {
		p.EliteChance = min(0.05+0.01*float64(depth), 0.3)
	}
	return p
}

// Generator holds scratch buffers so repeated generation stays cheap. The
// zero value is ready to use; one Generator per goroutine.
type Generator struct {
	dist, queue []int32
	order       []int
	inTree      []bool
	best        []int
	bestD       []int
}

// Generate makes the floor for a seed and depth.
func Generate(seed uint64, depth int) *Floor {
	var g Generator
	return g.Generate(seed, depth)
}

// Generate makes the floor for a seed and depth.
func (g *Generator) Generate(seed uint64, depth int) *Floor {
	return g.GenerateWith(seed, depth, ParamsFor(depth))
}

// GenerateWith makes a floor from explicit params.
func (g *Generator) GenerateWith(seed uint64, depth int, p Params) *Floor {
	f := &Floor{Seed: seed, Depth: depth, W: p.W, H: p.H, Tiles: make([]Tile, p.W*p.H)}
	n := p.W * p.H
	if cap(g.dist) < n {
		g.dist = make([]int32, n)
		g.queue = make([]int32, n)
	}
	g.dist, g.queue = g.dist[:n], g.queue[:n]
	rng := rng{seed ^ 0x9e3779b97f4a7c15*uint64(depth+1)}

	g.placeRooms(f, &rng, p)
	g.connect(f, &rng, p)
	f.Start = f.Rooms[0].Center

	// The exit is the room farthest from the start on foot.
	f.DistanceField(f.Start, g.dist, g.queue)
	exitRoom, far := 0, int32(-1)
	for i, r := range f.Rooms {
		if d := g.dist[f.Index(r.Center)]; d > far {
			exitRoom, far = i, d
		}
	}
	f.Exit = f.Rooms[exitRoom].Center

	g.clusters(f, &rng, p.PitClusters, p.PitSize, Pit, exitRoom)
	g.clusters(f, &rng, p.Hazards, p.HazardSize, Hazard, exitRoom)
	f.Set(f.Exit, Stairs)
	g.spawn(f, &rng, p, exitRoom)
	return f
}

// placeRooms drops non-overlapping hex rooms with a lobe or two each, so
// they read as caverns rather than perfect hexagons.
func (g *Generator) placeRooms(f *Floor, rng *rng, p Params) {
	f.Rooms = make([]Room, 0, p.Rooms)
	margin := p.MaxRadius + 2
	for try := 0; try < p.Rooms*40 && len(f.Rooms) < p.Rooms; try++ {
		rad := p.MinRadius + rng.intn(p.MaxRadius-p.MinRadius+1)
		row := margin + rng.intn(max(1, p.H-2*margin))
		col := margin + rng.intn(max(1, p.W-2*margin))
		c := skill.Coord{Q: col - row>>1, R: row}
		ok := true
		for _, o := range f.Rooms {
			if skill.Dist(c, o.Center) < rad+o.Radius+3 {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		f.Rooms = append(f.Rooms, Room{Center: c, Radius: rad})
		carveHex(f, c, rad)
		for l := rng.intn(3); l > 0; l-- {
			dir := skill.HexDirs[rng.intn(6)]
			lc := c
			for k := 0; k < rad; k++ {
				lc = lc.Add(dir)
			}
			carveHex(f, lc, max(1, rad-1-rng.intn(2)))
		}
	}
	if len(f.Rooms) == 0 { // tiny params: one room in the middle
		c := skill.Coord{Q: p.W/2 - (p.H/2)>>1, R: p.H / 2}
		f.Rooms = append(f.Rooms, Room{Center: c, Radius: 1})
		carveHex(f, c, 1)
	}
}

// carveHex opens every tile within rad of c, never touching the outer
// ring so the floor is always walled in (pushes off the map hit a wall).
func carveHex(f *Floor, c skill.Coord, rad int) {
	for dr := -rad; dr <= rad; dr++ {
		for dq := max(-rad, -dr-rad); dq <= min(rad, -dr+rad); dq++ {
			t := skill.Coord{Q: c.Q + dq, R: c.R + dr}
			if interior(f, t) {
				f.Tiles[f.Index(t)] = Ground
			}
		}
	}
}

func interior(f *Floor, c skill.Coord) bool {
	if c.R < 1 || c.R >= f.H-1 {
		return false
	}
	col := c.Q + c.R>>1
	return col >= 1 && col < f.W-1
}

// connect links every room with a minimum spanning tree over room centers
// (so the floor is always one piece) and then adds a few extra corridors
// between near rooms so the layout has loops to kite around.
func (g *Generator) connect(f *Floor, rng *rng, p Params) {
	n := len(f.Rooms)
	g.inTree = resize(g.inTree, n)
	g.best = resizeInt(g.best, n)
	g.bestD = resizeInt(g.bestD, n)
	for i := range g.inTree {
		g.inTree[i] = false
		g.best[i] = 0
		g.bestD[i] = math.MaxInt
	}
	// Prim's on hex distance. n is small (MaxRooms), so O(n^2) is fine.
	g.inTree[0] = true
	for i := 1; i < n; i++ {
		g.bestD[i] = skill.Dist(f.Rooms[0].Center, f.Rooms[i].Center)
	}
	for added := 1; added < n; added++ {
		pick := -1
		for i := 0; i < n; i++ {
			if !g.inTree[i] && (pick < 0 || g.bestD[i] < g.bestD[pick]) {
				pick = i
			}
		}
		g.inTree[pick] = true
		corridor(f, rng, f.Rooms[g.best[pick]].Center, f.Rooms[pick].Center, p.WideChance)
		for i := 0; i < n; i++ {
			if g.inTree[i] {
				continue
			}
			if d := skill.Dist(f.Rooms[pick].Center, f.Rooms[i].Center); d < g.bestD[i] {
				g.bestD[i], g.best[i] = d, pick
			}
		}
	}
	// Loops: near pairs that the tree didn't join directly.
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			if g.best[j] == i || g.best[i] == j {
				continue
			}
			d := skill.Dist(f.Rooms[i].Center, f.Rooms[j].Center)
			if d < 16 && rng.float() < p.LoopChance {
				corridor(f, rng, f.Rooms[i].Center, f.Rooms[j].Center, p.WideChance)
			}
		}
	}
}

// corridor carves a hex line from a to b, sometimes two wide, with a
// mid-point jog so long corridors aren't ruler-straight.
func corridor(f *Floor, rng *rng, a, b skill.Coord, wide float64) {
	w := rng.float() < wide
	mid := skill.Coord{Q: (a.Q + b.Q) / 2, R: (a.R + b.R) / 2}
	if skill.Dist(a, b) > 8 {
		j := skill.HexDirs[rng.intn(6)]
		mid = mid.Add(j).Add(j)
	}
	line(f, a, mid, w)
	line(f, mid, b, w)
}

func line(f *Floor, a, b skill.Coord, wide bool) {
	n := skill.Dist(a, b)
	for i := 0; i <= n; i++ {
		t := 0.0
		if n > 0 {
			t = float64(i) / float64(n)
		}
		// Nudge so ties round the same way every time.
		q := float64(a.Q) + (float64(b.Q-a.Q))*t + 1e-6
		r := float64(a.R) + (float64(b.R-a.R))*t + 2e-6
		c := cubeRound(q, r)
		if interior(f, c) {
			f.Tiles[f.Index(c)] = Ground
		}
		if wide {
			if e := c.Add(skill.HexDirs[0]); interior(f, e) {
				f.Tiles[f.Index(e)] = Ground
			}
		}
	}
}

func cubeRound(q, r float64) skill.Coord {
	s := -q - r
	rq, rr, rs := math.Round(q), math.Round(r), math.Round(s)
	dq, dr, ds := math.Abs(rq-q), math.Abs(rr-r), math.Abs(rs-s)
	switch {
	case dq > dr && dq > ds:
		rq = -rr - rs
	case dr > ds:
		rr = -rq - rs
	}
	return skill.Coord{Q: int(rq), R: int(rr)}
}

// clusters scatters random-walk blobs of pits or hazards inside rooms. A
// blob never covers a room center (where corridors meet), and a pit blob
// that would cut a room off from the start is undone, so every floor stays
// fully walkable. Pits favor room edges, next to walls, where a push can
// send a foe in.
func (g *Generator) clusters(f *Floor, rng *rng, count, size int, kind Tile, exitRoom int) {
	if count <= 0 || len(f.Rooms) < 2 {
		return
	}
	var placed [16]int
	for c := 0; c < count; c++ {
		ri := 1 + rng.intn(len(f.Rooms)-1)
		room := f.Rooms[ri]
		at := room.Center
		steps := 1 + rng.intn(room.Radius)
		dir := skill.HexDirs[rng.intn(6)]
		for k := 0; k < steps; k++ {
			at = at.Add(dir)
		}
		want := 1 + rng.intn(max(1, size))
		if want > len(placed) {
			want = len(placed)
		}
		n := 0
		for tries := 0; n < want && tries < want*4; tries++ {
			if f.At(at) == Ground && !g.isRoomCenter(f, at) && skill.Dist(at, f.Start) > 2 && skill.Dist(at, f.Exit) > 1 {
				i := f.Index(at)
				f.Tiles[i] = kind
				placed[n] = i
				n++
			}
			at = at.Add(skill.HexDirs[rng.intn(6)])
		}
		if kind != Pit || n == 0 {
			continue
		}
		if !g.roomsConnected(f, exitRoom) {
			for _, i := range placed[:n] {
				f.Tiles[i] = Ground
			}
		}
	}
}

func (g *Generator) isRoomCenter(f *Floor, c skill.Coord) bool {
	for _, r := range f.Rooms {
		if r.Center == c {
			return true
		}
	}
	return false
}

func (g *Generator) roomsConnected(f *Floor, exitRoom int) bool {
	f.DistanceField(f.Start, g.dist, g.queue)
	for _, r := range f.Rooms {
		if g.dist[f.Index(r.Center)] < 0 {
			return false
		}
	}
	return true
}

// spawn places enemies on reachable open floor away from the start. Deeper
// floors get more of them, tougher kinds, and the odd elite.
func (g *Generator) spawn(f *Floor, rng *rng, p Params, exitRoom int) {
	f.DistanceField(f.Start, g.dist, g.queue)
	f.Spawns = make([]Spawn, 0, p.Enemies)
	if len(f.Rooms) < 2 {
		return
	}
	for tries := 0; len(f.Spawns) < p.Enemies && tries < p.Enemies*20; tries++ {
		ri := 1 + rng.intn(len(f.Rooms)-1)
		room := f.Rooms[ri]
		at := room.Center
		for k := rng.intn(room.Radius + 1); k > 0; k-- {
			at = at.Add(skill.HexDirs[rng.intn(6)])
		}
		if f.At(at) != Ground || g.dist[f.Index(at)] < int32(p.SafeRadius) {
			continue
		}
		taken := false
		for _, s := range f.Spawns {
			if s.At == at {
				taken = true
				break
			}
		}
		if taken {
			continue
		}
		kind := Grunt
		roll := rng.float()
		switch {
		case f.Depth >= 3 && roll < 0.15:
			kind = Brute
		case f.Depth >= 2 && roll < 0.40:
			kind = Skitter
		case f.Depth >= 3 && roll < 0.55:
			kind = Gunner
		}
		lvl := p.EnemyLevel
		if rng.float() < p.EliteChance {
			lvl += 2
		}
		f.Spawns = append(f.Spawns, Spawn{At: at, Kind: kind, Level: lvl, Room: ri})
	}
}

func resize(b []bool, n int) []bool {
	if cap(b) < n {
		return make([]bool, n)
	}
	return b[:n]
}

func resizeInt(b []int, n int) []int {
	if cap(b) < n {
		return make([]int, n)
	}
	return b[:n]
}

// rng is SplitMix64, the same generator the skill package uses: identical
// on every platform and Go version.
type rng struct{ s uint64 }

func (r *rng) next() uint64 {
	r.s += 0x9e3779b97f4a7c15
	z := r.s
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}

func (r *rng) intn(n int) int { return int(r.next() % uint64(n)) }

func (r *rng) float() float64 { return float64(r.next()>>11) / (1 << 53) }

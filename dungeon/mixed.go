package dungeon

import "hexes/skill"

// Mixed floors. From MixDepth down most floors hold two layouts, west to
// east, and from TripleDepth some hold three. Between two regions is a
// crossing a couple of dozen columns wide where the two layouts' rules
// overlap: each room there is built as one layout or the other, more
// often the one you're walking toward, so halls' pillared rooms thin out
// among warrens' little ones; a rift's chasm crumbles away across it; the
// palette blends (cmd/hexes). You arrive in the first region and the
// stairs are in the last, so every mixed floor walks you through each
// crossing.
const (
	MixDepth    = 20
	TripleDepth = 30
)

// KindsFor lists the layouts a floor is built from, west to east. Above
// MixDepth it's just LayoutFor. Below it a quarter of floors stay one
// layout, the rest take two different ones, and from TripleDepth half of
// those take three. The first is always LayoutFor's, so a floor's arrival
// is the layout it would have been anyway.
func KindsFor(seed uint64, depth int) []Layout {
	first := LayoutFor(seed, depth)
	if depth < MixDepth {
		return []Layout{first}
	}
	x := seed ^ 0x51ed270b27d4eb4f
	x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
	x = (x ^ (x >> 27)) * 0x94d049bb133111eb
	x ^= x >> 31
	n := 2
	switch {
	case x%4 == 0:
		n = 1
	case depth >= TripleDepth && (x>>2)%2 == 0:
		n = 3
	}
	kinds := []Layout{first}
	pool := make([]Layout, 0, NumLayouts)
	for l := Layout(0); l < NumLayouts; l++ {
		if l != first {
			pool = append(pool, l)
		}
	}
	for len(kinds) < n {
		i := int((x >> uint(8+8*len(kinds))) % uint64(len(pool)))
		kinds = append(kinds, pool[i])
		pool = append(pool[:i], pool[i+1:]...)
	}
	return kinds
}

// GenerateMixed builds a floor from several layouts, west to east. p is
// the depth's caverns params; each region takes its layout's version of
// them for its share of the floor.
func (g *Generator) GenerateMixed(seed uint64, depth int, p Params, kinds []Layout) *Floor {
	f := &Floor{Seed: seed, Depth: depth, W: p.W, H: p.H, Tiles: make([]Tile, p.W*p.H)}
	n := p.W * p.H
	if cap(g.dist) < n {
		g.dist = make([]int32, n)
		g.queue = make([]int32, n)
	}
	g.dist, g.queue = g.dist[:n], g.queue[:n]
	rng := rng{seed ^ 0x9e3779b97f4a7c15*uint64(depth+1) ^ 0x6d1f}
	g.mixed = true
	defer func() { g.mixed = false }()
	for _, k := range kinds {
		g.kp[k] = Tune(p, k)
	}

	// Regions: even shares, each border nudged a little.
	nk := len(kinds)
	f.Layout = kinds[0]
	f.Blend = max(6, p.W/(5*nk))
	f.Bend = rng.next()
	from := 0
	for k, l := range kinds {
		to := p.W
		if k < nk-1 {
			share := p.W / nk
			to = share*(k+1) + rng.intn(share/4+1) - share/8
		}
		f.Regions = append(f.Regions, Region{Layout: l, From: from, To: to})
		from = to
	}

	for k, r := range f.Regions {
		kp := g.kp[r.Layout]
		if r.Layout == Reactor {
			g.placeCoreIn(f, kp, r)
		}
		g.placeRoomsIn(f, &rng, kp.Rooms*(r.To-r.From)/p.W, k)
	}
	if len(f.Rooms) == 0 {
		c := skill.Coord{Q: p.W/2 - (p.H/2)>>1, R: p.H / 2}
		f.Rooms = append(f.Rooms, Room{Center: c, Radius: 1, Kind: kinds[0]})
		carveHex(f, c, 1)
	}
	// You arrive at the west end of the first region, never in a
	// reactor's core, so the walk to the stairs crosses every border.
	start, west := 0, 1<<30
	for pass := 0; pass < 2 && west == 1<<30; pass++ {
		for i, r := range f.Rooms {
			if r.Core || pass == 0 && f.RegionAt(r.Center) != 0 {
				continue
			}
			if col := r.Center.Q + r.Center.R>>1; col < west {
				start, west = i, col
			}
		}
	}
	f.Rooms[0], f.Rooms[start] = f.Rooms[start], f.Rooms[0]
	g.connect(f, &rng, p)

	for _, r := range f.Rooms {
		if r.Kind == Halls && !r.Core {
			pillars(f, r)
		}
	}
	for i := range f.Rooms {
		if f.Rooms[i].Core {
			g.coreWorks(f, i)
		}
	}
	for _, l := range kinds {
		if l == Rift {
			g.rift(f, &rng, p)
		}
	}
	g.bridge(f)
	f.Start = f.Rooms[0].Center

	// The stairs are in the last region: the room there farthest on foot.
	f.DistanceField(f.Start, g.dist, g.queue)
	last := nk - 1
	exitRoom, far := -1, int32(-1)
	for pass := 0; pass < 2 && exitRoom < 0; pass++ {
		for i, r := range f.Rooms {
			if pass == 0 && (r.Core || f.RegionAt(r.Center) != last) {
				continue
			}
			if d := g.dist[f.Index(r.Center)]; d > far {
				exitRoom, far = i, d
			}
		}
	}
	if exitRoom < 0 {
		exitRoom = 0
	}
	f.Exit = f.Rooms[exitRoom].Center

	pits, vents := 0, 0
	for _, r := range f.Regions {
		w := r.To - r.From
		pits += g.kp[r.Layout].PitClusters * w / p.W
		vents += g.kp[r.Layout].Hazards * w / p.W
	}
	g.clusters(f, &rng, pits, p.PitSize, Pit, exitRoom)
	g.clusters(f, &rng, vents, p.HazardSize, Hazard, exitRoom)
	f.Set(f.Exit, Stairs)
	g.spawn(f, &rng, p, exitRoom)
	g.placePads(f, exitRoom)
	return f
}

// paramsOf is the params room i was built with: its layout's on a mixed
// floor, the floor's otherwise.
func (g *Generator) paramsOf(f *Floor, p Params, i int) Params {
	if g.mixed {
		return g.kp[f.Rooms[i].Kind]
	}
	return p
}

// placeCoreIn opens a reactor's chamber in the middle of its region.
func (g *Generator) placeCoreIn(f *Floor, p Params, r Region) {
	q := p
	q.W = r.To - r.From
	rad := coreRadius(q)
	row := p.H / 2
	col := (r.From + r.To) / 2
	c := skill.Coord{Q: col - row>>1, R: row}
	f.Rooms = append(f.Rooms, Room{Center: c, Radius: rad, Kind: Reactor, Core: true})
	carveHex(f, c, rad)
}

// placeRoomsIn drops up to want rooms with their middles in region k.
// Each takes the layout of where it lands; in a crossing that's a roll
// weighted toward the nearer side.
func (g *Generator) placeRoomsIn(f *Floor, rng *rng, want, k int) {
	r := f.Regions[k]
	placed := 0
	for try := 0; try < want*40 && placed < want; try++ {
		col := r.From + rng.intn(max(1, r.To-r.From))
		row := 2 + rng.intn(max(1, f.H-4))
		c := skill.Coord{Q: col - row>>1, R: row}
		a, b, t := f.Mix(c)
		kind := f.Regions[a].Layout
		if a != b && rng.float() < t {
			kind = f.Regions[b].Layout
		}
		kp := g.kp[kind]
		rad := kp.MinRadius + rng.intn(kp.MaxRadius-kp.MinRadius+1)
		m := kp.MaxRadius + 2
		if row < m || row >= f.H-m || col < m || col >= f.W-m {
			continue
		}
		ok := true
		for _, o := range f.Rooms {
			if skill.Dist(c, o.Center) < rad+o.Radius+max(kp.Gap, g.kp[o.Kind].Gap) {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		f.Rooms = append(f.Rooms, Room{Center: c, Radius: rad, Kind: kind})
		placed++
		carveHex(f, c, rad)
		for l := rng.intn(kp.Lobes + 1); l > 0; l-- {
			dir := skill.HexDirs[rng.intn(6)]
			lc := c
			for s := 0; s < rad; s++ {
				lc = lc.Add(dir)
			}
			carveHex(f, lc, max(1, rad-1-rng.intn(2)))
		}
	}
}

// coordHash mixes a tile's coordinate, for choices that shouldn't draw
// from the floor's random stream.
func coordHash(c skill.Coord) uint32 {
	x := uint32(c.Q)*0x9e3779b1 ^ uint32(c.R)*0x85ebca6b
	x ^= x >> 15
	x *= 0x2c1b3c6d
	x ^= x >> 12
	return x
}

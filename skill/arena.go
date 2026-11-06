package skill

import "math"

// Coord is an axial hex coordinate.
type Coord struct{ Q, R int }

// HexDirs are the six neighbor offsets, clockwise from east.
var HexDirs = [6]Coord{{1, 0}, {1, -1}, {0, -1}, {-1, 0}, {-1, 1}, {0, 1}}

// Add returns c moved by d.
func (c Coord) Add(d Coord) Coord { return Coord{c.Q + d.Q, c.R + d.R} }

// Dist is hex distance.
func Dist(a, b Coord) int {
	dq, dr := a.Q-b.Q, a.R-b.R
	return (iabs(dq) + iabs(dr) + iabs(dq+dr)) / 2
}

func (c Coord) pixel() (float64, float64) {
	return math.Sqrt(3) * (float64(c.Q) + float64(c.R)/2), 1.5 * float64(c.R)
}

func iabs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// coneCos is the cosine of the cone's half-angle (35 degrees).
var coneCos = math.Cos(35 * math.Pi / 180)

// Arena is a small reference World on a rhombus of hex tiles. The real game
// will have its own map; this one exists so the evaluator can be tested and
// benchmarked against something concrete. None of its World methods
// allocate.
type Arena struct {
	W, H   int
	Tiles  []Terrain
	Units  []Unit
	Pos    []Coord
	Facing []uint8 // index into HexDirs
	Reg    *Registry
	Rules  Rules

	occ      []EntityID // unit standing on each tile, or NoEntity
	dotClock float32
}

// NewArena makes a w by h arena of floor.
func NewArena(w, h int, reg *Registry, rules Rules) *Arena {
	a := &Arena{W: w, H: h, Tiles: make([]Terrain, w*h), occ: make([]EntityID, w*h),
		Reg: reg, Rules: rules}
	for i := range a.occ {
		a.occ[i] = NoEntity
	}
	return a
}

// In reports whether c is on the map.
func (a *Arena) In(c Coord) bool { return c.Q >= 0 && c.R >= 0 && c.Q < a.W && c.R < a.H }

func (a *Arena) idx(c Coord) int { return c.R*a.W + c.Q }

// Tile returns the terrain at c; off the map is wall.
func (a *Arena) Tile(c Coord) Terrain {
	if !a.In(c) {
		return TerrainWall
	}
	return a.Tiles[a.idx(c)]
}

// SetTile changes the terrain at c.
func (a *Arena) SetTile(c Coord, t Terrain) { a.Tiles[a.idx(c)] = t }

// Spawn adds a unit and returns its ID.
func (a *Arena) Spawn(team uint8, at Coord, level int, maxHP float32) EntityID {
	id := EntityID(len(a.Units))
	a.Units = append(a.Units, Unit{ID: id, Team: team, Alive: true, HP: maxHP, MaxHP: maxHP,
		Level: level, MarkOwner: NoEntity})
	a.Pos = append(a.Pos, at)
	a.Facing = append(a.Facing, 0)
	a.occ[a.idx(at)] = id
	return id
}

// Distance implements Distancer.
func (a *Arena) Distance(x, y EntityID) int {
	if x < 0 || y < 0 || int(x) >= len(a.Pos) || int(y) >= len(a.Pos) {
		return 0
	}
	return Dist(a.Pos[x], a.Pos[y])
}

// Unit implements World.
func (a *Arena) Unit(id EntityID) *Unit {
	if id < 0 || int(id) >= len(a.Units) {
		return nil
	}
	return &a.Units[id]
}

func (a *Arena) foe(caster EntityID, i int) bool {
	u := &a.Units[i]
	return u.Alive && u.Team != a.Units[caster].Team
}

// Select implements World.
func (a *Arena) Select(kind TargetKind, origin, caster EntityID, radius float32, out []EntityID) int {
	if len(out) == 0 {
		return 0
	}
	if kind == TargetSelf {
		out[0] = caster
		return 1
	}
	o := a.Pos[origin]
	switch kind {
	case TargetNearest:
		return a.nearest(o, origin, caster, math.MaxInt, out, nil)
	case TargetAdjacent:
		n := 0
		for i := range a.Units {
			if n == len(out) {
				break
			}
			if EntityID(i) == origin || !a.foe(caster, i) {
				continue
			}
			if float32(Dist(a.Pos[i], o)) <= radius {
				out[n] = EntityID(i)
				n++
			}
		}
		return n
	case TargetCone:
		var dx, dy float64
		if origin == caster {
			dx, dy = HexDirs[a.Facing[caster]].pixel()
		} else {
			ox, oy := o.pixel()
			cx, cy := a.Pos[caster].pixel()
			dx, dy = ox-cx, oy-cy
		}
		dl := math.Hypot(dx, dy)
		ox, oy := o.pixel()
		n := 0
		for i := range a.Units {
			if n == len(out) {
				break
			}
			if EntityID(i) == origin || !a.foe(caster, i) {
				continue
			}
			d := Dist(a.Pos[i], o)
			if d == 0 || float32(d) > radius {
				continue
			}
			px, py := a.Pos[i].pixel()
			vx, vy := px-ox, py-oy
			if (vx*dx+vy*dy)/(math.Hypot(vx, vy)*dl) >= coneCos {
				out[n] = EntityID(i)
				n++
			}
		}
		return n
	case TargetChain:
		cur := o
		n := 0
		for n < len(out) {
			got := a.nearest(cur, origin, caster, int(radius), out[n:n+1], out[:n])
			if got == 0 {
				break
			}
			cur = a.Pos[out[n]]
			n++
		}
		return n
	}
	return 0
}

// nearest fills out with the closest foes to c within maxDist, closest
// first, skipping origin and anything in skip.
func (a *Arena) nearest(c Coord, origin, caster EntityID, maxDist int, out, skip []EntityID) int {
	var dist [MaxTargets]int
	n := 0
next:
	for i := range a.Units {
		if EntityID(i) == origin || !a.foe(caster, i) {
			continue
		}
		for _, s := range skip {
			if s == EntityID(i) {
				continue next
			}
		}
		d := Dist(a.Pos[i], c)
		if d > maxDist {
			continue
		}
		// insertion into the sorted top-len(out)
		j := n
		if j == len(out) {
			if d >= dist[j-1] {
				continue
			}
			j--
		} else {
			n++
		}
		for j > 0 && dist[j-1] > d {
			dist[j], out[j] = dist[j-1], out[j-1]
			j--
		}
		dist[j], out[j] = d, EntityID(i)
	}
	return n
}

// Push implements World.
func (a *Arena) Push(id, from EntityID, hexes int) Terrain {
	p, f := a.Pos[id], a.Pos[from]
	dir := HexDirs[a.Facing[from]]
	if p != f {
		px, py := p.pixel()
		fx, fy := f.pixel()
		vx, vy := px-fx, py-fy
		best := -math.MaxFloat64
		for _, d := range HexDirs {
			dx, dy := d.pixel()
			if dot := vx*dx + vy*dy; dot > best {
				best, dir = dot, d
			}
		}
	}
	for i := 0; i < hexes; i++ {
		next := p.Add(dir)
		t := a.Tile(next)
		if t == TerrainWall {
			return TerrainWall
		}
		if o := a.occ[a.idx(next)]; o != NoEntity && a.Units[o].Alive {
			return TerrainFloor
		}
		a.occ[a.idx(p)] = NoEntity
		a.occ[a.idx(next)] = id
		a.Pos[id] = next
		p = next
		if t == TerrainPit {
			return TerrainPit
		}
	}
	return TerrainFloor
}

// Raise implements World.
func (a *Arena) Raise(id, owner EntityID) bool {
	u := a.Unit(id)
	if u == nil || u.Alive || a.Tile(a.Pos[id]) == TerrainPit {
		return false
	}
	*u = Unit{ID: id, Team: a.Units[owner].Team, Alive: true, MaxHP: u.MaxHP,
		HP: u.MaxHP * a.Rules.RaiseHP, Level: u.Level, MarkOwner: NoEntity}
	return true
}

// Step advances time: status timers, resistance decay and damage-over-time
// ticks. It returns the damage the ticks dealt.
func (a *Arena) Step(dt float32) float32 {
	for i := range a.Units {
		u := &a.Units[i]
		if !u.Alive {
			continue
		}
		u.SlowLeft = max(0, u.SlowLeft-dt)
		u.StunLeft = max(0, u.StunLeft-dt)
		u.MarkLeft = max(0, u.MarkLeft-dt)
		u.TickStealth(dt)
		u.TickSustain(dt)
		a.Rules.Decay(u, dt)
	}
	var total float32
	a.dotClock += dt
	for a.dotClock >= a.Rules.DotInterval {
		a.dotClock -= a.Rules.DotInterval
		for i := range a.Units {
			u := &a.Units[i]
			for k := range u.Dots {
				d := &u.Dots[k]
				if d.Left == 0 || !u.Alive {
					continue
				}
				dealt, _ := a.Rules.ResolveTick(u, d.PerTick, d.Type, int(d.AttackerLevel))
				total += dealt
				d.Left--
				if !u.Alive && (a.Reg.Get(d.Type).RaiseOnKill || u.MarkLeft > 0) {
					a.Raise(u.ID, d.Owner)
				}
			}
		}
	}
	return total
}

// Behind reports whether attacker stands behind target: in the back third
// of the circle around it, opposite the way it faces.
func (a *Arena) Behind(attacker, target EntityID) bool {
	if target < 0 || int(target) >= len(a.Units) || attacker < 0 || int(attacker) >= len(a.Units) {
		return false
	}
	return BehindOf(a.Pos[target], HexDirs[a.Facing[target]], a.Pos[attacker])
}

// BehindOf reports whether a point at `from` is behind something at `at`
// facing dir: more than 120 degrees off its facing.
func BehindOf(at, dir, from Coord) bool {
	if at == from {
		return false
	}
	fx, fy := dir.pixel()
	ax, ay := at.pixel()
	px, py := from.pixel()
	vx, vy := px-ax, py-ay
	return (vx*fx+vy*fy)/(math.Hypot(vx, vy)*math.Hypot(fx, fy)) < -0.5
}

// Pixel converts a hex to pointy-top pixel space for a hex of size 1
// (center to corner).
func (c Coord) Pixel() (float64, float64) { return c.pixel() }

// FromPixel is the hex containing the pointy-top pixel-space point (x, y),
// the inverse of Pixel for a hex of size 1. It rounds in cube coordinates,
// so points on an edge go to one side consistently.
func FromPixel(x, y float64) Coord {
	fr := y * (2.0 / 3)
	fq := x/math.Sqrt(3) - fr/2
	fs := -fq - fr
	q, r, s := math.Round(fq), math.Round(fr), math.Round(fs)
	dq, dr, ds := math.Abs(q-fq), math.Abs(r-fr), math.Abs(s-fs)
	switch {
	case dq > dr && dq > ds:
		q = -r - s
	case dr > ds:
		r = -q - s
	}
	return Coord{int(q), int(r)}
}

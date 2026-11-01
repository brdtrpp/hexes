package skill

// Terrain is what a tile is, or what a push struck.
type Terrain uint8

const (
	TerrainFloor Terrain = iota
	TerrainWall
	TerrainPit
)

// World is what the evaluator needs from the game. Implementations must not
// allocate in these calls if Run is to stay allocation-free.
type World interface {
	// Select writes up to len(out) foes of the caster picked around origin
	// and returns how many it wrote. Self returns the caster.
	Select(kind TargetKind, origin, caster EntityID, radius float32, out []EntityID) int
	// Unit returns the unit with this ID, or nil.
	Unit(id EntityID) *Unit
	// Push shoves id up to hexes away from `from` and reports what it hit:
	// floor (nothing, or stopped by a body), a wall, or a pit it fell into.
	Push(id, from EntityID, hexes int) Terrain
	// Raise turns a corpse into the owner's minion.
	Raise(id, owner EntityID) bool
}

// Observer hears what a program does while it runs: every hit and every
// kill, with who caused it. Trigger dispatch uses it to turn hits into
// on-hit events; the game uses it for damage numbers. It is called in the
// middle of Run, so it must not run another program itself (queue the
// work instead) and must not allocate if Run is to stay allocation-free.
type Observer interface {
	// Hit reports damage landing on target. terrain is true for wall slams
	// and pit falls. A damage-over-time hit reports dealt 0 when applied.
	Hit(caster, target EntityID, dealt float32, t TypeID, terrain bool)
	// Killed reports that caster's program killed target.
	Killed(caster, target EntityID)
}

// Ctx is the moment a program fires.
type Ctx struct {
	Caster EntityID
	// Subject is the trigger's entity: the foe hit for OnHit, the corpse for
	// OnKill. NoEntity for cast triggers, which then use the nearest foe.
	Subject EntityID
	Level   int // caster level, for the per-hit cap
}

// Result tallies one firing.
type Result struct {
	Hits, Kills, Capped, Raised, Statuses, Dots int
	Damage                                      float32
}

// Evaluator runs compiled programs. It keeps every working buffer in fixed
// arrays, so Run does not allocate. One Evaluator per goroutine.
type Evaluator struct {
	Reg   *Registry
	Rules Rules
	// Obs, if set, hears every hit and kill.
	Obs Observer

	regs    [MaxRegs][MaxTargets]EntityID
	n       [MaxRegs]uint8
	scratch [MaxTargets]EntityID
}

// NewEvaluator makes an evaluator for a type registry and rule set.
func NewEvaluator(reg *Registry, rules Rules) *Evaluator {
	return &Evaluator{Reg: reg, Rules: rules}
}

// Run fires a program once.
func (e *Evaluator) Run(p *Program, w World, ctx Ctx) Result {
	var res Result
	for i := 0; i < p.Regs && i < MaxRegs; i++ {
		e.n[i] = 0
	}
	e.regs[RegCaster][0] = ctx.Caster
	e.n[RegCaster] = 1
	if ctx.Subject != NoEntity {
		e.regs[RegSubject][0] = ctx.Subject
		e.n[RegSubject] = 1
	} else {
		e.n[RegSubject] = uint8(w.Select(TargetNearest, ctx.Caster, ctx.Caster, 0, e.regs[RegSubject][:1]))
	}
	// On-hit and on-kill hexes reach out from the foe they fired on (a
	// target's target for free); the rest reach out from the caster.
	e.regs[RegOrigin][0] = ctx.Caster
	if (p.Trigger == OnHit || p.Trigger == OnKill) && ctx.Subject != NoEntity {
		e.regs[RegOrigin][0] = ctx.Subject
	}
	e.n[RegOrigin] = 1
	e.exec(p.Steps, w, &ctx, &res)
	return res
}

func (e *Evaluator) exec(steps []Step, w World, ctx *Ctx, res *Result) {
	for pc := 0; pc < len(steps); pc++ {
		s := &steps[pc]
		switch s.Op {
		case OpSelect:
			if s.Target == TargetSelf {
				e.regs[s.Dst][0] = ctx.Caster
				e.n[s.Dst] = 1
				continue
			}
			if e.n[s.Src] == 0 {
				e.n[s.Dst] = 0
				continue
			}
			limit := MaxTargets
			if s.Target == TargetNearest || s.Target == TargetChain {
				limit = int(s.Count)
			}
			e.n[s.Dst] = uint8(w.Select(s.Target, e.regs[s.Src][0], ctx.Caster, s.Value, e.regs[s.Dst][:limit]))
		case OpUnion:
			dst := &e.regs[s.Dst]
			n := copy(dst[:], e.regs[s.Src][:e.n[s.Src]])
		outer:
			for _, id := range e.regs[s.Src2][:e.n[s.Src2]] {
				if n == MaxTargets {
					break
				}
				for _, have := range dst[:n] {
					if have == id {
						continue outer
					}
				}
				dst[n] = id
				n++
			}
			e.n[s.Dst] = uint8(n)
		case OpDamage:
			for i := 0; i < int(e.n[s.Src]); i++ {
				e.damage(w, ctx, e.regs[s.Src][i], s.Value, s.Type, res)
			}
		case OpPush:
			hexes := int(s.Value + 0.5)
			for i := 0; i < int(e.n[s.Src]); i++ {
				if u := w.Unit(e.regs[s.Src][i]); u != nil && u.Alive {
					e.shove(w, ctx, u, hexes, 0, res)
				}
			}
		case OpStatus:
			e.status(w, ctx, s, res)
		case OpForEach:
			body := steps[pc+1 : pc+1+int(s.Span)]
			for i := 0; i < int(e.n[s.Src]); i++ {
				e.regs[s.Dst][0] = e.regs[s.Src][i]
				e.n[s.Dst] = 1
				e.exec(body, w, ctx, res)
			}
			pc += int(s.Span)
		}
	}
}

func (e *Evaluator) damage(w World, ctx *Ctx, id EntityID, raw float32, t TypeID, res *Result) {
	u := w.Unit(id)
	if u == nil || !u.Alive {
		return
	}
	dt := e.Reg.Get(t)
	res.Hits++
	if dt.Ticks > 0 {
		u.AddDot(Dot{PerTick: raw / float32(dt.Ticks), Left: dt.Ticks, Type: t,
			AttackerLevel: int16(ctx.Level), Owner: ctx.Caster})
		res.Dots++
		if e.Obs != nil {
			e.Obs.Hit(ctx.Caster, id, 0, t, false)
		}
		return
	}
	if u.Frozen {
		if m := e.Reg.Get(u.FrozenBy).ShatterMult; m > 0 {
			raw *= m
		}
		u.Frozen, u.Chill, u.StunLeft = false, 0, 0
	}
	d, capped := e.Rules.Resolve(u, raw, t, ctx.Level, false)
	res.Damage += d
	if capped {
		res.Capped++
	}
	if e.Obs != nil {
		e.Obs.Hit(ctx.Caster, id, d, t, false)
	}
	if dt.SpreadFrac > 0 {
		n := w.Select(TargetAdjacent, id, ctx.Caster, 1, e.scratch[:])
		for j := 0; j < n; j++ {
			v := w.Unit(e.scratch[j])
			if v == nil || !v.Alive {
				continue
			}
			d2, c2 := e.Rules.Resolve(v, raw*dt.SpreadFrac, t, ctx.Level, false)
			res.Damage += d2
			if c2 {
				res.Capped++
			}
			if e.Obs != nil {
				e.Obs.Hit(ctx.Caster, v.ID, d2, t, false)
			}
			if !v.Alive {
				e.killed(w, ctx, v, dt.RaiseOnKill, res)
			}
		}
	}
	if !u.Alive {
		e.killed(w, ctx, u, dt.RaiseOnKill, res)
		return
	}
	if dt.PushPerHit > 0 {
		e.shove(w, ctx, u, int(dt.PushPerHit+0.5), dt.WallImpactFrac, res)
	}
}

// shove pushes u away from the caster and applies terrain: walls hurt,
// pits kill. Both ignore the per-hit cap.
func (e *Evaluator) shove(w World, ctx *Ctx, u *Unit, hexes int, extraImpact float32, res *Result) {
	if u.ID == ctx.Caster || hexes <= 0 {
		return
	}
	var d float32
	switch w.Push(u.ID, ctx.Caster, hexes) {
	case TerrainWall:
		d, _ = e.Rules.Resolve(u, (e.Rules.WallImpact+extraImpact)*u.MaxHP, 0, ctx.Level, true)
	case TerrainPit:
		d, _ = e.Rules.Resolve(u, u.HP, 0, ctx.Level, true)
	default:
		return
	}
	res.Damage += d
	if e.Obs != nil {
		e.Obs.Hit(ctx.Caster, u.ID, d, 0, true)
	}
	if !u.Alive {
		e.killed(w, ctx, u, false, res)
	}
}

func (e *Evaluator) killed(w World, ctx *Ctx, u *Unit, raise bool, res *Result) {
	res.Kills++
	if e.Obs != nil {
		e.Obs.Killed(ctx.Caster, u.ID)
	}
	if raise || u.MarkLeft > 0 {
		if w.Raise(u.ID, ctx.Caster) {
			res.Raised++
		}
	}
}

func (e *Evaluator) status(w World, ctx *Ctx, s *Step, res *Result) {
	dt := e.Reg.Get(s.Type)
	dur := s.Value
	if dt.Status == StatusCloak {
		if c := w.Unit(ctx.Caster); c != nil && dur > c.Stealth {
			c.Stealth = dur
		}
		res.Statuses++
		return
	}
	for i := 0; i < int(e.n[s.Src]); i++ {
		u := w.Unit(e.regs[s.Src][i])
		if u == nil || !u.Alive {
			continue
		}
		res.Statuses++
		switch dt.Status {
		case StatusBurn, StatusToxin:
			ticks := int(dur/e.Rules.DotInterval + 0.5)
			if ticks < 1 {
				ticks = 1
			} else if ticks > 255 {
				ticks = 255
			}
			u.AddDot(Dot{PerTick: dt.StatusPower, Left: uint8(ticks), Type: s.Type,
				AttackerLevel: int16(ctx.Level), Owner: ctx.Caster})
		case StatusChill:
			if u.Chill < 255 {
				u.Chill++
			}
			u.SlowFrac = dt.Slow
			u.SlowLeft = max(u.SlowLeft, dur)
			if dt.FreezeStacks > 0 && u.Chill >= dt.FreezeStacks {
				u.Frozen, u.FrozenBy = true, s.Type
				u.StunLeft = max(u.StunLeft, dur)
			}
		case StatusStagger:
			u.StunLeft = max(u.StunLeft, dur)
		case StatusMark:
			u.MarkLeft = max(u.MarkLeft, dur)
			u.MarkOwner = ctx.Caster
		}
	}
}

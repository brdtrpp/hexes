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

// StatusObserver is an Observer that also hears statuses landing, so the
// game can show a status-only hex's area. It's optional.
type StatusObserver interface {
	Statused(caster, target EntityID, t TypeID)
}

// SustainObserver is an Observer that also hears Mend, Shield and Purge
// landing, so the game can show and sound them. It's optional. kind is
// the effect; amount is hull restored, shield added or Heat removed.
type SustainObserver interface {
	Sustained(caster, target EntityID, kind EffectKind, amount float32)
}

// Ctx is the moment a program fires.
type Ctx struct {
	Caster EntityID
	// Subject is the trigger's entity: the foe hit for OnHit, the corpse for
	// OnKill. NoEntity for cast triggers, which then use the nearest foe.
	Subject EntityID
	Level   int // caster level, for the per-hit cap
	// Rig is the caster's rig, for gates that test Heat or Power. Nil
	// reads as an empty, cold rig.
	Rig *Rig
	// Bank is the firing hex's charge banks, which keep their counts
	// between firings. Nil gives the run a scratch bank that starts empty
	// and is thrown away after (a basic strike has nowhere to keep one).
	// Hexes it calls use the same banks.
	Bank *Bank
	// Calls is the equipped hexes a Call part can run: Call n runs
	// Calls[n-1]. Nil leaves only self-calls working.
	Calls *[MaxEquip]*Program
	// Board is the board's shared banks, X and Y. Nil gives the run a
	// scratch pair that starts empty.
	Board *BoardBank
}

// Bank is one equipped hex's charge banks, A to D. Each is a counter that
// Charge parts add to and drain, and charge gates read.
type Bank [MaxBanks]uint32

// MaxCharge is as high as a bank counts; adding past it stays there.
const MaxCharge = 1 << 30

// Result tallies one firing.
type Result struct {
	Hits, Kills, Capped, Raised, Statuses, Dots int
	Damage                                      float32
	// Mended is hull restored, Shielded shield added and Purged Heat
	// taken off by sustain effects.
	Mended, Shielded, Purged float32
	// Calls is how many called hexes ran. Called has bit n-1 set for each
	// equipped hex n a call ran, for the HUD. Dropped counts calls lost to
	// a full queue.
	Calls, Dropped int
	Called         uint8
	// Halted says the chain was cut short: the rig overheated or was
	// locked when a call came up, or the run hit MaxCalls.
	Halted bool
}

// Call limits. MaxCalls caps how many hexes one firing can call in total
// whatever the rig, so a frame can never stall; MaxPending is how many
// calls can wait at once (more are dropped and counted).
const (
	MaxCalls   = 256
	MaxPending = 32
)

// pending is a queued call.
type pending struct {
	prog    *Program
	subject EntityID
	heat    float32 // scale on the Heat it costs (rarer calls run cooler)
	code    uint8   // which equipped hex it is (0 if unknown)
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
	bank    Bank      // stand-in when Ctx.Bank is nil
	board   BoardBank // stand-in when Ctx.Board is nil

	// The call queue and what's running now.
	queue   [MaxPending]pending
	qHead   int
	qLen    int
	cur     *Program
	curCode uint8
	curHeat float32
	dropped int
	// killBase is the run's kill count when the running hex started, so
	// a Kill-lock can tell whether this hex has killed yet.
	killBase int
}

// NewEvaluator makes an evaluator for a type registry and rule set.
func NewEvaluator(reg *Registry, rules Rules) *Evaluator {
	return &Evaluator{Reg: reg, Rules: rules}
}

// Run fires a program once, then runs every hex its calls queued, in the
// order they queued, until the queue is empty or a limit stops it.
func (e *Evaluator) Run(p *Program, w World, ctx Ctx) Result {
	var res Result
	e.scratch2(&ctx)
	e.qHead, e.qLen, e.dropped = 0, 0, 0
	e.start(p, w, &ctx, ctx.Subject, 0, 1, &res)
	for e.qLen > 0 {
		c := e.queue[e.qHead]
		e.qHead = (e.qHead + 1) % MaxPending
		e.qLen--
		if res.Calls >= MaxCalls || !e.payCall(ctx.Rig, c) {
			res.Halted = true
			break
		}
		res.Calls++
		if c.code > 0 && c.code <= 8 {
			res.Called |= 1 << (c.code - 1)
		}
		e.start(c.prog, w, &ctx, c.subject, c.code, c.heat, &res)
		if r := ctx.Rig; r != nil && r.Locked() && e.qLen > 0 {
			// That call tipped the rig over: like any overheat, it ran,
			// and nothing more does.
			res.Halted = true
			break
		}
	}
	e.qLen = 0
	res.Dropped = e.dropped
	return res
}

// scratch2 gives a run without banks of its own empty stand-ins.
func (e *Evaluator) scratch2(ctx *Ctx) {
	if ctx.Bank == nil {
		e.bank = Bank{}
		ctx.Bank = &e.bank
	}
	if ctx.Board == nil {
		e.board = BoardBank{}
		ctx.Board = &e.board
	}
}

// Check runs a watch hex's check alone and reports how many it found:
// more than zero means the hex should fire. It changes nothing in the
// world (a watched library only picks and tests).
func (e *Evaluator) Check(p *Program, w World, ctx Ctx) int {
	if p == nil || p.WatchSteps == 0 {
		return 0
	}
	var res Result
	e.scratch2(&ctx)
	e.prepare(p, w, &ctx, ctx.Subject, 0, 1)
	e.exec(p.Steps[:p.WatchSteps], w, &ctx, &res)
	return int(e.n[p.WatchRet])
}

// payCall adds a call's Heat to the rig. It refuses while the rig is
// locked. Reaching max heat still runs the call but locks the rig, the
// same as TryFire.
func (e *Evaluator) payCall(r *Rig, c pending) bool {
	if r == nil {
		return true
	}
	if r.Locked() {
		return false
	}
	r.Heat = max(0, r.Heat+max(c.prog.Heat, CallMinHeat)*c.heat)
	if r.Heat >= r.MaxHeat {
		r.Heat = r.MaxHeat
		r.ShutdownLeft = OverheatShutdown
	}
	return true
}

// prepare loads a hex's fixed registers: caster, subject, origin.
func (e *Evaluator) prepare(p *Program, w World, ctx *Ctx, subject EntityID, code uint8, heat float32) {
	for i := 0; i < p.Regs && i < MaxRegs; i++ {
		e.n[i] = 0
	}
	e.regs[RegCaster][0] = ctx.Caster
	e.n[RegCaster] = 1
	if subject != NoEntity {
		e.regs[RegSubject][0] = subject
		e.n[RegSubject] = 1
	} else {
		e.n[RegSubject] = uint8(w.Select(TargetNearest, ctx.Caster, ctx.Caster, 0, e.regs[RegSubject][:1]))
	}
	// On-hit and on-kill hexes reach out from the foe they fired on (a
	// target's target for free); the rest reach out from the caster.
	e.regs[RegOrigin][0] = ctx.Caster
	if (p.Trigger == OnHit || p.Trigger == OnKill) && subject != NoEntity {
		e.regs[RegOrigin][0] = subject
	}
	e.n[RegOrigin] = 1
	e.cur, e.curCode, e.curHeat = p, code, heat
}

// start runs one hex's steps with subject as its subject. A watch hex
// stops after its check when the check found nobody.
func (e *Evaluator) start(p *Program, w World, ctx *Ctx, subject EntityID, code uint8, heat float32, res *Result) {
	e.prepare(p, w, ctx, subject, code, heat)
	e.killBase = res.Kills
	if p.WatchSteps > 0 && p.Trigger == Watch {
		e.exec(p.Steps[:p.WatchSteps], w, ctx, res)
		if e.n[p.WatchRet] == 0 {
			return
		}
		e.exec(p.Steps[p.WatchSteps:], w, ctx, res)
		return
	}
	e.exec(p.Steps, w, ctx, res)
}

// call queues a hex for every entity in a step's group.
func (e *Evaluator) call(ctx *Ctx, s *Step) {
	var p *Program
	code := s.Count
	heat := s.Value
	if code == CallSelf {
		p, code = e.cur, e.curCode
	} else if ctx.Calls != nil && int(code) <= MaxEquip {
		p = ctx.Calls[code-1]
	}
	if p == nil {
		return
	}
	if heat <= 0 {
		heat = 1
	}
	for i := 0; i < int(e.n[s.Src]); i++ {
		if e.qLen == MaxPending {
			e.dropped++
			continue
		}
		e.queue[(e.qHead+e.qLen)%MaxPending] = pending{prog: p, subject: e.regs[s.Src][i], heat: heat, code: code}
		e.qLen++
	}
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
			if s.Flag&FlagCrowd != 0 {
				e.n[s.Dst] = e.crowd(w, ctx, s, limit)
				continue
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
			v := e.bent(ctx, s)
			for i := 0; i < int(e.n[s.Src]); i++ {
				before := res.Damage
				e.damage(w, ctx, e.regs[s.Src][i], v, s.Type, res)
				if r := ctx.Rig; s.Flag&FlagLeech != 0 && r != nil {
					r.Power = min(r.MaxPower, r.Power+(res.Damage-before)*LeechFrac)
				}
			}
		case OpPush:
			hexes := int(e.bent(ctx, s) + 0.5)
			for i := 0; i < int(e.n[s.Src]); i++ {
				if u := w.Unit(e.regs[s.Src][i]); u != nil && u.Alive {
					e.shove(w, ctx, u, hexes, 0, res)
				}
			}
		case OpStatus:
			e.status(w, ctx, s, e.bent(ctx, s), res)
		case OpIfKill:
			if res.Kills <= e.killBase {
				pc += int(s.Span) // nothing killed yet: skip the copy
			}
		case OpGate:
			e.gate(w, ctx, s)
		case OpCall:
			e.call(ctx, s)
		case OpCharge:
			if e.n[s.Src2] == 0 {
				continue // in a gate branch nobody took
			}
			b := e.bankAt(ctx, s.Bank)
			*b = uint32(min(max(int64(*b)+int64(s.Value), 0), MaxCharge))
		case OpMend, OpShield:
			if e.n[s.Src2] == 0 {
				continue // in a gate branch nobody took
			}
			kind := EffectMend
			if s.Op == OpShield {
				kind = EffectShield
			}
			so, _ := e.Obs.(SustainObserver)
			for i := 0; i < int(e.n[s.Src]); i++ {
				u := w.Unit(e.regs[s.Src][i])
				if u == nil || !u.Alive {
					continue
				}
				var got float32
				if kind == EffectMend {
					got = u.Mend(s.Value)
					res.Mended += got
				} else {
					got = u.AddShield(s.Value)
					res.Shielded += got
				}
				if so != nil {
					so.Sustained(ctx.Caster, u.ID, kind, got)
				}
			}
		case OpPurge:
			r := ctx.Rig
			if e.n[s.Src2] == 0 || r == nil {
				continue
			}
			got := min(r.Heat, s.Value)
			r.Heat -= got
			res.Purged += got
			if so, ok := e.Obs.(SustainObserver); ok {
				so.Sustained(ctx.Caster, ctx.Caster, EffectPurge, got)
			}
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

func (e *Evaluator) status(w World, ctx *Ctx, s *Step, dur float32, res *Result) {
	dt := e.Reg.Get(s.Type)
	if dt.Status == StatusCloak {
		if e.n[s.Src2] == 0 {
			return // in a gate branch nobody took
		}
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
		if so, ok := e.Obs.(StatusObserver); ok {
			so.Statused(ctx.Caster, u.ID, s.Type)
		}
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

// gate splits register Src into Dst (passed) and Dst2 (failed).
func (e *Evaluator) gate(w World, ctx *Ctx, s *Step) {
	src := e.regs[s.Src][:e.n[s.Src]]
	var np, nf uint8
	if s.Gate.Charged() {
		// Whole group, one way, decided by the bank.
		pass := false
		b := e.bankAt(ctx, s.Bank)
		switch s.Gate {
		case GateEmpty:
			pass = *b == 0
		case GateSpend:
			if len(src) > 0 && *b >= uint32(s.Count) {
				*b -= uint32(s.Count)
				pass = true
			}
		}
		dst, n := &e.regs[s.Dst2], &nf
		if pass {
			dst, n = &e.regs[s.Dst], &np
		}
		*n = uint8(copy(dst[:], src))
		e.n[s.Dst], e.n[s.Dst2] = np, nf
		return
	}
	if s.Gate == GateCond {
		if e.cur == nil || int(s.Count) >= len(e.cur.Conds) {
			e.n[s.Dst], e.n[s.Dst2] = 0, uint8(copy(e.regs[s.Dst2][:], src))
			return
		}
		cp := &e.cur.Conds[s.Count]
		if !cp.Foe {
			dst, n := &e.regs[s.Dst2], &nf
			if e.testCond(w, ctx, cp, NoEntity) {
				dst, n = &e.regs[s.Dst], &np
			}
			*n = uint8(copy(dst[:], src))
			e.n[s.Dst], e.n[s.Dst2] = np, nf
			return
		}
		for _, id := range src {
			if e.testCond(w, ctx, cp, id) {
				e.regs[s.Dst][np] = id
				np++
			} else {
				e.regs[s.Dst2][nf] = id
				nf++
			}
		}
		e.n[s.Dst], e.n[s.Dst2] = np, nf
		return
	}
	if !s.Gate.PerFoe() || (s.Gate == GateStatus && e.Reg.Get(s.Type).Status == StatusCloak) {
		// One test for the whole group: it all goes one way.
		dst, n := &e.regs[s.Dst2], &nf
		if e.casterTest(w, ctx, s) {
			dst, n = &e.regs[s.Dst], &np
		}
		*n = uint8(copy(dst[:], src))
		e.n[s.Dst], e.n[s.Dst2] = np, nf
		return
	}
	for _, id := range src {
		if e.foeTest(w, ctx, s, id) {
			e.regs[s.Dst][np] = id
			np++
		} else {
			e.regs[s.Dst2][nf] = id
			nf++
		}
	}
	e.n[s.Dst], e.n[s.Dst2] = np, nf
}

// casterTest is a gate condition that reads the caster, not the foe.
func (e *Evaluator) casterTest(w World, ctx *Ctx, s *Step) bool {
	r := ctx.Rig
	switch s.Gate {
	case GateHeat:
		return r != nil && r.Heat > s.Value*r.MaxHeat
	case GatePower:
		return r != nil && r.Power > s.Value*r.MaxPower
	case GateStatus: // a cloak type: the caster is stealthed right now
		u := w.Unit(ctx.Caster)
		return u != nil && u.Stealth > 0
	}
	return false
}

// foeTest is a gate condition on one foe. Dead or missing foes fail.
func (e *Evaluator) foeTest(w World, ctx *Ctx, s *Step, id EntityID) bool {
	u := w.Unit(id)
	if u == nil || !u.Alive {
		return false
	}
	switch s.Gate {
	case GateHull:
		return u.HP < s.Value*u.MaxHP
	case GateCrowd:
		return w.Select(TargetAdjacent, id, ctx.Caster, s.Value, e.scratch[:]) >= int(s.Count)
	case GateStatus:
		return e.hasStatus(u, s.Type)
	}
	return false
}

// hasStatus reports whether u suffers type t's status: burning or eaten
// by toxin of that type, chilled or frozen, staggered, marked.
func (e *Evaluator) hasStatus(u *Unit, t TypeID) bool {
	switch e.Reg.Get(t).Status {
	case StatusBurn, StatusToxin:
		for _, d := range u.Dots {
			if d.Left > 0 && d.Type == t {
				return true
			}
		}
	case StatusChill:
		return u.Frozen || u.SlowLeft > 0
	case StatusStagger:
		return u.StunLeft > 0
	case StatusMark:
		return u.MarkLeft > 0
	}
	return false
}

// bankRead is what bank b holds right now.
func (e *Evaluator) bankRead(ctx *Ctx, b uint8) uint32 { return *e.bankAt(ctx, b) }

// bankAt is bank b: the hex's own A to D, or the board's X and Y.
func (e *Evaluator) bankAt(ctx *Ctx, b uint8) *uint32 {
	switch {
	case b < MaxBanks:
		return &ctx.Bank[b]
	case b < NumBanks && ctx.Board != nil:
		return &ctx.Board[b-MaxBanks]
	}
	return &e.board[(b-MaxBanks)%BoardBanks]
}

// bent is a step's number after the frame bends that act at run time:
// Lonely reads how many it lands on, Overflow reads the rig's Heat.
func (e *Evaluator) bent(ctx *Ctx, s *Step) float32 {
	v := s.Value
	if s.Flag&FlagLonely != 0 {
		if e.n[s.Src] == 1 {
			v *= LonelyHit
		} else {
			v *= LonelyMiss
		}
	}
	if r := ctx.Rig; s.Flag&FlagOverflow != 0 && r != nil {
		if over := r.Heat - OverflowAt*r.MaxHeat; over > 0 {
			v *= 1 + OverflowPer*over
		}
	}
	return v
}

// crowd runs a Crowd-framed select: one more pick for every foe within
// CrowdRadius of where it picks from, and elites (foes above the
// caster's level) are passed over.
func (e *Evaluator) crowd(w World, ctx *Ctx, s *Step, limit int) uint8 {
	from := e.regs[s.Src][0]
	limit = min(MaxTargets, limit+w.Select(TargetAdjacent, from, ctx.Caster, CrowdRadius, e.scratch[:]))
	dst := &e.regs[s.Dst]
	got := w.Select(s.Target, from, ctx.Caster, s.Value, dst[:MaxTargets])
	n := 0
	for _, id := range dst[:got] {
		if u := w.Unit(id); u != nil && u.Level > ctx.Level {
			continue
		}
		dst[n] = id
		n++
		if n == limit {
			break
		}
	}
	return uint8(n)
}

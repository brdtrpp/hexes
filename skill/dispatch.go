package skill

// Trigger dispatch: turning things that happen in the game (a button
// press, a hit landing, a kill, the clock, taking damage, a strike from
// stealth or from behind) into hexes firing, with each firing paid for in
// Power and Heat on the owner's rig.
//
// Events go through a fixed ring buffer. A program that runs can produce
// more events (its hits and kills); those queue behind it rather than
// running inside it, so the Evaluator is never re-entered and the whole
// chain stays allocation-free. Three things keep chains finite: Power,
// a per-trigger proc cooldown, and MaxChain.

// MaxEquip is how many hexes one loadout holds.
const MaxEquip = 8

// MaxQueue is how many pending events the dispatcher holds. Events past
// this are dropped and counted.
const MaxQueue = 256

// Event is something that happened that may fire hexes.
type Event struct {
	Trigger TriggerKind
	// Actor is whose hexes may fire: the attacker for on-hit and on-kill,
	// the victim for on-hurt.
	Actor EntityID
	// Subject becomes the program's subject: the foe hit, the corpse, the
	// attacker for on-hurt. NoEntity lets the program pick the nearest foe.
	Subject EntityID
	// Chain counts how many program firings led here. Events past MaxChain
	// fire nothing.
	Chain uint8
	// Slot, if 0 or more, fires only that slot (periodic timers and button
	// casts); -1 fires every matching slot.
	Slot int8
	// Pulse is a stance hex's pulse while it's on, not a toggle.
	Pulse bool
	// Rung is how high an overload was held or rage stood (1-3), for the
	// pieces wired to rungs II and III. 0 runs every rung.
	Rung uint8
	// Scale multiplies the firing's damage: a guard's stored hits, a
	// follow-up's step.
	Scale float32
	// AtOrigin runs the hex from where Origin stands, not the caster: a
	// device running a blueprint, a minion's brain.
	AtOrigin bool
	Origin   EntityID
}

// Outcome is what happened when a slot tried to fire.
type Outcome uint8

const (
	Fired      Outcome = iota
	NoPower            // not enough Power
	Locked             // rig overheated or venting
	OnCooldown         // proc cooldown still running
	Empty              // nothing in the slot, or wrong trigger for the call
	NoFuel             // a sacrifice with too little hull, or a wreck cost with no wreck
	Off                // a stance hex switched off
)

var outcomeNames = [...]string{"fired", "no power", "locked", "cooldown", "empty", "no fuel", "off"}

func (o Outcome) String() string { return outcomeNames[o] }

// Listener hears every hit, every kill and every firing attempt. The
// game uses it for effects and HUD flashes. Like Observer, it must not run
// programs or allocate.
type Listener interface {
	Observer
	Fired(owner EntityID, slot int, p *Program, o Outcome, res Result)
}

// Senses is optional world knowledge the dispatcher uses for surprise
// triggers. A World that also implements it gets from-behind triggers.
type Senses interface {
	Behind(attacker, target EntityID) bool
}

// Loadout is one entity's equipped hexes and the rig that pays for them.
type Loadout struct {
	Owner EntityID
	Rig   *Rig
	Progs [MaxEquip]*Program
	// Timer is seconds until each periodic slot fires next.
	Timer [MaxEquip]float32
	// Cool is the proc cooldown left on each slot.
	Cool [MaxEquip]float32
	// Banks, if set, is where each slot's charge banks live, so the
	// caller can keep them when the dispatcher is rebuilt (a new floor).
	// Nil uses banks inside the loadout, which Equip clears.
	Banks *[MaxEquip]Bank
	own   [MaxEquip]Bank
	// Board is the board's shared banks X and Y. Nil gives each firing an
	// empty scratch pair.
	Board *BoardBank
	// Hold is whether CAST is held down, for channel hexes.
	Hold bool
	// On is which stance hexes are on.
	On [MaxEquip]bool

	// Held is the overload or guard button held down (-1 none), and
	// HeldFor how long it has been.
	Held    int8
	HeldFor float32
	// Soak is the hull a held guard has taken, which its release spends.
	Soak float32
	// Rage is the berserker's meter, 0 to RageMax: kills and hull spent
	// fill it, time drains it, a rage hex spends it.
	Rage float32
	// Combo is the seconds left to follow up on ComboFoe, the foe the
	// last pressed hex hit, at step ComboStep.
	Combo     float32
	ComboFoe  EntityID
	ComboStep uint8
	lastHit   EntityID
}

// School tuning.
const (
	// OverloadRung2 and OverloadRung3 are the seconds held to reach
	// rungs II and III.
	OverloadRung2 = 0.6
	OverloadRung3 = 1.4
	// GuardSoak is how much of a hit a raised guard stops; GuardScale is
	// what stored hull adds to the release (x1 + GuardScale x share of
	// max hull taken), up to GuardMax.
	GuardSoak  = 0.5
	GuardScale = 6
	GuardMax   = 4
	// RageMax is a full meter; RageKill and RageHull (per share of max
	// hull spent) fill it and RageDrain empties it per second.
	RageMax   = 100
	RageKill  = 20
	RageHull  = 150
	RageDrain = 6
	// ComboWindow is the seconds after a pressed hex to follow up, and
	// ComboStep what each step adds to a follow-up's damage.
	ComboWindow = 1.5
	ComboBonus  = 0.35
	ComboSteps  = 4
	// BrainCD is the least time between one slot's brain firings.
	BrainCD = 0.5
)

// RageRung is the rung a rage hex fires at for a meter level.
func RageRung(rage float32) uint8 {
	switch {
	case rage >= 2*RageMax/3:
		return 3
	case rage >= RageMax/3:
		return 2
	}
	return 1
}

// HoldRung is the rung an overload held this long fires at.
func HoldRung(held float32) uint8 {
	switch {
	case held >= OverloadRung3:
		return 3
	case held >= OverloadRung2:
		return 2
	}
	return 1
}

// Guarding reports whether a guard hex is held up: hits on the owner
// should go through Soaked.
func (l *Loadout) Guarding() bool {
	return l.Held >= 0 && l.Progs[l.Held] != nil && l.Progs[l.Held].Trigger == Guard
}

// Soaked takes a hit of raw damage on a held guard: it returns what
// gets through and stores the rest for the release.
func (l *Loadout) Soaked(raw float32) float32 {
	if !l.Guarding() || raw <= 0 {
		return raw
	}
	l.Soak += raw * GuardSoak
	return raw * (1 - GuardSoak)
}

// Following reports whether a follow-up pressed now would land on the
// last foe hit, and at what step.
func (l *Loadout) Following() (EntityID, uint8, bool) {
	return l.ComboFoe, l.ComboStep, l.Combo > 0 && l.ComboFoe != NoEntity
}

// Stances is the stances on right now, one bit per StanceKind.
func (l *Loadout) Stances() uint8 {
	var m uint8
	for s, p := range l.Progs {
		if k, ok := p.StanceKind(); ok && l.On[s] {
			m |= 1 << k
		}
	}
	return m
}

// Bank is slot's charge banks.
func (l *Loadout) Bank(slot int) *Bank {
	if l.Banks != nil {
		return &l.Banks[slot]
	}
	return &l.own[slot]
}

// Stats counts what the dispatcher did, for tuning and tests.
type Stats struct {
	Events, Fired, NoPower, Locked, OnCooldown, ChainCut, Dropped, Checks int
}

// Dispatcher routes events to loadouts.
type Dispatcher struct {
	Eval  *Evaluator
	World World
	// Listener, if set, hears hits, kills and firings.
	Listener Listener
	// MaxChain is how deep firings may cascade. A cast is chain 0, its hits
	// are chain 1, a hex those hits fire makes chain-2 hits, and so on;
	// events deeper than MaxChain fire nothing.
	MaxChain uint8
	// ProcCooldown is the minimum seconds between firings of one slot, per
	// trigger kind. Button casts are limited by Power and Heat instead.
	ProcCooldown [NumTriggers]float32
	Stats        Stats

	senses Senses
	loads  []Loadout
	q      [MaxQueue]Event
	head   int
	n      int
	chain  uint8 // chain of the program running now
}

// Default dispatch tuning.
const (
	DefaultMaxChain   = 3
	DefaultHitProcCD  = 0.3
	DefaultHurtProcCD = 0.5
)

// NewDispatcher wires a dispatcher to an evaluator and world. It takes
// over the evaluator's Observer.
func NewDispatcher(e *Evaluator, w World) *Dispatcher {
	d := &Dispatcher{Eval: e, World: w, MaxChain: DefaultMaxChain}
	d.ProcCooldown[OnHit] = DefaultHitProcCD
	d.ProcCooldown[OnHurt] = DefaultHurtProcCD
	d.ProcCooldown[Brain] = BrainCD
	d.senses, _ = w.(Senses)
	e.Obs = d
	return d
}

// Equip gives owner a loadout and returns it for further tweaks. Calling
// it again for the same owner replaces the loadout.
func (d *Dispatcher) Equip(owner EntityID, rig *Rig, progs ...*Program) *Loadout {
	var l *Loadout
	for i := range d.loads {
		if d.loads[i].Owner == owner {
			l = &d.loads[i]
		}
	}
	if l == nil {
		d.loads = append(d.loads, Loadout{})
		l = &d.loads[len(d.loads)-1]
	}
	*l = Loadout{Owner: owner, Rig: rig, Held: -1, ComboFoe: NoEntity, lastHit: NoEntity}
	for i, p := range progs {
		if i == MaxEquip {
			break
		}
		l.Progs[i] = p
		l.Timer[i] = startTimer(p)
	}
	return l
}

// startTimer is a slot's timer when its hex is first equipped.
func startTimer(p *Program) float32 {
	switch {
	case p == nil:
	case p.Trigger == Periodic:
		return p.Period
	case p.Trigger == Watch:
		return WatchEvery
	}
	return 0
}

// StartTimer resets a slot's timer for a newly edited hex: a periodic
// hex waits a full period, a watch a check, a channel fires at once.
func (l *Loadout) StartTimer(slot int) {
	if slot >= 0 && slot < MaxEquip {
		l.Timer[slot] = startTimer(l.Progs[slot])
	}
}

// Channel tells the dispatcher whether owner is holding CAST: channel
// hexes run every ChannelEvery seconds while it's held.
func (d *Dispatcher) Channel(owner EntityID, held bool) {
	if l := d.Loadout(owner); l != nil {
		l.Hold = held
	}
}

// Unequip drops owner's loadout, for when it dies or leaves.
func (d *Dispatcher) Unequip(owner EntityID) {
	for i := range d.loads {
		if d.loads[i].Owner == owner {
			d.loads = append(d.loads[:i], d.loads[i+1:]...)
			return
		}
	}
}

// Loadout returns owner's loadout, or nil.
func (d *Dispatcher) Loadout(owner EntityID) *Loadout {
	for i := range d.loads {
		if d.loads[i].Owner == owner {
			return &d.loads[i]
		}
	}
	return nil
}

// Update advances time: rigs recharge and cool, cooldowns run down and
// periodic hexes fire when their timer comes up.
func (d *Dispatcher) Update(dt float32) {
	for i := range d.loads {
		l := &d.loads[i]
		l.Rig.Tick(dt)
		if l.Held >= 0 {
			l.HeldFor += dt
		}
		l.Rage = max(0, l.Rage-RageDrain*dt)
		if l.Combo = max(0, l.Combo-dt); l.Combo == 0 {
			l.ComboStep = 0
		}
		for s, p := range l.Progs {
			if p == nil {
				continue
			}
			l.Cool[s] = max(0, l.Cool[s]-dt)
			if p.Stance != 0 {
				if l.On[s] && l.Rig.Overheated() {
					// An overheat drops every stance: holding one is a Heat
					// budget, not a free switch. A vent keeps them.
					l.On[s] = false
				}
				if l.On[s] {
					l.Timer[s] -= dt
					if l.Timer[s] <= 0 {
						l.Timer[s] = StancePulse
						if p.Pulses {
							// The pulse pays the whole hex again.
							d.push(Event{Trigger: p.Trigger, Actor: l.Owner, Subject: NoEntity, Slot: int8(s), Pulse: true})
						} else {
							// A bare stance pays its upkeep in Heat.
							l.Rig.AddHeat(StanceUpkeep)
						}
					}
				}
				if p.Trigger != Periodic && p.Trigger != Watch && p.Trigger != Channel {
					continue
				}
			}
			switch p.Trigger {
			case Channel:
				d.channel(l, s, dt)
				continue
			case Watch:
				d.watch(l, s, dt)
				continue
			case Periodic:
			default:
				continue
			}
			l.Timer[s] -= dt
			if l.Timer[s] <= 0 {
				l.Timer[s] += p.Period
				if l.Timer[s] <= 0 {
					l.Timer[s] = p.Period
				}
				d.push(Event{Trigger: Periodic, Actor: l.Owner, Subject: NoEntity, Slot: int8(s)})
			}
		}
	}
	d.drain()
}

// channel runs a held channel hex: at once on the press, then every
// ChannelEvery seconds until it's let go or the rig locks.
func (d *Dispatcher) channel(l *Loadout, s int, dt float32) {
	if !l.Hold {
		l.Timer[s] = 0
		return
	}
	l.Timer[s] -= dt
	if l.Timer[s] > 0 {
		return
	}
	l.Timer[s] += ChannelEvery
	if l.Timer[s] <= 0 {
		l.Timer[s] = ChannelEvery
	}
	d.push(Event{Trigger: Channel, Actor: l.Owner, Subject: NoEntity, Slot: int8(s)})
}

// watch checks a watch hex every WatchEvery seconds, paying WatchHeat
// each time whatever it finds, and fires it when the check finds anyone.
// After firing it rests WatchRest seconds. A locked rig doesn't check.
func (d *Dispatcher) watch(l *Loadout, s int, dt float32) {
	l.Timer[s] -= dt
	if l.Timer[s] > 0 {
		return
	}
	l.Timer[s] = WatchEvery
	p := l.Progs[s]
	u := d.World.Unit(l.Owner)
	if p.WatchSteps == 0 || l.Rig.Locked() || u == nil || !u.Alive {
		return
	}
	l.Rig.AddHeat(WatchHeat)
	d.Stats.Checks++
	ctx := Ctx{Caster: l.Owner, Subject: NoEntity, Level: u.Level, Rig: l.Rig, Bank: l.Bank(s), Board: l.Board}
	if d.Eval.Check(p, d.World, ctx) == 0 {
		return
	}
	d.chain = 0
	if d.fire(l, s, Event{Trigger: Watch, Actor: l.Owner, Subject: NoEntity, Slot: int8(s)}) == Fired {
		l.Timer[s] = WatchRest
	}
}

// Cast is a button press on one slot. Only on-cast slots answer it. If
// the caster is stealthed or behind subject, its from-stealth and
// from-behind hexes fire too (and the cast breaks stealth).
func (d *Dispatcher) Cast(owner EntityID, slot int, subject EntityID) Outcome {
	l := d.Loadout(owner)
	if l == nil || slot < 0 || slot >= MaxEquip || l.Progs[slot] == nil || l.Progs[slot].Trigger != OnCast {
		return Empty
	}
	d.surprise(owner, subject)
	o := d.fire(l, slot, Event{Trigger: OnCast, Actor: owner, Subject: subject, Slot: int8(slot)})
	d.unambush(owner)
	d.drain()
	d.unhide(owner)
	return o
}

// Press is a button press on one slot, for every hex a button runs: an
// on-cast hex casts, a rage hex fires at the meter's rung and spends it,
// a follow-up lands on the last foe hit, and an overload or guard starts
// being held (Release fires it).
func (d *Dispatcher) Press(owner EntityID, slot int, subject EntityID) Outcome {
	l := d.Loadout(owner)
	if l == nil || slot < 0 || slot >= MaxEquip || l.Progs[slot] == nil {
		return Empty
	}
	p := l.Progs[slot]
	ev := Event{Trigger: p.Trigger, Actor: owner, Subject: subject, Slot: int8(slot)}
	switch p.Trigger {
	case OnCast:
		return d.Cast(owner, slot, subject)
	case Overload, Guard:
		if l.Held >= 0 {
			return Empty
		}
		if l.Rig.Locked() {
			return Locked
		}
		l.Held, l.HeldFor, l.Soak = int8(slot), 0, 0
		return Fired
	case Rage:
		ev.Rung = RageRung(l.Rage)
	case FollowUp:
		if foe, step, ok := l.Following(); ok {
			if u := d.World.Unit(foe); u != nil && u.Alive {
				ev.Subject = foe
				ev.Scale = 1 + ComboBonus*float32(step)
			}
		}
	default:
		return Empty
	}
	d.surprise(owner, ev.Subject)
	o := d.fire(l, slot, ev)
	if o == Fired && p.Trigger == Rage {
		l.Rage = max(0, l.Rage-RageMax/3*float32(ev.Rung-1))
	}
	d.unambush(owner)
	d.drain()
	d.unhide(owner)
	return o
}

// Release lets go of a held overload or guard and fires it: an overload
// at the rung it was held to, a guard scaled by the hull it stored.
func (d *Dispatcher) Release(owner EntityID, subject EntityID) Outcome {
	l := d.Loadout(owner)
	if l == nil || l.Held < 0 {
		return Empty
	}
	slot := int(l.Held)
	p := l.Progs[slot]
	l.Held = -1
	if p == nil {
		return Empty
	}
	ev := Event{Trigger: p.Trigger, Actor: owner, Subject: subject, Slot: int8(slot)}
	if p.Trigger == Overload {
		ev.Rung = HoldRung(l.HeldFor)
	} else if u := d.World.Unit(owner); u != nil && u.MaxHP > 0 {
		ev.Scale = min(GuardMax, 1+GuardScale*l.Soak/u.MaxHP)
	}
	l.HeldFor, l.Soak = 0, 0
	d.surprise(owner, subject)
	o := d.fire(l, slot, ev)
	d.unambush(owner)
	d.drain()
	d.unhide(owner)
	return o
}

// unambush ends the ambush a cast or strike out of stealth had.
func (d *Dispatcher) unambush(owner EntityID) {
	if u := d.World.Unit(owner); u != nil {
		u.Ambush = false
	}
}

// unhide ends what's left of a cast out of stealth once its events ran.
func (d *Dispatcher) unhide(owner EntityID) {
	if u := d.World.Unit(owner); u != nil {
		u.Hidden = false
	}
}

// Post queues an event without running the queue: for game-side causes
// that happen while a hex is running (a wreck falling mid-firing). The
// next Update, cast or event runs it.
func (d *Dispatcher) Post(ev Event) { d.push(ev) }

// RunDevice runs a placed device's hex from where it stands, on its
// owner's rig and banks, then runs whatever its hits set off. subject is
// who it lands on with nothing else beside it (NoEntity: the foe nearest
// the device). It costs nothing: the hex paid when it was placed.
func (d *Dispatcher) RunDevice(dep Deployment, origin, subject EntityID) Result {
	var res Result
	u := d.World.Unit(dep.Caster)
	if u == nil || !u.Alive || dep.Prog == nil {
		return res
	}
	ctx := Ctx{Caster: dep.Caster, Subject: subject, Level: u.Level}
	if l := d.Loadout(dep.Caster); l != nil {
		ctx.Rig, ctx.Calls, ctx.Board = l.Rig, &l.Progs, l.Board
		for s, p := range l.Progs {
			if p == dep.Prog {
				ctx.Bank = l.Bank(s)
			}
		}
	}
	d.chain = 1
	res = d.Eval.RunAt(dep.Prog, dep.Lo, dep.Hi, d.World, ctx, origin)
	d.drain()
	return res
}

// Strike runs a basic attack: p fires for free (no Power, no Heat, no
// cooldown) and its hits and kills trigger hexes like any other. A strike
// counts as an attack for from-stealth and from-behind. p may be nil to
// only check the surprise triggers.
func (d *Dispatcher) Strike(owner EntityID, p *Program, subject EntityID) Result {
	d.surprise(owner, subject)
	var res Result
	if p != nil {
		u := d.World.Unit(owner)
		if u == nil || !u.Alive {
			return res
		}
		d.chain = 0
		ctx := Ctx{Caster: owner, Subject: subject, Level: u.Level}
		if l := d.Loadout(owner); l != nil {
			ctx.Rig = l.Rig
		}
		res = d.Eval.Run(p, d.World, ctx)
	}
	d.unambush(owner)
	d.drain()
	d.unhide(owner)
	return res
}

// Hurt tells the dispatcher victim took a hit from attacker, for on-hurt
// hexes. Damage the game deals outside programs (enemy claws, hazards)
// goes through here.
func (d *Dispatcher) Hurt(victim, attacker EntityID) {
	d.push(Event{Trigger: OnHurt, Actor: victim, Subject: attacker, Slot: -1})
	d.drain()
}

// Emit queues any event and runs the queue, for game-side causes such as
// a damage-over-time tick killing something (Trigger OnKill, Chain 1).
func (d *Dispatcher) Emit(ev Event) {
	d.push(ev)
	d.drain()
}

// Hit implements Observer: a program's hit queues an on-hit event for the
// caster. Terrain hits (wall slams, pit falls) don't count as hits.
func (d *Dispatcher) Hit(caster, target EntityID, dealt float32, t TypeID, terrain bool) {
	if d.Listener != nil {
		d.Listener.Hit(caster, target, dealt, t, terrain)
	}
	if !terrain {
		if l := d.Loadout(caster); l != nil {
			l.lastHit = target
		}
	}
	if !terrain {
		d.push(Event{Trigger: OnHit, Actor: caster, Subject: target, Chain: d.chain + 1, Slot: -1})
	}
}

// Sustained implements SustainObserver, passing Mend, Shield and Purge
// on to a Listener that wants them.
func (d *Dispatcher) Sustained(caster, target EntityID, kind EffectKind, amount float32) {
	if so, ok := d.Listener.(SustainObserver); ok {
		so.Sustained(caster, target, kind, amount)
	}
}

// Statused implements StatusObserver, passing statuses on to a Listener
// that wants them.
func (d *Dispatcher) Statused(caster, target EntityID, t TypeID) {
	if so, ok := d.Listener.(StatusObserver); ok {
		so.Statused(caster, target, t)
	}
}

// Killed implements Observer: a program's kill queues an on-kill event.
func (d *Dispatcher) Killed(caster, target EntityID) {
	if d.Listener != nil {
		d.Listener.Killed(caster, target)
	}
	if l := d.Loadout(caster); l != nil {
		l.Rage = min(RageMax, l.Rage+RageKill)
	}
	d.push(Event{Trigger: OnKill, Actor: caster, Subject: target, Chain: d.chain + 1, Slot: -1})
}

// surprise queues from-stealth and from-behind events for an attack and
// breaks the attacker's stealth.
func (d *Dispatcher) surprise(owner, subject EntityID) {
	u := d.World.Unit(owner)
	if u == nil {
		return
	}
	if u.Stealth > 0 {
		u.Reveal()
		u.Ambush = true // this attack lands at AmbushMult
		u.Hidden = true // and the cloak gate passes for it
		d.push(Event{Trigger: FromStealth, Actor: owner, Subject: subject, Slot: -1})
	}
	if subject != NoEntity && d.senses != nil && d.senses.Behind(owner, subject) {
		d.push(Event{Trigger: FromBehind, Actor: owner, Subject: subject, Slot: -1})
	}
}

func (d *Dispatcher) push(ev Event) {
	d.Stats.Events++
	if d.n == MaxQueue {
		d.Stats.Dropped++
		return
	}
	d.q[(d.head+d.n)%MaxQueue] = ev
	d.n++
}

// drain runs queued events until none are left. Each firing can queue
// more; MaxChain, cooldowns and Power make sure it ends, and the loop cap
// is a last guard.
func (d *Dispatcher) drain() {
	for guard := 0; d.n > 0 && guard < 4*MaxQueue; guard++ {
		ev := d.q[d.head]
		d.head = (d.head + 1) % MaxQueue
		d.n--
		if ev.Chain > d.MaxChain {
			d.Stats.ChainCut++
			continue
		}
		l := d.Loadout(ev.Actor)
		if l == nil {
			continue
		}
		if ev.Slot >= 0 {
			if p := l.Progs[ev.Slot]; p != nil && p.Trigger == ev.Trigger {
				d.fire(l, int(ev.Slot), ev)
			}
			continue
		}
		for s, p := range l.Progs {
			if p != nil && p.Trigger == ev.Trigger {
				d.fire(l, s, ev)
			}
		}
	}
}

// fire tries one slot: cooldown, then Power and Heat, then run.
func (d *Dispatcher) fire(l *Loadout, slot int, ev Event) Outcome {
	p := l.Progs[slot]
	u := d.World.Unit(l.Owner)
	if u == nil || !u.Alive {
		return Empty
	}
	var o Outcome
	var res Result
	act, _ := d.World.(Actions)
	toggle := p.Stance != 0 && !ev.Pulse
	switch {
	case toggle && l.On[slot] && ev.Trigger == OnCast:
		// A cast stance switches off for free.
		l.On[slot] = false
		o = Off
	case p.Inert:
		o = Empty
	case l.Cool[slot] > 0:
		o = OnCooldown
		d.Stats.OnCooldown++
	case l.Rig.Locked() && ev.Trigger != OnVent:
		// A vent locks the rig, but its own on-vent hexes are the point of
		// venting: they still fire, into the Heat the vent just dumped.
		o = Locked
		d.Stats.Locked++
	case p.HullCost > 0 && u.HP <= p.HullCost*u.MaxHP:
		o = NoFuel
	case p.WreckCost > 0 && (act == nil || !act.Wrecks(l.Owner, int(p.WreckCost), false)):
		o = NoFuel
	case !l.Rig.TryFire(p) && (ev.Trigger != OnVent || !l.Rig.Pay(p)):
		o = NoPower
		d.Stats.NoPower++
	default:
		if p.HullCost > 0 {
			u.HP = max(1, u.HP-p.HullCost*u.MaxHP)
			l.Rage = min(RageMax, l.Rage+RageHull*p.HullCost)
		}
		if p.WreckCost > 0 {
			act.Wrecks(l.Owner, int(p.WreckCost), true)
		}
		if toggle {
			// A stance comes on: a common one turns the others off.
			l.On[slot] = true
			l.Timer[slot] = StancePulse
			if !p.StanceKeep {
				for s, q := range l.Progs {
					if s != slot && q != nil && q.Stance != 0 && !q.StanceKeep {
						l.On[s] = false
					}
				}
			}
		}
		o = Fired
		d.Stats.Fired++
		l.Cool[slot] = d.ProcCooldown[p.Trigger]
		if p.Haste > 0 {
			l.Cool[slot] *= p.Haste
		}
		d.chain = ev.Chain
		ctx := Ctx{Caster: l.Owner, Subject: ev.Subject, Level: u.Level, Rig: l.Rig,
			Bank: l.Bank(slot), Calls: &l.Progs, Board: l.Board, Rung: ev.Rung, Scale: ev.Scale}
		if ev.AtOrigin {
			res = d.Eval.RunAt(p, 0, len(p.Steps), d.World, ctx, ev.Origin)
		} else {
			l.lastHit = NoEntity
			res = d.Eval.Run(p, d.World, ctx)
		}
		if p.Trigger.Pressed() || p.Trigger.Held() {
			// A pressed hex opens the follow-up window on who it hit last.
			if l.lastHit != NoEntity {
				if p.Trigger == FollowUp && l.Combo > 0 {
					l.ComboStep = min(ComboSteps, l.ComboStep+1)
				} else {
					l.ComboStep = 1
				}
				l.Combo, l.ComboFoe = ComboWindow, l.lastHit
			}
		}
	}
	if d.Listener != nil {
		d.Listener.Fired(l.Owner, slot, p, o, res)
	}
	return o
}

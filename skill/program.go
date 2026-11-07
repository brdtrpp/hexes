package skill

import (
	"fmt"
	"strings"
)

// Op is one instruction in a compiled hex.
type Op uint8

const (
	// OpSelect fills register Dst with targets picked around the first
	// entity in register Src.
	OpSelect Op = iota
	// OpUnion fills Dst with Src then any of Src2 not already in it. Used
	// when an effect sits between two targets and lands on both.
	OpUnion
	// OpDamage hits every entity in Src.
	OpDamage
	// OpPush shoves every entity in Src away from the caster.
	OpPush
	// OpStatus applies Type's status to every entity in Src.
	OpStatus
	// OpForEach runs the next Span steps once per entity in Src, with that
	// entity alone in register Dst. This is how nested hexes run without a
	// tree: the body is inlined right after this step.
	OpForEach
	// OpGate splits Src in two: entities that pass condition Gate go to
	// Dst, the rest to Dst2. Branches are just effects on those two
	// registers, so a branch nobody took runs on an empty set and does
	// nothing: no jumps.
	OpGate
	// OpCharge adds Value (negative drains) to charge bank Bank, clamped
	// to 0..MaxCharge, but only while guard register Src2 holds someone:
	// in a gate branch nobody took it does nothing.
	OpCharge
	// OpCall queues hex Count (CallSelf for the running one) to run once
	// per entity in Src, each with that entity as its subject, after the
	// running hex finishes. Value scales the Heat each run costs. Calls
	// queue rather than nest, so a loop is a hex calling itself again and
	// never grows a stack; Heat and MaxCalls end it.
	OpCall
	// OpIfKill runs the next Span steps only if this hex's run has killed
	// something by now (a Kill-lock frame's second firing).
	OpIfKill
	// OpMend restores hull to every entity in Src: Value of each one's
	// max hull, less for recent mends (Unit.Mend). It only runs while
	// guard register Src2 holds someone.
	OpMend
	// OpShield puts a Value share of max hull as a shield on every entity
	// in Src, guarded by Src2 like OpMend.
	OpShield
	// OpPurge takes Value Heat off the caster's rig, once, while guard
	// register Src2 holds someone.
	OpPurge
	// OpTaunt holds every foe in Src on the caster for Value seconds.
	OpTaunt
	// OpBuff puts effect Count (thorns or haste) on the caster for Value
	// seconds, guarded by Src2.
	OpBuff
	// OpRaise raises every wreck in Src as the caster's drone for Value
	// seconds.
	OpRaise
	// OpMove moves the caster up to Value hexes in mode Count (see
	// MoveMode), toward or away from the first entity in Src. Guarded by
	// Src2.
	OpMove
	// OpConsume uses up every wreck in Src.
	OpConsume
	// OpDeploy places form Count (rule Bank) and skips the Span steps
	// after it: the device runs them from where it stands (see
	// Deployment). A world that can't place devices runs them here once.
	OpDeploy
	// OpRung skips the next Span steps unless the firing reached rung
	// Value (an overload held that long, rage that high). A firing with
	// no rung reaches every rung.
	OpRung
)

var opNames = [...]string{"select", "union", "damage", "push", "status", "foreach", "gate", "charge", "call", "ifkill", "mend", "shield", "purge",
	"taunt", "buff", "raise", "move", "consume", "deploy", "rung"}

// MoveMode is how OpMove moves the caster.
type MoveMode uint8

const (
	MoveDash   MoveMode = iota // along the line to the target, up to Value
	MoveToward                 // a jump to beside the target
	MoveAway                   // a jump away from the target
	MoveBack                   // back to where the last dash started
	MoveHide                   // back, and unseen for a moment
)

// Step flags: runtime bends a frame puts on one step.
const (
	FlagLeech    = 1 << iota // damage: LeechFrac of what it deals comes back as Power
	FlagLonely               // damage, push, status: LonelyHit on one foe, LonelyMiss on more
	FlagOverflow             // damage, push, status: grows with Heat over OverflowAt
	FlagCrowd                // select: +1 per foe within CrowdRadius, no elites
	FlagLive                 // select: keep Src when it's alive, else the nearest live one to it
)

func (o Op) String() string { return opNames[o] }

// Fixed registers every program starts with.
const (
	RegCaster  = 0 // the caster
	RegOrigin  = 1 // where top-level targets select around (see Run)
	RegSubject = 2 // where untargeted top-level effects land (see Run)
)

// Limits that keep evaluation in fixed-size arrays.
const (
	MaxRegs    = 128 // registers per program (a docked chain needs room)
	MaxTargets = 24  // entities per register
)

// Step is one flat instruction. Every number a multiplier could touch has
// already been multiplied in at compile time.
type Step struct {
	Op     Op
	Target TargetKind
	Type   TypeID
	Src    uint8
	Src2   uint8
	Dst    uint8
	Count  uint8
	Dst2   uint8    // OpGate: where the failures go
	Gate   GateKind // OpGate: the condition
	Bank   uint8    // OpCharge and charge gates: which bank
	Span   uint16
	Flag   uint8 // frame bends that act at run time (Flag...)
	Value  float32
	// Bonus, on damage, multiplies it on marked foes (a marked
	// multiplier). Zero means none.
	Bonus float32
	// Tag names the effect a step came from, for the working. Only
	// Explain sets it, and play never reads it.
	Tag uint16
}

// Program is a compiled hex.
type Program struct {
	Name    string
	Trigger TriggerKind
	Steps   []Step
	// Power is spent and Heat added each time the program fires. Heat can
	// be negative when cryo parts vent more than the rest generates.
	Power, Heat float32
	Depth       int
	Regs        int
	// Period is seconds between firings for Periodic programs.
	Period float32
	// Banks has bit b set when the hex uses charge bank b. Spend[b] is
	// what the first spend gate on bank b takes (0 if none), for the HUD.
	Banks uint8
	Spend [NumBanks]uint8
	// Calls has bit 0 set when the hex calls itself and bit n when it
	// calls equipped hex n.
	Calls uint16
	// Shape is the hex's shape. Loops is set when it's a Loop whose last
	// slot carries someone back round: its last step is the back edge.
	Shape ShapeKind
	Loops bool
	// Docked is how many docked hexes a firing runs (see Hex.Docks).
	Docked uint8
	// Conds are the conditions its condition gates test; a GateCond
	// step's Count indexes them.
	Conds []CondProg
	// A watch hex's first WatchSteps steps are its check, which leaves
	// who it found in register WatchRet; the rest only run when that's
	// anyone. Zero WatchSteps on a watch hex watches nothing yet.
	WatchSteps int
	WatchRet   uint8
	// Inert is set on a hex with nothing in it that acts (no effect,
	// charge or call, here or in what it nests, docks or calls). It
	// doesn't fire: a hex you equip needs one.
	Inert bool
	// Haste scales the trigger's proc cooldown (a Hair trigger frame);
	// zero means 1. A periodic's Period is already scaled.
	Haste float32
	// LockExtra is seconds added to an overheat this hex's firing causes
	// (an Overflow frame).
	LockExtra float32
	// HullCost is the share of max hull a sacrifice hex takes each firing
	// in place of Heat; WreckCost is how many wrecks a wreck-cost hex uses
	// up. See CostKind.
	HullCost  float32
	WreckCost uint8
	// Stance is 1 + the StanceKind of a stance hex, zero for the rest.
	// StanceKeep is set when it can stay on beside another stance.
	Stance     uint8
	StanceKeep bool
	// Pulses is set on a stance hex with something to pulse.
	Pulses bool
	// Device is 1 + the FormKind of a hex that places a device, zero for
	// the rest.
	Device uint8
}

// StanceKind is a stance hex's stance; ok is false for other hexes.
func (p *Program) StanceKind() (StanceKind, bool) {
	if p == nil || p.Stance == 0 {
		return 0, false
	}
	return StanceKind(p.Stance - 1), true
}

// CallsHex reports whether the program has a Call aimed at target
// (CallSelf or an equipped hex number).
func (p *Program) CallsHex(target uint8) bool { return target < 16 && p.Calls&(1<<target) != 0 }

// UsesBank reports whether the program touches charge bank b.
func (p *Program) UsesBank(b int) bool { return p.Banks&(1<<b) != 0 }

// String disassembles the program, one step per line, nested bodies
// indented under their foreach.
func (p *Program) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s  trigger=%s power=%.1f heat=%.1f depth=%d regs=%d\n",
		p.Name, p.Trigger, p.Power, p.Heat, p.Depth, p.Regs)
	var ends []int
	for i, s := range p.Steps {
		for len(ends) > 0 && i >= ends[len(ends)-1] {
			ends = ends[:len(ends)-1]
		}
		fmt.Fprintf(&b, "%3d %s", i, strings.Repeat("  ", len(ends)))
		switch s.Op {
		case OpSelect:
			fmt.Fprintf(&b, "r%d = select %s from r%d", s.Dst, s.Target, s.Src)
			if s.Target.Counted() {
				fmt.Fprintf(&b, " count=%d", s.Count)
			}
			if s.Target.Ranged() {
				fmt.Fprintf(&b, " radius=%.2f", s.Value)
			}
		case OpUnion:
			fmt.Fprintf(&b, "r%d = r%d + r%d", s.Dst, s.Src, s.Src2)
		case OpDamage:
			fmt.Fprintf(&b, "damage r%d type=%d amount=%.2f", s.Src, s.Type, s.Value)
			if s.Bonus != 0 {
				fmt.Fprintf(&b, " marked x%.2f", s.Bonus)
			}
		case OpPush:
			fmt.Fprintf(&b, "push r%d hexes=%.2f", s.Src, s.Value)
		case OpStatus:
			fmt.Fprintf(&b, "status r%d type=%d seconds=%.2f", s.Src, s.Type, s.Value)
		case OpForEach:
			fmt.Fprintf(&b, "foreach r%d in r%d (%d steps)", s.Dst, s.Src, s.Span)
			ends = append(ends, i+1+int(s.Span))
		case OpGate:
			fmt.Fprintf(&b, "r%d, r%d = gate %s r%d", s.Dst, s.Dst2, s.Gate, s.Src)
			switch s.Gate {
			case GateStatus:
				fmt.Fprintf(&b, " type=%d", s.Type)
			case GateCrowd:
				fmt.Fprintf(&b, " count=%d radius=%.2f", s.Count, s.Value)
			case GateSpend:
				fmt.Fprintf(&b, " bank=%s n=%d", BankName(s.Bank), s.Count)
			case GateEmpty:
				fmt.Fprintf(&b, " bank=%s", BankName(s.Bank))
			case GateCond:
				fmt.Fprintf(&b, " cond=%d", s.Count)
			default:
				fmt.Fprintf(&b, " at=%.2f", s.Value)
			}
		case OpCharge:
			fmt.Fprintf(&b, "bank %s %+g if r%d", BankName(s.Bank), s.Value, s.Src2)
		case OpCall:
			fmt.Fprintf(&b, "call %s on r%d heat x%.2f", CallName(s.Count), s.Src, s.Value)
		case OpMend:
			fmt.Fprintf(&b, "mend r%d share=%.3f if r%d", s.Src, s.Value, s.Src2)
		case OpShield:
			fmt.Fprintf(&b, "shield r%d share=%.3f if r%d", s.Src, s.Value, s.Src2)
		case OpPurge:
			fmt.Fprintf(&b, "purge heat=%.2f if r%d", s.Value, s.Src2)
		case OpIfKill:
			fmt.Fprintf(&b, "if killed (%d steps)", s.Span)
			ends = append(ends, i+1+int(s.Span))
		case OpTaunt:
			fmt.Fprintf(&b, "taunt r%d seconds=%.2f", s.Src, s.Value)
		case OpBuff:
			fmt.Fprintf(&b, "%s self seconds=%.2f if r%d", EffectKind(s.Count), s.Value, s.Src2)
		case OpRaise:
			fmt.Fprintf(&b, "raise r%d seconds=%.2f", s.Src, s.Value)
		case OpMove:
			fmt.Fprintf(&b, "move mode=%d at r%d hexes=%.2f if r%d", s.Count, s.Src, s.Value, s.Src2)
		case OpConsume:
			fmt.Fprintf(&b, "consume r%d", s.Src)
		case OpRung:
			fmt.Fprintf(&b, "rung %g (%d steps)", s.Value, s.Span)
			ends = append(ends, i+1+int(s.Span))
		case OpDeploy:
			fmt.Fprintf(&b, "deploy %s rule=%d (%d steps)", FormKind(s.Count), s.Bank, s.Span)
			ends = append(ends, i+1+int(s.Span))
		}
		for f, name := range [...]string{"leech", "lonely", "overflow", "crowd"} {
			if s.Flag&(1<<f) != 0 {
				b.WriteString(" +" + name)
			}
		}
		b.WriteByte('\n')
	}
	return b.String()
}

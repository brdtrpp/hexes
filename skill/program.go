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
)

var opNames = [...]string{"select", "union", "damage", "push", "status", "foreach", "gate", "charge", "call"}

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
	Value  float32
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
			if s.Target == TargetNearest || s.Target == TargetChain {
				fmt.Fprintf(&b, " count=%d", s.Count)
			}
			if s.Target != TargetSelf && s.Target != TargetNearest {
				fmt.Fprintf(&b, " radius=%.2f", s.Value)
			}
		case OpUnion:
			fmt.Fprintf(&b, "r%d = r%d + r%d", s.Dst, s.Src, s.Src2)
		case OpDamage:
			fmt.Fprintf(&b, "damage r%d type=%d amount=%.2f", s.Src, s.Type, s.Value)
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
		}
		b.WriteByte('\n')
	}
	return b.String()
}

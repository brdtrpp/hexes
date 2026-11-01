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
)

var opNames = [...]string{"select", "union", "damage", "push", "status", "foreach"}

func (o Op) String() string { return opNames[o] }

// Fixed registers every program starts with.
const (
	RegCaster  = 0 // the caster
	RegOrigin  = 1 // where top-level targets select around (see Run)
	RegSubject = 2 // where untargeted top-level effects land (see Run)
)

// Limits that keep evaluation in fixed-size arrays.
const (
	MaxRegs    = 32 // registers per program
	MaxTargets = 24 // entities per register
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
}

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
		}
		b.WriteByte('\n')
	}
	return b.String()
}

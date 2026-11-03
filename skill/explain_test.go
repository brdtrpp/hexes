package skill

import (
	"strings"
	"testing"
)

func useHex(parts ...Part) *Hex {
	h := NewHex("t", [RingSlots]SocketKind{SocketTrigger, SocketWild, SocketWild, SocketWild, SocketWild, SocketWild})
	for i, p := range parts {
		if p.Kind != KindNone {
			h.Slots[i] = Slot{Kind: SlotPart, Part: p}
		}
	}
	return h
}

func wantUse(t *testing.T, h *Hex, s int, ok bool, has string) {
	t.Helper()
	what, got := h.Use(s)
	if got != ok || !strings.Contains(what, has) {
		t.Errorf("slot %d: %q ok=%v, want ok=%v containing %q", s+1, what, got, ok, has)
	}
}

func TestUseFollowsTheOrder(t *testing.T) {
	plasma := reg.MustID("plasma")
	none := Part{}
	h := useHex(NewTrigger(OnCast), NewTarget(TargetChain, 4, 3), NewDamage(plasma, 30), NewMult(VerbDamage, 2))
	wantUse(t, h, 0, true, "fires")
	wantUse(t, h, 1, true, "for slot 3")
	wantUse(t, h, 2, true, "slot 2's targets")
	wantUse(t, h, 3, true, "boosts slot 3")

	// A damage booster after a target, a count booster after damage.
	h = useHex(NewTrigger(OnCast), NewTarget(TargetChain, 4, 3), NewMult(VerbDamage, 2), NewDamage(plasma, 10), NewMult(VerbCount, 2))
	wantUse(t, h, 2, false, "damage booster does nothing to a chain target")
	wantUse(t, h, 4, false, "count booster does nothing to damage")
	// A count booster after a chain target works.
	h.Slots[2].Part = NewMult(VerbCount, 2)
	wantUse(t, h, 2, true, "boosts slot 2")
	// A booster straight after the trigger, and one after an empty slot.
	h = useHex(NewTrigger(OnCast), NewMult(VerbAll, 2), none, NewMult(VerbAll, 2))
	wantUse(t, h, 1, false, "trigger")
	wantUse(t, h, 3, true, "what goes in slot 3")
	// Stacked boosters reach past each other.
	h = useHex(NewTrigger(OnCast), NewDamage(plasma, 10), NewMult(VerbDamage, 2), NewMult(VerbDamage, 2))
	wantUse(t, h, 3, true, "boosts slot 2")

	// An effect with no target beside it falls back to the trigger's own
	// target, which depends on the trigger.
	h = useHex(NewTrigger(OnHit), NewPush(2))
	wantUse(t, h, 1, true, "the foe you hit")
	// A target walled in by boosters is wasted; with an empty neighbor
	// it's waiting for an effect.
	h = useHex(NewTrigger(OnCast), NewMult(VerbAll, 2), NewTarget(TargetAdjacent, 2, 0), NewMult(VerbAll, 2))
	wantUse(t, h, 2, false, "nothing next to it uses targets")
	h.Slots[3] = Slot{}
	wantUse(t, h, 2, true, "put an effect")

	// Gates: route into the next slots, and need one.
	h = useHex(NewTrigger(OnHit), NewPush(2), NewGate(GateHull, 0.4), NewDamage(plasma, 18), NewDamage(plasma, 6))
	wantUse(t, h, 2, true, "splits whoever slot 2 reached")
	wantUse(t, h, 3, true, "whoever passes slot 3")
	wantUse(t, h, 4, true, "whoever fails slot 3")
	h = useHex(NewTrigger(OnCast), none, none, none, none, NewGate(GateHull, 0.4))
	wantUse(t, h, 5, false, "nothing after it")

	// Hub center pools the ring; a Loop's last target loops.
	h = useHex(NewTrigger(OnCast), NewTarget(TargetAdjacent, 2, 0), none, NewTarget(TargetNearest, 0, 2))
	h.Shape = Shape{Kind: ShapeHub}
	h.Slots[Center] = Slot{Kind: SlotPart, Part: NewDamage(plasma, 10)}
	wantUse(t, h, Center, true, "everyone the ring targets")
	wantUse(t, h, 1, true, "for slot C")
	h = useHex(NewTrigger(OnCast), none, none, none, none, NewTarget(TargetNearest, 0, 1))
	h.Shape = Shape{Kind: ShapeLoop}
	h.Slots[4] = Slot{Kind: SlotPart, Part: NewMult(VerbAll, 2)}
	wantUse(t, h, 5, true, "loops")
	wantUse(t, h, 3, true, "")
	wantUse(t, h, 6, false, "outside")
}

package game

import (
	"testing"

	"hexes/skill"
)

func TestLoadoutCompiles(t *testing.T) {
	reg := skill.DefaultRegistry()
	rig := skill.NewRig(skill.Points{Capacity: RigPoints, Recharge: RigPoints, Processing: 5})
	for _, lvl := range []int{1, 10, 30} {
		l := BuildLoadout(reg, lvl, rig.MaxDepth)
		want := []skill.TriggerKind{skill.OnCast, skill.OnHit, skill.OnKill, skill.Periodic}
		for i, p := range l.Progs {
			if p.Trigger != want[i] {
				t.Fatalf("slot %d trigger %s", i, p.Trigger)
			}
			if lvl == 1 {
				t.Logf("%-13s power %5.1f heat %5.1f\n%s", l.Names[i], p.Power, p.Heat, p)
			}
		}
		if l.Strike.Power != 0 || l.Strike.Heat != 0 {
			t.Fatal("strike should be free")
		}
	}
	t.Logf("rig: max power %.0f recharge %.1f/s cool %.1f/s", rig.MaxPower, rig.RechargeRate, rig.CoolRate)
}

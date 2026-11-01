package game

import (
	"testing"

	"hexes/dungeon"
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

// The preset Kinetic Ram gates on hull: a weak foe takes the heavy jab.
func TestKineticRamGatesOnHull(t *testing.T) {
	dealt := func(hull float32) float32 {
		s := New(5)
		p := s.P()
		for i := range s.Actors {
			if skill.EntityID(i) != s.Player {
				s.Actors[i].U.Alive = false
				s.occ[s.Floor.Index(s.Actors[i].At)] = skill.NoEntity
			}
		}
		// Open floor all around so the shove hits nothing.
		for q := -5; q <= 5; q++ {
			for r := -5; r <= 5; r++ {
				s.Floor.Set(p.At.Add(skill.Coord{Q: q, R: r}), dungeon.Ground)
			}
		}
		foe := s.spawn(KindGrunt, 1, p.At.Add(skill.HexDirs[0]), 1, 1000)
		s.Actors[foe].U.HP = 1000 * hull
		before := s.Actors[foe].U.HP
		s.Eval.Run(s.Load.Progs[1], s, skill.Ctx{Caster: s.Player, Subject: foe, Level: 1})
		return before - s.Actors[foe].U.HP
	}
	weak, healthy := dealt(0.3), dealt(1)
	if weak < 2.5*healthy || healthy <= 0 {
		t.Fatalf("weak foe took %.1f, healthy %.1f", weak, healthy)
	}
}

package game

import (
	"testing"

	"hexes/skill"
)

// drones counts the live drones and returns the first.
func drones(s *Sim) (int, *Actor) {
	n, first := 0, (*Actor)(nil)
	for i := range s.Actors {
		if a := &s.Actors[i]; a.U.Alive && a.Kind == KindDevice && a.Dev == skill.FormDrone {
			n++
			if first == nil {
				first = a
			}
		}
	}
	return n, first
}

func launch(t *testing.T, s *Sim, rule uint8, seed uint32) *Actor {
	t.Helper()
	if !s.Deploy(skill.Deployment{Caster: s.Player, At: skill.NoEntity, Form: skill.FormDrone, Rule: rule,
		Seed: seed, Rarity: skill.Rarity(rule + 1)}) {
		t.Fatal("drone didn't deploy")
	}
	_, a := drones(s)
	return a
}

func step(s *Sim, secs float64) {
	for k := 0; k < int(secs*60); k++ {
		s.Step(1.0 / 60)
	}
}

// The Wasp's drone hunts the foe with its own cryo gun: it flies off to
// circle it, and the Wasp hex itself does nothing but launch it.
func TestDroneHuntsWithItsOwnGun(t *testing.T) {
	s := classSim(t, "Engineer")
	f := foeAt(s, 0, 4, 1e5)
	s.refreshSight()
	hp := s.Actors[f].U.HP
	cool(s)
	if o := s.Disp.Cast(s.Player, 2, f); o != skill.Fired {
		t.Fatalf("wasp %s", o)
	}
	s.Step(1.0 / 60)
	if s.Actors[f].U.HP != hp {
		t.Fatal("the hex that launched the drone hit the foe itself")
	}
	if n, _ := drones(s); n != 1 {
		t.Fatalf("%d drones", n)
	}
	step(s, 2)
	_, a := drones(s)
	if a.Prey != f {
		t.Fatalf("drone hunts %d, want %d", a.Prey, f)
	}
	if d := s.distF(a, &s.Actors[f]); d > DroneOrbit+0.6 {
		t.Fatalf("drone %.1f hexes from its quarry, want to circle it", d)
	}
	if s.Actors[f].U.HP >= hp {
		t.Fatal("drone's gun never hit")
	}
	if s.Actors[f].LastHit != s.Reg.MustID("cryo") {
		t.Fatalf("drone hit with %s, want its cryo gun", s.Reg.Get(s.Actors[f].LastHit).Name)
	}
	// It goes round: a second later it's somewhere else on the circle.
	x, y := a.Pos()
	step(s, 1)
	if nx, ny := a.Pos(); (nx-x)*(nx-x)+(ny-y)*(ny-y) < 0.5 {
		t.Fatal("drone isn't circling")
	}
}

// A rare drone goes for the foe you aren't hitting.
func TestRareDroneTakesAnotherFoe(t *testing.T) {
	s := classSim(t, "Engineer")
	f := foeAt(s, 0, 3, 1e5)
	g := foeAt(s, 3, 3, 1e5)
	s.refreshSight()
	a := launch(t, s, 1, 9)
	s.lastHit, s.lastHitAt = f, s.Time
	s.Step(1.0 / 60)
	if a.Prey != g {
		t.Fatalf("rare drone hunts %d, want %d (you're hitting %d)", a.Prey, g, f)
	}
	// With you hitting nothing, it takes the most hurt.
	s.lastHitAt = -100
	s.Actors[f].U.HP = s.Actors[f].U.MaxHP / 3
	s.Step(1.0 / 60)
	if a.Prey != f {
		t.Fatalf("rare drone hunts %d, want the hurt one %d", a.Prey, f)
	}
}

// An exotic drone guards you: it stays near you, takes what closes in
// over what's far off, and a gunner lining up a shot over both.
func TestExoticDroneGuards(t *testing.T) {
	s := classSim(t, "Engineer")
	far := foeAt(s, 0, 3, 1e5)
	near := foeAt(s, 3, 1, 1e5)
	s.refreshSight()
	a := launch(t, s, 2, 9)
	i := int(a.U.ID)
	s.drone(i, 0)
	if a.Prey != near {
		t.Fatalf("exotic drone hunts %d, want the nearest closing in %d", a.Prey, near)
	}
	s.Actors[far].Aim = 1
	s.drone(i, 0)
	if a.Prey != far {
		t.Fatalf("exotic drone hunts %d, want the gunner aiming at you %d", a.Prey, far)
	}
	s.Actors[far].Aim = 0
	step(s, 1)
	if d := s.distF(a, s.P()); d > DroneGuard+0.6 {
		t.Fatalf("exotic drone %.1f hexes from you, want it guarding", d)
	}
}

// A drone part from an old save gets a gun of its own on load, and keeps
// it after.
func TestOldDronePartsGetGuns(t *testing.T) {
	s := classSim(t, "Engineer")
	sv := s.Snapshot()
	sv.Inv.Parts[0] = skill.NewForm(skill.FormDrone)
	sv.Inv.Parts[1] = skill.NewForm(skill.FormDrone)
	if !selfOnly(s.Reg, &sv) {
		t.Fatal("old drones not filled in")
	}
	a, b2 := sv.Inv.Parts[0].Seed, sv.Inv.Parts[1].Seed
	if a == 0 || b2 == 0 || a == b2 {
		t.Fatalf("seeds %d, %d", a, b2)
	}
	if selfOnly(s.Reg, &sv) || sv.Inv.Parts[0].Seed != a {
		t.Fatal("a drone's gun changed on a second load")
	}
}

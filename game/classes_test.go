package game

import (
	"testing"

	"hexes/skill"
)

// classSim loads the named example on a fresh veteran run with every foe
// cleared away, so a test can place what it needs.
func classSim(t *testing.T, name string) *Sim {
	t.Helper()
	s := NewVeteran(11)
	if err := s.SetTestShapes(true); err != nil {
		t.Fatal(err)
	}
	i := exampleNamed(Examples(s.Reg), name)
	if i < 0 {
		t.Fatalf("no example %q", name)
	}
	if err := s.LoadExample(i); err != nil {
		t.Fatal(err)
	}
	ring(t, s)
	for k := range s.Actors {
		if skill.EntityID(k) != s.Player {
			s.Actors[k].U.Alive = false
			s.Actors[k].DeadFor = WreckLife // old bodies, not wrecks
			s.occ[s.Floor.Index(s.Actors[k].At)] = skill.NoEntity
		}
	}
	p := s.P()
	p.U.MaxHP, p.U.HP = 1e6, 1e6
	return s
}

// foeAt puts a grunt n steps from the player in direction d.
func foeAt(s *Sim, d, n int, hp float32) skill.EntityID {
	c := s.P().At
	for k := 0; k < n; k++ {
		c = c.Add(skill.HexDirs[d])
	}
	id := s.spawn(KindGrunt, 1, c, 1, hp)
	s.Actors[id].Awake = true
	return id
}

func cool(s *Sim) { s.Rig.Heat, s.Rig.Power, s.Rig.ShutdownLeft = 0, s.Rig.MaxPower, 0 }

func TestRogueKit(t *testing.T) {
	s := classSim(t, "Rogue")
	f := foeAt(s, 0, 3, 1e5)
	s.refreshSight()
	if o := s.Disp.Cast(s.Player, 0, f); o != skill.Fired {
		t.Fatalf("vanish %s", o)
	}
	if s.P().U.Stealth <= 0 || s.P().U.HasteLeft <= 0 {
		t.Fatalf("vanish left stealth %v haste %v", s.P().U.Stealth, s.P().U.HasteLeft)
	}
	// Out of stealth the backstab lands at AmbushMult and stealth breaks.
	cool(s)
	hp := s.Actors[f].U.HP
	start := s.P().At
	if o := s.Disp.Cast(s.Player, 1, f); o != skill.Fired {
		t.Fatalf("backstab %s", o)
	}
	if s.Actors[f].U.HP >= hp {
		t.Fatal("backstab missed")
	}
	if s.P().U.Stealth != 0 {
		t.Fatal("stealth survived the hit")
	}
	if s.P().At != start {
		t.Fatalf("rare dash didn't come back: %v, started %v", s.P().At, start)
	}
	// The combo pays out on the third hit.
	cool(s)
	for k := 0; k < 3; k++ {
		s.Disp.Emit(skill.Event{Trigger: skill.OnHit, Actor: s.Player, Subject: f, Chain: 1, Slot: -1})
		s.Disp.Update(2) // past the proc cooldown
	}
	toxic := false
	for _, d := range s.Actors[f].U.Dots {
		if d.Left > 0 && s.Reg.Get(d.Type).Name == "nanite" {
			toxic = true
		}
	}
	if !toxic {
		t.Fatal("combo never corroded")
	}
}

func TestNecromancerKit(t *testing.T) {
	s := classSim(t, "Necromancer")
	// No wreck: the pact won't fire and Corpse Blast finds nothing.
	f := foeAt(s, 0, 2, 40)
	s.refreshSight()
	if o := s.Disp.Cast(s.Player, 3, f); o != skill.NoFuel {
		t.Fatalf("pact without a wreck: %s", o)
	}
	// Kill it: Raise gets it up as a minion.
	cool(s)
	big := foeAt(s, 2, 2, 1e5)
	s.refreshSight()
	s.Actors[f].U.HP = 1
	s.Disp.Cast(s.Player, 3, f) // still no fuel
	dealt, _ := s.Rules.Resolve(&s.Actors[f].U, 100, 0, 1, false)
	if dealt <= 0 || s.Actors[f].U.Alive {
		t.Fatal("couldn't kill the foe")
	}
	s.Killed(s.Player, f)
	s.Disp.Emit(skill.Event{Trigger: skill.OnKill, Actor: s.Player, Subject: f, Chain: 1, Slot: -1})
	if !s.Actors[f].Minion || !s.Actors[f].U.Alive {
		t.Fatal("kill didn't raise a minion")
	}
	// The minion fights.
	hp := s.Actors[big].U.HP
	for k := 0; k < 240; k++ {
		s.Step(1.0 / 60)
	}
	if s.Actors[big].U.HP >= hp {
		t.Fatal("minion never hit")
	}
	// A fresh wreck: the blast uses it up and the pact can pay.
	w := foeAt(s, 4, 2, 10)
	s.Actors[w].U.Alive, s.Actors[w].U.HP = false, 0
	s.died(w)
	cool(s)
	if !s.wreck(int(w)) {
		t.Fatal("fresh body isn't a wreck")
	}
	heat := s.Rig.Heat
	if o := s.Disp.Cast(s.Player, 3, big); o != skill.Fired {
		t.Fatalf("pact with a wreck: %s", o)
	}
	if s.Rig.Heat > heat || s.wreck(int(w)) {
		t.Fatalf("pact heat %v -> %v, wreck left %v", heat, s.Rig.Heat, s.wreck(int(w)))
	}
}

func TestDetonate(t *testing.T) {
	s := classSim(t, "Necromancer")
	w := foeAt(s, 0, 2, 10)
	s.Actors[w].U.Alive = false
	s.died(w)
	f := foeAt(s, 0, 3, 1e5)
	s.refreshSight()
	hp := s.Actors[f].U.HP
	if o := s.Disp.Cast(s.Player, 1, f); o != skill.Fired {
		t.Fatalf("blast %s", o)
	}
	if s.Actors[f].U.HP >= hp || s.wreck(int(w)) {
		t.Fatalf("blast: foe hp %v -> %v, wreck left %v", hp, s.Actors[f].U.HP, s.wreck(int(w)))
	}
}

func TestEngineerDevices(t *testing.T) {
	s := classSim(t, "Engineer")
	f := foeAt(s, 0, 3, 1e5)
	g := foeAt(s, 3, 1, 1e5)
	s.refreshSight()
	for slot := 0; slot < 3; slot++ {
		cool(s)
		if o := s.Disp.Cast(s.Player, slot, f); o != skill.Fired {
			t.Fatalf("slot %d: %s", slot, o)
		}
	}
	forms := map[skill.FormKind]int{}
	for i := range s.Actors {
		if a := &s.Actors[i]; a.Kind == KindDevice && a.U.Alive {
			forms[a.Dev]++
		}
	}
	if forms[skill.FormTurret] != 1 || forms[skill.FormTrap] != 1 || forms[skill.FormDrone] != 1 {
		t.Fatalf("devices %v", forms)
	}
	hp, hg := s.Actors[f].U.HP, s.Actors[g].U.HP
	for k := 0; k < 120; k++ {
		s.Step(1.0 / 60)
	}
	if s.Actors[f].U.HP >= hp {
		t.Fatal("turret and drone never hit")
	}
	if s.Actors[g].U.HP >= hg || s.Actors[g].U.StunLeft <= 0 && s.Actors[g].U.SlowLeft <= 0 && !s.Actors[g].U.Frozen {
		t.Fatalf("trap didn't go off on the foe beside it: hp %v -> %v", hg, s.Actors[g].U.HP)
	}
	// They run out.
	for k := 0; k < 60*30; k++ {
		s.Step(1.0 / 60)
	}
	for i := range s.Actors {
		if a := &s.Actors[i]; a.Kind == KindDevice && a.U.Alive && a.Dev != skill.FormTrap {
			t.Fatalf("%s outlived its life", a.Dev)
		}
	}
}

func TestWardenKit(t *testing.T) {
	s := classSim(t, "Warden")
	f := foeAt(s, 0, 1, 1e5)
	s.refreshSight()
	if o := s.Disp.Cast(s.Player, 0, f); o != skill.Fired {
		t.Fatalf("challenge %s", o)
	}
	if s.Actors[f].U.TauntLeft <= 0 || s.P().U.ThornsLeft <= 0 {
		t.Fatalf("taunt %v thorns %v", s.Actors[f].U.TauntLeft, s.P().U.ThornsLeft)
	}
	hp := s.Actors[f].U.HP
	s.claw(f, 40)
	if s.Actors[f].U.HP >= hp {
		t.Fatal("thorns didn't hit back")
	}
	// The stance comes on, guards, and a second tap drops it.
	cool(s)
	if o := s.Disp.Cast(s.Player, 1, f); o != skill.Fired {
		t.Fatalf("stance %s", o)
	}
	s.stances()
	if s.Stance&(1<<skill.StanceBulwark) == 0 || s.guard() >= 1 {
		t.Fatalf("stance off after casting: %b", s.Stance)
	}
	if o := s.Disp.Cast(s.Player, 1, f); o != skill.Off {
		t.Fatalf("second tap %s", o)
	}
	s.stances()
	if s.Stance != 0 {
		t.Fatal("stance stayed on")
	}
	// Blood: no Heat, paid in hull.
	cool(s)
	p := s.P()
	before := p.U.HP
	if o := s.Disp.Cast(s.Player, 2, f); o != skill.Fired {
		t.Fatalf("blood price %s", o)
	}
	if s.Rig.Heat != 0 || p.U.HP >= before {
		t.Fatalf("blood price heat %v hull %v -> %v", s.Rig.Heat, before, p.U.HP)
	}
}

// Devices and minions standing and running keep the step allocation-free.
func TestDevicesStepAllocFree(t *testing.T) {
	s := classSim(t, "Engineer")
	for d := 0; d < 6; d++ {
		foeAt(s, d, 3, 1e9)
	}
	s.refreshSight()
	for slot := 0; slot < 3; slot++ {
		cool(s)
		s.Disp.Cast(s.Player, slot, skill.NoEntity)
	}
	k := 0
	allocs := testing.AllocsPerRun(300, func() {
		k++
		if k%90 == 0 {
			cool(s)
			s.Disp.Cast(s.Player, k/90%3, skill.NoEntity)
		}
		s.Step(1.0 / 60)
	})
	if allocs != 0 {
		t.Fatalf("%.2f allocs per step with devices up", allocs)
	}
}

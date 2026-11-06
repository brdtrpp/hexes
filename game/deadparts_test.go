package game

import (
	"testing"

	"hexes/skill"
)

// only equips p alone, so nothing else in the kit fires.
func only(s *Sim, p *skill.Program) {
	s.Load.Progs = [len(s.Load.Progs)]*skill.Program{p}
	l := s.Disp.Equip(s.Player, &s.Rig, s.Load.Progs[:]...)
	l.Banks, l.Board = &s.Banks, &s.BoardBanks
}

func prog(t *testing.T, s *Sim, parts ...skill.Part) *skill.Program {
	t.Helper()
	h := skill.NewHex("t", [6]skill.SocketKind{skill.SocketTrigger, skill.SocketWild, skill.SocketWild,
		skill.SocketWild, skill.SocketWild, skill.SocketWild})
	for i, p := range parts {
		h.Slots[i] = skill.Slot{Kind: skill.SlotPart, Part: p}
	}
	p, err := skill.Compile(h, s.Reg, 4)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// An untargeted on-kill hex lands on the nearest live foe, not the body.
func TestOnKillLandsOnTheLiving(t *testing.T) {
	s := classSim(t, "Necromancer")
	only(s, prog(t, s, skill.NewTrigger(skill.OnKill), skill.NewDamage(s.Reg.MustID("plasma"), 30)))
	dead := foeAt(s, 0, 2, 10)
	near := foeAt(s, 0, 3, 1e5)
	s.refreshSight()
	s.Actors[dead].U.Alive, s.Actors[dead].U.HP = false, 0
	s.died(dead)
	hp := s.Actors[near].U.HP
	s.Disp.Emit(skill.Event{Trigger: skill.OnKill, Actor: s.Player, Subject: dead, Chain: 1, Slot: -1})
	if s.Actors[near].U.HP >= hp {
		t.Fatal("on-kill damage hit nobody")
	}
}

// A wreck target hits the foes standing around the wreck.
func TestWreckTargetHitsAround(t *testing.T) {
	s := classSim(t, "Necromancer")
	only(s, prog(t, s, skill.NewTrigger(skill.OnCast),
		skill.NewTarget(skill.TargetWreck, skill.WreckReach, 1), skill.NewDamage(s.Reg.MustID("plasma"), 30)))
	w := foeAt(s, 0, 3, 10)
	by := foeAt(s, 0, 4, 1e5)
	s.refreshSight()
	s.Actors[w].U.Alive, s.Actors[w].U.HP = false, 0
	s.died(w)
	cool(s)
	hp := s.Actors[by].U.HP
	if o := s.Disp.Cast(s.Player, 0, by); o != skill.Fired {
		t.Fatalf("cast %s", o)
	}
	if s.Actors[by].U.HP >= hp {
		t.Fatal("wreck target hit nobody")
	}
}

// Venting fires the on-vent hexes, though the vent locks the rig.
func TestOnVentFires(t *testing.T) {
	s := classSim(t, "Necromancer")
	only(s, prog(t, s, skill.NewTrigger(skill.OnVent), skill.NewForm(skill.FormNova),
		skill.NewDamage(s.Reg.MustID("plasma"), 30)))
	f := foeAt(s, 0, 1, 1e5)
	s.refreshSight()
	s.Rig.Heat = 50
	hp := s.Actors[f].U.HP
	s.Input.Vent = true
	s.Step(1.0 / 60)
	s.Input.Vent = false
	if !s.Rig.Locked() {
		t.Fatal("vent didn't lock")
	}
	if s.Disp.Stats.Fired == 0 || s.Actors[f].U.HP >= hp {
		t.Fatalf("on-vent never fired: %+v", s.Disp.Stats)
	}
}

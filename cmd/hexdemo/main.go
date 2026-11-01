// Command hexdemo learns the same hex with two seeds, slots the same parts
// into each, and shows how the rolled layouts compile and hit differently.
//
//	go run ./cmd/hexdemo
package main

import (
	"fmt"
	"log"

	"hexes/skill"
)

func main() {
	reg := skill.DefaultRegistry()
	tmpl := skill.Template{
		Name: "Frost Lattice",
		Sockets: [6]skill.SocketKind{
			skill.SocketTrigger, skill.SocketTarget, skill.SocketEffect,
			skill.SocketEffect, skill.SocketEffect, skill.SocketWild,
		},
		Broken: 1,
	}
	parts := []skill.Part{
		skill.NewTrigger(skill.OnCast),
		skill.NewTarget(skill.TargetNearest, 0, 1),
		skill.NewStatus(reg.MustID("cryo"), 2),
		skill.NewStatus(reg.MustID("cryo"), 2),
		skill.NewDamage(reg.MustID("cryo"), 40),
		skill.NewMult(skill.VerbDamage, 2).WithRarity(skill.Rare),
	}
	rig := skill.NewRig(skill.Points{Capacity: 5, Processing: 5})

	for _, seed := range []uint64{7, 1234} {
		h := skill.Learn(tmpl, seed)
		place(h, parts)
		p, err := skill.Compile(h, reg, rig.MaxDepth)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("seed %d  sockets %v  linked %v\n", seed, h.Sockets, h.Linked)
		fmt.Print(p)

		a := skill.NewArena(12, 12, reg, skill.DefaultRules())
		c := a.Spawn(0, skill.Coord{Q: 2, R: 6}, 10, 1000)
		foe := a.Spawn(1, skill.Coord{Q: 4, R: 6}, 10, 1000)
		r := rig
		if !r.TryFire(p) {
			log.Fatal("rig can't fire")
		}
		res := skill.NewEvaluator(reg, skill.DefaultRules()).Run(p, a, skill.Ctx{Caster: c, Subject: skill.NoEntity, Level: 10})
		fmt.Printf("  -> %+v  foe hp %.0f  rig power %.0f heat %.0f\n\n", res, a.Unit(foe).HP, r.Power, r.Heat)
	}
}

// place puts each part in the first empty socket that takes it, the way a
// player dragging loot onto a hex would fill it.
func place(h *skill.Hex, parts []skill.Part) {
	for _, p := range parts {
		for i := range h.Slots {
			if h.Slots[i].Kind == skill.SlotEmpty && h.Sockets[i].Accepts(p.Kind) {
				if err := h.Set(i, p); err != nil {
					log.Fatal(err)
				}
				break
			}
		}
	}
}

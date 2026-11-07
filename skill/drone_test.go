package skill

import (
	"strings"
	"testing"
)

func TestDroneKitsRollEveryGun(t *testing.T) {
	var guns [NumGuns]int
	types := map[string]int{}
	for s := uint32(1); s < 2000; s++ {
		k := DroneKitOf(DroneSeed(uint64(s)), Common)
		guns[k.Gun]++
		types[k.Type]++
		if k.Damage <= 0 || k.Text() == "" {
			t.Fatalf("seed %d: %+v", s, k)
		}
		if strings.ContainsAny(k.Text(), "\u2014\u2015") {
			t.Fatalf("seed %d text has a long dash", s)
		}
	}
	for g, n := range guns {
		if n == 0 {
			t.Fatalf("gun %s never rolls", DroneGun(g))
		}
	}
	if types["cryo"] <= types["kinetic"] || types["kinetic"] <= types["plasma"] || types["plasma"] == 0 {
		t.Fatalf("type odds off: %v", types)
	}
}

func TestDroneSeedIsNeverZero(t *testing.T) {
	for x := uint64(0); x < 5000; x++ {
		if s := DroneSeed(x); s == 0 || s >= 1<<24 {
			t.Fatalf("DroneSeed(%d) = %d", x, s)
		}
	}
}

func TestDroneKitGrowsWithRarity(t *testing.T) {
	for s := uint32(1); s < 50; s++ {
		c, e := DroneKitOf(s, Common), DroneKitOf(s, Exotic)
		if c.Gun != e.Gun || c.Type != e.Type || c.Rider != e.Rider || e.Damage <= c.Damage {
			t.Fatalf("seed %d: common %+v, exotic %+v", s, c, e)
		}
	}
}

// A drone keeps its seed and rarity through the compile, and the rest of
// the hex runs from you, not from the drone.
func TestDroneDeployCarriesItsGun(t *testing.T) {
	d := NewForm(FormDrone).WithRarity(Rare)
	d.Seed = 4242
	p := mk(t, "wasp", NewTrigger(OnCast), d, NewDamage(id("cryo"), 10))
	for _, s := range p.Steps {
		if s.Op == OpDeploy {
			if s.Span != 0 || uint32(s.Value) != 4242 || Rarity(s.Dst) != Rare {
				t.Fatalf("deploy step %+v", s)
			}
			return
		}
	}
	t.Fatal("no deploy step")
}

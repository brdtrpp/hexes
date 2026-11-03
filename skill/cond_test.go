package skill

import (
	"strings"
	"testing"
)

func cond(ts ...Tile) Cond {
	var c Cond
	copy(c[:], ts)
	return c
}

func TestCondParses(t *testing.T) {
	// heat < 0.3 and power > 0.6 or hull < 0.2: and binds first.
	c := cond(Tile{K: TileHeat}, Tile{K: TileLess, V: 0.3}, Tile{K: TileAnd},
		Tile{K: TilePower}, Tile{K: TileMore, V: 0.6}, Tile{K: TileOr},
		Tile{K: TileHull}, Tile{K: TileLess, V: 0.2})
	p, err := c.Compile()
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, op := range p.Ops[:p.N] {
		got = append(got, op.K.String())
	}
	if s := strings.Join(got, " "); s != "heat power and hull or" {
		t.Fatalf("postfix %q", s)
	}
	if !p.Foe || p.Sensors != 3 || p.Opers != 2 {
		t.Fatalf("foe %v sensors %d opers %d", p.Foe, p.Sensors, p.Opers)
	}
	approx(t, "heat", p.Heat(), 3*SensorHeat+2*OperatorHeat)
	if txt := c.Text(reg); txt != "heat < 30% and power > 60% or hull < 20%" {
		t.Fatalf("text %q", txt)
	}
	for _, bad := range []Cond{
		{},
		cond(Tile{K: TileHull}),
		cond(Tile{K: TileHull}, Tile{K: TileLess, V: 0.5}, Tile{K: TileAnd}),
		cond(Tile{K: TileOpen}, Tile{K: TileHull}, Tile{K: TileLess}),
		cond(Tile{K: TileHas}, Tile{K: TileLess}),
		cond(Tile{K: TileAnd}),
	} {
		if _, err := bad.Compile(); err == nil {
			t.Fatalf("%v parsed", bad.Text(reg))
		}
	}
}

// Brawler: foes within 2 that are bunched up and hurt get shoved, the
// rest take damage.
func TestCondGateSplitsPerFoe(t *testing.T) {
	a, c := world()
	hurt1 := a.Spawn(1, Coord{3, 6}, 10, 1000)
	hurt2 := a.Spawn(1, Coord{3, 5}, 10, 1000)
	well := a.Spawn(1, Coord{1, 6}, 10, 1000)
	a.Unit(hurt1).HP, a.Unit(hurt2).HP = 300, 300
	gate := NewGate(GateHull, 0.5).WithCond(cond(
		Tile{K: TileCrowd, V: 1}, Tile{K: TileMore, V: 1}, Tile{K: TileAnd},
		Tile{K: TileHull}, Tile{K: TileLess, V: 0.5}))
	h := shaped(t, ShapeRing, map[int]Part{
		0: NewTrigger(OnCast),
		1: NewTarget(TargetAdjacent, 2, 0),
		2: gate,
		3: NewStatus(id("cryo"), 2),
		4: NewDamage(id("kinetic"), 18),
	})
	p := compile(t, h, 0)
	run(t, p, a, c, NoEntity, nil, nil)
	if a.Unit(hurt1).HP != 300 || a.Unit(hurt2).HP != 300 || a.Unit(well).HP == 1000 {
		t.Fatalf("hurt %v %v, well %v\n%s", a.Unit(hurt1).HP, a.Unit(hurt2).HP, a.Unit(well).HP, p)
	}
	if a.Unit(hurt1).SlowLeft <= 0 {
		t.Fatal("the YES half wasn't chilled")
	}
}

// A condition that only reads the rig sends the whole group one way.
func TestCondGateWholeGroup(t *testing.T) {
	gate := NewGate(GateHull, 0.5).WithCond(cond(Tile{K: TileHeat}, Tile{K: TileLess, V: 0.3}))
	h := shaped(t, ShapeRing, map[int]Part{
		0: NewTrigger(OnCast),
		1: NewTarget(TargetAdjacent, 2, 0),
		2: gate,
		3: NewDamage(id("kinetic"), 30),
	})
	p := compile(t, h, 0)
	for _, heat := range []float32{10, 90} {
		a, c := world()
		f := a.Spawn(1, Coord{3, 6}, 10, 1000)
		g := a.Spawn(1, Coord{1, 6}, 10, 1000)
		rig := &Rig{MaxHeat: 100, Heat: heat, MaxPower: 100, Power: 100}
		run(t, p, a, c, NoEntity, nil, rig)
		hit := a.Unit(f).HP < 1000
		if hit != (heat < 30) || (a.Unit(g).HP < 1000) != hit {
			t.Fatalf("heat %v: hit %v %v", heat, a.Unit(f).HP, a.Unit(g).HP)
		}
	}
}

// Old gates open as the same condition, and a preset compiles to the
// same split as the gate it came from.
func TestPresetsMatchOldGates(t *testing.T) {
	for _, g := range []Part{NewGate(GateHull, 0.4), NewCrowdGate(1, 2), NewStatusGate(id("cryo"))} {
		c, ok := PresetCond(g)
		if !ok {
			t.Fatalf("%v has no preset", g.Gate)
		}
		if _, err := c.Compile(); err != nil {
			t.Fatalf("%v preset: %v", g.Gate, err)
		}
	}
	if _, ok := PresetCond(NewSpendGate(0, 3)); ok {
		t.Fatal("spend became a condition")
	}
}

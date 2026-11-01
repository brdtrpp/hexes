package skill

import "testing"

// Three hexes of growing size, all built from parts a player could slot.

func simpleHex(tb testing.TB) *Hex {
	h := NewHex("simple", [6]SocketKind{T, G, E, E, W, G})
	h.Slots[0] = Slot{Kind: SlotPart, Part: NewTrigger(OnCast)}
	h.Slots[1] = Slot{Kind: SlotPart, Part: NewTarget(TargetNearest, 0, 1)}
	h.Slots[2] = Slot{Kind: SlotPart, Part: NewDamage(id("cryo"), 20)}
	h.Slots[3] = Slot{Kind: SlotPart, Part: NewMult(VerbDamage, 1.5)}
	return h
}

func nestedHex(tb testing.TB) *Hex {
	h := targetsTargetHex(tb, id("plasma"), 10)
	h.Slots[3] = Slot{Kind: SlotPart, Part: NewMult(VerbDamage, 2)}
	return h
}

// fullHex: chain three foes, then for each, burst everything adjacent to
// it, and for each of those, chill the nearest foe to it. Depth 2.
func fullHex(tb testing.TB) *Hex {
	inner2 := NewHex("chill nearest", [6]SocketKind{G, E, E, E, E, E})
	inner2.Slots[0] = Slot{Kind: SlotPart, Part: NewTarget(TargetNearest, 0, 1)}
	inner2.Slots[1] = Slot{Kind: SlotPart, Part: NewStatus(id("cryo"), 2)}
	inner2.Slots[2] = Slot{Kind: SlotPart, Part: NewMult(VerbDuration, 1.5)}

	inner1 := NewHex("burst", [6]SocketKind{G, E, W, E, E, E})
	inner1.Slots[0] = Slot{Kind: SlotPart, Part: NewTarget(TargetAdjacent, 1, 0)}
	inner1.Slots[1] = Slot{Kind: SlotPart, Part: NewDamage(id("plasma"), 10)}
	_ = inner1.Nest(2, inner2)
	inner1.Slots[3] = Slot{Kind: SlotPart, Part: NewDamage(id("nanite"), 30)}

	h := NewHex("full", [6]SocketKind{T, G, E, G, W, E})
	h.Slots[0] = Slot{Kind: SlotPart, Part: NewTrigger(OnCast)}
	h.Slots[1] = Slot{Kind: SlotPart, Part: NewTarget(TargetChain, 3, 3)}
	h.Slots[2] = Slot{Kind: SlotPart, Part: NewDamage(id("cryo"), 15)}
	h.Slots[3] = Slot{Kind: SlotPart, Part: NewTarget(TargetNearest, 0, 2)}
	_ = h.Nest(4, inner1)
	h.Slots[5] = Slot{Kind: SlotPart, Part: NewMult(VerbAll, 1.25)}
	return h
}

var benchHexes = []struct {
	name  string
	build func(testing.TB) *Hex
}{
	{"simple", simpleHex},
	{"nested", nestedHex},
	{"full", fullHex},
}

func BenchmarkCompile(b *testing.B) {
	for _, bh := range benchHexes {
		h := bh.build(b)
		b.Run(bh.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := Compile(h, reg, 4); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// benchArena is a crowded room: 30 foes that can't die, so every run does
// the same work.
func benchArena() (*Arena, EntityID) {
	a := NewArena(16, 16, reg, DefaultRules())
	c := a.Spawn(0, Coord{1, 8}, 10, 1e12)
	for i := 0; i < 30; i++ {
		a.Spawn(1, Coord{4 + i%6, 5 + i/6}, 10, 1e12)
	}
	return a, c
}

func BenchmarkEvaluate(b *testing.B) {
	for _, bh := range benchHexes {
		p, err := Compile(bh.build(b), reg, 4)
		if err != nil {
			b.Fatal(err)
		}
		b.Run(bh.name, func(b *testing.B) {
			a, c := benchArena()
			ev := NewEvaluator(reg, DefaultRules())
			ctx := Ctx{Caster: c, Subject: NoEntity, Level: 10}
			res := ev.Run(p, a, ctx)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				ev.Run(p, a, ctx)
			}
			b.ReportMetric(float64(res.Hits+res.Statuses), "effects/run")
		})
	}
}

func BenchmarkLearn(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		Learn(testTemplate, uint64(i))
	}
}

func TestFullHexRuns(t *testing.T) {
	p := compile(t, fullHex(t), 2)
	a, c := benchArena()
	res := NewEvaluator(reg, DefaultRules()).Run(p, a, Ctx{Caster: c, Subject: NoEntity, Level: 10})
	if res.Hits == 0 || res.Statuses == 0 || res.Dots == 0 {
		t.Fatalf("full hex did little: %+v\n%s", res, p)
	}
}

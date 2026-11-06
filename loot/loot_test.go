package loot

import (
	"strings"
	"testing"

	"hexes/skill"
)

var reg = skill.DefaultRegistry()

func TestRollsAreDeterministic(t *testing.T) {
	tab := NewTable(reg)
	for seed := uint64(0); seed < 200; seed++ {
		a, b := NewRNG(seed), NewRNG(seed)
		pa, oka := tab.Drop(&a, 7, 0.5)
		pb, okb := tab.Drop(&b, 7, 0.5)
		if oka != okb || pa != pb {
			t.Fatalf("seed %d rolled %v/%v and %v/%v", seed, pa, oka, pb, okb)
		}
	}
	if Seed(99, 3) == Seed(99, 4) || Seed(99, 3) == Seed(98, 3) {
		t.Fatal("drop seeds collide")
	}
}

func TestRarityShiftsWithDepth(t *testing.T) {
	share := func(depth int) [skill.NumRarities]float64 {
		var n [skill.NumRarities]float64
		r := NewRNG(uint64(depth))
		const N = 20000
		for i := 0; i < N; i++ {
			n[RollRarity(&r, depth)]++
		}
		for i := range n {
			n[i] /= N
		}
		return n
	}
	d1, d10, d40 := share(1), share(10), share(40)
	t.Logf("depth 1  %.3f", d1)
	t.Logf("depth 10 %.3f", d10)
	t.Logf("depth 40 %.3f", d40)
	if d1[skill.Common] < 0.65 || d1[skill.Exotic] > 0.02 {
		t.Fatalf("depth 1 should be mostly common: %v", d1)
	}
	for r := skill.Uncommon; r <= skill.Exotic; r++ {
		if !(d1[r] < d10[r] && d10[r] < d40[r]) {
			t.Fatalf("%s should grow with depth: %.3f %.3f %.3f", r, d1[r], d10[r], d40[r])
		}
	}
	if d40[skill.Common] > 0.3 {
		t.Fatalf("depth 40 still %.2f common", d40[skill.Common])
	}
}

func TestDropChance(t *testing.T) {
	tab := NewTable(reg)
	r := NewRNG(5)
	got := 0
	for i := 0; i < 10000; i++ {
		if _, ok := tab.Drop(&r, 3, 0.3); ok {
			got++
		}
	}
	if got < 2800 || got > 3200 {
		t.Fatalf("%d drops in 10000 at 30%%", got)
	}
	if _, ok := tab.Drop(&r, 3, 0); ok {
		t.Fatal("chance 0 dropped")
	}
}

func TestQuirksOnlyOnRareAndExotic(t *testing.T) {
	tab := NewTable(reg)
	r := NewRNG(11)
	var quirked [skill.NumRarities]int
	var total [skill.NumRarities]int
	for i := 0; i < 4000; i++ {
		rar := skill.Rarity(i % skill.NumRarities)
		p := tab.RollAt(&r, rar)
		total[rar]++
		if p.Quirk != skill.QuirkNone {
			quirked[rar]++
			if !p.Quirk.Fits(p) {
				t.Fatalf("%s carries %s", p, p.Quirk)
			}
		}
	}
	if quirked[skill.Common]+quirked[skill.Uncommon] != 0 {
		t.Fatalf("low tiers bent rules: %v", quirked)
	}
	if quirked[skill.Exotic] != total[skill.Exotic] {
		t.Fatalf("every exotic should bend a rule: %d of %d", quirked[skill.Exotic], total[skill.Exotic])
	}
	if f := float32(quirked[skill.Rare]) / float32(total[skill.Rare]); f < 0.15 || f > 0.5 { // targets, gates, charges and calls never quirk
		t.Fatalf("rare quirk rate %.2f", f)
	}
}

// Every rolled part must slot into the sockets it claims and compile.
func TestRolledPartsCompile(t *testing.T) {
	tab := NewTable(reg)
	r := NewRNG(1)
	var kinds [8]int
	for i := 0; i < 3000; i++ {
		p := tab.Roll(&r, 1+i%50)
		kinds[p.Kind]++
		h := skill.NewHex("x", [6]skill.SocketKind{skill.SocketTrigger, skill.SocketTarget,
			skill.SocketWild, skill.SocketWild, skill.SocketWild, skill.SocketWild})
		trig := skill.NewTrigger(skill.OnCast)
		if p.Kind == skill.KindTrigger {
			trig = p
		}
		_ = h.Set(0, trig)
		_ = h.Set(1, skill.NewTarget(skill.TargetNearest, 0, 1))
		_ = h.Set(2, skill.NewDamage(0, 10))
		if p.Kind != skill.KindTrigger {
			if err := h.Set(3, p); err != nil {
				t.Fatalf("%v: %v", p, err)
			}
		}
		prog, err := skill.Compile(h, reg, 0)
		if err != nil {
			t.Fatalf("%v: %v", p, err)
		}
		if prog.Power <= 0 {
			t.Fatalf("%v costs nothing", p)
		}
		if title, _ := Label(p, reg); title == "" || title == "EMPTY" {
			t.Fatalf("no label for %v", p)
		}
	}
	for k := skill.KindTrigger; k <= skill.KindCall; k++ {
		if kinds[k] < 120 {
			t.Fatalf("too few %s parts: %v", k, kinds)
		}
	}
}

func TestRollDoesNotAllocate(t *testing.T) {
	tab := NewTable(reg)
	r := NewRNG(3)
	n := testing.AllocsPerRun(1000, func() { tab.Drop(&r, 20, 0.8) })
	if n != 0 {
		t.Fatalf("%v allocs per drop", n)
	}
}

func TestInventory(t *testing.T) {
	var v Inventory
	for i := 0; i < InventorySize; i++ {
		if got := v.Add(skill.NewPush(float32(i + 1))); got != i {
			t.Fatalf("add %d went to %d", i, got)
		}
	}
	if !v.Full() || v.Add(skill.NewPush(1)) != -1 {
		t.Fatal("full inventory took a part")
	}
	p, ok := v.Take(4)
	if !ok || p.Value != 5 || v.Has[4] || v.Count() != InventorySize-1 {
		t.Fatal("take")
	}
	if _, ok := v.Take(4); ok {
		t.Fatal("took from an empty cell")
	}
	if v.Add(skill.NewPush(9)) != 4 {
		t.Fatal("add should fill the gap")
	}
}

func BenchmarkRoll(b *testing.B) {
	tab := NewTable(reg)
	r := NewRNG(1)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		tab.Roll(&r, 1+i%60)
	}
}

func TestSustainDropsUncommonAndRare(t *testing.T) {
	tab := NewTable(reg)
	r := NewRNG(42)
	var got [skill.NumRarities][skill.NumEffects]int
	for i := 0; i < 20000; i++ {
		rar := skill.Rarity(i % skill.NumRarities)
		p := tab.RollAt(&r, rar)
		if p.Kind == skill.KindEffect {
			got[rar][p.Effect]++
		}
	}
	for _, e := range []skill.EffectKind{skill.EffectMend, skill.EffectPurge, skill.EffectShield} {
		if got[skill.Common][e]+got[skill.Exotic][e] != 0 {
			t.Fatalf("%s dropped common or exotic", e)
		}
		if got[skill.Uncommon][e] == 0 || got[skill.Rare][e] == 0 {
			t.Fatalf("%s never dropped: %v", e, got)
		}
	}
	for i := 0; i < 2000; i++ {
		p := tab.RollAt(&r, skill.Rare)
		if p.Kind != skill.KindEffect || !p.Effect.Sustain() {
			continue
		}
		title, detail := Label(p, reg)
		if title == "" || detail == "" || !strings.Contains(Describe(p, reg), "Power") && !strings.Contains(Describe(p, reg), "power") {
			t.Fatalf("label %q %q", title, detail)
		}
		if p.Quirk != skill.QuirkNone {
			t.Fatalf("sustain part rolled quirk %v", p.Quirk)
		}
	}
}

// Cloak is a self effect: no roll, rider or class part deals cloak damage.
func TestNoCloakDamageRolls(t *testing.T) {
	tab := NewTable(reg)
	cloak := reg.MustID("cloak")
	bad := func(p skill.Part) bool {
		return p.Kind == skill.KindEffect && p.Effect == skill.EffectDamage && p.Type == cloak
	}
	seenCloak := false
	for seed := uint64(0); seed < 3000; seed++ {
		r := NewRNG(seed)
		for rar := skill.Rarity(0); rar < skill.NumRarities; rar++ {
			for _, p := range []skill.Part{tab.RollAt(&r, rar), tab.Rider(&r, rar), tab.RollNew(&r, rar), tab.Roll(&r, int(seed%40)+1)} {
				if bad(p) {
					t.Fatalf("seed %d rolled cloak damage: %+v", seed, p)
				}
				if p.Kind == skill.KindEffect && p.Effect == skill.EffectStatus && p.Type == cloak {
					seenCloak = true
					if p.Quirk == skill.QuirkRider {
						t.Fatalf("seed %d rolled a cloak rider: %+v", seed, p)
					}
				}
			}
		}
	}
	if !seenCloak {
		t.Fatal("cloak never rolled at all")
	}
	if title, detail := Label(skill.NewStatus(cloak, 3), reg); title != "CLOAK" || !strings.Contains(detail, "self") {
		t.Fatalf("cloak reads %q %q", title, detail)
	}
}

// Every quirk that fits can roll, and a cloak never carries an arc or a
// rider, which would do nothing on a status that only hides you.
func TestQuirksAllRollAndSkipCloak(t *testing.T) {
	tab := NewTable(reg)
	r := NewRNG(5)
	var seen [skill.NumQuirks]int
	for i := 0; i < 6000; i++ {
		p := tab.RollAt(&r, skill.Exotic)
		seen[p.Quirk]++
		if p.Kind == skill.KindEffect && reg.Cloak(p.Type) && (p.Quirk == skill.QuirkRelay || p.Quirk == skill.QuirkRider) {
			t.Fatalf("%s carries %s on a cloak", p, p.Quirk)
		}
	}
	for q := skill.Quirk(1); q < skill.NumQuirks; q++ {
		if seen[q] == 0 {
			t.Fatalf("%s never rolled: %v", q, seen)
		}
	}
}

// The shadow family rolls something that marks, so its marked target and
// marked multiplier have something to work on.
func TestShadowRollsAMark(t *testing.T) {
	tab := NewTable(reg)
	r := NewRNG(3)
	mark, ok := reg.StatusOf(skill.StatusMark)
	if !ok {
		t.Skip("no mark type")
	}
	for i := 0; i < 400; i++ {
		p := tab.RollFam(&r, 1, skill.Rare)
		if p.Kind == skill.KindEffect && p.Effect == skill.EffectStatus && p.Type == mark {
			return
		}
	}
	t.Fatal("shadow family never rolled a mark")
}

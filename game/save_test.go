package game

import (
	"testing"

	"hexes/skill"
)

func kill(s *Sim) {
	p := s.P()
	p.U.HP, p.U.Alive = 0, false
	s.Input = Input{}
	s.Step(1.0 / 60)
}

func TestSaveRoundTrip(t *testing.T) {
	s := NewVeteran(11)
	cryo := skill.NewDamage(s.Reg.MustID("cryo"), 20)
	cell := s.Stash.Inv.Add(cryo)
	if err := s.SwapIn(3, 4, cell); err != nil {
		t.Fatal(err)
	}
	s.Stash.Inv.Add(skill.NewPush(1))
	s.Stash.Fresh[0] = true
	s.load(3, 0.5)
	s.Kills = 41
	s.Banks[2][0] = 3
	b, err := s.Save()
	if err != nil {
		t.Fatal(err)
	}
	if s.Dirty {
		t.Fatal("Save should clear Dirty")
	}
	r := NewVeteran(999)
	if err := r.LoadSave(b); err != nil {
		t.Fatal(err)
	}
	if r.Depth != 3 || r.Best != 3 || r.Kills != 41 || r.RunSeed != 11 {
		t.Fatalf("depth %d best %d kills %d seed %d", r.Depth, r.Best, r.Kills, r.RunSeed)
	}
	if got := r.Load.Hexes[3].Slots[4]; got.Kind != skill.SlotPart || got.Part.Type != cryo.Type || !r.Load.Edited[3] {
		t.Fatalf("hex edit lost: %+v", got)
	}
	if r.Stash.Inv.Count() != 1 || r.Unsecured() != 1 {
		t.Fatalf("pack %d, unsecured %d", r.Stash.Inv.Count(), r.Unsecured())
	}
	if r.Banks[2][0] != 0 {
		t.Fatal("charge banks should start empty on the loaded floor")
	}
	if p := r.P(); p.U.HP/p.U.MaxHP < 0.49 || p.U.HP/p.U.MaxHP > 0.51 {
		t.Fatalf("hull share %v, want 0.5", p.U.HP/p.U.MaxHP)
	}
	if r.Disp.Loadout(r.Player).Progs[3] != r.Load.Progs[3] {
		t.Fatal("loaded hexes aren't equipped")
	}
	// The same depth regenerates the same floor.
	if r.Floor.Seed != s.Floor.Seed {
		t.Fatal("floor layout changed across a save")
	}
}

func TestBadSaveLeavesTheGameAlone(t *testing.T) {
	s := NewVeteran(5)
	s.load(4, 1)
	for _, b := range []string{"", "{", `{"Version":99,"Depth":3}`, `{"Version":1,"Depth":0}`, `{"Version":1,"Depth":3}`} {
		if err := s.LoadSave([]byte(b)); err == nil {
			t.Fatalf("%q loaded", b)
		}
	}
	if s.Depth != 4 || s.Load.Hexes[0] == nil {
		t.Fatal("a refused save changed the game")
	}
}

func TestFallFloors(t *testing.T) {
	cases := []struct {
		depth int
		since float64
		want  int
	}{
		{1, -1, 2}, {12, -1, 2}, {30, -1, 3}, {50, -1, 5}, {100, -1, 10},
		{50, 0, 3}, {50, MercyWindow / 2, 4}, {50, MercyWindow, 5}, {3, 0, 1},
	}
	for _, c := range cases {
		if got, _ := FallFloors(c.depth, c.since); got != c.want {
			t.Errorf("FallFloors(%d, %v) = %d, want %d", c.depth, c.since, got, c.want)
		}
	}
}

func TestDeathDropsBackAndLeavesSalvage(t *testing.T) {
	s := NewVeteran(7)
	s.load(30, 1)
	safe := s.Stash.Inv.Add(skill.NewPush(1)) // picked up before the checkpoint
	for i := 0; i < 3; i++ {
		c := s.Stash.Inv.Add(skill.NewDamage(0, float32(10+i)))
		s.Stash.Fresh[c] = true
	}
	kills := s.Kills
	kill(s)
	if s.State != Dead {
		t.Fatal("not dead")
	}
	d := s.LastDeath
	if d.From != 30 || d.To != 27 || d.Lost != 3 || d.Mercy {
		t.Fatalf("death %+v", d)
	}
	if s.Stash.Inv.Count() != 1 || !s.Stash.Inv.Has[safe] {
		t.Fatal("secured part should stay in the pack")
	}
	if !s.Cache.Active || s.Cache.Depth != 30 || s.Cache.N != 3 {
		t.Fatalf("cache %+v", s.Cache)
	}
	// A save taken on the death screen already counts the fall.
	if sv := s.Snapshot(); sv.Depth != 27 || sv.HPFrac != 1 {
		t.Fatalf("snapshot while dead: depth %d hull %v", sv.Depth, sv.HPFrac)
	}
	s.StateT = 1
	s.Input.Restart = true
	s.Step(1.0 / 60)
	if s.State != Playing || s.Depth != 27 || s.P().U.HP != s.P().U.MaxHP {
		t.Fatalf("respawn: state %d depth %d", s.State, s.Depth)
	}
	if s.Kills != kills || s.Best != 30 || s.Load.Hexes[0] == nil {
		t.Fatal("respawn lost progress")
	}
	// Elsewhere the cache isn't there.
	s.P().At = s.Cache.At
	s.pickup(0)
	if !s.Cache.Active {
		t.Fatal("cache recovered on the wrong floor")
	}
	// Back on 30, standing on it gives the parts back, still unsecured.
	s.load(30, 1)
	s.P().At = s.Cache.At
	s.pickup(0)
	if s.Cache.Active || s.Stash.Inv.Count() != 4 || s.Unsecured() != 3 {
		t.Fatalf("salvage: cache %v pack %d unsecured %d", s.Cache.Active, s.Stash.Inv.Count(), s.Unsecured())
	}
}

func TestSecondDeathCostsLessAndLosesTheOldCache(t *testing.T) {
	s := NewVeteran(8)
	s.load(50, 1)
	c := s.Stash.Inv.Add(skill.NewPush(1))
	s.Stash.Fresh[c] = true
	kill(s)
	if s.LastDeath.To != 45 {
		t.Fatalf("first death to %d, want 45", s.LastDeath.To)
	}
	s.respawn()
	s.Time += 5
	c = s.Stash.Inv.Add(skill.NewPush(2))
	s.Stash.Fresh[c] = true
	kill(s)
	d := s.LastDeath
	if !d.Mercy || d.To != 42 || d.Gone != 1 || d.Lost != 1 {
		t.Fatalf("second death %+v", d)
	}
	if s.Cache.Depth != 45 || s.Cache.N != 1 || s.Cache.Parts[0].Value != 2 {
		t.Fatalf("new cache %+v", s.Cache)
	}
}

func TestCheckpointSecuresThePack(t *testing.T) {
	s := NewVeteran(9)
	s.load(4, 1)
	c := s.Stash.Inv.Add(skill.NewPush(1))
	s.Stash.Fresh[c] = true
	if s.NextCheckpoint() != 5 {
		t.Fatalf("next checkpoint %d", s.NextCheckpoint())
	}
	s.load(5, 1)
	if s.Unsecured() != 0 || s.CheckpointT == 0 {
		t.Fatal("checkpoint didn't secure the pack")
	}
	kill(s)
	if s.LastDeath.Lost != 0 || s.Cache.Active || s.Stash.Inv.Count() != 1 {
		t.Fatal("a secured part was lost")
	}
}

func TestSlottingSecuresAPart(t *testing.T) {
	s := NewVeteran(10)
	c := s.Stash.Inv.Add(skill.NewDamage(s.Reg.MustID("cryo"), 20))
	s.Stash.Fresh[c] = true
	if err := s.SwapIn(3, 4, c); err != nil {
		t.Fatal(err)
	}
	if s.Unsecured() != 0 {
		t.Fatal("slotted part still counts as unsecured")
	}
	// Taking it back out keeps it safe.
	if err := s.SwapIn(3, 4, c); err != nil {
		t.Fatal(err)
	}
	if !s.Stash.Inv.Has[c] || s.Unsecured() != 0 {
		t.Fatal("part out of a hex should be secured")
	}
}

func TestPickupMarksFreshAndRevisitRollsNewLoot(t *testing.T) {
	s := NewVeteran(12)
	s.load(20, 1)
	at := s.P().At
	s.Stash.Drops = append(s.Stash.Drops, Drop{At: at, Part: skill.NewPush(1)})
	s.Dirty = false
	s.pickup(0)
	if s.Unsecured() != 1 || !s.Dirty {
		t.Fatal("pickup should be unsecured and worth saving")
	}
	roll := func() []skill.Part {
		var got []skill.Part
		s.Stash.Drops = s.Stash.Drops[:0]
		for i := range s.Actors {
			if skill.EntityID(i) != s.Player {
				s.dropLoot(skill.EntityID(i))
			}
		}
		for _, d := range s.Stash.Drops {
			got = append(got, d.Part)
		}
		return got
	}
	a := roll()
	s.load(20, 1)
	b := roll()
	same := len(a) == len(b)
	for i := 0; same && i < len(a); i++ {
		same = a[i] == b[i]
	}
	if same {
		t.Fatal("a second visit dropped exactly the same loot")
	}
}

// A cache saved where the regenerated floor now has rock moves next to the
// arrival point instead of being lost.
func TestStaleCacheMovesNextToStart(t *testing.T) {
	s := NewVeteran(11)
	s.Cache = Cache{Active: true, Depth: 3, At: skill.Coord{Q: -500, R: 2}, N: 1}
	s.load(3, 1)
	f := s.Floor
	if !s.Cache.Active || !f.At(s.Cache.At).Walkable() || skill.Dist(s.Cache.At, f.Start) != 1 {
		t.Fatalf("cache at %v (%s), start %v", s.Cache.At, f.At(s.Cache.At), f.Start)
	}
}

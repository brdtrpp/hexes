package game

import (
	"testing"

	"hexes/dungeon"
	"hexes/skill"
)

// bot drives the player like a simple human: walk toward the stairs, cast
// the lance at anything close, vent when hot.
type bot struct {
	exit       []int32
	q          []int32
	floorSeed  uint64
	fired      [MaxSlots]int
	pitKills   int
	wallSlams  int
	overheated int
}

func (b *bot) input(s *Sim) Input {
	f := s.Floor
	if b.exit == nil || b.floorSeed != f.Seed {
		b.exit = make([]int32, len(f.Tiles))
		b.q = make([]int32, len(f.Tiles))
		f.DistanceField(f.Exit, b.exit, b.q)
		b.floorSeed = f.Seed
	}
	var in Input
	p := s.P()
	if p.Arrived() {
		cur := b.exit[f.Index(p.At)]
		for _, d := range skill.HexDirs {
			n := p.At.Add(d)
			if f.In(n) && b.exit[f.Index(n)] >= 0 && b.exit[f.Index(n)] < cur && !s.occupied(n) {
				dx, dy := d.Pixel()
				in.MoveX, in.MoveY = dx, dy
				break
			}
		}
	}
	if foe := s.nearestFoe(p.At, 4); foe != skill.NoEntity && s.Rig.Power > 40 {
		in.Cast[0] = true
	}
	if s.Rig.Heat > 85 {
		in.Vent = true
	}
	if s.Rig.Locked() && s.Rig.Heat >= s.Rig.MaxHeat-1 {
		b.overheated++
	}
	return in
}

type tally struct {
	*Sim
	b *bot
}

func TestBotPlays(t *testing.T) {
	descended, died := 0, 0
	var total bot
	for seed := uint64(1); seed <= 12; seed++ {
		s := New(seed)
		b := &bot{}
		for frame := 0; frame < 60*240 && s.Depth < 4; frame++ {
			s.Input = b.input(s)
			before := s.Disp.Stats.Fired
			s.Step(1.0 / 60)
			_ = before
			for i := range s.SlotFlash {
				if s.SlotFlash[i] == 0.4-1.0/60 || (s.SlotFlash[i] > 0.38 && s.SlotLast[i] == skill.Fired) {
					b.fired[i]++
				}
			}
			s.FX.Each(func(f *FX) {
				if f.Age == 0 {
					switch f.Kind {
					case FXFall:
						b.pitKills++
					case FXSlam:
						b.wallSlams++
					}
				}
			})
			if s.State == Dead {
				died++
				break
			}
		}
		if s.Depth > 1 {
			descended++
		}
		for i := range total.fired {
			total.fired[i] += b.fired[i]
		}
		total.pitKills += b.pitKills
		total.wallSlams += b.wallSlams
		total.overheated += b.overheated
		t.Logf("seed %2d: depth %d kills %3d state %d fired %v pits %d slams %d",
			seed, s.Depth, s.Kills, s.State, b.fired, b.pitKills, b.wallSlams)
	}
	if descended == 0 {
		t.Fatal("bot never reached depth 2")
	}
	for i, n := range total.fired {
		if n == 0 {
			t.Fatalf("slot %d never fired", i)
		}
	}
	if total.pitKills+total.wallSlams == 0 {
		t.Fatal("no terrain kills or slams in 12 runs")
	}
	t.Logf("descended %d/12, died %d, pit falls %d, wall slams %d", descended, died, total.pitKills, total.wallSlams)
}

func TestDeterministicRun(t *testing.T) {
	run := func() (int, float32, int) {
		s := New(99)
		b := &bot{}
		for frame := 0; frame < 60*30; frame++ {
			s.Input = b.input(s)
			s.Step(1.0 / 60)
		}
		return s.Kills, s.P().U.HP, s.Disp.Stats.Fired
	}
	k1, h1, f1 := run()
	k2, h2, f2 := run()
	if k1 != k2 || h1 != h2 || f1 != f2 {
		t.Fatalf("same seed, same inputs, different runs: %d/%d %.1f/%.1f %d/%d", k1, k2, h1, h2, f1, f2)
	}
}

func TestPushIntoPitKills(t *testing.T) {
	s := New(5)
	p := s.P()
	// Build a small test room around the player: floor, with a pit two
	// hexes east and a grunt between.
	for _, d := range skill.HexDirs {
		s.Floor.Set(p.At.Add(d), dungeon.Ground)
	}
	east := skill.HexDirs[0]
	foeAt := p.At.Add(east)
	pit := foeAt.Add(east)
	s.Floor.Set(pit, dungeon.Pit)
	for i := range s.Actors {
		if skill.EntityID(i) != s.Player {
			s.Actors[i].U.Alive = false
			s.occ[s.Floor.Index(s.Actors[i].At)] = skill.NoEntity
		}
	}
	foe := s.spawn(KindGrunt, 1, foeAt, 1, 1e6)
	res := s.Eval.Run(s.Load.Progs[1], s, skill.Ctx{Caster: s.Player, Subject: foe, Level: 1})
	if s.Actors[foe].U.Alive || !s.Actors[foe].Fell || res.Kills != 1 {
		t.Fatalf("grunt with 1e6 hp should die in the pit: alive %v fell %v %+v", s.Actors[foe].U.Alive, s.Actors[foe].Fell, res)
	}
}

func BenchmarkStep(b *testing.B) {
	for _, depth := range []int{1, 10, 30} {
		b.Run("depth"+itoa(depth), func(b *testing.B) {
			s := New(3)
			s.load(depth, 1)
			s.P().U.MaxHP, s.P().U.HP = 1e9, 1e9
			bt := &bot{}
			// Wake everything so the AI is doing real work.
			for i := range s.Actors {
				s.Actors[i].Awake = true
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				s.Input = bt.input(s)
				s.Step(1.0 / 60)
				if s.State != Playing {
					s.load(depth, 1)
					s.P().U.MaxHP, s.P().U.HP = 1e9, 1e9
				}
			}
			b.ReportMetric(float64(len(s.Actors)), "actors")
		})
	}
}

func itoa(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + string(rune('0'+n%10))
}

package game

import (
	"errors"
	"fmt"

	"hexes/loot"
	"hexes/skill"
)

// Logic tiles: the sensors, compares and operators conditions are built
// from. A new rig knows the starter set; the rest drop as loot and, like
// shapes, are learned for good the moment you walk over one. A tile you
// already know just disappears into your notes.

// Tile drop tuning. Placeholder balance.
const (
	TileDropChance = 0.05 // per kill
	TileDropElite  = 2.5  // times for an elite
	MaxTileDrops   = 4    // lying on one floor at once
)

// tierWeights is how often each tier comes up when a tile drops.
var tierWeights = [skill.NumRarities]int{60, 25, 12, 3}

// TileDrop is a logic tile lying on the floor.
type TileDrop struct {
	At   skill.Coord
	Tile skill.TileKind
	Age  float32
}

// KnownTiles is every tile the player can build with: the starter set,
// what they've found, and everything while the test switch is on.
func (s *Sim) KnownTiles() uint32 {
	k := s.Tiles | skill.StarterTiles
	if s.TestShapes {
		k |= skill.AllTiles
	}
	return k
}

// Knows reports whether the player can use tile k.
func (s *Sim) Knows(k skill.TileKind) bool { return s.KnownTiles()&skill.TileBit(k) != 0 }

// rollTileDrop maybe drops a tile the player doesn't know yet.
func (s *Sim) rollTileDrop(id skill.EntityID, at skill.Coord, elite bool) {
	if len(s.Stash.Tiles) >= MaxTileDrops || !s.Has(UQuirks) {
		return
	}
	chance := float32(TileDropChance)
	if elite {
		chance *= TileDropElite
	}
	r := loot.NewRNG(loot.Seed(s.Floor.Seed^hash(uint64(s.Visits))^0x711e5, int(id)))
	if r.Float() >= chance {
		return
	}
	if k, ok := s.pickTile(&r); ok {
		s.Stash.Tiles = append(s.Stash.Tiles, TileDrop{At: at, Tile: k})
	}
}

// pickTile picks an unknown tile, its tier rolled by tierWeights. With
// none left in that tier it takes any unknown one, commonest first.
func (s *Sim) pickTile(r *loot.RNG) (skill.TileKind, bool) {
	known := s.Tiles | skill.StarterTiles
	for _, d := range s.Stash.Tiles {
		known |= skill.TileBit(d.Tile)
	}
	total := 0
	for _, w := range tierWeights {
		total += w
	}
	roll := int(r.Float() * float32(total))
	tier := skill.Common
	for t, w := range tierWeights {
		if roll < w {
			tier = skill.Rarity(t)
			break
		}
		roll -= w
	}
	var pool []skill.TileKind
	for k := skill.TileHull; k < skill.NumTiles; k++ {
		if known&skill.TileBit(k) == 0 && k.Tier() == tier {
			pool = append(pool, k)
		}
	}
	if len(pool) == 0 {
		for t := skill.Common; t < skill.NumRarities && len(pool) == 0; t++ {
			for k := skill.TileHull; k < skill.NumTiles; k++ {
				if known&skill.TileBit(k) == 0 && k.Tier() == t {
					pool = append(pool, k)
				}
			}
		}
	}
	if len(pool) == 0 {
		return 0, false
	}
	return pool[int(r.Float()*float32(len(pool)))%len(pool)], true
}

// pickupTiles learns any tile under the player.
func (s *Sim) pickupTiles(at skill.Coord, dt float32) {
	d := s.Stash.Tiles
	for i := 0; i < len(d); {
		d[i].Age += dt
		if d[i].At != at {
			i++
			continue
		}
		s.Tiles |= skill.TileBit(d[i].Tile)
		// The brackets come as a pair.
		if d[i].Tile == skill.TileOpen || d[i].Tile == skill.TileClose {
			s.Tiles |= skill.TileBit(skill.TileOpen) | skill.TileBit(skill.TileClose)
		}
		s.LastTile = d[i].Tile
		s.Dirty = true
		s.addFX(FX{Kind: FXTile, At: at, Slot: int(d[i].Tile), Life: 2.2})
		d[i] = d[len(d)-1]
		d = d[:len(d)-1]
	}
	s.Stash.Tiles = d
}

// SetCond puts condition c on the gate in a hex slot. Every tile in it
// must be one the player knows, and it must parse. A preset gate becomes
// a condition gate. On error nothing changes.
func (s *Sim) SetCond(hex, slot int, c skill.Cond) error {
	if !s.Load.Has(hex) || slot < 0 || slot >= skill.Slots {
		return errors.New("no such slot")
	}
	sl := &s.Load.Hex(hex).Slots[slot]
	watch := sl.Kind == skill.SlotPart && sl.Part.Kind == skill.KindTrigger && sl.Part.Trigger == skill.Watch
	if !watch && (sl.Kind != skill.SlotPart || sl.Part.Kind != skill.KindGate || sl.Part.Gate.Charged()) {
		return errors.New("only an IF gate or a watch holds a condition")
	}
	if missing := c.Uses() &^ s.KnownTiles(); missing != 0 {
		for k := skill.TileHull; k < skill.NumTiles; k++ {
			if missing&skill.TileBit(k) != 0 {
				return fmt.Errorf("you haven't found the %s tile yet", k)
			}
		}
	}
	if _, err := c.Compile(); err != nil {
		return err
	}
	old := sl.Part
	if watch {
		sl.Part.Cond, sl.Part.Lib = c, ""
	} else {
		sl.Part = old.WithCond(c)
	}
	if err := s.recompile(hex); err != nil {
		sl.Part = old
		return err
	}
	return nil
}

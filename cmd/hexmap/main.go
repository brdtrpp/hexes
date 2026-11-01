// Command hexmap prints a generated floor as text, for eyeballing the
// generator without the game.
//
//	go run ./cmd/hexmap -seed 7 -depth 5
//
// Legend: # wall, . floor, O pit, ~ hazard vent, > stairs, @ start,
// g/s/b grunt, skitter, brute spawns (capitals are elites).
package main

import (
	"flag"
	"fmt"
	"strings"

	"hexes/dungeon"
	"hexes/skill"
)

func main() {
	seed := flag.Uint64("seed", 1, "floor seed")
	depth := flag.Int("depth", 1, "dungeon depth (1 is the first floor)")
	flag.Parse()

	f := dungeon.Generate(*seed, *depth)
	spawns := map[skill.Coord]dungeon.Spawn{}
	for _, s := range f.Spawns {
		spawns[s.At] = s
	}
	base := dungeon.ParamsFor(*depth).EnemyLevel
	var b strings.Builder
	for r := 0; r < f.H; r++ {
		if r%2 == 1 {
			b.WriteByte(' ') // odd rows sit half a hex to the right
		}
		for col := 0; col < f.W; col++ {
			c := skill.Coord{Q: col - r>>1, R: r}
			ch := byte('?')
			switch f.At(c) {
			case dungeon.Wall:
				ch = '#'
			case dungeon.Ground:
				ch = '.'
			case dungeon.Pit:
				ch = 'O'
			case dungeon.Hazard:
				ch = '~'
			case dungeon.Stairs:
				ch = '>'
			}
			if s, ok := spawns[c]; ok {
				ch = "gsb"[s.Kind]
				if s.Level > base {
					ch -= 'a' - 'A'
				}
			}
			if c == f.Start {
				ch = '@'
			}
			b.WriteByte(ch)
			b.WriteByte(' ')
		}
		b.WriteByte('\n')
	}
	fmt.Print(b.String())
	fmt.Printf("seed %d depth %d: %dx%d, %d rooms, %d enemies (level %d), %d pits, %d vents\n",
		f.Seed, f.Depth, f.W, f.H, len(f.Rooms), len(f.Spawns), base, f.Count(dungeon.Pit), f.Count(dungeon.Hazard))
}

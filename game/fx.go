package game

import "hexes/skill"

// FXKind is a visual effect the renderer draws. The sim records them so
// the renderer never has to guess what happened.
type FXKind uint8

const (
	FXNumber FXKind = iota // floating damage number at At
	FXBolt                 // a hex's hit, From caster to At
	FXRing                 // a hex fired from At
	FXPush                 // a push slid something From to At
	FXSlam                 // pushed into a wall at At (wall at From)
	FXFall                 // fell into the pit at At
	FXKill                 // something died at At
	FXClaw                 // enemy melee From attacker to At
	FXVent                 // the rig vented at At
	FXLoot                 // picked up a part at At; Slot is its rarity
	FXFull                 // walked over a part with a full inventory
)

// FX is one effect.
type FX struct {
	Kind     FXKind
	At, From skill.Coord
	// X, Y and FromX, FromY are where to draw At and From in hex units.
	// They follow the player's true position when the tile is the
	// player's, so rings and numbers sit on the player, not its hex.
	X, Y, FromX, FromY float64
	Value              float32
	Type               skill.TypeID
	Slot               int
	Terrain            bool
	Hurt               bool // damage to the player
	Small              bool // damage-over-time and hazard ticks
	Age                float32
	Life               float32
}

// MaxFX is how many effects can be alive at once; the oldest drop first.
const MaxFX = 256

// FXBuf is a fixed ring of effects.
type FXBuf struct {
	buf  [MaxFX]FX
	head int
	n    int
}

// Add records an effect.
func (b *FXBuf) Add(f FX) {
	if b.n == MaxFX {
		b.head = (b.head + 1) % MaxFX
		b.n--
	}
	b.buf[(b.head+b.n)%MaxFX] = f
	b.n++
}

// Update ages effects and drops the finished ones from the front.
func (b *FXBuf) Update(dt float32) {
	for i := 0; i < b.n; i++ {
		b.buf[(b.head+i)%MaxFX].Age += dt
	}
	for b.n > 0 && b.buf[b.head].Age >= b.buf[b.head].Life {
		b.head = (b.head + 1) % MaxFX
		b.n--
	}
}

// Each calls fn for every live effect, oldest first.
func (b *FXBuf) Each(fn func(*FX)) {
	for i := 0; i < b.n; i++ {
		f := &b.buf[(b.head+i)%MaxFX]
		if f.Age < f.Life {
			fn(f)
		}
	}
}

// Len is how many effects are live (or waiting to be dropped).
func (b *FXBuf) Len() int { return b.n }

// Clear drops every effect.
func (b *FXBuf) Clear() { b.head, b.n = 0, 0 }

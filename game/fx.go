package game

import "hexes/skill"

// FXKind is a visual effect the renderer draws. The sim records them so
// the renderer never has to guess what happened.
type FXKind uint8

const (
	FXNumber  FXKind = iota // floating damage number at At
	FXBolt                  // a hex's hit, From caster to At
	FXRing                  // a hex fired from At
	FXPush                  // a push slid something From to At
	FXSlam                  // pushed into a wall at At (wall at From)
	FXFall                  // fell into the pit at At
	FXKill                  // something died at At
	FXClaw                  // enemy melee From attacker to At
	FXVent                  // the rig vented at At
	FXLoot                  // picked up a part at At; Slot is its rarity
	FXFull                  // walked over a part with a full inventory
	FXSalvage               // recovered a salvage cache at At; Value is how many parts
	FXShape                 // learned a shape at At; Slot is its kind
	FXTile                  // learned a logic tile at At; Slot is its kind
	FXWave                  // a hex touched a round area: centre X, Y, Value is its radius in hexes
	FXCone                  // a hex swept a cone from X, Y along DX, DY, Value hexes long
	FXChain                 // a hex chained through Pts, in hop order
	FXLens                  // picked up a lens at At; Slot is Optics points gained, Value the new light radius
	FXFrame                 // racked a frame at At; Slot is its bend
	FXMend                  // a Mend restored Value hull at At; Mag is how much of max hull
	FXShield                // a Shield added Value buffer at At; Mag is how much of max hull
	FXPurge                 // a Purge took Value Heat off the rig at At
	FXSoak                  // a shield at At soaked Value of a hit; Hurt if it's the player's
	FXLesson                // took a lesson pickup at At; Slot is its rung on the Ladder
	FXData                  // earned Value Data at At
	FXSignal                // found a signal fragment at At
	FXMuzzle                // a gunner fired from X, Y along DX, DY
	FXZap                   // a gunner's shot stopped at X, Y (Hurt: on the player)
	FXBeam                  // a hex swept a line from X, Y along DX, DY, Value hexes long; Slot 1 both ways
	FXWreck                 // a wreck at At was used up; Slot is its kind
	FXRaise                 // a wreck at At got up as a minion; Slot is its kind
	FXDeploy                // a device was placed at At; Slot is its form
	FXDeviceFire            // a device at At ran its hex; Slot is its form
	FXExpire                // a device (or a minion, Value 1) at At ran out
	FXThorns                // thorns hit back at At, From you
	FXTaunt                 // a taunt landed on At, From the taunter
	FXBuff                  // thorns or haste came on at At; Slot is the effect
	FXAmbush                // a hit out of stealth landed on At
)

// MaxChainPts is how many hops an FXChain draws (the caster plus seven).
const MaxChainPts = 8

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

	// What the renderer needs to animate it procedurally.
	// Mag is how hard it landed (fx.MagOf); Heat and Power are what the
	// hex cost when it fired (FXRing and the area forms).
	Mag, Heat, Power float32
	// DX, DY is a unit direction (cones).
	DX, DY float32
	// Pts are an FXChain's hop points in hex units, NPts of them.
	Pts  [MaxChainPts][2]float32
	NPts uint8
	// Impact marks an FXBolt that landed inside an area form: draw the
	// hit, not a line back to the caster.
	Impact bool
	// Grade is how much hex made it, 0 to 3: the renderer adds structure
	// (echoes, forks, sparks, a hotter palette) as it climbs. Set on a
	// hex's hits and forms when it fires.
	Grade uint8
	// Seen is set by the renderer once it has spawned this effect's
	// particles, so each effect bursts once however often it's drawn.
	Seen bool
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

// Back calls fn for every effect, newest first, until fn returns false.
func (b *FXBuf) Back(fn func(*FX) bool) {
	for i := b.n - 1; i >= 0; i-- {
		if !fn(&b.buf[(b.head+i)%MaxFX]) {
			return
		}
	}
}

// Len is how many effects are live (or waiting to be dropped).
func (b *FXBuf) Len() int { return b.n }

// Clear drops every effect.
func (b *FXBuf) Clear() { b.head, b.n = 0, 0 }

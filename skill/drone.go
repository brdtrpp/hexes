package skill

import "fmt"

// A drone carries its own gun. The drone part rolls it when it drops
// (Part.Seed), so no two drones need shoot alike, and you don't wire
// it: the rest of the hex a drone sits in runs from you as usual. The
// drone hunts on its own (see the game's devices) and fires its gun at
// the foe it picked, every DroneEvery, from wherever it is.

// DroneGun is how a drone's gun hits.
type DroneGun uint8

const (
	GunPulse   DroneGun = iota // one hard shot at its quarry
	GunScatter                 // the three foes nearest the drone
	GunArc                     // jumps from its quarry through three foes
	GunLance                   // every foe on a short line through its quarry
	GunFlak                    // a burst round the drone
	NumGuns
)

var gunNames = [NumGuns]string{"pulse", "scatter", "arc", "lance", "flak"}

func (g DroneGun) String() string { return gunNames[g] }

// gunDamage is each gun's damage per foe per shot, at common.
var gunDamage = [NumGuns]float32{GunPulse: 14, GunScatter: 6, GunArc: 7, GunLance: 8, GunFlak: 7}

// DroneRider is what a drone's shot does beside damage.
type DroneRider uint8

const (
	RiderNone  DroneRider = iota
	RiderChill            // a cryo chill
	RiderShove            // a shove away from the drone
	NumRiders
)

// Drone gun tuning.
const (
	DroneChill = 1.5 // seconds of chill a chilling gun leaves
	DroneShove = 1   // hexes a shoving gun knocks a foe
)

// DroneTypes are the damage types a drone gun rolls, by name, and how
// often of 20: the Engineer's cryo most, then kinetic slugs, then plasma.
var DroneTypes = [...]struct {
	Name string
	Odds int
}{{"cryo", 12}, {"kinetic", 5}, {"plasma", 3}}

// DroneKit is a drone's gun.
type DroneKit struct {
	Gun   DroneGun
	Rider DroneRider
	// Type is the gun's damage type by name.
	Type string
	// Damage is per foe per shot.
	Damage float32
}

// DroneSeed turns any number into a drone part's Seed: under 1<<24 and
// never 0, which marks a drone from before they carried guns.
func DroneSeed(x uint64) uint32 {
	x ^= x >> 31
	x *= 0x94d049bb133111eb
	x ^= x >> 29
	return uint32(x)&(1<<24-1) | 1
}

// DroneKitOf is the gun a drone part with this seed and rarity carries.
// It's worked out, not stored, so a part keeps its gun for good.
func DroneKitOf(seed uint32, r Rarity) DroneKit {
	h := uint64(seed)*0x9e3779b97f4a7c15 + 0x632be59bd9b4e019
	h ^= h >> 29
	h *= 0xbf58476d1ce4e5b9
	h ^= h >> 32
	k := DroneKit{Gun: DroneGun(h % uint64(NumGuns))}
	h /= uint64(NumGuns)
	roll := int(h % 20)
	h /= 20
	for _, t := range DroneTypes {
		if roll < t.Odds {
			k.Type = t.Name
			break
		}
		roll -= t.Odds
	}
	k.Rider = DroneRider(h % uint64(NumRiders))
	k.Damage = float32(int(gunDamage[k.Gun]*r.Scale() + 0.5))
	return k
}

// DroneKit is the gun this drone part carries.
func (p Part) DroneKit() DroneKit { return DroneKitOf(p.Seed, p.Rarity) }

// Title names the gun: "cryo pulse".
func (k DroneKit) Title() string { return k.Type + " " + k.Gun.String() }

// Text says what the gun does, in a line.
func (k DroneKit) Text() string {
	var s string
	switch k.Gun {
	case GunPulse:
		s = fmt.Sprintf("a %s pulse at its quarry for %g", k.Type, k.Damage)
	case GunScatter:
		s = fmt.Sprintf("%s scatter at the 3 foes nearest it for %g each", k.Type, k.Damage)
	case GunArc:
		s = fmt.Sprintf("a %s arc from its quarry through 3 foes for %g each", k.Type, k.Damage)
	case GunLance:
		s = fmt.Sprintf("a %s lance along a line through its quarry for %g each", k.Type, k.Damage)
	case GunFlak:
		s = fmt.Sprintf("%s flak round itself for %g each", k.Type, k.Damage)
	}
	switch k.Rider {
	case RiderChill:
		s += fmt.Sprintf(", chilling for %g s", float32(DroneChill))
	case RiderShove:
		s += ", shoving them away from the drone"
	}
	return s
}

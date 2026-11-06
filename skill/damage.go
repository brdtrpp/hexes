package skill

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// TypeID indexes a damage type in a Registry.
type TypeID uint8

// MaxTypes is the most damage types a Registry can hold. Units keep one
// resistance value per type in a fixed array, so this bounds their size.
const MaxTypes = 16

// StatusKind is the lingering effect a damage type's Status parts apply.
type StatusKind uint8

const (
	StatusNone    StatusKind = iota
	StatusBurn               // damage over time
	StatusChill              // slow, stacks to a freeze
	StatusStagger            // stun
	StatusToxin              // damage over time, many small ticks
	StatusMark               // marked foes are raised if they die while marked
	StatusCloak              // caster gains stealth
)

var statusNames = map[string]StatusKind{
	"none": StatusNone, "burn": StatusBurn, "chill": StatusChill,
	"stagger": StatusStagger, "toxin": StatusToxin, "mark": StatusMark, "cloak": StatusCloak,
}

func (s StatusKind) String() string {
	for k, v := range statusNames {
		if v == s {
			return k
		}
	}
	return fmt.Sprintf("status(%d)", uint8(s))
}

// UnmarshalJSON reads a status from its name, so type files stay readable.
func (s *StatusKind) UnmarshalJSON(b []byte) error {
	var name string
	if err := json.Unmarshal(b, &name); err != nil {
		return err
	}
	v, ok := statusNames[strings.ToLower(name)]
	if !ok {
		return fmt.Errorf("unknown status %q", name)
	}
	*s = v
	return nil
}

// DamageType is pure data. Every behavior a type has is a number here, and
// the evaluator reads these numbers. Adding a type means adding a row to the
// JSON, not writing code. A zero value means "this type doesn't do that".
type DamageType struct {
	Name   string     `json:"name"`
	Status StatusKind `json:"status"`
	// StatusPower is damage per tick for burn and toxin statuses.
	StatusPower float32 `json:"status_power"`
	// SpreadFrac splashes this fraction of each hit onto foes next to the target.
	SpreadFrac float32 `json:"spread_frac"`
	// Slow is the movement slow a chill applies.
	Slow float32 `json:"slow"`
	// FreezeStacks is how many chills freeze a foe.
	FreezeStacks uint8 `json:"freeze_stacks"`
	// ShatterMult multiplies the hit that breaks a freeze this type caused.
	ShatterMult float32 `json:"shatter_mult"`
	// HeatOffset is added to a hex's Heat for every part of this type.
	// Negative values vent heat.
	HeatOffset float32 `json:"heat_offset"`
	// PushPerHit pushes the target this many hexes on every damage hit.
	PushPerHit float32 `json:"push_per_hit"`
	// WallImpactFrac is terrain damage (a fraction of max health, uncapped)
	// when this type's push slams a foe into a wall.
	WallImpactFrac float32 `json:"wall_impact_frac"`
	// Ticks delivers damage as this many ticks over time instead of one hit.
	// Each tick is capped on its own, so ticks slide under the per-hit cap.
	Ticks uint8 `json:"ticks"`
	// RaiseOnKill raises a foe this type kills as the caster's minion.
	RaiseOnKill bool `json:"raise_on_kill"`
	// StealthMult multiplies this type's damage when the hex fires from
	// stealth or from behind.
	StealthMult float32 `json:"stealth_mult"`
}

// Registry holds the damage types in play.
type Registry struct {
	defs   [MaxTypes]DamageType
	n      int
	byName map[string]TypeID
}

//go:embed default_types.json
var defaultTypesJSON []byte

// DefaultRegistry returns the six seed types from default_types.json.
func DefaultRegistry() *Registry {
	r, err := LoadRegistry(strings.NewReader(string(defaultTypesJSON)))
	if err != nil {
		panic("skill: bad embedded default_types.json: " + err.Error())
	}
	return r
}

// LoadRegistry reads a JSON array of damage types.
func LoadRegistry(rd io.Reader) (*Registry, error) {
	var defs []DamageType
	if err := json.NewDecoder(rd).Decode(&defs); err != nil {
		return nil, err
	}
	if len(defs) == 0 {
		return nil, fmt.Errorf("no damage types")
	}
	if len(defs) > MaxTypes {
		return nil, fmt.Errorf("%d damage types, max is %d", len(defs), MaxTypes)
	}
	r := &Registry{byName: make(map[string]TypeID, len(defs))}
	for i, d := range defs {
		if d.Name == "" {
			return nil, fmt.Errorf("damage type %d has no name", i)
		}
		if _, dup := r.byName[d.Name]; dup {
			return nil, fmt.Errorf("duplicate damage type %q", d.Name)
		}
		r.defs[i] = d
		r.byName[d.Name] = TypeID(i)
	}
	r.n = len(defs)
	return r, nil
}

// Len is the number of types loaded.
func (r *Registry) Len() int { return r.n }

// Get returns a type's data. The pointer is stable and must not be modified
// during combat.
func (r *Registry) Get(id TypeID) *DamageType { return &r.defs[id] }

// ID looks a type up by name.
func (r *Registry) ID(name string) (TypeID, bool) {
	id, ok := r.byName[name]
	return id, ok
}

// MustID is ID for names known to exist; it panics otherwise.
func (r *Registry) MustID(name string) TypeID {
	id, ok := r.byName[name]
	if !ok {
		panic("skill: unknown damage type " + name)
	}
	return id
}

// Cloak is a self effect: it hides the caster and never deals damage.
// CloakTimeMin and CloakTimeMax bound how long a cloak converted from an
// old cloak damage part lasts; CloakPerDamage turns its damage into time.
const (
	CloakTimeMin   float32 = 1.5
	CloakTimeMax   float32 = 4.5
	CloakPerDamage float32 = 3.0 / 16
	CloakHeat      float32 = 4 // a cloak's Heat on top of its status cost
)

// StatusOf finds the first type whose status is st.
func (r *Registry) StatusOf(st StatusKind) (TypeID, bool) {
	for i := 0; i < r.n; i++ {
		if r.defs[i].Status == st {
			return TypeID(i), true
		}
	}
	return 0, false
}

// Cloak reports whether t's status is a cloak.
func (r *Registry) Cloak(t TypeID) bool {
	return int(t) < r.n && r.defs[t].Status == StatusCloak
}

// SelfOnly turns a cloak damage part into the cloak status it should have
// been: the same type and rarity, its damage read as seconds of stealth.
// A Rider or a Relay on a cloak status adds nothing (the cloak only hides
// you), so it goes. Anything else comes back as it was. The bool reports a
// change.
func (r *Registry) SelfOnly(p Part) (Part, bool) {
	if p.Kind != KindEffect || !p.Effect.Typed() || !r.Cloak(p.Type) {
		return p, false
	}
	if p.Effect == EffectStatus {
		if p.Quirk == QuirkRider || p.Quirk == QuirkRelay {
			p.Quirk = QuirkNone
			return p, true
		}
		return p, false
	}
	secs := min(CloakTimeMax, max(CloakTimeMin, p.Value*CloakPerDamage))
	q := NewStatus(p.Type, float32(int(secs*2+0.5))/2)
	q.Name, q.Rarity = p.Name, p.Rarity
	if p.Quirk != QuirkRider && p.Quirk != QuirkRelay && p.Quirk.Fits(q) {
		q.Quirk = p.Quirk
	}
	return q, true
}

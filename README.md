# hexes

Hexes: a real-time, procedurally generated sci-fi dungeon crawler in Go and Ebitengine. This folder has the hex skill compiler, the floor generator, trigger dispatch, and a first playable slice.

```
go test ./skill ./dungeon ./game                                  # unit tests (no graphics needed)
go test ./skill ./dungeon ./game -run '^$' -bench . -benchmem     # benchmarks
go run ./cmd/hexes                        # play (desktop window)
go run ./cmd/hexmap -seed 7 -depth 6      # print a floor as text
go run ./cmd/hexdemo                      # same parts, two rolled hex layouts
```

`go run ./cmd/hexes` needs Ebitengine's usual system libraries: nothing extra on macOS or Windows; on Linux the X11 and GL dev packages (`libx11-dev libxrandr-dev libxcursor-dev libxinerama-dev libxi-dev libxxf86vm-dev libgl1-mesa-dev`). `go test ./...` also builds the game window, so on a bare Linux box test the three packages by name as above.

### Browser build

```
GOOS=js GOARCH=wasm go build -trimpath -ldflags="-s -w" -o web/hexes.wasm ./cmd/hexes
gzip -9 -k -f web/hexes.wasm && base64 -w0 web/hexes.wasm.gz > web/hexes.wasm.gz.b64.txt && rm web/hexes.wasm.gz
cd web && python3 -m http.server 8000      # then open http://localhost:8000
```

`web/index.html` loads the gzipped build (shipped as base64 text, since the artifact host doesn't serve .gz) and falls back to the raw `hexes.wasm`. Raw is about 15 MB, the fast path about 4.7 MB.

## Controls

- Desktop: WASD or arrows to move (or hold the mouse button and point), Space casts Arc Lance, R or Shift vents heat, Enter restarts after dying.
- Touch: hold anywhere to walk toward your finger, tap CAST and VENT.
- Strikes on an adjacent foe are automatic. Walk onto the stairs to go down a level.

## What's here

| Path | What it does |
| --- | --- |
| `skill/` | Hex compiler and evaluator (below), plus `dispatch.go`: trigger dispatch. |
| `dungeon/` | Floor generator: rooms, corridors, walls, pits, hazard vents, spawns, stairs, from a seed and a depth. |
| `game/` | Real-time simulation: movement on the hex grid, enemy AI, the player's rig and loadout, terrain kills, effects. No rendering, so it runs in tests. |
| `cmd/hexes/` | The Ebitengine front end: drawing, HUD, keyboard, mouse and touch. |
| `cmd/hexmap/` | Prints a floor as text. |
| `web/` | Browser page and wasm build. |

### skill package files

| File | What it does |
| --- | --- |
| `damage.go`, `default_types.json` | Damage types as data. The six seed types live in the JSON; adding one is a new row, not code. |
| `part.go` | Loot parts: triggers, targets, effects, multipliers, with rarity and Power/Heat costs. |
| `hex.go` | Templates, sockets, `Learn` (seeded layout roll), slotting parts and nesting hexes. |
| `compile.go`, `program.go` | Compiles a hex tree into a flat `[]Step` with all multipliers baked in, plus a disassembler. |
| `eval.go` | Runs a program against a `World`. Fixed arrays only: zero allocations per run. |
| `combat.go` | Units, per-hit cap by level gap, adaptive resistance, damage over time. |
| `arena.go` | A small reference `World` on a hex grid, for tests and benchmarks. |
| `rig.go` | Power, Heat, overheat shutdown, manual vent, and the per-stat soft cap. |
| `dispatch.go` | Trigger dispatch: events in, hexes fire, Power and Heat paid. |

## How a hex resolves

1. The hex resolves clockwise starting at its trigger slot. A nested hex has no trigger and resolves clockwise from its slot 0, once per entity fed to it, with that entity as its origin (a target's target).
2. Parts only feed slots they touch across a linked edge. `Learn` shuffles the template's sockets around the ring and cuts `Broken` edges from the seed, so the same parts in the same hex can wire up differently for two players.
3. Targets select first. An effect lands on the targets beside it: both if it sits between two (merged, no double hits), the trigger's subject if none.
4. A multiplier scales the part just before it, if that edge is linked. A run of multipliers stacks. After a nested hex it scales the matching numbers everywhere inside. With nothing linked before it, it's wasted. Multipliers can scale damage, push, duration, count, radius, or all of them.
5. Compile bakes all of that into flat steps. A nested hex becomes a `foreach` step followed by its inlined body, so combat never walks a tree.

## Defaults I picked

None of these are decided. All are one constant or one JSON field to change.

Hex shape and wiring
- Sockets: trigger, target, effect (also takes multipliers), wild (target, effect, multiplier or nested hex). Exactly one trigger per top-level hex; nested hexes may not have one.
- Top-level targets reach out from the subject for on-hit and on-kill (so "on kill: adjacent" hits around the corpse), and from the caster otherwise. Cast triggers land untargeted effects on the nearest foe.
- Targets ignore rarity; rarity scales effect values (x1, 1.15, 1.35, 1.6) and a multiplier's bonus above 1.
- Surprise triggers: from stealth x2 damage, from behind x1.5. Cloak damage doubles again on either.

Costs
- Part costs: trigger 2 Power; targets 0 to 3; damage 0.2 Power and 0.1 Heat per point; push 2 Power per hex; status 2 plus duration; multiplier 3 Power and 1 + 4x(factor - 1) Heat.
- Parts inside nested hexes cost 1.5x at depth 1, 2x at depth 2, and so on.
- Plasma adds 2 Heat per part; cryo vents 3. A hex's total Heat can go negative.

Combat
- Per-hit cap: 25% of the foe's max health at even level, doubling every 5 levels above (one-shot at +10), halving every 5 below, floor 1%.
- Adaptive resistance: each hit moves that type's resistance 15% of the way to a 60% ceiling; other types fade 2% per hit and 5% per second. Damage-over-time ticks adapt at a quarter rate so poison stays the anti-boss tool.
- Terrain (wall slams, pits) skips the cap, resistance and adaptation. Any push into a wall deals 10% max health; kinetic adds 15%. A pit kills.
- Damage over time ticks every 0.5 s. Each tick is capped on its own.

Damage types (sci-fi names from the brief; the design doc still lists fire, frost, force, poison, raise dead, stealth)
- Plasma: splashes 35% onto foes next to the target; burn status 4 per tick.
- Cryo: chill slows 30%, three chills freeze, the next hit on a frozen foe deals x2.5 and breaks it.
- Kinetic: every hit pushes 1 hex; stagger status stuns.
- Nanite toxin: damage arrives as 8 ticks; toxin status 2 per tick.
- Drone hijack: raises what it kills as your minion at 50% health; mark status raises a foe that dies while marked.
- Cloak: big surprise-trigger bonus; its status gives the caster stealth.

Rig (soft cap per stat, as decided)
- Effective points are one for one up to 10, then 10 x (1 + ln(points / 10)). 20 points is worth 16.9.
- Max Power 100 + 8 per effective Capacity; Recharge 8 + 1.5 per point per second; Cooling 6 + 1.5; max Heat 100; overheat locks the rig for 3 s.
- Max nesting depth is effective Processing / 5: 0 points none, 5 one level, 10 two, 20 three, 40 four.

## Floor generator (`dungeon`)

A floor is a pointy-top hex grid stored as rows, so it looks rectangular on screen, but every API uses the same axial `skill.Coord` as combat. Generation is deterministic per seed and depth (SplitMix64, no math/rand), and a reusable `Generator` keeps scratch buffers so regenerating doesn't churn memory.

1. Rooms: hexagonal rooms with one or two lobes so they read as caverns, placed with rejection sampling.
2. Corridors: a minimum spanning tree over room centers (the floor is always one piece) plus extra corridors between near rooms, so there are loops to kite around. Some corridors are two wide; long ones jog at the midpoint.
3. Start is the first room; the stairs go in the room farthest from it on foot.
4. Pits and hazard vents: random-walk clusters inside rooms, never on a room center. A pit cluster that would cut any room off from the start is undone, so every floor is fully walkable. Pits sit next to open floor, which is where a push can send a foe in.
5. Spawns: on reachable open floor, at least 6 steps from the start, never stacked.

Tests check 3,600 floors (60 seeds by depths 1 to 60) for: walled border, start and stairs connected, every room reachable, valid spawns.

Depth drives everything (`ParamsFor`). Floors grow with the square root of depth so deep floors stay playable on a phone; enemy count and level keep climbing, which is the progression.

| | depth 1 | depth 10 | depth 50 |
| --- | --- | --- | --- |
| Size | 36x26 | 48x34 | 72x50 |
| Rooms | 7 | 10 | 16 |
| Enemies | 6 | 21 | 70 (cap) |
| Pit clusters | 2 | 7 | 24 (cap) |
| Hazard clusters | 0 | 5 | 16 (cap) |

## Trigger dispatch (`skill/dispatch.go`)

Events go into a fixed ring buffer and run in order. A program's hits and kills reach the dispatcher through the evaluator's new `Observer` hook and queue behind it instead of running inside it, so the evaluator is never re-entered and the whole path stays allocation-free.

- Triggers: on cast (button), on hit, on kill, from stealth, from behind, periodic, and on hurt (new). Periodic hexes carry a period (`NewPeriodic(seconds)`).
- `Cast(owner, slot, subject)` fires one on-cast slot. `Strike(owner, program, subject)` runs a free basic attack whose hits still trigger hexes. Both count as attacks for from-stealth (and break stealth) and from-behind. `Hurt(victim, attacker)` fires on-hurt. `Update(dt)` ticks rigs, cooldowns and periodic timers.
- Every firing pays Power and Heat on the owner's rig. Refusals come back as outcomes: no power, locked (overheated or venting), on cooldown.
- A `Listener` hears hits, kills and every firing attempt; the game uses it for numbers, bolts and HUD flashes.
- `World`s that also implement `Senses` get from-behind (`BehindOf`: more than 120 degrees off the target's facing).

## The playable slice (`game`, `cmd/hexes`)

One generated floor at a time. Your rig moves tile to tile in real time (positions lerp between hexes), strikes whatever is next to it, and carries a fixed loadout with one hex per trigger kind:

| Slot | Hex | Trigger | What it does |
| --- | --- | --- | --- |
| 1 | Arc Lance | on cast | Plasma chains 3 foes, x2 damage. 14 Power, 10 Heat. |
| 2 | Kinetic Ram | on hit | Shoves the foe 2 hexes plus 1 from kinetic, so 3 total. Walls slam, pits kill. |
| 3 | Cryo Bloom | on kill | Corpse bursts cold: damage and chill to everything within 2. Cryo vents 3.6 Heat. |
| 4 | Nanite Swarm | every 4 s | Toxin cloud on everything within 2. |

Enemies: grunts (average), skitters (fast, fragile, from depth 2), brutes (slow, tough, from depth 3), plus elites two levels up from depth 4 (white ring). They wake when you come within 10 steps on foot or when hit, chase along a distance field from the player (recomputed only when you change tile), and claw when adjacent. Stairs take you to the next depth with 30% hull back. Dying restarts at depth 1 on a new seed.

A headless bot (walk to the stairs, cast at anything within 4, vent above 85 heat) plays 12 seeds to depth 4 in tests without dying, so every slot fires, pits and walls get kills, and the run is deterministic for a given seed and input stream.

## Benchmarks

Intel Xeon 2.1 GHz, 4 cores, Go 1.24.7 (`go test ./skill ./dungeon ./game -run '^$' -bench . -benchmem`):

| Benchmark | Time | Allocs |
| --- | --- | --- |
| Generate floor, depth 1 | 30 µs | 4 |
| Generate floor, depth 10 | 107 µs | 4 |
| Generate floor, depth 50 | 523 µs | 4 |
| Generate floor, depth 200 | 755 µs | 4 |
| Distance field, depth 20 floor | 11 µs | 0 |
| Dispatch: one cast that cascades into 35 firings | 5.5 µs | 0 |
| Evaluate full depth-2 hex | 3.3 µs | 0 |
| Game step (1/60 s), depth 1, 7 actors | 0.67 µs | 0 |
| Game step, depth 10, 22 actors | 1.3 µs | 0 |
| Game step, depth 30, 54 actors | 2.7 µs | 0 |

A whole frame of simulation at depth 30 is under 0.02% of a 60 fps frame budget. Rendering is the real cost on phones, and the floor draws as pre-rendered hex sprites with off-screen culling for that reason.

## Defaults picked in this pass

All placeholders; each is a constant or a field.

Floors
- Size W = 30 + 6*sqrt(depth) (max 90), H = 22 + 4*sqrt(depth) (max 64). Rooms 6 + 1.5*sqrt(depth) (max 22), radius 2 up to 3 + depth/6 (max 6).
- 25% chance each near pair of rooms gets a loop corridor; 35% of corridors are two wide.
- Pit clusters 2 + depth/2 (max 24); hazard vents start at depth 2.
- Enemies 5 + 1.6*depth (max 70), level = depth; elites +2 levels from depth 4 at 5% plus 1% per depth (max 30%).

Dispatch
- Chains stop after 3 cascades (`MaxChain`). On-hit procs have a 0.3 s per-slot cooldown by default (the slice uses 1.2 s for Kinetic Ram); on-hurt 0.5 s. Casts are limited by Power and Heat only.
- Wall slams and pit falls count as kills but not as hits, so terrain doesn't trigger on-hit hexes.
- Manual vent: dumps all Heat and locks the rig 1 s (overheat locks 3 s).
- Periodic default period 3 s.

Open design questions, answered for now
- Damage cap curve: unchanged, 25% of max health at even level, doubling every 5 levels.
- Leveling: the player's level is the depth. Health and damage grow 10% per level on both sides, so the cap and level gap do the work. Hex damage scales with level as if deeper loot rolls bigger numbers; Power and Heat costs stay at base.
- How hexes are learned: not in the slice. The loadout is fixed and built with `NewHex` (every edge linked) so it plays the same every run.
- Other shapes: only the six-slot hex.

Slice tuning
- Rig: 8 points in Capacity and Recharge, 0 in Cooling, 5 in Processing (164 Power, 20/s recharge, 6/s cooling). Heat, not Power, is what stops lance spam.
- Player: 220 hull at level 1, 4.5 tiles/s, strike every 0.45 s (16 plasma, splashes).
- Grunt 60 hp, 18 dmg, 2.4 tiles/s; skitter 32 hp, 11 dmg, 3.9 tiles/s; brute 150 hp, 34 dmg, 1.5 tiles/s. Enemy claws are capped by level gap but don't build the player's resistance.
- Hazard vents burn 5% max health every 0.5 s, ignoring the cap. Anything standing on one burns, enemies included.

## Not built yet
- Loot, learning hexes, and swapping the loadout. Minions (drone raises are off in the slice).
- Element combos beyond freeze-then-shatter, gated enemies, overkill reflect.
- Rule-bending rare parts (ignite neighbors, open a nested slot).
- Procedural loot rolls. Parts are made with constructors for now.
- Line of sight and fog of war. Enemies sense you through walls by walking distance.
- Enemies carrying hexes of their own. Right now they only claw.
- Saving a run, menus, art and audio. Everything on screen is flat shapes.

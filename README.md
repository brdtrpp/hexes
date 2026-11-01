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

The editor's layout test (every shape fits phone and desktop screens without overlaps) lives in `cmd/hexes`, which only builds for js here. It runs under node with a stub DOM:

```
GOOS=js GOARCH=wasm go test -exec=$PWD/tools/jstest/exec.sh ./cmd/hexes
```

(If node complains the command line is too long, run it under `env -i` with just PATH, HOME and the Go cache variables.)

## Controls

- Desktop: WASD or arrows to move (or hold the mouse button and point), Space casts every on-cast hex, R or Shift vents heat, Tab or E opens the hex editor (1 to 4 pick a hex, Esc closes), Enter redeploys after dying, Esc or M opens the menu.
- Touch: hold anywhere to walk toward your finger, tap CAST, VENT and HEX, and MENU (top right) for your record and the start-fresh button. Or tap MOVE (top right) to switch to STICK: press anywhere on the left half to plant a thumb joystick there, drag to steer (light push walks slow, 15% deadzone, the anchor follows your thumb past the rim). The choice is saved in the browser. The mouse follows the same mode.
- Strikes on an adjacent foe are automatic. Walk over a dropped part to pick it up. Walk onto the stairs to go down a level.
- Hex editor (game paused): tap a part, then tap where it goes. Pack to slot swaps loot in, slot to slot rewires the hex, slot to an empty pack cell takes a part out, SCRAP throws away the picked pack part. The hex recompiles on every tap; a refused edit says why.

## What's here

| Path | What it does |
| --- | --- |
| `skill/` | Hex compiler and evaluator (below), plus `dispatch.go`: trigger dispatch. |
| `dungeon/` | Floor generator: rooms, corridors, walls, pits, hazard vents, spawns, stairs, from a seed and a depth. |
| `game/` | Real-time simulation: free movement over the hex floor with wall sliding, enemy AI, the player's rig and loadout, terrain kills, effects. No rendering, so it runs in tests. |
| `loot/` | Drop rolls (rarity by depth, part kinds, values, quirks), the inventory grid, and part labels for the screen. |
| `cmd/hexes/` | The Ebitengine front end: drawing, HUD, keyboard, mouse and touch. `editor.go` is the hex editor. |
| `cmd/hexmap/` | Prints a floor as text. |
| `web/` | Browser page and wasm build. |

### skill package files

| File | What it does |
| --- | --- |
| `damage.go`, `default_types.json` | Damage types as data. The six seed types live in the JSON; adding one is a new row, not code. |
| `part.go` | Loot parts: triggers, targets, effects, multipliers, with rarity and Power/Heat costs. |
| `hex.go` | Templates, sockets, `Learn` (seeded layout roll), slotting parts and nesting hexes. |
| `shape.go` | Shapes: Triangle, Ring, Hub, Fork, Loop as control flow, their quirks, and who feeds whom. |
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
5. A gate splits a group in two and sends each half its own way (below).
6. Compile bakes all of that into flat steps. A nested hex becomes a `foreach` step followed by its inlined body, so combat never walks a tree.

## Gates (`skill/part.go`, `skill/hex.go`, `skill/compile.go`)

A gate is a part that tests a condition and routes the chain two ways. It is the first of the three pieces that make hexes Turing complete (gate, charge, call). Charges and Calls are below.

- **Which slots.** The slot right after the gate (clockwise) is its YES slot; the one after that is its NO slot. Each needs a linked edge to the slot before it, and neither wraps past the end of the ring back to the trigger. The editor tags them YES and NO.
- **Who gets tested.** The targets in the slot just before the gate, if linked. Otherwise the trigger's subject (the foe hit, the corpse, the nearest foe on a cast), or, inside a nested hex, the one entity it runs on. So a gate in a nested hex is "for each target: if ... then ... else ...".
- **What runs.** Effects and nested hexes in the YES slot land only on the ones that passed; in the NO slot only on the ones that failed. They ignore targets beside them. A branch nobody took runs on an empty group and does nothing.
- **Chaining.** A gate in a NO slot tests the ones that failed, an else-if: `if A then X, else if B then Y, else Z` fills a hex exactly. A gate in a YES slot tests the ones that passed (both must hold) and takes the next two slots as its own, so the outer gate has no NO slot.
- **Targets and multipliers in a branch slot** behave as they would anywhere: a target still selects and feeds its other neighbor; a multiplier still scales the slot before it. So a multiplier right after a NO slot boosts the NO branch, and one sitting in the NO slot boosts the YES branch.

| Condition | Tests | Default | Loot rolls |
| --- | --- | --- | --- |
| Hull | each foe: hull below X of max | 30% | 20 to 50% |
| Heat | your rig: Heat above X of max (the whole group goes one way) | 50% | 35 to 75% |
| Power | your rig: Power above X of max (whole group) | 50% | 35 to 75% |
| Status | each foe: burning (plasma), chilled or frozen (cryo), staggered (kinetic), eaten by toxin (nanite), marked (drone) | | any type with a status, not cloak |
| Crowd | each foe: at least N other foes within R hexes of it | 3 within 1 | 2 to 4 within 1 or 2 |

Defaults picked:
- Gates fit target and wild sockets. They cost 1 Power and no Heat; rarity cuts the Power (an exotic gate costs 0.6). Gates roll no quirk, so exotics are never gates.
- The cost of a gated hex counts both branches, even though each foe only takes one. Simpler to read on the HUD and it never under-charges.
- Heat and Power gates read the rig after this firing has been paid for, so a hex's own Heat counts.
- Status gates for cloak would test whether you are stealthed, but casting breaks stealth first, so loot never rolls them.
- Under the hood a gate is one `OpGate` step that splits a register in two. No jumps, no allocations; branches are ordinary steps on the two halves.

## Charges (`skill/part.go`, `skill/eval.go`, `skill/compile.go`)

A charge is a counter that keeps its value between firings. Each equipped hex has four banks, A to D. Charge parts write a bank; two gate conditions read one. Together they are a counter machine (a Minsky machine): increment, decrement, and "is it zero?". Calls add the loop, and with Heat paying for every call that set is Turing complete with overheating as the halting budget.

- **Charge part (CHARGE A +1).** Adds its amount to its bank each time the ring passes its slot. It counts whatever fired the hex: on-kill counts kills, on-hit hits, on-cast casts. Inside a gate branch it only counts when its half holds someone, so a charge in a YES slot counts the times the gate passed.
- **Drain part (DRAIN A -1).** Takes its amount off, never below zero.
- **Spend gate (SPEND A at 5).** If bank A holds at least 5 and the gate's group isn't empty, it takes 5 off and the whole group goes to the YES slot; otherwise A is untouched and the group goes to NO. A full gate with nobody to hit holds its charge until it has somewhere to go. With "at 1" it is the counter machine's decrement-or-branch-on-zero.
- **Empty gate (IF A EMPTY).** The whole group goes YES while A is zero, NO otherwise. It never changes A.
- **Same letter, same counter.** Every part in a hex naming A shares one counter, nested hexes included, and a hex run by a Call uses its caller's banks. Hexes firing on their own triggers never share.
- **Order.** Charges run in clockwise order with everything else, so a charge before a spend gate counts before the gate reads it.

Starter: **Cryo Bloom** now reads ON KILL, CHARGE A +1, CHILL, AROUND 2, SPEND A at 5, CRYO 45. Every kill chills what's around the corpse and charges A; the fifth sets off a cryo nova on everything within two hexes and empties A. The HUD card shows `A 3/5` with a strip filling toward the spend, and the editor shows `A = 3` on each charge tile.

Defaults picked:
- **When banks reset:** every new floor starts with all banks empty (Brad's call), and so does a new run. Edits on a floor keep the counts, and switching a part's letter doesn't move any.
- **Editor:** charges go in effect and wild sockets, charge gates where gates go. Pick a charge part or charge gate in a hex, then tap the center plate to switch it A to B to C to D. No typing.
- **Loot:** charge parts weigh 8 against gates' 10; a quarter roll as drains. Spend and empty gates join the gate table (weights 18 and 6). Parts roll bank A, or B three times in ten. Spend gates roll needing 3 to 8, one less per rarity tier above common, never under 2. Charges and charge gates roll no quirk.
- **Cost:** a charge part costs 0.5 Power and no Heat, a charge gate 1 Power like any gate; rarity cuts both. Like gates, the hex pays for every slot on every firing, so the nova's Power is paid on every kill (Cryo Bloom is 19 Power, -0.5 Heat per kill).
- **Limits:** a bank counts to 2^30 and stays there. An echo hex runs its charges twice. A basic strike has no bank: its charges count in a scratch bank that empties after each swing.
- Under the hood: `OpCharge` adds to `Ctx.Bank`, guarded by a register the way cloak is; spend and empty are `OpGate` conditions. The dispatcher keeps each slot's banks in `Loadout.Banks`, which the game points at `Sim.Banks` and clears when a floor loads. Still zero allocations per run.

## Calls (`skill/part.go`, `skill/eval.go`, `skill/compile.go`)

A Call part runs another of your hexes, or its own hex again, once the hex holding it finishes. It is the last of the three pieces (gate, charge, call): gates branch, charges remember, calls loop. With all three, hexes are Turing complete, with Heat as the step budget.

- **Which hex.** A Call points at itself (the tile says LOOP) or at one of your four hexes, shown by name on the tile ("to Swarm") in that hex's color. In the editor, pick the Call in a hex and tap the center plate to point it at the next hex: self, then each of the others. No typing.
- **Who it runs on.** It lands like an effect: on the targets beside it, or on its gate half if it sits in a YES or NO slot, or on the trigger's subject if nothing's beside it. The called hex runs once per entity there, with that entity as its subject, as if its own trigger had fired on them (an on-hit hex reaches out from the foe, an on-cast one from you). Its trigger part is skipped.
- **When.** Calls queue and run after the hex holding them finishes, in clockwise order. Nothing runs inside anything else, so a loop is a hex calling itself again and never builds a stack.
- **Counters.** A called hex uses the caller's charge banks, so a loop across hexes counts in one place.
- **Heat is the budget.** Every time a call runs a hex, that hex's Heat (at least 1, less for rarer Calls: x0.87 uncommon, x0.74 rare, x0.63 exotic) goes on the rig. Overheat and the chain stops right there, and the rig locks like any overheat. A loop with no way out ends in a shutdown, which is the point. Calls don't cost Power beyond the Call part itself (1 Power, 0.5 Heat per firing, like any slot).
- **Hard caps.** One firing runs at most 256 calls whatever the rig, and at most 32 can wait at once (more are dropped), so a frame can never stall.
- **Order matters.** Inside a hex the ring runs clockwise and a part only feeds its neighbors; across hexes, two Calls run their hexes in the order they sit on the ring. A test swaps two Calls and gets a different answer.

Why it's Turing complete: a counter machine (Minsky machine) is a list of states, each "add 1 to counter r and go to the next state" or "if r is zero go to one state, else take 1 off and go to another". Two counters are enough to compute anything a Turing machine can. Each state is one hex: `CHARGE r +1 · CALL next`, or `SPEND r at 1 · CALL next (yes) · CALL other (no)`. `skill/call_test.go` builds a six-state machine that multiplies (C = A x B, with B restored through D) and runs it at 3x4, 0x5, 5x0 and 2x9. Real limits apply: the slice has four hexes, four counters, 2^30 per counter, 256 calls a firing and about 100 Heat. Turing complete in the way a real computer is: the rules can express any program, the hardware runs out.

Starter: **Arc Lance** has a Call in its last slot pointing at Nanite Swarm. Cast it and the swarm goes off too; both slots flash and both hexes' Heat lands on the rig. A save from before Calls existed gets one Call in the pack instead (pointing at hex 4), once.

- **Loot:** Calls weigh 7 against charges' 8. Three in ten roll pointing at themselves, the rest at hex 1 to 4. No quirks.
- Under the hood: `OpCall` queues `{program, subject}` on a fixed 32-slot ring in the Evaluator; `Run` drains it after the first program, charging Heat through `Ctx.Rig` before each one. `Ctx.Calls` is the dispatcher's equipped programs. `Result` gains `Calls`, `Called` (which hexes ran, for the HUD flash), `Dropped` and `Halted`. Still zero allocations per run: the 3x4 multiply is 69 calls in about 2 µs.

## Shapes (`skill/shape.go`, `skill/compile.go`, `game/shape.go`)

A hex's shape is its control flow. The parts are the same in every shape; the shape decides which slots feed which and in what order they run. Every hex used to be a six-slot ring; that's now one of five.

| Shape | Slots | How it runs |
| --- | --- | --- |
| Triangle | 3 | A line: trigger, then two slots, no edge back. Runs on 25% less Heat. The cheap starter. |
| Ring | 6 | Clockwise from the trigger, each slot feeding the next. Unchanged. |
| Hub | 7 | A ring plus a center. Every ring slot feeds the center, which runs last. An effect there lands once on everything the ring targeted, pooled with no double hits (on the subject if the ring targets nothing). A multiplier there scales the whole ring. The center takes effects, multipliers, charges and calls, not targets or gates, and is never a gate's branch. Heat x1.2. |
| Fork | 7 | The trigger sits in the center and feeds two chains, slots 1-3 and 4-6, which run one after the other in the same firing. Neither chain feeds the other and gates stay inside their chain. Heat x1.1. |
| Loop | 6 | A ring whose last slot feeds back into the first. After the ring runs, it runs again once for each entity the last slot carries: its targets, whoever its effect or Call landed on, a gate's yes half, a charge's half in a branch. Each pass goes through the same queue as a self Call, so it pays the hex's Heat and counts against the 256-call cap. A last slot that carries nobody (empty, a multiplier, a charge outside a branch, a cut back edge, an empty gate half) ends it. `SPEND A at 1` in the last slot loops once per coin in A. |

**Learned.** Everyone has the plain Ring and Triangle. Hub, Fork, Loop, and quirked Triangles and Rings are found: rare drops from depth 5 down (0.6% a kill there, +0.2% a depth, capped at 4%, three times that from a foe above the floor's level), rolled on their own random stream so they never change which parts drop. The first time a save reaches depth 5 without one, a shape (never a plain Ring or Triangle) waits next to where you arrive, and keeps waiting until it's picked up. Walk over a shape to learn it: it's kept for good, like the rest of progress, and a death never takes it. Up to 32.

**Quirks.** Every found copy rolls one, on a slot or edge, so two Hubs differ:

- **Twin** (slot): the part there fires twice and costs twice. A multiplier there applies twice; a target just picks once.
- **Surge** (slot): every number of the part there is x1.5 (damage, push, duration, radius and count, or a multiplier's bonus).
- **One-way** (edge): the edge only feeds clockwise. The slot after still reads the one before; the one before stops seeing the targets after it.
- **Lean**: the whole hex runs on 75% of its Heat.

**In the editor.** The SHAPE button above the hex steps through the shapes you have: Ring, Triangle, then each found copy not already on another hex. Parts stay in their slots where the new shape has that slot and it takes them, the trigger moves to where the shape wants it (a Fork's center), and leftovers go in the first free slot that takes them, then into the pack (refused if the pack can't hold them). The caption names the shape and its quirk; a TWIN or SURGE slot gets a gold outline and tag, a one-way edge a gold arrow, a Loop's back edge a pink one, and small arrows on every edge show which way it runs. Tapping a picked charge or Call again switches its bank or target, which works in shapes with a center slot where the plate used to be. A hex's tab says TRI, HUB, FORK or LOOP under its trigger.

**Saves.** A hex saved before shapes has no shape field and six-slot arrays; it loads as a Ring with an empty center. Found shapes save in `Save.Shapes`.

Under the hood: `Hex.Shape` and `Hex.Feeds(a, b)` replace the ring-only adjacency, `Hex.Flow` gives the run order, `Slots` is now 7 (`RingSlots` 6 plus `Center`). Programs carry `Shape` and `Loops`; the Loop's back edge is one `OpCall` to self after the body (after an Echo copy, so Echo doesn't double it). Still zero allocations per run.

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

One generated floor at a time. The floor is hexes, but your rig moves freely in any direction (360 degrees, not snapped to the six hex directions), strikes whatever is in a neighboring hex, and carries a fixed loadout with one hex per trigger kind:

| Slot | Hex | Trigger | What it does |
| --- | --- | --- | --- |
| 1 | Arc Lance | on cast | Plasma chains 3 foes, x2 damage. 14 Power, 10 Heat. |
| 2 | Kinetic Ram | on hit | Shoves the foe 2 hexes, then a gate: under 40% hull it takes an 18 kinetic jab (YES), otherwise a 6 (NO). Kinetic adds a 1-hex shove either way. Walls slam, pits kill. |
| 3 | Cryo Bloom | on kill | Chills everything within 2 of the corpse and charges bank A; every fifth kill a 45 cryo nova hits them all. Cryo vents Heat. |
| 4 | Nanite Swarm | every 4 s | Toxin cloud on everything within 2. |

Movement (`game/move.go`):
- The player has a float position; its hex is whichever hex that point falls in (`skill.FromPixel`). Diagonal keys are normalized, so they're no faster. Touch and mouse walk toward the finger at full speed; analog input under full deflection walks slower.
- The body is a circle (radius 0.38, hex center to corner is 1). It slides along wall hexes by pushing out of each hex polygon. Hex walls are jagged, so when that push-out alone would catch the body in a notch, the step is redone along the wall's averaged line. Pushing almost straight into a wall is a stop.
- Enemies still step tile to tile. Each one blocks the player with a circle on its tile, sized so the player's center can never share a hex with an enemy.
- Hazards and stairs go off when the player's center is on the hex. Walking into a pit (center enters it) costs 25% of max hull, puts you back on the last tile you stood on and stuns you 0.5 s. Pushes into a pit still kill.
- Facing is your last movement direction, as a true angle. Cones and from-behind use it; targeting otherwise works from your current hex.

Enemies: grunts (average), skitters (fast, fragile, from depth 2), brutes (slow, tough, from depth 3), plus elites two levels up from depth 4 (white ring). They wake when you come within 10 steps on foot or when hit, chase along a distance field from the player (recomputed only when you change tile), and claw when adjacent. Stairs take you to the next depth with 30% hull back. Dying drops you back a few floors (see Permanent progress).

A headless bot (steer toward the next hex on the way to the stairs, cast at anything within 4, vent above 85 heat) plays 12 seeds to depth 4 in tests without dying, so every slot fires, pits and walls get kills, and the run is deterministic for a given seed and input stream.

## Permanent progress (`game/save.go`, `cmd/hexes/menu.go`)

Hexes is not a roguelike. The rig's points, the depth you've reached, your best depth, kills, the pack and every hex edit save on your device and come back when you reload: localStorage (key `hexes.save`) in the browser, a file in the user config folder (`hexes/hexes.save`) on desktop. Floors are the only temporary thing. Each depth always has the same layout for a save (the floor seed comes from the save's seed and the depth), but enemies respawn and loot rolls again on every visit, so reloading or walking a floor twice doesn't hand out the same parts twice. Charge banks still start empty on every floor.

Saving: the sim sets `Dirty` when something worth keeping changes (a new floor, a pickup, an edit, a death, a recovered cache). The front end writes then, at most once a second while you play and at once while paused. Nothing saves per frame, and the game step stays allocation-free. A reload starts you at the top of the floor you were on with the hull share you had. A save taken on the death screen already counts the death.

Start fresh: MENU (top right, or Esc / M) shows your record, and START FRESH wipes everything after a second tap on WIPE IT. It's the only way to lose progress.

Death:
- Fall: you come back at full hull `max(2, round(depth / 10))` floors up, never above depth 1. Depth 12 costs 2 floors, 30 costs 3, 50 costs 5, 100 costs 10. Level is depth, so it drops with you.
- Mercy: a death within 300 s of play of the last one costs less, from half (right after) back up to the full fall at 300 s, and never less than a floor. A bad patch can't spiral.
- Checkpoints: every 5th depth (5, 10, 15...) secures the pack. Parts picked up since the last checkpoint are unsecured; the pack line on the HUD counts them and the editor tags them NEW.
- Slotting secures: a part you put in a hex is safe from then on, and so is anything that comes out of a hex. Death only takes unsecured parts still loose in the pack.
- Salvage cache: the unsecured parts drop where you died, as a gold crate on that depth. Walk onto it and they go back in the pack (still unsecured until the next checkpoint); what doesn't fit lies on the floor. An arrow at the screen edge points to it on that floor, and the HUD says which depth it's on elsewhere. Die again before you reach it and it's gone for good; the new death leaves a new cache.

Tunable (all constants at the top of `game/save.go`): `CheckpointEvery` 5, `FallMin` 2, `FallFrac` 0.1, `MercyWindow` 300 s, `MercyMin` 0.5.

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
| Evaluate a gated hex (3 crowd checks) | 1.9 µs | 0 |
| Evaluate the charge nova hex (count, chill, spend) | 0.4 µs | 0 |
| Counter machine 3x4 through Calls (69 calls) | 2.1 µs | 0 |
| Evaluate a Triangle (chain 3, damage) | 1.5 µs | 0 |
| Evaluate a Hub (3 targets pooled into the center) | 2.7 µs | 0 |
| Evaluate a Fork (two chains, a crowd gate) | 1.8 µs | 0 |
| Evaluate a Loop making 8 extra passes | 7.6 µs | 0 |
| Game step (1/60 s), depth 1, 7 actors | 1.4 µs | 0 |
| Game step, depth 10, 22 actors | 2.2 µs | 0 |
| Game step, depth 30, 54 actors | 4.1 µs | 0 |
| Roll one drop | 0.1 µs | 0 |
| Swap a part into a hex and recompile it | 1.9 µs | 19 |

Game step numbers are from the free-movement pass, rerun on a busier machine (the tile-step version measured 1.0, 1.7 and 3.5 µs in the same run). The rest are from the first pass. A whole frame of simulation at depth 30 is about 0.025% of a 60 fps frame budget. Rendering is the real cost on phones, and the floor draws as pre-rendered hex sprites with off-screen culling for that reason.

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
- Player: 220 hull at level 1, 4.5 tiles/s (about 7.8 hex units/s), body radius 0.38, strike every 0.45 s (16 plasma, splashes). Walking into a pit: 25% hull and a 0.5 s stun.
- Grunt 60 hp, 18 dmg, 2.4 tiles/s; skitter 32 hp, 11 dmg, 3.9 tiles/s; brute 150 hp, 34 dmg, 1.5 tiles/s. Enemy claws are capped by level gap but don't build the player's resistance.
- Hazard vents burn 5% max health every 0.5 s, ignoring the cap. Anything standing on one burns, enemies included.

## Loot (`loot`, `game/loot.go`, `cmd/hexes/editor.go`)

Enemies drop parts when they die. The drop is seeded by the floor and the enemy, so a run drops the same parts whatever order things die in. Parts lie on the floor as diamonds in their rarity color and go into the pack when you walk over them. The four hexes start as the preset below and keep your edits for the whole run; the pack carries over between floors. Both reset on a new run.

Rarity (common, uncommon, rare, exotic) is weighted by depth. Rarity scales a part's numbers (x1, 1.15, 1.35, 1.6), and rare and exotic parts can bend a hex rule:

| Quirk | On | What it bends |
| --- | --- | --- |
| Radiant | multipliers | Scales every part in its ring, not just the one before it. +2 Heat. |
| Relay | damage and status effects | Arcs on to everything next to each target (damage at half, statuses at full duration). A target's target without spending a nesting level. Costs 1.5x. |
| Echo | triggers | The whole hex runs twice per firing; the second run picks targets again. Costs 1.25x. |

Rare parts roll a quirk 35% of the time; exotics always do (so exotics are never targets, which have no quirk yet).

Defaults picked (all constants):
- Drop chance: grunt 30%, skitter 22%, brute 55%, elites +45%.
- Rarity weights: common 72 falling 2.5 per depth to 20; uncommon 22 rising 1 per depth to 40; rare 5 rising 0.6 to 28; exotic 1 rising 0.3 to 14. Depth 1 is 73/21/5/1 percent, depth 10 is 52/33/11/4, depth 40 is 20/39/28/13.
- Part kinds: effects 40, targets 22, multipliers 22, triggers 16, gates 10 (relative weights). Damage per type: plasma 18, cryo 15, kinetic 10, nanite 28, drone 14, cloak 16, rolled x0.8 to 1.25. Multipliers x1.25 to 2 (all-verb x1.15 to 1.55). Periodic triggers every 2.5 to 5.5 s.
- Part values don't grow with depth; the level scaling that already grows hex damage does that, so depth buys rarity, not bigger base numbers.
- Pack: 12 cells. A full pack leaves parts on the floor and says PACK FULL. Parts left on a floor are gone when you go down. At most 96 parts lie on one floor.
- The CAST button and Space fire every on-cast hex, so a trigger swapped into any slot has a button.
- The trigger stays in the top slot: triggers only fit there and it can't be emptied.

Drop rolls and pickup run inside the sim step with no allocations. Recompiling on a swap allocates (the compiler builds a fresh program) but only runs from the paused editor.

## Not built yet
- Learning hexes (rolled layouts and broken edges are in `skill`, but the slice's four hexes are fixed). Minions (drone raises are off in the slice).
- Element combos beyond freeze-then-shatter, gated enemies, overkill reflect.
- More rule-bending parts (ignite neighbors, open a nested slot, bridge a broken edge).
- Line of sight and fog of war. Enemies sense you through walls by walking distance.
- Enemies carrying hexes of their own. Right now they only claw.
- Spending attribute points (the rig's points save, but the slice still starts everyone on the same rig). Art and audio: everything on screen is flat shapes.

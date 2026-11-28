# Physics: map rules, movement models, slopes

The new-physics programme (2026-11): Source movement, slopes and surfing, gravity
boots, water and a boat, and maps built on them. This file records the design as it
lands; each part adds its section.

## Map rules: a map pins server settings

`content.Map.Rules` (`map[string]string`, cvar name → value) pins server cvars for as
long as the map runs. A surf map plays with source movement whatever the server runs;
on the next map the server's own values apply again.

- Only the cvars in `content.RuleCvars` may be pinned (`TestMapRulesAreRuleCvars`), and
  every pinned value must be one rcon would set (`TestMapRulesAreValidCvars` runs it
  through `cvarValue`). Today: `sv_movement`, `sv_airaccelerate`, `sv_autobunnyhopping`,
  `mp_gravityboots`, and the house rules below (`mp_respawn`, `mp_respawntime`,
  `mp_loadout`, `mp_nade_refill`, `mp_buy_guns`).
- Maps pin with the builder: `mapgen.New(...).Rule("sv_movement", "source")`.
- Server: `Server.cvar(name)` is the value in force (the map's pin, else `s.cvars`);
  `cvarF` reads through it, so every numeric cvar can be pinned without more code.
  `s.cvars` stays the server's own value: rcon and the Server settings screen change
  that, and the announcement says when the current map pins it.
- Client: the map (welcome / `map` message, also `/maps.json`) carries `rules`; the
  `cvars` message carries the server's values. `web/game/rules.js`
  `effectiveCvars(cvars, map)` overlays them exactly as `Server.cvar` does, and is
  re-applied on both messages. No extra message: whichever arrives last, the client
  ends on the server's effective rules, and a prediction mismatch in between is
  reconciled like any other.
- Server settings shows a "pinned by <map>: <value> applies on this map" line under a
  pinned row; the control still edits the server's value (for the other maps).

## Movement models (internal/sim, generated to web/engine/sim.gen.js)

`sim.World.Move` (`sim.MoveRules{Model, AirAccel, AutoBhop}`) selects the model per
world. The server sets it every tick from the effective cvars (`Server.moveRules`,
via the shared `MoveRulesOf` in `internal/game/rules_shared.go`); the client sets the
same on its physics (`phys.setMovement(moveRules(effectiveCvars(...)))`), so the
prediction runs the same model bit for bit (`TestParityWithJS` runs both models,
source with auto bhop and air accelerate 150).

- **classic** (`MoveClassic`, default, unchanged): velocity eases toward the wish
  velocity at `groundAcc`/`airAcc` per axis; no friction, no air strafing.
- **source** (`MoveSource`, `source.go`): Quake/Source `PM_Friction`, `PM_Accelerate`,
  `PM_AirAccelerate`, in metres with Run (7 m/s) = CS's 250 u/s, so 1 u = 0.028 m:
  `sv_accelerate` 5.5, `sv_friction` 5.2, `sv_stopspeed` 80 u, air wish speed cap
  30 u (`airCap`), `sv_maxvelocity` 3500 u. Order per 120 Hz sub-step: jump (before
  friction, so a jump on the landing sub-step keeps all the speed: bunny hop), friction
  on the ground, then ground or air acceleration, then gravity and the box moves.
  Gravity (22 m/s²) and the jump speed (7.2 m/s) are classic's, so every map's jumps
  stay the same height in both models.
- `sv_airaccelerate` (default 12 = CS2; surf servers 100-150; clamped 0-1000; 0 = the
  default) scales air strafing.
- `sv_autobunnyhopping` (either model): holding jump jumps again on landing.
  `sim.Input.JumpHeld` (from `MoveInput`: jump held and allowed) carries the held
  state; `Input.Jump` stays the press edge.
- Max speed in source is the same `mx` as classic (run/walk/crouch × the held weapon's
  speed), used as the wish speed.

What it feels like (numbers from `TestSourceBhopChain` and the case
`bhop-strafe-chain`): accelerating to full run takes ~0.2 s (classic 0.1 s), letting
go stops you in ~0.3 s; strafe-jumping with the default air accelerate gains ~1-2 m/s
per hop (7 → 10 m/s in 5 hops on the case's mouse path, 7 → 17 m/s in 8 near-optimal
hops). Running into a wall at a shallow angle stalls, as in CS: friction beats the
sideways acceleration (`TestWalkToSites` steers along walls in source, as a player does).

Tests: `internal/sim/source_test.go` (ground speed and friction, air strafe gain vs
classic, bhop chain, air accelerate), `TestParityWithJS` (both models),
`TestWalkToSites` (every map's routes in both models), `internal/game/maprules_test.go`
(pins, rcon, runCmd), `web/game/rules.test.mjs`, cases `bhop-strafe-chain` (lab_bhop:
go, js, e2e) and `movement-cvars` (e2e: the rcon cvars on de_slop), scenario
`movement-lab` (map pins → the client's prediction and Server settings).

## lab_bhop

A 120 m runway lab (`changelevel lab_bhop`) whose rules pin `sv_movement source` and
`sv_autobunnyhopping 1`. Cases and scenarios use it; it has a golden file so the node
case runner can load it (`web/game/casemap.mjs`).

## Wedges (sloped solids)

A wedge is a `content.Box` with `Ramp` set (`RampPX`, `RampNX`, `RampPZ`,
`RampNZ`): a right triangular prism filling the lower half of the box, its
sloped face rising from the box's bottom edge on one side to its top edge on
the other (`RampPX`: low at `Min[0]`, high at `Max[0]`). Build one with
`content.W(...)` or the prefabs `prefabs.Ramp` (one wedge on a base) and
`prefabs.SurfRamp` (two steep wedges back to back: a surf ridge).
`lab_slopes` (`changelevel lab_slopes`, scenario `slopes-lab`) has ramps from
15 to 60 degrees, a deck with ramps down, a surf ridge and a target ramp.

**Why a field on Box, not a new solid list.** About thirty places on the
server and the client walk the map's boxes (bots, death drops, the radar, the
light grid, casings, spawn checks, ...). With a field they all keep working
and treat a wedge as its bounding box: conservative, never "a bullet passes
through a ramp". Only the code where the slope matters knows it:

| what | where | how |
|---|---|---|
| geometry | `internal/sim/wedge.go` | `IsWedge`, `SlopeNormal`, `Walkable` (normal y >= `MinGroundNY` 0.7, about 45.6 degrees), `TopAt` (slope height at a point), `HullTop` (where a player's hull stands), `RaySolid` (ray vs box or wedge: entry, exit, face; 3 = the slope), `FaceNormal`, `Ray` |
| movement | `internal/sim/physics.go` + `wedge.go` (generated to the client) | see below |
| bullets, wallbang | `WallTrace` (`wallbang_shared.go`) | `RaySolid`: the real path through the wedge sets the depth paid; entering a walkable slope is a floor (never through), a steep one is a wall |
| sight (bots, HE, flashes) | `mapClear` (`hitbox.go`) | `RaySolid` |
| grenades | `grenades.go` `nadeHit`, `slopeBounce` | overlap under the slope; a bounce off the slope mirrors the velocity about its normal (× `nadeBounce`), a walkable slope is a floor (friction, rolling downhill) |
| ground under a point | `floorBelow` (death drops), `groundTop` (bot nades), `floorUnder` | `TopAt` |
| bot walk grid | `ai_grid.go` | walkable wedges are floors at `HullTop`; steep ones aren't; `hullFreeSpan` knows the slope |
| client raycast (sparks, chase cam, sound occlusion), name tags, casings | `physics.js`, `tags.js`, `casings.js` | the generated `RaySolid` / `FaceNormal` |
| renderer | `web/engine/boxmesh.js` | the box with its top corners pulled down onto the slope: slope face (uv along its length), back face, two triangles, bottom |
| light grid | `web/engine/lights.js` | solid cells and blocked faces only under the slope |

Still the bounding box (fine for now): the radar's footprint, `segmentClear`
(bot straight-line checks: a ramp is in the way), spawn validity, `solidNear`.

In JS a plain box has no `ramp` field at all (`omitempty`), so shared Go code
tests `IsWedge(b)` (`b.Ramp > 0`), never `b.Ramp == 0` (`undefined == 0` is
false).

### Movement against a wedge

The hull stays an axis-aligned box. Against a wedge only one of its bottom
edges can touch the slope (the uphill one: the "corner"), so the overlap test
is the box test plus "the corner is under the slope" (`slopeDepth`).

- **Walking up** (on the ground, walkable): moving into the slope lifts the
  hull onto it like a step (`hitWedge`), without the step's camera smoothing:
  the rise is continuous.
- **Standing / landing** (walkable): falling into it puts the feet on the
  slope (`HullTop`), on the ground, vertical speed 0: no sliding.
- **Walking down**: leaving the ground without jumping snaps the body down
  onto a walkable slope within `hypot(vx, vz)·dt·1.1 + 1 cm` (`snapRamp`), and
  onto anything under it when it walked off a slope (the foot of a ramp).
  Walking off a box onto a box still falls, so classic movement on box maps
  is bit-identical (simdiff).
- **Steep slopes** (normal y < 0.7): not ground. Touching one pushes the hull
  out along the normal and removes the velocity component into it (Quake's
  ClipVelocity, `pushOff`): you slide off it. Running at one gives a little
  upward speed and you fall back. Surfing is this plus air control (below).
  A hull already a hair inside a wedge before a horizontal move (another
  solid's push left it there, as at a surf ridge's peak between its two
  wedges) is pushed out along the normal too, never stopped as by a face.
- **Vertical faces** (the high end, the triangular sides): blocked along the
  axis like a box's face; a low side under a step height is stepped onto.

Tests: `internal/sim/wedge_test.go` (geometry, walking up and down, standing,
sliding, faces, the snap only for ramps), Go/JS parity on `lab_slopes`
(`physics_test.go`), `internal/game/wedge_test.go` (wallbang, sight,
grenades), `ai_grid_test.go` `TestGridPathRamps`, `web/engine/wedge.test.mjs`,
`boxmesh.test.mjs`, `lights.test.mjs`, scenario `slopes-lab`.

## Surfing and surf_scope_creep

Surfing needs no code of its own: a steep slope is not ground, so no friction
and no jump; touching it clips the velocity into it (`pushOff`), and gravity
pulls you down along it. In the air, source movement's air acceleration (up to
`airCap` along the wish direction, scaled by `sv_airaccelerate`) lets a strafe
key held into the slope cancel the slide: you ride along the ridge at your
speed, slowly climbing, as in Source. Let go and you slide off its side. Turning
with the strafe adds speed as in bunny hopping. Classic movement has no air
control worth the name, so surfing is a source-movement thing.

**surf_scope_creep** (`internal/content/maps/surf_scope_creep.go`; Competitive,
defusal; a `surf_*` name is allowed for a defusal map, `ModeProblems`). Its
Rules pin `sv_movement source`, `sv_airaccelerate 150`, `sv_autobunnyhopping 1`.
The project's scope grew until the roadmap became four Gantt-chart surf ridges
(60 degree sides, material `gantt`) over a void of deprecated code (material
`deprecated`, at y -21; falling under y -20 respawns you at your spawn, as on
every map). Each team starts on a deck at y 12, runs off its front edge onto a
ridge and strafes along it to a site (y 4). The slow way is the legacy path: a
ramp down from the deck to a long, narrow bridge through the middle. The sites
are nearer the Artisans' deck (z -30), so defenders get there first; the Vibe
Coders' ridges are the long ones. Bots take the legacy path (they don't surf).

Bots: `segmentClear` (a straight walk between nav nodes) walks over walkable
ramps, blocked only where the slope under the disc rises more than a step over
the walk's own height (`rampClear`).

Light grid: the void widens the grid's height; the map is ~9M cells (~0.3 s of
sky visibility at load), so the void stays only as wide as can be seen.

Tests: `TestSurfRidge`, `TestWedgeAlreadyInsideNoStop` (`internal/sim`),
`TestSegmentClearRamps` (`internal/game`), case `surf-ridge` (go, js, e2e: off
the deck onto the A ridge, strafe only, rides it; let go, slides off), scenario
`map-surf_scope_creep` (surfing predicted in sync, snapshots of the ridges and
the void), the map's golden file and the map tests every map gets.

## Per-player gravity (Gravity Override, part 1: the sim)

`internal/sim/gravity.go`. Every body falls along one of the six axis
directions, `Body.Grav` (`GravDown` 0 = the world's -y, `GravUp` 1, `GravNX` 2,
`GravPX` 3, `GravNZ` 4, `GravPZ` 5). The movement code itself knows only -y:

- Each gravity has a **frame**, a quarter-turn rotation (a signed permutation
  of the axes, `ToFrame` / `FromFrame`) taking that gravity to -y. A body with
  its own gravity steps in its frame: its position and velocity are turned in,
  the world's static boxes (turned once per gravity and cached on the World),
  the movers (per sub-step, as they move: below) and the other players' hulls
  are turned in, the unchanged `step` runs, and the body is turned back.
- **Movers in a turned frame** (`movers.go movers`, the frame's `fg`): every
  sub-step the movers near the body (`nearTurned`: a cube round its feet, any
  way up) have their boxes at t0 and t1 and their travel turned into the
  frame, and the same riding, lifting and shoving run as on a mover's top in
  the world's gravity. So a body standing on any face of a mover is carried
  with it: along the face (a sled's side), or out and back with it (a sled's
  front, a face moving along its normal), standing still on the same spot;
  and a mover sliding along the wall it stands on shoves it. Tests:
  `TestRotatedBodyCarriedAlongTheFace`, `TestHookMoverFaceAlongItsNormalFlips`,
  `TestRotatedBodyShovedByAMover`; parity in `TestHookParityWithJS` (de_rackmount).
- Permutations and sign flips are exact, so the round trip loses nothing and
  Go and the generated JS stay bit for bit (`TestGravityParityWithJS`). A body
  in `GravDown` takes the old path untouched: every existing trace is
  identical (simdiff).
- `TestGravityRotatedRunsMatchTurned`: a body with gravity g in a map turned
  by g moves exactly as a normal body in the map, turned.
- The hull stays an axis-aligned box (its dimensions swap); a player's hull
  (`HullBox`, `sim.Peer.G`) is turned with its own gravity, so players on a
  wall collide with players on the floor as the boxes they are.
- v1 limit in a turned frame: a wedge collides as its bounding box.
- Walking over a box's edge in a turned gravity wraps you onto its next face
  (Edge wrap, below).
- Yaw and pitch are the body's own, in its frame: a flip keeps them.

Wire: `gv` in the self-snapshot (`me.gv`, reconciled like the body), on each
player in full snapshots and in pose rows (flags >> 2), so the predictor
collides with turned peers as the server does. Cases set it per player
(`"grav": 3`); `lab_boots` is a closed cube with something on every face
(`sim/cases/grav-walls.json`).

### Part 2: the view, the model and the hits

- **Eye and aim.** `sim.EyePos` is the eye out of the floor the body stands
  on (`sim.Above`: along its up; with the world's gravity the old sum, the
  same bits). The view's yaw and pitch are in the body's frame: the server
  turns a shot's direction out of it (`simtick.go ray`), and the throw
  (`NadeLaunchG`) and the floor-weapon prompt (`AimedItemG`, shared with the
  client) run in the frame. Bots, radar spotting and flashes aim at and see
  from points along each player's up.
- **Hit test.** `toPartFrame` (shared) turns the ray into the target's frame
  before its yaw: a player on a wall is hit as the upright model turned with
  it. `TestHitTurnedWithGravity` (exact against the upright model in its
  frame), `TestHitParityWithJS` and `TestPoseParityWithJS` (drawn corners)
  cover all six gravities; case `grav-shoot` shoots wall to wall on a line
  an upright model would not be on.
- **Drawing.** `boxmodel.js poseFrames` puts the gravity's frame between the
  position and the yaw (as the hit test), so models, hitbox outlines, name
  tags, muzzle lights and casings all stand out of their wall.
- **Camera roll** (`web/engine/gravity.js`): the camera eases (quaternion
  slerp, `ROLL_RATE` 9/s) from the old frame to the new gravity's; the eye
  swings round with it. `fpsView` / `basis` take the frame; the viewmodel and
  the listener turn with it. A spawn snaps back to the world's. Render only:
  movement, shots and hits use the exact frames. Spectators in first person
  see a turned player's view in its frame.
- Scenario `gravity-boots`: on the wall, seen from the floor, mid-roll, on the
  ceiling. The test API's `set` takes `grav`.

### Edge wrap (`internal/sim/gravity.go edgeWrap`)

In a gravity of its own, walking over the edge of the box you stand on turns
you a quarter turn round that edge onto the box's next face, your gravity
into it: up the outside of a ship's hull, over the top edge, and you are on
the deck (or a rail's top, and a step later on the deck) instead of falling
across the ship.

- Where: the body was on the ground on a static box (in its frame: on its
  top), walked off (no jump, the hook not on), and its hull is now wholly past
  exactly one edge of that box. Not when a floor is within 1 m below (a step
  down) or the turned hull would overlap anything. Never in the world's
  gravity: there a ledge is a drop, as always.
- What: the feet go onto the side face, as far down it as they were past the
  edge; the speed over the edge becomes speed down the face (falling into
  the face is dropped); on the ground at once. Exact (no products), so Go
  and the generated JS wrap alike (`TestEdgeWrapParityWithJS`).
- The view turns with the body: the yaw changes by the quarter-turn multiple
  that keeps it looking where it looked (`Events.Turn`, `wrapTurn`: the
  edge's line is the same in both frames). The client adds it to its view
  yaw on its predicted wrap and twists the camera's eased frame back by it
  (`engine/gravity.js twist`), so the picture doesn't jump; the roll that
  follows goes round the edge. The gravity-flip sound plays.
- Tests: `TestEdgeWrapEveryEdge` (every face of a cube in every gravity,
  every edge: the face, the gravity, the speed, the view),
  `TestEdgeWrapHullOntoDeck`, `TestEdgeWrapNot` (jump, step down, no room, the
  world's gravity); the twist's in `gravity.test.mjs`; case `grav-edge-wrap`
  (de_hostile_takeover: up the hull's side, over the deck rail, onto the deck).

### Part 3: the hook (`internal/sim/hook.go`)

A Quake-style grappling hook, worn with the boots (`Body.Boots`), bound to
`+hook` (F by default, `BtnHook`):

- **Fire** on the button's down edge: a line from the eye along the view
  (in the body's frame) latches onto the first solid within `sv_hook_range`
  (any solid face, wedges too). Every shot costs `sv_hook_cost`, hit or miss.
- **Pull** while held: up to `sv_hook_pull` m/s along the rope (60 m/s²,
  beating gravity, which still pulls), off the ground when the rope points
  up. The rope only shortens: taut, the body can't move away from the
  anchor, so it swings under it. Let go: the momentum stays.
- **Energy**: holding drains `sv_hook_drain`/s; at 0 the hook lets go; it
  recharges `sv_hook_recharge`/s while not hooked, up to `sv_hook_energy`.
- **Flip**: touching the hooked face (a box face, not a wedge: v1) turns the
  body's gravity into it: it stands there (the hull's middle on the face),
  if there's room; the hook lets go. Hook the floor to come back down.
- **Movers** (`MoverSlide` sleds and lifts, escalator treads) are hooked like
  any solid, traced where they are at the command's time (`World.T`, as body
  collision), so the server and the prediction latch the same face. The hook
  keeps the mover (`Hook.Mover`: index + 1), the tread (`U`: an escalator
  tread's u less the treads' travel, a whole number) and the anchor relative
  to the box's moving origin (`Local`, `moverOrigin`); every command, and at
  the step's end for the flip, the anchor and the hooked box are put back
  where the mover is (`hookFollow`): the rope rides with it. Touching any
  face of a mover flips onto it as onto a static face (the flip's room check
  sees the movers too), whether it slides within its own plane (a sled's
  side) or along its normal (a sled's front, a lift's underside): the body
  then stands, walks and is carried on it (Movers in a turned frame, above).
  A lift's top in the world's gravity is just the floor (no flip, ride it).
- **Where it lands**: the new feet are the hull's middle on the face, moved
  in so the whole footprint is on the face (the body was pulled in low or to
  one side; a face narrower than the footprint: its middle): it stands on
  the face, not on a sliver of its edge.
- A tread that goes under its comb plate, or a mover a map edit removed, lets
  go. All of it is in the self-snapshot's `hk` (`mv`, `u`, `lc`).
- Everything is in the body (`Boots`, `Energy`, `Hook`), stepped by the shared
  code and carried in every self-snapshot (`bt`, `en`, `hk`): predicted and
  reconciled like the movement. Others see the rope from `hk` on each player
  in full snapshots (pose rows leave it out: it is drawn, never hit).
- Cvars `sv_hook_range|energy|recharge|drain|cost|pull` (defaults
  `sim.DefaultHook`), all in `content.RuleCvars`: a map may pin them.
- Sounds: `hookFire`, `hookMiss`, `hookRelease`, `gravFlip` (Events.Hook).
- Tests: `hook_test.go` (pull and flip and back, energy, release momentum,
  wedge, rope never lengthens, no room no flip, a mover hit and followed, a
  mover face along its normal flipped onto and ridden, an escalator tread, Go/JS parity on
  lab_boots, de_slop and de_rackmount with runs aimed at its sleds); cases
  `hook-flip` (through the real key, e2e too), `hook-mover` (a sled's front
  whatever its phase: flips onto it and rides it out and back; e2e too),
  `hook-mover-flip` (onto a sled's side and carried with it, go/js: the clock
  pinned) and `hook-cvars`; scenario `gravity-boots` (pulled, the rope seen from the
  floor, flipped). The test API's `set` takes `boots`.
- v1: a body in a gravity of its own doesn't swim (water is the world's: its frame has none).

### Part 4: the item, the rule, the HUD

- **Gear** `boots` ("Gravity Override", $1000, `economy.json`): for sale only
  while `mp_gravityboots` is 1. The cvar is in `content.RuleCvars`, so a map
  may pin it (`lab_boots` does). The buy menu lists it under the same rule
  (`buyCategories(team, rules)`); the server's `purchase` refuses it otherwise.
- The purchase is the player's (`Player.Boots`, server only); every body the
  server makes for that player wears it, meter full (`fitBoots`: spawn,
  first command, out-of-world reset). Dead: gone; a survivor keeps it into
  the next round (`carried.boots`) with a fresh meter, back in the world's
  gravity. Bots don't buy it (v1).
- **Out of the world**: below y -20 as ever, and now also 30 m past the map's
  boxes in any direction (`outOfWorld`): a body that walks off a ceiling in
  `GravUp` falls up forever otherwise. The reset body keeps the gear.
- **HUD**: an energy meter next to HP/armor while worn (the predicted
  `me.energy`, so it moves without waiting for a snapshot), orange under 20%.
- **For everyone**: `mp_gravityboots 2` puts it on every body, free (not sold):
  the hook on by default (`bootsForAll`; de_serverless pins it). The case
  harnesses fit it the same way.
- Tests: `TestBuyGravityOverride`, `TestGravityOverrideForAll`,
  `TestOutOfWorldEveryWay`, buyrules tests; case `buy-gravity-override` (the
  console's `mp_gravityboots` + `buy boots`, e2e); scenario `gravity-boots`
  shot `hooked-hud`.

### The map for it: de_rackmount

The inside of one server, scaled up until the RAM is cover
(`internal/content/maps/de_rackmount.go`). The motherboard is the floor and
the next board up is the ceiling (16 m). The rack's side panels are the long
walls, and a house-sized case fan fills each end. Its Rules sell the Gravity
Override (`mp_gravityboots 1`). Every surface has its own cover:
- **Floor:** three lanes between blade-server rows, a finned CPU heatsink in
  mid with its fan on top, RAM sticks on both sites, capacitors.
- **Side panels:** RAM sticks plugged in sideways (3 m out), vertical cable
  looms, and four hot-swap drive sleds (`MoverSlide`, 4..5.5 m up: over the
  floor's heads) that slide 3 m out and back every 6 s. They carry and shove
  whoever stands on the panel.
- **Ceiling:** hanging RAM sticks and the cable looms along its edges and
  across mid.

Bots stay on the floor routes (`TestWalkToSites`). Two map fields came with
it, both renderer-only:
- `Map.Fog` (`mapgen.Fog`): the map's own fog density and colour, in place
  of the palette's warm outdoor haze. Here a dark, dusty blue.
- `Map.Cut` (`mapgen.Cut`): with the eye above it, the renderer leaves out
  the boxes wholly above it (the lid), so the Maps preview and the
  spectators' overview see inside. They stay solid.

Lights fall off as 1/d² (`web/gen/glsl/lights.js`). In a 16 m tall room, the
mid-air points need about 110 to light a surface 8 m away, and the ceiling
spots about 700. Tests: `TestDeRackmount` (rules, the lid is the ceiling board
alone, the sleds' strokes clear every static box), boxmesh `splitCut`;
scenario `map-de_rackmount` (in SMOKE; on the side panel, on the ceiling).

## House rules: grenades only (he_merge_hell)

A map can change what the game is, not just how you move: he_merge_hell pins team
deathmatch with grenades and nothing else.

- `mp_loadout nades` (an enum: `default|nades`): every spawn (respawn, changelevel)
  gets the keyboard and one of every grenade, the team's own fire grenade included,
  the HE in hand (`Registry.NadeLoadout`, internal/game/nades.go).
- `mp_nade_refill <s>`: a grenade slot that went empty comes back full that many
  seconds later (`tickNadeRefill`: the server writes the inventory, the client adopts
  it from the next self-snapshot like any buy; 0 is off). Only with `mp_loadout nades`.
- `mp_buy_guns 0`: the buy menu and `buy` sell armor, kits and grenades, no guns.
- `mp_respawn 1` + `mp_respawntime 2`: team deathmatch. `Map.NoBomb()` says so; such a
  map's sites are only where the bots go to fight (no bomb rounds, the defuse/site-hold
  tests skip it).
- Maps named `he_*` must pin `mp_loadout nades` (`ModeProblems`); the Maps screen lists
  them under the new "Fun" category.
- `Map.Pits`: deadly volumes. A body inside one dies at the end of its command, a world
  kill named after the zone; the kill feed shows "merged into main" (glyph ⤓).
- LGTM walls (material `lgtm`) send grenades back at `lgtmBounce` 0.92 (any other
  surface: 0.45): approved without reading, so bank shots are the game.
- Bots throw at a seen enemy 7..aiEnemyNadeMax m away, HE then fire then flash, again
  as each refills; they don't lob grenades into empty sites. On bridges the path code
  checks for floor (`floorAlong`): no cutting a corner over the pit, and the stuck
  sidestep only steps onto floor.
  On a map with pits, `aiEdgeGuard` stops any bot command that would walk (1.2 m
  ahead) or jump (2.5 m) off the floor: fights, chases and strafes took bots over the
  edges and over the rails. People fall in; bots don't (`TestBotsStayOutOfMain`).
- The side branches have knee-high LGTM rails ("required reviewers", 0.7 m: over a step,
  under a jump) on their outer edges; the middle line and the merge commit have none.

## Water (internal/sim water.go)

`content.Map.Water` lists water volumes (boxes of kind `water`; mapgen
`Builder.Water`): a volume's top is its surface. A body's water level
(`World.WaterLevel`): 0 dry, 1 wading (feet under: max speed x0.8), 2
swimming with the head out, 3 under.

Swimming (2 and 3), the same in both movement models: no gravity, no ground.
The wish direction follows the view (`Input.Pitch`), jump held adds up, crouch
adds down; at `SwimSpeed` 3.6 m/s (walk halves it, the weapon's speed scales it).
With nothing held you sink at 0.6 m/s. Velocity approaches the wish at 10 m/s²
plus 12 x the difference per second (a plunge off the quay stops ~2 m under).
With the head out you can't swim higher than the surface. Jump held while
swimming forward into a ledge whose top is within 2.6 m of the feet is a water
jump (10.5 m/s up, against gravity, not the water): out onto a quay. A tall
wall (a ship's hull) has no ledge: nothing happens. `Events.Splash` is the
entry speed when the feet go in (the client plays `splash`).

The client: `Input.Pitch` comes from the command (`cmd.js` moveInput: the
client's commands carry `pitch`, the wire's `p`). The renderer draws each
volume's surface as a 2 cm slab of material `water` (animated; from below an
emissive sky tint), and with the eye in a volume the fog turns to water
(`WATER_FOG`, thick) and the sky isn't drawn. Bullets, sight and grenades pass
through water (v1). In a rotated gravity frame (boots) there is no water (v1).

**lab_harbor** (`changelevel lab_harbor`): a quay at y 0, a 3.4 m deep basin
(surface y -0.6) with a pier and a slipway, and a 16 m ship's hull to the
south.

Tests: `internal/sim/water_test.go` (levels, plunge and sink, surfacing,
along the view, water jump, none at a wall, wading), Go/JS parity on
lab_harbor (`physics_test.go`, swimming 2000+ ticks), case `swim-harbor`,
scenario `harbor-swim` (under water, the surface from below, the water jump,
predicted in sync).

## Boats (internal/sim boat.go, internal/game boats.go)

The inflatable dinghy: `content.Map.Boats` (`BoatSpawn`: waterline centre and
heading; mapgen `Builder.Boat`; optional per map) are where each round's boats
start (`resetBoats` at the round start and on a map change).

**The sim** (`World.StepBoat`, shared: the driver's prediction runs it): state
`Boat{Pos, Vel, Yaw, YawVel}`, input `BoatInput{F, R}` (throttle, tiller).
Thrust along the heading (7 m/s² to `BoatMax` 11 m/s; astern 4 m/s² to 4 m/s),
drag 0.35/s along and 3/s sideways (it carves, it doesn't skid), the tiller
turns it up to 1.5 rad/s (scaled by speed, a quarter at rest; astern swaps).
On the water it sits on the surface (bobbing is drawn only). The hull is two
discs (radius 0.85 m, 0.95 m fore and aft of Pos) at the waterline (0.35 m
under to 0.6 m over) against the map's boxes (a wedge as its box): pushed out,
the speed into the box removed, the turn halved. So it comes to rest against
a quay or a ship's hull under thrust, and slides along one at an angle.

**The server** steps every boat nobody drove each tick with no input (it
drifts to a stop) and sends them in every full snapshot (`"boats"`: id, `p`
mm, `y` angle, `v` mm; `snapMsg.Boats`, both codecs, replays). The client
draws them (`web/game/boats.js`, model `web/assets/models/boat.js`: grey
tubes, a pink "SERIES A" stripe, a lime outboard) run ahead by their velocity
from the last snapshot, bobbing. Its floor is solid to bodies (the deck,
below); no cover from bullets (v1: it is mostly air). MCP debugger:
`boat_set` (move one, list them).

**Riding** (`internal/game/ride_shared.go`, shared; `boats.go`). Three seats:
the driver's at the outboard (seat 0), two passengers forward of it. E within
2.6 m of a boat (feet, x and z; not in a bomb site, not on a floor weapon or a
defuse) takes its first free seat, the driver's first; the server decides
(who sits where) and the client adopts it from its self-snapshot (`me.ride`,
`me.seat`, and `me.rb`: the boat's exact state after the acked command). A
rider's body stays a world body, seated (crouched) at `SeatWorld`, moving with
the boat: shots, throws, hits, rows and bots see it where it is, nothing else
changes. `RideStep` is a rider's command: the driver's move keys are the
throttle and the tiller (`StepBoat`), so the driver predicts the boat exactly
(it is drawn at its predicted pose: `ownBoat` in main.js); a passenger's
command leaves the boat alone and the server seats them where the boat is
after the tick (their client draws them on the boat as drawn, and the
lockstep harness doesn't compare a passenger's body: `_passenger`). E or a jump
press gets you up (`RideLeaves`, `RideGetUp`): standing on the deck where you
sat (a jump hops up from it), predicted on both sides; that E sits you nowhere
again. Firing the Gravity Override's hook (`+hook`) from a seat pulls you out
into the map (`RideLeave`, with the boat's velocity), and the same command
fires it on foot: drive to a ship's hull (the boat
rests against it), hook the hull from your seat, and touching it puts you on
it (case `boat-hook-hull`). Death, a respawn, a teleport and the round's reset put riders out.
Riders can shoot and throw as on foot (the seat's crouched pose).

**Seats** (`boats.go` `freeSeat`, `switchSeat`; server only, the client adopts
them from `me.seat` like boarding). E (or `+seat`) standing on a deck sits you
down in that boat: at the tiller if nobody drives, else in the free passenger
seat nearest to where you stand; nothing free, nothing happens. `+seat`
(`BtnSeat`, default X, "Boat: switch seat") while seated moves a passenger to
the free tiller and the driver to the free passenger seat nearest the stern
(seat 1 first); the tiller taken or every seat full: nothing.

**The deck: standing on a boat** (`internal/sim boat.go` `Deck`, `StepDeck`,
`LandOnBoat`; `ride_shared.go` `DeckCarry`, `DeckLeaves`). A body that is not
seated but on a boat has seat `SeatDeck` (-1) and a place in the boat's frame
(`sim.Deck`: feet and velocity, x to starboard, y up from the waterline, z
aft). Its command runs the ordinary movement (`World.Step`, either model:
walk, jump, crouch) in a tiny world of its own, a floor under the whole hull
at `DeckTop` (0.1 m over the waterline), with the view's yaw taken relative to
the boat's heading; its body in the map is that place on the boat
(`DeckWorld`, `DeckVel`: carried with the boat, turning with it). Shots,
throws, hits, rows and bots see the body there, as a seated rider's.

- **Landing**: a command on foot that takes the feet from above the floor (or
  from up to 0.3 m under it: walking on from a pontoon) down onto it, with the
  centre over the hull (the two discs' stadium, `OnHull`), lands on the deck
  (`LandOnBoat`, after `World.Step`; not flying, hooked or in a gravity of its
  own). Jumping in from the quay or a ship lands on the floor; swimming under a
  boat doesn't.
- **Leaving**: the centre walking off the hull's outline (over the side, the
  bow, the transom) puts you in the map again, moving with the boat (in the
  water, or onto a pontoon). The hook (boots) fired on the deck takes you off it
  and fires on foot in the same command. E sits down (above); a jump jumps on
  the deck.
- **Not in the deck's world**: the map's boxes (the hull's discs keep clear of
  them) and other players (bodies on a deck don't push each other or the
  seated; the others' commands do collide with a body on a deck, where it is in
  the map). v1.

*Why the boat's frame* (and not a moving solid like `Map.Movers`): a mover's
boxes are a pure function of time, so the server and every client's
prediction put it in the same place for any command. A boat is simulated
state stepped by its driver's commands, which only the driver's client knows
ahead of the server. So the deck's movement doesn't depend on where the boat
is, only on its heading (the view's yaw relative to it), and the state that
is predicted and reconciled is the place in the boat's frame (`me.dk`, exact,
with `me.rb`, the boat's exact state). To make that order-free on the server,
every deck command of a tick stands on the boat as the tick started
(`Boat.start`: whoever's command runs first, the driver's or the rider's),
and `tickBoats` then carries every body on a deck to where the boat ended the
tick (as it seats the seated); in the client's prediction `me.rb` is that same
pose (the boat after the acked tick = as the next one starts). A boat nobody
drives (`rb.rd` false) drifts, and the client steps it with no input after
each predicted command (as `tickBoats`), so a body on a coasting boat is
predicted exactly. Under someone else's driving the client can't know the
boat's next pose: it draws itself on the drawn boat (as a seated passenger)
and the lockstep harness doesn't compare its body or deck place
(`_passenger`); turning under you changes your heading on board until the
next snapshot (small, reconciled). Your view turns with the boat you ride
(main.js `rideView`: by the drawn boat's change of heading), so your heading
on board stays as the boat turns.

**HUD** (`web/game/boatseat.js`, `BoatBadge` in `ui/hud.js`): in a boat a badge
says the role and the keys from your binds: "DRIVER · you own the roadmap"
(throttle, tiller, X passenger seat, E stand up), "PASSENGER · SEAT n · along
for the ride" (X take the tiller when it is free, E stand up), "STANDING ROOM
ONLY" on the deck (E sit down, walk off the side to leave), and on foot within
E's reach of a boat "E sit down (dinghy)".

Sound and look: a moving boat's outboard buzzes (`web/assets/sounds/boat.js`
`outboard`: a two-stroke saw and pulse, revs and churn from its speed,
positioned, within 45 m: `boats.js` `sound`), and it leaves a foam wake (a V
growing with speed, `boatParts`).

Riding tests: `boats_test.go` (`TestSeatWorld`, `TestRideStepAndLeave`,
`TestBoardingRules`, `TestDropOntoBoat`, `TestSeatSwitching`, `TestRideGetUp`,
`TestDeckCarriedByTheBoat`), `internal/sim/boat_test.go` (`TestDeckFrame`,
`TestLandOnBoat`, `TestStepDeck`, `TestDeckParityWithJS`), case `boat-drive` (go, e2e: board, a passenger, full
ahead, the tiller, a hop up onto the deck and off the side, E up and off; the driver predicted in sync every tick),
case `boat-deck` (go, e2e: off the quay onto the deck, a jump on it, walking
forward, another body landing, E to the tiller, carried while it is driven and
while it coasts, every `+seat` switch, E up and off the side),
case `boat-hook-hull` (go, e2e: to the hull, hook it from the seat, on it),
`boats.test.mjs` (drawing, the outboard, the wake, adopting and replaying a
ride, landing, walking and drifting on a deck), `boatseat.test.mjs` (the
badge), scenario `harbor-boat` (the driver's, the passenger's and the quay's
views, up on the deck and overboard, at rest against the hull), scenario
`harbor-deck` (the badge near a boat, on the deck, driving, a passenger's, the
tiller free; the seat switches).

Tests: `internal/sim/boat_test.go` (thrust and drift, turning, resting against
the hull and sliding along it, the pier, Go/JS parity), `internal/game/boats_test.go`
(drift to rest, the round reset, the snapshot, `boat_set`), scenario
`harbor-swim` (a moored dinghy).

### The map for it: de_hostile_takeover

A startup (the Vibe Coders) boards Megacorp's container ship MV Synergy (the
Artisans) from its quay (`internal/content/maps/de_hostile_takeover.go`). The
quay (y 0) faces an 80 x 20 m hull across a 20 m basin (water surface y -0.6);
the deck (y 8) carries two-high container rows either side of midships, where
the Artisans spawn; A is the foredeck, B the afterdeck under the bridge. Its
Rules sell the Gravity Override (`mp_gravityboots 1`), and four dinghies lie
at the quay. Three ways aboard:

- **The loading pier** (west): a ramp between parapets up to a gangway tower
  at the bow, into A. The bots' route (they ignore boats) and the nav graph's
  only way aboard.
- **The accommodation ladder** (the hull's quay side, by B): drive a dinghy
  to its pontoon at the waterline (or swim and water-jump onto it), hop out
  and run up the ramp along the hull to the deck.
- **The hook**: let the boat come to rest against the hull anywhere, hook the
  hull from the seat, walk up its side and hook the deck or a container to
  come aboard. Walking on over the top edge doesn't wrap onto the deck (the
  boots keep the hull's gravity and you fall across the ship): v1.

Rails run along the deck's sides (a gap at the ladder's landing). Bots never
step into water: a straight walk off a quay isn't clear to them
(`segmentClear`), and `aiKeepDry` stops any move (a strafe, a sidestep, a
slide along a wall) that would take a bot on dry ground over a drop into
water (`TestSegmentClearNotIntoWater`, `TestAIKeepDry`). Live: a 21-round bot
match on the map with no bot in the water, all five attackers aboard by the
pier.

Tests: `TestDeHostileTakeover` (rules, boats on the surface, walking up the
ladder into B), the shared map tests (nav to both sites from both spawns
through the real movement code, a golden file), scenario
`map-de_hostile_takeover` (in SMOKE: spawns, overview, the pier and the
gangway, B, a dinghy across to the hull and hooked onto it, up the ladder).

## Movers and the command clock (`internal/sim/movers.go`, `internal/game/simtick.go`)

A mover's boxes are a pure function of time (escalator treads, slides, path
movers such as MVP-1: docs/starslop.md). Every command runs at its player's own
command time, `CmdTime(tickBase, seq)` (`me.tb` in each self-snapshot), on the
server and in the client's prediction and replays alike. So a body standing on
a mover is carried by exactly the motion between one command's time and the
next.

**Skipped commands coast.** The server does not run every command it is sent.
In prod (test mode runs them all):
- an overflowing queue drops its oldest commands;
- a backlog that doesn't drain for a second is trimmed down to 4;
- commands lost past the client's 16-command resend window never arrive.

Each of these acknowledges commands without running them, and the next
command runs that much later on the movers' clock. Off movers nothing in the
world moves in that gap, and the client corrects itself from the next snapshot.
On a fast mover it threw riders off. MVP-1 climbs at 60-80 m/s: one skipped
tenth of a second moved the cabin floor 5-8 m in a single step, past a rider's
feet without carrying them, and they fell back to the pad. That happened on a
prod server and never in lockstep.

So the server counts what it skipped (`Player.skipped`, plus an arrival gap of
at most `maxSeqGap` seqs). Before running the next command, `coast` steps a
body on foot near a mover through those ticks with no input (crouch held as
it was), so the mover carries it across. Bodies off movers, in cars or in
boats are unchanged. A gap over `maxSeqGap` moves the clock on by only one
tick (`tickBase` absorbs it), as before.

Tests:
- `TestRideRealtime` (`internal/game/ride_realtime_test.go`): a rider through
  a whole flight on lab_moon and cq_moonshot, standing and walking, under
  eight link patterns, on a server with the real command budget;
- the realtime scenario `mvp1-ride-realtime` (nightly): a browser at real
  frame rate with fake lag, jitter and loss, watched frame by frame
  (`__cs.rideStart`/`expectRide`).

## Frames of reference (`internal/sim/frames.go`, `internal/game/frames.go`, `web/engine/net.js`)

Riding a mover is standing still in another frame of reference: a passenger
in MVP-1's cabin at 60-90 m/s should look as still to the other passengers as
someone standing in a room. Three clocks meet there. Our own body and the
movers are drawn at our predicted command time (`moverTime` in main.js); the
other players are interpolated from snapshots ~30-100 ms in the past; and each
snapshot has a player where its body was at the end of its own last command,
on its own command clock, which runs ahead of the server's by a varying few
ticks (queues, budgets, skipped commands). In world space the gap between
those clocks times the ship's speed put a standing passenger metres off the
floor, a different amount each frame. And 60-90 m/s is over net.js's
teleport speed (30 m/s), so it wasn't even interpolated: it jumped from
snapshot to snapshot. Measured in the realtime scenario before this:
another standing passenger moved up to 15 m from one frame to the next
relative to the cabin (rms 3 m).

**A frame** is a mover (1-based index into `Map.Movers`; 0: the world). v1
movers only translate, so its transform at time t is `MoverOffset`
(`FrameOffset`, `ToFrameLocal`, `FromFrameLocal`). A body is in a mover's frame
(`sim.FrameOf`, shared) when a gravity zone carried by that mover holds it
(the cabin, airborne too), or else when it stands on a slide or path mover's
box (a lift, a sled). Escalator treads are no frame.

**A ship's cabin is its passengers' frame in movement too** (`World.carry`,
`internal/sim/frames.go`). Every sub-step a body (world gravity) whose centre
is in a gravity zone carried by a mover is moved by that mover's travel, in
the air as on the floor, and so are the peers' hulls in the same cabin; the
movers' solids of the sub-step are then taken relative to it, so the cabin's
own walls, floor and roof don't move against it. Its velocity is its velocity
in the cabin: jumping, falling and walking in a ship at 90 m/s are as in a
room. Before, only a body standing on the floor was carried (riding, below in
"Movers and the command clock"): one in the air (a jump, or lifted off the
floor as the climb eased out faster than 1 g: MVP-1 decelerates at 18 m/s² on
lab_moon and 73 m/s² on cq_moonshot, and its cross accelerates 30-86 m/s²
sideways) kept its world velocity, and the walls, floor and roof ran into it
at 60-90 m/s and past it (a shove only goes along x or z, and only where
there is room): every jump in flight threw the jumper out of the rocket
(`TestCabinIsItsPassengersFrame`: 16 of 32 riders out at the climb's end
before; case `mvp1-jump`). A docked ship has no travel, and anything off a
ship steps as before. Lifts and sleds (no zone) still carry only who stands
on them. v1: a body with a gravity of its own (Gravity Override) isn't carried.

Frames are also how riders are replicated, drawn and rewound:

- **Server**: each tick (`updateFrames`, and after every command:
  `runCmd`) a player gets `Frame` and `Local`, its place in the frame at its
  body's time (`cmdTime(ackSeq - skipped + 1)`, mm), sent as `fr`/`fl` (hot)
  besides `p`. Pose rows carry them after the flags when set. Not a body, in a
  car or in a boat (its own deck frame, below): none. A rider standing still
  has a constant `fl`, so the delta codec sends almost nothing for it.
- **Client** (`createRemotes().setFrames`): a remote in the same frame in two
  snapshots is interpolated in that frame (its place, the teleport check on
  the place too) and placed with the frame's offset at the time we draw the
  movers (`sample(now, ft)`, `sampleAt(t, gone, ft)`). Our ship and its
  passengers are then drawn from the same clock, and a passenger standing in
  it stays still relative to it and to us, at any speed. Entering or leaving
  a frame between two snapshots is interpolated in the world (the later pose
  placed after a teleport), and the drawn gap between the two placings eases
  out over `BLEND_MS` when it is under `FRAME_BLEND_M` (25 m). Extrapolation
  uses the velocity in the frame. A frame the map lacks (a map edit removed
  the mover) falls back to the world position.
- **Lag compensation and peers** (gameplay): `History` records each rider's
  frame and place; `AtIn(t, place)` interpolates exactly as `sampleAt` does and
  places riders at the command's time (`posesAt(vt, ft)`: `ft` is the shot's or
  the step's command time, where the shooter's client drew the movers). The
  hull a command bumps into for a rider is its place in the frame at that
  time (`peerPos`; the client's `placePeers`, `sampleAt(vt, true, ft)`). So
  what you hit and bump into in a moving cabin is what you saw, and the
  predictor agrees with the server (cases.js mirrors `peerPos`).
- **Drawing the movers** (`web/engine/movers.js drawnMoverParts`, gl.js
  `aOrg`): a mover's faces are patterned in its own space (world position
  minus its offset), so the carpet and the seats' fabric ride with the ship
  instead of sliding over it (before, the seats even changed colour as the
  ship crossed, the colour being picked from the world position). Lighting
  and fog use the world position. The static map is unchanged (offset 0);
  escalator treads keep the world's pattern.

Boats already worked this way for their passengers (`me.dk`: a place on the
deck in the boat's frame, below); cars' passengers would be the next frame
(a frame with a rotation: `FrameOffset` becomes a transform). Effects in a
frame (tracers, decals, sounds started aboard) are still placed in the world
where they happened: at the speeds of a ship they are left behind (v1).

Tests: `TestFrameOfRocketPassenger` (a passenger's place in the cabin moves
3e-11 m over the whole climb), `TestFrameOfLift`, `TestRidersInTheirFrame`
(two passengers with uneven command clocks: the rewound pose and the hull
are at the passenger's place in the cabin; in the world they were up to 4 m
off), `TestHistoryAtInFrames`, `TestPoseRowsAreHotFields`; net.test.mjs (a
passenger in an accelerating ship: 0 relative to it; lost, duplicated and
reordered snapshots; into and out of a frame without a hop); movers.test.mjs
(texture origins); case `mvp1-riders-bump` (go, js, e2e: walking into a
standing passenger in the climb); scenario `mvp1-frames` (lockstep: bob stays
within 5 mm of his place in the cabin as alice draws it through the climb
and the cross, screenshots of the carpet 0.25 s apart); the realtime
scenario `mvp1-frames-realtime` (nightly: two browsers with 80 ms fake lag
ride one flight, `rideStart({watch})` measures the other passenger relative
to the cabin frame by frame: max 0 m, was 15 m).

## Gravity zones and launch pads

`content.Map.GravZones` (boxes with their own gravity: the Moon at 1/6 g, space at 0 g,
where bodies coast and thrust along the view) and `content.Map.Launchers` (launch pads
that throw a body standing on them) are shared sim (`internal/sim/zones.go`), predicted
like the rest. A map without them steps exactly as before. Design, numbers and tests:
`docs/starslop.md` "Phase 2"; the lab is `lab_moon`.

### Jump pads and accelerators (`internal/sim/zones.go`)

A launcher (`content.Launcher`) throws a body standing on it (its feet in the
pad's box). Three kinds, one mechanic:

- **Launch pad** (`Vel`): that velocity, whatever the body was doing
  (lab_moon's, a ship's drop pods with `Air`).
- **Accelerator** (`Vel` mostly sideways and a little up): the same, a low hop
  over a gap (de_serverless's east pier).
- **Jump pad** (`Apex > 0`, `Target`): Quake-style, it aims. `PadVel(feet,
  target, apex, g)` is the ballistic arc from where the body stands to the
  target, its top `Apex` m over the higher end, under the gravity at the pad
  (a gravity zone's too; none in space). Wherever on the pad you step, you land
  on the target (`TestJumpPadLandsOnTarget`, both movement models).
  `mapgen.JumpPad(name, x, y, z, w, d, target, apex)`.

**Air control** (classic model): in the air, faster than a run (a pad's throw,
a hook's swing let go, a fast boat jumped off), the keys steer the velocity
but never slow it (`TestAirKeysSteerAThrowDontBrakeIt`); at a run or slower
it is as before. Source's air control never braked. A throw lands fast and
skids on (classic ground friction, 70 m/s²: 6 to 12 m): a map aims each pad
short of its platform's far edge by that much
(`TestServerlessPadsLandOnPlatforms`).

**Drawn**: the pad is a box of the map's (`jumppad`: rings pulsing out;
`accelpad`/`accelpadr`: chevrons chasing toward +x/+z or -x/-z), the arc a
faint dashed line flowing from it (`web/game/padarcs.js`: from the generated
`PadVel`; a fixed pad's first 1.5 s), the throw the `launch` whoosh
(`Events.Launched`).

**Bots**: navgen links every fixed pad (no mover, not `Air`): a node on the
pad that the nodes around walk onto one way (`NavNode.OneWay`: stepping off a
pad is never walked, it throws you; no walking link crosses a pad) and a
one-way link from it to a node where it lands (a jump pad: its target; a
fixed one: where its flight first comes down on floor, on a map without
gravity zones). The flood goes through pads, so a platform only a pad reaches
gets nodes. A thrown bot (`aiThrown`: in the air, faster than a run) drops
the waypoints behind it and coasts: the arc lands it on the next one.
mapcheck's walk coasts the same. Tests: navgen `TestPadIslandsWalk`,
`TestAcceleratorWalk`; `TestServerlessBots`.

### The map for it: de_serverless

Defusal in space with no server under anything (`internal/content/maps/de_serverless.go`):
floating `lambda` platforms over the void under the space sky, everyone in the
Gravity Override (`mp_gravityboots 2`: the hook), a pit under everything (bots keep
out of it; a fall goes on past the world's bottom).

- The spawns west ("cold start") and east ("warm start"), bridges and ramps
  up to the hub (4 m) in the middle, a ramp and bridge down south to site B
  (cold storage, the spawns' level).
- Site A (the hot path) north is the high tier (8 m): only by jump pads (from
  the hub, from each spawn, and the long one from B's back over everything)
  or the shuttle (a `MoverSlide` platform between the hub's north edge and
  A's dock, a ramp up from there). Back: a pad down to the hub, or the shuttle.
- Site B also by a pad from each spawn's south corner (half the map in one
  throw), the west pier's ferry (another slide mover) or its pad, and the east
  pier's accelerator over a 10 m gap.
- Its nav graph is navgen's (built at load, `autoNav`); bots fly the pads to
  both sites and plant on both (`TestServerlessBots`: no falls in 6 minutes
  of 4 v 4). Case `jump-pads` (the hub's pad onto A holding forward, the
  accelerator onto B, the rule's boots; go/js/e2e); scenario
  `map-de_serverless` (overview, the hub's pad and its arc, the sites,
  thrown, landed on A).
- Ramps over the void needed two fixes: a bot's `floorAlong` follows the
  floor sample by sample (up a ramp, not within a step of where it started:
  `TestFloorAlongUpARamp`), and mapcheck's `Solid` takes a ramp's slope, not
  its box (`TestSolidOnARamp`).

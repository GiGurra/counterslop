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
  through `cvarValue`). Today: `sv_movement`, `sv_airaccelerate`, `sv_autobunnyhopping`.
  Later parts add theirs (e.g. `mp_gravityboots`).
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
  the movers' boxes at the command's time and the other players' hulls are
  turned in, the unchanged `step` runs, and the body is turned back.
- Permutations and sign flips are exact, so the round trip loses nothing and
  Go and the generated JS stay bit for bit (`TestGravityParityWithJS`). A body
  in `GravDown` takes the old path untouched: every existing trace is
  identical (simdiff).
- `TestGravityRotatedRunsMatchTurned`: a body with gravity g in a map turned
  by g moves exactly as a normal body in the map, turned.
- The hull stays an axis-aligned box (its dimensions swap); a player's hull
  (`HullBox`, `sim.Peer.G`) is turned with its own gravity, so players on a
  wall collide with players on the floor as the boxes they are.
- v1 limits in a turned frame: a wedge collides as its bounding box, a mover
  as its box at the command's time (no riding).
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
- Everything is in the body (`Boots`, `Energy`, `Hook`), stepped by the shared
  code and carried in every self-snapshot (`bt`, `en`, `hk`): predicted and
  reconciled like the movement. Others see the rope from `hk` on each player
  in full snapshots (pose rows leave it out: it is drawn, never hit).
- Cvars `sv_hook_range|energy|recharge|drain|cost|pull` (defaults
  `sim.DefaultHook`), all in `content.RuleCvars`: a map may pin them.
- Sounds: `hookFire`, `hookMiss`, `hookRelease`, `gravFlip` (Events.Hook).
- Tests: `hook_test.go` (pull and flip and back, energy, release momentum,
  wedge, rope never lengthens, no room no flip, Go/JS parity on lab_boots
  and de_slop); cases `hook-flip` (through the real key, e2e too) and
  `hook-cvars`; scenario `gravity-boots` (pulled, the rope seen from the
  floor, flipped). The test API's `set` takes `boots`.
- v1: a body in a gravity of its own doesn't swim (water is the world's: its frame has none).

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
from the last snapshot, bobbing. Not solid to players, no cover from bullets
(v1: it is mostly air). MCP debugger: `boat_set` (move one, list them).

Tests: `internal/sim/boat_test.go` (thrust and drift, turning, resting against
the hull and sliding along it, the pier, Go/JS parity), `internal/game/boats_test.go`
(drift to rest, the round reset, the snapshot, `boat_set`), scenario
`harbor-swim` (a moored dinghy).

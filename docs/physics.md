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

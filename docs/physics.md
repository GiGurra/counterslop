# Physics: map rules and movement models

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

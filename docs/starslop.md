# Star Slop: design (living doc)

A silly parody of a parody: Earth against the Moon in a large-scale, ticket-based
conquest shooter. Earth attacks: its players board rockets, launch to the Moon and
assault the Moon fortress; the Moon defends. Everything here is our own: no names,
ships, uniforms or catchphrases from any existing series or its parodies.

Internally the sides are still `vibe` (Earth) and `artisan` (the Moon); only the
display names change per mode (`web/assets/teams.js` `setTeamMode`).

## Phases

1. **Tickets mode** (done): backlogs, capture points, respawns with a spawn screen, kits,
   bots, the test map `lab_conquest`. This document's rules below.
2. Low gravity and space (below: "Phase 2"): gravity zones (the Moon at 1/6 g, space at
   0 g), the space sky, and launch pads that throw you from Earth through space to the
   Moon. The lab `lab_moon` shows it.
3. MVP-1 (below: "Phase 3", done): the spaceport terminal, the "tickets please" gate,
   a passenger rocket on a 30 s timetable (a path mover carrying its cabin), the ride
   and the drop pods onto the Moon; bots board and ride. Deferred: piloted ships (a
   shared-sim vehicle, pilot-predicted), ship health, turrets.
4. The real map (below: "Phase 4", done): `cq_moonshot`, Earth's spaceport terminal,
   MVP-1, the Lunar Office Campus; signs on the map (departure boards, posters,
   split-flap counters).
5. The crew, the music, the bot balance (below: "Phase 5", done): the gate agent, the
   captain, the cabin crew and the Moon's HR PA in SlopVox; a procedural space opera;
   bots that push in waves, flank with the pods and hold what they take; the sky from
   where you stand; redeploying aboard MVP-1.

## The backlog (tickets)

Player-facing, a team's respawn tickets are its **ticket BACKLOG** (a coding parody):
the top bar reads `EARTH BACKLOG 142 · 87 BACKLOG MOON`. Every respawn closes a ticket;
a team at 0 has **shipped everything** and loses. Internal names stay `tickets`
(`Map.Mode "tickets"`, `mp_tickets`).

## Rules (server-authoritative, `internal/game/tickets.go`)

- A tickets map has `Mode: "tickets"` and capture points `Map.Points`
  (`content.CapturePoint`: id, name, centre, radius, initial owner, `Locked`, `Spawn`,
  spawn spots). It is named `cq_*` (category Conquest), or `lab_*` for a test map; each
  team has exactly one main (a locked spawn point it owns) and at least one point can be
  captured (`content.Map.ModeProblems`).
- **Tickets:** each team starts the round with `mp_tickets` (default 150; a map may pin
  it in `Rules`). Every death of a team member in the live round costs that team one.
- **Win:** a team at 0 tickets loses **at once** (that tick; clearer than waiting for its
  last living player). The round has a 20-minute limit; then the team with more tickets
  wins (a tie goes to the Moon, who held). The match is that one round: the result
  screen and the map vote follow.
- **Respawn:** the dead come back after `mp_respawn_delay` seconds (default 5) at an
  owned spawn point of their choice. Without a choice (or when the chosen point can't be
  used) it is the **auto** pick: the spawn point nearest a point the team doesn't hold
  (the action), or the main. A point with an enemy standing in it can't be spawned at
  (a main always can). A player joining mid-round spawns after the delay too.
- **Capture:** standing in a point (within its radius across, 3 m up or down) with more
  living players of your side than the other moves its bar. An enemy point is first
  neutralised (one bar), then captured (a second bar). One player up fills a bar in 10 s,
  two in 5 s, three or more in 3.3 s. Equal numbers (both present) contest it: nothing
  moves. Defenders undo an attacker's partial bar; an empty point's partial bar drains at
  one player's speed. Locked points never change hands. Capturers in the point score
  objective points.
- **Bleed:** a team holding more points (mains included) than the other bleeds the other
  one ticket every 4 s per point of difference.
- **No buying:** a kit choice on (re)spawn instead (`web/assets/economy.json`
  `tickets.kits`, shared with the client): Intern Rifleman (the team rifle, a smoke),
  Senior Sniper (Code Review, Hotfix), Staff Engineer (Token Burner and grenades; rockets
  later), Product Manager (Autocomplete, Rubber Duck, a smoke). Freeze time is at most 5 s.
- **State:** the round state in every snapshot carries `tk` (both backlogs), `tkMax` and
  `pts` (per point: owner, the filling bar and whose, head counts, contested). The
  self-snapshot carries the spawn choice (`spk`), the kit (`kt`), where the next spawn
  goes (`spAt`) and the respawn countdown (`rsp`). The client sends its choice as
  `{t:"deploy", pt, kit}` and resends it until the self-snapshot shows it, so a lost
  message costs nothing. `{t:"point"}` events (neutralised, captured) are optional.

## Client

- Top bar: both backlogs and the round clock; under it the points strip (owner colour,
  the capture bar in the capturer's colour, a blink while contested, ⌂ for the mains).
- Spawn screen while dead in a live round (`web/ui/tickets.js`): the countdown and where,
  a map overview with the points (click one you hold), AUTO, and the kits. It frees the
  cursor and takes clicks (`pointer-events: auto` inside `#hud`, which has none: without it
  every click fell through to the game); you aim again on respawn. With one point to spawn
  at (cq_moonshot: each side's main is its only spawn point) there is no choice to offer:
  a line says where you deploy instead of AUTO and that point. Console: `deploy <point|auto>`, `kit <id>`.
- Team names: Earth and the Moon in this mode, everywhere a team name shows.

## Bots

Simple: go for the nearest point the team doesn't hold; with everything held, defend a
point under attack, else the held point nearest the enemy's main. In a point each bot
stands on its own spot in the circle (spread by id) and fights whoever comes. A bot's
kit is picked by its id.

## Test map: `lab_conquest`

A flat 128 x 64 m field: Earth's main west, the Moon's east (both locked), points A
(launch pad) and E (dome) by the mains, B (crater) and D (antenna) on the flanks, C
(relay) in the middle (no spawn). Crates and low walls for cover. Loaded with
`changelevel lab_conquest`.

## Tests

- Go: `internal/game/tickets_test.go` (ticket loss, respawn delay, capture step rules,
  locked mains, bleed, win and match end, time limit, spawn selection, kits, late join),
  `ai_tickets_test.go` (bots play a round through), `internal/content/tickets_test.go`
  and `maps/lab_conquest_test.go` (map checks, walking to every point).
- JS: `web/game/tickets.test.mjs` (HUD state, overview, the deploy resend).
- Cases: `tickets-backlog` (cvars, capture, bleed, death, respawn, win),
  `tickets-deploy` (the spawn screen, `deploy` / `kit`, respawn at the choice).
- Scenarios: `tickets-hud` (the top bar, the points strip, the spawn screen),
  `tickets-spawn-click` (real clicks on the spawn screen: cq_moonshot's kit and its
  one-point line, lab_conquest's point buttons, AUTO and the map's circles).

## The opening (lead's design; the operator asked for absolutely over the top)

Earth side is not a base, it's a **spaceport terminal**: "EARTH DEPARTURES · Terminal 0 (Moon)". Every Earth (re)spawn happens inside it and the only way to the front is the next departure.

1. **Check-in**: you spawn in the terminal hall: carpet, rows of chairs, a departures board (MOON 14:05 DELAYED — scope creep; MOON 14:06 BOARDING; MOON 14:07 CANCELLED — reorg), duty-free kiosk = the kit picker ("Duty-free: Senior Sniper, now 0% off").
2. **Security**: a walk-through scanner that beeps at everyone ("please remove grenades, laptops and opinions from your bag"), a SlopVox TSA voice. Purely gag; no slowdown.
3. **Gate + "tickets please"**: a ticket gate where your respawn ticket gets scanned: a conductor voice ("tickets please... thank you, enjoy your sprint"), the team backlog visibly ticks down on a giant split-flap counter above the gate.
4. **Huge windows**: the whole far wall is glass, and outside stands the rocket: absurdly huge (taller than any map so far), named MVP-1 (Minimum Viable Payload), duct tape, a "BETA" sticker, steaming, gantries, a countdown clock "T-minus 30 · pending code review".
5. **Boarding**: a jet bridge into the rocket's passenger cabin: economy rows, overhead bins, seatbelt signs, a safety-demo announcer ("in case of decompression, blame the last commit").
6. **Launch**: departures every ~30 s (a wave, like a respawn wave); doors close, the cabin shakes, the windows show the pad dropping away, Earth shrinking, space (the phase-2 space sky). Players can shoot out of nothing; pure ride ~8-10 s (skippable later via a 'window seat' cam).
7. **Arrival**: the cabin ejects passengers as drop pods / escape slides into the moon ("Escape pods: do not use unless laid off"), each pod a short ballistic flight onto the battlefield (reuse the launch-pad arc tech), or the rocket crash-lands into the fortress parking lot and the doors blow.

Moon side: **the Lunar Office Campus**, a corporate fortress under a return-to-office mandate: "Lunar Compliance Dept.", a giant moon-logo HQ, badge gates, motivational posters ("Synergy has no atmosphere"), standing desks as cover, a cafeteria with zero-g pudding. Moon respawns in the HQ lobby elevator banks.

Later spice: in-flight trolley (buy grenades mid-flight), delayed departures when Earth's backlog is low ("flight delayed: low tickets"), a single first-class seat for the top scorer.

Phases: the rocket/launch/boarding/arrival is phase 3 (ships); terminal + campus geometry is phase 4 (the map); voices + music phase 5.

Reuse from phase 2: the drop pods / escape slides of step 7 are launch-pad throws
(`content.Launcher`: a velocity set on a body, then the zones' gravity carries it), and
the flight of step 6 shows the space sky (`sky: "space"`).

## Phase 2: gravity zones, space and the launch pads

### Gravity zones (`content.Map.GravZones`, shared sim `internal/sim/zones.go`)

A map lists boxes with their own gravity, `content.GravZone`:

| field   | what |
|---------|------|
| `name`  | shown nowhere yet; for the map's own tests ("moon", "space") |
| `min`, `max` | the box (world coordinates) |
| `g`     | gravity as a fraction of the world's (22 m/s²): `0.1667` the Moon, `0` space |
| `jump`  | the jump speed as a fraction (0 = 1): low gravity with full jumps would be 7 m high |

- A body is in a zone when its centre (feet + half its height) is inside the box; where
  zones overlap, the one listed last wins. Outside every zone the world's gravity holds,
  so a map without zones plays exactly as before (`simdiff`: every case trace the same).
- The zone is looked up every 120 Hz sub-step from the body's position. Nothing about it
  is extra state: the server and the client's prediction (the same Go, generated) agree
  because they step the same position (`TestParityWithJS` gets zone vectors).
- **Low gravity** (`0 < g`): walking is unchanged (ground friction and acceleration are
  the same; you are not slower on the Moon), falling and jumping use `g` and `jump`. The
  Moon: `g 1/6`, `jump 0.6` → a jump peaks at about 2.5 m and lasts 2.3 s (Earth: 1.2 m,
  0.65 s). Both movement models; source bunny hops carry far. A fall in low gravity moves
  at least 2·10⁻⁴ m per sub-step (more than the collision tolerance), so a body at rest
  still finds the ground every sub-step.
- **Space** (`g == 0`): no ground and no up. You coast: velocity is kept (no drag, no
  gravity), so a throw from a launch pad flies straight. Your suit's thrusters push you
  along the view (pitch counts, like swimming): W/S/A/D, jump up, crouch down, at
  `spaceThrust` 5 m/s² up to `spaceSpeed` 6 m/s. Thrust never slows you below the speed
  you came in with: it only steers (the launch's 15 m/s stays 15 m/s). With nothing held
  you drift on. Touching a wall stops you along it as any collision does.
- The Gravity Override still works: the hook pulls you (that is the way to stop in space:
  hook a hull and reel in), and a flip turns your own gravity as before. In a rotated
  gravity frame zones do not apply in v1 (like water): the boots' magnetism wins.
- Directions other than down: a zone's direction (`dir`, one of the six `sim.Grav*`
  axes, for ship interiors and spinning stations) is designed but not built in phase 2:
  it would set the body's own gravity on entry, as a boots flip does. Phase 3 decides.
- **Grenades** fall at their usual 0.4 of the zone's gravity (server, `grenades.go`):
  lobbed far on the Moon, straight on in space. **Bullets** fly straight everywhere, as
  before. **Water** is unchanged (no water on the Moon). Falling has no damage in this
  game, so a Moon fall is only slow.

### Launch pads (`content.Map.Launchers`, shared sim)

`content.Launcher {name, min, max, vel}`: a body standing on the ground with its feet in
the box is thrown: its velocity becomes `vel` (m/s, world axes) and it leaves the ground.
A jump pad, in the sim (predicted, so no rubber band on the throw), with an event for the
sound (`Events.Launched`). Phase 2's catapults are this: the Earth pad throws you up and
east through the space band (where you coast) into the Moon's zone, where the low gravity
brings you down on the Moon; the Moon pad throws you back. Ships replace them in phase 3.

### The space sky (`content.Map.Sky`, `web/assets/sky.js`)

`sky: "space"` swaps the day sky for our own space shader: a black sky with a seeded
star field (hashed cells, twinkle-free so screenshots are stable), a faint band of
galactic haze, the Earth (a blue-green disc with drifting cloud noise and a lit limb) and
the Moon (grey, cratered noise) as big discs at fixed directions, and the sun as a small
hard disc. The Earth is lit by the sun; the Moon hangs near the sun, so it is lit from the
sun pulled round towards the viewer (gibbous instead of a dark new moon). The engine is
told the sky names (`look.skies`, `web/assets/sky.js` `SKIES`) and sets `uSkyMode` to the
map's; one shader branches on it. The map lighting is unchanged (the same sun); lab_moon
adds a faint black fog. The planned `de_serverless` arena reuses it. New material
`regolith` (Moon dust with craters), new sound `launch` (a catapult thunk and a whoosh).

### lab_moon

A lab (`changelevel lab_moon`, tickets mode): Earth's field in the west (x -80..-20,
the world's gravity, Earth's main, point A by the launch pad), space over the void
between (x -20..38, every height: 0 g), and the Moon east (a regolith plateau 20 m up,
x 40..120, in a 1/6 g zone: craters as low rims, points B and C, the Moon's main, the
return pad). The Earth pad throws at (12, 16, 0) m/s: 0.5 s up through Earth's gravity,
4.8 s coasting through space, then a slow arc onto the Moon (x ≈ 85, 9.25 s in all). The
Moon pad throws at (-12, 5, 0) and lands you on Earth's field (x ≈ -42, 7.5 s). In space
you don't fall: stray off Earth's edge and you drift on (thrust back). Under the Moon
there is gravity again, and falling there kills ("the void", a pit at y -20..-6: a pit
must reach above y -20, where the server's out-of-world rescue puts a body back at a
spawn without a death; `mapcheck` now says so). Drifting 30 m past the map's boxes in
space is such a rescue too. Bots can't use the pads yet (phase 5): the nav
graph has two islands. navgen now gives up at once on a route between islands (union-find
over every link): before, every such route searched the whole island, a minute per
`changelevel lab_moon`.

### Tests

- `internal/sim/zones_test.go`: the Moon's jump (2.5 m, 2.3 s; outside zones the same
  bits as before), coasting in space, thrust (to 6 m/s; it steers a faster body without
  braking), a launch, zones ignored in a rotated frame, both lab_moon flights in both
  movement models. `TestParityWithJS` runs the zone world in both models.
- `internal/game`: `TestNadeGravityZones` (a grenade in space flies straight, on the Moon
  it falls at 1/6). `mapcheck`: zone, pad and sky checks. `navgen`:
  `TestRouteBetweenIslandsGivesUpAtOnce`.
- Case `moon-launch` (go, js, e2e): the throw, coasting, thrust forward / up / back to a
  stop, the landing on the Moon, a Moon jump.
- Scenario `moon-flight`: Earth's pad and the Moon in the sky, mid-flight in space, the
  Earth from the Moon, a Moon jump.

## Phase 3: MVP-1, boarding, the ride and the drop pods

Everything that moves the passengers is in the shared sim, so the server and
every client's prediction agree on the ride bit for bit; nothing is sent for it
but the map.

### Path movers (`content.Mover` kind `path`, `internal/sim/movers.go`)

A mover's boxes offset by a pure function of time: `Path` keys `[t, dx, dy, dz]`
within `Period` s, eased (smoothstep) between keys, the last key held, then the
cycle wraps (the ship is back at its dock). `Hide [from, to)` (may wrap) takes a
box out of the world for part of the cycle: the open door, the floor and seats
when the pods fire. Riding, lifting and shoving are the escalator/slide code
(`movers()`): a body on the floor is carried, a body under it is lifted. Each
command runs at its own command time (`CmdTime`), as for every mover.

Commands the server never runs (lost past the client's 16-command resend
window, dropped from an overflowing queue, trimmed from a backlog that won't
drain: prod only, test mode runs every command) still pass on the movers'
clock: the next command runs that much later. A rider near a mover is
stepped through them first with no input (`coast`, `internal/game/simtick.go`),
so the ship carries it across. Without that, MVP-1's climb (60-80 m/s) left a
rider metres under its floor after one skipped tenth of a second: players
were thrown out of the rocket on the test server (`TestRideRealtime`, the
realtime scenario `mvp1-ride-realtime`).

### What a ship carries (`GravZone.Mover`, `Launcher.Mover`, 1-based)

- **Its cabin's gravity**: a zone carried by the mover (MVP-1: 1 g) wins over the
  fixed zones, so passengers stand on its floor through space (0 g outside).
  The cabin is its passengers' frame (docs/physics.md "Frames of reference"):
  in the air (a jump) they move with the ship as on its floor.
- **The drop pods**: a launcher carried by the mover, live only in the cycle
  window `[From, To)` and `Air` (it throws falling bodies too). MVP-1's floor is
  hidden for that window, so the cabin's passengers fall through it into the
  launcher and are thrown down at 14 m/s onto the Lunar Office Campus. A body
  already moving at least 90 % of the pad's speed along it is not thrown again.
  This is the phase-2 launcher reused, in the sim (predicted), instead of
  server-side velocity sets.

### MVP-1 (`prefabs.Rocket`, `mapgen.Ship`, lab_moon)

A 16 x 8 m economy cabin (seats, aisle, window bands, a door on the west side)
on a stack to 80 m with boosters and engine bells, on a pad over the flame trench.
Timetable (30 s): docked with the door open 0-14 s (the jet bridge meets the
sill), climb 110 m by 20 s, cross to 50 m over the Moon by 26 s, floor open and
pods live 26-29.5 s, then back at the dock. `Map.Departures` names it for the
HUD and the bots: the mover, when the door closes, the door spot (the jet
bridge's end) and the boarding spot (the aisle).

### The terminal and the gate (`Map.TicketGates`, `tickets.go tickGates`)

Earth spawns in a spaceport hall: departures board, duty free, chair rows, a
security arch, then the ticket gate before the jet bridge. Walking through a
gate scans your respawn ticket once per life: a `ticket` event (optional: the
gate is a gag, nothing depends on it) and the conductor's "Tickets please...
thank you, enjoy your sprint." for you, with a feed line. The split-flap
backlog counter on the gate is phase 4 art.

### Client (presentation only)

- `web/game/ships.js`: the departure board in the HUD (BOARDING, doors close in
  N / ABOARD / IN FLIGHT / DEPARTED, next in N), from the map and the movers'
  clock; the cabin crew's safety line as the doors close with you aboard.
- Remote passengers are replicated in the ship's frame (`fr`, `fl`) and drawn
  where we draw the ship (docs/physics.md "Frames of reference"); the cabin's
  carpet and seats are patterned in the ship's own space.

### Bots (`internal/game/ai_board.go`)

The nav graph comes in islands (Earth, the Moon). A tickets bot whose wanted
points are all on another island walks to a departure's door on its own island,
waits there while the ship is away, walks in while it is docked (to its own spot
in the aisle), stands while riding (no route: any waypoint is a point in the
world the ship carries it off, and it used to walk back at run speed against
the travel, wobbling along the cabin: `TestBotsRideStill`) and fights on from
where the pods put it down.
The bots' floor checks count the mover boxes where they are now (the docked
cabin's floor); the flame trench has rails.

### Known limits

- At the cycle's wrap the ship snaps back to its dock (it shoves anyone standing
  in its footprint, as a slide does).
- Shots at passengers of a moving ship are rewound in the ship's frame, placed
  where the shooter drew the ship (docs/physics.md "Frames of reference");
  bullets don't hit mover boxes.
- Grenades don't feel a cabin's gravity (fixed zones only).

### Tests

- `internal/sim`: `TestPathMover`, `TestRocketRideAndDropPod`,
  `TestLabMoonRocketRide` (a passenger reaches the Moon, someone on the bridge
  stays behind), parity Go vs JS on a rocket map in both movement models.
- `internal/game`: `TestTicketGate`, `TestAIBoardsTheRocket` (Earth's bots
  board, ride and land on the Moon), `TestOutOfWorldCountsMoverPaths`.
- `sim/cases/mvp1-ride.json` (go, js, e2e); `web/game/ships.test.mjs`;
  scenario `mvp1-boarding` (terminal, gate, cabin, climb, window, drop, landing).

## Phase 4: cq_moonshot

The first Conquest map in the rotation (category `conquest`, Maps screen section
"Conquest"), 32 players, `mp_tickets` pinned to 300 (Earth bleeds from the start: the
Moon holds its campus, and Earth only lands with the first drop, ~25 s in).

### Earth: the spaceport terminal (x -290..-200)

A 90 x 80 m hall, 12 m high, lit from the ceiling. West to east:

- **Check-in** (Earth's main, two rings of 16 spawns): the departures board over the
  whole west wall (MOON 14:05 MVP-0 CANCELLED - REORG, MVP-1 DELAYED - SCOPE CREEP,
  ... MARS MVP-2 ON THE ROADMAP), its live last line ("NEXT DEPARTURE MVP-1 DOORS CLOSE
  IN 09"), check-in counters, waiting rows.
- **Security**: a 2.6 m glass partition across the hall with three walk-through
  scanners (`Map.Scanners`: the client beeps and the TSA voice says "please remove
  grenades, laptops and opinions from your bag", at most every 8 s), x-ray belts.
- **Airside**: duty free (two kiosks, the ads "SENIOR SNIPER NOW 0% OFF").
- **Gate 0**: the "tickets please" gate under Earth's backlog on split-flap tiles (a
  live sign: the digits flip as tickets close), rails funnelling into the jet bridge.
- **The window wall**: open from a 1.3 m rail to 11 m (the renderer has no see-through
  glass), onto the pad: MVP-1 at 150 m, its gantry with the T-minus clock ("T-09",
  "GO"; PENDING CODE REVIEW), the flame trench. The jet bridge's end has a door
  that is there unless MVP-1 is docked with its doors open (a still path mover).

### MVP-1 on cq_moonshot

`prefabs.Rocket` with `Height` 150, overhead bins, its name and a BETA sticker on its
flanks, FASTEN SEATBELT and "ESCAPE PODS: DO NOT USE UNLESS LAID OFF" in the cabin (signs
on the mover). Timetable (35 s): boarding 0-12 s, up 150 m by 17, across 290 m to 40 m
over the drop zone by 23, pods 23-26.5, **back empty with its doors shut** by 31.5 (up
and across) and 34.5 (down onto the pad): docked again before the cycle wraps, so it
never jumps back and shoves nobody (`TestShipsDockBeforeTheWrap` holds every path mover
on a built-in map to that). lab_moon keeps its 30 s cycle with the jump: a lab.

### The Moon: the Lunar Office Campus (x 75..302, 30 m up, 1/6 g)

- The open surface: **A Drop Zone** (craters, boulders, the welcome billboard: "RETURN TO
  OFFICE IS MANDATORY · VISITORS MUST BADGE IN"), **B Solar Farm** (tilted panel rows),
  **C Rover Parking** (rovers, the VP of Synergy's reserved bay).
- The campus wall at x 170 (6 m): the **badge gates** (turnstiles, readers) under
  "LUNAR COMPLIANCE DEPT. · BADGE IN", staff gates north and south; posters inside
  ("SYNERGY HAS NO ATMOSPHERE", "RETURN TO OFFICE: IT'S ONLY 384,400 KM").
- The quad with standing desks as cover; **D Compliance Dept.** (open plan, rows of
  standing desks); **E Cafeteria** (tables, "TODAY: ZERO-G PUDDING"); the promenade
  between them to **HQ** (the moon logo, "LUNAR OFFICE CAMPUS"), whose lobby is the
  Moon's main: it respawns in front of the elevator doors.
- D and E start the Moon's; A, B, C neutral. Earth spawns at any point it holds.

### Signs (`content.Map.Signs`, `web/assets/models/signs.js`)

Text on the map in our own 5x7 dot font (`web/assets/font.js`, drawn glyph by glyph).
A still sign's letters are boxes merged into the map's mesh on the client (free to draw;
the server never sees them: no collision). A sign on a mover (the rocket's name) or a
live one (`{backlog:vibe}`, `{departure:MVP-1}`, filled from the round state and the
movers' clock: `web/game/ships.js signValues`) is drawn every frame as solids, live ones
on split-flap tiles whose flap falls when a character changes. Materials `led` (lit
amber), `ink` (dark lettering), `flap` (the boards' tiles) and `poster`.

### The sky from where you stand (`Map.SkyZones`)

The space sky has both bodies in it; a map's sky zones give each side its own
(`web/assets/sky.js skyAt`, by the eye): on the Earth (`earth`) a blue day sky with a
pale day Moon, fading into space as the eye climbs (60..170 m up: the launch), on the
Moon (`moon`) space with the Earth alone in it. In flight, between them, both.

### Bots

Any bot goal on another nav island goes by ship (`aiAcross`: to the door, in, ride, on
from the drop), so warmup roaming and test `goTo` cross too. `mapcheck.WalkProblems`
walks a team to an objective across a departure (to its door; from where its pods land)
for every built-in map and custom maps alike (`Crossings`).

A full team spawning at one point overflows to its next points (tickets mode used to
put two players on one spot when a team outnumbered a point's ring).

### Tests

`TestMoonshotCrossing`, `TestShipsDockBeforeTheWrap`, `TestAIMoonshot` (Earth's bots
take a Moon point, both sides kill), the map walks (both teams, every spawn, every point,
across MVP-1), `TestBotsWalkFromEverySpawn` (cq_moonshot: a departure's time added),
`TestFullTeamsSpawnApart` (the overflow), `signs.test.mjs` (the font, the layout, the
flaps), `ships.test.mjs` (sign values, the scanner gag), scenario `map-cq_moonshot`.

### Known limits

- Bots on the Moon defend well and Earth's attack is thin (4 v 4: Earth had its
  backlog down to 26 of 300 in 4 minutes, the Moon at 292): fixed in phase 5.
- The interiors are dim (the client's light budget); no see-through glass.

## Phase 5: the crew, the music, the balance

### The balance (`internal/game/ai_tickets.go`, `TestAIMoonshot`)

Measured with `AI_MOONSHOT=1 go test ./internal/game -run TestAIMoonshotSweep -v`
(2v2, 4v4, 8v8, four seeds, 8 minutes; `AI_MOONSHOT_LOG=<n>` logs one n v n match
every 15 s: tickets, owners, each bot's target and place). Before: the Moon won every
match at every size, Earth's backlog at 0 inside 8 minutes. Where it went wrong, and
what changed:

- **Earth's walk**: the generated nav graph crossed the security line once, at its
  south end (a 3 m scanner lane is too narrow for its links), so Earth's bots took ~40 s
  from check-in to the gate. Nav hints (`Map.NavHints`, `mapgen.NavHint`) through
  each scanner and into the gate's funnel, 6 m scanner lanes and airside rows along
  the hall: ~17 s.
- **The first ship**: its doors shut before the opening's wave got there. The
  timetable is now 38 s: boarding 0-19 s, up by 22.5, across by 27, the pods
  27-30.5, back empty by 37.5.
- **Redeploying aboard** (`Departure.Team`, `tickets.go spawnAboard`): Earth's dead
  respawning at their main while MVP-1 boards (3 s or more to go) spawn in its aisle.
  A respawn, not the opening (that is the check-in hall).
- **Spawns on the mains only**: the Moon used to respawn on the front points next to
  Earth's landing; now both sides come from their mains (the Moon on foot from HQ,
  Earth by ship), about equally far in time.
- **The points**: D (Compliance Dept.) and E start neutral (no +2 bleed from the first
  second); E is the **Badge Gates** (160, 0), the midfield before the campus wall, not
  the cafeteria (which stays, with its pudding).
- **Waves** (attack): a bot scores the points it does not hold by the walk there, the
  enemies in them (`aiTkEnemy` 30 m each) and the teammates already headed there
  (`aiTkFollow` 20 m each): a team moves together. Across a ship it picks the wave's
  target by the drop point.
- **The flank pods** (`Timetable.Flank`, 22 m/s): the cabin's aft third throws its passengers
  towards the solar farm (B), the fore third towards the rovers (C), the middle onto
  the drop zone (A). Every third first drops straight out through the open floor; the
  aft and fore pods veer only once a body is clear of the hull (feet 2.4 m under the
  floor). They used to throw sideways at once, in the cabin: a passenger went out under
  the side wall (the operator: "thrown out sideways of the rocket instead of straight
  down"), and one in the air as the floor opened (a jump) hit the wall, lost the throw
  and landed in the drop zone (`TestFlankPodsClearTheHull`: 24 of 96 throws lost their flank before, 18 of them stopped by the wall).
  Which third goes where is told three ways: signs over the bins (PODS THIS SIDE: ...),
  markings on the floor of each third between the seat rows, read facing the front
  (`<- SOLAR FARM`, `PODS STRAIGHT DOWN`, `ROVER PARKING ->`; a floor sign, face `+y`,
  on the floor's mover: gone as the floor opens), and the cabin crew's drop line
  (`crew_pods_flank`, in place of `crew_pods` on a ship whose pods flank). A bot stands
  over the pods that land nearest its target (`aisleSpot`). `TestMoonshotPodsFlank`.
- **Holding** (`aiGarrison`): a share of a team's bots as large as its held points'
  share stays on them; the rest attack.
- **Defence** (`aiDefendPoint`): a point under attack draws the nearest half; the rest
  spread over the held points, the front first, and rotate every 45 s (no more four
  defenders camping the drop zone).

After (8 minutes, 4 seeds a size): Earth leads in 4 of 12 matches, no side wiped out;
2v2 and 8v8 lean to the Moon, 4v4 is even. `TestAIMoonshot` (CI): two seeds a size,
5 minutes, nobody wiped out, both sides ahead on points at times in every match, Earth
leading in a quarter to three quarters of them.

### The crew and HR (`web/game/ships.js createCrew`, `assets/voice/lines.js`)

New SlopVox voices `captain` (low, the cockpit's cheap intercom), `gate` (bright, the
hall) and `hr` (smooth, too synthetic). Lines (all original):

- Gate agent, near the gate: "Now boarding MVP-1 to the Moon. All sprint groups, please."
  as it docks; "Final call for MVP-1. Unmerged passengers will be left behind." 6 s
  before the doors; when Earth's backlog is under a quarter, "MVP-1 is delayed due to
  budget review. It will depart on time anyway."
- Aboard: the captain as the doors shut ("This is your captain speaking. We are
  cleared for launch, pending code review."), the safety demo, and one line for the drop:
  the cabin crew arming the pods, 4 s before (on a ship whose pods flank, `crew_pods_flank`:
  "Door end: solar farm. Front rows: rover parking. Middle: straight down."; the captain's "Your seat is about to become
  a door." is in the gallery but no longer triggered: two lines 2 s apart made the second
  come late). Any queued announcer line that can't start within a second (voice
  MAX_WAIT) is dropped unsaid rather than played out of place.
- The Moon: HR welcomes Earth's visitors as they land ("Remote work is not supported at
  this altitude."), reminds the Moon's staff that return to office is mandatory, and
  tells them when Earth books one of their meeting rooms (a point neutralised) and when
  it is back with middle management (taken back from Earth). Point events (`"point"`)
  are optional, presentation only.

### The music (`assets/music/themes.js`, `game/music.js`)

An original space opera on the music engine, two new voices (`strings`, `timp`):
**Minimum Viable Overture** (lydian, brass over strings, timpani, a march) in tickets
mode's warmup and freeze; **Escape Velocity Standup** (a minor string ostinato, brass,
timpani) aboard a ship in flight, the one loop besides a race's under live play; the
`launch` stinger (a timpani roll under rising brass fifths) as the doors shut with you
aboard.

### The sky from where you stand

See "The sky from where you stand" above (phase 4's follow-up).

### Spice

- The departures board's live line: "NEXT MVP-1 BOARDING / IN FLIGHT / DELAYED -
  BUDGET REVIEW  DOORS CLOSE IN 12" (`status:<ship>`; the delay is a gag: the
  timetable is pure time and does not move).
- First class: a plush seat at the front of the cabin and, over it, "FIRST CLASS:" and
  the team's top scorer's name (a live sign, `first:<team>`, from the scoreboard).
- Not done: an in-flight trolley.

### The Monument to the Postmortem

`prefabs.FacepalmMonument` (with `prefabs.Facepalm`, the figure alone, any size and
facing): a giant bronze box statue of a weary engineer in a hoodie, palm flat over
the eyes, coffee mug in the other hand, head bowed (the gesture is the joke: an
original figure, nobody's likeness), on a plinth with lit letters ("MONUMENT TO
THE POSTMORTEM / IN MEMORIAM: THE LAST SPRINT / IT WORKED ON MY MACHINE"), two
spotlights. On cq_moonshot it stands in check-in's south side facing the spawn
ring. Stand at its plinth and it sighs ("...sigh. Who merged on a Friday?", the
`statue` voice): `Map.VoiceSpots` (a zone named after a SlopVox line; the
client's `createVoiceSpots`, at most once every 20 s), reusable on any map.
Earth's assault banners (`prefabs.CrestBanner`, material `crest`: the facepalm on a
hoodie-purple shield, neon rim) flank it and stand where the pods land on the Moon.
Material `bronze`: polished highlights, verdigris on the tops and in the creases.
Lighting: local lights fall off as w²/(1+d²) (`gen/glsl/lights.js`), so the
terminal's ceiling lamps, tuned like an office's, barely reached the floor 11.7 m
below; the hall was a gloom. Its lamps now have the power for their height (90,
range 26), the monument's spotlights are sized for their distance (the prefab
computes it), and the map sets its own indoor ambient (`Map.Indoor` 0.85; the
renderer's default with lamps is 0.3: `indoorAmbient` in `gen/glsl/lighting.js`).
Tests: `TestFacepalmMonument` (no z-fighting at any size/facing, the figure on its
cap, the plaque, the lights, the spot), `ships.test.mjs` (the sigh), scenario
`map-cq_moonshot` (monument-from-check-in, monument, monument-plaque, earth-banners).

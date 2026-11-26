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
3. Spaceships and rockets (reusing the racing mode's vehicle ideas): boarding at the
   launch pads, with a "tickets please" boarding gag (the conductor checks your ticket
   against the backlog).
4. The real map: Earth's launch site, the flight, the Moon fortress.
5. Announcer lines, music, smarter bots.

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
  cursor; you aim again on respawn. Console: `deploy <point|auto>`, `kit <id>`.
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
- Scenario: `tickets-hud` (the top bar, the points strip, the spawn screen).

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

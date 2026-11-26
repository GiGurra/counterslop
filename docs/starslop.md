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
2. Low gravity and space: the Moon's gravity, vacuum zones.
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

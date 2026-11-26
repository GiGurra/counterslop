# Auto nav: bot nav graphs from geometry (internal/content/navgen)

Bots route on a map's nav graph (`content.Map.Nav`: named points linked by
straight walkable legs; `internal/game/ai.go` `navPath`, the walk grid for
the last leg off it). The built-in maps carry hand-made graphs. A map without
one (a custom map) gets one built from its geometry when it loads
(`internal/game/maps.go` `autoNav`), and `bot_autonav 1` builds one for every
map in place of its own (from the next map load; `navAuto` in sim/cases,
case `bot-autonav`).

`navgen.Build`:

1. A walk grid over the whole map, 0.25 m columns: per column the box tops a
   standing hull fits on (on a ramp, where the hull stands on the slope). Too
   steep wedges, escalator treads (they move: `Map.Movers`) and floors under
   water (bots don't swim) are no floor.
2. Two-way walking links between neighbouring floors (a step, no jump or
   drop: a nav leg is walked both ways), flood-filled from every team spawn.
3. Clearance: each reachable floor's distance to the edge of the reachable
   floor (a wall, a ledge, a pit).
4. Nodes: `vibe` and `artisan` at each team's first spawn; one per objective
   zone (sites, GPU Drop's GPUs and exits), named after it, at its most open
   level spot well inside it (a route ends at the first node inside a zone);
   one per 8 m block and 3 m level at the block's most open level spot (none
   inside an objective zone or next to a named node).
5. Links: nodes within 13 m whose straight line walks the reachable floor
   both ways with 0.5 m to the sides (a walker arrives up to half a metre off
   a node). Pieces left apart (a narrow corridor, a bend) are joined along a
   grid route that prefers open floor, string-pulled into new nodes; a corner
   on stairs or a slope moves back to level floor, one on an objective zone's
   edge into the zone or out of it.

Deterministic (no randomness, no map order): the same map, the same graph.

A map file (docs/custom-maps.md) may leave `nav` out: the checks (`mapcheck.Problems`) build one the same way and walk it (`TestProblemsWithoutNav`).

Tests: `navgen_test.go` (an L-shaped yard with a crate, a ledge and a pool; two
rooms joined by a bent corridor), `TestAutoNavWalkToSites`
(`internal/content/maps`: on de_slop, de_monorepo, de_standup, de_pipeline,
de_legacy, de_runway, de_rackmount, surf_scope_creep and he_merge_hell, the
hand graph deleted and built instead, every team walks from every spawn to
every objective with the real movement code, both movement models),
`TestAutoNav` (`internal/game`), case `bot-autonav`. `NAVGEN_ALL=1 go test
./internal/content/maps -run TestAutoNavEveryMap -v` reports the rest.
Live: 5v5 bot matches on de_slop (14 rounds, defuses) and de_rackmount (13
rounds, 7-6) with `bot_autonav 1`.

Not yet (where TestAutoNavEveryMap still fails): stairs that the walk test
reaches at another height (de_stack, de_cloudfarm, gs_megabyte), escalators
as routes (de_stack's are only walked as stairs by bots with a hand graph),
GPU Drop's small grab zones (gs_kiosk), lanes (`NavNode.Lane`: none are
built), jump-ups and drops, and nav hints (points a map maker places to steer
the graph: Custom maps 1's file format).

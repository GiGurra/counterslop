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
   steep wedges and floors under water (bots don't swim) are no floor; an
   escalator's static treads stand in for its moving ones.
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
   prefers level floor, and one on an objective zone's edge moves into the zone
   or out of it.

Planned for the walker, not the point: a bot (and mapcheck's walk) counts a
waypoint reached half a metre short of it and turns for the next one from
there (`arrive`). On stairs that is another step than the node's, so:

- `stopShort(a, b)`: the floor where a walker going a -> b stops. A corner b
  between a and c only `turns` if that stop is at b's height (within a step)
  and walks straight on to c (de_stack: a node at the top of a stair beside a
  raised floor, reached a step lower, where the side is a 1 m wall).
- `enters(a, b)`: a link into an objective zone from outside it must stop
  inside it (a route ends at the first node inside a zone; the bot must stand
  in it). Links and chain corners that don't are not made.
- An objective node less than a metre inside its zone (GPU Drop's grab zones:
  a pedestal fills most of the zone, a 0.3 m sliver of floor is left along it)
  gets an `approach`: two nodes on a line out of the zone, so a walker from
  anywhere around the outer one comes down the line, stops short of the inner
  one on it, and walks on along the sliver into the zone. The inner one links
  to nothing else; the approach legs run along the prop (no room to the sides
  needed).
- Escalators (`Map.Movers`, kind escalator): a lane is 1.2 m between
  balustrades, too narrow for a link with room to the sides, so each gets a
  chain of its own: an outer node on open floor 2.5-5 m out on the lane's axis
  at each end, an inner one just off the lane's end (linked along the chain
  only), so a walker lines up before it gets on. Walked both ways: riding up,
  or walking against the treads (gs_megabyte's west escalators are the
  Vibe Coders' way upstairs; `TestEscalatorWalk`).
- A chain the join can't make walk this way is not made (the join tries the
  next pair; a failed pair is not tried again).

6. Nav hints (`Map.NavHints`, placed in a map file, with the edit ops
   `hint_*` or the editor's hint tool): each is a node at the reachable floor
   nearest it (dropped if none within 2 m), linked and joined like any.
7. Lanes (`NavNode.Lane`: each round an attacking bot, a Vibe Coder, picks
   one whose detour is within 1.6 times the straight route and walks spawn ->
   lane -> objective; `internal/game/ai_lanes.go`): per objective, the node
   halfway along the Vibe Coders' shortest route (going straight stays a
   choice), and up to two more: each the node furthest (at least 8 m) from
   the routes picked so far whose way through is within 1.5 times the
   shortest. Never a named node; one within 4 m of a lane is that lane. A map
   with lane hints has those as its lanes and no others.

Deterministic (no randomness, no map order): the same map, the same graph.

A map file (docs/custom-maps.md) may leave `nav` out: the checks (`mapcheck.Problems`) build one the same way and walk it (`TestProblemsWithoutNav`).

Tests: `navgen_test.go` (an L-shaped yard with a crate, a ledge and a pool; two
rooms joined by a bent corridor; `TestStairTurn`), `walk_test.go` (a stair
beside a raised floor; a pedestal's sliver of zone; two floors joined by an
escalator alone), `TestAutoNavWalkToSites`
(`internal/content/maps`: on every built-in map, the hand graph deleted and
built instead, every team walks from every spawn to every objective with the
real movement code, both movement models, and a Vibe Coder through every
lane a bot may pick to every objective; `TestLanes` in navgen), `TestAutoNav` (`internal/game`),
case `bot-autonav`.
Live: 5v5 bot matches on de_slop (14 rounds, defuses) and de_rackmount (13
rounds, 7-6) with `bot_autonav 1`.

mapcheck's walk checks the height a waypoint is reached at where the body
stands: coming down a long stair the classic model falls off each step and
reaches a node on the bottom step in the air, so that one is checked when it
lands (`TestWalkDownStairsChecksOnLanding`).

Not yet: jump-ups and drops.

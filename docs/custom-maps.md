# Custom maps

Maps from files, loaded by the server at start and again whenever they change:

    counterslop --maps-dir ./maps            # env: COUNTERSLOP_MAPS_DIR

Every `<name>.json` in the directory is one map. A map that breaks the rules is
refused with the reason (logged as `custom map refused file=... why=...`); the
others load. Custom maps join the Maps screen (tagged *custom*), `changelevel`,
the map vote and `mp_maprotation` (name them; the default rotation is the
built-in maps), the MCP's `maps` and `changelevel`.

## The format

A map file is `content.Map` as JSON: the same bytes the built-in maps export to
and their goldens (`internal/content/maps/testdata/*.json`) hold. Everything a
built-in map has, a file can have:

| field | what |
|---|---|
| `name` | the map's name; must be the file's name (`de_x` in `de_x.json`) and not a built-in map's or a lab's |
| `boxes` | `{min, max, kind}` solids; `kind` a material (`content.Materials`); `ramp` makes a wedge (docs/physics.md) |
| `spawns`, `teamSpawns` | every spawn; each team's (`vibe`, `artisan`) |
| `buyZones` | per team; every team spawn must be inside one |
| `sites` | bomb sites `A` and `B` (defusal; `mode` `gpudrop` has `gpus` and `exits` instead) |
| `nav` | optional: the bots' graph, nodes `{name, pos, links}`; each team needs a node named after it, linked to one inside each objective. Left out, the server builds one from the geometry (docs/navgen.md), and the checks walk that |
| `items` | weapons lying on the map |
| `movers` | moving solids (escalators, `slide` boxes: lifts, doors, sleds) |
| `water`, `boats` | water volumes, boat spawns |
| `pits` | deadly volumes (`he_merge_hell`'s main) |
| `lights`, `fog`, `cut` | point/spot lights, fog, the height the overview cuts the lid off at |
| `rules` | server cvars the map pins (only `content.RuleCvars`: movement, boots, house rules) |
| `desc`, `maxPlayers`, `category`, `mode` | the Maps screen's card; `he_*` maps pin `mp_loadout nades` |

Unknown fields are an error (a typo would otherwise be a missing wall).

## The checks (`mapcheck.Check`, `mapcheck.Problems`)

A file is held to what every built-in map passes in the tests: known
materials, non-empty boxes, spawns and items out of the walls, team spawns in
their buy zones, valid lights, rules only from `RuleCvars`, the mode's naming
and sites, a category, a description, a nav graph without dangling links (or none: one is built, docs/navgen.md), and
the walk: every team, from every one of its spawns, walks the nav route to
every objective with the real movement code (the map's pinned `sv_movement`,
else both models) and must arrive. That is how bots get there. Limits: 16 MB,
20 000 boxes, 256 lights, 256 movers, 2 km from the origin.

## Hot reload

The server looks at the directory every 2 s (names, sizes, modify times). On a
change it loads every file again. The custom map being played, if its file
changed, loads again for everyone (a new match); if its file is gone or now
refused, it plays on until the next map, and the players are told.

## Making one

    counterslop map export de_slop --as de_my_slop --out ./maps   # a built-in map to start from
    counterslop map export all --out ./export                     # every map (labs by name)
    counterslop map export lab_harbor --out -                     # to stdout
    counterslop map check ./maps/de_my_slop.json                  # what --maps-dir would say

`sim/maps/de_hello_world.json` is a small hand-written one; `go run ./cmd/sim`
loads `sim/maps` (scenario `custom-map`).

Code: `internal/content/maps/custom.go` (the registry beside the built-in
maps, `Export`), `internal/content/mapcheck` (`Parse`, `LoadDir`, `Check`,
`Problems`, the walk: apart from `maps` so that `maps` never imports
`internal/sim`), `internal/app` (`ReloadMaps`, `MapsChanged`, `WatchMaps`),
`game.Server.MapsReloaded`, `mapcmd.go` (CLI).

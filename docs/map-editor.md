# The map editor: the server's edit session

One map is edited at a time, live, on the map in play: everyone on the server
sees each edit as it lands. Custom maps 3 is the server side and its one API;
the in-game editor UI (Custom maps 4) and the MCP tools (Custom maps 5) are
both clients of it. Code: `internal/game/mapedit.go`; client:
`web/game/mappatch.js`; flight: `internal/sim/noclip.go`.

## Commands (admin; rcon, the console, the MCP's rcon)

| command | what |
|---|---|
| `map_edit new <name>` | a blank map (a floor, a spawn and buy zone a side, sites A and B) |
| `map_edit clone <map> [<name>]` | any map or lab, under a new name (default `<map>_edit`; a built-in `de_x` becomes `de_myx`) |
| `map_edit` | the session: name, revision, boxes, undo/redo depth, playtest |
| `map_edit log [n]` | the last n ops: revision, author (the admin's name, or `mcp`), the op |
| `map_edit stop` | stop editing; the map plays on as it is |
| `map_op <json>` | one edit (below); `map_op help` lists them |
| `map_undo`, `map_redo` | step back and forward through everyone's ops; a new op drops the redo |
| `map_playtest [0\|1]` | play it as it is: a fresh match, bots on, nobody flies; again: back to editing |
| `map_save [name]` | check it as a custom map file is checked (mapcheck: docs/custom-maps.md) and write `<maps-dir>/<name>.json`; refused with the reasons; the custom maps then load again |

`changelevel` ends the session (unsaved edits are gone, and everyone is told).

## Ops (`EditOp`)

Typed and checked against the map before anything changes: a refused op
changes nothing and says why.

| op | fields |
|---|---|
| `box_add` | `box {min, max, kind, ramp}` (a wedge: ramp 1-4); prints the new index |
| `box_set` / `box_move` / `box_resize` / `box_material` / `box_ramp` / `box_delete` | `i`, and `box` / `by [dx,dy,dz]` / `min, max` / `kind` / `ramp` |
| `spawn_add` / `spawn_move` / `spawn_delete` | `team`, `pos`, `i` (`spawns` follows the team spawns) |
| `zone_set` / `zone_delete` | `kind` site (`name` A or B), pit (`name`), buy (`team`, `i`; `i` = the count adds one); `min, max` |
| `item_add` / `item_move` / `item_delete` | `weapon`, `pos`, `i` |
| `nav_set` / `nav_delete` | `name`, `pos`, `links` (deleting a node drops the links to it): the bots' hints |
| `rule_set` / `rule_delete` | `key` (only `content.RuleCvars`), `value` (as rcon would set it) |
| `meta_set` | `desc`, `maxPlayers`, `category` |

Checks: known materials, non-empty boxes, ramps 0-4, everything within 2 km,
real teams and weapons, sites A/B only, nav links to existing nodes, rules a map
may pin with values rcon accepts, and the limits (`mapcheck.MaxBoxes`, ...).
The whole-map rules (a walk to the sites, spawns in buy zones) are `map_save`'s.

## Deltas, revisions, and a network that may lose them

Each op compiles to patches on the map's JSON (`mapPatch`: at path `p`, `set`,
`ins` or `del` the array element `i`, the key `k`, or the field). The server
applies them to its own map and sends them to everyone as
`{"t":"mapdelta","rev":n,"patch":[...]}`; the client applies them the same way
(`mappatch.js`; `testdata/mappatch_golden.json` pins it to the Go) and redraws
without a loading screen. Undo sends the inverse patches, redo the patches.

The revision is state, the delta an event: every self-snapshot carries the
map's revision (`me.mr`). A client whose map is at another revision (a delta
lost, or one it could not apply) asks for the whole map (`{"t":"mapget"}`, at
most once a second) and gets `{"t":"mapsync","map":...,"rev":n}`.

## Editing vs playtest

While editing (a session on, playtest off):

- Admins fly: `sim.Body.Noclip` (along the view, jump up, crouch down, walk
  slow, through everything; no pits, no out-of-world respawn). The server sets it on
  every body it gives an admin; the self-snapshot carries it (`nc`) and the
  client predicts the flight with the same generated code (case `map-edit`).
- Bots stand still.

Playtest is the map as players will get it: a fresh match on spawns, nobody
flies, bots play. On a map without a nav graph (or with `bot_autonav 1`) the
bots route on one built from the geometry (`autoNav`, Custom maps 2,
docs/navgen.md): the server's alone, the map's own nav back when editing goes
on, never saved. Edits, undo and redo wait until playtest is off (the built
graph is not on the clients' copy of the map). A map saved without nav is fine:
mapcheck builds one the same way to walk it.

## The editor UI (web/game/editor.js)

While you fly in a session as an admin (noclip, playtest off), the client is an
editor: its tools take the mouse buttons and their keys (the gun is put away;
fire, scope, use, hook and reload never reach the command), and every edit goes
out as one of the ops above (`{"t":"rcon","cmd":"map_op",...}`), so undo, the
log and the other clients see exactly what an rcon line would do. Nothing
changes locally until the mapdelta comes back. A panel (web/ui/editorpanel.js)
says what the tool will make and lists the keys.

| key | |
|---|---|
| 1 select | click a box (outline, a handle on each face); click it again on a face and drag: resize that face (one `box_resize` on release); Shift+drag: move it (`box_move`); arrows / PgUp / PgDn nudge it a snap (along the world axis nearest the view); `[ ]` its material, R its ramp (none, +x, -x, +z, -z), C a copy stacked on top, Delete |
| 2 box | two clicks on any surface: a box standing on the first (the wheel sets its height); `[ ]` material, R ramp |
| 3 zone | two clicks: site A, site B, or the next buy zone of a team (Z cycles), 0.5 m under the floor to 3.5 m over it |
| 4 spawn | click: a spawn for the team (T); right click deletes that team's nearest |
| 5 item | click: a weapon on the floor (`[ ]` picks it); right click deletes the nearest |
| G | the snap: 0.25, 0.5, 1, 2 m (the cursor snaps on the surface's plane; the surface itself stays exact) |
| Ctrl+Z / Ctrl+Y / Ctrl+S / P | map_undo, map_redo, map_save, map_playtest |
| right click | cancel a half-made box or zone, else deselect |

Drawn as thin emissive bars: the selection and its handles, the hovered box,
the preview of what a click makes, sites, buy zones, spawns and items.
Scenario `map-editor-ui` drives it with real keys and the mouse; `editor.test.mjs`
pins the ops each gesture sends.

### The other editors

The editor publishes its presence (below: aim point, hovered box, selection,
tool) and draws everyone else's from the `editors` list: a cursor, a marker
over their head, their hovered object outlined thin and their selection thick,
in their colour (the agent, id 0, in cyan #00e6ff; an admin whose client sent nothing yet, age -1, shows by the marker over their head alone), and lists them in the panel.

## The op log

Every op, undo, redo, playtest and save is logged with its author and the
revision (`map_edit log`), up to 2000 lines; undo goes back 500 ops.

## Editor presence (who aims where, what they selected)

So editors (people in the in-game editor, the agent over MCP) see each other
and can act on "this" (`internal/game/mapedit_presence.go`). State, not
events: a lost message is made good by the next.

- Client -> server, at most 10 Hz, from an admin while a map is edited:
  `{t:"editpresence", at:[x,y,z]|null, hover:ref|null, sel:[ref...], tool:"..."}`.
  The server sanitises it (at most 64 refs, a tool name of at most 32
  characters, the aim point inside the world, refs that name nothing dropped).
- A ref is a thing in the edit ops' terms, so a selection feeds straight into
  an op: `{kind:"box",i}`, `{kind:"spawn",team,i}`, `{kind:"zone",zone:"site"|"pit",name}`,
  `{kind:"zone",zone:"buy",team,i}`, `{kind:"item",i}`, `{kind:"nav",name}`.
- Server -> everyone: `{t:"editors", rev, list:[{id, name, mcp?, at, hover,
  sel, tool, age, pos, yaw, pitch}]}`, the whole list, when it changed (at most
  every 100 ms) and every second anyway; an empty list when editing stops.
  `rev` is the map revision the refs index into; `pos/yaw/pitch` is a player's
  body as the server has it; `age` the seconds since that editor's last update.
- The agent is editor id 0 (`mcp: true`): its presence is set by the MCP
  (`map_presence`, and after each op it applies: what the op touched).
- An op that shifts indices (a box deleted before a selected one) moves every
  editor's refs with it; refs to what it deleted go.

## Co-editing with an agent (Claude Code over the server MCP)

The in-game editor and an agent edit the same session at the same time: each
op from either side is a delta everyone sees at once; undo is shared; the
agent sees where the people in game aim and what they selected, and they see
its selection (cyan in renders; the in-game editor draws it with its name).
The tools are in docs/mcp.md ("Map editor tools"); they are the admin MCP's,
so the same token, localhost by default.

```sh
# the server, with somewhere to save maps
export COUNTERSLOP_MCP_TOKEN=$(openssl rand -hex 16)
go run . --addr 127.0.0.1:8080 --maps-dir ./mymaps --mcp-addr 8091
# Claude Code, in the same shell
claude mcp add --transport http counterslop http://127.0.0.1:8091/mcp \
  --header "Authorization: Bearer $COUNTERSLOP_MCP_TOKEN"
```

Then, in game: log in as admin, and either of you starts the session
(`map_edit clone de_slop de_mine` in the console, or ask the agent). A loop
that works:

1. **Look**: `map_render` (top; then `side` or a `min`/`max` region for one
   floor or one corner, `labels` for box indices) and `map_get` (`fields:
   ["boxes"]` for exact numbers).
2. **"This"**: `map_editors` says what you aim at (`at`, or the server's
   `aim` from your view), hover and selected, with the objects themselves:
   "make this wall 2 m taller" is box `sel[0].i`'s `box_resize`.
3. **Edit**: `map_op` with a few ops at a time (a player is 1.8 m tall and
   jumps ~1.2 m; a crouch-jump clears ~1.5 m; floors' tops are usually y 0).
   What the agent touched becomes its selection, so you see where it worked.
   Anything it did can be undone from either side (`map_undo`).
4. **Check**: `map_validate` before saving; its reasons are concrete ("vibe
   spawn 2 -> A: stuck at [-2.4 0 8]": a box in the way). `map_playtest`
   plays it with bots; `map_playtest` again to edit on.
5. **Save**: `map_save` (or `map_save <name>` in game): it is then a custom
   map in the maps list for `changelevel` and the vote.

The agent's ops are logged as `mcp` (`map_log`), and the MCP logs every call.

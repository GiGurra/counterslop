# GPU Drop: design spec (map category `gs_`)

A second game mode next to Big Diff defusal (`de_`). The setting is a huge computer and
hardware superstore. The parody: the Vibe Coders are scalpers grabbing the last boxed GPUs
to run their local LLMs, and the Artisans are the store staff (the "Geek Squad of
craftsmanship") defending the stockroom. It plays like CS hostage rescue, with crates
instead of hostages. Our own layout and rules, not a copy of any CS map.

Owners: **vox** = the mode (server rules, net state, HUD, bots, cases).
**shadows** = the first map `gs_megabyte` (layout, prefabs, nav, scenario).
**music** = store assets (models, materials, sounds, announcer lines, music).
**lead** = this spec; changes to it go through lead.

## Rules (server-authoritative, `internal/game`)

- A map in this category has `Mode: "gpudrop"` (new `content.Map.Mode`; empty means
  defusal) and `Category: content.GPUDrop` (`"gpudrop"`, a third map-menu category "GPU
  Drop" after Training and Competitive).
- Map data (new in `content.Map`): `GPUs []GPUSpot` (pos, yaw: boxed GPUs on stockroom
  shelves, 4 by default) and `Exits []Zone` (named boxes: the loading dock, the front
  entrance; at least 2, far apart). There are no bomb sites; `Sites` stays empty.
- Vibe Coders attack and Artisans defend. No Big Diff is given in this mode.
- **Grab:** an attacker holding Use (E) within 1.5 m of a GPU for 1.0 s picks it up (a
  progress bar, like the defuse). One GPU per player.
- **Carry:** the carrier moves at 80% speed and can still shoot. The box is drawn on their
  back (third person) and as a hand item (first person). The carrier shows on the
  defenders' radar.
- **Drop:** on death, or G with nothing else to drop, the box falls where the player stands
  and anyone on the attacking team can pick it up again. A defender holding E on a dropped
  GPU for 3 s "restocks" it (back to its shelf spot).
- **Secure:** a carrier entering an exit zone secures the GPU (removed from play, +1 for the
  attackers, money).
- **Win:** attackers win when 2 GPUs are secured (cvar `mp_gpus_to_win`, default 2) or
  every defender is dead. Defenders win when the time runs out or every attacker is dead.
  A round can end mid-carry. Win reasons go to the kill feed and the announcer.
- Economy (economy.json `gpudrop` block): grab +$200, secure +$1000 (carrier) / +$500
  (team), restock +$300, round win/loss as in defusal.
- State: every snapshot's round state carries `gpus: [{st: shelf|carried|dropped|secured,
  by, p}]` and `secured: n`. The client draws and predicts from state only (CLAUDE.md:
  never depend on an event arriving). Events (grab/drop/secure/restock) are optional extras
  for sounds, the feed and the announcer.
- Freeze, buy time, warmup, halftime and side switching work as in defusal (shared round
  code, not a fork).

## Client

- HUD: a GPU counter in the top bar (secured / to win, and how many are left in stock), the
  grab/restock progress bar, hints ("Hold E to grab the RTX 9090", "Get it to an exit",
  "Hold E to restock"), and exit zones outlined like buy zones while carrying.
- Radar: GPU dots, plus the carrier for defenders.
- Announcer lines (SlopVox): "GPU secured", "Scalpers win", "Out of stock!", "Restocked",
  "Staff wins", "Someone grabbed a GPU".
- Music (assets/music): a "Black Friday" shopping-muzak stinger on a secure and on the
  round win. Muzak in the buy and freeze phases instead of the elevator loop (themed per
  mode).

## The map: `gs_megabyte` ("MegaByte Mart")

CS-scale or bigger, at least 3 levels, our own layout:

- **Ground floor:** the entrance and parking edge (one attacker spawn outside), checkout
  lanes, the gaming section (demo kiosks, TVs), a central atrium open to the floors above.
- **Upper floor:** computer rooms (PC build demo room, a LAN cafe with rows of desks and
  monitors), component aisles (shelving canyons), a cafe or food court, catwalks over the
  atrium.
- **Basement or back of house:** the stockroom (GPU shelves, the defenders' side), the
  loading dock (an exit zone), staff corridors, the server room.
- **Escalators:** at least 2 pairs between floors (ramps with step treads; an animated
  moving-tread material; moving the player is optional, ask lead first), plus stairwells
  and a freight lift shaft as a vertical flank (static).
- Defender spawn near the stock; attacker spawn outside (parking / front entrance).
  Several routes from attackers to stock and from stock to each exit, so carrying back out
  is a fight.
- Needs nav graph links for bots (stock, exits, spawns), TestWalkToSites generalized to
  GPU spots and exits, TestNoZFighting clean, a golden file, a scenario with views of every
  area, and a bot balance sweep (40-60%, or the operator's "short of 90/10").

## Assets (music worker; procedural only)

- Models: a boxed GPU (glossy box, fan art, a "9090" label), a carried pose on the back and
  a first-person hand item, shelving units, checkout counters with registers, PC towers and
  monitors (with a screen glow), a TV wall, shopping carts, price-tag signs, escalator
  balustrades.
- Materials: store floor (polished tile with reflection-ish specular), shelf metal, carpet
  (the LAN cafe), escalator treads (animated), glass balustrade, signage/neon.
- Sounds: grab (cardboard rip), restock (barcode beep), secure (cash register cha-ching),
  store PA chime before announcer lines, escalator hum (spatial).
- Everything goes in the gallery (/dev/assets).

## Testing (everyone)

Every rule gets a Go unit test and a sim/case (grab, carry speed, drop on death, restock,
secure, both win conditions, freeze/warmup). New commands and cvars need cases (the registry
rule). There is a scenario per visible thing. Small commits to main, green CI between
pushes, and coordinate pushes in the group when touching shared files (content.go, rounds
code, hud.js).

## Roof, windows and indoor lighting (lighting worker; engine)

The operator asked for windows, a roof, and indoor lights with more than one shadow
source. The engine today has one sun (a directional shadow map with PCF,
gen/glsl/shadow.js) and hemisphere ambient (gen/glsl/lighting.js). There are no local
lights.

- **The map** has a real roof and walls with windows. The glass storefront and skylights
  over the atrium let the sun in, so sun shadows still matter indoors: sunlit patches on
  the floor through windows and skylights. Under the roof, the sky ambient must fall off:
  an ambient-occlusion or "indoor" term so roofed areas aren't lit by the sky hemisphere.
  This could be a baked per-map sky-visibility volume, or a coarse 3D occupancy grid
  computed at load, seeded and deterministic.
- **Local lights** come from map data: `Lights []Light` in content.Map, {pos, colour,
  intensity, range, kind: point|spot, dir/cone for spots, shadow: bool, flicker seed}.
  Examples: fluorescent ceiling panels (many, unshadowed), store spotlights on displays,
  TV/monitor glow, neon signs, the emergency-exit green, the server room's blue.
- **Renderer** (web/engine/gl.js + gen/glsl):
  - Many unshadowed lights, through clustered or tiled forward shading, or a per-map
    light grid texture. This must stay cheap: the map is one merged draw call.
  - A few shadowed lights (spots with their own shadow maps, or a cube or dual-paraboloid
    map for a point light), budgeted: the nearest/most important N shadowed lights per
    frame (cvar `r_light_shadows`, 0-4, in Options → Video).
  - Static light shadows can be cached; only dynamic solids (players) need re-rendering.
  - Dynamic solids (players, weapons, corpses) are lit by and cast shadows from the
    shadowed lights.
- **Performance:** measure with the perf overlay on de_cloudfarm and gs_megabyte. Show the
  light count and shadow-map cost in the overlay. Keep the software-renderer CI scenarios
  workable (lower defaults under SwiftShader if needed).
- This also enables the board idea tk_e747ef2e (day/night with lamps). Design the API so
  outdoor maps can use lamps too.
- Tests: unit tests for the light grid and clustering math, a lighting scenario (a test
  room map with a few coloured lights and shadowed spots, checked by eye), and the
  gallery showing the lighting test room.

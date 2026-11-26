# Profiling: where a frame (and a server tick) goes

Two profilers, one idea: every frame (client) or tick (server) is split into named
sections, timed, and summarised over a rolling window as mean, p95, p99, max and calls
per frame. Both report wall time, so they never touch gameplay.

## Client: `cl_showprof`, `prof_dump`, `prof_reset`

| | |
|---|---|
| `cl_showprof 1` | the overlay (top left, under `cl_showfps`): **what limits the frame rate**, the GPU, resolution and refresh rate, frame/CPU/idle/GPU ms, and the six sections costing the most (self time, GPU ms in brackets) |
| `cl_showprof 2` | the whole tree: per section CPU mean, self, p95, p99, max, calls per frame, and GPU mean/p95 |
| `prof_dump` | prints the `cl_showprof 2` table into the console (copy it into a bug report) |
| `prof_reset` | starts the window afresh |
| Options → Video | "Frame profiler" next to "Show FPS" |

The profiler only runs while `cl_showprof` or `cl_showfps` is on (or a bench asks). Off,
every `begin`/`end` is one boolean check (~0.6 µs a frame for all of them); on, ~3 µs a
frame for ~30 sections plus one GPU timer query per render pass. The overlay's text is
rebuilt 4 times a second, never per frame.

### The verdict (top line)

`verdict()` in `web/engine/prof.js` (pure, unit tested) reads the window:

- **GPU-bound**: the GPU's time for the frame (timer queries, scaled by the frames that drew
  anything; or a bench's `gpu-wait`) is ≥ 80 % of the frame interval. It names the heaviest GPU pass. Lower `r_render_scale`,
  `r_shadow_quality`, `r_aa`, `r_light_shadows`.
- **CPU-bound**: the main thread's measured work is ≥ 80 % of the frame. It names the
  section with the most self time.
- **Capped by fps_max** / **Holding the display's N Hz (vsync)**: the frame interval is
  the cap or the refresh; the rest is headroom ("x % of the frame used").
- **Throttled by the browser**: frames far slower than the refresh with CPU and GPU idle
  (power saving, the window not focused). **Background tab**: `document.hidden`.
- **Probably GPU-bound (this browser can't time the GPU)**: no
  `EXT_disjoint_timer_query_webgl2` (Firefox, Safari), slow frames, idle CPU.

### Sections

Time between two frame boundaries that no top-level section covers is **idle**: waiting
for vsync or the cap, the browser's own work (compositing, input, GC).

| section | what |
|---|---|
| `frame` | the loop's tick (`main.js frame`); its self time is the glue between the steps |
| `frame/link` | the link status, a replay's clock |
| `frame/input` | mouse look, observer camera |
| `frame/predict` | our commands: built, sent and predicted (`simTick`), the reconciliation offset's decay |
| `frame/world` | cosmetics: puffs, bursts, rounds, hums, boats, editor, grenades, debris, damage numbers |
| `frame/mapload` | handing a new map to the renderer (the light grid and meshes are built here) |
| `frame/render/view` | camera, viewmodel, crosshair, damage HUD |
| `frame/render/remotes` | other players interpolated (`remotes.sample`) |
| `frame/render/scene` | every solid to draw: player models, corpses, items, grenades, effects |
| `frame/render/gfx` | the renderer (`engine/gl.js frame`); its GPU column is the whole frame's GPU time |
| `.../gfx/shadow` | the sun's shadow map (map + solids into the depth map) |
| `.../gfx/lights` | light culling, dynamic light updates (CPU only) |
| `.../gfx/lamps` | lamp shadows: baking static tiles, players drawn into live ones |
| `.../gfx/dynshadow` | cube shadows of dynamic lights (muzzle flashes) |
| `.../gfx/setup` | packing lights, contact blobs, programs, binding textures, the render target, clear |
| `.../gfx/sky`, `world`, `marks`, `solids`, `viewmodel`, `post` | the main passes: sky, the map (and escalator treads), bullet holes and casings (instanced), solids, the viewmodel, the AA/upscale resolve |
| `.../gfx/submit` | the frame's fence, `flush` (and `finish` with `r_finish`) |
| `frame/render/hud` | radar, screen effects, name tags |
| `net` | socket messages between frames: `decode` (JSON / delta snapshots), then one section per message type (`snap`: `remotes` ingest, `reconcile`) |
| `ui` | Preact re-rendering the HUD and menus (batched, after the frame) |
| `audio` | building WebAudio graphs for sounds (top level, or inside the section that played them) |
| `gpu-wait` | benches only: waiting for the GPU to finish the frame (`gl.readPixels`) |

GPU times come from `EXT_disjoint_timer_query_webgl2`: one `TIME_ELAPSED` query per pass,
back to back (queries can't nest), so the passes add up to the frame. Results arrive a
few frames later (WebGL hands them out only between tasks); at most 6 frames are in
flight, a disjoint event throws them away. Without the extension the GPU column says `n/a`.

## Headless: test API and `cmd/sim`

- `__cs.prof()`: the current window (`table()` rows, verdict, the `cl_showprof 2` text).
- `__cs.profile(n, {dt, label})`: n drawn frames, each waited for on the GPU, over every
  frame (not the 1.5 s window); pushed to `state().prof.log`. `state().prof` also has the
  overlay's text and the sections measured.
- `go run ./cmd/sim perf-profile` (realtime, nightly): profiles every competitive map from
  vibe's spawn and writes `out/snapshots/perf-profile/profile.md`, also printed to the log
  and the CI job summary, plus snapshots of the overlay at `cl_showprof 2` and `1` on de_slop.
  Headless Chrome renders with SwiftShader (software GL): the GPU is the CPU, so compare
  sections within a run, never across machines.

## Server: `sv_prof`

`rcon sv_prof` (admin; the MCP's `server_profile` tool returns the same as JSON) prints the
100 Hz tick over the last 200 ticks (2 s): the whole tick against its 10 ms budget, then each
section largest first. `sv_prof reset` measures afresh.

| section | what |
|---|---|
| `vote` | the map vote (a finished one changes the map) |
| `nades` | grenades in flight, refills |
| `editors` | map-editor sessions |
| `ai` | playing bots think and queue their command |
| `puppets` | MCP puppets' scripted commands |
| `cmds` | every queued command run (`simTick` → `runCmd`: movement, weapons, hits) |
| `boats`, `round`, `bots`, `items` | boats, the round state machine, target bots, item respawns |
| `history` | lag-compensation history |
| `snapshot` | build, encode and queue every player's snapshot (the net codec) |
| `tv` | the replay recorder's frame |
| `mcp` | the MCP's observer snapshot |

The server reads the wall clock only in `profClock` (allowed in `TestWallNowOnlyWhereAllowed`:
a report of how long work took); tests inject a scripted clock (`Server.prof.now`).

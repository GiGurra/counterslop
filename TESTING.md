# Testing counterslop

| command | what | time |
|---|---|---|
| `make test-fast` | `go vet` + `go test ./...`: Go unit tests, JS unit tests (`web/**/*.test.mjs`, run from Go), sim/cases at the go and js levels, server↔client trace agreement, weapon golden vectors, physics parity | ~10 s |
| `make coverage` | the coverage ratchet: Go per package and JS lines vs `coverage-floors.txt` (raise a floor when you raise coverage, never lower one) | ~15 s |
| `make cases` | sim/cases end to end in lockstep (real server + headless Chrome, real input) + the action registry check | ~25 s |
| `make sim-smoke` | the CI subset of sim scenarios (assertions; `SHOTS=` for the screenshots too), 4 at a time (`J=4`) | ~2–3 min |
| `make test` | all of the above (what CI runs) | |
| `make simdiff` | what your change does to gameplay, per case and tick, vs HEAD~1 (`BASE=main make simdiff`) | ~10 s |
| `make snapshots` | every scenario, contact sheet at `out/snapshots/index.html` | minutes |

CI (`.github/workflows/ci.yml`, gate check `ci`) runs on every push: test (unit tests once, with
coverage; behaviour diff in the summary), cases (lockstep e2e + registry), sim (the smoke scenarios in
four shards, `-no-shots`) and sim-throttled (the short timing-sensitive scenarios at 6x CPU). The
nightly workflow (`nightly.yml`) is the same workflow with input `scope: full` (also
`gh workflow run ci.yml -f scope=full`): the same jobs run every scenario with screenshots (sim, in
three shards), the long timing-sensitive scenarios at 6x CPU (`make sim-throttled-nightly`: 45-160 s
each throttled, too slow for the gate) and every case, plus a golden-image diff against the previous
nightly's snapshots. That diff is advisory and goes in the summary.

**What CI draws:** nothing, unless a step asks. No assertion reads pixels, and software GL
(SwiftShader) costs seconds a frame, so a lockstep page's frames do all but the GPU work (the
camera for `gfx.project`, HUD, tags, radar; `noGL` in `engine/gl.js`). A `snapshot` draws one frame
(`__cs.draw`, once the map is ready; `settleLights` first for baked lamp shadows) and saves the PNG; under `-no-shots` (the CI targets)
it writes only the JSON. A step that asserts what the renderer did (`gfx.*`, `marks.*` stats) puts
`{"draw": true}` (`"bake": true` for lamp-shadow counts) before it. A failed step always writes
its `fail-<client>.png`.

## The layers

1. **Unit tests** next to the code: `*_test.go`, `web/**/*.test.mjs` (node `--test`, no browser).
   Keep client logic in pure modules (controls.js, arsenal.js, localsim.js, ...) so it can be tested here.
2. **sim/cases** (`sim/cases/*.json`, format at the top of `web/game/cases.js`): players, a per-tick
   command script, expectations. One file, three levels:
   - `go`: the server's command path (`internal/game/cases_test.go`);
   - `js`: the client predictor (`web/game/cases.test.mjs`);
   - `e2e`: lockstep (`go run ./cmd/sim -cases`). The harness steps the server one tick at a time and the
     client one fixed frame at a time, and drives input through real key, mouse and wheel events.
     Client and server must agree every tick. Nothing in a case reads the wall clock: the server is
     in lockstep from the world reset on (tick 0, player IDs from 1), setup goes through the gate,
     the pages run on the virtual clock (vclock.js, 10 ms per tick), and every tick the server first
     reads everything each page sent (a ping barrier) and each page takes in everything the tick sent
     (a sync marker). Fake link faults (net_fakelag, jitter, reorder) hold packets back on the page's
     clock: the harness moves that clock on while one is held, and resends a lost command as the next
     packet would. So a case plays the same bit for bit on any machine, at any `-j`, in any order:
     `-trace DIR` writes each case's per-tick server state (players, bots, command seqs) to compare
     runs. A case's `wallMs` step stalls in real time on purpose: it checks that nothing moves then.
   `TestCasesAgree` fails on the first tick where the go and js levels disagree.
3. **Action registry:** a full `-cases` run fails if any bindable action, command button, console
   command, client cvar (web/ui/settings.js CVARS) or admin/rcon command (read live from the client
   and the server) has no passing e2e case. A case covers a cvar by setting it (`"exec": "<cvar> <value>"`);
   restore the default at the end of the case (settings persist across case pages).
   Exempt one only with a reason, in `sim/registry-exempt.json`.
4. **Sim scenarios** (`sim/scenarios/*.json`, `cmd/sim`): scripted headless sessions with screenshots.
   Every scenario must assert something (`expect*` steps) and not just take pictures. They run in
   **lockstep** like the cases: `step` runs the client frames and exactly the server ticks they cover
   (each frame's commands reach the server first), `wait` runs server time, and a call only gets
   game time when a helper waits for the server (testapi's gate: one tick per request, so
   `waitState` takes exactly the ticks its state needs). The page's clocks and timers run on game
   time too (`cmd/sim/vclock.js`, injected into lockstep pages only). Nothing reads the wall clock,
   so a run plays the same on a loaded machine: `-cpu-throttle 6` (CI job `sim-throttled`) checks
   that. Gameplay code on the server reads `s.clock()`, never `time.Now()` (`TestGameClockOnly`);
   helpers wait on state, never on a fixed time. Every tick's snapshot reaches the pages 10 ms
   after the last on their clocks (calls, waits and frames alike), so a page sees a clean link
   (`lockstep-link`); fake link faults (lag, jitter, loss, reorder) and the live loop
   (`__cs.live`, the probes) run on the virtual clock too, so lag compensation, what-you-see
   checks and bad-link probes are lockstep. Only scenarios about wall time itself set
   `"realtime": true`: frame pacing and the loop (pacing, frame-pacing, latency-knobs), audio
   (audio-resume) and render cost (perf-*). They run nightly, not in the CI gate. A failed step writes
   `fail-<client>.png/.json`, the server's view (`-server.json`) and the last shot (`-testctl.json`).
   Precise gameplay rules still belong in sim/cases.

## Rules

- **A bug fix lands with a test that fails without the fix.** Prefer a sim/case (it pins both sims
  and the e2e path at once); otherwise a unit test.
- **A feature lands with unit tests**, plus a scenario assertion or case if it is user-visible.
- A new command, bind or cvar needs a sim/case exercising it (CI enforces this).
- After a gameplay change run `make simdiff` and check that it shows only the intended effect.
- **Tests never depend on real-time clocks; every clock is scriptable.** The server ticks when the
  harness steps it (lockstep) and reads game time (`s.clock()`); every page runs on the virtual clock
  (`cmd/sim/vclock.js`: `performance.now`, `Date.now`, timers, animation frames), moved by the
  harness 10 ms per server tick; fake link faults and the live loop run on it too. So a case or
  scenario plays the same, bit for bit, on any machine, at any `-j`, in any order. Real time is
  left only for:
  - hang guards (a stuck page fails the run, never hangs it: CDP calls, the per-case deadline,
    waits for the browser's own work such as a shader compile or a reload);
  - reports of how long something took;
  - scenarios whose subject is wall time itself (`"realtime": true`: frame pacing, the latency
    readout, audio, render benches; nightly only, `TestRealtimeOnlyForWallTime`).
  Checks: `TestWallClockOnlyWhereMarked` (every real-time use in `cmd/sim` and `web/game/testapi.js`
  carries a `wall:` reason on its line), `TestWallNowOnlyWhereAllowed` and `TestGameClockOnly`
  (the server's wall clock only for socket deadlines, the production ticker and the harness's
  waits). A test helper waits on state in game time (`until`, `gameSleep`), never on a fixed wall
  time. `go run ./cmd/sim -cases -trace DIR` writes per-tick server state to compare two runs.

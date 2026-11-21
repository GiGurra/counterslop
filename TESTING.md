# Testing counterslop

| command | what | time |
|---|---|---|
| `make test-fast` | `go vet` + `go test ./...`: Go unit tests, JS unit tests (`web/**/*.test.mjs`, run from Go), sim/cases at the go and js levels, server↔client trace agreement, weapon golden vectors, physics parity | ~10 s |
| `make coverage` | the coverage ratchet: Go per package and JS lines vs `coverage-floors.txt` (raise a floor when you raise coverage, never lower one) | ~15 s |
| `make cases` | sim/cases end to end in lockstep (real server + headless Chrome, real input) + the action registry check | ~25 s |
| `make sim-smoke` | the CI subset of sim scenarios (screenshots + assertions) | ~2–3 min |
| `make test` | all of the above (what CI runs) | |
| `make simdiff` | what your change does to gameplay, per case and tick, vs HEAD~1 (`BASE=main make simdiff`) | ~10 s |
| `make snapshots` | every scenario, contact sheet at `out/snapshots/index.html` | minutes |

CI (`.github/workflows/ci.yml`, gate check `ci`) runs on every push: test (unit tests, coverage,
behaviour diff in the summary), cases (lockstep e2e + registry), sim (the smoke scenarios in two
shards). The nightly workflow (`nightly.yml`) runs every scenario and every case, plus a golden-image
diff against the previous nightly's snapshots. That diff is advisory and goes in the summary.

## The layers

1. **Unit tests** next to the code: `*_test.go`, `web/**/*.test.mjs` (node `--test`, no browser).
   Keep client logic in pure modules (controls.js, arsenal.js, localsim.js, ...) so it can be tested here.
2. **sim/cases** (`sim/cases/*.json`, format at the top of `web/game/cases.js`): players, a per-tick
   command script, expectations. One file, three levels:
   - `go`: the server's command path (`internal/game/cases_test.go`);
   - `js`: the client predictor (`web/game/cases.test.mjs`);
   - `e2e`: lockstep (`go run ./cmd/sim -cases`). The harness steps the server one tick at a time and the
     client one fixed frame at a time, and drives input through real key, mouse and wheel events.
     Client and server must agree every tick.
   `TestCasesAgree` fails on the first tick where the go and js levels disagree.
3. **Action registry:** a full `-cases` run fails if any bindable action, command button, console
   command or admin/rcon command (read live from the client and the server) has no passing e2e case.
   Exempt one only with a reason, in `sim/registry-exempt.json`.
4. **Sim scenarios** (`sim/scenarios/*.json`, `cmd/sim`): scripted headless sessions with screenshots.
   Every scenario must assert something (`expect*` steps) and not just take pictures. They run in
   **lockstep** like the cases: `step` runs the client frames and exactly the server ticks they cover
   (each frame's commands reach the server first), `wait` runs server time, and during calls the
   server ticks along so helpers get their replies. So a run doesn't race a loaded machine. Only
   scenarios about wall time itself set `"realtime": true` (pacing, frame-pacing, audio-resume,
   latency-knobs, bandwidth, fake-lag lag-compensation, resync-faults' dropped commands). A failed step writes
   `fail-<client>.png/.json`, the server's view (`-server.json`) and the last shot (`-testctl.json`).
   Precise gameplay rules still belong in sim/cases.

## Rules

- **A bug fix lands with a test that fails without the fix.** Prefer a sim/case (it pins both sims
  and the e2e path at once); otherwise a unit test.
- **A feature lands with unit tests**, plus a scenario assertion or case if it is user-visible.
- A new command, bind or cvar needs a sim/case exercising it (CI enforces this).
- After a gameplay change run `make simdiff` and check that it shows only the intended effect.

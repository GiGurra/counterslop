# Shared game logic: one source for the server (Go) and the client (JS)

Status: **adopted, on main** (phase 2 below). The rest of this document is the
phase-1 research that led there.
Date: 2026-11-21. Toolchain measured: go1.27.4 linux/amd64, node v26.13.0 (V8).

## Phase 2: what is generated now

The client's simulation and model code is generated from Go; see CLAUDE.md "sim and
model code is written in Go" for the rules agents follow.

| Go (one copy) | generated | hand-written adapter |
|---|---|---|
| `internal/sim` movement + collision | `web/engine/sim.gen.js` (195 lines) | `physics.js` (42, was 130) |
| `internal/detmath` | `web/gen/detmath.gen.js` (219) | — |
| `internal/content/rng` | `web/gen/rng.gen.js` (31) | `rng.js` (22) |
| `internal/game/cmd_shared.go`: buttons, `MoveInput` | `web/game/shared.gen.js` | `cmd.js` (18) |
| `weapons_shared.go`: registry, settle/fire/trigger/reload/switch/drawn, `Ticks`, `ShotSpread`, `MoveSpeed` | ″ | `arsenal.js` (286, was 334; now only sounds, HUD, viewmodel, scope, input) |
| `recoil_shared.go`: spray index/offset/angles, `SpreadSeed`, `ShotAngles` | ″ | `arsenal.js` re-exports |
| `hitbox_shared.go`: `Model.Seeded`, `PartsFor`, pose, `RayModel`, `segPointDist` | ″ (825 lines in all) | `boxmodel.js` (89, was 121; plus the draw matrices) |

1,270 lines of generated JS replace the hand-written rules. Left hand-written on purpose:
`localsim.js` (22 lines: one predicted command over the generated pieces), `predict.js`
(reconciliation), the renderer's float32 draw matrices, `hitbox.extern.js` (the team-filter
JSON decoder, 8 lines, `//gojs:extern`), and everything audiovisual.

Every pre-existing test passed unchanged against the generated code: the JS unit tests
(174 then, 290 after merging main), the weapon golden vectors, the sim/cases at the go, js
and lockstep e2e levels (44, then 64 after the merge), and simdiff showed no behaviour change.

### Drift the generation found (bugs in the hand-written twins)

- **Reload completing on the switch tick:** `arsenal.js` deployed the new weapon before
  settling the reload, so the client lost a reload the server counted (ammo 27 vs 30). Golden
  case "a reload completing on the switch tick fills the mag" fails on the old JS.
- **Move speed, 1 ulp:** JS multiplied weapon × drink × carry, Go drink × carry × weapon.
  For the Hotfix with the drink while carrying a GPU: 0.8096000000000002 vs …01, so every
  predicted tick in that state mismatched and replayed. Pinned in `arsenal.test.mjs`.
- **Hit test:** the client ray-tested float32 render matrices, the server float64 frames. The
  two agreed within 2e-4 m, now they agree to the bit. Go's random map order also made the sum
  of the pom-pom's two `vary` terms vary by an ulp between server calls; it is sorted now.
- **arm64 FMAs** in shared code: `ShotAngles`, the recoil offsets, `Ticks`, the pose and the
  slab test, all found by the translator's lint or `TestNoFusedMultiplyAdd` and fixed with
  `float64()` barriers.

The magnitudes are tiny (1e-16 relative), and a 1e-6 aim difference doesn't change who you
hit. They still matter here because prediction compares state **exactly**: any mismatch is a
reconcile and replay, and a mismatch in a timer (the reload case) is a visible rubber-band.

### Guards added in phase 2

- CI step **"generated JS up to date"** (`go run ./cmd/gojs -all -check`) in the `test` job,
  before the tests: a commit whose `.gen.js` doesn't match its Go fails.
- `TestNoFusedMultiplyAdd` checks only the translated functions of a partially shared
  package (`internal/game`'s `//gojs:shared` files), closures included.
- Translator: nil maps are `null` (they were `{}`, so `m == nil` was never true), struct
  slices from `make` were `undefined` elements, named results are rejected by name.

### Directives

- `//gojs:shared` above `package`: only those files of the package are translated.
- `//gojs:skip` on a declaration: not translated (shared code calling it is an error).
- `//gojs:extern ./x.js` on a function: imported from hand-written JS (for hand-optimised
  or browser-specific pieces). Comment why, and pin the JS to the Go with a test.

## TL;DR

- **Recommendation: write the shared rules in Go and translate them with a small
  in-repo transpiler for a restricted subset (`cmd/gojs`, `internal/gojs`), one twin at a time.**
  The prototype does this for movement (`internal/sim` → `web/engine/sim.gen.js`).
  Server and client now agree **bit for bit**; before, they agreed only within 1e-6.
  Every existing test passes, and the generated JS is ~25% *faster* than the hand-written twin.
- Bit-exactness needs two more things a transpiler alone doesn't give you. Both are in the
  prototype: **deterministic math** (`internal/detmath`, because V8's `Math.sin/cos/exp` differ
  from Go's in the last bit for 15–38% of inputs) and **no fused multiply-add** (Go fuses
  `a*b+c` on arm64 and GOAMD64=v3, and we ship arm64 binaries).
- Go→WASM works, but it is 2 MB (600 KB gzipped) against ~10 KB of JS and costs 24 ms to start.
  It is only fast if the JS↔Go boundary is redesigned around shared memory (35× slower through
  `syscall/js` objects), and it isn't bit-exact either without the same detmath work.
  GopherJS supports only Go ≤ 1.21, so it can't build this module (Go 1.27). Going the other way
  (JS/TS → Go) has the same work as gojs, plus a less-typed source language.
- **Keeping hand-written twins is a legitimate choice.** The twins change rarely (physics: 3
  commits in 188), and the golden vectors, cases and simdiff already catch drift. The case for
  gojs is the weapon rules (17 commits each side, the biggest twin, the bugs found by the golden
  vectors), plus what bit-exactness buys (see [Recommendation](#recommendation)).

## What is twinned today

| rule set | Go | JS twin | guard |
|---|---|---|---|
| movement | `internal/sim/physics.go` (215 lines) | `web/engine/physics.js` (130) | `TestParityWithJS` (1e-6), sim/cases, `TestCasesAgree` |
| weapon timers, ammo, reload, burst, bolt | `internal/game/weapons.go` (rules ≈ 150 of 435) | `web/game/arsenal.js` (334, mixed with HUD/sound/animation) | `weapons_golden.json`, cases |
| recoil spray | `internal/game/recoil.go` (≈ 50) | `arsenal.js` `recoilIndex/Offset/Angles` | golden vectors, `spray-pattern` case |
| seeded PRNG | `internal/content/rng` (23) | `web/gen/rng.js` (23) | `rng_test.go` against JS output |

History: `physics.js` changed in 3 of 188 commits, `arsenal.js` in 16, `weapons.go` in 17.
Commit 88d2612 ("Weapon golden vectors shared by Go and JS; fix two reload bugs they found")
records real drift between the twins.

**The twins were never bit-exact.** With the parity tolerance set to 0, `HEAD` fails at
tick 77 of the first run: y = 1.0987499999999986 in Go and 1.0987499999999983 in JS. The cause
is Go's exact constant arithmetic. `legs = H - HCrouch` is the untyped constant 0.6 in Go, while
JS computes `1.8 - 1.2` = 0.6000000000000001 at run time. Reconciliation hides it: the server's
state wins, and corrections under 1e-4 are invisible. But it shows that hand-kept twins drift in
ways nobody intended.

## Determinism evidence (measured)

### 1. Go math vs V8 Math (200,000 seeded inputs in ±20)

| function | results that differ | max error |
|---|---|---|
| sin | 25.1% | 1 ulp |
| cos | 24.4% | 1 ulp |
| exp | 15.6% | 2 ulp |
| atan / atan2 | 26.3% / 27.0% | 1 / 2 ulp |
| pow | 38.4% | 3 ulp |
| log | 0.6% | 1 ulp |
| sqrt, hypot | 0 | 0 |

So any approach that maps `math.Sin` to `Math.sin` (a transpiler, GopherJS, or a TS→Go port)
cannot be bit-exact. Go itself isn't consistent across architectures either. `math.Exp` and
`math.Hypot` have assembly versions on amd64 (Exp is 1–2 ulp from the pure-Go algorithm). The
pure-Go versions used on arm64 and WASM are compiled with FMA on arm64.

**Fix in the prototype:** `internal/detmath` holds Go's own Cephes `sin`/`cos`, its pure `exp`
and its `hypot` algorithm, written using only IEEE-exact operations. It is transpiled like any
other shared package (`web/gen/detmath.gen.js`). Measured results:
- Sin/Cos equal the standard library's on amd64, bit for bit, so the server's behaviour is unchanged.
- Hypot shows 0 ulp difference on 200,000 inputs.
- Exp is ≤ 2 ulp from amd64's assembly `math.Exp`. In the sim, Exp only feeds the cosmetic
  `eyeH`/`eyeOff`.
- `TestDetmathBitsJS` checks 400,000 Go↔JS results bit for bit, including inputs up to 1e9.

### 2. Fused multiply-add (FMA)

The Go spec allows fusing `x*y + z` into one FMA instruction, which rounds once instead of twice.
Measured with `go build -gcflags=-S`:

| target | `a*b + c` | `t := a*b; t + c` (across statements) | `float64(a*b) + c` |
|---|---|---|---|
| amd64 (GOAMD64=v1, the default) | not fused | not fused | not fused |
| amd64 GOAMD64=v3 | **fused** (VFMADD231SD) | — | not fused |
| arm64 | **fused** (FMADDD) | **fused** | not fused |

JS never fuses. The only portable barrier is the explicit conversion `float64(a*b)`. The spec
says it forces rounding, and V8 doesn't fuse anyway, so in JS it is a no-op.
`-gcflags=all=-d=fmahash=n` also disables fusion, but it is an undocumented compiler debug flag.

### 3. The arm64 FMA finding affects `main` today

`.github/workflows/release.yml` ships linux/arm64 and darwin/arm64 server binaries. On arm64,
`internal/sim` on `main` compiles to **7 fused multiply-adds**, at physics.go:157 (eyeH), 171 ×2
(move direction), 192–193 (horizontal velocity), 200 (gravity) and 211 (stride). An arm64
server's movement is therefore a different function from the amd64 server's and from the
client's. (internal/game has more such sites: `Ticks()`'s `sec*SimHz - 1e-6`, recoil
interpolation and angles.)

**How big is the drift?** I emulated the arm64 build exactly: a copy of the sim with `math.FMA` at
those 7 sites, compared against the unfused sim over the `TestParityWithJS` input scripts on
de_slop and de_runway. That is 15 runs × 1,500 ticks, with "reconcile" windows that restart both
from the same state the way the predictor does.

| inputs | window | max position drift | windows ending different | ground-flag / footstep mismatches |
|---|---|---|---|---|
| keyboard (F, R ∈ {-1, 0, 1}) | 1–1500 ticks | **0 m** | 0 | 0 / 0 |
| analog (F×0.73, R×0.41) | 10 ticks (~1 RTT) | 7.1e-15 m | 1/900 (de_slop), 2/1500 (de_runway) | 0 / 0 |
| analog | 1500 ticks (no correction) | 7.1e-15 m | 3/6, 5/10 | 0 / 0 |

Why keyboard input shows none: the direction products are exact when F and R are ±1. The fused
velocity residual (`wx*mx - vel`) is at most half an ulp of `vel`, so adding it rounds away. The
gravity term changes the rounding too rarely to show up in these runs. Stride differs by ~1e-15
and eyeH by 2e-16 (cosmetic).

**Severity: low today.** The drift is ≤ 1e-14 m, eleven orders of magnitude below the
predictor's 1e-4 correction threshold, so reconciliation hides it completely and players can't
see it. It is still latent. Analog input (a gamepad), more arithmetic, or a bit-exact protocol
(see Recommendation) would expose it. The branch fixes it for the shared packages, and a test
keeps it fixed:
- `internal/gojs` rejects a float product used directly as an operand of `+`/`-` ("write
  float64(a*b)").
- `TestNoFusedMultiplyAdd` cross-compiles every shared package for arm64 and fails on any
  FMADD/FMSUBD instruction. It runs on the amd64 CI runners; the same build of `main`'s
  `internal/sim` has 7 such instructions; the branch has 0.

After the wraps, an amd64 test with `GOAMD64=v3` stays bit-exact, and the arm64 build contains
0 fused instructions.

## The options

### a. In-repo Go→JS transpiler for a subset (prototyped)

**How it works:** `go/parser` + `go/types` (standard library only, no new dependencies) load a
package. A ~1,300-line translator (`internal/gojs/gojs.go`) emits one readable ES module per
package. The generated files are committed: no build step, which fits CLAUDE.md, and
`index.html` keeps loading plain modules. `go generate ./internal/gojs` rewrites them, and
`TestGeneratedUpToDate` fails when one is stale. That test runs in `go test ./...`, so CI already
gates on it.

**The output:** `web/engine/sim.gen.js` (195 lines) mirrors `physics.go` almost line for line.
It keeps the Go doc comments, adds a `// physics.go:149` marker per function, uses the same names
(fields become lowerCamel or take the json tag, so `Pos` → `pos`, `Min` → `min`; the objects are
the ones the client already uses), and adds a JSDoc typedef per struct. In the browser you debug
real JS: breakpoints work, there is no source map, and the marker points at the Go line.
`web/engine/physics.js` is now a 42-line adapter: it re-exports constants, wraps `NewBody` /
`World_Step`, and keeps the client-only `raycast`.

**The subset** (enforced with errors; `TestRejects` pins them):

| allowed | rejected (with a position and a reason) |
|---|---|
| consts (folded exactly by go/types, emitted as the exact float64), package vars, funcs, methods (→ `T_name(recv, …)`), struct types, type aliases | goroutines, channels, select, defer, closures, interfaces, embedding, generics |
| bool, string (literals, ==, +), ints (must stay within ±2^53), float64, fixed arrays, slices, structs, pointers to structs | maps (*next to add*), bit operations (*needed for rng*), append, slicing, variadics |
| := = op= ++ --, var, if/else, for (3-clause, cond, forever), range over int/slice/array, switch (no fallthrough; `break` leaves the switch), labels, break/continue | `math.Sin/Cos/Exp/Pow/Atan…` (use detmath), fusable `a*b ± c` |
| len, min, max, `math.Abs/Sqrt/Floor/Ceil/Trunc/Min/Max/Mod/Inf/NaN`, numeric conversions | exported functions with several results (unexported helpers may: `a, b := f()`) |

Semantics it keeps (`TestSampleAgrees` runs `internal/gojs/internal/sample` in Go and in node and
compares bits):
- integer `/` truncates;
- float→int conversion truncates;
- struct and array *values are copied* on assignment, in literals, and on return;
- parallel assignment evaluates every right-hand side first (`x, y = y, x`);
- shadowing (`s := s + 10`) gets a fresh JS name, so there is no temporal-dead-zone error;
- `switch` breaks.

One rule replaces copying on calls: struct and array parameters and range values are passed by
reference, so the translator rejects any function that assigns into one. This keeps the hot path
allocation-free.

**Results:**

| check | result |
|---|---|
| `TestParityWithJS` (de_slop, 6 runs × 1,500 ticks, walk/run/strafe/jump/crouch-jump/steps) | tolerance tightened **1e-6 → 0**: bit-identical every tick |
| `make test-fast` (go vet, Go + JS unit tests, sim/cases go+js, `TestCasesAgree`, golden vectors) | pass |
| `go run ./cmd/sim -cases` (lockstep e2e, headless Chrome, action registry) | 44/44 pass |
| `go run ./cmd/simdiff` | "No behaviour change: all 42 case traces are identical" (traces are rounded to 1e-4) |
| server trajectories, HEAD vs branch (amd64, bit-level hash over all parity runs) | identical (`25993237038d29c8`) |
| `make coverage` | pass (floors added: gojs 82, detmath 99) |
| `jump-crouched` scenario + screenshot | pass, renders normally |
| per-tick cost, node, de_slop, `node web/engine/physics.bench.mjs old.mjs` | **generated ≈ 700 ns, old hand-written ≈ 940 ns** |
| size | sim.gen.js 5.2 KB + detmath.gen.js 4.5 KB (raw) |

The generated code is faster because it uses index loops where the old code used
`boxes.some(closure)`, `for…of` and default parameters. JS detmath is not measurably slower than
`Math.sin`. A prediction replay (10–30 ticks per snapshot) costs ~20 µs.

**Maintenance cost:**
- ~1,300 lines of translator plus 200 lines of tests.
- The subset grows only when a twin needs it: the next ones are maps (≈ 60 lines), uint32 bit ops
  for the rng (`Math.imul`, `>>> 0`, ≈ 40 lines) and exported multi-results.
- Writers of shared code follow three rules: detmath instead of math, `float64(a*b)` in `+`/`-`,
  and no mutating value params. The translator enforces all three with a message naming the fix.

### b. Go → WASM (measured: standard Go 1.27; `internal/gojs/wasmbench`)

| | standard Go wasm | generated JS |
|---|---|---|
| size (sim only, no encoding/json) | 2,060 KB raw / **602 KB gzip** (4.5 MB if the package pulls in encoding/json) | 9.7 KB raw |
| startup (instantiate + Go runtime) | 24 ms | 0 |
| per tick, JS object in/out through `syscall/js` (today's call shape) | **52,400 ns** | ~1,500 ns (same script) |
| per tick, `//go:wasmexport` + state in a shared `Float64Array` | 1,500 ns | ~1,500 ns |

(This benchmark's input script collides more than physics.bench's, hence 1,500 instead of 700 ns.)

- **Determinism:** WASM has no FMA, and the Go wasm backend runs the pure-Go math, which is
  *not* the amd64 server's assembly `Exp`/`Hypot`. It needs the same detmath discipline to match
  the server.
- **Integration:** the body, inputs and map must live in Go memory and cross the boundary as
  numbers. predict.js, cases.js, main.js and the tests all change shape.
- **GC and debugging:** the Go runtime's GC runs inside the page; in the browser you step through
  wasm, not Go.
- **TinyGo:** not installed here, so not measured. Its docs cite much smaller binaries (a 93 KB →
  1.6 KB example), but they say GC "is also a lot slower than the usual Go garbage collector",
  recover doesn't work on WebAssembly, and it is a second toolchain to pin in CI.

Verdict: WASM pays off for large, CPU-heavy shared code (a full server sim in the browser for
offline play or bots). For ~400 lines of rules called per tick, it costs more than it saves.

### c. GopherJS and similar whole-program compilers

- GopherJS's README: "requires Go 1.21 or newer". Its latest release is 1.21.0, "for Go 1.21.13"
  (2026-06-26). This module is `go 1.27.4` and uses range-over-int (1.22), so GopherJS can't
  build it. (Installing it here to measure wasn't permitted in this session.)
- Its output carries a runtime and emulates Go semantics; per its README, `int` is 32-bit
  precision.
- It maps `math.Sin` to its own port. Whether that is bit-identical to amd64 Go is unknown, and
  FMA still applies on the server.

Verdict: it lags the Go release by 6 versions and produces non-readable output for a small rule
set. Rejected.

### d. The other way: JS/TS as the source, Go generated

- You would need a TS parser plus a type checker (tsc via node, or a Go port), and you would lose
  go/types' exact constants.
- JS has no integer type, so ints would need annotations. JS `number` has no notion of
  `float64(...)` rounding barriers, and the Go output would still need the FMA barriers (the Go
  compiler fuses whatever it is given).
- The server is the authority and holds most of the game (rounds, economy, bots, lag
  compensation). The shared rules would be the only TS the server depends on, with a node
  toolchain in the server build.

Verdict: the same work as (a), with a weaker source of truth. Not recommended.

## Recommendation

**Adopt (a), gojs, incrementally. Keep the existing guards.** The golden vectors, sim/cases,
`TestCasesAgree` and simdiff keep testing the server against the client end to end, so they catch
any translator bug.

### Why, against "just keep the twins"

The operator's alternative is reasonable, and for movement alone the trade is close: 130 lines of
JS versus a 1,300-line tool. The deciding facts:
1. The weapon rules are where the twins really cost effort: 16–17 commits each side so far,
   drift bugs already found by golden vectors, and every new weapon mechanic (burst, bolt, belt,
   pellets, drink) written twice.
2. Bit-exactness, which hand-kept twins never had, turns `TestCasesAgree` and the parity tests
   from "close enough" into equality. The prediction can then become an exact replay:
   reconciliation can compare and skip work, and any non-zero correction is a real bug, not noise.
3. The determinism work (detmath, FMA barriers, the arm64 check) is needed for *any* sharing,
   including WASM. Part of it, the FMA check, protects the arm64 release binaries even if we keep
   the twins.

**Keep the twins instead if** the weapon rules settle and rarely change, and nobody wants exact
prediction. In that case still take `TestNoFusedMultiplyAdd` and the `float64()` wraps (or
detmath) for the arm64 servers, and tighten the parity tests to the bit where they can be.
**Sharing becomes clearly worth it** when any of these holds: a third copy of the rules appears
(bots in JS, offline mode, a replay viewer), the weapon or economy rules keep growing, or the
netcode moves to bit-exact replays or deterministic lockstep.

### Migration order

1. **Movement (`internal/sim`)**: done on this branch.
2. **Recoil** (`RecoilIndex`, `RecoilOffset`, `recoilAngles`): pure functions, ≈ 50 lines. Needs
   4 `float64()` wraps and an exported multi-result for `recoilAngles` (or return `[2]float64`).
   Move them to a small `internal/rules` package. `arsenal.js` imports them.
3. **Seeded PRNG (`content/rng` ↔ `gen/rng.js`)**: needs uint32 ops in the subset (`Math.imul`,
   `>>> 0`). Small, and it removes a twin that every seeded asset depends on.
4. **Weapon rules** (`settle`, `fire`, `trigger`, `reload`, `switchTo`, `drawn`, `bestSlot`,
   `Ticks`): running the translator on a copy of today's weapons.go + recoil.go (321 lines)
   rejects only **maps (11 sites: index, literal, delete)** and **6 fusable products**. Nothing
   else is outside the subset. The real work is on the JS side: split `arsenal.js` into rules (the
   `Inventory` timers, generated) and presentation (HUD, sounds, raise/bob/kick animation, which
   stays hand-written and reads the rules' results). Server-only parts (`LoadWeapons`, items,
   drops, `aiPullDown`) stay in `internal/game`. Then retire the golden vectors' JS side, or keep
   them as translator regression tests.
5. Later candidates: grenade flight, the hitbox ray tests, economy prices. Only if they get a JS
   twin.

### CI guards (all in `go test ./...`, so they run in the existing `test` job)

| guard | catches |
|---|---|
| `TestGeneratedUpToDate` | Go changed and `go generate ./internal/gojs` wasn't run |
| translator errors (run by the test above) | code outside the subset: maps, closures, `math.Sin`, fusable `a*b+c`, mutated value params |
| `TestNoFusedMultiplyAdd` | any FMA in a shared package, on arm64 (cross-compiled on the amd64 runner) |
| `TestDetmathBitsJS` | Go and JS deterministic math differing in any bit (400,000 results) |
| `TestSampleAgrees`, `TestRejects` | translator semantics regressions |
| `TestParityWithJS` (now tolerance 0) | movement Go ≠ generated JS, bit-level |
| existing: golden vectors, sim/cases (go, js, e2e), `TestCasesAgree`, simdiff | the integrated behaviour, as before |

### Risks

- **Translator bugs** produce wrong JS that passes review. Mitigated by the sample semantics
  test, the twin-level parity tests, and the e2e cases. The translator is small enough to read.
- **The subset feels restrictive** (no maps or closures yet). Mitigated by growing it per twin,
  with rejections that name the fix.
- **`float64()` wraps** are noise in physics code (7 in physics.go). The translator says exactly
  where they go, and the alternative is an undocumented compiler flag.
- **detmath Exp is 1–2 ulp from amd64 `math.Exp`.** Only cosmetic fields use it today. If gameplay
  ever needs `math.Exp` bit-equal to old recordings, that changes behaviour once (simdiff shows it).
- **Large angles:** detmath reduces |x| ≥ 2^29 with an exact `mod 2π` instead of Payne-Hanek
  (the result differs from the standard library there, but is the same in Go and JS). Yaw never
  gets near that range.
- **Go language drift:** the translator rejects unknown syntax, so a new Go feature fails loudly
  instead of translating wrongly.

## Reproducing the numbers

| what | command |
|---|---|
| regenerate | `go generate ./internal/gojs` (= `go run ./cmd/gojs -all`) |
| translate one package to stdout | `go run ./cmd/gojs -pkg internal/sim -import github.com/gigurra/counterslop/internal/detmath=../gen/detmath.gen.js` |
| per-tick cost vs a previous physics.js | `git show <rev>:web/engine/physics.js > /tmp/old.mjs && node web/engine/physics.bench.mjs /tmp/old.mjs` |
| WASM | `GOOS=js GOARCH=wasm go build -o /tmp/sim.wasm ./internal/gojs/wasmbench && node internal/gojs/wasmbench/bench.mjs /tmp/sim.wasm` |
| FMA per arch | `GOARCH=arm64 go build -gcflags=-S ./internal/sim 2>&1 \| grep -c FMADDD` (main: 7, branch: 0) |
| Go vs V8 math | `go run ./internal/detmath/mathcmp \| node internal/detmath/mathcmp/cmp.mjs` |
| arm64 drift emulation | a throwaway copy of internal/sim with `math.FMA` at the 7 sites, run over `randomRuns` (method above; not committed) |

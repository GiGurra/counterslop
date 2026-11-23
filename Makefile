.PHONY: balance snapshots test test-fast cases sim-smoke sim-throttled simdiff coverage

# The CI gate's scenarios (lockstep, see TESTING.md); nightly runs every scenario.
SMOKE = prediction hitboxes weapons scope pickup-swap teams team-cap shoot-the-bot admin jump-hold \
	jump-crouched sim-rate console-open spawn-overview aim audio-resume weapon-keys \
	weapon-keys-migrated reload-indicator resync-faults maps-vote maps-menu spawn-marks buy-menu ai-match ai-melee lag-compensation damage-direction weapons-roster grenades radar gear kill-feed chat-radio name-tags buy-edges buy-badge rounds match gpu-drop options-controls spray-pattern music corpses anti-aliasing buy-keys-keep-weapon escalator map-loading

# Timing-sensitive scenarios, run again with every page's CPU 6x slower (CI job sim-throttled):
# lockstep runs on game time only, so a slow machine must not change a result.
SMOKE_THROTTLED = buy-menu deathcam escalator gpu-drop kill-feed maps-vote match name-tags \
	options-controls resync-faults rounds shoot-the-bot weapon-keys-migrated weapons

# Unit tests, sim/cases at the go + js levels, golden vectors, parity (seconds).
test-fast:
	go vet ./... && go test ./...

# Go per-package + JS coverage against the floors in coverage-floors.txt.
coverage:
	./scripts/coverage.sh

# Everything CI runs: test-fast, lockstep e2e cases + action registry, sim smoke.
test: test-fast coverage cases sim-smoke

# sim/cases end to end in lockstep (headless Chrome) + the action registry check.
cases:
	go run ./cmd/sim -cases

# The CI subset of sim scenarios (headless Chrome). SHARD=1/4 runs a quarter of them (CI runs four shards).
sim-smoke:
	go run ./cmd/sim $(if $(SHARD),-shard $(SHARD)) $(SMOKE)

sim-throttled:
	go run ./cmd/sim -cpu-throttle 6 $(if $(SHARD),-shard $(SHARD)) $(SMOKE_THROTTLED)

# Per-case behaviour diff of the working tree vs HEAD~1 (BASE=main to compare with main).
simdiff:
	go run ./cmd/simdiff $(or $(BASE),HEAD~1)

# Hits and time to kill per weapon / armor / part / distance (from weapons.json).
balance:
	go run ./cmd/balance

# Run every sim scenario and write out/snapshots/index.html.
snapshots:
	go run ./cmd/sim

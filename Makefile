.PHONY: balance snapshots test test-fast cases sim-smoke simdiff coverage

# The CI gate's scenarios: only ones that pass every time. Real-time bot shooting
# (shoot-the-bot, hitboxes) flaked under CI load; their hit rules are pinned
# deterministically by the lockstep cases headshot-kill / body-shots and the Go
# hitbox tests, and they still run nightly (every scenario).
SMOKE = prediction weapons scope pickup-swap teams admin jump-hold \
	jump-crouched sim-rate console-open spawn-overview aim audio-resume weapon-keys \
	weapon-keys-migrated reload-indicator resync-faults maps-vote buy-menu ai-match lag-compensation damage-direction weapons-roster grenades radar gear kill-feed chat-radio

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

# The CI subset of sim scenarios (headless Chrome). SHARD=1/3 runs a third of them (CI runs three shards).
sim-smoke:
	go run ./cmd/sim $(if $(SHARD),-shard $(SHARD)) $(SMOKE)

# Per-case behaviour diff of the working tree vs HEAD~1 (BASE=main to compare with main).
simdiff:
	go run ./cmd/simdiff $(or $(BASE),HEAD~1)

# Hits and time to kill per weapon / armor / part / distance (from weapons.json).
balance:
	go run ./cmd/balance

# Run every sim scenario and write out/snapshots/index.html.
snapshots:
	go run ./cmd/sim

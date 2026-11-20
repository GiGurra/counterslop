.PHONY: snapshots test

# Run every sim scenario (headless Chrome) and write out/snapshots/index.html.
snapshots:
	go run ./cmd/sim

test:
	go vet ./... && go test ./...

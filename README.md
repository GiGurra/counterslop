# counterslop

A browser parody of a certain tactical shooter. Everything you see is shaders and
flat colours on boxes (no textures), everything you hear is synthesized with WebAudio
(no sound files), and the guns are toys.

One Go binary serves the client (embedded `web/`, no JS build: Preact + htm via an
import map) and a websocket game server at `/ws`.

## Install

```sh
brew install gigurra/tap/counterslop   # macOS / Linux (Homebrew)
counterslop                            # then open http://localhost:8080
```

Prebuilt binaries (linux/darwin, amd64/arm64) are on the
[releases page](https://github.com/GiGurra/counterslop/releases). With Go:
`go install github.com/gigurra/counterslop@latest`.

## Run from source

```sh
go run . --addr 127.0.0.1:8080             # embedded client
go run . --addr 127.0.0.1:8080 --dev web   # serve web/ from disk for live edits
go run . --help                            # every flag and its env var (two dashes: --addr, --bots, ...)
go build -ldflags "-X main.version=$(git rev-parse --short HEAD)" -o counterslop .
```

Endpoints: `/` client, `/ws` game socket, `/map.json`, `/healthz`, `/version`.

## Controls

WASD move · Shift sneak (silent) · Space jump · Mouse aim/shoot · R reload · Tab scores · Esc pause

## Layout

- `internal/game` – the authoritative game server: 100 Hz sim ticks; clients send their input
  commands at 100 Hz (each packet repeats the unacknowledged ones) and get a snapshot every
  3rd tick (~33 Hz). Rounds, economy, bots, grenades, GPU Drop.
- `internal/sim` – movement and collision, written once in Go; the client runs the same code
  translated to JS (`cmd/gojs`) to predict its own player.
- `internal/content` – maps built from prefabs, sent to the client as data.
- `web/engine` – WebGL2 renderer, input, audio, the socket and remote-player interpolation.
- `web/game` – client glue: prediction and reconciliation, weapons, HUD wiring.
- `web/ui` – Preact HUD, menus and console. `web/gen`, `web/assets` – procedural shaders,
  models, synthesized sounds and voice.

Contributor notes: `CLAUDE.md` (architecture, rules), `TESTING.md`, `docs/`.

## Deploy

Run the binary (a release, or `go build`) behind any HTTPS reverse proxy that passes
websockets through, and keep the `Host` header: the game socket only accepts its own origin.

```sh
COUNTERSLOP_ADMIN_PASSWORD=... counterslop --addr 127.0.0.1:8080
```

- No admin password (neither `--admin-password-file` nor `$COUNTERSLOP_ADMIN_PASSWORD`) means
  every player is an admin.
- `curl /version` shows what a server runs; `/healthz` is the health check.
- Deploy only commits whose `ci` check is green; every green push to `main` is also released
  (tag, binaries, Homebrew).
- Never pass `--test` to a public server: it turns off the speed checks.

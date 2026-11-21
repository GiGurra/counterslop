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

- `internal/game` – websocket server (Nagle off, 30 Hz snapshots, movement sanity checks), map data.
- `web/js/gl.js` – WebGL2 renderer: procedural sand/brick/crate/concrete shading, sky, players, viewmodel.
- `web/js/physics.js` – AABB movement with step-up, gravity, jumping, raycasts.
- `web/js/net.js` – socket with auto-reconnect, ping, 100 ms remote interpolation.
- `web/js/hud.js` – Preact HUD and menu. `web/js/audio.js` – synthesized sounds.

## Deploy

`main` auto-deploys to the test env. Prod follows the `prod` tag, moved only with the operator's OK.

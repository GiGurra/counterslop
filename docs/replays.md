# Replays: GOTV-style round recordings

Operator 2026-11-25: every round is recorded on the server from snapshots, the files can
be downloaded per round while the match runs, and the player seeks both ways. Parts:
1. **the recorder and the file format** (this file; `internal/game/tv.go`),
2. **`/replays` listing and per-round download, `tv_access`** (`internal/game/tv_http.go`),
3. the client player (Replays in the main menu; `web/engine/replay.js` reads the files),
4. client-side recording in the same format.

## Recording

- On by default: `--replay-dir` (env `COUNTERSLOP_REPLAY_DIR`, default `replays`,
  relative to the server's working directory; empty = off). `--replay-max-mb` (default
  2048) caps what the replays take: after each round file closes, the oldest round files
  go first, then match directories left empty. Other files in the directory are left
  alone and not counted.
- `tv_autorecord` (cvar, default 1; a map may not pin it): 0 stops the round being
  recorded at once and starts no new file.
- One file per round: `<replay-dir>/<match>/round-NN.cstv.gz`. A match id is the match's
  start (UTC, the server's game clock) and map, `20261125-213000-de_slop` (a suffix if it
  exists). A new match starts at round 1 or on another map.
- A round's file opens at the round start (the freeze, so the buy phase is in it) and
  closes at the next round's start, a map change, warmup (a restart) or `tv_autorecord 0`.
  The post-round time (MVP, round-end banner) belongs to the round before. Warmup and
  deathmatch are not recorded (no rounds).
- Cost: on the tick, the recorder builds the observer snapshot every 3rd tick (the delta
  codec's shared world, `deltaWorldOf`) and encodes it. Gzip and disk run on a goroutine
  behind a 4096-op queue; if the disk can't keep up, the round's file is cut where the
  queue overflowed (a warning in the log), never the tick. cmd/sim records every scenario
  into a temp directory, so the recorder runs under every lockstep scenario.
- Size: 5v5 AI on de_standup records 2.3 KB/s (`TestReplayAIMatchDecodesAndStaysSmall`,
  budget 8 KB/s): ~300 KB a round, ~6 MB a 24-round match.

## Listing and downloading (part 2; `internal/game/tv_http.go`)

- `GET /replays.json`: `{"matches": [...]}`, newest match first. A match: `id` (its
  directory), `map`, `started` (unix ms), `version`, `live` (a round of it is being
  recorded) and `rounds` in order: `round`, `file`, `url`, `size` (bytes now),
  `modified`, `live`, `score` (before the round), `players`, `started`. The map, score and
  players come from each file's header, read once per file and cached (a header never
  changes); files that aren't replays (no header, not ours) are left out, except the live
  round whose header may not be flushed yet.
- `GET /replays/{match}/{file}`: the round file as stored (`application/gzip`, no
  `Content-Encoding`, `Content-Disposition: attachment; filename="<match>-round-NN.cstv.gz"`).
  A live round is served as far as it is on disk (`X-Replay-Live: 1`, `no-store`): it
  reads as a replay up to its last keyframe, and downloading it again later gets more.
  Closed rounds are cacheable (`private`, an hour) and support ranges. Names are checked
  against the match id and `round-NN.cstv.gz` patterns: nothing else in the directory is
  served.
- `GET /replays`: the same list as a plain page with download links (for a browser
  without the game).
- `tv_access` (cvar, `public|admin`, default `public`; Server settings → Replays):
  `admin` asks for the admin password as HTTP Basic auth (any user name; failures count
  towards the IP's `admin_login` backoff) or `?token=` with the token admins get in their
  `cvars` message (`tvToken`, random per server start: the client's Replays screen uses
  it). With no admin password set everyone is admin, so it changes nothing there. Without
  `--replay-dir` the endpoints answer 404.

## File format (format 1)

A gzip stream, flushed (sync flush) at every keyframe, so a round still being played
reads up to its last second; a reader keeps what decompressed before the cut.

    "CSTV" | uvarint n | header JSON (n bytes)
    records, in the order they happened:
      'S' | uvarint n | snapshot: a delta-codec message (docs/replication.md)
      'E' | uvarint tick | uvarint n | event: a message's JSON

**Header** (`tvHeader`): `format`, `version` (the server binary), `match`, `round`, the
whole `map` (a replay plays without the server or the map's current version),
`schemaHash` and the `schema` itself (the delta codec's `NET_SCHEMA` layout, so an old
replay decodes after the snapshot schema changes), `simHz`, `every` (3: ticks between
snapshots), `keyEvery` (100: ticks between keyframes), `startTick`, `started` (unix ms),
`players` (id, name, team, bot) and `score` as the round started.

**Snapshots** are the observer's view: `snapMsg` with no `spot` and no `me` (`players`,
`items`, `nades`, `round`, `time`, `tick`), what a spectator of the whole game sees. Every
3 ticks (~33 Hz), each a delta from the snapshot before (`back` = 3), and a keyframe (a
full one, `back` = 0) once a second. Checksums as on the wire. Seeking to tick T: decode
from the keyframe at or before T to the last snapshot at or before T; forward from the
current one when that is on the way (`createReplayCursor` in `web/engine/replay.js`).

**Events** are the optional messages players got: `round` (start, live, planted, defused,
exploded, end, ...), `kill`, `shot` (with its wall impacts), `nade` (throw, bounce, pop,
smoke, fire, out), `gpu`, and `hit` with the shooter added as `"by"` (live, only the
shooter got it). Everything that matters is in the snapshots, as on the wire; events are
the kill feed, sounds and effects.

## Tests

`internal/game/tv_test.go`: rounds to files (header, every snapshot decodes with
checksums to where the server had everyone, every 3 ticks, keyframes a second apart, the
events), the live round's file reads up to its last keyframe, a map change closes it,
`tv_autorecord 0`, no directory = off, retention, queue overflow, unique match ids, a
5v5 AI match (decodes whole, size budget), and `TestReplayGolden`
(`testdata/replay_golden.json`, `UPDATE=1` rewrites it) which `web/engine/replay.test.mjs`
reads with the client's reader. Case `tv-autorecord` (the rcon cvar, e2e).
`internal/game/tv_http_test.go`: the list (rounds in order, header fields, the live
round), downloads byte for byte (the live one decodes), bad names 404, strays and deleted
rounds left out, `tv_access admin` (password, token, lockout, the token only for admins),
no recorder. Case `tv-access` (the rcon cvar, e2e).

# Replication: a declared schema instead of hand-built snapshots

Status: **prep, on main.** Every field of every type in a state message carries a `net` tag
and tests enforce it. The snapshot is still built by hand; nothing on the wire changed.
Date: 2026-11-24.

## Goal

Today a new piece of gameplay state reaches the client only if someone remembers to copy it
into `meState` (`meStateOf`), the player list, or a hand-assembled fragment (`snapBytes`,
`posesJSON`, `nadesJSON`, `spottedJSON`). The plan is one **declared replication schema**: the
engine walks each client's replicated state, diffs it against what that client last
acknowledged, and sends what changed. Gameplay code then sets fields and never contains "send
X" logic. If a field has no tag, the build's tests fail, so nobody can forget to decide.

## The vocabulary (`internal/game/netschema.go`)

A tag is a class word followed by optional modifiers, at most one from each group.

| word | group | meaning |
|---|---|---|
| `-` | class | never sent: server-only state. An exported one needs a comment saying why. |
| `hot` | class | changes most ticks (pose, the self body, timers, a flying grenade): sent in every snapshot |
| `cold` | class | changes rarely (name, team, score, money, HP, weapon in hand): may go only on change |
| `self` | scope | only in the recipient's own state (`me`, `inv`, welcome's id/team) |
| `team` | scope | only to the recipient's team (`spot`: the enemies your team sees) |
| `exact` | precision | full float64 (the client replays prediction from it) |
| `mm` | precision | millimetres (`Vec3`) |
| `angle` | precision | 4 decimals (`Angle`; Player's yaw/pitch via `Player.MarshalJSON`) |
| `gait` | precision | `Gait`'s quantised ints (`gaitQ`) |

The class says what the field *is*, meaning how often it changes. It does not describe
today's transport; the differences are listed below. Server-only fields use `net:"-"`, the same
convention as `json:"-"`. We dropped a planned `event` word: no state field is cosmetic. Events
(kill, hit, shot, nade throw, round events) are separate optional messages, not fields.

### What the tests enforce (`netschema_test.go`)

- **TestNetTagsValid**: every field of every registered type (`netTypes`) has a tag that
  parses, with no unknown words, a single class, and no two words from the same group.
  - A sent field must be written by encoding/json, so it must be exported (or embedded)
    without `json:"-"`. A `-` field must not be written by json.
  - A float-carrying field (float64, `Vec3`, `[3]float64`, `Gait`, or a container of them)
    needs exactly one precision, and it must be the one its type allows. Other fields may not
    have a precision.
- **TestNetRegistryCoversTheWire** walks the sent fields from the root messages (`snapMsg`,
  `welcomeMsg`, `invMsg`) through pointers, slices and maps. It fails if it reaches a struct
  type that isn't in `netTypes`, and if a registered type is unreachable. Scopes must nest:
  everything under a `self` field is `self` too, and a type that also goes to everyone can't
  have `self` fields.
- **TestNetNeverHasReason**: every exported `net:"-"` field has a comment.
- **TestNetPrecisionMatchesEncoding** puts a probe value in every float field and checks
  that json writes it back rounded exactly as the field's tag says.
- **TestNetCheckerCatches / TestNetWalkCatches** run the checker on fixture types to show
  that each rule fires: an untagged field, unknown words, conflicts, json disagreements, an
  unregistered nested type, and broken scopes.
- **TestSnapMsgMatchesSnapBytes**: `snapMsg`, the declared full snapshot, marshals to the
  same bytes as the hand-built `snapBytes` for a populated world (players on both teams and a
  spectator, an item, a grenade, spotted lists, round state, `me`).
- **TestPoseRowsAreHotFields**: a pose-snapshot row (`posesJSON`) carries Player's hot fields
  (minus `poseGaps`), encoded the same way as in the full snapshot.

Two kinds of type are leaves (`netLeaves`), which the walk does not enter. `Vec3`, `Angle`
and `Gait` have their own encodings. `content.Map` is static data, sent whole in welcome and
on a map change.

## Codecs: how snapshots go on the wire

A server picks one snapshot codec at startup: `--net-codec` (env `COUNTERSLOP_NET_CODEC`),
default `json`. The welcome names it (`"codec"`; absent means `json`, so the json welcome is
the same as before codecs existed) and the client decodes with the matching decoder. Every
codec carries the same state and the client turns each snapshot into the same object shape,
so nothing above `web/engine/net.js` knows which codec is running.

| codec | what goes out |
|---|---|
| `json` | the hand-built JSON snapshots described below (full every 3rd tick, pose rows between) |
| `delta` | binary, laid out by the `net` tags, only what changed since the last snapshot sent on that connection |

The seam is `snapCodec` in `internal/game/netcodec.go`. Adding a codec: implement it, list it
in `netCodecs`, add its decoder to `web/engine/net.js` (keyed by the welcome's `codec`), and
run every case with it: `TestCasesDelta` is the pattern for the Go cases (decode every tick
and compare with the json codec's snapshot), and the CI `cases` job has a codec matrix
(`go run ./cmd/sim -cases -net-codec <name>`).

### The delta codec

One schema, two ends. `internal/game/netdelta.go` compiles `snapMsg` and every type under it
from the `net` tags into a layout (`netSchema`): per struct its sent fields (hot ones first),
JSON names, `omitempty`, class and precision. The encoder walks Go values with it.
`cmd/gojs -all` (`go generate ./internal/gojs`) writes the same layout to
`web/engine/netschema.gen.js`, and the client's decoder (`web/engine/netdelta.js`) is generic
over it. A tag change without regenerating fails `TestNetSchemaJSUpToDate` and
`go run ./cmd/gojs -all -check`. The welcome carries the schema's hash (`"schema"`); a client
built with another one refuses the connection.

**Message** (a binary websocket frame; JSON messages are text frames):

    0xD5 | uvarint tick | uvarint ticks back to the base (0: full) | u32 LE checksum | root struct

A full snapshot is a delta from the zero state, so there is one code path. Values:

| kind | from | encoding when changed |
|---|---|---|
| int | any integer | zigzag varint: hot = delta from the base, cold = the value |
| angle | float64 `angle` | as int, in 1e-4 rad (what json's 4 decimals give) |
| vec3 | `Vec3` `mm` | three ints in mm (`sim.RoundMM`) |
| exact | float64 `exact` | hot: varint of the bits XOR the base's; cold: 8 bytes |
| exact3 | `[3]float64` `exact` | a 3-bit changed mask, then each changed one as exact |
| gait | `Gait` | a byte (length << 4 \| changed mask), then each changed part as a zigzag delta |
| bool, str | | a byte; uvarint length + UTF-8 |
| struct | | uvarint mask of changed fields (hot first, so usually one byte), then each changed field |
| ptr, list, map | | a byte: 0 nil, 1 same ids/keys as the base, 3 new ids/keys follow |

Lists of structs with an int `id` (players, items, grenades) are matched by id, so a join or
leave sends the id list and the newcomer, not everyone again. Maps are matched by key.
Players go sorted by id and `spot` sorted (both are sets to the client). Decoded, a snapshot is
exactly the object `JSON.parse` gives for the json codec's full snapshot: `omitempty` and
`null` are reproduced, so nothing above `net.js` changes; every delta snapshot is full-shaped.

**Base and resync.** The base is the last snapshot actually queued to that connection (a
websocket is reliable and ordered; a queue overflow that drops one leaves the base where it
was). The checksum is FNV-1a over the canonical bytes of the whole decoded state. A client
that gets a delta whose base it lacks (net_fakeloss drops at the application level) or whose
checksum doesn't match drops it and sends `{"t":"full"}`; the server then sends it a full one.
A checksum mismatch is a codec bug: `__cs.netCodec()` counts them and cmd/sim fails the case.

**Fulls are spread: no egress bursts.** At most `fullsPerTick(n)` fulls go out per tick (one
up to 100 connections): first connections without a base (joined, or asked), oldest request
first, the others waiting a tick or two; then the periodic refresh, about once a second each,
to the connections whose last full is oldest. Any full resets that age, so joins, leaves and
reconnect waves rebalance by themselves. Deltas are never held back: every connection with a
base gets its delta every tick (a late delta is state its player waits for: a shot, a pickup).
`TestDeltaFullsSpreadAcrossRoundRestart` and `TestDeltaFullsSpreadAcrossJoinWaves` bound what
a tick's fulls add at 0.6x the mean per-tick egress (12v12 restart: 0.24x; 30 s of joins,
leaves and reconnect waves of 10: 0.32x), and with `TestDeltaOneHumanAndBotsEveryTick` check
that every connection with a base gets a delta every tick, so the drawn delay on a clean link
(net.js `delayTicks`) is exactly 30 ms.

The trade-off: what changes everyone's state at once goes out at once. A round restart peaks
at 1.8x the mean per-tick egress and a reconnect wave of 10 at 2.2x; that burst is the state
change itself, not fulls, and is accepted. (Pacing the deltas, as an earlier version did,
held it to 1.5x by deferring some connections' snapshots up to 3 ticks: no player should wait
for their own state to save a few KB for one tick.)

**Numbers** (12v12, everyone moving, `TestSnapshotBandwidth*`): json 239 KB/s raw, 76 KB/s
deflated per client; delta 24.8 KB/s raw (254 B per snapshot), 22 KB/s deflated. Per-message
deflate gains ~10% on delta. The drawn delay is unchanged (100 snapshots/s either way).
Commands are unchanged so far.

## Today's transport and where it differs from the tags

- Snapshots go out every tick (100 Hz). Every `fullEvery` (3) ticks a **full** snapshot
  carries `players` (all exported Player fields), `items`, `nades`, `spot`, `round` and `me`.
  The ticks between carry a **pose** snapshot with `[id,x,y,z,yaw,pitch,flags]` rows and `me`.
  So today "hot" means 100 Hz for Player's pose and 33 Hz for everything else, and "cold"
  means 33 Hz, unconditionally.
- `Player.Gait` is hot (it changes every tick while walking) but rides full snapshots only;
  clients interpolate it (`poseGaps`).
- `Player.Dead` is cold. Pose rows also carry flag 2 ("not a body": dead, spectating or not
  ready), which is derived, so it isn't its own field.
- `me` is sent whole every snapshot, cold fields included. The standing rule (full
  self-state every snapshot) makes this the correctness baseline.
- `Player.FallYaw` is an angle sent `exact`. It could be `angle` (saving bytes) once the
  client's corpse code is checked with it.
- `Player.Weapon/Ammo/Reserve` are copied from the inventory at send time in
  `sendSnapshot`. That is a derived projection, not stored state.
- `Nade.Left`, `roundMsg.Left/Buy` and bomb/GPU `Progress` count down every tick (hot). The
  client could compute them from a cold deadline in ticks.
- `me.Weapons` timers (`ns`, `ce`, `re`, `bo`, `ls`) are relative to the acked command, so
  they change every command even when nothing happens. Absolute command ticks would be cold.

## The design (what the engine will do)

1. **One declared state per recipient.** The root message structs (`snapMsg` and friends)
   are the schema. The engine builds the client's view from them by reflection or generated
   code, filters it by scope (`self`, `team`, plus visibility later) and encodes each field at
   its precision.
2. **Per-client diff against the last acknowledged baseline** (the Quake 3 scheme). Each
   snapshot names its baseline tick. The client acks snapshot ticks in its command packets,
   and the server diffs against the newest snapshot the client has acked, never one it only
   sent. Loss, duplication and reordering then stay harmless, as the standing rule requires: a
   lost diff is superseded by the next diff from the same acked baseline. Hot fields are
   always sent. Cold fields are sent when they differ from the baseline, and a full refresh
   goes out every N ticks and whenever no baseline is acked.
3. **Encodings this allows**, in order of effort:
   - JSON Merge Patch (RFC 7386) against the baseline: the same JSON client, with `null` for
     removal. Arrays of entities need ids (players, items and nades already have them), so
     the patch is keyed by id, not by index.
   - Cold-on-change inside the current format: drop unchanged cold keys from full snapshots.
     The client keeps the last value per entity.
   - Protobuf or a bit-packed binary encoding, generated from the same tags. The precision
     words map directly to fixed-point varints (mm → int32 of mm, angle → int16 of 1e-4 rad).

## Gaps: moving the hand-written pieces onto the schema

- **`meStateOf` / `meBytes`** copies about 20 fields from `Player`, `sim.Body`, the
  inventory (`weaponStateOf`) and derived values (`canBuyNow`, `flashLeft`, `len(cmds)`). To
  move it, those become fields the engine reads directly (tagged on `Player`/`Inventory`, or
  `self` getters registered in the schema). The derived ones stay as small registered
  projection functions. `TestSnapMsgMatchesSnapBytes` is the byte-for-byte guard while this
  moves.
- **`posesJSON`** is a hand-packed projection of Player's hot fields. Under the schema it
  becomes the encoder's output for "hot fields of every visible player", and
  `TestPoseRowsAreHotFields` already pins the two together. The positional array format can
  stay as the hot encoding (it is the main bandwidth win), generated from the field order.
- **`snapBytes`, `nadesJSON`, `spottedJSON`** become one encoder over `snapMsg`.
- **Client (`web/game/net.js`, `predict.js`)**: it must apply diffs onto its last state
  per baseline, and it acks snapshot ticks. That is the part that needs sim/cases with
  `net_fakeloss`/`dup`/`reorder` to prove convergence.
- **Events stay events.** `hit`, `kill`, `hurt`, `shot`, `nade` and round events remain
  optional messages. Anything they carry that matters must also be in state (it already is:
  HP, dead, kills, round state).

## Next steps

1. Snapshot ack: the client sends the newest snapshot tick in each command packet, and the
   server stores per-client baselines. The wire doesn't change yet.
2. A reflective encoder that builds `snapMsg` per recipient and replaces `snapBytes`,
   checked byte-identical by `TestSnapMsgMatchesSnapBytes` and simdiff.
3. Cold-on-change against the acked baseline for `players` and `round`, with a full refresh
   every second, plus sim `resync-faults` cases.
4. Move `me`'s cold fields onto the diff once the client applies diffs to its self-state.

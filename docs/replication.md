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
default `delta` since 2026-11-27 (4-8x less egress than json; `--net-codec json` stays as the
fallback, per server, no client change: the client follows the welcome). The welcome names it (`"codec"`; absent means `json`, so the json welcome is
the same as before codecs existed) and the client decodes with the matching decoder. Every
codec carries the same state and the client turns each snapshot into the same object shape,
so nothing above `web/engine/net.js` knows which codec is running.

| codec | what goes out |
|---|---|
| `json` | the hand-built JSON snapshots described below (full every 3rd tick, pose rows between) |
| `delta` | binary, laid out by the `net` tags, only what changed since the newest snapshot the client acked |

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

**Base and resync: acked bases (the Quake 3 scheme).** Each delta is based on the newest
snapshot the client has *acked*, never on one only sent, so the transport may lose, duplicate
and reorder snapshots (the standing rule). The client acks the newest tick it decoded: every
command packet carries it (`"sa"`), and with no commands going out (dead, spectating, the menus)
a `{"t":"sack","sa":tick}` goes on its own once it is 4 snapshots past its last ack (the first
one at once). Acks are datagrams too (net_fakeloss drops them); the server takes the newest one
that is still a base. The server keeps what it sent per connection (`deltaConn.sent`) for a
window: 128 ticks (1.28 s: a round trip plus a long main-thread hitch), or twice the age that
connection's acks have when they arrive if that is more (a slow link, a client that falls
behind; a decaying max), up to 512, and all 512 until its first ack (its link is unknown). The
client keeps what it decoded for 512 ticks (`BASE_WINDOW` in netdelta.js, pinned to Go's
`maxBaseWindow` by a test), so a base it acked is always still there when a delta from it
arrives. A lost snapshot costs that frame; a duplicate or one overtaken by a newer one is
dropped as stale (one frame). Until anything is acked (just joined, or after asking for a full)
a delta is based on the newest full sent; nothing acked in the window (a client stalled longer
than it, or all its acks lost) means the same, or a full if none is left. On a websocket that
full always arrives; only fake loss can drop it. The kept snapshots share every unchanged part
with the tick before (`shareNV`, as the JS decoder shares with its base), so a window costs what
changed, not a tree per tick: about 23 MB for 128 ticks at 24 humans all moving, half of what
plain copies took.

A delta whose base the client lacks (based on a full it hasn't had: lost, or still behind it in
flight) is answered with an ack of what it has, at once (`sack`, at most every 20 ms), so the
next ones come from a base it has. It asks for a full (`{"t":"full"}`) only if it isn't in step within `reaskMs`
(2 round trips, at least 100 ms; again after each), or at once on a checksum mismatch; the
server then forgets every base it had for it. Asking at once on a missing base made a loop under
reordering: each full asked for wiped the bases and was overtaken by deltas based on it (36
fulls in 25 s live). The checksum is FNV-1a over the canonical bytes of the whole decoded
state. A checksum mismatch is a codec bug: `__cs.netCodec()` counts them and cmd/sim fails the
case.

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

**Testing it against the json wire.** The delta tests compare what a client decodes with the
json codec's *actual* full snapshot bytes (`jsonFullSnap`), not the declared `snapMsg`, so a
projection only one path makes fails a test (a car parked in the pit stop was dropped from json's
`cars` but sent by delta; `carsSent` is now the one list for both, replays and MCP). Beyond the
cases (`TestCasesDelta`), the wire watch (`internal/game/wirewatch_test.go`) runs beside any test:
every tick it encodes each human's snapshot (and one bot's per team) as the delta codec does,
decodes it with the checksum, and compares it with the json snapshot (every 7th tick and whenever
an entity list changes length). `econServer` (race, pit stop, garage, pickups, tickets, parkour,
GPU Drop), `mvpServer` and `harborServer` tests always run with it;
`COUNTERSLOP_TEST_NET_CODEC=delta go test ./internal/game` watches every server of every test,
every tick, and lists each one's first difference. cmd/sim fails a scenario (not only a case)
whose pages saw a checksum mismatch or an undecodable snapshot on a clean link, and CI runs the
smoke scenarios once per codec.

**Loss.** Before acked bases (until 2026-11-28) a delta was from the last snapshot *sent*: a
client that lost one dropped every delta after it until the full it asked for arrived. On
`net-loss` (lockstep; 120 ms lag, 40 ms jitter, 3% loss) that was 422 decode errors, 25 fulls
asked and 155 interpolation under-runs in 10 s; now 0, 0 and 0 (it is in the CI smoke set). `resync-faults` (20% loss, 10%
duplicates, 80 ms reordering): 590 errors and 196 fulls, now none. Go:
`TestDeltaSurvivesLossDupReorder` (10% loss and duplicates both ways, reordering, a 450 ms
stall: about 1250 errors and 71 fulls per client before, 0 now), `TestDeltaLostFirstFullAndLongStall`;
JS: netdelta.test.mjs decodes `testdata/netdelta_loss_golden.json`, a lossy link's arrivals
recorded from the Go encoder (deltas from older acked bases, out of order).

**The cost on a clean link** is that a base is a round trip old instead of one tick: hot values
move further, and cold ones changed in the meantime go again. `NETBW_ACKLAG=n make netbw`
measures it with the base n ticks back (n=1 is the old last-sent base):

| match, delta B/s per client | lag 1 (before) | 3 (30 ms) | 6 (60 ms) | 12 (120 ms) |
|---|---|---|---|---|
| race (8 cars) | 25.4K | 27.7K | 29.8K | 32.5K |
| moonshot 8v8 | 16.0K | 18.0K | 19.7K | 20.4K |
| slop5v5 | 7.5K | 8.6K | 9.3K | 10.0K |

About +10% at 30 ms, +20% at 60 ms: still 3-6x under json.

**Bandwidth harness:** `make netbw` (`TestNetBandwidthReport`) plays scripted bot matches (race:
rc_merge_train with 8 cars; moonshot: cq_moonshot 8v8; slop5v5) and prints, per client, json and
delta bytes per snapshot (raw and per-message deflated) and the delta bytes by field
(`players[].p`, `cars[].v`, ...). `NETBW=race,moonshot NETBW_SECS=60` picks matches and length.

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

1. ~~Snapshot ack~~: done for the delta codec (acked bases, above).
2. A reflective encoder that builds `snapMsg` per recipient and replaces `snapBytes`,
   checked byte-identical by `TestSnapMsgMatchesSnapBytes` and simdiff.
3. Cold-on-change against the acked baseline for `players` and `round`, with a full refresh
   every second, plus sim `resync-faults` cases.
4. Move `me`'s cold fields onto the diff once the client applies diffs to its self-state.

// Client-side prediction + reconciliation for the server-authoritative sim.
// Every sim tick the client runs its command locally (instant feel) and sends
// it; the server runs the same command with the same physics (internal/sim,
// parity-tested) and reports back {ack, p, v, g, c}: its body after the last
// command it applied. We rewind to that state and replay the commands it
// hasn't seen yet. With matching physics the result equals what we predicted,
// so corrections are rare and tiny; any error left is smoothed out visually.
import { moveInput } from "../engine/cmd.js";
import { CmdTime, SeatDeck } from "./shared.gen.js";
import { moveCmd } from "./localsim.js";
import { H, H_CROUCH, EYE, EYE_CROUCH } from "../engine/physics.js";

const MAX_PENDING = 1024; // 10 s of commands at 100 Hz (beyond that the link is dead anyway)

// Re-sync safety net: what is drawn (the predicted body + the smoothing offset)
// must never stay apart from where the server has us. A correction, or a drawn
// position at the acked command, off by SNAP_M or more is snapped, not
// smoothed; a smoothing offset above STUCK_M for longer than STUCK_MS (it
// should decay well within that: 0.5 m to 5 cm in ~150 ms) is dropped. Either
// counts in stats.resyncs and calls onResync (the view snaps).
export const SNAP_M = 0.5, STUCK_M = 0.05, STUCK_MS = 300;
// How fast the smoothing offset decays (1/s): e^(-15 t).
const DECAY = 15;

// The other players' bodies our body collides with (internal/sim/peers.go),
// from a snapshot's player list: as the server's peersOf, alive players on a
// team, in ID order, at their snapshot positions (the sim rounds them to the
// millimetre like the snapshot, so a standing peer predicts bit-exactly).
// solid: mp_solid_players. Null when there are none.
export function peersFrom(players, myId, solid = true) {
  if (!solid || !players) return null;
  const out = players
    .filter((p) => p.id !== myId && !p.dead && !p.nb && p.team !== "spec" && p.p) // = simtick.go solid() (nb: a pose snapshot's "not a body")
    .sort((a, b) => a.id - b.id)
    .map((p) => ({ pos: p.p, h: p.c ? H_CROUCH : H, lo: p.id < myId, id: p.id, g: p.gv || 0 }));
  return out.length ? out : null;
}
// The cores for a command `ahead` commands past the one the newest snapshot
// acked: each peer of the newest snapshot moved on at its velocity (m per
// tick, vel(id), from its last two snapshots) to where the server will have
// it when it runs that command (a peer with a lower ID moves first in a
// tick: one tick more). The server takes the core where the peer really is;
// a core predicted short of that would stop us late and the correction
// would pull us back (rubber banding), so a peer that stopped meanwhile errs
// the safe way: we stop early, and the correction only moves us on.
export function coresAhead(newest, vel, ahead) {
  if (!newest || !(ahead > 0)) return newest;
  return newest.map((n) => {
    const v = vel(n.id), k = Math.min(ahead - (n.lo ? 0 : 1), 30);
    if (!v || k <= 0) return n;
    return { ...n, pos: [n.pos[0] + v[0] * k, n.pos[1], n.pos[2] + v[2] * k] };
  });
}

// The peers one command collides with: the remotes as drawn at its view tick
// (remotes.sampleAt(vt)), exactly the poses the server rewinds that command's
// collisions to (simtick.go peersOf, History.At). Replays reuse them.
// newest: the newest snapshot's peers (peersFrom), their cores (where they are
// "now", as far as we know): simtick.go peersOf sends both for a rewound command.
export function peersAt(samples, myId, solid = true, newest = null) {
  if (!solid || !samples) return null;
  const out = [];
  // The hull where it was drawn if it was a body then (r.solid: the sample at
  // t), its core where it is now if it is one now (newest): = simtick.go peersOf.
  const ids = [...new Set([...samples.map((r) => r.id), ...(newest || []).map((n) => n.id)])].filter((id) => id !== myId).sort((a, b) => a - b);
  for (const id of ids) {
    const r = samples.find((x) => x.id === id);
    if (r?.solid && r.p) out.push({ pos: r.p, h: r.c ? H_CROUCH : H, lo: id < myId, core: false, drawn: true, g: r.gv || 0 });
    const now = newest?.find((n) => n.id === id);
    if (now) out.push({ pos: now.pos, h: now.h, lo: id < myId, core: true, g: now.g || 0 });
  }
  return out.length ? out : null;
}

// The boat we ride from a self-snapshot s (me.ride, me.seat, me.boat: the
// boat's exact state after the acked command; internal/game boats.go). Getting
// in and out is the server's: we adopt it here.
export function adoptRide(me, s) {
  me.ride = s.ride || 0;
  me.seat = s.seat || 0;
  me.boat = me.ride && s.rb ? { pos: [...s.rb.p], vel: [...s.rb.v], yaw: s.rb.y, yawVel: s.rb.yv, driven: !!s.rb.rd } : null;
  me.deck = me.ride && s.dk ? { pos: [...s.dk.p], vel: [...s.dk.v] } : null; // standing on its deck (seat SeatDeck), in the boat's frame
  if (me.ride && !me.boat) me.ride = 0; // (never: the server sends the boat with the seat)
  if (me.ride && me.seat === SeatDeck && !me.deck) me.ride = me.seat = 0; // (never: nor the deck)
}

export function createPredictor(phys) {
  let pending = []; // commands sent, not yet acknowledged
  let ackB = 0; // buttons of the last acknowledged command (for jump edges in the replay)
  let minAck = 0; // after a teleport: ignore server states from before it
  let peers = null; // the solid players of the newest snapshot (peersFrom)
  let boats = null; // the boats of the newest snapshot (boats.js list)
  let tb = 0; // tick base of our command clock (me.tb): command seq runs at CmdTime(tb, seq)
  const err = [0, 0, 0]; // visual correction offset, decays to zero
  const stats = { corrections: 0, lastError: 0, pending: 0, ackDelta: 0, resyncs: 0, lastResync: null };
  let stuckSince = null; // when the smoothing offset last went above STUCK_M
  let snappedThrough = 0; // commands up to this seq were drawn before the last re-sync: their drawn place is stale
  const clear = () => { err[0] = err[1] = err[2] = 0; stuckSince = null; };
  let pr = null;
  const resync = (why, size) => {
    clear();
    if (pending.length) snappedThrough = pending[pending.length - 1].seq;
    stats.resyncs++;
    stats.lastResync = { why, size: Math.round(size * 1000) / 1000 };
    pr.onResync?.(stats.lastResync);
  };

  return (pr = {
    stats,
    // Called when the safety net re-syncs the drawn body (main.js: snap the view, log it).
    onResync: null,
    get error() { return err; },
    pendingCmds: () => pending,
    // When command seq runs on the movers' clock (the server's tick base for us).
    cmdTime: (seq) => CmdTime(tb, seq),
    get tickBase() { return tb; },
    // The other players to collide with: set from each snapshot before reconcile;
    // predicted commands and replays both use the newest.
    get peers() { return peers; },
    setPeers(p) { peers = p; },
    // The boats of the newest snapshot (boats.js list), to land on (localsim moveCmd).
    get boats() { return boats; },
    setBoats(b) { boats = b; },
    // The predicted command; cmd.pos (set by the caller after the step) and the
    // smoothing offset now are what gets drawn for it.
    push(cmd) {
      cmd.err = [err[0], err[1], err[2]];
      pending.push(cmd);
      if (pending.length > MAX_PENDING) pending.shift();
      stats.pending = pending.length;
    },
    // Forget predictions (respawn, teleport). Server states acking less than
    // minAckSeq predate the reset and are ignored. The others' places too: a
    // round restart respawns everyone, and the next snapshot has them.
    reset(minAckSeq = 0) { pending = []; ackB = 0; minAck = minAckSeq; clear(); peers = null; }, // peers: the old snapshot's places predate the respawn
    // The server restarted our command clock (spawn message, me.tb).
    setTickBase(v) { if (v !== undefined) tb = v; },
    // A server move (admin setpos) as of command s.ack: reconcile to it, and
    // ignore older server states (sent before the move) from now on.
    moved(me, s, simDt) { this.reconcile(me, s, simDt, { teleport: true }); minAck = s.ack + 1; },
    // Apply a server state to body `me` and replay unacknowledged commands.
    // watch: the safety net is on (alive and playing a body; off for the dead,
    // spectators and observers). teleport: a server move, snapped without counting.
    reconcile(me, s, simDt, { now = performance.now(), watch = !me.dead, teleport = false } = {}) {
      // The tick base first: it is the server's current one even in a state sent
      // before our teleport (it changes only on a spawn or a catch-up, between commands).
      if (s.tb !== undefined) tb = s.tb; // the replay below runs on the server's clock for us
      if (s.ack < minAck) return; // sent before our teleport reached the server
      const acked = pending.filter((c) => c.seq <= s.ack);
      let drawnGap = 0; // where we drew ourselves for the acked command vs where the server put us
      if (acked.length) {
        const last = acked[acked.length - 1];
        ackB = last.b;
        // Where we predicted ourselves after that command vs where the server put us.
        if (last.pos) {
          stats.ackDelta = Math.hypot(...last.pos.map((v, i) => v - s.p[i]));
          const le = last.err || [0, 0, 0];
          if (last.seq > snappedThrough) drawnGap = Math.hypot(...last.pos.map((v, i) => v + le[i] - s.p[i]));
        }
      }
      pending = pending.filter((c) => c.seq > s.ack);
      stats.pending = pending.length;
      const before = [...me.pos];
      // Cosmetic state the server doesn't track: keep ours.
      const keep = { eyeH: me.eyeH, eyeOff: me.eyeOff, stride: me.stride, yaw: me.yaw, pitch: me.pitch, jumps: me.jumps };
      me.pos = [...s.p]; me.vel = [...s.v]; me.onGround = s.g; me.crouched = s.c; me.grav = s.gv || 0;
      me.boots = !!s.bt; me.energy = s.en || 0; // the Gravity Override (internal/sim hook.go)
      me.noclip = !!s.nc; // flying through walls: an admin editing the map (internal/sim noclip.go)
      me.hook = s.hk ? { on: true, at: [...s.hk.at], n: [...s.hk.n], lo: [...s.hk.lo], hi: [...s.hk.hi], flip: !!s.hk.f, rope: s.hk.r, mover: s.hk.mv || 0, u: s.hk.u || 0, local: s.hk.lc ? [...s.hk.lc] : [0, 0, 0] } : { on: false, at: [0, 0, 0], n: [0, 0, 0], lo: [0, 0, 0], hi: [0, 0, 0], flip: false, rope: 0, mover: 0, u: 0, local: [0, 0, 0] };
      me.h = s.c ? H_CROUCH : H;
      adoptRide(me, s);
      let prevB = ackB;
      for (const c of pending) {
        me.yaw = c.yaw;
        const inp = { ...moveInput(c, prevB, me), speed: c.speed }; // before getting out, as the server
        moveCmd(phys, me, c, inp, prevB, simDt, CmdTime(tb, c.seq), c.peers !== undefined ? c.peers : peers, boats); // speed: as predicted (localsim); the peers it was predicted with (drawn at its vt); in a boat (ride_shared.go)
        prevB = c.b;
      }
      Object.assign(me, keep);
      if (me.eyeH === undefined) me.eyeH = me.crouched ? EYE_CROUCH : EYE;
      const e = [before[0] - me.pos[0], before[1] - me.pos[1], before[2] - me.pos[2]];
      const len = Math.hypot(...e);
      stats.lastError = len;
      if (len > 1e-4) stats.corrections++;
      if (teleport || !watch) { if (len < 2) for (let i = 0; i < 3; i++) err[i] += e[i]; else clear(); return; } // a move: snap; not playing: nothing to watch
      if (len >= SNAP_M || drawnGap >= SNAP_M) return resync(len >= SNAP_M ? "correction" : "drawn", Math.max(len, drawnGap));
      for (let i = 0; i < 3; i++) err[i] += e[i]; // smooth small corrections
      if (Math.hypot(...err) < SNAP_M) { pr.watch(now); return; }
      resync("offset", Math.hypot(...err)); // small ones piling up
    },
    // The stuck-offset check (also run on every reconcile): above STUCK_M for
    // longer than STUCK_MS means it isn't decaying (or keeps being refilled).
    watch(now = performance.now()) {
      const l = Math.hypot(...err);
      if (l <= STUCK_M) { stuckSince = null; return; }
      stuckSince ??= now;
      if (now - stuckSince > STUCK_MS) resync("stuck", l);
    },
    // Decay the visual offset: call every frame with the frame's dt. Not
    // playing a body (dead, spectating, observing): no offset to draw.
    decay(dt, playing = true, now = performance.now()) {
      if (!playing) { clear(); return; }
      const k = Math.exp(-dt * DECAY);
      for (let i = 0; i < 3; i++) err[i] = Math.abs(err[i] * k) < 1e-4 ? 0 : err[i] * k;
      pr.watch(now);
    },
  });
}

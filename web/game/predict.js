// Client-side prediction + reconciliation for the server-authoritative sim.
// Every sim tick the client runs its command locally (instant feel) and sends
// it; the server runs the same command with the same physics (internal/sim,
// parity-tested) and reports back {ack, p, v, g, c}: its body after the last
// command it applied. We rewind to that state and replay the commands it
// hasn't seen yet. With matching physics the result equals what we predicted,
// so corrections are rare and tiny; any error left is smoothed out visually.
import { moveInput } from "../engine/cmd.js";
import { CmdTime } from "./shared.gen.js";
import { H, H_CROUCH, EYE, EYE_CROUCH } from "../engine/physics.js";

const MAX_PENDING = 1024;

// The other players' bodies our body collides with (internal/sim/peers.go),
// from a snapshot's player list: as the server's peersOf, alive players on a
// team, in ID order, at their snapshot positions (the sim rounds them to the
// millimetre like the snapshot, so a standing peer predicts bit-exactly).
// solid: mp_solid_players. Null when there are none.
export function peersFrom(players, myId, solid = true) {
  if (!solid || !players) return null;
  const out = players
    .filter((p) => p.id !== myId && !p.dead && p.team !== "spec" && p.p) // = simtick.go solid()
    .sort((a, b) => a.id - b.id)
    .map((p) => ({ pos: p.p, h: p.c ? H_CROUCH : H, lo: p.id < myId }));
  return out.length ? out : null;
} // 10 s of commands at 100 Hz (beyond that the link is dead anyway)

export function createPredictor(phys) {
  let pending = []; // commands sent, not yet acknowledged
  let ackB = 0; // buttons of the last acknowledged command (for jump edges in the replay)
  let minAck = 0; // after a teleport: ignore server states from before it
  let peers = null; // the solid players of the newest snapshot (peersFrom)
  let tb = 0; // tick base of our command clock (me.tb): command seq runs at CmdTime(tb, seq)
  const err = [0, 0, 0]; // visual correction offset, decays to zero
  const stats = { corrections: 0, lastError: 0, pending: 0, ackDelta: 0 };

  return {
    stats,
    get error() { return err; },
    pendingCmds: () => pending,
    // When command seq runs on the movers' clock (the server's tick base for us).
    cmdTime: (seq) => CmdTime(tb, seq),
    get tickBase() { return tb; },
    // The other players to collide with: set from each snapshot before reconcile;
    // predicted commands and replays both use the newest.
    get peers() { return peers; },
    setPeers(p) { peers = p; },
    push(cmd) {
      pending.push(cmd);
      if (pending.length > MAX_PENDING) pending.shift();
      stats.pending = pending.length;
    },
    // Forget predictions (respawn, teleport). Server states acking less than
    // minAckSeq predate the reset and are ignored. The others' places too: a
    // round restart respawns everyone, and the next snapshot has them.
    reset(minAckSeq = 0) { pending = []; ackB = 0; minAck = minAckSeq; err[0] = err[1] = err[2] = 0; peers = null; }, // peers: the old snapshot's places predate the respawn
    // The server restarted our command clock (spawn message, me.tb).
    setTickBase(v) { if (v !== undefined) tb = v; },
    // A server move (admin setpos) as of command s.ack: reconcile to it, and
    // ignore older server states (sent before the move) from now on.
    moved(me, s, simDt) { this.reconcile(me, s, simDt); minAck = s.ack + 1; },
    // Apply a server state to body `me` and replay unacknowledged commands.
    reconcile(me, s, simDt) {
      // The tick base first: it is the server's current one even in a state sent
      // before our teleport (it changes only on a spawn or a catch-up, between commands).
      if (s.tb !== undefined) tb = s.tb; // the replay below runs on the server's clock for us
      if (s.ack < minAck) return; // sent before our teleport reached the server
      const acked = pending.filter((c) => c.seq <= s.ack);
      if (acked.length) {
        const last = acked[acked.length - 1];
        ackB = last.b;
        // Where we predicted ourselves after that command vs where the server put us.
        if (last.pos) stats.ackDelta = Math.hypot(...last.pos.map((v, i) => v - s.p[i]));
      }
      pending = pending.filter((c) => c.seq > s.ack);
      stats.pending = pending.length;
      const before = [...me.pos];
      // Cosmetic state the server doesn't track: keep ours.
      const keep = { eyeH: me.eyeH, eyeOff: me.eyeOff, stride: me.stride, yaw: me.yaw, pitch: me.pitch, jumps: me.jumps };
      me.pos = [...s.p]; me.vel = [...s.v]; me.onGround = s.g; me.crouched = s.c;
      me.h = s.c ? H_CROUCH : H;
      let prevB = ackB;
      for (const c of pending) {
        me.yaw = c.yaw;
        phys.step(me, { ...moveInput(c, prevB, me), speed: c.speed }, simDt, CmdTime(tb, c.seq), peers); // speed: as predicted (localsim)
        prevB = c.b;
      }
      Object.assign(me, keep);
      if (me.eyeH === undefined) me.eyeH = me.crouched ? EYE_CROUCH : EYE;
      const e = [before[0] - me.pos[0], before[1] - me.pos[1], before[2] - me.pos[2]];
      const len = Math.hypot(...e);
      stats.lastError = len;
      if (len > 1e-4) stats.corrections++;
      if (len < 2) for (let i = 0; i < 3; i++) err[i] += e[i]; // smooth small corrections
      else err[0] = err[1] = err[2] = 0; // teleports and respawns: snap
    },
    // Decay the visual offset (call per frame with the frame's dt).
    decay(dt) {
      const k = Math.exp(-dt * 15);
      for (let i = 0; i < 3; i++) err[i] = Math.abs(err[i] * k) < 1e-4 ? 0 : err[i] * k;
    },
  };
}

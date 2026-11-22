// Gait: a cartoon walk cycle driven by how far a body actually moved (pure, no
// game state, allocation-free). Draw-only: it never touches hit geometry.
//
//   const g = newGait();
//   gaitStep(g, dx, dy, dz, dt, {crouch, air?})   // once per sim tick, world delta
//   gaitPose(g, alpha, k, out)                     // per frame: fills `out`
//
// The phase advances with distance travelled (one cycle = two steps), scaled so a
// planted foot slides back exactly as fast as the body goes forward: no skating.
// amp follows ground speed / k.run (0 at rest, capped at 1), air follows the
// airborne flag (or vertical speed when the caller can't know), all smoothed, so
// speed changes never pop. A move faster than TELEPORT_SPEED (a teleport, a
// respawn) resets the cycle instead of spinning it.

// m/s: anything faster than this between two ticks is a teleport, not a step
// (1.5 m per 100 Hz tick, ~20x run speed).
export const TELEPORT_SPEED = 150;
// m/s of vertical speed that counts as airborne when the caller can't tell.
export const AIR_VY = 3;
// s: an inferred airborne state holds this long past its last fast vertical
// move (a jump's apex is slow; it is still a jump).
export const AIR_HOLD = 0.3;
// s without moving before a body counts as stopped (longer than a snapshot gap).
export const STILL_AFTER = 0.15;
const TAU = Math.PI * 2;

// Tunables (metres / radians at full amp). Per-player factors multiply these.
export const GAIT = {
  run: 7,          // ground speed for amp 1
  crouchRun: 2.4,  // ...when crouched (a full-size shuffle at crouch speed)
  swing: 0.28,     // foot fore/aft reach
  swingCrouch: 0.12,
  lift: 0.18,      // foot lift on the swing
  liftCrouch: 0.07,
  waddle: 0.11,    // side-to-side roll per step
  bob: 0.07,       // body bounce
  lean: 0.16,      // forward lean at full tilt
  wobble: 0.22,    // head wobble
  flail: 0.2,      // airborne foot flail reach
  sway: 0.035,     // idle sway at rest
};

export function newGait() {
  return { phase: 0, phase0: 0, amp: 0, amp0: 0, air: 0, air0: 0, crouch: 0, crouch0: 0, t: 0, t0: 0, airHold: 0, speed: 0, vy: 0, still: 0 };
}

export function resetGait(g) {
  g.phase = g.phase0 = g.amp = g.amp0 = g.air = g.air0 = g.airHold = g.speed = g.vy = g.still = 0;
  return g;
}

const ease = (cur, target, rate, dt) => cur + (target - cur) * (1 - Math.exp(-rate * dt));

// The fore/aft reach at this amp/crouch (the phase scale depends on it).
const reach = (k, amp, crouch) => (k.swing ?? 1) * amp * (GAIT.swing + (GAIT.swingCrouch - GAIT.swing) * crouch);

// One sim tick: (dx, dy, dz) the body's world movement over dt seconds.
// o.crouch: bool; o.air: bool, or undefined to infer it from vertical speed;
// o.k: per-player factors (only k.swing matters here).
export function gaitStep(g, dx, dy, dz, dt, o = {}) {
  g.phase0 = g.phase; g.amp0 = g.amp; g.air0 = g.air; g.crouch0 = g.crouch; g.t0 = g.t;
  if (!(dt > 0)) return g;
  g.t += dt;
  const d = Math.hypot(dx, dz);
  if (!Number.isFinite(d) || !Number.isFinite(dy) || d > TELEPORT_SPEED * dt || Math.abs(dy) > TELEPORT_SPEED * dt) {
    resetGait(g);
    return g;
  }
  // Speed = distance since the body last moved / time since then: a remote
  // drawn from bursty snapshots (lag, loss, a starved buffer) holds still for a
  // few ticks and then jumps; per-tick deltas would read that as stop-go.
  g.still += dt;
  let speed = g.speed, vy = g.vy;
  if (d > 1e-6 || Math.abs(dy) > 1e-6) { speed = d / g.still; vy = dy / g.still; g.still = 0; }
  else if (g.still > STILL_AFTER) speed = vy = 0;
  g.speed = speed; g.vy = vy;
  if (Math.abs(vy) > AIR_VY) g.airHold = AIR_HOLD;
  else g.airHold = Math.max(0, (g.airHold || 0) - dt);
  const air = o.air ?? g.airHold > 0;
  g.air = ease(g.air, air ? 1 : 0, air ? 14 : 9, dt);
  g.crouch = ease(g.crouch, o.crouch ? 1 : 0, 18, dt);
  const target = Math.min(1, speed / (GAIT.run + (GAIT.crouchRun - GAIT.run) * g.crouch));
  g.amp = ease(g.amp, target, 10, dt);
  // A foot sweeps 2·reach per half cycle while the body moves half a stride:
  // stride = 2π·reach keeps the planted foot still. Floor it so a tiny amp
  // doesn't spin the phase. The stride follows the speed now (not the eased
  // amp) so a standing start doesn't whirl the feet while the swing grows.
  const stride = Math.max(0.35, TAU * reach(o.k || {}, Math.max(target, 0.25), g.crouch));
  g.phase = (g.phase + (d / stride) * TAU * (1 - g.air)) % (TAU * 64);
  return g;
}

// The pose between the last two ticks (alpha 0..1) into `out` (reused; see
// newPose). k: per-player factors {swing, waddle, bob, wobble, lean, offset}.
// Fields: feet [L, R] {x, y, z, rx}; legsRoll; body {y, roll, pitch}; head {roll, pitch}.
export function newPose() {
  return { footL: { x: 0, y: 0, z: 0, rx: 0 }, footR: { x: 0, y: 0, z: 0, rx: 0 }, legsRoll: 0, body: { y: 0, roll: 0, pitch: 0 }, head: { roll: 0, pitch: 0 } };
}

export function gaitPose(g, alpha, k, out) {
  let dp = g.phase - g.phase0;
  if (dp < 0) dp += TAU * 64; // the phase wrapped
  const a = alpha < 0 ? 0 : alpha > 1 ? 1 : alpha;
  const ph = g.phase0 + dp * a + (k.offset || 0);
  const amp = g.amp0 + (g.amp - g.amp0) * a;
  const air = g.air0 + (g.air - g.air0) * a;
  const cr = g.crouch0 + (g.crouch - g.crouch0) * a;
  const t = g.t0 + (g.t - g.t0) * a;
  const ground = amp * (1 - air), rest = 1 - amp;
  const r = reach(k, ground, cr);
  const lift = ground * (k.swing ?? 1) * (GAIT.lift + (GAIT.liftCrouch - GAIT.lift) * cr);
  const flail = air * GAIT.flail;
  foot(out.footL, ph, r, lift, flail, t, 1);
  foot(out.footR, ph + Math.PI, r, lift, flail, t + 0.37, -1);
  const s = Math.sin(ph), sway = GAIT.sway * rest * (1 - air) * Math.sin(t * 1.7 + (k.offset || 0));
  // The legs block rocks onto the planted foot; the body over-rolls on top of it.
  out.legsRoll = GAIT.waddle * (k.waddle ?? 1) * ground * s * (1 - 0.5 * cr) + sway * 0.5;
  out.body.roll = out.legsRoll * 1.4 + sway;
  out.body.y = GAIT.bob * (k.bob ?? 1) * ground * Math.abs(Math.sin(ph)) + 0.06 * air;
  out.body.pitch = -GAIT.lean * (k.lean ?? 1) * ground + 0.18 * air * Math.sin(t * 9);
  out.head.roll = GAIT.wobble * (k.wobble ?? 1) * (ground * Math.sin(ph * 2 + 0.6) * 0.5 + air * Math.sin(t * 13)) - sway;
  out.head.pitch = 0.08 * ground * Math.cos(ph * 2) + 0.03 * rest * Math.sin(t * 1.1);
  return out;
}

// side: +1 left, -1 right. Forward is -Z.
function foot(f, ph, reachM, lift, flail, t, side) {
  const s = Math.sin(ph), c = Math.cos(ph);
  f.z = -reachM * s - flail * Math.sin(t * 19);
  f.y = lift * (c > 0 ? c : 0) + flail * 0.4 * (0.5 + 0.5 * Math.sin(t * 23));
  f.x = side * flail * 0.35 * (0.5 + 0.5 * Math.cos(t * 17));
  // Heel up as it leaves the ground (behind), toe up as it reaches (in front).
  f.rx = 0.5 * (lift / GAIT.lift) * s * (c > 0 ? c : 0) + flail * 1.5 * Math.sin(t * 19);
}

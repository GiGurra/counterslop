// Hit feedback, all presentation (never gameplay): the slop burst where a
// server-confirmed hit landed, and what taking a hit looks like (a red
// vignette from the attacker's side, a view-only flinch, a grey rim at low HP). Pure: time and randomness come in as
// arguments, so every client builds the same burst from the same hit.
import { rng } from "../gen/rng.js";
import { PALETTE } from "../assets/palette.js";

// FNV-1a over the hit (victim id + the hit point in cm): the burst's seed, so
// everyone who sees the hit sees the same flecks.
export function hitSeed(id, at) {
  let h = 0x811c9dc5;
  for (const v of [id, ...at.map((x) => Math.round(x * 100))]) {
    h = Math.imul(h ^ (v & 0xffff), 0x01000193);
    h = Math.imul(h ^ ((v >>> 16) & 0xffff), 0x01000193);
  }
  return h >>> 0;
}

export const BURST_LIFE = 0.6; // seconds: short, never lingers

// The particles of one hit, sized to read at a glance at 20-30 m: a quick
// green splat puff on the impact point, bright green slop flecks spraying out
// in a cone along `dir` (the shot direction, i.e. away from the shooter, wide
// enough to fan out past the victim's silhouette) and chunky bits in the
// victim's team colour. All unlit (emit 1) so they read in shade. Never blood.
// Headshots spray more. Returns [{p, v, size, life, life0, color, emit, grav,
// puff?}] in metres / seconds (size: the drawn cube's edge).
export function slopBurst(seed, at, dir, teamRgb, { head = false } = {}) {
  const r = rng(seed);
  const l = Math.hypot(dir[0], dir[1], dir[2]) || 1;
  const d = dir.map((x) => x / l);
  const p0 = at.map((x, k) => x - d[k] * 0.12); // on the near surface
  const green = [Math.min(1, PALETTE.slop[0] * 1.35), Math.min(1, PALETTE.slop[1] * 1.2), PALETTE.slop[2]];
  const out = [];
  // the splat: a loose cluster of small puffs on the impact point that swell
  // outward and vanish (a cluster, so it reads as a splat, not a box)
  for (let i = 0; i < 7; i++) {
    const life = r.range(0.2, 0.3);
    const o = [r.range(-1, 1), r.range(-1, 1), r.range(-1, 1)];
    out.push({ p: p0.map((x, k) => x + o[k] * 0.12), v: o.map((x, k) => x * 1.2 + (k === 1 ? 0.5 : 0)), life, life0: life,
      size: (head ? 0.24 : 0.2) * r.range(0.8, 1.2), color: green, emit: 1, grav: 0, puff: true });
  }
  const flecks = head ? 32 : 24, chunks = head ? 7 : 5;
  for (let i = 0; i < flecks + chunks; i++) {
    const chunk = i >= flecks;
    const speed = chunk ? r.range(2, 3.2) : r.range(3, 6);
    // a wide cone around the shot direction (~±60°, a little up): it fans out
    // past the victim's silhouette, so the shooter sees it too
    const v = [0, 1, 2].map((k) => (d[k] * 0.6 + r.range(-1, 1)) * speed);
    v[1] += r.range(1, 2.4);
    const life = chunk ? r.range(0.45, BURST_LIFE) : r.range(0.35, 0.55);
    out.push({
      p: [...p0], v, life, life0: life,
      size: chunk ? r.range(0.22, 0.3) : r.range(0.13, 0.2),
      color: chunk ? teamRgb : green,
      emit: 1, grav: chunk ? 7 : 4,
    });
  }
  return out;
}

// Advance burst particles by dt (in place); drops dead ones.
export function stepBurst(parts, dt) {
  for (let i = parts.length - 1; i >= 0; i--) {
    const p = parts[i];
    p.life -= dt;
    if (p.life <= 0) { parts.splice(i, 1); continue; }
    for (let k = 0; k < 3; k++) p.p[k] += p.v[k] * dt;
    p.v[1] -= p.grav * dt;
    const drag = Math.exp(-dt * 2.5);
    p.v[0] *= drag; p.v[2] *= drag;
  }
}
// Drawn size: a puff swells to ~2x then shrinks away; a fleck or chunk keeps
// its size, shrinking only over the last third of its life.
export function burstSize(p) {
  const k = p.life / p.life0; // 1 -> 0
  return p.puff ? p.size * (1 + 1.2 * (1 - k)) * Math.min(1, k * 3) : p.size * Math.min(1, k * 3);
}

// ---- taking a hit ----

export const VIGNETTE_S = 0.4; // the red edge fades out over this
export const FLINCH_S = 0.35;  // the view springs back within this
const DEG = Math.PI / 180;

// Angle of the attacker, clockwise from straight ahead (radians), for a body
// at pos facing yaw (yaw 0 faces -Z, like the damage arcs).
export function relAngle(pos, yaw, from) {
  const dx = from[0] - pos[0], dz = from[2] - pos[2];
  return -(Math.atan2(-dx, -dz) - yaw);
}

// One hit we took: { at (s), dmg, a0 (attacker angle when hit, for the flinch) }.
// Flinch: kick the view away from the attacker, a degree or two by damage,
// as a spring (up fast, back smoothly). View only: never touches aim.
export function flinch(hits, t) {
  let yaw = 0, pitch = 0;
  for (const h of hits) {
    const age = t - h.at;
    if (age < 0 || age >= FLINCH_S) continue;
    const mag = Math.min(2, 0.6 + h.dmg / 40) * DEG;
    const k = 0.06, env = (age / k) * Math.exp(1 - age / k); // peaks at 60 ms, ~0 by FLINCH_S
    yaw += -Math.sin(h.a0) * mag * env;        // pushed away from the side it came from
    pitch += (0.35 + 0.65 * Math.abs(Math.cos(h.a0))) * mag * env; // and up a bit
  }
  return { yaw, pitch };
}

// The red vignette: one soft edge per recent hit, on the attacker's side
// (re-aimed as we turn: angle(h) gives the current relative angle), its
// strength by damage, fading over VIGNETTE_S. Returns [{x, y, a}] with x, y
// the edge point in % of the screen and a the alpha.
export function vignette(hits, t, angle) {
  const out = [];
  for (const h of hits) {
    const age = t - h.at;
    if (age < 0 || age >= VIGNETTE_S) continue;
    const a = Math.min(0.75, 0.25 + h.dmg / 80) * (1 - age / VIGNETTE_S);
    const rel = angle(h);
    out.push({ x: 50 + 62 * Math.sin(rel), y: 50 - 62 * Math.cos(rel), a });
  }
  return out;
}

// CSS background for the vignette edges and the low-HP edge strength
// (lowHpEdge) ("" when nothing shows).
export function vignetteCss(edges, low = 0) {
  const layers = edges.map((e) => `radial-gradient(ellipse 75% 75% at ${e.x.toFixed(1)}% ${e.y.toFixed(1)}%, rgba(255, 24, 24, ${e.a.toFixed(3)}) 0%, rgba(255, 24, 24, 0) 70%)`);
  if (low > 0) layers.push(`radial-gradient(ellipse 72% 72% at 50% 50%, rgba(118, 118, 124, 0) 40%, rgba(118, 118, 124, ${(0.85 * low).toFixed(3)}) 76%, rgba(52, 50, 56, ${Math.min(1, 0.95 * low).toFixed(3)}) 100%)`);
  return layers.join(", ");
}

// Low HP: how strong the grey rim is (0 above 25 HP; ~0.65-0.85 between
// beats, more nearer death), pulsing with the heartbeat: a swell right on the beat that
// settles back over ~0.35 s. sinceBeat: seconds since the last heartbeat.
export const HEART_PULSE_S = 0.35;
export function lowHpEdge(hp, sinceBeat = Infinity) {
  if (!(hp > 0) || hp > 25) return 0;
  const base = 0.75 + 0.25 * (1 - hp / 25);
  const pulse = sinceBeat >= 0 ? Math.exp(-sinceBeat / HEART_PULSE_S * 3) : 0;
  return Math.min(1, base * (0.85 + 0.15 * pulse));
}

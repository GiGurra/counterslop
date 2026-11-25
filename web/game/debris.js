// What shots leave behind for the round: bullet holes where rays stop in the
// map, casings out of the ejection port, and the sounds of both (anyone's,
// heard near the listener). Glue over marks.js / casings.js / impactsfx.js;
// purely cosmetic: the server never hears of any of it.
import { createHoles, createLifetime, boxAt, holeKind } from "./marks.js";
import { createCasings, casingKind, ejectDelay, ejection, KINDS } from "./casings.js";
import { surfaceOf, impactSound, hardFloor, createLimiter, IMPACT_RANGE, CASING_RANGE } from "./impactsfx.js";
import { basis } from "../engine/math.js";

// Hole size (radius of the decal, m) by weapon class: the pit is about a fifth
// of it, the chipped ring out to about half (GitHub #17: a little bigger than
// before, 0.07-0.11, so they read from 20 m; not cartoon craters).
const HOLE_SIZE = { pistol: 0.08, smg: 0.08, rifle: 0.1, heavy: 0.1, sniper: 0.125, shotgun: 0.07 };
const EYE = 1.62, EYE_CROUCH = 1.05;

// sfx: the sound catalog; spatial(p) -> {pan, gain}; listener() -> [x,y,z] | null;
// cfg() -> {r_decals, r_casings}; boxes() -> the map's boxes; raycast(o, d, maxT) -> {t, n}.
export function createDebris({ sfx, spatial, listener, cfg, boxes, raycast }) {
  const holes = createHoles(), casings = createCasings(), life = createLifetime();
  const pending = []; // bolt actions: casings waiting for the bolt {at, w, eye, b, vel, seed}
  const impactLimit = createLimiter({ rate: 14, burst: 5 }), casingLimit = createLimiter({ rate: 8, burst: 3 });
  let clock = 0, seq = 0;
  const near = (p, range) => { const l = listener(); return !!l && Math.hypot(p[0] - l[0], p[1] - l[1], p[2] - l[2]) < range; };
  const pitch = () => 0.9 + Math.random() * 0.2;

  // A ray of w stopped on the map at p (on the face, n its normal); hole:
  // leave a bullet hole (not on a moving solid: the hole would stay behind).
  function impact(p, n, w, hole = true) {
    const surface = surfaceOf(boxAt(boxes(), p, n)?.kind);
    if (hole && cfg().r_decals) holes.add(p, n, HOLE_SIZE[w?.class] ?? 0.09, holeKind(surface));
    if (!near(p, IMPACT_RANGE) || !impactLimit.allow("impact", clock)) return;
    sfx.bulletImpact?.(impactSound(surface), spatial(p), pitch());
  }

  // A ray of w from o along d (unit) through the walls passes (Go WallTrace):
  // a hole where it went into each and where it came out of each it got
  // through, up to end (where a body stopped it). The face and its normal
  // come from a short ray at each point (none on a moving solid).
  function wallHoles(o, d, passes, end, w) {
    const at = (t) => [o[0] + d[0] * t, o[1] + d[1] * t, o[2] + d[2] * t];
    for (const p of passes ?? []) { // (null: no walls, as the generated Go gives an empty slice)
      if (!(p.in < end)) break;
      face(at(p.in - 0.05), d, w);
      if (p.keep > 0 && p.out < end) face(at(p.out + 0.05), [-d[0], -d[1], -d[2]], w);
    }
  }
  // The face 5 cm along d from p: a hole and its sound there.
  function face(p, d, w) {
    const hit = raycast(p, d, 0.1);
    if (hit) impact([p[0] + d[0] * hit.t, p[1] + d[1] * hit.t, p[2] + d[2] * hit.t], hit.n, w, !hit.mover);
  }

  // A shot of w from eye, aimed along basis b, the shooter moving at vel: its casing.
  function eject(w, eye, b, vel, seed) {
    if (!cfg().r_casings || casingKind(w) < 0) return;
    const delay = ejectDelay(w);
    if (delay > 0) { pending.push({ at: clock + delay, w, eye: [...eye], b, vel: [...vel], seed }); return; }
    spawnCasing(w, eye, b, vel, seed);
  }
  function spawnCasing(w, eye, b, vel, seed) {
    const { p, v } = ejection(eye, b, vel, seed);
    casings.spawn(p, v, casingKind(w, seed), seed);
  }
  const nextSeed = () => ((Math.imul(++seq, 2654435761) >>> 0) / 4294967296);

  // Our own shot: rays already traced (the client knows where they stopped).
  function localShot(w, eye, yaw, pitchA, vel) { eject(w, eye, basis(yaw, pitchA), vel, nextSeed()); }

  // Someone else's (the "shot" event): its casing from their pose, holes where
  // the server says its rays went into the map (m.h, from m.o) and came out of
  // a wall (m.x, wallbangs); the normal from our own short ray at each.
  function remoteShot(m, w, pose) {
    if (pose) {
      const eye = [pose.p[0], pose.p[1] + (pose.c ? EYE_CROUCH : EYE), pose.p[2]];
      eject(w, eye, basis(pose.yaw, pose.pitch), [0, 0, 0], nextSeed());
    }
    if (!m.o || !Array.isArray(m.h)) return;
    for (const h of m.h) {
      const d = [h[0] - m.o[0], h[1] - m.o[1], h[2] - m.o[2]], t = Math.hypot(...d);
      if (!(t > 0.01)) continue;
      for (let k = 0; k < 3; k++) d[k] /= t;
      face([h[0] - d[0] * 0.05, h[1] - d[1] * 0.05, h[2] - d[2] * 0.05], d, w); // (a wall behind a wallbang too: not the first face from m.o)
    }
    for (const x of Array.isArray(m.x) ? m.x : []) { // wallbang exits: the face the ray came out of, looking back
      const d = [m.o[0] - x[0], m.o[1] - x[1], m.o[2] - x[2]], t = Math.hypot(...d);
      if (!(t > 0.01)) continue;
      for (let k = 0; k < 3; k++) d[k] /= t;
      face([x[0] - d[0] * 0.05, x[1] - d[1] * 0.05, x[2] - d[2] * 0.05], d, w);
    }
  }

  function onBounce(e) {
    if (!e.first || !near(e.p, CASING_RANGE) || !casingLimit.allow("casing", clock)) return;
    const hard = hardFloor(surfaceOf(e.box?.kind)), sp = spatial(e.p);
    if (KINDS[e.kind].base < 1) sfx.casingShell?.(sp, hard, pitch());
    else sfx.casingBrass?.(sp, hard, pitch());
  }

  function clear() { holes.clear(); casings.clear(); pending.length = 0; }

  // Every frame: dt (s), the round state (rounds.js state) and the map's name.
  function update(dt, round, mapName) {
    clock += dt;
    if (life.update(round, mapName)) clear();
    const c = cfg();
    if (!c.r_decals && holes.count) holes.clear();
    if (!c.r_casings && casings.count) { casings.clear(); pending.length = 0; }
    for (let i = pending.length - 1; i >= 0; i--) {
      const e = pending[i];
      if (clock < e.at) continue;
      pending.splice(i, 1);
      spawnCasing(e.w, e.eye, e.b, e.vel, e.seed);
    }
    casings.step(dt, boxes(), onBounce);
  }

  // Benchmarks: nh holes and nc lying casings on the floor (y) in a grid around c.
  function testFill(c, nh, nc) {
    for (let i = 0; i < nh; i++) holes.add([c[0] + ((i % 32) - 16) * 0.15, c[1] + 0.001, c[2] + (Math.floor(i / 32) - 16) * 0.15], [0, 1, 0], 0.08);
    for (let i = 0; i < nc; i++) casings.spawn([c[0] + ((i % 24) - 12) * 0.2, c[1] + 0.01, c[2] + (Math.floor(i / 24) - 10) * 0.2], [0, 0, 0], i % KINDS.length, i / nc);
    casings.step(0.05, boxes());
  }

  // For the renderer (engine/instanced.js).
  const marks = { decals: holes.ring, bits: casings.ring, kinds: KINDS };
  return {
    impact, wallHoles, localShot, remoteShot, update, clear, marks, testFill,
    stats: () => ({ holes: holes.count, casings: casings.count, flying: casings.flying, pending: pending.length }),
  };
}

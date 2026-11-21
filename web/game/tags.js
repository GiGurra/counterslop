// Which name tags to draw, CS-style (pure; main.js renders them).
//   - Playing: teammates always (fading with distance); an enemy only while your
//     crosshair is on them and nothing in the map is in between, so names
//     never show through walls.
//   - Observing (spectating or dead): everyone.
//   - Either way, de-cluttered: a tag that overlaps a nearer one is dropped.
import { basis } from "../engine/math.js";

export const TAG_FAR = 60;      // metres: no tag beyond this
const FADE_FROM = 20, FADE_MIN = 0.35;
const AIM_RADIUS = 0.45;        // metres around the body's axis that count as "on them"
const TAG_H = 16, CHAR_W = 7;   // approximate tag box in px, for overlap pruning

// Does the segment a->b pass through any box ({min, max})? Slab test.
export function segmentBlocked(boxes, a, b) {
  const d = [b[0] - a[0], b[1] - a[1], b[2] - a[2]];
  for (const bx of boxes) {
    let t0 = 0, t1 = 1, hit = true;
    for (let i = 0; i < 3 && hit; i++) {
      if (Math.abs(d[i]) < 1e-9) {
        if (a[i] < bx.min[i] || a[i] > bx.max[i]) hit = false;
        continue;
      }
      let ta = (bx.min[i] - a[i]) / d[i], tb = (bx.max[i] - a[i]) / d[i];
      if (ta > tb) [ta, tb] = [tb, ta];
      t0 = Math.max(t0, ta); t1 = Math.min(t1, tb);
      if (t0 > t1) hit = false;
    }
    if (hit) return true;
  }
  return false;
}

// Is the view ray (eye, fwd) on a player standing at p (feet), height h?
// The distance between the ray and the body's vertical axis, in front of you.
export function aimedAt(eye, fwd, p, h) {
  // closest points between the ray eye + s*fwd (s >= 0) and the segment p..p+h*up
  const w = [eye[0] - p[0], eye[1] - p[1], eye[2] - p[2]];
  const b = fwd[1], d = fwd[0] * w[0] + fwd[1] * w[1] + fwd[2] * w[2], e = w[1];
  const den = 1 - b * b;
  let s = den > 1e-9 ? (b * e - d) / den : 0, t = den > 1e-9 ? (e - b * d) / den : e;
  t = Math.min(h, Math.max(0, t));
  s = Math.max(0, b * t - d); // re-project with t clamped
  const q = [eye[0] + fwd[0] * s - p[0], eye[1] + fwd[1] * s - p[1] - t, eye[2] + fwd[2] * s - p[2]];
  return Math.hypot(...q) <= AIM_RADIUS;
}

// tagAlpha: full up close, fading to FADE_MIN at TAG_FAR.
export const tagAlpha = (dist) => (dist <= FADE_FROM ? 1 : Math.max(FADE_MIN, 1 - (1 - FADE_MIN) * (dist - FADE_FROM) / (TAG_FAR - FADE_FROM)));

// pickTags: the tags to draw this frame, nearest first.
//   view: { eye, yaw, pitch, team, observing }
//   players: [{ id, name, team, p, crouch }]   (remote players)
//   boxes: the map's solid boxes; project(world) -> [x, y, depth] | null
// Returns [{ id, name, team, x, y, alpha }].
export function pickTags(view, players, boxes, project) {
  const { fwd } = basis(view.yaw, view.pitch);
  const cand = [];
  for (const pl of players) {
    const h = pl.crouch ? 1.2 : 1.8;
    const head = [pl.p[0], pl.p[1] + 2.15 - (pl.crouch ? 0.6 : 0), pl.p[2]];
    const dist = Math.hypot(pl.p[0] - view.eye[0], pl.p[1] - view.eye[1], pl.p[2] - view.eye[2]);
    if (dist > TAG_FAR) continue;
    if (!view.observing && pl.team !== view.team) {
      if (!aimedAt(view.eye, fwd, pl.p, h)) continue;
      const chest = [pl.p[0], pl.p[1] + h * 0.75, pl.p[2]];
      if (segmentBlocked(boxes, view.eye, chest)) continue;
    }
    const s = project(head);
    if (!s) continue;
    cand.push({ id: pl.id, name: pl.name, team: pl.team, x: s[0], y: s[1], dist, alpha: tagAlpha(dist) });
  }
  cand.sort((a, b) => a.dist - b.dist);
  const kept = [];
  for (const c of cand) {
    const w = c.name.length * CHAR_W;
    const over = kept.some((k) => Math.abs(k.x - c.x) * 2 < (w + k.name.length * CHAR_W) && Math.abs(k.y - c.y) < TAG_H);
    if (!over) kept.push(c);
  }
  return kept.map(({ dist, ...t }) => t);
}

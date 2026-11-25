// Where a sound is, as the listener hears it (GitHub #10). Pure: main.js hands
// in the listener (ear position, view yaw / pitch) and the map's raycast;
// gen/synth/core.js dest() turns the result into WebAudio nodes:
//   pan     -1..1, the stereo fallback (snd_hrtf 0): left / right of the view
//   gain    distance falloff (falloff()), less when walls are in between
//   dir     unit direction in WebAudio listener space (x right, y up, -z ahead)
//           for an HRTF panner (snd_hrtf 1): front / back and up / down too
//   cut     low-pass cutoff (Hz): ~20 kHz in the open, ~800 Hz behind a wall
//   hrtf    whether to use the HRTF panner
import { basis } from "../engine/math.js";

export const NEAR = 2; // m: full volume up to here
export const HALF = 7; // m past NEAR where it is at half volume
export const FAR = 90; // m: silent from here (faded over the last 30%)
export const OPEN_CUT = 20000, WALL_CUT = 800; // Hz
export const WALL_GAIN = 0.55; // gain behind a wall (both rays blocked)
const LIFT = 0.8; // m: the second occlusion ray, above the first (a sound over a low wall is half muffled)

// Gain by distance (m): 1 near, 1/2 at NEAR + HALF, smoothly to 0 at FAR.
export function falloff(d) {
  if (d <= NEAR) return 1;
  const g = 1 / (1 + (d - NEAR) / HALF);
  const fadeFrom = FAR * 0.7;
  return d >= FAR ? 0 : d > fadeFrom ? g * (FAR - d) / (FAR - fadeFrom) : g;
}

// How much of a sound at src the walls hide from the ear: 0 (open), 0.5 (one
// of the two rays blocked), 1 (walled off). raycast(o, dir, maxT) -> {t} | null
// against the map. A hit within 0.4 m of the source is the surface it is on
// (a bullet impact on a wall), not a wall in between.
export function occlusion(ear, src, raycast) {
  if (!raycast) return 0;
  let blocked = 0;
  for (const up of [0, LIFT]) {
    const o = [ear[0], ear[1] + up, ear[2]], t = [src[0], src[1] + up, src[2]];
    const v = [t[0] - o[0], t[1] - o[1], t[2] - o[2]], d = Math.hypot(v[0], v[1], v[2]);
    if (d < 0.5) continue;
    const hit = raycast(o, [v[0] / d, v[1] / d, v[2] / d], d);
    if (hit && hit.t < d - 0.4) blocked++;
  }
  return blocked / 2;
}

// listener: {pos (the ear), yaw, pitch}; p: the sound's position;
// opts: {hrtf, raycast}. Returns {pan, gain, dir, cut, hrtf} (see the top).
export function spatialize(listener, p, { hrtf = true, raycast = null } = {}) {
  const e = listener.pos, v = [p[0] - e[0], p[1] - e[1], p[2] - e[2]];
  const d = Math.hypot(v[0], v[1], v[2]);
  const b = basis(listener.yaw, listener.pitch || 0);
  const dot = (a) => a[0] * v[0] + a[1] * v[1] + a[2] * v[2];
  const x = dot(b.right), y = dot(b.up), z = dot(b.fwd); // right, up, ahead
  const dir = d > 0.01 ? [x / d, y / d, -z / d] : [0, 0, -1];
  const flat = Math.hypot(x, z);
  const pan = flat > 0.01 ? (x / flat) * 0.8 : 0;
  const m = occlusion(e, p, raycast);
  return {
    pan, dir, hrtf: !!hrtf,
    gain: falloff(d) * (1 - (1 - WALL_GAIN) * m),
    cut: m > 0 ? OPEN_CUT * Math.pow(WALL_CUT / OPEN_CUT, m) : OPEN_CUT,
  };
}

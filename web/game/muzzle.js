// Where a shot's flash light sits and which way it shines. The flash starts at
// the muzzle (ahead of the gun, right of and below the eye, as the viewmodel
// holds it) and shines mostly forward, so the shooter's own gun isn't lit from
// behind (operator report). The GLSL (gen/glsl/lights.js dynLight) uses the
// same spread: keep MUZZLE.back and the smoothstep edges equal there.
import { basis } from "../engine/math.js";

export const MUZZLE = { ahead: 0.9, right: 0.12, down: 0.1, wallGap: 0.15, back: 0.25 };

// eye, yaw, pitch; wallT: distance to the first wall along the aim (null: none within reach).
export function muzzlePose(eye, yaw, pitch, wallT) {
  const b = basis(yaw, pitch);
  const reach = Math.max(0, Math.min(MUZZLE.ahead, (wallT ?? Infinity) - MUZZLE.wallGap));
  const k = reach / MUZZLE.ahead; // pulled back: the side offsets shrink with it
  const pos = eye.map((v, i) => v + b.fwd[i] * reach + b.right[i] * MUZZLE.right * k - b.up[i] * MUZZLE.down * k);
  return { pos, dir: b.fwd };
}

const smooth = (a, b, x) => { const t = Math.min(1, Math.max(0, (x - a) / (b - a))); return t * t * (3 - 2 * t); };
// cos: the angle between the aim and the direction from the flash to the lit point.
export const flashSpread = (cos) => MUZZLE.back + (1 - MUZZLE.back) * smooth(-0.2, 0.8, cos);

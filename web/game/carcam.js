// The chase camera (cl_carcam 1): behind and over the car we drive, looking
// where we aim. The gun fires from the seat, a few metres ahead of and below
// the camera, so a shot along the camera's own angles would land beside the
// crosshair up close (parallax). Driving under the chase camera, the
// command's angles are instead the seat's eye to the point the camera's
// centre ray hits (convergeAim): the shot goes where the crosshair is.
import { basis } from "../engine/math.js";

// The chase camera's eye for a car of type spec at carPos, aimed at yaw, pitch.
export function chaseEye(spec, carPos, yaw, pitch) {
  const b = basis(yaw, pitch), back = 3.5 + (spec?.size?.[2] ?? 4);
  const pivot = [carPos[0], carPos[1] + (spec?.size?.[1] ?? 1.2) + 1.1, carPos[2]];
  const eye = pivot.map((v, i) => v - b.fwd[i] * back);
  eye[1] = Math.max(eye[1], carPos[1] + 0.5);
  return eye;
}

// The angles [yaw, pitch] from the seat's eye `from` to what the camera at
// `cam` sees down its centre (yaw, pitch): raycast(o, d, max) -> distance (or
// null: nothing within max, aim at max).
export function convergeAim(cam, yaw, pitch, from, raycast, max = 250) {
  const d = basis(yaw, pitch).fwd;
  const hit = raycast(cam, d, max);
  const t = hit && Number.isFinite(hit.t ?? hit) ? (hit.t ?? hit) : max;
  const p = [cam[0] + d[0] * t, cam[1] + d[1] * t, cam[2] + d[2] * t];
  const dx = p[0] - from[0], dy = p[1] - from[1], dz = p[2] - from[2];
  if (Math.hypot(dx, dz) < 0.5) return [yaw, pitch]; // right on top of it: keep the view's
  return [Math.atan2(-dx, -dz), Math.atan2(dy, Math.hypot(dx, dz))];
}

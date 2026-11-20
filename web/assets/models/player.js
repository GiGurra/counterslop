// Player figure built from boxes: body, feet, head with glowing visor, toy gun.
// Returns render parts [{m, color, emit}] for the renderer's solid pass.
import { chain, translate, scale, rotX, rotY, rotZ } from "../../engine/math.js";
import { PALETTE as P } from "../palette.js";
import { scaleRgb } from "../../gen/color.js";

// p: {p:[x,y,z], yaw, pitch, rgb, moving, phase, crouch 0..1, flash}
export function playerParts(p) {
  const c = p.rgb, out = [];
  const add = (m, color, emit = 0) => out.push({ m, color, emit });
  const waddle = p.moving ? Math.sin(p.phase) * 0.12 : 0;
  const base = chain(translate(...p.p), rotY(p.yaw), rotZ(waddle));
  const cr = p.crouch || 0, up = translate(0, -0.6 * cr, 0); // squash the torso when crouched
  add(chain(base, translate(0, 0.7 - 0.3 * cr, 0), scale(0.72, 1.3 - 0.6 * cr, 0.46)), c);
  add(chain(base, translate(-0.2, 0.08, 0), scale(0.26, 0.16, 0.5)), scaleRgb(c, 0.5));
  add(chain(base, translate(0.2, 0.08, 0), scale(0.26, 0.16, 0.5)), scaleRgb(c, 0.5));
  const head = chain(base, up, translate(0, 1.58, 0), rotX(p.pitch * 0.6));
  add(chain(head, scale(0.52, 0.48, 0.52)), c);
  add(chain(head, translate(0, 0.03, -0.25), scale(0.44, 0.16, 0.06)), P.visor, 0.9);
  const gun = chain(base, up, translate(0.32, 1.12, -0.3), rotX(p.pitch));
  add(chain(gun, translate(0, 0, -0.1), scale(0.1, 0.13, 0.5)), P.toyTeal);
  add(chain(gun, translate(0, 0, -0.38), scale(0.07, 0.07, 0.08)), P.toyTip, 0.6);
  if (p.flash > 0) add(chain(gun, translate(0, 0, -0.5), scale(0.22, 0.22, 0.22)), P.flash, 1);
  return out;
}

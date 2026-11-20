// First-person toy rifle (with the orange safety tip, obviously).
// v: {bobX, bobY, kick 0..1, flash s, reload 0..1}. Returns render parts in world space.
import { chain, translate, scale, rotX, rotY, rotZ } from "../../engine/math.js";
import { PALETTE as P } from "../palette.js";

export function viewmodelParts(eye, yaw, pitch, v) {
  const out = [];
  const add = (m, color, emit = 0) => out.push({ m, color, emit });
  const cam = chain(translate(...eye), rotY(yaw), rotX(pitch));
  const dip = Math.sin(Math.min(v.reload, 1) * Math.PI); // reload dip 0..1..0
  const g = chain(cam,
    translate(0.24 + v.bobX, -0.22 + v.bobY - dip * 0.25, -0.5 + v.kick * 0.08),
    rotX(v.kick * 0.25 - dip * 0.9), rotZ(dip * 0.6), rotY(0.04), scale(0.8, 0.8, 0.8));
  add(chain(g, scale(0.09, 0.11, 0.38)), P.toyTeal);
  add(chain(g, translate(0, 0.075, -0.05), scale(0.03, 0.04, 0.2)), P.toyPink);
  add(chain(g, translate(0, -0.12 - dip * 0.15, 0.04), scale(0.055, 0.15, 0.07)), P.toyYellow);
  add(chain(g, translate(0, -0.1, 0.16), scale(0.07, 0.16, 0.08)), P.toyTealDark);
  add(chain(g, translate(0, 0.01, -0.3), scale(0.04, 0.04, 0.22)), P.gunmetal);
  add(chain(g, translate(0, 0.01, -0.43), scale(0.05, 0.05, 0.05)), P.toyTip, 0.6);
  if (v.flash > 0) add(chain(g, translate(0, 0.01, -0.52), rotZ(v.flash * 7), scale(0.12, 0.12, 0.05)), P.flashHot, 1);
  return out;
}

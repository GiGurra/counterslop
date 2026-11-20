// Player figures built from boxes. Shared skeleton (body, feet, head, toy gun)
// plus a team outfit:
//   Vibe Coders: dark hoodie with hood, RGB-cycling headset and visor.
//   Artisans: linen shirt, oak leather apron, a hand-knit beanie whose
//   ribbing, size and pom-pom are generated from the player's id.
// Returns render parts [{m, color, emit}] for the renderer's solid pass.
import { chain, translate, scale, rotX, rotY, rotZ } from "../../engine/math.js";
import { PALETTE as P } from "../palette.js";
import { scaleRgb, hue } from "../../gen/color.js";
import { rng } from "../../gen/rng.js";

// p: {p, yaw, pitch, rgb (personal accent), team, id, time, moving, phase, crouch 0..1, flash}
export function playerParts(p) {
  const out = [];
  const add = (m, color, emit = 0) => out.push({ m, color, emit });
  const waddle = p.moving ? Math.sin(p.phase) * 0.12 : 0;
  const base = chain(translate(...p.p), rotY(p.yaw), rotZ(waddle));
  const cr = p.crouch || 0, up = translate(0, -0.6 * cr, 0); // squash the torso when crouched
  const torsoY = 0.7 - 0.3 * cr, torsoH = 1.3 - 0.6 * cr;
  const head = chain(base, up, translate(0, 1.58, 0), rotX(p.pitch * 0.6));
  const outfit = OUTFITS[p.team] || plain;
  outfit({ add, base, head, torsoY, torsoH, p });
  const gun = chain(base, up, translate(0.32, 1.12, -0.3), rotX(p.pitch));
  add(chain(gun, translate(0, 0, -0.1), scale(0.1, 0.13, 0.5)), P.toyTeal);
  add(chain(gun, translate(0, 0, -0.38), scale(0.07, 0.07, 0.08)), P.toyTip, 0.6);
  if (p.flash > 0) add(chain(gun, translate(0, 0, -0.5), scale(0.22, 0.22, 0.22)), P.flash, 1);
  return out;
}

function plain({ add, base, head, torsoY, torsoH, p }) {
  add(chain(base, translate(0, torsoY, 0), scale(0.72, torsoH, 0.46)), p.rgb);
  feet(add, base, scaleRgb(p.rgb, 0.5));
  add(chain(head, scale(0.52, 0.48, 0.52)), p.rgb);
  add(chain(head, translate(0, 0.03, -0.25), scale(0.44, 0.16, 0.06)), P.visor, 0.9);
}

function feet(add, base, color) {
  add(chain(base, translate(-0.2, 0.08, 0), scale(0.26, 0.16, 0.5)), color);
  add(chain(base, translate(0.2, 0.08, 0), scale(0.26, 0.16, 0.5)), color);
}

const OUTFITS = {
  vibe({ add, base, head, torsoY, torsoH, p }) {
    const rgbGlow = hue(((p.time || 0) * 0.25 + (p.id || 0) * 0.17) % 1);
    add(chain(base, translate(0, torsoY, 0), scale(0.78, torsoH, 0.5)), P.vibeHoodie); // hoodie
    add(chain(base, translate(0, torsoY - torsoH * 0.46, 0), scale(0.8, torsoH * 0.08, 0.52)), P.vibeNeon, 0.8); // neon hem
    for (const sx of [-1, 1]) add(chain(base, translate(sx * 0.395, torsoY, 0), scale(0.02, torsoH * 0.8, 0.08)), P.vibeNeon, 0.8); // side stripes
    add(chain(base, translate(0, torsoY - torsoH * 0.18, -0.255), scale(0.5, torsoH * 0.22, 0.02)), p.rgb, 0.3); // kangaroo pocket
    for (const sx of [-1, 1]) add(chain(base, translate(sx * 0.1, torsoY + torsoH * 0.32, -0.255), scale(0.03, torsoH * 0.25, 0.02)), P.vibeCyan, 0.7); // drawstrings
    feet(add, base, P.vibeDark);
    add(chain(head, scale(0.52, 0.48, 0.52)), P.vibeDark); // face in the hood's shadow
    add(chain(head, translate(0, 0.06, 0.06), scale(0.6, 0.6, 0.5)), scaleRgb(P.vibeHoodie, 0.85)); // hood
    add(chain(head, translate(0, 0.33, -0.17), scale(0.62, 0.06, 0.06)), P.vibeNeon, 0.8); // hood rim
    add(chain(head, translate(0, 0.03, -0.25), scale(0.46, 0.15, 0.06)), rgbGlow, 0.95); // RGB visor
    add(chain(head, translate(0, 0.38, 0.02), scale(0.66, 0.05, 0.1)), P.vibeDark); // headset band
    for (const sx of [-1, 1]) add(chain(head, translate(sx * 0.32, 0.02, 0), scale(0.08, 0.22, 0.22)), rgbGlow, 0.8); // ear cups
    add(chain(head, translate(0.25, -0.14, -0.2), scale(0.03, 0.03, 0.18)), P.vibeDark); // boom mic
    add(chain(head, translate(0.25, -0.14, -0.3), scale(0.05, 0.05, 0.05)), P.vibeCyan, 0.9);
  },
  artisan({ add, base, head, torsoY, torsoH, p }) {
    const r = rng(1000 + (p.id || 0)); // the same hand-knit beanie for this player on every client
    add(chain(base, translate(0, torsoY, 0), scale(0.72, torsoH, 0.46)), P.artisanLinen); // shirt
    add(chain(base, translate(0, torsoY - torsoH * 0.08, -0.24), scale(0.6, torsoH * 0.8, 0.03)), P.artisanOak); // apron
    add(chain(base, translate(0, torsoY - torsoH * 0.05, -0.26), scale(0.3, torsoH * 0.18, 0.02)), scaleRgb(P.artisanOak, 0.75)); // apron pocket
    for (const sx of [-1, 1]) {
      add(chain(base, translate(sx * 0.22, torsoY + torsoH * 0.42, -0.235), scale(0.05, torsoH * 0.18, 0.03)), P.artisanOak); // straps
      add(chain(base, translate(sx * 0.22, torsoY + torsoH * 0.33, -0.262), scale(0.07, 0.07, 0.02)), P.artisanCopper, 0.35); // copper rivets
    }
    add(chain(base, translate(0, torsoY - torsoH * 0.42, -0.262), scale(0.6, 0.04, 0.02)), P.artisanCopper, 0.35); // copper-stitched hem
    feet(add, base, scaleRgb(P.artisanOak, 0.7));
    add(chain(head, scale(0.52, 0.48, 0.52)), P.artisanSkin);
    add(chain(head, translate(0, 0.05, -0.25), scale(0.44, 0.12, 0.06)), P.artisanAmber, 0.7); // warm visor (reading glasses)
    // a beard, grown slowly and organically: length varies per player
    const beard = r.range(0.14, 0.26), beardRgb = scaleRgb(P.artisanCopper, r.range(0.55, 0.8));
    add(chain(head, translate(0, -0.2 - beard * 0.35, -0.2), scale(0.46, 0.12 + beard, 0.16)), beardRgb);
    add(chain(head, translate(0, -0.08, -0.27), scale(0.24, 0.05, 0.03)), scaleRgb(beardRgb, 0.85)); // moustache
    // beanie: ribbed rows that taper, slightly uneven like real hand-knitting
    const rows = r.int(3, 5), wool = scaleRgb(P.artisanWool, r.range(0.85, 1.15));
    for (let i = 0; i < rows; i++) {
      const w = 0.58 - i * (0.3 / rows) + r.range(-0.02, 0.02);
      add(chain(head, translate(r.range(-0.01, 0.01), 0.24 + i * 0.07, 0.01), scale(w, 0.075, w)), i % 2 ? scaleRgb(wool, 0.85) : wool);
    }
    const pom = r.range(0.12, 0.18);
    add(chain(head, translate(0, 0.24 + rows * 0.07 + pom * 0.3, 0.01), scale(pom, pom, pom)), p.rgb); // pom-pom in the personal colour
  },
};

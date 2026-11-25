// The crosshair's look (Options -> Crosshair, the cl_crosshair* cvars) and its
// share code. Pure (the component is hud.js Crosshair); the
// dynamic gap itself (where the next shot can land) is game/crosshair.js,
// reaching the CSS as --gap on the root (main.js drawCrosshair).
//
// Style dynamic (the default, as before): the lines' inner tips sit at the
// shot cone's edge, plus cl_crosshairgap. Static: they stay at MIN_GAP plus
// cl_crosshairgap whatever the spread. The defaults draw exactly the old
// crosshair: 9 px lines, 2 px thick, slop green, a thin dark outline.
import { MIN_GAP } from "../game/crosshair.js";

export const XHAIR_CVARS = {
  cl_crosshairstyle: { def: "dynamic", enum: ["dynamic", "static"], help: "crosshair: dynamic = the gap opens with your spread (where the next shot can land), static = a fixed gap" },
  cl_crosshairsize: { def: 9, min: 0, max: 20, step: 0.5, help: "crosshair line length (px)" },
  cl_crosshairgap: { def: 0, min: -10, max: 10, step: 0.5, help: "crosshair gap added to the dynamic (or the static 3 px) gap (px; negative closes it)" },
  cl_crosshairthickness: { def: 2, min: 0.5, max: 8, step: 0.5, help: "crosshair line thickness (px)" },
  cl_crosshairdot: { def: 0, bool: true, help: "a dot in the crosshair's centre" },
  cl_crosshair_t: { def: 0, bool: true, help: "T crosshair: no top line" },
  cl_crosshair_drawoutline: { def: 1, bool: true, help: "a thin dark outline around the crosshair lines" },
  cl_crosshaircolor: { def: "#8bff4f", color: true, help: "crosshair colour, #rrggbb" },
  cl_crosshairalpha: { def: 255, min: 0, max: 255, int: true, help: "crosshair opacity, 0..255" },
};
export const XHAIR_KEYS = Object.keys(XHAIR_CVARS);
export const XHAIR_DEFAULTS = Object.fromEntries(XHAIR_KEYS.map((k) => [k, XHAIR_CVARS[k].def]));

// The CSS custom properties for the .crosshair element (style.css draws the lines from them).
export function xhairVars(v) {
  const th = v.cl_crosshairthickness, g = v.cl_crosshairgap;
  const gap = v.cl_crosshairstyle === "static" ? `${Math.max(0, MIN_GAP + g)}px`
    : g ? `max(0px, calc(var(--gap) + ${g}px))` : "var(--gap)";
  return {
    "--xh-g": gap,
    "--xh-len": `${v.cl_crosshairsize}px`,
    "--xh-th": `${th}px`,
    "--xh-c": v.cl_crosshaircolor,
    "--xh-o": v.cl_crosshair_drawoutline ? "0 0 0 1px #000a" : "none",
    opacity: String(Math.round((v.cl_crosshairalpha / 255) * 1000) / 1000),
  };
}

// The share code: SLOP- and twelve base64 characters (in groups of four), nine bytes: flags
// (static, dot, T, outline), size, gap, thickness in half pixels, r, g, b,
// alpha, and a check byte so a mistyped code is refused rather than half applied.
const B64 = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"; // no "-": it separates the groups
const check = (b) => (b.reduce((a, x, i) => (a * 31 + x + i) & 0xff, 7) ^ 0x5a);
export function xhairCode(v) {
  const c = /^#([0-9a-f]{6})$/i.exec(v.cl_crosshaircolor)?.[1] || "8bff4f";
  const b = [
    (v.cl_crosshairstyle === "static" ? 1 : 0) | (v.cl_crosshairdot ? 2 : 0) | (v.cl_crosshair_t ? 4 : 0) | (v.cl_crosshair_drawoutline ? 8 : 0),
    Math.round(v.cl_crosshairsize * 2), Math.round((v.cl_crosshairgap + 10) * 2), Math.round(v.cl_crosshairthickness * 2),
    parseInt(c.slice(0, 2), 16), parseInt(c.slice(2, 4), 16), parseInt(c.slice(4, 6), 16), Math.round(v.cl_crosshairalpha),
  ];
  b.push(check(b));
  let s = "";
  for (let i = 0; i < 9; i += 3) {
    const n = (b[i] << 16) | (b[i + 1] << 8) | b[i + 2];
    s += B64[(n >> 18) & 63] + B64[(n >> 12) & 63] + B64[(n >> 6) & 63] + B64[n & 63];
  }
  return `SLOP-${s.slice(0, 4)}-${s.slice(4, 8)}-${s.slice(8)}`;
}

// A code back to the cvars it sets, or null when it isn't one (any case of
// the prefix, dashes and spaces optional, out-of-range values refused).
export function parseXhairCode(code) {
  const s = String(code || "").trim().replace(/^slop-?/i, "").replace(/[-\s]/g, "");
  if (s.length !== 12) return null;
  const b = [];
  for (let i = 0; i < 12; i += 4) {
    let n = 0;
    for (let j = 0; j < 4; j++) {
      const d = B64.indexOf(s[i + j]);
      if (d < 0) return null;
      n = (n << 6) | d;
    }
    b.push((n >> 16) & 255, (n >> 8) & 255, n & 255);
  }
  if (check(b.slice(0, 8)) !== b[8] || b[0] > 15) return null;
  const v = {
    cl_crosshairstyle: b[0] & 1 ? "static" : "dynamic", cl_crosshairdot: b[0] & 2 ? 1 : 0,
    cl_crosshair_t: b[0] & 4 ? 1 : 0, cl_crosshair_drawoutline: b[0] & 8 ? 1 : 0,
    cl_crosshairsize: b[1] / 2, cl_crosshairgap: b[2] / 2 - 10, cl_crosshairthickness: b[3] / 2,
    cl_crosshaircolor: "#" + [b[4], b[5], b[6]].map((x) => x.toString(16).padStart(2, "0")).join(""), cl_crosshairalpha: b[7],
  };
  for (const k of ["cl_crosshairsize", "cl_crosshairgap", "cl_crosshairthickness"]) {
    const c = XHAIR_CVARS[k];
    if (v[k] < c.min || v[k] > c.max) return null;
  }
  return v;
}

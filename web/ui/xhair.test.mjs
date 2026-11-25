import { test } from "node:test";
import assert from "node:assert/strict";
import { XHAIR_DEFAULTS, xhairVars, xhairCode, parseXhairCode } from "./xhair.js";
import { coerce } from "./settings.js";

test("the default crosshair is the old one: 9 px lines, 2 px thick, slop green, outlined, the dynamic gap", () => {
  const v = xhairVars(XHAIR_DEFAULTS);
  assert.deepEqual(v, { "--xh-g": "var(--gap)", "--xh-len": "9px", "--xh-th": "2px", "--xh-c": "#8bff4f", "--xh-o": "0 0 0 1px #000a", opacity: "1" });
});

test("static keeps a fixed gap (3 px + cl_crosshairgap, never negative); dynamic adds the gap to the spread's", () => {
  assert.equal(xhairVars({ ...XHAIR_DEFAULTS, cl_crosshairstyle: "static" })["--xh-g"], "3px");
  assert.equal(xhairVars({ ...XHAIR_DEFAULTS, cl_crosshairstyle: "static", cl_crosshairgap: 2.5 })["--xh-g"], "5.5px");
  assert.equal(xhairVars({ ...XHAIR_DEFAULTS, cl_crosshairstyle: "static", cl_crosshairgap: -8 })["--xh-g"], "0px");
  assert.equal(xhairVars({ ...XHAIR_DEFAULTS, cl_crosshairgap: -2 })["--xh-g"], "max(0px, calc(var(--gap) + -2px))");
  assert.equal(xhairVars({ ...XHAIR_DEFAULTS, cl_crosshairalpha: 128 }).opacity, "0.502");
  assert.equal(xhairVars({ ...XHAIR_DEFAULTS, cl_crosshair_drawoutline: 0 })["--xh-o"], "none");
});

test("share codes round-trip every setting", () => {
  const looks = [
    XHAIR_DEFAULTS,
    { cl_crosshairstyle: "static", cl_crosshairsize: 3.5, cl_crosshairgap: -1.5, cl_crosshairthickness: 1, cl_crosshairdot: 1, cl_crosshair_t: 1, cl_crosshair_drawoutline: 0, cl_crosshaircolor: "#00ffff", cl_crosshairalpha: 200 },
    { cl_crosshairstyle: "dynamic", cl_crosshairsize: 20, cl_crosshairgap: 10, cl_crosshairthickness: 8, cl_crosshairdot: 0, cl_crosshair_t: 1, cl_crosshair_drawoutline: 1, cl_crosshaircolor: "#ff0000", cl_crosshairalpha: 0 },
    { cl_crosshairstyle: "static", cl_crosshairsize: 0, cl_crosshairgap: -10, cl_crosshairthickness: 0.5, cl_crosshairdot: 1, cl_crosshair_t: 0, cl_crosshair_drawoutline: 1, cl_crosshaircolor: "#123abc", cl_crosshairalpha: 255 },
  ];
  for (const v of looks) {
    const code = xhairCode(v);
    assert.match(code, /^SLOP-[\w+/]{4}-[\w+/]{4}-[\w+/]{4}$/);
    assert.deepEqual(parseXhairCode(code), v, code);
    assert.deepEqual(parseXhairCode(code.toLowerCase().replace("slop-", "slop ")), null, "base64 is case-sensitive: a lowercased code is refused, not misread");
    assert.deepEqual(parseXhairCode(`  ${code.replace(/-/g, "")} `.replace("SLOP", "slop")), v, "dashes, spaces and the prefix's case don't matter");
  }
  assert.equal(xhairCode(XHAIR_DEFAULTS), xhairCode({ ...XHAIR_DEFAULTS }), "stable");
});

test("a mistyped or foreign code is refused, not half applied", () => {
  const code = xhairCode(XHAIR_DEFAULTS);
  const flip = (i) => code.slice(0, i) + (code[i] === "A" ? "B" : "A") + code.slice(i + 1);
  for (const i of [5, 6, 10, 12, 16, 17]) assert.equal(parseXhairCode(flip(i)), null, flip(i));
  for (const bad of ["", "SLOP-", "hello", "SLOP-AAAA-AAAA-AAA", "CSGO-AAAAA-AAAAA-AAAAA-AAAAA-AAAAA", null, undefined]) assert.equal(parseXhairCode(bad), null, String(bad));
});

test("the cvars coerce: colours to #rrggbb, sizes to half pixels within range", () => {
  assert.equal(coerce("cl_crosshaircolor", "FF00aa"), "#ff00aa");
  assert.equal(coerce("cl_crosshaircolor", "#0f0"), "#00ff00");
  assert.equal(coerce("cl_crosshaircolor", "green"), undefined);
  assert.equal(coerce("cl_crosshairsize", "3.3"), 3.5);
  assert.equal(coerce("cl_crosshairsize", "99"), 20);
  assert.equal(coerce("cl_crosshairgap", "-2.2"), -2);
  assert.equal(coerce("cl_crosshairalpha", "128.4"), 128);
  assert.equal(coerce("cl_crosshairstyle", "classic"), undefined);
});

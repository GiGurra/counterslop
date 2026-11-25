import { DefaultHook } from "../engine/sim.gen.js";
import { test } from "node:test";
import assert from "node:assert/strict";
import { effectiveCvars, moveRules } from "./rules.js";
import { MoveClassic, MoveSource } from "../engine/sim.gen.js";

test("the map's rules pin cvars over the server's", () => {
  const cv = effectiveCvars({ sv_movement: "classic", sv_airaccelerate: "12", mp_c4timer: "40" }, { rules: { sv_movement: "source" } });
  assert.equal(cv.sv_movement, "source");
  assert.equal(cv.sv_airaccelerate, "12");
  assert.equal(cv.mp_c4timer, "40");
  assert.deepEqual(effectiveCvars({ a: "1" }, null), { a: "1" });
  assert.deepEqual(effectiveCvars({ a: "1" }, { boxes: [] }), { a: "1" });
});

test("moveRules = Server.moveRules", () => {
  const hook = DefaultHook(); // sv_hook_* unset: the server's defaults
  assert.deepEqual(moveRules({}), { model: MoveClassic, airAccel: 0, autoBhop: false, hook });
  assert.deepEqual(moveRules({ sv_movement: "source", sv_airaccelerate: "150", sv_autobunnyhopping: "1" }), { model: MoveSource, airAccel: 150, autoBhop: true, hook });
  // cvarF: not a number or negative = the default (0: the sim's DefaultAirAccel; no bhop)
  assert.deepEqual(moveRules({ sv_movement: "bogus", sv_airaccelerate: "-3", sv_autobunnyhopping: "x" }), { model: MoveClassic, airAccel: 0, autoBhop: false, hook });
  // the hook's cvars (a map may pin them)
  assert.deepEqual(moveRules({ sv_hook_range: "3", sv_hook_energy: "50", sv_hook_recharge: "0", sv_hook_drain: "1", sv_hook_cost: "2", sv_hook_pull: "9", sv_hook_bogus: "1" }).hook,
    { range: 3, energy: 50, recharge: 0, drain: 1, cost: 2, pull: 9 });
  assert.equal(moveRules({ sv_hook_range: "-4" }).hook.range, hook.range);
  assert.equal(moveRules({ sv_airaccelerate: "5000" }).airAccel, 1000); // clamped like the rcon value
  assert.equal(moveRules({ sv_autobunnyhopping: "1.0" }).autoBhop, true);
});

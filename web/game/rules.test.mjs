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
  assert.deepEqual(moveRules({}), { model: MoveClassic, airAccel: 0, autoBhop: false });
  assert.deepEqual(moveRules({ sv_movement: "source", sv_airaccelerate: "150", sv_autobunnyhopping: "1" }), { model: MoveSource, airAccel: 150, autoBhop: true });
  // cvarF: not a number or negative = the default (0: the sim's DefaultAirAccel; no bhop)
  assert.deepEqual(moveRules({ sv_movement: "bogus", sv_airaccelerate: "-3", sv_autobunnyhopping: "x" }), { model: MoveClassic, airAccel: 0, autoBhop: false });
  assert.equal(moveRules({ sv_airaccelerate: "5000" }).airAccel, 1000); // clamped like the rcon value
  assert.equal(moveRules({ sv_autobunnyhopping: "1.0" }).autoBhop, true);
});

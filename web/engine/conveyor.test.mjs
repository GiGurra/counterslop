import test from "node:test";
import assert from "node:assert/strict";
import { conveyorSteps, phase, treads, insideConveyor } from "./conveyor.js";

// gs_megabyte's west up lane: 20 treads rising 5 m towards +z over z -5..4.
const up = { min: [-21, 0, -5], max: [-19.8, 5, 4], vel: [0, 0, 0.75], axis: 2, up: 1, rise: 0.25, run: 0.45, kind: "escalator" };
const down = { ...up, vel: [0, 0, -0.75] };
// An x-axis flight rising towards -x (lab_store's shape), moving down.
const xflight = { min: [6.2, 0, 2.3], max: [8, 1, 3.3], vel: [0.75, 0, 0], axis: 0, up: -1, rise: 0.25, run: 0.45, kind: "escalator" };
const near = (a, b, eps = 1e-9) => Math.abs(a - b) < eps;

test("the phase moves towards the top on an up conveyor, towards the foot on a down one", () => {
  assert.equal(treads(up), 20);
  assert.ok(near(phase(up, 0.3), 0.5)); // 0.225 m of a 0.45 m tread
  assert.ok(near(phase(down, 0.3), 0.5));
  assert.ok(near(phase(up, 0.15), 0.25) && near(phase(down, 0.15), 0.75));
});

test("at phase 0 the steps are the static treads", () => {
  const s = conveyorSteps(up, 0);
  assert.equal(s.length, 20);
  s.forEach((b, j) => {
    assert.ok(near(b.min[2], -5 + j * 0.45) && near(b.max[2], -5 + (j + 1) * 0.45), `step ${j} z ${b.min[2]}..${b.max[2]}`);
    assert.ok(near(b.max[1], (j + 1) * 0.25), `step ${j} top ${b.max[1]}`);
    assert.ok(b.min[0] > up.min[0] && b.max[0] < up.max[0], "inside the balustrades");
    assert.equal(b.kind, "escalator");
  });
});

test("mid-cycle a step rises out of the floor at the foot and flattens under the comb at the head", () => {
  const s = conveyorSteps(up, 0.3); // phase 0.5
  assert.equal(s.length, 21);
  const foot = s[0], head = s[s.length - 1];
  assert.ok(near(foot.min[2], -5) && near(foot.max[2], -5 + 0.225) && near(foot.max[1], 0.125), JSON.stringify(foot));
  assert.ok(near(head.min[2], 4 - 0.225) && near(head.max[2], 4) && near(head.max[1], 5), JSON.stringify(head));
  // No gaps: consecutive steps meet, and each riser reaches below the step before.
  for (let i = 1; i < s.length; i++) {
    assert.ok(near(s[i].min[2], s[i - 1].max[2]), `gap before step ${i}`);
    assert.ok(s[i].min[1] < s[i - 1].max[1], `riser of step ${i} doesn't reach the step before`);
  }
});

test("a step travels with the belt: up the up lane, down the down lane", () => {
  const at = (cv, t) => conveyorSteps(cv, t)[5];
  const a = at(up, 0.1), b = at(up, 0.4);
  assert.ok(near(b.min[2] - a.min[2], 0.225) && near(b.max[1] - a.max[1], 0.125));
  const c = at(down, 0.1), d = at(down, 0.4);
  assert.ok(near(d.min[2] - c.min[2], -0.225) && near(d.max[1] - c.max[1], -0.125));
});

test("x flights rising towards -x", () => {
  const s = conveyorSteps(xflight, 0);
  assert.equal(s.length, treads(xflight));
  assert.ok(near(s[0].max[0], 8) && near(s[0].max[1], 0.25), "the first tread at the +x foot");
  assert.ok(near(s[s.length - 1].min[0], 6.2) && near(s[s.length - 1].max[1], 1), "the top tread at -x");
  assert.ok(s.every((b) => near(b.min[2], 2.305) && near(b.max[2], 3.295)));
});

test("static treads inside a conveyor are replaced, balustrades and floors aren't", () => {
  assert.ok(insideConveyor({ min: [-21, 0, -5], max: [-19.8, 0.25, 4] }, [up]));
  assert.ok(insideConveyor({ min: [-21, 4.75, 3.55], max: [-19.8, 5, 4] }, [up]));
  assert.ok(!insideConveyor({ min: [-21.15, 0, -5], max: [-21, 1.19, -4.55] }, [up]));
  assert.ok(!insideConveyor({ min: [-30, -0.4, -30], max: [30, 0, 30] }, [up]));
  assert.ok(!insideConveyor({ min: [0, 0, 0], max: [1, 1, 1] }, undefined));
});

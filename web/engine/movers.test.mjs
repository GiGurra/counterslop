import test from "node:test";
import assert from "node:assert/strict";
import { drawnMoverBoxes, moverBoxes, replacedByMover } from "./movers.js";

// gs_megabyte's west up lane: 20 treads rising 5 m towards +z over z -5..4.
const up = { kind: "escalator", mat: "escalator", min: [-21, 0, -5], max: [-19.8, 5, 4], axis: 2, up: 1, rise: 0.25, run: 0.45, speed: 0.75, flat: 2, move: [0, 0, 0], period: 0, phase: 0 };
const down = { ...up, speed: -0.75 };
// An x-axis flight rising towards -x (lab_store's shape).
const xflight = { ...up, min: [6.2, 0, 2.3], max: [8, 1, 3.3], axis: 0, up: -1, flat: 1 };
const slide = { kind: "slide", mat: "metal", min: [0, 0, 0], max: [1, 1, 1], axis: 0, up: 0, rise: 0, run: 0, speed: 0, flat: 0, move: [2, 0, 0], period: 4, phase: 0 };
const near = (a, b, eps = 1e-9) => Math.abs(a - b) < eps;
// The up lane's profile: 2 treads level at each end, 16 steps of 5/16 m between.
const STEP = 5 / 16, topOf = (u) => STEP * Math.max(0, Math.min(u + 1 - 2, 16));

test("at time 0 the treads stand where the static ones do: level landings, equal steps", () => {
  const s = moverBoxes([up], 0);
  assert.equal(s.length, 20);
  s.forEach((b, j) => {
    assert.ok(near(b.min[2], -5 + j * 0.45) && near(b.max[2], -5 + (j + 1) * 0.45), `tread ${j} z ${b.min[2]}..${b.max[2]}`);
    assert.ok(near(b.max[1], topOf(j)), `tread ${j} top ${b.max[1]}`);
    assert.ok(b.min[0] > up.min[0] && b.max[0] < up.max[0], "inside the balustrades");
    assert.equal(b.kind, "escalator");
  });
});

test("mid-cycle a tread comes out of the floor level at the foot and slides under the comb level at the head", () => {
  const s = moverBoxes([up], 0.3); // half a tread on
  assert.equal(s.length, 21);
  const foot = s[0], head = s[s.length - 1];
  assert.ok(near(foot.min[2], -5) && near(foot.max[2], -5 + 0.225) && near(foot.max[1], 0), JSON.stringify(foot));
  assert.ok(near(head.min[2], 4 - 0.225) && near(head.max[2], 4) && near(head.max[1], 5), JSON.stringify(head));
  for (let i = 1; i < s.length; i++) {
    assert.ok(near(s[i].min[2], s[i - 1].max[2]), `gap before tread ${i}`);
    assert.ok(s[i].min[1] < s[i - 1].max[1], `riser of tread ${i} doesn't reach the tread before`);
  }
});

test("a tread travels up the up lane and down the down lane", () => {
  const at = (m, t) => moverBoxes([m], t)[5];
  const a = at(up, 0.1), b = at(up, 0.4);
  assert.ok(near(b.min[2] - a.min[2], 0.225) && near(b.max[1] - a.max[1], STEP / 2));
  const c = at(down, 0.1), d = at(down, 0.4);
  assert.ok(near(d.min[2] - c.min[2], -0.225) && near(d.max[1] - c.max[1], -STEP / 2));
});

test("x flights rising towards -x", () => {
  const s = moverBoxes([xflight], 0);
  assert.equal(s.length, 4);
  assert.ok(near(s[0].max[0], 8) && near(s[0].max[1], 0), "the first tread at the +x foot, level with the floor");
  assert.ok(near(s[1].max[1], 0.5) && near(s[2].max[1], 1), "one landing tread, then two steps of 0.5");
  assert.ok(near(s[3].min[0], 6.2) && near(s[3].max[1], 1), "the top tread at -x");
});

test("a slide goes out and back once per period", () => {
  assert.ok(near(moverBoxes([slide], 0)[0].min[0], 0));
  assert.ok(near(moverBoxes([slide], 1)[0].min[0], 1));
  assert.ok(near(moverBoxes([slide], 2)[0].min[0], 2));
  assert.ok(near(moverBoxes([slide], 3)[0].max[0], 2));
});

test("static treads inside an escalator are replaced, balustrades and floors aren't", () => {
  const t = (min, max, kind = "escalator") => ({ min, max, kind });
  assert.ok(replacedByMover(t([-21, 0, -5], [-19.8, 0.25, 4]), [up]));
  assert.ok(!replacedByMover(t([-21.15, 0, -5], [-21, 1.19, -4.55]), [up]));
  assert.ok(!replacedByMover(t([-21, 0, -5], [-19.8, 0.25, 4], "glass"), [up]));
  assert.ok(!replacedByMover(t([0, 0, 0], [1, 1, 1]), undefined));
});

test("drawn treads sit a centimetre low and none run level with the floor (no z-fighting at the landings)", () => {
  for (const t of [0, 0.1, 0.3, 0.55]) {
    const col = moverBoxes([up], t), drawn = drawnMoverBoxes([up], t);
    const kept = col.filter((b) => b.max[1] - 0.01 >= up.min[1] + 0.01);
    assert.equal(drawn.length, kept.length);
    drawn.forEach((b, j) => {
      assert.ok(near(b.max[1], kept[j].max[1] - 0.01), `tread ${j} top ${b.max[1]}`);
      assert.ok(b.max[1] > up.min[1] + 0.005, `tread ${j} clears the floor`);
    });
  }
});

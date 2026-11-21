// The client's weapon predictor (arsenal.js) against the shared golden vectors
// that internal/game/weapons_golden_test.go checks the server's rules against.
// Same command stream in, same shots and ammo out, or one side has drifted.
import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { createArsenal, ticks, shotAngles } from "./arsenal.js";

const WEAPONS = JSON.parse(readFileSync(new URL("../assets/weapons/weapons.json", import.meta.url))).weapons;
const reloadOf = (id) => WEAPONS.find((w) => w.id === id).reload;
const golden = JSON.parse(readFileSync(new URL("../../internal/game/testdata/weapons_golden.json", import.meta.url)));
const still = { vel: [0, 0, 0], onGround: true, crouched: false, pitch: 0, yaw: 0 };

function arsenal() {
  const hud = { last: null, set(p) { this.last = { ...this.last, ...p }; } };
  const sent = [];
  const a = createArsenal({ net: { send: (m) => sent.push(m) }, hud });
  return { a, hud, sent };
}

// Replay one golden case through arsenal.tick on the command clock.
function replay(gc) {
  const { a } = arsenal();
  a.setInv({ cur: "primary", slots: structuredClone(gc.slots) });
  let trace = "";
  const obs = [];
  for (const c of gc.cmds) {
    for (let i = 0; i < c.n; i++) {
      let shot = false;
      a.takeCmd(); // the real client sends these; the vectors spell commands out
      a.tick(0.01, !!c.fire, still, () => { shot = true; }, false, { w: i === 0 ? c.w || null : null, reload: i === 0 && !!c.reload });
      trace += shot ? "*" : ".";
    }
    const h = a.slots[a.cur];
    obs.push({ tick: a.tickN, cur: a.cur, ammo: h?.ammo ?? 0, reserve: h?.reserve ?? 0, reloading: !!a.reloadEnd });
  }
  return { trace, obs };
}

for (const gc of golden.cases) {
  test(`golden: ${gc.name}`, () => {
    const got = replay(gc);
    const at = [...got.trace].findIndex((ch, i) => ch !== gc.trace[i]);
    assert.equal(got.trace, gc.trace, `shot trace differs from the server's at tick ${at + 1}`);
    assert.deepEqual(got.obs, gc.obs);
  });
}

test("ticks rounds seconds up to whole command ticks (= Go Ticks)", () => {
  assert.equal(ticks(0.1), 10);
  assert.equal(ticks(0.09), 9);
  assert.equal(ticks(0.16), 16);
  assert.equal(ticks(1.45), 145);
  assert.equal(ticks(0.001), 1);
});

test("hud shows ammo, the weapon, slots and the reload state", () => {
  const { a, hud } = arsenal();
  a.setInv({ cur: "primary", slots: { primary: { id: "prompt", ammo: 2, reserve: 5 }, melee: { id: "keyboard", ammo: 0, reserve: 0 } } });
  assert.equal(hud.last.ammo, 2);
  assert.equal(hud.last.reserve, 5);
  assert.equal(hud.last.weapon.length > 0, true);
  assert.deepEqual(hud.last.slots.map((s) => s.cur), [true, false, false]);
  a.tick(0.01, false, still, () => {}, false, { reload: true });
  assert.equal(hud.last.reloading, true, "reloading shows while the reload runs");
  for (let i = 0; i < ticks(reloadOf("prompt")); i++) a.tick(0.01, false, still, () => {});
  assert.equal(hud.last.reloading, false, "and is gone once it's done");
  assert.equal(hud.last.ammo, 7);
  assert.equal(hud.last.reserve, 0);
  a.setInv({ cur: "melee", slots: a.slots });
  assert.equal(hud.last.ammo, "∞", "melee has no ammo count");
});

test("dry fire asks for a reload in the next command, once per press", () => {
  const { a } = arsenal();
  a.setInv({ cur: "primary", slots: { primary: { id: "prompt", ammo: 0, reserve: 30 } } });
  for (let i = 0; i < 30; i++) a.tick(0.01, true, still, () => {});
  assert.deepEqual(a.takeCmd(), { w: null, reload: true });
  a.tick(0.01, true, still, () => {});
  assert.deepEqual(a.takeCmd(), { w: null, reload: false }, "holding the trigger doesn't ask again");
});

test("switch requests go out in one command and cycle skips empty slots", () => {
  const { a } = arsenal();
  a.setInv({ cur: "primary", slots: { primary: { id: "prompt", ammo: 30, reserve: 90 }, melee: { id: "keyboard", ammo: 0, reserve: 0 } } });
  a.switchTo("sidearm"); // not carried
  assert.deepEqual(a.takeCmd(), { w: null, reload: false });
  a.cycle(1);
  assert.deepEqual(a.takeCmd(), { w: "melee", reload: false });
  a.tick(0.01, false, still, () => {}, false, { w: "melee" });
  assert.equal(a.cur, "melee");
  a.lastWeapon();
  assert.deepEqual(a.takeCmd(), { w: "primary", reload: false });
});

test("drop sends a drop message, never for melee", () => {
  const { a, sent } = arsenal();
  a.setInv({ cur: "melee", slots: { primary: { id: "prompt", ammo: 30, reserve: 90 }, melee: { id: "keyboard", ammo: 0, reserve: 0 } } });
  a.drop();
  assert.equal(sent.length, 0);
  a.setInv({ cur: "primary", slots: a.slots });
  a.drop();
  assert.deepEqual(sent, [{ t: "drop" }]);
});

test("bolt scope: unscoped while cycling, re-zooms when the bolt is done (toggle mode)", () => {
  const { a } = arsenal();
  a.setInv({ cur: "primary", slots: { primary: { id: "codereview", ammo: 5, reserve: 20 } } });
  for (let i = 0; i < 40; i++) a.tick(0.01, false, still, () => {}); // raise + deploy
  a.toggleScope();
  assert.equal(a.scoped, true);
  a.tick(0.01, true, still, () => {});
  assert.equal(a.scoped, false, "the shot unscopes");
  for (let i = 0; i < ticks(1.1); i++) a.tick(0.01, false, still, () => {});
  assert.equal(a.scoped, true, "re-zoomed after the cycle");
});

test("spread grows with movement and air, shrinks crouched, noSpread zeroes it", () => {
  const { a } = arsenal();
  a.setInv({ cur: "primary", slots: { primary: { id: "prompt", ammo: 30, reserve: 90 } } });
  const base = a.spread(still);
  assert.ok(a.spread({ ...still, vel: [5, 0, 0] }) > base);
  assert.ok(a.spread({ ...still, onGround: false }) > base);
  assert.ok(a.spread({ ...still, crouched: true }) < base);
  a.noSpread = true;
  assert.equal(a.spread({ ...still, vel: [5, 0, 0] }), 0);
});

test("shot angles (spread and shotgun pellets) match the server's golden vectors", () => {
  const cases = JSON.parse(readFileSync(new URL("../../internal/game/testdata/shot_angles_golden.json", import.meta.url)));
  for (const c of cases) {
    const got = shotAngles(c.Yaw, c.Pitch, c.Spread, c.Seed, c.Pellets);
    assert.equal(got.length, c.Out.length);
    got.forEach(([y, p], i) => {
      assert.ok(Math.abs(y - c.Out[i][0]) < 1e-12 && Math.abs(p - c.Out[i][1]) < 1e-12, `pellet ${i}: ${y},${p} vs ${c.Out[i]}`);
    });
  }
});

test("belt spread: the heavy gets less accurate as the belt empties", () => {
  const { a } = arsenal();
  a.setInv({ cur: "primary", slots: { primary: { id: "contextwindow", ammo: 150, reserve: 200 } } });
  const full = a.spread(still);
  a.slots.primary.ammo = 15;
  assert.ok(a.spread(still) > full + 0.02);
});

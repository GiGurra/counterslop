// The client's weapon predictor (arsenal.js) against the shared golden vectors
// that internal/game/weapons_golden_test.go checks the server's rules against.
// Same command stream in, same shots and ammo out, or one side has drifted.
import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { createArsenal, ticks, shotAngles, recoilOffset, recoilIndex, recoilAngles } from "./arsenal.js";

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
  const obs = [], kicks = [];
  for (const c of gc.cmds) {
    for (let i = 0; i < c.n; i++) {
      let shot = false;
      a.takeCmd(); // the real client sends these; the vectors spell commands out
      a.tick(0.01, !!c.fire, still, (w) => { shot = true; kicks.push([a.shotIdx, ...recoilOffset(w.recoil?.pattern, a.shotIdx)]); }, false, { w: i === 0 ? c.w || null : null, reload: i === 0 && !!c.reload });
      trace += shot ? "*" : ".";
    }
    const h = a.slots[a.cur];
    obs.push({ tick: a.tickN, cur: a.cur, ammo: h?.ammo ?? 0, reserve: h?.reserve ?? 0, reloading: !!a.reloadEnd });
  }
  return { trace, obs, kicks };
}

for (const gc of golden.cases) {
  test(`golden: ${gc.name}`, () => {
    const got = replay(gc);
    const at = [...got.trace].findIndex((ch, i) => ch !== gc.trace[i]);
    assert.equal(got.trace, gc.trace, `shot trace differs from the server's at tick ${at + 1}`);
    assert.deepEqual(got.obs, gc.obs);
    assert.deepEqual(got.kicks, gc.kicks || [], "spray kicks (index, up, right) differ from the server's");
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

test("drop with the knife in hand: only while carrying a GPU (GPU Drop)", () => {
  const { a, sent } = arsenal();
  a.setInv({ cur: "melee", slots: { melee: { id: "keyboard", ammo: 0, reserve: 0 } } });
  a.drop();
  assert.equal(sent.length, 0);
  a.setWstate({ ...a.wstate(), ca: true }, a.tickN);
  a.drop();
  assert.deepEqual(sent, [{ t: "drop" }]);
});

test("carrying a GPU: moves at the economy's carrySpeed, adopted from the server's ca", () => {
  const { a } = arsenal();
  a.setInv({ cur: "melee", slots: { melee: { id: "keyboard", ammo: 0, reserve: 0 } } });
  const free = a.moveSpeed();
  a.setWstate({ ...a.wstate(), ca: true }, a.tickN);
  assert.equal(a.carry, true);
  assert.ok(Math.abs(a.moveSpeed() - free * 0.8) < 1e-9, `carrying: ${a.moveSpeed()} vs ${free}`);
  assert.equal(a.wstate().ca, true);
  a.setWstate({ ...a.wstate(), ca: false }, a.tickN);
  assert.equal(a.moveSpeed(), free);
  assert.equal(a.wstate().ca, undefined);
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
  // counter-strafe: under 34% of the weapon's max speed (Prompt: 0.34 × 7 × 0.86 = 2.05 m/s) costs nothing
  assert.equal(a.spread({ ...still, vel: [2, 0, 0] }), base);
  const w = WEAPONS.find((x) => x.id === "prompt");
  assert.ok(Math.abs(a.spread({ ...still, vel: [5, 0, 0] }) - (w.spread.base + (5 - 0.34 * 7 * w.speed) * w.spread.move)) < 1e-12, "= Go movePenalty");
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

test("recoil: keyframes interpolate, the last is held, nothing before the first shot", () => {
  const p = [[1, 0.01, 0], [3, 0.03, -0.02]];
  assert.deepEqual(recoilOffset(p, 0), [0, 0]);
  assert.deepEqual(recoilOffset(p, 0.5), [0.005, 0]);
  const mid = recoilOffset(p, 2);
  assert.ok(Math.abs(mid[0] - 0.02) < 1e-12 && Math.abs(mid[1] + 0.01) < 1e-12, String(mid));
  assert.deepEqual(recoilOffset(p, 9), [0.03, -0.02]);
  assert.deepEqual(recoilOffset([], 4), [0, 0]);
  // recovery starts one interval after the shot, at recover per second
  assert.equal(recoilIndex(5, 100, 110, 10, 10), 5);
  assert.equal(recoilIndex(5, 100, 130, 10, 10), 3);
  assert.equal(recoilIndex(5, 100, 900, 10, 10), 0);
  // up raises the pitch, right turns right (yaw down), pitch clamped
  assert.deepEqual(recoilAngles(1, 0.2, [0.1, 0.05], 2), [0.9, 0.4]);
  assert.deepEqual(recoilAngles(0, 1.55, [0.1, 0], 1), [0, 1.6]);
});

test("recoil: the view punch is half the offset, recovers, and a weapon switch starts a fresh spray", () => {
  const { a } = arsenal();
  a.setInv({ cur: "primary", slots: { primary: { id: "prompt", ammo: 30, reserve: 90 }, sidearm: { id: "stub", ammo: 12, reserve: 48 } } });
  const prompt = WEAPONS.find((w) => w.id === "prompt");
  for (let i = 0; i < 40; i++) { a.takeCmd(); a.tick(0.01, i >= 30, still, () => {}, false, {}); } // deploy, then 10 ticks held: one shot
  for (let i = 0; i < 40; i++) { a.takeCmd(); a.tick(0.01, true, still, () => {}, false, {}); } // 4 more
  assert.equal(a.recoil, 5);
  const off = recoilOffset(prompt.recoil.pattern, 5);
  const vp = a.viewPunch(a.tickN, 1);
  assert.ok(Math.abs(vp[0] - off[0] / 2) < 1e-12 && Math.abs(vp[1] - off[1] / 2) < 1e-12, String(vp));
  assert.deepEqual(a.viewPunch(a.tickN, 0), [0, 0]);
  assert.ok(a.viewPunch(a.tickN + 300, 1)[0] === 0, "recovered after 3 s");
  assert.deepEqual(a.wstate().ri, 5);
  a.takeCmd(); a.tick(0.01, false, still, () => {}, false, { w: "sidearm" });
  assert.equal(a.recoil, 0);
  assert.equal(a.wstate().ri, undefined);
});

// The selected slot comes only from the server's rule (internal/game acquired:
// any primary goes to hand, a replacing pistol too unless the primary is in
// hand, nothing else moves it). The client never
// picks a slot itself when the inventory changes: a new item in another slot
// keeps the hand and its timers; the server's slot is adopted as sent.
test("inventory changes: the hand follows the server's cur only, a new item elsewhere doesn't redeploy", () => {
  const { a } = arsenal();
  const stub = { id: "stub", ammo: 12, reserve: 24 }, kb = { id: "keyboard", ammo: 0, reserve: 0 };
  a.setInv({ cur: "melee", slots: { sidearm: stub, melee: kb } });
  for (let i = 0; i < 40; i++) a.tick(0.01, false, still, () => {}); // deployed, ready
  const ready = a.nextShot;
  // new items in other slots, and the server says the knife stays in hand
  a.setInv({ cur: "melee", slots: { sidearm: { id: "hotfix", ammo: 7, reserve: 35 }, melee: kb, he: { id: "mergeconflict", ammo: 1, reserve: 0 } } });
  assert.equal(a.cur, "melee");
  assert.equal(a.nextShot, ready, "no deploy for an item that isn't in hand");
  // the first primary: the server moves the hand, the client follows and draws it
  a.setInv({ cur: "primary", slots: { ...a.slots, primary: { id: "prompt", ammo: 30, reserve: 90 } } });
  assert.equal(a.cur, "primary");
  assert.ok(a.nextShot > a.tickN, "the new primary is drawn (deploy delay)");
});

test("move speed multiplies in the server's order (Go MoveSpeed: drink × carry, then the weapon)", () => {
  const { a } = arsenal();
  a.setInv({ cur: "primary", slots: { primary: { id: "hotfix", ammo: 1, reserve: 0 } } });
  a.boostEnd = a.tickN + 10;
  a.carry = true;
  // The hand-written twin did weapon × drink × carry: 0.8096000000000002, an
  // ulp off the server, so every predicted tick in this state mismatched.
  assert.equal(a.moveSpeed(), 0.8096000000000001);
});

// HUD weapon list: key 4 is one row of every grenade carried, in the order
// pressing 4 cycles them; only the grenade in hand is highlighted (not the row).
test("hud grenade row: one item per grenade in cycle order, only the held one is cur, counts kept", () => {
  const { a, hud } = arsenal();
  const slots = { primary: { id: "prompt", ammo: 30, reserve: 90 }, he: { id: "mergeconflict", ammo: 1, reserve: 0 },
    flash: { id: "stacktrace", ammo: 2, reserve: 0 }, smoke: { id: "smoketest", ammo: 1, reserve: 0 } };
  a.setInv({ cur: "flash", slots: structuredClone(slots) });
  const row = () => hud.last.slots.find((r) => r.key === 4);
  assert.deepEqual(row().items.map((n) => [n.slot, n.count, n.cur]), [["he", 1, false], ["flash", 2, true], ["smoke", 1, false]]);
  assert.equal(row().name, "Merge Conflict, Stack Trace ×2, Smoke Test");
  assert.deepEqual(row().nades, { mergeconflict: 1, stacktrace: 2, smoketest: 1 }, "the buy menu's counts are unchanged");
  // Pressing 4 goes to the next item on the row, left to right, wrapping.
  const order = row().items.map((n) => n.slot);
  a.nextGrenade();
  assert.equal(a.takeCmd().w, order[(order.indexOf("flash") + 1) % order.length]);
  a.setInv({ cur: "primary", slots: structuredClone(slots) });
  assert.ok(row().items.every((n) => !n.cur), "a gun in hand: no grenade lit");
});

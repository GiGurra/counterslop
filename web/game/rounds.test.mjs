import { test } from "node:test";
import assert from "node:assert/strict";
import { createRounds } from "./rounds.js";
import { mulberry32 } from "../gen/rng.js";

// A rounds client fed by a jittered (in-order) link: snapshots sent every
// 30 ms carry the server's seconds left at send time and arrive 50-250 ms
// later. Returns every roundLeft the HUD was given.
function shown(seed) {
  const sets = [];
  const hud = { set: (h) => { if ("roundLeft" in h) sets.push(h.roundLeft); }, get: () => ({ screen: "playing" }) };
  const r = createRounds({ hud, sfx: {}, say() {}, announcer: {}, spatial: () => null, feed() {},
    me: () => ({ id: 1, team: "vibe", pos: [0, 0, 0] }), nameOf: () => "", c4time: () => 40 });
  const rnd = mulberry32(seed);
  const end = 90; // the warmup ends 90 s after t = 0
  let arrive = 0;
  const packets = [];
  for (let t = 0; t < 8000; t += 30) {
    arrive = Math.max(arrive, t + 50 + rnd() * 200); // in order: never before the previous one
    packets.push({ at: arrive, round: { ph: "warmup", n: 0, left: end - t / 1000, sc: { vibe: 0, artisan: 0 } } });
  }
  let i = 0;
  for (let now = 0; now < 8000; now += 8) {
    while (i < packets.length && packets[i].at <= now) r.ingest(packets[i].round, packets[i++].at);
    r.update(0.008, now, null);
    if (now % 40 === 0) r.update(0.008, now - 6, null); // an input-driven frame stamped a little earlier
  }
  return sets;
}

// Jitter must not make the countdown tick back up (1:24 -> 1:23 -> 1:24): the
// timer's width changes with the digits and the centred top bar (team names,
// WARMUP) jumps left and right (operator report).
test("the round timer never counts back up under jitter", () => {
  for (const seed of [1, 2, 3, 4, 5]) {
    const s = shown(seed);
    const ups = s.filter((v, i) => i > 0 && v > s[i - 1]);
    assert.equal(ups.length, 0, `seed ${seed}: went back up ${ups.length} times: ${s.join(" ")}`);
    assert.ok(s.length >= 6 && s[0] - s.at(-1) >= 6, `seed ${seed}: counted down ${s.join(" ")}`);
  }
});

// No warmup timer (left 0): a frame stamped a few ms before the snapshot
// arrived (an input-driven frame, e.g. switching weapons) showed "WARMUP 0:01"
// for one frame (operator report).
test("no timer stays no timer, even for a frame stamped before the snapshot", () => {
  const sets = [];
  const hud = { set: (h) => { if ("roundLeft" in h) sets.push(h.roundLeft); }, get: () => ({ screen: "playing" }) };
  const r = createRounds({ hud, sfx: {}, say() {}, announcer: {}, spatial: () => null, feed() {},
    me: () => ({ id: 1, team: "vibe", pos: [0, 0, 0] }), nameOf: () => "", c4time: () => 40 });
  r.ingest({ ph: "warmup", n: 0, left: 0, sc: { vibe: 0, artisan: 0 } }, 1000);
  for (const now of [1001, 993, 1016, 1009]) r.update(0.008, now, null);
  assert.deepEqual([...new Set(sets)], [0]);
});

// A rounds client with recording stubs: feed lines, HUD sets, sounds, lines said, music calls.
function rig(meState = {}) {
  const log = { feed: [], hud: [], sfx: [], say: [], music: [], ann: [] };
  const hudState = { screen: "playing" };
  const rec = (k) => new Proxy({}, { get: (_, name) => (...a) => { log[k].push(name); return false; } });
  const me = { id: 1, team: "vibe", pos: [0, 0, 0], dead: false, held: "prompt", ...meState };
  const r = createRounds({
    hud: { set: (h) => { Object.assign(hudState, h); log.hud.push(h); }, get: () => hudState },
    sfx: rec("sfx"), say: (l) => log.say.push(l), announcer: rec("ann"), spatial: (p) => p,
    feed: (t) => log.feed.push(t), me: () => me, nameOf: (id) => `p${id}`, c4time: () => 40, music: rec("music"),
  });
  return { r, log, hud: hudState, me };
}

test("events: start, warmup, halftime, swap, pickup/drop, plant, defuse, explode, round and match end", () => {
  const { r, log, hud } = rig();
  r.event({ ev: "start", n: 3, sc: { vibe: 1, artisan: 1 }, by: 1 }, 0);
  assert.ok(log.ann.includes("onRoundStart") && log.feed.some((l) => /Round 3/.test(l)) && log.feed.some((l) => /You carry/.test(l)));
  r.event({ ev: "warmup" }, 0);
  r.event({ ev: "halftime", sc: { vibe: 6, artisan: 6 } }, 0);
  assert.deepEqual(hud.halftime, { sc: { vibe: 6, artisan: 6 } });
  r.event({ ev: "swap" }, 0);
  r.event({ ev: "pickup", by: 1 }, 0);
  r.event({ ev: "dropped" }, 0);
  r.event({ ev: "planted", by: 2, site: "A", p: [1, 0, 1] }, 0);
  assert.ok(log.sfx.includes("plantClick") && log.say.includes("force_push") && log.music.includes("planted"));
  r.event({ ev: "defused", by: 3 }, 0);
  assert.ok(log.sfx.includes("reverted") && log.feed.some((l) => /p3 reverted/.test(l)));
  r.event({ ev: "exploded", p: [3, 0, 4] }, 100);
  assert.ok(log.sfx.includes("mergeBoom") && hud.flash === 100);
  const eye = r.shakeEye([0, 1.6, 0]);
  assert.ok(eye.some((v, i) => v !== [0, 1.6, 0][i]), "the blast shakes the camera");
  assert.ok(r.solids([], 0, 200, 0).length > 0, "the shockwave is drawn");
  r.event({ ev: "end", win: "vibe", why: "elim", n: 3 }, 0);
  assert.equal(hud.roundEnd.win, "vibe");
  assert.ok(log.sfx.includes("roundEnd"), "no music stinger: the sfx fanfare plays");
  r.event({ ev: "matchend", win: "artisan", sc: { vibe: 5, artisan: 13 } }, 0);
  assert.equal(hud.matchEnd.win, "artisan");
  r.event({ ev: "matchend", win: "draw", sc: { vibe: 12, artisan: 12 } }, 0);
  assert.ok(log.feed.at(-1).includes("draw"));
});

test("the carrier's hints, the plant progress and the CI countdown", () => {
  const { r, hud, me } = rig({ pos: [0, 0, 0] });
  const site = { name: "A", min: [-5, -1, -5], max: [5, 3, 5] };
  r.ingest({ ph: "live", n: 1, left: 90, sc: {}, bomb: { st: "carried", by: 1 } }, 0);
  r.update(0.01, 10, { sites: [site] });
  assert.match(hud.hint, /Site A: press 5/);
  me.held = "bomb";
  r.update(0.01, 20, { sites: [site] });
  assert.match(hud.hint, /hold fire to force-push/);
  me.pos = [50, 0, 50];
  r.update(0.01, 30, { sites: [site] });
  assert.match(hud.hint, /Plant at site A or B/);
  assert.equal(r.solids([{ id: 1, p: [0, 0, 0], yaw: 0 }], 0, 40, 0).length > 0, true, "carried on the back");
  r.ingest({ ph: "live", n: 1, left: 80, sc: {}, bomb: { st: "carried", by: 1, who: 1, pr: 0.5 } }, 1000);
  r.update(0.01, 1010, { sites: [site] });
  assert.deepEqual(hud.work, { kind: "push", pr: 0.5 });
  r.ingest({ ph: "planted", n: 1, left: 40, sc: {}, bomb: { st: "planted", p: [0, 0, 0], site: "A", who: 2, pr: 0.25 } }, 2000);
  r.update(0.01, 2010, { sites: [site] });
  assert.equal(hud.ci.site, "A");
  assert.equal(hud.work.other, "p2");
  assert.ok(r.solids([], 0, 2020, 0).length > 0, "the planted Big Diff is drawn");
  me.team = "artisan"; me.pos = [0.5, 0, 0];
  r.ingest({ ph: "planted", n: 1, left: 39, sc: {}, bomb: { st: "planted", p: [0, 0, 0], site: "A" } }, 3000);
  r.update(0.01, 3010, { sites: [site] });
  assert.equal(hud.hint, "Hold E to git revert");
  assert.equal(r.frozen(), false);
});

test("the MVP rides in the round state", () => {
  const { r, hud } = rig();
  r.ingest({ ph: "over", n: 2, left: 5, sc: {}, mvp: 1, mvpWhy: "most kills" }, 0);
  assert.deepEqual(hud.mvp, { name: "p1", why: "most kills", me: true });
  r.ingest({ ph: "freeze", n: 3, left: 5, sc: {} }, 100);
  assert.equal(hud.mvp, null);
  assert.equal(r.frozen(), true);
});

// The buy badge's seconds: counted down from the snapshot's "buy" (seconds of
// buy time left), never back up under jitter; none in warmup (no limit).
test("buy time left counts down without going back up; null in warmup", () => {
  const sets = [];
  const hud = { set: (h) => { if ("buyLeft" in h) sets.push(h.buyLeft); }, get: () => ({ screen: "playing" }) };
  const r = createRounds({ hud, sfx: {}, say() {}, announcer: {}, spatial: () => null, feed() {},
    me: () => ({ id: 1, team: "vibe", pos: [0, 0, 0] }), nameOf: () => "", c4time: () => 40 });
  r.ingest({ ph: "warmup", n: 0, left: 60, sc: {} }, 0);
  r.update(0.008, 0, null);
  assert.equal(sets.at(-1), null);
  const rnd = mulberry32(7);
  for (let t = 0; t < 6000; t += 30) {
    const at = t + 50 + rnd() * 100; // sent at t, arrives 50-150 ms later
    r.ingest({ ph: "freeze", n: 1, left: 6 - t / 1000, buy: Math.max(0, 5 - t / 1000), sc: {} }, at);
    r.update(0.008, at, null);
  }
  const s = sets.slice(1);
  assert.equal(s[0], 5);
  assert.equal(s.filter((v, i) => i > 0 && v > s[i - 1]).length, 0, s.join(" "));
  r.ingest({ ph: "live", n: 1, left: 100, sc: {} }, 6100); // buy time over: no "buy"
  r.update(0.008, 6100, null);
  assert.equal(sets.at(-1), null);
});

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

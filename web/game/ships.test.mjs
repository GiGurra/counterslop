import { test } from "node:test";
import assert from "node:assert/strict";
import { mapFor } from "./casemap.mjs";
import { createCrew, createScannerGag, createVoiceSpots, departureHud, signValues } from "./ships.js";
import { MoverOffset } from "../engine/sim.gen.js";
import { skyAt } from "../assets/sky.js";
import { LINES } from "../assets/voice/lines.js";

const moon = mapFor("lab_moon");
const dep = moon.departures[0];

test("the departure board: boarding, in flight, aboard, far away", () => {
  const near = [dep.door[0] - 5, 0, 0];
  assert.deepEqual(departureHud(moon, near, 3.2), { name: "MVP-1", state: "boarding", in: 11, aboard: false });
  assert.deepEqual(departureHud(moon, near, 20), { name: "MVP-1", state: "flight", in: 10, aboard: false });
  assert.equal(departureHud(moon, dep.board, 5).aboard, true);
  assert.equal(departureHud(moon, [90, 20, 0], 5), null); // on the Moon: no board
  assert.equal(departureHud(mapFor("de_slop"), [0, 0, 0], 5), null);
});

test("live sign values: each team's backlog, the seconds to MVP-1's doors closing", () => {
  const shot = mapFor("cq_moonshot"), d = shot.departures[0], period = shot.movers[d.mover - 1].period;
  const two = (n) => String(n).padStart(2, "0");
  assert.deepEqual(signValues(shot, 3.2, { tk: { vibe: 287, artisan: 9 } }), { "backlog:vibe": "287", "backlog:artisan": "  9", "first:vibe": "TBD", "first:artisan": "TBD", "departure:MVP-1": two(Math.ceil(d.close - 3.2)), "status:MVP-1": "BOARDING" });
  assert.equal(signValues(shot, d.close + 1, null)["departure:MVP-1"], "GO");
  assert.equal(signValues(shot, d.close + 1, null)["backlog:vibe"], "---");
  assert.equal(signValues(shot, period + 1.5, null)["departure:MVP-1"], two(Math.ceil(d.close - 1.5))); // the next one
});

test("the security scanner beeps as you walk in, not while you stand in it, not twice in a row", () => {
  const shot = mapFor("cq_moonshot"), z = shot.scanners[1], gag = createScannerGag(8);
  const inside = [(z.min[0] + z.max[0]) / 2, 0, (z.min[2] + z.max[2]) / 2], out = [-260, 0, 0];
  assert.equal(gag.enter(shot, out, 0), false);
  assert.equal(gag.enter(shot, inside, 1), true);
  assert.equal(gag.enter(shot, inside, 1.1), false);
  assert.equal(gag.enter(shot, out, 2), false);
  assert.equal(gag.enter(shot, inside, 3), false, "cooling down");
  gag.enter(shot, out, 9);
  assert.equal(gag.enter(shot, inside, 10), true);
});

test("the crew: the gate's calls, the captain and the cabin crew aboard, HR on the Moon", () => {
  const cq = mapFor("cq_moonshot"), d = cq.departures[0], m = cq.movers[d.mover - 1], pods = cq.launchers.find((l) => l.mover === d.mover);
  const run = (where, team, from, to) => { // lines said, with the cycle time each was said at
    const crew = createCrew(), said = [];
    for (let k = 0; from + k / 10 <= to + 1e-9; k++) {
      const t = Math.round((from + k / 10) * 10) / 10, pos = where(t);
      for (const id of crew.step({ map: cq, pos, t, team, sky: skyAt(cq, pos) })) said.push([id, Math.round(t * 10) / 10]);
    }
    return said;
  };
  const near = (said, want) => { // the same lines in order, each within a frame (0.1 s) of its time
    assert.deepEqual(said.map(([id]) => id), want.map(([id]) => id));
    said.forEach(([, t], i) => assert.ok(Math.abs(t - want[i][1]) <= 0.11, `${said[i][0]} at ${t}, want ${want[i][1]}`));
  };
  const gate = run(() => [d.door[0] - 10, 0, 0], "vibe", m.period - 2, 2 * m.period - 2);
  near(gate, [["gate_boarding", m.period], ["gate_final", m.period + d.close - 6]]);
  const aboard = (t) => { const o = MoverOffset(m, t); return [d.board[0] + o[0], d.board[1] + o[1], d.board[2] + o[2]]; };
  const ride = run(aboard, "vibe", 1, pods.from - 0.05);
  near(ride, [["captain_welcome", d.close], ["safety_demo", d.close], ["crew_pods_flank", pods.from - 4]]); // its pods flank: which third lands where
  { // lab_moon's pods all drop straight down: the plain line
    const lm = mapFor("lab_moon"), ld = lm.departures[0], lmv = lm.movers[ld.mover - 1], lp = lm.launchers.find((l) => l.mover === ld.mover);
    const crew = createCrew(), said = [];
    for (let k = 0; 1 + k / 10 <= lp.from; k++) {
      const t = Math.round((1 + k / 10) * 10) / 10, o = MoverOffset(lmv, t), pos = [ld.board[0] + o[0], ld.board[1] + o[1], ld.board[2] + o[2]];
      said.push(...crew.step({ map: lm, pos, t, team: "vibe", sky: skyAt(lm, pos) }));
    }
    assert.ok(said.includes("crew_pods") && !said.includes("crew_pods_flank"), said.join(" "));
  }
  // Landed on the Moon: Earth's visitors are welcomed, the Moon's staff reminded (a minute apart at most).
  const landed = (t) => (t < 1 ? [-272, 0, 0] : [120, 30, 0]);
  assert.deepEqual(run(landed, "vibe", 0, 3), [["hr_welcome", 1]]);
  assert.deepEqual(run(landed, "artisan", 0, 3), [["hr_rto", 1]]);
  // HR on points: Earth breaks into one (to the Moon's ears only), the Moon takes it back.
  const crew = createCrew();
  assert.deepEqual(crew.point({ ev: "neutral", id: "D", team: "vibe", from: "artisan" }, "artisan"), ["hr_breach"]);
  assert.deepEqual(crew.point({ ev: "neutral", id: "D", team: "vibe", from: "artisan" }, "vibe"), []);
  assert.deepEqual(crew.point({ ev: "captured", id: "A", team: "artisan" }, "artisan"), []); // a neutral one: not a reclaim
  crew.point({ ev: "neutral", id: "B", team: "artisan", from: "vibe" }, "artisan");
  assert.deepEqual(crew.point({ ev: "captured", id: "B", team: "artisan" }, "artisan"), ["hr_reclaimed"]);
});

test("spice: the board's status (DELAYED once Earth's backlog runs low, a gag), the gate's apology, the first-class seat's name", () => {
  const cq = mapFor("cq_moonshot"), d = cq.departures[0], m = cq.movers[d.mover - 1];
  const ok = { tk: { vibe: 200, artisan: 150 }, tkMax: 300 }, low = { tk: { vibe: 60, artisan: 290 }, tkMax: 300 };
  assert.equal(signValues(cq, 1, ok)["status:MVP-1"], "BOARDING");
  assert.equal(signValues(cq, d.close + 1, ok)["status:MVP-1"], "IN FLIGHT");
  assert.equal(signValues(cq, 1, low)["status:MVP-1"], "DELAYED - BUDGET REVIEW");
  assert.equal(signValues(cq, 1, null)["status:MVP-1"], "BOARDING");
  const players = [{ name: "alice", team: "vibe", sp: 4 }, { name: "bob", team: "vibe", sp: 9, k: 2 }, { name: "carol", team: "artisan", sp: 0, k: 0 }];
  assert.equal(signValues(cq, 1, ok, players)["first:vibe"], "BOB");
  assert.equal(signValues(cq, 1, ok, players)["first:artisan"], "TBD"); // nobody has scored
  const sign = cq.signs.find((s) => s.text.includes("{first:vibe}"));
  const hull = sign && cq.movers[sign.mover - 1];
  assert.ok(sign && sign.live && hull && JSON.stringify(hull.path) === JSON.stringify(m.path) && hull.hide[0] === hull.hide[1], "the first-class sign rides the ship, on its hull (never hidden: not the floor, gone at the drop)");
  // The gate's call as the ship docks: an apology when the board says DELAYED.
  const gate = [d.door[0] - 10, 0, 0];
  const call = (st) => { const crew = createCrew(); crew.step({ map: cq, pos: gate, t: m.period - 0.1, team: "vibe", st }); return crew.step({ map: cq, pos: gate, t: m.period + 0.05, team: "vibe", st }); };
  assert.deepEqual(call(ok), ["gate_boarding"]);
  assert.deepEqual(call(low), ["gate_delayed"]);
});

test("voice spots: the monument sighs as you step up to its plinth, not again while you stand there, nor within 20 s", () => {
  const cq = mapFor("cq_moonshot"), spot = cq.voiceSpots.find((z) => z.name === "sigh");
  assert.ok(spot, "cq_moonshot has the monument's spot");
  const inside = [(spot.min[0] + spot.max[0]) / 2, spot.min[1], (spot.min[2] + spot.max[2]) / 2], away = [inside[0] - 20, 0, inside[2]];
  const v = createVoiceSpots();
  assert.equal(v.enter(cq, away, 0), null);
  assert.equal(v.enter(cq, inside, 1), "sigh");
  assert.equal(v.enter(cq, inside, 2), null, "still standing there");
  v.enter(cq, away, 3);
  assert.equal(v.enter(cq, inside, 10), null, "back within 20 s");
  v.enter(cq, away, 30);
  assert.equal(v.enter(cq, inside, 31), "sigh");
  assert.ok(LINES.sigh && LINES.sigh.voice === "statue");
});

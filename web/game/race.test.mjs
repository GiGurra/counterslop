import { test } from "node:test";
import assert from "node:assert/strict";
import { raceHud, raceEnd, lapOf, fmtTime, fmtDelta, ordinal } from "./race.js";

const cp = (name, x0, z0, x1, z1) => ({ name, min: [x0, -1, z0], max: [x1, 6, z1], spawn: [0, 0, 0], yaw: 0 });
const map = { checkpoints: [cp("start", -1, 0, 1, 24), cp("west", -60, -24, -44, 24), cp("south", -1, -24, 1, 0), cp("east", 44, -24, 60, 24)] };
const names = { 1: "alice", 2: "bob", 3: "carol" };
const nameOf = (id) => names[id];
const round = (over = {}) => ({ mode: "race", ph: "live", laps: 2, gates: 4, rt: 12.34, left: 300,
  race: [{ id: 2, cp: 3, car: 2 }, { id: 1, cp: 1, car: 1 }, { id: 3, cp: 0, wr: 1.23, car: 3 }], ...over });

test("lapOf: over the line starts lap 1; each lap is every gate; capped at the laps", () => {
  assert.deepEqual([0, 1, 4, 5, 8, 9, 12].map((c) => lapOf(c, 4, 2)), [0, 1, 1, 2, 2, 2, 2]);
  assert.equal(lapOf(3, 0, 2), 0);
});

test("fmtTime and ordinal", () => {
  assert.equal(fmtTime(0), "0:00.0");
  assert.equal(fmtTime(83.46), "1:23.4");
  assert.equal(fmtTime(-1), "0:00.0");
  assert.deepEqual([1, 2, 3, 4, 11, 12, 13, 21, 22, 103].map(ordinal), ["1st", "2nd", "3rd", "4th", "11th", "12th", "13th", "21st", "22nd", "103rd"]);
});

test("raceHud: nothing outside the mode, in warmup or without gates", () => {
  assert.equal(raceHud({ mode: "tickets", ph: "live" }, { id: 1 }, map, nameOf), null);
  assert.equal(raceHud(round({ ph: "warmup" }), { id: 1 }, map, nameOf), null);
  assert.equal(raceHud(round({ gates: 0 }), { id: 1 }, map, nameOf), null);
});

test("raceHud: my lap, position, the standings, the next gate, the clock", () => {
  const r = raceHud(round(), { id: 1 }, map, nameOf, 0, 12.37);
  assert.equal(r.lap, 1);
  assert.equal(r.pos, 2);
  assert.equal(r.n, 3);
  assert.equal(r.clock, 12.3);
  assert.deepEqual(r.standings.map((s) => [s.pos, s.name, s.lap, s.me, s.wreck]), [[1, "bob", 1, false, 0], [2, "alice", 1, true, 0], [3, "carol", 0, false, 1.3]]);
  assert.deepEqual(r.next, { name: "west", pos: [-52, 2.5, 0] });
  assert.equal(r.countdown, 0);
  assert.equal(r.go, false);
  assert.equal(r.results, false);
  // The line again after a lap: start/finish; after the last: finish.
  assert.equal(raceHud(round({ race: [{ id: 1, cp: 4 }] }), { id: 1 }, map, nameOf).next.name, "start/finish");
  assert.equal(raceHud(round({ race: [{ id: 1, cp: 8 }] }), { id: 1 }, map, nameOf).next.name, "finish");
  // A gate without a name.
  const m2 = { checkpoints: map.checkpoints.map((c) => ({ ...c, name: "" })) };
  assert.equal(raceHud(round(), { id: 1 }, m2, nameOf).next.name, "gate 1");
});

test("raceHud: the grid's countdown, GO!, a wreck, the finish, the results", () => {
  const grid = raceHud(round({ ph: "freeze", race: [{ id: 1, cp: 0 }] }), { id: 1 }, map, nameOf, 4.2, 0);
  assert.equal(grid.countdown, 5);
  assert.equal(grid.next, null);
  assert.equal(raceHud(round({ ph: "freeze" }), { id: 1 }, map, nameOf, 2.1, 0).countdown, 3);
  assert.equal(raceHud(round(), { id: 1 }, map, nameOf, 0, 0.4).go, true);
  assert.equal(raceHud(round(), { id: 3 }, map, nameOf).wreck, 1.3);
  const fin = raceHud(round({ race: [{ id: 1, cp: 9, pl: 1, t: 71.25 }, { id: 2, cp: 6 }] }), { id: 1 }, map, nameOf);
  assert.equal(fin.finished, 1);
  assert.equal(fin.finishTime, 71.25);
  assert.equal(fin.next, null);
  const over = raceHud(round({ ph: "over", race: [{ id: 1, cp: 9, pl: 1, t: 71.25 }, { id: 2, cp: 6 }] }), { id: 2 }, map, nameOf);
  assert.equal(over.results, true);
  assert.equal(over.standings[0].place, 1);
  assert.equal(over.standings[1].place, 0);
  assert.equal(raceHud(round({ race: undefined }), { id: 1 }, map, nameOf).pos, 0); // nobody racing yet
});

test("raceEnd: the winner's name and time, or the leader when the time ran out", () => {
  assert.deepEqual(raceEnd(round({ race: [{ id: 1, cp: 9, pl: 1, t: 71.25 }] }), nameOf), { name: "alice", text: "takes the checkered flag in 1:11.2" });
  assert.deepEqual(raceEnd(round(), nameOf), { name: "bob", text: "led when the time ran out" });
  assert.equal(raceEnd(round({ race: [] }), nameOf), null);
});

test("raceHud on a course (point to point, on foot): gates, the split against the best, the finish", () => {
  const course = { checkpoints: [cp("start", 0, -8, 1, 8), cp("middle", 40, -8, 41, 8), cp("finish", 90, -8, 91, 8)] };
  const st = (me) => ({ mode: "race", ph: "live", laps: 1, gates: 3, p2p: true, rt: 20, race: [{ id: 1, ...me }] });
  let r = raceHud(st({ cp: 1, sp: 2.5 }), { id: 1 }, course, nameOf);
  assert.equal(r.p2p, true);
  assert.equal(r.gate, 1);
  assert.equal(r.onFoot, true);
  assert.equal(r.lap, 0);
  assert.equal(r.delta, null, "no best yet: no delta");
  assert.equal(r.next.name, "middle");
  r = raceHud(st({ cp: 2, sp: 9.1, dl: -0.42, hd: true }), { id: 1 }, course, nameOf);
  assert.equal(r.next.name, "finish", "the last gate finishes a course");
  assert.equal(r.split, 9.1);
  assert.equal(r.delta, -0.42);
  assert.equal(r.standings[0].gate, 2);
  r = raceHud(st({ cp: 3, pl: 1, t: 30.2, pb: 30.2, best: true }), { id: 1 }, course, nameOf);
  assert.equal(r.finished, 1);
  assert.equal(r.newBest, true);
  assert.equal(r.best, 30.2);
});

test("fmtDelta: ahead negative, behind positive, a tenth", () => {
  assert.deepEqual([-0.42, 1.26, 0, -0.04].map(fmtDelta), ["-0.4", "+1.3", "+0.0", "+0.0"]);
});

test("raceHud: on a team, every car taken mid-race: no slot until the next race", () => {
  const m = { ...map, cars: [{}, {}] };
  const full = round({ race: [{ id: 1, cp: 1, car: 1 }, { id: 2, cp: 1, car: 2 }] });
  assert.equal(raceHud(full, { id: 3, team: "vibe" }, m, nameOf).noSlot, true);
  assert.equal(raceHud(full, { id: 3, team: "spectator" }, m, nameOf).noSlot, false); // spectating anyway
  assert.equal(raceHud(full, { id: 1, team: "vibe" }, m, nameOf).noSlot, false); // racing
  assert.equal(raceHud(round({ race: [{ id: 1, cp: 1, car: 1 }] }), { id: 3, team: "vibe" }, m, nameOf).noSlot, false); // a slot free: joining
  assert.equal(raceHud({ ...full, ph: "freeze" }, { id: 3, team: "vibe" }, m, nameOf).noSlot, false);
});

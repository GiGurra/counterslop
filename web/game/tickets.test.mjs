import { test } from "node:test";
import assert from "node:assert/strict";
import { ticketsHud, overview, createDeploy, endText, KITS } from "./tickets.js";
import { TEAMS, setTeamMode } from "../assets/teams.js";

const map = {
  boxes: [{ min: [-64, -1, -32], max: [64, 0, 32] }],
  points: [
    { id: "earth", name: "Earth", pos: [-56, 0, 0], r: 7, owner: "vibe", locked: true, spawn: true },
    { id: "A", name: "Launch Pad", pos: [-32, 0, 0], r: 6, spawn: true },
    { id: "C", name: "Relay", pos: [0, 0, 0], r: 6 },
    { id: "moon", name: "Moon", pos: [56, 0, 0], r: 7, owner: "artisan", locked: true, spawn: true },
  ],
};
const round = (over = {}) => ({ mode: "tickets", ph: "live", tk: { vibe: 142, artisan: 87 }, tkMax: 150,
  pts: [{ id: "earth", own: "vibe" }, { id: "A", own: "vibe", by: "artisan", pr: 0.333, a: 1 }, { id: "C", by: "vibe", pr: 0.5, v: 1, a: 1, cont: true }, { id: "moon", own: "artisan" }], ...over });

test("ticketsHud: backlogs and points; nothing outside the mode or in warmup", () => {
  assert.equal(ticketsHud({ mode: "gpudrop", ph: "live" }, { team: "vibe" }), null);
  assert.equal(ticketsHud(round({ ph: "warmup" }), { team: "vibe" }), null);
  const h = ticketsHud(round(), { team: "vibe", dead: false }, {}, map);
  assert.deepEqual(h.tickets, { vibe: 142, artisan: 87, max: 150 });
  assert.deepEqual(h.points.map((p) => `${p.id}:${p.own || "-"}:${p.by}:${p.pr}:${p.cont}`), ["earth:vibe::0:false", "A:vibe:artisan:0.34:false", "C:-:vibe:0.5:true", "moon:artisan::0:false"]);
  assert.equal(h.spawn, null, "alive: no spawn screen");
});

test("ticketsHud: the spawn screen while dead lists our points; one with an enemy in it can't be picked", () => {
  const h = ticketsHud(round(), { team: "vibe", dead: true }, { rsp: 2.3, spAt: "earth", kt: "senior" }, map, { pt: "A", kit: "" });
  assert.equal(h.spawn.left, 3);
  assert.equal(h.spawn.at, "earth");
  assert.equal(h.spawn.pick, "A");
  assert.equal(h.spawn.kit, "senior", "no kit chosen here: the server's");
  assert.deepEqual(h.spawn.points.map((p) => `${p.id}:${p.ok}`), ["earth:true", "A:false", "C:false"]);
  const free = ticketsHud(round({ pts: round().pts.map((p) => (p.id === "A" ? { id: "A", own: "vibe" } : p)) }), { team: "vibe", dead: true }, {}, map);
  assert.equal(free.spawn.points.find((p) => p.id === "A").ok, true);
  assert.equal(ticketsHud(round({ ph: "over" }), { team: "vibe", dead: true }, {}, map).spawn, null, "no respawns once the round is over");
});

test("ticketsHud: one point to spawn at is no choice: only names it, fixed when the map has no other for us", () => {
  // A taken by the enemy right now: Earth's main is the only one open, but A could be ours again.
  const contested = ticketsHud(round(), { team: "vibe", dead: true }, {}, map);
  assert.equal(contested.spawn.only, "Earth");
  assert.equal(contested.spawn.fixed, false);
  // Both open: a choice (AUTO and the points).
  const free = ticketsHud(round({ pts: [{ id: "earth", own: "vibe" }, { id: "A", own: "vibe" }, { id: "C" }, { id: "moon", own: "artisan" }] }), { team: "vibe", dead: true }, {}, map);
  assert.equal(free.spawn.only, "");
  // The Moon here (cq_moonshot's rule: spawn points on the mains only): its main is all it ever has.
  const mains = { ...map, points: map.points.map((p) => ({ ...p, spawn: !!p.locked })) };
  const moon = ticketsHud(round(), { team: "artisan", dead: true }, {}, mains);
  assert.equal(moon.spawn.only, "Moon");
  assert.equal(moon.spawn.fixed, true);
});

test("overview: points placed on the floor plan, inside the picture", () => {
  const h = ticketsHud(round(), { team: "vibe" }, {}, map);
  const o = overview(h.points, map, 360, 200);
  for (const p of o.points) {
    assert.ok(p.x >= 0 && p.x <= 360 && p.y >= 0 && p.y <= 200, `${p.id} at ${p.x},${p.y}`);
  }
  const [e, , c, m] = o.points;
  assert.ok(e.x < c.x && c.x < m.x && Math.abs(c.y - 100) < 1e-9 && Math.abs(c.x - 180) < 1e-9);
});

test("deploy: sent until the self-snapshot shows the choice, at most every 250 ms; the kit is remembered", () => {
  const sent = [], mem = {};
  let t = 0;
  const store = { getItem: (k) => mem[k] ?? null, setItem: (k, v) => { mem[k] = v; } };
  const d = createDeploy({ send: (m) => sent.push(m), now: () => t, store });
  d.sync({});
  assert.equal(sent.length, 0, "auto and no kit: nothing to say");
  d.choose("A");
  d.sync({}); d.sync({});
  assert.deepEqual(sent, [{ t: "deploy", pt: "A", kit: "" }], "one send, then wait");
  t = 300; d.sync({});
  assert.equal(sent.length, 2, "lost: sent again");
  t = 600; d.sync({ spk: "A" });
  assert.equal(sent.length, 2, "the server shows it: done");
  assert.equal(d.setKit("admiral"), false);
  assert.equal(d.setKit(KITS[1].id), true);
  d.sync({ spk: "A", kt: KITS[0].id });
  assert.deepEqual(sent.at(-1), { t: "deploy", pt: "A", kit: KITS[1].id });
  assert.equal(mem["cs.kit"], KITS[1].id);
  assert.equal(createDeploy({ send() {}, now: () => 0, store }).want.kit, KITS[1].id, "the next page starts with it");
});

test("kits match economy.json and the end texts and team names fit the mode", () => {
  assert.deepEqual(KITS.map((k) => k.id), ["intern", "senior", "staff", "pm"]);
  assert.match(endText("vibe", "tickets"), /Moon shipped everything/);
  setTeamMode("tickets");
  assert.equal(TEAMS.vibe.name, "Earth"); assert.equal(TEAMS.artisan.short, "MOON");
  setTeamMode(undefined);
  assert.equal(TEAMS.vibe.name, "Vibe Coders"); assert.equal(TEAMS.artisan.short, "ARTS");
});

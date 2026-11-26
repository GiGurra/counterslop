import { test } from "node:test";
import assert from "node:assert/strict";
import { byScore, aliveCount, hudPlayers } from "./scoreorder.js";

// Operator: tied bots flickered up and down on the scoreboard, because rows
// tied on points and kills kept whatever order the player list arrived in.
test("scoreboard order is the same whatever order the rows arrive in", () => {
  const rows = [
    { id: 3, name: "bot Lint", sp: 2, k: 1, a: 0, d: 1 },
    { id: 1, name: "bot Diff", sp: 2, k: 1, a: 0, d: 1 },
    { id: 7, name: "alice", sp: 2, k: 1, a: 1, d: 1 },
    { id: 5, name: "bot Stub", sp: 2, k: 1, a: 0, d: 0 },
    { id: 9, name: "bob", sp: 5, k: 2, a: 0, d: 3 },
    { id: 2, name: "bot Diff", sp: 2, k: 1, a: 0, d: 1 },
  ];
  const want = [...rows].sort(byScore).map((r) => r.id);
  assert.deepEqual(want, [9, 7, 1, 2, 3, 5], "points, kills, name, id");
  for (let i = 0; i < 20; i++) {
    const shuffled = [...rows].sort(() => Math.sin(i * 7 + rows.length) - 0.5 + Math.random() - 0.5);
    assert.deepEqual([...shuffled].sort(byScore).map((r) => r.id), want);
  }
});

// Operator: the scoreboard should show who is alive, so you know how many are left.
test("aliveCount counts the members without the dead flag", () => {
  assert.equal(aliveCount([]), 0);
  assert.equal(aliveCount([{ id: 1 }, { id: 2, dead: true }, { id: 3, dead: false }]), 2);
});

// The snapshot lists the players in a different order from one to the next: the
// HUD's rows (compared as JSON before a re-render) must not change with it, or
// the whole HUD re-rendered on every full snapshot (~27 times a second with 10 bots).
test("the HUD's player rows are the same whatever order the snapshot lists them in", () => {
  const snap = [
    { id: 4, name: "bot Lint", team: "artisan", k: 2, ai: true, p: [1, 0, 2] },
    { id: 1, name: "alice", team: "vibe", ping: 12 },
    { id: 9, name: "bob", team: "artisan", ping: 30, dead: true },
    { id: 2, name: "target", team: "vibe", bot: true },
  ];
  const a = JSON.stringify(hudPlayers(snap, 1)), b = JSON.stringify(hudPlayers([...snap].reverse(), 1));
  assert.equal(a, b);
  assert.deepEqual(hudPlayers(snap, 1).map((p) => p.id), [2, 4, 9]);
  assert.deepEqual(hudPlayers(snap, 1)[1], { id: 4, name: "bot Lint", color: undefined, team: "artisan", k: 2, d: 0, a: 0, sp: 0, dead: false, ping: "BOT", ai: true });
  assert.equal(hudPlayers(snap, 1)[0].ping, "BOT");
  assert.equal(hudPlayers(snap, 1)[2].dead, true);
});

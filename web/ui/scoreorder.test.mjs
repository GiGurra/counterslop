import { test } from "node:test";
import assert from "node:assert/strict";
import { byScore, aliveCount } from "./scoreorder.js";

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

import { test } from "node:test";
import assert from "node:assert/strict";
import { deathCaption } from "./deathcaption.js";

// The line as plain text, and the highlighted parts with their team.
const text = (c) => c.line.map((p) => (typeof p === "string" ? p : p.b)).join("");
const bold = (c) => c.line.filter((p) => typeof p !== "string").map((p) => [p.b, p.team ?? ""]);

test("an artisan kills a vibe coder: REVIEWED", () => {
  const c = deathCaption({ killer: "bob", killerTeam: "artisan", myTeam: "vibe", weapon: "rifle", weaponName: "Autocomplete" });
  assert.equal(c.title, "REVIEWED");
  assert.equal(text(c), "you got reviewed by bob with Autocomplete");
  assert.deepEqual(bold(c), [["bob", "artisan"], ["Autocomplete", ""]]);
});

test("a vibe coder kills an artisan: DEPRECATED", () => {
  const c = deathCaption({ killer: "alice", killerTeam: "vibe", myTeam: "artisan", weapon: "rifle", weaponName: "Autocomplete" });
  assert.equal(c.title, "DEPRECATED");
  assert.equal(text(c), "alice deprecated you with Autocomplete");
  assert.deepEqual(bold(c)[0], ["alice", "vibe"]);
});

test("a teammate kill: BROKE THE BUILD (either team)", () => {
  for (const t of ["vibe", "artisan"]) {
    const c = deathCaption({ killer: "carol", killerTeam: t, myTeam: t, weapon: "he", weaponName: "Merge Conflict" });
    assert.equal(c.title, "BROKE THE BUILD");
    assert.equal(text(c), "your own teammate carol with Merge Conflict");
    assert.deepEqual(bold(c)[0], ["carol", t]);
  }
});

test("yourself or no killer: SEGFAULT", () => {
  assert.deepEqual(deathCaption({ self: true, myTeam: "vibe", weapon: "he", weaponName: "Merge Conflict" }),
    { title: "SEGFAULT", line: ["you crashed yourself", " with ", { b: "Merge Conflict" }] });
  assert.equal(text(deathCaption({ myTeam: "artisan" })), "you crashed yourself");
});

test("the Big Diff's blast: PROD WENT DOWN, planter credited or not", () => {
  const want = { title: "PROD WENT DOWN", line: ["you were in prod when it went down"] };
  assert.deepEqual(deathCaption({ killer: "bob", killerTeam: "artisan", myTeam: "vibe", weapon: "bigdiff" }), want);
  assert.deepEqual(deathCaption({ myTeam: "vibe", weapon: "bigdiff", self: true }), want);
});

test("the killer left (no team): REVIEWED wording", () => {
  for (const myTeam of ["vibe", "artisan"]) {
    const c = deathCaption({ killer: "someone", killerTeam: "", myTeam, weapon: "pistol", weaponName: "Hallucination" });
    assert.equal(c.title, "REVIEWED");
    assert.equal(text(c), "you got reviewed by someone with Hallucination");
  }
});

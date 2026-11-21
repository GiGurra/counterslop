import test from "node:test";
import assert from "node:assert/strict";
import { menuWantsCursor } from "./cursor.js";

test("menuWantsCursor: any clickable menu keeps the cursor; plain play doesn't", () => {
  assert.equal(menuWantsCursor({ screen: "playing" }), false);
  assert.equal(menuWantsCursor({ screen: "playing", teamMenu: true }), true, "team select right after PLAY");
  assert.equal(menuWantsCursor({ screen: "playing", buyMenu: true }), true);
  for (const screen of ["menu", "ingame", "options", "controls", "maps"]) assert.equal(menuWantsCursor({ screen }), true, screen);
});

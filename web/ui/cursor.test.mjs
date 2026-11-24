import test from "node:test";
import assert from "node:assert/strict";
import { menuWantsCursor, unlockOpensMenu } from "./cursor.js";

test("menuWantsCursor: any clickable menu keeps the cursor; plain play doesn't", () => {
  assert.equal(menuWantsCursor({ screen: "playing" }), false);
  assert.equal(menuWantsCursor({ screen: "playing", teamMenu: true }), true, "team select right after PLAY");
  assert.equal(menuWantsCursor({ screen: "playing", buyMenu: true }), true);
  assert.equal(menuWantsCursor({ screen: "playing", link: { mode: "lost" } }), true, "connection lost: RETRY NOW needs the cursor");
  assert.equal(menuWantsCursor({ screen: "playing", link: { mode: "blip" } }), false, "a blip: keep playing");
  for (const screen of ["menu", "ingame", "options", "controls", "maps"]) assert.equal(menuWantsCursor({ screen }), true, screen);
});

test("unlockOpensMenu: only a plain unlock in play is the player's Esc", () => {
  const play = { screen: "playing" };
  assert.equal(unlockOpensMenu(play), true);
  for (const k of ["console", "teamMenu", "buyMenu", "chatOpen"]) assert.equal(unlockOpensMenu({ ...play, [k]: k === "chatOpen" ? "all" : true }), false, k);
  assert.equal(unlockOpensMenu({ screen: "menu" }), false);
});

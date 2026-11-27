import test from "node:test";
import assert from "node:assert/strict";
import { menuWantsCursor, unlockOpensMenu, wantsAim } from "./cursor.js";

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

// After an auto-respawn in tickets mode the spawn screen closes and the game
// asks for the lock without a gesture; a browser may refuse it. wantsAim is
// when the HUD says so and a key press asks again (engine/input.js aimOnKey).
test("wantsAim: playing uncaptured with nothing up that takes the keys or the cursor", () => {
  const play = { screen: "playing" };
  assert.equal(wantsAim(play), true, "back from the spawn screen, the lock refused");
  assert.equal(wantsAim({ ...play, aiming: true }), false, "captured");
  for (const k of ["console", "teamMenu", "buyMenu", "crouchNotice", "chatOpen", "replay"]) assert.equal(wantsAim({ ...play, [k]: true }), false, k);
  assert.equal(wantsAim({ ...play, tickets: { spawn: {} } }), false, "the spawn screen is up: it wants the cursor");
  assert.equal(wantsAim({ screen: "menu" }), false);
});

// CS2: Esc in the buy menu (or the garage, team select) only closes it; the
// browser drops the pointer lock on that same Esc, and that must not open the
// main menu. A plain Esc later in play still does.
test("unlockOpensMenu: not for the Esc that just closed an overlay", async () => {
  const { OVERLAY_ESC_MS, escClosedOverlay } = await import("./cursor.js");
  const play = { screen: "playing", overlayClosedAt: 1000 };
  assert.equal(unlockOpensMenu(play, 1000 + 50), false, "the unlock from the buy menu's Esc");
  assert.equal(escClosedOverlay(play, 1000 + 50), true);
  assert.equal(unlockOpensMenu(play, 1000 + OVERLAY_ESC_MS + 1), true, "a later Esc in play opens the menu");
  assert.equal(unlockOpensMenu({ screen: "playing" }, 5), true, "never closed one");
});

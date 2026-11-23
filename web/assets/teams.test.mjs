import { test } from "node:test";
import assert from "node:assert/strict";
import { teamOpen, teamCapped } from "./teams.js";

test("teamOpen: a side that would end up 2+ bigger is closed", () => {
  assert.equal(teamOpen("vibe", { vibe: 1, artisan: 0 }), false);
  assert.equal(teamOpen("vibe", { vibe: 1, artisan: 1 }), true);
});

test("teamCapped: only humans fill a side up to the map's cap", () => {
  assert.equal(teamCapped("vibe", { vibe: 4 }, 4), true);
  assert.equal(teamCapped("vibe", { vibe: 3 }, 4), false); // the 4th may be a bot: it makes room
  assert.equal(teamCapped("vibe", { vibe: 9 }, 0), false); // no cap
  assert.equal(teamCapped("artisan", {}, 1), false);
});

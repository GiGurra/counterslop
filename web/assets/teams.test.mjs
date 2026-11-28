import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { teamOpen, teamCapped, teamOver } from "./teams.js";

test("teamOpen: a side that would end up more than the limit bigger is closed", () => {
  assert.equal(teamOpen("vibe", { vibe: 1, artisan: 0 }, 1), false);
  assert.equal(teamOpen("vibe", { vibe: 1, artisan: 1 }, 1), true);
  assert.equal(teamOpen("vibe", { vibe: 1, artisan: 0 }), false); // no limit known yet: the server's default, 1
  assert.equal(teamOpen("vibe", { vibe: 2, artisan: 0 }, 0), true); // 0: no limit, any side open
  assert.equal(teamOpen("vibe", { vibe: 2, artisan: 0 }, 2), false);
  assert.equal(teamOpen("vibe", { vibe: 1, artisan: 0 }, 2), true);
});

// The vectors the server's teamFull answers too (internal/game TestTeamLimitVectors).
test("teamOpen: the shared team limit vectors", () => {
  const { cases } = JSON.parse(readFileSync(new URL("../../internal/game/testdata/teamlimit_cases.json", import.meta.url)));
  assert.ok(cases.length > 0);
  for (const c of cases) {
    assert.equal(teamOpen(c.team, { vibe: c.vibe, artisan: c.artisan }, c.limit), c.open, JSON.stringify(c));
  }
});

test("teamOver: how much bigger the side would be after the join", () => {
  assert.equal(teamOver("vibe", { vibe: 2, artisan: 0 }), 3);
  assert.equal(teamOver("artisan", { vibe: 2, artisan: 0 }), -1);
});

test("teamCapped: only humans fill a side up to the map's cap", () => {
  assert.equal(teamCapped("vibe", { vibe: 4 }, 4), true);
  assert.equal(teamCapped("vibe", { vibe: 3 }, 4), false); // the 4th may be a bot: it makes room
  assert.equal(teamCapped("vibe", { vibe: 9 }, 0), false); // no cap
  assert.equal(teamCapped("artisan", {}, 1), false);
});

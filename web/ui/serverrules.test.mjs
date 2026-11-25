import { test } from "node:test";
import assert from "node:assert/strict";
import { SERVER_SETTINGS, SERVER_CVARS, normalize, toggleRotation, rotationList } from "./serverrules.js";

const row = (cvar) => SERVER_SETTINGS.flatMap((g) => g.rows).find((r) => r.cvar === cvar);

test("the curated list: game rules only, each once, no debug or cheat cvars", () => {
  assert.equal(new Set(SERVER_CVARS).size, SERVER_CVARS.length);
  for (const c of ["mp_autoteambalance", "mp_limitteams", "mp_friendlyfire", "mp_solid_players", "mp_roundtime", "mp_freezetime",
    "mp_buytime", "mp_maxrounds", "mp_overtime_enable", "mp_warmup_time", "mp_c4timer", "mp_respawn", "bot_quota", "bot_difficulty"]) {
    assert.ok(SERVER_CVARS.includes(c), c);
  }
  for (const c of ["sv_cheats", "weapon_spread", "sv_showimpacts", "sv_maxunlag", "sv_gravity"]) assert.ok(!SERVER_CVARS.includes(c), c);
  for (const r of SERVER_SETTINGS.flatMap((g) => g.rows)) {
    assert.ok(["toggle", "number", "select", "maps"].includes(r.kind), r.cvar);
    if (r.kind === "number") assert.ok(r.min <= r.max, r.cvar);
  }
});

test("numbers: clamped to the row's range, rounded to its step; junk is refused", () => {
  const lt = row("mp_limitteams");
  assert.equal(normalize(lt, "0"), "0");
  assert.equal(normalize(lt, "2"), "2");
  assert.equal(normalize(lt, "2.6"), "3");
  assert.equal(normalize(lt, "-4"), "0");
  assert.equal(normalize(lt, "999"), "32");
  assert.equal(normalize(lt, ""), null);
  assert.equal(normalize(lt, "  "), null);
  assert.equal(normalize(lt, "abc"), null);
  assert.equal(normalize(lt, "Infinity"), null);
  assert.equal(normalize(row("mp_roundtime"), "5"), "10");
  assert.equal(normalize(row("mp_roundtime"), 115), "115");
  assert.equal(normalize({ kind: "number", min: 0, max: 1, step: 0.05 }, "0.333"), "0.35");
});

test("toggles and selects", () => {
  const ff = row("mp_friendlyfire");
  assert.equal(normalize(ff, true), "1");
  assert.equal(normalize(ff, false), "0");
  assert.equal(normalize(ff, "1"), "1");
  assert.equal(normalize(ff, "yes"), null);
  const bal = row("mp_autoteambalance");
  for (const v of ["0", "1", "auto"]) assert.equal(normalize(bal, v), v);
  assert.equal(normalize(bal, "2"), null);
  assert.equal(normalize(row("bot_difficulty"), 3), "3");
  assert.equal(normalize(row("bot_difficulty"), "4"), null);
  assert.equal(normalize({ kind: "maps" }, "x"), null);
});

test("map rotation checklist: catalog order, the last map stays, all = every map", () => {
  const all = ["de_slop", "de_stack", "gs_kiosk"];
  assert.deepEqual(rotationList("all", all), all);
  assert.deepEqual(rotationList("", all), all);
  assert.deepEqual(rotationList("gs_kiosk,de_slop,nope", all), ["de_slop", "gs_kiosk"]);
  assert.equal(toggleRotation("de_slop,de_stack,gs_kiosk", all, "de_stack", false), "de_slop,gs_kiosk");
  assert.equal(toggleRotation("gs_kiosk", all, "de_slop", true), "de_slop,gs_kiosk");
  assert.equal(toggleRotation("gs_kiosk", all, "gs_kiosk", false), "gs_kiosk");
  assert.equal(toggleRotation("all", all, "de_slop", false), "de_stack,gs_kiosk");
});

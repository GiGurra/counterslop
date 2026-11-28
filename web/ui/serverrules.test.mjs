import { test } from "node:test";
import assert from "node:assert/strict";
import { SERVER_SETTINGS, SERVER_CVARS, rowNote, normalize, toggleRotation, rotationList } from "./serverrules.js";

const row = (cvar) => SERVER_SETTINGS.flatMap((g) => g.rows).find((r) => r.cvar === cvar);

test("the curated list: game rules (and sv_showimpacts under Practice) only, each once, no debug or cheat cvars", () => {
  assert.equal(new Set(SERVER_CVARS).size, SERVER_CVARS.length);
  for (const c of ["mp_autoteambalance", "mp_limitteams", "mp_friendlyfire", "mp_solid_players", "mp_roundtime", "mp_freezetime",
    "mp_buytime", "mp_maxrounds", "mp_overtime_enable", "mp_warmup_time", "mp_c4timer", "mp_c4_show_timer", "mp_respawn", "bot_quota", "bot_difficulty", "sv_showimpacts", "mp_wallbang"]) {
    assert.ok(SERVER_CVARS.includes(c), c);
  }
  assert.deepEqual(SERVER_SETTINGS.find((g) => g.title === "Practice").rows.map((r) => r.cvar), ["sv_showimpacts"]);
  for (const c of ["sv_cheats", "weapon_spread", "sv_maxunlag", "sv_gravity"]) assert.ok(!SERVER_CVARS.includes(c), c);
  for (const r of SERVER_SETTINGS.flatMap((g) => g.rows)) {
    assert.ok(["toggle", "number", "select", "maps", "speed"].includes(r.kind), r.cvar);
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

// GitHub #12: the Weapon speeds section, one row per weapon; "0" is the default.
test("weapon speeds: a row per weapon showing the speed in force; typing the default stores 0; clamped; reset", async () => {
  const { SPEED_ROWS, speedShown, speedResets, MIN_SPEED, MAX_SPEED } = await import("./serverrules.js");
  const { WEAPONS } = await import("../assets/weapons/index.js");
  assert.equal(SPEED_ROWS.length, Object.keys(WEAPONS).length);
  assert.ok(SERVER_SETTINGS.find((g) => g.title === "Weapon speeds").rows.every((r) => r.kind === "speed"));
  const p = row("weapon_speed_prompt");
  assert.equal(p.base, WEAPONS.prompt.speed);
  assert.equal(speedShown(p, "0"), p.base);
  assert.equal(speedShown(p, undefined), p.base);
  assert.equal(speedShown(p, "0.5"), 0.5);
  assert.equal(normalize(p, "0.5"), "0.5");
  assert.equal(normalize(p, String(p.base)), "0", "the default value is stored as 0");
  assert.equal(normalize(p, "0"), "0");
  assert.equal(normalize(p, "9"), String(MAX_SPEED));
  assert.equal(normalize(p, "0.05"), String(MIN_SPEED));
  assert.equal(normalize(p, "0.777"), "0.78");
  assert.equal(normalize(p, "fast"), null);
  assert.equal(normalize(p, ""), null);
  assert.deepEqual(speedResets({ weapon_speed_prompt: "0.5", weapon_speed_stub: "0", mp_roundtime: "115" }), [["weapon_speed_prompt", "0"]]);
  assert.deepEqual(speedResets({}), []);
});

test("movement: sv_movement, sv_airaccelerate and sv_autobunnyhopping; a map's pin shows on its row", async () => {
  const { pinned } = await import("./serverrules.js");
  assert.deepEqual(SERVER_SETTINGS.find((g) => g.title === "Movement").rows.map((r) => r.cvar), ["sv_movement", "sv_airaccelerate", "sv_autobunnyhopping"]);
  assert.equal(normalize(row("sv_movement"), "source"), "source");
  assert.equal(normalize(row("sv_movement"), "quake"), null);
  assert.equal(normalize(row("sv_airaccelerate"), "5000"), "1000");
  assert.equal(normalize(row("sv_autobunnyhopping"), true), "1");
  assert.equal(pinned(row("sv_movement"), { sv_movement: "source" }), "source");
  assert.equal(pinned(row("sv_airaccelerate"), { sv_movement: "source" }), undefined);
  assert.equal(pinned(row("sv_movement"), null), undefined);
});

test("rowNote: Auto team balance says the join limit is separate, warns when balance is off and the limit is on", () => {
  const bal = row("mp_autoteambalance");
  assert.equal(rowNote(row("mp_limitteams"), {}), null);
  const off = rowNote(bal, { mp_autoteambalance: "0", mp_limitteams: "1" });
  assert.equal(off.warn, true);
  assert.match(off.text, /Team limit \(1/);
  assert.match(off.text, /Team limit 0/);
  assert.equal(rowNote(bal, { mp_autoteambalance: "auto", mp_limitteams: "2" }).warn, false);
  assert.equal(rowNote(bal, { mp_autoteambalance: "0" }).warn, true); // the default limit, 1
  assert.match(rowNote(bal, { mp_autoteambalance: "0", mp_limitteams: "0" }).text, /any side/);
});

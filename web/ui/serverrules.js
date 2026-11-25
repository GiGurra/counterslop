// Server settings rules (pure, tested in node): the curated cvars the admin
// screen shows (web/ui/serversettings.js) and how a typed or picked value
// becomes the string the rcon command sends.

// kind: toggle (0|1), number (min/max/step, unit), select (choices: [value, label]),
// maps (mp_maprotation: a checklist of the maps), speed (weapon_speed_<id>:
// the weapon's move speed, shown as the value in force, base = weapons.json's).
import { WEAPONS } from "../assets/weapons/index.js";

// Weapon speeds (GitHub #12): one row per weapon; "0" (the cvar's default)
// means weapons.json's value, so a row shows the base until an admin changes it.
export const MIN_SPEED = 0.3, MAX_SPEED = 1.5;
export const SPEED_ROWS = Object.values(WEAPONS).map((w) => ({
  cvar: `weapon_speed_${w.id}`, label: w.name, kind: "speed", base: w.speed || 1,
}));
// The speed a row shows: the override, or the base when the cvar is 0 / missing.
export const speedShown = (row, v) => (parseFloat(v) > 0 ? parseFloat(v) : row.base);
// The rcon values that put every weapon speed back to weapons.json's: [cvar, "0"] for each override.
export const speedResets = (cvars = {}) => SPEED_ROWS.filter((r) => parseFloat(cvars[r.cvar]) > 0).map((r) => [r.cvar, "0"]);
// The map's pin on a row's cvar (content.Map.Rules), or undefined: the map's
// value applies while it runs, the server's on other maps.
export const pinned = (row, mapRules) => mapRules?.[row.cvar];
export const SERVER_SETTINGS = [
  { title: "Teams", rows: [
    { cvar: "mp_autoteambalance", label: "Auto team balance", kind: "select", choices: [["auto", "Auto"], ["1", "On"], ["0", "Off"]] },
    { cvar: "mp_limitteams", label: "Team limit", kind: "number", min: 0, max: 32, step: 1, unit: "0 = off" },
    { cvar: "mp_friendlyfire", label: "Friendly fire", kind: "toggle" },
    { cvar: "mp_solid_players", label: "Solid players", kind: "toggle" },
  ] },
  { title: "Combat", rows: [
    { cvar: "mp_wallbang", label: "Wallbangs (shoot through thin walls and props)", kind: "toggle" },
  ] },
  { title: "Rounds & match", rows: [
    { cvar: "mp_respawn", label: "Mode", kind: "select", choices: [["auto", "Auto"], ["0", "Rounds"], ["1", "Deathmatch"]] },
    { cvar: "mp_roundtime", label: "Round time", kind: "number", min: 10, max: 3600, step: 1, unit: "s" },
    { cvar: "mp_freezetime", label: "Freeze time", kind: "number", min: 0, max: 120, step: 1, unit: "s" },
    { cvar: "mp_buytime", label: "Buy time", kind: "number", min: 0, max: 600, step: 1, unit: "s" },
    { cvar: "mp_maxrounds", label: "Max rounds", kind: "number", min: 1, max: 60, step: 1 },
    { cvar: "mp_overtime_enable", label: "Overtime", kind: "toggle" },
    { cvar: "mp_warmup_time", label: "Warmup", kind: "number", min: 0, max: 3600, step: 1, unit: "s" },
    { cvar: "mp_c4timer", label: "Big Diff timer", kind: "number", min: 10, max: 120, step: 1, unit: "s" },
    { cvar: "mp_c4_show_timer", label: "Show the Big Diff countdown", kind: "toggle" },
  ] },
  { title: "Bots", rows: [
    { cvar: "bot_quota", label: "Bot quota", kind: "number", min: 0, max: 32, step: 1, unit: "players" },
    { cvar: "bot_difficulty", label: "Bot difficulty", kind: "select", choices: [["0", "Easy"], ["1", "Normal"], ["2", "Hard"], ["3", "Expert"]] },
  ] },
  { title: "Movement", rows: [
    { cvar: "sv_movement", label: "Movement", kind: "select", choices: [["classic", "Classic"], ["source", "Source (air strafe, bunny hop)"]] },
    { cvar: "sv_airaccelerate", label: "Air accelerate (source)", kind: "number", min: 0, max: 1000, step: 1, unit: "12 = CS2, surf 100-150" },
    { cvar: "sv_autobunnyhopping", label: "Auto bunny hop (hold jump)", kind: "toggle" },
  ] },
  { title: "Practice", rows: [
    { cvar: "sv_showimpacts", label: "Show impacts (your shot vs the server's, with hitboxes)", kind: "toggle" },
  ] },
  { title: "Weapon speeds", speeds: true, rows: SPEED_ROWS },
  { title: "Map vote", rows: [
    { cvar: "mp_maprotation", label: "Rotation", kind: "maps" },
  ] },
];

export const SERVER_CVARS = SERVER_SETTINGS.flatMap((g) => g.rows.map((r) => r.cvar));

// The value to send for raw input on row (a string, as the cvar holds it), or
// null when it isn't one: numbers are clamped to the row's range and rounded
// to its step; a toggle takes a boolean or "0"/"1"; a select one of its choices.
export function normalize(row, raw) {
  switch (row.kind) {
    case "toggle":
      return raw === true || raw === "1" || raw === 1 ? "1" : raw === false || raw === "0" || raw === 0 ? "0" : null;
    case "select":
      return row.choices.some(([v]) => v === String(raw)) ? String(raw) : null;
    case "speed": { // the base value itself is "0" (default), so the default survives a weapons.json change
      if (typeof raw === "string" && raw.trim() === "") return null;
      let v = Number(raw);
      if (!Number.isFinite(v)) return null;
      if (v <= 0) return "0";
      v = Math.min(MAX_SPEED, Math.max(MIN_SPEED, Math.round(v * 100) / 100));
      return Math.abs(v - row.base) < 1e-9 ? "0" : String(v);
    }
    case "number": {
      if (typeof raw === "string" && raw.trim() === "") return null;
      let v = Number(raw);
      if (!Number.isFinite(v)) return null;
      if (row.step) v = Math.round(v / row.step) * row.step;
      v = Math.min(row.max ?? Infinity, Math.max(row.min ?? -Infinity, v));
      return String(+v.toFixed(6));
    }
    default:
      return null;
  }
}

// mp_maprotation with name turned on or off, in the order of all (the
// catalog); the last map can't be turned off (an empty rotation means all).
// Unknown names in the current value are dropped.
export function toggleRotation(current, all, name, on) {
  const set = new Set(rotationList(current, all));
  if (on) set.add(name);
  else if (set.size > 1) set.delete(name);
  return all.filter((n) => set.has(n)).join(",");
}

// The maps in the rotation (all of them for "all" or "").
export function rotationList(current, all) {
  if (!current || current === "all") return [...all];
  const names = String(current).split(/[,\s]+/);
  return all.filter((n) => names.includes(n));
}

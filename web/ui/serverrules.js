// Server settings rules (pure, tested in node): the curated cvars the admin
// screen shows (web/ui/serversettings.js) and how a typed or picked value
// becomes the string the rcon command sends.

// kind: toggle (0|1), number (min/max/step, unit), select (choices: [value, label]),
// maps (mp_maprotation: a checklist of the maps).
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
  { title: "Practice", rows: [
    { cvar: "sv_showimpacts", label: "Show impacts (your shot vs the server's, with hitboxes)", kind: "toggle" },
  ] },
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

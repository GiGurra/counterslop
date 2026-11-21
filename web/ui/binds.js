// Key binds: one store (settings.binds: code -> console command) behind both
// the Options → Controls menu and the console's bind/unbind. Game actions are
// console commands, CS style: "+cmd" ones are held (pressed while the key is
// down), plain ones run once on key down. One key per action.
//
// Codes are KeyboardEvent.code values plus MouseN (the raw DOM event.button: 0
// primary, 2 secondary, ...) and MWheelUp/MWheelDown.

export const ACTIONS = [
  { cmd: "+forward", label: "Move forward", def: "KeyW" },
  { cmd: "+back", label: "Move back", def: "KeyS" },
  { cmd: "+moveleft", label: "Strafe left", def: "KeyA" },
  { cmd: "+moveright", label: "Strafe right", def: "KeyD" },
  { cmd: "+jump", label: "Jump", def: "Space" },
  { cmd: "+duck", label: "Crouch", def: "ControlLeft" },
  { cmd: "+speed", label: "Walk (quiet)", def: "ShiftLeft" },
  { cmd: "+attack", label: "Fire", def: "Mouse0" },
  { cmd: "+zoom", label: "Zoom / scope", def: "Mouse2" },
  { cmd: "reload", label: "Reload", def: "KeyR" },
  { cmd: "drop", label: "Drop weapon", def: "KeyG" },
  { cmd: "slot1", label: "Primary weapon", def: "Digit1" },
  { cmd: "slot2", label: "Sidearm", def: "Digit2" },
  { cmd: "slot3", label: "Melee", def: "Digit3" },
  { cmd: "lastinv", label: "Last weapon", def: "KeyQ" },
  { cmd: "invnext", label: "Next weapon", def: "MWheelDown" },
  { cmd: "invprev", label: "Previous weapon", def: "MWheelUp" },
  { cmd: "+showscores", label: "Scoreboard", def: "Tab" },
  { cmd: "teammenu", label: "Team select", def: "KeyM" },
  { cmd: "toggleconsole", label: "Console", def: "Backquote" },
];

export const DEFAULT_BINDS = Object.fromEntries(ACTIONS.map((a) => [a.def, a.cmd]));
const ACTION_CMDS = new Set(ACTIONS.map((a) => a.cmd));

// The key bound to a command, or "" when unbound.
export function keyFor(binds, cmd) {
  for (const [code, c] of Object.entries(binds)) if (c === cmd) return code;
  return "";
}

// Bind code to cmd. An action keeps a single key (its old key is freed); a code
// already bound to another command moves here. Returns {binds, moved: the
// command that lost this key, or null}.
export function bindKey(binds, code, cmd) {
  const out = { ...binds };
  if (ACTION_CMDS.has(cmd)) for (const [c, v] of Object.entries(out)) if (v === cmd && c !== code) delete out[c];
  const prev = out[code];
  out[code] = cmd;
  return { binds: out, moved: prev && prev !== cmd ? prev : null };
}

export function clearAction(binds, cmd) {
  const out = { ...binds };
  for (const [c, v] of Object.entries(out)) if (v === cmd) delete out[c];
  return out;
}

// Stored form: only the differences from the defaults (null = a default key unbound),
// so new default binds still reach players who changed others.
export function bindsDiff(binds) {
  const out = {};
  for (const code of new Set([...Object.keys(DEFAULT_BINDS), ...Object.keys(binds)])) {
    if (binds[code] !== DEFAULT_BINDS[code]) out[code] = binds[code] ?? null;
  }
  return out;
}

export function applyDiff(diff) {
  const out = { ...DEFAULT_BINDS };
  for (const [code, cmd] of Object.entries(diff || {})) {
    if (cmd == null) delete out[code];
    else if (typeof cmd === "string") out[code] = cmd;
  }
  return out;
}

const NAMES = {
  MWheelUp: "Wheel up", MWheelDown: "Wheel down", Backquote: "§ / `", Space: "Space",
  ControlLeft: "Ctrl", ControlRight: "Right Ctrl", ShiftLeft: "Shift", ShiftRight: "Right Shift",
  AltLeft: "Alt", AltRight: "Right Alt", Escape: "Esc",
};
export function keyName(code) {
  if (!code) return "unbound";
  const m = /^Mouse(\d+)$/.exec(code);
  if (m) return `Mouse ${m[1]}`; // the number the browser reports
  return NAMES[code] || code.replace(/^Key/, "").replace(/^Digit/, "").replace(/(Left|Right)$/, " $1");
}

// Binds saved before v4 named mouse buttons MOUSE<event.button + 1>; now the
// code is the raw button: MOUSEk -> Mouse(k-1) (the same physical button).
export function migrateMouseCodes(diff) {
  const out = {};
  for (const [code, cmd] of Object.entries(diff || {})) {
    const m = /^MOUSE(\d+)$/.exec(code);
    out[m ? `Mouse${+m[1] - 1}` : code] = cmd;
  }
  return out;
}

// One settings store behind both the options menu and the console cvars.
// Persisted to localStorage; every change notifies subscribers immediately.

// CS-style yaw: degrees per mouse count at sensitivity 1.
export const M_YAW = 0.022;

export const CVARS = {
  sensitivity: { def: 2.0, min: 0.05, max: 20, help: "mouse sensitivity, CS scale (0.022°/count × value)" },
  m_rawinput: { def: 1, bool: true, help: "raw mouse counts (no OS acceleration) when the browser supports it" },
  m_invert_y: { def: 0, bool: true, help: "invert vertical mouse" },
  fps_limit: { def: "auto", enum: ["off", "custom", "auto"], help: "off = follow display, custom = fps_max, auto = refresh − 3" },
  fps_max: { def: 141, min: 20, max: 1000, help: "frame cap when fps_limit is custom (fps_max 0 turns the cap off)" },
  cl_loop: { def: "raf", enum: ["raf", "unlocked"], help: "raf = vsync'd requestAnimationFrame, unlocked = experimental off-vsync loop" },
  cl_showfps: { def: 0, bool: true, help: "fps / frame-time / ping overlay" },
  volume: { def: 0.5, min: 0, max: 1, help: "master volume" },
  cl_fullscreen: { def: 0, bool: true, help: "play in fullscreen with keyboard lock, so Ctrl+W / Ctrl+S can't close or save the tab" },
  key_crouch: { def: "KeyC", key: true, help: "crouch key (Ctrl always crouches when the keyboard is locked in fullscreen)" },
};

const KEY = "cs.settings";
let values = load();
const subs = new Set();

function load() {
  const v = Object.fromEntries(Object.entries(CVARS).map(([k, c]) => [k, c.def]));
  let binds = {};
  try {
    const s = JSON.parse(localStorage.getItem(KEY) || "{}");
    for (const k of Object.keys(CVARS)) if (k in s) v[k] = coerce(k, s[k]) ?? v[k];
    if (s.binds && typeof s.binds === "object") binds = s.binds;
  } catch {}
  v.binds = binds;
  return v;
}

function save() { try { localStorage.setItem(KEY, JSON.stringify(values)); } catch {} }

// Parse a raw value for cvar k; returns undefined if invalid.
export function coerce(k, raw) {
  const c = CVARS[k];
  if (!c) return undefined;
  if (c.enum) return c.enum.includes(String(raw)) ? String(raw) : undefined;
  if (c.key) return typeof raw === "string" && raw ? raw : undefined;
  let n = typeof raw === "boolean" ? +raw : parseFloat(raw);
  if (!Number.isFinite(n)) return undefined;
  if (c.bool) return n ? 1 : 0;
  return Math.min(c.max, Math.max(c.min, n));
}

export const settings = {
  get: () => values,
  set(patch) {
    values = { ...values, ...patch };
    save();
    subs.forEach((f) => f(values));
  },
  subscribe(f) { subs.add(f); return () => subs.delete(f); },
  reset() { settings.set(Object.fromEntries(Object.entries(CVARS).map(([k, c]) => [k, c.def]))); }, // keeps binds
};

// Radians of view rotation per mouse count.
export const radPerCount = () => values.sensitivity * M_YAW * Math.PI / 180;

// One settings store behind both the options menu and the console cvars.
// Persisted to localStorage; every change notifies subscribers immediately.

// CS-style yaw: degrees per mouse count at sensitivity 1.
export const M_YAW = 0.022;

export const CVARS = {
  sensitivity: { def: 2.0, min: 0.05, max: 20, help: "mouse sensitivity, CS scale (0.022°/count × value)" },
  zoom_mode: { def: "hold", enum: ["hold", "toggle"], help: "scope: hold = zoom while the right button is held; toggle = right-click toggles" },
  zoom_sensitivity_ratio: { def: 1, min: 0.1, max: 5, help: "scoped sensitivity multiplier; 1 = same on-screen speed as unzoomed" },
  m_rawinput: { def: 1, bool: true, help: "raw mouse counts (no OS acceleration) when the browser supports it" },
  m_invert_y: { def: 0, bool: true, help: "invert vertical mouse" },
  fps_limiter: { def: "even", enum: ["even", "latency"], help: "vsync-mode limiter: even = caps at refresh/n only (smooth), latency = any cap by skipping frames (may micro-stutter)" },
  fps_limit: { def: "off", enum: ["off", "custom", "auto"], help: "off = every refresh, custom = fps_max, auto = refresh − 3 (latency limiter only; the even limiter treats it as off)" },
  fps_max: { def: 72, min: 1, max: 1000, help: "vsync-mode cap when fps_limit is custom; the even limiter snaps it to refresh/n (fps_max 0 turns the cap off)" },
  fps_max_unlocked: { def: 1000, min: 0, max: 100000, int: true, help: "unlocked-mode tick cap, evenly paced (0 = uncapped; console only)" },
  cl_loop: { def: "raf", enum: ["raf", "unlocked"], help: "raf = one frame per vsync, unlocked = experimental low-latency loop off vsync (fps_max_unlocked)" },
  cl_showfps: { def: 0, bool: true, help: "fps / frame-time / ping overlay" },
  r_shadow_quality: { def: 2, min: 0, max: 3, int: true, help: "sun shadows: 0 off, 1 hard, 2 soft (3x3 PCF), 3 softest (5x5 PCF)" },
  r_shadowmap_size: { def: 2048, min: 512, max: 4096, int: true, help: "sun shadow map resolution (rounded to a power of two)" },
  cl_showhitboxes: { def: 0, bool: true, help: "draw every player's server hitboxes (head/body/legs) as wireframes" },
  volume: { def: 0.5, min: 0, max: 1, help: "master volume" },
  cl_announcer: { def: 1, bool: true, help: "SlopVox kill-streak announcer on/off" },
  announcer_volume: { def: 0.8, min: 0, max: 1, help: "announcer volume (on top of master volume)" },
  cl_fullscreen: { def: 1, bool: true, help: "play in fullscreen with keyboard lock (Chromium), so Ctrl+W crouch-walks instead of closing the tab" },
  key_crouch: { def: "ControlLeft", key: true, help: "crouch key" },
  key_crouch2: { def: "KeyC", key: true, help: "alternate crouch key" },
};

const KEY = "cs.settings";
let values = load();
const subs = new Set();

function load() {
  const v = Object.fromEntries(Object.entries(CVARS).map(([k, c]) => [k, c.def]));
  let binds = {};
  try {
    const s = JSON.parse(localStorage.getItem(KEY) || "{}");
    if (!s._v) { // v1 saved every value, defaults included: drop the old defaults so new ones apply
      if (s.key_crouch === "KeyC") delete s.key_crouch;
      if (s.cl_fullscreen === 0) delete s.cl_fullscreen;
    }
    for (const k of Object.keys(CVARS)) if (k in s) v[k] = coerce(k, s[k]) ?? v[k];
    if (s.binds && typeof s.binds === "object") binds = s.binds;
  } catch {}
  v.binds = binds;
  return v;
}

// Only non-default values are stored, so changing a default reaches existing players.
function save() {
  const out = { _v: 2, binds: values.binds };
  for (const [k, c] of Object.entries(CVARS)) if (values[k] !== c.def) out[k] = values[k];
  try { localStorage.setItem(KEY, JSON.stringify(out)); } catch {}
}

// Parse a raw value for cvar k; returns undefined if invalid.
export function coerce(k, raw) {
  const c = CVARS[k];
  if (!c) return undefined;
  if (c.enum) return c.enum.includes(String(raw)) ? String(raw) : undefined;
  if (c.key) return typeof raw === "string" && raw ? raw : undefined;
  let n = typeof raw === "boolean" ? +raw : parseFloat(raw);
  if (!Number.isFinite(n)) return undefined;
  if (c.bool) return n ? 1 : 0;
  if (c.int) n = Math.round(n);
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

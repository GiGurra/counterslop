// One settings store behind both the options menu and the console cvars.
// Persisted to localStorage; every change notifies subscribers immediately.
import { DEFAULT_BINDS, applyDiff, bindsDiff, bindKey, migrateMouseCodes } from "./binds.js";
import { XHAIR_CVARS } from "./xhair.js";

// CS-style yaw: degrees per mouse count at sensitivity 1.
export const M_YAW = 0.022;

export const CVARS = {
  sensitivity: { def: 2.0, min: 0.05, max: 20, help: "mouse sensitivity, CS scale (0.022°/count × value)" },
  zoom_mode: { def: "toggle", enum: ["toggle", "hold"], help: "scope: toggle = right click zooms in/out (default); hold = zoom while the right button is held" },
  zoom_sensitivity_ratio: { def: 1, min: 0.1, max: 5, help: "scoped sensitivity multiplier; 1 = same on-screen speed as unzoomed" },
  m_rawinput: { def: 1, bool: true, help: "raw mouse counts (no OS acceleration) when the browser supports it" },
  m_invert_y: { def: 0, bool: true, help: "invert vertical mouse" },
  fps_limiter: { def: "even", enum: ["even", "latency"], help: "vsync-mode limiter: even = caps at refresh/n only (smooth), latency = any cap by skipping frames (may micro-stutter)" },
  fps_limit: { def: "off", enum: ["off", "custom", "auto"], help: "off = every refresh, custom = fps_max, auto = refresh − 3 (latency limiter only; the even limiter treats it as off)" },
  fps_max: { def: 72, min: 1, max: 1000, help: "vsync-mode cap when fps_limit is custom; the even limiter snaps it to refresh/n (fps_max 0 turns the cap off)" },
  fps_max_unlocked: { def: 1000, min: 0, max: 100000, int: true, help: "unlocked-mode tick cap, evenly paced (0 = uncapped; Options → Frame pacing when the loop is unlocked)" },
  r_gpu_fence: { def: 0, min: 0, max: 2, int: true, help: "experimental: max frames in flight on the GPU (fence-limited, Reflex-like); 0 = off" },
  r_late_input: { def: 0, bool: true, help: "experimental: vsync mode renders as late as possible before the display deadline (later input latch)" },
  r_finish: { def: 0, bool: true, help: "experimental: gl.finish() after every frame" },
  r_desynchronized: { def: 0, bool: true, help: "experimental: desynchronized (low-latency) WebGL canvas; applies on reload" },
  r_power_preference: { def: "default", enum: ["default", "high-performance", "low-power"], help: "experimental: WebGL powerPreference; applies on reload" },
  cl_loop: { def: "raf", enum: ["raf", "unlocked"], help: "raf = one frame per vsync, unlocked = experimental low-latency loop off vsync (fps_max_unlocked)" },
  cl_showfps: { def: 0, bool: true, help: "fps / frame-time / ping overlay" },
  r_shadow_quality: { def: 2, min: 0, max: 3, int: true, help: "sun shadows: 0 off, 1 hard, 2 soft (3x3 PCF), 3 softest (5x5 PCF)" },
  r_aa: { def: "auto", enum: ["auto", "off", "msaa2", "msaa4", "msaa8", "fxaa", "msaa4_fxaa"], help: "anti-aliasing: auto = the browser's MSAA, off, msaa2/4/8 (capped at what the GPU can do), fxaa (post pass), msaa4_fxaa; applies live (to or from auto fully after a reload)" },
  r_render_scale: { def: 100, min: 50, max: 100, int: true, help: "render resolution in % of the window, upscaled (lower = faster on weak GPUs)" },
  r_light_shadows: { def: 4, min: 0, max: 8, int: true, help: "lamp shadows of players: the 0-8 shadow-casting map lights with players nearest the camera draw them in (the map's own lamp shadows are always on)" },
  r_decals: { def: 1, bool: true, help: "bullet holes where shots hit the map (they stay for the round)" },
  r_casings: { def: 1, bool: true, help: "ejected casings (they stay for the round)" },
  r_muzzle_light: { def: 1, bool: true, help: "muzzle flashes light the room for a few frames (walls stop them)" },
  r_dynlight_shadows: { def: 2, min: 0, max: 4, int: true, help: "cube shadows for the 0-4 most important dynamic lights (muzzle flashes; your own shots first); the rest light without shadows, walls still stop them" },
  r_shadowmap_size: { def: 2048, min: 512, max: 4096, int: true, help: "sun shadow map resolution (rounded to a power of two)" },
  net_fakeloss: { def: 0, min: 0, max: 0.9, help: "testing: drop this fraction of command and snapshot packets" },
  net_fakedup: { def: 0, min: 0, max: 0.9, help: "testing: duplicate this fraction of command and snapshot packets" },
  net_fakejitter: { def: 0, min: 0, max: 500, int: true, help: "testing: delay everything by a varying 0..this many ms, in order (a TCP link with jitter)" },
  net_fakereorder: { def: 0, min: 0, max: 500, int: true, help: "testing: delay each command and snapshot packet by 0..this many ms on its own, so they overtake each other (datagrams)" },
  net_fakelag: { def: 0, min: 0, max: 1000, int: true, help: "testing: add this many ms of round-trip latency to the connection (half each way)" },
  cl_interp: { def: 0, min: 0, max: 0.25, help: "interpolation delay floor in seconds (CS cl_interp): other players are drawn at least this far in the past; 0 = adaptive only" },
  cl_debug_shots: { def: 0, bool: true, help: "print every shot to the console: the client's predicted hit vs the server's result" },
  cl_netdebug: { def: 0, bool: true, help: "net debug readout: commands sent/acked, backlog, server queue, tick drift, ping, prediction, last shot" },
  cl_radar: { def: 1, bool: true, help: "radar (top left): teammates, enemies your team sees, the Big Diff, the sites; the whole map shows with the scoreboard" },
  cl_radar_rotate: { def: 1, bool: true, help: "radar turns with your view (0: north up)" },
  cl_radar_scale: { def: 30, min: 10, max: 120, help: "radar range: metres from the centre to the edge" },
  cl_damage_numbers: { def: 0, bool: true, help: "floating damage numbers at your confirmed hits" },
  cl_showhitboxes: { def: 0, bool: true, help: "draw every player's server hitboxes (head/body/legs) as wireframes" },
  volume: { def: 0.5, min: 0, max: 1, help: "master volume" },
  snd_hrtf: { def: 1, bool: true, help: "3D sound (HRTF): hear whether a sound is in front, behind, above or below you (0: plain stereo left/right); walls muffle either way" },
  cl_announcer: { def: 1, bool: true, help: "SlopVox kill-streak announcer on/off" },
  cl_captions: { def: 1, bool: true, help: "show announcer lines as centre text and taunts as subtitles" },
  announcer_volume: { def: 0.8, min: 0, max: 1, help: "announcer volume (on top of master volume)" },
  cl_buy_close: { def: 0, bool: true, help: "close the buy menu after a purchase (CS 1.6); 0 = it stays open on the categories so buys chain (B 4 2 6 1)" },
  cl_music: { def: 1, bool: true, help: "music on/off: buy-time, warmup, match-end and MVP loops (never during live rounds) and short stingers" },
  music_volume: { def: 0.5, min: 0, max: 1, help: "music volume (on top of master volume)" },
  ...XHAIR_CVARS, // cl_crosshair*: Options -> Crosshair (xhair.js)
  cl_fullscreen: { def: 1, bool: true, help: "play in fullscreen with keyboard lock (Chromium), so Ctrl+W crouch-walks instead of closing the tab" },
};

const KEY = "cs.settings";
let values = load();
const subs = new Set();

function load() {
  const v = Object.fromEntries(Object.entries(CVARS).map(([k, c]) => [k, c.def]));
  let binds = { ...DEFAULT_BINDS };
  try {
    const s = JSON.parse(localStorage.getItem(KEY) || "{}");
    if (!s._v) { // v1 saved every value, defaults included: drop the old defaults so new ones apply
      if (s.key_crouch === "KeyC") delete s.key_crouch;
      if (s.cl_fullscreen === 0) delete s.cl_fullscreen;
    }
    for (const k of Object.keys(CVARS)) if (k in s) v[k] = coerce(k, s[k]) ?? v[k];
    if ((s._v || 0) >= 4) binds = applyDiff(s.binds);
    else if (s._v === 3) binds = applyDiff(migrateMouseCodes(s.binds));
    else {
      // v1/v2: binds held only custom console binds, crouch had a key + an alt key.
      // Keep the player's primary crouch key, drop the alt.
      if (s.binds && typeof s.binds === "object") for (const [code, cmd] of Object.entries(s.binds)) if (typeof cmd === "string") binds[code] = cmd;
      if (typeof s.key_crouch === "string" && s.key_crouch) binds = bindKey(binds, s.key_crouch, "+duck").binds;
    }
  } catch {}
  v.binds = binds;
  return v;
}

// Only non-default values are stored, so changing a default reaches existing players.
function save() {
  const out = { _v: 4, binds: bindsDiff(values.binds) };
  for (const [k, c] of Object.entries(CVARS)) if (values[k] !== c.def) out[k] = values[k];
  try { localStorage.setItem(KEY, JSON.stringify(out)); } catch {}
}

// Parse a raw value for cvar k; returns undefined if invalid.
export function coerce(k, raw) {
  const c = CVARS[k];
  if (!c) return undefined;
  if (c.enum) return c.enum.includes(String(raw)) ? String(raw) : undefined;
  if (c.key) return typeof raw === "string" && raw ? raw : undefined;
  if (c.color) { // #rrggbb (also rrggbb, #rgb)
    let m = /^#?([0-9a-f]{6})$/i.exec(String(raw).trim())?.[1];
    if (!m) m = /^#?([0-9a-f]{3})$/i.exec(String(raw).trim())?.[1]?.replace(/./g, "$&$&");
    return m ? `#${m.toLowerCase()}` : undefined;
  }
  let n = typeof raw === "boolean" ? +raw : parseFloat(raw);
  if (!Number.isFinite(n)) return undefined;
  if (c.bool) return n ? 1 : 0;
  if (c.int) n = Math.round(n);
  if (c.step) n = Math.round(n / c.step) * c.step;
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
  resetBinds() { settings.set({ binds: { ...DEFAULT_BINDS } }); },
  reset() { settings.set(Object.fromEntries(Object.entries(CVARS).map(([k, c]) => [k, c.def]))); }, // keeps binds
};

// Radians of view rotation per mouse count.
export const radPerCount = () => values.sensitivity * M_YAW * Math.PI / 180;

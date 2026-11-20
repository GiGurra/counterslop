import { createRenderer } from "../engine/gl.js";
import { chain, translate, scale, rotY } from "../engine/math.js";
import { hexRgb } from "../gen/color.js";
import { PALETTE } from "../assets/palette.js";
import { MATERIALS } from "../assets/materials/index.js";
import { SKY, SUN_DIR } from "../assets/sky.js";
import { playerParts, hitParts } from "../assets/models/player.js";
import { viewmodelParts, itemParts } from "../assets/models/weapons.js";
import { createArsenal } from "./arsenal.js";
import { team as teamInfo, SPECTATOR } from "../assets/teams.js";
import { createPhysics, newBody, R, H, H_CROUCH } from "../engine/physics.js";
import { BTN, moveInput } from "../engine/cmd.js";
import { keyFor as keyForBind } from "../ui/binds.js";
import { createInput } from "../engine/input.js";
import { createNet, createRemotes } from "../engine/net.js";
import { mountHud } from "../ui/hud.js";
import { basis, zoomSensitivity } from "../engine/math.js";
import * as sfx from "../assets/sounds/index.js";
import * as voice from "../assets/voice/index.js";
import { createAnnouncer } from "./announcer.js";
import { settings, radPerCount } from "../ui/settings.js";
import { createLoop, createOverlay } from "../engine/loop.js";
import { createConsole } from "../ui/console.js";
import { installTestApi } from "./testapi.js";
import { wireBoxIn } from "../engine/debugdraw.js";
import { WEAPONS, FIRE_SOUND } from "../assets/weapons/index.js";
import { createObserver, MODE_NAMES } from "./observer.js";

const SEND_HZ = 30, ROUND = 115, BASE_FOV = 1.3; // vertical fov (radians) unzoomed, as gl.js

const canvas = document.getElementById("gl");
const tagLayer = document.createElement("div");
tagLayer.className = "tags";
document.body.appendChild(tagLayer);

// Context flags (experimental options) apply when the context is created, i.e. on reload.
const ctxFlags = { desynchronized: !!settings.get().r_desynchronized, powerPreference: settings.get().r_power_preference };
const gfx = createRenderer(canvas, { palette: PALETTE, materials: MATERIALS, sky: SKY, sun: SUN_DIR }, ctxFlags);
const phys = createPhysics();
const input = createInput(canvas, () => settings.get().m_rawinput);
const remotes = createRemotes();

let map = null, me = null, myId = 0, myColor = "#fff", myTeam = "", spawnSeq = 0, wantPlay = false;
const known = new Map(); // id -> {name, team}, for join/leave/switch feed
const joinedText = (name, t) => t === SPECTATOR ? `${name} is spectating` : `${name} joined the ${teamInfo(t)?.name ?? "server"}`;
const spectating = () => myTeam === SPECTATOR;
const friendlyFire = () => serverCvars.mp_friendlyfire === "1"; // server cvar (rcon mp_friendlyfire 1)
const bodies = (list) => list.filter((r) => r.team !== SPECTATOR); // spectators have no body
// Watching instead of playing: spectating, or dead (the death cam, then following someone).
const observing = () => spectating() || !!me?.dead;
const remoteFx = new Map(); // id -> {flash, phase, last, stride}
const puffs = [];
let hitMarker = 0; // seconds left on the hit marker
const damageNumbers = [];
const HITBOX_COLORS = { head: [1, 0.2, 0.2], body: [1, 0.85, 0.2], legs: [0.3, 0.8, 1], gun: [0.6, 0.6, 0.6] };
let items = []; // weapons lying on the map, from snapshots
const announcer = createAnnouncer();
// Temporary until real deaths exist: bots never die, so each bot gets a virtual 100 HP
// on this client and running it down counts as a kill for the announcer.
const botHp = new Map();
let warmedUp = false;
function startAudio() {
  sfx.initAudio();
  if (!warmedUp) { warmedUp = true; voice.warmup(); }
}
let serverCvars = {}; // mp_*/sv_* from the server, plus openAdmin

const hud = mountHud(document.getElementById("hud"), {
  play(name) {
    startAudio();
    lockIn();
    // Leave the main menu whatever the lock does; without it the crosshair says "click to aim".
    hud.set({ screen: "playing" });
    startPlay(name);
  },
  aim() { lockIn(); },
  leave() { leaveMatch(); },
  teamMenu() { openTeamMenu(); },
  pickTeam(t) { joinTeam(t); hud.set({ teamMenu: false, screen: "playing" }); lockIn(); },
  closeTeamMenu() { hud.set({ teamMenu: false }); },
  // Back to the running match. Esc isn't user activation, so pointer lock may be
  // refused here; then the crosshair says "click to aim" until the next click.
  closeMenu() {
    hud.set({ screen: "playing" });
    input.lock();
  },
  loop: () => loop,
  latency: () => ({ lines: latencyReadout(), info: gfx.info(), requested: ctxFlags }),
  // Context flags changed: reload into the match (the leave prompt is skipped).
  reloadPage() {
    try { sessionStorage.setItem("cs.rejoin", JSON.stringify({ name: hud.get().name, inGame: wantPlay })); } catch {}
    skipLeaveConfirm = true;
    location.reload();
  },
});

// Capture the mouse, first going fullscreen with keyboard lock if that option is on.
let warnedNoKbLock = false;
// Pointer lock is requested first, synchronously inside the click: requestFullscreen
// consumes the click's user activation, so asking for the lock after it (as we
// used to) was refused on the first Play and bounced back to the menu.
let lockingUntil = 0; // the fullscreen switch may drop the lock briefly; don't treat that as Esc
function lockIn() {
  input.lock();
  if (!settings.get().cl_fullscreen || document.fullscreenElement) return;
  lockingUntil = performance.now() + 1500;
  input.enterFullscreen().then((ok) => {
    if (!ok && !warnedNoKbLock) {
      warnedNoKbLock = true;
      hud.feed("No fullscreen keyboard lock in this browser: crouch with C (Ctrl+W asks before leaving)");
    }
  }).finally(() => input.lock()); // fullscreen documents may lock without a fresh gesture
}
document.addEventListener("fullscreenchange", () => { if (wantPlay && hud.get().screen === "playing") input.lock(); });

// Closing, reloading or navigating away always asks first, on every screen, for
// the page's whole life (Ctrl+W in a window can't be blocked any other way).
// Only the auto-reload skips it. Browsers show the prompt only after sticky user
// activation (a click or key press in this document), so a fresh page, including
// one just auto-reloaded, is unprotected until the first click; the PLAY / click
// to aim click is that first interaction.
let skipLeaveConfirm = false;
addEventListener("beforeunload", (e) => {
  if (skipLeaveConfirm) return;
  e.preventDefault(); // Firefox
  e.returnValue = ""; // Chrome
});

// Join the match (connect, or rename if already connected).
function startPlay(name) {
  wantPlay = true;
  if (!net.open) net.connect(name);
  else net.send({ t: "hello", name });
}

function leaveMatch() {
  wantPlay = false;
  net.disconnect();
  if (document.fullscreenElement) document.exitFullscreen().catch(() => {});
  document.exitPointerLock?.();
  me = null; myId = 0;
  hud.set({ screen: "menu", connected: false, players: [] });
}

const net = createNet((m) => {
  const now = performance.now();
  switch (m.t) {
    case "open": hud.set({ connected: true }); con.print(`connected to ${location.host}`, "dim"); break;
    case "close":
      hud.set({ connected: false });
      con.print("connection lost", "warn");
      remotes.clear(); known.clear();
      break;
    case "welcome":
      if (checkBuild(m)) return;
      myId = m.id; myColor = m.color; map = m.map; myTeam = m.team; spawnSeq = 0;
      gfx.setMap(map); phys.setMap(map);
      me = newBody(m.spawn);
      me.yaw = Math.atan2(m.spawn[0], m.spawn[2]); // face the middle
      if (m.simHz > 0) simDt = 1 / m.simHz;
      simAcc = 0; vmCur = vmPrev = null; snapView();
      hud.set({ myId, myColor, myTeam, map: map.name });
      con.print(`map ${map.name}, you are #${myId}`, "dim");
      if (spectating() && wantPlay) openTeamMenu(); // the team-select screen on join
      break;
    case "inv":
      arsenal.setInv(m.inv);
      break;
    case "snap": {
      remotes.ingest(m, myId, now);
      items = m.items || [];
      const ids = new Set();
      for (const p of m.players) {
        if (p.id === myId) continue;
        ids.add(p.id);
        const k = known.get(p.id);
        if (!k) { hud.feed(p.team === SPECTATOR || !p.team ? `${p.name} slid into the server` : joinedText(p.name, p.team)); sfx.honk(); }
        else if (k.team !== p.team) hud.feed(joinedText(p.name, p.team));
        known.set(p.id, { name: p.name, team: p.team });
      }
      for (const [id, k] of known) if (!ids.has(id)) { known.delete(id); hud.feed(`${k.name} rage-quit`); }
      const players = m.players.filter((p) => p.id !== myId).map(({ id, name, color, team, k, d }) => ({ id, name, color, team, k: k || 0, d: d || 0 }));
      if (JSON.stringify(players) !== JSON.stringify(hud.get().players)) hud.set({ players });
      const mine = m.players.find((p) => p.id === myId);
      if (mine && (mine.k !== hud.get().myK || mine.d !== hud.get().myD)) hud.set({ myK: mine.k || 0, myD: mine.d || 0 });
      break;
    }
    case "hit": // server-confirmed hit (bots for now): marker, floating number, console line
      testCtl.lastHit = m;
      hitMarker = 0.25;
      damageNumbers.push({ at: m.at, text: `${m.dmg}`, part: m.part, life: 1.2 });
      { const w = WEAPONS[m.w];
        if (w?.hitGlyph) damageNumbers.push({ at: [m.at[0], m.at[1] + 0.35, m.at[2]], text: w.hitGlyph, part: "glyph", life: 0.9 });
        if (w?.hitText) damageNumbers.push({ at: [m.at[0], m.at[1] + 0.6, m.at[2]], text: w.hitText, part: "review", life: 1.6 }); }
      con.print(`hit ${m.name}: ${m.part} for ${m.dmg}`, "dim");
      {
        const hp = (botHp.get(m.id) ?? 100) - m.dmg;
        if (hp <= 0) { botHp.delete(m.id); announcer.onKill({ headshot: m.part === "head" }); }
        else botHp.set(m.id, hp);
      }
      break;
    case "notice":
      hud.feed(m.text);
      break;
    case "kicked":
      leaveMatch();
      hud.feed("You were kicked by an admin.");
      break;
    case "help": // rcon help: structured rows
      con.printSections(m.sections);
      break;
    case "print": // server text for the console only (rcon output, motd)
      con.print(m.text);
      break;
    case "cvars": // server cvars (admin-set); sv_gravity etc. apply with the authoritative sim
      serverCvars = { ...m.cvars, openAdmin: m.openAdmin };
      break;
    case "map": // changelevel
      map = m.map; gfx.setMap(map); phys.setMap(map);
      hud.set({ map: map.name });
      break;
    case "spawn": { // (re)spawn after a death or a team switch; spectators get no body
      const switched = m.team !== undefined && m.team !== myTeam;
      if (m.team !== undefined) myTeam = m.team;
      if (m.sp !== undefined) spawnSeq = m.sp;
      if (me && !spectating()) { me.pos = [...m.p]; me.vel = [0, 0, 0]; me.yaw = m.yaw ?? Math.atan2(m.p[0], m.p[2]); snapView(); }
      hud.set({ myTeam, ...(m.hp !== undefined && !spectating() ? { hp: m.hp } : {}) });
      if (switched) hud.feed(joinedText(hud.get().name, myTeam));
      break;
    }
    case "teamfull": // the server refused: that side would be 2+ players bigger
      hud.feed(`The ${teamInfo(m.team)?.name ?? m.team} are full. Pick the other side.`);
      openTeamMenu();
      break;
    case "correct":
      if (me) { me.pos = [...m.p]; me.vel = [0, 0, 0]; snapView(); }
      break;
    case "shot": {
      const fx = fxFor(m.id);
      fx.flash = 0.06;
      if (fx.last) (FIRE_SOUND[m.w] || sfx.pew)(spatial(fx.last));
      break;
    }
  }
});
const arsenal = createArsenal({ net, hud, zoomMode: () => settings.get().zoom_mode });
const observer = createObserver({ raycast: (o, d, maxT) => phys.raycast(o, d, maxT) });

// ---- actions via binds (ui/binds.js): "+cmd" held, plain commands run on press ----
const keyFor = (cmd) => keyForBind(settings.get().binds, cmd);
// Whether any key bound to cmd is held.
function down(cmd) {
  for (const [code, c] of Object.entries(settings.get().binds)) if (c === cmd && input.down(code)) return true;
  return false;
}
let jumpQueued = false, jumpBit = false;
// Jump button state for one command. A press since the last command shows up as
// a down edge even if already released (a wheel notch, a tap shorter than a tick);
// a re-press while the bit is still set gets one released command first.
function jumpButton() {
  if (jumpQueued) {
    if (jumpBit) { jumpBit = false; return false; }
    jumpQueued = false;
    return (jumpBit = true);
  }
  return (jumpBit = down("+jump"));
}
const playing = () => hud.get().screen === "playing" && !hud.get().teamMenu;
// Plain (run-once) game commands, also typed in the console.
function action(cmd) {
  if (cmd === "teammenu") { if (me && !hud.get().teamMenu) openTeamMenu(); return; }
  if (cmd === "toggleconsole") { con.toggle(); return; }
  if (!playing()) return;
  if (cmd === "reload") arsenal.reload();
  else if (cmd === "drop") arsenal.drop();
  else if (cmd === "slot1") arsenal.switchTo("primary");
  else if (cmd === "slot2") arsenal.switchTo("sidearm");
  else if (cmd === "slot3") arsenal.switchTo("melee");
  else if (cmd === "lastinv") arsenal.lastWeapon();
  else if (cmd === "invnext") arsenal.cycle(1);
  else if (cmd === "invprev") arsenal.cycle(-1);
}

input.onLock = (locked) => {
  hud.set({ aiming: locked });
  if (locked) hud.set({ screen: "playing" });
  else if (wantPlay && hud.get().screen === "playing" && !hud.get().console && performance.now() > lockingUntil && !hud.get().teamMenu) openMenu();
};

// The Esc menu: an overlay on the running match. Held keys are released so
// nobody keeps walking while in it.
function openMenu() {
  input.releaseKeys();
  hud.set({ screen: "ingame", menuOpenedAt: performance.now(), scoreboard: false });
}

// Team select (M, the menu, or 'jointeam' in the console).
function openTeamMenu() {
  document.exitPointerLock?.();
  hud.set({ teamMenu: true });
}
function joinTeam(t) {
  if (!["vibe", "artisan", "auto", SPECTATOR].includes(t)) return false;
  net.send({ t: "team", team: t });
  return true;
}
input.onClickLock = () => { if (wantPlay) { startAudio(); lockIn(); } };

// ---- auto-reload onto a new client build ----
const PAGE_BUILD = document.querySelector('meta[name="cs-build"]')?.content || "";
const RELOAD_GUARD_MS = 30000;

// Returns true when a reload is under way. The page's build was stamped in by
// the server that served it; the server we just reached reports its own.
function checkBuild(m) {
  if (!m.build || !PAGE_BUILD || m.build === PAGE_BUILD) return false;
  let last = null;
  try { last = JSON.parse(sessionStorage.getItem("cs.reloaded") || "null"); } catch {}
  if (last && last.build === m.build && Date.now() - last.at < RELOAD_GUARD_MS) {
    console.warn(`server build ${m.build} != page build ${PAGE_BUILD}, but a reload just tried that; staying put`);
    return false;
  }
  try {
    sessionStorage.setItem("cs.reloaded", JSON.stringify({ build: m.build, at: Date.now() }));
    sessionStorage.setItem("cs.rejoin", JSON.stringify({ name: hud.get().name, inGame: wantPlay }));
  } catch {}
  hud.feed(`new version (${m.version?.slice(0, 7) || m.build}), reloading…`);
  net.disconnect();
  setTimeout(() => {
    skipLeaveConfirm = true;
    location.reload();
    // Still here a moment later? The reload didn't happen: protect the page again.
    setTimeout(() => { skipLeaveConfirm = false; }, 2000);
  }, 600);
  return true;
}

// Coming back from such a reload: rejoin straight away, straight into play.
(() => {
  let r = null;
  try { r = JSON.parse(sessionStorage.getItem("cs.rejoin") || "null"); sessionStorage.removeItem("cs.rejoin"); } catch {}
  if (!r?.inGame) return;
  wantPlay = true;
  hud.set({ name: r.name, screen: "playing" });
  net.connect(r.name || "slopper");
})();
// Esc in play without the mouse captured (after closing the menu) opens the menu.
addEventListener("keydown", (e) => {
  if (e.code === "Escape" && !e.repeat && hud.get().screen === "playing" && !input.locked && !con.open) openMenu();
});
input.onKey = (code, isDown) => {
  if (con.open) return;
  const cmd = settings.get().binds[code];
  if (!cmd) return;
  if (cmd[0] === "+") {
    if (cmd === "+showscores") { hud.set({ scoreboard: isDown }); return; }
    // Observing: fire / zoom pick the next / previous player, jump changes the camera.
    if (observing() && inPlay()) {
      if (isDown && cmd === "+attack") observer.next(remotes.sample(performance.now()));
      else if (isDown && cmd === "+zoom") observer.prev(remotes.sample(performance.now()));
      else if (isDown && cmd === "+jump") observer.cycleMode();
      return;
    }
    if (isDown && cmd === "+jump") jumpQueued = true;
    else if (isDown && cmd === "+zoom" && playing() && settings.get().zoom_mode === "toggle") arsenal.toggleScope();
    return;
  }
  // The console's own key handler toggles it for keyboard keys; mouse binds go through here.
  if (cmd === "toggleconsole" && !/^M(OUSE|Wheel)/.test(code)) return;
  if (isDown) con.exec(cmd);
};

let clientVersion = "?";
fetch("/version", { cache: "no-store" }).then((r) => r.text()).then((v) => { clientVersion = v.trim(); }).catch(() => {});

const con = createConsole({
  get clientVersion() { return clientVersion; },
  onToggle(open) { if (open) input.releaseKeys(); hud.set({ console: open }); },
  getName: () => hud.get().name,
  setName(n) {
    hud.set({ name: n });
    try { localStorage.setItem("cs.name", n); } catch {}
    net.send({ t: "hello", name: n });
  },
  connect() { net.connect(hud.get().name || "slopper"); },
  disconnect() { leaveMatch(); },
  status: () => {
    const s = hud.get();
    return { map: s.map, connected: s.connected, ping: s.ping,
      friendlyFire: friendlyFire(),
      players: [{ id: myId, name: s.name, team: myTeam, me: true }, ...s.players], openAdmin: serverCvars.openAdmin };
  },
  joinTeam(t) { joinTeam(t); },
  slop() { startAudio(); sfx.honk(); sfx.land(14); setTimeout(() => sfx.jump(), 180); },
  announcerLines: () => voice.LINES,
  vox({ line, ph, voice: v }) {
    startAudio();
    if (line) { voice.say(line); return; }
    voice.sayPhonemes(ph, v && voice.VOICES[v] ? v : "announcer");
  },
  announceKill(k) { startAudio(); return announcer.onKill(k); },
  jump: () => { jumpQueued = true; },
  action,
  consoleKey: () => keyFor("toggleconsole"),
  bot(cmd, arg) { net.send({ t: "bot", cmd, arg: String(arg ?? "") }); },
  send(m) { if (!net.open) return con.print("not connected", "warn"); net.send(m); },
});
// Feed lines (joins, leaves, round ends) also go to the console scrollback.
const feed = hud.feed;
hud.feed = (text) => { feed(text); con.print(text, "dim"); };

fetch("/map.json").then((r) => r.json()).then((m) => { if (!map) { map = m; gfx.setMap(m); phys.setMap(m); } });

setInterval(() => {
  hud.set({ ping: net.ping, roundLeft: ROUND - 1 - Math.floor(Date.now() / 1000) % ROUND });
  if (hud.get().roundLeft === 0) hud.feed("Round draw. Everyone smells.");
  if (hud.get().roundLeft === ROUND - 1 && hud.get().screen === "playing") announcer.onRoundStart();
}, 1000);
setInterval(() => {
  if (me && net.open && !spectating()) net.send({ t: "state", p: me.pos.map(round3), yaw: round3(me.yaw), pitch: round3(me.pitch), c: me.crouched, sp: spawnSeq });
}, 1000 / SEND_HZ);

function fxFor(id) {
  let fx = remoteFx.get(id);
  if (!fx) remoteFx.set(id, (fx = { flash: 0, phase: 0, phase0: 0, last: null, stride: 0, speed: 0, crouch: 0 }));
  return fx;
}

// Stereo pan + distance falloff relative to the local listener.
function spatial(p) {
  if (!me) return { pan: 0, gain: 0.3 };
  const dx = p[0] - me.pos[0], dz = p[2] - me.pos[2], d = Math.hypot(dx, dz);
  const b = basis(me.yaw, 0);
  const pan = d > 0.01 ? (dx * b.right[0] + dz * b.right[2]) / d : 0;
  return { pan: pan * 0.8, gain: Math.min(1, 6 / (d + 3)) };
}

function shoot(w, now) {
  const spread = arsenal.spread(me);
  const b = basis(me.yaw + (Math.random() - 0.5) * spread, me.pitch + (Math.random() - 0.5) * spread);
  const eye = eyePos();
  let hit = phys.raycast(eye, b.fwd), color = PALETTE.spark, player = false;
  if (hit && hit.t > w.range) hit = null;
  for (const r of bodies(remotes.sample(now))) {
    if (!friendlyFire() && r.team === myTeam) continue; // shots pass through teammates
    const t = rayBox(eye, b.fwd, [r.p[0] - R, r.p[1], r.p[2] - R], [r.p[0] + R, r.p[1] + (r.c ? H_CROUCH : H), r.p[2] + R]);
    if (t !== null && t <= w.range && t < (hit?.t ?? Infinity)) { hit = { t, n: [0, 0, 0] }; color = teamInfo(r.team)?.rgb ?? hexRgb(r.color); player = true; }
  }
  if (player) sfx.bonk();
  if (hit) {
    const p = eye.map((v, i) => v + b.fwd[i] * hit.t + hit.n[i] * 0.05);
    for (let i = 0; i < 4; i++) {
      puffs.push({ p: [...p], v: [hit.n[0] + rs(), hit.n[1] + rs() + 0.5, hit.n[2] + rs()], life: 0.4, size: 0.12, color });
    }
  }
  arsenal.recoil(me, w);
  net.send({ t: "shot", o: eye.map(round3), d: b.fwd.map((v) => Math.round(v * 1e5) / 1e5) }); // the server hit-tests this ray
}

const rs = () => (Math.random() - 0.5) * 2;
const round3 = (v) => Math.round(v * 1000) / 1000;
const eyePos = () => [me.pos[0], me.pos[1] + me.eyeH + me.eyeOff, me.pos[2]];

function rayBox(o, d, mn, mx) {
  let t0 = 0, t1 = 200;
  for (let a = 0; a < 3; a++) {
    const inv = 1 / d[a];
    let ta = (mn[a] - o[a]) * inv, tb = (mx[a] - o[a]) * inv;
    if (ta > tb) [ta, tb] = [tb, ta];
    t0 = Math.max(t0, ta); t1 = Math.min(t1, tb);
    if (t0 > t1) return null;
  }
  return t0;
}

// Test harness hooks (server -test plus ?test=1): frames are stepped by the
// harness with a fixed dt, the shader clock can be pinned, the camera overridden.
const TEST = document.querySelector('meta[name="cs-test"]')?.content === "1" && new URLSearchParams(location.search).has("test");
const testCtl = { pinnedTime: null, camera: null, noSpread: false, lastHit: null };

// ---- fixed-step simulation + interpolated rendering ----
// The sim (movement, physics, weapons, recoil, remote animation state) advances
// in fixed ticks of 1/simHz seconds (the server announces simHz in welcome).
// Each tick consumes one input command, the shape the authoritative server sim
// will take. Rendering interpolates the local player between the last two ticks;
// mouse look is applied per frame so aiming never waits for a tick.
const MAX_TICKS = 25; // per frame: after a hitch, drop time instead of spiralling
let simDt = 1 / 100, simAcc = 0, cmdSeq = 0, prevButtons = 0;
let prevView = null, vmPrev = null, vmCur = null;

// Interpolation state of the local player before the latest tick.
const viewOf = (b) => ({ pos: [...b.pos], eyeH: b.eyeH, eyeOff: b.eyeOff });
// After a teleport (spawn, correction, harness set): nothing to interpolate from.
function snapView() { if (me) prevView = viewOf(me); vmPrev = vmCur; }

function inPlay() {
  const s = hud.get();
  return s.screen === "playing" && !s.console && !s.teamMenu;
}

// Sample input into one command (engine/cmd.js has the shape).
function buildCmd() {
  const play = inPlay();
  const d = (cmd) => (play && down(cmd) ? 1 : 0);
  const mv = { f: d("+forward") - d("+back"), r: d("+moveright") - d("+moveleft"), walk: d("+speed"), jump: play && jumpButton(), crouch: play ? d("+duck") : me.crouched };
  const b = (mv.jump ? BTN.JUMP : 0) | (mv.crouch ? BTN.CROUCH : 0) | (mv.walk ? BTN.WALK : 0) | (d("+attack") ? BTN.FIRE : 0) | (arsenal.scoped ? BTN.SCOPE : 0);
  return { seq: ++cmdSeq, ms: Math.round(simDt * 1000), f: mv.f, r: mv.r, yaw: me.yaw, pitch: me.pitch, b };
}

// One sim tick at wall time `now` (ms) for network-interpolated remotes.
function simTick(now) {
  if (me && map && !spectating()) {
    const cmd = buildCmd();
    prevView = viewOf(me);
    const b = cmd.b;
    const ev = phys.step(me, moveInput(cmd, prevButtons, me), simDt);
    prevButtons = b;
    if (ev.jumped) me.jumps = (me.jumps || 0) + 1;
    if (ev.jumped) sfx.jump();
    if (ev.landed) sfx.land(ev.landed);
    for (let i = 0; i < ev.steps; i++) sfx.footstep();
    if (me.pos[1] < -20) { me.pos = [...(map.teamSpawns?.[myTeam] ?? map.spawns)[0]]; me.vel = [0, 0, 0]; prevView = viewOf(me); }
    vmPrev = vmCur;
    vmCur = arsenal.tick(simDt, !!(b & BTN.FIRE), me, (w) => shoot(w, now), down("+zoom") && inPlay());
  }
  // Remote animation state: stride phase from tick-to-tick movement, speed from snapshots.
  for (const r of bodies(remotes.sample(now))) {
    const fx = fxFor(r.id);
    fx.phase0 = fx.phase;
    if (fx.last) {
      const d = Math.hypot(r.p[0] - fx.last[0], r.p[2] - fx.last[2]);
      fx.phase += d * 3;
      if (fx.speed > 4.2) { fx.stride += d; if (fx.stride > 2.2) { fx.stride = 0; sfx.footstep(spatial(r.p)); } }
    }
    fx.speed += (r.v - fx.speed) * (1 - Math.exp(-simDt * 12));
    fx.last = r.p;
    fx.flash = Math.max(0, fx.flash - simDt);
    fx.crouch += ((r.c ? 1 : 0) - fx.crouch) * (1 - Math.exp(-simDt * 18));
  }
  hitMarker = Math.max(0, hitMarker - simDt);
}

const lerp = (a, b, t) => a + (b - a) * t;
const VM_LERP = ["bobX", "bobY", "kick", "raise", "bolt"];
function viewmodelAt(alpha) {
  if (!vmCur) return null;
  if (!vmPrev || vmPrev.id !== vmCur.id) return vmCur;
  const v = { ...vmCur };
  for (const k of VM_LERP) v[k] = lerp(vmPrev[k], vmCur[k], alpha);
  return v;
}

let last = performance.now();
// Advance by dt seconds (wall clock, or the harness's fixed dt), run the due
// sim ticks, then draw unless draw is false (or the GPU queue is full). Returns whether it drew.
function frame(now, fixedDt, draw = true) {
  const dt = fixedDt ?? Math.min(0.25, (now - last) / 1000);
  last = now;
  const latch = performance.now(); // input is sampled from here on

  if (me && map && !spectating()) {
    const [mx, my] = input.takeMouse();
    const zoom = arsenal.scoped && arsenal.weapon()?.scope;
    const rpc = radPerCount() * (zoom ? zoomSensitivity(zoom.fov, BASE_FOV, settings.get().zoom_sensitivity_ratio) : 1), inv = settings.get().m_invert_y ? -1 : 1;
    me.yaw -= mx * rpc;
    me.pitch = Math.max(-1.55, Math.min(1.55, me.pitch - my * rpc * inv));
  } else {
    // Observing (or attract mode): the mouse and movement keys drive the observer camera.
    const [mx, my] = input.takeMouse();
    if (observing() && inPlay()) {
      const rpc = radPerCount(), inv = settings.get().m_invert_y ? -1 : 1;
      const d = (cmd) => (down(cmd) ? 1 : 0);
      observer.update(dt, [-mx * rpc, -my * rpc * inv], { f: d("+forward") - d("+back"), r: d("+moveright") - d("+moveleft"), up: 0, fast: !!d("+speed") });
    } else if (observing()) observer.update(dt, [0, 0], { f: 0, r: 0, up: 0, fast: false });
  }
  simAcc += dt;
  let ticks = 0;
  while (simAcc >= simDt - 1e-9) {
    simAcc -= simDt;
    if (++ticks > MAX_TICKS) { simAcc = 0; break; }
    simTick(now - simAcc * 1000);
  }
  const alpha = Math.min(1, Math.max(0, simAcc / simDt));

  // Render-only cosmetics: per frame, dt-correct.
  for (let i = puffs.length - 1; i >= 0; i--) {
    const p = puffs[i];
    p.life -= dt;
    if (p.life <= 0) { puffs.splice(i, 1); continue; }
    for (let a = 0; a < 3; a++) p.p[a] += p.v[a] * dt * 2;
    p.v[1] -= dt * 3;
    p.size = 0.14 * (p.life / 0.4);
  }
  draw = draw && gfx.canRender(); // r_gpu_fence: keep the sim running, skip the frame
  updateDamageNumbers(dt, draw);
  if (draw) render(now, alpha, latch);
  return draw;
}

function render(now, alpha, latch) {
  const time = testCtl.pinnedTime ?? now / 1000;
  let eye, yaw, pitch, viewmodel = null, watched = null, watchFirst = false;

  if (me && map && !spectating()) {
    const pv = prevView || viewOf(me);
    const pos = pv.pos.map((v, i) => lerp(v, me.pos[i], alpha));
    eye = [pos[0], pos[1] + lerp(pv.eyeH, me.eyeH, alpha) + lerp(pv.eyeOff, me.eyeOff, alpha), pos[2]];
    yaw = me.yaw; pitch = me.pitch;
    viewmodel = viewmodelAt(alpha);
    const sp = Math.hypot(me.vel[0], me.vel[2]);
    const gap = (6 + sp * 2.2 + (me.onGround ? 0 : 14) + (viewmodel?.kick || 0) * 8) * (me.crouched ? 0.6 : 1);
    document.documentElement.style.setProperty("--gap", gap.toFixed(1) + "px");
  } else if (map && observing()) {
    ({ eye, yaw, pitch, follow: watched, first: watchFirst } = observer.view(bodies(remotes.sample(now)), map));
  } else {
    // attract mode: slow orbit over the map
    const a = time * 0.08;
    eye = [Math.sin(a) * 34, 16, Math.cos(a) * 34]; yaw = a; pitch = -0.38;
  }
  showObserverHud(watched, watchFirst);

  // remote players: positions interpolated by the network clock, animation from the sim
  const players = bodies(remotes.sample(now)).map((r) => {
    const fx = fxFor(r.id);
    return { ...r, rgb: hexRgb(r.color), time, moving: fx.speed > 0.5, phase: lerp(fx.phase0, fx.phase, alpha), flash: fx.flash, crouch: !!r.c }; // replicated state: the drawn pose is the hit pose
  });

  // First-person follow: you are inside that player, so their body and name tag
  // go and their weapon is drawn as the viewmodel.
  const shown = watchFirst && !testCtl.camera ? players.filter((r) => r.id !== watched.id) : players;
  if (watchFirst && !testCtl.camera) {
    const fx = fxFor(watched.id);
    viewmodel = { id: WEAPONS[watched.w] ? watched.w : "prompt", bobX: 0, bobY: 0, kick: fx.flash * 4, flash: fx.flash, reload: 0, raise: 1 };
  }
  const solids = shown.flatMap(playerParts);
  if (observing() && observer.mode === "overview" && !observer.freeze) {
    // Overview: a big glowing dot above every player in their team colour.
    for (const r of players) solids.push({ m: chain(translate(r.p[0], r.p[1] + 4, r.p[2]), scale(2.6, 0.5, 2.6)), color: teamInfo(r.team)?.rgb ?? hexRgb(r.color), emit: 1 });
  }
  for (const it of items) solids.push(...itemParts(it.p, it.w, time));
  for (const p of puffs) solids.push({ m: chain(translate(...p.p), scale(p.size, p.size, p.size)), color: p.color, emit: 1 });
  if (testCtl.camera) {
    // Free camera: the local player is drawn too, since we're not looking through their eyes.
    ({ eye, yaw, pitch } = testCtl.camera);
    if (me && !spectating()) solids.push(...playerParts({ id: myId, team: myTeam, p: me.pos, yaw: me.yaw, pitch: me.pitch, rgb: hexRgb(myColor), time, crouch: me.crouched ? 1 : 0 }));
    viewmodel = null;
  }
  if (settings.get().cl_showhitboxes) {
    // Exactly what the server ray-traces: the hittable model parts, posed from the replicated state.
    const who = players.map((r) => ({ p: r.p, yaw: r.yaw, pitch: r.pitch, crouch: !!r.c, team: r.team, id: r.id }));
    if (testCtl.camera && me && !spectating()) who.push({ p: me.pos, yaw: me.yaw, pitch: me.pitch, crouch: me.crouched, team: myTeam, id: myId });
    for (const st of who) for (const h of hitParts(st)) solids.push(...wireBoxIn(h.m, h.c, h.s, HITBOX_COLORS[h.region] || [1, 1, 1]));
  }


  // Scope state is read live (a toggle is immediate input, not sim state).
  const scoped = !!viewmodel && !watched && arsenal.scoped, scope = scoped && arsenal.weapon()?.scope;
  const overlay = viewmodel && !scoped ? viewmodelParts(eye, yaw, pitch, viewmodel, viewmodel.id) : [];
  document.documentElement.classList.toggle("hitmark", hitMarker > 0);
  gfx.frame({ eye, yaw, pitch, time, solids, overlay, fov: scope ? scope.fov : undefined, latch });
  updateTags(shown);
}

// The observer HUD line: who you watch (name, team, HP, weapon, ammo), the
// camera mode, or the death cam caption. Only pushed to the HUD on change.
function showObserverHud(watched, first) {
  let o = null;
  if (observing()) {
    const f = observer.freeze, k = f && remotes.sample(performance.now()).find((r) => r.id === f.killer);
    o = f ? { freeze: true, killer: k?.name ?? "someone", killerTeam: k?.team ?? "", weapon: WEAPONS[f.weapon]?.name ?? f.weapon ?? "" }
      : { mode: observer.mode, modeName: MODE_NAMES[observer.mode], first: !!first, dead: !spectating(),
        name: watched?.name ?? "", team: watched?.team ?? "", hp: watched?.hp ?? 0,
        weapon: watched ? WEAPONS[watched.w]?.name ?? "" : "", ammo: watched?.ammo ?? 0, res: watched?.res ?? 0 };
  }
  if (JSON.stringify(o) !== JSON.stringify(hud.get().observe ?? null)) hud.set({ observe: o });
}

// Floating damage numbers at server-reported hit points: rise and fade.
const dmgLayer = document.createElement("div");
dmgLayer.className = "tags dmg-layer";
document.body.appendChild(dmgLayer);
function updateDamageNumbers(dt, draw = true) {
  for (let i = damageNumbers.length - 1; i >= 0; i--) {
    const d = damageNumbers[i];
    d.life -= dt;
    if (!d.el) {
      d.el = document.createElement("div");
      d.el.className = `dmg dmg-${d.part}`;
      d.el.textContent = d.part === "head" ? `${d.text}!` : d.text;
      dmgLayer.appendChild(d.el);
    }
    if (d.life <= 0) { d.el.remove(); damageNumbers.splice(i, 1); continue; }
    if (!draw) continue;
    const rise = (1.2 - d.life) * 0.6;
    const s = gfx.project([d.at[0], d.at[1] + 0.3 + rise, d.at[2]]);
    d.el.style.display = s ? "" : "none";
    if (s) {
      d.el.style.transform = `translate(${s[0].toFixed(0)}px, ${s[1].toFixed(0)}px) translate(-50%, -100%)`;
      d.el.style.opacity = Math.min(1, d.life * 2).toFixed(2);
    }
  }
}

function updateTags(players) {
  const want = new Set(players.map((p) => p.id));
  for (const el of [...tagLayer.children]) if (!want.has(+el.dataset.id)) el.remove();
  for (const p of players) {
    let el = tagLayer.querySelector(`[data-id="${p.id}"]`);
    if (!el) {
      el = document.createElement("div");
      el.dataset.id = p.id;
      tagLayer.appendChild(el);
    }
    if (el.textContent !== p.name) el.textContent = p.name;
    el.style.color = teamInfo(p.team)?.color ?? p.color;
    const s = gfx.project([p.p[0], p.p[1] + 2.15 - p.crouch * 0.6, p.p[2]]);
    if (!s || s[2] > 60) { el.style.display = "none"; continue; }
    el.style.display = "";
    el.style.transform = `translate(${s[0].toFixed(0)}px, ${s[1].toFixed(0)}px) translate(-50%, -100%)`;
  }
}

// The harness steps frames itself unless it asks for the live loop (testCtl.live).
const loop = createLoop((now) => (!TEST || testCtl.live ? frame(now) : false), settings);
createOverlay(loop, settings, () => net.ping, () => latencyReadout().join("\n"));

// Latency / queue readout for the overlay and the Experimental options panel.
function latencyReadout() {
  const r = gfx.stats(), l = loop.stats(), i = gfx.info(), s = settings.get();
  const ms = (v) => (v == null ? "n/a" : `${v.toFixed(1)} ms`);
  const est = r.latchToGpuMs != null ? r.latchToGpuMs + (l.presented.avg || l.avg) : null;
  const shadows = r.shadowQuality ? `shadows q${r.shadowQuality} ${r.shadowSize}² gpu ${ms(r.shadowGpuMs)}` : "shadows off";
  const flag = (req, got) => (req ? (got ? "on" : "REFUSED") : "off");
  return [
    `rendered ${l.renderedFps} fps · shown ${l.presented.fps.toFixed(0)} fps · gpu frame ${ms(r.gpuFrameMs)} · ${shadows}`,
    `in flight ${r.inFlight}${r.maxInFlight ? `/${r.maxInFlight} (skipped ${r.skipped})` : ""} · latch→gpu ${ms(r.latchToGpuMs)} · est. input→present ${ms(est)}`,
    `desync ${flag(ctxFlags.desynchronized, i.desynchronized)} · power ${i.powerPreference} · fence ${r.maxInFlight || "off"}${!s.r_gpu_fence && r.maxInFlight ? " (unlocked default)" : ""} · late latch ${s.r_late_input ? "on" : "off"} · finish ${s.r_finish ? "on" : "off"}`,
  ];
}
sfx.setVolume(settings.get().volume);
settings.subscribe((s) => sfx.setVolume(s.volume));
const applyGfx = (s) => gfx.setOptions({
  shadowQuality: s.r_shadow_quality, shadowSize: 2 ** Math.round(Math.log2(s.r_shadowmap_size)),
  // Unlocked mode always limits the GPU queue (2 unless set lower): submitting
  // frames faster than the GPU drains them grows the driver queue without bound.
  maxInFlight: s.r_gpu_fence || (s.cl_loop === "unlocked" ? 2 : 0), finish: !!s.r_finish,
});
applyGfx(settings.get());
settings.subscribe(applyGfx);
const syncAnnouncer = (s) => voice.setAnnouncer({ volume: s.announcer_volume, enabled: s.cl_announcer, onLine: showCaption });
function showCaption(l) {
  if (!settings.get().cl_captions) return;
  hud.caption({ text: l.text, color: l.color, dur: l.dur, kind: l.voice === "taunt" ? "sub" : "shout" });
}
syncAnnouncer(settings.get());
settings.subscribe(syncAnnouncer);

// Debug handle for the console / automated checks.
window.cs = { get me() { return me; }, remotes, net };

if (TEST) {
  installTestApi({
    frame, testCtl, input, snapView, latencyReadout, keyFor, get simDt() { return simDt; }, loop,
    resetSim() { simAcc = 0; prevButtons = 0; jumpQueued = jumpBit = false; vmCur = vmPrev = null; snapView(); }, hud, con, settings, net, remotes, gfx, announcer,
    get me() { return me; }, get myId() { return myId; }, get map() { return map; }, arsenal, items: () => items, hitParts, observer, sampleRemotes: () => remotes.sample(performance.now()),
    get spawnSeq() { return spawnSeq; }, get myTeam() { return myTeam; },
    play: (name) => startPlay(name),
  });
}

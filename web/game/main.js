import { createRenderer } from "../engine/gl.js";
import { chain, translate, scale, rotY } from "../engine/math.js";
import { hexRgb } from "../gen/color.js";
import { PALETTE } from "../assets/palette.js";
import { MATERIALS } from "../assets/materials/index.js";
import { SKY, SUN_DIR } from "../assets/sky.js";
import { createMapCatalog } from "./maps.js";
import { muzzlePose, MUZZLE } from "./muzzle.js";
import { playerParts, hitParts, PLAYER } from "../assets/models/player.js";
import { newGait, gaitStep, resetGait, lerpGait, gaitFromWire } from "../gen/gait.js";
import { rayModel } from "../gen/boxmodel.js";
import { viewmodelParts, itemParts } from "../assets/models/weapons.js";
import { gpuHandParts } from "../assets/models/gpu.js";
import { spawnMarks } from "../assets/models/spawnmarks.js";
import { createArsenal, shotAngles, recoilOffset, recoilAngles, applySpeeds } from "./arsenal.js";
import { shotCone, createCrosshair } from "./crosshair.js";
import { spatialize } from "./spatial.js";
import { effectiveCvars, moveRules } from "./rules.js";
import { createPredictor, peersFrom, peersAt, coresAhead } from "./predict.js";
import { pickTags } from "./tags.js";
import { corpsePose, corpseParts, bodyRests, deathAge, landedBetween } from "./corpse.js";
import { createNetDebug } from "./netdebug.js";
import { team as teamInfo, SPECTATOR } from "../assets/teams.js";
import { createPhysics, newBody, R, H, H_CROUCH, EYE, EYE_CROUCH } from "../engine/physics.js";
import { BTN } from "../engine/cmd.js";
import { keyFor as keyForBind, keyName, ACTIONS, bindKey } from "../ui/binds.js";
import { createInput } from "../engine/input.js";
import { createNet, createRemotes, unpackPoses } from "../engine/net.js";
import { createLink, linkView, sameView, STALL_MS } from "./link.js";
import { mountHud } from "../ui/hud.js";
import { hasKeyboardLock, needsCrouchNotice } from "../engine/browser.js";
import { menuWantsCursor, unlockOpensMenu } from "../ui/cursor.js";
import { basis, zoomSensitivity } from "../engine/math.js";
import { createRoll, frameOf, above, turn } from "../engine/gravity.js";
import * as sfx from "../assets/sounds/index.js";
import { createHums } from "./hum.js";
import * as voice from "../assets/voice/index.js";
import { chatPrefix } from "../ui/chat.js";
import { createAnnouncer } from "./announcer.js";
import { createMusic, catalog as musicCatalog } from "../assets/music/index.js";
import { createMusicDirector } from "./music.js";
import { stepLocal } from "./localsim.js";
import { createControls } from "./controls.js";
import { settings, radPerCount } from "../ui/settings.js";
import { createLoop, createOverlay } from "../engine/loop.js";
import { createConsole } from "../ui/console.js";
import { installTestApi } from "./testapi.js";
import { wireBox, wireBoxIn } from "../engine/debugdraw.js";
import { WEAPONS, FIRE_SOUND, muzzleLight as muzzleLightOf } from "../assets/weapons/index.js";
import { AimedItemG, ShotView, WallTrace, WallStop, MapBounds } from "./shared.gen.js";
import { createObserver, MODE_NAMES } from "./observer.js";
import { gunzip, readReplay } from "../engine/replay.js";
import { NET_SCHEMA_HASH } from "../engine/netdelta.js";
import { createReplayer, replayNotes, roundLabel } from "./replayer.js";
import { createRecorder } from "./recorder.js";
import { createRounds } from "./rounds.js";
import { createNades } from "./nades.js";
import { createRadar } from "./radar.js";
import { createDebris } from "./debris.js";
import { buildAction, nextAttempt, bustURL } from "./build.js";
import { hitSeed, slopBurst, stepBurst, burstSize, relAngle, flinch, vignette, vignetteCss, lowHpEdge, VIGNETTE_S, FLINCH_S } from "./hitfx.js";
import { deathCaption } from "../ui/deathcaption.js";

const SEND_HZ = 30, BASE_FOV = 1.3; // vertical fov (radians) unzoomed, as gl.js

const canvas = document.getElementById("gl");
const tagLayer = document.createElement("div");
tagLayer.className = "tags";
document.body.appendChild(tagLayer);

// Context flags (experimental options) apply when the context is created, i.e. on reload.
// antialias: the browser's MSAA on the canvas, only for r_aa auto; every other mode renders
// offscreen (engine/aa.js), so it changes live, but the canvas's own buffer stays until a reload.
const ctxFlags = { desynchronized: !!settings.get().r_desynchronized, powerPreference: settings.get().r_power_preference, antialias: settings.get().r_aa === "auto" };
const look = { palette: PALETTE, materials: MATERIALS, sky: SKY, sun: SUN_DIR };
const gfx = createRenderer(canvas, look, ctxFlags);
const mapCatalog = createMapCatalog(look);
const phys = createPhysics();
const input = createInput(canvas, () => settings.get().m_rawinput);
const remotes = createRemotes();

let replay = null; // the replay playing (playReplay), or null: the server's game
let map = null, me = null, myId = 0, myColor = "#fff", myTeam = "", spawnSeq = 0, wantPlay = false;
const known = new Map(); // id -> {name, team}, for join/leave/switch feed
const joinedText = (name, t) => t === SPECTATOR ? `${name} is spectating` : `${name} joined the ${teamInfo(t)?.name ?? "server"}`;
const spectating = () => myTeam === SPECTATOR;
let marksFor = null, marks = []; // spawnMarks(map), rebuilt on a map change
const friendlyFire = () => serverCvars.mp_friendlyfire === "1"; // server cvar (rcon mp_friendlyfire 1)
const bodies = (list) => list.filter((r) => r.team !== SPECTATOR); // spectators have no body
const alive = (list) => bodies(list).filter((r) => !r.dead); // the dead are corpses (corpse.js): drawn, never hit
const corpseAge = new Map(); // id -> death age last frame (s), for the dust puff on landing
let myCorpse = null; // our own body while we're dead, from our entry in the snapshot
// Watching instead of playing: spectating, or dead (the death cam, then following someone).
const observing = () => spectating() || !!me?.dead;
const remoteFx = new Map(); // id -> {flash, last, stride, speed, crouch}
const puffs = [];
const damageNumbers = [];
const bursts = []; // slop burst particles at confirmed hits (hitfx.js)
// Hits we took, for the vignette and the flinch: {at (fxClock s), dmg, p (attacker eye), a0}.
const hurtHits = [];
let fxClock = 0; // seconds, advanced by frame dt (so stepped test frames are exact)
// Our recent shots by command seq (client-predicted hit), matched with the server's "fired".
const shotLog = new Map();
// sv_showimpacts: per shot, what we saw (red) and what the server traced (blue), for 2 s.
const impacts = [];
const HITBOX_COLORS = { head: [1, 0.2, 0.2], body: [1, 0.85, 0.2], legs: [0.3, 0.8, 1], gun: [0.6, 0.6, 0.6] };
let items = []; // weapons lying on the map, from snapshots
let mapLo = [0, 0, 0], mapHi = [0, 0, 0]; // MapBounds of the map (WallTrace: its shell is never shot through)
let clickView = null; // [yaw, pitch] at the last fire press, until its command takes it (subtick shots)
const announcer = createAnnouncer();
const music = createMusic();
const musicDir = createMusicDirector({ music });
let warmedUp = false;
function startAudio() {
  sfx.initAudio();
  if (!warmedUp) { warmedUp = true; voice.warmup(); }
}
// One AudioContext per page; browsers only start (or resume) it in a user
// gesture. Every gesture anywhere on the page does it, so audio comes back
// after any reload (new version, renderer restart: the page auto-rejoins
// without the Play button) or a suspension, whatever gets clicked first.
for (const ev of ["pointerdown", "keydown", "touchstart"]) {
  addEventListener(ev, () => { if (sfx.audioState() !== "running") startAudio(); }, { capture: true });
}
let serverCvars = {}; // mp_*/sv_* from the server, plus openAdmin

const hud = mountHud(document.getElementById("hud"), {
  play(name) {
    startAudio();
    // Leave the main menu whatever the lock does; without it the crosshair says "click to aim".
    hud.set({ screen: "playing" });
    startPlay(name); // may open team select at once (the observer socket is up): then no lock
    lockIn();
  },
  aim() { lockIn(); },
  replayFiles(files) { replayFiles(files); },
  replayList() { hud.set({ myRecordings: recorder.list() }); replayList(); },
  playRecording(i) { playRecording(i); },
  saveRecording(i) { saveRecording(i); },
  replayServerRound(match, ri) { replayServerRound(match, ri); },
  get replay() { return replayCtl; }, // the bar's buttons (defined below)
  crouchNotice(moveToC) { answerCrouchNotice(moveToC); },
  leave() { if (replay) stopReplay(); else leaveMatch(); },
  // The connection-lost overlay's buttons.
  retryNow() { net.retryNow(); },
  reloadPage() { skipLeaveConfirm = true; location.reload(); },
  teamMenu() { openTeamMenu(); },
  pickTeam(t) { joinTeam(t); hud.set({ teamMenu: false, screen: "playing" }); lockIn(); },
  closeTeamMenu() { hud.set({ teamMenu: false }); },
  buy(item) { net.send({ t: "buy", item }); },
  buyAmmo(item) { buyAmmo(item); }, // the buy menu's ammo cards: = , and .
  closeBuyMenu() { closeBuyMenu(); },
  say(text, team) { net.send({ t: "say", text, tm: !!team }); },
  closeChat() { hud.set({ chatOpen: false }); input.lock(); },
  radio(rid) { net.send({ t: "radio", rid }); },
  closeRadio() { hud.set({ radioOpen: false }); },
  // Back to the running match. Esc isn't user activation, so pointer lock may be
  // refused here; then the crosshair says "click to aim" until the next click.
  closeMenu() {
    hud.set({ screen: "playing" });
    input.lock();
  },
  // Maps screen: the catalog, then each preview as it renders.
  async maps() {
    try {
      hud.set({ mapCards: await mapCatalog.cards() });
      await wantPreviews(hud.get().mapCards.map((c) => c.name));
    } catch { hud.set({ mapCards: [] }); }
  },
  // Picked with no connection at all (the server is down or restarting):
  // remembered and sent on the next welcome.
  changeLevel(name) {
    if (net.open) return con.exec(`changelevel ${name}`);
    hud.set({ pendingMap: name });
  },
  vote(name) { castVote(name); },
  // Server settings: one rcon cvar command (the server checks admin, applies and announces it).
  rcon(cmd, value) { net.send({ t: "rcon", cmd, args: [String(value)] }); },
  // The map names for the rotation checklist (no previews).
  async mapList() {
    if (hud.get().mapCards?.length) return;
    try { hud.set({ mapCards: await mapCatalog.cards() }); } catch { hud.set({ mapCards: [] }); }
  },
  loop: () => loop,
  latency: () => ({ lines: latencyReadout(), info: gfx.info(), requested: ctxFlags }),
  gfxInfo: () => ({ info: gfx.info(), requested: ctxFlags }),
  // Context flags changed: reload into the match (the leave prompt is skipped).
  reloadPage() {
    try { sessionStorage.setItem("cs.rejoin", JSON.stringify({ name: hud.get().name, inGame: wantPlay })); } catch {}
    skipLeaveConfirm = true;
    location.reload();
  },
});

// Map previews (rendered on the client from /maps.json) into hud.mapPreviews as they finish.
async function wantPreviews(names) {
  for (const n of names) {
    if (hud.get().mapPreviews?.[n]) continue;
    const url = await mapCatalog.preview(n);
    if (url) hud.set({ mapPreviews: { ...hud.get().mapPreviews, [n]: url } });
  }
}
// Vote by map name or option number (1-3); false when there's no such option.
function castVote(v) {
  const vote = hud.get().vote;
  if (!vote || vote.done) return false;
  const name = vote.options[+v - 1] || v;
  if (!vote.options.includes(name)) return false;
  net.send({ t: "vote", map: name });
  return true;
}

// Capture the mouse, first going fullscreen with keyboard lock if that option is on.
let warnedNoKbLock = false;
// Pointer lock is requested first, synchronously inside the click: requestFullscreen
// consumes the click's user activation, so asking for the lock after it (as we
// used to) was refused on the first Play and bounced back to the menu.
let lockingUntil = 0; // the fullscreen switch may drop the lock briefly; don't treat that as Esc
// Without keyboard lock (Firefox, Safari) Ctrl+W, T and N belong to the browser
// even in fullscreen, so crouch-walking on Ctrl closes the tab (it asks first).
// Entering play there with crouch on Ctrl asks once: move crouch to C, or keep
// Ctrl (remembered in this browser). The buttons' click captures the mouse.
const KEEP_CTRL = "cs.keepCtrlCrouch";
const kbLockMissing = () => testCtl.noKbLock ?? !hasKeyboardLock();
function crouchNoticeDue() {
  let kept = false;
  try { kept = localStorage.getItem(KEEP_CTRL) === "1"; } catch { /* private window: ask again next time */ }
  return !menuWantsCursor(hud.get()) && needsCrouchNotice({ kbLock: !kbLockMissing(), duckKey: keyFor("+duck"), kept: kept || crouchNoticeAnswered });
}
let crouchNoticeAnswered = false;
function answerCrouchNotice(moveToC) {
  crouchNoticeAnswered = true;
  if (moveToC) settings.set({ binds: bindKey(settings.get().binds, "KeyC", "+duck").binds });
  else try { localStorage.setItem(KEEP_CTRL, "1"); } catch { /* this page only */ }
  hud.set({ crouchNotice: false });
  lockIn();
}
function lockIn() {
  if (crouchNoticeDue()) { hud.set({ crouchNotice: true }); return; }
  input.lock();
  if (!settings.get().cl_fullscreen || document.fullscreenElement) return;
  lockingUntil = performance.now() + 1500;
  input.enterFullscreen().then((ok) => {
    if (!ok && !warnedNoKbLock) {
      warnedNoKbLock = true;
      hud.feed(`No fullscreen keyboard lock: Ctrl+W asks before leaving${/^Control/.test(keyFor("+duck")) ? " (Options → Controls: crouch on another key)" : ""}`);
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

// Join the match: say hello on the observer socket the menus already hold
// (or connect, if it isn't up), which makes us a player (team select next).
function startPlay(name) {
  const joining = !wantPlay;
  wantPlay = true;
  if (!net.open) return net.connect(name);
  net.hello(name);
  if (joining && me && spectating()) openTeamMenu();
}

// In the menus the client stays connected as a hidden observer (no hello:
// the server leaves it out of snapshots, teams and rounds), so the Maps
// screen's changelevel, the current map and the admin state work before PLAY.
function observeFromMenu() {
  if (!wantPlay && !net.open) net.connect(hud.get().name || "slopper", false);
}

// observe: back to a hidden observer (the menus' Leave); false = off the
// server entirely (console disconnect, a kick).
function leaveMatch(observe = true) {
  wantPlay = false;
  net.disconnect();
  if (document.fullscreenElement) document.exitFullscreen().catch(() => {});
  document.exitPointerLock?.();
  me = null; myId = 0;
  hud.set({ screen: "menu", connected: false, players: [] });
  if (observe) observeFromMenu(); // the menus still reach the server
}

// ---- replays (docs/replays.md): a recorded round, played through the game's own handler ----
// A replay leaves the server (the menus' observer socket too) and plays as a
// spectator would see the round: the observer cameras, HUD, kill feed and
// sounds, with the remotes drawn at the replay's playhead. list: the rounds to
// step through with prev/next, [{name, load: () => Promise<bytes of .cstv.gz>}].
async function playReplay(list, index = 0) {
  const entry = list[index];
  if (!entry) return false;
  let rep;
  try {
    const { bytes, complete } = await gunzip(await entry.load());
    rep = readReplay(bytes);
    rep.complete = rep.complete && complete;
    if (!rep.snaps.length) throw new Error("no snapshots in it (the round had just started)");
  } catch (e) {
    const why = `${entry.name}: ${e.message || e}`;
    hud.set({ replayError: why });
    con.print(`replay ${why}`, "warn");
    return false;
  }
  if (!replay) { // from the menus or a match: off the server for the replay
    wantPlay = false;
    net.disconnect();
  }
  replay = { list, index, player: null, key: "" };
  myId = -1; myTeam = SPECTATOR; spawnSeq = 0; myCorpse = null; lastRound = null;
  me = newBody([0, 0, 0]);
  loadMap(rep.header.map);
  simDt = 1 / (rep.header.simHz || 100); simAcc = 0;
  const notes = replayNotes(rep.header, { complete: rep.complete, schemaHash: NET_SCHEMA_HASH, clientVersion });
  replay.player = createReplayer(rep, { deliver: onServerMsg, reset: replayReset });
  remotes.setPlayhead(replay.player.tick);
  observer.reset();
  if (rep.header.pov) { observer.setMode("first"); observer.target = rep.header.pov; } // a client's recording: through its player's eyes
  const names = new Map((rep.header.players || []).map((p) => [p.id, p]));
  const p = replay.player, span = Math.max(1, p.last - p.first);
  replay.marks = p.markers().map((m) => ({ ...m, at: (m.tick - p.first) / span,
    kname: names.get(m.k)?.name ?? "", vname: names.get(m.v)?.name ?? "", kteam: names.get(m.k)?.team ?? "", vteam: names.get(m.v)?.team ?? "" }));
  hud.set({ screen: "playing", myId, myTeam, map: map.name, players: [], teamMenu: false, buyMenu: false, scoreboard: false, replayError: null,
    replayMarks: replay.marks, replayNotes: notes, replay: replayView() });
  for (const n of notes) hud.feed(n);
  con.print(`replay ${roundLabel(rep.header)} (${entry.name})${notes.length ? ": " + notes.join("; ") : ""}`, "dim");
  return true;
}
// What the HUD shows from the round being watched: gone on a seek (the
// snapshots and events bring back what applies) and when the replay ends.
const ROUND_HUD = { feed: [], roundEnd: null, dmgReport: null, halftime: null, matchEnd: null, shout: null, subtitle: null, work: null };
// A seek: the world is rebuilt from the snapshots before the target.
function replayReset() {
  lastSnapTick = testCtl.snapTick = -1; lastRound = null;
  remotes.clear(); known.clear();
  bursts.length = 0; damageNumbers.length = 0; impacts.length = 0; items = [];
  hud.set(ROUND_HUD);
}
function stopReplay(toMenu = true) {
  if (!replay) return;
  replay = null;
  remotes.setPlayhead(null); remotes.clear(); known.clear();
  me = null; myId = 0; myTeam = ""; lastSnapTick = testCtl.snapTick = -1;
  if (input.locked) document.exitPointerLock?.();
  hud.set({ ...ROUND_HUD, replay: null, replayMarks: null, replayNotes: null, players: [], observe: null, ...(toMenu ? { screen: "menu" } : {}) });
  if (toMenu) observeFromMenu(); // back to the server for the menus
}
// The bar's state (hud ReplayBar): time, speed, paused, which round of the list.
function replayView() {
  const p = replay.player;
  return { label: roundLabel(p.header), name: replay.list[replay.index].name, index: replay.index, count: replay.list.length,
    t: p.time, len: p.length, speed: p.speed, paused: p.paused, ended: p.ended, cam: observer.mode };
}
// Per frame: run the playhead, draw there, and refresh the bar (10 Hz of its clock).
function tickReplay(dt) {
  const p = replay.player;
  p.update(dt);
  remotes.setPlayhead(p.tick);
  if (p.paused && lastRound) rounds.ingest(lastRound, performance.now()); // the round clock holds too
  const v = replayView(), key = `${Math.floor(v.t * 10)}|${v.speed}|${v.paused}|${v.index}|${v.cam}|${v.ended}`;
  if (key !== replay.key) { replay.key = key; hud.set({ replay: v }); }
}
// The bar's buttons, the replay keys and the console's replay command.
const replayCtl = {
  pause() { replay?.player.togglePause(); },
  seek(frac) { if (replay) { const p = replay.player; p.seek(p.first + frac * (p.last - p.first)); } },
  skip(s) { replay?.player.skip(s); },
  speed(dir) { replay?.player.stepSpeed(dir); },
  round(dir) { if (replay && replay.list[replay.index + dir]) playReplay(replay.list, replay.index + dir); },
  cam() { observer.cycleMode(); },
  next(dir) { const ps = remotes.sample(performance.now()); dir > 0 ? observer.next(ps) : observer.prev(ps); },
  stop() { stopReplay(); },
};
// Files dropped on the page or picked in the Replays screen, in name order
// (a match's rounds: round-01, round-02, ...).
function replayFiles(files) {
  const list = [...files].sort((a, b) => a.name.localeCompare(b.name, undefined, { numeric: true }))
    .map((f) => ({ name: f.name, load: async () => new Uint8Array(await f.arrayBuffer()) }));
  if (list.length) { startAudio(); playReplay(list, 0); }
}
addEventListener("dragover", (e) => { if ([...(e.dataTransfer?.items || [])].some((i) => i.kind === "file")) e.preventDefault(); });
addEventListener("drop", (e) => {
  if (!e.dataTransfer?.files?.length) return;
  e.preventDefault();
  replayFiles(e.dataTransfer.files);
});
// The server's recorded rounds (/replays.json, docs/replays.md): matches,
// newest first, each with its rounds. tv_access admin: our admin token.
let tvToken = "";
const withToken = (url) => (tvToken ? `${url}${url.includes("?") ? "&" : "?"}token=${encodeURIComponent(tvToken)}` : url);
async function replayList() {
  hud.set({ replayMatches: null, replayListError: null });
  try {
    const r = await fetch(withToken("/replays.json"), { cache: "no-store" });
    if (r.status === 401 || r.status === 403) throw new Error("this server's replays are for admins (admin_login in the console)");
    if (r.status === 404) throw new Error("this server records no replays");
    if (!r.ok) throw new Error(`${r.status} ${r.statusText}`);
    hud.set({ replayMatches: (await r.json()).matches || [] });
  } catch (e) {
    hud.set({ replayMatches: [], replayListError: e.message || String(e) });
  }
}
// Play round ri of a listed match; prev/next step through its other rounds.
function replayServerRound(match, ri) {
  startAudio();
  playReplay(match.rounds.map((r) => replayURL(withToken(r.url), `${match.map} ${match.id} round ${r.round}${r.live ? " (live)" : ""}`)), ri);
}
// From a URL (the console's replay open, the server's list).
const replayURL = (url, name = decodeURIComponent(url.split("?")[0].split("/").pop() || url)) => ({ name, load: async () => {
  const r = await fetch(url, { cache: "no-store" }); // a live round grows: always the newest
  if (!r.ok) throw new Error(`${r.status} ${r.statusText}`);
  return new Uint8Array(await r.arrayBuffer());
} });
// The replay keys (fixed, not binds: they only mean something in a replay).
const REPLAY_KEYS = { KeyP: () => replayCtl.pause(), ArrowLeft: () => replayCtl.skip(-5), ArrowRight: () => replayCtl.skip(5),
  ArrowUp: () => replayCtl.speed(1), ArrowDown: () => replayCtl.speed(-1), PageUp: () => replayCtl.round(-1), PageDown: () => replayCtl.round(1),
  Home: () => replayCtl.seek(0), End: () => replayCtl.seek(1) };

// Every message from the server, and in a replay from the file (replayer.js):
// the same handler, so a replay draws exactly what a spectator saw.
function onServerMsg(m) {
  const now = performance.now();
  switch (m.t) {
    case "open": hud.set({ connected: true }); con.print(`connected to ${location.host}`, "dim"); break;
    case "close":
      hud.set({ connected: false, admin: false }); // admin_login lasts one connection; the next cvars message says again
      if (replay) break; // the socket a replay closed: the replay's world stays
      con.print("connection lost", "warn");
      remotes.clear(); known.clear();
      break;
    case "welcome":
      lastSnapTick = testCtl.snapTick = -1; lastAck = -1; // a new connection (maybe a restarted server): ticks start over
      if (checkBuild(m)) return;
      myId = m.id; myColor = m.color; myTeam = m.team; spawnSeq = 0;
      loadMap(m.map);
      me = newBody(m.spawn);
      me.yaw = Math.atan2(m.spawn[0], m.spawn[2]); // face the middle
      if (m.simHz > 0) simDt = 1 / m.simHz;
      simAcc = 0; vmCur = vmPrev = null; snapView();
      hud.set({ myId, myColor, myTeam, map: map.name });
      con.print(`map ${map.name}, you are #${myId}`, "dim");
      { const pm = hud.get().pendingMap; if (pm) { hud.set({ pendingMap: null }); if (pm !== map.name) con.exec(`changelevel ${pm}`); } }
      if (spectating() && wantPlay) openTeamMenu(); // the team-select screen on join
      break;
    case "inv":
      arsenal.setInv(m.inv);
      break;
    case "snap": {
      // Snapshots are full state: an older one than we have is just dropped.
      if (m.tick !== undefined) { if (m.tick <= lastSnapTick) break; lastSnapTick = testCtl.snapTick = m.tick; } // snapTick: lockstep cases wait for it
      // A pose snapshot (two of every three): the players' poses and our own
      // full state only; everyone's status, items and the round come with the full ones.
      if (m.pose) m.players = unpackPoses(m.players);
      else lastRound = m.round;
      link.snap(linkNow());
      remotes.ingest(m, myId, now);
      predictor.setPeers(peersFrom(m.players, myId, serverCvars.mp_solid_players !== "0"));
      netDebug.snap(m, performance.now());
      if (m.me && me && !spectating()) { // the server's word on our own body
        testCtl.lastMe = m.me;
        if (m.me.ack !== undefined) lastAck = m.me.ack;
        predictor.reconcile(me, m.me, simDt, { watch: !observing() });
        // A weapon correction can change the replayed commands' speed (the drink): replay the body again.
        if (m.me.w && reconcileWeapons(m.me.ack, m.me.w)) predictor.reconcile(me, m.me, simDt, { watch: !observing() });
        me.dead = !!m.me.dead;
        if (m.me.hp !== hud.get().hp) hud.set({ hp: m.me.hp });
        if (m.me.team !== undefined && m.me.team !== myTeam) { myTeam = m.me.team; hud.set({ myTeam }); }
        if (m.me.money !== undefined && m.me.money !== hud.get().money) hud.set({ money: m.me.money });
        const gear = { armor: m.me.armor || 0, helmet: !!m.me.helmet, kit: !!m.me.kit, shades: !!m.me.shades, boost: Math.ceil((m.me.w?.bo || 0) / 100), canBuy: !!m.me.buy, warmup: !lastRound || lastRound.ph === "warmup" };
        const hs = hud.get();
        if (gear.armor !== hs.armor || gear.helmet !== hs.helmet || gear.kit !== hs.kit || gear.shades !== hs.shades || gear.boost !== hs.boost || gear.canBuy !== hs.canBuy || gear.warmup !== hs.warmup) hud.set(gear);
        if (!gear.canBuy && hs.buyMenu) closeBuyMenu(); // left the zone or buy time ran out
        nades.flashed(m.me.fl || 0, m.me.fld || 0, now);
      }
      if (m.pose) break;
      items = m.items || [];
      nades.ingest(m.nades, now);
      spot = new Set(m.spot || []);
      if (m.round) rounds.ingest(m.round, now);
      const ids = new Set();
      for (const p of m.players) {
        if (p.id === myId) continue;
        ids.add(p.id);
        const k = known.get(p.id);
        if (!k && replay) { /* a replay: who plays is in the bar; no join line per seek */ }
        else if (!k) { hud.feed(p.team === SPECTATOR || !p.team ? `${p.name} slid into the server` : joinedText(p.name, p.team)); sfx.honk(); }
        else if (k.team !== p.team && !replay) hud.feed(joinedText(p.name, p.team));
        known.set(p.id, { name: p.name, team: p.team });
      }
      for (const [id, k] of known) if (!ids.has(id)) { known.delete(id); if (!replay) hud.feed(`${k.name} rage-quit`); }
      const players = m.players.filter((p) => p.id !== myId).map(({ id, name, color, team, k, d, a, sp, dead, ping, ai, bot }) => ({ id, name, color, team, k: k || 0, d: d || 0, a: a || 0, sp: sp || 0, dead: !!dead, ping: ai || bot ? "BOT" : ping || null, ...(ai ? { ai: true } : {}) }));
      if (JSON.stringify(players) !== JSON.stringify(hud.get().players)) hud.set({ players });
      const mine = m.players.find((p) => p.id === myId);
      myCorpse = mine?.dead && mine.dk ? { id: myId, team: mine.team, color: mine.color, p: mine.p, yaw: mine.yaw, dk: mine.dk, fy: mine.fy || 0, dv: !!mine.dv } : null;
      if (mine && (!!mine.dead !== !!hud.get().myDead || (mine.k || 0) !== hud.get().myK || (mine.d || 0) !== hud.get().myD || (mine.a || 0) !== hud.get().myA || (mine.sp || 0) !== hud.get().mySp)) {
        hud.set({ myDead: !!mine.dead, myK: mine.k || 0, myD: mine.d || 0, myA: mine.a || 0, mySp: mine.sp || 0 });
      }
      break;
    }
    case "hit": { // server-confirmed hit: sound, slop burst, console line (never on a prediction); no hit marker
      testCtl.lastHit = m;
      const head = m.part === "head";
      if (!replay) (head ? sfx.hitHead : m.ar ? sfx.hitArmor : sfx.hitBody)(); // a replay's hits are everyone's: no hit sound
      const vteam = known.get(m.id)?.team;
      const from = me && !spectating() ? eyePos() : m.at.map((v, k) => v - [0, 0, 1][k]);
      bursts.push(...slopBurst(hitSeed(m.id, m.at), m.at, m.at.map((v, k) => v - from[k]), teamInfo(vteam)?.rgb ?? [0.8, 0.8, 0.8], { head }));
      if (bursts.length > 240) bursts.splice(0, bursts.length - 240);
      if (settings.get().cl_damage_numbers) damageNumbers.push({ at: m.at, text: `${m.dmg}`, part: m.part, life: 1.2 });
      { const w = WEAPONS[m.w];
        if (w?.hitGlyph) damageNumbers.push({ at: [m.at[0], m.at[1] + 0.35, m.at[2]], text: w.hitGlyph, part: "glyph", life: 0.9 });
        if (w?.hitText) damageNumbers.push({ at: [m.at[0], m.at[1] + 0.6, m.at[2]], text: w.hitText, part: "review", life: 1.6 }); }
      if (!replay) con.print(`hit ${m.name}: ${m.part} for ${m.dmg}`, "dim");
      break;
    }
    case "notice":
      hud.feed(m.text);
      break;
    case "kicked":
      leaveMatch(false);
      hud.feed("You were kicked by an admin.");
      break;
    case "chat": // someone said something (an event: losing one loses a line, nothing else)
      addChat({ ...m, key: Math.random(), at: performance.now() });
      con.print(`${chatPrefix(m)}${m.name}: ${m.text}`);
      break;
    case "radio": { // a teammate's callout: say it in the radio voice, log it
      const r = voice.RADIO_BY_ID[m.id];
      if (!r) break;
      addChat({ name: m.name, team: myTeam, text: r.text, radio: true, key: Math.random(), at: performance.now() });
      voice.sayPhonemes(r.ph, "radio");
      hud.set({ radioPing: { p: m.p, at: performance.now(), from: m.from } });
      break;
    }
    case "bought": // the server's answer to a buy (the state itself is in the snapshots)
      hud.set({ buyMsg: m.ok ? "" : m.why, buyOk: m.ok ? m.item : "" }); // a line in the menu: bought X / why not
      if (!m.ok) sfx.click?.();
      break;
    case "money": // a reward or penalty, with the reason (the total rides in every snapshot)
      hud.set({ money: m.total, moneyPop: { d: m.d, why: m.why, at: performance.now() } });
      con.print(`${m.d > 0 ? "+" : "−"}$${Math.abs(m.d)} ${m.why}`, "dim");
      break;
    case "fired": // what the server made of one of our shots (cl_netdebug, cl_debug_shots)
      netDebug.fired(m, performance.now());
      if (settings.get().cl_debug_shots) {
        const c = shotLog.get(m.s)?.hit;
        con.print(`shot #${m.s}  client: ${c ? `${c.region} (${c.name}) t=${c.t.toFixed(2)}` : "miss"}  server: ${m.r || "?"}`, c && !String(m.r).includes(c.region) ? "warn" : "dim");
      }
      break;
    case "impacts": // sv_showimpacts: the poses the server traced our shot against (blue)
      testCtl.lastImpacts = m;
      impacts.push({ life: 2, color: [0.3, 0.55, 1], end: m.end, poses: m.poses.map((r) => ({ p: r.p, yaw: r.yaw, pitch: r.pitch, crouch: !!r.c, team: r.team, id: r.id, gait: gaitFromWire(r.g), t: r.t })) });
      if (impacts.length > 12) impacts.shift();
      break;
    case "sync": // test harness (lockstep cases): everything sent before this has arrived
      testCtl.sync = m.n;
      break;
    case "help": // rcon help: structured rows
      con.printSections(m.sections);
      break;
    case "print": // server text for the console only (rcon output, motd)
      con.print(m.text);
      break;
    case "cvars": // server cvars (admin-set); sv_gravity etc. apply with the authoritative sim
      serverCvars = { ...m.cvars, openAdmin: m.openAdmin };
      tvToken = m.tvToken || ""; // replay downloads under tv_access admin (the Replays screen)
      applySpeeds(serverCvars); // weapon_speed_<id>: the same move speeds the server runs (prediction stays exact)
      applyRules();
      // admin: this player may change them (the menu's Server settings); help comes with it
      hud.set({ openAdmin: !!m.openAdmin, admin: !!m.admin, cvars: m.cvars, cvarHelp: m.help ?? null });
      break;
    case "map": // changelevel
      loadMap(m.map);
      break;
    case "mapvote": // end-of-match map vote: options + live counts, then the winner
      if (m.done) {
        hud.set({ vote: { ...(hud.get().vote || { options: [m.winner] }), votes: m.votes, done: true, winner: m.winner } });
        setTimeout(() => { if (hud.get().vote?.done) hud.set({ vote: null }); }, 6000);
      } else {
        if (!hud.get().vote || hud.get().vote.done) con.print(`map vote: ${m.options.map((o, i) => `${i + 1} ${o}`).join(" · ")} (F1-F3 or vote <n>)`);
        hud.set({ vote: { options: m.options, votes: m.votes, mine: m.mine || "", endsAt: performance.now() + m.endsIn * 1000 } });
        wantPreviews(m.options);
      }
      break;
    case "spawn": { // (re)spawn after a death or a team switch; spectators get no body
      const switched = m.team !== undefined && m.team !== myTeam;
      if (m.team !== undefined) myTeam = m.team;
      if (m.sp !== undefined) spawnSeq = m.sp;
      if (me && !spectating()) { me.pos = [...m.p]; me.vel = [0, 0, 0]; me.onGround = true; me.grav = 0; roll.snap(0); // = the server respawn (a new body stands in the world's gravity)
         me.yaw = m.yaw ?? Math.atan2(m.p[0], m.p[2]); me.dead = false; predictor.reset(); predictor.setTickBase(m.tb); arsenal.respawned(); prevButtons = 0; snapView(); }
      hud.set({ myTeam, ...(m.hp !== undefined && !spectating() ? { hp: m.hp } : {}) });
      observer.reset(); // alive again (or a fresh spectator): no death cam
      if (switched) hud.feed(joinedText(hud.get().name, myTeam));
      break;
    }
    case "setpos": // admin setpos moved us after command m.ack (= server setPos)
      if (me && !spectating() && !me.dead) { me.yaw = m.yaw; predictor.moved(me, m, simDt); snapView(); }
      break;
    case "teamfull": // the server refused: that side is at the map's cap (m.cap per team), or would be more than mp_limitteams players bigger
      if (!m.team) hud.feed(`Both sides are full: ${m.map} has room for ${m.cap} per team. Spectate until a slot opens.`);
      else if (m.cap) hud.feed(`The ${teamInfo(m.team)?.name ?? m.team} are full: ${m.map} has room for ${m.cap} per team.`);
      else hud.feed(`The ${teamInfo(m.team)?.name ?? m.team} already have too many players over the other side. Pick that one.`);
      openTeamMenu();
      break;
    case "hurt": // the server damaged us (optional event: HP itself is in every snapshot)
      hud.set({ hp: m.hp });
      (m.ar ? sfx.hurtArmor : sfx.hurtFlesh)();
      if (m.fp) dmgFrom.push({ p: m.fp, at: performance.now(), dmg: m.dmg });
      if (me && !spectating()) { // the red edge from that side + a view-only flinch
        hurtHits.push({ at: fxClock, dmg: m.dmg, p: m.fp, a0: m.fp ? relAngle(me.pos, me.yaw, m.fp) : Math.PI });
        if (hurtHits.length > 8) hurtHits.shift();
      }
      break;
    case "kill": { // kill feed; real kills drive the announcer
      const nameOf = (id) => (id === 0 ? "" : id === myId ? hud.get().name : known.get(id)?.name ?? `#${id}`); // 0: the world (a blast)
      const teamOf = (id) => (id === myId ? myTeam : known.get(id)?.team);
      const wname = WEAPONS[m.w]?.name ?? (m.w === "bigdiff" ? "The Big Diff" : m.w);
      hud.feed(`${m.k ? nameOf(m.k) + " " : ""}[${wname}${m.ns ? " noscope" : ""}${m.pn ? " penetrated" : ""}${m.hs ? " ✚head" : ""}] ${nameOf(m.v)}`, {
        killer: nameOf(m.k), victim: nameOf(m.v), kteam: teamOf(m.k), vteam: teamOf(m.v),
        weapon: m.w, wname, hs: !!m.hs, ns: !!m.ns, pn: !!m.pn, mine: m.k === myId || m.v === myId,
      });
      if (m.v === myId && me) {
        me.dead = true; hud.set({ hp: 0 });
        // Death cam: freeze on the killer, then follow a teammate until the respawn.
        observer.died({ killer: m.k === myId ? 0 : m.k, eye: eyePos(), yaw: me.yaw, pitch: me.pitch, weapon: m.w, team: myTeam });
      }
      if (m.v === myId) { announcer.onDeath(); hurtHits.length = 0; } // ends the spree; no red edge on the death cam
      if (m.k === myId && m.v !== myId) sfx.killConfirm(!!m.hs); // our kill, confirmed
      if (m.k === myId && m.v !== myId && teamOf(m.v) !== myTeam) { // a team kill is no streak
        announcer.onKill({ headshot: !!m.hs });
        musicDir.kill({ multi: announcer.state.multi, headshot: !!m.hs });
      }
      break;
    }
    case "nade": // grenade events: throw, bounce, he, flash, smoke (the grenades themselves ride in snapshots)
      nades.event(m, performance.now());
      break;
    case "dmgreport": { // end of round: our damage given/taken per opponent (optional, like CS's console report)
      const rows = m.rows || [];
      hud.set({ dmgReport: rows.length ? rows : null });
      con.print(`---------- Round ${m.n}: damage ----------`, "dim");
      const hits = (n) => `${n} hit${n === 1 ? "" : "s"}`;
      for (const r of rows) {
        if (r.gh) con.print(`Damage Given to "${r.name}" - ${r.g} in ${hits(r.gh)}`);
        if (r.th) con.print(`Damage Taken from "${r.name}" - ${r.t} in ${hits(r.th)}`);
      }
      if (!rows.length) con.print("No damage given or taken.", "dim");
      break;
    }
    case "round": // round events: start, live, pickup, dropped, planted, defused, exploded, end, warmup, halftime, swap, matchend
      if (m.ev === "swap") { // halftime: everyone changes sides at once; no "joined the ..." line per player
        const other = { vibe: "artisan", artisan: "vibe" };
        for (const k of known.values()) if (other[k.team]) k.team = other[k.team];
        if (other[myTeam]) { myTeam = other[myTeam]; hud.set({ myTeam }); } // our own respawn on the other side isn't a "joined" either
      }
      rounds.event(m, performance.now());
      break;
    case "gpu": // GPU Drop: grab / drop / restock / secure (sounds, lines, feed; the state is in the snapshots)
      rounds.gpuEvent(m);
      break;
    case "correct":
      if (me) { me.pos = [...m.p]; me.vel = [0, 0, 0]; snapView(); }
      break;
    case "shot": {
      const fx = fxFor(m.id);
      fx.flash = 0.06;
      remoteMuzzleLight(m.id, m.w);
      if (fx.last) (FIRE_SOUND[m.w] || sfx.pew)(spatial(fx.last));
      debris.remoteShot(m, WEAPONS[m.w], remotes.sample(performance.now()).find((r) => r.id === m.id));
      break;
    }
  }
}
// While a replay plays the server's messages are not the world (the socket is closed; a late one is dropped).
const net = createNet((m) => {
  if (replay && m.t !== "open" && m.t !== "close") return;
  if (!replay) recordMsg(m);
  onServerMsg(m);
});
// Client-side recording (recorder.js, cl_autorecord): the rounds we play, from
// what we received, before the handler sees it. Not in the menus (the hidden
// observer), not while watching a replay.
const recorder = createRecorder();
let recordedRounds = 0;
function recordMsg(m) {
  if (!wantPlay || !settings.get().cl_autorecord) { recorder.stop(); return; }
  const ctx = { map, version: clientVersion, myId, simHz: Math.round(1 / simDt) };
  if (m.t === "snap") recorder.snap(m, ctx);
  else if (m.t === "welcome" || m.t === "map" || m.t === "close") recorder.stop(); // a new connection or map: that round's file ends
  else recorder.event(m, ctx);
  const n = recorder.list().length;
  if (n !== recordedRounds) { recordedRounds = n; hud.set({ myRecordings: recorder.list() }); }
}
// Watch one of ours (prev/next: the others), or save it as a .cstv.gz.
function playRecording(i) {
  const list = recorder.list().map((x, j) => ({ name: x.name, load: () => recorder.file(j) }));
  startAudio();
  playReplay(list, i);
}
async function saveRecording(i) {
  const x = recorder.list()[i];
  if (!x) return;
  const gz = await recorder.file(i);
  const url = URL.createObjectURL(new Blob([gz], { type: "application/gzip" }));
  const a = Object.assign(document.createElement("a"), { href: url, download: x.name });
  document.body.appendChild(a); a.click(); a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 10000);
  testCtl.lastDownload = { name: x.name, bytes: gz }; // tests: what was saved
  con.print(`saved ${x.name} (${(gz.length / 1024).toFixed(1)} KB)`, "dim");
}
const predictor = createPredictor(phys);
const arsenal = createArsenal({ net, hud, zoomMode: () => settings.get().zoom_mode });
const observer = createObserver({ raycast: (o, d, maxT) => phys.raycast(o, d, maxT) });
const nades = createNades({ sfx, spatial, myId: () => myId });
const radar = createRadar();
// Bullet holes and casings for the round, with their sounds (anyone's, near us).
const debris = createDebris({
  sfx, spatial: (p) => spatial(p), listener: () => (me ? me.pos : null), cfg: () => settings.get(),
  boxes: () => (map ? phys.solids(moverTime(performance.now(), 0)) : []), raycast: (o, d, maxT) => phys.raycast(o, d, maxT), // casings bounce on the steps we see
});
let spot = new Set(); // enemies our team sees now (the server's list in each snapshot)
// Full-screen tint over the world (not the HUD): the flash whiteout, the inside of a smoke.
const screenFx = document.createElement("div");
screenFx.className = "screenfx";
document.getElementById("hud")?.before(screenFx);
let screenFxKey = "";
const rounds = createRounds({
  hud, sfx, say: (id) => voice.say(id), announcer, spatial, music: musicDir,
  feed: (t) => hud.feed(t),
  me: () => ({ id: myId, team: myTeam, pos: me && !spectating() ? me.pos : null, dead: !!me?.dead, held: arsenal.cur }),
  nameOf: (id) => (id === myId ? hud.get().name : known.get(id)?.name ?? `#${id}`),
  c4time: () => Number(serverCvars.mp_c4timer) || 40,
  showC4Timer: () => serverCvars.mp_c4_show_timer === "1",
  teamOf: (id) => (id === myId ? myTeam : known.get(id)?.team ?? ""),
  takeName: () => { const it = aimedItem(); return it ? WEAPONS[it.w]?.name || it.w : ""; },
});
// The floor weapon E would take (Go AimedItem, as the server picks it), or null.
function aimedItem() {
  if (!me || me.dead || spectating() || !items.length) return null;
  const i = AimedItemG(me.grav | 0, eyePos(), me.yaw, me.pitch, items.map((it) => it.p));
  return i < 0 ? null : items[i];
}

// ---- actions via binds (ui/binds.js): "+cmd" held, plain commands run on press ----
const keyFor = (cmd) => keyForBind(settings.get().binds, cmd);
const controls = createControls({
  binds: () => settings.get().binds, isDown: (code) => input.down(code), arsenal,
  playing: () => playing(), zoomMode: () => settings.get().zoom_mode,
});
const down = controls.down;
const playing = () => hud.get().screen === "playing" && !hud.get().teamMenu && !hud.get().buyMenu && !hud.get().chatOpen;
// Plain (run-once) game commands, also typed in the console.
function action(cmd) {
  if (cmd === "teammenu") { if (me && !hud.get().teamMenu) openTeamMenu(); return; }
  if (cmd === "buymenu") { if (hud.get().buyMenu) closeBuyMenu(); else openBuyMenu(); return; }
  if (cmd === "buyammo1" || cmd === "buyammo2") { buyAmmo(cmd === "buyammo1" ? "ammo1" : "ammo2"); return; }
  if (cmd === "messagemode" || cmd === "messagemode2") { openChat(cmd === "messagemode2" ? "team" : "all"); return; }
  if (cmd === "radio") { if (me && !spectating() && !me.dead) hud.set({ radioOpen: !hud.get().radioOpen, radioKey: keyFor("radio") }); return; }
  if (cmd === "toggleconsole") { con.toggle(); return; }
  controls.action(cmd);
}

// A clickable menu keeps the cursor: no lock while one is up, and a lock that
// lands anyway (asked for just before team select opened, or re-asked after
// the fullscreen switch) is given back at once.
input.mayLock = () => !menuWantsCursor(hud.get());
input.onRaw = (raw) => hud.set({ rawInput: raw }); // the Options hint when raw input isn't there
input.onLock = (locked) => {
  if (locked && !input.mayLock()) { document.exitPointerLock?.(); return; }
  hud.set({ aiming: locked });
  if (locked) hud.set({ screen: "playing" });
  else if (wantPlay && performance.now() > lockingUntil && unlockOpensMenu(hud.get())) openMenu();
};

// The Esc menu: an overlay on the running match. Held keys are released so
// nobody keeps walking while in it.
function openMenu() {
  input.releaseKeys();
  hud.set({ screen: "ingame", menuOpenedAt: performance.now(), scoreboard: false });
}

// The buy menu (B): only where and when the server says we may buy (me.buy).
// While it is open the mouse is free and game input is off (playing() is
// false), so we stand still; nothing else stops.
function openBuyMenu() {
  if (!me || spectating() || me.dead) return;
  if (!hud.get().canBuy) { con.print("can't buy here: get back to your spawn during buy time", "dim"); hud.set({ buyMsg: "" }); return; }
  input.releaseKeys();
  const key = keyFor("buymenu");
  hud.set({ buyMenu: true, buyMsg: "", buyOk: "", buyKey: key, buyKeyName: keyName(key), ammoKeys: { buyammo1: keyFor("buyammo1"), buyammo2: keyFor("buyammo2") } });
  document.exitPointerLock?.();
}
// CS 1.6's , and . : refill the primary's / the pistol's reserve (the server
// charges economy.json's ammo price and says why not in {t:"bought"}).
function buyAmmo(item) {
  if (!me || spectating() || me.dead) return;
  if (!hud.get().canBuy) { con.print("can't buy here: get back to your spawn during buy time", "dim"); return; }
  net.send({ t: "buy", item });
}
function closeBuyMenu() {
  if (!hud.get().buyMenu) return;
  hud.set({ buyMenu: false });
  input.lock();
}

// Chat (Y / U, or say / say_team in the console) and the chat log.
function openChat(mode) {
  if (!me || hud.get().chatOpen) return;
  input.releaseKeys();
  document.exitPointerLock?.();
  hud.set({ chatOpen: mode, radioOpen: false });
}
function addChat(line) { hud.set({ chat: [...(hud.get().chat || []), line].slice(-30) }); }
setInterval(() => { if (hud.get().chat?.length && !hud.get().chatOpen) hud.set({ chat: hud.get().chat }); }, 1000); // fade old lines

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
input.onClickLock = () => {
  if (replay) { startAudio(); input.lock(); } // a replay: the mouse looks (free cam), no fullscreen
  else if (wantPlay) { startAudio(); lockIn(); }
};

// ---- auto-reload onto a new client build ----
const PAGE_BUILD = document.querySelector('meta[name="cs-build"]')?.content || "";

// Returns true when this page must not play on: a reload is under way, or the
// page is stale and couldn't get onto the server's build (game/build.js).
function checkBuild(m) {
  let last = null;
  try { last = JSON.parse(sessionStorage.getItem("cs.reloaded") || "null"); } catch {}
  const now = Date.now();
  const action = buildAction(PAGE_BUILD, m.build, last, now);
  if (action === "ok") return false;
  net.disconnect();
  if (action === "stale") {
    const msg = `this page runs client build ${PAGE_BUILD}, the server runs ${m.build} and reloading didn't fix it: refresh the page (Ctrl+Shift+R)`;
    console.error(msg);
    con.print(msg, "warn");
    hud.feed("client out of date: refresh the page (Ctrl+Shift+R)");
    return true;
  }
  try {
    sessionStorage.setItem("cs.reloaded", JSON.stringify(nextAttempt(action, m.build, last, now)));
    sessionStorage.setItem("cs.rejoin", JSON.stringify({ name: hud.get().name, inGame: wantPlay }));
  } catch {}
  hud.feed(`new version (${m.version?.slice(0, 7) || m.build}), reloading…`);
  setTimeout(() => {
    skipLeaveConfirm = true;
    if (action === "bust") location.replace(bustURL(location.href, m.build));
    else location.reload();
    // Still here a moment later? The reload didn't happen: protect the page again.
    setTimeout(() => { skipLeaveConfirm = false; }, 2000);
  }, 600);
  return true;
}

// Coming back from such a reload: rejoin straight away, straight into play.
(() => {
  let r = null;
  try { r = JSON.parse(sessionStorage.getItem("cs.rejoin") || "null"); sessionStorage.removeItem("cs.rejoin"); } catch {}
  if (!r?.inGame) return observeFromMenu(); // the menus connect as an observer
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
  if (replay && REPLAY_KEYS[code] && hud.get().screen === "playing") { if (isDown) REPLAY_KEYS[code](); return; }
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
    if (isDown) controls.pressHeld(cmd);
    return;
  }
  // The console's own key handler toggles it for keyboard keys; mouse binds go through here.
  if (cmd === "toggleconsole" && !/^M(ouse|Wheel)/.test(code)) return;
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
  disconnect() { stopReplay(false); leaveMatch(false); },
  // replay [pause|speed <x>|seek <s|+s|-s>|next|prev|cam|stop|open <url>]: the replay player from the console
  replay(a) {
    const [sub = "", arg = ""] = a;
    if (sub === "open") { if (!arg) return "replay open <url of a .cstv.gz>"; startAudio(); playReplay([replayURL(arg)], 0); return `loading ${arg}`; }
    if (!replay) return "no replay playing (drop a .cstv.gz on the page, Replays in the menu, or replay open <url>)";
    const p = replay.player;
    switch (sub) {
      case "": break;
      case "pause": p.setPaused(true); break;
      case "play": p.setPaused(false); break;
      case "speed": if (!(Number(arg) > 0)) return "replay speed <0.25-4>"; p.setSpeed(Number(arg)); break;
      case "seek": {
        const v = Number(arg);
        if (!arg || !Number.isFinite(v)) return "replay seek <seconds> (+s / -s: from here)";
        if (/^[+-]/.test(arg)) p.skip(v); else p.seek(p.first + v * p.header.simHz);
        break;
      }
      case "next": case "prev": replayCtl.round(sub === "next" ? 1 : -1); break;
      case "cam": observer.cycleMode(); break;
      case "stop": stopReplay(); return "replay stopped";
      default: return "replay [pause|play|speed <x>|seek <s|+s|-s>|next|prev|cam|stop|open <url>]";
    }
    return `${roundLabel(p.header)}  ${p.time.toFixed(1)}/${p.length.toFixed(1)} s  ${p.speed}x${p.paused ? "  paused" : ""}  (${replay.index + 1}/${replay.list.length})`;
  },
  status: () => {
    const s = hud.get();
    return { map: s.map, connected: s.connected, ping: s.ping,
      friendlyFire: friendlyFire(),
      players: [{ id: myId, name: s.name, team: myTeam, me: true }, ...s.players], openAdmin: serverCvars.openAdmin };
  },
  joinTeam(t) { joinTeam(t); },
  buy(item) { if (me && !spectating() && !me.dead) net.send({ t: "buy", item }); },
  slop() { startAudio(); sfx.honk(); sfx.land(14); setTimeout(() => sfx.jump(), 180); },
  announcerLines: () => voice.LINES,
  musicCatalog,
  playMusic(id) { startAudio(); return music.preview(id); },
  vox({ line, ph, voice: v }) {
    startAudio();
    if (line) { voice.say(line); return; }
    voice.sayPhonemes(ph, v && voice.VOICES[v] ? v : "announcer");
  },
  jump: () => controls.queueJump(),
  action,
  consoleKey: () => keyFor("toggleconsole"),
  maps: () => mapCatalog.cards(),
  vote: (v) => castVote(v),
  bot(cmd, arg) { net.send({ t: "bot", cmd, arg: String(arg ?? "") }); },
  send(m) { if (!net.open) return con.print("not connected", "warn"); net.send(m); },
});
// Feed lines (joins, leaves, round ends) also go to the console scrollback.
const feed = hud.feed;
hud.feed = (text, kill) => { feed(text, kill); con.print(text, "dim"); };

fetch("/map.json").then((r) => r.json()).then((m) => { if (!map) loadMap(m); });

setInterval(() => {
  hud.set({ ping: net.ping });
}, 1000);
// Movement goes to the server as per-tick commands (simTick); the server moves us.

// A sampled remote's pose state (what the server traces: gen/boxmodel.js st).
const poseOfRemote = (r) => ({ p: r.p, yaw: r.yaw, pitch: r.pitch, crouch: !!r.c, team: r.team, id: r.id, gait: r.g, t: r.t, gv: r.gv | 0 });

function fxFor(id) {
  let fx = remoteFx.get(id);
  if (!fx) remoteFx.set(id, (fx = { flash: 0, last: null, stride: 0, speed: 0, crouch: 0 }));
  return fx;
}

// Map-placed ambient loops (escalator hum), positioned like every other sound.
const hums = createHums({ start: (sp) => sfx.escalatorHum(sp), spatial: (p) => spatial(p) });

// Where a sound is for our ears (game/spatial.js): direction from the view
// (HRTF with snd_hrtf 1, stereo pan without), distance falloff, and muffled
// behind walls (two rays through the map).
function spatial(p) {
  if (!me) return { pan: 0, gain: 0.3 };
  const ear = above(me.pos, me.grav | 0, me.eyeH ?? 1.6);
  const sp = spatialize({ pos: ear, yaw: me.yaw, pitch: me.pitch, frame: frameOf(me.grav) }, p, {
    hrtf: !!settings.get().snd_hrtf, raycast: map ? (o, d, t) => phys.raycast(o, d, t) : null,
  });
  if (TEST) testCtl.lastSpatial = { ...sp, at: [...p] };
  return sp;
}

// The next shot's spread (Go ShotSpread), 0 with weapon_spread 0 (exact shots).
const shotSpread = () => (serverCvars.weapon_spread === "0" ? 0 : arsenal.spread(me));

// The server fires the same shot from the same command: spread is seeded by
// (player id, command seq) exactly like internal/game SpreadSeed.
// A weapon's wallbang power (Go Server.wallPower): its wallPen, 0 under mp_wallbang 0.
const wallPower = (w) => (serverCvars.mp_wallbang === "0" ? 0 : w.wallPen || 0);
function shoot(w, now, wire) {
  testCtl.shots++;
  const [vy, vp] = wire ? ShotView(wire) : [me.yaw, me.pitch]; // the click's view (subtick), as the server shoots
  muzzleLight(w.id, eyePos(), vy, vp, true, frameOf(me.grav));
  const spread = shotSpread();
  const eye = eyePos();
  const seen = alive(remotes.sample(now));
  const [yaw0, pitch0] = recoilAngles(vy, vp, recoilOffset(w.recoil?.pattern, arsenal.shotIdx), recoilScale());
  const angles = shotAngles(yaw0, pitch0, spread, (Math.imul(myId, 2654435761) ^ cmdSeq) >>> 0, w.pellets);
  let shotRay = null;
  angles.forEach(([yaw, pitch], i) => {
    const b = basis(yaw, pitch, frameOf(me.grav)); // our view's angles are in our gravity's frame (as the server shoots)
    let hit = phys.raycast(eye, b.fwd, 200, moverTime(now, 0)), color = PALETTE.spark; // the steps where we see them
    if (hit && hit.t > w.range) hit = null;
    const ray = { seq: cmdSeq, o: eye, d: b.fwd, hit: null };
    // The walls it goes through (wallbangs, Go WallTrace as the server traces it) and where it stops.
    const passes = map ? WallTrace(map.boxes, mapLo, mapHi, eye, b.fwd, w.range, wallPower(w)) : [], stop = WallStop(passes, w.range);
    if (i === 0) {
      shotRay = ray;
      testCtl.lastShotRay = ray;
      if (serverCvars.sv_showimpacts === "1") { // what we shot at (red); the server's answer is blue
        const end = eye.map((v, k) => v + b.fwd[k] * Math.min(hit?.t ?? w.range, w.range));
        impacts.push({ life: 2, color: [1, 0.25, 0.2], end, poses: seen.map(poseOfRemote) });
        if (impacts.length > 12) impacts.shift();
      }
    }
    // Predicted player hit: the server's own test (rayModel = RayModel) on the
    // pose we see. Only the impact puff is shown now; the hit sound, burst and
    // damage come with the server's confirmation ("hit").
    for (const r of seen) {
      if (!friendlyFire() && r.team === myTeam) continue; // shots pass through teammates
      const h = rayModel(PLAYER, poseOfRemote(r), eye, b.fwd);
      if (h && h.t <= w.range && h.t < Math.min(stop, ray.hit?.t ?? Infinity)) { // through a wall it gets through too
        if (h.t < (hit?.t ?? Infinity)) { hit = { t: h.t, n: [0, 0, 0] }; color = teamInfo(r.team)?.rgb ?? hexRgb(r.color); }
        ray.hit = { id: r.id, region: h.region, name: h.name, t: h.t };
        if (!shotRay.hit) shotRay.hit = ray.hit;
      }
    }
    debris.wallHoles(eye, b.fwd, passes, ray.hit?.t ?? Infinity, w); // bullet holes: in, and out of each wall it went through
    if (hit) {
      const p = eye.map((v, k) => v + b.fwd[k] * hit.t + hit.n[k] * 0.05);
      for (let k = 0; k < (angles.length > 1 ? 2 : 4); k++) {
        puffs.push({ p: [...p], v: [hit.n[0] + rs(), hit.n[1] + rs() + 0.5, hit.n[2] + rs()], life: 0.4, size: 0.12, color });
      }
    }
  });
  debris.localShot(w, eye, yaw0, pitch0, me.vel, frameOf(me.grav));
  shotLog.set(cmdSeq, shotRay);
  if (shotLog.size > 64) shotLog.delete(shotLog.keys().next().value);
}

// weapon_recoil_scale (= Server.recoilScale): 0 with weapon_spread 0 (exact shots).
function recoilScale() {
  if (serverCvars.weapon_spread === "0") return 0;
  const v = parseFloat(serverCvars.weapon_recoil_scale ?? "1");
  return Number.isFinite(v) && v >= 0 ? v : 1;
}

const rs = () => (Math.random() - 0.5) * 2;
const round3 = (v) => Math.round(v * 1000) / 1000;
const round5 = (v) => Math.round(v * 1e5) / 1e5;
// Our eye (= Go sim.EyePos): out of the floor we stand on, in a gravity of our own.
const eyePos = () => (me.grav ? above(me.pos, me.grav, me.eyeH + me.eyeOff) : [me.pos[0], me.pos[1] + me.eyeH + me.eyeOff, me.pos[2]]);
// The camera's roll to our gravity's frame (engine/gravity.js), eased; snapped on a spawn.
const roll = createRoll();


// Test harness hooks (server -test plus ?test=1): frames are stepped by the
// harness with a fixed dt, the shader clock can be pinned, the camera overridden.
const TEST = document.querySelector('meta[name="cs-test"]')?.content === "1" && new URLSearchParams(location.search).has("test");
// noGL: frames do everything but the GPU work (cmd/sim lockstep pages set __csNoGL before
// boot; a draw the harness asks for lifts it, testapi.js withGL).
const testCtl = { pinnedTime: null, camera: null, noSpread: false, lastHit: null, shots: 0, lastShout: "", noGL: !!globalThis.__csNoGL }; // lastShout: the last centre caption shown (cases: what was said, whatever the wall clock did since)

// ---- fixed-step simulation + interpolated rendering ----
// The sim (movement, physics, weapons, recoil, remote animation state) advances
// in fixed ticks of 1/simHz seconds (the server announces simHz in welcome).
// Each tick consumes one input command, the shape the authoritative server sim
// will take. Rendering interpolates the local player between the last two ticks;
// mouse look is applied per frame so aiming never waits for a tick.
const MAX_TICKS = 25; // per frame: after a hitch, drop time instead of spiralling
const myGait = newGait(), myGaitPrev = newGait(); // our own body's walk cycle, predicted (drawn in free camera only)
let simDt = 1 / 100, simAcc = 0, cmdSeq = 0, prevButtons = 0;
let prevView = null, vmPrev = null, vmCur = null;

// Interpolation state of the local player before the latest tick.
const viewOf = (b) => ({ pos: [...b.pos], eyeH: b.eyeH, eyeOff: b.eyeOff });
// After a teleport (spawn, correction, harness set): nothing to interpolate from.
// The safety net re-synced our drawn body onto the server's (predict.js): the
// view jumps there at once (one snap beats a lasting desync). Shown in
// cl_netdebug; a console note in test/dev builds so we see when it fires.
predictor.onResync = (r) => {
  snapView();
  if (TEST || /^(localhost|127\.)/.test(location.hostname)) console.warn(`prediction re-sync (${r.why}, ${r.size} m)`);
};
function snapView() { if (me) prevView = viewOf(me); vmPrev = vmCur; resetGait(myGait); resetGait(myGaitPrev); }

function inPlay() {
  const s = hud.get();
  return s.screen === "playing" && !s.console && !s.teamMenu && !s.buyMenu && !s.chatOpen && s.link?.mode !== "lost";
}

// ---- the link to the server (game/link.js): banner on a blip, overlay when lost ----
// Its clock is wall time. Under the test harness it is the sum of the stepped
// frames' dt, and it only runs while a test cut is on (__cs.cutLink): scenarios
// step many client frames with no server ticks between them (rateCheck, jump
// tests), and a silence the harness made must not read as a dead link and eat
// input. A closed socket and a deliberate disconnect need no clock: unchanged.
const link = createLink();
let linkClock = 0;
const linkNow = () => (TEST ? linkClock : performance.now());
function updateLink() {
  const st = link.status(linkNow(), net, performance.now());
  if (st.why === "stall" && st.age >= STALL_MS) net.retryNow(); // a half-dead socket: drop it, reconnect
  const v = linkView(st), was = hud.get().link;
  if (sameView(v, was)) return;
  hud.set({ link: v });
  // Lost: gameplay input stops (inPlay) and the mouse is given back for the buttons.
  if (v?.mode === "lost" && was?.mode !== "lost") {
    input.releaseKeys();
    if (input.locked) document.exitPointerLock?.();
  }
}

// Sample input into one command (engine/cmd.js has the shape; game/controls.js builds it).
// frozen (round freeze time) also rides on the command for the prediction: it holds the drink's countdown (Go HoldBoost).
const buildCmd = () => { const frozen = rounds.frozen(); return Object.assign(controls.buildCmd({ play: inPlay(), me, seq: ++cmdSeq, dt: simDt, frozen }), { frozen }); };

// Damage direction arcs: where recent hits came from (attacker eye at the
// shot), re-aimed every frame as we turn. Low HP: a heartbeat.
const dmgFrom = [];
const DMG_ARC_MS = 1500;
let nextBeat = 0, beatAt = -Infinity; // heartbeat times on fxClock (s): the low-HP rim pulses with it
function updateDamageHud(t) {
  if (testCtl.holdDamageArcs) t = Math.min(t, (dmgFrom.at(-1)?.at ?? t) + DMG_ARC_MS / 2); // tests: freeze them half-faded
  while (dmgFrom.length && t - dmgFrom[0].at > DMG_ARC_MS) dmgFrom.shift();
  const arcs = me ? dmgFrom.map((h) => {
    const dx = h.p[0] - me.pos[0], dz = h.p[2] - me.pos[2];
    // angle clockwise from straight ahead (yaw 0 faces -Z)
    const rel = Math.atan2(-dx, -dz) - me.yaw;
    return { a: -rel, k: 1 - (t - h.at) / DMG_ARC_MS, id: h.at };
  }) : [];
  if (arcs.length || hud.get().dmgArcs?.length) hud.set({ dmgArcs: arcs });
  const hp = hud.get().hp;
  if (me && !me.dead && !spectating() && hp > 0 && hp <= 25 && fxClock >= nextBeat) {
    sfx.heartbeat();
    beatAt = fxClock;
    nextBeat = fxClock + 0.6 + hp * 0.02;
  }
}

let lastSnapTick = -1;
let lastAck = -1; // the newest snapshot's acked command (coresAhead)
let lastRound = null; // the round state of the newest full snapshot (pose snapshots carry none)
// Commands per packet: the new one + up to 15 unacknowledged. Datagrams
// reorder: a packet overtaken by newer ones loses nothing as long as the newer
// ones still carry its commands (4 lost input whenever a packet was overtaken
// by 4+; net_fakereorder 100 cost 51 corrections in 8 s).
const CMD_REDUNDANCY = 16;

// Weapon reconciliation, like the body's: our weapon state after each command,
// compared with the server's at its ack; on a difference, adopt the server's
// and replay the commands it hasn't applied yet (quietly: no sounds, no HUD).
const weaponHist = new Map(); // seq -> {tick, w}
const wkey = (w) => JSON.stringify([Object.keys(w.slots || {}).sort().map((k) => [k, w.slots[k]?.id, w.slots[k]?.ammo, w.slots[k]?.reserve]), w.cur, w.ns, w.ce, w.re, !!w.trig, w.bl || 0, w.bo || 0, w.ri || 0, w.ri ? w.ls || 0 : 0, !!w.ca, w.ub || "", w.pin || 0]);
function recordWeapons(seq) {
  weaponHist.set(seq, { tick: arsenal.tickN, w: arsenal.wstate() });
  if (weaponHist.size > 1100) weaponHist.delete(weaponHist.keys().next().value);
}
const replayBody = { vel: [0, 0, 0], crouched: false, onGround: true };
function reconcileWeapons(ack, server) {
  const rec = weaponHist.get(ack);
  if (!rec || wkey(rec.w) === wkey(server)) return false;
  let speedChanged = false;
  predictor.stats.weaponCorrections = (predictor.stats.weaponCorrections || 0) + 1;
  const keep = {}; // cosmetics and queued input: not part of the replayed state
  for (const k of ["kick", "flash", "bob", "raise", "swing", "cock", "cockLob", "burst", "scoped", "zoomWant", "pendingSwitch", "pendingReload", "last"]) keep[k] = arsenal[k];
  arsenal.quiet = true;
  // The unacknowledged commands run after the snapshot's tick: frozen as its round says.
  const frozen = lastRound?.ph === "freeze";
  try {
    arsenal.setWstate(server, rec.tick);
    for (const c of predictor.pendingCmds()) {
      const sp = arsenal.moveSpeed(); // the body replay moves at the corrected speed (a drink the server just sold us)
      if (sp !== c.speed) { c.speed = sp; speedChanged = true; }
      arsenal.tick(simDt, !!(c.b & BTN.FIRE), replayBody, () => {}, false, { w: c.w, reload: !!(c.b & BTN.RELOAD), frozen, use: c.use, alt: !!(c.b & BTN.ALT) });
      weaponHist.set(c.seq, { tick: arsenal.tickN, w: arsenal.wstate() });
    }
  } finally {
    arsenal.quiet = false;
    Object.assign(arsenal, keep);
  }
  arsenal.show();
  return speedChanged;
}

// One sim tick at wall time `now` (ms) for network-interpolated remotes.
function simTick(now) {
  stepRemoteFx(now); // first: they animate while we're dead too (deathcam)
  if (me && map && !spectating()) {
    const cmd = buildCmd();
    prevView = viewOf(me);
    const b = cmd.b;
    // Send it (the server runs the same command) and keep it for replay.
    cmd.yaw = round5(cmd.yaw); cmd.pitch = round5(cmd.pitch); // exactly what the server gets
    if (me.dead) { prevButtons = b; vmPrev = vmCur; return; } // dead: no input until the server respawns us
    // Every command says which server tick the other players were drawn at:
    // the server traces its shots and collides its movement with them there
    // (lag compensation), and so does the prediction (cmd.peers, kept for replays).
    // testCtl.viewTick (lockstep cases): the view tick pinned to the server's
    // tick (the wall clock that viewTick reads would make it machine-speed
    // dependent), at most the newest snapshot as viewTick is.
    const view = testCtl.viewTick !== undefined && lastSnapTick >= 0 ? Math.min(testCtl.viewTick, lastSnapTick) : remotes.viewTick(now);
    const vt = Math.round(view * 100) / 100 || undefined;
    // The cores: the newest snapshot's players moved on to when the server runs this command.
    const cores = coresAhead(predictor.peers, (id) => remotes.velocity(id), lastAck >= 0 ? cmd.seq - lastAck : 0);
    cmd.peers = vt === undefined ? predictor.peers : peersAt(remotes.sampleAt(vt, true), myId, serverCvars.mp_solid_players !== "0", cores);
    cmd.wire = { s: cmd.seq, f: cmd.f, r: cmd.r, y: cmd.yaw, p: cmd.pitch, b, vt, w: cmd.w };
    // The fire press's command: the shot goes where the view was at the click (Go ShotView).
    if (b & BTN.FIRE && !(prevButtons & BTN.FIRE) && clickView) Object.assign(cmd.wire, { sv: true, sy: round5(clickView[0]), sp: round5(clickView[1]) });
    if (b & BTN.FIRE && !(prevButtons & BTN.FIRE)) clickView = null;
    // Each packet also repeats the unacknowledged commands (CMD_REDUNDANCY),
    // so a lost or overtaken packet costs nothing (the server ignores seqs it
    // already has and slots late ones in): the protocol never relies on every
    // packet arriving, or arriving in order.
    const resend = predictor.pendingCmds().slice(-(CMD_REDUNDANCY - 1)).map((c) => c.wire);
    if (net.open) net.send({ t: "cmd", cmds: [...resend, cmd.wire] });
    netDebug.sent(cmd.seq, !!(b & BTN.FIRE), performance.now());
    predictor.push(cmd);
    vmPrev = vmCur;
    lastStepSeq = cmd.seq;
    const before = testCtl.onCmd && { pos: [...me.pos], vel: [...me.vel] }; // test API: collideProbe
    const { ev, vm } = stepLocal({ phys, arsenal, me, cmd, prevB: prevButtons, dt: simDt, t: predictor.cmdTime(cmd.seq), peers: cmd.peers, shoot: (w) => shoot(w, now, cmd.wire), zoomHeld: down("+zoom") && inPlay(), sites: map.sites || [], items: items.map((it) => it.p) });
    vmCur = vm;
    testCtl.onCmd?.({ cmd, vt, before, me, drawn: vt === undefined ? null : remotes.sampleAt(vt) });
    Object.assign(myGaitPrev, myGait);
    gaitStep(myGait, me.pos[0] - prevView.pos[0], me.pos[1] - prevView.pos[1], me.pos[2] - prevView.pos[2], { onGround: me.onGround, crouch: me.crouched, id: myId }); // drawn only in free cam
    cmd.pos = [...me.pos]; // predicted result, compared with the server's when acked (cl_netdebug)
    recordWeapons(cmd.seq);
    prevButtons = b;
    if (ev.jumped) sfx.jump();
    if (ev.landed) sfx.land(ev.landed);
    if (ev.splash) sfx.splash(ev.splash);
    for (let i = 0; i < ev.steps; i++) sfx.footstep();
  }
  for (const im of impacts) im.life -= simDt;
  while (impacts.length && impacts[0].life <= 0) impacts.shift();
}

function stepRemoteFx(now) {
  // Remote animation state (everyone else: players, bots, whoever we spectate):
  // footsteps from the interpolated movement (the gait itself is replicated: r.g).
  for (const r of alive(remotes.sample(now))) {
    const fx = fxFor(r.id);
    if (fx.last) {
      const d = Math.hypot(r.p[0] - fx.last[0], r.p[2] - fx.last[2]);
      if (fx.speed > 4.2) { fx.stride += d; if (fx.stride > 2.2) { fx.stride = 0; sfx.footstep(spatial(r.p)); } }
    }
    fx.speed += (r.v - fx.speed) * (1 - Math.exp(-simDt * 12));
    fx.last = r.p;
    fx.flash = Math.max(0, fx.flash - simDt);
    fx.crouch += ((r.c ? 1 : 0) - fx.crouch) * (1 - Math.exp(-simDt * 18));
  }
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
// Muzzle lights run on the frame clock (the harness's fixed dt in lockstep), so a
// snapshot mid-flash is the same every run.
let flashClock = 0;
let sinceDrawn = 0;
// A shot's muzzle light: a little ahead of the eye along the aim (pulled back when a
// wall is closer, so the flash floods from our side of it). Weapon class decides the look;
// it casts shadows if it makes the budget (own: our shots rank first).
function muzzleLight(id, eye, yaw, pitch, own = false, frame = null) {
  const ml = muzzleLightOf(id);
  if (!ml || !eye || !settings.get().r_muzzle_light) return;
  const b = basis(yaw, pitch, frame), hit = phys.raycast(eye, b.fwd, MUZZLE.ahead + MUZZLE.wallGap);
  const { pos, dir } = muzzlePose(eye, yaw, pitch, hit ? hit.t : null, frame); // at the muzzle, shining forward
  gfx.addDynamicLight({ ...ml, shadow: true, own, pos, dir }, flashClock);
}
function remoteMuzzleLight(id, w) {
  const r = remotes.sample(performance.now()).find((x) => x.id === id);
  if (r) muzzleLight(w, above(r.p, r.gv | 0, r.c ? EYE_CROUCH : EYE), r.yaw, r.pitch, false, frameOf(r.gv));
}
// Advance by dt seconds (wall clock, or the harness's fixed dt), run the due
// sim ticks, then draw unless draw is false (or the GPU queue is full). Returns whether it drew.
function frame(now, fixedDt, draw = true) {
  const dt = fixedDt ?? Math.min(0.25, (now - last) / 1000);
  last = now;
  flashClock += dt;
  if (!TEST || net.cut) linkClock += dt * 1000;
  if (replay) tickReplay(dt);
  else updateLink();
  const latch = performance.now(); // input is sampled from here on

  if (me && map && !observing()) {
    const [mx, my] = input.takeMouse();
    const zoom = arsenal.scoped && arsenal.weapon()?.scope;
    const rpc = radPerCount() * (zoom ? zoomSensitivity(zoom.fov, BASE_FOV, settings.get().zoom_sensitivity_ratio) : 1), inv = settings.get().m_invert_y ? -1 : 1;
    // A fire press since the last frame: the view at that moment (CS2 subtick), for the command
    // that carries the press (simTick): the frame's start plus the counts pending when it went down.
    for (const [code, c] of Object.entries(settings.get().binds)) {
      const at = c === "+attack" && input.takePress(code);
      if (at) clickView = [me.yaw - at[0] * rpc, Math.max(-1.55, Math.min(1.55, me.pitch - at[1] * rpc * inv))];
    }
    me.yaw -= mx * rpc;
    me.pitch = Math.max(-1.55, Math.min(1.55, me.pitch - my * rpc * inv));
  } else {
    // Observing (or attract mode): the mouse and movement keys drive the observer camera.
    const [mx, my] = input.takeMouse();
    input.pressedAt = {}; clickView = null; // no shot from a click made while not in a body
    if (observing() && inPlay()) {
      const rpc = radPerCount(), inv = settings.get().m_invert_y ? -1 : 1;
      const d = (cmd) => (down(cmd) ? 1 : 0);
      observer.update(dt, [-mx * rpc, -my * rpc * inv], { f: d("+forward") - d("+back"), r: d("+moveright") - d("+moveleft"), up: 0, fast: !!d("+speed") });
    } else if (observing()) observer.update(dt, [0, 0], { f: 0, r: 0, up: 0, fast: false });
  }
  simAcc += dt * (testCtl.clockScale ?? 1); // harness: a client clock running fast/slow
  let ticks = 0;
  while (simAcc >= simDt - 1e-9) {
    simAcc -= simDt;
    if (++ticks > MAX_TICKS) { simAcc = 0; break; }
    simTick(now - simAcc * 1000);
  }
  const alpha = Math.min(1, Math.max(0, simAcc / simDt));
  // The reconciliation offset eases out (and is watched: predict.js re-syncs a
  // stuck one) while we play a body; dead, spectating or observing, it's dropped.
  if (!testCtl.noDecay) predictor.decay(dt, !!me && !observing()); // noDecay: tests of the safety net (the old bug: never eased out)

  // Render-only cosmetics: per frame, dt-correct.
  for (let i = puffs.length - 1; i >= 0; i--) {
    const p = puffs[i];
    p.life -= dt;
    if (p.life <= 0) { puffs.splice(i, 1); continue; }
    for (let a = 0; a < 3; a++) p.p[a] += p.v[a] * dt * 2;
    p.v[1] -= dt * 3;
    p.size = 0.14 * (p.life / 0.4);
  }
  stepBurst(bursts, dt);
  fxClock += dt;
  while (hurtHits.length && fxClock - hurtHits[0].at > Math.max(VIGNETTE_S, FLINCH_S)) hurtHits.shift();
  rounds.update(dt, now, map);
  hums.update(map, me?.pos ?? null); // escalators hum where the map has them
  nades.update(dt);
  debris.update(dt, rounds.state, map?.name);
  draw = draw && gfx.canRender(); // r_gpu_fence: keep the sim running, skip the frame
  updateDamageNumbers(dt, draw);
  loadGfxMap(); // also on frames not drawn (tests step without drawing)
  sinceDrawn += dt; // the crosshair eases over the time since it was last drawn (frames may skip drawing)
  if (draw) { render(now, alpha, latch, sinceDrawn); sinceDrawn = 0; }
  return draw;
}

// A new map: the simulation takes it at once; the renderer a frame later, so
// the loading screen paints before the build (light grid, meshes) blocks, and
// the screen stays up until the map's shader has compiled (in the background
// where the driver can) and the map is drawn.
let gfxMapDue = null, gfxMapWait = 0;
function loadMap(m) {
  map = m; phys.setMap(m);
  applyRules();
  [mapLo, mapHi] = MapBounds(m.boxes); // wallbangs: the map's shell
  gfxMapDue = m; gfxMapWait = 1;
  hud.set({ map: m.name, loadingMap: m.name, teamCap: Math.floor((m.maxPlayers || 0) / 2) });
}
// The rules in force (the map's pins over the server cvars, rules.js) into
// the movement the prediction runs, as the server's moveRules.
function applyRules() {
  phys.setMovement(moveRules(effectiveCvars(serverCvars, map)));
  hud.set({ mapRules: map?.rules ?? null });
}
function loadGfxMap() {
  if (gfxMapDue && gfxMapWait-- <= 0) { gfx.setMap(gfxMapDue); gfxMapDue = null; }
  if (!gfxMapDue && hud.get().loadingMap && !gfx.loading()) hud.set({ loadingMap: null });
}

// When to draw the movers (escalator treads): playing, at our own predicted
// command clock between the last two commands (as our drawn body), so what we
// stand on is exactly what we see under us; otherwise on the server's clock
// as the remotes are drawn.
let lastStepSeq = 0;
function moverTime(now, alpha) {
  if (me && !me.dead && !observing() && lastStepSeq) return predictor.cmdTime(lastStepSeq) + alpha * simDt;
  return remotes.viewTick(now) * simDt;
}

// The crosshair's CSS: --gap, the lines' inner tips (px from the screen centre).
// testCtl.crosshair: what was drawn, and where the next shot's cone sits (test API).
const crosshair = createCrosshair();
let crosshairCss = "";
function drawCrosshair(cone, dt) {
  const c = crosshair.update(cone, dt);
  testCtl.crosshair = { gap: c.gap, r: c.r, x: c.x, y: c.y };
  const css = c.gap.toFixed(1) + "px";
  if (css === crosshairCss) return;
  crosshairCss = css;
  document.documentElement.style.setProperty("--gap", css);
}

function render(now, alpha, latch, dt = 0) {
  updateDamageHud(performance.now());
  const time = testCtl.pinnedTime ?? now / 1000;
  let eye, yaw, pitch, frame = null, viewmodel = null, watched = null, watchFirst = false; // frame: the view's own (a gravity of our own: engine/gravity.js)

  if (me && map && !observing()) {
    const pv = prevView || viewOf(me);
    const pos = pv.pos.map((v, i) => lerp(v, me.pos[i], alpha) + predictor.error[i]); // + smoothed reconciliation error
    frame = roll.update(dt, me.grav);
    eye = [pos[0], pos[1] + lerp(pv.eyeH, me.eyeH, alpha) + lerp(pv.eyeOff, me.eyeOff, alpha), pos[2]];
    if (frame) { const u = turn(frame, [0, eye[1] - pos[1], 0]); eye = [pos[0] + u[0], pos[1] + u[1], pos[2] + u[2]]; } // the eye swings round with the roll
    const punch = arsenal.viewPunch(arsenal.tickN + alpha, recoilScale());
    const fl = flinch(hurtHits, fxClock); // view only: aim stays me.yaw/me.pitch
    yaw = me.yaw - punch[1] + fl.yaw; pitch = me.pitch + punch[0] + fl.pitch;
    if (TEST) testCtl.view = { eye, yaw: me.yaw, pitch: me.pitch, vel: [...me.vel], t: now, grav: me.grav | 0, rolled: roll.settled };
    if (TEST && testCtl.skipGfx) return; // net probes: the view as computed, no GPU work (slow CI runners)
    viewmodel = viewmodelAt(alpha);
    if (viewmodel) viewmodel.frame = frame;
    // The crosshair (crosshair.js): centred, its gap covering the next shot's cone
    // around its true direction (the whole spray offset; the view shows only the punch).
    const [aimYaw, aimPitch] = recoilAngles(me.yaw, me.pitch, arsenal.sprayOffset(arsenal.tickN + alpha, recoilScale()), 1);
    const zoom = arsenal.scoped && arsenal.weapon()?.scope;
    drawCrosshair(shotCone({ camYaw: yaw, camPitch: pitch, yaw: aimYaw, pitch: aimPitch, spread: shotSpread(), fov: zoom ? zoom.fov : BASE_FOV, h: canvas.clientHeight || innerHeight }), dt);
  } else if (map && observing()) {
    ({ eye, yaw, pitch, frame = null, follow: watched, first: watchFirst } = observer.view(bodies(remotes.sample(now)), map));
    drawCrosshair({ x: 0, y: 0, r: 0, reach: 0 }, dt); // first-person follow: their spread isn't ours to know; a resting crosshair
  } else {
    // attract mode: slow orbit over the map
    const a = time * 0.08;
    eye = [Math.sin(a) * 34, 16, Math.cos(a) * 34]; yaw = a; pitch = -0.38;
  }
  showObserverHud(watched, watchFirst);

  // remote players: positions interpolated by the network clock, animation from the sim
  const sampled = remotes.sample(now);
  const players = alive(sampled).map((r) => {
    const fx = fxFor(r.id);
    return { ...r, rgb: hexRgb(r.color), time, gait: r.g, t: r.t, flash: fx.flash, crouch: !!r.c }; // replicated state, gait included: the hit pose
  });

  // First-person follow: you are inside that player, so their body and name tag
  // go and their weapon is drawn as the viewmodel.
  const shown = watchFirst && !testCtl.camera ? players.filter((r) => r.id !== watched.id) : players;
  if (watchFirst && !testCtl.camera) {
    const fx = fxFor(watched.id);
    viewmodel = { id: WEAPONS[watched.w] ? watched.w : "prompt", bobX: 0, bobY: 0, kick: fx.flash * 4, flash: fx.flash, reload: 0, raise: 1, frame };
  }
  const solids = shown.flatMap(playerParts);
  solids.push(...corpseSolids(sampled, now, time));
  if (observing() && observer.mode === "overview" && !observer.freeze) {
    // Overview: a big glowing dot above every player in their team colour.
    for (const r of players) solids.push({ m: chain(translate(r.p[0], r.p[1] + 4, r.p[2]), scale(2.6, 0.5, 2.6)), color: teamInfo(r.team)?.rgb ?? hexRgb(r.color), emit: 1 });
  }
  if (marksFor !== map) { marksFor = map; marks = spawnMarks(map); } // team start pads + buy-zone outlines
  solids.push(...marks);
  for (const it of items) solids.push(...itemParts(it.p, it.w, time));
  solids.push(...rounds.solids(players, time, now, watchFirst ? watched?.id : 0, map));
  solids.push(...nades.solids(time, now));
  for (const p of puffs) solids.push({ m: chain(translate(...p.p), scale(p.size, p.size, p.size)), color: p.color, emit: 1 });
  for (const p of bursts) { const z = burstSize(p); solids.push({ m: chain(translate(...p.p), scale(z, z, z)), color: p.color, emit: p.emit }); }
  if (testCtl.camera) {
    // Free camera: the local player is drawn too, since we're not looking through their eyes.
    ({ eye, yaw, pitch } = testCtl.camera);
    if (me && !spectating()) solids.push(...playerParts({ id: myId, team: myTeam, p: me.pos, yaw: me.yaw, pitch: me.pitch, rgb: hexRgb(myColor), time, crouch: me.crouched ? 1 : 0, gait: lerpGait(myGaitPrev, myGait, alpha), t: time, gv: me.grav | 0 }));
    viewmodel = null;
    frame = null;
  }
  for (const im of impacts) {
    for (const st of im.poses) for (const h of hitParts(st)) solids.push(...wireBoxIn(h.m, h.c, h.s, im.color));
    if (im.end) solids.push(...wireBox(im.end.map((v) => v - 0.05), im.end.map((v) => v + 0.05), im.color)); // impact point
  }
  if (settings.get().cl_showhitboxes) {
    // Exactly what the server ray-traces: the hittable model parts, posed from the replicated state.
    const who = players.map(poseOfRemote);
    if (testCtl.camera && me && !spectating()) who.push({ p: me.pos, yaw: me.yaw, pitch: me.pitch, crouch: me.crouched, team: myTeam, id: myId, gait: lerpGait(myGaitPrev, myGait, alpha), t: time });
    for (const st of who) for (const h of hitParts(st)) solids.push(...wireBoxIn(h.m, h.c, h.s, HITBOX_COLORS[h.region] || [1, 1, 1]));
  }


  // Scope state is read live (a toggle is immediate input, not sim state).
  const scoped = !!viewmodel && !watched && arsenal.scoped, scope = scoped && arsenal.weapon()?.scope;
  const overlay = viewmodel && !scoped ? viewmodelParts(eye, yaw, pitch, viewmodel, viewmodel.id) : [];
  // GPU Drop: the box you (or the player you watch in first person) carry, under the left arm.
  const handId = watchFirst ? watched?.id : myId;
  if (viewmodel && !scoped && rounds.state?.gpus?.some((g) => g.st === "carried" && g.by === handId)) overlay.push(...gpuHandParts(eye, yaw, pitch, viewmodel, time));
  if (!testCtl.camera) eye = rounds.shakeEye(eye);
  gfx.frame({ eye, yaw, pitch, frame, time, solids, overlay, fov: scope ? scope.fov : undefined, latch, players: shown.map((r) => r.p), marks: debris.marks, dynNow: flashClock,
    moverTime: moverTime(now, alpha), noGL: TEST && testCtl.noGL });
  const hs = hud.get(), cfg = settings.get();
  radar.draw({
    map, me: me && !observing() ? { pos: eye, yaw } : null, team: spectating() ? "spec" : myTeam, players, spot,
    bomb: rounds.state?.bomb, gpus: rounds.state?.mode === "gpudrop" ? rounds.state.gpus : null, rotate: !!cfg.cl_radar_rotate, scale: cfg.cl_radar_scale, now,
    enabled: !!cfg.cl_radar && hs.screen === "playing", full: !!hs.scoreboard && hs.screen === "playing",
  });
  const white = spectating() ? 0 : nades.whiteout(now), smoky = nades.smokeAt(eye);
  const shaded = !!hs.shades && !spectating(); // the sunglasses: a faint cool tint
  const fxKey = white > 0.005 ? `w${white.toFixed(2)}` : smoky > 0.005 ? `s${smoky.toFixed(2)}` : shaded ? "g" : "";
  if (fxKey !== screenFxKey) {
    screenFxKey = fxKey;
    screenFx.style.background = white > 0.005 ? "#fff" : smoky > 0.005 ? "rgb(176, 180, 186)" : "rgb(20, 34, 52)";
    screenFx.style.opacity = String(white > 0.005 ? white : smoky > 0.005 ? smoky * 0.97 : shaded ? 0.1 : 0);
  }
  updateHurtFx(hs);
  updateTags(shown, eye, yaw, pitch, frame);
}

// Corpses: the dead topple over stiffly and lie there for the round, or shrink
// away in warmup (corpse.js), posed from the death tick in the snapshot.
function corpseSolids(sampled, now, time) {
  const tick = remotes.viewTick(now), out = [], ids = new Set();
  let drawn = 0;
  const dead = bodies(sampled).filter((r) => r.dead && r.dk);
  if (myCorpse && me?.dead && !spectating()) dead.push({ ...myCorpse, p: me.pos ?? myCorpse.p });
  for (const r of dead) {
    const age = testCtl.corpseAge ?? deathAge(r, tick), pose = corpsePose(age, r.dv); // tests can pin the pose
    ids.add(r.id);
    const before = corpseAge.get(r.id);
    corpseAge.set(r.id, age);
    if (!pose) continue;
    drawn++;
    const upright = playerParts({ id: r.id, team: r.team, p: r.p, yaw: r.yaw, pitch: 0, rgb: hexRgb(r.color), time, crouch: 0 });
    out.push(...corpseParts(upright, r.p, r.fy, pose, bodyRests));
    if (!r.dv && before !== undefined && landedBetween(before, age)) dustPuff(r.p, r.fy);
  }
  for (const id of corpseAge.keys()) if (!ids.has(id)) corpseAge.delete(id);
  testCtl.corpses = drawn;
  return out;
}

// A little cloud of dust along a body that just hit the floor.
function dustPuff(feet, fy) {
  const f = [-Math.sin(fy), -Math.cos(fy)];
  for (let i = 0; i < 10; i++) {
    const d = 0.2 + 1.6 * (i / 9), side = (i % 2 ? 1 : -1) * 0.5;
    const p = [feet[0] + f[0] * d - f[1] * side * 0.4, feet[1] + 0.05, feet[2] + f[1] * d + f[0] * side * 0.4];
    const j = ((i * 0.618) % 1) - 0.5; // a fixed spread per particle, no randomness
    puffs.push({ p, v: [-f[1] * side + j * 0.3, 0.6 + (j + 0.5) * 0.4, f[0] * side - j * 0.3], life: 0.4, size: 0.14, color: PALETTE.sand });
  }
}

// Taking hits: the red vignette on the attacker's side (re-aimed as we turn),
// and at low HP a grey rim that swells on each heartbeat.
const hurtFx = document.createElement("div");
hurtFx.className = "hurtfx";
document.getElementById("hud")?.before(hurtFx);
let hurtFxKey = "";
function updateHurtFx(hs) {
  const live = me && !observing() && hs.screen === "playing";
  const edges = live ? vignette(hurtHits, fxClock, (h) => (h.p ? relAngle(me.pos, me.yaw, h.p) : Math.PI)) : [];
  const low = live ? lowHpEdge(hs.hp, fxClock - beatAt) : 0;
  const bg = vignetteCss(edges, low);
  if (bg !== hurtFxKey) { hurtFxKey = bg; hurtFx.style.background = bg || "none"; }
  if (TEST) testCtl.hurtFx = { edges: edges.length, alpha: edges.reduce((a, e) => Math.max(a, e.a), 0), lowHp: low, flinchDeg: (({ yaw, pitch }) => Math.hypot(yaw, pitch) * 180 / Math.PI)(flinch(hurtHits, fxClock)), bursts: bursts.length };
}

// The observer HUD line: who you watch (name, team, HP, weapon, ammo), the
// camera mode, or the death cam caption. Only pushed to the HUD on change.
function showObserverHud(watched, first) {
  let o = null;
  if (observing()) {
    const f = observer.freeze, k = f && remotes.sample(performance.now()).find((r) => r.id === f.killer);
    const kk = f && f.killer ? k ?? known.get(f.killer) : null; // a dead killer is not in remotes; one who left is in neither
    o = f ? { freeze: true, killer: f.killer ? kk?.name ?? "someone" : "", killerTeam: kk?.team ?? "", weapon: WEAPONS[f.weapon]?.name ?? f.weapon ?? "",
      caption: deathCaption({ killer: f.killer ? kk?.name ?? "someone" : "", killerTeam: kk?.team ?? "", myTeam, weapon: f.weapon,
        weaponName: f.weapon === "bigdiff" ? "" : WEAPONS[f.weapon]?.name ?? f.weapon ?? "", self: !f.killer }) }
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

// Name tags (which ones: tags.js): teammates, an enemy under your crosshair
// and in sight, everyone when observing; overlapping far ones dropped.
function updateTags(players, eye, yaw, pitch, frame) {
  const view = { eye, yaw, pitch, frame, team: myTeam, observing: observing() || !!testCtl.camera };
  const tags = pickTags(view, players, map?.boxes ?? [], (w) => gfx.project(w), nades.smokeCores());
  testCtl.tags = tags.map((t) => t.name);
  const want = new Set(tags.map((t) => t.id));
  for (const el of [...tagLayer.children]) if (!want.has(+el.dataset.id)) el.remove();
  for (const t of tags) {
    let el = tagLayer.querySelector(`[data-id="${t.id}"]`);
    if (!el) {
      el = document.createElement("div");
      el.dataset.id = t.id;
      tagLayer.appendChild(el);
    }
    if (el.textContent !== t.name) el.textContent = t.name;
    el.style.color = teamInfo(t.team)?.color ?? "#fff";
    el.style.opacity = t.alpha.toFixed(2);
    el.style.transform = `translate(${t.x.toFixed(0)}px, ${t.y.toFixed(0)}px) translate(-50%, -100%)`;
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
  const lamps = r.lights ? `lights ${r.lightsActive}/${r.lights} in play · ${r.lightEntries} cell entries, ${r.lightGpuKB} KB, built ${r.lightBuildMs.toFixed(0)} ms · `
    + (r.lightShadowTiles ? `lamp shadows: ${r.lightShadowBaked} lights baked${r.lightShadowQueued ? ` (${r.lightShadowQueued} to go)` : ""}, ${r.lightShadowTiles} tiles ${r.lightShadowSize}², ${r.lightShadowKB} KB, bake ${r.lightShadowBakeMs.toFixed(0)} ms · live ${r.lightShadows}/${r.lightShadowBudget} cpu ${ms(r.lightShadowCpuMs)} gpu ${ms(r.lightShadowGpuMs)} · player redraws ${r.lightShadowSolidRenders}` : "no lamp shadows")
    : "no map lights";
  const dyn = `dynamic lights ${r.dynLights ?? 0} lit · ${r.dynLightsAdded ?? 0} added (${r.dynLightsDropped ?? 0} dropped) · add ${ms(r.dynLightAddMs)} (peak ${ms(r.dynLightAddPeakMs)}, worst frame ${ms(r.dynLightFrameMsMax)}) · floods ${r.dynLightFloods ?? 0}, mask hits ${r.dynLightMaskHits ?? 0}, skipped ${r.dynLightMaskSkips ?? 0} · shadows ${r.dynShadows ?? 0}/${r.dynShadowBudget ?? 0}, ${r.dynShadowRenders ?? 0} rendered, cpu ${ms(r.dynShadowCpuMs)} gpu ${ms(r.dynShadowGpuMs)}`;
  const flag = (req, got) => (req ? (got ? "on" : "REFUSED") : "off");
  return [
    `rendered ${l.renderedFps} fps · shown ${l.presented.fps.toFixed(0)} fps · gpu frame ${ms(r.gpuFrameMs)} · ${shadows}`,
    `in flight ${r.inFlight}${r.maxInFlight ? `/${r.maxInFlight} (skipped ${r.skipped})` : ""} · latch→gpu ${ms(r.latchToGpuMs)} · est. input→present ${ms(est)}`,
    lamps,
    dyn,
    `aa ${r.aa}${r.renderScale < 1 ? ` · scale ${Math.round(r.renderScale * 100)}% (${r.target.join("×")})` : ""}`,
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
  aa: s.r_aa, renderScale: s.r_render_scale / 100, lightShadows: s.r_light_shadows, dynShadows: s.r_dynlight_shadows,
});
applyGfx(settings.get());
settings.subscribe(applyGfx);
const netDebug = createNetDebug(() => remotes.timing(performance.now()).viewDelayMs);
settings.subscribe((s) => netDebug.show(!!s.cl_netdebug));
netDebug.show(!!settings.get().cl_netdebug);
setInterval(() => { if (settings.get().cl_netdebug) netDebug.render(performance.now(), { ping: net.ping, prediction: predictor.stats, audio: sfx.audioState() }); }, 100);
const applyNetFaults = (s) => { net.fakeLag = s.net_fakelag || 0; net.faults = { loss: s.net_fakeloss || 0, dup: s.net_fakedup || 0, jitter: s.net_fakejitter || 0, reorder: s.net_fakereorder || 0 }; };
settings.subscribe(applyNetFaults);
settings.subscribe((s) => remotes.setInterp((s.cl_interp || 0) * 1000));
applyNetFaults(settings.get());
net.fakeLag = settings.get().net_fakelag || 0;
const syncAnnouncer = (s) => voice.setAnnouncer({ volume: s.announcer_volume, enabled: s.cl_announcer, onLine: showCaption });
function showCaption(l) {
  if (!settings.get().cl_captions || voice.LINES[l.id]?.banner) return; // banner lines: the HUD shows them (round end)
  const kind = l.voice === "taunt" ? "sub" : "shout";
  if (kind === "shout") testCtl.lastShout = l.text;
  hud.caption({ text: l.text, color: l.color, dur: l.dur, kind });
}
syncAnnouncer(settings.get());
settings.subscribe(syncAnnouncer);
const syncMusic = (s) => music.set({ enabled: !!s.cl_music, volume: s.music_volume });
syncMusic(settings.get());
settings.subscribe(syncMusic);

// Debug handle for the console / automated checks.
window.cs = { get me() { return me; }, remotes, net };

if (TEST) {
  installTestApi({
    frame, testCtl, input, snapView, latencyReadout, keyFor, muzzleLight, get simDt() { return simDt; }, loop,
    resetSim() { simAcc = 0; prevButtons = 0; controls.reset(); vmCur = vmPrev = null; snapView(); }, hud, con, settings, net, remotes, gfx, announcer, music,
    get me() { return me; }, get myId() { return myId; }, get map() { return map; }, arsenal, items: () => items, nades, debris, radar, get spot() { return spot; }, hitParts, observer, rounds, predictor, actions: ACTIONS, wkey: (w) => wkey(w), settingsKey: "cs.settings", audioState: () => sfx.audioState(), restartPage: () => { skipLeaveConfirm = true; location.reload(); }, get netDebug() { return netDebug; }, bumpSeq: () => ++cmdSeq,
    playReplay: (list) => playReplay(list, 0), get replay() { return replay; }, recorder, playRecording, saveRecording,
    moverClock: () => { const now = performance.now(); return { t: moverTime(now, 0), view: remotes.viewTick(now) * simDt, tb: predictor.tickBase }; },
    // What the next command packet would repeat (the unacknowledged commands), without a new one:
    // lockstep cases stand in for the next frame when net_fakeloss dropped a packet.
    resendCmds: () => { const p = predictor.pendingCmds(); if (net.open && p.length) net.send({ t: "cmd", cmds: p.slice(-CMD_REDUNDANCY).map((c) => c.wire) }); return p.length; }, sampleRemotes: () => remotes.sample(performance.now()),
    get spawnSeq() { return spawnSeq; }, get myTeam() { return myTeam; }, movement: () => phys.movement(),
    play: (name) => startPlay(name), resetCrouchNotice: () => { crouchNoticeAnswered = false; }, thumbStats: () => mapCatalog.thumbStats(),
  });
}

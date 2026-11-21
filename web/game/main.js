import { createRenderer } from "../engine/gl.js";
import { chain, translate, scale, rotY } from "../engine/math.js";
import { hexRgb } from "../gen/color.js";
import { PALETTE } from "../assets/palette.js";
import { MATERIALS } from "../assets/materials/index.js";
import { SKY, SUN_DIR } from "../assets/sky.js";
import { createMapCatalog } from "./maps.js";
import { playerParts, hitParts, PLAYER } from "../assets/models/player.js";
import { rayModel } from "../gen/boxmodel.js";
import { viewmodelParts, itemParts } from "../assets/models/weapons.js";
import { spawnMarks } from "../assets/models/spawnmarks.js";
import { createArsenal, shotAngles, recoilOffset, recoilAngles } from "./arsenal.js";
import { createPredictor } from "./predict.js";
import { pickTags } from "./tags.js";
import { createNetDebug } from "./netdebug.js";
import { team as teamInfo, SPECTATOR } from "../assets/teams.js";
import { createPhysics, newBody, R, H, H_CROUCH } from "../engine/physics.js";
import { BTN } from "../engine/cmd.js";
import { keyFor as keyForBind, keyName, ACTIONS } from "../ui/binds.js";
import { createInput } from "../engine/input.js";
import { createNet, createRemotes, INTERP_DELAY } from "../engine/net.js";
import { mountHud } from "../ui/hud.js";
import { basis, zoomSensitivity } from "../engine/math.js";
import * as sfx from "../assets/sounds/index.js";
import * as voice from "../assets/voice/index.js";
import { chatPrefix } from "../ui/chat.js";
import { createAnnouncer } from "./announcer.js";
import { stepLocal } from "./localsim.js";
import { createControls } from "./controls.js";
import { settings, radPerCount } from "../ui/settings.js";
import { createLoop, createOverlay } from "../engine/loop.js";
import { createConsole } from "../ui/console.js";
import { installTestApi } from "./testapi.js";
import { wireBox, wireBoxIn } from "../engine/debugdraw.js";
import { WEAPONS, FIRE_SOUND } from "../assets/weapons/index.js";
import { createObserver, MODE_NAMES } from "./observer.js";
import { createRounds } from "./rounds.js";
import { createNades } from "./nades.js";
import { createRadar } from "./radar.js";

const SEND_HZ = 30, BASE_FOV = 1.3; // vertical fov (radians) unzoomed, as gl.js

const canvas = document.getElementById("gl");
const tagLayer = document.createElement("div");
tagLayer.className = "tags";
document.body.appendChild(tagLayer);

// Context flags (experimental options) apply when the context is created, i.e. on reload.
const ctxFlags = { desynchronized: !!settings.get().r_desynchronized, powerPreference: settings.get().r_power_preference };
const look = { palette: PALETTE, materials: MATERIALS, sky: SKY, sun: SUN_DIR };
const gfx = createRenderer(canvas, look, ctxFlags);
const mapCatalog = createMapCatalog(look);
const phys = createPhysics();
const input = createInput(canvas, () => settings.get().m_rawinput);
const remotes = createRemotes();

let map = null, me = null, myId = 0, myColor = "#fff", myTeam = "", spawnSeq = 0, wantPlay = false;
const known = new Map(); // id -> {name, team}, for join/leave/switch feed
const joinedText = (name, t) => t === SPECTATOR ? `${name} is spectating` : `${name} joined the ${teamInfo(t)?.name ?? "server"}`;
const spectating = () => myTeam === SPECTATOR;
let marksFor = null, marks = []; // spawnMarks(map), rebuilt on a map change
const friendlyFire = () => serverCvars.mp_friendlyfire === "1"; // server cvar (rcon mp_friendlyfire 1)
const bodies = (list) => list.filter((r) => r.team !== SPECTATOR); // spectators have no body
// Watching instead of playing: spectating, or dead (the death cam, then following someone).
const observing = () => spectating() || !!me?.dead;
const remoteFx = new Map(); // id -> {flash, phase, last, stride}
const puffs = [];
let hitMarker = 0; // seconds left on the hit marker
const damageNumbers = [];
// Our recent shots by command seq (client-predicted hit), matched with the server's "fired".
const shotLog = new Map();
// sv_showimpacts: per shot, what we saw (red) and what the server traced (blue), for 2 s.
const impacts = [];
const HITBOX_COLORS = { head: [1, 0.2, 0.2], body: [1, 0.85, 0.2], legs: [0.3, 0.8, 1], gun: [0.6, 0.6, 0.6] };
let items = []; // weapons lying on the map, from snapshots
const announcer = createAnnouncer();
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
  buy(item) { net.send({ t: "buy", item }); },
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
  loop: () => loop,
  latency: () => ({ lines: latencyReadout(), info: gfx.info(), requested: ctxFlags }),
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
      lastSnapTick = -1; // a new connection (maybe a restarted server): ticks start over
      if (checkBuild(m)) return;
      myId = m.id; myColor = m.color; map = m.map; myTeam = m.team; spawnSeq = 0;
      gfx.setMap(map); phys.setMap(map);
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
      if (m.tick !== undefined) { if (m.tick <= lastSnapTick) break; lastSnapTick = m.tick; }
      remotes.ingest(m, myId, now);
      netDebug.snap(m, performance.now());
      if (m.me && me && !spectating()) { // the server's word on our own body
        testCtl.lastMe = m.me;
        predictor.reconcile(me, m.me, simDt);
        // A weapon correction can change the replayed commands' speed (the drink): replay the body again.
        if (m.me.w && reconcileWeapons(m.me.ack, m.me.w)) predictor.reconcile(me, m.me, simDt);
        me.dead = !!m.me.dead;
        if (m.me.hp !== hud.get().hp) hud.set({ hp: m.me.hp });
        if (m.me.team !== undefined && m.me.team !== myTeam) { myTeam = m.me.team; hud.set({ myTeam }); }
        if (m.me.money !== undefined && m.me.money !== hud.get().money) hud.set({ money: m.me.money });
        const gear = { armor: m.me.armor || 0, helmet: !!m.me.helmet, kit: !!m.me.kit, shades: !!m.me.shades, boost: Math.ceil((m.me.w?.bo || 0) / 100), canBuy: !!m.me.buy, warmup: !m.round || m.round.ph === "warmup" };
        const hs = hud.get();
        if (gear.armor !== hs.armor || gear.helmet !== hs.helmet || gear.kit !== hs.kit || gear.shades !== hs.shades || gear.boost !== hs.boost || gear.canBuy !== hs.canBuy || gear.warmup !== hs.warmup) hud.set(gear);
        if (!gear.canBuy && hs.buyMenu) closeBuyMenu(); // left the zone or buy time ran out
        nades.flashed(m.me.fl || 0, m.me.fld || 0, now);
      }
      items = m.items || [];
      nades.ingest(m.nades, now);
      spot = new Set(m.spot || []);
      if (m.round) rounds.ingest(m.round, now);
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
      const players = m.players.filter((p) => p.id !== myId).map(({ id, name, color, team, k, d, a, sp }) => ({ id, name, color, team, k: k || 0, d: d || 0, a: a || 0, sp: sp || 0 }));
      if (JSON.stringify(players) !== JSON.stringify(hud.get().players)) hud.set({ players });
      const mine = m.players.find((p) => p.id === myId);
      if (mine && ((mine.k || 0) !== hud.get().myK || (mine.d || 0) !== hud.get().myD || (mine.a || 0) !== hud.get().myA || (mine.sp || 0) !== hud.get().mySp)) {
        hud.set({ myK: mine.k || 0, myD: mine.d || 0, myA: mine.a || 0, mySp: mine.sp || 0 });
      }
      break;
    }
    case "hit": // server-confirmed hit: sound, marker, floating number, console line
      testCtl.lastHit = m;
      sfx.bonk();
      hitMarker = 0.25;
      damageNumbers.push({ at: m.at, text: `${m.dmg}`, part: m.part, life: 1.2 });
      { const w = WEAPONS[m.w];
        if (w?.hitGlyph) damageNumbers.push({ at: [m.at[0], m.at[1] + 0.35, m.at[2]], text: w.hitGlyph, part: "glyph", life: 0.9 });
        if (w?.hitText) damageNumbers.push({ at: [m.at[0], m.at[1] + 0.6, m.at[2]], text: w.hitText, part: "review", life: 1.6 }); }
      con.print(`hit ${m.name}: ${m.part} for ${m.dmg}`, "dim");
      break;
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
      hud.set({ buyMsg: m.ok ? "" : m.why });
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
      impacts.push({ life: 2, color: [0.3, 0.55, 1], end: m.end, poses: m.poses.map((r) => ({ p: r.p, yaw: r.yaw, pitch: r.pitch, crouch: !!r.c, team: r.team, id: r.id })) });
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
      hud.set({ openAdmin: !!m.openAdmin });
      break;
    case "map": // changelevel
      map = m.map; gfx.setMap(map); phys.setMap(map);
      hud.set({ map: map.name });
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
      if (me && !spectating()) { me.pos = [...m.p]; me.vel = [0, 0, 0]; me.onGround = true; // = the server respawn
         me.yaw = m.yaw ?? Math.atan2(m.p[0], m.p[2]); me.dead = false; predictor.reset(); arsenal.respawned(); prevButtons = 0; snapView(); }
      hud.set({ myTeam, ...(m.hp !== undefined && !spectating() ? { hp: m.hp } : {}) });
      observer.reset(); // alive again (or a fresh spectator): no death cam
      if (switched) hud.feed(joinedText(hud.get().name, myTeam));
      break;
    }
    case "teamfull": // the server refused: that side would be 2+ players bigger
      hud.feed(`The ${teamInfo(m.team)?.name ?? m.team} are full. Pick the other side.`);
      openTeamMenu();
      break;
    case "hurt": // the server damaged us (optional event: HP itself is in every snapshot)
      hud.set({ hp: m.hp });
      (m.ar ? sfx.hurtArmor : sfx.hurtFlesh)();
      if (m.fp) dmgFrom.push({ p: m.fp, at: performance.now(), dmg: m.dmg });
      break;
    case "kill": { // kill feed; real kills drive the announcer
      const nameOf = (id) => (id === 0 ? "" : id === myId ? hud.get().name : known.get(id)?.name ?? `#${id}`); // 0: the world (a blast)
      const teamOf = (id) => (id === myId ? myTeam : known.get(id)?.team);
      const wname = WEAPONS[m.w]?.name ?? (m.w === "bigdiff" ? "The Big Diff" : m.w);
      hud.feed(`${m.k ? nameOf(m.k) + " " : ""}[${wname}${m.ns ? " noscope" : ""}${m.hs ? " ✚head" : ""}] ${nameOf(m.v)}`, {
        killer: nameOf(m.k), victim: nameOf(m.v), kteam: teamOf(m.k), vteam: teamOf(m.v),
        weapon: m.w, wname, hs: !!m.hs, ns: !!m.ns, mine: m.k === myId || m.v === myId,
      });
      if (m.v === myId && me) {
        me.dead = true; hud.set({ hp: 0 });
        // Death cam: freeze on the killer, then follow a teammate until the respawn.
        observer.died({ killer: m.k === myId ? 0 : m.k, eye: eyePos(), yaw: me.yaw, pitch: me.pitch, weapon: m.w, team: myTeam });
      }
      if (m.v === myId) announcer.onDeath(); // ends the spree
      if (m.k === myId && m.v !== myId && teamOf(m.v) !== myTeam) announcer.onKill({ headshot: !!m.hs }); // a team kill is no streak
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
const predictor = createPredictor(phys);
const arsenal = createArsenal({ net, hud, zoomMode: () => settings.get().zoom_mode });
const observer = createObserver({ raycast: (o, d, maxT) => phys.raycast(o, d, maxT) });
const nades = createNades({ sfx, spatial, myId: () => myId });
const radar = createRadar();
let spot = new Set(); // enemies our team sees now (the server's list in each snapshot)
// Full-screen tint over the world (not the HUD): the flash whiteout, the inside of a smoke.
const screenFx = document.createElement("div");
screenFx.className = "screenfx";
document.getElementById("hud")?.before(screenFx);
let screenFxKey = "";
const rounds = createRounds({
  hud, sfx, say: (id) => voice.say(id), announcer, spatial,
  feed: (t) => hud.feed(t),
  me: () => ({ id: myId, team: myTeam, pos: me && !spectating() ? me.pos : null, dead: !!me?.dead, held: arsenal.cur }),
  nameOf: (id) => (id === myId ? hud.get().name : known.get(id)?.name ?? `#${id}`),
  c4time: () => Number(serverCvars.mp_c4timer) || 40,
});

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

input.onLock = (locked) => {
  hud.set({ aiming: locked });
  if (locked) hud.set({ screen: "playing" });
  else if (wantPlay && hud.get().screen === "playing" && !hud.get().console && performance.now() > lockingUntil && !hud.get().teamMenu && !hud.get().buyMenu) openMenu();
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
  hud.set({ buyMenu: true, buyMsg: "", buyKey: key, buyKeyName: keyName(key) });
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
  disconnect() { leaveMatch(false); },
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

fetch("/map.json").then((r) => r.json()).then((m) => { if (!map) { map = m; gfx.setMap(m); phys.setMap(m); } });

setInterval(() => {
  hud.set({ ping: net.ping });
}, 1000);
// Movement goes to the server as per-tick commands (simTick); the server moves us.

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

// The server fires the same shot from the same command: spread is seeded by
// (player id, command seq) exactly like internal/game SpreadSeed.
function shoot(w, now) {
  testCtl.shots++;
  const spread = serverCvars.weapon_spread === "0" ? 0 : arsenal.spread(me);
  const eye = eyePos();
  const seen = bodies(remotes.sample(now));
  const [yaw0, pitch0] = recoilAngles(me.yaw, me.pitch, recoilOffset(w.recoil?.pattern, arsenal.shotIdx), recoilScale());
  const angles = shotAngles(yaw0, pitch0, spread, (Math.imul(myId, 2654435761) ^ cmdSeq) >>> 0, w.pellets);
  let shotRay = null;
  angles.forEach(([yaw, pitch], i) => {
    const b = basis(yaw, pitch);
    let hit = phys.raycast(eye, b.fwd), color = PALETTE.spark;
    if (hit && hit.t > w.range) hit = null;
    const ray = { seq: cmdSeq, o: eye, d: b.fwd, hit: null };
    if (i === 0) {
      shotRay = ray;
      testCtl.lastShotRay = ray;
      if (serverCvars.sv_showimpacts === "1") { // what we shot at (red); the server's answer is blue
        const end = eye.map((v, k) => v + b.fwd[k] * Math.min(hit?.t ?? w.range, w.range));
        impacts.push({ life: 2, color: [1, 0.25, 0.2], end, poses: seen.map((r) => ({ p: r.p, yaw: r.yaw, pitch: r.pitch, crouch: !!r.c, team: r.team, id: r.id })) });
        if (impacts.length > 12) impacts.shift();
      }
    }
    // Predicted player hit: the server's own test (rayModel = RayModel) on the
    // pose we see. Only the impact puff is shown now; the hit sound, marker and
    // damage come with the server's confirmation ("hit").
    for (const r of seen) {
      if (!friendlyFire() && r.team === myTeam) continue; // shots pass through teammates
      const h = rayModel(PLAYER, { p: r.p, yaw: r.yaw, pitch: r.pitch, crouch: !!r.c, team: r.team, id: r.id }, eye, b.fwd);
      if (h && h.t <= w.range && h.t < (hit?.t ?? Infinity)) {
        hit = { t: h.t, n: [0, 0, 0] }; color = teamInfo(r.team)?.rgb ?? hexRgb(r.color);
        ray.hit = { id: r.id, region: h.region, name: h.name, t: h.t };
        if (!shotRay.hit) shotRay.hit = ray.hit;
      }
    }
    if (hit) {
      const p = eye.map((v, k) => v + b.fwd[k] * hit.t + hit.n[k] * 0.05);
      for (let k = 0; k < (angles.length > 1 ? 2 : 4); k++) {
        puffs.push({ p: [...p], v: [hit.n[0] + rs(), hit.n[1] + rs() + 0.5, hit.n[2] + rs()], life: 0.4, size: 0.12, color });
      }
    }
  });
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
const eyePos = () => [me.pos[0], me.pos[1] + me.eyeH + me.eyeOff, me.pos[2]];


// Test harness hooks (server -test plus ?test=1): frames are stepped by the
// harness with a fixed dt, the shader clock can be pinned, the camera overridden.
const TEST = document.querySelector('meta[name="cs-test"]')?.content === "1" && new URLSearchParams(location.search).has("test");
const testCtl = { pinnedTime: null, camera: null, noSpread: false, lastHit: null, shots: 0 };

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
  return s.screen === "playing" && !s.console && !s.teamMenu && !s.buyMenu && !s.chatOpen;
}

// Sample input into one command (engine/cmd.js has the shape; game/controls.js builds it).
const buildCmd = () => controls.buildCmd({ play: inPlay(), me, seq: ++cmdSeq, dt: simDt, frozen: rounds.frozen() });

// Damage direction arcs: where recent hits came from (attacker eye at the
// shot), re-aimed every frame as we turn. Low HP: a heartbeat.
const dmgFrom = [];
const DMG_ARC_MS = 1500;
let nextBeat = 0;
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
  if (me && !me.dead && !spectating() && hp > 0 && hp <= 25 && t >= nextBeat) {
    sfx.heartbeat();
    nextBeat = t + 600 + hp * 20;
  }
}

let lastSnapTick = -1;
const CMD_REDUNDANCY = 4; // commands per packet: the new one + up to 3 unacknowledged

// Weapon reconciliation, like the body's: our weapon state after each command,
// compared with the server's at its ack; on a difference, adopt the server's
// and replay the commands it hasn't applied yet (quietly: no sounds, no HUD).
const weaponHist = new Map(); // seq -> {tick, w}
const wkey = (w) => JSON.stringify([Object.keys(w.slots || {}).sort().map((k) => [k, w.slots[k]?.id, w.slots[k]?.ammo, w.slots[k]?.reserve]), w.cur, w.ns, w.ce, w.re, !!w.trig, w.bl || 0, w.bo || 0, w.ri || 0, w.ri ? w.ls || 0 : 0]);
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
  for (const k of ["kick", "flash", "bob", "raise", "swing", "burst", "scoped", "zoomWant", "pendingSwitch", "pendingReload", "last"]) keep[k] = arsenal[k];
  arsenal.quiet = true;
  try {
    arsenal.setWstate(server, rec.tick);
    for (const c of predictor.pendingCmds()) {
      const sp = arsenal.moveSpeed(); // the body replay moves at the corrected speed (a drink the server just sold us)
      if (sp !== c.speed) { c.speed = sp; speedChanged = true; }
      arsenal.tick(simDt, !!(c.b & BTN.FIRE), replayBody, () => {}, false, { w: c.w, reload: !!(c.b & BTN.RELOAD) });
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
  if (me && map && !spectating()) {
    const cmd = buildCmd();
    prevView = viewOf(me);
    const b = cmd.b;
    // Send it (the server runs the same command) and keep it for replay.
    cmd.yaw = round5(cmd.yaw); cmd.pitch = round5(cmd.pitch); // exactly what the server gets
    if (me.dead) { prevButtons = b; vmPrev = vmCur; return; } // dead: no input until the server respawns us
    // A fire command says which server tick the other players were drawn at (lag compensation).
    const vt = b & BTN.FIRE ? Math.round(remotes.viewTick(now) * 100) / 100 : undefined;
    cmd.wire = { s: cmd.seq, f: cmd.f, r: cmd.r, y: cmd.yaw, p: cmd.pitch, b, vt, w: cmd.w };
    // Each packet also repeats the last few unacknowledged commands, so a lost
    // packet costs nothing (the server ignores seqs it already has): the
    // protocol never relies on every packet arriving, or arriving in order.
    const resend = predictor.pendingCmds().slice(-(CMD_REDUNDANCY - 1)).map((c) => c.wire);
    if (net.open) net.send({ t: "cmd", cmds: [...resend, cmd.wire] });
    netDebug.sent(cmd.seq, !!(b & BTN.FIRE), performance.now());
    predictor.push(cmd);
    vmPrev = vmCur;
    const { ev, vm } = stepLocal({ phys, arsenal, me, cmd, prevB: prevButtons, dt: simDt, shoot: (w) => shoot(w, now), zoomHeld: down("+zoom") && inPlay() });
    vmCur = vm;
    cmd.pos = [...me.pos]; // predicted result, compared with the server's when acked (cl_netdebug)
    recordWeapons(cmd.seq);
    prevButtons = b;
    if (ev.jumped) sfx.jump();
    if (ev.landed) sfx.land(ev.landed);
    for (let i = 0; i < ev.steps; i++) sfx.footstep();
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
  for (const im of impacts) im.life -= simDt;
  while (impacts.length && impacts[0].life <= 0) impacts.shift();
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

  if (me && map && !observing()) {
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
  simAcc += dt * (testCtl.clockScale ?? 1); // harness: a client clock running fast/slow
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
  rounds.update(dt, now, map);
  nades.update(dt);
  draw = draw && gfx.canRender(); // r_gpu_fence: keep the sim running, skip the frame
  updateDamageNumbers(dt, draw);
  if (draw) render(now, alpha, latch);
  return draw;
}

function render(now, alpha, latch) {
  updateDamageHud(performance.now());
  const time = testCtl.pinnedTime ?? now / 1000;
  let eye, yaw, pitch, viewmodel = null, watched = null, watchFirst = false;

  if (me && map && !observing()) {
    const pv = prevView || viewOf(me);
    const pos = pv.pos.map((v, i) => lerp(v, me.pos[i], alpha) + predictor.error[i]); // + smoothed reconciliation error
    eye = [pos[0], pos[1] + lerp(pv.eyeH, me.eyeH, alpha) + lerp(pv.eyeOff, me.eyeOff, alpha), pos[2]];
    const punch = arsenal.viewPunch(arsenal.tickN + alpha, recoilScale());
    yaw = me.yaw - punch[1]; pitch = me.pitch + punch[0];
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
  if (marksFor !== map) { marksFor = map; marks = spawnMarks(map); } // team start pads + buy-zone outlines
  solids.push(...marks);
  for (const it of items) solids.push(...itemParts(it.p, it.w, time));
  solids.push(...rounds.solids(players, time, now, watchFirst ? watched?.id : 0));
  solids.push(...nades.solids(time, now));
  for (const p of puffs) solids.push({ m: chain(translate(...p.p), scale(p.size, p.size, p.size)), color: p.color, emit: 1 });
  if (testCtl.camera) {
    // Free camera: the local player is drawn too, since we're not looking through their eyes.
    ({ eye, yaw, pitch } = testCtl.camera);
    if (me && !spectating()) solids.push(...playerParts({ id: myId, team: myTeam, p: me.pos, yaw: me.yaw, pitch: me.pitch, rgb: hexRgb(myColor), time, crouch: me.crouched ? 1 : 0 }));
    viewmodel = null;
  }
  for (const im of impacts) {
    for (const st of im.poses) for (const h of hitParts(st)) solids.push(...wireBoxIn(h.m, h.c, h.s, im.color));
    if (im.end) solids.push(...wireBox(im.end.map((v) => v - 0.05), im.end.map((v) => v + 0.05), im.color)); // impact point
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
  if (!testCtl.camera) eye = rounds.shakeEye(eye);
  gfx.frame({ eye, yaw, pitch, time, solids, overlay, fov: scope ? scope.fov : undefined, latch });
  const hs = hud.get(), cfg = settings.get();
  radar.draw({
    map, me: me && !observing() ? { pos: eye, yaw } : null, team: spectating() ? "spec" : myTeam, players, spot,
    bomb: rounds.state?.bomb, rotate: !!cfg.cl_radar_rotate, scale: cfg.cl_radar_scale, now,
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
  updateTags(shown, eye, yaw, pitch);
}

// The observer HUD line: who you watch (name, team, HP, weapon, ammo), the
// camera mode, or the death cam caption. Only pushed to the HUD on change.
function showObserverHud(watched, first) {
  let o = null;
  if (observing()) {
    const f = observer.freeze, k = f && remotes.sample(performance.now()).find((r) => r.id === f.killer);
    o = f ? { freeze: true, killer: f.killer ? k?.name ?? "someone" : f.weapon === "bigdiff" ? "the merge" : "yourself", killerTeam: k?.team ?? "", weapon: WEAPONS[f.weapon]?.name ?? f.weapon ?? "" }
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
function updateTags(players, eye, yaw, pitch) {
  const view = { eye, yaw, pitch, team: myTeam, observing: observing() || !!testCtl.camera };
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
const netDebug = createNetDebug(INTERP_DELAY);
settings.subscribe((s) => netDebug.show(!!s.cl_netdebug));
netDebug.show(!!settings.get().cl_netdebug);
setInterval(() => { if (settings.get().cl_netdebug) netDebug.render(performance.now(), { ping: net.ping, prediction: predictor.stats, audio: sfx.audioState() }); }, 100);
const applyNetFaults = (s) => { net.fakeLag = s.net_fakelag || 0; net.faults = { loss: s.net_fakeloss || 0, dup: s.net_fakedup || 0, jitter: s.net_fakejitter || 0 }; };
settings.subscribe(applyNetFaults);
applyNetFaults(settings.get());
net.fakeLag = settings.get().net_fakelag || 0;
const syncAnnouncer = (s) => voice.setAnnouncer({ volume: s.announcer_volume, enabled: s.cl_announcer, onLine: showCaption });
function showCaption(l) {
  if (!settings.get().cl_captions || voice.LINES[l.id]?.banner) return; // banner lines: the HUD shows them (round end)
  hud.caption({ text: l.text, color: l.color, dur: l.dur, kind: l.voice === "taunt" ? "sub" : "shout" });
}
syncAnnouncer(settings.get());
settings.subscribe(syncAnnouncer);

// Debug handle for the console / automated checks.
window.cs = { get me() { return me; }, remotes, net };

if (TEST) {
  installTestApi({
    frame, testCtl, input, snapView, latencyReadout, keyFor, get simDt() { return simDt; }, loop,
    resetSim() { simAcc = 0; prevButtons = 0; controls.reset(); vmCur = vmPrev = null; snapView(); }, hud, con, settings, net, remotes, gfx, announcer,
    get me() { return me; }, get myId() { return myId; }, get map() { return map; }, arsenal, items: () => items, nades, radar, get spot() { return spot; }, hitParts, observer, rounds, predictor, actions: ACTIONS, wkey: (w) => wkey(w), settingsKey: "cs.settings", audioState: () => sfx.audioState(), restartPage: () => { skipLeaveConfirm = true; location.reload(); }, get netDebug() { return netDebug; }, bumpSeq: () => ++cmdSeq, sampleRemotes: () => remotes.sample(performance.now()),
    get spawnSeq() { return spawnSeq; }, get myTeam() { return myTeam; },
    play: (name) => startPlay(name),
  });
}

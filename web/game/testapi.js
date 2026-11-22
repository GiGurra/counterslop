// window.__cs: the test harness's handle on a client (cmd/sim drives it over CDP).
// Installed only when the server runs with -test AND the page has ?test=1, so
// it never exists in prod. Every call returns plain JSON; expect*/rateCheck
// throw on failure, which fails the scenario.
import { mulberry32 } from "../gen/rng.js";
import { overview } from "./observer.js";
import { BTN } from "../engine/cmd.js";
import { segmentBlocked } from "./tags.js";
import { CVARS } from "../ui/settings.js";

// The harness's own waits run on real timers: in a lockstep run the page's
// clocks and timers are virtual (cmd/sim/vclock.js) and only move when the
// harness advances them.
const setTimeout = globalThis.__real?.setTimeout ?? globalThis.setTimeout.bind(globalThis);
const wallNow = globalThis.__real?.perfNow ?? (() => performance.now());
// Waiting on the browser itself (a shader compiling, a worker rendering
// previews): that work polls on the page's timers, so let the virtual clock
// run in 10 ms slices meanwhile. Loading only; never the server's game time.
const pumpPage = () => globalThis.__vclock?.advance(10);

const ARSENAL_KEYS = ["slots", "cur", "last", "tickN", "nextShot", "cycleEnd", "reloadEnd", "cycleLeft", "cycleTime", "pendingSwitch", "pendingReload", "reloadLeft", "reloadTime", "kick", "flash", "bob", "raise", "swing", "clicked", "scoped", "burst", "recoil", "lastShot", "shotIdx"];

// Lockstep (cmd/sim lockstep.go): the server only ticks when the harness steps
// it. A helper that waits for the server asks for game time through the gate:
// gameTick() parks until the harness has run one server tick (10 ms of game
// time) and every client has synced; so the game time a call takes is set by
// the state it waits for, never by how long the machine takes. Off (realtime
// runs, or no harness), the waits are wall-clock timers as before.
const gate = { lockstep: false, net: null, waiters: [], ticks: 0 };
const TICK_MS = 10;
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
function gameTick() {
  return new Promise((r) => gate.waiters.push(r));
}
// Everything the server has to say without a tick (welcome, replies to
// messages) is here: a ping round trip on the same socket.
const barrier = () => (gate.net?.barrier ? gate.net.barrier() : Promise.resolve(false));

// Wait for cond for up to ms: game time in lockstep (one tick at a time,
// cond re-checked after every tick and after a network barrier), wall time
// otherwise (long enough for a busy CI box). Resolves early.
async function until(cond, ms = 10000) {
  if (gate.lockstep) {
    for (let t = 0; ; t += TICK_MS) {
      if (cond()) return true;
      await barrier();
      if (cond() || t >= ms) return cond();
      await gameTick();
    }
  }
  const end = performance.now() + ms;
  while (!cond() && performance.now() < end) await sleep(10);
  return cond();
}

// ms of game time (lockstep: that many server ticks), else of wall time.
async function gameSleep(ms) {
  if (!gate.lockstep) return sleep(ms);
  for (let t = 0; t < ms; t += TICK_MS) await gameTick();
}

export function installTestApi(g) {
  gate.net = g.net;
  // Let Preact render. A timer, not rAF: background headless tabs throttle rAF.
  const nextFrame = () => new Promise((r) => setTimeout(r, 30)).then(() => globalThis.__vclock?.settle());
  const round = (v) => Math.round(v * 1000) / 1000;

  const api = {
    // Join the match and wait for the welcome, then pick a team from the
    // team-select screen (vibe | artisan | auto | spec; null = stay on that screen).
    // The harness can't take pointer lock (no user gesture), so play is entered
    // directly and aiming faked.
    async join(name = "bot", team = "auto") {
      g.play(name);
      await until(() => g.me);
      if (!g.me) throw new Error("no welcome from server");
      g.hud.set({ screen: "playing", aiming: true, name });
      if (team) await api.team(team);
      api.step(1);
      return { id: g.myId };
    },
    // Advance by n frames of dt seconds each (the sim runs its fixed ticks
    // inside) and render the last one.
    // The last shot as both sides saw it (cmd/sim writes it next to a failed step).
    shotDebug() {
      const c = g.testCtl;
      return { shots: c.shots, ray: c.lastShotRay, hit: c.lastHit, impacts: c.lastImpacts, viewTick: g.remotes?.viewTick(performance.now()) };
    },
    // One lockstep frame (cmd/sim): step, then report what the harness waits for.
    stepSeq(draw = false, dt = 1 / 60) {
      g.frame(performance.now(), dt, draw);
      return { seq: g.netDebug?.state.sent ?? 0, id: g.myId ?? 0 };
    },
    // Render cost: n frames with gl.finish after each, frozen in time (dt 0); returns
    // ms per frame {avg, min, max} and the renderer's light stats. Comparisons only make
    // sense within one machine (headless is SwiftShader: a CPU rasteriser).
    // Renderer options for a look under the hood (engine/gl.js opts), e.g. {lightDebug: 1}.
    gfxOptions(o) { g.gfx.setOptions(o); },
    // Lamp shadows bake a few lights per rendered frame after a map loads; the harness
    // renders one frame per step, so render frozen frames (dt 0) until all are baked.
    settleLights(maxFrames = 300) {
      let i = 0;
      for (; i < maxFrames && g.gfx.stats().lightShadowQueued > 0; i++) g.frame(performance.now(), 0, true);
      const left = g.gfx.stats().lightShadowQueued;
      if (left) throw new Error(`settleLights: ${left} lights still to bake after ${i} frames`);
      return i;
    },
    // Benchmarks: fill the debris rings (holes, casings) around the local player's feet.
    debrisFill(holes = 1024, casings = 512) { g.debris.clear(); g.debris.testFill(g.me.pos, holes, casings); return g.debris.stats(); },
    // n muzzle lights of weapon id from points spread around the view (free camera or eye) (benches: frames
    // stepped with dt 0 keep them lit at their peak). Returns how many are lit.
    muzzleFlashes(n = 8, id = "prompt") {
      const v = g.testCtl.camera || g.testCtl.view; // the free camera when set
      for (let i = 0; i < n; i++) g.muzzleLight(id, [v.eye[0] + (i % 4) * 0.7 - 1, v.eye[1], v.eye[2] + (i >> 2) * 0.7], v.yaw, v.pitch);
      g.frame(performance.now(), 0, true);
      return g.gfx.stats().dynLights;
    },
    // Synthetic spray load: `shooters` remote shooters (as bots' "shot" events: no own
    // priority) walking a circle of `radius` m round `centre` at 4 m/s (a new cell almost
    // every shot), each firing weapon every `every` frames (6 = a rifle's 10/s at 60 fps),
    // staggered; n rendered frames of 1/60 s. Continues the same walk on the next call.
    // Returns this call's worst and median CPU time per frame spent adding muzzle lights
    // (the shot path: raycast, mask cache, pool, upload) and the renderer's stats.
    sprayLoad(n = 10, { shooters = 10, centre = [0, 0, -20], radius = 15, every = 6, weapon = "prompt" } = {}) {
      const t = g.testCtl;
      t.sprayF ??= 0;
      const per = [];
      let shots = 0;
      for (let i = 0; i < n; i++, t.sprayF++) {
        const f = t.sprayF, t0 = performance.now();
        for (let k = 0; k < shooters; k++) {
          if ((f + k) % every) continue;
          const a = (2 * Math.PI * k) / shooters + (4 * f) / 60 / radius;
          g.muzzleLight(weapon, [centre[0] + radius * Math.cos(a), centre[1] + 1.62, centre[2] + radius * Math.sin(a)], a, 0);
          shots++;
        }
        per.push(performance.now() - t0);
        g.frame(performance.now(), 1 / 60, true);
      }
      const r = g.gfx.stats(), sorted = [...per].sort((a, b) => a - b);
      const out = {
        spray: shooters, frames: n, shots, shotCpuMaxMs: round(sorted[n - 1]), shotCpuMedianMs: round(sorted[n >> 1]),
        frameMsMax: round(r.dynLightFrameMsMax), floods: r.dynLightFloods, hits: r.dynLightMaskHits, skips: r.dynLightMaskSkips,
        dropped: r.dynLightsDropped, added: r.dynLightsAdded, uploads: r.dynLightUploads, lit: r.dynLights, shadows: r.dynShadows, shadowRenders: r.dynShadowRenders,
      };
      (g.benchLog ||= []).push(out);
      return out;
    },
    // opts.lights false: the same frames without map lights or sky occlusion (A/B).
    // opts.dt: seconds each frame advances (default 0: frozen frames; 1/60 runs the sim,
    // e.g. with fire held: the frame time while spraying).
    bench(n = 30, opts = {}) {
      g.gfx.setOptions({ lights: opts.lights ?? true });
      const dt = opts.dt ?? 0;
      g.frame(performance.now(), 0, true); // warm up
      g.gfx.sync();
      const ms = [];
      for (let i = 0; i < n; i++) { const t = performance.now(); g.frame(performance.now(), dt, true); g.gfx.sync(); ms.push(performance.now() - t); }
      const r = g.gfx.stats(), avg = ms.reduce((a, b) => a + b, 0) / n;
      g.gfx.setOptions({ lights: true });
      const out = { avg: round(avg), min: round(Math.min(...ms)), max: round(Math.max(...ms)), lights: r.lights, active: r.lightsActive, shadows: r.lightShadows, shadowCpuMs: round(r.lightShadowCpuMs), buildMs: round(r.lightBuildMs), map: g.map?.name, on: opts.lights ?? true, marks: r.marks, median: round(ms.sort((a, b) => a - b)[n >> 1]), lightShadowBudget: r.lightShadowBudget, dynLights: r.dynLights, dynLightsAdded: r.dynLightsAdded, dynLightAddMs: round(r.dynLightAddMs ?? 0), dynLightAddPeakMs: round(r.dynLightAddPeakMs ?? 0), shots: g.testCtl.shots, gpu: g.gfx.info().gpu };
      (g.benchLog ||= []).push(out);
      return out;
    },
    // Until a map that is loading is drawn (its shader compiles in the background):
    // frozen frames (dt 0), yielding so the driver can finish. Snapshots wait on it.
    async mapReady(ms = 30000) {
      const t0 = wallNow();
      while (g.hud.get().loadingMap) {
        if (wallNow() - t0 > ms) throw new Error(`map ${g.hud.get().loadingMap} still loading after ${ms} ms`);
        g.frame(performance.now(), 0, true);
        await new Promise((r) => setTimeout(r, 5));
        await pumpPage();
      }
      return true;
    },
    step(n = 1, dt = 1 / 60) {
      for (let i = 0; i < n; i++) g.frame(performance.now(), dt, i === n - 1);
      return api.state();
    },
    // Controls menu: click the key button of the row labelled `label`, then press `code`
    // (KeyboardEvent.code, or MouseN = raw event.button / MWheelUp / MWheelDown) like a player would.
    async uiRebind(label, code) {
      const row = [...document.querySelectorAll(".controls .row.bind")].find((r) => r.firstElementChild?.textContent.trim() === label);
      if (!row) throw new Error(`no controls row "${label}"`);
      row.querySelector(".rebind").click();
      for (let i = 0; i < 100 && !row.querySelector(".rebind.waiting"); i++) { await new Promise((r) => setTimeout(r, 10)); await globalThis.__vclock?.settle(); }
      const m = /^Mouse(\d+)$/.exec(code);
      if (m) dispatchEvent(new MouseEvent("mousedown", { button: +m[1] }));
      else if (/^MWheel/.test(code)) dispatchEvent(new WheelEvent("wheel", { deltaY: code === "MWheelDown" ? 100 : -100, cancelable: true }));
      else dispatchEvent(new KeyboardEvent("keydown", { code, cancelable: true }));
      await nextFrame();
      return g.settings.get().binds;
    },
    // Arrange the rejoin a reload does (version update, renderer restart): the
    // next page load goes straight back into the match, no Play button.
    prepareRejoin() { sessionStorage.setItem("cs.rejoin", JSON.stringify({ name: g.hud.get().name, inGame: true })); return true; },
    // Store a saved-settings blob as an older client wrote it (applies on the next load).
    saveSettingsBlob(blob) { localStorage.setItem(g.settingsKey, JSON.stringify(blob)); return true; },
    // Reload the way the game does (no leave prompt); the harness "reload" step calls it.
    restartPage() { g.restartPage(); return true; },
    // Drop and re-open the server connection (a redeploy) without reloading.
    reconnect() { g.net.disconnect(); g.net.connect(g.hud.get().name || "slopper"); return true; },
    // Press and release a key (KeyboardEvent.code) through the real input
    // path, as if the pointer were locked in play.
    // Fail unless the chat log has every string in want and none in lacks.
    expectChat(want = [], lacks = []) {
      const text = (g.hud.get().chat || []).map((l) => `${l.radio ? "(RADIO) " : l.tm ? "(TEAM) " : ""}${l.name}: ${l.text}`).join("\n");
      const miss = want.filter((w) => !text.includes(w)), extra = lacks.filter((w) => text.includes(w));
      if (miss.length || extra.length) throw new Error(`expectChat: missing ${JSON.stringify(miss)}, unexpected ${JSON.stringify(extra)} in ${JSON.stringify(text)}`);
      return text;
    },
    // Keep damage arcs on screen (they fade in 1.5 s of wall time; captures are slower).
    // Pin every corpse's death age (s) so a snapshot catches it mid-fall
    // (snapshots tick the server on wall time); null follows the server again.
    corpseAge(s = null) { g.testCtl.corpseAge = s; return true; },
    holdDamageArcs(on = true) { g.testCtl.holdDamageArcs = on; return true; },
    // Lose exactly the next n command packets (more than the redundancy covers).
    dropCmds(n) { g.net.dropCmds = n; return true; },
    // Fail unless client and server agree on our state: every command acked,
    // the body where the server has it, the weapon state identical.
    async expectInSync() {
      const nd = g.netDebug.state;
      // The server runs our last commands on its next ticks (lockstep: game time, a tick or two).
      await until(() => g.testCtl.lastMe && g.testCtl.lastMe.ack >= nd.sent, 1000);
      const sv = g.testCtl.lastMe, me = g.me;
      if (!sv) throw new Error("no snapshot with our state yet");
      const out = { sent: nd.sent, ack: sv.ack, pos: me.pos.map(round), server: sv.p.map(round), client: g.wkey(g.arsenal.wstate()), serverW: g.wkey(sv.w), corrections: g.predictor.stats.corrections, weaponCorrections: g.predictor.stats.weaponCorrections || 0 };
      const posErr = Math.hypot(...me.pos.map((v, i) => v - sv.p[i]));
      if (sv.ack !== nd.sent || posErr > 1e-6 || out.client !== out.serverW) throw new Error(`out of sync: ${JSON.stringify(out)}`);
      return out;
    },
    // Fail unless every bindable plain action is a command the console runs.
    expectActionsWired() {
      const missing = g.actions.filter((x) => x.cmd[0] !== "+" && !g.con.has(x.cmd)).map((x) => x.cmd);
      if (missing.length) throw new Error(`actions without a command: ${missing.join(", ")}`);
      return true;
    },
    keyDebug(code) {
      const cmd = g.settings.get().binds[code];
      const before = g.arsenal.pendingReload;
      api.keyTap(code);
      return { cmd, screen: g.hud.get().screen, teamMenu: g.hud.get().teamMenu, conOpen: g.con.open, pendingReload: g.arsenal.pendingReload, pendingSwitch: g.arsenal.pendingSwitch, before };
    },
    // ---- lockstep sim/cases (cmd/sim -cases) ----
    // One tick through the real input path: codes in `hold` are held (real
    // keydown/keyup or mousedown/mouseup events on change), `tap` codes are pressed
    // and released before the tick (MWheelUp/Down = a wheel notch), the view is set
    // to yaw/pitch (mouse look), then one fixed 10 ms frame runs. Returns caseState().
    caseTick({ hold = [], tap = [], yaw, pitch, mouse } = {}) {
      const was = g.input.locked;
      g.input.locked = true;
      try {
        for (const c of [...caseHeld]) if (!hold.includes(c)) { ev(c, false); caseHeld.delete(c); }
        for (const c of hold) if (!caseHeld.has(c)) { ev(c, true); caseHeld.add(c); }
        for (const c of tap) {
          if (c.startsWith("MWheel")) dispatchEvent(new WheelEvent("wheel", { deltaY: c === "MWheelDown" ? 100 : -100 }));
          else { ev(c, true); ev(c, false); }
        }
      } finally { g.input.locked = was; }
      if (yaw !== undefined && g.me) g.me.yaw = yaw;
      if (pitch !== undefined && g.me) g.me.pitch = pitch;
      if (mouse) { // real mouse counts, as with the pointer locked: sensitivity, m_invert_y, zoom_sensitivity_ratio apply
        const was2 = g.input.locked;
        g.input.locked = true;
        try { dispatchEvent(new MouseEvent("mousemove", { movementX: mouse[0], movementY: mouse[1] })); } finally { g.input.locked = was2; }
      }
      g.frame(performance.now(), g.simDt, false); // no render: cases only need the state
      return api.caseState();
    },
    // The local player's predicted state in sim/cases terms (web/game/cases.js FIELDS).
    // shout: the last centre caption said, not the one still on screen (that one times out in wall time).
    caseState() {
      const me = g.me, a = g.arsenal, h = a.slots[a.cur], h2 = g.hud.get(), r4 = (v) => Math.round(v * 1e4) / 1e4;
      if (!me) return { id: g.myId, connected: !!h2.connected, seq: g.netDebug?.state.sent ?? 0, scoreboard: !!h2.scoreboard, console: !!g.con.open };
      return {
        id: g.myId,
        pos: me.pos.map(r4), vel: me.vel.map(r4), onGround: me.onGround, crouched: me.crouched, yaw: r4(me.yaw), pitch: r4(me.pitch),
        cur: a.cur, ammo: h?.ammo ?? 0, reserve: h?.reserve ?? 0, reloading: !!a.reloadEnd, shots: g.testCtl.shots, spray: r4(a.recoil),
        scoped: a.scoped, jumps: me.jumps || 0, carry: !!a.carry, seq: g.netDebug?.state.sent ?? 0,
        scoreboard: !!h2.scoreboard, teamMenu: !!h2.teamMenu, buyMenu: !!h2.buyMenu, mvp: h2.mvp ? `${h2.mvp.name}: ${h2.mvp.why}` : "", shout: g.testCtl.lastShout, chatOpen: h2.chatOpen || "", radioOpen: !!h2.radioOpen, chatLines: (h2.chat || []).map((l) => `${l.radio ? "(RADIO) " : l.tm ? "(TEAM) " : ""}${l.name}: ${l.text}`).join("\n"), console: !!g.con.open, name: h2.name, team: h2.myTeam, connected: !!h2.connected,
        consoleText: [...document.querySelectorAll(".console-log div")].slice(-300).map((d) => d.textContent).join("\n"),
        binds: g.settings.get().binds,
        holes: g.debris?.stats().holes ?? 0, casings: g.debris?.stats().casings ?? 0,
        dynLights: (g.gfx.stats().dynLightsAdded ?? 0) - flashBase, // muzzle lights added since the case began
      };
    },
    // Before a case's first tick: release keys, zero the counters, and draw the
    // held weapon again (the harness's SetInventory drew it on the server too).
    caseReset() {
      for (const c of [...caseHeld]) { ev(c, false); caseHeld.delete(c); }
      g.testCtl.shots = 0; g.testCtl.lastShout = ""; g.me.jumps = 0; g.resetSim();
      g.debris?.clear(); // no holes or casings left over from the case before
      flashBase = g.gfx.stats().dynLightsAdded ?? 0;
      g.remotes.forget(); // = server ForgetPoses: nobody is drawn (or shot at) where they stood before the case
      g.arsenal.setInv({ cur: g.arsenal.cur, slots: g.arsenal.slots }, true);
      return api.caseState();
    },
    // Resend the unacknowledged commands, as the next packet would (cmd/sim lockstep under net_fakeloss).
    resendCmds() { return g.resendCmds(); },
    // Wait until the server's sync marker n has arrived, then report caseState().
    // Lockstep frames: every message the server sent before sync marker n is in (0: none asked).
    async syncTo(n) {
      for (let i = 0; n && i < 2000 && !(g.testCtl.sync >= n); i++) await new Promise((r) => setTimeout(r, 1));
      if (n && !(g.testCtl.sync >= n)) throw new Error(`sync ${n} never arrived`);
      return true;
    },
    async caseSync(n) {
      for (let i = 0; i < 2000 && !(g.testCtl.sync >= n); i++) await new Promise((r) => setTimeout(r, 1));
      if (!(g.testCtl.sync >= n)) throw new Error(`sync ${n} never arrived`);
      return api.caseState();
    },
    // Everything a player can do, for the action registry check (cmd/sim -cases):
    // bindable actions, command button bits, console commands (and which of them
    // the console forwards to the server as admin commands).
    registry() {
      return { actions: g.actions.map((a) => a.cmd), buttons: Object.keys(BTN).map((b) => b.toLowerCase()),
        console: g.con.commands(), forwarded: g.con.serverCommands(), cvars: Object.keys(CVARS) };
    },
    // The bound key for each command (sim/cases inputs go through the binds).
    keysFor(cmds = g.actions.map((a) => a.cmd)) { return Object.fromEntries(cmds.map((c) => [c, g.keyFor(c)])); },
    keyTap(code) {
      const was = g.input.locked;
      g.input.locked = true;
      try {
        // Like a real keyboard: to the focused element (usually body), bubbling up
        // through window, so capture listeners (menus) run before the game's.
        const t = document.activeElement || document.body;
        t.dispatchEvent(new KeyboardEvent("keydown", { code, cancelable: true, bubbles: true }));
        t.dispatchEvent(new KeyboardEvent("keyup", { code, cancelable: true, bubbles: true }));
      } finally { g.input.locked = was; }
      return true;
    },
    // Click a mouse button as the browser reports it (raw event.button) through
    // the real input path, as if the pointer were locked in play.
    mouseClick(button) {
      const was = g.input.locked;
      g.input.locked = true;
      try {
        dispatchEvent(new MouseEvent("mousedown", { button }));
        dispatchEvent(new MouseEvent("mouseup", { button }));
      } finally { g.input.locked = was; }
      return true;
    },
    // Fail unless binds[code] === cmd for every entry of want ({code: cmd|null}).
    expectBinds(want) {
      const b = g.settings.get().binds, bad = [];
      for (const [code, cmd] of Object.entries(want)) if ((b[code] ?? null) !== cmd) bad.push(`${code}: got ${b[code] ?? null}, want ${cmd}`);
      if (bad.length) throw new Error("expectBinds failed: " + bad.join("; "));
      return true;
    },
    // Scroll an element (e.g. ".options") so a snapshot shows its lower part.
    scroll(selector, y) { const el = document.querySelector(selector); if (!el) throw new Error(`no ${selector}`); el.scrollTop = y; return true; },
    // Let the real frame loop drive the game for ms (normally the harness steps frames).
    async live(ms = 1000) {
      g.testCtl.live = true;
      try { await new Promise((r) => setTimeout(r, ms)); } finally { g.testCtl.live = false; }
      return api.state().latency;
    },
    // Fail unless every dotted path of state() matches: {"me.jumps": 1, "me.pos.1": [">", 0.5]}.
    // expectState, polled until it holds (for state that arrives over the
    // network: hit feedback, round events) or `ms` pass; then it throws.
    async waitState(want, ms = 3000) {
      const ok = () => { try { api.expectState(want); return true; } catch { return false; } };
      await until(ok, ms);
      return api.expectState(want);
    },
    // The harness's side of the gate (cmd/sim lockstep.go). __lockstep: on/off.
    // __run starts a call without awaiting it; __poll resolves once it is done
    // or parked on gameTick (wants: the harness steps the server one tick,
    // syncs, then __ticked releases it).
    __lockstep(on) { gate.lockstep = !!on; return true; },
    // The server has read everything this page sent (a teleport, a console
    // line) before the harness ticks it: a ping round trip on the socket.
    __barrier() { return barrier().then(() => true); },
    __run(name, args) {
      const job = { done: false };
      gate.job = job;
      (async () => api[name](...args))().then((v) => { job.value = v ?? null; }, (e) => { job.err = String(e?.stack || e); }).finally(() => { job.done = true; });
      return true;
    },
    async __poll() {
      const job = gate.job;
      while (!job.done && !gate.waiters.length) await sleep(1);
      if (job.done) return { done: true, err: job.err ?? null, value: job.value ?? null };
      return { done: false, wants: true };
    },
    __ticked() {
      const w = gate.waiters; gate.waiters = []; gate.ticks++;
      w.forEach((r) => r());
      return true;
    },
    expectState(want) {
      const st = api.state(), bad = [];
      for (const [path, w] of Object.entries(want)) {
        const got = path.split(".").reduce((o, k) => o?.[k], st);
        const [op, v] = Array.isArray(w) ? w : ["==", w];
        const ok = op === "==" ? (typeof v === "number" ? Math.abs(got - v) < 1e-3 : got === v)
          : op === "!=" ? got !== v : op === ">" ? got > v : op === "<" ? got < v
          : op === "~" ? Math.abs(got - v[0]) <= v[1] : op === "has" ? String(got).includes(v) : op === "lacks" ? !String(got).includes(v) : false;
        if (!ok) bad.push(`${path}: got ${JSON.stringify(got)}, want ${op} ${JSON.stringify(v)}`);
      }
      if (bad.length) throw new Error("expectState failed: " + bad.join("; "));
      return true;
    },
    // Run the same input timeline at several frame rates (0 = "uncapped", a
    // 2400 fps step) from the same start and fail unless the sim ends in the
    // same state. spec: {pos, yaw, pitch, seconds, rates, timeline: [{t, keys, fire}]}
    // with t on multiples of 0.5 s (a frame boundary at every rate).
    rateCheck(spec) {
      const me = g.me;
      if (!me) throw new Error("not joined");
      const rates = spec.rates || [30, 60, 144, 1000, 0];
      const saved = JSON.parse(JSON.stringify(Object.fromEntries(ARSENAL_KEYS.map((k) => [k, g.arsenal[k]]))));
      const rnd = Math.random, results = [];
      try {
        for (const rate of rates) {
          const hz = rate || 2400, frames = Math.round(spec.seconds * hz);
          Object.assign(g.arsenal, JSON.parse(JSON.stringify(saved)));
          Math.random = mulberry32(1234);
          api.set({ pos: spec.pos, yaw: spec.yaw ?? 0, pitch: spec.pitch ?? 0, crouch: false });
          me.vel = [0, 0, 0]; me.onGround = false; me.eyeOff = 0; me.stride = 0; me.jumps = 0;
          g.resetSim();
          const tl = [...(spec.timeline || [])].sort((a, b) => a.t - b.t);
          for (const e of tl) if (Math.abs(e.t * hz - Math.round(e.t * hz)) > 1e-6) throw new Error(`timeline t=${e.t} is not a frame boundary at ${hz} fps`);
          let ti = 0;
          for (let i = 0; i < frames; i++) {
            while (ti < tl.length && tl[ti].t * hz <= i + 1e-6) { api.hold(tl[ti].keys || [], !!tl[ti].fire); ti++; }
            g.frame(performance.now(), 1 / hz, false);
          }
          api.hold([], false);
          const h = g.arsenal.slots[g.arsenal.cur];
          results.push({ rate, pos: me.pos.map(round), vel: me.vel.map(round), yaw: round(me.yaw), pitch: round(me.pitch),
            jumps: me.jumps, crouched: me.crouched, onGround: me.onGround, ammo: h?.ammo ?? null, reload: round(g.arsenal.reloadLeft) });
        }
      } finally {
        Math.random = rnd;
        Object.assign(g.arsenal, saved);
      }
      const ref = JSON.stringify({ ...results[0], rate: 0 });
      const diff = results.filter((r) => JSON.stringify({ ...r, rate: 0 }) !== ref);
      if (diff.length) throw new Error("rateCheck: results differ by frame rate: " + JSON.stringify(results));
      return results;
    },
    // Real-time frame pacing: set the loop mode/caps, let it run, then check
    // tick intervals. want: {mean: [ms, tol], maxSd: ms, vsyncRatio: n, refreshMultiple: n}
    async expectPacing(cvars, ms, want) {
      // fps_max "refresh/N": N-th of the measured refresh (headless rAF rates vary).
      const m = /^refresh\/(\d+)$/.exec(cvars.fps_max ?? "");
      if (m) {
        for (let i = 0; i < 100 && g.loop.rafs < 40; i++) await new Promise((r) => setTimeout(r, 50)); // measure the refresh first
        g.loop.pinnedRefresh = true; // headless rAF rates drift; keep the divisor stable
        cvars = { ...cvars, fps_max: g.loop.stats().hz / +m[1] };
      } else g.loop.pinnedRefresh = false;
      g.settings.set(cvars);
      await new Promise((r) => setTimeout(r, 300)); // settle + refresh measurement
      const t0 = g.loop.frames, r0 = g.loop.rafs;
      await new Promise((r) => setTimeout(r, ms));
      const rafs = g.loop.rafs - r0;
      const s = g.loop.stats(), out = { fps: round(s.fps), avg: round(s.avg), sd: round(s.sd), max: round(s.max), hz: round(s.hz), ticks: g.loop.frames - t0, rafs, presented: round(s.presented.fps) };
      const bad = [];
      if (want.mean && Math.abs(s.avg - want.mean[0]) > want.mean[1]) bad.push(`mean ${s.avg.toFixed(2)} ms, want ${want.mean[0]}±${want.mean[1]}`);
      if (want.maxSd !== undefined && s.sd > want.maxSd) bad.push(`sd ${s.sd.toFixed(2)} ms > ${want.maxSd}`);
      // vsync mode: exactly one tick per n vsyncs (headless rAF itself is too irregular to time).
      // "capped" = the divisor the loop chose (headless rAF rates drift, so refresh/2 may snap to 1/3).
      const ratio = want.vsyncRatio === "capped" ? g.loop.divisor : want.vsyncRatio;
      if (want.vsyncRatio === "capped" && ratio < 2) bad.push(`divisor ${ratio}, want a cap`);
      if (ratio && Math.abs(out.ticks * ratio - rafs) > Math.max(2, rafs * 0.05)) bad.push(`${out.ticks} ticks in ${rafs} vsyncs, want 1 per ${ratio}`);
      if (want.refreshMultiple) {
        const exp = want.refreshMultiple * (1000 / s.hz);
        if (Math.abs(s.avg - exp) > exp * 0.1) bad.push(`mean ${s.avg.toFixed(2)} ms, want ${want.refreshMultiple}× refresh = ${exp.toFixed(2)}`);
      }
      if (bad.length) throw new Error("expectPacing failed: " + bad.join("; ") + " " + JSON.stringify(out));
      return out;
    },
    // Teleport / turn / crouch the local player (the -test server allows teleports).
    set({ pos, yaw, pitch, crouch } = {}) {
      const me = g.me;
      if (!me) throw new Error("not joined");
      if (pos) { me.pos = [...pos]; me.vel = [0, 0, 0]; me.onGround = false; } // as the server's teleport: lands on the next command
      if (yaw !== undefined) me.yaw = yaw;
      if (pitch !== undefined) me.pitch = pitch;
      if (crouch !== undefined) { me.crouched = !!crouch; me.h = crouch ? 1.2 : 1.8; me.eyeH = crouch ? 1.05 : 1.62; }
      g.snapView();
      // The server owns movement: teleport its body too (test mode only) and restart prediction.
      // The teleport takes its own command seq: server states acking less predate it.
      const seq = g.bumpSeq();
      g.predictor.reset(seq);
      g.net.send({ t: "tp", p: me.pos, yaw: me.yaw, c: me.crouched, seq });
      return api.state();
    },
    // Hold exactly these keys (event.code names) until changed; fire = mouse button.
    // Hold exactly these codes; fire/zoom add the keys bound to +attack/+zoom.
    hold(keys = [], fire = false, zoom = false) {
      const extra = [fire && g.keyFor("+attack"), zoom && g.keyFor("+zoom")].filter(Boolean);
      g.input.setHeld([...keys, ...extra]);
      return true;
    },
    // Wait until every map card (Maps screen) or vote option has its rendered preview.
    async mapPreviews(timeoutMs = 60000) {
      const t0 = wallNow();
      for (;;) {
        const h = g.hud.get(), names = h.vote ? h.vote.options : (h.mapCards || []).map((c) => c.name);
        if (names.length && names.every((n) => h.mapPreviews?.[n])) { await nextFrame(); return names; }
        if (wallNow() - t0 > timeoutMs) throw new Error(`map previews not ready: have ${Object.keys(h.mapPreviews || {})}`);
        await new Promise((r) => setTimeout(r, 50));
        await pumpPage();
      }
    },
    // Click a map card on the Maps screen (changelevel).
    async pickMap(name) {
      const card = document.querySelector(`.map-card[data-map="${name}"]`);
      if (!card) throw new Error(`no map card ${name}`);
      card.click();
      await nextFrame();
      return true;
    },
    // Show a UI state: play | menu | options | controls | maps | console | scoreboard | teammenu
    async ui(name) {
      if (g.con.open && name !== "console") g.con.toggle(false);
      g.hud.set({ scoreboard: false, teamMenu: false });
      if (name === "play") g.hud.set({ screen: "playing" });
      else if (["menu", "options", "controls", "maps"].includes(name)) g.hud.set({ screen: "ingame", menuOpenedAt: 0 });
      else if (name === "console") g.con.toggle(true);
      else if (name === "scoreboard") g.hud.set({ scoreboard: true });
      else if (name === "teammenu") g.hud.set({ teamMenu: true });
      else throw new Error(`unknown ui state ${name}`);
      await nextFrame();
      const click = async (text) => {
        [...document.querySelectorAll("button")].find((b) => b.textContent.trim() === text)?.click();
        await nextFrame();
      };
      if (name === "options" || name === "controls") await click("OPTIONS");
      if (name === "controls") await click("CONTROLS ›");
      if (name === "maps") await click("MAPS");
      return true;
    },
    // Pick a team like the team-select screen does and wait for the server's answer.
    async team(t) {
      const seq = g.spawnSeq;
      g.hud.set({ teamMenu: false });
      g.con.exec(`jointeam ${t}`);
      // The server's (re)spawn for the new team. Already on it, the server does
      // nothing (no free respawn), so there is nothing to wait for: waiting ran
      // the 10 s until() out on every such call.
      if (t !== g.myTeam) await until(() => g.spawnSeq !== seq);
      api.step(1);
      return g.myTeam;
    },
    // Observer camera (spectating or dead): {mode: first|chase|free|overview,
    // follow: name, next: n, look: [dyaw, dpitch], fly: {f, r, up, fast, secs}}.
    observe({ mode, follow, next, look, fly } = {}) {
      const o = g.observer, players = g.sampleRemotes().filter((r) => r.team !== "spec");
      if (mode) o.setMode(mode);
      if (follow) { const t = players.find((r) => r.name === follow); if (!t) throw new Error(`no player ${follow}`); o.target = t.id; o.chase.set = false; }
      for (let i = 0; i < (next || 0); i++) o.next(players);
      if (look) o.update(0, look, { f: 0, r: 0, up: 0, fast: false });
      if (fly) for (let i = 0; i < Math.round((fly.secs ?? 1) * 60); i++) o.update(1 / 60, [0, 0], { f: fly.f || 0, r: fly.r || 0, up: fly.up || 0, fast: !!fly.fast });
      return { mode: o.mode, target: o.target, free: o.free.p.map(round), freeze: !!o.freeze };
    },
    exec(line) { g.con.exec(line); return true; },
    // Stand `dist` metres from the first bot and aim at its head/body/legs (zero spread aside).
    aimAtBot(part = "body", dist = 8) {
      const bot = g.remotes.sample(performance.now()).find((r) => /\(bot/.test(r.name));
      if (!bot) throw new Error("no bot in view");
      const pos = [bot.p[0], 0, bot.p[2] + dist];
      const y = { head: 1.6, body: 0.9, legs: 0.25 }[part] ?? 0.9;
      const eyeY = pos[1] + 1.62;
      api.set({ pos, yaw: 0, pitch: Math.atan2(bot.p[1] + y - eyeY, dist) });
      return { bot: bot.p };
    },
    // Weapons: switch to a slot (primary|sidearm|melee), drop, reload, scope.
    weapon(slot) { g.arsenal.switchTo(slot); return true; },
    drop() { g.arsenal.drop(); return true; },
    reload() { g.arsenal.reload(); return true; },
    // Zoom: hold mode sets the held button; toggle mode flips it.
    scope(on) {
      if (g.settings.get().zoom_mode === "toggle") g.arsenal.toggleScope();
      else setAction("+zoom", on ?? !g.input.down(g.keyFor("+zoom")));
      return true;
    },
    // Scoped sensitivity check: the same mouse delta moves a point at the screen
    // centre by the same number of pixels unzoomed and zoomed (±1 px). Throws otherwise.
    zoomSensCheck(counts = 40) {
      const me = g.me, shift = () => {
        const yaw0 = me.yaw, pitch0 = me.pitch;
        api.step(1, 0);
        const cp = Math.cos(me.pitch), eye = [me.pos[0], me.pos[1] + me.eyeH, me.pos[2]];
        const pt = [eye[0] - Math.sin(me.yaw) * cp * 20, eye[1] + Math.sin(me.pitch) * 20, eye[2] - Math.cos(me.yaw) * cp * 20];
        const a = g.gfx.project(pt);
        g.input.dx += counts;
        api.step(1, 0);
        const b = g.gfx.project(pt);
        me.yaw = yaw0; me.pitch = pitch0;
        return Math.hypot(b[0] - a[0], b[1] - a[1]);
      };
      const zoom = (on) => { g.settings.set({ zoom_mode: "hold" }); setAction("+zoom", on); api.step(2, 0.01); };
      zoom(false);
      const plain = shift();
      zoom(true);
      if (!g.arsenal.scoped) throw new Error("current weapon has no scope");
      const zoomed = shift();
      zoom(false);
      if (Math.abs(plain - zoomed) > 1) throw new Error(`zoom sensitivity mismatch: ${plain.toFixed(1)} px unzoomed vs ${zoomed.toFixed(1)} px zoomed`);
      return { plain, zoomed };
    },
    // Stand `dist` m from the first bot at compass angle `angle` (radians around
    // it, 0 = in front of it) and `height` m up, aim exactly (no spread) at the
    // centre of its `region` part as drawn, fire one shot and wait for the
    // server's verdict. Throws unless the server reports that region.
    async shootBot(region = "head", angle = 0, dist = 6, height = 0) {
      const bot = g.remotes.sample(performance.now()).find((r) => /\(bot/.test(r.name));
      if (!bot) throw new Error("no bot");
      const st = { p: bot.p, yaw: bot.yaw, pitch: bot.pitch, crouch: !!bot.c, team: bot.team, id: bot.id };
      const part = g.hitParts(st).find((h) => h.region === region);
      const m = part.m, c = part.c;
      const target = [0, 1, 2].map((i) => m[i] * c[0] + m[4 + i] * c[1] + m[8 + i] * c[2] + m[12 + i]);
      const a = bot.yaw + angle; // the bot faces -Z rotated by yaw
      const pos = [bot.p[0] - Math.sin(a) * dist, bot.p[1] + height, bot.p[2] - Math.cos(a) * dist];
      const me = g.me;
      api.set({ pos, crouch: false });
      api.step(30, 0.01); // weapon ready (deploy takes ~0.2 s of sim time); lands if it was in the air
      // The server runs those commands before the aiming teleport below: a teleport
      // resyncs the command stream, so commands it hadn't run yet are dropped and
      // its weapon would still be deploying (the shot refused). Slow state() calls
      // in set() used to hide this wait.
      const nd = g.netDebug.state;
      if (!(await until(() => nd.acked >= nd.sent, 3000))) throw new Error(`shootBot: server didn't ack the deploy commands (${nd.acked}/${nd.sent})`);
      me.pos = [...pos]; me.vel = [0, 0, 0];
      const eye = [pos[0], pos[1] + me.eyeH, pos[2]];
      const d = target.map((v, i) => v - eye[i]);
      api.set({ pos, yaw: Math.atan2(-d[0], -d[2]), pitch: Math.atan2(d[1], Math.hypot(d[0], d[2])) });
      await gameSleep(150); // the server checks fire rate on its own clock
      g.testCtl.lastHit = null;
      const ammo0 = g.hud.get().ammo;
      g.arsenal.noSpread = true;
      setAction("+attack", true); api.step(3, 0.01); setAction("+attack", false); api.step(1, 0.01);
      g.arsenal.noSpread = false;
      await until(() => g.testCtl.lastHit, 1000);
      const hit = g.testCtl.lastHit;
      if (!hit || hit.part !== region) throw new Error(`shot at ${region} from angle ${angle.toFixed(2)} height ${height}: server says ${hit ? hit.part : "miss"} (ammo ${ammo0}->${g.hud.get().ammo}, me ${me.pos.map(round)} yaw ${round(me.yaw)} pitch ${round(me.pitch)}, target ${target.map(round)}, bot ${bot.p.map(round)} yaw ${round(bot.yaw)})`);
      return { part: hit.part, dmg: hit.dmg };
    },
    // Real-time pacing check: the live loop runs for `ms` with the client's sim
    // clock scaled (1.02 = 2% fast) and the server applying its real command
    // budget, tapping fire at a frozen bot's head every `tapMs`. Fails if the
    // backlog grows past maxBacklog ticks, a tap gets no server result, or a
    // tap misses (aim held still, weapon_spread 0).
    // Jitter probe (realtime scenarios): the live loop for `ms` while strafing
    // left/right every `strafeMs` (and running forward), sampling every frame:
    // own-view smoothness (drawn eye step vs velocity*dt), prediction
    // corrections, view angles moved by anything but input, and the remote
    // timeline (under-runs: drawn past the newest snapshot; time jumps).
    // `max` ({field: limit}) fails the step when a measured field exceeds it.
    // draw: false computes the view every frame but skips the GPU work, so a slow
    // (software-rendered) runner still runs the loop at a realistic frame rate.
    async jitterProbe({ ms = 6000, strafeMs = 700, forward = true, paced, max = {}, draw = true } = {}) {
      g.testCtl.skipGfx = !draw;
      if (paced !== undefined) g.net.send({ t: "paced", c: paced });
      const sleep = (t) => new Promise((r) => setTimeout(r, t));
      g.testCtl.live = true;
      const st = g.predictor.stats, c0 = st.corrections, t0p = performance.now();
      const R = { frames: 0, corrections: 0, maxCorrection: 0, maxErr: 0, jerkMax: 0, jerkOver1cm: 0, jerkOver5cm: 0, angleMoves: 0, underruns: 0, underrunMaxTicks: 0, tickJumps: 0, tickJumpMax: 0, renderBack: 0 };
      let prev = null, prevT = null, prevBot = null, lastC = c0, run = true, held = [];
      const press = (a) => { for (const h of held) setAction(h, false); held = a; for (const h of held) setAction(h, true); };
      const sampleFrame = () => {
        if (!run) return;
        const v = g.testCtl.view, tm = v ? g.remotes.timing(v.t) : g.remotes.timing(performance.now());
        const bot = v && g.remotes.sample(v.t).find((r) => /\(bot/.test(r.name));
        if (v && prev && v.t !== prev.t) {
          R.frames++;
          const dt = (v.t - prev.t) / 1000;
          const step = v.eye.map((x, i) => x - prev.eye[i]);
          const want = [prev.vel[0] * dt, 0, prev.vel[2] * dt];
          const jerk = Math.hypot(step[0] - want[0], step[2] - want[2]);
          R.jerkMax = Math.max(R.jerkMax, jerk);
          if (jerk > 0.01) R.jerkOver1cm++;
          if (jerk > 0.05) R.jerkOver5cm++;
          if (v.yaw !== prev.yaw || v.pitch !== prev.pitch) R.angleMoves++;
          const e = Math.hypot(...g.predictor.error);
          R.maxErr = Math.max(R.maxErr, e);
          if (st.corrections !== lastC) { R.corrections += st.corrections - lastC; lastC = st.corrections; R.maxCorrection = Math.max(R.maxCorrection, st.lastError); (R.log ||= []).length < 12 && R.log.push({ ms: Math.round(performance.now() - t0p), ph: g.rounds?.state?.ph, frameMs: Math.round(v.t - prev.t), drop: g.testCtl.lastMe?.drop, ack: g.testCtl.lastMe?.ack, err: +st.lastError.toFixed(3), srvVsUs: +st.ackDelta.toFixed(3), pending: st.pending, q: g.testCtl.lastMe?.q, sent: g.netDebug.state.sent }); }
          if (tm.newest > -Infinity && tm.renderTick > tm.newest) { R.underruns++; R.underrunMaxTicks = Math.max(R.underrunMaxTicks, tm.renderTick - tm.newest); }
          if (bot && prevBot && bot.v > 0.5) { // a moving remote: per-frame step vs its own speed
            const stp = Math.hypot(bot.p[0] - prevBot.p[0], bot.p[2] - prevBot.p[2]), exp = bot.v * dt;
            R.remoteFrames = (R.remoteFrames || 0) + 1;
            if (stp < 0.3 * exp) R.remoteStalls = (R.remoteStalls || 0) + 1;
            if (stp > 1.7 * exp) R.remoteJumps = (R.remoteJumps || 0) + 1;
          }
          if (prevT) {
            const adv = tm.renderTick - prevT.renderTick, exp = (v.t - prev.t) / 10;
            const j = Math.abs(adv - exp);
            // a jump: off real time by more than the slew allows (10% of the frame) plus rounding
            if (j > 0.15 * exp + 0.1) { R.tickJumps++; R.tickJumpMax = Math.max(R.tickJumpMax, j); }
            if (adv < 0) R.renderBack++;
          }
          prevT = tm;
        }
        if (v) prev = { ...v, eye: [...v.eye], vel: [...v.vel] };
        prevBot = bot || null;
        requestAnimationFrame(sampleFrame);
      };
      requestAnimationFrame(sampleFrame);
      try {
        const t0 = performance.now();
        let n = 0;
        while (performance.now() - t0 < ms) {
          press([...(forward ? ["+forward"] : []), n++ % 2 ? "+moveleft" : "+moveright"]);
          await sleep(strafeMs);
        }
      } finally {
        press([]); run = false; g.testCtl.live = false; g.testCtl.skipGfx = false;
      }
      for (const k of ["maxCorrection", "maxErr", "jerkMax", "underrunMaxTicks", "tickJumpMax"]) R[k] = +R[k].toFixed(4);
      const over = Object.entries(max).filter(([k, lim]) => (R[k] ?? 0) > lim);
      if (over.length) throw new Error(`jitterProbe: ${over.map(([k, lim]) => `${k} ${R[k]} > ${lim}`).join(", ")}: ${JSON.stringify(R)}`);
      return R;
    },
    async pacedTaps({ scale = 1, ms = 8000, tapMs = 400, maxBacklog = 60, maxQueue = 4, walk = true } = {}) {
      const sleep = (t) => new Promise((r) => setTimeout(r, t));
      const me = g.me, nd = g.netDebug.state;
      g.net.send({ t: "paced", c: true });
      g.testCtl.clockScale = scale;
      g.testCtl.live = true;
      api.weapon("sidearm"); // semi-automatic: one shot per tap, whatever the frame timing
      await sleep(600);
      const taps = [];
      let worst = 0, worstQueue = 0, hits = 0, disagree = 0, lostPress = 0, results = [];
      try {
        const t0 = performance.now();
        let n = 0;
        while (performance.now() - t0 < ms) {
          // walk a bit between taps (as the operator did), then stand still and aim
          if (walk) { setAction(n % 2 ? "+moveleft" : "+moveright", true); await sleep(tapMs / 2); setAction(n % 2 ? "+moveleft" : "+moveright", false); }
          await sleep(walk ? tapMs / 4 : tapMs / 2);
          if ((g.hud.get().ammo ?? 99) < 3) { api.reload(); await sleep(3000); }
          for (let i = 0; i < 50 && Math.hypot(me.vel[0], me.vel[2]) > 0.01; i++) await sleep(10); // stopped
          const bot = g.remotes.sample(performance.now()).find((r) => /\(bot/.test(r.name));
          const part = g.hitParts({ p: bot.p, yaw: bot.yaw, pitch: bot.pitch, crouch: !!bot.c, team: bot.team, id: bot.id }).find((h) => h.region === "head");
          const m = part.m, c = part.c;
          const target = [0, 1, 2].map((i) => m[i] * c[0] + m[4 + i] * c[1] + m[8 + i] * c[2] + m[12 + i]);
          const eye = [me.pos[0], me.pos[1] + me.eyeH + me.eyeOff, me.pos[2]];
          const d = target.map((v, i) => v - eye[i]);
        // A wall between us is the scenario's geometry, not lag compensation:
        // say so instead of counting a miss (lag-compensation once patrolled
        // along de_slop's mid building and shot into its bricks).
        if (segmentBlocked(g.map?.boxes || [], eye, target)) throw new Error(`shootMovingBot: shot ${n + 1} is blocked by the map (eye ${eye.map(round)} -> ${target.map(round)}): move the patrol to open ground`);
          me.yaw = Math.atan2(-d[0], -d[2]); me.pitch = Math.atan2(d[1], Math.hypot(d[0], d[2]));
          const aimed = { eye: eye.map(round), target: target.map(round) };
          const seq0 = nd.sent;
          g.testCtl.lastImpacts = null; g.testCtl.lastShotRay = null;
          setAction("+attack", true);
          for (let i = 0; i < 40 && nd.sent < seq0 + 2; i++) await sleep(5); // pressed for 2 sim ticks at least
          setAction("+attack", false);
          taps.push(seq0);
          worst = Math.max(worst, nd.sent - nd.acked);
          worstQueue = Math.max(worstQueue, nd.queue);
          await sleep(walk ? tapMs / 4 : tapMs / 2);
          for (let i = 0; i < 100 && !(nd.lastShot && nd.lastShot.seq > seq0); i++) await sleep(10);
          let r = nd.lastShot && nd.lastShot.seq >= seq0 ? nd.lastShot.result : "no result";
          const cl = g.testCtl.lastShotRay;
          const agree = r !== "no result" && typeof r === "string" && (cl?.hit ? String(r).includes(` ${cl.hit.region} `) : r === "miss");
          if (r === "no result") r = { r, clientFired: !!cl && cl.seq > seq0, clientSeq: cl?.seq, seq0, ars: (({ cur, tickN, nextShot, reloadEnd, cycleEnd, clicked, pendingReload, pendingSwitch }) => ({ cur, tickN, nextShot, reloadEnd, cycleEnd, clicked, pendingReload, pendingSwitch, ammo: g.arsenal.slots[cur]?.ammo, reserve: g.arsenal.slots[cur]?.reserve }))(g.arsenal), play: g.hud.get().screen, sent: nd.sent, acked: nd.acked, last: nd.lastShot };
          if (r?.r === "no result" && !r.clientFired) lostPress++; // the press never made a command: neither side fired
          else if (!agree) disagree++;
          if (/head/.test(r)) hits++;
          else {
            const cl = g.testCtl.lastShotRay, sv = g.testCtl.lastImpacts;
            if (cl && sv) r = { r, client: cl.hit ? `${cl.hit.region}/${cl.hit.name}` : "miss", aimed, shot: { o: cl.o.map(round), d: cl.d.map((v) => +v.toFixed(4)) }, seq: [cl.seq, sv.seq], originErr: round(Math.hypot(...cl.o.map((v, i) => v - sv.o[i]))), dirErr: Math.acos(Math.min(1, cl.d.reduce((s, v, i) => s + v * sv.d[i], 0))).toFixed(4), me: me.pos.map(round), vel: me.vel.map(round) };
          }
          results.push(r);
          n++;
        }
        await sleep(600);
      } finally {
        g.testCtl.live = false; g.testCtl.clockScale = 1;
        g.net.send({ t: "paced", c: false });
      }
      const out = { scale, taps: taps.length, hits, disagree, lostPress, worstBacklog: worst, worstQueue, dropped: nd.dropped, results: results.filter((r) => !/head/.test(r?.r ?? r)) };
      if (worst > maxBacklog || worstQueue > maxQueue || nd.dropped) throw new Error(`pacedTaps: backlog ${worst} ticks, server queue ${worstQueue}: ${JSON.stringify(out)}`);
      // Every tap answered, and the client's predicted result is the server's.
      if (disagree || lostPress > 1) throw new Error(`pacedTaps: ${disagree} taps where client and server disagree: ${JSON.stringify(out)}`);
      if (hits < (taps.length - lostPress) * 0.8) throw new Error(`pacedTaps: only ${hits}/${taps.length} taps hit the head: ${JSON.stringify(out)}`);
      return out;
    },
    // Aim check (needs rcon sv_showimpacts 1 and weapon_spread 0): shoot the
    // frozen bot's head from every side, level, from a crouch (aiming up) and
    // from above (aiming down). Every shot must hit the head, and the ray the
    // server traced must be the client's own (origin < 1 cm, direction < 0.001 rad).
    async aimCheck({ dist = 8, timeoutMs = 20000 } = {}) {
      // Waits on state, never on a fixed wall time: a loaded runner (or fake lag) only
      // makes it slower. timeoutMs bounds each wait.
      const waitFor = async (cond, what) => {
        if (!(await until(cond, timeoutMs))) throw new Error(`aimCheck: timed out after ${timeoutMs} ms waiting for ${what}`);
      };
      // Every command sent so far acknowledged (no frames run between test steps, so none are added meanwhile).
      const serverCaughtUp = () => waitFor(() => g.predictor.pendingCmds().length === 0, "the server to acknowledge our commands");
      const rows = [];
      const setups = [{ height: 0, crouch: false }, { height: 0, crouch: true }, { height: 2.5, crouch: false }];
      for (const su of setups) for (const k of [0, 1, 2, 3]) {
        const angle = k * Math.PI / 2;
        const bot = g.remotes.sample(performance.now()).find((r) => /\(bot/.test(r.name));
        const st = { p: bot.p, yaw: bot.yaw, pitch: bot.pitch, crouch: !!bot.c, team: bot.team, id: bot.id };
        const part = g.hitParts(st).find((h) => h.region === "head");
        const m = part.m, c = part.c;
        const target = [0, 1, 2].map((i) => m[i] * c[0] + m[4 + i] * c[1] + m[8 + i] * c[2] + m[12 + i]);
        const a = bot.yaw + angle;
        const pos = [bot.p[0] - Math.sin(a) * dist, bot.p[1] + su.height, bot.p[2] - Math.cos(a) * dist];
        const me = g.me;
        api.set({ pos: [pos[0], bot.p[1], pos[2]], crouch: su.crouch });
        if (su.crouch) setAction("+duck", true);
        api.step(60, 0.01); // on the floor: settle the eye height, weapon ready
        // From above: nothing stands there, so lift the shooter just before the shot (it has
        // fallen a few mm by then). Settled at that height first, it would have landed (0.5 s).
        // A teleport drops the server's queued commands (and the weapon timers they run), so
        // the settling must have run there first.
        if (su.height) { await serverCaughtUp(); api.set({ pos }); api.step(2, 0.01); }
        await serverCaughtUp(); // aim from where the server agrees we are
        const eye = [me.pos[0], me.pos[1] + me.eyeH + me.eyeOff, me.pos[2]];
        const d = target.map((v, i) => v - eye[i]);
        me.yaw = Math.atan2(-d[0], -d[2]); me.pitch = Math.atan2(d[1], Math.hypot(d[0], d[2]));
        g.testCtl.lastHit = null; g.testCtl.lastImpacts = null; g.testCtl.lastShotRay = null;
        setAction("+attack", true); api.step(1, 0.01); setAction("+attack", false); api.step(1, 0.01);
        // The server's account of this shot (by its seq, not whatever arrived last): the
        // impacts message goes out after the shot's hit, so once it is here the hit is too.
        const shotSeq = g.testCtl.lastShotRay?.seq;
        await waitFor(() => g.testCtl.lastImpacts && g.testCtl.lastImpacts.seq === shotSeq, `the server's impacts for shot ${shotSeq}`);
        if (su.crouch) setAction("+duck", false);
        const cl = g.testCtl.lastShotRay, sv = g.testCtl.lastImpacts, hit = g.testCtl.lastHit;
        const row = { angle: round(angle), height: su.height, crouch: su.crouch, pitch: round(me.pitch), hit: hit?.part ?? "miss", client: cl?.hit ? `${cl.hit.region}/${cl.hit.name}` : "miss" };
        if (cl && sv) {
          row.originErr = round(Math.hypot(...cl.o.map((v, i) => v - sv.o[i])));
          row.dirErr = Math.acos(Math.min(1, cl.d.reduce((s, v, i) => s + v * sv.d[i], 0))).toFixed(5);
          row.seq = [cl.seq, sv.seq];
        }
        rows.push(row);
      }
      // The client's predicted part must be the server's; and from the front and back nothing covers the head.
      // From above means aiming down at the head: a level pitch there is a shooter that fell.
      const bad = rows.filter((r) => r.client.split("/")[0] !== r.hit || (r.height > 0 && !(r.pitch < -0.2)) || (r.angle % 3.14 < 0.1 && r.hit !== "head") || !(r.originErr < 0.01) || !(+r.dirErr < 1e-3) || r.seq?.[0] !== r.seq?.[1]);
      if (bad.length) throw new Error(`aimCheck: ${bad.length}/${rows.length} bad: ${JSON.stringify(bad)}`);
      return rows;
    },
    // Lag compensation check: stand beside a wandering bot and shoot at `region`
    // where it is drawn right now (interpolated, behind the server), one shot at
    // a time. Fails unless the server's hit count is within [minHits, maxHits].
    async shootMovingBot({ region = "body", dist = 7, shots = 6, minHits = 0, maxHits = Infinity } = {}) {
      const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
      const findBot = () => g.remotes.sample(performance.now()).find((r) => /\(bot/.test(r.name));
      let bot = findBot();
      if (!bot) throw new Error("no bot");
      const me = g.me;
      let hits = 0, parts = [], moved = 0, attempts = 0, stale = 0;
      // A shot fired from a view more than STALE ticks behind the server tests
      // a stalled runner (the page's event loop blocked for seconds under CI
      // load), not lag compensation: beyond sv_maxunlag the server can't rewind
      // that far (as in CS). Such shots don't count and are taken again.
      const STALE = 60; // 0.6 s; a 150 ms ping + interpolation is ~25 ticks
      for (let n = 0; n < shots; n++) {
        if (++attempts > shots * 3) throw new Error(`shootMovingBot: ${stale} of ${attempts - 1} shots fired from a view over ${STALE} ticks stale: the runner is too loaded to test lag compensation`);
        if (n % 3 === 0) { // (re)position beside it: it crosses our view
          bot = findBot();
          const a = bot.yaw + Math.PI / 2;
          api.set({ pos: [bot.p[0] - Math.sin(a) * dist, 0, bot.p[2] - Math.cos(a) * dist], crouch: false });
          await sleep(500); // the teleport reaches the server (fake lag included)
          api.step(30, 0.01);
        }
        await sleep(160);
        api.step(20, 0.01); // fire rate: weapon timers run on the command clock (ours and the server's)
        // Stepping renders frames synchronously (software GL: ~30 ms each), so
        // no snapshot got in meanwhile; let the stream catch up, as it does
        // for a real client between frames, before aiming at what's on screen.
        await sleep(150);
        const now = performance.now();
        bot = findBot();
        const st = { p: bot.p, yaw: bot.yaw, pitch: bot.pitch, crouch: !!bot.c, team: bot.team, id: bot.id };
        const part = g.hitParts(st).find((h) => h.region === region);
        const m = part.m, c = part.c;
        const target = [0, 1, 2].map((i) => m[i] * c[0] + m[4 + i] * c[1] + m[8 + i] * c[2] + m[12 + i]);
        const eye = [me.pos[0], me.pos[1] + me.eyeH + me.eyeOff, me.pos[2]];
        const d = target.map((v, i) => v - eye[i]);
        me.yaw = Math.atan2(-d[0], -d[2]); me.pitch = Math.atan2(d[1], Math.hypot(d[0], d[2]));
        g.testCtl.lastHit = null; g.testCtl.lastImpacts = null;
        const vtSent = g.remotes.viewTick(now);
        const newestTick = g.remotes.viewTick(now + 1e6); // the newest snapshot we have
        g.arsenal.noSpread = true;
        setAction("+attack", true); g.frame(now, 0.01, false); setAction("+attack", false); g.frame(performance.now(), 0.01, true);
        g.arsenal.noSpread = false;
        const at = bot.p;
        for (let i = 0; i < 40 && !g.testCtl.lastHit; i++) await sleep(25);
        for (let i = 0; i < 20 && !g.testCtl.lastImpacts; i++) await sleep(25); // the server's account (sv_showimpacts)
        const imp = g.testCtl.lastImpacts;
        if (imp && imp.tick - imp.vt > STALE) {
          stale++; parts.push({ stale: Math.round(imp.tick - imp.vt), hit: !!g.testCtl.lastHit });
          n--; continue; // fired from a stalled view: take it again
        }
        const latest = findBot();
        moved = Math.max(moved, Math.hypot(latest.p[0] - at[0], latest.p[2] - at[2]));
        if (g.testCtl.lastHit) { hits++; parts.push(g.testCtl.lastHit.part); }
        else {
          const im = g.testCtl.lastImpacts, sp = im?.poses.find((p) => p.id === bot.id);
          parts.push({ miss: true, seen: bot.p.map(round), server: sp?.p.map(round), vt: round(vtSent), newest: newestTick, cmdVt: im?.vt, seq: im?.seq, sentSeq: g.netDebug.state.sent, at: im?.at, tick: im?.tick, me: me.pos.map(round), yaw: round(me.yaw) });
        }
      }
      const out = { hits, shots, stale, parts, ping: g.net.ping, botMovedWhileWaiting: round(moved) };
      if (hits < minHits || hits > maxHits) throw new Error(`shootMovingBot: ${hits}/${shots} hits, want ${minHits}..${maxHits}: ${JSON.stringify(out)}`);
      return out;
    },
    // Show/hide the HUD and name tags (clean map shots).
    hud(show = true) {
      for (const el of [document.getElementById("hud"), document.querySelector(".tags")]) if (el) el.style.display = show ? "" : "none";
      return true;
    },
    cvar(k, v) { g.settings.set({ [k]: v }); return g.settings.get()[k]; },
    // Set a form field in the open UI as a user would (value + change event).
    async uiField(selector, value) {
      const el = document.querySelector(selector);
      if (!el) throw new Error(`no field ${selector}`);
      el.scrollIntoView?.({ block: "center" });
      if (el.type === "checkbox") el.checked = !!+value;
      else el.value = String(value);
      if (el.type === "range") el.dispatchEvent(new Event("input", { bubbles: true }));
      el.dispatchEvent(new Event("change", { bubbles: true }));
      await nextFrame();
      return true;
    },
    // Pin the shader clock (clouds, slop wobble) so captures are stable.
    time(t) { g.testCtl.pinnedTime = t; return true; },
    // Free camera: {eye, yaw, pitch}, "overview" (top-down), {orbit: angle}, or null for first person.
    camera(spec) {
      if (!spec) g.testCtl.camera = null;
      else if (spec === "overview") g.testCtl.camera = g.map?.name === "de_slop" ? { eye: [0, 75, 0.01], yaw: 0, pitch: -1.5 } : overview(g.map);
      else if (spec.orbit !== undefined) {
        const a = spec.orbit, r = spec.radius ?? 42, h = spec.height ?? 26;
        g.testCtl.camera = { eye: [Math.sin(a) * r, h, Math.cos(a) * r], yaw: a, pitch: -Math.atan2(h, r) * 0.9 };
      } else g.testCtl.camera = { eye: spec.eye, yaw: spec.yaw ?? 0, pitch: spec.pitch ?? 0 };
      return true;
    },
    state() {
      const me = g.me, h = g.hud.get(), rs = g.remotes.sample(performance.now());
      return {
        tags: g.testCtl.tags ?? [], // name tags drawn last frame (tags.js)
        debris: g.debris?.stats() ?? null, // bullet holes, casings (lying + flying), bolt casings waiting (debris.js)
        marks: g.gfx.stats().marks ?? null, // what the renderer drew of them last frame: {decals, bits, draws}
        corpses: g.testCtl.corpses ?? 0, // bodies drawn last frame (corpse.js): lying, falling or shrinking away
        spray: { idx: g.arsenal.recoil, shot: g.arsenal.shotIdx }, // the recoil spray: index as of the last shot, the index that shot used
        id: g.myId, team: h.myTeam, observe: h.observe ?? null, round: g.rounds?.state ?? null, roundEnd: h.roundEnd ?? null,
        items: g.items?.() ?? [],
        me: me && { pos: me.pos.map(round), vel: me.vel.map(round), yaw: round(me.yaw), pitch: round(me.pitch), crouched: me.crouched, onGround: me.onGround, jumps: me.jumps || 0 },
        movers: g.moverClock?.(), // the movers' clock: t (drawn at, s), view (the remotes' server time, s), tb (our tick base)
        net: g.netDebug && (({ sent, acked, queue, dropped, serverTick, lastShot }) => ({ sent, acked, backlog: sent - acked, queue, dropped, serverTick, lastShot }))(g.netDebug.state),
        audio: g.audioState?.(),
        gfx: (({ aa, renderScale, target, lights, lightsActive, lightShadows, lightShadowBudget, lightShadowMapRenders, lightShadowSolidRenders, lightShadowTiles, lightShadowBaked, lightShadowQueued, lightShadowKB, lightShadowBakeMs, dynLights, dynLightsAdded, dynLightsDropped, dynLightAddMs, dynLightAddPeakMs, dynLightFloods, dynShadows, dynShadowBudget, dynShadowRenders, dynLightFrameMsMax, dynLightMaskHits, dynLightMaskSkips, dynLightUploads, beltSteps, mapLoadMs, mapLoading }) => ({
          aa, renderScale, target, antialias: g.gfx.info().antialias, msaa: g.gfx.info().msaaSamples, // the AA path taken (engine/aa.js)
          lights, lightsActive, lightShadows, lightShadowBudget, lightShadowMapRenders, lightShadowSolidRenders, // map lights (engine/lights.js)
          lightShadowTiles, lightShadowBaked, lightShadowQueued, lightShadowKB, lightShadowBakeMs, // lamp shadows: static tiles, lights baked / queued, memory, bake time
          dynLights, dynLightsAdded, dynLightsDropped, dynLightAddMs, dynLightAddPeakMs, dynLightFloods, // dynamic lights (muzzle flashes)
          dynShadows, dynShadowBudget, dynShadowRenders, // their cube shadows: in use, budget, renders
          beltSteps, // mover boxes (escalator treads) drawn (engine/movers.js)
          mapLoadMs, mapLoading, // where the last map load's time went; still compiling (engine/gl.js)
          dynLightFrameMsMax, dynLightMaskHits, dynLightMaskSkips, dynLightUploads, // the bounded per-frame cost: worst frame, mask cache, uploads: lit now, added, dropped, add cost (flood + upload)
        }))(g.gfx.stats()),
        bench: g.benchLog ?? null, // results of the bench calls so far
        music: g.music?.state() ?? null, // { on, want, loop, playing, last, stingers, tracks } (assets/music)
        reloadText: !!document.querySelector(".bottom-right.reloading"), // the "slopping…" indicator
        thumbs: (({ mapPrograms, lightGridBuilds, times, where } = g.thumbStats?.() || {}) => ({ mapPrograms, lightGridBuilds, times, where }))(), // map previews: shaders compiled, light grids built
        prediction: g.predictor && { ...g.predictor.stats, lastError: round(g.predictor.stats.lastError * 1e6) / 1e6 },
        radar: g.radar && { spot: [...g.spot], shown: g.radar.seen, small: document.querySelector(".radar")?.style.display === "block", big: document.querySelector(".radar-big")?.style.display === "block" },
        nades: g.nades && { live: g.nades.list.map((n) => ({ w: n.w, st: n.st, p: n.p })), blasts: g.nades.blastCount, events: g.nades.events, log: g.nades.log, white: +g.nades.whiteout(performance.now()).toFixed(2) },
        settings: g.settings.get(), // client cvars ("settings.fps_max_unlocked")
        map: h.map, pendingMap: h.pendingMap ?? null, vote: h.vote && { options: h.vote.options, votes: h.vote.votes, mine: h.vote.mine, done: !!h.vote.done, winner: h.vote.winner },
        scoped: g.arsenal?.scoped, screen: h.screen, console: !!h.console, scoreboard: !!h.scoreboard,
        hud: { loadingMap: h.loadingMap ?? null, loadingText: document.querySelector(".map-loading")?.textContent ?? "", gpu: h.gpu ?? null, gpuText: document.querySelector(".gpu-count")?.textContent ?? "", hint: h.hint || "", work: h.work ?? null, workText: document.querySelector(".work-label")?.textContent ?? "",
          roundEndText: document.querySelector(".round-end .re-line")?.textContent ?? "", hp: h.hp, ammo: h.ammo, reserve: h.reserve, weapon: h.weapon, money: h.money, armor: h.armor, mvp: h.mvp ?? null, dmgReport: h.dmgReport ?? null, shades: !!h.shades, boost: h.boost || 0, ping: h.ping, connected: h.connected, buyMenu: !!h.buyMenu, canBuy: !!h.canBuy, buyMsg: h.buyMsg || "", helmet: !!h.helmet, kit: !!h.kit, buyItems: [...document.querySelectorAll(".buy-item")].map((b) => `${b.querySelector("b")?.textContent || ""} ${b.querySelector(".bi-name")?.textContent} ${b.querySelector(".bi-price")?.textContent}${b.disabled ? " x" : ""}`).join(" | "), dmgArcs: (h.dmgArcs || []).length, killRows: document.querySelectorAll('.kill-row').length, screenShades: [...document.querySelectorAll('#hud *')].filter((e) => { const r = e.getBoundingClientRect(), cs = getComputedStyle(e); return r.width >= innerWidth * 0.9 && r.height >= innerHeight * 0.9 && cs.display !== 'none' && +cs.opacity > 0 && (cs.backgroundImage !== 'none' || !/rgba\(.*, 0\)|transparent/.test(cs.backgroundColor)); }).map((e) => e.className).join(' '), sbDead: document.querySelectorAll('.scoreboard tr.dead').length, sbAlive: [...document.querySelectorAll('.scoreboard .sb-alive')].map((e) => e.textContent).join(" | "), tagsUnderHud: (() => { const t = document.querySelector('.tags'), hd = document.getElementById('hud'); return !!t && !!hd && +getComputedStyle(t).zIndex < +getComputedStyle(hd).zIndex; })(), mapCats: [...document.querySelectorAll('.maps-panel .map-cat')].map((h) => `${h.dataset.cat}: ${[...h.nextElementSibling.querySelectorAll('.map-card')].map((c) => c.dataset.map).join(" ")}`).join(" | "), optionSections: [...document.querySelectorAll('.panel.options h4')].map((e) => e.firstChild?.textContent?.trim()).join(" | "), chatLines: (h.chat || []).map((l) => `${l.radio ? "(RADIO) " : l.tm ? "(TEAM) " : ""}${l.name}: ${l.text}`).join("\n"), feed: (h.feed || []).map((f) => f.text + (f.kill ? ' ' + f.kill.kteam + '>' + f.kill.vteam : '')), killIcons: [...document.querySelectorAll('.kill-row img.kw')].filter((i) => i.naturalWidth > 0).length, dmgArcEls: [...document.querySelectorAll('.dmg-arc')].map((e) => { const r = e.getBoundingClientRect(); return [Math.round(r.x), Math.round(r.y), Math.round(r.width), e.style.transform, e.style.opacity]; }) },
        players: rs.map((r) => ({ id: r.id, name: r.name, team: r.team, pos: r.p.map(round), crouched: !!r.c })),
        byName: Object.fromEntries(rs.map((r) => [r.name, { team: r.team, pos: r.p.map(round), crouched: !!r.c }])), // "byName.bob.team"
        roundWin: h.roundEnd?.win ?? null, // the round-end banner's winning team (gone when the next round starts)
        score: h.score ?? null, // rounds won per team: {vibe, artisan}
        feed: h.feed.map((f) => f.text),
        teamMenu: !!h.teamMenu,
        caption: { shout: h.shout?.text ?? null, subtitle: h.subtitle?.text ?? null }, // announcer captions on screen
        lastHit: g.testCtl.lastHit ? { part: g.testCtl.lastHit.part, dmg: g.testCtl.lastHit.dmg } : null, // last server-confirmed hit
        consoleLog: consoleLog(),
        latency: latencyState(),
      };
    },
  };
  // Press or release the key bound to an action, keeping the other held keys.
  function setAction(cmd, on) {
    const code = g.keyFor(cmd);
    if (!code) throw new Error(`${cmd} is unbound`);
    g.input.setHeld([...g.input.held().filter((c) => c !== code), ...(on ? [code] : [])]);
  }
  // The console log: line count, help-table rows, section headers, and the last line.
  function consoleLog() {
    const log = document.querySelector(".console-log");
    if (!log) return null;
    return { lines: log.childElementCount, rows: log.querySelectorAll(".help-row").length,
      heads: [...log.querySelectorAll(".help-head")].map((d) => d.textContent), last: log.lastElementChild?.textContent ?? "" };
  }
  // Overlay numbers for the JSON dump (GPU-dependent: recorded, not asserted).
  function latencyState() {
    const l = g.loop.stats(), r = g.gfx.stats();
    const s = g.settings.get();
    return {
      flags: { cl_loop: s.cl_loop, fps_limiter: s.fps_limiter, r_gpu_fence: s.r_gpu_fence, r_late_input: s.r_late_input, r_finish: s.r_finish,
        r_desynchronized: s.r_desynchronized, r_power_preference: s.r_power_preference },
      granted: g.gfx.info(),
      ticksPerSec: round(l.fps), renderedFps: l.renderedFps, presentedFps: round(l.presented.fps), workMs: round(l.workMs),
      inFlight: r.inFlight, skipped: r.skipped, gpuFrameMs: r.gpuFrameMs == null ? null : round(r.gpuFrameMs),
      latchToGpuMs: r.latchToGpuMs == null ? null : round(r.latchToGpuMs), lines: g.latencyReadout(),
    };
  }
  // A real input event for a key or mouse-button code (KeyR, Mouse0, ...).
  const caseHeld = new Set();
  let flashBase = 0; // gfx dynLightsAdded when the case began (caseState muzzleLights)
  function ev(code, down) {
    const m = /^Mouse(\d+)$/.exec(code);
    if (m) dispatchEvent(new MouseEvent(down ? "mousedown" : "mouseup", { button: +m[1] }));
    else dispatchEvent(new KeyboardEvent(down ? "keydown" : "keyup", { code, cancelable: true }));
  }
  window.__cs = api;
}

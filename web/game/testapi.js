// window.__cs: the test harness's handle on a client (cmd/sim drives it over CDP).
// Installed only when the server runs with -test AND the page has ?test=1, so
// it never exists in prod. Every call returns plain JSON; expect*/rateCheck
// throw on failure, which fails the scenario.
import { mulberry32 } from "../gen/rng.js";
import { overview } from "./observer.js";
import { BTN } from "../engine/cmd.js";
import { segmentBlocked } from "./tags.js";
import { CVARS } from "../ui/settings.js";
import { INTERP_DELAY } from "../engine/net.js";

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
const wallSleep = (ms) => new Promise((r) => setTimeout(r, ms)); // wall: realtime runs and the harness's own polling only
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
  while (!cond() && performance.now() < end) await wallSleep(10); // wall: realtime runs only (lockstep: game ticks above)
  return cond();
}

// ms of game time (lockstep: that many server ticks), else of wall time.
async function gameSleep(ms) {
  if (!gate.lockstep) return wallSleep(ms); // wall: realtime runs only
  for (let t = 0; t < ms; t += TICK_MS) await gameTick();
}

export function installTestApi(g) {
  gate.net = g.net;
  // Let Preact render. A timer, not rAF: background headless tabs throttle rAF.
  const nextFrame = () => new Promise((r) => setTimeout(r, 30)).then(() => globalThis.__vclock?.settle()); // wall: the browser's own work (Preact renders), no game time
  const round = (v) => Math.round(v * 1000) / 1000;

  // Lockstep pages (cmd/sim) draw nothing unless asked (testCtl.noGL): withGL lifts that
  // for one call that renders on purpose (a snapshot, a bench, lamp-shadow bakes).
  const withGL = (fn) => {
    const was = g.testCtl.noGL;
    g.testCtl.noGL = false;
    let r;
    try { r = fn(); } catch (e) { g.testCtl.noGL = was; throw e; }
    if (r && typeof r.then === "function") return r.finally(() => { g.testCtl.noGL = was; });
    g.testCtl.noGL = was;
    return r;
  };

  const api = {
    // Draw one frame on the GPU, frozen in time (dt 0), once the map is drawn. Lockstep
    // runs draw only here (cmd/sim snapshot and "draw" steps); stepping never draws.
    // bake: first render frozen frames until the lamp shadows are baked (settleLights;
    // on a big map that is many frames, so pictures that need them call settleLights).
    async draw({ bake = false } = {}) {
      await api.mapReady();
      return withGL(() => {
        for (let i = 0; bake && i < 300 && g.gfx.stats().lightShadowQueued > 0; i++) { g.gfx.bakeLightShadows(); g.frame(performance.now(), 0, true); }
        return api.step(1, 0);
      });
    },
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
      // Each frame bakes the whole queue (bakeLightShadows), not BAKE_TILES: one frame
      // instead of up to 300 (gs_megabyte: ~19 s of SwiftShader frames, past the hang guard in CI).
      for (; i < maxFrames && g.gfx.stats().lightShadowQueued > 0; i++) { g.gfx.bakeLightShadows(); g.frame(performance.now(), 0, true); }
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
      const t0 = wallNow(); // wall: hang guard (shaders and the map build in real time)
      while (g.hud.get().loadingMap) {
        if (wallNow() - t0 > ms) throw new Error(`map ${g.hud.get().loadingMap} still loading after ${ms} ms`); // wall: hang guard
        g.frame(performance.now(), 0, true);
        await new Promise((r) => setTimeout(r, 5)); // wall: polls browser work (no game time)
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
      for (let i = 0; i < 100 && !row.querySelector(".rebind.waiting"); i++) { await new Promise((r) => setTimeout(r, 10)); await globalThis.__vclock?.settle(); } // wall: polls the browser's own rendering
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
    // Fail unless this page is client build `build` and every module,
    // stylesheet and JSON asset it loaded came from that build's /b/<build>/
    // URLs (so no cache can have mixed in another build's code).
    // No build: whatever this page says it is.
    expectBuild(build) {
      const page = document.querySelector('meta[name="cs-build"]')?.content;
      build ??= page;
      const pre = `/b/${build}/`;
      const assets = performance.getEntriesByType("resource")
        .map((e) => new URL(e.name))
        .filter((u) => u.origin === location.origin && /\.(js|mjs|css|json)$/.test(u.pathname) && !["/map.json", "/maps.json"].includes(u.pathname));
      const bad = assets.filter((u) => !u.pathname.startsWith(pre)).map((u) => u.pathname);
      if (page !== build || assets.length < 20 || bad.length) {
        throw new Error(`expectBuild ${build}: page build ${page}, ${assets.length} assets, not from ${pre}: ${JSON.stringify(bad.slice(0, 10))}`);
      }
      return { build: page, assets: assets.length };
    },
    // Drop and re-open the server connection (a redeploy) without reloading.
    reconnect() { g.net.disconnect(); g.net.connect(g.hud.get().name || "slopper"); return true; },
    // What the browser said about raw input on the last capture (headless has no
    // real pointer lock): false is Firefox < 152 or on Linux (Options shows why).
    // Act as a browser without keyboard lock (Firefox, Safari): on = true,
    // false = with it, null = this browser's own; forgets "keep Ctrl".
    noKbLock(on) { g.testCtl.noKbLock = on; g.resetCrouchNotice(); try { localStorage.removeItem("cs.keepCtrlCrouch"); } catch { /* none */ } return true; },
    // Click an element (a button, or the canvas: "click to aim").
    async uiClick(selector) {
      const el = document.querySelector(selector);
      if (!el) throw new Error(`nothing matches ${selector}`);
      el.click();
      await nextFrame();
      return true;
    },
    rawInput(r) { g.input.raw = r; g.input.onRaw?.(r); return true; },
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
    // snap: the server tick of the last snapshot the server queued to this client (lockstep):
    // wait until it is in (wall time) before the frame, so the command's view
    // tick (vt), the remotes it collides with and the reconcile never depend
    // on whether the socket delivered it before the harness asked.
    // tick: the server's tick now (lockstep): the command's view tick is
    // pinned to INTERP_DELAY behind it, as a client on a steady link sees.
    // flick: [{mouse: [dx, dy]} | {click: code}] real events in this order after the rest (a click mid-flick).
    async caseTick({ hold = [], tap = [], yaw, pitch, mouse, flick, snap, tick } = {}) {
      if (tick !== undefined) g.testCtl.viewTick = tick - INTERP_DELAY / 1000 / g.simDt;
      // Not with faked loss/lag (those cases lose snapshots on purpose), and not
      // forever: a socket that connected after it went out never gets it.
      const f = g.net.faults, faulty = g.net.fakeLag > 0 || f.loss > 0 || f.dup > 0 || f.jitter > 0 || f.reorder > 0 || g.net.delayed.down > 0 || g.net.fullPendingFaulted?.(); // or still resyncing from a fault's loss
      if (faulty) faultAt = performance.now();
      const faked = faulty || performance.now() - faultAt < 500; // a snapshot lost just before the faults went off
      // snapLate: it never came (the harness fails the case: the client would
      // be acting on stale state, e.g. a delta it couldn't decode).
      let snapLate = false;
      if (snap !== undefined && snap >= 0 && !faked) {
        const t0 = wallNow(); // wall: hang guard on the snapshot wait
        while ((g.testCtl.snapTick ?? -1) < snap && g.hud.get().connected && g.myId && wallNow() - t0 < 2000) await new Promise((r) => setTimeout(r, 0)); // wall: hang guard
        snapLate = (g.testCtl.snapTick ?? -1) < snap && g.hud.get().connected && !!g.myId;
      }
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
      if (flick) {
        const was3 = g.input.locked;
        g.input.locked = true;
        try {
          for (const e of flick) {
            if (e.mouse) dispatchEvent(new MouseEvent("mousemove", { movementX: e.mouse[0], movementY: e.mouse[1] }));
            else { ev(e.click, true); ev(e.click, false); }
          }
        } finally { g.input.locked = was3; }
      }
      g.frame(performance.now(), g.simDt, false); // no render: cases only need the state
      return { ...api.caseState(), snapTick: g.testCtl.snapTick ?? -1, snapLate };
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
        scoped: a.scoped, jumps: me.jumps || 0, carry: !!a.carry, boost: Math.max(0, a.boostEnd - a.tickN), seq: g.netDebug?.state.sent ?? 0,
        scoreboard: !!h2.scoreboard, teamMenu: !!h2.teamMenu, buyMenu: !!h2.buyMenu, mvp: h2.mvp ? `${h2.mvp.name}: ${h2.mvp.why}` : "", shout: g.testCtl.lastShout, chatOpen: h2.chatOpen || "", radioOpen: !!h2.radioOpen, chatLines: (h2.chat || []).map((l) => `${l.radio ? "(RADIO) " : l.tm ? "(TEAM) " : ""}${l.name}: ${l.text}`).join("\n"), killfeed: (h2.feed || []).filter((f) => f.kill).map((f) => f.text).join("\n"), console: !!g.con.open, name: h2.name, team: h2.myTeam, connected: !!h2.connected,
        consoleText: [...document.querySelectorAll(".console-log div")].slice(-300).map((d) => d.textContent).join("\n"),
        binds: g.settings.get().binds,
        holes: g.debris?.stats().holes ?? 0, casings: g.debris?.stats().casings ?? 0,
        dynLights: (g.gfx.stats().dynLightsAdded ?? 0) - flashBase, // muzzle lights added since the case began
        dmgNumbers: document.querySelectorAll(".dmg-layer .dmg:not(.dmg-glyph):not(.dmg-review)").length, // floating damage numbers up (cl_damage_numbers)
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
    // Packets to the server a fake link fault (net_fakelag, ...) holds back right now, and
    // how many it lost so far (engine/net.js delayed, lost).
    netUp() { return { held: g.net.delayed.up, lost: g.net.lost.up }; },
    // The snapshot codec's health (engine/net.js): undecodable snapshots, of
    // them checksum mismatches (a codec bug: cmd/sim fails the case), fulls asked.
    netCodec() { const n = g.net; return { codec: n.codec, errors: n.codecErrors, mismatches: n.codecMismatches, last: n.codecLastError, fullsAsked: n.fullsAsked, clean: n.codecErrorsClean }; },
    // Wait until the server's sync marker n has arrived, then report caseState().
    // Lockstep frames: every message the server sent before sync marker n is in (0: none asked).
    async syncTo(n) {
      for (let i = 0; n && i < 2000 && !(g.testCtl.sync >= n); i++) await new Promise((r) => setTimeout(r, 1)); // wall: hang guard (polls for the harness's sync marker)
      if (n && !(g.testCtl.sync >= n)) throw new Error(`sync ${n} never arrived`);
      return true;
    },
    async caseSync(n) {
      for (let i = 0; i < 2000 && !(g.testCtl.sync >= n); i++) await new Promise((r) => setTimeout(r, 1)); // wall: hang guard (polls for the harness's sync marker)
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
    // Click the element a selector finds (a DOM click, as a mouse click on it would).
    // Type text into an input as a user would (its input event fires: Preact's onInput sees it).
    async typeInto(selector, text) { const el = document.querySelector(selector); if (!el) throw new Error(`no ${selector}`); el.focus(); el.value = text; el.dispatchEvent(new Event("input", { bubbles: true })); await nextFrame(); return true; },
    // Hover an element as the mouse would (mouseenter: Preact's onMouseEnter).
    async hoverEl(selector) { const el = document.querySelector(selector); if (!el) throw new Error(`no ${selector}`); el.dispatchEvent(new MouseEvent("mouseenter")); await nextFrame(); return true; },
    async clickEl(selector) { const el = document.querySelector(selector); if (!el) throw new Error(`no ${selector}`); el.click(); await nextFrame(); return true; },
    scroll(selector, y) { const el = document.querySelector(selector); if (!el) throw new Error(`no ${selector}`); el.scrollTop = y; return true; },
    // Let the real frame loop drive the game for ms (normally the harness steps frames).
    async live(ms = 1000) {
      g.testCtl.live = true;
      try { await gameSleep(ms); } finally { g.testCtl.live = false; }
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
    // Returns this page's server ID (0: none yet), for the harness's sync markers.
    __barrier() { return barrier().then(() => g.myId || 0); },
    __run(name, args) {
      const job = { done: false };
      gate.job = job;
      (async () => api[name](...args))().then((v) => { job.value = v ?? null; }, (e) => { job.err = String(e?.stack || e); }).finally(() => { job.done = true; });
      return true;
    },
    async __poll() {
      const job = gate.job;
      while (!job.done && !gate.waiters.length) await wallSleep(1); // wall: the harness polls the gate (no game time passes)
      if (job.done) return { done: true, err: job.err ?? null, value: job.value ?? null };
      return { done: false, wants: true };
    },
    __ticked() {
      const w = gate.waiters; gate.waiters = []; gate.ticks++;
      w.forEach((r) => r());
      return true;
    },
    // Crosshair comparisons across moments: remember the drawn gap under a name,
    // then expectCrosshair("<", "running") fails unless the gap now is smaller.
    markCrosshair(name) {
      const c = g.testCtl.crosshair;
      if (!c) throw new Error("markCrosshair: no crosshair drawn yet");
      (gate.crosshairMarks ??= {})[name] = c.gap;
      return c;
    },
    expectCrosshair(op, name, factor = 1) {
      const c = g.testCtl.crosshair, ref = gate.crosshairMarks?.[name];
      if (!c || ref == null) throw new Error(`expectCrosshair: nothing marked as ${name}`);
      const v = ref * factor, ok = op === "<" ? c.gap < v : op === ">" ? c.gap > v : op === "==" ? Math.abs(c.gap - v) < 0.05 : false;
      if (!ok) throw new Error(`expectCrosshair failed: gap ${c.gap.toFixed(2)} px, want ${op} ${name} (${ref.toFixed(2)}) x ${factor}`);
      return c;
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
        for (let i = 0; i < 100 && g.loop.rafs < 40; i++) await new Promise((r) => setTimeout(r, 50)); // wall: frame-pacing (realtime: the loop's own timing); measure the refresh first
        g.loop.pinnedRefresh = true; // headless rAF rates drift; keep the divisor stable
        cvars = { ...cvars, fps_max: g.loop.stats().hz / +m[1] };
      } else g.loop.pinnedRefresh = false;
      g.settings.set(cvars);
      await new Promise((r) => setTimeout(r, 300)); // wall: frame-pacing (realtime); settle + refresh measurement
      const t0 = g.loop.frames, r0 = g.loop.rafs;
      await new Promise((r) => setTimeout(r, ms)); // wall: frame-pacing (realtime)
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
    // Collision against the DRAWN model (solid players): from now on, every
    // predicted command is checked against the named remote as drawn at that
    // command's view tick. collideProbeStop() returns
    // {cmds, near, maxIn, maxWall, minGap, min/maxDelayMs}: maxIn = the deepest our own move took
    // us into its drawn hull (m), maxWall = the largest gap to it at a command
    // whose move was stopped while near it (an invisible wall), minGap = the
    // closest gap (max(|dx|, |dz|) - 0.8; < 0 inside). Also in state().collide
    // (expectState {"collide.maxWall": ["<", 0.2]}).
    // viewDelay(ms): pin the remotes' interpolation delay (null: adaptive).
    // Lockstep steps server ticks and page frames at their own pace, which the
    // adaptive delay reads as a jittery link; a scenario about what a client
    // on a real link sees pins the delay that link would have.
    viewDelay(ms = null) { g.remotes.pinDelay(ms); return true; },
    collideProbe(name) {
      const st = g.testCtl.collide = { name, cmds: 0, near: 0, maxIn: 0, maxWall: 0, minGap: 99, maxDelayMs: 0, minDelayMs: 999 };
      g.testCtl.onCmd = ({ before, me, drawn }) => {
        const r = drawn?.find((x) => x.name === name);
        if (!r || !before) return;
        st.cmds++;
        const dl = Math.round(g.remotes.timing(performance.now()).viewDelayMs); // the view delay the commands were drawn with
        st.maxDelayMs = Math.max(st.maxDelayMs, dl); st.minDelayMs = Math.min(st.minDelayMs, dl);
        const h = r.c ? 1.2 : 1.8, R2 = 0.8;
        const yOver = me.pos[1] < r.p[1] + h && r.p[1] < me.pos[1] + me.h;
        const pen = (p) => Math.min(R2 - Math.abs(p[0] - r.p[0]), R2 - Math.abs(p[2] - r.p[2]));
        const gap = -pen(me.pos);
        if (!yOver) return;
        st.minGap = Math.min(st.minGap, gap);
        if (pen(me.pos) > pen(before.pos)) st.maxIn = Math.max(st.maxIn, pen(me.pos));
        if (gap < 3) {
          st.near++;
          // A horizontal velocity component the step zeroed: stopped by something.
          const stopped = [0, 2].some((i) => Math.abs(before.vel[i]) > 0.5 && me.vel[i] === 0);
          if (stopped) st.maxWall = Math.max(st.maxWall, gap);
        }
      };
      return true;
    },
    collideProbeStop() {
      g.testCtl.onCmd = null;
      return g.testCtl.collide ?? null;
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
      const t0 = wallNow(); // wall: hang guard (previews render in a worker in real time)
      for (;;) {
        const h = g.hud.get(), names = h.vote ? h.vote.options : (h.mapCards || []).map((c) => c.name);
        if (names.length && names.every((n) => h.mapPreviews?.[n])) { await nextFrame(); return names; }
        if (wallNow() - t0 > timeoutMs) throw new Error(`map previews not ready: have ${Object.keys(h.mapPreviews || {})}`); // wall: hang guard
        await new Promise((r) => setTimeout(r, 50)); // wall: polls browser work (no game time)
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
    // Change a control on the Server settings screen as a player does: a cvar's
    // checkbox (value "1"/"0"), select or number field ([data-cvar]), or a map's
    // rotation checkbox ("map:<name>", value "1"/"0"); the change event commits it.
    async serverSetting(key, value) {
      const el = key.startsWith("map:") ? document.querySelector(`.server-settings [data-map="${key.slice(4)}"]`)
        : document.querySelector(`.server-settings [data-cvar="${key}"]`);
      if (!el) throw new Error(`no server setting ${key}`);
      if (el.type === "checkbox") el.checked = String(value) === "1";
      else el.value = String(value);
      el.dispatchEvent(new Event("change", { bubbles: true }));
      await nextFrame();
      return true;
    },
    // Show a UI state: play | menu | options | controls | maps | server | console | scoreboard | teammenu
    // (server: the admins' Server settings; fails without its menu entry)
    async ui(name) {
      if (g.con.open && name !== "console") g.con.toggle(false);
      g.hud.set({ scoreboard: false, teamMenu: false });
      if (name === "play") g.hud.set({ screen: "playing" });
      else if (["menu", "options", "controls", "maps", "server"].includes(name)) {
        if (g.hud.get().screen !== "playing") { g.hud.set({ screen: "playing" }); await nextFrame(); } // the menu opens at its top, as Esc does
        g.hud.set({ screen: "ingame", menuOpenedAt: 0 });
      }
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
      if (name === "server") {
        if (!document.querySelector(".server-settings-btn")) throw new Error("no SERVER SETTINGS in the menu (not admin?)");
        await click("SERVER SETTINGS");
      }
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
      // The bot as drawn now, and the centre of its `region` hit part.
      const drawn = () => {
        const bot = g.remotes.sample(performance.now()).find((r) => /\(bot/.test(r.name));
        if (!bot) throw new Error("no bot");
        const st = { p: bot.p, yaw: bot.yaw, pitch: bot.pitch, crouch: !!bot.c, team: bot.team, id: bot.id, gait: bot.g, t: bot.t };
        const part = g.hitParts(st).find((h) => h.region === region);
        const m = part.m, c = part.c;
        return { bot, target: [0, 1, 2].map((i) => m[i] * c[0] + m[4 + i] * c[1] + m[8 + i] * c[2] + m[12 + i]) };
      };
      let { bot, target } = drawn();
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
      // A walking bot moved on while the weapon deployed: aim again at where it is
      // drawn now (view angles only: a teleport would restart the command stream).
      ({ bot, target } = drawn());
      const d2 = target.map((v, i) => v - eye[i]);
      me.yaw = Math.atan2(-d2[0], -d2[2]); me.pitch = Math.atan2(d2[1], Math.hypot(d2[0], d2[2]));
      g.snapView();
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
    // Jitter probe: the live loop (on the virtual clock in lockstep) for `ms` while strafing
    // left/right every `strafeMs` (and running forward), sampling every frame:
    // own-view smoothness (drawn eye step vs velocity*dt), prediction
    // corrections, view angles moved by anything but input, and the remote
    // timeline (under-runs: drawn past the newest snapshot; time jumps).
    // `max` ({field: limit}) fails the step when a measured field exceeds it.
    // draw: false computes the view every frame but skips the GPU work, so a slow
    // (software-rendered) runner still runs the loop at a realistic frame rate.
    // What you see is where the server has you: the live
    // loop for the `moves` schedule ([[ms, [actions...]], ...]), and every frame
    // the drawn feet (predicted body + the smoothing offset, predictor.error) at
    // the newest command; when the server acks that command, its body there is
    // compared with what was drawn. Returns the horizontal gaps (m) and how long
    // the smoothing offset stayed above errLimit. `max` ({field: limit}) fails it.
    // Drive this client in the background on the live loop (its own timers: virtual in lockstep):
    // cycle the `moves` schedule ([[ms, [actions...], yaw?], ...]) until called
    // with stop: true, so another client's probe can run against it moving.
    drive({ moves = [], stop = false } = {}) {
      const d = (g.testCtl.drive ||= { gen: 0, held: [] });
      const press = (a) => { for (const h of d.held) setAction(h, false); d.held = a; for (const h of d.held) setAction(h, true); };
      const gen = ++d.gen;
      if (stop || !moves.length) { press([]); g.testCtl.live = false; return true; }
      g.testCtl.live = true;
      (async () => {
        for (let i = 0; d.gen === gen; i = (i + 1) % moves.length) {
          const [ms, acts, yaw] = moves[i];
          if (yaw !== undefined) g.me.yaw = yaw;
          press(acts);
          await new Promise((r) => globalThis.setTimeout(r, ms)); // the page's timers: virtual in lockstep (moves with the harness's ticks)
        }
      })();
      return true;
    },
    // series: also return [ms, gap, |error|, last correction] per matched ack.
    // chase: a regex on a remote's name: every frame, turn towards it as drawn
    // (keep pushing into a moving player; the moves' keys are relative to that).
    // Forced desync: knock the drawn body off the server's on the
    // live loop, standing still: kind "body" moves the predicted body `size` m
    // (the next snapshot corrects it), "offset" adds `size` m to the smoothing
    // offset. Returns how long until the drawn feet are within 5 cm of the
    // server's again and how many re-syncs fired; throws past `withinMs` or
    // unless `resyncs` fired (when given).
    // noDecay: the offset is never eased out (the old bug), so only the safety net brings it back.
    async desyncProbe({ kind = "body", size = 1, withinMs = 400, resyncs, noDecay = false } = {}) {
      g.testCtl.live = true;
      g.testCtl.noDecay = noDecay;
      g.testCtl.skipGfx = true; // the view is still computed every frame, without the GPU work
      try {
        await gameSleep(300);
        const st = g.predictor.stats, r0 = st.resyncs;
        if (kind === "body") g.me.pos[0] += size; else g.predictor.error[0] += size;
        const t0 = performance.now();
        let back = null, lastT = g.testCtl.view?.t ?? t0, lastSeen = t0, maxFrameGap = 0;
        while (performance.now() - t0 < withinMs + 1000) {
          await gameSleep(10);
          const vt = g.testCtl.view?.t, now = performance.now();
          if (vt !== lastT) { lastT = vt; lastSeen = now; } else maxFrameGap = Math.max(maxFrameGap, now - lastSeen);
          const m = g.testCtl.lastMe, e = g.predictor.error;
          const gap = m ? Math.hypot(g.me.pos[0] + e[0] - m.p[0], g.me.pos[2] + e[2] - m.p[2]) : Infinity;
          if (gap <= 0.05) { back = performance.now() - t0; break; }
        }
        // A starved page (a loaded runner: no frame for 200+ ms) can't ease the
        // offset out in time, so the stuck check rightly fires: don't count it.
        const starved = maxFrameGap > 200;
        const out = { kind, size, backMs: back === null ? null : Math.round(back), resyncs: st.resyncs - r0, last: st.lastResync, maxFrameGap: Math.round(maxFrameGap), starved };
        console.warn("desyncProbe", JSON.stringify(out));
        if (back === null || back > withinMs) throw new Error(`desyncProbe: not back within ${withinMs} ms: ${JSON.stringify(out)}`);
        if (resyncs !== undefined && out.resyncs !== resyncs && !starved) throw new Error(`desyncProbe: ${out.resyncs} re-syncs, want ${resyncs}: ${JSON.stringify(out)}`);
        return out;
      } finally { g.testCtl.live = false; g.testCtl.noDecay = false; g.testCtl.skipGfx = false; }
    },
    async ownProbe({ moves = [[3000, ["+forward"]]], errLimit = 0.2, max = {}, draw = false, series: wantSeries = false, chase } = {}) {
      const chaseRe = chase ? new RegExp(chase) : null;
      const series = wantSeries ? [] : null, t0 = performance.now();
      g.testCtl.skipGfx = !draw;
      g.testCtl.live = true;
      const drawn = new Map(); // seq -> drawn feet [x, z]
      const gaps = [];
      let lastAck = -1, run = true, held = [], errSince = null;
      const R = { frames: 0, acks: 0, matched: 0, maxGap: 0, p95Gap: 0, over5cm: 0, maxErr: 0, errAboveMs: 0, corrections: 0, maxCorrection: 0 };
      const st = g.predictor.stats, c0 = st.corrections, r0 = st.resyncs;
      const press = (a) => { for (const h of held) setAction(h, false); held = a; for (const h of held) setAction(h, true); };
      const sample = () => {
        if (!run) return;
        R.frames++;
        if (chaseRe && g.me) {
          const t = g.remotes.sample(performance.now()).find((r) => chaseRe.test(r.name) && !r.dead);
          if (t) { R.near = Math.min(R.near ?? Infinity, +Math.hypot(t.p[0] - g.me.pos[0], t.p[2] - g.me.pos[2]).toFixed(3)); g.me.yaw = Math.atan2(-(t.p[0] - g.me.pos[0]), -(t.p[2] - g.me.pos[2])); }
        }
        const pend = g.predictor.pendingCmds(), e = g.predictor.error;
        if (pend.length && g.me) drawn.set(pend[pend.length - 1].seq, [g.me.pos[0] + e[0], g.me.pos[2] + e[2]]);
        const m = g.testCtl.lastMe;
        if (m && m.ack !== lastAck && m.p) {
          lastAck = m.ack; R.acks++;
          const d = drawn.get(m.ack);
          if (d) {
            R.matched++; const gp = Math.hypot(d[0] - m.p[0], d[1] - m.p[2]); gaps.push(gp);
            if (series) series.push([Math.round(performance.now() - t0), +gp.toFixed(3), +Math.hypot(g.predictor.error[0], g.predictor.error[2]).toFixed(3), +g.predictor.stats.lastError.toFixed(3)]);
          }
          for (const k of drawn.keys()) if (k <= m.ack) drawn.delete(k);
        }
        const el = Math.hypot(e[0], e[2]), now = performance.now();
        R.maxErr = Math.max(R.maxErr, el);
        if (el > errLimit) { errSince ??= now; R.errAboveMs = Math.max(R.errAboveMs, now - errSince); } else errSince = null;
        if (st.corrections !== c0) { R.corrections = st.corrections - c0; R.maxCorrection = Math.max(R.maxCorrection, st.lastError); }
        requestAnimationFrame(sample);
      };
      requestAnimationFrame(sample);
      try {
        for (const [ms, acts, yaw] of moves) { if (yaw !== undefined) g.me.yaw = yaw; press(acts); await gameSleep(ms); }
      } finally {
        run = false; press([]); g.testCtl.live = false; g.testCtl.skipGfx = false;
      }
      gaps.sort((a, b) => a - b);
      R.maxGap = +(gaps[gaps.length - 1] || 0).toFixed(3);
      R.p95Gap = +(gaps[Math.floor(gaps.length * 0.95)] || 0).toFixed(3);
      R.medGap = +(gaps[Math.floor(gaps.length / 2)] || 0).toFixed(3);
      R.over5cm = gaps.filter((x) => x > 0.05).length;
      R.resyncs = st.resyncs - r0;
      R.fps = Math.round(R.frames / ((performance.now() - t0) / 1000)); // a starved runner (CI load) draws a few frames a second
      R.maxErr = +R.maxErr.toFixed(3); R.maxCorrection = +R.maxCorrection.toFixed(3); R.errAboveMs = Math.round(R.errAboveMs);
      console.warn("ownProbe", JSON.stringify(R));
      if (series) { R.series = series; console.warn("ownProbe series", JSON.stringify(series)); }
      const bad = Object.entries(max).filter(([k, v]) => R[k] > v);
      if (bad.length) throw new Error(`ownProbe: ${bad.map(([k, v]) => `${k} ${R[k]} > ${v}`).join(", ")} (${JSON.stringify(R)})`);
      if (!R.matched) throw new Error(`ownProbe: no acked command matched a drawn one (${JSON.stringify(R)})`);
      return R;
    },
    async jitterProbe({ ms = 6000, strafeMs = 700, forward = true, paced, max = {}, draw = true } = {}) {
      g.testCtl.skipGfx = !draw;
      if (paced !== undefined) g.net.send({ t: "paced", c: paced });
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
          await gameSleep(strafeMs);
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
      const me = g.me, nd = g.netDebug.state;
      g.net.send({ t: "paced", c: true });
      g.testCtl.clockScale = scale;
      g.testCtl.live = true;
      api.weapon("sidearm"); // semi-automatic: one shot per tap, whatever the frame timing
      await gameSleep(600);
      const taps = [];
      let worst = 0, worstQueue = 0, hits = 0, disagree = 0, lostPress = 0, results = [];
      try {
        const t0 = performance.now();
        let n = 0;
        while (performance.now() - t0 < ms) {
          // walk a bit between taps (as the operator did), then stand still and aim
          if (walk) { setAction(n % 2 ? "+moveleft" : "+moveright", true); await gameSleep(tapMs / 2); setAction(n % 2 ? "+moveleft" : "+moveright", false); }
          await gameSleep(walk ? tapMs / 4 : tapMs / 2);
          if ((g.hud.get().ammo ?? 99) < 3) { api.reload(); await gameSleep(3000); }
          for (let i = 0; i < 50 && Math.hypot(me.vel[0], me.vel[2]) > 0.01; i++) await gameSleep(10); // stopped
          const bot = g.remotes.sample(performance.now()).find((r) => /\(bot/.test(r.name));
          const part = g.hitParts({ p: bot.p, yaw: bot.yaw, pitch: bot.pitch, crouch: !!bot.c, team: bot.team, id: bot.id, gait: bot.g, t: bot.t }).find((h) => h.region === "head");
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
          // A short click, as a player makes: +attack is latched (game/controls.js), so the
          // next command carries it even when a stalled frame loop builds none until the
          // release (it once had to be held until two commands sampled it).
          setAction("+attack", true);
          await gameSleep(30);
          setAction("+attack", false);
          taps.push(seq0);
          worst = Math.max(worst, nd.sent - nd.acked);
          worstQueue = Math.max(worstQueue, nd.queue);
          await gameSleep(walk ? tapMs / 4 : tapMs / 2);
          for (let i = 0; i < 100 && !(nd.lastShot && nd.lastShot.seq > seq0); i++) await gameSleep(10);
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
        await gameSleep(600);
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
        const st = { p: bot.p, yaw: bot.yaw, pitch: bot.pitch, crouch: !!bot.c, team: bot.team, id: bot.id, gait: bot.g, t: bot.t };
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
    // target: a regex on the remote's name (default: a bot). offset: aim that many
    // m beside the part's centre (to the side, across the line of fire): 1 must
    // miss. The aim is taken from the drawn eye (prediction + its smoothing
    // offset), as the player sees it.
    async shootMovingBot({ region = "body", dist = 7, shots = 6, minHits = 0, maxHits = Infinity, target: who = "\\(bot", offset = 0 } = {}) {
      const re = new RegExp(who);
      const findBot = () => g.remotes.sample(performance.now()).find((r) => re.test(r.name) && !r.dead);
      let bot = findBot();
      if (!bot) throw new Error(`no target matching ${who}`);
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
          await gameSleep(500); // the teleport reaches the server (fake lag included)
          api.step(30, 0.01);
        }
        await gameSleep(160);
        api.step(20, 0.01); // fire rate: weapon timers run on the command clock (ours and the server's)
        // Stepping renders frames synchronously (software GL: ~30 ms each), so
        // no snapshot got in meanwhile; let the stream catch up, as it does
        // for a real client between frames, before aiming at what's on screen.
        await gameSleep(150);
        const now = performance.now();
        bot = findBot();
        const st = { p: bot.p, yaw: bot.yaw, pitch: bot.pitch, crouch: !!bot.c, team: bot.team, id: bot.id, gait: bot.g, t: bot.t };
        const part = g.hitParts(st).find((h) => h.region === region);
        const m = part.m, c = part.c;
        const target = [0, 1, 2].map((i) => m[i] * c[0] + m[4 + i] * c[1] + m[8 + i] * c[2] + m[12 + i]);
        const e = g.predictor.error;
        const eye = [me.pos[0] + e[0], me.pos[1] + e[1] + me.eyeH + me.eyeOff, me.pos[2] + e[2]];
        if (offset) { // sideways, perpendicular to the line of fire
          const fx = target[0] - eye[0], fz = target[2] - eye[2], fl = Math.hypot(fx, fz) || 1;
          target[0] += (-fz / fl) * offset; target[2] += (fx / fl) * offset;
        }
        const d = target.map((v, i) => v - eye[i]);
        me.yaw = Math.atan2(-d[0], -d[2]); me.pitch = Math.atan2(d[1], Math.hypot(d[0], d[2]));
        g.testCtl.lastHit = null; g.testCtl.lastImpacts = null;
        const vtSent = g.remotes.viewTick(now);
        const newestTick = g.remotes.viewTick(now + 1e6); // the newest snapshot we have
        g.arsenal.noSpread = true;
        setAction("+attack", true); g.frame(now, 0.01, false); setAction("+attack", false); g.frame(performance.now(), 0.01, true);
        g.arsenal.noSpread = false;
        const at = bot.p;
        for (let i = 0; i < 40 && !g.testCtl.lastHit; i++) await gameSleep(25);
        for (let i = 0; i < 20 && !g.testCtl.lastImpacts; i++) await gameSleep(25); // the server's account (sv_showimpacts)
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
    // Cut the link to the server (engine/net.js cut): "silent" = the socket
    // stays open but nothing gets through (a dead link); "closed" = it drops
    // and every reconnect fails; null = heal (the next retry gets through).
    cutLink(mode = "silent") { g.net.setCut(mode); return true; },
    // Click a visible <button> by its text, as a player would (with focus).
    async clickButton(text) {
      const b = [...document.querySelectorAll("button")].find((e) => e.textContent.trim() === text && e.offsetParent && getComputedStyle(e).visibility !== "hidden");
      if (!b) throw new Error(`no visible button "${text}"`);
      b.focus(); b.click();
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
        collide: g.testCtl.collide ?? null, // collideProbe: our moves vs a remote's drawn model
        debris: g.debris?.stats() ?? null, // bullet holes, casings (lying + flying), bolt casings waiting (debris.js)
        marks: g.gfx.stats().marks ?? null, // what the renderer drew of them last frame: {decals, bits, draws}
        corpses: g.testCtl.corpses ?? 0, // bodies drawn last frame (corpse.js): lying, falling or shrinking away
        crosshair: g.testCtl.crosshair ?? null, // drawn last frame (crosshair.js, centred): gap (px, the lines' inner tips), r (the spread cone's own radius, px), x/y (where the next shot's cone is centred, px from the screen centre)
        spray: { idx: g.arsenal.recoil, shot: g.arsenal.shotIdx }, // the recoil spray: index as of the last shot, the index that shot used
        id: g.myId, team: h.myTeam, observe: h.observe ?? null, round: g.rounds?.state ?? null, roundEnd: h.roundEnd ?? null,
        items: g.items?.() ?? [],
        me: me && { pos: me.pos.map(round), vel: me.vel.map(round), yaw: round(me.yaw), pitch: round(me.pitch), crouched: me.crouched, onGround: me.onGround, jumps: me.jumps || 0 },
        movers: g.moverClock?.(), // the movers' clock: t (drawn at, s), view (the remotes' server time, s), tb (our tick base)
        codec: api.netCodec(), // snapshot codec errors / mismatches / fulls asked (engine/net.js)
        net: g.netDebug && (({ sent, acked, queue, dropped, serverTick, lastShot }) => ({ sent, acked, backlog: sent - acked, queue, dropped, serverTick, lastShot, viewDelayMs: Math.round(g.remotes.timing(performance.now()).viewDelayMs) }))(g.netDebug.state),
        audio: g.audioState?.(),
        gfx: (({ drawn, aa, renderScale, target, lights, lightsActive, lightShadows, lightShadowBudget, lightShadowMapRenders, lightShadowSolidRenders, lightShadowTiles, lightShadowBaked, lightShadowQueued, lightShadowKB, lightShadowBakeMs, dynLights, dynLightsAdded, dynLightsDropped, dynLightAddMs, dynLightAddPeakMs, dynLightFloods, dynShadows, dynShadowBudget, dynShadowRenders, dynLightFrameMsMax, dynLightMaskHits, dynLightMaskSkips, dynLightUploads, beltSteps, mapLoadMs, mapLoading }) => ({
          drawn, // frames rendered on the GPU (a lockstep page: only when a step asks, __cs.draw)
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
        // The menu's buttons as shown, whether the server says we're admin, and the Server settings
        // screen's controls (cvar: value shown; rotation: the maps ticked).
        menuButtons: [...document.querySelectorAll(".overlay .panel > button")].map((b) => b.textContent.trim()), admin: !!h.admin, cvars: h.cvars ?? null,
        serverSettings: document.querySelector(".server-settings") && Object.fromEntries([
          ...[...document.querySelectorAll(".server-settings [data-cvar]")].map((e) => [e.dataset.cvar, e.type === "checkbox" ? (e.checked ? "1" : "0") : e.value]),
          ["rotation", [...document.querySelectorAll(".server-settings [data-map]:checked")].map((e) => e.dataset.map).join(",")]]),
        // The server link: mode (ok | blip | lost), why, and what the banner/overlay says and offers.
        link: { mode: h.link?.mode ?? "ok", why: h.link?.why ?? "", attempt: h.link?.attempt ?? 0, text: (() => { const e = document.querySelector(".link-blip:not(.link-gone), .link-lost:not(.link-gone)"); return e ? e.textContent.replace(/\s+/g, " ").trim() : ""; })(),
          buttons: [...document.querySelectorAll(".link-lost:not(.link-gone) .ll-buttons button")].map((b) => b.textContent.trim()), gone: !!document.querySelector(".link-gone") },
        hud: { nadeRow: [...document.querySelectorAll(".slots .nade")].map((e) => { const t = e.querySelector(".ktag")?.textContent; return (e.classList.contains("cur") ? "*" : "") + (t ? `[${t}] ` : "") + e.textContent.slice(t?.length || 0); }).join(" | "), loadingMap: h.loadingMap ?? null, loadingText: document.querySelector(".map-loading")?.textContent ?? "", gpu: h.gpu ?? null, gpuText: document.querySelector(".gpu-count")?.textContent ?? "", hint: h.hint || "", work: h.work ?? null, workText: document.querySelector(".work-label")?.textContent ?? "",
          roundEndText: document.querySelector(".round-end .re-line")?.textContent ?? "", hp: h.hp, ammo: h.ammo, reserve: h.reserve, weapon: h.weapon, money: h.money, armor: h.armor, mvp: h.mvp ?? null, dmgReport: h.dmgReport ?? null, shades: !!h.shades, boost: h.boost || 0, ping: h.ping, connected: h.connected, buyMenu: !!h.buyMenu, canBuy: !!h.canBuy, buyMsg: h.buyMsg || "", buyLine: document.querySelector(".buy-msg")?.textContent ?? "", xhair: (() => { const c = [...document.querySelectorAll(".crosshair")].find((e) => !e.closest(".xh-cell")); if (!c) return ""; const o = c.getBoundingClientRect(), r = c.querySelector(".r")?.getBoundingClientRect(), cs = getComputedStyle(c.querySelector("i")); return `${[...c.children].map((i) => i.className).join(" ")} · gap ${Math.round((r.left - o.left) * 2) / 2} len ${Math.round(r.width * 2) / 2} th ${Math.round(r.height * 2) / 2} · ${cs.backgroundColor} · ${getComputedStyle(c).opacity}${cs.boxShadow === "none" ? "" : " · outline"}`; })(), xhCode: document.querySelector(".xh-code")?.value ?? "", ciBadge: document.querySelector(".top-mid .timer.ci")?.textContent ?? "", topMid: document.querySelector(".top-mid")?.textContent ?? "", helmet: !!h.helmet, kit: !!h.kit, buyItems: [...document.querySelectorAll(".buy-item")].map((b) => `${b.querySelector("b")?.textContent || ""} ${b.querySelector(".bi-name")?.textContent} ${b.querySelector(".bi-price")?.textContent}${b.disabled ? " x" : ""}`.replace(/ /g, " ")).join(" | "), buyTags: [...document.querySelectorAll(".buy-item")].map((b) => b.querySelector(".ktag")?.textContent || "").filter(Boolean).join(" "), killTags: [...document.querySelectorAll(".kill-row .ktag")].map((e) => e.textContent).join(" "), buyDetail: (() => { const d = document.querySelector(".buy-detail"); if (!d) return ""; return [d.dataset.item, d.querySelector(".bd-type")?.textContent, d.querySelector(".bd-good")?.textContent, [...d.querySelectorAll(".bi-stats > span:nth-child(3n+1)")].map((e) => e.textContent).join(",")].join(" | ").replace(/\s+/g, " "); })(), buyTypes: [...document.querySelectorAll(".buy-item .bi-type")].map((e) => e.textContent).join(" | "), buyLeft: h.buyLeft ?? null, buyLayout: document.querySelector(".buy-a") ? "catalog" : document.querySelector(".buy-c") ? "column" : "", buyAmmo: [...document.querySelectorAll(".ammo-card")].map((c) => `${c.dataset.item} ${c.querySelector(".key")?.textContent} ${c.querySelector(".ac-name")?.textContent.replace(/\s+/g, " ").trim()} ${Math.round(parseFloat(c.querySelector(".bar i")?.style.width) || 0)}% ${c.querySelector(".ac-act")?.textContent.replace(/\s+/g, " ").trim()}${c.disabled ? " x" : ""}`.replace(/ /g, " ")).join(" | "), buyBadge: (() => { const b = document.querySelector(".buy-badge"); if (!b) return ""; const r = b.getBoundingClientRect(), x = r.x + r.width / 2, y = r.y + r.height / 2; return b.contains(document.elementFromPoint(x, y)) ? [...b.children].map((e) => e.textContent.trim()).filter(Boolean).join(" ") : "covered"; })(), scrollsX: document.documentElement.scrollWidth > innerWidth || [...document.querySelectorAll(".buy-menu, .buy-overlay")].some((e) => e.scrollWidth > e.clientWidth + 1), dmgArcs: (h.dmgArcs || []).length, killRows: document.querySelectorAll('.kill-row').length, screenShades: [...document.querySelectorAll('#hud *')].filter((e) => { const r = e.getBoundingClientRect(), cs = getComputedStyle(e); return r.width >= innerWidth * 0.9 && r.height >= innerHeight * 0.9 && cs.display !== 'none' && +cs.opacity > 0 && (cs.backgroundImage !== 'none' || !/rgba\(.*, 0\)|transparent/.test(cs.backgroundColor)); }).map((e) => e.className).join(' '), sbDead: document.querySelectorAll('.scoreboard tr.dead').length, crouchNotice: !!document.querySelector('.crouch-notice'), rawHint: document.querySelector('.options .raw-hint')?.textContent ?? '', sbPings: [...document.querySelectorAll('.scoreboard tr:not(.me) .sb-ping')].filter((e) => /^(\d+|BOT)$/.test(e.textContent)).length, sbAlive: [...document.querySelectorAll('.scoreboard .sb-alive')].map((e) => e.textContent).join(" | "), tagsUnderHud: (() => { const t = document.querySelector('.tags'), hd = document.getElementById('hud'); return !!t && !!hd && +getComputedStyle(t).zIndex < +getComputedStyle(hd).zIndex; })(), mapCats: [...document.querySelectorAll('.maps-panel .map-cat')].map((h) => `${h.dataset.cat}: ${[...h.nextElementSibling.querySelectorAll('.map-card')].map((c) => c.dataset.map).join(" ")}`).join(" | "), optionSections: [...document.querySelectorAll('.panel.options h4')].map((e) => e.firstChild?.textContent?.trim()).join(" | "), chatLines: (h.chat || []).map((l) => `${l.radio ? "(RADIO) " : l.tm ? "(TEAM) " : ""}${l.name}: ${l.text}`).join("\n"), feed: (h.feed || []).map((f) => f.text + (f.kill ? ' ' + f.kill.kteam + '>' + f.kill.vteam : '')), killIcons: [...document.querySelectorAll('.kill-row img.kw')].filter((i) => i.naturalWidth > 0).length, dmgArcEls: [...document.querySelectorAll('.dmg-arc')].map((e) => { const r = e.getBoundingClientRect(); return [Math.round(r.x), Math.round(r.y), Math.round(r.width), e.style.transform, e.style.opacity]; }) },
        players: rs.map((r) => ({ id: r.id, name: r.name, team: r.team, pos: r.p.map(round), crouched: !!r.c })),
        byName: Object.fromEntries(rs.map((r) => { const gt = r.g; return [r.name, { team: r.team, pos: r.p.map(round), crouched: !!r.c, gait: gt && { amp: round(gt.amp), air: round(gt.air), phase: round(gt.phase) } }]; })), // "byName.bob.team"
        roundWin: h.roundEnd?.win ?? null, // the round-end banner's winning team (gone when the next round starts)
        score: h.score ?? null, // rounds won per team: {vibe, artisan}
        feed: h.feed.map((f) => f.text),
        teamMenu: !!h.teamMenu,
        caption: { shout: h.shout?.text ?? null, subtitle: h.subtitle?.text ?? null }, // announcer captions on screen
        lastHit: g.testCtl.lastHit ? { part: g.testCtl.lastHit.part, dmg: g.testCtl.lastHit.dmg } : null, // last server-confirmed hit
        hurtFx: g.testCtl.hurtFx ?? null, // hit feedback on screen: vignette edges/alpha, lowHp edge, burst particles, flinch
        dmgNumbers: document.querySelectorAll(".dmg-layer .dmg:not(.dmg-glyph):not(.dmg-review)").length,
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
  let faultAt = -Infinity; // caseTick: when a fake link fault was last on
  let flashBase = 0; // gfx dynLightsAdded when the case began (caseState muzzleLights)
  function ev(code, down) {
    const m = /^Mouse(\d+)$/.exec(code);
    if (m) dispatchEvent(new MouseEvent(down ? "mousedown" : "mouseup", { button: +m[1] }));
    else dispatchEvent(new KeyboardEvent(down ? "keydown" : "keyup", { code, cancelable: true }));
  }
  // Helpers that render on purpose (benches, lamp-shadow bakes, muzzle-light stats, the
  // net probe's drawn frames) draw even on a lockstep page.
  for (const k of ["settleLights", "muzzleFlashes", "sprayLoad", "bench", "jitterProbe"]) {
    const f = api[k];
    api[k] = (...a) => withGL(() => f.apply(api, a));
  }
  window.__cs = api;
}

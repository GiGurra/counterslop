// window.__cs: the test harness's handle on a client (cmd/sim drives it over CDP).
// Installed only when the server runs with -test AND the page has ?test=1, so
// it never exists in prod. Every call returns plain JSON; expect*/rateCheck
// throw on failure, which fails the scenario.
import { mulberry32 } from "../gen/rng.js";

const ARSENAL_KEYS = ["slots", "cur", "last", "nextShot", "reloadLeft", "reloadTime", "kick", "flash", "bob", "raise", "swing", "clicked", "scoped", "burst", "clock"];

export function installTestApi(g) {
  // Let Preact render. A timer, not rAF: background headless tabs throttle rAF.
  const nextFrame = () => new Promise((r) => setTimeout(r, 30));
  const round = (v) => Math.round(v * 1000) / 1000;

  const api = {
    // Join the match and wait for the welcome, then pick a team from the
    // team-select screen (vibe | artisan | auto | spec; null = stay on that screen).
    // The harness can't take pointer lock (no user gesture), so play is entered
    // directly and aiming faked.
    async join(name = "bot", team = "auto") {
      g.play(name);
      for (let i = 0; i < 200 && !g.me; i++) await new Promise((r) => setTimeout(r, 25));
      if (!g.me) throw new Error("no welcome from server");
      g.hud.set({ screen: "playing", aiming: true, name });
      if (team) await api.team(team);
      api.step(1);
      return { id: g.myId };
    },
    // Advance by n frames of dt seconds each (the sim runs its fixed ticks
    // inside) and render the last one.
    step(n = 1, dt = 1 / 60) {
      for (let i = 0; i < n; i++) g.frame(performance.now(), dt, i === n - 1);
      return api.state();
    },
    // Fail unless every dotted path of state() matches: {"me.jumps": 1, "me.pos.1": [">", 0.5]}.
    expectState(want) {
      const st = api.state(), bad = [];
      for (const [path, w] of Object.entries(want)) {
        const got = path.split(".").reduce((o, k) => o?.[k], st);
        const [op, v] = Array.isArray(w) ? w : ["==", w];
        const ok = op === "==" ? (typeof v === "number" ? Math.abs(got - v) < 1e-3 : got === v)
          : op === "!=" ? got !== v : op === ">" ? got > v : op === "<" ? got < v
          : op === "~" ? Math.abs(got - v[0]) <= v[1] : false;
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
      if (pos) { me.pos = [...pos]; me.vel = [0, 0, 0]; }
      if (yaw !== undefined) me.yaw = yaw;
      if (pitch !== undefined) me.pitch = pitch;
      if (crouch !== undefined) { me.crouched = !!crouch; me.h = crouch ? 1.2 : 1.8; me.eyeH = crouch ? 1.05 : 1.62; }
      g.snapView();
      g.net.send({ t: "state", p: me.pos, yaw: me.yaw, pitch: me.pitch, c: me.crouched, sp: g.spawnSeq });
      return api.state();
    },
    // Hold exactly these keys (event.code names) until changed; fire = mouse button.
    hold(keys = [], fire = false, zoom = false) { g.input.setHeld(keys); g.input.fire = fire; g.input.alt = zoom; return true; },
    // Show a UI state: play | menu | options | console | scoreboard | teammenu
    async ui(name) {
      if (g.con.open && name !== "console") g.con.toggle(false);
      g.hud.set({ scoreboard: false, teamMenu: false });
      if (name === "play") g.hud.set({ screen: "playing" });
      else if (name === "menu" || name === "options") g.hud.set({ screen: "ingame", menuOpenedAt: 0 });
      else if (name === "console") g.con.toggle(true);
      else if (name === "scoreboard") g.hud.set({ scoreboard: true });
      else if (name === "teammenu") g.hud.set({ teamMenu: true });
      else throw new Error(`unknown ui state ${name}`);
      await nextFrame();
      if (name === "options") {
        [...document.querySelectorAll("button")].find((b) => b.textContent.trim() === "OPTIONS")?.click();
        await nextFrame();
      }
      return true;
    },
    // Pick a team like the team-select screen does and wait for the server's answer.
    async team(t) {
      const seq = g.spawnSeq;
      g.hud.set({ teamMenu: false });
      g.con.exec(`jointeam ${t}`);
      for (let i = 0; i < 80 && g.spawnSeq === seq; i++) await new Promise((r) => setTimeout(r, 25));
      api.step(1);
      return g.myTeam;
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
      else g.input.alt = on ?? !g.input.alt;
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
      const zoom = (on) => { g.settings.set({ zoom_mode: "hold" }); g.input.alt = on; api.step(2, 0.01); };
      zoom(false);
      const plain = shift();
      zoom(true);
      if (!g.arsenal.scoped) throw new Error("current weapon has no scope");
      const zoomed = shift();
      zoom(false);
      if (Math.abs(plain - zoomed) > 1) throw new Error(`zoom sensitivity mismatch: ${plain.toFixed(1)} px unzoomed vs ${zoomed.toFixed(1)} px zoomed`);
      return { plain, zoomed };
    },
    // Show/hide the HUD and name tags (clean map shots).
    hud(show = true) {
      for (const el of [document.getElementById("hud"), document.querySelector(".tags")]) if (el) el.style.display = show ? "" : "none";
      return true;
    },
    cvar(k, v) { g.settings.set({ [k]: v }); return g.settings.get()[k]; },
    // Pin the shader clock (clouds, slop wobble) so captures are stable.
    time(t) { g.testCtl.pinnedTime = t; return true; },
    // Free camera: {eye, yaw, pitch}, "overview" (top-down), {orbit: angle}, or null for first person.
    camera(spec) {
      if (!spec) g.testCtl.camera = null;
      else if (spec === "overview") g.testCtl.camera = { eye: [0, 75, 0.01], yaw: 0, pitch: -1.5 };
      else if (spec.orbit !== undefined) {
        const a = spec.orbit, r = spec.radius ?? 42, h = spec.height ?? 26;
        g.testCtl.camera = { eye: [Math.sin(a) * r, h, Math.cos(a) * r], yaw: a, pitch: -Math.atan2(h, r) * 0.9 };
      } else g.testCtl.camera = { eye: spec.eye, yaw: spec.yaw ?? 0, pitch: spec.pitch ?? 0 };
      return true;
    },
    state() {
      const me = g.me, h = g.hud.get();
      return {
        id: g.myId, team: h.myTeam,
        items: g.items?.() ?? [],
        me: me && { pos: me.pos.map(round), vel: me.vel.map(round), yaw: round(me.yaw), pitch: round(me.pitch), crouched: me.crouched, onGround: me.onGround, jumps: me.jumps || 0 },
        scoped: g.arsenal?.scoped, screen: h.screen, console: !!h.console, scoreboard: !!h.scoreboard,
        hud: { hp: h.hp, ammo: h.ammo, reserve: h.reserve, weapon: h.weapon, money: h.money, ping: h.ping, connected: h.connected },
        players: g.remotes.sample(performance.now()).map((r) => ({ id: r.id, name: r.name, team: r.team, pos: r.p.map(round), crouched: !!r.c })),
        feed: h.feed.map((f) => f.text),
      };
    },
  };
  window.__cs = api;
}

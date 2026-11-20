// window.__cs: the test harness's handle on a client (cmd/sim drives it over CDP).
// Installed only when the server runs with -test AND the page has ?test=1, so
// it never exists in prod. Every call returns plain JSON.
export function installTestApi(g) {
  // Let Preact render. A timer, not rAF: background headless tabs throttle rAF.
  const nextFrame = () => new Promise((r) => setTimeout(r, 30));
  const round = (v) => Math.round(v * 1000) / 1000;

  const api = {
    // Join the match and wait for the welcome. The harness can't take pointer
    // lock (no user gesture), so play is entered directly and aiming faked.
    async join(name = "bot") {
      g.play(name);
      for (let i = 0; i < 200 && !g.me; i++) await new Promise((r) => setTimeout(r, 25));
      if (!g.me) throw new Error("no welcome from server");
      g.hud.set({ screen: "playing", aiming: true, name });
      api.step(1);
      return { id: g.myId };
    },
    // Advance the simulation by n frames of dt seconds (fixed step) and render.
    step(n = 1, dt = 1 / 60) {
      for (let i = 0; i < n; i++) g.frame(performance.now(), dt);
      return api.state();
    },
    // Teleport / turn / crouch the local player (the -test server allows teleports).
    set({ pos, yaw, pitch, crouch } = {}) {
      const me = g.me;
      if (!me) throw new Error("not joined");
      if (pos) { me.pos = [...pos]; me.vel = [0, 0, 0]; }
      if (yaw !== undefined) me.yaw = yaw;
      if (pitch !== undefined) me.pitch = pitch;
      if (crouch !== undefined) { me.crouched = !!crouch; me.h = crouch ? 1.2 : 1.8; me.eyeH = crouch ? 1.05 : 1.62; }
      g.net.send({ t: "state", p: me.pos, yaw: me.yaw, pitch: me.pitch, c: me.crouched });
      return api.state();
    },
    // Hold exactly these keys (event.code names) until changed; fire = mouse button.
    hold(keys = [], fire = false) { g.input.setHeld(keys); g.input.fire = fire; return true; },
    // Show a UI state: play | menu | options | console | scoreboard
    async ui(name) {
      if (g.con.open && name !== "console") g.con.toggle(false);
      g.hud.set({ scoreboard: false });
      if (name === "play") g.hud.set({ screen: "playing" });
      else if (name === "menu" || name === "options") g.hud.set({ screen: "ingame", menuOpenedAt: 0 });
      else if (name === "console") g.con.toggle(true);
      else if (name === "scoreboard") g.hud.set({ scoreboard: true });
      else throw new Error(`unknown ui state ${name}`);
      await nextFrame();
      if (name === "options") {
        [...document.querySelectorAll("button")].find((b) => b.textContent.trim() === "OPTIONS")?.click();
        await nextFrame();
      }
      return true;
    },
    exec(line) { g.con.exec(line); return true; },
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
        id: g.myId,
        me: me && { pos: me.pos.map(round), vel: me.vel.map(round), yaw: round(me.yaw), pitch: round(me.pitch), crouched: me.crouched, onGround: me.onGround },
        screen: h.screen, console: !!h.console, scoreboard: !!h.scoreboard,
        hud: { hp: h.hp, ammo: g.gun.ammo, reserve: g.gun.reserve, money: h.money, ping: h.ping, connected: h.connected },
        players: g.remotes.sample(performance.now()).map((r) => ({ id: r.id, name: r.name, team: r.team, pos: r.p.map(round), crouched: !!r.c })),
        feed: h.feed.map((f) => f.text),
      };
    },
  };
  window.__cs = api;
}

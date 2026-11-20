import { createRenderer, hexRgb } from "./gl.js";
import { createPhysics, newBody, EYE, R, H } from "./physics.js";
import { createInput } from "./input.js";
import { createNet, createRemotes } from "./net.js";
import { mountHud } from "./hud.js";
import { basis } from "./math.js";
import * as sfx from "./audio.js";
import { settings, radPerCount } from "./settings.js";
import { createLoop, createOverlay } from "./loop.js";

const SEND_HZ = 30, FIRE_INTERVAL = 0.1, MAG = 30, RELOAD_TIME = 1.6, ROUND = 115;

const canvas = document.getElementById("gl");
const tagLayer = document.createElement("div");
tagLayer.className = "tags";
document.body.appendChild(tagLayer);

const gfx = createRenderer(canvas);
const phys = createPhysics();
const input = createInput(canvas, () => settings.get().m_rawinput);
const remotes = createRemotes();

let map = null, me = null, myId = 0, myColor = "#fff", wantPlay = false;
const known = new Map(); // id -> name, for join/leave feed
const remoteFx = new Map(); // id -> {flash, phase, last, stride}
const puffs = [];
const gun = { ammo: MAG, reserve: 90, reloadLeft: 0, nextShot: 0, kick: 0, flash: 0, bob: 0, clicked: false };

const hud = mountHud(document.getElementById("hud"), {
  play(name) {
    wantPlay = true;
    sfx.initAudio();
    input.lock();
    if (!net.open) net.connect(name);
    else net.send({ t: "hello", name });
  },
  resume() { input.lock(); },
  loop: () => loop,
});

const net = createNet((m) => {
  const now = performance.now();
  switch (m.t) {
    case "open": hud.set({ connected: true }); break;
    case "close":
      hud.set({ connected: false });
      remotes.clear(); known.clear();
      break;
    case "welcome":
      myId = m.id; myColor = m.color; map = m.map;
      gfx.setMap(map); phys.setMap(map);
      me = newBody(m.spawn);
      me.yaw = Math.atan2(m.spawn[0], m.spawn[2]); // face the middle
      hud.set({ myId, myColor, map: map.name });
      break;
    case "snap": {
      remotes.ingest(m, myId, now);
      const ids = new Set();
      for (const p of m.players) {
        if (p.id === myId) continue;
        ids.add(p.id);
        if (!known.has(p.id)) { hud.feed(`${p.name} slid into the server`); sfx.honk(); }
        known.set(p.id, p.name);
      }
      for (const [id, name] of known) if (!ids.has(id)) { known.delete(id); hud.feed(`${name} rage-quit`); }
      const players = m.players.filter((p) => p.id !== myId).map(({ id, name, color }) => ({ id, name, color }));
      if (JSON.stringify(players) !== JSON.stringify(hud.get().players)) hud.set({ players });
      break;
    }
    case "correct":
      if (me) { me.pos = [...m.p]; me.vel = [0, 0, 0]; }
      break;
    case "shot": {
      const fx = fxFor(m.id);
      fx.flash = 0.06;
      if (fx.last) sfx.pew(spatial(fx.last));
      break;
    }
  }
});

input.onLock = (locked) => {
  if (locked) hud.set({ screen: "playing" });
  else if (wantPlay) hud.set({ screen: "paused", scoreboard: false });
};
input.onKey = (code, down) => {
  if (code === "Tab") hud.set({ scoreboard: down });
  if (code === "KeyR" && down) startReload();
};

fetch("/map.json").then((r) => r.json()).then((m) => { if (!map) { map = m; gfx.setMap(m); phys.setMap(m); } });

setInterval(() => {
  hud.set({ ping: net.ping, roundLeft: ROUND - 1 - Math.floor(Date.now() / 1000) % ROUND });
  if (hud.get().roundLeft === 0) hud.feed("Round draw. Everyone smells.");
}, 1000);
setInterval(() => {
  if (me && net.open) net.send({ t: "state", p: me.pos.map(round3), yaw: round3(me.yaw), pitch: round3(me.pitch) });
}, 1000 / SEND_HZ);

function fxFor(id) {
  let fx = remoteFx.get(id);
  if (!fx) remoteFx.set(id, (fx = { flash: 0, phase: 0, last: null, stride: 0, speed: 0 }));
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

function startReload() {
  if (gun.reloadLeft > 0 || gun.ammo === MAG) return;
  if (gun.reserve === 0) gun.reserve = 90; // the slop is infinite
  gun.reloadLeft = RELOAD_TIME;
  sfx.reload(RELOAD_TIME);
  hud.set({ reloading: true });
}

function shoot(now) {
  const sp = Math.hypot(me.vel[0], me.vel[2]);
  const spread = 0.004 + sp * 0.004 + (me.onGround ? 0 : 0.05);
  const b = basis(me.yaw + (Math.random() - 0.5) * spread, me.pitch + (Math.random() - 0.5) * spread);
  const eye = eyePos();
  let hit = phys.raycast(eye, b.fwd), color = [1, 0.95, 0.6];
  for (const r of remotes.sample(now)) {
    const t = rayBox(eye, b.fwd, [r.p[0] - R, r.p[1], r.p[2] - R], [r.p[0] + R, r.p[1] + H, r.p[2] + R]);
    if (t !== null && t < (hit?.t ?? Infinity)) { hit = { t, n: [0, 0, 0] }; color = hexRgb(r.color); sfx.bonk(); }
  }
  if (hit) {
    const p = eye.map((v, i) => v + b.fwd[i] * hit.t + hit.n[i] * 0.05);
    for (let i = 0; i < 4; i++) {
      puffs.push({ p: [...p], v: [hit.n[0] + rs(), hit.n[1] + rs() + 0.5, hit.n[2] + rs()], life: 0.4, size: 0.12, color });
    }
  }
  gun.ammo--; gun.kick = 1; gun.flash = 0.05;
  me.pitch = Math.min(1.55, me.pitch + 0.012);
  sfx.pew();
  net.send({ t: "shot" });
  hud.set({ ammo: gun.ammo });
}

const rs = () => (Math.random() - 0.5) * 2;
const round3 = (v) => Math.round(v * 1000) / 1000;
const eyePos = () => [me.pos[0], me.pos[1] + EYE + me.eyeOff, me.pos[2]];

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

let last = performance.now();
function frame(now) {
  const dt = Math.min(0.1, (now - last) / 1000);
  last = now;
  const time = now / 1000;
  let eye, yaw, pitch, viewmodel = null;

  if (me && map) {
    const [mx, my] = input.takeMouse();
    const rpc = radPerCount(), inv = settings.get().m_invert_y ? -1 : 1;
    me.yaw -= mx * rpc;
    me.pitch = Math.max(-1.55, Math.min(1.55, me.pitch - my * rpc * inv));
    const mv = input.locked ? input.move() : { f: 0, r: 0, walk: false, jump: false };
    const ev = phys.step(me, mv, dt);
    if (ev.jumped) sfx.jump();
    if (ev.landed) sfx.land(ev.landed);
    for (let i = 0; i < ev.steps; i++) sfx.footstep();
    if (me.pos[1] < -20) { me.pos = [...map.spawns[myId % map.spawns.length]]; me.vel = [0, 0, 0]; }

    // gun
    if (gun.reloadLeft > 0) {
      gun.reloadLeft -= dt;
      if (gun.reloadLeft <= 0) {
        const take = Math.min(MAG - gun.ammo, gun.reserve);
        gun.ammo += take; gun.reserve -= take; gun.reloadLeft = 0;
        hud.set({ ammo: gun.ammo, reserve: gun.reserve, reloading: false });
      }
    }
    if (input.fire && gun.reloadLeft === 0) {
      if (gun.ammo > 0) {
        if (time >= gun.nextShot) { shoot(now); gun.nextShot = time + FIRE_INTERVAL; }
      } else if (!gun.clicked) { sfx.click(); gun.clicked = true; startReload(); }
    }
    if (!input.fire) gun.clicked = false;
    gun.kick *= Math.exp(-dt * 12);
    gun.flash = Math.max(0, gun.flash - dt);

    const sp = Math.hypot(me.vel[0], me.vel[2]);
    const k = me.onGround ? Math.min(1, sp / 7) : 0.2;
    gun.bob += dt * sp * 1.4;
    viewmodel = {
      bobX: Math.sin(gun.bob) * 0.014 * k, bobY: -Math.abs(Math.cos(gun.bob)) * 0.018 * k,
      kick: gun.kick, flash: gun.flash, reload: gun.reloadLeft > 0 ? 1 - gun.reloadLeft / RELOAD_TIME : 0,
    };
    eye = eyePos(); yaw = me.yaw; pitch = me.pitch;
    const gap = 6 + sp * 2.2 + (me.onGround ? 0 : 14) + gun.kick * 8;
    document.documentElement.style.setProperty("--gap", gap.toFixed(1) + "px");
  } else {
    // attract mode: slow orbit over the map
    const a = time * 0.08;
    eye = [Math.sin(a) * 34, 16, Math.cos(a) * 34]; yaw = a; pitch = -0.38;
  }

  // remote players
  const players = remotes.sample(now).map((r) => {
    const fx = fxFor(r.id);
    if (fx.last) {
      const d = Math.hypot(r.p[0] - fx.last[0], r.p[2] - fx.last[2]);
      fx.speed = fx.speed * 0.8 + (dt > 0 ? d / dt : 0) * 0.2;
      fx.phase += d * 3;
      if (fx.speed > 4.2) { fx.stride += d; if (fx.stride > 2.2) { fx.stride = 0; sfx.footstep(spatial(r.p)); } }
    }
    fx.last = r.p;
    fx.flash = Math.max(0, fx.flash - dt);
    return { ...r, rgb: hexRgb(r.color), moving: fx.speed > 0.5, phase: fx.phase, flash: fx.flash };
  });

  for (let i = puffs.length - 1; i >= 0; i--) {
    const p = puffs[i];
    p.life -= dt;
    if (p.life <= 0) { puffs.splice(i, 1); continue; }
    for (let a = 0; a < 3; a++) p.p[a] += p.v[a] * dt * 2;
    p.v[1] -= dt * 3;
    p.size = 0.14 * (p.life / 0.4);
  }

  gfx.frame({ eye, yaw, pitch, time, players, puffs, viewmodel });
  updateTags(players);
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
    el.style.color = p.color;
    const s = gfx.project([p.p[0], p.p[1] + 2.15, p.p[2]]);
    if (!s || s[2] > 60) { el.style.display = "none"; continue; }
    el.style.display = "";
    el.style.transform = `translate(${s[0].toFixed(0)}px, ${s[1].toFixed(0)}px) translate(-50%, -100%)`;
  }
}

const loop = createLoop(frame);
createOverlay(loop, () => net.ping);
sfx.setVolume(settings.get().volume);
settings.subscribe((s) => sfx.setVolume(s.volume));

// Debug handle for the console / automated checks.
window.cs = { get me() { return me; }, remotes, net };

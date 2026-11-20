// The local player's weapons: slots, switching, fire timing, reload, recoil,
// scope. Stats come from the registry (assets/weapons); the server owns the
// inventory and validates every shot with the same numbers, so this is just
// prediction: the client fires at once and the server agrees (or ignores it).
import { WEAPONS, SLOTS, FIRE_SOUND } from "../assets/weapons/index.js";
import * as sfx from "../assets/sounds/index.js";

const RAISE_TIME = 0.35;

export function createArsenal({ net, hud, zoomMode = () => "toggle" }) {
  const a = {
    slots: {}, cur: "primary", last: "sidearm",
    nextShot: 0, reloadLeft: 0, reloadTime: 0, kick: 0, flash: 0, bob: 0, raise: 1, swing: 0,
    clicked: false, scoped: false, burst: 0,
    clock: 0, // sim seconds: advanced only by tick(), so fire timing is render-rate independent
    zoomWant: false, // hold mode: zoom button held; toggle mode: toggled on
    cycleLeft: 0, cycleTime: 0, // bolt-action cycling after a shot
  };

  const held = () => a.slots[a.cur];
  a.weapon = () => WEAPONS[held()?.id];
  const show = () => {
    const h = held(), w = a.weapon();
    hud.set({
      ammo: w?.mag ? h.ammo : "∞", reserve: w?.mag ? h.reserve : "", reloading: a.reloadLeft > 0,
      weapon: w?.name || "", scoped: a.scoped && (w?.scope?.overlay || "lens"),
      slots: SLOTS.map((s, i) => ({ key: i + 1, slot: s, name: WEAPONS[a.slots[s]?.id]?.name || "", cur: s === a.cur })),
    });
  };

  // Server's view of the inventory (join, switch, drop, pickup). It's authoritative.
  a.setInv = (inv) => {
    const changed = inv.cur !== a.cur || a.slots[a.cur]?.id !== inv.slots[inv.cur]?.id;
    a.slots = inv.slots;
    if (changed) { a.cur = inv.cur; deployed(); }
    show();
  };

  function deployed() {
    a.reloadLeft = 0; a.raise = 0; a.scoped = false; a.burst = 0; a.cycleLeft = 0;
    if (zoomMode() === "toggle") a.zoomWant = false;
    a.nextShot = Math.max(a.nextShot, a.clock + RAISE_TIME * 0.6);
    sfx.deploy();
  }

  a.switchTo = (slot) => {
    if (!a.slots[slot] || slot === a.cur) return;
    a.last = a.cur; a.cur = slot;
    deployed();
    net.send({ t: "switch", slot });
    show();
  };
  a.cycle = (dir) => {
    const have = SLOTS.filter((s) => a.slots[s]);
    a.switchTo(have[(have.indexOf(a.cur) + dir + have.length) % have.length]);
  };
  a.lastWeapon = () => a.switchTo(a.last);
  a.drop = () => { if (a.cur !== "melee" && held()) net.send({ t: "drop" }); };

  a.reload = () => {
    const h = held(), w = a.weapon();
    if (!w?.mag || a.reloadLeft > 0 || h.ammo >= w.mag || h.reserve <= 0) return;
    a.reloadLeft = a.reloadTime = w.reload;
    a.scoped = false;
    sfx.reload(w.reload);
    net.send({ t: "reload" });
    show();
  };

  // Zoom button. Hold mode: zoom while held (zoomHeld in tick). Toggle mode: each press flips it.
  a.toggleScope = () => {
    if (!a.weapon()?.scope) return;
    a.zoomWant = !a.zoomWant;
    updateScope();
  };
  // Scoped = wanted, and the weapon can be looked through right now (not reloading
  // or cycling a bolt). So a bolt-action re-zooms by itself after cycling while
  // the zoom is still held (or toggled on).
  function updateScope() {
    const s = a.zoomWant && !!a.weapon()?.scope && a.reloadLeft === 0 && a.cycleLeft === 0 && a.raise >= 1;
    if (s !== a.scoped) { a.scoped = s; show(); }
  }

  // Spread in radians for the current weapon and movement.
  a.spread = (me) => {
    const s = a.weapon()?.spread;
    if (!s) return 0;
    const sp = Math.hypot(me.vel[0], me.vel[2]);
    let r = s.base + sp * s.move + (me.onGround ? 0 : s.air);
    if (s.unscoped && !a.scoped) r += s.unscoped;
    return r * (me.crouched ? s.crouch : 1);
  };

  // One sim tick (fixed dt): advance timers, call shoot(weapon) when a shot
  // goes off. Returns viewmodel params (the renderer interpolates between ticks).
  a.tick = (dt, firing, me, shoot, zoomHeld = false) => {
    a.clock += dt;
    const t = a.clock, h = held(), w = a.weapon();
    if (zoomMode() === "hold") a.zoomWant = zoomHeld;
    a.raise = Math.min(1, a.raise + dt / RAISE_TIME);
    if (a.cycleLeft > 0) a.cycleLeft = Math.max(0, a.cycleLeft - dt);
    if (a.reloadLeft > 0) {
      a.reloadLeft -= dt;
      if (a.reloadLeft <= 0) {
        const take = Math.min(w.mag - h.ammo, h.reserve);
        h.ammo += take; h.reserve -= take; a.reloadLeft = 0;
        show();
      }
    }
    if (w && firing && a.reloadLeft === 0 && a.cycleLeft === 0 && (w.action === "auto" || !a.clicked)) {
      if (w.mag && h.ammo <= 0) {
        if (!a.clicked) { sfx.click(); a.reload(); }
      } else if (t >= a.nextShot) {
        a.nextShot = t + w.interval;
        if (w.mag) h.ammo--;
        a.kick = w.slot === "melee" ? 0 : (me.crouched ? 0.6 : 1);
        a.flash = w.slot === "melee" ? 0 : 0.05;
        if (w.slot === "melee") a.swing = 1e-3;
        a.burst++;
        shoot(w);
        (FIRE_SOUND[w.id] || sfx.pew)();
        if (w.action === "bolt") { // unscope, work the bolt, re-zoom if still wanted
          a.cycleLeft = a.cycleTime = w.cycle || w.interval;
          sfx.boltCycle(a.cycleTime);
        }
        show();
      }
      a.clicked = true;
    }
    if (!firing) { a.clicked = false; a.burst = 0; }
    updateScope();
    a.kick *= Math.exp(-dt * 12);
    a.flash = Math.max(0, a.flash - dt);
    if (a.swing > 0) { a.swing += dt / Math.max(0.2, w?.interval || 0.4); if (a.swing >= 1) a.swing = 0; }

    const sp = Math.hypot(me.vel[0], me.vel[2]);
    const k = me.onGround ? Math.min(1, sp / 7) : 0.2;
    a.bob += dt * sp * 1.4;
    return {
      id: h?.id, scoped: a.scoped,
      bobX: Math.sin(a.bob) * 0.014 * k, bobY: -Math.abs(Math.cos(a.bob)) * 0.018 * k,
      kick: a.kick, flash: a.flash, raise: a.raise, swing: a.swing,
      reload: a.reloadLeft > 0 ? 1 - a.reloadLeft / a.reloadTime : 0,
      bolt: a.cycleLeft > 0 ? 1 - a.cycleLeft / a.cycleTime : 0,
    };
  };

  // Recoil after a shot: kick up, plus sideways wander. The Prompt's sideways
  // drift is a slow sine with jitter: it "hallucinates" off target.
  a.recoil = (me, w) => {
    const c = me.crouched ? 0.5 : 1;
    me.pitch = Math.min(1.55, me.pitch + w.recoil.kick * c);
    let side = (Math.random() - 0.5) * 2 * w.recoil.side;
    if (w.id === "prompt") side += Math.sin(a.burst * 0.7) * w.recoil.side * 0.8;
    me.yaw += side * c;
  };

  show();
  return a;
}

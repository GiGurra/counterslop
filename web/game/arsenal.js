// The local player's weapons: slots, switching, fire timing, reload, recoil,
// scope. Stats come from the registry (assets/weapons). This is prediction of
// the server's weapon logic (internal/game weapons.go fire/reload/switchTo):
// the same rules in the same order on the same clock (command ticks, integer),
// so every predicted shot is one the server fires too. Switches and reloads
// travel inside the tick's command and take effect in that tick on both sides.
import { WEAPONS, SLOTS, FIRE_SOUND } from "../assets/weapons/index.js";
import econ from "../assets/economy.json" with { type: "json" };
const DRINK_SPEED = econ.gear.find((g) => g.id === "drink")?.speed || 1;
// The grenade slots (weapons whose action is "throw"): key 4 cycles them.
const SLOTS_NADE = new Set(Object.values(WEAPONS).filter((w) => w.action === "throw").map((w) => w.slot));
import * as sounds from "../assets/sounds/index.js";
import { mulberry32 } from "../gen/rng.js";
import { RUN } from "../engine/physics.js";

const ACCURATE_FRAC = 0.34; // = internal/game accurateFrac

const RAISE_TIME = 0.35;
const DEPLOY_TICKS = 21; // = internal/game DeployTicks (RAISE_TIME * 0.6 at 100 Hz)
const HZ = 100;
// Seconds to whole ticks, rounding up: = internal/game Ticks.
export const ticks = (sec) => Math.ceil(sec * HZ - 1e-6);

// [yaw, pitch] of every ray of one shot: one, or `pellets` (shotguns), each
// offset by its own pair of draws from the shot's seeded RNG. = internal/game ShotAngles.
export function shotAngles(yaw, pitch, spread, seed, pellets = 0) {
  const r = mulberry32(seed >>> 0), out = [];
  for (let i = 0; i < Math.max(1, pellets || 0); i++) {
    const dy = (r() - 0.5) * spread, dp = (r() - 0.5) * spread;
    out.push([yaw + dy, pitch + dp]);
  }
  return out;
}

export function createArsenal({ net, hud, zoomMode = () => "toggle" }) {
  const a = {
    slots: {}, cur: "primary", last: "sidearm",
    // Timers in command ticks (as the server's Inventory): tickN counts ticks.
    tickN: 0, nextShot: 0, cycleEnd: 0, reloadEnd: 0, boostEnd: 0,
    reloadLeft: 0, reloadTime: 0, kick: 0, flash: 0, bob: 0, raise: 1, swing: 0,
    clicked: false, scoped: false, burst: 0,
    burstLeft: 0, // burst weapons: rounds of the current burst still to fire (= Inventory.burstLeft)
    zoomWant: false, // hold mode: zoom button held; toggle mode: toggled on
    cycleLeft: 0, cycleTime: 0, // bolt-action cycling after a shot (seconds, for the animation)
    pendingSwitch: null, pendingReload: false, // go out in the next command
  };

  // Replays (reconciliation) run the real rules with a.quiet set: no sounds, no HUD.
  a.quiet = false;
  const sfx = new Proxy(sounds, { get: (t, k) => (a.quiet ? () => {} : t[k]) });
  const fireSound = (id) => (a.quiet ? () => {} : FIRE_SOUND[id] || sounds.pew);

  const held = () => a.slots[a.cur];
  a.weapon = () => WEAPONS[held()?.id];
  const show = () => {
    if (a.quiet) return;
    const h = held(), w = a.weapon();
    hud.set({
      ammo: w?.mag ? h.ammo : "∞", reserve: w?.mag ? h.reserve : "", reloading: !!a.reloadEnd,
      weapon: w?.name || "", scoped: a.scoped && (w?.scope?.overlay || "lens"),
      slots: hudSlots(),
    });
  };

  // Server's view of the inventory (join, switch, drop, pickup). It's authoritative.
  // redraw: deploy even if the held weapon didn't change (the server drew it again).
  a.setInv = (inv, redraw = false) => {
    const changed = redraw || inv.cur !== a.cur || a.slots[a.cur]?.id !== inv.slots[inv.cur]?.id;
    a.slots = inv.slots;
    if (changed) { a.cur = inv.cur; deployed(); }
    show();
  };

  // Keys 1-3 are the gun slots; key 4 is every grenade you carry (CS), one row.
  function hudSlots() {
    const isNade = (s) => WEAPONS[a.slots[s]?.id]?.action === "throw";
    const guns = SLOTS.filter((s) => !SLOTS_NADE.has(s));
    const rows = guns.map((s, i) => ({ key: i + 1, slot: s, id: a.slots[s]?.id, name: WEAPONS[a.slots[s]?.id]?.name || "", cur: s === a.cur }));
    const nades = SLOTS.filter(isNade);
    if (nades.length) {
      const name = nades.map((s) => WEAPONS[a.slots[s].id].name + (a.slots[s].ammo > 1 ? ` ×${a.slots[s].ammo}` : "")).join(", ");
      rows.push({ key: 4, slot: "grenade", name, cur: nades.includes(a.cur), nades: Object.fromEntries(nades.map((s) => [a.slots[s].id, a.slots[s].ammo])) });
    }
    return rows;
  }
  function deployed() { // = Inventory.drawn
    a.reloadEnd = 0; a.cycleEnd = 0; a.reloadLeft = 0; a.cycleLeft = 0;
    a.raise = 0; a.scoped = false; a.burst = 0; a.burstLeft = 0;
    if (zoomMode() === "toggle") a.zoomWant = false;
    a.nextShot = Math.max(a.nextShot, a.tickN + DEPLOY_TICKS);
    sfx.deploy();
  }

  // The weapon state the server reconciles (= internal/game weaponState): slots,
  // current slot, timers in ticks from now, trigger.
  a.wstate = () => {
    const slots = {};
    for (const k of Object.keys(a.slots).sort()) { const h = a.slots[k]; if (h) slots[k] = { id: h.id, ammo: h.ammo, reserve: h.reserve }; }
    return {
      slots, cur: a.cur, ns: Math.max(0, a.nextShot - a.tickN), ce: Math.max(0, a.cycleEnd - a.tickN),
      re: a.reloadEnd ? Math.max(0, a.reloadEnd - a.tickN) : 0, trig: a.clicked, bl: a.burstLeft, ...(a.boostEnd > a.tickN ? { bo: a.boostEnd - a.tickN } : {}),
    };
  };
  // Adopt the server's weapon state as of tick `tick` (our tickN at its ack).
  a.setWstate = (w, tick) => {
    const switched = w.cur !== a.cur;
    a.slots = Object.fromEntries(Object.entries(w.slots || {}).map(([k, h]) => [k, h && { ...h }]));
    a.cur = w.cur; a.tickN = tick;
    a.nextShot = tick + w.ns; a.cycleEnd = tick + w.ce; a.reloadEnd = w.re ? tick + w.re : 0; a.clicked = !!w.trig;
    a.burstLeft = w.bl || 0;
    a.boostEnd = tick + (w.bo || 0);
    if (switched && !a.quiet) a.raise = 0;
  };
  a.show = show;

  // Respawn: a fresh life has no reload or bolt in progress (server: new loadout).
  a.respawned = () => { a.reloadEnd = 0; a.cycleEnd = 0; a.reloadLeft = 0; a.cycleLeft = 0; a.pendingReload = false; show(); };

  // Input asks; the next command carries it; tick() applies it (as the server).
  const wanted = () => a.pendingSwitch ?? a.cur;
  a.switchTo = (slot) => {
    if (!a.slots[slot] || slot === wanted()) return;
    a.last = wanted();
    a.pendingSwitch = slot;
  };
  // CS key 4: the next grenade you carry (he, flash, smoke), wrapping around.
  a.nextGrenade = () => {
    const have = SLOTS.filter((s) => WEAPONS[a.slots[s]?.id]?.action === "throw");
    if (have.length) a.switchTo(have[(have.indexOf(wanted()) + 1) % have.length]);
  };
  a.cycle = (dir) => {
    const have = SLOTS.filter((s) => a.slots[s]);
    a.switchTo(have[(have.indexOf(wanted()) + dir + have.length) % have.length]);
  };
  // What the next command carries: {w: slot to switch to, reload: bool}.
  a.takeCmd = () => {
    const c = { w: a.pendingSwitch, reload: a.pendingReload };
    a.pendingSwitch = null; a.pendingReload = false;
    return c;
  };
  a.lastWeapon = () => a.switchTo(a.last);
  a.drop = () => { if (a.cur !== "melee" && held()) net.send({ t: "drop" }); };

  a.reload = () => { a.pendingReload = true; };
  // = Registry.settle: a finished reload fills the mag.
  function settle() {
    if (!a.reloadEnd || a.tickN < a.reloadEnd) return;
    a.reloadEnd = 0;
    const h = held(), w = a.weapon();
    if (!h) return;
    const take = Math.min(w.mag - h.ammo, h.reserve);
    h.ammo += take; h.reserve -= take;
    show();
  }
  // = Registry.reload
  function startReload() {
    settle();
    const h = held(), w = a.weapon();
    if (!h || a.reloadEnd || !w?.mag || h.ammo >= w.mag || h.reserve <= 0) return;
    a.reloadEnd = a.tickN + ticks(w.reload);
    a.reloadTime = w.reload;
    a.scoped = false;
    sfx.reload(w.reload);
    show();
  }

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
    const s = a.zoomWant && !!a.weapon()?.scope && !a.reloadEnd && a.tickN >= a.cycleEnd && a.raise >= 1;
    if (s !== a.scoped) { a.scoped = s; show(); }
  }

  // Spread in radians for the current weapon and movement.
  a.noSpread = false; // test harness: exact shots
  a.spread = (me) => {
    const s = a.weapon()?.spread;
    if (!s || a.noSpread) return 0;
    // Movement counts above 34% of the weapon's max speed only (= Go movePenalty):
    // stop before you shoot (counter-strafe) and the shot is accurate.
    const over = Math.max(0, Math.hypot(me.vel[0], me.vel[2]) - ACCURATE_FRAC * RUN * (a.weapon()?.speed || 1));
    let r = s.base + over * s.move + (me.onGround ? 0 : s.air);
    if (s.unscoped && !a.scoped) r += s.unscoped;
    const w = a.weapon(), h = held();
    if (s.belt && w.mag && h) r += s.belt * (1 - h.ammo / w.mag); // after this shot's round left the belt
    return r * (me.crouched ? s.crouch : 1);
  };

  // One sim tick (fixed dt): advance timers, call shoot(weapon) when a shot
  // goes off. Returns viewmodel params (the renderer interpolates between ticks).
  // `cmd` is what this tick's command carried ({w, reload}); the order of
  // switch, reload, release, fire is the server's runCmd.
  // Max move speed scale for the next command (= server heldSpeed): the weapon
  // in hand, times the energy drink's while it lasts (command tickN + 1 <= boostEnd).
  a.moveSpeed = () => (a.weapon()?.speed || 1) * (a.tickN + 1 <= a.boostEnd ? DRINK_SPEED : 1);
  a.tick = (dt, firing, me, shoot, zoomHeld = false, cmd = {}) => {
    const T = ++a.tickN;
    if (cmd.w && a.slots[cmd.w] && cmd.w !== a.cur) { a.cur = cmd.w; deployed(); show(); }
    if (cmd.reload) startReload();
    settle();
    const h = held(), w = a.weapon();
    if (zoomMode() === "hold") a.zoomWant = zoomHeld;
    a.raise = Math.min(1, a.raise + dt / RAISE_TIME);
    a.cycleLeft = Math.max(0, a.cycleEnd - T) / HZ;
    a.reloadLeft = a.reloadEnd ? Math.max(0, a.reloadEnd - T) / HZ : 0;
    if (!firing) { a.clicked = false; a.burst = 0; }
    // One shot goes off: ammo, kick, sounds, shoot(w). = the server's fire.
    const fireOne = () => {
      if (w.mag) h.ammo--;
      a.kick = w.slot === "melee" ? 0 : (me.crouched ? 0.6 : 1);
      a.flash = w.slot === "melee" ? 0 : 0.05;
      if (w.slot === "melee") a.swing = 1e-3;
      a.burst++;
      if (w.action === "throw") { // = Registry.fire: the last grenade leaves the slot empty
        a.flash = 0; a.swing = 1e-3;
        fireSound(w.id)();
        if (h.ammo <= 0) {
          delete a.slots[a.cur];
          a.cur = SLOTS.find((s) => a.slots[s]) || a.cur;
          deployed();
        }
        return;
      }
      shoot(w);
      fireSound(w.id)();
    };
    // = Registry.trigger / fire, in the same order.
    if (w && (firing || a.burstLeft > 0)) {
      if (a.reloadEnd) a.burstLeft = 0;
      else if (T < a.cycleEnd) { /* working the bolt */ }
      else if (a.burstLeft > 0) { // the rest of a burst, trigger or not
        if (T >= a.nextShot) {
          if (w.mag && h.ammo <= 0) a.burstLeft = 0;
          else {
            a.burstLeft--;
            a.nextShot = T + ticks(a.burstLeft > 0 ? w.burstGap : w.interval);
            fireOne();
            show();
          }
        }
      } else if (w.action === "auto" || !a.clicked) {
        const wasClicked = a.clicked;
        a.clicked = true;
        if (w.mag && h.ammo <= 0) {
          if (!wasClicked) { sfx.click(); a.reload(); } // dry fire: the next command reloads
        } else if (T >= a.nextShot) {
          a.nextShot = T + ticks(w.interval);
          if (w.action === "burst" && w.burst > 1) { a.burstLeft = w.burst - 1; a.nextShot = T + ticks(w.burstGap); }
          fireOne();
          if (w.action === "bolt") { // unscope, work the bolt, re-zoom if still wanted
            a.cycleTime = w.cycle || w.interval;
            a.cycleEnd = T + ticks(a.cycleTime);
            a.cycleLeft = a.cycleTime;
            sfx.boltCycle(a.cycleTime);
          }
          show();
        }
      }
    }
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

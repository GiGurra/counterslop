// The local player's weapons: slots, switching, fire timing, reload, recoil,
// scope. The rules (timers, ammo, reload, burst, bolt, spray index, spread,
// shot angles) are the server's own, internal/game/*_shared.go translated by
// cmd/gojs into shared.gen.js: the same code on the same clock (command ticks),
// so every predicted shot is one the server fires too. This file is the
// presentation around them: sounds, HUD, viewmodel animation, scope, input.
import { WEAPONS, SLOTS, FIRE_SOUND } from "../assets/weapons/index.js";
import weaponsData from "../assets/weapons/weapons.json" with { type: "json" };
import econ from "../assets/economy.json" with { type: "json" };
import * as R from "./shared.gen.js";
const DRINK_SPEED = econ.gear.find((g) => g.id === "drink")?.speed || 1;
const CARRY_SPEED = econ.gpudrop?.carrySpeed || 1; // GPU Drop: carrying a GPU (= Go heldSpeed)
// The grenade slots (weapons whose action is "throw"): key 4 cycles them.
const SLOTS_NADE = new Set(Object.values(WEAPONS).filter((w) => w.action === "throw").map((w) => w.slot));
// The Big Diff's slot (action "plant"): key 5. It never fires; fire held in a site plants it (server rounds.go).
const SLOT_BOMB = "bomb";
import * as sounds from "../assets/sounds/index.js";

// The rules' registry: weapons.json as the server loads it.
const REG = R.NewRegistry(weaponsData.weapons.map(R.Weapon_fromJSON), weaponsData.slots);

const RAISE_TIME = 0.35;
const VIEW_PUNCH = 0.5; // the view shows this part of the spray offset (CS: about half), so shots land above the crosshair
const HZ = 100;
// Seconds to whole command ticks, rounding up (Go Ticks).
export const ticks = (sec) => R.Ticks(sec);

// [yaw, pitch] of every ray of one shot: one, or `pellets` (shotguns), each
// offset by its own pair of draws from the shot's seeded RNG (Go ShotAngles).
export const shotAngles = (yaw, pitch, spread, seed, pellets = 0) => R.ShotAngles(yaw, pitch, spread, seed >>> 0, pellets || 0);

// Spray patterns (Go recoil_shared.go): the spray index at a tick, the
// [up, right] offset at an index, and a shot's [yaw, pitch] with the offset.
export const recoilIndex = (idx, lastShot, tick, interval, recover) => R.RecoilIndex(idx, lastShot, tick, interval, recover);
export const recoilOffset = (pattern, idx) => R.RecoilOffset(pattern, idx);
export const recoilAngles = (yaw, pitch, off, scale) => R.recoilAngles(yaw, pitch, off, scale);

// The Inventory fields the rules own, as arsenal properties (callers and the
// weapon-state sync read and write them by these names).
const INV_FIELDS = { slots: "slots", cur: "cur", nextShot: "nextShot", cycleEnd: "cycleEnd", reloadEnd: "reloadEnd",
  boostEnd: "boostEnd", carry: "carry", burstLeft: "burstLeft", recoil: "recoil", lastShot: "lastShot", shotIdx: "shotIdx", clicked: "trigger", useBack: "useBack", pin: "pin", thrown: "thrown" };

export function createArsenal({ net, hud, zoomMode = () => "toggle" }) {
  // The rules' state (Go Inventory): slots, current slot, timers in command
  // ticks, trigger, burst, spray. tickN counts command ticks.
  const inv = R.Inventory_fromJSON({ slots: {}, cur: "primary" });
  const a = {
    last: "sidearm", tickN: 0,
    reloadLeft: 0, reloadTime: 0, kick: 0, flash: 0, bob: 0, raise: 1, swing: 0, cock: 0, cockLob: false,
    scoped: false, burst: 0,
    zoomWant: false, // hold mode: zoom button held; toggle mode: toggled on
    cycleLeft: 0, cycleTime: 0, // bolt-action cycling after a shot (seconds, for the animation)
    pendingSwitch: null, pendingReload: false, // go out in the next command
  };

  for (const [k, f] of Object.entries(INV_FIELDS)) {
    Object.defineProperty(a, k, { get: () => inv[f], set: (v) => { inv[f] = v; }, enumerable: true });
  }

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
      ammo: w?.mag ? h.ammo : w?.action === "plant" ? "" : "∞", reserve: w?.mag ? h.reserve : "", reloading: !!a.reloadEnd,
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

  // Keys 1-3 are the gun slots; key 4 is every grenade you carry (CS), one row; key 5 The Big Diff.
  function hudSlots() {
    const isNade = (s) => WEAPONS[a.slots[s]?.id]?.action === "throw";
    const guns = SLOTS.filter((s) => !SLOTS_NADE.has(s) && s !== SLOT_BOMB);
    const rows = guns.map((s, i) => ({ key: i + 1, slot: s, id: a.slots[s]?.id, name: WEAPONS[a.slots[s]?.id]?.name || "", reserve: a.slots[s]?.reserve ?? 0, cur: s === a.cur })); // reserve: the buy menu's ammo strip
    const nades = SLOTS.filter(isNade);
    if (nades.length) {
      // One entry per grenade, in the order key 4 cycles them (nextGrenade walks
      // the same SLOTS order); only the one in hand is "cur", not the whole row.
      const items = nades.map((s) => ({ slot: s, id: a.slots[s].id, name: WEAPONS[a.slots[s].id].name, count: a.slots[s].ammo, cur: s === a.cur }));
      const name = items.map((n) => n.name + (n.count > 1 ? ` ×${n.count}` : "")).join(", ");
      rows.push({ key: 4, slot: "grenade", name, cur: nades.includes(a.cur), items, nades: Object.fromEntries(items.map((n) => [n.id, n.count])) });
    }
    if (a.slots[SLOT_BOMB]) rows.push({ key: 5, slot: SLOT_BOMB, id: a.slots[SLOT_BOMB].id, name: WEAPONS[a.slots[SLOT_BOMB].id]?.name || "", cur: a.cur === SLOT_BOMB });
    return rows;
  }
  // A weapon was taken in hand: the rules' timers (Inventory.drawn) and the view.
  function deployed() {
    R.Inventory_drawn(inv, a.tickN);
    deployedFx();
  }
  function deployedFx() {
    a.reloadLeft = 0; a.cycleLeft = 0;
    a.raise = 0; a.scoped = false; a.burst = 0;
    if (zoomMode() === "toggle") a.zoomWant = false;
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
      ...(a.recoil > 0 ? { ri: a.recoil, ls: a.tickN - a.lastShot } : {}),
      ...(a.carry ? { ca: true } : {}),
      ...(a.useBack ? { ub: a.useBack } : {}),
      ...(a.pin ? { pin: a.pin } : {}),
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
    a.carry = !!w.ca; // only the server grabs, drops and secures: the client adopts it
    a.useBack = w.ub || "";
    a.pin = w.pin || 0;
    a.recoil = w.ri || 0; a.lastShot = tick - (w.ls || 0);
    if (switched && !a.quiet) a.raise = 0;
  };
  a.show = show;

  // Respawn: a fresh life has no reload or bolt in progress (server: new loadout).
  a.respawned = () => { a.recoil = 0; a.shotIdx = 0; a.reloadEnd = 0; a.cycleEnd = 0; a.reloadLeft = 0; a.cycleLeft = 0; a.pendingReload = false; show(); };

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
  // G: the weapon in hand (never melee); with nothing droppable in hand, a
  // carried GPU (GPU Drop: the server puts it down in front).
  a.drop = () => { if ((a.cur !== "melee" && held()) || a.carry) net.send({ t: "drop" }); };

  a.reload = () => { a.pendingReload = true; };
  // Registry.settle: a finished reload fills the mag.
  function settle() {
    const was = inv.reloadEnd;
    R.Registry_settle(REG, inv, a.tickN);
    if (was && !inv.reloadEnd) show();
  }
  // Registry.reload (which settles first), and the reload's sound and view.
  function startReload() {
    const was = inv.reloadEnd;
    R.Registry_reload(REG, inv, a.tickN);
    if (inv.reloadEnd === was) return;
    if (inv.reloadEnd) {
      const w = a.weapon();
      a.reloadTime = w.reload;
      a.scoped = false;
      sfx.reload(w.reload);
    }
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
  // Go ShotSpread: movement counts above 34% of the weapon's max speed only,
  // so stop before you shoot (counter-strafe) and the shot is accurate.
  a.spread = (me) => {
    const h = held(), w = h && R.Registry_Get(REG, h.id);
    if (!w || a.noSpread) return 0;
    return R.ShotSpread(w, h, me, a.scoped);
  };

  // One sim tick (fixed dt): advance timers, call shoot(weapon) when a shot
  // goes off. Returns viewmodel params (the renderer interpolates between ticks).
  // `cmd` is what this tick's command carried ({w, reload, frozen, alt: attack2 held}); the order of
  // switch, reload, release, fire is the server's runCmd. frozen: the command
  // runs in round freeze time, which doesn't use up the drink (Go HoldBoost).
  // Max move speed scale for the next command (= server heldSpeed): the weapon
  // in hand, times the energy drink's while it lasts (command tickN + 1 <= boostEnd).
  // Go MoveSpeed (server heldSpeed): the next command (tickN + 1) is boosted while <= boostEnd.
  a.moveSpeed = () => { const h = held(); return R.MoveSpeed(h ? R.Registry_Get(REG, h.id) : null, inv.boostEnd > 0 && a.tickN + 1 <= inv.boostEnd, DRINK_SPEED, inv.carry, CARRY_SPEED); };
  a.tick = (dt, firing, me, shoot, zoomHeld = false, cmd = {}) => {
    const T = ++a.tickN;
    if (cmd.frozen) R.HoldBoost(inv, T);
    // The server's runCmd order: switch, reload, settle, trigger.
    if (cmd.w && R.Registry_switchTo(REG, inv, cmd.w, T)) { deployedFx(); show(); }
    // +use (Go UseDraw): E in a site draws The Big Diff, letting go puts it back. cmd.use: {b, prevB, site, aim}.
    if (cmd.use && R.Registry_UseDraw(REG, inv, cmd.use.b, cmd.use.prevB, cmd.use.site, cmd.use.aim, T)) { deployedFx(); show(); }
    if (cmd.reload) startReload();
    settle();
    const h = held(), w = a.weapon(); // as of this tick's trigger (the swing below uses it)
    if (zoomMode() === "hold") a.zoomWant = zoomHeld;
    a.raise = Math.min(1, a.raise + dt / RAISE_TIME);
    a.cycleLeft = Math.max(0, a.cycleEnd - T) / HZ;
    a.reloadLeft = a.reloadEnd ? Math.max(0, a.reloadEnd - T) / HZ : 0;
    if (!firing) a.burst = 0;
    // Registry.trigger: the shot (or the rest of a burst) goes off, or says why not.
    const wasTrigger = inv.trigger, burstBefore = inv.burstLeft, curBefore = inv.cur;
    const [fired, why] = R.Registry_trigger(REG, inv, T, firing, !!cmd.alt);
    if (fired) {
      const fw = WEAPONS[fired.id];
      a.kick = fw.slot === "melee" ? 0 : (me.crouched ? 0.6 : 1);
      a.flash = fw.slot === "melee" ? 0 : 0.05;
      if (fw.slot === "melee") a.swing = 1e-3;
      a.burst++;
      if (fw.action === "throw") { // the last grenade may have left the slot empty: the rules drew the next
        a.flash = 0; a.swing = 1e-3;
        fireSound(fw.id)();
        if (inv.cur !== curBefore) deployedFx();
      } else {
        shoot(fw);
        fireSound(fw.id)();
        if (fw.action === "bolt") { // unscope, work the bolt, re-zoom if still wanted
          a.cycleTime = fw.cycle || fw.interval;
          a.cycleLeft = a.cycleTime;
          sfx.boltCycle(a.cycleTime);
        }
      }
      show();
    } else if (why === "empty" && burstBefore === 0 && !wasTrigger) {
      sfx.click(); a.reload(); // dry fire: the next command reloads
    }
    updateScope();
    a.kick *= Math.exp(-dt * 12);
    a.flash = Math.max(0, a.flash - dt);
    if (a.swing > 0) { a.swing += dt / Math.max(0.2, w?.interval || 0.4); if (a.swing >= 1) a.swing = 0; }
    // A pulled pin draws the grenade back (over the shoulder, or low for the lob): eased, render only.
    a.cock += ((inv.pin ? 1 : 0) - a.cock) * Math.min(1, dt * 14);
    if (inv.pin) a.cockLob = inv.pin === R.ThrowLob;

    const sp = Math.hypot(me.vel[0], me.vel[2]);
    const k = me.onGround ? Math.min(1, sp / 7) : 0.2;
    a.bob += dt * sp * 1.4;
    return {
      id: h?.id, scoped: a.scoped,
      bobX: Math.sin(a.bob) * 0.014 * k, bobY: -Math.abs(Math.cos(a.bob)) * 0.018 * k,
      kick: a.kick, flash: a.flash, raise: a.raise, swing: a.swing, cock: a.cock, cockLob: a.cockLob,
      reload: a.reloadLeft > 0 ? 1 - a.reloadLeft / a.reloadTime : 0,
      bolt: a.cycleLeft > 0 ? 1 - a.cycleLeft / a.cycleTime : 0,
    };
  };

  // The [up, right] spray offset (times scale) the next shot would get at
  // (fractional) tick t, recovering as the server's index does. Render only.
  a.sprayOffset = (t, scale) => {
    const w = a.weapon();
    if (!w?.recoil?.pattern?.length || !scale) return [0, 0];
    const off = recoilOffset(w.recoil.pattern, recoilIndex(a.recoil, a.lastShot, t, ticks(w.interval), w.recoil.recover || 0));
    return [off[0] * scale, off[1] * scale];
  };
  // The view punch: VIEW_PUNCH of that offset (the camera shows part of it; the
  // crosshair, crosshair.js, sits on the whole of it).
  a.viewPunch = (t, scale) => a.sprayOffset(t, scale).map((v) => v * VIEW_PUNCH);

  show();
  return a;
}

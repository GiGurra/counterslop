// sim/cases: declarative command → simulation → state cases, run at three levels:
//   go   internal/game/cases_test.go: the server's real command path (go test, ms)
//   js   this file via web/game/cases.test.mjs: the client's predictor (node --test, ms)
//   e2e  the real server + a headless client in lockstep (cmd/sim, tagged cases)
// The same expectations at every level catch drift between the twin sims.
//
// A case (sim/cases/<name>.json):
// {
//   "name": "what it checks",
//   "levels": ["go", "js"],            optional; default go + js
//   "map": "gs_kiosk",                 the map it is played on, at every level (default de_slop)
//   "players": {                       sorted by name = join order on the server
//     "alice": { "team": "vibe", "pos": [x, y, z], "yaw": 0, "pitch": 0, "crouch": false,
//                "slots": { "primary": { "id": "prompt", "ammo": 30, "reserve": 90 } },  default: team loadout
//                "cur": "primary", "boost": 1000 }   boost: energy drink for the first n commands
//   },
//   "script": [
//     { "n": 30, "alice": { "f": 1, "r": 0, "yaw": 0.5, "pitch": 0, "b": ["fire", "jump"], "w": "sidearm" } },
//         n ticks (default 1) of one command per tick per player; players not named send
//         an idle command. f/r are the move axes, b the held buttons (jump crouch walk fire
//         scope reload), w a slot to switch to (e2e presses its key; a grenade slot is key 4, the next grenade, so carry only that kind); yaw/pitch persist until changed.
//         Players on one team: keep the teams balanced (the server refuses a side 2+ bigger).
//         "drop": true drops the held weapon (server only: such cases leave out "js").
//     { "expect": { "alice": { "pos": [x, null, z], "ammo": 29, ... }, "world": { "items": 1 } } }
//         checked after the ticks so far. Vectors: [x, y, z] ±0.01 (null skips a
//         component, {"min": a, "max": b} bounds it) or {"v": [...], "eps": e}. Numbers: exact, or {"min": a, "max": b}.
//   ]
// }
//     "mouse": [dx, dy]  e2e: mouse counts this tick through the real mousemove path
//         (sensitivity, m_invert_y, zoom_sensitivity_ratio); the view then stays there.
//         Expect it with "yaw" / "pitch" (the client's view, radians).
//     { "wallMs": 1600 }  e2e: the harness stalls 1.6 s of real time between ticks (a loaded
//         machine; go and js skip it): what a case checks must not depend on wall time.
//     "noBack": 0.0015 on a run step: every tick of it, a player with move input
//         must not move more than this (m) backwards against it (no rubber banding).
//     Also: "act": ["lastinv", "invnext", ...] run-once actions (their bound key in e2e;
//     the arsenal in js; not go), "tap": ["fire", "jump", "use"] buttons pressed and released
//     before the step's first tick, between two commands (e2e taps the key; go and js
//     set the bit in that one command: a press must never be lost), "hold": ["+showscores"] held UI actions (e2e only),
//     "exec": "console line" typed into the console before the tick (e2e only).
// Fields and who knows them: sim/case-fields.json (sim = server + predictor,
// predictor, ui = the real client, server, world: items = dropped weapons on the
// floor, nades = live grenades (flying or a standing smoke), players / bots = how many, "cvar.<name>" = a server cvar, "round.<k>" = the
// round: phase, number, bomb, site, carrier, winner, reason, score.vibe / score.artisan,
// and the match: max, overtime, played, swapped, matchwinner). "points" = score points.
// "use" (b) holds +use: defuse The Big Diff (e2e; the predictor ignores it). Planting is
// "w": "bomb" (slot 5) then "fire" held in a site. Strings also
// match {"contains": "..."} / {"lacks": "..."};
// "binds" matches the listed codes only.
import { createPhysics, newBody, H, H_CROUCH, EYE_CROUCH } from "../engine/physics.js";
import { BTN } from "../engine/cmd.js";
import { createArsenal } from "./arsenal.js";
import { stepLocal } from "./localsim.js";
import { CmdTime } from "./shared.gen.js";

import FIELDS from "../../sim/case-fields.json" with { type: "json" };
export { FIELDS };
const BUTTONS = { jump: BTN.JUMP, crouch: BTN.CROUCH, walk: BTN.WALK, fire: BTN.FIRE, scope: BTN.SCOPE, reload: BTN.RELOAD, use: BTN.USE };
// = internal/game Registry.Loadout, from the weapon data
import WEAPONS from "../assets/weapons/weapons.json" with { type: "json" };
const full = (id) => { const w = WEAPONS.weapons.find((x) => x.id === id); return { id, ammo: w.mag, reserve: w.reserve }; };
const LOADOUT = (team) => ({ primary: full(team === "artisan" ? "linter" : "prompt"), sidearm: full("stub"), melee: full("keyboard") });

// Run a case through the client predictor. Returns {fails, trace}: failures as
// strings, and every player's state after each tick (for cmd/simdiff).
export function runCase(c, map, dt = 0.01) {
  const phys = createPhysics();
  phys.setMap(map);
  const names = Object.keys(c.players).sort();
  const ps = {};
  for (const n of names) {
    const cp = c.players[n];
    const me = newBody([...cp.pos]);
    me.yaw = cp.yaw || 0; me.pitch = cp.pitch || 0;
    if (cp.crouch) { me.crouched = true; me.h = H_CROUCH; me.eyeH = EYE_CROUCH; }
    const arsenal = createArsenal({ net: { send() {} }, hud: { set() {} }, zoomMode: () => "hold" });
    arsenal.setInv({ cur: cp.cur || "primary", slots: structuredClone(cp.slots || LOADOUT(cp.team)) });
    if (cp.boost) arsenal.boostEnd = arsenal.tickN + cp.boost; // energy drink for the first n commands
    ps[n] = { me, arsenal, prevB: cp.crouch ? BTN.CROUCH : 0, yaw: me.yaw, pitch: me.pitch, shots: 0 };
  }
  const fails = [], trace = [];
  let tick = 0;
  // The others as the server's peersOf sees them while n's command runs (join
  // order = name order = ID order; those earlier this tick have moved already).
  const peersOf = (n) => {
    const out = names.filter((o) => o !== n).map((o) => ({ pos: ps[o].me.pos, h: ps[o].me.crouched ? H_CROUCH : H, lo: o < n }));
    return out.length ? out : null;
  };
  c.script.forEach((step, si) => {
    if (step.expect) {
      for (const [who, fields] of Object.entries(step.expect)) {
        for (const [k, want] of Object.entries(fields)) {
          const err = check(ps[who], who, k, want);
          if (err) fails.push(`step ${si} (tick ${tick}) ${who}.${k}: ${err}`);
        }
      }
      return;
    }
    if (step.wallMs !== undefined) return; // e2e only (a stall in real time): no tick here
    for (const k of Object.keys(step)) if (k !== "n" && k !== "noBack" && !ps[k]) throw new Error(`step ${si}: unknown key or player "${k}"`);
    const backed = new Set();
    for (let i = 0; i < (step.n ?? 1); i++) {
      tick++;
      const before = Object.fromEntries(names.map((n) => [n, [...ps[n].me.pos]]));
      for (const n of names) {
        const st = ps[n], inp = step[n] || {};
        if (inp.yaw !== undefined) st.yaw = inp.yaw;
        if (inp.pitch !== undefined) st.pitch = inp.pitch;
        if (inp.drop || inp.exec || inp.hold) throw new Error(`step ${si}: drop/exec/hold need the server or the real client; leave "js" out of this case's levels`);
        for (const act of inp.act || []) {
          const f = { reload: () => st.arsenal.reload(), slot1: () => st.arsenal.switchTo("primary"), slot2: () => st.arsenal.switchTo("sidearm"),
            slot3: () => st.arsenal.switchTo("melee"), slot4: () => st.arsenal.nextGrenade(), slot5: () => st.arsenal.switchTo("bomb"), lastinv: () => st.arsenal.lastWeapon(), invnext: () => st.arsenal.cycle(1), invprev: () => st.arsenal.cycle(-1) }[act];
          if (!f) throw new Error(`step ${si}: act "${act}" is not a predictor action; leave "js" out of this case's levels`);
          if (i === 0) f(); // a key press: once, at the start of the run
        }
        let b = 0;
        // A tap (press and release before the command) reaches exactly the step's first command.
        for (const bn of [...(inp.b || []), ...(i === 0 ? inp.tap || [] : [])]) {
          if (!(bn in BUTTONS)) throw new Error(`step ${si}: unknown button "${bn}"`);
          b |= BUTTONS[bn];
        }
        const wc = st.arsenal.takeCmd(); // what "act" asked for rides in this command, as in the client
        if (wc.reload) b |= BTN.RELOAD;
        const cmd = { seq: tick, f: inp.f || 0, r: inp.r || 0, yaw: st.yaw, pitch: st.pitch, b, w: inp.w || wc.w || undefined };
        stepLocal({ phys, arsenal: st.arsenal, me: st.me, cmd, prevB: st.prevB, dt, t: CmdTime(0, cmd.seq), peers: peersOf(n), shoot: () => { st.shots++; }, zoomHeld: !!(b & BTN.SCOPE) });
        st.me.yaw = st.yaw; st.me.pitch = st.pitch;
        st.prevB = b;
      }
      if (step.noBack !== undefined) {
        for (const n of names) {
          const back = caseBack(before[n], ps[n].me.pos, ps[n].yaw, step[n] || {});
          if (back > step.noBack && !backed.has(n)) { backed.add(n); fails.push(`step ${si} (tick ${tick}) ${n} moved ${back.toFixed(4)} m backwards against its input (noBack ${step.noBack}): rubber banding`); }
        }
      }
      trace.push(Object.fromEntries(names.map((n) => [n, traceState(ps[n])])));
    }
  });
  return { fails, trace };
}

// How far (m) a body moved from a to b against its wish direction (move
// input f/r at yaw, as the sim reads it); 0 without input. = cases_test.go caseBack.
export function caseBack(a, b, yaw, inp) {
  const f = inp.f || 0, r = inp.r || 0;
  const wx = -Math.sin(yaw) * f + Math.cos(yaw) * r, wz = -Math.cos(yaw) * f - Math.sin(yaw) * r, l = Math.hypot(wx, wz);
  return l ? -((b[0] - a[0]) * wx + (b[2] - a[2]) * wz) / l : 0;
}

const r4 = (v) => Math.round(v * 1e4) / 1e4;
function traceState({ me, arsenal: a, shots }) {
  const h = a.slots[a.cur];
  return {
    pos: me.pos.map(r4), vel: me.vel.map(r4), onGround: me.onGround, crouched: me.crouched,
    cur: a.cur, ammo: h?.ammo ?? 0, reserve: h?.reserve ?? 0, reloading: !!a.reloadEnd, shots, spray: r4(a.recoil),
    scoped: a.scoped, jumps: me.jumps || 0, carry: !!a.carry, boost: Math.max(0, a.boostEnd - a.tickN),
  };
}

function check(st, who, k, want) {
  if (who === "world") return FIELDS[k.split(".")[0]] === "world" ? null : `unknown world field`;
  if (!st) return "unknown player";
  if (!(k in FIELDS) || k.startsWith("_")) return "unknown field";
  if (FIELDS[k] !== "sim" && FIELDS[k] !== "predictor") return null; // not the predictor's business
  const { me, arsenal: a } = st, h = a.slots[a.cur];
  const got = {
    pos: me.pos, vel: me.vel, onGround: me.onGround, crouched: me.crouched,
    cur: a.cur, ammo: h?.ammo ?? 0, reserve: h?.reserve ?? 0, reloading: !!a.reloadEnd, shots: st.shots, spray: a.recoil,
    scoped: a.scoped, jumps: me.jumps || 0, carry: !!a.carry, boost: Math.max(0, a.boostEnd - a.tickN),
  }[k];
  if (k === "pos" || k === "vel") return cmpVec(got, want);
  if (typeof got === "number") return cmpNum(got, want);
  return JSON.stringify(got) === JSON.stringify(want) ? null : `got ${JSON.stringify(got)}, want ${JSON.stringify(want)}`;
}

function cmpNum(got, want) {
  if (want && typeof want === "object") {
    if ((want.min !== undefined && got < want.min) || (want.max !== undefined && got > want.max)) return `got ${got}, want in ${JSON.stringify(want)}`;
    return null;
  }
  return Math.abs(got - want) <= 1e-9 ? null : `got ${got}, want ${want}`;
}

function cmpVec(got, want) {
  const v = Array.isArray(want) ? want : want.v, eps = (!Array.isArray(want) && want.eps) || 0.01;
  for (let i = 0; i < 3; i++) {
    if (v[i] === null) continue;
    const bad = typeof v[i] === "object" ? cmpNum(got[i], v[i]) : Math.abs(got[i] - v[i]) > eps;
    if (bad) return `got [${got.map((x) => x.toFixed(4))}], want ${JSON.stringify(want)} ±${eps}`;
  }
  return null;
}

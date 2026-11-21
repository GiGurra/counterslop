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
//   "players": {                       sorted by name = join order on the server
//     "alice": { "team": "vibe", "pos": [x, y, z], "yaw": 0, "pitch": 0, "crouch": false,
//                "slots": { "primary": { "id": "prompt", "ammo": 30, "reserve": 90 } },  default: team loadout
//                "cur": "primary" }
//   },
//   "script": [
//     { "n": 30, "alice": { "f": 1, "r": 0, "yaw": 0.5, "pitch": 0, "b": ["fire", "jump"], "w": "sidearm" } },
//         n ticks (default 1) of one command per tick per player; players not named send
//         an idle command. f/r are the move axes, b the held buttons (jump crouch walk fire
//         scope reload), w a slot to switch to; yaw/pitch persist until changed.
//         "drop": true drops the held weapon (server only: such cases leave out "js").
//     { "expect": { "alice": { "pos": [x, null, z], "ammo": 29, ... }, "world": { "items": 1 } } }
//         checked after the ticks so far. Vectors: [x, y, z] ±0.01 (null skips a
//         component, {"min": a, "max": b} bounds it) or {"v": [...], "eps": e}. Numbers: exact, or {"min": a, "max": b}.
//   ]
// }
// Fields (and the levels that know them): pos vel onGround crouched cur ammo reserve
// reloading shots (go js); scoped jumps (js: client-side state); hp dead kills deaths
// slots, world.items (go: server-side state).
import { createPhysics, newBody, H_CROUCH, EYE_CROUCH } from "../engine/physics.js";
import { BTN } from "../engine/cmd.js";
import { createArsenal } from "./arsenal.js";
import { stepLocal } from "./localsim.js";

export const FIELDS = {
  pos: "go js", vel: "go js", onGround: "go js", crouched: "go js",
  cur: "go js", ammo: "go js", reserve: "go js", reloading: "go js", shots: "go js",
  scoped: "js", jumps: "js",
  hp: "go", dead: "go", kills: "go", deaths: "go", slots: "go", items: "go",
};
const BUTTONS = { jump: BTN.JUMP, crouch: BTN.CROUCH, walk: BTN.WALK, fire: BTN.FIRE, scope: BTN.SCOPE, reload: BTN.RELOAD };
// = internal/game Registry.Loadout
const LOADOUT = (team) => ({
  primary: team === "artisan" ? { id: "linter", ammo: 30, reserve: 90 } : { id: "prompt", ammo: 30, reserve: 90 },
  sidearm: { id: "stub", ammo: 12, reserve: 48 },
  melee: { id: "keyboard", ammo: 0, reserve: 0 },
});

// Run a case through the client predictor; returns a list of failures (strings).
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
    ps[n] = { me, arsenal, prevB: cp.crouch ? BTN.CROUCH : 0, yaw: me.yaw, pitch: me.pitch, shots: 0 };
  }
  const fails = [];
  let tick = 0;
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
    for (const k of Object.keys(step)) if (k !== "n" && !ps[k]) throw new Error(`step ${si}: unknown key or player "${k}"`);
    for (let i = 0; i < (step.n ?? 1); i++) {
      tick++;
      for (const n of names) {
        const st = ps[n], inp = step[n] || {};
        if (inp.yaw !== undefined) st.yaw = inp.yaw;
        if (inp.pitch !== undefined) st.pitch = inp.pitch;
        if (inp.drop) throw new Error(`step ${si}: drop is server-only; leave "js" out of this case's levels`);
        let b = 0;
        for (const bn of inp.b || []) {
          if (!(bn in BUTTONS)) throw new Error(`step ${si}: unknown button "${bn}"`);
          b |= BUTTONS[bn];
        }
        st.arsenal.takeCmd(); // the case spells out every command
        const cmd = { seq: tick, f: inp.f || 0, r: inp.r || 0, yaw: st.yaw, pitch: st.pitch, b, w: inp.w };
        stepLocal({ phys, arsenal: st.arsenal, me: st.me, cmd, prevB: st.prevB, dt, shoot: () => { st.shots++; }, zoomHeld: !!(b & BTN.SCOPE) });
        st.me.yaw = st.yaw; st.me.pitch = st.pitch;
        st.prevB = b;
      }
    }
  });
  return fails;
}

function check(st, who, k, want) {
  if (who === "world") return FIELDS[k] === "go" ? null : `unknown world field`;
  if (!st) return "unknown player";
  if (!(k in FIELDS)) return "unknown field";
  if (!FIELDS[k].includes("js")) return null; // server-only
  const { me, arsenal: a } = st, h = a.slots[a.cur];
  const got = {
    pos: me.pos, vel: me.vel, onGround: me.onGround, crouched: me.crouched,
    cur: a.cur, ammo: h?.ammo ?? 0, reserve: h?.reserve ?? 0, reloading: !!a.reloadEnd, shots: st.shots,
    scoped: a.scoped, jumps: me.jumps || 0,
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

// The server rules in force on the client: the map's Rules (content.Map.Rules)
// pin some cvars over the server's (= Go Server.cvar), and the movement rules
// come from them through the shared MoveRulesOf (= Server.moveRules), so the
// prediction runs the same movement as the server.
import { MoveRulesOf, HookRulesOf } from "./shared.gen.js";
import { DefaultHook } from "../engine/sim.gen.js";

const HOOK = DefaultHook(); // the sv_hook_* defaults (= the server's cvar table)

// The cvars in force: the server's, with the map's pins over them.
export const effectiveCvars = (cvars = {}, map = null) => ({ ...cvars, ...(map?.rules ?? {}) });

// = Server.cvarF: a number, or def when not a number or negative.
const num = (v, def) => { const f = parseFloat(v); return Number.isFinite(f) && f >= 0 ? f : def; };

// The sim MoveRules {model, airAccel, autoBhop} of effective cvars.
export const moveRules = (cv = {}) => ({
  ...MoveRulesOf(cv.sv_movement ?? "", num(cv.sv_airaccelerate, 0), num(cv.sv_autobunnyhopping, 0) !== 0),
  hook: HookRulesOf(num(cv.sv_hook_range, HOOK.range), num(cv.sv_hook_energy, HOOK.energy), num(cv.sv_hook_recharge, HOOK.recharge),
    num(cv.sv_hook_drain, HOOK.drain), num(cv.sv_hook_cost, HOOK.cost), num(cv.sv_hook_pull, HOOK.pull)), // = Server.moveRules
});

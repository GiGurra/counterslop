// One local command tick: what the client does to its own player and weapons
// for each command it sends (main.js simTick), and what the server does with
// the same command (internal/game runCmd). Pure apart from the arsenal's sounds,
// so sim/cases replays it in Node (web/game/cases.js).
import { BTN, moveInput } from "../engine/cmd.js";
import { AimedItemG } from "./shared.gen.js";
import { above } from "../engine/gravity.js";

// Feet at p in zone z (= content.Zone Contains).
export const inZone = (z, p) => p[0] >= z.min[0] && p[0] <= z.max[0] && p[1] >= z.min[1] && p[1] <= z.max[1] && p[2] >= z.min[2] && p[2] <= z.max[2];

// Moves `me` with the command's yaw (bit-identical to the server and the
// replay; the view yaw is kept), then ticks the weapons. Returns {ev, vm}:
// physics events and the viewmodel params. t: when the command runs on the
// movers' clock (CmdTime of the player's tick base and the seq). peers: the
// other players' hulls [{pos, h, lo}] (peersFrom in predict.js). sites: the
// map's bomb sites; items: the floor weapons' points as last seen (an E press
// aimed at one takes it on the server instead of drawing The Big Diff).
export function stepLocal({ phys, arsenal, me, cmd, prevB, dt, t = 0, peers = null, shoot = () => {}, zoomHeld = false, sites = [], items = [] }) {
  const viewYaw = me.yaw;
  me.yaw = cmd.yaw;
  const inp = moveInput(cmd, prevB, me);
  // The weapon in hand sets the max speed (= server heldSpeed); kept on the
  // command so the reconciliation replay (predict.js) moves at the same speed.
  inp.speed = cmd.speed = arsenal.moveSpeed();
  const ev = phys.step(me, inp, dt, t, peers); // t: the command's time on the movers' clock (predictor.cmdTime); peers: the solid players (predictor.peers)
  me.yaw = viewYaw;
  if (ev.jumped) me.jumps = (me.jumps || 0) + 1;
  // +use's inputs, as the server sees them after the move (kept on the command for replays).
  const down = !!(cmd.b & BTN.USE) && !(prevB & BTN.USE);
  cmd.use = { b: cmd.b, prevB, site: sites.some((s) => inZone(s, me.pos)),
    aim: down && items.length > 0 && AimedItemG(me.grav | 0, me.grav ? above(me.pos, me.grav, me.eyeH + me.eyeOff) : [me.pos[0], me.pos[1] + me.eyeH + me.eyeOff, me.pos[2]], cmd.yaw, cmd.pitch, items) >= 0 }; // as the server: the eye out of its floor, the view in its frame
  const vm = arsenal.tick(dt, !!(cmd.b & BTN.FIRE), me, shoot, zoomHeld, { w: cmd.w, reload: !!(cmd.b & BTN.RELOAD), frozen: !!cmd.frozen, use: cmd.use, alt: !!(cmd.b & BTN.ALT) });
  return { ev, vm };
}

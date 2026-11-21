// One local command tick: what the client does to its own player and weapons
// for each command it sends (main.js simTick), and what the server does with
// the same command (internal/game runCmd). Pure apart from the arsenal's sounds,
// so sim/cases replays it in Node (web/game/cases.js).
import { BTN, moveInput } from "../engine/cmd.js";

// Moves `me` with the command's yaw (bit-identical to the server and the
// replay; the view yaw is kept), then ticks the weapons. Returns {ev, vm}:
// physics events and the viewmodel params. t: when the command runs on the
// movers' clock (CmdTime of the player's tick base and the seq).
export function stepLocal({ phys, arsenal, me, cmd, prevB, dt, t = 0, shoot = () => {}, zoomHeld = false }) {
  const viewYaw = me.yaw;
  me.yaw = cmd.yaw;
  const inp = moveInput(cmd, prevB, me);
  // The weapon in hand sets the max speed (= server heldSpeed); kept on the
  // command so the reconciliation replay (predict.js) moves at the same speed.
  inp.speed = cmd.speed = arsenal.moveSpeed();
  const ev = phys.step(me, inp, dt, t); // t: the command's time on the movers' clock (predictor.cmdTime)
  me.yaw = viewYaw;
  if (ev.jumped) me.jumps = (me.jumps || 0) + 1;
  const vm = arsenal.tick(dt, !!(cmd.b & BTN.FIRE), me, shoot, zoomHeld, { w: cmd.w, reload: !!(cmd.b & BTN.RELOAD) });
  return { ev, vm };
}

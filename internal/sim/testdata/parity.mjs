// Runs web/engine/physics.js on a scripted input sequence and prints the
// trajectory, for internal/sim's parity test. Usage: node parity.mjs case.json
import { readFileSync } from "node:fs";
import { createPhysics, newBody } from "../../../web/engine/physics.js";

const c = JSON.parse(readFileSync(process.argv[2], "utf8"));
const phys = createPhysics();
phys.setMap(c.map);
phys.setMovement(c.move);
const out = [];
for (const run of c.runs) {
  const s = newBody(run.spawn);
  s.yaw = run.yaw;
  s.grav = run.grav || 0;
  const traj = [];
  for (const [ti, t] of run.ticks.entries()) {
    s.yaw = t.yaw ?? s.yaw;
    const ev = phys.step(s, { f: t.f, r: t.r, walk: !!t.walk, jump: !!t.jump, crouch: !!t.crouch, speed: run.speed, jumpHeld: !!t.held }, c.dt, (ti + 1) / 100, run.peers);
    traj.push({ p: [...s.pos], v: [...s.vel], g: s.onGround, c: s.crouched, j: ev.jumped, l: ev.landed, s: ev.steps });
  }
  out.push(traj);
}
process.stdout.write(JSON.stringify(out));

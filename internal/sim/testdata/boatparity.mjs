// Steps boats with web/engine/physics.js (the generated internal/sim boat.go)
// for internal/sim's TestBoatParityWithJS. Usage: node boatparity.mjs boats.json
import { readFileSync } from "node:fs";
import { createPhysics } from "../../../web/engine/physics.js";

const c = JSON.parse(readFileSync(process.argv[2], "utf8"));
const phys = createPhysics();
phys.setMap(c.map);
const out = c.runs.map((rn) => {
  const b = { pos: [...rn.boat.Pos], vel: [...rn.boat.Vel], yaw: rn.boat.Yaw, yawVel: rn.boat.YawVel };
  return rn.steps.map((s) => {
    phys.stepBoat(b, { f: s.F, r: s.R }, 0.01);
    return [b.pos[0], b.pos[1], b.pos[2], b.vel[0], b.vel[2], b.yaw, b.yawVel];
  });
});
process.stdout.write(JSON.stringify(out));

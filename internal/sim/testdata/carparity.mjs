// Steps cars with web/engine/physics.js (the generated internal/sim car.go)
// for internal/sim's TestCarParityWithJS. Usage: node carparity.mjs cars.json
import { readFileSync } from "node:fs";
import { createPhysics } from "../../../web/engine/physics.js";
import { NewCar } from "../../../web/engine/sim.gen.js";

const c = JSON.parse(readFileSync(process.argv[2], "utf8"));
const phys = createPhysics();
phys.setMap(c.map);
const out = c.runs.map((rn) => {
  const sp = c.specs[rn.type];
  const car = NewCar(sp, rn.pos, rn.yaw);
  car.boost = rn.boost;
  car.slip = rn.slip;
  car.engine = rn.engine;
  car.tyres = rn.tyres;
  const other = rn.other ? [rn.other] : null;
  return rn.steps.map((s) => {
    phys.stepCar(sp, car, { throttle: s.T, steer: s.S, handbrake: s.H, boost: s.B }, 0.01, other);
    return [car.pos[0], car.pos[1], car.pos[2], car.vel[0], car.vel[1], car.vel[2], car.yaw, car.yawVel, car.pitch, car.roll, car.steer, car.contact, car.comp[0], car.comp[3], car.onGround ? 1 : 0, car.wet ? 1 : 0, car.boost, car.slip];
  });
});
process.stdout.write(JSON.stringify(out));

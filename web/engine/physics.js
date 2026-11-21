// Player movement against the map's AABBs. Position is the feet centre.
// The rules are internal/sim (Go), translated by cmd/gojs into sim.gen.js:
// one source for the server and the predictor. This file only adapts it to
// the client's API and adds the client-only raycast.
import { NewBody, World_Step, R, H, Eye as EYE, HCrouch as H_CROUCH, EyeCrouch as EYE_CROUCH, Run as RUN } from "./sim.gen.js";

export { R, H, EYE, H_CROUCH, EYE_CROUCH, RUN };

export const newBody = (spawn) => NewBody(spawn);

export function createPhysics() {
  const world = { boxes: [], conveyors: [] };
  const boxes = () => world.boxes;

  return {
    setMap(map) { world.boxes = map.boxes; world.conveyors = map.conveyors ?? []; },
    // inp: {f, r, walk, jump, crouch, speed}; returns sound-worthy events {jumped, landed, steps}.
    step: (s, inp, dt) => World_Step(world, s, inp, dt),
    // Ray vs boxes (slab test). Returns {t, n} for the nearest hit within maxT.
    raycast(o, d, maxT = 200) {
      let best = null;
      for (const b of boxes()) {
        let t0 = 0, t1 = maxT, nAx = -1, nSign = 0;
        for (let a = 0; a < 3; a++) {
          if (Math.abs(d[a]) < 1e-9) { if (o[a] < b.min[a] || o[a] > b.max[a]) { t0 = Infinity; break; } continue; }
          let ta = (b.min[a] - o[a]) / d[a], tb = (b.max[a] - o[a]) / d[a];
          let sgn = -Math.sign(d[a]);
          if (ta > tb) [ta, tb] = [tb, ta];
          if (ta > t0) { t0 = ta; nAx = a; nSign = sgn; }
          if (tb < t1) t1 = tb;
          if (t0 > t1) break;
        }
        if (t0 <= t1 && t0 < (best?.t ?? Infinity) && nAx >= 0) {
          const n = [0, 0, 0]; n[nAx] = nSign;
          best = { t: t0, n };
        }
      }
      return best;
    },
  };
}


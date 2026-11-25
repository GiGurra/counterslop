// Player movement against the map's AABBs. Position is the feet centre.
// The rules are internal/sim (Go), translated by cmd/gojs into sim.gen.js:
// one source for the server and the predictor. This file only adapts it to
// the client's API and adds the client-only raycast.
import { NewBody, NewWorld, World_Step, MoverBoxes, R, H, Eye as EYE, HCrouch as H_CROUCH, EyeCrouch as EYE_CROUCH, Run as RUN } from "./sim.gen.js";

export { R, H, EYE, H_CROUCH, EYE_CROUCH, RUN };

export const newBody = (spawn) => NewBody(spawn);

export function createPhysics() {
  let world = NewWorld({ boxes: [], movers: null });
  // The movement rules in force (sim MoveRules: {model, airAccel, autoBhop}),
  // kept across map changes; the game sets them from the effective server rules.
  let move = world.move;
  // What is visible: the static boxes less the treads the movers replace, plus
  // the movers at time t (when given). Bullets on the server hit the static
  // treads; the client's cosmetics (holes, casings) use what is drawn.
  const boxes = (t) => (t !== undefined && world.movers?.length ? [...world.boxes, ...moverBoxes(t)] : world.boxes);
  const moverBoxes = (t) => (MoverBoxes(world.movers, t) ?? []).map((b) => ({ ...b, mover: true }));

  return {
    setMap(map) { world = NewWorld(map); world.move = move; },
    // rules: {model: MoveClassic|MoveSource, airAccel, autoBhop} (game/rules.js moveRules).
    setMovement(rules) { move = { model: rules.model | 0, airAccel: +rules.airAccel || 0, autoBhop: !!rules.autoBhop }; world.move = move; },
    movement: () => move,
    // inp: {f, r, walk, jump, crouch, speed}; t: the command's time on the
    // movers' clock (CmdTime). peers: the other solid players [{pos, h}] (their
    // snapshot positions; internal/sim/peers.go). Returns sound-worthy events {jumped, landed, steps}.
    step(s, inp, dt, t = 0, peers = null) { world.t = t; world.peers = peers; return World_Step(world, s, inp, dt); },
    // The map's moving solids at time t (escalator treads, ...): what to draw.
    moverBoxes,
    // The visible solids at mover time t (casings bounce on these).
    solids: boxes,
    // Ray vs the visible boxes (slab test; with the movers at time t if given).
    // Returns {t, n, mover} for the nearest hit within maxT (mover: a moving box).
    raycast(o, d, maxT = 200, t = undefined) {
      let best = null;
      for (const b of boxes(t)) {
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
          best = { t: t0, n, mover: !!b.mover };
        }
      }
      return best;
    },
  };
}


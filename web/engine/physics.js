// Player movement against the map's solids (boxes and wedges). Position is the feet centre.
// The rules are internal/sim (Go), translated by cmd/gojs into sim.gen.js:
// one source for the server and the predictor. This file only adapts it to
// the client's API and adds the client-only raycast.
import { NewBody, NewWorld, World_Step, MoverBoxes, RaySolid, FaceNormal, R, H, Eye as EYE, HCrouch as H_CROUCH, EyeCrouch as EYE_CROUCH, Run as RUN } from "./sim.gen.js";

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
    // Ray vs the visible solids (boxes and wedges: RaySolid, internal/sim; with
    // the movers at time t if given). Returns {t, n, mover} for the nearest hit
    // within maxT (mover: a moving box); a solid the ray starts in is ignored.
    raycast(o, d, maxT = 200, t = undefined) {
      let best = null;
      for (const b of boxes(t)) {
        const [t0, , face] = RaySolid(o, d, b);
        if (face >= 0 && t0 <= maxT && t0 < (best?.t ?? Infinity)) best = { t: t0, b, face, mover: !!b.mover };
      }
      return best && { t: best.t, n: FaceNormal(best.b, best.face, d), mover: best.mover };
    },
  };
}


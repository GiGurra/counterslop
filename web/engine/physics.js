// Player movement against the map's solids (boxes and wedges). Position is the feet centre.
// The rules are internal/sim (Go), translated by cmd/gojs into sim.gen.js:
// one source for the server and the predictor. This file only adapts it to
// the client's API and adds the client-only raycast.
import { RideStep, RideLeave } from "../game/shared.gen.js";
import { NewBody, NewWorld, World_Step, World_StepBoat, World_StepDeck, World_WaterLevel, MoverBoxes, RaySolid, FaceNormal, R, H, Eye as EYE, HCrouch as H_CROUCH, EyeCrouch as EYE_CROUCH, Run as RUN } from "./sim.gen.js";

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
    // rules: {model: MoveClassic|MoveSource, airAccel, autoBhop, hook?} (game/rules.js moveRules;
    // hook: sim HookRules {range, energy, recharge, drain, cost, pull}, the Gravity Override's).
    setMovement(rules) {
      const h = rules.hook || {}, n = (v) => +v || 0;
      move = { model: rules.model | 0, airAccel: n(rules.airAccel), autoBhop: !!rules.autoBhop,
        hook: { range: n(h.range), energy: n(h.energy), recharge: n(h.recharge), drain: n(h.drain), cost: n(h.cost), pull: n(h.pull) } };
      world.move = move;
    },
    movement: () => move,
    // inp: {f, r, walk, jump, crouch, speed}; t: the command's time on the
    // movers' clock (CmdTime). peers: the other solid players [{pos, h}] (their
    // snapshot positions; internal/sim/peers.go). Returns sound-worthy events {jumped, landed, steps}.
    step(s, inp, dt, t = 0, peers = null) { world.t = t; world.peers = peers; return World_Step(world, s, inp, dt); },
    // A boat (internal/sim boat.go: {pos, vel, yaw, yawVel}) one step of dt
    // with the driver's input {f, r}: the driver's prediction, and the replay.
    stepBoat(b, inp, dt) { World_StepBoat(world, b, inp, dt); },
    // One command of a rider of boat b in seat (internal/game ride_shared.go):
    // the driver (seat 0) steers it with the move axes f, r; the rider sits.
    ride(b, s, f, r, seat, dt) { RideStep(world, b, s, f, r, seat, dt); },
    // Out of boat b (E or jump: hop), moving with it; on foot from here.
    leaveBoat(b, s, hop) { RideLeave(b, s, hop); },
    // One command of body s standing on boat b's deck d ({pos, vel} in the
    // boat's frame; internal/sim boat.go StepDeck): moves it there and puts s
    // where that is in the map. Returns the events, as step.
    deck(b, s, d, inp, dt) { return World_StepDeck(world, b, s, d, inp, dt); },
    // How deep body s is in the water (0 dry, 1 wading, 2 swimming, 3 under).
    waterLevel: (s) => World_WaterLevel(world, s),
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


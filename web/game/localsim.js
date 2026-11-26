// One local command tick: what the client does to its own player and weapons
// for each command it sends (main.js simTick), and what the server does with
// the same command (internal/game runCmd). Pure apart from the arsenal's sounds,
// so sim/cases replays it in Node (web/game/cases.js).
import { BTN, moveInput } from "../engine/cmd.js";
import { AimedItemG, CarLeaves, RideLeaves, RideGetUp, DeckLeaves, DeckCarry, SeatDeck } from "./shared.gen.js";
import { LandOnBoat, OnHull } from "../engine/sim.gen.js";
import { above } from "../engine/gravity.js";
import { carSpec } from "./cars.js";

const offBoat = (me) => { me.ride = 0; me.seat = 0; me.boat = null; me.deck = null; };

// Up out of our seat (= server boats.go leaveBoat): E or a jump stands us up
// on the deck where we sat, the hook pulls us out (on foot).
export function leave(phys, me, b, prevB) {
  const d = { pos: [0, 0, 0], vel: [0, 0, 0] };
  if (RideGetUp(me.boat, me, me.seat, b, prevB, !!me.boots, d)) { me.seat = SeatDeck; me.deck = d; } else offBoat(me);
}

// Out of the car we drive (= server cars.go carCmd): over its seat, on foot.
export function leaveCar(phys, me) {
  phys.leaveCar(carSpec(me.carType), me.carState, me);
  me.car = 0; me.carState = null; me.carType = "";
}

// A boat of the newest snapshot (boats.js list: {id, p, y, v}) as sim.Boat.
const simBoat = (b) => ({ pos: [...b.p], vel: [...(b.v || [0, 0, 0])], yaw: b.y, yawVel: 0, driven: false });

// One command's movement of `me` (= server simtick.go runCmd with boats.go
// rideCmd and landOnBoats): seated (the driver steers), standing on a deck
// (StepDeck in the boat's frame; a boat nobody drives drifts on under us, as
// the server's tickBoats), or on foot, landing on a boat's floor (boats: the
// newest snapshot's, in id order). Returns the physics events.
export function moveCmd(phys, me, c, inp, prevB, dt, t = 0, peers = null, boats = null, cars = null) {
  // Driving a car (internal/game car_shared.go; = server cars.go carCmd): E gets
  // out (on foot from here, this same command), else the move keys drive it.
  // cars: the other cars' discs (cars.js discs).
  if (me.car && CarLeaves(c.b, prevB, !!me.cl)) leaveCar(phys, me);
  if (me.car) { phys.drive(carSpec(me.carType), me.carState, me, c.f || 0, c.r || 0, c.b | 0, cars, dt); return { jumped: false, landed: 0, steps: 0, splash: 0 }; }
  if (me.ride && me.seat !== SeatDeck && RideLeaves(c.b, prevB, !!me.boots)) leave(phys, me, c.b, prevB);
  if (me.ride && me.seat === SeatDeck && DeckLeaves(c.b, prevB, !!me.boots)) offBoat(me);
  if (me.ride && me.seat === SeatDeck) {
    const ev = phys.deck(me.boat, me, me.deck, inp, dt);
    if (!OnHull(me.deck.pos[0], me.deck.pos[2])) offBoat(me); // walked off the hull: on foot from here
    else if (!me.boat.driven) { phys.stepBoat(me.boat, { f: 0, r: 0 }, dt); DeckCarry(me.boat, me, me.deck); }
    return ev;
  }
  if (me.ride) { phys.ride(me.boat, me, c.f || 0, c.r || 0, me.seat, dt); return { jumped: false, landed: 0, steps: 0, splash: 0 }; }
  const y0 = me.pos[1];
  const ev = phys.step(me, inp, dt, t, peers, cars); // the cars are solid to a body on foot
  for (const sb of boats || []) {
    const b = simBoat(sb), d = { pos: [0, 0, 0], vel: [0, 0, 0] };
    if (LandOnBoat(b, me, y0, d)) {
      me.ride = sb.id; me.seat = SeatDeck; me.boat = b; me.deck = d;
      phys.stepBoat(b, { f: 0, r: 0 }, dt); DeckCarry(b, me, d); // (assumed undriven until the snapshot says)
      break;
    }
  }
  return ev;
}

// Feet at p in zone z (= content.Zone Contains).
export const inZone = (z, p) => p[0] >= z.min[0] && p[0] <= z.max[0] && p[1] >= z.min[1] && p[1] <= z.max[1] && p[2] >= z.min[2] && p[2] <= z.max[2];

// Moves `me` with the command's yaw (bit-identical to the server and the
// replay; the view yaw is kept), then ticks the weapons. Returns {ev, vm}:
// physics events and the viewmodel params. t: when the command runs on the
// movers' clock (CmdTime of the player's tick base and the seq). peers: the
// other players' hulls [{pos, h, lo}] (peersFrom in predict.js). sites: the
// map's bomb sites; items: the floor weapons' points as last seen (an E press
// aimed at one takes it on the server instead of drawing The Big Diff).
export function stepLocal({ phys, arsenal, me, cmd, prevB, dt, t = 0, peers = null, boats = null, cars = null, shoot = () => {}, zoomHeld = false, sites = [], items = [] }) {
  const viewYaw = me.yaw;
  me.yaw = cmd.yaw;
  const inp = moveInput(cmd, prevB, me);
  // The weapon in hand sets the max speed (= server heldSpeed); kept on the
  // command so the reconciliation replay (predict.js) moves at the same speed.
  inp.speed = cmd.speed = arsenal.moveSpeed();
  // In a boat (internal/game ride_shared.go): seated (the driver steers it with
  // the move keys; E or jump stands up), on its deck, or on foot.
  // t: the command's time on the movers' clock (predictor.cmdTime); peers: the solid players (predictor.peers);
  // boats: the newest snapshot's (predictor.boats), to land on.
  const ev = moveCmd(phys, me, cmd, inp, prevB, dt, t, peers, boats, cars); // cars: the other cars' discs (predictor.cars)
  me.yaw = viewYaw;
  if (ev.jumped) me.jumps = (me.jumps || 0) + 1;
  // +use's inputs, as the server sees them after the move (kept on the command for replays).
  const down = !!(cmd.b & BTN.USE) && !(prevB & BTN.USE);
  cmd.use = { b: cmd.b, prevB, site: sites.some((s) => inZone(s, me.pos)),
    aim: down && items.length > 0 && AimedItemG(me.grav | 0, me.grav ? above(me.pos, me.grav, me.eyeH + me.eyeOff) : [me.pos[0], me.pos[1] + me.eyeH + me.eyeOff, me.pos[2]], cmd.yaw, cmd.pitch, items) >= 0 }; // as the server: the eye out of its floor, the view in its frame
  const vm = arsenal.tick(dt, !!(cmd.b & BTN.FIRE), me, shoot, zoomHeld, { w: cmd.w, reload: !!(cmd.b & BTN.RELOAD), frozen: !!cmd.frozen, use: cmd.use, alt: !!(cmd.b & BTN.ALT) });
  return { ev, vm };
}

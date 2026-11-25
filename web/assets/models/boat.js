// The inflatable dinghy (internal/sim boat.go): the startup's boarding craft.
// Grey tubes with a pink "SERIES A" livery stripe, a slatted floor, a transom
// and a lime-cowled outboard, all boxes. Model space: the waterline at y 0,
// the bow towards -z (as a player faces), 3.6 m long, 1.7 m wide.
//   boatParts({pos, yaw, time, id, speed})   bobbing and rolling a little
import { chain, translate, scale, rotX, rotY, rotZ } from "../../engine/math.js";
import { PALETTE as P } from "../palette.js";

export const BOAT = { len: 3.6, beam: 1.7, tube: 0.4 };

// [x, y, z, sx, sy, sz, colour, emit, rotY]: centred boxes, y up from the waterline.
function hull() {
  const t = BOAT.tube, y = 0.08, side = BOAT.beam / 2 - t / 2;
  const out = [
    [-side, y, 0.25, t, t, 2.9, "stubGrey"], [side, y, 0.25, t, t, 2.9, "stubGrey"], // the side tubes
    [-0.42, y, -1.42, t, t, 0.95, "stubGrey", 0, -0.75], [0.42, y, -1.42, t, t, 0.95, "stubGrey", 0, 0.75], // the bow
    [0, y + 0.01, -1.72, 0.5, t - 0.02, 0.3, "stubGrey"],
    [0, 0.08, 0.1, BOAT.beam - 2 * t, 0.04, 3.0, "gunmetal"], // the floor (over the water, bobbing included)
    [0, 0.1, 1.63, BOAT.beam - 2 * t + 0.02, 0.44, 0.08, "caseBlack"], // the transom
    [0, 0.3, 0.35, BOAT.beam - 2 * t, 0.05, 0.28, "oak"], // the thwart (a bench)
    // The livery: a pink stripe along each tube and the name on it in lime.
    [-side - t / 2 - 0.003, y + 0.05, 0.3, 0.006, 0.09, 2.4, "toyPink", 0.25], [side + t / 2 + 0.003, y + 0.05, 0.3, 0.006, 0.09, 2.4, "toyPink", 0.25],
    [-side - t / 2 - 0.006, y - 0.07, 0.6, 0.006, 0.06, 1.1, "neonLime", 0.5], [side + t / 2 + 0.006, y - 0.07, 0.6, 0.006, 0.06, 1.1, "neonLime", 0.5],
    // The outboard: a cowl on a shaft over the transom, a tiller arm forward.
    [0, 0.42, 1.78, 0.32, 0.34, 0.4, "caseBlack"], [0, 0.5, 1.78, 0.33, 0.06, 0.41, "neonLime", 0.4],
    [0, -0.15, 1.82, 0.08, 0.6, 0.1, "gunmetal"], [0, 0.4, 1.4, 0.05, 0.05, 0.5, "gunmetal"],
  ];
  return out;
}
const HULL = hull();

// pos: the waterline centre, yaw: its heading (a player's yaw), time: seconds
// (the bobbing), id: its phase, speed: m/s (the bow lifts with it).
export function boatParts({ pos, yaw = 0, time = 0, id = 1, speed = 0 }) {
  const bob = 0.05 * Math.sin(time * 1.7 + id * 2.1), roll = 0.04 * Math.sin(time * 1.3 + id), lift = Math.min(0.12, speed * 0.012);
  const base = chain(translate(pos[0], pos[1] + bob, pos[2]), rotY(yaw), rotX(lift + 0.02 * Math.sin(time * 1.1 + id)), rotZ(roll));
  return HULL.map(([x, y, z, sx, sy, sz, col, emit = 0, ry = 0]) => ({
    m: chain(base, translate(x, y, z), rotY(ry), scale(sx, sy, sz)), color: P[col] || P.gunmetal, emit,
  }));
}

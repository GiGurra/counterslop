// The race garage's bay (game/garage.js, docs/racing.md "The garage"): a
// client-side scene, never a map the server knows. Drawn by its own renderer
// (game/garagescene.js) from boxes in our materials and our own lights:
// a concrete floor and walls, a roller door under the neon sign "GARAGE ·
// NO REFUNDS", a tool wall with a workbench, ceiling lamps, a turntable in
// the middle, and a row of four parking bays behind it the car types stand in.
//   GARAGE_ROOM            {boxes, lights, signs, indoor}: the bay's map
//   BAYS                   where each parked type stands: [{pos, yaw}] (cars.json order)
//   TURNTABLE              {pos, r}: the turntable's centre and radius
//   garageProps(time)      the room's solids each frame: the turntable, tyre stacks, tools
//   turntableParts(angle)  the turning disc
import { chain, translate, scale, rotY } from "../engine/math.js";
import { PALETTE as P } from "./palette.js";

const W = 15, D = 11, H = 6.2; // half width (x), half depth (z), height
export const TURNTABLE = { pos: [0, 0.12, 1.5], r: 3.3 };
// A row of bays behind the turntable, the cars nosed out towards it.
export const BAYS = [-8.4, -2.8, 2.8, 8.4].map((x) => ({ pos: [x, 0, -6.6], yaw: Math.PI }));

const box = (min, max, kind) => ({ min, max, kind });

function room() {
  const b = [];
  b.push(box([-W - 1, -0.5, -D - 1], [W + 1, 0, D + 1], "concrete")); // the floor
  b.push(box([-W - 1, H, -D - 1], [W + 1, H + 0.5, D + 1], "concrete")); // the ceiling
  b.push(box([-W - 1, 0, -D - 1], [-W, H, D + 1], "concrete")); // west wall
  b.push(box([W, 0, -D - 1], [W + 1, H, D + 1], "concrete")); // east wall
  b.push(box([-W, 0, D], [W, H, D + 1], "plaster")); // the south wall (behind the camera)
  // The north wall: the roller door (slats of metal) in a concrete frame.
  b.push(box([-W, 0, -D - 1], [-6, H, -D], "concrete"));
  b.push(box([6, 0, -D - 1], [W, H, -D], "concrete"));
  b.push(box([-6, 4.6, -D - 1], [6, H, -D], "concrete"));
  for (let k = 0; k < 11; k++) b.push(box([-6, k * 0.42, -D - 0.85 + (k % 2) * 0.04], [6, k * 0.42 + 0.38, -D - 0.5], "metal"));
  b.push(box([-6.4, 0, -D - 0.2], [-6, 4.8, -D + 0.25], "hazard")); // the door's posts
  b.push(box([6, 0, -D - 0.2], [6.4, 4.8, -D + 0.25], "hazard"));
  // The sign's backing over the door.
  b.push(box([-5.4, 4.75, -D + 0.02], [5.4, 5.85, -D + 0.12], "metal"));
  // The tool wall (west, by the door): a workbench, a shelf, pegboard panels.
  b.push(box([-W, 0, -D + 0.6], [-W + 0.9, 0.95, -7.8], "metal"));
  b.push(box([-W, 1.6, -D + 0.6], [-W + 0.45, 1.68, -7.8], "metal"));
  b.push(box([-W, 0.95, -D + 0.4], [-W + 0.06, 2.6, -7.6], "desk"));
  // Pillars on the side walls, hazard striped; a wheel stop at the back of each bay.
  for (const s of [-1, 1]) b.push(box([s * W - (s > 0 ? 0.7 : 0), 0, -0.6], [s * W + (s < 0 ? 0.7 : 0), H, 0.6], "hazard"));
  for (const bay of BAYS) b.push(box([bay.pos[0] - 1, 0, -9.3], [bay.pos[0] + 1, 0.15, -9.1], "curb"));
  // The turntable's pit rim (the disc itself turns: garageProps).
  b.push(box([-3.6, 0, TURNTABLE.pos[2] - 3.6], [3.6, 0.06, TURNTABLE.pos[2] + 3.6], "metal"));
  // Ceiling lamps.
  for (const [x, z] of [[-7, -5], [7, -5], [-7, 4], [7, 4], [0, -6], [0, 5]]) b.push(box([x - 1.2, H - 0.2, z - 0.25], [x + 1.2, H, z + 0.25], "lamp"));
  // Floor tape round each bay (glowing strips).
  for (const bay of BAYS) {
    const [x, , z] = bay.pos;
    for (const dx of [-1.6, 1.6]) b.push(box([x + dx - 0.06, 0, z - 3], [x + dx + 0.06, 0.015, z + 3], "lamp"));
  }
  return b;
}

export const GARAGE_ROOM = {
  name: "garage",
  boxes: room(),
  signs: [{ text: "GARAGE · NO REFUNDS", pos: [0, 5.3, -D + 0.13], face: "+z", size: 0.55, mat: "led" }],
  lights: [
    ...[[-7, -5], [7, -5], [-7, 4], [7, 4], [0, -6], [0, 5]].map(([x, z]) => ({ kind: "point", pos: [x, H - 0.5, z], color: [1, 0.93, 0.82], intensity: 2.2, range: 13 })),
    { kind: "point", pos: [0, 4.4, -D + 1.2], color: [1, 0.25, 0.7], intensity: 2.5, range: 8 }, // the sign's glow
    { kind: "spot", pos: [0, H - 0.3, 4.5], dir: [0, -1, -0.45], color: [0.9, 0.95, 1], intensity: 4, range: 12, cone: 35, shadow: true }, // on the turntable
  ],
  indoor: 1,
  spawns: [], teamSpawns: {},
};

// The turntable's disc at angle a (rad): four squares turned 22.5° apart, a cross,
// chevrons that show it turn.
export function turntableParts(a) {
  const [x, y, z] = TURNTABLE.pos, r = TURNTABLE.r, out = [];
  const base = chain(translate(x, 0, z), rotY(a));
  const add = (m, c, emit = 0) => out.push({ m: chain(base, m), color: P[c] || c, emit });
  const k = r * 2 * 0.95;
  for (let i = 0; i < 4; i++) add(chain(rotY((i * Math.PI) / 8), translate(0, y / 2, 0), scale(k, y, k)), "gunmetal");
  add(chain(translate(0, y + 0.005, 0), scale(k * 0.96, 0.01, 0.12)), "hazard");
  add(chain(rotY(Math.PI / 2), translate(0, y + 0.005, 0), scale(k * 0.96, 0.01, 0.12)), "hazard");
  for (let i = 0; i < 8; i++) add(chain(rotY((i * Math.PI) / 4), translate(0, y + 0.01, -r * 0.88), scale(0.5, 0.012, 0.18)), "neonCyan", 0.5);
  return out;
}

// The room's props each frame: the turntable (turned to angle), stacks of
// tyres, tools on the pegboard, a jerry can.
export function garageProps(angle) {
  const out = turntableParts(angle);
  const add = (x, y, z, sx, sy, sz, c, emit = 0, r = 0) => out.push({ m: chain(translate(x, y, z), rotY(r), scale(sx, sy, sz)), color: P[c] || c, emit });
  for (const [x, z, n] of [[-13.4, 8.5, 4], [-12.2, 9.2, 3], [13.2, 8.7, 5], [12.6, -9.5, 2]]) {
    for (let i = 0; i < n; i++) { add(x, 0.15 + i * 0.3, z, 0.9, 0.28, 0.9, "tyre", 0, i * 0.4); add(x, 0.15 + i * 0.3, z, 0.9, 0.28, 0.9, "tyre", 0, i * 0.4 + Math.PI / 4); }
  }
  for (let i = 0; i < 7; i++) add(-W + 0.12, 1.4 + (i % 3) * 0.35, -D + 0.9 + i * 0.4, 0.06, 0.28, 0.05, i % 2 ? "steel" : "reviewRed"); // spanners and screwdrivers
  add(-W + 0.45, 1.15, -9, 0.5, 0.4, 0.3, "reviewRed"); // a toolbox on the bench
  add(-W + 0.45, 1.4, -9, 0.3, 0.06, 0.08, "steel");
  add(-W + 0.4, 0.3, 7, 0.35, 0.6, 0.2, "containerGreen"); // a jerry can
  return out;
}

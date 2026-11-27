// Moving solids (content.Mover, map JSON "movers"): the boxes to draw at time
// t, in place of the map's static treads they replace. Where they are is the
// shared sim (internal/sim movers.go, generated into sim.gen.js), so what the
// renderer draws at a command's time is exactly what the body collides with.
import { DrawnMoverBoxes, MoverBoxes, MoverOffset, Replaced } from "./sim.gen.js";

// Every mover box showing at time t (seconds on the movers' clock).
export const moverBoxes = (movers, t) => (movers?.length ? MoverBoxes(movers, t) ?? [] : []);

// The same boxes as drawn: escalator treads a centimetre low, none where they
// run level with the floor (they would z-fight with it). Collision: moverBoxes.
export const drawnMoverBoxes = (movers, t) => (movers?.length ? DrawnMoverBoxes(movers, t) ?? [] : []);

// drawnMoverBoxes, each box with its texture origin (org: where the mover has
// moved it from where it was built, MoverOffset): the renderer patterns a
// mover's faces in the mover's own space (world position - org), so a ship's
// carpet travels with the ship instead of sliding over it. Escalator treads
// keep the world's (org 0): each comes out of the floor and goes under the comb.
export function drawnMoverParts(movers, t) {
  const out = [];
  for (let i = 0; i < (movers?.length ?? 0); i++) {
    const m = movers[i], bs = DrawnMoverBoxes([m], t) ?? [];
    if (!bs.length) continue;
    const org = m.kind === "escalator" ? null : MoverOffset(m, t);
    for (const b of bs) out.push(org ? { ...b, org } : b);
  }
  return out;
}

// A static map box the movers stand in for (an escalator's static treads):
// not drawn (bullets and sight lines still use it).
export const replacedByMover = (b, movers) => !!movers?.length && Replaced(b, movers);

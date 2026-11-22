// Moving solids (content.Mover, map JSON "movers"): the boxes to draw at time
// t, in place of the map's static treads they replace. Where they are is the
// shared sim (internal/sim movers.go, generated into sim.gen.js), so what the
// renderer draws at a command's time is exactly what the body collides with.
import { DrawnMoverBoxes, MoverBoxes, Replaced } from "./sim.gen.js";

// Every mover box showing at time t (seconds on the movers' clock).
export const moverBoxes = (movers, t) => (movers?.length ? MoverBoxes(movers, t) ?? [] : []);

// The same boxes as drawn: escalator treads a centimetre low, none where they
// run level with the floor (they would z-fight with it). Collision: moverBoxes.
export const drawnMoverBoxes = (movers, t) => (movers?.length ? DrawnMoverBoxes(movers, t) ?? [] : []);

// A static map box the movers stand in for (an escalator's static treads):
// not drawn (bullets and sight lines still use it).
export const replacedByMover = (b, movers) => !!movers?.length && Replaced(b, movers);

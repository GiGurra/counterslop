// Boats on the client (internal/sim boat.go, the server's snapshot "boats"):
// each drawn where its last snapshot put it, run ahead by its velocity for
// the time since (at most 0.15 s), bobbing.
import { boatParts } from "../assets/models/boat.js";

export function createBoats() {
  let live = [], at = 0;
  return {
    get list() { return live; },
    ingest(list, now) { live = list || []; at = now; },
    // Where boat b is drawn at now (ms).
    drawnPos(b, now) {
      const dt = Math.min(0.15, Math.max(0, (now - at) / 1000));
      return [b.p[0] + b.v[0] * dt, b.p[1], b.p[2] + b.v[2] * dt];
    },
    // own: {id, p, y, v} the boat we drive, drawn there instead (predicted).
    solids(time, now, own = null) {
      return live.flatMap((b) => {
        const d = own && own.id === b.id ? own : { ...b, p: this.drawnPos(b, now) };
        return boatParts({ pos: d.p, yaw: d.y, time, id: b.id, speed: Math.hypot(d.v[0], d.v[2]) });
      });
    },
  };
}

// Boats on the client (internal/sim boat.go, the server's snapshot "boats"):
// each drawn where its last snapshot put it, run ahead by its velocity for
// the time since (at most 0.15 s), bobbing; a moving one's outboard buzzes
// (positioned, its revs from its speed) while the listener is in ENGINE_RANGE.
//   createBoats({ start(sp) -> handle {update(sp), stop()} | null, spatial(pos) -> {pan, gain} })
import { boatParts } from "../assets/models/boat.js";

export const ENGINE_RANGE = 45; // metres
const BOAT_MAX = 11; // m/s (internal/sim BoatMax): full revs

export function createBoats({ start = null, spatial = null } = {}) {
  let live = [], at = 0;
  const engines = new Map(); // boat id -> sound handle
  return {
    // Every frame: start, steer and stop the outboards (listener: where we hear from; null: nowhere).
    sound(listener, now) {
      const seen = new Set();
      for (const b of live) {
        const speed = Math.hypot(b.v[0], b.v[2]), p = this.drawnPos(b, now);
        const d = listener ? Math.hypot(p[0] - listener[0], p[1] - listener[1], p[2] - listener[2]) : Infinity;
        if (!start || speed < 0.3 || d > ENGINE_RANGE) continue;
        seen.add(b.id);
        const sp = spatial ? spatial(p) : { pan: 0, gain: 1 }, rev = Math.min(1, speed / BOAT_MAX);
        const out = { pan: sp.pan, gain: sp.gain * (0.25 + 0.35 * rev) * Math.min(1, (ENGINE_RANGE - d) / 6), rev };
        const h = engines.get(b.id);
        if (h) h.update(out);
        else { const n = start(out); if (n) engines.set(b.id, n); }
      }
      for (const [id, h] of engines) if (!seen.has(id)) { h.stop(); engines.delete(id); }
    },
    get engines() { return engines.size; },
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

// Ships (Star Slop's MVP-1, docs/starslop.md phase 3): presentation only.
// Where a ship is, is the shared sim (MoverOffset, generated from internal/sim):
// the rocket is drawn at our own predicted command time (moverTime in main.js),
// the other players at the (older) network view time.
import { MoverOffset, CycleT } from "../engine/sim.gen.js";

const inZone = (z, o, p) => p[0] >= z.min[0] + o[0] && p[0] <= z.max[0] + o[0] && p[1] + 0.9 >= z.min[1] + o[1] && p[1] + 0.9 < z.max[1] + o[1] &&
  p[2] >= z.min[2] + o[2] && p[2] <= z.max[2] + o[2];

// A remote standing in a ship's cabin (a gravity zone carried by a mover) at
// the view time tView is drawn where that cabin is at tDraw, the time the ship
// itself is drawn at: else, a rocket climbing at 30 m/s would leave its other
// passengers drawn metres under its floor. Remotes elsewhere are unchanged.
export function carriedRemotes(remotes, map, tView, tDraw) {
  const zones = (map?.gravZones || []).filter((z) => z.mover > 0 && map.movers?.[z.mover - 1]);
  if (!zones.length || tView === tDraw) return remotes;
  return remotes.map((r) => {
    for (const z of zones) {
      const m = map.movers[z.mover - 1], o0 = MoverOffset(m, tView);
      if (!inZone(z, o0, r.p)) continue;
      const o1 = MoverOffset(m, tDraw);
      return { ...r, p: [r.p[0] + o1[0] - o0[0], r.p[1] + o1[1] - o0[1], r.p[2] + o1[2] - o0[2]] };
    }
    return r;
  });
}

// The departure board for a player at pos at time t (the movers' clock): the
// nearest departure within 120 m of its door, or the one carrying the player.
// {name, state: "boarding" | "flight", in: seconds to the door closing
// (boarding) or to the next one docking (flight), aboard} or null.
export function departureHud(map, pos, t) {
  if (!pos) return null;
  let best = null, bd = Infinity;
  for (const d of map?.departures || []) {
    const m = map.movers?.[d.mover - 1];
    if (!m) continue;
    const zone = (map.gravZones || []).find((z) => z.mover === d.mover);
    const aboard = !!zone && inZone(zone, MoverOffset(m, t), pos);
    const dist = Math.hypot(pos[0] - d.door[0], pos[2] - d.door[2]);
    if (!aboard && dist > 120) continue;
    const c = CycleT(m, t), boarding = c < d.close;
    const hud = { name: d.name, state: boarding ? "boarding" : "flight", in: Math.max(0, Math.ceil(boarding ? d.close - c : m.period - c)), aboard };
    if (aboard) return hud;
    if (dist < bd) { best = hud; bd = dist; }
  }
  return best;
}

// The values live signs show (content.Sign live; assets/models/signs.js):
// "backlog:<team>" each team's tickets left (round state tk; "---" before), and
// "departure:<name>" the seconds until that ship's doors close, "GO" once
// they have (the T-minus clock).
export function signValues(map, t, st) {
  const v = { "backlog:vibe": "---", "backlog:artisan": "---" }; // before the match: no backlogs yet
  for (const [team, n] of Object.entries(st?.tk || {})) v[`backlog:${team}`] = String(n).padStart(3, " ");
  for (const d of map?.departures || []) {
    const m = map.movers?.[d.mover - 1];
    if (!m) continue;
    const c = CycleT(m, t);
    v[`departure:${d.name}`] = c < d.close ? String(Math.ceil(d.close - c)).padStart(2, "0") : "GO";
  }
  return v;
}

// The security scanners (content.Map.Scanners, a gag): enter(map, pos, now)
// is true when pos has just walked into one (feet in its box), at most once
// every coolS seconds: the beep and the TSA line.
export function createScannerGag(coolS = 8) {
  let inside = false, last = -Infinity;
  return {
    enter(map, pos, now) {
      const z = pos && (map?.scanners || []).find((b) => pos[0] >= b.min[0] && pos[0] <= b.max[0] && pos[1] >= b.min[1] && pos[1] < b.max[1] && pos[2] >= b.min[2] && pos[2] <= b.max[2]);
      const fire = !!z && !inside && now - last >= coolS;
      inside = !!z;
      if (fire) last = now;
      return fire;
    },
  };
}

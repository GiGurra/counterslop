// Ships (Star Slop's MVP-1, docs/starslop.md phase 3): presentation only.
// Where a ship is, is the shared sim (MoverOffset, generated from internal/sim):
// the rocket is drawn at our own predicted command time (moverTime in main.js),
// and so are its passengers (in its frame).
import { MoverOffset, CycleT } from "../engine/sim.gen.js";
import { byScore } from "../ui/scoreorder.js";

const inZone = (z, o, p) => p[0] >= z.min[0] + o[0] && p[0] <= z.max[0] + o[0] && p[1] + 0.9 >= z.min[1] + o[1] && p[1] + 0.9 < z.max[1] + o[1] &&
  p[2] >= z.min[2] + o[2] && p[2] <= z.max[2] + o[2];

// Remote passengers are drawn in their ship's frame (net.js sampleAt's ft:
// the server sends each rider's frame and place in it; docs/physics.md
// "Frames of reference"), where the ship itself is drawn.

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
//   "status:<name>" its line on the departures board: BOARDING, IN FLIGHT, or (its
//   team's backlog under a quarter: Earth is running out) DELAYED - BUDGET REVIEW,
//   a gag (the timetable is the timetable: the ship goes all the same);
//   "first:<team>" the team's top scorer (players: a full snapshot's), first class.
export const DELAYED_BELOW = 0.25;
export const delayed = (d, st) => !!d.team && st?.tkMax > 0 && st.tk?.[d.team] != null && st.tk[d.team] < DELAYED_BELOW * st.tkMax;
export function signValues(map, t, st, players = []) {
  const v = { "backlog:vibe": "---", "backlog:artisan": "---", "first:vibe": "TBD", "first:artisan": "TBD" }; // before the match: no backlogs yet
  for (const [team, n] of Object.entries(st?.tk || {})) v[`backlog:${team}`] = String(n).padStart(3, " ");
  for (const d of map?.departures || []) {
    const m = map.movers?.[d.mover - 1];
    if (!m) continue;
    const c = CycleT(m, t);
    v[`departure:${d.name}`] = c < d.close ? String(Math.ceil(d.close - c)).padStart(2, "0") : "GO";
    v[`status:${d.name}`] = delayed(d, st) ? "DELAYED - BUDGET REVIEW" : c < d.close ? "BOARDING" : "IN FLIGHT";
  }
  for (const team of ["vibe", "artisan"]) {
    const top = players.filter((p) => p.team === team).sort(byScore)[0];
    if (top && (top.sp || top.k)) v[`first:${team}`] = String(top.name).toUpperCase().slice(0, 16);
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

// Voice spots (content.Map.VoiceSpots, the client's alone): enter(map, pos,
// now) is the line id (the spot's name) when pos has just stepped into one
// (feet in its box), at most once every coolS seconds; else null. On
// cq_moonshot the Monument to the Postmortem sighs at whoever stands at its
// plinth.
export function createVoiceSpots(coolS = 20) {
  let inside = null, last = -Infinity;
  return {
    enter(map, pos, now) {
      const z = pos && (map?.voiceSpots || []).find((b) => pos[0] >= b.min[0] && pos[0] <= b.max[0] && pos[1] >= b.min[1] && pos[1] < b.max[1] && pos[2] >= b.min[2] && pos[2] <= b.max[2]);
      const fire = !!z && z !== inside && now - last >= coolS;
      inside = z || null;
      if (fire) last = now;
      return fire ? z.name : null;
    },
  };
}

// MVP-1's crew and the Moon's PA (Star Slop phase 5), presentation only: what
// to say this frame. step({map, pos, t, team, sky}) -> line ids (assets/voice/lines.js),
// from the departure cycle (the movers' clock t) and where we are:
//   near the gate (within 80 m of a door, not aboard): the gate agent's boarding
//     call as the ship docks (an apology for the delay instead when the board says
//     DELAYED: signValues), the final call 6 s before the doors close;
//   aboard: the captain as the doors close (then the cabin crew's safety demo),
//     the captain's descent 3 s before the pods fire, the crew arming them 1 s before;
//   the Moon (the "moon" sky zone, not aboard): HR's welcome for Earth's visitors
//     as they land, the return-to-office reminder for the Moon's staff (once a minute
//     at most for either).
// point(ev, team) -> line ids for a capture-point event ({ev, id, team, from}) heard by
//   team: HR tells the Moon's side when Earth breaks into one of its points (neutralised
//   from the Moon's), and when the Moon captures one it took back from Earth.
export function createCrew() {
  let prev = null, onMoon = false, hrAt = -Infinity;
  const earths = new Set(); // points last held by Earth (a Moon capture there is a reclaim)
  return {
    step({ map, pos, t, team, sky, now = t, st = null }) {
      const out = [];
      const dh = departureHud(map, pos, t);
      const d = dh && map.departures.find((x) => x.name === dh.name), m = d && map.movers[d.mover - 1];
      if (d && m) {
        const c = CycleT(m, t), p = prev && prev.name === d.name ? prev.c : null;
        // crossed(at): the cycle passed at since the last frame (across its wrap too)
        const crossed = (at) => p != null && (p <= c ? p < at && at <= c : at > p || at <= c);
        const docked = p != null && c < p; // the cycle wrapped: back at the dock, doors open
        const pods = (map.launchers || []).find((l) => l.mover === d.mover);
        const nearGate = !dh.aboard && Math.hypot(pos[0] - d.door[0], pos[2] - d.door[2]) < 80;
        if (nearGate && docked) out.push(delayed(d, st) ? "gate_delayed" : "gate_boarding");
        if (nearGate && crossed(d.close - 6)) out.push("gate_final");
        if (dh.aboard && crossed(d.close)) out.push("captain_welcome", "safety_demo");
        if (dh.aboard && pods && crossed(pods.from - 4)) out.push("crew_pods"); // the drop's one line (the captain's descent was 2 s before it, so this one came late)
        prev = { name: d.name, c };
      } else prev = null;
      const moon = sky === "moon" && !dh?.aboard;
      if (moon && !onMoon && now - hrAt >= 60) {
        out.push(team === "artisan" ? "hr_rto" : "hr_welcome");
        hrAt = now;
      }
      onMoon = moon;
      return out;
    },
    point(ev, team) {
      if (!ev?.id) return [];
      if (ev.ev === "neutral" && ev.from === "vibe") earths.add(ev.id); // the Moon took it off Earth
      if (team !== "artisan") return [];
      if (ev.ev === "neutral" && ev.from === "artisan" && ev.team === "vibe") return ["hr_breach"];
      if (ev.ev === "captured" && ev.team === "artisan" && earths.delete(ev.id)) return ["hr_reclaimed"];
      return [];
    },
  };
}

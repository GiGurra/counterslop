// Music director: which loop plays for the round phase, and which stinger an event
// gets. The one rule: no loop under live play (live, planted); only stingers. But a race.
//   update({ ph, buyMenu, mvp, mode, n, map, race })   every frame: round phase, buy menu open, the MVP's name,
//                                  the game mode (round state; "gpudrop" or none = defusal), the round number,
//                                  the map's id and the race HUD state (race mode; null otherwise)
// A race (mode "race") is the exception: its live phase plays the map's race track
// (assets/music RACE_MAP_TRACKS: map id -> track; a parkour course on foot "parkour"),
// the race's number in the match picking the variation (1..3, then again), and our
// final lap (a parkour run: its last stretch, the last gate to the finish) lifting it
// (faster, a step up, heavier drums), cut in at the next bar line. From the race HUD
// (game/race.js raceHud) the director also plays the race's stingers, once per
// change in state (so a duplicate or lost snapshot neither repeats nor loses one):
// "lap" over the line into a new lap, "finalLap" into the last one (or a run's last
// stretch), "finish" (by place) over the finish line.
// So is a ride (Star Slop, mode "tickets", ride: aboard a ship in flight): the space opera "liftoff",
// with the "launch" stinger as the doors shut; tickets mode's warmup and freeze play its "overture".
//   kill({ multi, headshot })      the local player's kill (multi from the announcer)
//   planted() / defused() / secured() / roundEnd(won) / matchEnd() -> whether music is on (played it)
// GPU Drop themes it: store muzak instead of the buy-time loop, the Black Friday jingle for a win.
import { RACE_MAP_TRACKS, DEFAULT_RACE_TRACK, PARKOUR_TRACK, RACE_VARIATIONS, raceLoopId } from "../assets/music/themes.js";

export const MVP_DELAY = 2.2; // seconds into the round-over phase: after the win/loss stinger

// The race track a map plays: its own, else parkour on a course (point to point, on
// foot) or a pk_ map, else the default.
export function raceTrackFor(map = "", race = null) {
  return RACE_MAP_TRACKS[map] ?? (race?.p2p || map.startsWith("pk_") ? PARKOUR_TRACK : DEFAULT_RACE_TRACK);
}

// Whether the race's music lifts: our final lap of a race of laps (more than one),
// a course's last stretch (past the second-to-last gate; the finish next), and
// after we finish, until the race is over.
export function raceLift(race) {
  if (!race) return false;
  if (race.p2p) return race.gates > 1 && race.gate >= race.gates - 1;
  return race.laps > 1 && race.lap >= race.laps;
}

export function loopFor({ ph, buyMenu = false, mvp = "", overFor = 0, mode = "", n = 1, ride = false, map = "", race = null }) {
  const buy = mode === "gpudrop" ? "muzak" : mode === "tickets" ? "overture" : "buytime";
  if (mode === "race" && ph === "live") { // a race: the map's own track
    return raceLoopId(raceTrackFor(map, race), ((Math.max(1, n | 0) - 1) % RACE_VARIATIONS) + 1, raceLift(race));
  }
  if (mode === "tickets" && ride && ph === "live") return "liftoff"; // aboard a ship in flight (Star Slop): the ride's space opera
  switch (ph) {
    case "warmup": return buyMenu ? buy : "warmup";
    case "freeze": case "halftime": return buy;
    case "over": return mvp && overFor >= MVP_DELAY ? `mvp:${mvp}` : null;
    case "matchover": return "matchend";
    default: return null; // live, planted, no round yet
  }
}

export function createMusicDirector({ music, now = () => performance.now() / 1000 }) {
  let ph, since = 0, mode = "", riding = false, loop = null;
  // The race as last seen (null: none, or a new one): the stingers fire on a change of
  // it. The first sight of a race (joined mid-race, a reload) fires nothing.
  let seen = null;
  const raceSaid = []; // the race stingers played, in order (the last 20; state())
  const sting = (id, arg) => { music.stinger(id, arg); raceSaid.push(arg === undefined ? id : `${id}:${arg}`); if (raceSaid.length > 20) raceSaid.shift(); };
  function raceStingers(race) {
    if (!race || race.results) { seen = null; return; }
    const lap = race.p2p ? 0 : race.lap | 0, gate = race.gate | 0, fin = race.finished | 0;
    if (!seen || race.countdown > 0 || lap < seen.lap || gate < seen.gate) { seen = { lap, gate, fin }; return; } // a new race: from here
    if (!race.p2p && lap > seen.lap && seen.lap > 0 && !fin) { if (lap === race.laps) sting("finalLap"); else sting("lap", lap); }
    if (race.p2p && gate > seen.gate && seen.gate > 0 && race.gates > 2 && gate === race.gates - 1) sting("finalLap");
    if (fin && !seen.fin) sting("finish", fin);
    seen = { lap, gate, fin };
  }
  return {
    update({ ph: p = null, buyMenu, mvp, mode: m = "", n = 1, ride = false, map = "", race = null } = {}) {
      if (p !== ph) { ph = p; since = now(); }
      mode = m || "";
      if (ride && !riding && mode === "tickets" && ph === "live") music.stinger("launch"); // the doors shut with us aboard: liftoff
      riding = ride;
      raceStingers(mode === "race" ? race : null);
      loop = loopFor({ ph, buyMenu, mvp, overFor: now() - since, mode, n, ride, map, race });
      music.setLoop(loop);
    },
    // What the director chose (the test API): the loop and the race stingers played.
    state: () => ({ loop, raceSaid: [...raceSaid] }),
    kill({ multi = 1, headshot = false } = {}) {
      music.stinger("kill", multi);
      if (headshot) music.stinger("headshot");
    },
    planted: () => music.stinger("plant"),
    defused: () => music.stinger("defuse"),
    secured: () => music.stinger("secure"),
    roundEnd: (won) => music.stinger(won ? (mode === "gpudrop" ? "blackFriday" : "roundWon") : "roundLost"),
    matchEnd: () => music.state().on, // the match-end loop is its music; true = skip the sfx fanfare
  };
}

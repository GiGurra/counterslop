// Music director: which loop plays for the round phase, and which stinger an event
// gets. The one rule: no loop under live play (live, planted); only stingers. But a race.
//   update({ ph, buyMenu, mvp, mode, n })   every frame: round phase, buy menu open, the MVP's name,
//                                  the game mode (round state; "gpudrop" or none = defusal), the round number
// A race (mode "race") is the exception: its live phase plays a race track (race1..3 by the race's number).
// So is a ride (Star Slop, mode "tickets", ride: aboard a ship in flight): the space opera "liftoff",
// with the "launch" stinger as the doors shut; tickets mode's warmup and freeze play its "overture".
//   kill({ multi, headshot })      the local player's kill (multi from the announcer)
//   planted() / defused() / secured() / roundEnd(won) / matchEnd() -> whether music is on (played it)
// GPU Drop themes it: store muzak instead of the buy-time loop, the Black Friday jingle for a win.
export const MVP_DELAY = 2.2; // seconds into the round-over phase: after the win/loss stinger

export const RACE_TRACKS = 3; // themes.js race1..race3, in turn race by race

export function loopFor({ ph, buyMenu = false, mvp = "", overFor = 0, mode = "", n = 1, ride = false }) {
  const buy = mode === "gpudrop" ? "muzak" : mode === "tickets" ? "overture" : "buytime";
  if (mode === "race" && ph === "live") return `race${((Math.max(1, n | 0) - 1) % RACE_TRACKS) + 1}`; // a race: its own rock track
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
  let ph, since = 0, mode = "", riding = false;
  return {
    update({ ph: p = null, buyMenu, mvp, mode: m = "", n = 1, ride = false } = {}) {
      if (p !== ph) { ph = p; since = now(); }
      mode = m || "";
      if (ride && !riding && mode === "tickets" && ph === "live") music.stinger("launch"); // the doors shut with us aboard: liftoff
      riding = ride;
      music.setLoop(loopFor({ ph, buyMenu, mvp, overFor: now() - since, mode, n, ride }));
    },
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

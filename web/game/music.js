// Music director: which loop plays for the round phase, and which stinger an event
// gets. The one rule: no loop under live play (live, planted); only stingers.
//   update({ ph, buyMenu, mvp, mode })   every frame: round phase, buy menu open, the MVP's name,
//                                  the game mode (round state; "gpudrop" or none = defusal)
//   kill({ multi, headshot })      the local player's kill (multi from the announcer)
//   planted() / defused() / secured() / roundEnd(won) / matchEnd() -> whether music is on (played it)
// GPU Drop themes it: store muzak instead of the buy-time loop, the Black Friday jingle for a win.
export const MVP_DELAY = 2.2; // seconds into the round-over phase: after the win/loss stinger

export function loopFor({ ph, buyMenu = false, mvp = "", overFor = 0, mode = "" }) {
  const buy = mode === "gpudrop" ? "muzak" : "buytime";
  switch (ph) {
    case "warmup": return buyMenu ? buy : "warmup";
    case "freeze": case "halftime": return buy;
    case "over": return mvp && overFor >= MVP_DELAY ? `mvp:${mvp}` : null;
    case "matchover": return "matchend";
    default: return null; // live, planted, no round yet
  }
}

export function createMusicDirector({ music, now = () => performance.now() / 1000 }) {
  let ph, since = 0, mode = "";
  return {
    update({ ph: p = null, buyMenu, mvp, mode: m = "" } = {}) {
      if (p !== ph) { ph = p; since = now(); }
      mode = m || "";
      music.setLoop(loopFor({ ph, buyMenu, mvp, overFor: now() - since, mode }));
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

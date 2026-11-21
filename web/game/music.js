// Music director: which loop plays for the round phase, and which stinger an event
// gets. The one rule: no loop under live play (live, planted); only stingers.
//   update({ ph, buyMenu, mvp })   every frame: round phase, buy menu open, the MVP's name
//   kill({ multi, headshot })      the local player's kill (multi from the announcer)
//   planted() / defused() / roundEnd(won) / matchEnd() -> whether music is on (played it)
export const MVP_DELAY = 2.2; // seconds into the round-over phase: after the win/loss stinger

export function loopFor({ ph, buyMenu = false, mvp = "", overFor = 0 }) {
  switch (ph) {
    case "warmup": return buyMenu ? "buytime" : "warmup";
    case "freeze": case "halftime": return "buytime";
    case "over": return mvp && overFor >= MVP_DELAY ? `mvp:${mvp}` : null;
    case "matchover": return "matchend";
    default: return null; // live, planted, no round yet
  }
}

export function createMusicDirector({ music, now = () => performance.now() / 1000 }) {
  let ph, since = 0;
  return {
    update({ ph: p = null, buyMenu, mvp } = {}) {
      if (p !== ph) { ph = p; since = now(); }
      music.setLoop(loopFor({ ph, buyMenu, mvp, overFor: now() - since }));
    },
    kill({ multi = 1, headshot = false } = {}) {
      music.stinger("kill", multi);
      if (headshot) music.stinger("headshot");
    },
    planted: () => music.stinger("plant"),
    defused: () => music.stinger("defuse"),
    roundEnd: (won) => music.stinger(won ? "roundWon" : "roundLost"),
    matchEnd: () => music.state().on, // the match-end loop is its music; true = skip the sfx fanfare
  };
}

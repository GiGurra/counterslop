// Kill-streak announcer: turns kill events into announcer lines. The trigger API is
// small on purpose; only the server's kill events call it (game/main.js case "kill"):
//   onKill({ headshot })   a kill by the local player
//   onDeath()              the local player died (ends the spree)
//   onRoundStart(silent)   new round (resets the "first" kill); silent: no opening line (a race: the caller has the grid)
// Lines and their phonemes are data in assets/voice/lines.js.
import { say, MULTI, SPREES } from "../assets/voice/index.js";

const MULTI_WINDOW = 4; // seconds between kills that still count as one streak

export function createAnnouncer({ now = () => performance.now() / 1000 } = {}) {
  let multi = 0, spree = 0, lastKill = -Infinity, firstDone = false;
  return {
    onKill({ headshot = false } = {}) {
      const t = now();
      multi = t - lastKill <= MULTI_WINDOW ? multi + 1 : 1;
      lastKill = t; spree++;
      const lines = [];
      if (multi >= 2) lines.push(MULTI[Math.min(multi, MULTI.length - 1)]);
      else if (headshot) lines.push("brain_dump");
      else if (!firstDone) lines.push("first_bug");
      const sp = SPREES.find(([n]) => n === spree);
      if (sp) lines.push(sp[1]);
      firstDone = true;
      for (const l of lines) say(l);
      return lines;
    },
    onDeath() { spree = 0; multi = 0; },
    onRoundStart(silent = false) { firstDone = false; multi = 0; if (!silent) say("prompt_locked"); },
    get state() { return { multi, spree, firstDone }; },
  };
}

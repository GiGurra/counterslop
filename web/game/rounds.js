// Client side of rounds and The Big Diff: the server owns the state machine
// (internal/game/rounds.go); this turns its snapshots ("round" in every snap)
// and events ({t:"round", ev}) into HUD state, sounds, announcer lines and
// what to draw (the Big Diff, the merge shockwave, screen shake).
import { bigDiffParts, carriedParts, shockwaveParts, BLAST_TIME } from "../assets/models/bigdiff.js";
import { ROUND_END, LINES } from "../assets/voice/index.js";
import { team as teamInfo } from "../assets/teams.js";

const inSite = (s, p) => p[0] >= s.min[0] && p[0] <= s.max[0] && p[1] >= s.min[1] && p[1] <= s.max[1] && p[2] >= s.min[2] && p[2] <= s.max[2];

// deps: {hud, sfx, say, announcer, spatial(p), feed(text), me() -> {id, team, pos, dead} , nameOf(id), c4time() -> s}
export function createRounds(deps) {
  const { hud, sfx, say, announcer, spatial, feed, me, nameOf, c4time } = deps;
  let st = null, at = 0; // last round snapshot and when it arrived (ms)
  let blast = null, shake = 0, nextBeep = 0, lastHud = "", mvpKey = "";

  // Seconds left in the phase, counted down locally between snapshots.
  const left = (now) => (st ? Math.max(0, st.left - (now - at) / 1000) : 0);

  const r = {
    get state() { return st; },
    // Freeze time: no moving, jumping or shooting (the server ignores them; don't predict them).
    frozen: () => st?.ph === "freeze",

    ingest(round, now) {
      st = round; at = now;
      // The MVP rides in the round state (a lost "end" event doesn't lose it).
      const key = round.ph === "over" && round.mvp ? `${round.n}:${round.mvp}` : "";
      if (key !== mvpKey) {
        mvpKey = key;
        const p = key && nameOf(round.mvp);
        hud.set({ mvp: key ? { name: p, why: round.mvpWhy || "", me: round.mvp === me().id } : null });
      }
    },

    event(m, now) {
      const my = me();
      switch (m.ev) {
        case "start":
          blast = null;
          announcer.onRoundStart();
          feed(`Round ${m.n}. ${teamInfo("vibe").name} ${m.sc?.vibe ?? 0} : ${m.sc?.artisan ?? 0} ${teamInfo("artisan").name}`);
          if (m.by && m.by === my.id) feed("You carry The Big Diff. Force-push it at site A or B (hold E).");
          hud.set({ roundEnd: null, dmgReport: null });
          break;
        case "warmup":
          feed("Warmup: rounds start when both sides have players.");
          hud.set({ roundEnd: null });
          break;
        case "pickup":
          if (m.by === my.id) feed("You picked up The Big Diff.");
          break;
        case "dropped":
          feed("The Big Diff was dropped!");
          break;
        case "planted":
          sfx.plantClick(spatial(m.p));
          say("force_push");
          feed(`${nameOf(m.by)} force-pushed The Big Diff to ${m.site}. CI is running...`);
          nextBeep = 0;
          break;
        case "defused":
          sfx.reverted();
          feed(`${nameOf(m.by)} reverted The Big Diff.`);
          break;
        case "exploded":
          sfx.mergeBoom(spatial(m.p));
          blast = { p: m.p, at: now };
          if (my.pos) {
            const d = Math.hypot(my.pos[0] - m.p[0], my.pos[2] - m.p[2]);
            shake = Math.max(0.25, 1.2 - d / 40);
          } else shake = 0.4;
          hud.set({ flash: now });
          break;
        case "end": {
          const line = ROUND_END[m.win]?.[m.why];
          if (LINES[`${m.win}_win`]) say(`${m.win}_win`);
          const t = teamInfo(m.win);
          sfx.roundEnd(!my.team || my.team === "spec" || my.team === m.win);
          hud.set({ roundEnd: { win: m.win, name: t?.name ?? m.win, color: t?.color, text: LINES[line]?.text.replace(/!$/, "") ?? "", n: m.n } });
          feed(`${t?.name ?? m.win} win: ${LINES[line]?.text ?? m.why}`);
          break;
        }
      }
    },

    // Per frame: CI beeps while planted, screen shake decay, HUD state.
    update(dt, now, map) {
      shake = Math.max(0, shake - dt * 1.2);
      if (!st) return;
      const my = me(), b = st.bomb || {};
      const l = left(now);
      if (st.ph === "planted" && now >= nextBeep) {
        const urgency = 1 - Math.min(1, l / c4time());
        sfx.ciBeep(spatial(b.p), urgency);
        nextBeep = now + 1000 * Math.max(0.1, 1 - urgency * 0.95);
      }
      // Plant / defuse progress and the carrier's hints.
      let work = null, hint = "";
      if (b.who === my.id && b.pr > 0) work = { kind: st.ph === "planted" ? "revert" : "push", pr: b.pr };
      else if (b.who && b.pr > 0) work = { kind: st.ph === "planted" ? "revert" : "push", pr: b.pr, other: nameOf(b.who) };
      const carrying = b.st === "carried" && b.by === my.id && !my.dead;
      if (carrying && !work && my.pos && st.ph === "live") {
        const site = (map?.sites || []).find((s) => inSite(s, my.pos));
        hint = site ? `Site ${site.name}: hold E to force-push The Big Diff` : "You carry The Big Diff: get it to site A or B";
      }
      if (st.ph === "planted" && my.team === "artisan" && !my.dead && !work && my.pos && Math.hypot(my.pos[0] - b.p[0], my.pos[2] - b.p[2]) < 1.8) hint = "Hold E to git revert";
      const h = {
        phase: st.ph, roundLeft: Math.ceil(l), score: st.sc,
        ci: st.ph === "planted" ? { left: Math.ceil(l), frac: Math.min(1, 1 - l / c4time()), site: b.site } : null,
        work: work && { ...work, pr: Math.round(work.pr * 50) / 50 }, hint, carrying,
      };
      const key = JSON.stringify(h);
      if (key !== lastHud) { lastHud = key; hud.set(h); }
    },

    // Camera shake offset for this frame (metres).
    shakeEye(eye) {
      if (shake <= 0) return eye;
      const s = shake * 0.25;
      return [eye[0] + (Math.random() - 0.5) * s, eye[1] + (Math.random() - 0.5) * s, eye[2] + (Math.random() - 0.5) * s];
    },

    // What to draw: the Big Diff (carried on someone's back, dropped or planted) and the shockwave.
    // hideCarrier: the player whose back we don't draw (yourself in first person).
    solids(players, time, now, hideCarrier) {
      const out = [];
      if (blast && (now - blast.at) / 1000 > BLAST_TIME) blast = null;
      if (blast) out.push(...shockwaveParts(blast.p, (now - blast.at) / 1000));
      const b = st?.bomb;
      if (!b?.st || st.ph === "warmup") return out;
      if (b.st === "carried") {
        const c = players.find((p) => p.id === b.by);
        if (c && c.id !== hideCarrier) out.push(...carriedParts(c.p, c.yaw, !!c.c));
      } else if (b.st !== "exploded") {
        out.push(...bigDiffParts({ pos: b.p, yaw: 0.4, time, planted: b.st === "planted", left: left(now), total: c4time(), progress: st.ph === "planted" ? b.pr || 0 : 0 }));
      }
      return out;
    },
  };
  return r;
}

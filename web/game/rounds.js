// Client side of rounds and The Big Diff: the server owns the state machine
// (internal/game/rounds.go); this turns its snapshots ("round" in every snap)
// and events ({t:"round", ev}) into HUD state, sounds, announcer lines and
// what to draw (the Big Diff, the merge shockwave, screen shake).
import { createRaceAnnouncer } from "./raceannouncer.js";
import { bigDiffParts, carriedParts, shockwaveParts, BLAST_TIME } from "../assets/models/bigdiff.js";
import { torsoFrame } from "../assets/models/player.js";
import { ROUND_END, LINES } from "../assets/voice/index.js";
import { team as teamInfo } from "../assets/teams.js";
import { createGpuDrop, gpuHud, gpuSolids, endOf } from "./gpudrop.js";
import { ticketsHud, endText as ticketEnd } from "./tickets.js";
import { raceHud, raceEnd } from "./race.js";

const inSite = (s, p) => p[0] >= s.min[0] && p[0] <= s.max[0] && p[1] >= s.min[1] && p[1] <= s.max[1] && p[2] >= s.min[2] && p[2] <= s.max[2];

// deps: {hud, sfx, say, announcer, spatial(p), feed(text), me() -> {id, team, pos, dead} , nameOf(id), c4time() -> s,
//        showC4Timer() -> bool (mp_c4_show_timer; default: no countdown, as in CS), music (game/music.js director; optional), teamOf(id) (GPU Drop: who is grabbing vs restocking),
//        takeName() -> the floor weapon E would take ("" none; optional),
//        self() -> the last self-snapshot, deployWant() -> our spawn / kit choice (tickets mode; optional)}
// The planted Big Diff's beep tempo, seconds to the next beep with l seconds
// left of total. Without a countdown on the HUD (mp_c4_show_timer 0) the
// beeps are the clock, so the tempo steps at the times a retake decides on: a
// revert takes 8 s (rounds.go defuseSecs), 4 s with a kit. Above 8 s it speeds
// up smoothly from 1 s to 0.6 s; at 8 s it jumps to 0.3 s (too late without a
// kit), at 4 s to 0.15 s (too late with one), and the last second is a buzz.
export const REVERT_SECS = 8, REVERT_KIT_SECS = 4;
export function beepInterval(l, total) {
  if (l <= 1) return 0.07;
  if (l <= REVERT_KIT_SECS) return 0.15;
  if (l <= REVERT_SECS) return 0.3;
  const span = total - REVERT_SECS;
  const u = span > 0 ? Math.min(1, Math.max(0, (total - l) / span)) : 1;
  return 1 - 0.4 * u;
}

const NO_MUSIC = { update() {}, planted() {}, defused() {}, roundEnd: () => false, matchEnd: () => false };
export function createRounds(deps) {
  const { hud, sfx, say, announcer, spatial, feed, me, nameOf, c4time } = deps;
  const showC4Timer = deps.showC4Timer || (() => false);
  const music = deps.music || NO_MUSIC;
  const gpus = createGpuDrop({ sfx, say, spatial, feed, nameOf, me, music }); // GPU Drop's sounds and lines (gpudrop.js)
  const raceSaid = []; // the race caller's lines in order (the last 30; the test API's state().raceSaid)
  const raceCaller = createRaceAnnouncer({ say: (id) => { raceSaid.push(id); if (raceSaid.length > 30) raceSaid.shift(); say(id); } }); // the race mode's caller: the grid, the lead, wrecks, the finish
  const gpuMode = () => st?.mode === "gpudrop";
  let st = null, endAt = 0, endKey = ""; // last round snapshot; when its phase ends (local ms)
  let buyAt = 0; // when buy time ends (local ms; 0: no limit, warmup, or over), kept like endAt
  let rtAt = 0; // race mode: when the race clock was 0 (local ms), kept like endAt
  let blast = null, shake = 0, nextBeep = 0, beepAt = -Infinity, beeps = 0, lastHud = "", mvpKey = "";

  // Seconds left in the phase, counted down locally. Each snapshot implies an
  // end time (arrival + left); network jitter moves that by up to ~±100 ms, so
  // the end time is kept unless the phase or round changed or it moved by
  // more than ENDSLACK (a pause, a cvar change, a real clock step). Taking
  // every snapshot as is made the shown seconds tick back up (88 87 88) and the
  // centred top bar jump sideways with the digits' width.
  const ENDSLACK = 400; // ms
  // Frames can be stamped a few ms before the latest one (input-driven
  // frames), so within a phase the shown time only ever goes down.
  let latest = -Infinity;
  const left = (now) => { latest = Math.max(latest, now); return st ? Math.max(0, (endAt - latest) / 1000) : 0; };

  const r = {
    get state() { return st; },
    get raceSaid() { return raceSaid; },
    // Freeze time: no moving, jumping or shooting (the server ignores them; don't predict them).
    frozen: () => st?.ph === "freeze",
    // The race clock (s since GO, counted locally; the HUD's, unrounded): the ghost's time (ghost.js).
    raceClock: (now) => (st?.ph === "live" && rtAt ? Math.max(0, (Math.max(latest, now) - rtAt) / 1000) : st?.rt || 0),

    ingest(round, now) {
      st = round;
      gpus.ingest(round);
      // No timer (left 0: warmup without mp_warmup_time) ends "long ago", so a
      // frame stamped a few ms before this arrival (input-driven) can't show 0:01.
      const end = round.left > 0 ? now + round.left * 1000 : -Infinity, phaseKey = `${round.n}:${round.ph}`;
      if (phaseKey !== endKey || !(Math.abs(end - endAt) <= ENDSLACK)) { endAt = end; endKey = phaseKey; }
      const go = round.rt > 0 ? now - round.rt * 1000 : 0; // the race clock: counted locally between snapshots
      if (!go || !rtAt || Math.abs(go - rtAt) > ENDSLACK) rtAt = go;
      const buyEnd = round.buy > 0 ? now + round.buy * 1000 : 0;
      if (!buyEnd || !buyAt || Math.abs(buyEnd - buyAt) > ENDSLACK) buyAt = buyEnd;
      // The MVP rides in the round state (a lost "end" event doesn't lose it).
      const key = round.ph === "over" && round.mvp ? `${round.n}:${round.mvp}` : "";
      if (key !== mvpKey) {
        mvpKey = key;
        const p = key && nameOf(round.mvp);
        hud.set({ mvp: key ? { name: p, why: round.mvpWhy || "", me: round.mvp === me().id } : null });
      }
    },

    // {t:"gpu"} events (GPU Drop): sounds, lines and the feed only.
    gpuEvent(m) { gpus.event(m); },

    event(m, now) {
      const my = me();
      switch (m.ev) {
        case "start":
          blast = null;
          announcer.onRoundStart(st?.mode === "race"); // a race: the race caller has the grid (raceannouncer.js)
          feed(`Round ${m.n}. ${teamInfo("vibe").name} ${m.sc?.vibe ?? 0} : ${m.sc?.artisan ?? 0} ${teamInfo("artisan").name}`);
          if (m.by && m.by === my.id) feed("You carry The Big Diff (5). At site A or B: hold E (or draw it and hold fire) to force-push.");
          if (gpuMode() && my.team === "vibe") feed(`Scalp ${st.toWin || 2} RTX 9090s: hold E at the shelves, carry them out.`);
          if (gpuMode() && my.team === "artisan") feed("Guard the stock: the scalpers want the RTX 9090s. Hold E on a dropped one to restock it.");
          if (st?.mode === "race" && st.p2p) feed("Every gate in order, start to finish, on foot: jump, hook, fly. Fall off and you're back on the last gate at once. Your splits race your best run.");
          else if (st?.mode === "race") feed("Every gate in order, lap after lap: the first over the line wins. Fall off the track and you're back on the last gate in 3 s.");
          if (st?.mode === "tickets") feed("Every respawn closes a ticket from your team's backlog. Hold more points than them to burn theirs down; a team at 0 has shipped everything and loses.");
          hud.set({ roundEnd: null, dmgReport: null, halftime: null, matchEnd: null });
          break;
        case "warmup":
          feed("Warmup: the match starts when the warmup timer runs out (with a player on each side).");
          hud.set({ roundEnd: null, halftime: null, matchEnd: null });
          break;
        case "halftime":
          feed(`Halftime: ${teamInfo("vibe").name} ${m.sc?.vibe ?? 0} : ${m.sc?.artisan ?? 0} ${teamInfo("artisan").name}. Sides switch.`);
          hud.set({ roundEnd: null, halftime: { sc: m.sc } });
          break;
        case "swap": // main.js has already moved us (and everyone it knows) to the other side
          if (teamInfo(my.team)) feed(`Sides switched: you are now with the ${teamInfo(my.team).name}. The score follows you.`);
          break;
        case "matchend": {
          const draw = m.win === "draw", t = teamInfo(m.win);
          if (!draw && LINES[`${m.win}_win`]) say(`${m.win}_win`);
          if (!music.matchEnd()) sfx.roundEnd(draw || !my.team || my.team === "spec" || my.team === m.win);
          hud.set({ roundEnd: null, matchEnd: { win: m.win, name: draw ? "" : t?.name ?? m.win, color: t?.color, sc: m.sc } });
          feed(draw ? `The match is a draw, ${m.sc?.vibe ?? 0} : ${m.sc?.artisan ?? 0}.` : `${t?.name ?? m.win} win the match ${m.sc?.[m.win] ?? 0} : ${m.sc?.[m.win === "vibe" ? "artisan" : "vibe"] ?? 0}!`);
          break;
        }
        case "pickup":
          if (m.by === my.id) feed("You picked up The Big Diff.");
          break;
        case "dropped":
          feed("The Big Diff was dropped!");
          break;
        case "planted":
          sfx.plantClick(spatial(m.p));
          say("force_push");
          music.planted();
          feed(`${nameOf(m.by)} force-pushed The Big Diff to ${m.site}. CI is running...`);
          nextBeep = 0;
          break;
        case "defused":
          sfx.reverted();
          music.defused();
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
          const t = teamInfo(m.win);
          const won = !my.team || my.team === "spec" || my.team === m.win;
          let text, say1;
          if (gpuMode()) { // GPU Drop: the store's lines ("Scalpers win!" / "Staff wins!")
            const e = endOf(m.win, m.why);
            text = e.text || m.why; say1 = e.line;
          } else if (st?.mode === "race") {
            const e = raceEnd(st, nameOf);
            if (e) { // the winner is a racer, not a team: the fanfare is the winner's alone
              const first = st.race[0].id === my.id;
              if (!music.roundEnd(first)) sfx.roundEnd(first);
              hud.set({ roundEnd: { win: m.win, name: e.name, title: `${e.name.toUpperCase()} WINS!`, color: t?.color, text: e.text, n: m.n } });
              feed(`${e.name} wins the race: ${e.text}`);
              break;
            }
            text = m.why; say1 = `${m.win}_win`;
          } else if (st?.mode === "tickets") {
            text = ticketEnd(m.win, m.why) || m.why; say1 = `${m.win}_win`;
          } else {
            const line = ROUND_END[m.win]?.[m.why];
            text = LINES[line]?.text.replace(/!$/, "") ?? ""; say1 = `${m.win}_win`;
          }
          if (LINES[say1]) say(say1);
          if (!music.roundEnd(won)) sfx.roundEnd(won); // the music stinger replaces the sfx fanfare
          hud.set({ roundEnd: { win: m.win, name: t?.name ?? m.win, color: t?.color, text, n: m.n } });
          feed(`${t?.name ?? m.win} win: ${text || m.why}`);
          break;
        }
      }
    },

    // Per frame: CI beeps while planted, screen shake decay, HUD state.
    update(dt, now, map) {
      shake = Math.max(0, shake - dt * 1.2);
      // Music follows the phase: loops in warmup/freeze/over/matchover, none live.
      const hs = hud.get?.() || {}; // the main menu (not in a match): no music
      music.update({ ph: st && hs.screen !== "menu" ? st.ph : null, mode: st?.mode, n: st?.n, buyMenu: !!hs.buyMenu,
        ride: !!hs.departure?.aboard && hs.departure.state === "flight", // aboard MVP-1 in flight (ships.js departureHud)
        mvp: st?.ph === "over" && st.mvp ? nameOf(st.mvp) : "" });
      if (!st) return;
      const my = me(), b = st.bomb || {};
      const l = left(now);
      if (st.ph === "planted" && now >= nextBeep) {
        const urgency = 1 - Math.min(1, l / c4time());
        sfx.ciBeep(spatial(b.p), urgency);
        nextBeep = now + 1000 * beepInterval(l, c4time());
        beepAt = now; beeps++; // the light on the slab and the HUD badge flash with it
      }
      // Plant / defuse progress and the carrier's hints.
      let work = null, hint = "";
      if (b.who === my.id && b.pr > 0) work = { kind: st.ph === "planted" ? "revert" : "push", pr: b.pr };
      else if (b.who && b.pr > 0) work = { kind: st.ph === "planted" ? "revert" : "push", pr: b.pr, other: nameOf(b.who) };
      const carrying = b.st === "carried" && b.by === my.id && !my.dead;
      if (carrying && !work && my.pos && st.ph === "live") {
        const site = (map?.sites || []).find((s) => inSite(s, my.pos));
        const drawn = my.held === "bomb";
        hint = site ? (drawn ? `Site ${site.name}: hold fire or E to force-push The Big Diff` : `Site ${site.name}: hold E to force-push The Big Diff`)
          : drawn ? "Plant at site A or B" : "You carry The Big Diff (5): get it to site A or B";
      }
      // E: defuse > take a floor weapon > plant (server runCmd / roundUse).
      const defuse = st.ph === "planted" && my.team === "artisan" && !my.dead && my.pos && Math.hypot(my.pos[0] - b.p[0], my.pos[2] - b.p[2]) <= 1.8 && Math.abs(my.pos[1] - b.p[1]) <= 1.5;
      if (defuse && !work) hint = "Hold E to git revert";
      const take = !defuse && !work && deps.takeName?.();
      let gpu = null;
      if (gpuMode()) { // GPU Drop: no Big Diff; the counter, the grab / restock bar and its hints
        const g = gpuHud(st, my, map, deps.teamOf);
        gpu = g.gpu; hint = g.hint;
        work = g.work && { ...g.work, ...(g.work.other ? { other: nameOf(g.work.other) } : {}) };
      }
      if (take) hint = `E: take ${take}`;
      const tickets = ticketsHud(st, my, deps.self?.(), map, deps.deployWant?.()); // tickets mode (null otherwise)
      const clock = st.ph === "live" && rtAt ? Math.max(0, (latest - rtAt) / 1000) : st.rt || 0;
      const race = raceHud(st, my, map, nameOf, l, clock); // race mode (null otherwise)
      raceCaller.update(race, { pit: !!hs.car?.pit }); // the race caller's lines (raceannouncer.js)
      const h = {
        tickets, race,
        gpu, phase: st.ph, roundLeft: st.ph === "planted" && !showC4Timer() ? 0 : Math.ceil(l), score: st.sc,
        buyLeft: buyAt ? Math.max(0, Math.ceil((buyAt - latest) / 1000)) : null, // null: warmup, no limit
        match: { n: st.n, max: st.max || 0, ot: st.ot || 0, hist: st.hist || [], sw: !!st.sw },
        // Planted: the site and a light flashing with the beeps; seconds and a bar only with mp_c4_show_timer 1.
        ci: st.ph === "planted" ? { site: b.site, beep: beeps, ...(showC4Timer() ? { left: Math.ceil(l), frac: Math.min(1, 1 - l / c4time()) } : {}) } : null,
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
    solids(players, time, now, hideCarrier, map) {
      const out = gpuSolids(st, players, time, hideCarrier, map, me().id); // GPU Drop (nothing in defusal)
      if (blast && (now - blast.at) / 1000 > BLAST_TIME) blast = null;
      if (blast) out.push(...shockwaveParts(blast.p, (now - blast.at) / 1000));
      const b = st?.bomb;
      if (!b?.st || st.ph === "warmup") return out;
      if (b.st === "carried") {
        const c = players.find((p) => p.id === b.by);
        if (c && c.id !== hideCarrier) out.push(...carriedParts(torsoFrame(c), !!c.c));
      } else if (b.st !== "exploded") {
        out.push(...bigDiffParts({ pos: b.p, yaw: 0.4, time, planted: b.st === "planted", left: left(now), total: c4time(), bar: showC4Timer(), beep: (now - beepAt) / 1000, progress: st.ph === "planted" ? b.pr || 0 : 0 }));
      }
      return out;
    },
  };
  return r;
}

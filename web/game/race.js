// Client side of the race mode (Slop n Roll Racing, docs/racing.md; the
// server is internal/game/race.go). Everything shown comes from state: the
// round in every snapshot (laps, gates, race: the racers in position order
// [{id, cp: gates passed, pl: finishing place, t: finish time, wr: seconds
// until a wreck is back, car; on a course: sp the last gate's split, dl/hd
// its delta against the best run, pb the best time, best: this run set it}],
// p2p: point to point, rt: the race clock, ph and left: the grid's
// countdown) and the map's checkpoints (their names and where they are).

// fmtTime: seconds as m:ss.t
export function fmtTime(s) {
  const t = Math.max(0, Math.floor(s * 10) / 10);
  const m = Math.floor(t / 60), r = t - m * 60;
  return `${m}:${r.toFixed(1).padStart(4, "0")}`;
}

// lapOf: the lap a racer with cp gates passed is on (0: not over the line yet).
export function lapOf(cp, gates, laps) {
  if (!gates || cp <= 0) return 0;
  return Math.min(laps, Math.floor((cp - 1) / gates) + 1);
}

// ordinal: 1st, 2nd, 3rd, 4th...
export function ordinal(n) {
  const s = n % 100 >= 11 && n % 100 <= 13 ? "th" : ["th", "st", "nd", "rd"][n % 10] || "th";
  return `${n}${s}`;
}

// HUD state for the race, or null in any other mode / in warmup.
// st: the round state; my: {id}; map: the map (checkpoints); nameOf: id ->
// name; left: seconds left in the phase (the grid's countdown, counted down
// locally); clock: the race clock (seconds since GO, counted locally).
export function raceHud(st, my, map, nameOf, left = 0, clock = st?.rt || 0) {
  if (st?.mode !== "race" || st.ph === "warmup" || !st.gates) return null;
  const racers = st.race || [];
  const p2p = !!st.p2p; // a parkour course: the gates once, start to finish
  const standings = racers.map((r, i) => ({
    id: r.id, name: nameOf(r.id), pos: i + 1, lap: p2p ? 0 : lapOf(r.cp, st.gates, st.laps), gate: r.cp || 0, place: r.pl || 0,
    time: r.t || 0, wreck: r.wr > 0 ? Math.ceil(r.wr * 10) / 10 : 0, me: r.id === my.id, pit: !!r.pit,
  }));
  const mine = standings.find((s) => s.me) || null;
  const meR = racers.find((r) => r.id === my.id);
  let next = null;
  if (meR && !meR.pl && st.ph === "live") {
    const g = map?.checkpoints?.[meR.cp % st.gates];
    if (g) {
      const finish = p2p ? meR.cp === st.gates - 1 : meR.cp % st.gates === 0 && meR.cp > 0;
      next = { name: finish ? (p2p || meR.cp >= st.laps * st.gates ? "finish" : "start/finish") : g.name || `gate ${meR.cp % st.gates}`,
        pos: [(g.min[0] + g.max[0]) / 2, (g.min[1] + g.max[1]) / 2, (g.min[2] + g.max[2]) / 2] };
    }
  }
  return {
    laps: st.laps, gates: st.gates, lap: mine?.lap ?? 0, pos: mine?.pos ?? 0, n: standings.length,
    clock: Math.floor(clock * 10) / 10, standings, next,
    // The grid: the countdown's last three seconds big, then GO! for a second.
    countdown: st.ph === "freeze" ? Math.ceil(left) : 0, go: st.ph === "live" && clock < 1,
    results: st.ph === "over" || st.ph === "matchover",
    wreck: mine?.wreck || 0, pit: !!mine?.pit, finished: mine?.place || 0, finishTime: mine?.time || 0,
    // A course (point to point, on foot): gates passed of all, the last
    // gate's split and its delta against our best run (null: no best yet),
    // finished: our best time and whether this run set it.
    p2p, gate: mine?.gate ?? 0, onFoot: !meR?.car,
    split: meR?.sp || 0, delta: meR?.hd ? meR.dl || 0 : null, best: meR?.pb || 0, newBest: !!meR?.best,
  };
}

// The round-end banner: the winner's name, not a team's.
export function raceEnd(st, nameOf) {
  const w = st?.race?.[0];
  if (!w) return null;
  return { name: nameOf(w.id), text: w.pl ? `takes the checkered flag in ${fmtTime(w.t)}` : "led when the time ran out" };
}

// fmtDelta: a split's delta against the best run: -0.4 / +1.2 (s).
export function fmtDelta(d) {
  const t = Math.round(Math.abs(d) * 10) / 10;
  return `${d < 0 && t > 0 ? "-" : "+"}${t.toFixed(1)}`;
}

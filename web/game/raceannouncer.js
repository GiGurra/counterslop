// The race caller (race mode, docs/racing.md phase 4): turns the race HUD
// state (game/race.js raceHud, every frame) into SlopVox lines
// (assets/voice/lines.js race_*). Everything comes from state, so a lost
// event loses nothing and a line is said once per change:
//   the grid: "Start your linters!", then three, two, one, "Go! Ship it!"
//   the lead: ours taken or lost (a new leader otherwise, now and then)
//   wrecks: ours ("Total write-off!"), anyone else's (now and then)
//   the last lap, our finish by place, a pit stop
//   createRaceAnnouncer({say, now}) .update(race, {pit}) -> the lines said this time
export const RACE_LINE_GAP = 4; // seconds: the chatty lines (someone else's wreck, a new leader) at most this often

export function createRaceAnnouncer({ say = () => {}, now = () => performance.now() / 1000 } = {}) {
  let p = null, chatAt = -Infinity, raceKey = "", pitWas = false;
  const reset = () => { p = { countdown: 0, go: false, leader: 0, lap: 0, wreck: 0, finished: 0, wrecks: new Set() }; };
  reset();
  return {
    update(race, { pit = false } = {}) {
      const out = [];
      const line = (id) => { say(id); out.push(id); };
      const chat = (id) => { const t = now(); if (t - chatAt >= RACE_LINE_GAP) { chatAt = t; line(id); } };
      if (!race || race.results) { reset(); raceKey = ""; pitWas = pit; return out; }
      // A new race (the grid's countdown starting): everything again.
      if (race.countdown > 0 && raceKey !== "grid") { reset(); raceKey = "grid"; line("race_start"); }
      if (race.countdown > 0 && race.countdown <= 3 && race.countdown !== p.countdown) line(["", "race_one", "race_two", "race_three"][race.countdown]);
      p.countdown = race.countdown;
      if (race.go && !p.go) { line("race_go"); raceKey = "live"; }
      p.go = p.go || race.go;
      const st = race.standings || [], me = st.find((s) => s.me), leader = st[0]?.id || 0;
      if (p.go && !race.go && leader && p.leader && leader !== p.leader) { // after the start's scramble
        if (st[0].me) line("race_lead");
        else if (st.find((s) => s.id === p.leader)?.me) line("race_lead_lost");
        else chat("race_new_leader");
      }
      if (leader) p.leader = leader;
      if (race.laps > 1 && race.lap === race.laps && p.lap > 0 && p.lap < race.laps) line("race_last_lap");
      p.lap = race.lap;
      if (race.wreck > 0 && !p.wreck) line("race_wrecked");
      p.wreck = race.wreck;
      for (const s of st) {
        if (s.me) continue;
        if (s.wreck > 0 && !p.wrecks.has(s.id)) { p.wrecks.add(s.id); chat("race_wreck"); }
        if (!s.wreck) p.wrecks.delete(s.id);
      }
      if (race.finished && !p.finished) line(race.finished === 1 ? "race_winner" : race.finished <= 3 ? "race_podium" : "race_finished");
      p.finished = race.finished;
      if (pit && !pitWas && me && !race.finished) line("race_pit");
      pitWas = pit;
      return out;
    },
  };
}

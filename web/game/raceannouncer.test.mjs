import test from "node:test";
import assert from "node:assert/strict";
import { createRaceAnnouncer, RACE_LINE_GAP } from "./raceannouncer.js";
import { LINES } from "../assets/voice/lines.js";

const st = (ids, extra = {}) => ids.map((id, i) => ({ id, pos: i + 1, me: id === 1, wreck: 0, ...(extra[id] || {}) }));
const race = (o) => ({ laps: 3, lap: 1, countdown: 0, go: false, results: false, wreck: 0, finished: 0, standings: st([1, 2, 3]), ...o });

test("the race caller: the grid, GO, the lead, wrecks, the last lap, the finish; each line once per change", () => {
  let t = 0;
  const said = [];
  const a = createRaceAnnouncer({ say: (id) => said.push(id), now: () => t });
  const step = (o, x) => a.update(race(o), x);
  step({ countdown: 5 }); step({ countdown: 4 }); step({ countdown: 3 }); step({ countdown: 3 }); step({ countdown: 2 }); step({ countdown: 1 });
  assert.deepEqual(said, ["race_start", "race_three", "race_two", "race_one"]);
  step({ go: true }); step({ go: true, standings: st([2, 1, 3]) }); // the start's scramble: no lead calls yet
  assert.equal(said.at(-1), "race_go");
  step({ standings: st([2, 1, 3]) });
  step({ standings: st([1, 2, 3]) });
  step({ standings: st([2, 1, 3]) });
  assert.deepEqual(said.slice(5), ["race_lead", "race_lead_lost"]);
  t = 10; step({ standings: st([3, 2, 1]) });
  t = 11; step({ standings: st([2, 3, 1]) }); // too soon for another chatty line
  assert.deepEqual(said.slice(7), ["race_new_leader"]);
  t = 20; step({ standings: st([2, 3, 1], { 3: { wreck: 2 } }) }); step({ standings: st([2, 3, 1], { 3: { wreck: 1.5 } }) });
  assert.deepEqual(said.slice(8), ["race_wreck"]);
  step({ wreck: 3, standings: st([2, 3, 1]) }); step({ wreck: 2, standings: st([2, 3, 1]) });
  assert.equal(said.at(-1), "race_wrecked");
  step({ lap: 2, standings: st([2, 3, 1]) }); step({ lap: 3, standings: st([2, 3, 1]) }); step({ lap: 3, standings: st([2, 3, 1]) });
  assert.equal(said.at(-1), "race_last_lap");
  step({ lap: 3, standings: st([2, 3, 1]) }, { pit: true }); step({ lap: 3, standings: st([2, 3, 1]) }, { pit: true });
  assert.equal(said.at(-1), "race_pit");
  const n = said.length;
  step({ lap: 3, finished: 2, standings: st([2, 1, 3]) }); step({ lap: 3, finished: 2, standings: st([2, 1, 3]) });
  assert.deepEqual(said.slice(n), ["race_podium"]);
  a.update(race({ results: true }));
  step({ countdown: 5 }); // the next race: from the top
  assert.equal(said.at(-1), "race_start");
  for (const id of new Set(said)) assert.ok(LINES[id], id);
  assert.ok(RACE_LINE_GAP > 0);
});

test("the race caller: first over the line is the winner, the rest finish; no race: nothing", () => {
  const said = [];
  const a = createRaceAnnouncer({ say: (id) => said.push(id), now: () => 0 });
  a.update(race({ go: true })); a.update(race({ finished: 1 }));
  assert.equal(said.at(-1), "race_winner");
  const b = createRaceAnnouncer({ say: (id) => said.push(id), now: () => 0 });
  b.update(race({ go: true })); b.update(race({ finished: 6 }));
  assert.equal(said.at(-1), "race_finished");
  assert.deepEqual(createRaceAnnouncer().update(null), []);
});

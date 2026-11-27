// The race mode's HUD (Slop n Roll Racing, docs/racing.md; state from
// game/race.js raceHud): the top bar (lap, position, the race clock), the
// standings, the grid's countdown and GO!, the wreck and finish banners, and
// the results once the race is over.
import { h } from "preact";
import htm from "htm";
import { fmtTime, fmtDelta, ordinal } from "../game/race.js";

const html = htm.bind(h);

// The top bar: LAP 2/4 · P1/3 · 1:23.4
export function RaceBar({ r }) {
  return html`<span class="race-bar">
    ${r.p2p ? html`<span class="race-lap">GATE <b>${r.gate}</b>/${r.gates}</span>` : html`<span class="race-lap">LAP <b>${Math.max(1, r.lap)}</b>/${r.laps}</span>`}
    <span class="race-pos">P<b>${r.pos || "–"}</b>/${r.n}</span>
    <span class="race-clock">${fmtTime(r.clock)}</span>${r.delta !== null && r.delta !== undefined && r.split > 0 && !r.finished ? html`
    <span class=${`race-split ${r.delta < 0 ? "ahead" : "behind"}`}>${fmtTime(r.split)} <b>${fmtDelta(r.delta)}</b></span>` : ""}${r.best > 0 ? html`
    <span class="race-best">BEST ${fmtTime(r.best)}</span>` : ""}</span>`;
}

// The standings: everyone in position order, the lap they are on, wrecks
// counting down, finishers' times.
export function Standings({ r }) {
  return html`<ol class="race-standings">${r.standings.map((s) => html`<li key=${s.id} class=${s.me ? "me" : ""}>
    <span class="rs-pos">${s.pos}</span><span class="rs-name">${s.name}</span>
    <span class="rs-lap">${s.place ? `🏁 ${fmtTime(s.time)}` : s.pit ? "PIT" : s.wreck ? `💥 ${s.wreck.toFixed(1)}` : r.p2p ? `G${s.gate}` : `L${Math.max(1, s.lap)}`}</span></li>`)}</ol>`;
}

// The middle of the screen: the countdown, GO!, a wreck, the finish, the next gate.
export function RaceCentre({ r }) {
  if (r.countdown > 0) {
    return html`<div class="race-count ${r.countdown <= 3 ? "big" : ""}" key=${r.countdown}>${r.countdown <= 3 ? r.countdown : r.onFoot ? "ON YOUR MARKS" : "ON THE GRID"}</div>`;
  }
  if (r.go) return html`<div class="race-count big go" key="go">GO!</div>`;
  if (r.pit) return html`<div class="race-wreck">IN THE PIT · the race clock is running</div>`;
  if (r.wreck > 0) return html`<div class="race-wreck">${r.onFoot ? "DOWN" : "WRECKED"} · back on the ${r.onFoot ? "course" : "track"} in ${r.wreck.toFixed(1)}</div>`;
  if (r.finished && !r.results) return html`<div class="race-finish">FINISHED ${ordinal(r.finished)} · ${fmtTime(r.finishTime)}${r.newBest ? " · NEW BEST!" : r.best > 0 ? ` · best ${fmtTime(r.best)}` : ""}</div>`;
  if (r.noSlot) return html`<div class="race-next">every car is taken · you race from the next race</div>`;
  if (r.next) return html`<div class="race-next">next: ${r.next.name}</div>`;
  return null;
}

// The results: the finishing order with times (the rest: still racing when it ended).
export function RaceResults({ r }) {
  return html`<table class="race-results"><tr><th></th><th>racer</th><th>time</th></tr>
    ${r.standings.map((s) => html`<tr key=${s.id} class=${s.me ? "me" : ""}><td>${s.place ? ordinal(s.place) : "DNF"}</td><td>${s.name}</td>
      <td>${s.place ? fmtTime(s.time) : `lap ${Math.max(1, s.lap)}`}</td></tr>`)}</table>`;
}

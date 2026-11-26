// The race mode's HUD (Slop n Roll Racing, docs/racing.md; state from
// game/race.js raceHud): the top bar (lap, position, the race clock), the
// standings, the grid's countdown and GO!, the wreck and finish banners, and
// the results once the race is over.
import { h } from "preact";
import htm from "htm";
import { fmtTime, ordinal } from "../game/race.js";

const html = htm.bind(h);

// The top bar: LAP 2/4 · P1/3 · 1:23.4
export function RaceBar({ r }) {
  return html`<span class="race-bar">
    <span class="race-lap">LAP <b>${Math.max(1, r.lap)}</b>/${r.laps}</span>
    <span class="race-pos">P<b>${r.pos || "–"}</b>/${r.n}</span>
    <span class="race-clock">${fmtTime(r.clock)}</span></span>`;
}

// The standings: everyone in position order, the lap they are on, wrecks
// counting down, finishers' times.
export function Standings({ r }) {
  return html`<ol class="race-standings">${r.standings.map((s) => html`<li key=${s.id} class=${s.me ? "me" : ""}>
    <span class="rs-pos">${s.pos}</span><span class="rs-name">${s.name}</span>
    <span class="rs-lap">${s.place ? `🏁 ${fmtTime(s.time)}` : s.wreck ? `💥 ${s.wreck.toFixed(1)}` : `L${Math.max(1, s.lap)}`}</span></li>`)}</ol>`;
}

// The middle of the screen: the countdown, GO!, a wreck, the finish, the next gate.
export function RaceCentre({ r }) {
  if (r.countdown > 0) {
    return html`<div class="race-count ${r.countdown <= 3 ? "big" : ""}" key=${r.countdown}>${r.countdown <= 3 ? r.countdown : "ON THE GRID"}</div>`;
  }
  if (r.go) return html`<div class="race-count big go" key="go">GO!</div>`;
  if (r.wreck > 0) return html`<div class="race-wreck">WRECKED · back on the track in ${r.wreck.toFixed(1)}</div>`;
  if (r.finished && !r.results) return html`<div class="race-finish">FINISHED ${ordinal(r.finished)} · ${fmtTime(r.finishTime)}</div>`;
  if (r.next) return html`<div class="race-next">next: ${r.next.name}</div>`;
  return null;
}

// The results: the finishing order with times (the rest: still racing when it ended).
export function RaceResults({ r }) {
  return html`<table class="race-results"><tr><th></th><th>racer</th><th>time</th></tr>
    ${r.standings.map((s) => html`<tr key=${s.id} class=${s.me ? "me" : ""}><td>${s.place ? ordinal(s.place) : "DNF"}</td><td>${s.name}</td>
      <td>${s.place ? fmtTime(s.time) : `lap ${Math.max(1, s.lap)}`}</td></tr>`)}</table>`;
}

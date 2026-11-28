// The tickets mode's HUD (Star Slop, docs/starslop.md; state from
// game/tickets.js ticketsHud): each side's ticket BACKLOG in the top bar, the
// capture points strip under it, and the spawn screen while dead (a map
// overview with the points: click one you hold to respawn there; the kits).
import { h } from "preact";
import htm from "htm";
import { TEAMS } from "../assets/teams.js";
import { KITS, overview } from "../game/tickets.js";

const html = htm.bind(h);
const colour = (team) => TEAMS[team]?.color || "#c8c8c8";

// One side's backlog in the top bar: "EARTH BACKLOG 142".
export function Backlog({ team, n, left }) {
  const t = TEAMS[team];
  const label = html`<span class="long">${t.name.toUpperCase()}</span><span class="short">${t.short}</span>`;
  return html`<span class="team-score backlog ${team}" style=${{ color: t.color }}>${left
    ? html`${label} <small>BACKLOG</small> <b>${n}</b>`
    : html`<b>${n}</b> <small>BACKLOG</small> ${label}`}</span>`;
}

// The points strip: one tile per point, its owner's colour, the bar of who is
// taking it, a blink while contested.
export function PointsStrip({ points, myTeam }) {
  return html`<div class="cq-points">${points.map((p) => html`<span key=${p.id}
      class="cq-pt ${p.own ? `own-${p.own}` : "neutral"}${p.cont ? " cont" : ""}${p.locked ? " locked" : ""}${p.own && p.own === myTeam ? " mine" : ""}"
      style=${{ "--c": p.own ? colour(p.own) : "#bbb" }} title=${p.name}>
      <b>${p.locked ? "⌂" : p.id}</b>${p.pr > 0 && html`<i class="cq-bar" style=${{ width: `${p.pr * 100}%`, background: colour(p.by) }}></i>`}</span>`)}</div>`;
}

// A ship's departure board (game/ships.js departureHud): boarding with the
// seconds to the door closing, in flight with the seconds to the next one,
// or aboard ("fasten your seatbelt").
export function DepartureBoard({ d }) {
  const what = d.aboard ? (d.state === "boarding" ? html`ABOARD · doors close in <b>${d.in}</b>` : html`IN FLIGHT · fasten your seatbelt`)
    : d.state === "boarding" ? html`BOARDING · doors close in <b>${d.in}</b>` : html`DEPARTED · next ${d.name} in <b>${d.in}</b>`;
  return html`<div class="departure ${d.state}${d.aboard ? " aboard" : ""}"><span class="dep-name">${d.name}</span> ${what}</div>`;
}

// The spawn screen: the countdown, the map with the points (yours clickable),
// AUTO, and the kits. With one point to spawn at, a line saying so instead of
// a choice that isn't one.
export function SpawnScreen({ s, map, deploy, kit }) {
  const sp = s.tickets.spawn, W = 360, H = 200;
  const ov = overview(s.tickets.points, map, W, H);
  const okIds = new Set(sp.only ? [] : sp.points.filter((p) => p.ok).map((p) => p.id)); // one point: nothing to click
  const atName = s.tickets.points.find((p) => p.id === sp.at)?.name || "";
  return html`<div class="spawn-screen">
    <div class="ss-title">Ticket closed. ${sp.left > 0 ? html`Respawning in <b>${sp.left}</b>` : "Respawning…"}${atName && html` at <b class="ss-at">${atName}</b>`}</div>
    <svg class="ss-map" viewBox="0 0 ${W} ${H}" width=${W} height=${H}>
      <rect class="ss-field" x=${ov.field.x} y=${ov.field.y} width=${ov.field.w} height=${ov.field.h} />
      ${ov.points.map((p) => html`<g key=${p.id} class="ss-pt${okIds.has(p.id) ? " ok" : ""}${sp.pick === p.id ? " picked" : ""}${sp.at === p.id ? " at" : ""}"
          data-pt=${p.id} onClick=${() => okIds.has(p.id) && deploy(p.id)}>
        <circle cx=${p.x} cy=${p.y} r=${p.rr} style=${{ fill: p.own ? colour(p.own) : "#888" }} />
        <text x=${p.x} y=${p.y + 4}>${p.locked ? p.name : p.id}</text></g>`)}
    </svg>
    ${sp.only ? html`<div class="ss-only">Deploying at <b>${sp.only}</b>: ${sp.fixed ? "your team's only spawn point on this map" : "the only spawn point you can use now (hold more to choose)"}</div>`
      : html`<div class="ss-row"><button type="button" class="ss-auto ${sp.pick === "auto" ? "on" : ""}" onClick=${() => deploy("auto")}>AUTO (nearest the action)</button>
      ${sp.points.filter((p) => p.ok).map((p) => html`<button type="button" key=${p.id} class="ss-btn ${sp.pick === p.id ? "on" : ""}" data-pt=${p.id} onClick=${() => deploy(p.id)}>${p.name}</button>`)}</div>`}
    <div class="ss-kits">${KITS.map((k) => html`<button type="button" key=${k.id} class="ss-kit ${sp.kit === k.id ? "on" : ""}" data-kit=${k.id} onClick=${() => kit(k.id)}>
      <b>${k.name}</b><span>${k.desc || ""}</span></button>`)}</div>
  </div>`;
}

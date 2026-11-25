// Server settings (main menu, admins only): the game rules an admin changes
// from a form instead of the console. Every change is the existing rcon cvar
// command (the server checks admin, applies it at once and announces it); the
// values shown are the server's cvars message, so every admin sees each
// change live. Debug and cheat cvars (sv_cheats, weapon_spread, sv_gravity,
// sv_maxunlag) stay in the console on purpose; sv_showimpacts is under Practice.
import { h } from "preact";
import { useEffect } from "preact/hooks";
import htm from "htm";
import { SERVER_SETTINGS, normalize, toggleRotation, rotationList } from "./serverrules.js";

const html = htm.bind(h);

// The screen. send(cvar, value) runs the rcon command; loadMaps() fills s.mapCards.
export function ServerSettings({ s, back, send, loadMaps }) {
  useEffect(() => { loadMaps?.(); }, []);
  const cv = s.cvars || {}, help = s.cvarHelp || {};
  const set = (row, raw, el) => {
    const v = normalize(row, raw);
    if (v === null || v === cv[row.cvar]) { if (el) el.value = cv[row.cvar] ?? ""; return; }
    if (el) el.value = v;
    send(row.cvar, v);
  };
  const control = (row) => {
    const v = cv[row.cvar];
    switch (row.kind) {
      case "toggle":
        return html`<input type="checkbox" data-cvar=${row.cvar} checked=${v === "1"} onChange=${(e) => set(row, e.target.checked)} />`;
      case "select":
        return html`<select data-cvar=${row.cvar} value=${v} onChange=${(e) => set(row, e.target.value)}>
          ${row.choices.map(([val, label]) => html`<option value=${val}>${label}</option>`)}
          ${v !== undefined && !row.choices.some(([val]) => val === v) ? html`<option value=${v}>${v}</option>` : ""}
        </select>`;
      case "number":
        return html`<input type="number" class="num" data-cvar=${row.cvar} key=${row.cvar + "=" + v} defaultValue=${v}
          min=${row.min} max=${row.max} step=${row.step ?? "any"}
          onChange=${(e) => set(row, e.target.value, e.target)} onKeyDown=${(e) => e.key === "Enter" && e.target.blur()} />
          ${row.unit ? html`<span class="unit">${row.unit}</span>` : ""}`;
      default:
        return null;
    }
  };
  const maps = (row) => {
    const all = (s.mapCards || []).map((m) => m.name);
    if (!all.length) return html`<div class="hint">loading the maps…</div>`;
    const on = rotationList(cv[row.cvar], all);
    return html`<div class="rotation">${all.map((n) => html`<label class="row check" key=${n}>
      <input type="checkbox" data-map=${n} checked=${on.includes(n)} disabled=${on.length === 1 && on.includes(n)}
        onChange=${(e) => send(row.cvar, toggleRotation(cv[row.cvar], all, n, e.target.checked))} /> ${n}</label>`)}</div>`;
  };
  return html`<div class="panel options server-settings" onClick=${(e) => e.stopPropagation()}>
    <h2>SERVER SETTINGS</h2>
    ${!s.admin ? html`<p class="hint warn">You're not admin on this server any more (admin_login in the console).</p>` : html`
    <div class="hint">Changes apply at once for everyone and are announced (as <code>rcon</code> does). Console-only: cheats and debug cvars.</div>
    ${SERVER_SETTINGS.map((g) => html`<section key=${g.title}>
      <h4>${g.title}</h4>
      ${g.rows.map((row) => html`<div class="srv-row" key=${row.cvar}>
        ${row.kind === "maps" ? maps(row) : html`<div class="row"><span>${row.label}</span>${control(row)}</div>`}
        <div class="hint"><code>${row.cvar}</code> ${help[row.cvar]?.[1] ?? ""}</div>
      </div>`)}
    </section>`)}`}
    <div class="buttons"><button type="button" onClick=${back}>BACK</button></div>
  </div>`;
}

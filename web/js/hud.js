// Preact HUD + menu overlay. Game code pushes state with hud.set({...}).
import { h, render } from "preact";
import { useState, useEffect, useRef } from "preact/hooks";
import htm from "htm";

const html = htm.bind(h);

export function mountHud(el, actions) {
  let state = {
    screen: "menu", // menu | paused | playing
    name: loadName(),
    hp: 100, armor: 0, money: 800,
    ammo: 30, reserve: 90, reloading: false,
    ping: null, connected: false, map: "de_slop",
    feed: [], players: [], myId: 0, scoreboard: false, roundLeft: 115,
  };
  const subs = new Set();
  const api = {
    get: () => state,
    set(p) { state = { ...state, ...p }; subs.forEach((f) => f(state)); },
    feed(text) {
      const item = { id: Math.random(), text, at: performance.now() };
      api.set({ feed: [...state.feed, item].slice(-5) });
      setTimeout(() => api.set({ feed: state.feed.filter((f) => f !== item) }), 5000);
    },
  };

  function App() {
    const [s, setS] = useState(state);
    useEffect(() => { subs.add(setS); return () => subs.delete(setS); }, []);
    return html`
      ${s.screen !== "menu" && html`<${Hud} s=${s} />`}
      ${s.screen !== "playing" && html`<${Menu} s=${s} />`}
      ${s.scoreboard && s.screen !== "menu" && html`<${Scoreboard} s=${s} />`}
    `;
  }

  function Menu({ s }) {
    const input = useRef();
    const go = (e) => {
      e?.preventDefault();
      const name = (input.current?.value || "").trim().slice(0, 16) || "slopper";
      saveName(name);
      api.set({ name });
      actions.play(name);
    };
    if (s.screen === "paused") {
      return html`<div class="overlay" onClick=${() => actions.resume()}>
        <div class="panel"><h2>PAUSED</h2><p>click to get back in there</p>${help}</div></div>`;
    }
    return html`<div class="overlay">
      <form class="panel" onSubmit=${go}>
        <h1>COUNTER<span>-</span>SLOP</h1>
        <div class="tag">Global Offensive Smell</div>
        <input ref=${input} maxlength="16" placeholder="your name" defaultValue=${s.name} autofocus />
        <button type="submit">PLAY</button>
        ${help}
      </form></div>`;
  }

  const help = html`<div class="help">
    <b>WASD</b> move · <b>Shift</b> sneak · <b>Space</b> boing · <b>Mouse</b> pew · <b>R</b> reload · <b>Tab</b> scores · <b>Esc</b> pause
  </div>`;

  function Hud({ s }) {
    const m = Math.floor(s.roundLeft / 60), sec = String(s.roundLeft % 60).padStart(2, "0");
    return html`
      <div class="crosshair"><i class="l"></i><i class="r"></i><i class="t"></i><i class="b"></i></div>
      <div class="vignette"></div>
      <div class="top-mid"><span class="ct">CT 0</span><span class="timer">${m}:${sec}</span><span class="t">0 T</span></div>
      <div class="top-left">
        <div class="money">$${s.money}</div>
        <div class="net ${s.connected ? "" : "bad"}">${s.connected ? `${s.map} · ${s.ping ?? "…"} ms` : "reconnecting… (stuck? reload the page)"}</div>
      </div>
      <div class="feed">${s.feed.map((f) => html`<div key=${f.id}>${f.text}</div>`)}</div>
      <div class="bottom-left">
        <span class="hp">✚ ${s.hp}</span><span class="armor">◈ ${s.armor}</span>
      </div>
      <div class="bottom-right ${s.reloading ? "reloading" : ""}">
        <span class="ammo">${s.reloading ? "slopping…" : s.ammo}</span><span class="reserve">/ ${s.reserve}</span>
      </div>`;
  }

  function Scoreboard({ s }) {
    const rows = [{ id: s.myId, name: s.name, color: s.myColor, me: true }, ...s.players];
    return html`<div class="scoreboard">
      <h3>${s.map} — ${rows.length} slopper${rows.length === 1 ? "" : "s"}</h3>
      <table><tr><th></th><th>name</th><th>K</th><th>D</th><th>ping</th></tr>
      ${rows.map((p) => html`<tr key=${p.id} class=${p.me ? "me" : ""}>
        <td><i class="swatch" style=${{ background: p.color }}></i></td>
        <td>${p.name}</td><td>0</td><td>0</td><td>${p.me ? s.ping ?? "…" : "?"}</td></tr>`)}
      </table></div>`;
  }

  render(html`<${App} />`, el);
  return api;
}

function loadName() { try { return localStorage.getItem("cs.name") || ""; } catch { return ""; } }
function saveName(n) { try { localStorage.setItem("cs.name", n); } catch {} }

// Preact HUD + menu overlay. Game code pushes state with hud.set({...}).
import { h, render } from "preact";
import { useState, useEffect, useRef } from "preact/hooks";
import htm from "htm";
import { settings, CVARS, M_YAW } from "./settings.js";

const html = htm.bind(h);

export function mountHud(el, actions) {
  let state = {
    screen: "menu", // menu | ingame (menu over a live match) | resume (menu closed, waiting for a click) | playing
    pausedAt: 0,
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
      ${(s.screen === "menu" || s.screen === "ingame") && !s.console && html`<${Menu} s=${s} />`}
      ${s.screen === "resume" && !s.console && html`<div class="resume-hint">click to resume</div>`}
      ${s.scoreboard && s.screen !== "menu" && html`<${Scoreboard} s=${s} />`}
    `;
  }

  function Menu({ s }) {
    const [view, setView] = useState("main");
    // Esc: Options goes back a level; the in-game menu closes. The console and key
    // rebinding catch Esc in the capture phase first, so it never gets here then.
    useEffect(() => {
      const onKey = (e) => {
        if (e.code !== "Escape" || e.repeat) return;
        if (view === "options") { setView("main"); return; }
        // The Esc that left pointer lock can arrive as a keydown too; don't let it close the menu it opened.
        if (s.screen === "ingame" && performance.now() - state.pausedAt > 250) actions.closeMenu();
      };
      addEventListener("keydown", onKey);
      return () => removeEventListener("keydown", onKey);
    }, [view, s.screen]);
    if (view === "options") return html`<div class="overlay"><${Options} back=${() => setView("main")} /></div>`;
    const opts = html`<button type="button" class="secondary" onClick=${(e) => { e.stopPropagation(); setView("options"); }}>OPTIONS</button>`;
    const input = useRef();
    const go = (e) => {
      e?.preventDefault();
      const name = (input.current?.value || "").trim().slice(0, 16) || "slopper";
      saveName(name);
      api.set({ name });
      actions.play(name);
    };
    if (s.screen === "ingame") {
      return html`<div class="overlay" onClick=${() => actions.resume()}>
        <div class="panel"><h2>MENU</h2><p>the match keeps going. click to jump back in</p>${opts}
          <button type="button" class="secondary" onClick=${(e) => { e.stopPropagation(); actions.leave(); }}>LEAVE MATCH</button>${help}</div></div>`;
    }
    return html`<div class="overlay">
      <form class="panel" onSubmit=${go}>
        <h1>COUNTER<span>-</span>SLOP</h1>
        <div class="tag">Global Offensive Smell</div>
        <input ref=${input} maxlength="16" placeholder="your name" defaultValue=${s.name} autofocus />
        <button type="submit">PLAY</button>
        ${opts}
        ${help}
      </form></div>`;
  }

  const help = html`<div class="help">
    <b>WASD</b> move · <b>Shift</b> sneak · <b>C</b>/<b>Ctrl</b> crouch · <b>Space</b> boing · <b>Mouse</b> pew · <b>R</b> reload · <b>Tab</b> scores · <b>§</b>/<b>\`</b> console · <b>Esc</b> menu
  </div>`;

  function useSettings() {
    const [v, setV] = useState(settings.get());
    useEffect(() => settings.subscribe(setV), []);
    return v;
  }

  // Number field that commits on change/enter and resyncs when the value moves elsewhere.
  const Num = ({ k, v, step = "any", disabled }) => html`<input type="number" class="num" key=${"n" + v}
    defaultValue=${+(+v).toFixed(3)} step=${step} min=${CVARS[k].min} max=${CVARS[k].max} disabled=${disabled}
    onChange=${(e) => set(k, e.target.value)} />`;

  function set(k, raw) {
    const c = CVARS[k];
    let v = c.enum || c.key ? raw : c.bool ? (raw ? 1 : 0) : Math.min(c.max, Math.max(c.min, parseFloat(raw)));
    if (!c.enum && !c.key && !Number.isFinite(v)) return;
    settings.set({ [k]: v });
  }

  function Options({ back }) {
    const v = useSettings();
    const [rebind, setRebind] = useState(false);
    const st = actions.loop?.()?.stats();
    const hz = st ? Math.round(st.hz) : 60;
    const counts360 = Math.round(360 / (v.sensitivity * M_YAW));
    useEffect(() => {
      if (!rebind) return;
      const onKey = (e) => {
        e.preventDefault(); e.stopPropagation();
        if (e.code !== "Escape") settings.set({ [rebind]: e.code });
        setRebind(false);
      };
      addEventListener("keydown", onKey, true);
      return () => removeEventListener("keydown", onKey, true);
    }, [rebind]);
    const check = (k, label) => html`<label class="row check"><input type="checkbox" checked=${!!v[k]} onChange=${(e) => set(k, e.target.checked)} /> ${label}</label>`;
    return html`<div class="panel options" onClick=${(e) => e.stopPropagation()}>
      <h2>OPTIONS</h2>
      <section>
        <h4>Mouse</h4>
        <div class="row"><span>Sensitivity</span>
          <input type="range" min="0.1" max="10" step="0.01" value=${v.sensitivity} onInput=${(e) => set("sensitivity", e.target.value)} />
          <${Num} k="sensitivity" v=${v.sensitivity} /></div>
        <div class="hint">CS scale: ${M_YAW}°/count × sensitivity → ${counts360.toLocaleString()} counts per 360°. Same number as your CS sens if raw input is on.</div>
        ${check("m_rawinput", "Raw input (no OS acceleration; applies next time the mouse is captured)")}
        ${check("m_invert_y", "Invert Y")}
      </section>
      <section>
        <h4>Video</h4>
        <div class="row"><span>FPS limit</span>
          <select value=${v.fps_limit} onChange=${(e) => set("fps_limit", e.target.value)}>
            <option value="auto">Refresh − 3 (${hz - 3} fps)</option>
            <option value="custom">Custom</option>
            <option value="off">Off (follow display)</option>
          </select>
          <${Num} k="fps_max" v=${v.fps_max} step="1" disabled=${v.fps_limit !== "custom"} /></div>
        <div class="hint">Display measured at ~${hz} Hz. Capping a few fps under refresh keeps frames from queueing in the driver (NVIDIA), which adds input lag.</div>
        <div class="row"><span>Frame loop</span>
          <select value=${v.cl_loop} onChange=${(e) => set("cl_loop", e.target.value)}>
            <option value="raf">VSync (requestAnimationFrame)</option>
            <option value="unlocked">Unlocked (experimental)</option>
          </select></div>
        <div class="hint">Browsers can't turn vsync off: the page is always shown at the display's rate. Unlocked runs the game loop
          as fast as it can so the shown frame is fresher, but burns CPU. Truly uncapped needs browser flags, e.g. Chrome
          <code>--disable-frame-rate-limit --disable-gpu-vsync</code>.</div>
        ${check("cl_showfps", "Show FPS / frame time / ping")}
      </section>
      <section>
        <h4>Audio</h4>
        <div class="row"><span>Volume</span>
          <input type="range" min="0" max="1" step="0.01" value=${v.volume} onInput=${(e) => set("volume", e.target.value)} />
          <span class="num">${Math.round(v.volume * 100)}%</span></div>
      </section>
      <section>
        <h4>Controls</h4>
        <div class="row"><span>Crouch</span>
          ${["key_crouch", "key_crouch2"].map((k) => html`<button type="button" class="rebind ${rebind === k ? "waiting" : ""}" onClick=${() => setRebind(k)}>
            ${rebind === k ? "press a key… (Esc cancels)" : keyName(v[k])}</button>`)}</div>
        ${check("cl_fullscreen", "Fullscreen play (needed for Ctrl crouch)")}
        <div class="hint">In a normal window the browser owns Ctrl+W and closes the tab. Fullscreen play locks the keyboard
          to the game (Chrome/Edge), so Ctrl+W just crouch-walks. Elsewhere use C, and leaving mid-match always asks first.</div>
      </section>
      <div class="buttons">
        <button type="button" class="secondary" onClick=${() => settings.reset()}>DEFAULTS</button>
        <button type="button" onClick=${back}>BACK</button>
      </div>
    </div>`;
  }

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

function keyName(code) {
  return code.replace(/^Key/, "").replace(/^Digit/, "").replace(/(Left|Right)$/, " $1");
}

function loadName() { try { return localStorage.getItem("cs.name") || ""; } catch { return ""; } }
function saveName(n) { try { localStorage.setItem("cs.name", n); } catch {} }

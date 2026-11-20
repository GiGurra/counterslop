// Preact HUD + menu overlay. Game code pushes state with hud.set({...}).
import { h, render } from "preact";
import { useState, useEffect, useRef } from "preact/hooks";
import htm from "htm";
import { settings, CVARS, M_YAW } from "./settings.js";

const html = htm.bind(h);

export function mountHud(el, actions) {
  let state = {
    // menu: title screen. ingame: Esc menu over the running match. playing: in the match.
    // Nothing ever stops the game; these only decide which overlay has focus.
    screen: "menu",
    menuOpenedAt: 0,
    aiming: false, // mouse captured; without it the crosshair says "click to aim"
    name: loadName(),
    hp: 100, armor: 0, money: 800,
    ammo: 30, reserve: 90, reloading: false, weapon: "", slots: [], scoped: false,
    ping: null, connected: false, map: "de_slop",
    feed: [], players: [], myId: 0, scoreboard: false, roundLeft: 115,
    shout: null, subtitle: null, // announcer captions: { id, text, color, dur }
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
    // Announcer caption: kind "shout" = big centre text, "sub" = subtitle at the bottom.
    caption({ text, color = "#fff", dur = 1.5, kind = "shout" }) {
      const slot = kind === "sub" ? "subtitle" : "shout", c = { id: Math.random(), text, color, dur: Math.max(1.4, dur + 0.3) };
      api.set({ [slot]: c });
      setTimeout(() => { if (state[slot] === c) api.set({ [slot]: null }); }, c.dur * 1000);
    },
  };

  function App() {
    const [s, setS] = useState(state);
    useEffect(() => { subs.add(setS); return () => subs.delete(setS); }, []);
    return html`
      ${s.screen !== "menu" && html`<${Hud} s=${s} />`}
      ${(s.screen === "menu" || s.screen === "ingame") && !s.console && html`<${Menu} s=${s} />`}
      ${s.screen === "playing" && !s.aiming && !s.console && html`<div class="aim-hint">click to aim</div>`}
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
        if (s.screen === "ingame" && performance.now() - state.menuOpenedAt > 250) actions.closeMenu();
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
      return html`<div class="overlay" onClick=${() => { actions.closeMenu(); actions.aim(); }}>
        <div class="panel"><h2>MENU</h2><p>the match keeps going · Esc or click outside to get back in</p>${opts}
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
    const div = v.fps_limiter !== "latency" && v.fps_limit === "custom" && v.fps_max > 0 ? Math.min(4, Math.max(1, Math.round(hz / v.fps_max))) : 1;
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
        <div class="row"><span>Scope zoom</span>
          <select value=${v.zoom_mode} onChange=${(e) => set("zoom_mode", e.target.value)}>
            <option value="toggle">Toggle with right click</option>
            <option value="hold">Hold right button</option>
          </select></div>
        <div class="row"><span>Zoom sensitivity</span>
          <input type="range" min="0.2" max="3" step="0.05" value=${v.zoom_sensitivity_ratio} onInput=${(e) => set("zoom_sensitivity_ratio", e.target.value)} />
          <${Num} k="zoom_sensitivity_ratio" v=${v.zoom_sensitivity_ratio} /></div>
        <div class="hint">1 = scoped aim feels like unscoped (the crosshair moves the same distance on screen per count).</div>
        ${check("m_rawinput", "Raw input (no OS acceleration; applies next time the mouse is captured)")}
        ${check("m_invert_y", "Invert Y")}
      </section>
      <section>
        <h4>Video</h4>
        <div class="row"><span>Limiter</span>
          <select value=${v.fps_limiter} disabled=${v.cl_loop === "unlocked"} onChange=${(e) => {
            set("fps_limiter", e.target.value);
            if (e.target.value === "even" && v.fps_limit === "auto") set("fps_limit", "off");
          }}>
            <option value="even">Even pacing</option>
            <option value="latency">Latency (may micro-stutter)</option>
          </select></div>
        ${v.fps_limiter === "latency" ? html`
        <div class="row"><span>FPS limit</span>
          <select value=${v.fps_limit} disabled=${v.cl_loop === "unlocked"} onChange=${(e) => set("fps_limit", e.target.value)}>
            <option value="off">Off (follow display)</option>
            <option value="auto">Refresh − 3 (${hz - 3} fps)</option>
            <option value="custom">Custom</option>
          </select>
          <${Num} k="fps_max" v=${v.fps_max} step="1" disabled=${v.fps_limit !== "custom" || v.cl_loop === "unlocked"} /></div>
        <div class="hint">Display measured at ~${hz} Hz. Caps any value by skipping refreshes, like native games do to keep
          the driver queue empty (refresh − 3). In a browser that skips a frame every so often: a small regular stutter.</div>` : html`
        <div class="row"><span>FPS limit</span>
          <select value=${String(div)} disabled=${v.cl_loop === "unlocked"} onChange=${(e) => {
            const n = +e.target.value;
            if (n === 1) set("fps_limit", "off");
            else { set("fps_max", Math.round(hz / n)); set("fps_limit", "custom"); }
          }}>
            ${[1, 2, 3, 4].map((n) => html`<option value=${String(n)}>${n === 1 ? `Off (${hz} fps, every refresh)` : `${Math.round(hz / n)} fps (every ${n}${["", "", "nd", "rd", "th"][n]} refresh)`}</option>`)}
          </select></div>
        <div class="hint">Display measured at ~${hz} Hz. Even pacing only caps at the refresh divided by a whole number,
          so every frame lasts exactly as long.</div>`}
        <div class="row"><span>Frame loop</span>
          <select value=${v.cl_loop} onChange=${(e) => set("cl_loop", e.target.value)}>
            <option value="raf">VSync (one frame per refresh)</option>
            <option value="unlocked">Unlocked (experimental low latency)</option>
          </select></div>
        <div class="hint">Browsers always show the page at the display's rate. Unlocked runs the game evenly paced at ${v.fps_max_unlocked > 0 ? `up to ${v.fps_max_unlocked} fps` : "uncapped fps"}
          (console: <code>fps_max_unlocked</code>, 0 = uncapped)
          so the frame that gets shown is fresher, at the cost of CPU/GPU time. The game simulation runs at a fixed tick
          either way. Showing more than the refresh rate needs browser flags, e.g. Chrome
          <code>--disable-frame-rate-limit --disable-gpu-vsync</code>.</div>
        <div class="row"><span>Shadows</span>
          <select value=${String(v.r_shadow_quality)} onChange=${(e) => set("r_shadow_quality", e.target.value)}>
            <option value="0">Off</option><option value="1">Hard</option>
            <option value="2">Soft</option><option value="3">Softest</option>
          </select>
          <select value=${String(v.r_shadowmap_size)} onChange=${(e) => set("r_shadowmap_size", e.target.value)} disabled=${!+v.r_shadow_quality}>
            ${[512, 1024, 2048, 4096].map((n) => html`<option value=${String(n)}>${n}²</option>`)}
          </select></div>
        ${check("cl_showfps", "Show FPS / frame time / ping")}
      </section>
      <section>
        <h4>Audio</h4>
        <div class="row"><span>Volume</span>
          <input type="range" min="0" max="1" step="0.01" value=${v.volume} onInput=${(e) => set("volume", e.target.value)} />
          <span class="num">${Math.round(v.volume * 100)}%</span></div>
        <div class="row"><span>Announcer</span>
          <input type="range" min="0" max="1" step="0.01" value=${v.announcer_volume} disabled=${!v.cl_announcer}
            onInput=${(e) => set("announcer_volume", e.target.value)} />
          <span class="num">${Math.round(v.announcer_volume * 100)}%</span></div>
        ${check("cl_announcer", "Announcer voice (SlopVox kill streaks)")}
        ${check("cl_captions", "Captions: announcer lines as big text, taunts as subtitles")}
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
      ${s.scoped === "lens" ? html`<${Scope} />` : html`<div class="crosshair"><i class="l"></i><i class="r"></i><i class="t"></i><i class="b"></i></div>`}
      <div class="vignette"></div>
      ${s.shout && html`<div class="shout" key=${s.shout.id} style=${{ color: s.shout.color, "--dur": `${s.shout.dur}s` }}>${s.shout.text}</div>`}
      ${s.subtitle && html`<div class="subtitle" key=${s.subtitle.id} style=${{ "--dur": `${s.subtitle.dur}s` }}><span style=${{ color: s.subtitle.color }}>${s.subtitle.text}</span></div>`}
      <div class="top-mid"><span class="ct">CT 0</span><span class="timer">${m}:${sec}</span><span class="t">0 T</span></div>
      <div class="top-left">
        <div class="money">$${s.money}</div>
        <div class="net ${s.connected ? "" : "bad"}">${s.connected ? `${s.map} · ${s.ping ?? "…"} ms` : "reconnecting… (stuck? reload the page)"}</div>
      </div>
      <div class="feed">${s.feed.map((f) => html`<div key=${f.id}>${f.text}</div>`)}</div>
      <div class="bottom-left">
        <span class="hp">✚ ${s.hp}</span><span class="armor">◈ ${s.armor}</span>
      </div>
      <div class="slots">${s.slots.filter((x) => x.name).map((x) => html`<div key=${x.slot} class=${x.cur ? "cur" : ""}><b>${x.key}</b> ${x.name}</div>`)}</div>
      <div class="bottom-right ${s.reloading ? "reloading" : ""}">
        <span class="ammo">${s.reloading ? "slopping…" : s.ammo}</span>${s.reserve !== "" && html`<span class="reserve">/ ${s.reserve}</span>`}
      </div>`;
  }

  // Code Review's scope: a lens framed by a unified diff of your target's life choices.
  const DIFF = ["@@ -1,7 +1,7 @@ class Opponent", "-  alive = true", "+  alive = false", "   // TODO: dodge", "-  return position", "+  return respawn()", " Changes requested."];
  function Scope() {
    return html`<div class="scope">
      <div class="lens"></div><i class="h"></i><i class="v"></i>
      <pre class="diff">${DIFF.map((l) => html`<span class=${l[0] === "-" ? "del" : l[0] === "+" ? "add" : l[0] === "@" ? "hunk" : ""}>${l}</span>`)}</pre>
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

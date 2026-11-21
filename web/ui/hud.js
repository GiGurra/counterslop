// Preact HUD + menu overlay. Game code pushes state with hud.set({...}).
import { h, render } from "preact";
import { useState, useEffect, useLayoutEffect, useRef } from "preact/hooks";
import htm from "htm";
import { settings, CVARS, M_YAW } from "./settings.js";
import { ACTIONS, keyFor, keyName, bindKey, clearAction } from "./binds.js";
import { TEAMS, TEAM_IDS, SPECTATOR, teamOpen } from "../assets/teams.js";
import { BuyMenu } from "./buymenu.js";
import { ChatLog, ChatInput, RadioMenu } from "./chat.js";
import { weaponIcon, KILL_GLYPH } from "./weaponicon.js";

const html = htm.bind(h);

// The match HUD: phase labels for the timer, the round counter, history glyphs.
const PHASE_LABEL = { warmup: "WARMUP", halftime: "HALFTIME", matchover: "MATCH OVER" };
const HOW = { elimination: "✕", exploded: "✹", defused: "↺", time: "⏱" };
const roundLabel = (m) => (m.ot > 0 ? `OVERTIME ${m.ot} · ROUND ${m.n}` : `ROUND ${m.n} / ${m.max} · first to ${Math.floor(m.max / 2) + 1}`);
// Scoreboard rows: you first, then everyone the snapshots list.
const allRows = (s) => [{ id: s.myId, name: s.name, color: s.myColor, team: s.myTeam, k: s.myK || 0, d: s.myD || 0, a: s.myA || 0, sp: s.mySp || 0, me: true },
  ...s.players.map((p) => ({ a: 0, sp: 0, ...p }))];

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
    feed: [], players: [], myId: 0, scoreboard: false, roundLeft: 0, phase: "warmup", score: null,
    ci: null, work: null, hint: "", carrying: false, roundEnd: null, flash: 0, // rounds (game/rounds.js)
    match: null, halftime: null, matchEnd: null, myA: 0, mySp: 0, // the match: round n/max, overtime, history; banners
    shout: null, subtitle: null, // announcer captions: { id, text, color, dur }
  };
  const subs = new Set();
  const api = {
    get: () => state,
    set(p) { state = { ...state, ...p }; subs.forEach((f) => f(state)); },
    feed(text, kill = null) {
      const item = { id: Math.random(), text, kill, at: performance.now() };
      api.set({ feed: [...state.feed, item].slice(-5) });
      setTimeout(() => api.set({ feed: state.feed.filter((f) => f !== item) }), kill?.mine ? 9000 : 5000);
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
      ${s.screen === "playing" && s.dmgArcs?.length > 0 && html`<div class="dmg-arcs">${s.dmgArcs.map((d) => html`<div class="dmg-arc" key=${d.id} style=${{ transform: `rotate(${d.a}rad)`, opacity: Math.max(0, d.k) }}></div>`)}</div>`}
      ${s.teamMenu && s.screen !== "menu" && !s.console && html`<${TeamMenu} s=${s} />`}
      ${s.screen !== "menu" && html`<${ChatLog} lines=${s.chat || []} open=${!!s.chatOpen} />`}
      ${s.chatOpen && s.screen === "playing" && html`<${ChatInput} team=${s.chatOpen === "team"} send=${actions.say} close=${actions.closeChat} />`}
      ${s.radioOpen && s.screen === "playing" && !s.chatOpen && html`<${RadioMenu} pick=${actions.radio} close=${actions.closeRadio} closeKey=${s.radioKey} />`}
      ${s.buyMenu && s.screen === "playing" && !s.console && !s.teamMenu && html`<${BuyMenu} s=${s} buy=${actions.buy} close=${actions.closeBuyMenu} />`}
      ${(s.screen === "menu" || s.screen === "ingame") && !s.console && !s.teamMenu && html`<${Menu} s=${s} />`}
      ${s.screen === "playing" && !s.aiming && !s.console && !s.teamMenu && !s.buyMenu && html`<div class="aim-hint">click to aim</div>`}
      ${s.scoreboard && s.screen !== "menu" && html`<${Scoreboard} s=${s} />`}
      ${s.vote && s.screen !== "menu" && html`<${MapVote} s=${s} />`}
    `;
  }

  function Menu({ s }) {
    const [view, setView] = useState("main");
    // Esc: Options goes back a level; the in-game menu closes. The console and key
    // rebinding catch Esc in the capture phase first, so it never gets here then.
    useEffect(() => {
      const onKey = (e) => {
        if (e.code !== "Escape" || e.repeat) return;
        if (view === "controls") { setView("options"); return; }
        if (view === "maps") { setView("main"); return; }
        if (view === "options") { setView("main"); return; }
        // The Esc that left pointer lock can arrive as a keydown too; don't let it close the menu it opened.
        if (s.screen === "ingame" && performance.now() - state.menuOpenedAt > 250) actions.closeMenu();
      };
      addEventListener("keydown", onKey);
      return () => removeEventListener("keydown", onKey);
    }, [view, s.screen]);
    if (view === "options") return html`<div class="overlay"><${Options} back=${() => setView("main")} controls=${() => setView("controls")} /></div>`;
    if (view === "controls") return html`<div class="overlay"><${Controls} back=${() => setView("options")} /></div>`;
    if (view === "maps") return html`<div class="overlay"><${Maps} s=${s} back=${() => setView("main")} /></div>`;
    const opts = html`<button type="button" class="secondary" onClick=${(e) => { e.stopPropagation(); setView("options"); }}>OPTIONS</button>
      <button type="button" class="secondary" onClick=${(e) => { e.stopPropagation(); setView("maps"); }}>MAPS</button>`;
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
          <button type="button" class="secondary" onClick=${(e) => { e.stopPropagation(); actions.teamMenu(); }}>CHANGE TEAM (M)</button>
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

  // Map selector: every map with a preview rendered on this client; picking
  // one runs changelevel (an admin command: everyone while the server has no
  // admin password).
  // The selector's sections, in order (content.Categories).
  const MAP_CATEGORIES = [
    { id: "training", name: "Training", desc: "small maps for a few players" },
    { id: "competitive", name: "Competitive", desc: "full-size, CS-scale maps" },
  ];
  function Maps({ s, back }) {
    useEffect(() => { actions.maps?.(); }, []);
    const cards = s.mapCards;
    return html`<div class="panel maps-panel" onClick=${(e) => e.stopPropagation()}>
      <h2>MAPS</h2>
      <p class="maps-hint">${s.openAdmin ? "open server: anyone can change the map · it changes for everyone and restarts the match"
        : "changing the map is for admins (admin_login in the console)"}</p>
      ${!cards ? html`<p>loading…</p>` : !cards.length ? html`<p>couldn't load the maps</p>` : MAP_CATEGORIES.filter((c) => cards.some((m) => m.category === c.id)).map((c) => html`
      <h3 class="map-cat" data-cat=${c.id}>${c.name} <small>${c.desc}</small></h3>
      <div class="map-grid">
        ${cards.filter((m) => m.category === c.id).map((m) => html`
          <button type="button" class="map-card ${s.map === m.name ? "current" : ""}" data-map=${m.name}
            onClick=${() => { if (s.map !== m.name) { actions.changeLevel(m.name); back(); } }}>
            <${Preview} url=${s.mapPreviews?.[m.name]} />
            <span class="mn">${m.name}${s.map === m.name ? html` <em>· playing</em>` : s.pendingMap === m.name ? html` <em>· loads when you join</em>` : ""}</span>
            <span class="mi">${Math.round(m.size[0])}×${Math.round(m.size[1])} m · up to ${m.maxPlayers} players · sites ${m.sites.join(" ")}</span>
            <span class="md">${m.desc}</span>
          </button>`)}
      </div>`)}
      <button type="button" class="secondary" onClick=${back}>BACK</button>
    </div>`;
  }
  const Preview = ({ url }) => url ? html`<img class="map-preview" src=${url} alt="" />` : html`<div class="map-preview pending">rendering…</div>`;

  // End-of-match map vote: three cards with live counts; F1-F3 (or a click,
  // or vote <n> in the console) votes, and you can change your mind.
  function MapVote({ s }) {
    const v = s.vote;
    const [, tick] = useState(0);
    useEffect(() => { const t = setInterval(() => tick((n) => n + 1), 250); return () => clearInterval(t); }, []);
    useEffect(() => {
      const onKey = (e) => {
        const i = { F1: 0, F2: 1, F3: 2 }[e.code];
        if (i === undefined || v.done || !v.options[i]) return;
        e.preventDefault();
        actions.vote(v.options[i]);
      };
      addEventListener("keydown", onKey);
      return () => removeEventListener("keydown", onKey);
    }, [v]);
    const left = v.done ? 0 : Math.max(0, Math.ceil((v.endsAt - performance.now()) / 1000));
    return html`<div class="map-vote">
      <div class="mv-title">${v.done ? html`NEXT MAP: <b>${v.winner}</b>` : html`VOTE FOR THE NEXT MAP <span>${left}s</span>`}</div>
      <div class="mv-cards">
        ${v.options.map((o, i) => html`<button type="button" class="mv-card ${v.mine === o ? "mine" : ""} ${v.done && v.winner === o ? "won" : ""}"
          onClick=${() => actions.vote(o)} disabled=${v.done}>
          <${Preview} url=${s.mapPreviews?.[o]} />
          <span class="mn"><b>F${i + 1}</b> ${o}</span>
          <span class="mv-count">${v.votes?.[o] ?? 0} vote${(v.votes?.[o] ?? 0) === 1 ? "" : "s"}${v.mine === o ? " · yours" : ""}</span>
        </button>`)}
      </div>
    </div>`;
  }

  // Key hints from the current binds.
  function Help() {
    const b = useSettings().binds, k = (cmd) => keyName(keyFor(b, cmd));
    const move = ["+forward", "+moveleft", "+back", "+moveright"].map(k).join("");
    return html`<div class="help">
      <b>${move}</b> move · <b>${k("+speed")}</b> sneak · <b>${k("+duck")}</b> crouch · <b>${k("+jump")}</b> boing · <b>${k("+attack")}</b> pew · <b>${k("reload")}</b> reload · <b>${k("teammenu")}</b> team · <b>${k("+showscores")}</b> scores · <b>${k("toggleconsole")}</b> console · <b>Esc</b> menu
    </div>`;
  }
  const help = html`<${Help} />`;

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

  // Live latency numbers and what the browser granted; * flags apply on reload.
  function LatencyReadout() {
    const [r, setR] = useState(() => actions.latency?.());
    useEffect(() => { const t = setInterval(() => setR(actions.latency?.()), 500); return () => clearInterval(t); }, []);
    const v = useSettings();
    if (!r) return null;
    const stale = !!v.r_desynchronized !== r.requested.desynchronized || v.r_power_preference !== r.requested.powerPreference;
    return html`<div class="readout">
      ${r.lines.map((l) => html`<div>${l}</div>`)}
      <div>GPU: ${r.info.gpu}${r.info.timerQuery ? "" : " (no GPU timer queries)"}</div>
      ${stale ? html`<div class="row"><span>* changed: applies after a reload</span>
        <button type="button" onClick=${() => actions.reloadPage?.()}>RELOAD NOW</button></div>` : ""}
    </div>`;
  }

  // Options → Controls: one key per action, × clears, a taken key moves (and flashes the row it left).
  function Controls({ back }) {
    const v = useSettings();
    const [wait, setWait] = useState(null);   // action waiting for a key
    const [flash, setFlash] = useState(null); // action that just lost its key
    useLayoutEffect(() => { // layout: listen as soon as the button says "press a key"
      if (!wait) return;
      const done = (code) => {
        setWait(null);
        if (!code) return;
        const { binds, moved } = bindKey(settings.get().binds, code, wait);
        settings.set({ binds });
        if (moved) { setFlash(moved); setTimeout(() => setFlash((f) => (f === moved ? null : f)), 1400); }
      };
      const onKey = (e) => { e.preventDefault(); e.stopPropagation(); done(e.code === "Escape" ? null : e.code); };
      const onMouse = (e) => { e.preventDefault(); e.stopPropagation(); done(`Mouse${e.button}`); };
      const onWheel = (e) => { e.preventDefault(); done(e.deltaY > 0 ? "MWheelDown" : "MWheelUp"); };
      const noMenu = (e) => e.preventDefault();
      addEventListener("keydown", onKey, true);
      addEventListener("mousedown", onMouse, true);
      addEventListener("wheel", onWheel, { capture: true, passive: false });
      addEventListener("contextmenu", noMenu, true);
      return () => {
        removeEventListener("keydown", onKey, true);
        removeEventListener("mousedown", onMouse, true);
        removeEventListener("wheel", onWheel, { capture: true });
        removeEventListener("contextmenu", noMenu, true);
      };
    }, [wait]);
    const check = (k, label) => html`<label class="row check"><input type="checkbox" checked=${!!v[k]} onChange=${(e) => settings.set({ [k]: e.target.checked ? 1 : 0 })} /> ${label}</label>`;
    return html`<div class="panel options controls" onClick=${(e) => e.stopPropagation()}>
      <h2>CONTROLS</h2>
      <section>
        ${ACTIONS.map((a) => {
          const code = keyFor(v.binds, a.cmd);
          return html`<div class="row bind ${flash === a.cmd ? "flash" : ""}" key=${a.cmd}>
            <span>${a.label}</span>
            <button type="button" class="rebind ${wait === a.cmd ? "waiting" : ""} ${code ? "" : "unbound"}"
              onClick=${(e) => { e.stopPropagation(); setTimeout(() => setWait(a.cmd), 0); }}>
              ${wait === a.cmd ? "press a key, button or wheel… (Esc cancels)" : keyName(code)}</button>
            <button type="button" class="clear" title="clear" disabled=${!code} onClick=${() => settings.set({ binds: clearAction(v.binds, a.cmd) })}>×</button>
          </div>`;
        })}
        <div class="hint">Taking a key that another action uses moves it here; that action is left unbound. The console's
          <code>bind</code>/<code>unbind</code> edit the same binds (e.g. <code>bind mwheeldown +jump</code>).</div>
        ${check("cl_fullscreen", "Fullscreen play (needed for Ctrl crouch)")}
        <div class="hint">In a normal window the browser owns Ctrl+W and closes the tab. Fullscreen play locks the keyboard
          to the game (Chrome/Edge), so Ctrl+W just crouch-walks. Leaving mid-match always asks first.</div>
      </section>
      <div class="buttons">
        <button type="button" class="secondary" onClick=${() => settings.resetBinds()}>RESET TO DEFAULTS</button>
        <button type="button" onClick=${back}>BACK</button>
      </div>
    </div>`;
  }

  function Options({ back, controls }) {
    const v = useSettings();
    const st = actions.loop?.()?.stats();
    const hz = st ? Math.round(st.hz) : 60;
    const div = v.fps_limiter !== "latency" && v.fps_limit === "custom" && v.fps_max > 0 ? Math.min(4, Math.max(1, Math.round(hz / v.fps_max))) : 1;
    const counts360 = Math.round(360 / (v.sensitivity * M_YAW));
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
        <div class="row"><span>Key binds</span>
          <button type="button" class="rebind" onClick=${controls}>CONTROLS ›</button></div>
      </section>
      <section>
        <h4>Video</h4>
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
        <h4>Frame pacing & latency <span class="exp">experimental</span></h4>
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
        ${v.cl_loop === "unlocked" && html`<div class="row"><span>Unlocked FPS cap</span>
          <select class="fps-unlocked-preset" value=${[0, 240, 360, 500, 1000, 2000].includes(+v.fps_max_unlocked) ? String(v.fps_max_unlocked) : "custom"}
            onChange=${(e) => { if (e.target.value !== "custom") set("fps_max_unlocked", e.target.value); }}>
            ${[240, 360, 500, 1000, 2000].map((n) => html`<option value=${String(n)}>${n} fps</option>`)}
            <option value="0">Uncapped</option>
            <option value="custom">Custom…</option>
          </select>
          <input class="fps-unlocked num" type="number" min="0" max="100000" step="1" value=${v.fps_max_unlocked}
            onChange=${(e) => set("fps_max_unlocked", Math.max(0, Math.round(+e.target.value || 0)))} /></div>`}
        <div class="hint">Browsers always show the page at the display's rate. Unlocked runs the game evenly paced at ${v.fps_max_unlocked > 0 ? `up to ${v.fps_max_unlocked} fps` : "uncapped fps"} (console: <code>fps_max_unlocked</code>, 0 = uncapped)
          so the frame that gets shown is fresher, at the cost of CPU/GPU time. The game simulation runs at a fixed tick
          either way. Showing more than the refresh rate needs browser flags, e.g. Chrome
          <code>--disable-frame-rate-limit --disable-gpu-vsync</code>.</div>
        <div class="row"><span>GPU queue limit</span>
          <select value=${String(v.r_gpu_fence)} onChange=${(e) => set("r_gpu_fence", e.target.value)}>
            <option value="0">Off (driver decides)</option>
            <option value="1">1 frame in flight</option>
            <option value="2">2 frames in flight</option>
          </select></div>
        <div class="hint">A Reflex-like limiter: a GPU fence per frame; while too many frames are still queued on the GPU the
          frame is skipped (the game keeps simulating), so the driver queue never grows.</div>
        ${check("r_late_input", "Late input latch (VSync mode: render as late as possible before the display deadline)")}
        ${check("r_finish", "gl.finish() after every frame (crude queue drain, for comparison)")}
        ${check("cl_netdebug", "Net debug readout (commands, backlog, server queue, prediction, last shot)")}
        ${check("r_desynchronized", "Desynchronized canvas (low-latency hint, skips the compositor queue where supported) *")}
        <div class="row"><span>GPU power</span>
          <select value=${v.r_power_preference} onChange=${(e) => set("r_power_preference", e.target.value)}>
            <option value="default">Default</option>
            <option value="high-performance">High performance</option>
            <option value="low-power">Low power</option>
          </select> *</div>
        <${LatencyReadout} />
        <div class="hint">On Windows/NVIDIA, Chrome renders via D3D11: the driver's Low Latency Mode for chrome.exe also affects this.</div>
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
      ${s.myTeam === SPECTATOR && html`<div class="spec-banner">SPECTATING · press <b>${keyName(keyFor(settings.get().binds, "teammenu"))}</b> to pick a team</div>`}
      ${s.observe ? html`<${Observe} o=${s.observe} />`
        : s.myTeam !== SPECTATOR && (s.scoped === "lens" ? html`<${Scope} />` : html`<div class="crosshair"><i class="l"></i><i class="r"></i><i class="t"></i><i class="b"></i></div>`)}
      <div class="vignette"></div>
      ${s.shout && html`<div class="shout" key=${s.shout.id} style=${{ color: s.shout.color, "--dur": `${s.shout.dur}s` }}>${s.shout.text}</div>`}
      ${s.subtitle && html`<div class="subtitle" key=${s.subtitle.id} style=${{ "--dur": `${s.subtitle.dur}s` }}><span style=${{ color: s.subtitle.color }}>${s.subtitle.text}</span></div>`}
      <div class="top-mid">
        <span class="team-score" style=${{ color: TEAMS.vibe.color }}><span class="long">${TEAMS.vibe.name.toUpperCase()}</span><span class="short">${TEAMS.vibe.short}</span> ${s.score?.vibe ?? 0}</span>
        ${s.ci ? html`<span class="timer ci"><span class="ci-label">CI ${s.ci.site}</span><span class="ci-bar"><i style=${{ width: `${s.ci.frac * 100}%` }}></i></span><span class="ci-left">${s.ci.left}s</span></span>`
          : html`<span class="timer ${s.phase}">${PHASE_LABEL[s.phase] ? html`${PHASE_LABEL[s.phase]}${s.phase === "warmup" && s.roundLeft > 0 ? html` <small>${m}:${sec}</small>` : ""}` : `${m}:${sec}`}</span>`}
        <span class="team-score" style=${{ color: TEAMS.artisan.color }}>${s.score?.artisan ?? 0} <span class="long">${TEAMS.artisan.name.toUpperCase()}</span><span class="short">${TEAMS.artisan.short}</span></span>
      </div>
      ${s.match?.max > 0 && !["warmup", "matchover"].includes(s.phase) && html`<div class="round-no">${roundLabel(s.match)}</div>`}
      ${s.phase === "freeze" && !s.roundEnd && html`<div class="freeze-note">code freeze · ${s.roundLeft}</div>`}
      ${s.halftime && s.phase === "halftime" && html`<div class="round-end" key="half">
        <div class="re-title">HALFTIME</div>
        <div class="re-line">${TEAMS.vibe.name} ${s.halftime.sc?.vibe ?? 0} : ${s.halftime.sc?.artisan ?? 0} ${TEAMS.artisan.name} · switching sides…</div></div>`}
      ${s.matchEnd && html`<${MatchEnd} s=${s} />`}
      ${s.work && html`<div class="work"><div class="work-label">${s.work.other ? `${s.work.other}: ` : ""}${s.work.kind === "revert" ? "git revert The Big Diff…" : "git push --force origin main…"}</div>
        <div class="work-bar ${s.work.kind}"><i style=${{ width: `${s.work.pr * 100}%` }}></i></div></div>`}
      ${s.hint && !s.work && html`<div class="bomb-hint">${s.hint}</div>`}
      ${s.roundEnd && html`<div class="round-end" key=${s.roundEnd.n}>
        <div class="re-title" style=${{ color: s.roundEnd.color }}>${s.roundEnd.name.toUpperCase()} WIN!</div>
        <div class="re-line">${s.roundEnd.text}</div>
        ${s.mvp && html`<div class="re-mvp ${s.mvp.me ? "me" : ""}">★ MVP: <b>${s.mvp.name}</b> <span>${s.mvp.why}</span></div>`}
        ${s.dmgReport && html`<table class="re-dmg"><tr><th></th><th>given</th><th>taken</th></tr>
          ${s.dmgReport.map((r) => html`<tr key=${r.id}><td>${r.name}</td><td>${r.gh ? `${r.g} (${r.gh})` : "–"}</td><td>${r.th ? `${r.t} (${r.th})` : "–"}</td></tr>`)}</table>`}</div>`}
      ${s.flash ? html`<div class="blast-flash" key=${s.flash}></div>` : null}
      <div class="top-left">
        ${s.myTeam !== SPECTATOR && s.canBuy && html`<div class="buyzone" title="buy zone: press ${s.buyKeyName || "B"}">🛒 ${s.buyKeyName || "B"}</div>`}
        ${s.myTeam !== SPECTATOR && s.armor > 0 && html`<div class="armor">${s.helmet ? "⛑" : "🛡"} ${s.armor}</div>`}
        ${s.myTeam !== SPECTATOR && (s.shades || s.boost > 0) && html`<div class="perks">${s.shades && html`<span title="Senior-dev Sunglasses: flashes blind you 60% shorter">😎</span>`}${s.boost > 0 && html`<span class="boost" title="10x Energy Drink: +10% move speed">⚡ ${s.boost}s</span>`}</div>`}
        ${s.myTeam !== SPECTATOR && html`<div class="money">$${s.money}
          ${s.moneyPop && performance.now() - s.moneyPop.at < 3000 && html`<div class="money-pop ${s.moneyPop.d < 0 ? "neg" : ""}" key=${s.moneyPop.at}>${s.moneyPop.d > 0 ? "+" : "−"}$${Math.abs(s.moneyPop.d)} <span>${s.moneyPop.why}</span></div>`}</div>`}
        ${TEAMS[s.myTeam] && html`<div class="my-team" style=${{ color: TEAMS[s.myTeam].color }}>${TEAMS[s.myTeam].name} · M to switch</div>`}
        <div class="net ${s.connected ? "" : "bad"}">${s.connected ? `${s.map} · ${s.ping ?? "…"} ms` : "reconnecting… (stuck? reload the page)"}</div>
      </div>
      <div class="feed">${s.feed.map((f) => (f.kill ? html`<${KillRow} key=${f.id} k=${f.kill} />` : html`<div key=${f.id}>${f.text}</div>`))}</div>
      ${s.myTeam !== SPECTATOR && !s.observe && html`
        <div class="bottom-left">
          <span class="hp">✚ ${s.hp}</span><span class="armor">◈ ${s.armor}</span>${s.carrying && html`<span class="carry-diff" title="You carry The Big Diff">±BIG DIFF</span>`}
        </div>
        <div class="slots">${s.slots.filter((x) => x.name).map((x) => html`<div key=${x.slot} class=${x.cur ? "cur" : ""}><b>${x.key}</b> ${x.name}</div>`)}</div>
        <div class="bottom-right ${s.reloading ? "reloading" : ""}">
          <span class="ammo">${s.reloading ? "slopping…" : s.ammo}</span>${s.reserve !== "" && html`<span class="reserve">/ ${s.reserve}</span>`}
        </div>`}`;
  }

  // Code Review's scope: a lens framed by a unified diff of your target's life choices.
  const DIFF = ["@@ -1,7 +1,7 @@ class Opponent", "-  alive = true", "+  alive = false", "   // TODO: dodge", "-  return position", "+  return respawn()", " Changes requested."];
  function Scope() {
    return html`<div class="scope">
      <div class="lens"></div><i class="h"></i><i class="v"></i>
      <pre class="diff">${DIFF.map((l) => html`<span class=${l[0] === "-" ? "del" : l[0] === "+" ? "add" : l[0] === "@" ? "hunk" : ""}>${l}</span>`)}</pre>
    </div>`;
  }

  // Observer HUD: the death cam caption, or whom you follow (their HP, weapon
  // and ammo) with the camera mode and controls. A crosshair in first person.
  function Observe({ o }) {
    const tc = (t) => TEAMS[t]?.color ?? "var(--hud)";
    if (o.freeze) {
      return html`<div class="deathcam">
        <div class="dc-title">REVIEWED</div>
        <div class="dc-line">you got reviewed by <b style=${{ color: tc(o.killerTeam) }}>${o.killer}</b>${o.weapon && html` with <b>${o.weapon}</b>`}</div>
      </div>`;
    }
    const b = settings.get().binds, k = (c) => keyName(keyFor(b, c));
    return html`
      ${o.first && html`<div class="crosshair"><i class="l"></i><i class="r"></i><i class="t"></i><i class="b"></i></div>`}
      <div class="observe">
        ${o.name
          ? html`<div class="ob-who"><b style=${{ color: tc(o.team) }}>${o.name}</b>
              <span class="ob-hp">✚ ${o.hp}</span>${o.weapon && html`<span class="ob-gun">${o.weapon} ${o.ammo} / ${o.res}</span>`}</div>`
          : o.mode !== "free" && o.mode !== "overview" && html`<div class="ob-who">nobody to follow</div>`}
        <div class="ob-mode">${o.dead ? "dead · " : ""}${o.modeName} · <b>${k("+attack")}</b> next · <b>${k("+zoom")}</b> previous · <b>${k("+jump")}</b> camera</div>
      </div>`;
  }

  // Match over: the winner, the final score, the top players by score points;
  // the next match (or the map vote's map) follows after mp_match_restart_delay.
  function MatchEnd({ s }) {
    const e = s.matchEnd, rows = allRows(s).filter((p) => TEAMS[p.team]);
    const top = [...rows].sort((a, b) => b.sp - a.sp || b.k - a.k).slice(0, 3);
    return html`<div class="match-end">
      <div class="me-title" style=${{ color: e.color }}>${e.win === "draw" ? "DRAW" : html`${e.name.toUpperCase()} WIN THE MATCH`}</div>
      <div class="me-score"><span style=${{ color: TEAMS.vibe.color }}>${e.sc?.vibe ?? 0}</span> : <span style=${{ color: TEAMS.artisan.color }}>${e.sc?.artisan ?? 0}</span></div>
      ${top.length > 0 && html`<table class="me-top"><tr><th></th><th>top players</th><th>K</th><th>A</th><th>D</th><th>pts</th></tr>
        ${top.map((p, i) => html`<tr key=${p.id} class=${p.me ? "me" : ""}><td>${i === 0 ? "★ MVP" : i + 1}</td>
          <td style=${{ color: TEAMS[p.team]?.color }}>${p.name}</td><td>${p.k}</td><td>${p.a}</td><td>${p.d}</td><td>${p.sp}</td></tr>`)}</table>`}
      <div class="me-next">next match in ${s.roundLeft}s</div>
    </div>`;
  }

  // Round-history strip: one cell per played round in the winner's colour, a glyph for how.
  function History({ m }) {
    if (!m?.hist?.length) return null;
    const half = Math.floor(m.max / 2);
    return html`<div class="sb-hist">${m.hist.map((h, i) => html`${i === half && i > 0 ? html`<i class="sb-half" title="halftime"></i>` : ""}
      <span key=${i} class="sb-round" title=${`round ${i + 1}: ${TEAMS[h.w]?.name} (${h.why})`} style=${{ background: TEAMS[h.w]?.color }}>${HOW[h.why] ?? "·"}</span>`)}</div>`;
  }

  function Scoreboard({ s }) {
    const rows = allRows(s);
    return html`<div class="scoreboard">
      <h3>${s.map} — ${rows.length} slopper${rows.length === 1 ? "" : "s"}${s.match?.max > 0 ? html` · <span class="sb-round-no">${roundLabel(s.match)}</span>` : ""}</h3>
      <${History} m=${s.match} />
      ${TEAM_IDS.map((id) => {
        const t = TEAMS[id], members = rows.filter((p) => p.team === id);
        return html`<div class="sb-team" key=${id}>
          <div class="sb-head" style=${{ color: t.color, borderColor: t.color }}>
            <span>${t.name}</span><span class="sb-role">${t.role} · ${members.length}</span><span class="sb-score">${s.score?.[id] ?? 0}</span></div>
          <table><tr><th></th><th>name</th><th>K</th><th>A</th><th>D</th><th>pts</th><th>ping</th></tr>
          ${[...members].sort((a, b) => b.sp - a.sp || b.k - a.k).map((p) => html`<tr key=${p.id} class=${p.me ? "me" : ""}>
            <td><i class="swatch" style=${{ background: p.color }}></i></td>
            <td>${p.name}</td><td>${p.k ?? 0}</td><td>${p.a ?? 0}</td><td>${p.d ?? 0}</td><td>${p.sp ?? 0}</td><td>${p.me ? s.ping ?? "…" : "?"}</td></tr>`)}
          </table>${!members.length && html`<div class="sb-empty">nobody yet</div>`}</div>`;
      })}
      ${(() => {
        const rest = rows.filter((p) => !TEAMS[p.team]);
        return rest.length > 0 && html`<div class="sb-spec">spectating: ${rest.map((p, i) => html`${i ? ", " : ""}<span class=${p.me ? "me" : ""}>${p.name}</span>`)}</div>`;
      })()}
    </div>`;
  }

  // Team select: on join and on M. 1 / 2 pick a side, 5 auto-assigns, 6 spectates.
  // A side that would end up 2+ players bigger is shown full (the server checks too).
  function TeamMenu({ s }) {
    const all = [{ team: s.myTeam }, ...s.players];
    const counts = Object.fromEntries(TEAM_IDS.map((id) => [id, all.filter((p) => p.team === id).length]));
    const others = { ...counts };
    if (TEAMS[s.myTeam]) others[s.myTeam]--; // you don't count against the side you'd join
    const open = Object.fromEntries(TEAM_IDS.map((id) => [id, s.myTeam === id || teamOpen(id, others)]));
    const spectators = all.filter((p) => p.team === SPECTATOR).length;
    useEffect(() => {
      const onKey = (e) => {
        if (e.code === "Escape" || e.code === keyFor(settings.get().binds, "teammenu")) { e.preventDefault(); actions.closeTeamMenu(); return; }
        const pick = { Digit1: "vibe", Digit2: "artisan", Digit5: "auto", Digit6: SPECTATOR }[e.code];
        if (pick && (!TEAMS[pick] || open[pick])) actions.pickTeam(pick);
      };
      addEventListener("keydown", onKey);
      return () => removeEventListener("keydown", onKey);
    }, [open.vibe, open.artisan]);
    return html`<div class="overlay"><div class="panel team-menu" onClick=${(e) => e.stopPropagation()}>
      <h2>PICK A SIDE</h2>
      <div class="team-cards">
        ${TEAM_IDS.map((id, i) => {
          const t = TEAMS[id], full = !open[id];
          return html`<button type="button" class="team-card ${s.myTeam === id ? "current" : ""} ${full ? "full" : ""}" style=${{ "--tc": t.color }}
            disabled=${full} onClick=${() => actions.pickTeam(id)}>
            <b>${i + 1}</b><span class="tn">${t.name}</span>
            <span class="tr">${t.role} · ${counts[id]} playing${s.myTeam === id ? " · you" : ""}</span>
            ${full ? html`<span class="tfull">FULL: would be 2+ bigger</span>` : html`<i>"${t.taunts[0]}"</i>`}</button>`;
        })}
      </div>
      <div class="team-extra">
        <button type="button" class="secondary" onClick=${() => actions.pickTeam("auto")}>5 · AUTO-ASSIGN</button>
        <button type="button" class="secondary ${s.myTeam === SPECTATOR ? "current" : ""}" onClick=${() => actions.pickTeam(SPECTATOR)}>6 · SPECTATE${spectators ? ` (${spectators})` : ""}</button>
      </div>
      <p class="team-hint">${s.myTeam === SPECTATOR ? "you are spectating · Esc to keep watching" : "switching sides respawns you · Esc to cancel"}</p>
    </div></div>`;
  }

  // One kill-feed row: killer, weapon icon (+ noscope / headshot flair), victim,
  // in team colours; rows you're part of get an outline and stay longer.
  function KillRow({ k }) {
    const col = (t) => TEAMS[t]?.color || "#ddd";
    const icon = KILL_GLYPH[k.weapon] ? "" : weaponIcon(k.weapon); // a glyph wins (the Big Diff's blast is 💥, not its model)
    return html`<div class="kill-row ${k.mine ? "mine" : ""}">
      ${k.killer !== k.victim && html`<span class="kn" style=${{ color: col(k.kteam) }}>${k.killer}</span>`}
      ${icon ? html`<img class="kw" src=${icon} alt=${k.wname} title=${k.wname} />` : html`<span class="kg" title=${k.wname}>${KILL_GLYPH[k.weapon] || KILL_GLYPH.world}</span>`}
      ${k.ns && html`<span class="kf" title="no scope">⌀</span>`}
      ${k.hs && html`<span class="kf kh" title="headshot">✚</span>`}
      <span class="kn" style=${{ color: col(k.vteam) }}>${k.victim}</span>
    </div>`;
  }

  render(html`<${App} />`, el);
  return api;
}


function loadName() { try { return localStorage.getItem("cs.name") || ""; } catch { return ""; } }
function saveName(n) { try { localStorage.setItem("cs.name", n); } catch {} }

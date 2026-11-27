// Preact HUD + menu overlay. Game code pushes state with hud.set({...}).
import { h, render, options } from "preact";
import { prof } from "../engine/prof.js";

// Preact's batched re-renders, timed as section "ui" (engine/prof.js); the same microtask Preact uses by default.
options.debounceRendering = (cb) => Promise.resolve().then(() => prof.scope("ui", cb));
import { useState, useEffect, useLayoutEffect, useRef } from "preact/hooks";
import htm from "htm";
import { settings, CVARS, M_YAW, coerce } from "./settings.js";
import { nadeTag } from "./nadetag.js";
import { editorPanel } from "./editorpanel.js";
import { wantsAim } from "./cursor.js";
import { copyText } from "./clipboard.js";
import { XHAIR_DEFAULTS, xhairVars, xhairCode, parseXhairCode } from "./xhair.js";
import { AA_LABELS, aaChoices } from "../engine/aa.js";
import { ACTIONS, keyFor, keyName, bindKey, clearAction } from "./binds.js";
import { TEAMS, TEAM_IDS, SPECTATOR, teamOpen, teamCapped, team as teamInfo } from "../assets/teams.js";
import { BuyMenu } from "./buymenu.js";
import { ServerSettings } from "./serversettings.js";
import { ChatLog, ChatInput, RadioMenu } from "./chat.js";
import { weaponIcon, KILL_GLYPH } from "./weaponicon.js";
import { rawInputHint } from "../engine/browser.js";
import { byScore, aliveCount } from "./scoreorder.js";
import { boatWords } from "../game/boatseat.js";
import { Backlog, DepartureBoard, PointsStrip, SpawnScreen } from "./tickets.js";
import { RaceBar, Standings, RaceCentre, RaceResults } from "./race.js";
import { Garage } from "./garage.js";

const html = htm.bind(h);

// The match HUD: phase labels for the timer, the round counter, history glyphs.
const PHASE_LABEL = { warmup: "WARMUP", halftime: "HALFTIME", matchover: "MATCH OVER" };
const HOW = { elimination: "✕", exploded: "✹", defused: "↺", time: "⏱" };
const roundLabel = (m) => (m.ot > 0 ? `OVERTIME ${m.ot} · ROUND ${m.n}` : `ROUND ${m.n} / ${m.max} · first to ${Math.floor(m.max / 2) + 1}`);
// Scoreboard rows: you first, then everyone the snapshots list.
const allRows = (s) => [{ id: s.myId, name: s.name, color: s.myColor, team: s.myTeam, k: s.myK || 0, d: s.myD || 0, a: s.myA || 0, sp: s.mySp || 0, dead: !!s.myDead, me: true },
  ...s.players.map((p) => ({ a: 0, sp: 0, ...p }))];

// The Gravity Override's hook meter (internal/sim hook.go): drains while
// hooked and per shot, refills unhooked; at 0 the hook lets go.
function EnergyMeter({ s }) {
  const frac = Math.max(0, Math.min(1, s.energy / (s.energyMax || 100)));
  return html`<span class="energy ${frac < 0.2 ? "low" : ""}" title="Gravity Override: hook energy (F)">⤒<span class="ebar"><i style=${{ width: `${Math.round(frac * 100)}%` }}></i></span></span>`;
}

// In a boat (game/boatseat.js): the role, a parody tag and the keys; near
// one on foot, just how to sit down.
function BoatBadge({ b }) {
  const binds = settings.get().binds, w = boatWords(b, (cmd) => keyName(keyFor(binds, cmd)));
  return html`<div class="boat-badge ${b.role}">${w.title && html`<div class="bb-title">${w.title} <span class="bb-tag">${w.tag}</span></div>`}
    <div class="bb-keys">${w.keys.map((x, i) => html`${i > 0 && html`<i> · </i>`}<span key=${i}>${x}</span>`)}</div></div>`;
}

// Driving (game/cars.js carHud): the speedometer and the car's name; near a
// free car on foot, how to get in. The car's HP sits next to ours (CarHP).
function CarBadge({ c }) {
  const k = keyName(keyFor(settings.get().binds, "+use")), v = keyName(keyFor(settings.get().binds, "carcam"));
  if (c.role === "near") return html`<div class="car-badge near">${k} drive the ${c.name}</div>`;
  const kf = (cmd) => keyName(keyFor(settings.get().binds, cmd));
  const item = (it, cmd) => it && html`<span class="ci ${it.n ? "" : "empty"}" title=${it.name}><b>${kf(cmd)}</b> ${it.name}${it.kind !== "ram" ? html` <em>×${it.n}</em>` : ""}</span>`;
  return html`<div class="car-badge ${c.slip ? "slipping" : ""}"><div class="speedo"><b>${c.kmh}</b><span>km/h</span></div>
    <div class="cb-name">${c.name}${c.pit ? html` <span class="cb-pit">· PIT LANE: repairing</span>` : ""}${c.shield ? html` <span class="cb-shield">· CODE FREEZE: no damage</span>` : ""}</div>
    ${(c.front || c.rear) && html`<div class="car-items">${item(c.front, "+carfront")}${item(c.rear, "+carrear")}
      <span class="ci boost ${c.boost > 0 ? "" : "empty"}" title="Boost left this lap"><b>${kf("+boost")}</b> boost <em>${c.boost.toFixed(1)}s</em></span></div>`}
    <div class="bb-keys">${k} get out<i> · </i>${v} camera</div></div>`;
}
function CarHP({ c }) {
  const frac = Math.max(0, Math.min(1, c.hp / (c.maxHP || 1)));
  return html`<span class="car-hp ${frac < 0.3 ? "low" : ""}" title="Your car's HP">⛍ ${c.hp}<span class="ebar"><i style=${{ width: `${Math.round(frac * 100)}%` }}></i></span></span>`;
}

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
    boat: null, // in or near a boat (game/boatseat.js boatHud)
    car: null, // driving or near a car (game/cars.js carHud)
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
      ${s.garageOpen && s.screen === "playing" && !s.console ? html`<${Garage} s=${s} act=${actions.garage} />` : s.screen !== "menu" && html`<${Hud} s=${s} />`}
      ${s.loadingMap && s.screen !== "menu" && html`<div class="map-loading"><div class="spinner"></div>loading ${s.loadingMap}…</div>`}
      ${s.screen === "playing" && s.dmgArcs?.length > 0 && html`<div class="dmg-arcs">${s.dmgArcs.map((d) => html`<div class="dmg-arc" key=${d.id} style=${{ transform: `rotate(${d.a}rad)`, opacity: Math.max(0, d.k) }}></div>`)}</div>`}
      ${s.teamMenu && s.screen !== "menu" && !s.console && html`<${TeamMenu} s=${s} />`}
      ${s.screen !== "menu" && html`<${ChatLog} lines=${s.chat || []} open=${!!s.chatOpen} />`}
      ${s.chatOpen && s.screen === "playing" && html`<${ChatInput} team=${s.chatOpen === "team"} send=${actions.say} close=${actions.closeChat} />`}
      ${s.editor && s.screen === "playing" && !s.scoreboard && editorPanel(html, s.editor)}
      ${s.radioOpen && s.screen === "playing" && !s.chatOpen && html`<${RadioMenu} pick=${actions.radio} close=${actions.closeRadio} closeKey=${s.radioKey} />`}
      ${s.buyMenu && s.screen === "playing" && !s.console && !s.teamMenu && html`<${BuyMenu} s=${s} buy=${actions.buy} buyAmmo=${actions.buyAmmo} close=${actions.closeBuyMenu} closeOnBuy=${!!settings.get().cl_buy_close} />`}
      ${(s.screen === "menu" || s.screen === "ingame") && !s.console && !s.teamMenu && html`<${Menu} s=${s} />`}
      ${s.crouchNotice && s.screen === "playing" && !s.console && !s.teamMenu && html`<${CrouchNotice} />`}
      ${s.tickets?.spawn && s.screen === "playing" && !s.console && !s.teamMenu && !s.scoreboard && html`<${SpawnScreen} s=${s} map=${actions.currentMap?.()} deploy=${actions.deploy} kit=${actions.kit} />`}
      ${wantsAim(s) && html`<div class="aim-hint">click or press any key to aim</div>`}
      ${s.replay && s.screen === "playing" && html`<${ReplayBar} r=${s.replay} marks=${s.replayMarks || []} aiming=${s.aiming} />`}
      ${s.scoreboard && s.screen !== "menu" && html`<${Scoreboard} s=${s} />`}
      ${s.vote && s.screen !== "menu" && html`<${MapVote} s=${s} />`}
      ${!s.replay && html`<${LinkStatus} v=${s.link} inMatch=${s.screen === "playing"} />`}
    `;
  }

  // Browsers without keyboard lock (Firefox, Safari) keep Ctrl+W/T/N even in
  // fullscreen: crouch-walking on Ctrl would close the tab. Asked once (main.js).
  function CrouchNotice() {
    return html`<div class="overlay"><div class="panel crouch-notice" onClick=${(e) => e.stopPropagation()}>
      <h2>CTRL IS THE BROWSER'S HERE</h2>
      <p>This browser can't hand Ctrl+W, Ctrl+T and Ctrl+N to the game, not even in fullscreen (Chrome and Edge can).
        Crouch-walking forward on Ctrl is Ctrl+W: close the tab (it asks first).</p>
      <div class="buttons">
        <button type="button" class="crouch-on-c" onClick=${() => actions.crouchNotice(true)}>CROUCH ON C</button>
        <button type="button" class="secondary keep-ctrl" onClick=${() => actions.crouchNotice(false)}>KEEP CTRL</button>
      </div>
      <div class="hint">Change it any time in Options → Controls.</div>
    </div></div>`;
  }

  // The server link (game/link.js): a blip is a small banner over the running
  // game; lost is a full-screen overlay over whatever is showing, menus too
  // (they talk to the server as well). It fades out when snapshots return.
  // In play it offers MAIN MENU; in the menus that would be where you are.
  function LinkStatus({ v, inMatch }) {
    const last = useRef(null), retry = useRef(null);
    if (v) last.current = v;
    const shown = v || last.current, lost = shown?.mode === "lost";
    useEffect(() => { // keyboard: Enter retries, unless something else (the console, chat) has focus
      if (v?.mode === "lost" && retry.current && (!document.activeElement || document.activeElement === document.body)) retry.current.focus();
    }, [v?.mode]);
    if (!shown) return null;
    const gone = v ? "" : " link-gone";
    if (!lost) {
      return html`<div class=${"link-blip" + gone} role="status">
        <${Unplugged} w=${84} /><span class="lb-text">CONNECTION INTERRUPTED</span><span class="lb-secs">${shown.secs.toFixed(1)} s</span>
      </div>`;
    }
    const line = shown.why === "closed"
      ? `Reconnecting… attempt ${shown.attempt} · ${shown.nextIn > 0 ? `next try in ${shown.nextIn} s` : "trying now"}`
      : `No word from the server for ${shown.secs.toFixed(1)} s · waiting…`;
    return html`<div class=${"link-lost" + (inMatch ? "" : " over-menu") + gone} role="alertdialog" aria-labelledby="link-title">
      <div class="ll-panel">
        <${Unplugged} w=${560} />
        <h2 id="link-title">CONNECTION LOST</h2>
        <p class="ll-line">${line}</p>
        <div class="ll-buttons">
          <button type="button" class="ll-retry" ref=${retry} onClick=${() => actions.retryNow()}>RETRY NOW</button>
          ${inMatch && html`<button type="button" class="ll-menu" onClick=${() => actions.leave()}>MAIN MENU</button>`}
        </div>
        <p class="ll-hint">Still stuck? <button type="button" class="ll-reload" onClick=${() => actions.reloadPage()}>Reload the page</button></p>
      </div>
    </div>`;
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
        if (view === "replays") { setView("main"); return; }
        if (view === "server") { setView("main"); return; }
        if (view === "options") { setView("main"); return; }
        // The Esc that left pointer lock can arrive as a keydown too; don't let it close the menu it opened.
        if (s.screen === "ingame" && performance.now() - state.menuOpenedAt > 250) actions.closeMenu();
      };
      addEventListener("keydown", onKey);
      return () => removeEventListener("keydown", onKey);
    }, [view, s.screen]);
    if (view === "options") return html`<div class="overlay"><${Options} s=${s} back=${() => setView("main")} controls=${() => setView("controls")} /></div>`;
    if (view === "controls") return html`<div class="overlay"><${Controls} back=${() => setView("options")} /></div>`;
    if (view === "maps") return html`<div class="overlay"><${Maps} s=${s} back=${() => setView("main")} /></div>`;
    if (view === "replays") return html`<div class="overlay"><${Replays} s=${s} back=${() => setView("main")} /></div>`;
    if (view === "server") return html`<div class="overlay"><${ServerSettings} s=${s} back=${() => setView("main")}
      send=${(cvar, v) => actions.rcon(cvar, v)} loadMaps=${() => actions.mapList?.()} /></div>`;
    const opts = html`<button type="button" class="secondary" onClick=${(e) => { e.stopPropagation(); setView("options"); }}>OPTIONS</button>
      <button type="button" class="secondary" onClick=${(e) => { e.stopPropagation(); setView("maps"); }}>MAPS</button>
      <button type="button" class="secondary replays-btn" onClick=${(e) => { e.stopPropagation(); setView("replays"); }}>REPLAYS</button>
      ${s.admin && html`<button type="button" class="secondary server-settings-btn" onClick=${(e) => { e.stopPropagation(); setView("server"); }}>SERVER SETTINGS</button>`}`;
    const input = useRef();
    const go = (e) => {
      e?.preventDefault();
      const name = (input.current?.value || "").trim().slice(0, 16) || "slopper";
      saveName(name);
      api.set({ name });
      actions.play(name);
    };
    if (s.screen === "ingame" && s.replay) {
      return html`<div class="overlay" onClick=${() => actions.closeMenu()}>
        <div class="panel"><h2>REPLAY</h2><p>${s.replay.label} · Esc or click outside to get back to it</p>${opts}
          <button type="button" class="secondary leave-replay" onClick=${(e) => { e.stopPropagation(); actions.leave(); }}>LEAVE REPLAY</button>${help}</div></div>`;
    }
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

  // Replays (docs/replays.md): open recorded rounds from files (several: a
  // match's rounds, in name order; or drop them anywhere on the page).
  function Replays({ s, back }) {
    const file = useRef();
    const [over, setOver] = useState(false);
    useEffect(() => { actions.replayList?.(); }, []);
    const date = (ms) => new Date(ms).toLocaleString(undefined, { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" });
    const score = (sc) => (sc ? Object.values(sc).join(":") : "");
    const drop = (e) => { e.preventDefault(); e.stopPropagation(); setOver(false); if (e.dataTransfer?.files?.length) actions.replayFiles(e.dataTransfer.files); };
    return html`<div class="panel replays-panel" onClick=${(e) => e.stopPropagation()}>
      <h2>REPLAYS</h2>
      <p class="maps-hint">every round on the server is recorded (GOTV style): pick one, or open a downloaded file${s.screen === "ingame" ? " · watching leaves the match" : ""}</p>
      <h3 class="replay-sub">ON THIS SERVER</h3>
      <div class="replay-list">
        ${s.replayMatches === null || s.replayMatches === undefined ? html`<p>loading…</p>`
          : s.replayListError ? html`<p class="replay-error">${s.replayListError}</p>`
          : !s.replayMatches.length ? html`<p class="replay-none">no rounds recorded yet (warmup isn't)</p>`
          : s.replayMatches.map((m) => html`<div class="replay-match" key=${m.id}>
              <div class="replay-match-head"><b>${m.map}</b><span>${date(m.started)}</span>${m.live && html`<span class="replay-live">● live</span>`}</div>
              <div class="replay-rounds">${m.rounds.map((r, i) => html`<button type="button" key=${r.file} class=${"replay-round" + (r.live ? " live" : "")}
                title=${`round ${r.round} · ${r.players} players · score before ${score(r.score) || "0:0"}${r.live ? " · still being played" : ""}`}
                onClick=${() => actions.replayServerRound(m, i)}>${r.round}</button>`)}</div>
            </div>`)}
      </div>
      <h3 class="replay-sub">RECORDED BY YOU</h3>
      <div class="replay-list replay-mine">
        ${!settings.get().cl_autorecord ? html`<p class="replay-none">recording is off (cl_autorecord 0)</p>`
          : !s.myRecordings?.length ? html`<p class="replay-none">the rounds you play are recorded here (what this client received; the last 30)</p>`
          : [...s.myRecordings].map((x, i) => ({ ...x, i })).reverse().map((x) => html`<div class="replay-mine-row" key=${x.name}>
              <span><b>${x.map}</b> · round ${x.round}${x.live && html` <span class="replay-live">● recording</span>`}</span>
              <span class="replay-size">${(x.size / 1024).toFixed(0)} KB</span>
              <button type="button" class="replay-round" onClick=${() => actions.playRecording(x.i)} title=${s.screen === "ingame" ? "watching leaves the match" : "watch it"}>watch</button>
              <button type="button" class="replay-round" onClick=${() => actions.saveRecording(x.i)} title="download the .cstv.gz">save</button>
            </div>`)}
      </div>
      <h3 class="replay-sub">FROM A FILE</h3>
      <div class=${"replay-drop" + (over ? " over" : "")} onDragOver=${(e) => { e.preventDefault(); setOver(true); }} onDragLeave=${() => setOver(false)} onDrop=${drop}
        onClick=${() => file.current?.click()}>
        <b>drop .cstv.gz files here</b><span>or click to open them · several play as one match, round by round</span>
      </div>
      <input ref=${file} type="file" accept=".gz,.cstv" multiple style="display:none" onChange=${(e) => { if (e.target.files?.length) actions.replayFiles(e.target.files); e.target.value = ""; }} />
      ${s.replayError && html`<p class="replay-error">couldn't play ${s.replayError}</p>`}
      <p class="replay-keys">in a replay: <kbd>P</kbd> pause · <kbd>←</kbd><kbd>→</kbd> 5 s · <kbd>↑</kbd><kbd>↓</kbd> speed · <kbd>PgUp</kbd><kbd>PgDn</kbd> round · <kbd>Space</kbd> camera · click the view to look around, click players to follow (mouse buttons)</p>
      <button type="button" class="secondary" onClick=${back}>BACK</button>
    </div>`;
  }

  // The replay player's bar: the timeline with kill and round marks (click
  // to seek), play/pause, speed, rounds, camera.
  const mmss = (t) => `${Math.floor(t / 60)}:${String(Math.floor(t % 60)).padStart(2, "0")}`;
  const SPEED_LABEL = { 0.25: "¼×", 0.5: "½×", 1: "1×", 2: "2×", 4: "4×" };
  const ROUND_MARK = { live: "▶", planted: "💣", defused: "✂", exploded: "💥", end: "■" };
  function ReplayBar({ r, marks, aiming }) {
    const R = actions.replay;
    const bar = useRef();
    const seekAt = (e) => { const b = bar.current.getBoundingClientRect(); R.seek(Math.max(0, Math.min(1, (e.clientX - b.left) / b.width))); };
    const stop = (e) => e.stopPropagation();
    return html`<div class=${"replay-bar" + (aiming ? " dim" : "")} onMouseDown=${stop} onClick=${stop}>
      <div class="replay-top">
        <span class="replay-label">REPLAY · ${r.label}${r.count > 1 ? ` (${r.index + 1}/${r.count})` : ""}</span>
        <span class="replay-time">${mmss(r.t)} / ${mmss(r.len)}</span>
      </div>
      <div class="replay-timeline" ref=${bar} onClick=${seekAt}>
        <div class="replay-done" style=${{ width: `${(r.len ? r.t / r.len : 0) * 100}%` }}></div>
        ${marks.map((m, i) => m.kind === "kill"
          ? html`<div key=${i} class="replay-mark kill" style=${{ left: `${m.at * 100}%`, background: teamInfo(m.vteam)?.color ?? "#fff" }} title=${`${m.kname || "world"} → ${m.vname}${m.hs ? " (head)" : ""}`}></div>`
          : html`<div key=${i} class="replay-mark round" style=${{ left: `${m.at * 100}%` }} title=${m.ev + (m.win ? ` (${m.win})` : "")}>${ROUND_MARK[m.ev] || "·"}</div>`)}
        <div class="replay-head" style=${{ left: `${(r.len ? r.t / r.len : 0) * 100}%` }}></div>
      </div>
      <div class="replay-buttons">
        <button type="button" disabled=${r.index === 0} onClick=${() => R.round(-1)} title="previous round (PgUp)">⏮</button>
        <button type="button" onClick=${() => R.skip(-5)} title="back 5 s (←)">−5s</button>
        <button type="button" class="replay-play" onClick=${() => R.pause()} title="play / pause (P)">${r.paused ? (r.ended ? "↺" : "▶") : "❚❚"}</button>
        <button type="button" onClick=${() => R.skip(5)} title="forward 5 s (→)">+5s</button>
        <button type="button" disabled=${r.index >= r.count - 1} onClick=${() => R.round(1)} title="next round (PgDn)">⏭</button>
        <span class="replay-sep"></span>
        <button type="button" onClick=${() => R.speed(-1)} disabled=${r.speed <= 0.25} title="slower (↓)">−</button>
        <span class="replay-speed">${SPEED_LABEL[r.speed] || r.speed + "×"}</span>
        <button type="button" onClick=${() => R.speed(1)} disabled=${r.speed >= 4} title="faster (↑)">+</button>
        <span class="replay-sep"></span>
        <button type="button" onClick=${() => R.next(-1)} title="previous player">◀</button>
        <button type="button" class="replay-cam" onClick=${() => R.cam()} title="camera (Space)">${r.cam}</button>
        <button type="button" onClick=${() => R.next(1)} title="next player">▶</button>
        <span class="replay-sep"></span>
        <button type="button" class="replay-stop" onClick=${() => R.stop()} title="leave the replay">✕</button>
      </div>
    </div>`;
  }

  // Map selector: every map with a preview rendered on this client; picking
  // one runs changelevel (an admin command: everyone while the server has no
  // admin password).
  // The selector's sections, in order (content.Categories).
  // The progress bar's label by kind (rounds.js: the Big Diff's push / revert, GPU Drop's grab / restock).
  const WORK_LABEL = { push: "git push --force origin main…", revert: "git revert The Big Diff…", grab: "Grabbing the RTX 9090…", restock: "Restocking the RTX 9090…" };
  const MAP_CATEGORIES = [
    { id: "training", name: "Training", desc: "small maps for a few players" },
    { id: "competitive", name: "Competitive", desc: "full-size, CS-scale maps" },
    { id: "gpudrop", name: "GPU Drop", desc: "scalpers grab the last boxed GPUs; the store staff defend the stockroom" },
    { id: "conquest", name: "Conquest", desc: "Star Slop: Earth against the Moon; respawn tickets, capture points, a rocket to the front" },
    { id: "racing", name: "Racing", desc: "Slop n Roll Racing: cars on a grid, laps through the checkpoints, falling off the track is a wreck" },
    { id: "parkour", name: "Parkour", desc: "a race on foot: jump, hook and fly the gates start to finish, a car or a boat for some legs; your splits against your best" },
    { id: "fun", name: "Fun", desc: "house rules: grenades only, team deathmatch, and worse" },
  ];
  function Maps({ s, back }) {
    useEffect(() => { actions.maps?.(); }, []);
    const cards = s.mapCards;
    const onPage = cards ? MAP_CATEGORIES.flatMap((c) => cards.filter((m) => m.category === c.id).map((m) => m.name)) : []; // page order: the previews' priority
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
            <${Preview} s=${s} name=${m.name} prio=${onPage.indexOf(m.name)} />
            <span class="mn">${m.name}${m.custom ? html` <em class="custom">· custom</em>` : ""}${s.map === m.name ? html` <em>· playing</em>` : s.pendingMap === m.name ? html` <em>· loads when you join</em>` : ""}</span>
            <span class="mi">${Math.round(m.size[0])}×${Math.round(m.size[1])} m · up to ${m.maxPlayers} players · sites ${m.sites.join(" ")}</span>
            <span class="md">${m.desc}</span>
          </button>`)}
      </div>`)}
      <button type="button" class="secondary" onClick=${back}>BACK</button>
    </div>`;
  }
  // A card's picture: drawn only once the card scrolls into view (plus a
  // margin; game/maps.js draws one at a time, prio = its place on the page,
  // the vote's options first) and taken off the queue when it scrolls away.
  // Until it is ready: the map from above (a placeholder from its boxes); then
  // the preview fades in over it.
  function Preview({ s, name, prio }) {
    const ref = useRef(null), url = s.mapPreviews?.[name], holder = s.mapHolders?.[name];
    useEffect(() => {
      const el = ref.current;
      if (!el || !name) return;
      if (typeof IntersectionObserver === "undefined") { actions.wantPreview?.(name, prio); return; }
      const io = new IntersectionObserver((es) => {
        if (es[es.length - 1].isIntersecting) actions.wantPreview?.(name, prio); else actions.dropPreview?.(name);
      }, { root: el.closest(".maps-panel"), rootMargin: "240px 0px" });
      io.observe(el);
      return () => { io.disconnect(); actions.dropPreview?.(name); };
    }, [name, prio]);
    return html`<div ref=${ref} class="map-preview ${url ? "" : "pending"}" style=${holder ? `background-image:url(${holder})` : ""}>
      ${url ? html`<img class="map-preview-img" src=${url} alt="" />` : html`<span>rendering…</span>`}
    </div>`;
  }

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
        ${v.options.map((o, i) => html`<button type="button" data-map=${o} class="mv-card ${v.mine === o ? "mine" : ""} ${v.done && v.winner === o ? "won" : ""}"
          onClick=${() => actions.vote(o)} disabled=${v.done}>
          <${Preview} s=${s} name=${o} prio=${i - 3} />
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

  // The crosshair (look: the cl_crosshair* cvars, xhair.js; gap: --gap from main.js,
  // or a wrapper's --gap in the options preview).
  function Crosshair({ v }) {
    const live = useSettings();
    v = v || live;
    return html`<div class="crosshair" style=${xhairVars(v)}>
      <i class="l"></i><i class="r"></i>${v.cl_crosshair_t ? "" : html`<i class="t"></i>`}<i class="b"></i>${v.cl_crosshairdot ? html`<i class="dot"></i>` : ""}</div>`;
  }

  // Options -> Crosshair: the look, a live preview (at rest and spraying, for the
  // dynamic style) and the share code (copy yours, paste a friend's).
  function CrosshairOptions({ v, check }) {
    const [paste, setPaste] = useState(""), [note, setNote] = useState("");
    const code = xhairCode(v);
    const slider = (k, label) => html`<div class="row"><span>${label}</span>
      <input type="range" class=${k} min=${CVARS[k].min} max=${CVARS[k].max} step=${CVARS[k].step || 1} value=${v[k]} onInput=${(e) => set(k, e.target.value)} />
      <span class="num">${v[k]}</span></div>`;
    const apply = () => {
      const p = parseXhairCode(paste);
      if (!p) return setNote("not a crosshair code");
      settings.set(p); setPaste(""); setNote("applied");
    };
    const copy = async () => {
      setNote((await copyText(code)) ? "copied" : "select the code and copy it"); // plain http too (ui/clipboard.js)
    };
    return html`<section class="xh-options">
      <h4>Crosshair</h4>
      <div class="xh-preview">
        <div class="xh-cell" style=${{ "--gap": "3px" }}><${Crosshair} v=${v} /><span>at rest</span></div>
        <div class="xh-cell" style=${{ "--gap": "14px" }}><${Crosshair} v=${v} /><span>${v.cl_crosshairstyle === "static" ? "spraying (static)" : "spraying"}</span></div>
        <div class="xh-cell light" style=${{ "--gap": "3px" }}><${Crosshair} v=${v} /><span>on a bright wall</span></div>
      </div>
      <div class="row"><span>Style</span>
        <select class="cl_crosshairstyle" value=${v.cl_crosshairstyle} onChange=${(e) => set("cl_crosshairstyle", e.target.value)}>
          <option value="dynamic">Dynamic (opens with your spread)</option>
          <option value="static">Static</option>
        </select></div>
      ${slider("cl_crosshairsize", "Size")}
      ${slider("cl_crosshairgap", "Gap")}
      ${slider("cl_crosshairthickness", "Thickness")}
      ${check("cl_crosshairdot", "Centre dot", "cl_crosshairdot")}
      ${check("cl_crosshair_t", "T style (no top line)", "cl_crosshair_t")}
      ${check("cl_crosshair_drawoutline", "Outline", "cl_crosshair_drawoutline")}
      <div class="row"><span>Colour</span>
        <input type="color" class="cl_crosshaircolor" value=${v.cl_crosshaircolor} onInput=${(e) => set("cl_crosshaircolor", e.target.value)} />
        <input type="range" class="cl_crosshairalpha" min="0" max="255" step="1" value=${v.cl_crosshairalpha} onInput=${(e) => set("cl_crosshairalpha", e.target.value)} />
        <span class="num">${Math.round((v.cl_crosshairalpha / 255) * 100)}%</span></div>
      <div class="row"><span>Share code</span>
        <input type="text" class="xh-code" readonly value=${code} onFocus=${(e) => e.target.select()} />
        <button type="button" class="rebind xh-copy" onClick=${copy}>COPY</button></div>
      <div class="row"><span></span>
        <input type="text" class="xh-paste" placeholder="paste a code: SLOP-…" value=${paste} onInput=${(e) => { setPaste(e.target.value); setNote(""); }}
          onKeyDown=${(e) => { if (e.key === "Enter") { e.stopPropagation(); apply(); } }} />
        <button type="button" class="rebind xh-apply" onClick=${apply}>APPLY</button>
        <button type="button" class="rebind xh-default" onClick=${() => { settings.set(XHAIR_DEFAULTS); setNote("default crosshair"); }}>DEFAULT</button>
        ${note && html`<span class="xh-note">${note}</span>`}</div>
      <div class="hint">The console has the same: <code>cl_crosshairsize 6</code>, <code>crosshair_code</code> prints your code, <code>crosshair_code SLOP-…</code> applies one.</div>
    </section>`;
  }

  // Number field that commits on change/enter and resyncs when the value moves elsewhere.
  const Num = ({ k, v, step = "any", disabled }) => html`<input type="number" class="num" key=${"n" + v}
    defaultValue=${+(+v).toFixed(3)} step=${step} min=${CVARS[k].min} max=${CVARS[k].max} disabled=${disabled}
    onChange=${(e) => set(k, e.target.value)} />`;

  function set(k, raw) {
    const v = coerce(k, raw); // enums, keys, bools, clamped/stepped numbers, colours
    if (v === undefined) return;
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

  function Options({ s, back, controls }) {
    const v = useSettings();
    const st = actions.loop?.()?.stats();
    const hz = st ? Math.round(st.hz) : 60;
    const div = v.fps_limiter !== "latency" && v.fps_limit === "custom" && v.fps_max > 0 ? Math.min(4, Math.max(1, Math.round(hz / v.fps_max))) : 1;
    const counts360 = Math.round(360 / (v.sensitivity * M_YAW));
    const gi = actions.gfxInfo?.();
    const aaStale = !!gi && gi.requested.antialias !== (v.r_aa === "auto");
    const check = (k, label, cls = "") => html`<label class="row check"><input type="checkbox" class=${cls} checked=${!!v[k]} onChange=${(e) => set(k, e.target.checked)} /> ${label}</label>`;
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
        ${v.m_rawinput && s.rawInput === false && html`<div class="hint warn raw-hint">${rawInputHint()}</div>`}
        ${check("m_invert_y", "Invert Y")}
      </section>
      <${CrosshairOptions} v=${v} check=${check} />
      <section>
        <h4>Audio</h4>
        <div class="row"><span>Volume</span>
          <input type="range" min="0" max="1" step="0.01" value=${v.volume} onInput=${(e) => set("volume", e.target.value)} />
          <span class="num">${Math.round(v.volume * 100)}%</span></div>
        <div class="row"><span>Announcer</span>
          <input type="range" min="0" max="1" step="0.01" value=${v.announcer_volume} disabled=${!v.cl_announcer}
            onInput=${(e) => set("announcer_volume", e.target.value)} />
          <span class="num">${Math.round(v.announcer_volume * 100)}%</span></div>
        <div class="row"><span>Music</span>
          <input type="range" class="music-volume" min="0" max="1" step="0.01" value=${v.music_volume} disabled=${!v.cl_music}
            onInput=${(e) => set("music_volume", e.target.value)} />
          <span class="num">${Math.round(v.music_volume * 100)}%</span></div>
        ${check("snd_hrtf", "3D sound (HRTF): front / behind / above / below, best with headphones; off = plain stereo", "snd-hrtf")}
        ${check("cl_announcer", "Announcer voice (SlopVox kill streaks)")}
        ${check("cl_music", "Music (buy time, warmup, match end, MVP; never during live rounds)", "music-toggle")}
        ${check("cl_captions", "Captions: announcer lines as big text, taunts as subtitles")}
      </section>
      <section>
        <h4>Buy menu</h4>
        ${check("cl_buy_close", "Close the buy menu after a purchase (CS 1.6); off: it goes back to the categories so buys chain (B 4 2 6 1)", "buy-close")}
      </section>
      <section>
        <h4>Replays</h4>
        ${check("cl_autorecord", "Record the rounds you play (what your client received; Replays in the menu: watch or save the last 30)", "autorecord")}
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
        <div class="row"><span>Player lamp shadows</span>
          <select class="light-shadows" value=${String(v.r_light_shadows)} onChange=${(e) => set("r_light_shadows", e.target.value)}>
            <option value="0">Off</option>${[1, 2, 4, 6, 8].map((n) => html`<option value=${String(n)}>${n} light${n > 1 ? "s" : ""}</option>`)}
          </select></div>
        ${check("r_muzzle_light", "Muzzle flashes light the room", "muzzle-light")}
        <div class="row"><span>Muzzle-light shadows</span>
          <select class="dynlight-shadows" value=${String(v.r_dynlight_shadows)} onChange=${(e) => set("r_dynlight_shadows", e.target.value)} disabled=${!+v.r_muzzle_light}>
            <option value="0">Off</option>${[1, 2, 3, 4].map((n) => html`<option value=${String(n)}>${n} light${n > 1 ? "s" : ""}</option>`)}
          </select></div>
        <div class="row"><span>Anti-aliasing</span>
          <select class="aa-mode" value=${v.r_aa} onChange=${(e) => set("r_aa", e.target.value)}>
            ${aaChoices(gi?.info.msaaSamples, v.r_aa).map((m) => html`<option value=${m}>${AA_LABELS[m]}</option>`)}
          </select></div>
        <div class="row"><span>Render scale</span>
          <input type="range" class="render-scale" min="50" max="100" step="5" value=${v.r_render_scale} onInput=${(e) => set("r_render_scale", e.target.value)} />
          <span class="num">${v.r_render_scale}%</span></div>
        ${aaStale ? html`<div class="row"><span class="hint">Auto ↔ other: fully applies after a reload</span>
          <button type="button" onClick=${() => actions.reloadPage?.()}>RELOAD NOW</button></div>` : ""}
        ${check("r_decals", "Bullet holes")}
        ${check("r_casings", "Ejected casings")}
        ${check("cl_showfps", "Show FPS / frame time / ping")}
        <div class="row"><span>Frame profiler</span>
          <select class="showprof" value=${v.cl_showprof} onChange=${(e) => set("cl_showprof", e.target.value)}>
            <option value="0">Off</option><option value="1">What limits it + top costs</option><option value="2">Full breakdown (CPU + GPU)</option>
          </select></div>
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
      ${s.myTeam === SPECTATOR && !s.replay && html`<div class="spec-banner">SPECTATING · press <b>${keyName(keyFor(settings.get().binds, "teammenu"))}</b> to pick a team</div>`}
      ${s.observe ? html`<${Observe} o=${s.observe} s=${s} />`
        : s.myTeam !== SPECTATOR && (s.scoped === "lens" ? html`<${Scope} />` : html`<${Crosshair} />`)}
      ${s.shout && html`<div class="shout" key=${s.shout.id} style=${{ color: s.shout.color, "--dur": `${s.shout.dur}s` }}>${s.shout.text}</div>`}
      ${s.subtitle && html`<div class="subtitle" key=${s.subtitle.id} style=${{ "--dur": `${s.subtitle.dur}s` }}><span style=${{ color: s.subtitle.color }}>${s.subtitle.text}</span></div>`}
      ${s.race ? html`<div class="top-mid"><${RaceBar} r=${s.race} /></div>
        <${Standings} r=${s.race} /><${RaceCentre} r=${s.race} />` : html`<div class="top-mid">
        ${s.tickets ? html`<${Backlog} team="vibe" n=${s.tickets.tickets.vibe} left=${true} />`
          : html`<span class="team-score" style=${{ color: TEAMS.vibe.color }}><span class="long">${TEAMS.vibe.name.toUpperCase()}</span><span class="short">${TEAMS.vibe.short}</span> ${s.score?.vibe ?? 0}</span>`}
        ${s.ci ? html`<span class="timer ci"><i class="ci-light" key=${s.ci.beep}></i><span class="ci-label">PLANTED · SITE ${s.ci.site}</span>${s.ci.left != null
            ? html`<span class="ci-bar"><i style=${{ width: `${s.ci.frac * 100}%` }}></i></span><span class="ci-left">${s.ci.left}s</span>` : ""}</span>`
          : html`<span class="timer ${s.phase}">${PHASE_LABEL[s.phase] ? html`${PHASE_LABEL[s.phase]}${s.phase === "warmup" && s.roundLeft > 0 ? html` <small>${m}:${sec}</small>` : ""}` : `${m}:${sec}`}</span>`}
        ${s.tickets ? html`<${Backlog} team="artisan" n=${s.tickets.tickets.artisan} left=${false} />`
          : html`<span class="team-score" style=${{ color: TEAMS.artisan.color }}>${s.score?.artisan ?? 0} <span class="long">${TEAMS.artisan.name.toUpperCase()}</span><span class="short">${TEAMS.artisan.short}</span></span>`}
      </div>`}
      ${s.tickets && html`<${PointsStrip} points=${s.tickets.points} myTeam=${s.myTeam} />`}
      ${s.departure && html`<${DepartureBoard} d=${s.departure} />`}
      ${s.match?.max > 1 && !["warmup", "matchover"].includes(s.phase) && html`<div class="round-no">${roundLabel(s.match)}</div>`}
      ${s.gpu && html`<div class="gpu-count"><span class="gpu-secured">${s.gpu.secured}/${s.gpu.toWin} secured</span><span class="gpu-stock">${s.gpu.stock ? `${s.gpu.stock} in stock` : "OUT OF STOCK"}</span>${s.gpu.loose ? html`<span class="gpu-loose">${s.gpu.loose} in play</span>` : ""}</div>`}
      ${s.phase === "freeze" && !s.roundEnd && !s.race && html`<div class="freeze-note">code freeze · ${s.roundLeft}</div>`}
      ${s.halftime && s.phase === "halftime" && html`<div class="round-end" key="half">
        <div class="re-title">HALFTIME</div>
        <div class="re-line">${TEAMS.vibe.name} ${s.halftime.sc?.vibe ?? 0} : ${s.halftime.sc?.artisan ?? 0} ${TEAMS.artisan.name} · switching sides…</div></div>`}
      ${s.matchEnd && html`<${MatchEnd} s=${s} />`}
      ${s.work && html`<div class="work"><div class="work-label">${s.work.other ? `${s.work.other}: ` : ""}${WORK_LABEL[s.work.kind] ?? "git push --force origin main…"}</div>
        <div class="work-bar ${s.work.kind}"><i style=${{ width: `${s.work.pr * 100}%` }}></i></div></div>`}
      ${s.hint && !s.work && html`<div class="bomb-hint">${s.hint}</div>`}
      ${s.boat && s.myTeam !== SPECTATOR && !s.observe && html`<${BoatBadge} b=${s.boat} />`}
      ${s.car && !s.boat && s.myTeam !== SPECTATOR && !s.observe && html`<${CarBadge} c=${s.car} />`}
      ${s.roundEnd && html`<div class="round-end" key=${s.roundEnd.n}>
        <div class="re-title" style=${{ color: s.roundEnd.color }}>${s.roundEnd.title ?? `${s.roundEnd.name.toUpperCase()} WIN!`}</div>
        <div class="re-line">${s.roundEnd.text}</div>
        ${s.race?.results && html`<${RaceResults} r=${s.race} />`}
        ${s.mvp && html`<div class="re-mvp ${s.mvp.me ? "me" : ""}">★ MVP: <b>${s.mvp.name}</b> <span>${s.mvp.why}</span></div>`}
        ${s.dmgReport && html`<table class="re-dmg"><tr><th></th><th>given</th><th>taken</th></tr>
          ${s.dmgReport.map((r) => html`<tr key=${r.id}><td>${r.name}</td><td>${r.gh ? `${r.g} (${r.gh})` : "–"}</td><td>${r.th ? `${r.t} (${r.th})` : "–"}</td></tr>`)}</table>`}</div>`}
      ${s.flash ? html`<div class="blast-flash" key=${s.flash}></div>` : null}
      <div class="top-left">
        ${s.myTeam !== SPECTATOR && s.canBuy && html`<${BuyBadge} s=${s} />`}
        ${s.myTeam !== SPECTATOR && s.armor > 0 && html`<div class="armor">${s.helmet ? "⛑" : "🛡"} ${s.armor}</div>`}
        ${s.myTeam !== SPECTATOR && (s.shades || s.boost > 0) && html`<div class="perks">${s.shades && html`<span title="Senior-dev Sunglasses: flashes blind you 60% shorter">😎</span>`}${s.boost > 0 && html`<span class="boost" title="10x Energy Drink: +10% move speed">⚡ ${s.boost}s</span>`}</div>`}
        ${s.myTeam !== SPECTATOR && !s.tickets && html`<div class="money">$${s.money}
          ${s.moneyPop && performance.now() - s.moneyPop.at < 3000 && html`<div class="money-pop ${s.moneyPop.d < 0 ? "neg" : ""}" key=${s.moneyPop.at}>${s.moneyPop.d > 0 ? "+" : "−"}$${Math.abs(s.moneyPop.d)} <span>${s.moneyPop.why}</span></div>`}</div>`}
        ${TEAMS[s.myTeam] && html`<div class="my-team" style=${{ color: TEAMS[s.myTeam].color }}>${TEAMS[s.myTeam].name} · M to switch</div>`}
        <div class="net ${s.connected || s.replay ? "" : "bad"}">${s.replay ? `${s.map} · replay` : s.connected ? `${s.map} · ${s.ping ?? "…"} ms` : "reconnecting…"}</div>
      </div>
      <div class="feed">${s.feed.map((f) => (f.kill ? html`<${KillRow} key=${f.id} k=${f.kill} />` : html`<div key=${f.id}>${f.text}</div>`))}</div>
      ${s.myTeam !== SPECTATOR && !s.observe && html`
        <div class="bottom-left">
          <span class="hp">✚ ${s.hp}</span><span class="armor">◈ ${s.armor}</span>${s.car?.role === "driver" && html`<${CarHP} c=${s.car} />`}${s.energy >= 0 && html`<${EnergyMeter} s=${s} />`}${s.carrying && html`<span class="carry-diff" title="You carry The Big Diff">±BIG DIFF</span>`}${s.garageMap && html`<${GarageHint} />`}
        </div>
        ${s.garageMap && (s.phase === "warmup" || s.phase === "freeze") && html`<${GarageCall} />`}
        <div class="slots">${s.slots.filter((x) => x.name).map((x) => x.items
          ? html`<div key=${x.slot} class="nade-row"><b>${x.key}</b> ${x.items.map((n, i) => html`${i > 0 && html`<i class="sep"> · </i>`}<span key=${n.slot} class=${n.cur ? "nade cur" : "nade"}>${nadeTag(n.id)}${n.name}${n.count > 1 && html`<small> ×${n.count}</small>`}</span>`)}</div>`
          : html`<div key=${x.slot} class=${x.cur ? "cur" : ""}><b>${x.key}</b> ${x.name}</div>`)}</div>
        <div class="bottom-right ${s.reloading ? "reloading" : ""}">
          <span class="ammo">${s.reloading ? "slopping…" : s.ammo}</span>${s.reserve !== "" && html`<span class="reserve">/ ${s.reserve}</span>`}
        </div>`}`;
  }

  // A car race map: the garage is always there (B or H, anywhere, any time;
  // mid-race a pit stop): a small permanent hint by the HP, and before GO a
  // bigger line saying what it is for.
  function GarageHint() {
    const b = keyName(keyFor(settings.get().binds, "buymenu")), g = keyName(keyFor(settings.get().binds, "garage"));
    return html`<span class="garage-hint" title="the race garage: buy and change your car (mid-race: a pit stop)"><b class="key">${b}</b>${g && g !== b ? html` / <b class="key">${g}</b>` : ""} garage</span>`;
  }
  function GarageCall() {
    return html`<div class="garage-call">press <b class="key">${keyName(keyFor(settings.get().binds, "buymenu"))}</b> for the garage: buy and change your car</div>`;
  }

  // In your buy zone during buy time: a cart, the buy key and the seconds left.
  // It sits above every overlay (the buy menu included), so you always see
  // that you can still buy and for how long; it goes when you can't.
  function BuyBadge({ s }) {
    const key = keyName(keyFor(settings.get().binds, "buymenu"));
    const garage = !!s.garageMap; // a car race map: B is the garage (pk_ keeps BUY)
    return html`<div class="buy-badge" title=${garage ? `press ${key} for the garage` : `buy zone: press ${key} to buy`}>
      <svg class="cart" viewBox="0 0 24 20" aria-hidden="true"><path d="M1 2h4l3 11h11l3-8H7" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linejoin="round" /><circle cx="9.5" cy="17" r="1.8" fill="currentColor" /><circle cx="17.5" cy="17" r="1.8" fill="currentColor" /></svg>
      <b class="key">${key}</b><span class="bb-label">${garage ? "GARAGE" : "BUY"}</span>${s.buyLeft != null && html`<span class="bb-left">${s.buyLeft} s</span>`}
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

  // Observer HUD: the death cam caption, or whom you follow (their HP, weapon
  // and ammo) with the camera mode and controls. A crosshair in first person.
  function Observe({ o, s = {} }) {
    const tc = (t) => TEAMS[t]?.color ?? "var(--hud)";
    if (o.freeze) {
      return html`<div class="deathcam">
        <div class="dc-title">${o.caption.title}</div>
        <div class="dc-line">${o.caption.line.map((p) => (typeof p === "string" ? p : html`<b style=${p.team ? { color: tc(p.team) } : null}>${p.b}</b>`))}</div>
      </div>`;
    }
    const b = settings.get().binds, k = (c) => keyName(keyFor(b, c));
    return html`
      ${o.first && html`<${Crosshair} />`}
      <div class=${"observe" + (s.replay ? " over-replay-bar" : "")}>
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
    const top = [...rows].sort(byScore).slice(0, 3);
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
            <span>${t.name}</span><span class="sb-role">${t.role} · <span class="sb-alive">${aliveCount(members)}/${members.length} alive</span></span><span class="sb-score">${s.score?.[id] ?? 0}</span></div>
          <table><tr><th></th><th>name</th><th>K</th><th>A</th><th>D</th><th>pts</th><th>ping</th></tr>
          ${[...members].sort(byScore).map((p) => html`<tr key=${p.id} class=${(p.me ? "me" : "") + (p.dead ? " dead" : "")}>
            <td><i class="swatch" style=${{ background: p.color }}></i></td>
            <td>${p.dead ? html`<span class="sb-skull" title="dead">☠</span> ` : ""}${p.name}</td><td>${p.k ?? 0}</td><td>${p.a ?? 0}</td><td>${p.d ?? 0}</td><td>${p.sp ?? 0}</td><td class="sb-ping">${(p.me ? s.ping : p.ping) ?? "…"}</td></tr>`)}
          </table>${!members.length && html`<div class="sb-empty">nobody yet</div>`}</div>`;
      })}
      ${(() => {
        const rest = rows.filter((p) => !TEAMS[p.team]);
        return rest.length > 0 && html`<div class="sb-spec">spectating: ${rest.map((p, i) => html`${i ? ", " : ""}<span class=${p.me ? "me" : ""}>${p.name}</span>`)}</div>`;
      })()}
    </div>`;
  }

  // Team select: on join and on M. 1 / 2 pick a side, 5 auto-assigns, 6 spectates.
  // A side that would end up 2+ players bigger, or is at the map's cap with humans, is shown full (the server checks too).
  function TeamMenu({ s }) {
    const all = [{ team: s.myTeam }, ...s.players];
    const counts = Object.fromEntries(TEAM_IDS.map((id) => [id, all.filter((p) => p.team === id).length]));
    const others = { ...counts };
    if (TEAMS[s.myTeam]) others[s.myTeam]--; // you don't count against the side you'd join
    const humans = Object.fromEntries(TEAM_IDS.map((id) => [id, s.players.filter((p) => p.team === id && !p.ai).length]));
    const cap = s.teamCap || 0;
    const capped = Object.fromEntries(TEAM_IDS.map((id) => [id, s.myTeam !== id && teamCapped(id, humans, cap)]));
    // a side at the cap with a bot on it: the bot makes room, so it counts one less
    for (const id of TEAM_IDS) if (cap > 0 && s.myTeam !== id && others[id] >= cap && humans[id] < others[id]) others[id]--;
    const open = Object.fromEntries(TEAM_IDS.map((id) => [id, s.myTeam === id || (!capped[id] && teamOpen(id, others))]));
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
            <span class="tr">${t.role} · ${counts[id]}${cap ? `/${cap}` : ""} playing${s.myTeam === id ? " · you" : ""}</span>
            ${capped[id] ? html`<span class="tfull">FULL: ${cap} per team on ${s.map}</span>` : full ? html`<span class="tfull">FULL: would be 2+ bigger</span>` : html`<i>"${t.taunts[0]}"</i>`}</button>`;
        })}
      </div>
      <div class="team-extra">
        <button type="button" class="secondary" onClick=${() => actions.pickTeam("auto")}>5 · AUTO-ASSIGN</button>
        <button type="button" class="secondary ${s.myTeam === SPECTATOR ? "current" : ""}" onClick=${() => actions.pickTeam(SPECTATOR)}>6 · SPECTATE${spectators ? ` (${spectators})` : ""}</button>
      </div>
      <p class="team-hint">${s.myTeam === SPECTATOR ? "you are spectating · Esc to keep watching" : "switching sides respawns you · Esc to cancel"}</p>
    </div></div>`;
  }

  // One kill-feed row: killer, weapon icon (+ noscope / penetrated / headshot flair), victim,
  // in team colours; rows you're part of get an outline and stay longer.
  function KillRow({ k }) {
    const col = (t) => TEAMS[t]?.color || "#ddd";
    const icon = KILL_GLYPH[k.weapon] ? "" : weaponIcon(k.weapon); // a glyph wins (the Big Diff's blast is 💥, not its model)
    return html`<div class="kill-row ${k.mine ? "mine" : ""}">
      ${k.killer !== k.victim && html`<span class="kn" style=${{ color: col(k.kteam) }}>${k.killer}</span>`}
      ${icon ? html`<img class="kw" src=${icon} alt=${k.wname} title=${k.wname} />` : html`<span class="kg" title=${k.wname}>${KILL_GLYPH[k.weapon] || KILL_GLYPH.world}</span>`}
      ${nadeTag(k.weapon)}
      ${k.ns && html`<span class="kf" title="no scope">⌀</span>`}
      ${k.pn && html`<span class="kf kp" title="penetrated: through another body">⇶</span>`}
      ${k.hs && html`<span class="kf kh" title="headshot">✚</span>`}
      <span class="kn" style=${{ color: col(k.vteam) }}>${k.victim}</span>
    </div>`;
  }

  render(html`<${App} />`, el);
  return api;
}


// An unplugged cable: plug, a spark in the accent colour, socket. Our own line drawing.
function Unplugged({ w }) {
  return html`<svg class="unplugged" width=${w} height=${w / 5.6} viewBox="0 0 560 100" fill="none" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
    <g stroke="var(--hud)" stroke-width="7">
      <path d="M4 50 H122" />
      <rect x="122" y="22" width="90" height="56" rx="9" />
      <path d="M212 36 H240 M212 64 H240" />
      <rect x="322" y="12" width="110" height="76" rx="10" />
      <path d="M342 36 H360 M342 64 H360" />
      <path d="M432 50 H556" />
    </g>
    <g stroke="var(--accent)" stroke-width="7">
      <path d="M268 22 L284 38 L272 52 L292 72" />
      <path d="M292 12 L300 24 M262 82 L278 78" />
    </g>
  </svg>`;
}

function loadName() { try { return localStorage.getItem("cs.name") || ""; } catch { return ""; } }
function saveName(n) { try { localStorage.setItem("cs.name", n); } catch {} }

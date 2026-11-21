// Chat and radio UI: the chat log (bottom left, lines fade after a while),
// the chat input (Y all / U team; Enter sends, Esc cancels; game input is off
// while typing), and the radio menu (Z: number keys pick a callout; you keep
// moving, the numbers don't switch weapons while it's open).
import { h } from "preact";
import { useEffect, useRef, useState } from "preact/hooks";
import htm from "htm";
import { TEAMS } from "../assets/teams.js";
import { RADIO } from "../assets/voice/radio.js";

const html = htm.bind(h);
export const CHAT_SHOW_MS = 12000;

// One chat log line's prefix, CS style: *DEAD* and (TEAM)/(RADIO).
export function chatPrefix(m) {
  return `${m.dead ? "*DEAD* " : ""}${m.radio ? "(RADIO) " : m.tm ? "(TEAM) " : ""}`;
}

export function ChatLog({ lines, open }) {
  const now = performance.now();
  const shown = open ? lines.slice(-10) : lines.filter((l) => now - l.at < CHAT_SHOW_MS).slice(-6);
  if (!shown.length) return null;
  return html`<div class="chat-log ${open ? "open" : ""}">
    ${shown.map((l) => html`<div class="chat-line" key=${l.key}>
      <span class="cp">${chatPrefix(l)}</span><b style=${{ color: TEAMS[l.team]?.color || "#ccc" }}>${l.name}</b>: <span class="ct">${l.text}</span>
    </div>`)}
  </div>`;
}

export function ChatInput({ team, send, close }) {
  const ref = useRef(null);
  const [text, setText] = useState("");
  useEffect(() => { ref.current?.focus(); }, []);
  const onKey = (e) => {
    e.stopPropagation(); // typing never reaches the game binds
    if (e.key === "Enter") { e.preventDefault(); if (text.trim()) send(text, team); close(); }
    else if (e.key === "Escape") { e.preventDefault(); close(); }
  };
  return html`<div class="chat-input">
    <span class="ci-label">${team ? "say (team):" : "say:"}</span>
    <input ref=${ref} maxlength="120" value=${text} onInput=${(e) => setText(e.target.value)} onKeyDown=${onKey} onKeyUp=${(e) => e.stopPropagation()} />
  </div>`;
}

export function RadioMenu({ pick, close, closeKey }) {
  useEffect(() => {
    const onKey = (e) => {
      const n = /^Digit([0-9])$/.exec(e.code)?.[1];
      if (e.code === "Escape" || e.code === closeKey || n === "0") { e.preventDefault(); e.stopImmediatePropagation(); close(); return; }
      if (!n) return;
      e.preventDefault(); e.stopImmediatePropagation(); // not a weapon switch
      const r = RADIO[+n - 1];
      if (r) pick(r.id);
      close();
    };
    addEventListener("keydown", onKey, true);
    return () => removeEventListener("keydown", onKey, true);
  }, [closeKey]);
  return html`<div class="radio-menu">
    <h3>RADIO</h3>
    ${RADIO.map((r, i) => html`<div class="radio-item" onClick=${() => { pick(r.id); close(); }}><b>${i + 1}</b> ${r.text}</div>`)}
    <div class="radio-item dim"><b>0</b> close</div>
  </div>`;
}

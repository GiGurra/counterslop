// The buy menu (B, in your buy zone during freeze/buy time). It's the one
// overlay that takes game input away (the world keeps running for everyone):
// you stand still while shopping. Categories as in CS (1 Pistols, 2 Heavy,
// 3 SMGs, 4 Rifles, 5 Gear, 6 Grenades): the menu opens with none picked, a
// number opens one, the next number buys (B 4 2: the second rifle) and closes
// the menu; 0 / Backspace goes back. Buying is a request; the server decides
// (money, team, slot rules) and the result shows in the next snapshot.
// Something you own or can't afford still asks (the server says why) but
// leaves the menu open. Clicks work as the keys do.
//
// Two layouts, one state: wide screens get the catalog (categories on the
// left, item cards with the weapon icon and stat bars, an ammo strip at the
// bottom); narrow or short ones one column (rows with a stat line, the same
// ammo section below). The ammo cards buy as , and . do (buyammo1/2).
import { h } from "preact";
import { useState, useEffect } from "preact/hooks";
import htm from "htm";
import { WEAPONS } from "../assets/weapons/index.js";
import { owned, itemPrice, ammoCards, itemStats, keyCap, buyCategories } from "./buyrules.js";
import { weaponIcon } from "./weaponicon.js";
export { itemPrice, buyCategories };

const html = htm.bind(h);

// The catalog needs this much room; below it the one-column layout.
export const WIDE_QUERY = "(min-width: 900px) and (min-height: 600px)";


// Prices with thin-space thousands, as in the mockups: $2 050.
const money = (n) => `$${String(n).replace(/\B(?=(\d{3})+$)/g, " ")}`;

function useMedia(q) {
  const mq = typeof matchMedia === "function" ? matchMedia(q) : null;
  const [on, setOn] = useState(mq ? mq.matches : true);
  useEffect(() => {
    if (!mq) return;
    const f = () => setOn(mq.matches);
    f();
    mq.addEventListener("change", f);
    return () => mq.removeEventListener("change", f);
  }, [q]);
  return on;
}

// Gear has no box model: small line drawings in the icon slot.
const GEAR_ICON = {
  vest: html`<svg viewBox="0 0 40 28"><path d="M13 3l7 4 7-4 8 5-3 6-4-2v13H12V12l-4 2-3-6z" /></svg>`,
  helmet: html`<svg viewBox="0 0 40 28"><path d="M9 18a11 11 0 0 1 22 0" fill="none" stroke-width="3" /><rect x="5" y="15" width="7" height="10" rx="2" /><rect x="28" y="15" width="7" height="10" rx="2" /></svg>`,
  kit: html`<svg viewBox="0 0 40 28"><path d="M27 8a9 9 0 1 0 2 9" fill="none" stroke-width="3" /><path d="M22 4l8 3-5 6z" /></svg>`,
  shades: html`<svg viewBox="0 0 40 28"><path d="M4 10h32" fill="none" stroke-width="2" /><path d="M6 10h11l-1 7a3 3 0 0 1-3 3H9a3 3 0 0 1-3-3zM23 10h11v7a3 3 0 0 1-3 3h-4a3 3 0 0 1-3-3z" /></svg>`,
  drink: html`<svg viewBox="0 0 40 28"><rect x="14" y="4" width="12" height="21" rx="3" /><path d="M21 8l-4 7h4l-2 6 5-8h-4z" fill="#000" /></svg>`,
};
function Icon({ id }) {
  const url = WEAPONS[id] ? weaponIcon(id) : "";
  return html`<span class="bi-icon">${url ? html`<img src=${url} alt="" />` : GEAR_ICON[id] || ""}</span>`;
}

function Bars({ bars }) {
  return html`<span class="bi-stats">${bars.map((b) => html`<span>${b.label}</span><span class="bar"><i style=${{ width: `${Math.round(b.frac * 100)}%` }}></i></span><span>${b.text}</span>`)}</span>`;
}

export function BuyMenu({ s, buy, buyAmmo, close }) {
  const cats = buyCategories(s.myTeam);
  const [cat, setCat] = useState(-1);
  const wide = useMedia(WIDE_QUERY);
  useEffect(() => {
    // Keys the menu handles stop here: the game must not see them too. The item's
    // digit closes the menu, and the game would then take it as a slot key
    // (b-5-3 bought shades and drew the keyboard: operator report).
    const own = (e) => { e.preventDefault(); e.stopImmediatePropagation(); };
    const onKey = (e) => {
      if (e.code === "Escape" || e.code === s.buyKey) { own(e); close(); return; }
      if (e.code === "Backspace" || e.code === "Digit0") { own(e); setCat(-1); return; }
      const n = /^Digit([1-9])$/.exec(e.code)?.[1];
      if (!n) return;
      own(e);
      const i = +n - 1;
      if (cat < 0) { if (cats[i]?.items.length) setCat(i); return; }
      const it = cats[cat].items[i];
      if (it) pick(it);
    };
    addEventListener("keydown", onKey, true);
    return () => removeEventListener("keydown", onKey, true);
  }, [cat, cats.length, s.buyKey]);
  function pick(it) {
    buy(it.id);
    if (!owned(it, s) && itemPrice(it, s) <= s.money) close();
  }
  const open = (i) => cats[i].items.length && setCat(cat === i ? -1 : i);
  const ammo = ammoCards(s), keys = s.ammoKeys || {};
  const time = s.buyLeft != null ? html`buy time <b>${Math.max(0, s.buyLeft)} s</b>` : html`warmup · <b>no time limit</b>`;
  const msg = s.buyMsg ? html`<span class="buy-msg">${s.buyMsg}</span>` : "";
  const closeKey = html`<b class="key">${s.buyKeyName || "B"}</b>`;
  const items = cat < 0 ? [] : cats[cat].items;
  const item = (it, ii) => {
    const p = itemPrice(it, s), have = owned(it, s), poor = p > s.money, st = itemStats(it);
    const cls = `buy-item ${have ? "have" : ""} ${poor ? "poor" : ""}`;
    const price = html`<span class="bi-price">${have ? "owned" : money(p)}</span>`;
    return wide
      ? html`<button type="button" class=${cls} disabled=${have || poor} title=${it.desc || ""} onClick=${() => pick(it)}>
          <${Icon} id=${it.id} />
          <span class="bi-head"><b class="key">${ii + 1}</b><span class="bi-name">${it.name}</span></span>${price}
          <span class="bi-flav">${it.desc || ""}</span>
          <${Bars} bars=${st.bars} /></button>`
      : html`<button type="button" class=${cls} disabled=${have || poor} title=${it.desc || ""} onClick=${() => pick(it)}>
          <b class="key">${ii + 1}</b><span class="bi-text"><span class="bi-name">${it.name}</span><span class="bi-line">${st.line}</span></span>${price}</button>`;
  };
  const ammoCard = (a) => html`<button type="button" class="ammo-card ${a.none || a.full ? "" : "need"} ${a.none ? "none" : ""} ${a.full ? "full" : ""} ${!a.none && !a.full && a.poor ? "poor" : ""}"
      disabled=${a.none || a.full || a.poor} data-item=${a.item} onClick=${() => buyAmmo(a.item)}>
      <b class="key big">${keyCap(keys[a.cmd] || (a.item === "ammo1" ? "Comma" : "Period"))}</b>
      <span class="ac-name">${a.none ? html`<span class="ac-c">no ${a.label}</span>` : wide ? html`${a.name} <span class="ac-c">· ${a.label}</span>` : a.name}
        ${!wide && !a.none ? html` <span class="ac-c ac-res">reserve ${a.reserve} / ${a.max}</span>` : ""}</span>
      <span class="bar"><i style=${{ width: `${Math.round(a.frac * 100)}%` }}></i></span>
      <span class="ac-act">${a.none ? "—" : a.full ? html`<span class="ac-full">FULL</span>` : html`${wide ? html`<span class="ac-add">+${a.missing}</span>` : ""}<span class="bi-price">${money(a.price)}</span>`}</span>
    </button>`;

  if (wide) {
    return html`<div class="overlay buy-overlay" onClick=${close}>
      <div class="buy-menu buy-a" onClick=${(e) => e.stopPropagation()}>
        <div class="buy-top"><span class="buy-title">BUY</span><span class="buy-money">${money(s.money)}</span>${msg}
          <span class="buy-time">${time} · ${closeKey} close</span></div>
        <div class="buy-cats">
          ${cats.map((c, i) => html`<button type="button" class="buy-cat ${cat === i ? "on" : ""} ${c.items.length ? "" : "empty"}" onClick=${() => open(i)}>
            <b class="key">${i + 1}</b>${c.name.toUpperCase()}<span class="n">${c.items.length}</span></button>`)}
          <button type="button" class="buy-cat back" onClick=${() => setCat(-1)}><b class="key">0</b>BACK</button>
        </div>
        <div class="buy-items">
          ${cat < 0
            ? cats.map((c, i) => html`<button type="button" class="buy-tile ${c.items.length ? "" : "empty"}" onClick=${() => open(i)}>
                <b class="key">${i + 1}</b><span class="bt-name">${c.name.toUpperCase()}</span>
                <span class="bt-icons">${c.items.slice(0, 3).map((it) => html`<${Icon} id=${it.id} />`)}</span>
                <span class="bt-from">${c.items.length ? html`${c.items.length} · from <span class="bi-price">${money(Math.min(...c.items.map((it) => itemPrice(it, s))))}</span>` : "nothing on sale yet"}</span></button>`)
            : items.map(item)}
        </div>
        <div class="buy-ammo">
          <div class="ba-title">AMMO<span>always here</span></div>
          ${ammo.map(ammoCard)}
        </div>
      </div></div>`;
  }
  return html`<div class="overlay buy-overlay" onClick=${close}>
    <div class="buy-menu buy-c" onClick=${(e) => e.stopPropagation()}>
      <div class="buy-top"><span class="buy-title">BUY</span><span class="buy-money">${money(s.money)}</span>
        <span class="buy-time">${s.buyLeft != null ? html`<b>${Math.max(0, s.buyLeft)} s</b> left` : html`<b>warmup</b>`}</span></div>
      <div class="buy-sub">${cat < 0 ? html`<span class="crumb">pick a category</span>`
        : html`<b class="key">${cat + 1}</b> ${cats[cat].name.toUpperCase()} <span class="crumb">· <button type="button" class="buy-back" onClick=${() => setCat(-1)}><b class="key">0</b> back</button></span>`}${msg}</div>
      <div class="buy-rows">
        ${cat < 0
          ? cats.map((c, i) => html`<button type="button" class="buy-cat ${c.items.length ? "" : "empty"}" onClick=${() => open(i)}>
              <b class="key">${i + 1}</b><span class="bi-text"><span class="bi-name">${c.name}</span><span class="bi-line">${c.items.map((it) => it.name).join(" · ") || "nothing on sale yet"}</span></span>
              <span class="n">${c.items.length}</span></button>`)
          : items.map(item)}
      </div>
      <div class="buy-ammo">
        <div class="ba-title">AMMO</div>
        ${ammo.map(ammoCard)}
      </div>
      <div class="buy-foot">number: ${cat < 0 ? "category" : "buy"} · click works too · Esc / ${s.buyKeyName || "B"} close</div>
    </div></div>`;
}

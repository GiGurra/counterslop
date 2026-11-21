// The buy menu (B, in your buy zone during freeze/buy time). It's the one
// overlay that takes game input away (the world keeps running for everyone):
// you stand still while shopping. Categories as in CS: press a category
// number, then the item number, or click. Buying is a request; the server
// decides (money, team, slot rules) and the result shows in the next snapshot.
import { h } from "preact";
import { useState, useEffect } from "preact/hooks";
import htm from "htm";
import econ from "../assets/economy.json" with { type: "json" };
import { WEAPONS } from "../assets/weapons/index.js";
import { owned, itemPrice } from "./buyrules.js";
export { itemPrice };

const html = htm.bind(h);

// The menu's items: weapons from the registry by class (team-specific ones
// only for that team), gear from economy.json.
export function buyCategories(team) {
  return econ.buyCategories.map((c) => ({
    name: c.name,
    items: c.gear
      ? econ.gear.filter((g) => !g.team || g.team === team).map((g) => ({ id: g.id, name: g.name, desc: g.desc, price: g.price, upgrade: g.upgrade }))
      : Object.values(WEAPONS)
          .filter((w) => c.classes.includes(w.class) && w.slot !== "melee" && (!w.team || w.team === team))
          .sort((a, b) => a.price - b.price)
          .map((w) => ({ id: w.id, name: w.name, desc: w.flavor, price: w.price, slot: w.slot })),
  })); // fixed numbers, CS-style: an empty category stays (greyed) so the keys never shift
}

// What this item costs this player right now (the helmet upgrade; warmup charges too, from full money).


export function BuyMenu({ s, buy, close }) {
  const cats = buyCategories(s.myTeam);
  const [cat, setCat] = useState(-1);
  useEffect(() => {
    const onKey = (e) => {
      if (e.code === "Escape" || e.code === s.buyKey) { e.preventDefault(); close(); return; }
      if (e.code === "Backspace" || e.code === "Digit0") { e.preventDefault(); setCat(-1); return; }
      const n = /^Digit([1-9])$/.exec(e.code)?.[1];
      if (!n) return;
      e.preventDefault();
      const i = +n - 1;
      if (cat < 0) { if (cats[i]?.items.length) setCat(i); return; }
      const it = cats[cat].items[i];
      if (it) buy(it.id);
    };
    addEventListener("keydown", onKey, true);
    return () => removeEventListener("keydown", onKey, true);
  }, [cat, cats.length, s.buyKey]);
  const shown = cat < 0 ? cats : [cats[cat]];
  return html`<div class="overlay buy-overlay" onClick=${close}>
    <div class="panel buy-menu" onClick=${(e) => e.stopPropagation()}>
      <div class="buy-head"><h2>BUY</h2><span class="buy-money">$${s.money}</span>
        <span class="buy-time">${s.buyLeft != null ? `${Math.max(0, Math.ceil(s.buyLeft))} s left` : ""}</span></div>
      <div class="buy-cats">
        ${shown.map((c) => {
          const ci = cats.indexOf(c);
          return html`<div class="buy-cat ${cat === ci ? "open" : ""}">
            <h3 onClick=${() => setCat(cat === ci ? -1 : ci)}><b>${ci + 1}</b> ${c.name}</h3>
            ${!c.items.length && html`<div class="buy-empty">nothing on sale yet</div>`}
            ${c.items.map((it, ii) => {
              const p = itemPrice(it, s), have = owned(it, s), poor = p > s.money;
              return html`<button type="button" class="buy-item ${have ? "have" : ""} ${poor ? "poor" : ""}" disabled=${have || poor}
                  title=${it.desc || ""} onClick=${() => buy(it.id)}>
                ${cat === ci && html`<b>${ii + 1}</b>`}<span class="bi-name">${it.name}</span>
                <span class="bi-price">${have ? "owned" : `$${p}`}</span></button>`;
            })}
          </div>`;
        })}
      </div>
      <p class="buy-hint">${cat < 0 ? "number: category" : "number: buy · 0 / Backspace: back"} · Esc or ${s.buyKeyName || "B"}: close · ${s.buyMsg || ""}</p>
    </div></div>`;
}

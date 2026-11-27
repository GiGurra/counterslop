// The race garage's panel (game/garage.js: what it shows; game/garagescene.js
// draws the bay under it). Shopping happens on the car: every part of the
// car on the turntable has a target on it (front and rear mount, tyres,
// engine, plating, boost tank, charges), and every parked car a label in its
// bay. Point at one: what it does; click a part: just that slot's options,
// with prices, bought in place (the part changes on the model as soon as the
// server's snapshot says so); click a parked car: it rolls onto the turntable
// for a look, its label then buys it. Keys: arrows walk the bays (top row),
// the parts and a part's options, Enter acts, Backspace / Esc steps back,
// Esc / the garage key leaves. The spec card on the left; the parts list on
// the right is the docked shop (every slot, what's fitted, the next price).
// Owned is green "OWNED", what you can afford a price, what you can't red.
import { h } from "preact";
import { useEffect } from "preact/hooks";
import htm from "htm";
import { garageView, moveFocus, focusTarget, TIERS } from "../game/garage.js";
import { RaceCentre } from "./race.js";

const html = htm.bind(h);
const money = (n) => `$${String(n).replace(/\B(?=(\d{3})+$)/g, " ")}`;
const LABEL = { owned: "OWNED", selected: "SELECTED", free: "FREE" };
const price = (o) => html`<span class="g-price ${o.state}">${LABEL[o.state] || money(o.price)}${o.state === "poor" ? html`<small> · can't afford</small>` : ""}</span>`;
const pips = (n, max = 3) => html`<span class="g-pips">${Array.from({ length: max }, (_, i) => html`<i class=${i < n ? "on" : ""}></i>`)}</span>`;

// s: the HUD state (s.garageUi: {preview, focus, hover}); act: {set(ui),
// buy(item), close(), state(): the HUD state now (keys can come faster than
// renders: each one works from the state the last one left)}.
export function Garage({ s, act }) {
  return html`<div class="garage">${garageBody(s, act)}</div>`;
}
function garageBody(s, act) {
  const s0 = s;
  const ui = s.garageUi || {};
  const view = garageView({ gar: s.garage, carType: s.carType, money: s.money, shopOpen: s.canBuy, preview: ui.preview });
  const focus = ui.focus || { row: "bays", i: Math.max(0, view.bays.findIndex((b) => b.shown)), slot: "" };
  const openSlot = focus.row === "options" ? focus.slot : "";
  const set = (p) => act.set({ ...ui, ...p });
  // What a pick does: a bay previews its car, then buys it; a part opens its options.
  const doTarget = (t) => {
    if (!t) return;
    if (t.kind === "preview") set({ preview: t.id, focus: { row: "bays", i: view.bays.findIndex((b) => b.id === t.id), slot: "" } });
    else if (t.kind === "open") set({ focus: { row: "options", i: 0, slot: t.slot } });
    else if (t.kind === "buy") act.buy(t.item);
  };
  const pickBay = (id) => { const i = view.bays.findIndex((b) => b.id === id); doTarget(focusTarget(view, { row: "bays", i })); };
  const pickSlot = (id) => {
    if (view.previewing) { set({ preview: "", focus: { row: "options", i: 0, slot: id } }); return; } // back to our own car, that part open
    set({ focus: openSlot === id ? { row: "slots", i: view.slots.findIndex((x) => x.id === id), slot: "" } : { row: "options", i: 0, slot: id } });
  };
  useEffect(() => {
    const own = (e) => { e.preventDefault(); e.stopImmediatePropagation(); };
    const onKey = (e) => {
      const s = act.state ? act.state() : s0, ui = s.garageUi || {};
      const view = garageView({ gar: s.garage, carType: s.carType, money: s.money, shopOpen: s.canBuy, preview: ui.preview });
      const focus = ui.focus || { row: "bays", i: Math.max(0, view.bays.findIndex((b) => b.shown)), slot: "" };
      const set = (p) => act.set({ ...ui, ...p });
      const doTarget = (t) => {
        if (!t) return;
        if (t.kind === "preview") set({ preview: t.id, focus: { row: "bays", i: view.bays.findIndex((b) => b.id === t.id), slot: "" } });
        else if (t.kind === "open") set({ focus: { row: "options", i: 0, slot: t.slot } });
        else if (t.kind === "buy") act.buy(t.item);
      };
      if (e.code === "Escape" || e.code === s.garageKey) {
        own(e);
        if (e.code === "Escape" && focus.row === "options") set({ focus: moveFocus(view, focus, "Backspace") });
        else act.close();
        return;
      }
      if (e.code === "KeyG") { own(e); act.guns(); return; } // the CS buy menu (guns, gear, grenades); closing it comes back here
      if (["ArrowLeft", "ArrowRight", "ArrowUp", "ArrowDown", "Backspace"].includes(e.code)) {
        own(e);
        let f = moveFocus(view, focus, e.code);
        if (f.row === "slots" && view.previewing) f = { row: "bays", i: focus.i, slot: "" }; // a previewed car has no parts of ours: stay on the bays
        const patch = { focus: f };
        if (f.row === "bays" && (e.code === "ArrowLeft" || e.code === "ArrowRight")) patch.preview = view.bays[f.i]?.id === view.own ? "" : view.bays[f.i]?.id; // walking the bays: each rolls onto the table
        set(patch);
        return;
      }
      if (e.code === "Enter" || e.code === "NumpadEnter" || e.code === "Space") {
        own(e);
        if (focus.row === "slots") set({ focus: moveFocus(view, focus, "Enter") });
        else doTarget(focusTarget(view, focus));
      }
    };
    addEventListener("keydown", onKey, true);
    return () => removeEventListener("keydown", onKey, true);
  }, []);

  const c = view.card;
  const hover = (slot) => ({ onMouseEnter: () => set({ hover: slot }), onMouseLeave: () => set({ hover: "" }) });
  const hovered = view.slots.find((x) => x.id === ui.hover);
  const msg = s.buyMsg ? html`<div class="g-msg bad">${s.buyMsg}</div>` : s.buyOk ? html`<div class="g-msg ok">bought ✓</div>` : "";
  const warm = s.phase === "warmup" ? html`<div class="g-warm">warmup: try things out; the match starts everyone stock</div>` : "";
  // Mid-race the garage is a pit stop (internal/game pitstop.go): what you pick is the car you drive when you leave.
  const pit = s.pit ? html`<div class="g-pit"><b>PIT STOP</b> · your car is off the track and the race clock is running · leave with <b class="key">${s.garageKeyName || "H"}</b> to rejoin at your last gate in the car you picked</div>` : "";
  const shop = view.shopOpen ? html`shop open${s.pit ? " · pit stop" : s.buyLeft > 0 ? html` · <b>${Math.max(0, s.buyLeft)} s</b> to GO` : s.phase === "warmup" ? " · warmup" : ""}`
    : html`<span class="bad">${s.phase === "live" ? "pulling into the pit…" : "shop closed now: look only"}</span>`;
  const slotRow = (x, i) => {
    const open = openSlot === x.id, foc = focus.row === "slots" && focus.i === i;
    return html`<div class="g-slot ${open ? "open" : ""} ${foc ? "focus" : ""}" data-slot=${x.id}>
      <button type="button" class="g-slot-head" onClick=${() => pickSlot(x.id)} ...${hover(x.id)} title=${x.desc}>
        <span class="g-slot-name">${x.name}</span>
        <span class="g-slot-fit">${x.tier != null && !x.none ? pips(x.tier, x.max) : ""}${x.fitted}</span>
        <span class="g-slot-next">${x.options.some((o) => o.state !== "owned") ? price(x.options.find((o) => o.state !== "owned")) : x.none ? "" : html`<span class="g-price owned">${x.tier != null ? "MAXED" : "OWNED"}</span>`}</span>
      </button>
      ${open && html`<div class="g-opts">${x.options.length ? x.options.map((o, oi) => html`
        <button type="button" class="g-opt ${o.state} ${focus.row === "options" && focus.i === oi ? "focus" : ""}" data-item=${o.buy}
          disabled=${o.state === "owned" || o.state === "poor" || !view.shopOpen} onClick=${() => act.buy(o.buy)} title=${o.desc}>
          <span class="g-opt-name">${o.name}${o.builtin ? html` <small>built in</small>` : ""}</span>${price(o)}
          <span class="g-opt-desc">${o.desc}</span></button>`) : html`<div class="g-opt-none">${x.none ? x.fitted : "maxed out: nothing more to fit"}</div>`}</div>`}
    </div>`;
  };
  return html`
    <div class="g-top"><span class="g-title">GARAGE</span><span class="g-money">${money(s.money)}</span><span class="g-shop">${shop}</span>
      ${warm}<button type="button" class="g-guns secondary" onClick=${() => act.guns()}><b class="key">G</b> guns & gear</button><span class="g-leave"><b class="key">${s.garageKeyName || "H"}</b> / <b class="key">Esc</b> leave</span></div>
    ${pit}
    ${s.race && !s.pit && html`<div class="g-race"><${RaceCentre} r=${s.race} /></div>`}
    ${view.bays.map((b, i) => html`<button type="button" class="g-bay ${b.state} ${b.shown ? "shown" : ""} ${focus.row === "bays" && focus.i === i ? "focus" : ""}" data-ganchor=${`bay:${b.id}`} data-bay=${b.id}
        onClick=${() => pickBay(b.id)} title=${b.desc}>
        <span class="g-bay-name"><b>T${b.tier}</b> ${b.name}</span>
        ${b.shown && b.state === "owned" ? html`<span class="g-bay-act">${view.shopOpen ? html`DRIVE THIS · ${price(b)}` : price(b)}</span>`
          : b.shown && b.state !== "selected" ? html`<span class="g-bay-act">${b.state === "poor" ? html`${price(b)}` : view.shopOpen ? html`BUY ${price(b)}` : price(b)}</span>`
          : b.shown ? html`<span class="g-bay-act">${price(b)} · on the table</span>` : html`<span class="g-bay-act">${price(b)}</span>`}
      </button>`)}
    ${!view.previewing && view.slots.map((x) => html`<button type="button" class="g-hot ${openSlot === x.id ? "open" : ""} ${ui.hover === x.id ? "hover" : ""} ${x.tier ? "up" : ""}"
        data-ganchor=${`slot:${x.id}`} data-hot=${x.id} onClick=${() => pickSlot(x.id)} ...${hover(x.id)}><i></i><span>${x.name}</span></button>`)}
    ${hovered && html`<div class="g-tip"><b>${hovered.name}</b>: ${hovered.desc}. <span>${hovered.fitted}</span></div>`}
    <div class="g-card">
      ${view.previewing && html`<div class="g-preview">PREVIEW · ${c.name} would come with its own built-ins; your upgrades carry over</div>`}
      <div class="g-card-name"><b>T${c.tier}</b> ${c.name}</div>
      <table>
        <tr><td>top speed</td><td>${c.topKmh} km/h</td></tr>
        <tr><td>car HP</td><td>${c.hp}</td></tr>
        <tr><td>boost tank</td><td>${c.tank} s a lap</td></tr>
        <tr><td>front</td><td>${c.front ? html`${c.front.name}${c.front.charges ? html` <small>×${c.front.charges} a lap</small>` : ""}` : "–"}</td></tr>
        <tr><td>rear</td><td>${c.rear ? html`${c.rear.name} <small>×${c.rear.charges} a lap</small>` : "–"}</td></tr>
        ${Object.entries(c.tiers).map(([k, n]) => html`<tr class="g-tier"><td>${k}</td><td>${pips(n)} ${TIERS[n]}</td></tr>`)}
      </table>
      ${view.previewing && html`<div class="g-card-act">
        ${view.bays.find((b) => b.shown)?.state === "owned"
          ? html`<button type="button" class="g-buy g-drive" data-item=${`car:${c.id}`} disabled=${!view.shopOpen} onClick=${() => act.buy(`car:${c.id}`)}>DRIVE THIS <small>owned · free</small></button>`
          : html`<button type="button" class="g-buy" data-item=${`car:${c.id}`} disabled=${!view.shopOpen || view.bays.find((b) => b.shown)?.state === "poor"} onClick=${() => act.buy(`car:${c.id}`)}>BUY ${price(view.bays.find((b) => b.shown))}</button>`}
        <button type="button" class="secondary" onClick=${() => set({ preview: "", focus: { row: "bays", i: view.bays.findIndex((b) => b.id === view.own), slot: "" } })}>back to yours</button></div>`}
    </div>
    <div class="g-parts">
      <div class="g-parts-title">${view.previewing ? (view.bays.find((b) => b.shown)?.state === "owned" ? "yours: drive it to fit parts" : "buy it to fit parts") : "PARTS · click one on the car or here"}</div>
      ${!view.previewing && view.slots.map(slotRow)}
      ${msg}
    </div>
    <div class="g-foot">click a car in a bay: it rolls onto the table · click a part: its options · ←→ walk · ↑↓ bays / parts / options · Enter buy · drag: turn · wheel: zoom</div>`;
}

// The grenade kind pill (HE / FLASH / SMOKE in the kind's colour) shown in front
// of a grenade's slang name: buy menu, HUD grenade row, kill feed (GitHub #3).
// "" for anything that isn't a grenade.
import { h } from "preact";
import htm from "htm";
import { nadeKind } from "../assets/weapons/types.js";

const html = htm.bind(h);

export function nadeTag(id) {
  const k = nadeKind(id);
  return k ? html`<span class="ktag" style=${{ "--k": k.color }} title=${k.long}>${k.tag}</span>` : "";
}

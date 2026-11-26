// The map editor's panel (web/game/editor.js hud()): the tool and what it
// will make, the selection, the other editors, and the keys. Top right, under
// the radar's height; it takes no clicks (the mouse is the editor's).
const TOOL_KEYS = [["1", "select"], ["2", "box"], ["3", "zone"], ["4", "spawn"], ["5", "item"]];
const rgb = (c) => `rgb(${c.map((v) => Math.round(v * 255)).join(",")})`;

export function editorPanel(html, e) {
  const what = {
    select: e.sel ? `box ${e.sel.i}: ${e.sel.kind}${e.sel.ramp ? ` ramp ${e.sel.ramp}` : ""}, ${e.sel.size.join(" × ")} m${e.drag ? ` (${e.drag})` : ""}` : "click a box",
    box: `${e.material}${e.ramp ? ` ramp ${e.ramp}` : ""}, ${e.height} m tall${e.anchor ? ": click the other corner" : ": click a corner"}`,
    zone: `${e.zone}${e.anchor ? ": click the other corner" : ": click a corner"}`,
    spawn: `a ${e.team} spawn: click; right click deletes the nearest`,
    item: `${e.weapon}: click; right click deletes the nearest`,
  }[e.tool];
  const keys = {
    select: "drag a face: resize · Shift+drag: move · arrows, PgUp/PgDn: nudge · [ ]: material · R: ramp · C: copy up · Del",
    box: "wheel: height · [ ]: material · R: ramp",
    zone: "Z: site A / site B / buy zones",
    spawn: "T: team",
    item: "[ ]: weapon",
  }[e.tool];
  return html`<div class="editor-panel">
    <h3>MAP EDITOR</h3>
    <div class="editor-tools">${TOOL_KEYS.map(([k, t]) => html`<span class=${t === e.tool ? "on" : ""} key=${t}><b>${k}</b> ${t}</span>`)}</div>
    <div class="editor-what">${what}</div>
    <div class="editor-meta">snap ${e.snap} m (G) · ${e.boxes} boxes${e.at ? ` · ${e.at.join(", ")}` : ""}</div>
    <div class="editor-keys">${keys}</div>
    <div class="editor-keys">Ctrl+Z / Ctrl+Y: undo / redo · Ctrl+S: save · P: playtest · right click: cancel</div>
    ${e.editors?.length > 0 && html`<div class="editor-others">${e.editors.map((o) => html`<div key=${o.name}><i style=${{ background: rgb(o.colour) }}></i>${o.name}${o.mcp ? " (agent)" : ""}${o.tool ? `: ${o.tool}` : ""}</div>`)}</div>`}
  </div>`;
}

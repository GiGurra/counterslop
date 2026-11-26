// The in-game map editor (Custom maps 4): tools over the server's edit
// session (internal/game mapedit.go, docs/map-editor.md). Every edit goes to
// the server as one op (map_op <json>, map_undo, ...) and comes back to every
// client as a mapdelta: this module never changes the map itself. Pure apart
// from what it is given, so node tests drive it (editor.test.mjs).
//
//   createEditor({ send(cmd, args), pick(o, d) -> {i, t, n} | null, map() -> the map,
//                  materials: [names], weapons: [ids] })
//   .setActive(on)               editing (we fly: noclip) as an admin
//   .key(code, down, mods)       true: the editor took it (mods: {shift, ctrl})
//   .frame(eye, dir)             every frame: the cursor, a drag in progress
//   .parts(time)                 solids to draw: selection, handles, previews, zones, spawns
//   .hud()                       the panel's state
//   .presence()                  what we show the other editors ({t:"editpresence"} minus t)
//   .setEditors(list, myId)      the server's {t:"editors"} list: drawn by parts()
//
// Presence (agreed with internal/game mapedit.go): refs address objects as the
// edit ops do: {kind:"box",i} | {kind:"spawn",team,i} | {kind:"zone",zone,name|team,i}
// | {kind:"item",i} | {kind:"hint",i} | {kind:"nav",name}.
// Tools (1-6): select (click a box; drag a face to resize it, Shift+drag to
// move it; arrows / PgUp / PgDn nudge it a snap; Delete; R its ramp; [ ] its
// material; C duplicates it), box (click two corners on any surface; the
// wheel sets the height), zone (two corners: site A/B or a team's buy zone; Z
// picks which), spawn (click: a spawn for the team, T picks the team; right
// click: delete the nearest), item (click: a weapon, [ ] picks it; right
// click: delete the nearest), hint (click: a nav hint, a point a generated
// bot graph passes through, docs/navgen.md; Shift+click: a lane hint, an
// attacker's route choice; L flips the nearest one's lane; right click:
// delete the nearest). G: the snap. Ctrl+Z / Ctrl+Y: undo / redo (the
// session's, everyone's), Ctrl+S: save, P: playtest. Right click: cancel.

export const TOOLS = ["select", "box", "zone", "spawn", "item", "hint"];
export const SNAPS = [0.25, 0.5, 1, 2];
export const ZONES = [{ kind: "site", name: "A" }, { kind: "site", name: "B" }, { kind: "buy", team: "vibe" }, { kind: "buy", team: "artisan" }];
export const TEAMS = ["vibe", "artisan"];
const ZONE_H = 3.5; // a zone's height over the floor it is painted on (and 0.5 below)

const snapTo = (v, s) => Math.round(v / s) * s;
const r3 = (v) => v.map((x) => Math.round(x * 1000) / 1000);
const dot = (a, b) => a[0] * b[0] + a[1] * b[1] + a[2] * b[2];

export function createEditor({ send, pick, map, materials = ["concrete"], weapons = ["prompt"] }) {
  const st = {
    active: false, tool: "select", snap: 0.5, mat: Math.max(0, materials.indexOf("concrete")), ramp: 0, height: 2,
    zone: 0, team: 0, weapon: 0,
    sel: -1, // the selected box
    anchor: null, // the box or zone tool's first corner
    drag: null, // {axis, sign, p0, min, max, move, s}
    hover: null, // {i, t, n, p: the hit point, at: the snapped cursor}
    note: "",
    editors: [], myId: 0,
  };
  const op = (o) => send("map_op", [JSON.stringify(o)]);
  const boxes = () => map()?.boxes || [];

  // The cursor: where the view ray meets the map, snapped on the surface's
  // plane (the coordinate across the surface stays exact: a floor at -0.6 is -0.6).
  function cursorOf(hit, eye, dir) {
    const p = [0, 1, 2].map((k) => eye[k] + dir[k] * hit.t);
    const axis = hit.n.findIndex((v) => Math.abs(v) > 0.5);
    return p.map((v, k) => (k === axis ? v : snapTo(v, st.snap)));
  }

  function frame(eye, dir) {
    st.eye = eye; st.dir = dir;
    const hit = st.active ? pick(eye, dir) : null;
    st.hover = hit ? { ...hit, p: [0, 1, 2].map((k) => eye[k] + dir[k] * hit.t), at: cursorOf(hit, eye, dir) } : null;
    if (st.drag) {
      const d = st.drag, n = [0, 0, 0];
      n[d.axis] = d.sign;
      // The point on the face's normal line through where we grabbed it nearest the view ray.
      const w0 = [0, 1, 2].map((k) => d.p0[k] - eye[k]);
      const b = dot(n, dir), den = 1 - b * b;
      if (den > 1e-4) d.s = snapTo((b * dot(dir, w0) - dot(n, w0)) / den, st.snap);
    }
  }

  // The box a drag makes of the box it holds (resize the face, or move the box).
  function dragged() {
    const d = st.drag;
    const min = [...d.min], max = [...d.max], by = d.s * d.sign;
    if (d.move) { min[d.axis] += by; max[d.axis] += by; }
    else if (d.sign > 0) max[d.axis] = Math.max(min[d.axis] + st.snap, max[d.axis] + by);
    else min[d.axis] = Math.min(max[d.axis] - st.snap, min[d.axis] + by);
    return { min: r3(min), max: r3(max) };
  }

  // Two corners on surfaces -> a box standing on the first one's floor (or, on
  // a wall, as thick as a snap), at least a snap in every direction.
  function spanBox(a, b, h) {
    const min = [Math.min(a[0], b[0]), a[1], Math.min(a[2], b[2])];
    const max = [Math.max(a[0], b[0]), a[1] + h, Math.max(a[2], b[2])];
    for (const k of [0, 2]) if (max[k] - min[k] < st.snap) max[k] = min[k] + st.snap;
    if (max[1] < min[1]) [min[1], max[1]] = [max[1], min[1]];
    return { min: r3(min), max: r3(max) };
  }

  const nearestIn = (list, p) => {
    let best = -1, bd = 1.5;
    (list || []).forEach((q, i) => { const d = Math.hypot(q[0] - p[0], q[1] - p[1], q[2] - p[2]); if (d < bd) { best = i; bd = d; } });
    return best;
  };

  // A primary click (Mouse0 down) with the current tool.
  function primary(shift) {
    const h = st.hover;
    switch (st.tool) {
      case "select": {
        if (!h) { st.sel = -1; return; }
        if (h.i === st.sel && h.i >= 0) { // grab the face we point at
          const b = boxes()[h.i], axis = h.n.findIndex((v) => Math.abs(v) > 0.5);
          st.drag = { axis, sign: Math.sign(h.n[axis]), p0: h.p, min: [...b.min], max: [...b.max], move: shift, s: 0 };
        } else st.sel = h.i;
        return;
      }
      case "box": case "zone": {
        if (!h) return;
        if (!st.anchor) { st.anchor = h.at; return; }
        if (st.tool === "box") {
          const b = spanBox(st.anchor, h.at, st.height);
          op({ op: "box_add", box: { ...b, kind: materials[st.mat], ...(st.ramp ? { ramp: st.ramp } : {}) } });
        } else {
          const z = ZONES[st.zone], b = spanBox([st.anchor[0], st.anchor[1] - 0.5, st.anchor[2]], h.at, ZONE_H + 0.5);
          const n = (map()?.buyZones?.[z.team] || []).length;
          op({ op: "zone_set", kind: z.kind, ...(z.name ? { name: z.name } : { team: z.team, i: n }), min: b.min, max: b.max });
        }
        st.anchor = null;
        return;
      }
      case "spawn": if (h) op({ op: "spawn_add", team: TEAMS[st.team], pos: r3(h.at) }); return;
      case "item": if (h) op({ op: "item_add", weapon: weapons[st.weapon], pos: r3([h.at[0], h.at[1] + 0.1, h.at[2]]) }); return;
      case "hint": if (h) op({ op: "hint_add", pos: r3(h.at), ...(shift ? { lane: true } : {}) }); return;
    }
  }

  const nearestHint = (p) => nearestIn((map()?.navHints || []).map((n) => n.pos), p);

  // Right click: cancel what is half done, else delete the nearest spawn / item, else deselect.
  function secondary() {
    if (st.anchor || st.drag) { st.anchor = null; st.drag = null; return; }
    const h = st.hover;
    if (st.tool === "spawn" && h) {
      const team = TEAMS[st.team], i = nearestIn(map()?.teamSpawns?.[team], h.at);
      if (i >= 0) op({ op: "spawn_delete", team, i });
    } else if (st.tool === "item" && h) {
      const i = nearestIn((map()?.items || []).map((it) => it.pos), h.at);
      if (i >= 0) op({ op: "item_delete", i });
    } else if (st.tool === "hint" && h) {
      const i = nearestHint(h.at);
      if (i >= 0) op({ op: "hint_delete", i });
    } else st.sel = -1;
  }

  // Arrow keys nudge the selection along the world axis nearest the view.
  function nudge(code) {
    const d = st.dir || [0, 0, -1];
    const fwd = Math.abs(d[0]) > Math.abs(d[2]) ? [Math.sign(d[0]), 0, 0] : [0, 0, Math.sign(d[2]) || -1];
    const right = [-fwd[2], 0, fwd[0]];
    const v = { ArrowUp: fwd, ArrowDown: fwd.map((x) => -x), ArrowRight: right, ArrowLeft: right.map((x) => -x), PageUp: [0, 1, 0], PageDown: [0, -1, 0] }[code];
    op({ op: "box_move", i: st.sel, by: v.map((x) => x * st.snap) });
  }

  function key(code, down, mods = {}) {
    if (!st.active) return false;
    if (code === "Mouse0") {
      if (down) primary(!!mods.shift);
      else if (st.drag) { // let go: one op for the whole drag (one undo step)
        const d = st.drag, b = dragged();
        st.drag = null;
        if (d.s !== 0) op(d.move ? { op: "box_move", i: st.sel, by: [0, 1, 2].map((k) => r3([b.min[k] - d.min[k]])[0]) } : { op: "box_resize", i: st.sel, min: b.min, max: b.max });
      }
      return true;
    }
    if (code === "Mouse2") { if (down) secondary(); return true; }
    if (!down) return ["MWheelUp", "MWheelDown"].includes(code);
    if (mods.ctrl) {
      if (code === "KeyZ") { send("map_undo", []); return true; }
      if (code === "KeyY") { send("map_redo", []); return true; }
      if (code === "KeyS") { send("map_save", []); return true; }
      return false;
    }
    const sel = st.sel >= 0 && st.sel < boxes().length;
    const step = (i, n, by) => (i + by + n) % n;
    switch (code) {
      case "Digit1": case "Digit2": case "Digit3": case "Digit4": case "Digit5": case "Digit6":
        st.tool = TOOLS[+code.slice(5) - 1]; st.anchor = null; st.drag = null; return true;
      case "KeyG": st.snap = SNAPS[step(SNAPS.indexOf(st.snap), SNAPS.length, 1)]; return true;
      case "KeyZ": st.zone = step(st.zone, ZONES.length, 1); return true;
      case "KeyT": st.team = step(st.team, TEAMS.length, 1); return true;
      case "KeyP": send("map_playtest", []); return true;
      case "KeyL": {
        if (st.tool !== "hint" || !st.hover) return false;
        const i = nearestHint(st.hover.at);
        if (i >= 0) op({ op: "hint_lane", i, lane: !map().navHints[i].lane });
        return true;
      }
      case "KeyR":
        st.ramp = step(sel && st.tool === "select" ? boxes()[st.sel].ramp || 0 : st.ramp, 5, 1);
        if (sel && st.tool === "select") op({ op: "box_ramp", i: st.sel, ramp: st.ramp });
        return true;
      case "BracketLeft": case "BracketRight": {
        const by = code === "BracketRight" ? 1 : -1;
        if (st.tool === "item") { st.weapon = step(st.weapon, weapons.length, by); return true; }
        st.mat = step(st.mat, materials.length, by);
        if (sel && st.tool === "select") op({ op: "box_material", i: st.sel, kind: materials[st.mat] });
        return true;
      }
      case "MWheelUp": case "MWheelDown":
        if (st.tool === "box") { st.height = Math.max(st.snap, st.height + (code === "MWheelUp" ? st.snap : -st.snap)); return true; }
        return false;
      case "Delete": case "Backspace":
        if (sel) { op({ op: "box_delete", i: st.sel }); st.sel = -1; }
        return true;
      case "KeyC":
        if (sel) { const b = boxes()[st.sel]; op({ op: "box_add", box: { min: [b.min[0], b.max[1], b.min[2]], max: [b.max[0], b.max[1] + (b.max[1] - b.min[1]), b.max[2]], kind: b.kind, ...(b.ramp ? { ramp: b.ramp } : {}) } }); }
        return true;
      case "ArrowUp": case "ArrowDown": case "ArrowLeft": case "ArrowRight": case "PageUp": case "PageDown":
        if (sel) nudge(code);
        return true;
    }
    return false;
  }

  // ---- drawing: outlines of thin emissive bars, markers, previews ----
  const EDGE = 0.04;
  function outline(min, max, color, emit = 1, w = EDGE) {
    const out = [], c = [0, 1, 2].map((k) => (min[k] + max[k]) / 2), s = [0, 1, 2].map((k) => max[k] - min[k]);
    for (let a = 0; a < 3; a++) {
      const b = (a + 1) % 3, d = (a + 2) % 3;
      for (const sb of [-1, 1]) for (const sd of [-1, 1]) {
        const p = [...c], sz = [w, w, w];
        p[b] += (sb * s[b]) / 2; p[d] += (sd * s[d]) / 2; sz[a] = s[a] + w;
        out.push(cube(p, sz, color, emit));
      }
    }
    return out;
  }
  function cube(p, s, color, emit = 1) {
    return { m: [s[0], 0, 0, 0, 0, s[1], 0, 0, 0, 0, s[2], 0, p[0], p[1], p[2], 1], color, emit };
  }
  const COL = { sel: [1, 0.85, 0.1], hover: [0.4, 0.9, 1], preview: [0.3, 1, 0.4], A: [1, 0.35, 0.2], B: [1, 0.35, 0.2], vibe: [0.95, 0.3, 0.9], artisan: [0.2, 0.75, 1], item: [1, 1, 1], hint: [0.6, 1, 0.2], lane: [1, 0.55, 0.1] };

  function parts() {
    if (!st.active) return [];
    const m = map(), out = [], bs = boxes();
    for (const z of m?.sites || []) out.push(...outline(z.min, z.max, COL[z.name] || COL.A, 0.8, 0.06));
    for (const t of TEAMS) for (const z of m?.buyZones?.[t] || []) out.push(...outline(z.min, z.max, COL[t], 0.6, 0.05));
    for (const t of TEAMS) for (const p of m?.teamSpawns?.[t] || []) out.push(cube([p[0], p[1] + 0.9, p[2]], [0.3, 1.8, 0.3], COL[t], 0.7));
    for (const it of m?.items || []) out.push(cube(it.pos, [0.5, 0.15, 0.15], COL.item, 0.8));
    for (const n of m?.navHints || []) { // a post, a lane's taller with a flag
      out.push(cube([n.pos[0], n.pos[1] + 0.6, n.pos[2]], [0.12, 1.2, 0.12], n.lane ? COL.lane : COL.hint, 0.8));
      out.push(cube([n.pos[0], n.pos[1] + (n.lane ? 1.3 : 1.2), n.pos[2]], n.lane ? [0.5, 0.3, 0.06] : [0.3, 0.3, 0.3], n.lane ? COL.lane : COL.hint, 1));
    }
    const h = st.hover;
    if (h && h.i >= 0 && h.i !== st.sel && st.tool === "select" && bs[h.i]) out.push(...outline(bs[h.i].min, bs[h.i].max, COL.hover, 0.6));
    if (st.sel >= 0 && bs[st.sel]) {
      const b = st.drag ? dragged() : bs[st.sel];
      out.push(...outline(b.min, b.max, COL.sel, 1, 0.06));
      // handles: a knob on every face's centre (grab one: drag that face)
      for (let a = 0; a < 3; a++) for (const s of [-1, 1]) {
        const p = [0, 1, 2].map((k) => (b.min[k] + b.max[k]) / 2);
        p[a] = s > 0 ? b.max[a] : b.min[a];
        out.push(cube(p, [0.25, 0.25, 0.25], COL.sel, 1));
      }
    }
    out.push(...others());
    if (h) out.push(cube(h.at, [0.12, 0.12, 0.12], COL.preview, 1)); // the cursor
    if (st.anchor && h) {
      const zone = st.tool === "zone";
      const b = zone ? spanBox([st.anchor[0], st.anchor[1] - 0.5, st.anchor[2]], h.at, ZONE_H + 0.5) : spanBox(st.anchor, h.at, st.height);
      const z = ZONES[st.zone];
      out.push(...outline(b.min, b.max, zone ? COL[z.name || z.team] : COL.preview, 1, 0.05));
    }
    return out;
  }

  // Where a ref is on the map: its box (min, max) or null.
  function refBox(r) {
    const m = map();
    if (!r || !m) return null;
    const pt = (p, h = 1.8) => (p ? { min: [p[0] - 0.4, p[1], p[2] - 0.4], max: [p[0] + 0.4, p[1] + h, p[2] + 0.4] } : null);
    switch (r.kind) {
      case "box": return m.boxes?.[r.i] || null;
      case "spawn": return pt(m.teamSpawns?.[r.team]?.[r.i]);
      case "item": return pt(m.items?.[r.i]?.pos, 0.4);
      case "hint": return pt(m.navHints?.[r.i]?.pos, 1.4);
      case "nav": return pt(m.nav?.find((n) => n.name === r.name)?.pos, 0.6);
      case "zone":
        if (r.zone === "site") return m.sites?.find((z) => z.name === r.name) || null;
        if (r.zone === "pit") return m.pits?.find((z) => z.name === r.name) || null;
        return m.buyZones?.[r.team]?.[r.i] || null;
    }
    return null;
  }
  // An editor's colour, from its id (the agent, id 0: cyan #00e6ff, as the server's renders draw it).
  const editorColour = (id) => (id === 0 ? [0, 0.9, 1] : [[1, 0.5, 0.1], [0.6, 0.4, 1], [0.2, 1, 0.6], [1, 0.3, 0.6], [0.9, 0.9, 0.2]][id % 5]);
  function others() {
    const out = [];
    for (const e of st.editors) {
      if (e.id === st.myId && !e.mcp) continue;
      const c = editorColour(e.id);
      if (e.at) out.push(cube(e.at, [0.25, 0.25, 0.25], c, 1));
      if (e.pos) out.push(cube([e.pos[0], e.pos[1] + 2.1, e.pos[2]], [0.15, 0.15, 0.15], c, 1)); // over the editor's head
      const h = refBox(e.hover);
      if (h) out.push(...outline(h.min, h.max, c, 0.5));
      for (const r of e.sel || []) { const b = refBox(r); if (b) out.push(...outline(b.min, b.max, c, 1, 0.06)); }
    }
    return out;
  }

  function presence() {
    const h = st.hover, bs = boxes();
    return {
      at: h ? r3(h.at) : null,
      hover: h && h.i >= 0 ? { kind: "box", i: h.i } : null,
      sel: st.sel >= 0 && st.sel < bs.length ? [{ kind: "box", i: st.sel }] : [],
      tool: st.tool,
    };
  }

  function hud() {
    if (!st.active) return null;
    const z = ZONES[st.zone], bs = boxes(), b = st.sel >= 0 ? bs[st.sel] : null;
    return {
      tool: st.tool, snap: st.snap, material: materials[st.mat], ramp: st.ramp, height: st.height,
      zone: z.name ? `site ${z.name}` : `buy zone (${z.team})`, team: TEAMS[st.team], weapon: weapons[st.weapon],
      sel: b ? { i: st.sel, kind: b.kind, ramp: b.ramp || 0, size: r3([0, 1, 2].map((k) => b.max[k] - b.min[k])) } : null,
      anchor: !!st.anchor, drag: st.drag ? (st.drag.move ? "move" : "resize") : "", at: st.hover ? r3(st.hover.at) : null,
      boxes: bs.length, hints: (map()?.navHints || []).length,
      editors: st.editors.filter((e) => e.mcp || e.id !== st.myId).map((e) => ({ name: e.name, tool: e.tool, mcp: !!e.mcp, colour: editorColour(e.id) })),
    };
  }

  return {
    get active() { return st.active; },
    setActive(on) { if (on !== st.active) { st.active = on; st.anchor = null; st.drag = null; st.hover = null; } },
    // The map changed under us (a delta, a sync): a selection past its end goes.
    mapChanged() { if (st.sel >= boxes().length) st.sel = -1; },
    key, frame, parts, hud, presence,
    setEditors(list, myId) { st.editors = Array.isArray(list) ? list : []; st.myId = myId; },
    get state() { return st; },
  };
}

// The radar: a top-down picture of the map drawn from its boxes (once per map,
// into an offscreen canvas), with you in the middle, teammates always, enemies
// only while your team spots them (the server's "spot" list in each snapshot,
// lingering a moment after), the Big Diff and the A/B sites. The small one sits
// top-left and turns with your view (cl_radar_rotate); holding the scoreboard
// shows the whole map, north up.
import { rgbHex } from "../gen/color.js";
import { PALETTE as P } from "../assets/palette.js";
import { team as teamInfo } from "../assets/teams.js";

const PX_PER_M = 8, LINGER_MS = 1200;

// Pure: the map's footprint and its boxes in drawing order (low to high).
export function radarLayout(map) {
  const boxes = (map?.boxes || []).filter((b) => b.max[1] > -0.5);
  if (!boxes.length) return null;
  let x0 = Infinity, z0 = Infinity, x1 = -Infinity, z1 = -Infinity;
  for (const b of boxes) { x0 = Math.min(x0, b.min[0]); z0 = Math.min(z0, b.min[2]); x1 = Math.max(x1, b.max[0]); z1 = Math.max(z1, b.max[2]); }
  const order = [...boxes].sort((a, b) => a.max[1] - b.max[1]);
  return { x0, z0, x1, z1, w: x1 - x0, d: z1 - z0, order };
}

// Pure: a box's shade by height: floor dark, low cover mid, walls light.
export function radarShade(top) {
  if (top <= 0.15) return 0.16;
  return Math.min(0.85, 0.36 + top * 0.12);
}

// Pure: world (x, z) to radar pixels around (cx, cy), you at `me`, turned by yaw when rotating.
export function toRadar(x, z, me, yaw, rotate, pxPerM, cx, cy) {
  let dx = (x - me[0]) * pxPerM, dz = (z - me[2]) * pxPerM;
  if (rotate) { const c = Math.cos(yaw), s = Math.sin(yaw); [dx, dz] = [dx * c - dz * s, dx * s + dz * c]; }
  return [cx + dx, cy + dz];
}

const css = (rgb, a = 1) => `rgba(${rgb.map((v) => Math.round(v * 255)).join(",")},${a})`;

export function createRadar() {
  const small = document.createElement("canvas"), big = document.createElement("canvas");
  small.className = "radar"; big.className = "radar-big";
  small.width = small.height = 220; big.width = big.height = 440;
  document.getElementById("hud")?.before(small, big);
  let bg = null, layout = null, mapName = "";
  const lastSeen = new Map(); // enemy id -> {p, at}

  function prerender(map) {
    layout = radarLayout(map);
    mapName = map?.name || "";
    if (!layout) { bg = null; return; }
    bg = document.createElement("canvas");
    bg.width = Math.ceil(layout.w * PX_PER_M); bg.height = Math.ceil(layout.d * PX_PER_M);
    const g = bg.getContext("2d");
    const px = (x) => (x - layout.x0) * PX_PER_M, pz = (z) => (z - layout.z0) * PX_PER_M;
    for (const b of layout.order) {
      const v = radarShade(b.max[1]);
      g.fillStyle = css([v, v * 1.02, v * 1.08]);
      g.fillRect(px(b.min[0]), pz(b.min[2]), (b.max[0] - b.min[0]) * PX_PER_M, (b.max[2] - b.min[2]) * PX_PER_M);
      if (b.max[1] > 0.15) { g.strokeStyle = "rgba(0,0,0,0.35)"; g.lineWidth = 1; g.strokeRect(px(b.min[0]) + 0.5, pz(b.min[2]) + 0.5, (b.max[0] - b.min[0]) * PX_PER_M - 1, (b.max[2] - b.min[2]) * PX_PER_M - 1); }
    }
    for (const [t, zones] of Object.entries(map.buyZones || {})) {
      g.fillStyle = css(teamInfo(t)?.rgb || [1, 1, 1], 0.12);
      for (const z of zones) g.fillRect(px(z.min[0]), pz(z.min[2]), (z.max[0] - z.min[0]) * PX_PER_M, (z.max[2] - z.min[2]) * PX_PER_M);
    }
    for (const s of map.sites || []) {
      g.fillStyle = css(P.hazard, 0.16); g.strokeStyle = css(P.hazard, 0.7); g.lineWidth = 2;
      const w = (s.max[0] - s.min[0]) * PX_PER_M, d = (s.max[2] - s.min[2]) * PX_PER_M;
      g.fillRect(px(s.min[0]), pz(s.min[2]), w, d); g.strokeRect(px(s.min[0]), pz(s.min[2]), w, d);
    }
  }

  // st: {map, me: {pos, yaw} | null, team, players: [{id, team, p, yaw, dead}], spot: Set(ids), bomb, gpus (GPU Drop: round.gpus), rotate, scale, enabled, full, now}
  function draw(st) {
    if (st.map && st.map.name !== mapName) prerender(st.map);
    small.style.display = st.enabled && !st.full && layout ? "block" : "none";
    big.style.display = st.full && layout ? "block" : "none";
    if (!layout) return;
    for (const p of st.players) if (st.spot.has(p.id) && !p.dead) lastSeen.set(p.id, { p: p.p, at: st.now });
    for (const [id, v] of lastSeen) if (st.now - v.at > LINGER_MS) lastSeen.delete(id);
    if (st.full) drawOn(big, st, true);
    else if (st.enabled) drawOn(small, st, false);
  }

  function drawOn(cv, st, full) {
    const g = cv.getContext("2d"), W = cv.width, cx = W / 2, cy = W / 2;
    g.clearRect(0, 0, W, W);
    g.save();
    if (!full) { g.beginPath(); g.arc(cx, cy, cx - 2, 0, Math.PI * 2); g.clip(); g.fillStyle = "rgba(8,10,14,0.75)"; g.fillRect(0, 0, W, W); }
    // Full: the whole map fitted, north up, centred on the map. Small: around you.
    const pxPerM = full ? (W - 24) / Math.max(layout.w, layout.d) : W / 2 / (st.scale || 30);
    const centre = full || !st.me ? [(layout.x0 + layout.x1) / 2, 0, (layout.z0 + layout.z1) / 2] : st.me.pos;
    const rot = !full && st.rotate && st.me ? st.me.yaw : 0;
    const at = (x, z) => toRadar(x, z, centre, rot, !!rot, pxPerM, cx, cy);
    g.save();
    g.translate(cx, cy); g.rotate(rot); g.scale(pxPerM / PX_PER_M, pxPerM / PX_PER_M);
    g.translate((layout.x0 - centre[0]) * PX_PER_M, (layout.z0 - centre[2]) * PX_PER_M);
    g.globalAlpha = 0.92; g.drawImage(bg, 0, 0); g.globalAlpha = 1;
    g.restore();
    // Site letters stay upright.
    g.font = `bold ${full ? 22 : 15}px ui-monospace, monospace`; g.textAlign = "center"; g.textBaseline = "middle";
    for (const s of st.map.sites || []) {
      const [x, y] = at((s.min[0] + s.max[0]) / 2, (s.min[2] + s.max[2]) / 2);
      g.fillStyle = "#000a"; g.fillText(s.name, x + 1, y + 1); g.fillStyle = rgbHex(P.hazard); g.fillText(s.name, x, y);
    }
    const dot = (p, color, r, yaw, ring) => {
      const [x, y] = at(p[0], p[2]);
      if (yaw !== undefined) { // a wedge pointing where they look
        const a = yaw - rot, fx = -Math.sin(a), fy = -Math.cos(a);
        g.beginPath(); g.moveTo(x + fx * r * 2.1, y + fy * r * 2.1); g.lineTo(x - fy * r, y + fx * r); g.lineTo(x + fy * r, y - fx * r); g.closePath();
        g.fillStyle = color; g.fill();
      }
      g.beginPath(); g.arc(x, y, r, 0, Math.PI * 2); g.fillStyle = color; g.fill();
      g.lineWidth = 1.5; g.strokeStyle = ring || "#000c"; g.stroke();
    };
    const spectator = !st.team || st.team === "spec";
    const g2 = (x, y, s, color) => { g.fillStyle = color; g.strokeStyle = "#000"; g.lineWidth = 1.5; g.fillRect(x - s / 2, y - s / 2, s, s); g.strokeRect(x - s / 2, y - s / 2, s, s); };
    for (const p of st.players) {
      if (p.dead || p.team === "spec") continue;
      const col = teamInfo(p.team)?.color || "#fff";
      if (p.team === st.team || spectator) dot(p.p, col, full ? 6 : 4.5, p.yaw);
    }
    if (!spectator) for (const [id, v] of lastSeen) {
      const fresh = st.spot.has(id);
      g.globalAlpha = fresh ? 1 : 0.45;
      dot(v.p, "#ff3030", full ? 6 : 4.5, undefined, "#fff8");
      g.globalAlpha = 1;
    }
    const b = st.bomb;
    if (b && b.st && b.p && (spectator || st.team === "vibe" || b.st === "planted")) {
      const [x, y] = at(b.p[0], b.p[2]), s = full ? 9 : 6;
      g.fillStyle = b.st === "planted" ? (Math.floor(st.now / 300) % 2 ? "#ff3030" : rgbHex(P.neonLime)) : rgbHex(P.neonLime);
      g.strokeStyle = "#000"; g.lineWidth = 1.5;
      g.fillRect(x - s / 2, y - s / 2 - (b.st === "carried" ? 9 : 0), s, s); g.strokeRect(x - s / 2, y - s / 2 - (b.st === "carried" ? 9 : 0), s, s);
    }
    // GPU Drop: every boxed GPU still in play (the server keeps a carried one's
    // p on its carrier), so the staff see who is running off with one.
    for (const g of st.gpus || []) {
      if (!g.p || g.st === "secured") continue;
      const [x, y] = at(g.p[0], g.p[2]), s = full ? 8 : 5.5, carried = g.st === "carried";
      g2(x, y - (carried ? 9 : 0), s, carried ? "#ff3df0" : g.st === "dropped" ? (Math.floor(st.now / 300) % 2 ? "#ffd24a" : rgbHex(P.neonLime)) : rgbHex(P.neonLime));
    }
    if (st.me) dot(st.me.pos, "#fff", full ? 6 : 5, st.me.yaw, teamInfo(st.team)?.color);
    g.restore();
    if (!full) { g.beginPath(); g.arc(cx, cy, cx - 2, 0, Math.PI * 2); g.lineWidth = 2; g.strokeStyle = "#fff4"; g.stroke(); }
    else { g.font = "13px ui-monospace, monospace"; g.fillStyle = "#fffa"; g.textAlign = "left"; g.fillText(mapName, 8, 14); }
  }

  return { draw, get seen() { return [...lastSeen.keys()]; } };
}

// A mesh's triangles sorted into xz columns (pure, no GL), so a pass that only needs
// the geometry near a point (a dynamic light's cube shadow: engine/gl.js) draws a
// few index ranges instead of the whole map. A triangle goes into every column its
// bounds touch (a big floor sits in many: the depth pass draws it more than once
// where ranges overlap, harmless), columns laid out row by row (z, then x) so the
// columns of one row that a sphere touches are one contiguous range.

// pos: vertex positions (xyz), idx: triangle indices; cell: column side (metres).
// Returns {idx (the reordered indices), x0, z0, nx, nz, cell, start, count, ylo, yhi}
// (per column: its first index, index count and the triangles' height span).
export function chunkMesh(pos, idx, cell = 4) {
  let x0 = Infinity, z0 = Infinity, x1 = -Infinity, z1 = -Infinity;
  for (let v = 0; v < pos.length; v += 3) {
    x0 = Math.min(x0, pos[v]); x1 = Math.max(x1, pos[v]);
    z0 = Math.min(z0, pos[v + 2]); z1 = Math.max(z1, pos[v + 2]);
  }
  if (!(x1 >= x0)) x0 = z0 = x1 = z1 = 0;
  const nx = Math.max(1, Math.ceil((x1 - x0) / cell) + 1), nz = Math.max(1, Math.ceil((z1 - z0) / cell) + 1);
  const n = nx * nz, count = new Uint32Array(n), ylo = new Float32Array(n).fill(Infinity), yhi = new Float32Array(n).fill(-Infinity);
  const colX = (x) => Math.min(Math.floor((x - x0) / cell), nx - 1), colZ = (z) => Math.min(Math.floor((z - z0) / cell), nz - 1);
  // Each triangle's column span, two passes: count, then fill.
  const span = (t, f) => {
    let ax = Infinity, bx = -Infinity, az = Infinity, bz = -Infinity, ay = Infinity, by = -Infinity;
    for (let k = 0; k < 3; k++) {
      const v = idx[t + k] * 3;
      ax = Math.min(ax, pos[v]); bx = Math.max(bx, pos[v]);
      ay = Math.min(ay, pos[v + 1]); by = Math.max(by, pos[v + 1]);
      az = Math.min(az, pos[v + 2]); bz = Math.max(bz, pos[v + 2]);
    }
    const cx1 = colX(bx), cz1 = colZ(bz);
    for (let cz = colZ(az); cz <= cz1; cz++) for (let cx = colX(ax); cx <= cx1; cx++) f(cz * nx + cx, ay, by);
  };
  for (let t = 0; t + 2 < idx.length; t += 3) span(t, (c, ay, by) => { count[c] += 3; ylo[c] = Math.min(ylo[c], ay); yhi[c] = Math.max(yhi[c], by); });
  const start = new Uint32Array(n);
  let total = 0;
  for (let c = 0; c < n; c++) { start[c] = total; total += count[c]; }
  const out = new Uint32Array(total), fill = start.slice();
  for (let t = 0; t + 2 < idx.length; t += 3) span(t, (c) => { out[fill[c]++] = idx[t]; out[fill[c]++] = idx[t + 1]; out[fill[c]++] = idx[t + 2]; });
  return { idx: out, x0, z0, nx, nz, cell, start, count, ylo, yhi };
}

// The index ranges of ch (chunkMesh) whose columns a sphere (p, r) touches, adjacent
// ones merged: [offset, count, offset, count, ...] in indices.
export function chunkRanges(ch, p, r, out = []) {
  out.length = 0;
  const { x0, z0, nx, nz, cell } = ch;
  const ax = Math.max(0, Math.floor((p[0] - r - x0) / cell)), bx = Math.min(nx - 1, Math.floor((p[0] + r - x0) / cell));
  const az = Math.max(0, Math.floor((p[2] - r - z0) / cell)), bz = Math.min(nz - 1, Math.floor((p[2] + r - z0) / cell));
  for (let cz = az; cz <= bz; cz++) {
    for (let cx = ax; cx <= bx; cx++) {
      const c = cz * nx + cx;
      if (!ch.count[c]) continue;
      // The column's box (its triangles' height span) against the sphere.
      const qx = Math.max(x0 + cx * cell, Math.min(p[0], x0 + (cx + 1) * cell));
      const qz = Math.max(z0 + cz * cell, Math.min(p[2], z0 + (cz + 1) * cell));
      const qy = Math.max(ch.ylo[c], Math.min(p[1], ch.yhi[c]));
      if ((qx - p[0]) ** 2 + (qy - p[1]) ** 2 + (qz - p[2]) ** 2 > r * r) continue;
      const k = out.length;
      if (k && out[k - 2] + out[k - 1] === ch.start[c]) out[k - 1] += ch.count[c];
      else out.push(ch.start[c], ch.count[c]);
    }
  }
  return out;
}

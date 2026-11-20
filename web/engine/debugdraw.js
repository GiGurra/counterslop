// Debug drawing with the renderer's solid cubes: wireframe boxes made of 12 thin, glowing edges.
import { chain, translate, scale } from "./math.js";

export function wireBox(mn, mx, color, t = 0.025) {
  const out = [], c = [(mn[0] + mx[0]) / 2, (mn[1] + mx[1]) / 2, (mn[2] + mx[2]) / 2];
  const s = [mx[0] - mn[0], mx[1] - mn[1], mx[2] - mn[2]];
  for (let ax = 0; ax < 3; ax++) {
    const u = (ax + 1) % 3, v = (ax + 2) % 3;
    for (const su of [-1, 1]) for (const sv of [-1, 1]) {
      const p = [...c], sz = [t, t, t];
      p[u] += su * s[u] / 2; p[v] += sv * s[v] / 2; sz[ax] = s[ax] + t;
      out.push({ m: chain(translate(...p), scale(...sz)), color, emit: 1 });
    }
  }
  return out;
}

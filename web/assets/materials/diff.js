// A git branch as a bridge (he_merge_hell): a diff, line after line across
// the deck: removed lines red with a "-" in the gutter, added ones green with
// a "+", context lines grey; the sides are the diff viewer's dark chrome.
export default {
  name: "diff",
  glsl: `vec3 mat_diff(Surf s){
    vec3 col = vec3(0.13, 0.14, 0.17);
    if (!s.top) return col * (0.8 + 0.2*aaPulse(s.uv.x*2.0, 0.5, 1.0)); // antialiased (aaStep & co, gen/glsl/patterns.js): MSAA leaves lines drawn inside a face jagged
    vec2 u = s.sz.x >= s.sz.y ? s.uv : s.uv.yx; // along the bridge, across it
    float line = floor(u.x / 0.6);
    float h = hash(vec2(line, floor(s.world.y) + 3.0));
    vec3 tint = h < 0.4 ? PAL_DIFF_DEL : (h < 0.8 ? PAL_DIFF_ADD : vec3(0.22, 0.23, 0.27));
    col = tint * (0.85 + 0.15*fbm(s.wuv*3.0));
    float gap = aaPulse(u.x / 0.6, 0.54, 1.0);               // the gap between lines
    col = mix(col, vec3(0.1), gap);
    // the gutter's sign near one edge: - (one bar) or + (two)
    vec2 g = vec2(fract(u.x / 0.6) * 0.6, u.y);
    // (g.x = 0.27 +- w as a band of fract(u.x/0.6): aaPulse, no blur at the wrap)
    float bar = aaPulse(u.x / 0.6, 0.23/0.6, 0.31/0.6) * aaLine(g.y - 0.5, 0.2);
    float up = aaLine(g.y - 0.5, 0.04) * aaPulse(u.x / 0.6, 0.07/0.6, 0.47/0.6);
    float mark = h < 0.4 ? bar : (h < 0.8 ? max(bar, up) : 0.0);
    col = mix(col, vec3(0.95), mark);
    // code: short pale dashes along the line
    float code = step(0.6, hash(vec2(floor(u.y*3.0), line))) * aaPulse(u.x / 0.6, 0.22/0.6, 0.32/0.6) * aaStep(0.9, u.y);
    return mix(col, col*1.6, code*0.5);
  }`,
};

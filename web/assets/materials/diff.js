// A git branch as a bridge (he_merge_hell): a diff, line after line across
// the deck: removed lines red with a "-" in the gutter, added ones green with
// a "+", context lines grey; the sides are the diff viewer's dark chrome.
export default {
  name: "diff",
  glsl: `vec3 mat_diff(Surf s){
    vec3 col = vec3(0.13, 0.14, 0.17);
    if (!s.top) return col * (0.8 + 0.2*step(0.5, fract(s.uv.x*2.0)));
    vec2 u = s.sz.x >= s.sz.y ? s.uv : s.uv.yx; // along the bridge, across it
    float line = floor(u.x / 0.6);
    float h = hash(vec2(line, floor(s.world.y) + 3.0));
    vec3 tint = h < 0.4 ? PAL_DIFF_DEL : (h < 0.8 ? PAL_DIFF_ADD : vec3(0.22, 0.23, 0.27));
    col = tint * (0.85 + 0.15*fbm(s.wuv*3.0));
    float gap = step(0.54, fract(u.x / 0.6));               // the gap between lines
    col = mix(col, vec3(0.1), gap);
    // the gutter's sign near one edge: - (one bar) or + (two)
    vec2 g = vec2(fract(u.x / 0.6) * 0.6, u.y);
    float bar = step(abs(g.x - 0.27), 0.04) * step(abs(g.y - 0.5), 0.2);
    float up = step(abs(g.y - 0.5), 0.04) * step(abs(g.x - 0.27), 0.2);
    float mark = h < 0.4 ? bar : (h < 0.8 ? max(bar, up) : 0.0);
    col = mix(col, vec3(0.95), mark);
    // code: short pale dashes along the line
    float code = step(0.6, hash(vec2(floor(u.y*3.0), line))) * step(abs(fract(u.x/0.6)*0.6 - 0.27), 0.05) * step(0.9, u.y);
    return mix(col, col*1.6, code*0.5);
  }`,
};

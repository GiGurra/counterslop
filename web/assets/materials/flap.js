// Split-flap tiles (the departure boards' backs, cq_moonshot): rows of dark
// plates 0.3 m apart, each split across its middle by a thin gap, edges a touch
// lighter; tops and thin faces plain.
export default {
  name: "flap",
  glsl: `vec3 mat_flap(Surf s){
    vec3 col = vec3(0.075, 0.075, 0.085) * (0.9 + 0.2*fbm(s.wuv*6.0));
    if (s.top || min(s.sz.x, s.sz.y) < 0.15) return col;
    vec2 p = s.wuv / vec2(0.26, 0.34);
    vec2 f = fract(p);
    float gap = step(f.x, 0.06) + step(f.y, 0.05);
    float split = 1.0 - smoothstep(0.0, 0.025, abs(f.y - 0.52));
    col *= 1.0 - 0.55*min(1.0, gap) - 0.6*split;
    return col + 0.02*step(0.94, f.y);
  }`,
};

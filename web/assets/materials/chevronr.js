// rc_ci_pipeline's corner signs: neon chevrons on a dark board chasing the way
// the road turns (right, as read by the driver facing the board), a
// hazard-yellow frame. chevron.js points left, chevronr.js right.
export default {
  name: "chevronr",
  glsl: `vec3 mat_chevronr(Surf s){
    vec2 r = readUV(s.n, s.uv, s.sz);
    vec3 col = PAL_SIGN_BACK;
    if (s.top || min(s.sz.x, s.sz.y) < 0.3) return col;
    float frame = 1.0 - aaBand(r.y, 0.08, s.sz.y - 0.08);
    float x = (s.sz.x - r.x) / s.sz.y, y = abs(r.y / s.sz.y - 0.5);
    float on = aaPulse(x*1.2 - y*1.0 + uTime*1.6, 0.55, 1.0) * (1.0 - aaStep(0.36, y)); // antialiased (MSAA leaves in-face lines jagged)
    gEmit = clamp(on + frame*0.6, 0.0, 1.0);
    return mix(mix(col, PAL_TOY_YELLOW, frame), PAL_TOY_YELLOW * 1.15 + 0.1, on);
  }`,
};

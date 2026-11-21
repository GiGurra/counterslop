// Poured concrete: 2 m floor tiles with joints, and 3 × 1.5 m wall panels with
// tie holes; fbm stains.
export default {
  name: "concrete",
  glsl: `vec3 mat_concrete(Surf s){
    vec3 col = PAL_CONCRETE * (0.8 + 0.3*fbm(s.wuv*1.3)) * (0.92 + 0.1*fbm(s.wuv*9.0));
    if (s.top) return mix(col, col*0.7, tiles(s.wuv, 2.0, 0.01));
    col = mix(col, col*0.72, max(gridLine(s.uv.x/3.0, 0.006), gridLine(s.uv.y/1.5, 0.012)));
    vec2 hole = fract(s.uv/vec2(0.75, 0.75)) - 0.5;
    return mix(col, col*0.55, 1.0 - smoothstep(0.03, 0.05, length(hole)));
  }`,
};

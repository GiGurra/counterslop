// Market awning cloth: broad sun-faded stripes, cream and one colour per stall
// (picked from where it hangs, so neighbouring stalls differ), a darker
// scalloped hem along the edges, faint weave.
export default {
  name: "awning",
  glsl: `vec3 mat_awning(Surf s){
    float pick = hash(floor(s.world.xz/5.0) + 3.7);
    vec3 dye = pick < 0.34 ? PAL_AWNING_RED : pick < 0.67 ? PAL_AWNING_TEAL : PAL_AWNING_SAFFRON;
    vec2 u = s.top || s.n.y < -0.5 ? s.wuv : s.uv;
    float stripe = step(0.5, fract((s.n.x != 0.0 ? u.y : u.x)/0.5));
    vec3 col = mix(PAL_AWNING_CREAM, dye, stripe) * (0.86 + 0.18*fbm(s.wuv*2.0));
    col *= 0.94 + 0.06*sin(s.wuv.x*90.0)*sin(s.wuv.y*90.0);
    float hem = 1.0 - smoothstep(0.08, 0.12, edgeDist(s.uv, s.sz));
    return mix(col, dye*0.6, hem*0.7);
  }`,
};

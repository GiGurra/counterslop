// Sun-baked lime render on old walls: warm ochre with fbm stains and a dusty
// band at the foot; where the render has flaked off, big sandstone blocks show
// through. Tops are worn bare stone.
export default {
  name: "stucco",
  glsl: `vec3 mat_stucco(Surf s){
    vec2 b = bricks(s.wuv, vec2(1.2, 0.6), 0.025, 0.05);
    vec3 stone = mix(PAL_SANDSTONE_DARK, PAL_SANDSTONE, b.y) * (0.85 + 0.25*fbm(s.wuv*4.0));
    stone = mix(stone, PAL_MORTAR*0.8, b.x);
    if (s.top) return PAL_SANDSTONE * (0.8 + 0.3*fbm(s.wuv*1.7));
    vec3 col = PAL_STUCCO * (0.86 + 0.18*fbm(s.wuv*0.9)) * (0.95 + 0.08*fbm(s.wuv*12.0));
    col = mix(col, col*0.78, smoothstep(0.55, 0.0, s.uv.y) * 0.6);
    float flake = smoothstep(0.60, 0.63, fbm(s.wuv*0.35 + 11.0) + 0.08*fbm(s.wuv*6.0));
    return mix(col, stone, flake);
  }`,
};

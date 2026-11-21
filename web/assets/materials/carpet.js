// Office carpet tiles: 0.5 m squares with alternating pile direction, flecks,
// and the odd coffee stain. Sides (if any) look like the same carpet.
export default {
  name: "carpet",
  glsl: `vec3 mat_carpet(Surf s){
    vec2 t = floor(s.wuv/0.5);
    float pile = mod(t.x + t.y, 2.0);
    vec3 col = PAL_CARPET * (0.9 + 0.12*pile) * (0.9 + 0.2*fbm(s.wuv*3.0));
    col = mix(col, PAL_CARPET_FLECK, smoothstep(0.55, 0.9, fbm(s.wuv*9.0))*0.3);
    col = mix(col, col*0.85, tiles(s.wuv, 0.5, 0.02));
    float stain = smoothstep(0.72, 0.75, fbm(s.wuv*0.25 + 7.0));
    return mix(col, col*vec3(0.75, 0.6, 0.45), stain*0.6);
  }`,
};

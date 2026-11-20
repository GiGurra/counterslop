// Concrete with hazard stripes along the top edge of side faces; tiled tops.
export default {
  name: "hazard",
  glsl: `vec3 mat_hazard(Surf s){
    vec3 col = PAL_CONCRETE * (0.85 + 0.25*fbm(s.wuv*1.5));
    if (s.top) return mix(col, col*0.75, tiles(s.wuv, 1.0, 0.015));
    if (s.sz.y - s.uv.y < 0.35) return mix(PAL_HAZARD_DARK, PAL_HAZARD, stripes(s.uv, 1.6));
    return col;
  }`,
};

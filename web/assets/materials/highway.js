// rc_hotfix_highway's roads: asphalt with a highway's paint. Along a deck's
// longer side: solid white edge lines and a dashed centre line (6 m dashes, 6 m
// gaps), reflective; tar seams and wear. The sides: dark concrete with a
// row of amber marker lights.
export default {
  name: "highway",
  glsl: `vec3 mat_highway(Surf s){
    if (!s.top) {
      vec3 c = vec3(0.16, 0.16, 0.18) * (0.8 + 0.3*fbm(s.wuv*2.0));
      float lamp = aaLine(s.uv.y - s.sz.y*0.5, 0.08) * aaPulse(s.uv.x/4.0, 0.46, 0.54); // antialiased (MSAA leaves in-face lines jagged)
      gEmit = lamp;
      return mix(c, PAL_LED_AMBER*1.2, lamp);
    }
    vec3 col = vec3(0.1, 0.1, 0.115) * (0.75 + 0.45*fbm(s.wuv*2.5)) + 0.04*hash(floor(s.wuv*30.0));
    vec2 u = s.sz.x >= s.sz.y ? s.uv : s.uv.yx; // along the road, across it
    float w = min(s.sz.x, s.sz.y);
    if (w < 9.0) return col; // a pad, a ramp's lip, a narrow bridge: no paint
    float edge = aaLine(u.y - 0.7, 0.08) + aaLine(u.y - (w - 0.7), 0.08);
    float centre = aaLine(u.y - w*0.5, 0.08) * aaPulse(u.x/12.0, 0.0, 0.5);
    float paint = clamp(edge + centre, 0.0, 1.0);
    gEmit = paint * 0.25;
    return mix(col, vec3(0.92, 0.9, 0.82), paint);
  }`,
};

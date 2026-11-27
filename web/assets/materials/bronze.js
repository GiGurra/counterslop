// Cast bronze (the Monument to the Postmortem, prefabs.FacepalmMonument):
// warm metal with polished highlights where hands have rubbed it, verdigris
// settled on the tops and in the creases (box edges), green streaks running
// down the sides where rain would have (in a terminal: the air conditioning).
export default {
  name: "bronze",
  glsl: `vec3 mat_bronze(Surf s){
    float edge = edgeDist(s.uv, s.sz);
    float n = fbm(s.wuv*1.7 + s.world.y*0.3);
    vec3 col = PAL_BRONZE * (0.8 + 0.3*n);
    col = mix(col, PAL_BRONZE*1.6, smoothstep(0.62, 0.8, fbm(s.wuv*0.5 + 3.1)) * 0.6); // rubbed to a shine
    float crease = 1.0 - smoothstep(0.0, 0.06, edge);
    float green = smoothstep(0.5, 0.78, n + (s.top ? 0.28 : 0.0) + 0.35*crease);
    if (!s.top) green = max(green, 0.7*smoothstep(0.62, 0.86, fbm(vec2(s.wuv.x*7.0, s.world.y*0.35)))); // streaks
    col = mix(col, PAL_VERDIGRIS*(0.85 + 0.2*n), green*0.8);
    return col * mix(0.72, 1.0, smoothstep(0.0, 0.03, edge));
  }`,
};

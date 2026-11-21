// Glass balustrades and panes: opaque for now (the renderer has no transparency yet), so
// it fakes glass: a dark teal tint, the sky reflected at grazing angles, diagonal
// reflection streaks that slide as you move, and a brushed steel frame. A thin top
// face is the black rubber handrail.
export default {
  name: "glass",
  glsl: `vec3 mat_glass(Surf s){
    float edge = edgeDist(s.uv, s.sz);
    if (s.top) return min(s.sz.x, s.sz.y) < 0.15 ? vec3(0.04) : PAL_SHELF_METAL;
    if (edge < 0.035) return PAL_SHELF_METAL * (0.85 + 0.1*fbm(s.uv*20.0));
    vec3 v = normalize(uEye - s.world);
    float fres = pow(1.0 - abs(dot(v, s.n)), 3.0);
    float streak = smoothstep(0.85, 1.0, sin((s.wuv.x + s.wuv.y)*3.0 + dot(v, vec3(4.0, 2.0, 4.0))));
    vec3 col = mix(PAL_GLASS_TINT, PAL_HEMI_SKY*1.1, 0.25 + 0.6*fres) + vec3(0.25)*streak;
    gEmit = 0.25 + 0.3*fres;
    return col;
  }`,
};

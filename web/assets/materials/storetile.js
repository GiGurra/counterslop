// Polished superstore floor: big 0.6 m off-white tiles with grey grout, a darker
// "aisle path" tile now and then, and a fake reflection: the ceiling's fluorescent
// strips (every 3 m along x) mirrored in the polish, sliding with the view, plus a
// sky-tinted sheen at grazing angles. Sides are a plain grey skirting.
export default {
  name: "storetile",
  glsl: `vec3 mat_storetile(Surf s){
    if (!s.top) return PAL_GROUT * (0.8 + 0.1*fbm(s.wuv*4.0));
    vec2 t = floor(s.wuv/0.6);
    float h = hash(t);
    vec3 col = (h > 0.93 ? PAL_STORE_TILE_ACCENT : PAL_STORE_TILE) * (0.95 + 0.06*h) * (0.96 + 0.06*fbm(s.wuv*1.7));
    col = mix(col, PAL_GROUT, tiles(s.wuv, 0.6, 0.012));
    // Reflection: the eye ray mirrored off the floor, hitting a ceiling 4 m up.
    vec3 v = normalize(uEye - s.world);
    vec3 r = reflect(-v, vec3(0.0, 1.0, 0.0));
    float k = 4.0 / max(r.y, 0.05);
    vec2 hit = s.world.xz + r.xz * k;
    float strip = 1.0 - smoothstep(0.12, 0.3, abs(fract(hit.x/3.0) - 0.5) * 3.0);
    strip *= 1.0 - smoothstep(0.35, 0.5, abs(fract(hit.y/2.4) - 0.5)); // tubes end now and then
    float fres = pow(1.0 - max(v.y, 0.0), 4.0);
    float blur = 0.6 + 0.4*fbm(s.wuv*0.7); // scuffed polish
    col = mix(col, PAL_HEMI_SKY, fres*0.35);
    col += vec3(0.9, 0.95, 1.0) * strip * 0.35 * blur * (0.3 + 0.7*fres);
    gEmit = strip * 0.35 * blur;
    return col;
  }`,
};

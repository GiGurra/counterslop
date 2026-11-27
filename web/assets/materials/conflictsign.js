// rc_merge_train's boards: a dark panel showing a merge conflict's three
// markers stacked, <<<<<<< over ======= over >>>>>>>, in glowing amber (the
// top and bottom rows chase: ours this way, theirs the other).
export default {
  name: "conflictsign",
  glsl: `vec3 mat_conflictsign(Surf s){
    vec3 col = PAL_SIGN_BACK;
    if (s.top || min(s.sz.x, s.sz.y) < 0.3) return col;
    vec2 r = readUV(s.n, s.uv, s.sz);
    float y = r.y / s.sz.y, row = floor(y*3.0), fy = fract(y*3.0) - 0.5;
    float cell = s.sz.y / 3.0 * 0.8;
    float gx = fract(r.x / cell + (row == 2.0 ? uTime : row == 0.0 ? -uTime : 0.0) * 0.8);
    float on;
    if (row == 1.0) on = step(abs(abs(fy) - 0.13), 0.06) * step(0.12, gx) * step(gx, 0.92);
    else if (row == 2.0) on = step(abs(abs(fy)*1.5 - (gx - 0.2)), 0.1) * step(abs(fy), 0.32); // <
    else on = step(abs(abs(fy)*1.5 - (0.8 - gx)), 0.1) * step(abs(fy), 0.32);                // >
    on *= step(0.04, r.x) * step(r.x, s.sz.x - 0.04);
    vec3 amber = mix(PAL_TOY_YELLOW, PAL_SUN_GLOW, 0.4);
    gEmit = on;
    return mix(col, amber*1.2 + 0.1, on);
  }`,
};

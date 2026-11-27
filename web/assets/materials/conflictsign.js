// rc_merge_train's boards: a dark panel showing a merge conflict's three
// markers stacked, <<<<<<< over ======= over >>>>>>>, in glowing amber (the
// top and bottom rows chase: ours this way, theirs the other).
export default {
  name: "conflictsign",
  glsl: `vec3 mat_conflictsign(Surf s){
    vec3 col = PAL_SIGN_BACK;
    if (s.top || min(s.sz.x, s.sz.y) < 0.3) return col;
    vec2 r = readUV(s.n, s.uv, s.sz);
    float y = r.y / s.sz.y, row = floor(y*3.0), ay = abs(fract(y*3.0) - 0.5);
    float cell = s.sz.y / 3.0 * 0.8, t = r.x / cell;
    // antialiased (aaStep & co, gen/glsl/patterns.js): MSAA leaves lines drawn inside a face jagged: all three rows'
    // glyphs worked out first (derivatives need uniform flow), the row picks one. The
    // chevrons as repeating bands of t -+ 1.5*|fy| (|fy|: continuous across the rows).
    float eq = aaBand(ay, 0.07, 0.19) * aaPulse(t, 0.12, 0.92);                              // =
    float lt = aaPulse(t + uTime*0.8 - 1.5*ay, 0.1, 0.3) * (1.0 - aaStep(0.32, ay));          // <
    float gt = aaPulse(t - uTime*0.8 + 1.5*ay, 0.7, 0.9) * (1.0 - aaStep(0.32, ay));          // >
    float on = row == 1.0 ? eq : row == 2.0 ? lt : gt;
    on *= aaBand(r.x, 0.04, s.sz.x - 0.04);
    vec3 amber = mix(PAL_TOY_YELLOW, PAL_SUN_GLOW, 0.4);
    gEmit = on;
    return mix(col, amber*1.2 + 0.1, on);
  }`,
};

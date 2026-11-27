// The conflict sea far under rc_merge_train: molten merge conflicts. A slow,
// glowing orange-red melt crusted dark at the cool edges, with rows of
// conflict markers drifting through it: <<<<<<< chevrons, ======= bars and
// >>>>>>> chevrons, burning bright. Read from above (world xz): no seams.
export default {
  name: "conflict",
  glsl: `vec3 mat_conflict(Surf s){
    vec2 p = s.wuv;
    float n = fbm(p*0.04 + vec2(uTime*0.02, uTime*0.015));
    float m = fbm(p*0.11 - vec2(uTime*0.03, 0.0) + n*2.0);
    float heat = smoothstep(0.35, 0.75, m);
    vec3 col = mix(vec3(0.06, 0.02, 0.02), mix(PAL_SUN_GLOW*0.9, PAL_SUN_CORE, smoothstep(0.65, 0.95, m)), heat);
    // the markers: rows 12 m apart drifting along x, each row one kind, 7 glyphs a block
    float row = floor(p.y/12.0), fy = fract(p.y/12.0) - 0.5;
    float x = p.x + uTime*(1.5 + hash(vec2(row, 3.0))*2.0);
    float blk = floor(x/40.0), gx = fract(x/2.0), inBlk = step(fract(x/40.0)*40.0, 14.0);
    float kind = floor(hash(vec2(row, blk))*3.0); // 0 <<<, 1 ===, 2 >>>
    float on = 0.0;
    if (kind < 0.5) on = step(abs(abs(fy)*1.6 - (gx - 0.15)), 0.12) * step(abs(fy), 0.3);
    else if (kind < 1.5) on = step(abs(abs(fy) - 0.12), 0.06) * step(0.15, gx) * step(gx, 0.95);
    else on = step(abs(abs(fy)*1.6 - (0.85 - gx)), 0.12) * step(abs(fy), 0.3);
    on *= inBlk * step(0.35, hash(vec2(row, blk + 7.0)));
    col = mix(col, vec3(1.0, 0.95, 0.75), on);
    gEmit = clamp(heat*0.9 + on, 0.0, 1.0);
    return col;
  }`,
};

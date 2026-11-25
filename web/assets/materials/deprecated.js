// The void under surf_scope_creep: an endless plain of deprecated code, far below.
// Dim lines of code in a dark editor (indents, tokens), most of them struck
// through in red, the odd yellow warning squiggle, and the faint glow of a
// screen nobody looks at any more. Read from above (world xz), so it tiles
// without seams under every ramp.
export default {
  name: "deprecated",
  glsl: `vec3 mat_deprecated(Surf s){
    vec2 p = s.wuv / 0.9;
    float ln = floor(p.y), fy = fract(p.y), blk = floor(p.x/30.0);
    float indent = floor(hash(vec2(ln, blk))*5.0)*1.6, len = 6.0 + 18.0*hash(vec2(blk, ln + 3.0));
    float x = p.x - blk*30.0;
    float tok = hash(vec2(floor(x/1.4), ln + blk*17.0));
    float on = step(indent, x) * step(x, indent + len) * step(0.3, fy) * step(fy, 0.7) * step(0.15, fract(x/1.4));
    vec3 ink = mix(vec3(0.35, 0.4, 0.5), tok < 0.3 ? PAL_NEON_CYAN*0.5 : tok < 0.55 ? PAL_NEON_MAGENTA*0.45 : vec3(0.45, 0.48, 0.55), 0.6);
    vec3 col = mix(vec3(0.035, 0.035, 0.05), ink, on);
    float dead = step(0.3, hash(vec2(ln*1.7, blk + 5.0)));
    float strike = dead * step(indent, x) * step(x, indent + len) * (1.0 - smoothstep(0.02, 0.06, abs(fy - 0.5)));
    col = mix(col, PAL_REVIEW_RED, strike);
    float warn = (1.0 - dead) * step(0.7, hash(vec2(ln, blk + 9.0))) * step(indent, x) * step(x, indent + len)
      * (1.0 - smoothstep(0.02, 0.05, abs(fy - 0.2 - 0.04*sin(x*12.0))));
    col = mix(col, PAL_TOY_YELLOW, warn);
    gEmit = 0.7;
    return col * (0.85 + 0.15*fbm(s.wuv*0.05));
  }`,
};

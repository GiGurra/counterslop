// Padded approval walls (he_merge_hell): quilted green padding, a big ✓ in
// every panel, LGTM stamps; grenades bounce straight back off them (the
// server's lgtmBounce). Seen it, approved it, didn't read it.
export default {
  name: "lgtm",
  glsl: `vec3 mat_lgtm(Surf s){
    vec2 cell = fract(s.uv / 1.5) - 0.5;
    vec3 col = PAL_LGTM * (0.9 + 0.1*fbm(s.wuv*2.0));
    // quilting: the panel bulges, darker at its seams
    float seam = max(abs(cell.x), abs(cell.y));
    col *= 1.05 - 0.45*smoothstep(0.38, 0.5, seam);
    // the button in each quilt corner
    col = mix(col, col*0.5, 1.0 - smoothstep(0.03, 0.05, length(abs(cell) - 0.5)));
    // a check mark in every other panel
    vec2 p = cell * vec2(1.0, -1.0);
    // its strokes antialiased (gen/glsl/patterns.js): MSAA leaves in-face lines jagged
    float a = aaLine(p.y - (-p.x - 0.05), 0.06) * aaBand(p.x, -0.22, -0.05);
    float b = aaLine(p.y - (p.x*1.4 - 0.12), 0.06) * aaBand(p.x, -0.07, 0.25);
    float on = step(0.5, hash(floor(s.uv / 1.5)));
    col = mix(col, vec3(0.97), max(a, b) * on);
    gEmit = max(a, b) * on * 0.4;
    return col;
  }`,
};

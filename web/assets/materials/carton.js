// Cardboard stock cartons (the stockroom): a big box reads as a stacked pallet of
// cartons, 0.6 x 0.4 m in world space, with seams, a packing-tape strip, a white label
// with a barcode and a "9090", and the odd stencilled arrow. Tops show the tape cross.
export default {
  name: "carton",
  glsl: `vec3 mat_carton(Surf s){
    vec2 c = s.top ? s.wuv/vec2(0.6, 0.6) : vec2(s.uv.x/0.6, s.uv.y/0.4);
    vec2 id = floor(c), f = fract(c);
    float h = hash(id + faceHash(s.world, s.n, s.uv, 1.0)*53.0);
    vec3 col = PAL_CARDBOARD * (0.85 + 0.2*h) * (0.9 + 0.15*fbm(s.wuv*vec2(30.0, 3.0)));
    if (s.top) {
      float tape = 1.0 - smoothstep(0.06, 0.08, abs(f.x - 0.5));
      col = mix(col, PAL_TAPE * 1.1, tape*0.85);
    } else {
      float tape = 1.0 - smoothstep(0.08, 0.1, abs(f.y - 0.93)*4.0);
      col = mix(col, PAL_TAPE*1.1, tape*0.7);
      vec2 l = (f - vec2(0.3, 0.45)) / vec2(0.18, 0.2);
      // lines antialiased (gen/glsl/patterns.js); the barcode's bars are per-cell picks
      float label = aaLine(l.x, 1.0) * aaLine(l.y, 1.0) * step(0.25, h);
      float bars = step(0.5, hash(vec2(floor(f.x*120.0), id.x))) * (1.0 - aaStep(-0.1, l.y));
      vec3 lc = mix(vec3(0.93), vec3(0.08), bars);
      lc = mix(lc, vec3(0.08), aaBand(l.y, 0.2, 0.8) * aaBand(l.x, -0.8, 0.4) * aaPulse(c.x*60.0, 0.5, 1.0));
      col = mix(col, lc, label);
      vec2 a = (f - vec2(0.78, 0.5)) / 0.12; // a "this side up" arrow
      float arrow = aaLine(a.x, 0.25) * aaLine(a.y + 0.3, 0.6) + aaStep(0.0, 0.9 - (a.y - 0.3) - abs(a.x)) * aaBand(a.y, 0.3, 1.0);
      col = mix(col, vec3(0.1), clamp(arrow, 0.0, 1.0) * step(0.6, h) * 0.8);
    }
    float seam = max(1.0 - smoothstep(0.0, 0.025, min(f.x, 1.0 - f.x)), 1.0 - smoothstep(0.0, 0.025, min(f.y, 1.0 - f.y)));
    return mix(col, col*0.45, seam);
  }`,
};

// de_serverless's jump pads (content.Launcher with an Apex): on top, neon
// rings pulsing out from the middle over a dark plate, a bright core; the
// sides a dark rim with a glowing band.
export default {
  name: "jumppad",
  glsl: `vec3 mat_jumppad(Surf s){
    vec2 r = readUV(s.n, s.uv, s.sz);
    if (!s.top) {
      float band = aaLine(r.y / max(s.sz.y, 0.01) - 0.5, 0.25); // antialiased (MSAA leaves in-face lines jagged)
      gEmit = band * 0.9;
      return mix(vec3(0.06, 0.07, 0.1), PAL_NEON_CYAN, band);
    }
    vec2 c = r - s.sz * 0.5;
    float d = length(c) / (0.5 * min(s.sz.x, s.sz.y));
    float ring = smoothstep(0.35, 0.5, fract(d * 2.5 - uTime * 1.8)) * (1.0 - smoothstep(0.5, 0.65, fract(d * 2.5 - uTime * 1.8)));
    float core = 1.0 - smoothstep(0.12, 0.22, d);
    float edge = aaStep(0.92, d);
    float on = clamp(ring * (1.0 - aaStep(1.0, d)) + core + edge * 0.7, 0.0, 1.0);
    gEmit = on;
    vec3 glow = mix(PAL_NEON_CYAN, PAL_NEON_MAGENTA, smoothstep(0.2, 1.0, d));
    return mix(vec3(0.05, 0.06, 0.09), glow * 1.1, on);
  }`,
};

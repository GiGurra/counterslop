// rc_ci_pipeline's roads: dark tarmac with a fine aggregate and, sunk in it, faint
// circuit traces that pulse now and then (the pipeline's data running under the
// wheels). A road's sides: a dark deck edge with a neon cyan underglow strip.
export default {
  name: "tarmac",
  glsl: `vec3 mat_tarmac(Surf s){
    vec3 base = vec3(0.07, 0.075, 0.09);
    if (!s.top) {
      float strip = aaBand(s.uv.y, 0.04, 0.12); // antialiased (MSAA leaves in-face lines jagged)
      gEmit = strip * 0.9;
      return mix(base * (0.8 + 0.3*fbm(s.wuv*2.0)), PAL_NEON_CYAN, strip);
    }
    vec3 col = base * (0.75 + 0.5*fbm(s.wuv*3.0)) + 0.05*hash(floor(s.wuv*40.0));
    // traces: a 2 m grid, each cell lit along one axis by its hash, a pulse running along
    vec2 cell = floor(s.wuv/2.0), f = fract(s.wuv/2.0);
    float hx = hash(cell + 3.0), hy = hash(cell + 31.0);
    float tr = step(0.6, hx) * (1.0 - smoothstep(0.01, 0.025, abs(f.y - 0.5)))
             + step(0.7, hy) * (1.0 - smoothstep(0.01, 0.025, abs(f.x - 0.5)));
    float pulse = 0.25 + 0.75*pow(0.5 + 0.5*sin(uTime*2.0 - (s.wuv.x + s.wuv.y)*0.15 + hx*6.0), 8.0);
    tr = clamp(tr, 0.0, 1.0);
    gEmit = tr * pulse * 0.6;
    return mix(col, PAL_NEON_CYAN * 0.6, tr * pulse * 0.7);
  }`,
};

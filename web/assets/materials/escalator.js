// Escalator steps: grooved aluminium treads
// (the grooves run along the travel axis, the step's shorter top side) with a yellow
// demarcation line and cleats that slide along it, towards +x / +z, so a flight of static
// step boxes reads as moving. Risers are dark cleated plates, side skirts brushed steel.
export default {
  name: "escalator",
  glsl: `vec3 mat_escalator(Surf s){
    float edge = edgeDist(s.uv, s.sz);
    if (s.top) {
      bool runU = s.sz.x <= s.sz.y; // travel along the shorter side
      float along = runU ? s.uv.x : s.uv.y, across = runU ? s.uv.y : s.uv.x, run = runU ? s.sz.x : s.sz.y;
      float m = along - 1.0*uTime*0.5;
      vec3 col = PAL_ESCALATOR_TREAD * (0.75 + 0.35*gridLine(across/0.02, 0.2));
      float line = smoothstep(0.43, 0.46, abs(fract(m/0.45) - 0.5)); // one yellow edge per tread (prefabs.EscRun 0.45 m)
      col = mix(col, PAL_HAZARD, line);
      float sides = 1.0 - smoothstep(0.04, 0.06, min(across, (runU ? s.sz.y : s.sz.x) - across));
      return mix(col, PAL_HAZARD*0.9, sides);
    }
    // The top 0.25 m of a vertical face is the riser (dark cleats; on the flight's side
    // it draws the step profile), the rest is brushed steel skirt.
    if (s.uv.y > s.sz.y - 0.25) return PAL_ESCALATOR_TREAD * 0.45 * (0.8 + 0.3*gridLine(s.uv.x/0.03, 0.25));
    vec3 col = PAL_SHELF_METAL * (0.9 + 0.08*sin(s.uv.x*240.0 + fbm(s.uv*3.0)*3.0));
    return mix(col*0.6, col, smoothstep(0.01, 0.03, edge));
  }`,
};

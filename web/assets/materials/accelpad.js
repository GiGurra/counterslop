// de_serverless's accelerator pads (a fixed-velocity content.Launcher, mostly
// sideways): lime chevrons chasing along the pad's longer side over a dark
// plate, toward +x / +z (accelpad.js); the sides a glowing rim.
export default {
  name: "accelpad",
  glsl: `vec3 mat_accelpad(Surf s){
    vec2 r = readUV(s.n, s.uv, s.sz);
    if (!s.top) { gEmit = 0.7; return PAL_NEON_LIME; }
    bool alongX = s.sz.x >= s.sz.y;
    float a = alongX ? r.x : r.y, w = alongX ? s.sz.y : s.sz.x;
    float y = abs((alongX ? r.y : r.x) / w - 0.5);
    float on = aaPulse((-a / w + y) * 1.0 + uTime * 2.2, 0.6, 1.0) * (1.0 - aaStep(0.4, y)); // antialiased (MSAA leaves in-face lines jagged)
    float edge = aaStep(0.45, y);
    gEmit = clamp(on + edge * 0.6, 0.0, 1.0);
    return mix(vec3(0.05, 0.07, 0.05), PAL_NEON_LIME * 1.1, clamp(on + edge * 0.6, 0.0, 1.0));
  }`,
};

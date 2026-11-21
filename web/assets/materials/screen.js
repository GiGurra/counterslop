// Monitors and TVs: thin faces are black plastic; the big vertical faces glow with a
// seeded picture behind a bezel: scrolling code (a dark IDE), a demo game (sky, hills,
// a crosshair sweeping), or a sale ad (bars and a blinking price). Each box picks its
// own from where it stands. Tops are plain bezel.
export default {
  name: "screen",
  glsl: `vec3 mat_screen(Surf s){
    float edge = edgeDist(s.uv, s.sz);
    if (s.top || min(s.sz.x, s.sz.y) < 0.2) return PAL_BEZEL * (1.0 + 0.3*fbm(s.wuv*8.0));
    float b = 0.035;
    if (edge < b) return PAL_BEZEL * 1.2;
    vec2 p = (readUV(s.n, s.uv, s.sz) - b) / (s.sz - 2.0*b); // 0..1 over the picture, as the viewer reads it
    float h = faceHash(s.world, s.n, s.uv, 3.0), t = uTime + h*40.0;
    vec3 col;
    if (h < 0.45) { // code scrolling up
      float lines = 14.0, ln = floor(p.y*lines + t*1.5), fy = fract(p.y*lines + t*1.5);
      float indent = floor(hash(vec2(ln, h))*4.0)*0.06, len = 0.2 + 0.6*hash(vec2(h, ln));
      float tok = step(0.5, hash(vec2(floor(p.x*24.0), ln)));
      float on = step(0.03 + indent, p.x) * step(p.x, indent + len) * step(0.25, fy) * step(fy, 0.75);
      vec3 syn = mix(PAL_NEON_CYAN, mix(PAL_NEON_MAGENTA, PAL_NEON_LIME, step(0.6, hash(vec2(ln, 3.0)))), tok);
      col = mix(vec3(0.07, 0.07, 0.1), syn*0.85, on);
    } else if (h < 0.8) { // demo game
      float hill = 0.35 + 0.1*sin(p.x*9.0 + t*0.8) + 0.05*sin(p.x*23.0 - t*1.3);
      col = mix(mix(vec3(0.3, 0.55, 1.0), vec3(1.0, 0.7, 0.5), p.y), vec3(0.2, 0.6, 0.25)*(0.7 + 0.3*p.y), step(p.y, hill));
      vec2 ch = p - vec2(0.5 + 0.3*sin(t*0.7), 0.5 + 0.15*sin(t*1.1));
      col = mix(col, vec3(0.6, 1.0, 0.3), (step(abs(ch.x), 0.004*s.sz.y/s.sz.x*4.0) + step(abs(ch.y), 0.008)) * step(length(ch), 0.06));
    } else { // sale ad
      col = mix(PAL_REVIEW_RED, PAL_TOY_YELLOW, step(0.5, fract(p.y*3.0 + floor(t*0.5)*0.33)));
      col = mix(col, vec3(1.0), step(0.25, p.x) * step(p.x, 0.75) * step(0.4, p.y) * step(p.y, 0.6) * step(0.5, fract(t)));
    }
    col *= 0.9 + 0.1*sin(p.y*s.sz.y*300.0); // scanlines
    gEmit = 0.85;
    return col;
  }`,
};

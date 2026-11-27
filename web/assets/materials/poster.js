// A printed poster (motivational posters on the Lunar Office Campus, the duty
// free's ads): off-white paper, a coloured header band and a halftone picture
// (a moon over hills, a rocket or a handshake, picked per poster), a thin white
// margin. Ink letters (a content.Sign) go on top. Thin faces: paper edge.
export default {
  name: "poster",
  glsl: `vec3 mat_poster(Surf s){
    vec3 paper = vec3(0.92, 0.9, 0.84);
    if (s.top || min(s.sz.x, s.sz.y) < 0.15) return paper*0.9;
    vec2 p = readUV(s.n, s.uv, s.sz) / s.sz; // 0..1 as the reader sees it
    float h = faceHash(s.world, s.n, s.uv, 7.0);
    vec3 band = h < 0.33 ? PAL_NEON_CYAN*0.8 : h < 0.66 ? PAL_TOY_PINK*0.9 : PAL_TOY_YELLOW*0.9;
    float m = 0.04;
    if (p.x < m || p.x > 1.0 - m || p.y < m || p.y > 1.0 - m) return paper;
    vec3 col = paper;
    if (p.y > 0.78) col = band; // the header band (the poster's title in ink on it)
    else if (p.y > 0.3) { // the picture: a halftone of a disc over a horizon
      vec2 q = (p - vec2(0.5, 0.55)) * vec2(s.sz.x/s.sz.y, 1.0);
      float shape = h < 0.5 ? step(length(q - vec2(0.12, 0.08)), 0.14) + step(q.y, -0.12 + 0.05*sin(q.x*9.0))
                            : step(abs(q.x), 0.05) * step(q.y, 0.2) * step(-0.2, q.y) + step(length(q - vec2(0.0, 0.22)), 0.06);
      vec2 dot = fract(s.wuv*28.0) - 0.5;
      float tone = clamp(shape, 0.0, 1.0)*0.75 + 0.15;
      col = mix(paper, band*0.55, step(length(dot), 0.5*sqrt(tone)));
    }
    return col;
  }`,
};

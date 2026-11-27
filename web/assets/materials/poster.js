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
    // antialiased (aaStep & co, gen/glsl/patterns.js): MSAA leaves lines drawn inside a face jagged: no per-pixel
    // branches (derivatives need uniform flow): every layer worked out, mixed by coverage.
    // The picture: a halftone of a disc over a horizon.
    vec2 q = (p - vec2(0.5, 0.55)) * vec2(s.sz.x/s.sz.y, 1.0);
    float shape = h < 0.5 ? (1.0 - aaStep(0.14, length(q - vec2(0.12, 0.08)))) + (1.0 - aaStep(0.0, q.y + 0.12 - 0.05*sin(q.x*9.0)))
                          : aaLine(q.x, 0.05) * aaBand(q.y, -0.2, 0.2) + (1.0 - aaStep(0.06, length(q - vec2(0.0, 0.22))));
    vec2 dot = fract(s.wuv*28.0) - 0.5;
    float tone = clamp(shape, 0.0, 1.0)*0.75 + 0.15, rad = 0.5*sqrt(tone);
    // A dot's edge over the pixel's footprint in dot cells (fwidth of the continuous
    // s.wuv*28, not of dot: that jumps at the cells' edges); dots finer than a pixel
    // fade to the area they cover.
    float fw = length(fwidth(s.wuv*28.0));
    float ht = mix(1.0 - aaStepFw(rad, length(dot), fw), 3.1416*rad*rad, clamp(fw*2.0 - 1.0, 0.0, 1.0));
    vec3 col = mix(paper, mix(paper, band*0.55, ht), aaBand(p.y, 0.3, 0.78));
    col = mix(col, band, aaStep(0.78, p.y)); // the header band (the poster's title in ink on it)
    return mix(paper, col, aaBand(p.x, m, 1.0 - m) * aaBand(p.y, m, 1.0 - m));
  }`,
};

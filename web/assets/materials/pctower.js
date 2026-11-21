// Gaming PC towers: a black case; the big side faces have a tinted glass window with
// three RGB fans whose rings cycle through the rainbow; the narrow front is a mesh
// panel with a glowing strip; the top a vent grille.
export default {
  name: "pctower",
  glsl: `vec3 pcRainbow(float x){ return 0.5 + 0.5*cos(6.2832*(x + vec3(0.0, 0.33, 0.67))); }
  vec3 mat_pctower(Surf s){
    vec3 col = PAL_CASE_BLACK * (0.9 + 0.2*fbm(s.wuv*6.0));
    float edge = edgeDist(s.uv, s.sz);
    if (s.top) return mix(col, col*0.4, step(0.5, fract(s.uv.x*40.0)) * step(0.05, edge));
    float h = faceHash(s.world, s.n, s.uv, 4.0);
    if (s.sz.x < 0.3) { // front: mesh and a light strip
      col *= 0.8 + 0.25*step(0.5, fract(s.uv.x*60.0) * fract(s.uv.y*60.0) * 2.0);
      float strip = step(abs(s.uv.x - s.sz.x*0.5), 0.012) * step(0.08, s.uv.y) * step(s.uv.y, s.sz.y - 0.08);
      gEmit = strip;
      return mix(col, pcRainbow(s.uv.y*0.8 - uTime*0.3 + h), strip);
    }
    float win = step(0.05, edge);
    if (win < 0.5) return col;
    col = mix(col, PAL_GLASS_TINT*0.25, 0.8);
    // three fans in a column near the front edge, rings glowing
    float fanR = min(s.sz.x, s.sz.y) * 0.14;
    float glow = 0.0; vec3 g = vec3(0.0);
    for (int i = 0; i < 3; i++) {
      vec2 c = vec2(s.sz.x*0.25, s.sz.y*(0.22 + 0.28*float(i)));
      float d = length(s.uv - c);
      float ring = 1.0 - smoothstep(0.012, 0.022, abs(d - fanR));
      float blades = step(d, fanR*0.92) * (0.5 + 0.5*sin(atan(s.uv.y - c.y, s.uv.x - c.x)*7.0 + uTime*25.0));
      vec3 rc = pcRainbow(float(i)*0.15 + uTime*0.25 + h);
      g += rc*ring + vec3(0.18)*blades*step(0.25, d/fanR);
      glow += ring;
    }
    // the GPU inside: a slab with its own light bar
    float gpu = step(abs(s.uv.y - s.sz.y*0.42), 0.04) * step(s.sz.x*0.45, s.uv.x) * step(s.uv.x, s.sz.x*0.92);
    g += gpu * mix(vec3(0.1), PAL_NEON_LIME, step(abs(s.uv.y - s.sz.y*0.42), 0.008));
    gEmit = clamp(glow, 0.0, 1.0);
    return col + g;
  }`,
};

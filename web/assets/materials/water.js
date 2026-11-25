// Water (content.Map.Water): the surface of a water volume, drawn as a thin
// slab at its top, seen from above and from under it. Deep teal with two
// layers of drifting wave noise, bright crests (fake sun glints) and a faint
// caustic net; from below (n.y < 0) a lighter, flat mirror of the sky.
export default {
  name: "water",
  glsl: `vec3 mat_water(Surf s){
    vec2 p = s.wuv;
    if (s.n.y < -0.5) { // from below: the sky through the surface, lit by itself
      gEmit = 0.9;
      return mix(vec3(0.12, 0.38, 0.44), vec3(0.6, 0.82, 0.86), 0.3 + 0.35*fbm(p*0.4 + uTime*0.1));
    }
    float a = fbm(p*0.35 + vec2(uTime*0.12, uTime*0.05));
    float b = fbm(p*0.9 - vec2(uTime*0.07, uTime*0.16));
    float h = a*0.6 + b*0.4;
    vec3 col = mix(vec3(0.03, 0.16, 0.22), vec3(0.08, 0.36, 0.42), h);
    float crest = smoothstep(0.62, 0.72, h);
    col = mix(col, vec3(0.75, 0.92, 0.95), crest*0.55);
    vec2 c = abs(fract(p*0.6 + vec2(a, b)*0.8) - 0.5);
    col += vec3(0.05, 0.12, 0.12) * (1.0 - smoothstep(0.0, 0.06, min(c.x, c.y)));
    gEmit = 0.15;
    return col;
  }`,
};

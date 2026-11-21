// Light fixtures: a metal housing on top and round the rim, a glowing diffuser
// with louvre bars on the other faces. The glow takes the hue of the map light
// nearest the fixture (put a content.Light just under it), white with none.
export default {
  name: "lamp",
  glsl: `vec3 mat_lamp(Surf s){
    vec3 housing = PAL_LAMP_HOUSING * (0.9 + 0.1*fbm(s.wuv*8.0));
    if (s.n.y > 0.5) return housing;
    vec3 l = localLight(s.world, s.n);
    float m = max(max(l.r, l.g), l.b);
    vec3 glow = m > 0.02 ? PAL_LAMP * mix(vec3(1.0), l/m, 0.85) : PAL_LAMP;
    float rim = 1.0 - smoothstep(0.03, 0.045, edgeDist(s.uv, s.sz));
    float louvre = gridLine(s.uv.x/0.25, 0.04) * step(0.3, s.sz.x);
    glow *= 1.0 - 0.25*louvre;
    gEmit = 1.0 - rim;
    return mix(glow, housing, rim);
  }`,
};

// LAN cafe carpet: near-black with blacklight-bright arcade shapes (rings, triangles,
// zigzags and dots in magenta, cyan and yellow) scattered on a 0.8 m grid; the shapes
// glow a little (UV-reactive). Sides look like the same carpet.
export default {
  name: "lancarpet",
  glsl: `vec3 mat_lancarpet(Surf s){
    vec3 col = PAL_LAN_CARPET * (0.85 + 0.3*fbm(s.wuv*6.0));
    vec2 c = s.wuv/0.8, id = floor(c), f = fract(c) - 0.5;
    float h = hash(id), k = hash(id + 17.0);
    vec2 g = f - (vec2(hash(id + 3.0), hash(id + 5.0)) - 0.5)*0.35;
    float a = h*6.2832; g = mat2(cos(a), -sin(a), sin(a), cos(a)) * g;
    float shape;
    if (k < 0.3) shape = 1.0 - smoothstep(0.015, 0.03, abs(length(g) - 0.16)); // ring
    else if (k < 0.55) { vec2 t = abs(g); shape = (1.0 - smoothstep(0.015, 0.03, abs(max(t.x*0.866 + g.y*0.5, -g.y) - 0.12))); } // triangle
    else if (k < 0.8) shape = (1.0 - smoothstep(0.015, 0.03, abs(g.y - 0.06*sin(g.x*40.0)))) * step(abs(g.x), 0.22); // zigzag
    else shape = 1.0 - smoothstep(0.035, 0.05, length(g)); // dot
    vec3 sc = h < 0.4 ? PAL_NEON_MAGENTA : h < 0.75 ? PAL_NEON_CYAN : PAL_TOY_YELLOW;
    gEmit = shape * 0.3;
    return mix(col, sc*0.9, shape);
  }`,
};

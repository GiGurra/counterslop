// Matte dark lettering (content.Sign with mat "ink": posters, wall signs,
// the rocket's name): near-black paint with a slight sheen of wear.
export default {
  name: "ink",
  glsl: `vec3 mat_ink(Surf s){
    return vec3(0.045, 0.045, 0.055) * (0.85 + 0.3*fbm(s.wuv*14.0));
  }`,
};

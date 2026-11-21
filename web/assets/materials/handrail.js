// Escalator handrail: black rubber with a soft sheen along its length and faint
// seams. Its own material (a box on top of the glass balustrade), so the glass
// shader never has to guess from a face's width whether it is a handrail.
export default {
  name: "handrail",
  glsl: `vec3 mat_handrail(Surf s){
    vec3 col = vec3(0.035) * (0.9 + 0.2*fbm(s.wuv*6.0));
    vec3 v = normalize(uEye - s.world);
    col += vec3(0.05) * pow(1.0 - abs(dot(v, s.n)), 4.0);
    return col;
  }`,
};

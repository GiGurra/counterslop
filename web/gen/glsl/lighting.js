// Shared uniforms, sun + hemisphere lighting and distance fog. Colours come from the palette constants.
export const LIGHTING = `
uniform vec3 uEye; uniform vec3 uSun; uniform vec3 uFog; uniform float uTime;
vec3 light(vec3 col, vec3 n, float ao){
  float diff = max(dot(n, uSun), 0.0);
  vec3 hemi = mix(PAL_HEMI_GROUND, PAL_HEMI_SKY, n.y*0.5 + 0.5);
  return col * (hemi*0.75 + PAL_SUN_LIGHT*diff*0.8) * ao;
}
vec3 fog(vec3 col, vec3 world){
  float d = length(world - uEye);
  return mix(col, uFog, (1.0 - exp(-d*0.014)) * 0.85);
}
`;

// Directional shadow-map lookup with soft PCF over hardware-compared taps.
// The engine renders the depth map from the light and sets these uniforms:
//   uShadowMap   depth texture with TEXTURE_COMPARE_MODE (bilinear = 2x2 PCF per tap)
//   uShadowVP    world -> light clip space
//   uShadowQ     0 off, 1 one tap, 2 3x3 taps, 3 5x5 taps
//   uShadowTexel world metres per shadow-map texel (for the normal offset)
export const SHADOW = `
uniform highp sampler2DShadow uShadowMap; uniform mat4 uShadowVP;
uniform int uShadowQ; uniform float uShadowTexel;
float shadow(vec3 world, vec3 n){
  if (uShadowQ == 0) return 1.0;
  int r = uShadowQ - 1;
  float ndl = clamp(dot(n, uSun), 0.0, 1.0);
  // Normal offset grows with the kernel and at grazing angles: no acne, no peter-panning.
  vec3 p = world + n * uShadowTexel * (1.0 + float(r)) * (1.0 + 1.5*(1.0 - ndl));
  vec4 c = uShadowVP * vec4(p, 1.0);
  vec3 s = c.xyz * 0.5 + 0.5;
  if (any(lessThan(s, vec3(0.0))) || any(greaterThan(s, vec3(1.0)))) return 1.0;
  s.z -= 0.0004;
  vec2 tx = 1.0 / vec2(textureSize(uShadowMap, 0));
  float sum = 0.0, wsum = 0.0;
  for (int y = -r; y <= r; y++) for (int x = -r; x <= r; x++) {
    vec2 o = vec2(x, y);
    float w = 1.0 - 0.5*length(o) / (float(r) + 1.0);
    sum += w * texture(uShadowMap, vec3(s.xy + o*tx*1.25, s.z));
    wsum += w;
  }
  return sum / wsum;
}
`;

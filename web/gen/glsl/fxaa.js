// Fast approximate anti-aliasing as one post-process function, written for this repo:
// find the local edge direction from the luma of the four diagonal neighbours, then
// blend along it (two taps inside, two further out; the wide blend only if its luma
// stays within the neighbourhood, so thin lines don't smear). Flat areas return the
// pixel untouched, which keeps texture detail sharp.
//   vec3 fxaa(sampler2D src, vec2 uv, vec2 texel)   texel = 1.0 / source size
export const FXAA = /* glsl */ `
float fxaaLuma(vec3 c) { return dot(c, vec3(0.299, 0.587, 0.114)); }
vec3 fxaa(sampler2D src, vec2 uv, vec2 texel) {
  vec3 cM = texture(src, uv).rgb;
  float lNW = fxaaLuma(texture(src, uv + vec2(-1.0, -1.0) * texel).rgb);
  float lNE = fxaaLuma(texture(src, uv + vec2( 1.0, -1.0) * texel).rgb);
  float lSW = fxaaLuma(texture(src, uv + vec2(-1.0,  1.0) * texel).rgb);
  float lSE = fxaaLuma(texture(src, uv + vec2( 1.0,  1.0) * texel).rgb);
  float lM = fxaaLuma(cM);
  float lMin = min(lM, min(min(lNW, lNE), min(lSW, lSE)));
  float lMax = max(lM, max(max(lNW, lNE), max(lSW, lSE)));
  if (lMax - lMin < max(0.0312, lMax * 0.125)) return cM; // no edge here
  vec2 dir = vec2(-((lNW + lNE) - (lSW + lSE)), (lNW + lSW) - (lNE + lSE));
  float reduce = max((lNW + lNE + lSW + lSE) * 0.25 * 0.125, 1.0 / 128.0);
  float rcpMin = 1.0 / (min(abs(dir.x), abs(dir.y)) + reduce);
  dir = clamp(dir * rcpMin, vec2(-8.0), vec2(8.0)) * texel;
  vec3 a = 0.5 * (texture(src, uv + dir * (1.0 / 3.0 - 0.5)).rgb + texture(src, uv + dir * (2.0 / 3.0 - 0.5)).rgb);
  vec3 b = a * 0.5 + 0.25 * (texture(src, uv - dir * 0.5).rgb + texture(src, uv + dir * 0.5).rgb);
  float lB = fxaaLuma(b);
  return (lB < lMin || lB > lMax) ? a : b;
}`;

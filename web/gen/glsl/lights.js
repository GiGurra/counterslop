// Local lights and sky visibility from the per-map light grid (web/engine/lights.js
// builds it, the renderer uploads it). One grid cell per world cell:
//   uLightCells  (x, z, layer y) -> offset << 8 | count into uLightList (a sub-box of the grid)
//   uLightList   light indices, LIST_W per row
//   uLightData   4 texels per light (row = light): see packLights
//   uSkyVis      sky visibility 0..1 over the whole grid, filtered
// A shaded point looks up the cell half a cell out along its normal, so a wall's
// face reads the cell in front of it, not the wall's own.
export const LIST_W = 4096;
// Dynamic lights (muzzle flashes, ...): slots in the uniform arrays, and the side (grid
// cells) of each one's cell mask (engine/dynlights.js): a range must stay under MASK/2 cells (10 m).
export const MAX_DYN_LIGHTS = 8;
export const MASK = 40;
// Lamp shadows: every Shadow light has static tiles (the map, baked after load) in one
// atlas; up to MAX_SHADOW_SLOTS lights at a time also have live tiles (players drawn
// in). A spot uses one tile, a point light six (cube faces).
export const MAX_SHADOW_SLOTS = 8;

export const LIGHTS = `
uniform highp usampler2DArray uLightCells; uniform highp usampler2D uLightList;
uniform highp sampler2D uLightData; uniform highp sampler2DArray uSkyVis;
uniform vec3 uLGOrigin; uniform float uLGCell; uniform ivec3 uLGDims;
uniform ivec3 uLCOff; uniform ivec3 uLCDims; uniform int uLightsOn; uniform int uSkyOn;
uniform float uIndoor; // ambient kept where no sky is visible

float skyVis(vec3 world, vec3 n){
  if (uSkyOn == 0) return 1.0;
  vec3 p = (world + n*uLGCell*0.5 - uLGOrigin)/uLGCell - 0.5;
  if (any(lessThan(p, vec3(-0.5))) || any(greaterThan(p, vec3(uLGDims) - 0.5))) return 1.0;
  float y0 = floor(p.y), t = p.y - y0, top = float(uLGDims.y - 1);
  vec2 uv = (p.xz + 0.5) / vec2(uLGDims.xz);
  float a = texture(uSkyVis, vec3(uv, clamp(y0, 0.0, top))).r;
  float b = texture(uSkyVis, vec3(uv, clamp(y0 + 1.0, 0.0, top))).r;
  return mix(a, b, t);
}

// A dying tube: a steady hum with now and then a drop-out, seeded per light.
float flicker(float seed){
  float h = hash(vec2(seed, floor(uTime*7.0)));
  float hum = 0.93 + 0.07*noise(vec2(seed*1.7, uTime*23.0));
  return h > 0.88 ? 0.15 + 0.5*hash(vec2(seed, floor(uTime*31.0))) : hum;
}

float lightShadow(vec4 sh, bool spot, vec3 world, vec3 n, vec3 toLight, float dist);

// Dynamic lights (engine/dynlights.js): up to MAX_DYN_LIGHTS brief point lights, each with a
// MASK^3 cell mask around it (layers slot*MASK + y of uDynMask) of the cells its flood
// reached, so a muzzle flash doesn't light the room behind a wall. No shadows. In every
// variant: maps without lamps get them too, and uDynN 0 skips the loop.
uniform vec4 uDynPos[${MAX_DYN_LIGHTS}]; // xyz, range
uniform vec4 uDynCol[${MAX_DYN_LIGHTS}]; // colour x strength, slot
uniform ivec3 uDynOff[${MAX_DYN_LIGHTS}]; // the mask's corner cell
uniform int uDynN; uniform highp sampler2DArray uDynMask;
vec3 dynLight(vec3 world, vec3 n){
  vec3 sum = vec3(0.0);
  if (uDynN == 0) return sum;
  ivec3 c = ivec3(floor((world + n*uLGCell*0.5 - uLGOrigin)/uLGCell));
  for (int i = 0; i < ${MAX_DYN_LIGHTS}; i++) {
    if (i >= uDynN) break;
    vec4 a = uDynPos[i];
    vec3 L = a.xyz - world;
    float d2 = dot(L, L), r2 = a.w*a.w;
    if (d2 >= r2) continue;
    float d = sqrt(d2);
    float ndl = dot(n, L / max(d, 1e-4));
    if (ndl <= 0.0) continue;
    ivec3 m = c - uDynOff[i];
    if (any(lessThan(m, ivec3(0))) || any(greaterThanEqual(m, ivec3(${MASK})))) continue;
    vec4 col = uDynCol[i];
    if (texelFetch(uDynMask, ivec3(m.x, m.z, int(col.w)*${MASK} + m.y), 0).r < 0.5) continue;
    float w = 1.0 - d2/r2;
    sum += col.rgb * (ndl * w*w / (1.0 + d2));
  }
  return sum;
}

// Sum of the local lights reaching this point (colour x intensity x N.L x falloff x cone).
// Compiled in only with HAS_LIGHTS (the renderer picks the variant per map): a map
// without lights runs none of this, which matters on CPU rasterisers (SwiftShader).
#ifndef HAS_LIGHTS
vec3 localLight(vec3 world, vec3 n){ return vec3(0.0); }
#else
vec3 localLight(vec3 world, vec3 n){
  if (uLightsOn == 0) return vec3(0.0);
  ivec3 c = ivec3(floor((world + n*uLGCell*0.5 - uLGOrigin)/uLGCell)) - uLCOff;
  if (any(lessThan(c, ivec3(0))) || any(greaterThanEqual(c, uLCDims))) return vec3(0.0);
  uint e = texelFetch(uLightCells, ivec3(c.x, c.z, c.y), 0).r;
  int cnt = int(e & 255u), off = int(e >> 8u);
  vec3 sum = vec3(0.0);
  for (int i = 0; i < cnt; i++) {
    int k = off + i;
    int li = int(texelFetch(uLightList, ivec2(k % ${LIST_W}, k / ${LIST_W}), 0).r);
    vec4 a = texelFetch(uLightData, ivec2(0, li), 0);
    vec3 L = a.xyz - world;
    float d2 = dot(L, L), r2 = a.w*a.w;
    if (d2 >= r2) continue;
    float d = sqrt(d2);
    L /= max(d, 1e-4);
    float ndl = dot(n, L);
    if (ndl <= 0.0) continue;
    vec4 b = texelFetch(uLightData, ivec2(1, li), 0);
    vec4 s = texelFetch(uLightData, ivec2(2, li), 0);
    vec4 x = texelFetch(uLightData, ivec2(3, li), 0);
    float w = 1.0 - d2/r2;
    float att = w*w / (1.0 + d2);
    if (s.w > -1.5) att *= smoothstep(s.w, x.x, dot(-L, s.xyz));
    if (b.w > 0.0) att *= flicker(b.w);
    if (att <= 0.0) continue;
    if (x.y >= 0.0) att *= lightShadow(x, s.w > -1.5, world, n, L, d);
    sum += b.rgb * (ndl * att);
  }
  return sum;
}
#endif
#ifndef HAS_LIGHT_SHADOWS
float lightShadow(vec4 sh, bool spot, vec3 world, vec3 n, vec3 toLight, float dist){ return 1.0; }
#else
// sh = the light's texel 3: (cos inner, static base tile, live base tile, live weight).
// The tile is the spot's one view, or the cube face looking towards the point
// (engine/lights.js cubeFace); its matrix is row base + face of uLSMat (texels 0-3
// the world -> clip matrix, texel 4 .x the texel size in metres per metre).
uniform highp sampler2DShadow uLShadow; uniform highp sampler2DShadow uLShadowDyn; uniform highp sampler2D uLSMat;
uniform int uLSOn; uniform float uLSTile; uniform vec2 uLSAtlas; uniform int uLSCols; uniform vec2 uLDAtlas; uniform int uLDCols;
mat4 lsMat(int r){
  return mat4(texelFetch(uLSMat, ivec2(0, r), 0), texelFetch(uLSMat, ivec2(1, r), 0),
              texelFetch(uLSMat, ivec2(2, r), 0), texelFetch(uLSMat, ivec2(3, r), 0));
}
// Soft PCF in tile t of an atlas: 4 hardware-filtered taps 1.5 texels apart (a smooth
// edge about 3 texels wide), clamped inside the tile.
float lsTap(highp sampler2DShadow tex, vec2 atlas, int cols, int t, vec3 s){
  vec2 tile = vec2(t % cols, t / cols), tx = 1.0 / atlas, size = uLSTile / atlas;
  vec2 lo = tile*size + tx*2.5, hi = (tile + 1.0)*size - tx*2.5, uv = (tile + s.xy)*size;
  float sum = 0.0;
  for (int y = 0; y < 2; y++) for (int x = 0; x < 2; x++)
    sum += texture(tex, vec3(clamp(uv + (vec2(x, y) - 0.5)*tx*1.5, lo, hi), s.z - 0.00005));
  return sum * 0.25;
}
float lightShadow(vec4 sh, bool spot, vec3 world, vec3 n, vec3 toLight, float dist){
  if (uLSOn == 0) return 1.0;
  int face = 0;
  if (!spot) {
    vec3 v = -toLight, a = abs(v);
    face = a.x >= a.y && a.x >= a.z ? (v.x > 0.0 ? 0 : 1) : a.y >= a.z ? (v.y > 0.0 ? 2 : 3) : (v.z > 0.0 ? 4 : 5);
  }
  int row = int(sh.y) + face;
  // Normal offset of ~2 texels at this distance, more at grazing angles.
  float ndl = clamp(dot(n, toLight), 0.0, 1.0);
  vec3 p = world + n * texelFetch(uLSMat, ivec2(4, row), 0).x * dist * (2.0 + 2.0*(1.0 - ndl));
  vec4 c = lsMat(row) * vec4(p, 1.0);
  vec3 s = c.xyz / c.w * 0.5 + 0.5;
  if (c.w <= 0.0 || s.z >= 1.0) return 1.0;
  float vis = lsTap(uLShadow, uLSAtlas, uLSCols, row, s);
  if (sh.z >= 0.0 && sh.w > 0.0) vis = mix(vis, lsTap(uLShadowDyn, uLDAtlas, uLDCols, int(sh.z) + face, s), sh.w);
  return vis;
}
#endif
`;

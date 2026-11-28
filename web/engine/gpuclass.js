// What kind of GPU the context runs on, from its renderer string (gl.js info().gpu):
// a CPU rasteriser (SwiftShader in headless Chrome and on machines without a usable GPU,
// Mesa's llvmpipe/softpipe, Windows' Basic Render Driver) is too slow for the optional
// effects, so their settings default off there (main.js; the player can turn them on).
const SOFTWARE = /swiftshader|llvmpipe|softpipe|lavapipe|software|basic render/i;
export const isSoftwareGpu = (renderer) => SOFTWARE.test(String(renderer || ""));

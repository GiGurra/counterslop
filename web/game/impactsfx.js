// Which sound a bullet impact or a landing casing makes: the surface from the
// map material, and a rate limiter so full auto doesn't stack a hundred voices.
// Pure: main.js plays the sounds.

// Map material (internal/content Materials) -> impact surface.
const SURFACE = {
  sand: "dirt",
  brick: "stone", concrete: "stone", plaster: "stone", stucco: "stone", storetile: "stone", hazard: "stone",
  crate: "wood", desk: "wood", shelf: "wood",
  metal: "metal", container: "metal", rack: "metal", escalator: "metal", escalator_rev: "metal", checkout: "metal", pctower: "metal", lamp: "metal",
  carton: "cardboard", awning: "cardboard",
  glass: "glass", screen: "glass", neon: "glass",
  carpet: "soft", lancarpet: "soft",
};
export const SURFACES = ["stone", "metal", "wood", "glass", "dirt", "cardboard", "soft"];
export const surfaceOf = (kind) => SURFACE[kind] || "stone";

// What a bullet sounds like hitting a surface: soft floors (carpet) take it like dirt.
export const impactSound = (surface) => (surface === "soft" ? "dirt" : surface);
// Whether a floor rings when a casing lands on it (hard) or swallows it (carpet, sand, cardboard).
export const hardFloor = (surface) => !["soft", "dirt", "cardboard"].includes(surface);

// Bullet impacts and casings are heard this far (m); beyond, nothing is played.
export const IMPACT_RANGE = 18, CASING_RANGE = 14;

// A token bucket per key: rate sounds a second, up to burst at once.
// allow(key, nowS) says whether one more may play now.
export function createLimiter({ rate = 12, burst = 4 } = {}) {
  const buckets = new Map();
  return {
    allow(key, t) {
      let b = buckets.get(key);
      if (!b) buckets.set(key, (b = { tokens: burst, t }));
      b.tokens = Math.min(burst, b.tokens + (t - b.t) * rate);
      b.t = t;
      if (b.tokens < 1) return false;
      b.tokens -= 1;
      return true;
    },
  };
}

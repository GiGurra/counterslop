// The one shared palette. Materials, sky, models and particles pick colours from here
// so everything stays in the same warm, toy-like range. Linear-ish 0..1 RGB.
export const PALETTE = {
  // environment
  sand: [0.86, 0.72, 0.48],
  slop: [0.45, 0.85, 0.15],
  brickDark: [0.74, 0.40, 0.30],
  brickLight: [0.92, 0.62, 0.42],
  mortar: [0.82, 0.78, 0.70],
  capstone: [0.72, 0.70, 0.66],
  wood: [0.74, 0.50, 0.27],
  woodFrame: [0.58, 0.38, 0.20],
  concrete: [0.56, 0.61, 0.70],
  hazard: [1.0, 0.82, 0.1],
  hazardDark: [0.08, 0.08, 0.08],
  // light and sky
  fog: [1.0, 0.84, 0.66],
  zenith: [0.30, 0.52, 0.92],
  cloud: [1.0, 0.97, 0.94],
  sunCore: [1.0, 0.9, 0.6],
  sunGlow: [1.0, 0.6, 0.3],
  sunLight: [1.0, 0.92, 0.78],
  hemiGround: [0.42, 0.36, 0.30],
  hemiSky: [0.55, 0.65, 0.85],
  // toys
  toyTeal: [0.15, 0.75, 0.7],
  toyTealDark: [0.12, 0.55, 0.5],
  toyPink: [0.95, 0.3, 0.6],
  toyYellow: [0.95, 0.85, 0.2],
  toyTip: [1.0, 0.45, 0.1],
  gunmetal: [0.2, 0.2, 0.25],
  visor: [0.2, 1.0, 0.95],
  flash: [1.0, 0.9, 0.3],
  flashHot: [1.0, 0.95, 0.4],
  spark: [1.0, 0.95, 0.6],
};

// ARPAbet-ish phoneme targets for SlopVox. Values are textbook acoustic-phonetics
// averages for an adult voice (formants F1..F3 in Hz, durations in ms at rate 1).
//
// kind:  v vowel | d diphthong (f -> f2) | a approximant | n nasal | f fricative |
//        h aspirate (takes the next sound's formants) | s stop | x affricate | _ silence
// av:    voicing amplitude        af: frication amplitude    fric: [centre Hz, bandwidth Hz]
// Stops: clo closure ms, burst amplitude, asp aspiration ms (voiceless), f = place locus.

const V = (f, dur, f2) => ({ kind: f2 ? "d" : "v", f, f2, dur, av: 1 });

export const PHONEMES = {
  // vowels
  AA: V([730, 1090, 2440], 150),
  AE: V([660, 1720, 2410], 150),
  AH: V([620, 1220, 2550], 110),
  AO: V([570, 840, 2410], 150),
  AX: V([500, 1500, 2500], 60),
  EH: V([530, 1840, 2480], 120),
  ER: V([470, 1350, 1690], 140),
  IH: V([390, 1990, 2550], 100),
  IY: V([290, 2300, 3000], 140),
  UH: V([440, 1020, 2240], 110),
  UW: V([300, 870, 2240], 150),
  EY: V([480, 1850, 2500], 180, [330, 2200, 2650]),
  AY: V([700, 1220, 2600], 210, [400, 1950, 2600]),
  OW: V([540, 1100, 2300], 180, [420, 850, 2300]),
  AW: V([720, 1240, 2600], 210, [450, 1000, 2400]),
  OY: V([550, 960, 2400], 210, [360, 1820, 2500]),

  // approximants
  L: { kind: "a", f: [330, 1050, 2880], dur: 70, av: 0.8 },
  R: { kind: "a", f: [330, 1060, 1380], dur: 70, av: 0.8 },
  W: { kind: "a", f: [290, 610, 2150], dur: 60, av: 0.8 },
  Y: { kind: "a", f: [260, 2070, 3020], dur: 60, av: 0.8 },

  // nasals: low F1, weak and dull
  M: { kind: "n", f: [270, 1100, 2150], dur: 75, av: 0.55 },
  N: { kind: "n", f: [270, 1500, 2600], dur: 70, av: 0.55 },
  NG: { kind: "n", f: [270, 2100, 2700], dur: 80, av: 0.55 },

  // fricatives
  S: { kind: "f", f: [320, 1700, 2700], dur: 115, af: 0.75, fric: [5600, 2600], av: 0 },
  Z: { kind: "f", f: [320, 1700, 2700], dur: 85, af: 0.45, fric: [5600, 2600], av: 0.45 },
  SH: { kind: "f", f: [300, 1840, 2750], dur: 120, af: 1.1, fric: [2900, 1900], av: 0 },
  ZH: { kind: "f", f: [300, 1840, 2750], dur: 80, af: 0.5, fric: [2900, 1900], av: 0.45 },
  F: { kind: "f", f: [340, 1100, 2080], dur: 100, af: 0.2, fric: [5000, 7000], av: 0 },
  V: { kind: "f", f: [340, 1100, 2080], dur: 60, af: 0.15, fric: [5000, 7000], av: 0.6 },
  TH: { kind: "f", f: [320, 1290, 2540], dur: 90, af: 0.15, fric: [5500, 7000], av: 0 },
  DH: { kind: "f", f: [320, 1290, 2540], dur: 50, af: 0.12, fric: [5500, 7000], av: 0.6 },
  HH: { kind: "h", dur: 65, ah: 0.75 },

  // stops: closure, burst, (aspiration)
  P: { kind: "s", f: [400, 1100, 2150], clo: 70, burst: 0.45, fric: [1300, 2200], asp: 45 },
  B: { kind: "s", f: [400, 1100, 2150], clo: 55, burst: 0.3, fric: [1300, 2200], asp: 0, voiced: true },
  T: { kind: "s", f: [400, 1650, 2600], clo: 60, burst: 0.6, fric: [4600, 2600], asp: 50 },
  D: { kind: "s", f: [400, 1650, 2600], clo: 50, burst: 0.4, fric: [4600, 2600], asp: 0, voiced: true },
  K: { kind: "s", f: [300, 1990, 2850], clo: 65, burst: 0.6, fric: [2300, 1000], asp: 55 },
  G: { kind: "s", f: [300, 1990, 2850], clo: 55, burst: 0.4, fric: [2300, 1000], asp: 0, voiced: true },

  // affricates: a stop closure released into a fricative
  CH: { kind: "x", f: [300, 1840, 2750], clo: 60, dur: 90, af: 0.85, fric: [3000, 2000], av: 0 },
  JH: { kind: "x", f: [300, 1840, 2750], clo: 50, dur: 70, af: 0.5, fric: [3000, 2000], av: 0.45 },

  // pauses
  _: { kind: "_", dur: 110 },
  ",": { kind: "_", dur: 220 },
};

export const isVowel = (p) => p.kind === "v" || p.kind === "d";

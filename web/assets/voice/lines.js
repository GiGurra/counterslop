// The announcer: voices (SlopVox presets) and every line as hand-written phonemes.
// Phoneme format: gen/voice/slopvox.js. "~3" = stutter the next syllable 3 times.

export const VOICES = {
  // Big, slow, doubled an octave down, a bit of grit, a lot of hall.
  announcer: {
    pitch: 96, rate: 0.82, shift: 0.9, robot: 0.15, excite: 1.1, tilt: 0.3, seed: 7,
    layers: [{ pitch: 1, gain: 1 }, { pitch: 0.5, gain: 0.45 }],
    drive: 0.25, reverb: { seconds: 2.6, decay: 3, wet: 0.3 },
  },
  // Same voice pushed into the red for the top streak.
  monster: {
    pitch: 90, rate: 0.8, shift: 0.86, robot: 0.1, excite: 1.4, tilt: 0.25, seed: 9,
    layers: [{ pitch: 1, gain: 1 }, { pitch: 0.5, gain: 0.55 }],
    stutter: { rise: 0.55, gap: 0.03, cut: 0.8 },
    drive: 0.5, reverb: { seconds: 3.2, decay: 2.5, wet: 0.35 },
  },
  // Team taunts: a smaller, robotic, bitcrushed voice.
  taunt: {
    pitch: 150, rate: 1, shift: 1.08, robot: 0.55, excite: 0.8, tilt: 0.2, seed: 3,
    layers: [{ pitch: 1, gain: 1 }],
    drive: 0.15, crush: { bits: 6, hold: 2 }, reverb: { seconds: 1.2, decay: 4, wet: 0.18 },
  },
};

// id -> { text, ph, voice }. Every id is a say_announcer argument and a gallery button.
export const LINES = {
  first_bug: { text: "First bug!", ph: "F ER1 S T _ B ^AH1 G" },
  double_commit: { text: "Double commit!", ph: "D AH1 B AX L _ K AX M ^IH1 T" },
  triple_merge: { text: "Triple merge!", ph: "T R IH1 P AX L _ M ^ER1 JH" },
  mega_refactor: { text: "Mega refactor!", ph: "M EH1 G AX _ R IY0 F ^AE1 K T ER0" },
  ultra_slop: { text: "Ultra slop!", ph: "AH1 L T R AX _ S L ^AA1 P" },
  monster_slop: { text: "Mo-mo-mo-MONSTER SLOP!", ph: "~3 M ^AA1 N S T ER0 _ S L ^AA1 P", voice: "monster" },

  shipping_spree: { text: "Shipping spree!", ph: "SH IH1 P IH0 NG _ S P R ^IY1" },
  context_overflow: { text: "Context overflow!", ph: "K AA1 N T EH2 K S T _ OW2 V ER0 F L ^OW1" },
  unreviewable: { text: "Unreviewable!", ph: "AH2 N R IY0 V Y ^UW1 AX B AX L" },
  agi_like: { text: "A-G-I-like!", ph: "EY1 _ JH IY1 _ AY1 _ L ^AY1 K" },

  brain_dump: { text: "Brain dump!", ph: "B R EY1 N _ D ^AH1 M P" },
  prompt_locked: { text: "Prompt locked. Ship it!", ph: "P R AA1 M P T _ L AA1 K T , SH ^IH1 P IH0 T" },

  vibes_only: { text: "Vibes only!", ph: "V ^AY1 B Z _ OW1 N L IY0", voice: "taunt" },
  hand_typed: { text: "Hand-typed, baby!", ph: "HH AE1 N D T AY2 P T , B ^EY1 B IY0", voice: "taunt" },
};

// Kill streak (kills in quick succession) -> line; 6 and up is the monster.
export const MULTI = [null, "first_bug", "double_commit", "triple_merge", "mega_refactor", "ultra_slop", "monster_slop"];
// Kills without dying -> line, at these counts.
export const SPREES = [[5, "shipping_spree"], [10, "context_overflow"], [15, "unreviewable"], [20, "agi_like"]];
export const TAUNTS = { vibe: "vibes_only", artisan: "hand_typed" };

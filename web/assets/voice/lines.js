// The announcer: voices (SlopVox presets) and every line as hand-written phonemes.
// Phoneme format: gen/voice/slopvox.js. "~3" = stutter the next syllable 3 times.

// Every voice keeps a clean dry take on top; the octave layer, drive and bitcrush sit on
// an FX bus mixed under it (fxMix) and the hall is a reverb send (wet), so words stay clear.
export const VOICES = {
  // Big and slow, an octave-down double and some grit underneath, a modest hall.
  announcer: {
    pitch: 100, rate: 0.72, shift: 0.95, robot: 0.1, excite: 0.8, tilt: 0.15, presence: 0.5, seed: 7,
    layers: [{ pitch: 1, gain: 1 }, { pitch: 0.5, gain: 0.5 }],
    drive: 0.35, fxMix: 0.35, reverb: { seconds: 2, decay: 3, wet: 0.14 },
  },
  // Same voice pushed harder for the top streak.
  monster: {
    pitch: 94, rate: 0.72, shift: 0.92, robot: 0.08, excite: 1, tilt: 0.12, presence: 0.5, seed: 9,
    layers: [{ pitch: 1, gain: 1 }, { pitch: 0.5, gain: 0.6 }],
    stutter: { rise: 0.6, gap: 0.04, cut: 0.85 },
    drive: 0.6, fxMix: 0.45, reverb: { seconds: 2.6, decay: 2.5, wet: 0.18 },
  },
  // Team taunts: a smaller, robotic voice with a bitcrushed copy underneath.
  taunt: {
    pitch: 150, rate: 0.85, shift: 1.05, robot: 0.35, excite: 0.7, tilt: 0.12, presence: 0.5, seed: 3,
    layers: [{ pitch: 1, gain: 1 }],
    drive: 0.2, crush: { bits: 6, hold: 2 }, fxMix: 0.4, reverb: { seconds: 1.2, decay: 4, wet: 0.1 },
  },
};

// id -> { color, text, ph, voice }. color tints the caption. Every id is a say_announcer argument and a gallery button.
export const LINES = {
  first_bug: { color: "#8bff4f", text: "First bug!", ph: "F ER1 S T _ B ^AH1 G" },
  double_commit: { color: "#ffe14f", text: "Double commit!", ph: "D AH1 B AX L _ K AX M ^IH1 T" },
  triple_merge: { color: "#ffb84f", text: "Triple merge!", ph: "T R IH1 P AX L _ M ^ER1 JH" },
  mega_refactor: { color: "#ff8a3d", text: "Mega refactor!", ph: "M EH1 G AX _ R IY0 F ^AE1 K T ER0" },
  ultra_slop: { color: "#ff4f4f", text: "Ultra slop!", ph: "AH1 L T R AX _ S L ^AA1 P" },
  monster_slop: { color: "#ff3df0", text: "Mo-mo-mo-MONSTER SLOP!", ph: "~3 M ^AA1 N S T ER0 _ S L ^AA1 P", voice: "monster" },

  shipping_spree: { color: "#4fd1ff", text: "Shipping spree!", ph: "SH IH1 P IH0 NG _ S P R ^IY1" },
  context_overflow: { color: "#4f9dff", text: "Context overflow!", ph: "K AA1 N T EH2 K S T _ OW2 V ER0 F L ^OW1" },
  unreviewable: { color: "#8a6bff", text: "Unreviewable!", ph: "AH2 N R IY0 V Y ^UW1 AX B AX L" },
  agi_like: { color: "#ffffff", text: "A-G-I-like!", ph: "EY1 _ JH IY1 _ AY1 _ L ^AY1 K" },

  brain_dump: { color: "#ff5a5a", text: "Brain dump!", ph: "B R EY1 N _ D ^AH1 M P" },
  prompt_locked: { color: "#8bff4f", text: "Prompt locked. Ship it!", ph: "P R AA1 M P T _ L AA1 K T , SH ^IH1 P IH0 T" },

  // Round end: the announcer names the winner; the round-end banner shows it as
  // big centre text (banner: no separate caption) with the flavour line below.
  vibe_win: { color: "#ff3df0", text: "Vibe Coders win!", ph: "V AY1 B _ K OW1 D ER0 Z , W ^IH1 N", banner: true },
  artisan_win: { color: "#ff9a3d", text: "Artisans win!", ph: "AA1 R T IH0 Z AX N Z , W ^IH1 N", banner: true },
  // The Big Diff: planted, and the round-end flavour lines by winner and reason (shown under the banner).
  force_push: { color: "#ff3df0", text: "Force push detected!", ph: "F AO1 R S _ P UH1 SH , D IY0 T ^EH1 K T IH0 D" },
  shipped_to_prod: { color: "#ff3df0", text: "Shipped to prod!", ph: "SH IH1 P T _ T UW0 _ P R ^AA1 D" },
  merged_no_review: { color: "#ff3df0", text: "Merged without review!", ph: "M ER1 JH D _ W IH0 DH AW1 T _ R IY0 V Y ^UW1" },
  reverted: { color: "#ff9a3d", text: "Reverted!", ph: "R IY0 V ^ER1 T IH0 D" },
  changes_requested: { color: "#ff9a3d", text: "Changes requested!", ph: "CH EY1 N JH IH0 Z _ R IY0 K W ^EH1 S T IH0 D" },
  vibe_check_failed: { color: "#ff9a3d", text: "Vibe check failed!", ph: "V AY1 B _ CH EH1 K _ F ^EY1 L D" },
  vibes_only: { color: "#ff4f8b", text: "Vibes only!", ph: "V ^AY1 B Z _ OW1 N L IY0", voice: "taunt" },
  hand_typed: { color: "#4fd1ff", text: "Hand-typed, baby!", ph: "HH AE1 N D T AY2 P T , B ^EY1 B IY0", voice: "taunt" },
};

// Kill streak (kills in quick succession) -> line; 6 and up is the monster.
export const MULTI = [null, "first_bug", "double_commit", "triple_merge", "mega_refactor", "ultra_slop", "monster_slop"];
// Kills without dying -> line, at these counts.
export const SPREES = [[5, "shipping_spree"], [10, "context_overflow"], [15, "unreviewable"], [20, "agi_like"]];
// Round end: winner -> reason -> flavour line (the banner's subline; the voice says <team>_win).
export const ROUND_END = {
  vibe: { exploded: "shipped_to_prod", elimination: "merged_no_review" },
  artisan: { defused: "reverted", time: "changes_requested", elimination: "vibe_check_failed" },
};
export const TAUNTS = { vibe: "vibes_only", artisan: "hand_typed" };

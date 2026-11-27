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
  // Radio callouts: quick, thin and crunchy, like a cheap headset; no hall.
  radio: {
    pitch: 128, rate: 1.0, shift: 1.08, robot: 0.25, excite: 0.8, tilt: 0.3, presence: 0.8, seed: 11,
    layers: [{ pitch: 1, gain: 1 }],
    drive: 0.55, crush: { bits: 5, hold: 2 }, fxMix: 0.55, reverb: { seconds: 0.3, decay: 6, wet: 0.02 },
  },
  // The superstore PA (GPU Drop): a bright, band-limited ceiling speaker in a big hall.
  pa: {
    pitch: 122, rate: 0.86, shift: 1.04, robot: 0.12, excite: 0.75, tilt: 0.4, presence: 0.9, seed: 13,
    layers: [{ pitch: 1, gain: 1 }],
    drive: 0.3, crush: { bits: 8, hold: 1 }, fxMix: 0.3, reverb: { seconds: 2.8, decay: 2.4, wet: 0.26 },
  },
  // Star Slop's spaceport tannoy (the "tickets please" conductor, the cabin crew):
  // a nasal, band-limited ceiling speaker, a little bored.
  conductor: {
    pitch: 112, rate: 0.92, shift: 1.1, robot: 0.15, excite: 0.7, tilt: 0.35, presence: 0.95, seed: 17,
    layers: [{ pitch: 1, gain: 1 }],
    drive: 0.25, crush: { bits: 7, hold: 1 }, fxMix: 0.35, reverb: { seconds: 1.6, decay: 3, wet: 0.18 },
  },
  // The Monument to the Postmortem (cq_moonshot): very low, very slow, bronze, a big hall.
  statue: {
    pitch: 66, rate: 0.6, shift: 0.88, robot: 0.05, excite: 0.5, tilt: 0.35, presence: 0.4, seed: 31,
    layers: [{ pitch: 1, gain: 1 }, { pitch: 0.5, gain: 0.35 }],
    drive: 0.15, fxMix: 0.4, reverb: { seconds: 3, decay: 2.2, wet: 0.22 },
  },
  // MVP-1's captain (Star Slop): low, unhurried, through the cockpit's cheap intercom.
  captain: {
    pitch: 88, rate: 0.8, shift: 0.93, robot: 0.12, excite: 0.65, tilt: 0.3, presence: 0.85, seed: 23,
    layers: [{ pitch: 1, gain: 1 }],
    drive: 0.3, crush: { bits: 6, hold: 2 }, fxMix: 0.45, reverb: { seconds: 0.6, decay: 5, wet: 0.06 },
  },
  // The gate agent (Gate 0): bright, brisk, the terminal's big hall.
  gate: {
    pitch: 158, rate: 1.0, shift: 1.12, robot: 0.1, excite: 0.75, tilt: 0.3, presence: 0.9, seed: 29,
    layers: [{ pitch: 1, gain: 1 }],
    drive: 0.2, crush: { bits: 8, hold: 1 }, fxMix: 0.25, reverb: { seconds: 2.6, decay: 2.6, wet: 0.24 },
  },
  // The Lunar Office Campus PA: HR's voice, smooth, warm and a little too synthetic.
  hr: {
    pitch: 140, rate: 0.84, shift: 1.08, robot: 0.3, excite: 0.6, tilt: 0.45, presence: 0.7, seed: 31,
    layers: [{ pitch: 1, gain: 1 }, { pitch: 2, gain: 0.12 }],
    drive: 0.15, fxMix: 0.3, reverb: { seconds: 2.2, decay: 2.8, wet: 0.2 },
  },
  // The race caller (race mode): fast, bright and over-excited, a stadium PA's hall.
  caller: {
    pitch: 138, rate: 1.12, shift: 1.06, robot: 0.08, excite: 1, tilt: 0.25, presence: 0.85, seed: 19,
    layers: [{ pitch: 1, gain: 1 }, { pitch: 0.5, gain: 0.25 }],
    drive: 0.45, fxMix: 0.35, reverb: { seconds: 1.8, decay: 2.8, wet: 0.16 },
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
  // GPU Drop: the store PA (a chime, then the line) and the round ends (the voice says these
  // instead of <team>_win).
  gpu_grabbed: { color: "#ff3df0", text: "Attention shoppers: someone grabbed a GPU!", ph: "AX T EH1 N SH AX N _ SH AA1 P ER0 Z , S AH1 M W AH2 N _ G R AE1 B D _ AX _ JH IY1 P IY1 Y ^UW1", voice: "pa", chime: true },
  gpu_secured: { color: "#ff3df0", text: "GPU secured!", ph: "JH IY1 P IY1 Y UW1 _ S IH0 K Y ^UH1 R D", voice: "pa", chime: true },
  out_of_stock: { color: "#ff4f4f", text: "Out of stock!", ph: "AW1 T _ AX V _ S T ^AA1 K", voice: "pa", chime: true },
  restocked: { color: "#ff9a3d", text: "Restocked. Thank you for shopping!", ph: "R IY0 S T ^AA1 K T , TH AE1 NG K _ Y UW0 _ F ER0 _ SH ^AA1 P IH0 NG", voice: "pa", chime: true },
  scalpers_win: { color: "#ff3df0", text: "Scalpers win!", ph: "S K AE1 L P ER0 Z , W ^IH1 N", banner: true },
  staff_wins: { color: "#ff9a3d", text: "Staff wins!", ph: "S T AE1 F , W ^IH1 N Z", banner: true },
  // Star Slop (docs/starslop.md): the ticket gate, and the safety demo as MVP-1's doors close.
  tickets_please: { color: "#ffb627", text: "Tickets please... thank you, enjoy your sprint.", ph: "T IH1 K IH0 T S _ P L ^IY1 Z , TH AE1 NG K _ Y UW0 , EH0 N JH ^OY1 _ Y ER0 _ S P R ^IH1 N T", voice: "conductor" },
  security_check: { color: "#ffb627", text: "Please remove grenades, laptops and opinions from your bag.", ph: "P L ^IY1 Z _ R IH0 M ^UW1 V _ G R AH0 N ^EY1 D Z , L ^AE1 P T AA2 P S , AE1 N D _ AX P ^IH1 N Y AX N Z , F R AH1 M _ Y ER0 _ B ^AE1 G", voice: "conductor" },
  // MVP-1's crew and Gate 0 (game/ships.js createCrew): the gate's calls, the captain at
  // the doors and over the drop, the cabin crew arming the pods.
  gate_boarding: { color: "#ffb627", text: "Now boarding MVP-1 to the Moon. All sprint groups, please.", ph: "N ^AW1 _ B ^AO1 R D IH0 NG _ EH1 M _ V IY1 _ P IY1 _ W ^AH1 N _ T UW0 _ DH AX _ M ^UW1 N , AO1 L _ S P R ^IH1 N T _ G R ^UW1 P S , P L ^IY1 Z", voice: "gate", chime: true },
  gate_delayed: { color: "#ffb627", text: "MVP-1 is delayed due to budget review. It will depart on time anyway. Now boarding.", ph: "EH1 M _ V IY1 _ P IY1 _ W ^AH1 N _ IH1 Z _ D IH0 L ^EY1 D _ D UW1 _ T UW0 _ B ^AH1 JH IH0 T _ R IY0 V Y ^UW1 , IH1 T _ W IH1 L _ D IH0 P ^AA1 R T _ AA1 N _ T ^AY1 M _ ^EH1 N IY0 W EY2 , N ^AW1 _ B ^AO1 R D IH0 NG", voice: "gate", chime: true },
  // The statue at its plinth (Map.VoiceSpots, game/ships.js createVoiceSpots): an easter egg.
  sigh: { color: "#c8a165", text: "...sigh. Who merged on a Friday?", ph: "HH HH HH S ^AY1 AY1 AY1 HH HH , HH ^UW1 _ M ^ER1 JH D _ AA1 N _ AX _ F R ^AY1 D EY2", voice: "statue" },
  gate_final: { color: "#ffb627", text: "Final call for MVP-1. Unmerged passengers will be left behind.", ph: "F ^AY1 N AX L _ K ^AO1 L _ F ER0 _ EH1 M _ V IY1 _ P IY1 _ W ^AH1 N , AH0 N M ^ER1 JH D _ P ^AE1 S AX N JH ER0 Z _ W IH1 L _ B IY1 _ L ^EH1 F T _ B IH0 HH ^AY1 N D", voice: "gate", chime: true },
  captain_welcome: { color: "#9fd0ff", text: "This is your captain speaking. We are cleared for launch, pending code review.", ph: "DH IH1 S _ IH1 Z _ Y ER0 _ K ^AE1 P T AX N _ S P ^IY1 K IH0 NG , W IY1 _ AA1 R _ K L ^IH1 R D _ F ER0 _ L ^AO1 N CH , P ^EH1 N D IH0 NG _ K ^OW1 D _ R IY0 V Y ^UW1", voice: "captain" },
  captain_descent: { color: "#9fd0ff", text: "We are beginning our descent. Your seat is about to become a door.", ph: "W IY1 _ AA1 R _ B IH0 G ^IH1 N IH0 NG _ AW1 ER0 _ D IH0 S ^EH1 N T , Y AO1 R _ S ^IY1 T _ IH1 Z _ AX B ^AW1 T _ T UW0 _ B IH0 K ^AH1 M _ AX _ D ^AO1 R", voice: "captain" },
  crew_pods: { color: "#ffb627", text: "Cabin crew, arm the drop pods. Thank you for flying Minimum Viable Payload.", ph: "K ^AE1 B IH0 N _ K R ^UW1 , ^AA1 R M _ DH AX _ D R ^AA1 P _ P ^AA1 D Z , TH AE1 NG K _ Y UW0 _ F ER0 _ F L ^AY1 IH0 NG _ M ^IH1 N AX M AX M _ V ^AY1 AX B AX L _ P ^EY1 L OW2 D", voice: "conductor" },
  // The Lunar Office Campus PA (HR): visitors landing, staff respawning, meeting rooms lost and won back.
  hr_welcome: { color: "#c9a0ff", text: "Welcome to the Lunar Office Campus. Please badge in. Remote work is not supported at this altitude.", ph: "W ^EH1 L K AX M _ T UW0 _ DH AX _ L ^UW1 N ER0 _ ^AO1 F IH0 S _ K ^AE1 M P AX S , P L ^IY1 Z _ B ^AE1 JH _ IH1 N , R IH0 M ^OW1 T _ W ^ER1 K _ IH1 Z _ N AA1 T _ S AX P ^AO1 R T IH0 D _ AE1 T _ DH IH1 S _ ^AE1 L T IH0 T UW2 D", voice: "hr", chime: true },
  hr_rto: { color: "#c9a0ff", text: "Good morning, colleagues. Return to office is mandatory. Your desk is under attack.", ph: "G UH1 D _ M ^AO1 R N IH0 NG , K ^AA1 L IY0 G Z , R IH0 T ^ER1 N _ T UW0 _ ^AO1 F IH0 S _ IH1 Z _ M ^AE1 N D AX T AO2 R IY0 , Y AO1 R _ D ^EH1 S K _ IH1 Z _ AH1 N D ER0 _ AX T ^AE1 K", voice: "hr", chime: true },
  hr_breach: { color: "#c9a0ff", text: "Attention, colleagues. Earth has booked one of our meeting rooms. Please escalate.", ph: "AX T ^EH1 N SH AX N , K ^AA1 L IY0 G Z , ^ER1 TH _ HH AE1 Z _ B ^UH1 K T _ W AH1 N _ AX V _ AW1 ER0 _ M ^IY1 T IH0 NG _ R ^UW1 M Z , P L ^IY1 Z _ ^EH1 S K AX L EY2 T", voice: "hr", chime: true },
  hr_reclaimed: { color: "#c9a0ff", text: "Thank you. The meeting room is back with its rightful owners: middle management.", ph: "TH AE1 NG K _ Y UW0 , DH AX _ M ^IY1 T IH0 NG _ R ^UW1 M _ IH1 Z _ B ^AE1 K _ W IH1 DH _ IH1 T S _ R ^AY1 T F AX L _ ^OW1 N ER0 Z , M ^IH1 D AX L _ M ^AE1 N IH0 JH M AX N T", voice: "hr", chime: true },
  safety_demo: { color: "#ffb627", text: "In case of decompression, blame the last commit.", ph: "IH0 N _ K EY1 S _ AX V _ D IY2 K AX M P R ^EH1 SH AX N , B L ^EY1 M _ DH AX _ L AE1 S T _ K AX M ^IH1 T", voice: "conductor" },
  // Slop n Roll Racing (game/raceannouncer.js): the grid, the lead, wrecks, the last lap, the finish.
  race_start: { color: "#ffd84f", text: "Start your linters!", ph: "S T AA1 R T _ Y ER0 _ L ^IH1 N T ER0 Z", voice: "caller" },
  race_three: { color: "#ffd84f", text: "Three!", ph: "TH R ^IY1", voice: "caller" },
  race_two: { color: "#ffd84f", text: "Two!", ph: "T ^UW1", voice: "caller" },
  race_one: { color: "#ffd84f", text: "One!", ph: "W ^AH1 N", voice: "caller" },
  race_go: { color: "#8bff4f", text: "Go! Ship it!", ph: "G ^OW1 , SH ^IH1 P IH0 T", voice: "caller" },
  race_lead: { color: "#8bff4f", text: "You take the lead!", ph: "Y UW1 _ T EY1 K _ DH AX _ L ^IY1 D", voice: "caller" },
  race_lead_lost: { color: "#ff9a3d", text: "Lead lost! Rebase and recover!", ph: "L IY1 D _ L ^AO1 S T , R IY0 B ^EY1 S _ AE1 N D _ R IY0 K ^AH1 V ER0", voice: "caller" },
  race_new_leader: { color: "#4fd1ff", text: "New leader on the board!", ph: "N UW1 _ L IY1 D ER0 _ AA1 N _ DH AX _ B ^AO1 R D", voice: "caller" },
  race_wrecked: { color: "#ff4f4f", text: "Total write-off!", ph: "T OW1 T AX L _ R AY1 T _ ^AO1 F", voice: "caller" },
  race_wreck: { color: "#ff9a3d", text: "Wreck! Somebody file a ticket!", ph: "R ^EH1 K , S AH1 M B AA2 D IY0 _ F AY1 L _ AX _ T ^IH1 K IH0 T", voice: "caller" },
  race_last_lap: { color: "#ffd84f", text: "Final lap! Last call for merges!", ph: "F AY1 N AX L _ L ^AE1 P , L AE1 S T _ K AO1 L _ F ER0 _ M ^ER1 JH IH0 Z", voice: "caller" },
  race_winner: { color: "#8bff4f", text: "Winner! First to production!", ph: "W ^IH1 N ER0 , F ER1 S T _ T UW0 _ P R AX D ^AH1 K SH AX N", voice: "caller" },
  race_podium: { color: "#4fd1ff", text: "Podium! Shipped, with caveats!", ph: "P ^OW1 D IY0 AX M , SH IH1 P T , W IH0 DH _ K ^AE1 V IY0 AE2 T S", voice: "caller" },
  race_finished: { color: "#cfcfcf", text: "Finished. Merged, eventually.", ph: "F ^IH1 N IH0 SH T , M ER1 JH D , IH0 V EH1 N CH UW0 AX L ^IY0", voice: "caller" },
  race_pit: { color: "#5cff8a", text: "Pit stop! Patch in progress.", ph: "P IH1 T _ S T ^AA1 P , P ^AE1 CH _ IH0 N _ P R AA1 G R EH2 S", voice: "caller" },
  // Race power-ups (internal/game pickups.go): the caller names what you drove through.
  pu_bonus: { color: "#ffe14f", text: "Bonus approved!", ph: "B ^OW1 N AX S _ AX P R ^UW1 V D", voice: "caller" },
  pu_hotfix: { color: "#5cff8a", text: "Hotfix patched in!", ph: "HH AA1 T F IH2 K S _ P ^AE1 CH T _ IH1 N", voice: "caller" },
  pu_reorg: { color: "#ffb84f", text: "Reorg budget: reloaded!", ph: "R IY0 AO1 R G _ B AH1 JH AX T , R IY0 L ^OW1 D AX D", voice: "caller" },
  pu_caffeine: { color: "#ff8a3d", text: "Caffeine!", ph: "K AE0 F ^IY1 N", voice: "caller" },
  pu_freeze: { color: "#7fdcff", text: "Code freeze! You can't touch this.", ph: "K OW1 D _ F R ^IY1 Z , Y UW1 _ K AE1 N T _ T ^AH1 CH _ DH IH1 S", voice: "caller" },
  pu_flag: { color: "#ff3df0", text: "Feature flag flipped!", ph: "F IY1 CH ER0 _ F L AE1 G _ F L ^IH1 P T", voice: "caller" },
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

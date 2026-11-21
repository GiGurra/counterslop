// Radio callouts (Z menu): what each says, as SlopVox phonemes, spoken in the
// "radio" voice to the sender's team. Ids are the wire format ({t:"radio", rid}).
export const RADIO = [
  { id: "need_backup", text: "Need backup, my context is full!", ph: "N IY1 D _ B ^AE1 K AH2 P _ M AY1 _ K AA1 N T EH2 K S T _ IH1 Z _ F ^UH1 L" },
  { id: "enemy_spotted", text: "Enemy spotted!", ph: "EH1 N AX M IY0 _ S P ^AA1 T IH0 D" },
  { id: "ship_it", text: "Ship it!", ph: "SH ^IH1 P _ IH1 T" },
  { id: "code_freeze", text: "Code freeze here. Hold.", ph: "K OW1 D _ F R ^IY1 Z _ HH IY1 R _ _ HH ^OW1 L D" },
  { id: "lgtm", text: "LGTM.", ph: "EH1 L _ JH IY1 _ T IY1 _ ^EH1 M" },
  { id: "changes_requested", text: "Changes requested.", ph: "CH EY1 N JH IH0 Z _ R IH0 K W ^EH1 S T IH0 D" },
  { id: "revert", text: "Revert! Revert!", ph: "R IH0 V ^ER1 T _ R IH0 V ^ER1 T" },
  { id: "pair_with_me", text: "Pair with me.", ph: "P ^EH1 R _ W IH1 DH _ M IY1" },
  { id: "tests_green", text: "Tests are green. Clear.", ph: "T EH1 S T S _ AA1 R _ G R ^IY1 N _ _ K L ^IY1 R" },
];
export const RADIO_BY_ID = Object.fromEntries(RADIO.map((r) => [r.id, r]));

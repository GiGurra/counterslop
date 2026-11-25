// The death cam caption: who (or what) ended you decides the words.
// Pure: deathCaption({ killer, killerTeam, myTeam, weapon, self }) ->
// { title, line }, line a list of plain strings and highlighted parts
// ({ b: text, team }: the HUD colours a part by its team, if it has one).
//
//   the Big Diff (the bomb's blast)  PROD WENT DOWN        you were in prod when it went down
//   suicide, no killer (own nade)    SEGFAULT              you crashed yourself
//   a teammate                       BROKE THE BUILD       your own teammate X with Y
//   a vibe coder kills an artisan    DEPRECATED            X deprecated you with Y
//   anything else (an artisan kills
//   a vibe coder, killer left)       REVIEWED              you got reviewed by X with Y
export function deathCaption({ killer = "", killerTeam = "", myTeam = "", weapon = "", weaponName = "", self = false }) {
  const who = { b: killer, team: killerTeam };
  const with_ = weaponName ? [" with ", { b: weaponName }] : [];
  if (weapon === "bigdiff") return { title: "PROD WENT DOWN", line: ["you were in prod when it went down"] };
  if (self || !killer) return { title: "SEGFAULT", line: ["you crashed yourself", ...with_] };
  if (killerTeam && killerTeam === myTeam) return { title: "BROKE THE BUILD", line: ["your own teammate ", who, ...with_] };
  if (killerTeam === "vibe" && myTeam === "artisan") return { title: "DEPRECATED", line: [who, " deprecated you", ...with_] };
  return { title: "REVIEWED", line: ["you got reviewed by ", who, ...with_] };
}

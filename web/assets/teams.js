// The two sides. Team ids match internal/content (Vibe, Artisan).
import { PALETTE as P } from "./palette.js";
import { rgbHex } from "../gen/color.js";

export const TEAMS = {
  vibe: {
    id: "vibe", name: "Vibe Coders", short: "VIBE", role: "attackers",
    rgb: P.vibeNeon, color: rgbHex(P.vibeNeon),
    wins: ["Merged without review", "Shipped to prod"],
    taunts: ["it works on my machine", "the AI said it was fine", "tests are for people who make mistakes"],
  },
  artisan: {
    id: "artisan", name: "Artisans", short: "ARTS", role: "defenders",
    rgb: P.artisanCopper, color: rgbHex(P.artisanCopper),
    wins: ["Reverted", "Changes requested"],
    taunts: ["every line hand-typed", "small-batch, organic code", "did you even read the man page?"],
  },
};

export const TEAM_IDS = ["vibe", "artisan"];

// Per-mode display names: the same two sides under another name (Star Slop's
// tickets mode: Earth attacks the Moon). setTeamMode(map.mode) on every map;
// everything that shows a team's name reads TEAMS, so it follows.
const MODE_NAMES = {
  tickets: { vibe: { name: "Earth", short: "EARTH" }, artisan: { name: "Moon", short: "MOON" } },
};
const BASE = Object.fromEntries(Object.entries(TEAMS).map(([id, t]) => [id, { name: t.name, short: t.short }]));
export function setTeamMode(mode) {
  for (const id of Object.keys(TEAMS)) Object.assign(TEAMS[id], BASE[id], MODE_NAMES[mode]?.[id] || {});
}
export const team = (id) => TEAMS[id] || null;

// Not a side: no body, can't shoot or be shot. New players start here.
export const SPECTATOR = "spec";

// Can someone join team id without it ending up 2+ bigger than the other side?
// counts: {teamId: players}, not counting the joining player. Mirrors the server.
export const teamOpen = (id, counts) => TEAM_IDS.every((o) => o === id || (counts[id] || 0) + 1 - (counts[o] || 0) < 2);

// Is team id at the map's cap (cap per team, 0 = none) for a human? humans:
// {teamId: human players}, not counting the joining player; a bot gives up its
// place to a human, so only humans fill a side. Mirrors the server (teamcap.go).
export const teamCapped = (id, humans, cap) => cap > 0 && (humans[id] || 0) >= cap;

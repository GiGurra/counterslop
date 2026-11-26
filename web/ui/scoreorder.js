// Scoreboard order: points, kills, then name (and id for equal names), so rows
// that tie keep one fixed order (the player list's order changes between
// snapshots, which made tied bots flicker up and down).
export const byScore = (a, b) =>
  (b.sp || 0) - (a.sp || 0) || (b.k || 0) - (a.k || 0) || String(a.name).localeCompare(String(b.name)) || a.id - b.id;

// Players of a team still alive this round (the scoreboard header's "n alive").
export const aliveCount = (members) => members.filter((p) => !p.dead).length;

// The scoreboard's rows from a full snapshot's players (everyone but us), in id
// order: the snapshot's own order changes from one to the next, and the HUD
// only re-renders when this list changes (main.js compares it as JSON).
export const hudPlayers = (players, myId) => players.filter((p) => p.id !== myId)
  .map(({ id, name, color, team, k, d, a, sp, dead, ping, ai, bot }) => ({ id, name, color, team, k: k || 0, d: d || 0, a: a || 0, sp: sp || 0, dead: !!dead, ping: ai || bot ? "BOT" : ping || null, ...(ai ? { ai: true } : {}) }))
  .sort((x, y) => x.id - y.id);

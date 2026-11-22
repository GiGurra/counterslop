// Scoreboard order: points, kills, then name (and id for equal names), so rows
// that tie keep one fixed order (the player list's order changes between
// snapshots, which made tied bots flicker up and down).
export const byScore = (a, b) =>
  (b.sp || 0) - (a.sp || 0) || (b.k || 0) - (a.k || 0) || String(a.name).localeCompare(String(b.name)) || a.id - b.id;

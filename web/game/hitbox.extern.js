// Hand-written halves of internal/game/hitbox_shared.go (//gojs:extern).

// A part's team filter from JSON (Go teamSet.UnmarshalJSON): absent = null
// (everyone), a string = that team, a list = any of those.
export function teamSet_fromJSON(v) {
  if (v == null) return null;
  return Array.isArray(v) ? v.slice() : [v];
}

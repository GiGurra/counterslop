// Buy menu rules that need no UI (tested in node): what you already own.
// s: the HUD state (armor, helmet, kit, slots from arsenal.js hudSlots: gun rows
// carry the weapon id, the grenade row "nades" {id: count}).

// Owned items show greyed out as "owned". Grenades count as owned once you
// carry as many as you may (weapons.json mag: 1 HE, 2 flashes, 1 smoke).
export function owned(it, s) {
  if (it.id === "vest") return s.armor >= 100;
  if (it.id === "helmet") return s.armor >= 100 && s.helmet;
  if (it.id === "kit") return s.kit;
  if (it.grenade) return ((s.slots || []).find((x) => x?.nades)?.nades[it.id] || 0) >= (it.mag || 1);
  return (s.slots || []).some((x) => x && x.id === it.id);
}

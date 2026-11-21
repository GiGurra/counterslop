// Buy menu rules that need no UI (tested in node): what you already own.
// s: the HUD state (armor, helmet, kit, slots from arsenal.js hudSlots: gun rows
// carry the weapon id, the grenade row "nades" {id: count}).
import econ from "../assets/economy.json" with { type: "json" };

// Owned items show greyed out as "owned". Grenades count as owned once you
// carry as many as you may (weapons.json mag: 1 HE, 2 flashes, 1 smoke).
export function owned(it, s) {
  if (it.id === "vest") return s.armor >= 100;
  if (it.id === "helmet") return s.armor >= 100 && s.helmet;
  if (it.id === "kit") return s.kit;
  if (it.id === "shades") return !!s.shades;
  if (it.id === "drink") return s.boost > 0; // one at a time: buy the next when it wears off
  if (it.grenade) return ((s.slots || []).find((x) => x?.nades)?.nades[it.id] || 0) >= (it.mag || 1);
  return (s.slots || []).some((x) => x && x.id === it.id);
}

// What an item costs you now (the server's purchase() charges the same).
export function itemPrice(it, s) {
  if (s.warmup) return 0;
  if (it.id === "helmet" && s.armor >= 100 && it.upgrade) return it.upgrade;
  if (it.id === "helmet" && s.helmet) return econ.gear.find((g) => g.id === "vest")?.price ?? it.price; // only the vest is worn: CS charges the vest
  return it.price;
}

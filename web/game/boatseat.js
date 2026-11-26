// What the HUD says about boats (ui/hud.js BoatBadge): our role in the boat
// we ride (the driver, a passenger in a seat, standing on its deck), or a
// boat within E's reach on foot (internal/game boats.go boardBoat). Pure.
import { BoardReach, SeatDeck } from "./shared.gen.js";

// me: our body (ride, seat, boat: {driven}, dead); boats: the snapshot's
// [{id, p}]. Returns {role: "driver" | "passenger" | "deck" | "near", seat,
// tiller: the driver's seat is free} or null.
export function boatHud(me, boats) {
  if (!me || me.dead) return null;
  if (me.ride && me.boat) {
    const tiller = !me.boat.driven;
    if (me.seat === SeatDeck) return { role: "deck", seat: 0, tiller };
    return me.seat === 0 ? { role: "driver", seat: 0, tiller: false } : { role: "passenger", seat: me.seat, tiller };
  }
  for (const b of boats || []) {
    if (Math.hypot(b.p[0] - me.pos[0], b.p[2] - me.pos[2]) < BoardReach && Math.abs(b.p[1] - me.pos[1]) < 2) return { role: "near", seat: 0, tiller: false };
  }
  return null;
}

// Its words: a title (null: none) and the key hints (k: an action's key name).
export function boatWords(b, k) {
  if (!b) return null;
  switch (b.role) {
    case "driver": return { title: "DRIVER", tag: "you own the roadmap", keys: [`${k("+forward")}/${k("+back")} throttle`, `${k("+moveleft")}/${k("+moveright")} tiller`, `${k("+seat")} passenger seat`, `${k("+use")} stand up`] };
    case "passenger": return { title: `PASSENGER · SEAT ${b.seat}`, tag: "along for the ride", keys: [b.tiller ? `${k("+seat")} take the tiller` : "the tiller is taken", `${k("+use")} stand up`] };
    case "deck": return { title: "STANDING ROOM ONLY", tag: "on deck", keys: [`${k("+use")} sit down`, "walk off the side to leave"] };
  }
  return { title: null, tag: "", keys: [`${k("+use")} sit down (dinghy)`] };
}

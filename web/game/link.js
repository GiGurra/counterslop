// Is the server still there? From the newest snapshot and net.js's reconnect
// state, decide what the player is told (pure: the caller passes the clock):
//   ok    - snapshots are flowing, or we're not meant to be connected (a
//           deliberate disconnect), or the very first connect is under way;
//   blip  - no snapshot for BLIP_MS: a small banner, the game stays playable;
//   lost  - no snapshot for LOST_MS on an open socket ("stall"), or the socket
//           closed / a reconnect failed ("closed"): the full-screen overlay.
// The server sends ~33 snapshots a second to every connection (observers in
// the menus too), so a quiet socket really is a dead one. After STALL_MS of
// silence the caller drops it and reconnects (a half-dead TCP link can stay
// "open" for minutes).
export const BLIP_MS = 500, LOST_MS = 2000, STALL_MS = 8000;

export function createLink({ blipMs = BLIP_MS, lostMs = LOST_MS } = {}) {
  let since = null; // clock ms of the newest sign of life on this socket (it opened, a snapshot)
  return {
    snap(now) { since = now; },
    // net: { wanted, open, attempt, retryAt } (engine/net.js); wallNow: the
    // clock retryAt is on (performance.now()), for the countdown.
    status(now, net, wallNow = now) {
      if (!net.wanted) { since = null; return { mode: "ok" }; }
      if (!net.open) {
        since = null;
        if (!(net.attempt > 0)) return { mode: "ok" }; // first connect still under way
        return { mode: "lost", why: "closed", attempt: net.attempt,
          nextIn: net.retryAt > 0 ? Math.max(0, Math.ceil((net.retryAt - wallNow) / 1000)) : 0 };
      }
      if (since === null) since = now; // it just opened: the clock starts here
      const age = Math.max(0, now - since);
      if (age >= lostMs) return { mode: "lost", why: "stall", age };
      if (age >= blipMs) return { mode: "blip", age };
      return { mode: "ok" };
    },
  };
}

// What the HUD shows (hud.set({ link })): only the parts that change what is
// drawn, so it re-renders at most every 0.1 s (the age) or 1 s (the countdown).
export function linkView(st) {
  if (st.mode === "ok") return null;
  const v = { mode: st.mode, why: st.why || "" };
  if (st.age !== undefined) v.secs = Math.floor(st.age / 100 + 1e-6) / 10; // + epsilon: 42 frames of 1/60 s are 0.7 s, not 0.6999
  if (st.why === "closed") { v.attempt = st.attempt; v.nextIn = st.nextIn; }
  return v;
}

export const sameView = (a, b) => (a === b) || (!!a && !!b && a.mode === b.mode && a.why === b.why && a.secs === b.secs && a.attempt === b.attempt && a.nextIn === b.nextIn);

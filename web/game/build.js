// What a page does when the server it reaches runs another client build.
//
// The page knows its own build (stamped into index.html by the server that
// served it) and loads every module from /b/<build>/..., so a reload fetches a
// fresh index.html (no-store) and with it the new build's URLs: no cache can
// hand it old code. A stale page must never keep playing:
//   "ok"     same build (or one side doesn't say): carry on
//   "reload" first meeting with this build: reload the page
//   "bust"   a reload just tried and we're still old (a cache served the
//            page anyway): navigate to a URL nobody has cached
//   "stale"  that failed too: stop, disconnect, tell the player
export const RELOAD_GUARD_MS = 30000;
export const BUST_PARAM = "build";

// last: what the previous attempt stored ({build, at, tries}) or null.
export function buildAction(page, server, last, now, guardMs = RELOAD_GUARD_MS) {
  if (!server || !page || server === page) return "ok";
  const recent = last && last.build === server && now - last.at < guardMs;
  if (!recent) return "reload";
  return (last.tries || 1) < 2 ? "bust" : "stale";
}

// The record to keep (sessionStorage) for an action that navigates.
export function nextAttempt(action, server, last, now) {
  return { build: server, at: now, tries: action === "reload" ? 1 : (last?.tries || 1) + 1 };
}

// The URL a "bust" navigates to: this page with ?build=<server build>.
export function bustURL(href, server) {
  const u = new URL(href);
  u.searchParams.set(BUST_PARAM, server);
  return u.toString();
}

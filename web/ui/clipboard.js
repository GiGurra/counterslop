// Clipboard helpers that also work over plain http (a LAN address or a
// hostname: not a secure context, so no navigator.clipboard). There the
// browser's own copy and paste (Ctrl/Cmd+C/V, Shift+Insert, the context menu)
// still work, and a programmatic copy falls back to execCommand("copy") on a
// hidden textarea, which needs the user's gesture: call copyText straight from
// the click or key handler, with no await before it. Reading the clipboard from
// code has no fallback, so pasting is left to the browser's paste event.

// How copyText will copy: "api" (navigator.clipboard.writeText, secure
// contexts) or "legacy" (execCommand on a hidden textarea).
export function copyMethod(nav = globalThis.navigator) {
  return typeof nav?.clipboard?.writeText === "function" ? "api" : "legacy";
}

// Copy text with a hidden, read-only textarea and execCommand("copy"). The
// element that had focus gets it back (the console input keeps the caret).
export function legacyCopy(text, doc = globalThis.document) {
  const prev = doc.activeElement;
  const area = doc.createElement("textarea");
  area.value = text;
  area.setAttribute("readonly", "");
  area.style.cssText = "position:fixed;left:-9999px;top:0;opacity:0";
  doc.body.append(area);
  area.select();
  let ok = false;
  try { ok = !!doc.execCommand("copy"); } catch { ok = false; }
  area.remove();
  prev?.focus?.({ preventScroll: true });
  return ok;
}

// Put text on the clipboard; resolves whether it got there. The API copy
// starts synchronously (inside the gesture); when it is refused (permission,
// an unfocused page) the legacy copy is tried.
export async function copyText(text, { nav = globalThis.navigator, doc = globalThis.document } = {}) {
  if (copyMethod(nav) === "api") {
    try { await nav.clipboard.writeText(text); return true; } catch { /* refused: try the legacy copy */ }
  }
  try { return legacyCopy(text, doc); } catch { return false; }
}

// Why a copy failed, for the player.
export function copyFailure(secure = globalThis.isSecureContext) {
  return secure === false
    ? "the browser refused the copy (the clipboard is limited over plain http): select the text and press Ctrl+C"
    : "the browser refused the copy: select the text and press Ctrl+C";
}

// Text pasted into a one-line console input: the input's text with the paste
// in place of the selection, cut at line breaks. Complete lines run as if typed
// and Enter pressed; the text after the last line break stays in the input
// (caret: where the cursor goes in it). Which lines run:
//   - a copied console log (some lines start with the "] " of an echo): only
//     those echoed commands, never the output between them (a help row starts
//     with a command's name too);
//   - anything else (a config): every line whose first word isCmd.
// The rest is skipped (counted in skipped). Blank lines don't count.
export function pasteLines(before, pasted, after, isCmd = () => true) {
  const parts = (before + pasted.replace(/\r\n?/g, "\n") + after).split("\n");
  const keep = parts.pop();
  const lines = parts.map((l) => l.trim()).filter(Boolean);
  const echoed = lines.some((l) => l.startsWith("]"));
  const run = lines.filter((l) => !echoed || l.startsWith("]")).map((l) => l.replace(/^\]\s?/, "").trim()).filter((l) => l && isCmd(l.split(/\s+/)[0]));
  const caret = Math.max(0, keep.length - after.length);
  return { run, skipped: lines.length - run.length, keep, caret };
}

// The last n lines (all when n is missing, not a number or < 1).
export function lastLines(lines, n) {
  const k = Math.floor(Number(n));
  return k >= 1 ? lines.slice(-k) : lines.slice();
}

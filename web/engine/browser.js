// What this browser can and can't do for a shooter, and the one-line advice
// for what it can't (pure: takes the user agent / navigator, for tests).

// Firefox's major version, or 0 for any other browser.
export function firefoxVersion(ua = globalThis.navigator?.userAgent ?? "") {
  const m = /Firefox\/(\d+)/.exec(ua);
  return m ? +m[1] : 0;
}

// Raw input was asked for but the mouse was captured without it (input.raw
// === false): moves are OS-accelerated, so the same sensitivity turns more on
// a fast flick than a slow one. Firefox takes raw input (unadjustedMovement)
// from 152 on Windows and macOS only.
export function rawInputHint(ua = globalThis.navigator?.userAgent ?? "") {
  const ff = firefoxVersion(ua), os = "turn off mouse acceleration in your OS (Windows: \"Enhance pointer precision\")";
  if (ff && ff < 152) return `Firefox ${ff} has no raw mouse input (152+ on Windows/macOS has): update Firefox, or ${os}.`;
  if (ff && /Linux|X11/.test(ua)) return `Firefox on Linux has no raw mouse input: use a flat acceleration profile (libinput "flat").`;
  return `This browser gave no raw mouse input, so the OS accelerates your mouse: ${os}.`;
}

// Keyboard Lock (navigator.keyboard.lock, Chromium only): in fullscreen,
// Ctrl+W/T/N reach the game instead of closing the tab or opening windows.
export const hasKeyboardLock = (nav = globalThis.navigator) => typeof nav?.keyboard?.lock === "function";

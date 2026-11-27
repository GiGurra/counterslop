// Is a clickable menu on screen? Then the mouse cursor must stay visible: no
// pointer lock (main menu, Esc menu, options, controls, maps, team select, buy
// menu, the race garage, the no-keyboard-lock crouch notice, the connection-lost overlay). Pressing PLAY used to lock the pointer just as team select opened.
export const menuWantsCursor = (s) => s.screen !== "playing" || !!s.teamMenu || !!s.buyMenu || !!s.garageOpen || !!s.crouchNotice || !!s.tickets?.spawn || s.link?.mode === "lost";

// The pointer unlocked during play: is that the player's Esc (open the menu)?
// Not when the game released it itself for the console, team select, the buy
// menu or chat (Y opened chat and the main menu on top of it), or the connection-lost overlay.
// Playing without the mouse captured, nothing up that takes the keys or the
// cursor: the HUD says "click to aim" and a key press (user activation, as a
// click is) asks for the lock again (engine/input.js aimOnKey). The browser
// may refuse a lock asked for without one: back from the tickets spawn screen
// after an auto-respawn, or from the Esc menu.
export const wantsAim = (s) => !menuWantsCursor(s) && !s.aiming && !s.replay && !s.console && !s.chatOpen;

// An Esc that just closed an overlay (the buy menu, the garage, team select)
// is that overlay's, not the player's "pause" (CS2: Esc in the buy menu only
// closes it; a second Esc opens the menu). The browser still drops the
// pointer lock on that Esc, and an Esc keydown can reach the page after the
// overlay closed, so neither opens the menu within this window of the close.
export const OVERLAY_ESC_MS = 600;
export const escClosedOverlay = (s, now) => now - (s.overlayClosedAt ?? -Infinity) < OVERLAY_ESC_MS;

export const unlockOpensMenu = (s, now = Infinity) => !escClosedOverlay(s, now) && s.screen === "playing" && !s.console && !s.teamMenu && !s.buyMenu && !s.garageOpen && !s.chatOpen && !s.tickets?.spawn && s.link?.mode !== "lost";

// A map change closes the in-game menu (Esc menu, its Maps / Options views): you
// picked the map there, or a vote or an admin changed it; either way the next
// thing to see is the loading screen and then the match. Not the title menu
// before you have joined (inMatch false). Pointer lock needs a gesture, so the
// HUD says "click to aim" and the next key or click aims (wantsAim).
export const menuClosesOnMapChange = (s, inMatch) => !!inMatch && s.screen === "ingame";

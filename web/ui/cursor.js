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

export const unlockOpensMenu = (s) => s.screen === "playing" && !s.console && !s.teamMenu && !s.buyMenu && !s.garageOpen && !s.chatOpen && !s.tickets?.spawn && s.link?.mode !== "lost";

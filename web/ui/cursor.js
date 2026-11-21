// Is a clickable menu on screen? Then the mouse cursor must stay visible: no
// pointer lock (main menu, Esc menu, options, controls, maps, team select, buy
// menu). Pressing PLAY used to lock the pointer just as team select opened.
export const menuWantsCursor = (s) => s.screen !== "playing" || !!s.teamMenu || !!s.buyMenu;

// The pointer unlocked during play: is that the player's Esc (open the menu)?
// Not when the game released it itself for the console, team select, the buy
// menu or chat (Y opened chat and the main menu on top of it).
export const unlockOpensMenu = (s) => s.screen === "playing" && !s.console && !s.teamMenu && !s.buyMenu && !s.chatOpen;

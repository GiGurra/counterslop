// Is a clickable menu on screen? Then the mouse cursor must stay visible: no
// pointer lock (main menu, Esc menu, options, controls, maps, team select, buy
// menu). Pressing PLAY used to lock the pointer just as team select opened.
export const menuWantsCursor = (s) => s.screen !== "playing" || !!s.teamMenu || !!s.buyMenu;

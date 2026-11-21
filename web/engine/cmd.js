// Per-tick input commands: what the client samples once per sim tick and what
// the server-authoritative sim will receive. Shape:
//   {seq, ms, f, r, yaw, pitch, b}
//   seq    increasing command number
//   ms     tick length in ms (1000 / simHz)
//   f, r   move axes -1..1 (forward, right)
//   yaw, pitch  view angles at the tick
//   b      button bits (BTN), the *held* state; edges come from comparing with
//          the previous command's b, on the client and on the server alike.
export const BTN = { JUMP: 1, CROUCH: 2, WALK: 4, FIRE: 8, SCOPE: 16, RELOAD: 32 }; // = internal/game Btn*

// Physics input for one command. A jump fires only on the JUMP down edge
// (holding it never re-jumps on landing) and never while crouching on the
// ground; physics.step jumps on inp.jump when on the ground, and crouching in
// the air (the crouch-jump) is untouched.
export function moveInput(cmd, prevB, body) {
  const b = cmd.b;
  const crouch = !!(b & BTN.CROUCH);
  const edge = !!(b & BTN.JUMP) && !(prevB & BTN.JUMP);
  const crouchedOnGround = body.onGround && (crouch || body.crouched);
  return { f: cmd.f, r: cmd.r, walk: !!(b & BTN.WALK), crouch, jump: edge && !crouchedOnGround };
}

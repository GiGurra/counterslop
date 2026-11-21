package game

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/maps"
)

// gpuServer: GPU Drop on gs_kiosk, one Vibe Coder and one Artisan, the round
// live (no freeze), economy.json loaded.
func gpuServer(t *testing.T) (*Server, *Player, *Player, *time.Time) {
	t.Helper()
	s, v, a := econServer(t)
	s.Weapons = registry(t)
	s.Map = maps.All["gs_kiosk"]()
	s.round = newRound()
	s.cvars["mp_freezetime"] = "0"
	s.cvars["mp_warmup_time"] = "0"
	s.cvars["mp_respawn"] = "0"
	now := time.Now()
	for _, p := range []*Player{v, a} {
		s.queueCmds(p, []Cmd{{Seq: 1}})
	}
	s.simTick(now)
	ticks(s, 3, &now, nil)
	if s.round.Phase != PhaseLive {
		t.Fatalf("phase %q, want live", s.round.Phase)
	}
	return s, v, a, &now
}

// at puts p on the floor at x, z (the stockroom aisle by default).
func at(s *Server, p *Player, x, z float64) { s.teleport(p, Vec3{x, 0, z}, 0, false, p.lastQueued) }

var use = Cmd{B: BtnUse}

func TestGPUDropRoundStart(t *testing.T) {
	s, _, _, now := gpuServer(t)
	r := &s.round
	if len(r.GPUs) != 4 || r.ToWin != 2 || r.Secured != 0 || r.Bomb.State != "" {
		t.Fatalf("round %+v: want 4 GPUs on the shelves, 2 to win, no Big Diff", r)
	}
	for i, g := range r.GPUs {
		if g.State != GPUShelf || g.Pos != Vec3(s.Map.GPUs[i].Pos) {
			t.Fatalf("gpu %d %+v", i, g)
		}
	}
	var msg map[string]any
	json.Unmarshal(s.roundJSON(*now), &msg)
	if msg["mode"] != "gpudrop" || len(msg["gpus"].([]any)) != 4 || msg["toWin"] != 2.0 {
		t.Fatalf("round JSON %v", msg)
	}
	for _, p := range s.players {
		if p.inv.Slots[BombSlot] != nil {
			t.Fatal("someone has The Big Diff in GPU Drop")
		}
	}
}

func TestGPUGrabTakesASecondStandingStill(t *testing.T) {
	s, v, a, now := gpuServer(t)
	at(s, v, 19.3, -3)
	at(s, a, 15, 8)
	money := v.Money
	ticks(s, SimHz-2, now, map[*Player]Cmd{v: use})
	if g := s.round.GPUs[1]; g.State != GPUShelf || g.Worker != v.ID || g.Progress < 0.95 {
		t.Fatalf("after 0.98 s: %+v, want still on the shelf, %d at it", g, v.ID)
	}
	ticks(s, 2, now, map[*Player]Cmd{v: use})
	g := s.round.GPUs[1]
	if g.State != GPUCarried || g.Carrier != v.ID || g.Progress != 0 || g.Worker != 0 {
		t.Fatalf("after 1 s: %+v, want carried by %d", g, v.ID)
	}
	if v.Money != money+200 || !v.inv.carry || !weaponStateOf(v).Carry {
		t.Fatalf("money %d (was %d), carry %v", v.Money, money, v.inv.carry)
	}
	// One per player: holding E on the next one does nothing.
	ticks(s, 2*SimHz, now, map[*Player]Cmd{v: use})
	if s.carriedBy(v) != 1 || s.round.GPUs[0].State != GPUShelf || s.round.GPUs[0].Progress != 0 {
		t.Fatalf("grabbed a second GPU: %+v", s.round.GPUs)
	}
}

func TestGPUGrabResetsAndStaffCantGrab(t *testing.T) {
	s, v, a, now := gpuServer(t)
	at(s, v, 19.3, -3)
	at(s, a, 19.3, 3)
	ticks(s, SimHz/2, now, map[*Player]Cmd{v: use, a: use})
	if g := s.round.GPUs[2]; g.Progress != 0 || g.Worker != 0 {
		t.Fatalf("the Artisan works on a GPU on the shelf: %+v", g)
	}
	ticks(s, 1, now, map[*Player]Cmd{v: {B: BtnUse, F: 1}}) // moving resets it
	if g := s.round.GPUs[1]; g.Progress != 0 || g.Worker != 0 {
		t.Fatalf("moving kept the progress: %+v", g)
	}
	ticks(s, 1, now, map[*Player]Cmd{v: {}})
	at(s, v, 16, -3) // out of reach
	ticks(s, 2*SimHz, now, map[*Player]Cmd{v: use})
	for _, g := range s.round.GPUs {
		if g.State != GPUShelf || g.Worker != 0 {
			t.Fatalf("grabbed from 3 m: %+v", g)
		}
	}
}

func grabbed(t *testing.T, s *Server, v *Player, now *time.Time) int {
	t.Helper()
	at(s, v, 19.3, -3)
	ticks(s, SimHz+1, now, map[*Player]Cmd{v: use})
	i := s.carriedBy(v)
	if i < 0 {
		t.Fatalf("no GPU after holding E for a second: %+v", s.round.GPUs)
	}
	return i
}

func TestGPUCarrierMovesSlower(t *testing.T) {
	s, v, _, now := gpuServer(t)
	free := s.heldSpeed(v)
	grabbed(t, s, v, now)
	if got := s.heldSpeed(v); math.Abs(got-free*0.8) > 1e-9 {
		t.Fatalf("carrying: speed %v, want 0.8 x %v", got, free)
	}
	at(s, v, 0, 7)
	start := v.Pos
	ticks(s, SimHz, now, map[*Player]Cmd{v: {F: 1}})
	slow := math.Hypot(v.Pos[0]-start[0], v.Pos[2]-start[2])
	s.dropGPUAt(v, v.Pos)
	at(s, v, 0, 7)
	ticks(s, SimHz, now, map[*Player]Cmd{v: {F: 1}})
	fast := math.Hypot(v.Pos[0]-start[0], v.Pos[2]-start[2])
	if math.Abs(slow/fast-0.8) > 0.02 {
		t.Fatalf("1 s of running: %.2f m carrying, %.2f m free: ratio %.3f, want 0.8", slow, fast, slow/fast)
	}
}

func TestGPUDropsOnDeathAndOnG(t *testing.T) {
	s, v, a, now := gpuServer(t)
	i := grabbed(t, s, v, now)
	at(s, v, 0, 7)
	ticks(s, 1, now, nil)
	v.inv.Current = "sidearm"
	s.drop(v) // G with a pistol in hand drops the pistol, not the GPU
	if s.carriedBy(v) != i {
		t.Fatal("G with a gun in hand dropped the GPU")
	}
	v.inv.Current = "melee"
	s.drop(v) // nothing droppable in hand: the GPU goes down in front
	g := s.round.GPUs[i]
	if g.State != GPUDropped || g.Carrier != 0 || v.inv.carry || math.Hypot(g.Pos[0]-v.Pos[0], g.Pos[2]-v.Pos[2]) < 1 {
		t.Fatalf("after G: %+v, carry %v", g, v.inv.carry)
	}
	// Any Vibe Coder picks it back up (hold E), then dies with it.
	at(s, v, g.Pos[0]+0.5, g.Pos[2])
	ticks(s, SimHz+1, now, map[*Player]Cmd{v: use})
	if s.carriedBy(v) != i {
		t.Fatalf("didn't pick up the dropped GPU: %+v", s.round.GPUs[i])
	}
	at(s, v, -2, 9)
	ticks(s, 1, now, nil)
	s.killBy(a, v, "prompt", false)
	g = s.round.GPUs[i]
	if g.State != GPUDropped || g.Pos[0] != v.Pos[0] || g.Pos[2] != v.Pos[2] {
		t.Fatalf("after the carrier died at %v: %+v", v.Pos, g)
	}
}

func TestGPURestock(t *testing.T) {
	s, v, a, now := gpuServer(t)
	i := grabbed(t, s, v, now)
	s.dropGPUAt(v, Vec3{4, 0, 0})
	at(s, v, -20, 0)
	at(s, a, 4.5, 0)
	money := a.Money
	ticks(s, 3*SimHz-1, now, map[*Player]Cmd{a: use})
	if g := s.round.GPUs[i]; g.State != GPUDropped || g.Worker != a.ID {
		t.Fatalf("restocked early: %+v", g)
	}
	ticks(s, 1, now, map[*Player]Cmd{a: use})
	g := s.round.GPUs[i]
	if g.State != GPUShelf || g.Pos != Vec3(s.Map.GPUs[i].Pos) || a.Money != money+300 {
		t.Fatalf("after 3 s: %+v money %d (was %d), want back on its shelf, +$300", g, a.Money, money)
	}
}

func TestGPUSecureAndWin(t *testing.T) {
	s, v, a, now := gpuServer(t)
	v2 := s.join("3.3.3.3")
	v2.ready, v2.Team = true, content.Vibe
	s.respawn(v2)
	at(s, v2, -18, 0)
	vm, v2m := v.Money, v2.Money
	i := grabbed(t, s, v, now)
	at(s, v, -22, 0) // the front entrance
	ticks(s, 1, now, nil)
	g := s.round.GPUs[i]
	if g.State != GPUSecured || s.round.Secured != 1 || v.inv.carry || s.round.Phase != PhaseLive {
		t.Fatalf("after the entrance: %+v secured %d phase %s", g, s.round.Secured, s.round.Phase)
	}
	if v.Money != vm+200+1000 || v2.Money != v2m+500 {
		t.Fatalf("money: carrier %d (was %d), team-mate %d (was %d)", v.Money, vm, v2.Money, v2m)
	}
	// The second one wins the round.
	at(s, v, 19.3, 3)
	ticks(s, SimHz+1, now, map[*Player]Cmd{v: use})
	at(s, v, 4, 22) // the loading dock
	ticks(s, 1, now, nil)
	if s.round.Phase != PhaseOver || s.round.Winner != content.Vibe || s.round.Reason != "secured" || s.round.Secured != 2 {
		t.Fatalf("round %s won by %q (%s), %d secured", s.round.Phase, s.round.Winner, s.round.Reason, s.round.Secured)
	}
	_ = a
}

func TestGPUsToWinCvar(t *testing.T) {
	s, v, _, now := gpuServer(t)
	s.cvars["mp_gpus_to_win"] = "9" // more than the stock: all 4
	if s.gpusToWin() != 4 {
		t.Fatalf("gpus to win %d, want the stock (4)", s.gpusToWin())
	}
	s.cvars["mp_gpus_to_win"] = "1"
	s.round.ToWin = s.gpusToWin() // as at the next round start
	grabbed(t, s, v, now)
	at(s, v, -22, 0)
	ticks(s, 1, now, nil)
	if s.round.Phase != PhaseOver || s.round.Reason != "secured" {
		t.Fatalf("mp_gpus_to_win 1: phase %s reason %q after one secure", s.round.Phase, s.round.Reason)
	}
}

func TestGPUDropTimeAndElimination(t *testing.T) {
	s, v, _, now := gpuServer(t)
	grabbed(t, s, v, now) // mid-carry
	*now = s.round.endsAt.Add(time.Millisecond)
	ticks(s, 1, now, nil)
	if s.round.Phase != PhaseOver || s.round.Winner != content.Artisan || s.round.Reason != "time" {
		t.Fatalf("time out mid-carry: %s %q %q", s.round.Phase, s.round.Winner, s.round.Reason)
	}
	// The next round restocks everything.
	*now = now.Add(time.Duration(overSecs+1) * time.Second)
	ticks(s, 3, now, nil)
	for _, g := range s.round.GPUs {
		if g.State != GPUShelf {
			t.Fatalf("round %d: %+v, want every GPU back on its shelf", s.round.Number, s.round.GPUs)
		}
	}
	if v.inv.carry {
		t.Fatal("still slowed in the next round")
	}

	s2, v2, a2, now2 := gpuServer(t)
	s2.killBy(a2, v2, "prompt", false)
	ticks(s2, 1, now2, nil)
	if s2.round.Winner != content.Artisan || s2.round.Reason != "elimination" {
		t.Fatalf("every scalper dead: %q %q", s2.round.Winner, s2.round.Reason)
	}
}

func TestGPUNoGrabInFreezeOrWarmup(t *testing.T) {
	s, v, _, now := gpuServer(t)
	s.round.Phase = PhaseFreeze
	at(s, v, 19.3, -3)
	ticks(s, 2*SimHz, now, map[*Player]Cmd{v: use})
	if s.carriedBy(v) >= 0 || s.round.GPUs[1].Progress != 0 {
		t.Fatal("grabbed in freeze time")
	}
	s.round = newRound()
	s.resetGPUs()
	if len(s.round.GPUs) != 0 {
		t.Fatalf("warmup has GPUs: %+v", s.round.GPUs)
	}
}

func TestDefusalHasNoGPUState(t *testing.T) {
	s, _, _ := roundServer(t)
	now := time.Now()
	ticks(s, 2, &now, nil)
	var msg map[string]any
	json.Unmarshal(s.roundJSON(now), &msg)
	for _, k := range []string{"mode", "gpus", "secured", "toWin"} {
		if _, ok := msg[k]; ok {
			t.Errorf("defusal round state has %q: %v", k, msg[k])
		}
	}
}

// The round tick is the safety net: a carrier who is gone, dead or on the
// other side without going through dropBomb still loses the box where it was.
func TestGPUCarrierGoneDropsIt(t *testing.T) {
	for _, how := range []string{"dead", "switched", "left"} {
		t.Run(how, func(t *testing.T) {
			s, v, _, now := gpuServer(t)
			v2 := s.join("3.3.3.3") // a second scalper: the round goes on without v
			v2.ready, v2.Team = true, content.Vibe
			s.respawn(v2)
			at(s, v2, -18, 0)
			i := grabbed(t, s, v, now)
			at(s, v, 2, 7)
			ticks(s, 1, now, nil)
			switch how {
			case "dead":
				v.Dead = true
			case "switched":
				v.Team = content.Artisan
			case "left":
				delete(s.players, v.ID)
			}
			ticks(s, 1, now, nil)
			if g := s.round.GPUs[i]; g.State != GPUDropped || g.Carrier != 0 || math.Abs(g.Pos[0]-2) > 0.1 {
				t.Fatalf("carrier %s: %+v, want dropped at x 2", how, g)
			}
		})
	}
}

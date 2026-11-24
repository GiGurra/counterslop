package game

import (
	"math"
	"strconv"

	"github.com/gigurra/counterslop/internal/content"
)

// GPU Drop (Map.Mode "gpudrop", gs_ maps; docs/gpu-drop.md): the Vibe Coders
// are scalpers grabbing the last boxed GPUs, the Artisans the store staff
// defending the stockroom. It runs on the shared round code (freeze, buy time,
// halftime, side switches); there is no Big Diff and no planted phase.
//
//   - Grab: a Vibe Coder holds +use standing still within GrabRange of a GPU on
//     the shelf, or a dropped one, for grabSecs. One GPU per player.
//   - Carry: the carrier moves at the economy's carrySpeed (the weapon state's
//     ca, which the client predicts with like the energy drink).
//   - Drop: on death, a team switch or leaving, or G with nothing droppable in
//     hand, the box goes down where the carrier is.
//   - Restock: an Artisan holds +use on a dropped GPU for restockSecs: back
//     to its shelf spot.
//   - Secure: a carrier inside an exit zone secures it (out of play, +1).
//   - Win: mp_gpus_to_win secured (or every Artisan dead) = Vibe Coders;
//     the time runs out (or every Vibe Coder dead) = Artisans.
//
// The round state carries every GPU (st, by, p, and the grab / restock
// progress) in every snapshot; the {t:"gpu"} events are optional extras for
// sounds, the feed and the announcer.

// GPU states (gpuState.State).
const (
	GPUShelf   = "shelf"
	GPUCarried = "carried"
	GPUDropped = "dropped"
	GPUSecured = "secured"

	grabSecs    = 1.0
	restockSecs = 3.0
	gpusToWin   = 2 // mp_gpus_to_win's default
)

type gpuState struct {
	State   string `json:"st" net:"cold"`
	Carrier int    `json:"by,omitempty" net:"cold"` // carrier while carried; who secured it once secured
	Pos     Vec3   `json:"p" net:"hot,mm"`
	// Progress of a grab (Vibe Coder) or restock (Artisan), 0..1, and who.
	Progress float64 `json:"pr,omitempty" net:"hot,exact"`
	Worker   int     `json:"who,omitempty" net:"cold"`
	ticks    int     `net:"-"` // the worker's commands so far
}

// gpuEvent is sent once on a grab, drop, restock or secure (optional: the
// state is in every snapshot). I is the GPU's index in round.gpus.
type gpuEvent struct {
	T  string `json:"t"` // "gpu"
	Ev string `json:"ev"`
	I  int    `json:"i"`
	By int    `json:"by,omitempty"`
	P  Vec3   `json:"p"`
}

// gpuMode: this map plays GPU Drop. Callers hold s.mu.
func (s *Server) gpuMode() bool { return s.Map.Mode == content.ModeGPUDrop }

// gpusToWin: mp_gpus_to_win, at least 1 and at most the map's stock.
func (s *Server) gpusToWin() int {
	n := gpusToWin
	if v, err := strconv.Atoi(s.cvars["mp_gpus_to_win"]); err == nil && v > 0 {
		n = v
	}
	return max(1, min(n, len(s.Map.GPUs)))
}

// resetGPUs restocks every shelf for a new round (and clears it in warmup).
func (s *Server) resetGPUs() {
	r := &s.round
	r.GPUs, r.Secured, r.ToWin = nil, 0, 0
	if !s.gpuMode() || r.Phase == PhaseWarmup {
		s.syncCarry()
		return
	}
	for _, g := range s.Map.GPUs {
		r.GPUs = append(r.GPUs, gpuState{State: GPUShelf, Pos: Vec3(g.Pos)})
	}
	r.ToWin = s.gpusToWin()
	s.syncCarry()
}

// carriedBy is the index of the GPU p carries, or -1.
func (s *Server) carriedBy(p *Player) int {
	for i, g := range s.round.GPUs {
		if g.State == GPUCarried && g.Carrier == p.ID {
			return i
		}
	}
	return -1
}

// syncCarry keeps every inventory's carry flag (the weapon state's ca: the
// slower move speed) in step with the round's GPUs. Callers hold s.mu.
func (s *Server) syncCarry() {
	for _, p := range s.players {
		if p.inv != nil {
			p.inv.carry = s.carriedBy(p) >= 0
		}
	}
}

func (s *Server) gpuSend(ev string, i int, by int) {
	s.sendAll(gpuEvent{T: "gpu", Ev: ev, I: i, By: by, P: s.round.GPUs[i].Pos})
}

// tickGPUs runs every live round tick: carried boxes follow their carriers
// (and drop if the carrier is gone or dead), a carrier inside an exit secures
// theirs, and a worker who died or left loses their progress.
func (s *Server) tickGPUs() {
	r := &s.round
	for i := range r.GPUs {
		g := &r.GPUs[i]
		if g.Worker != 0 {
			if w := s.players[g.Worker]; w == nil || w.Dead {
				g.stopWork()
			}
		}
		if g.State != GPUCarried {
			continue
		}
		c := s.players[g.Carrier]
		if c == nil || c.Dead || c.Team != content.Vibe {
			s.dropGPU(i, g.Pos)
			continue
		}
		g.Pos = c.Pos
		for _, e := range s.Map.Exits {
			if e.Contains([3]float64(c.Pos)) {
				s.secureGPU(i, c)
				break
			}
		}
		if r.Phase != PhaseLive {
			return // the round just ended (secured): nothing more this tick
		}
	}
}

func (g *gpuState) stopWork() { g.Progress, g.Worker, g.ticks = 0, 0, 0 }

func (s *Server) dropGPU(i int, at Vec3) {
	g := &s.round.GPUs[i]
	by := g.Carrier
	g.State, g.Carrier, g.Pos = GPUDropped, 0, at
	g.stopWork()
	s.syncCarry()
	s.gpuSend("drop", i, by)
}

// dropGPUAt: p's GPU (if any) goes down at pos (death, team switch, leaving, G).
func (s *Server) dropGPUAt(p *Player, pos Vec3) bool {
	if i := s.carriedBy(p); i >= 0 {
		s.dropGPU(i, pos)
		return true
	}
	return false
}

func (s *Server) secureGPU(i int, c *Player) {
	r := &s.round
	g := &r.GPUs[i]
	g.State = GPUSecured // Carrier stays: who secured it
	r.Secured++
	s.syncCarry()
	e := s.econ().GPUDrop
	s.pay(c, e.Secure, "secured a GPU")
	for _, p := range s.ordered() {
		if p != c && p.Team == content.Vibe {
			s.pay(p, e.SecureTeam, "your team secured a GPU")
		}
	}
	c.Points += scoreObjective
	s.gpuSend("secure", i, c.ID)
	if r.Secured >= r.ToWin {
		s.endRound(s.now, content.Vibe, "secured")
	}
}

// gpuUse runs after each of p's commands in GPU Drop (roundUse): +use held
// standing still grabs (Vibe Coder) or restocks (Artisan) the nearest GPU in
// reach. Letting go, moving or walking out of reach resets the progress.
func (s *Server) gpuUse(p *Player, still bool) {
	r := &s.round
	target := -1
	if still && r.Phase == PhaseLive {
		best := math.Inf(1)
		for i, g := range r.GPUs {
			ok := false
			switch p.Team {
			case content.Vibe:
				ok = (g.State == GPUShelf || g.State == GPUDropped) && s.carriedBy(p) < 0
			case content.Artisan:
				ok = g.State == GPUDropped
			}
			if !ok || (g.Worker != 0 && g.Worker != p.ID) {
				continue // someone else is on that one
			}
			d := math.Hypot(p.Pos[0]-g.Pos[0], p.Pos[2]-g.Pos[2])
			if content.InGrabReach([3]float64(p.Pos), [3]float64(g.Pos)) && d < best {
				target, best = i, d
			}
		}
	}
	for i := range r.GPUs { // one job at a time: anything else p was doing stops
		if g := &r.GPUs[i]; g.Worker == p.ID && i != target {
			g.stopWork()
		}
	}
	if target < 0 {
		return
	}
	g := &r.GPUs[target]
	total := grabSecs
	if p.Team == content.Artisan {
		total = restockSecs
	}
	g.Worker = p.ID
	g.ticks++
	g.Progress = math.Min(1, float64(g.ticks)/(total*SimHz))
	if g.Progress < 1 {
		return
	}
	g.stopWork()
	e := s.econ().GPUDrop
	if p.Team == content.Vibe {
		g.State, g.Carrier = GPUCarried, p.ID
		s.syncCarry()
		s.pay(p, e.Grab, "grabbed a GPU")
		s.gpuSend("grab", target, p.ID)
		return
	}
	g.State, g.Pos = GPUShelf, Vec3(s.Map.GPUs[target].Pos)
	s.pay(p, e.Restock, "restocked a GPU")
	p.Points += scoreObjective
	s.gpuSend("restock", target, p.ID)
}

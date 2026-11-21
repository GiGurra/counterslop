package game

import (
	"encoding/json"
	"math"
	"strconv"
)

// Lag compensation ("favour the shooter"). Clients draw other players
// interpolated between snapshots, a little in the past; a fire command says
// which server tick the shooter was looking at (vt, fractional). The server
// keeps the poses it sent in each snapshot and ray-traces the shot against
// the poses at vt, interpolated the same way the client did, then applies the
// damage now. World geometry is not rewound. Rewinds are capped by
// sv_maxunlag so very laggy shooters can't hit far into the past.

// histLen covers about 1.4 s of snapshots (one every snapEvery ticks).
const histLen = 48

type histFrame struct {
	tick  int64
	poses map[int]PoseState
}

// History is a ring of the poses sent in recent snapshots.
type History struct {
	frames []histFrame // oldest first
}

// Record stores the poses of a snapshot taken after tick.
func (h *History) Record(tick int64, poses map[int]PoseState) {
	h.frames = append(h.frames, histFrame{tick, poses})
	if len(h.frames) > histLen {
		h.frames = h.frames[len(h.frames)-histLen:]
	}
}

// At returns every player's pose at fractional tick t, interpolated between
// the two snapshots around it exactly as web/engine/net.js samples them:
// position and pitch linearly, yaw the short way round, crouch from the later
// snapshot. Outside the recorded range it clamps to the nearest snapshot.
// A player missing from either side keeps the pose from the side it is in.
func (h *History) At(t float64) map[int]PoseState {
	n := len(h.frames)
	if n == 0 {
		return nil
	}
	a, b := h.frames[0], h.frames[0]
	for i := 1; i < n; i++ {
		if float64(h.frames[i].tick) >= t {
			a, b = h.frames[i-1], h.frames[i]
			break
		}
		a, b = h.frames[i], h.frames[i]
	}
	k := 1.0
	if b.tick > a.tick {
		k = math.Max(0, math.Min(1, (t-float64(a.tick))/float64(b.tick-a.tick)))
	}
	out := make(map[int]PoseState, len(b.poses))
	for id, pb := range b.poses {
		pa, ok := a.poses[id]
		if !ok {
			out[id] = pb
			continue
		}
		p := pb
		for i := range 3 {
			p.Pos[i] = pa.Pos[i] + (pb.Pos[i]-pa.Pos[i])*k
		}
		p.Yaw = lerpAngle(pa.Yaw, pb.Yaw, k)
		p.Pitch = pa.Pitch + (pb.Pitch-pa.Pitch)*k
		out[id] = p
	}
	return out
}

func lerpAngle(a, b, k float64) float64 {
	d := b - a
	for d > math.Pi {
		d -= 2 * math.Pi
	}
	for d < -math.Pi {
		d += 2 * math.Pi
	}
	return a + d*k
}

// recordHistory stores the poses this snapshot is about to send. Callers hold s.mu.
func (s *Server) recordHistory() {
	poses := make(map[int]PoseState, len(s.players))
	for _, p := range s.ordered() {
		if p.ready && !p.Dead {
			ps := poseOf(p)
			for i := range 3 { // what the client gets: millimetres (Vec3.MarshalJSON)
				ps.Pos[i] = math.Round(ps.Pos[i]*1000) / 1000
			}
			poses[p.ID] = ps
		}
	}
	s.history.Record(s.tick, poses)
}

// rewindTick is the tick a shot is traced at: the shooter's view tick, no
// older than sv_maxunlag seconds and never in the future. 0 = no rewind.
func (s *Server) rewindTick(vt float64) float64 {
	maxUnlag, err := strconv.ParseFloat(s.cvars["sv_maxunlag"], 64)
	if vt <= 0 || !finiteF(vt) || err != nil || maxUnlag <= 0 {
		return 0
	}
	maxBack := maxUnlag * SimHz
	return math.Max(float64(s.tick)-maxBack, math.Min(vt, float64(s.tick)))
}

// posesAt returns the poses to trace a shot against: the history at vt, or
// nil (use current poses) without a usable view tick.
func (s *Server) posesAt(vt float64) map[int]PoseState {
	t := s.rewindTick(vt)
	if t == 0 {
		return nil
	}
	return s.history.At(t)
}

// sendImpacts (sv_showimpacts 1) tells the shooter which poses a shot was
// traced against, so the client can draw them next to what it saw.
func (s *Server) sendImpacts(p *Player, c Cmd, o, d Vec3, dist float64, rewound map[int]PoseState) {
	type pose struct {
		ID    int     `json:"id"`
		P     Vec3    `json:"p"`
		Yaw   float64 `json:"yaw"`
		Pitch float64 `json:"pitch"`
		C     bool    `json:"c"`
		Team  string  `json:"team"`
	}
	list := []pose{} // [] when alone: the client maps over it
	for _, q := range s.ordered() {
		if q == p || q.Dead || !q.ready {
			continue
		}
		ps := poseOf(q)
		if rp, ok := rewound[q.ID]; ok {
			ps = rp
		}
		list = append(list, pose{q.ID, ps.Pos, ps.Yaw, ps.Pitch, ps.Crouch, ps.Team})
	}
	end := Vec3{o[0] + d[0]*dist, o[1] + d[1]*dist, o[2] + d[2]*dist}
	b, _ := json.Marshal(map[string]any{"t": "impacts", "seq": c.Seq, "vt": c.VT, "at": s.rewindTick(c.VT), "tick": s.tick,
		"o": o, "d": [3]float64(d), "end": end, "yaw": c.Yaw, "pitch": c.Pitch, "poses": list})
	trySend(p, b)
}

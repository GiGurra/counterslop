package game

import (
	"encoding/json"
	"math"
	"sort"
	"strconv"
)

// Lag compensation ("favour the shooter"). Clients draw other players
// interpolated between snapshots, a little in the past; a fire command says
// which server tick the shooter was looking at (vt, fractional). The server
// keeps the poses it sent in each snapshot and ray-traces the shot against
// the poses at vt, interpolated the same way the client did, then applies the
// damage now. World geometry is not rewound. Rewinds are capped by
// sv_maxunlag so very laggy shooters can't hit far into the past.

// histLen covers about 1.4 s of snapshots (one every snapEvery ticks: every
// tick), beyond sv_maxunlag's 0.5 s rewind and the client's MAX_DELAY.
const histLen = 140 / snapEvery

type histFrame struct {
	tick   int64
	poses  map[int]PoseState
	frames map[int]framePos // the riders of a mover: their frame and place in it (Player.Frame, Local)
}

// framePos is a rider's frame (1-based mover) and its place in it, as sent.
type framePos struct {
	Fr int
	L  Vec3
}

// History is a ring of the poses sent in recent snapshots.
type History struct {
	frames []histFrame // oldest first
}

// Record stores the poses of a snapshot taken after tick.
func (h *History) Record(tick int64, poses map[int]PoseState) {
	h.RecordFrames(tick, poses, nil)
}

// RecordFrames is Record with the riders' frames (nil: none).
func (h *History) RecordFrames(tick int64, poses map[int]PoseState, frames map[int]framePos) {
	h.frames = append(h.frames, histFrame{tick, poses, frames})
	if len(h.frames) > histLen {
		h.frames = h.frames[len(h.frames)-histLen:]
	}
}

// At returns every player's pose at fractional tick t, interpolated between
// the two snapshots around it exactly as web/engine/net.js samples them:
// position and pitch linearly, yaw the short way round, crouch from the later
// snapshot. Outside the recorded range it clamps to the nearest snapshot.
// A player missing from either side keeps the pose from the side it is in.
func (h *History) At(t float64) map[int]PoseState { return h.AtIn(t, nil) }

// AtIn is At with the riders of movers placed by place (nil: as At, in
// the world): a rider in the same frame in both snapshots is interpolated in
// that frame (its place in it, the teleport check too) and placed where
// place puts the frame (Server.placeAt: the movers at the command's time,
// where the client drew them), so a passenger standing in a ship at 70 m/s
// is where the shooter saw it in the cabin. One entering or leaving a frame
// between the two is interpolated in the world as before (the later pose
// placed, after a teleport). = net.js sampleAt with a frame time.
func (h *History) AtIn(t float64, place func(fr int, l Vec3) (Vec3, bool)) map[int]PoseState {
	n := len(h.frames)
	if n == 0 {
		return nil
	}
	// The first frame at or after t (binary search: 140 frames, every command
	// asks), and the one before it; past the newest, the newest twice.
	i := sort.Search(n, func(i int) bool { return float64(h.frames[i].tick) >= t })
	a, b := h.frames[n-1], h.frames[n-1]
	switch {
	case i == 0 && n > 1: // before the oldest: the first pair (k clamps to it), as the scan did
		a, b = h.frames[0], h.frames[1]
	case i == 0:
		a, b = h.frames[0], h.frames[0]
	case i < n:
		a, b = h.frames[i-1], h.frames[i]
	}
	k := 1.0
	if b.tick > a.tick {
		k = math.Max(0, math.Min(1, (t-float64(a.tick))/float64(b.tick-a.tick)))
	}
	out := make(map[int]PoseState, len(b.poses))
	// placed: pose p with its position at l in frame fb placed (when it is in one)
	placed := func(p PoseState, f framePos, in bool) PoseState {
		if in && place != nil {
			if w, ok := place(f.Fr, f.L); ok {
				p.Pos = w
			}
		}
		return p
	}
	for id, pb := range b.poses {
		fb, inB := b.frames[id]
		pa, ok := a.poses[id]
		if !ok {
			out[id] = placed(pb, fb, inB)
			continue
		}
		p := pb
		fa, inA := a.frames[id]
		if same := place != nil && inA && inB && fa.Fr == fb.Fr; same {
			if teleported(fa.L, fb.L, b.tick-a.tick) {
				out[id] = placed(pb, fb, true) // a teleport within the frame
				continue
			}
			var l Vec3
			for i := range 3 {
				l[i] = fa.L[i] + float64((fb.L[i]-fa.L[i])*k) // no FMA: net.js gets the same bits
			}
			p = placed(p, framePos{fb.Fr, l}, true)
		} else if teleported(pa.Pos, pb.Pos, b.tick-a.tick) {
			out[id] = placed(pb, fb, inB) // a respawn or teleport: no sliding across the map (net.js too)
			continue
		} else {
			for i := range 3 {
				p.Pos[i] = pa.Pos[i] + float64((pb.Pos[i]-pa.Pos[i])*k) // no FMA: net.js gets the same bits
			}
		}
		p.Yaw = lerpAngle(pa.Yaw, pb.Yaw, k)
		p.Pitch = pa.Pitch + (pb.Pitch-pa.Pitch)*k
		out[id] = p
	}
	at := PoseTime(float64(a.tick), float64(b.tick), k)
	for id, p := range out {
		p.T = at
		p.Gait = h.gaitAt(id, t, i)
		out[id] = p
	}
	return out
}

// gaitAt is id's gait at tick t: between the full snapshots around t (the
// recorded frames with a gait for id), as web/engine/net.js sample does.
// i is the first frame at or after t (At's search): the nearest full frames
// are a few frames from it either way.
func (h *History) gaitAt(id int, t float64, i int) *Gait {
	var ga, gb *Gait
	ta, tb := 0.0, 0.0
	for j := min(i, len(h.frames)) - 1; j >= 0; j-- {
		if g := h.frames[j].poses[id].Gait; g != nil {
			ga, ta = g, float64(h.frames[j].tick)
			break
		}
	}
	for j := max(i, 0); j < len(h.frames); j++ {
		if g := h.frames[j].poses[id].Gait; g != nil {
			gb, tb = g, float64(h.frames[j].tick)
			break
		}
	}
	return GaitBetween(ta, ga, tb, gb, t)
}

// teleportSpeed: two snapshots of a player farther apart than this many
// metres per tick between them (30 m/s: no run, jump or fall gets near) are
// a respawn or a teleport, not a move: the pose jumps to the later one. =
// net.js TELEPORT_SPEED.
const teleportSpeed = 0.3

func teleported(a, b Vec3, ticks int64) bool {
	dx, dy, dz := b[0]-a[0], b[1]-a[1], b[2]-a[2]
	g := float64(teleportSpeed * float64(max(ticks, 1)))
	return float64(dx*dx)+float64(dy*dy)+float64(dz*dz) > float64(g*g)
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
	var frames map[int]framePos
	for _, p := range s.ordered() {
		if solid(p) { // a body others can hit or bump into then (net.js: a sample's s)
			ps := poseOf(p, 0) // T: History.At sets it
			for i := range 3 { // what the client gets: millimetres (Vec3.MarshalJSON)
				ps.Pos[i] = math.Round(ps.Pos[i]*1000) / 1000
			}
			if s.tick%fullEvery == 0 { // the gait goes in full snapshots only (At interpolates it between them)
				g := p.Gait.wire() // as Gait.MarshalJSON sends it
				ps.Gait = &g
			}
			poses[p.ID] = ps
			if p.Frame > 0 && p.Local != nil { // riding a mover (frameOf: already millimetres)
				if frames == nil {
					frames = map[int]framePos{}
				}
				frames[p.ID] = framePos{p.Frame, *p.Local}
			}
		}
	}
	s.history.RecordFrames(s.tick, poses, frames)
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
// ft: the command's time on the movers' clock: the riders of movers are
// placed where the movers are then (where the shooter's client drew them).
func (s *Server) posesAt(vt, ft float64) map[int]PoseState {
	t := s.rewindTick(vt)
	if t == 0 {
		return nil
	}
	return s.history.AtIn(t, s.placeAt(ft))
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
		G     *Gait   `json:"g"`
		T     float64 `json:"t"` // the pose time (s)
	}
	list := []pose{} // [] when alone: the client maps over it
	for _, q := range s.ordered() {
		if q == p || q.Dead || !q.ready {
			continue
		}
		ps := poseOf(q, s.poseTime())
		if rp, ok := rewound[q.ID]; ok {
			ps = rp
		}
		list = append(list, pose{q.ID, ps.Pos, ps.Yaw, ps.Pitch, ps.Crouch, ps.Team, ps.Gait, ps.T})
	}
	end := Vec3{o[0] + d[0]*dist, o[1] + d[1]*dist, o[2] + d[2]*dist}
	vy, vp := ShotView(c)
	b, _ := json.Marshal(map[string]any{"t": "impacts", "seq": c.Seq, "vt": c.VT, "at": s.rewindTick(c.VT), "tick": s.tick,
		"o": o, "d": [3]float64(d), "end": end, "yaw": vy, "pitch": vp, "poses": list})
	trySend(p, b)
}

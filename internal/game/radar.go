package game

import (
	"encoding/json"
	"math"

	"github.com/gigurra/counterslop/internal/content"
)

// Radar spotting (CS): an enemy shows on your radar while someone alive on
// your team can see them: in front of that teammate (within spotHalfFOV of
// where they look), with nothing solid and no smoke cloud in between. The
// server decides; each recipient's snapshot carries only their own team's
// list ("spot": [ids]). Spectators see everyone and get no list.

const spotCos = 0.34 // cos(70°): a generous half field of view

// spotted returns, per team, the ids of enemies that team sees now. Callers hold s.mu.
func (s *Server) spotted() map[string][]int {
	out := map[string][]int{}
	for _, team := range []string{content.Vibe, content.Artisan} {
		ids := []int{}
		for _, q := range s.players {
			if q.Team == team || q.Team == content.Spectator || q.Dead || !q.ready {
				continue
			}
			for _, p := range s.players {
				if p.Team == team && !p.Dead && p.ready && s.sees(p, q) {
					ids = append(ids, q.ID)
					break
				}
			}
		}
		out[team] = ids
	}
	return out
}

// sees: q's head or chest is in front of p's eye, in the open and not behind smoke.
func (s *Server) sees(p, q *Player) bool {
	eyeH := 1.6
	if p.body != nil {
		eyeH = p.body.EyeH + p.body.EyeOff
	}
	eye := Vec3{p.Pos[0], p.Pos[1] + eyeH, p.Pos[2]}
	cp := math.Cos(p.Pitch)
	view := Vec3{-math.Sin(p.Yaw) * cp, math.Sin(p.Pitch), -math.Cos(p.Yaw) * cp}
	for _, h := range []float64{1.5, 1.0} { // head, chest
		t := Vec3{q.Pos[0], q.Pos[1] + h, q.Pos[2]}
		d := Vec3{t[0] - eye[0], t[1] - eye[1], t[2] - eye[2]}
		l := math.Sqrt(d[0]*d[0] + d[1]*d[1] + d[2]*d[2])
		if l < 1e-6 {
			return true
		}
		if (view[0]*d[0]+view[1]*d[1]+view[2]*d[2])/l < spotCos {
			continue
		}
		if s.lineClear(eye, t) && !s.smokeBetween(eye, t) {
			return true
		}
	}
	return false
}

// smokeBetween: the segment a-b passes through a standing smoke cloud's core.
func (s *Server) smokeBetween(a, b Vec3) bool {
	for _, n := range s.nades {
		if n.State != "smoke" {
			continue
		}
		r := n.w.Grenade.Radius
		c := Vec3{n.Pos[0], n.Pos[1] + r*0.3, n.Pos[2]}
		if segPointDist(a, b, c) < r*0.75 {
			return true
		}
	}
	return false
}

// spottedJSON is each team's snapshot fragment (`"spot":[ids]`).
func (s *Server) spottedJSON() map[string][]byte {
	out := map[string][]byte{}
	for team, ids := range s.spotted() {
		b, _ := json.Marshal(ids)
		out[team] = append([]byte(`"spot":`), b...)
	}
	return out
}

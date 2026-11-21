package game

import (
	"encoding/json"
	"fmt"
	"math"

	"github.com/gigurra/counterslop/internal/content"
)

// The player model is data (web/assets/models/player.json) shared with the
// client, which draws every part; the server ray-traces the parts with hit !=
// false as oriented boxes, posed exactly like web/gen/boxmodel.js does (the
// parity test runs both). There are no separate hitboxes.

// UnmarshalJSON: a team or a list of teams.
func (t *teamSet) UnmarshalJSON(b []byte) error {
	var one string
	if json.Unmarshal(b, &one) == nil {
		*t = teamSet{one}
		return nil
	}
	var many []string
	if err := json.Unmarshal(b, &many); err != nil {
		return err
	}
	*t = many
	return nil
}

// PartCorners are a part's world-space corners (parity tests, debugging).
func PartCorners(p *ModelPart, st PoseState) [8]Vec3 {
	c, s, pivot := p.box(st.Crouch)
	var out [8]Vec3
	i := 0
	for _, x := range []float64{-0.5, 0.5} {
		for _, y := range []float64{-0.5, 0.5} {
			for _, z := range []float64{-0.5, 0.5} {
				v := Vec3{c[0] + x*s[0], c[1] + y*s[1], c[2] + z*s[2]}
				if pivot != nil {
					cp, sp := math.Cos(st.Pitch), math.Sin(st.Pitch)
					v = Vec3{v[0] + pivot[0], cp*v[1] - sp*v[2] + pivot[1], sp*v[1] + cp*v[2] + pivot[2]}
				}
				cy, sy := math.Cos(st.Yaw), math.Sin(st.Yaw)
				out[i] = Vec3{cy*v[0] + sy*v[2] + st.Pos[0], v[1] + st.Pos[1], -sy*v[0] + cy*v[2] + st.Pos[2]}
				i++
			}
		}
	}
	return out
}

// PlayerModel is loaded at startup (LoadModel); tests load the file directly.
var PlayerModel Model

func LoadModel(b []byte) error {
	var m Model
	if err := json.Unmarshal(b, &m); err != nil {
		return fmt.Errorf("player.json: %w", err)
	}
	if len(m.Parts) == 0 {
		return fmt.Errorf("player.json: no parts")
	}
	PlayerModel = m
	return nil
}

func poseOf(p *Player) PoseState {
	return PoseState{Pos: p.Pos, Yaw: p.Yaw, Pitch: p.Pitch, Crouch: p.Crouch, Team: p.Team, ID: p.ID}
}

// mapDistance is the distance to the first map box along the ray (or +Inf).
func mapDistance(m *content.Map, o, d Vec3) float64 {
	best := math.Inf(1)
	for _, b := range m.Boxes {
		if t := rayAABB(o, d, Vec3(b.Min), Vec3(b.Max)); t >= 0 && t < best {
			best = t
		}
	}
	return best
}

// hitPlayer ray-traces a player's posed model.
func hitPlayer(p *Player, o, d Vec3) (Hitbox, float64, bool) {
	return RayModel(&PlayerModel, poseOf(p), o, d)
}

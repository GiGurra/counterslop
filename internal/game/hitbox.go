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

type partShape struct {
	C     Vec3  `json:"c"`
	S     Vec3  `json:"s"`
	Pivot *Vec3 `json:"pivot"`
}

type ModelPart struct {
	Name   string `json:"name"`
	Region string `json:"region"`
	Hit    *bool  `json:"hit"`
	Team   string `json:"team"`
	partShape
	Crouch *partShape `json:"crouch"`
}

type Model struct {
	Mult  map[string]float64 `json:"mult"`
	Parts []ModelPart        `json:"parts"`
}

// Hitbox is what a ray hit: the part's region and its damage multiplier.
type Hitbox struct {
	Part string // region: head | body | legs
	Name string // model part
	Mult float64
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

// box returns the part's shape for this crouch state (crouch overrides per field).
func (p *ModelPart) box(crouched bool) (c, s Vec3, pivot *Vec3) {
	c, s, pivot = p.C, p.S, p.Pivot
	if crouched && p.Crouch != nil {
		if p.Crouch.C != (Vec3{}) {
			c = p.Crouch.C
		}
		if p.Crouch.S != (Vec3{}) {
			s = p.Crouch.S
		}
		if p.Crouch.Pivot != nil {
			pivot = p.Crouch.Pivot
		}
	}
	return
}

// PoseState is the replicated state a model is posed from.
type PoseState struct {
	Pos        Vec3
	Yaw, Pitch float64
	Crouch     bool
	Team       string
}

func poseOf(p *Player) PoseState {
	return PoseState{Pos: p.Pos, Yaw: p.Yaw, Pitch: p.Pitch, Crouch: p.Crouch, Team: p.Team}
}

// toPartFrame maps a world point (w=1) or direction (w=0) into the part's frame
// (before its own translate(c)): the inverse of translate(pos) rotY(yaw)
// [translate(pivot) rotX(pitch)].
func toPartFrame(v Vec3, point bool, st PoseState, pivot *Vec3) Vec3 {
	if point {
		v = Vec3{v[0] - st.Pos[0], v[1] - st.Pos[1], v[2] - st.Pos[2]}
	}
	// rotY(yaw): wx = c*x + s*z, wz = -s*x + c*z  =>  x = c*wx - s*wz, z = s*wx + c*wz
	c, s := math.Cos(st.Yaw), math.Sin(st.Yaw)
	v = Vec3{c*v[0] - s*v[2], v[1], s*v[0] + c*v[2]}
	if pivot != nil {
		if point {
			v = Vec3{v[0] - pivot[0], v[1] - pivot[1], v[2] - pivot[2]}
		}
		// rotX(pitch): wy = c*y - s*z, wz = s*y + c*z  =>  y = c*wy + s*wz, z = -s*wy + c*wz
		c, s := math.Cos(st.Pitch), math.Sin(st.Pitch)
		v = Vec3{v[0], c*v[1] + s*v[2], -s*v[1] + c*v[2]}
	}
	return v
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

// RayModel returns the nearest hittable part of the posed model along o+t*d.
func RayModel(m *Model, st PoseState, o, d Vec3) (Hitbox, float64, bool) {
	best, bestT, ok := Hitbox{}, math.Inf(1), false
	for i := range m.Parts {
		p := &m.Parts[i]
		if (p.Hit != nil && !*p.Hit) || (p.Team != "" && p.Team != st.Team) {
			continue
		}
		c, s, pivot := p.box(st.Crouch)
		lo, ld := toPartFrame(o, true, st, pivot), toPartFrame(d, false, st, pivot)
		mn := Vec3{c[0] - s[0]/2, c[1] - s[1]/2, c[2] - s[2]/2}
		mx := Vec3{c[0] + s[0]/2, c[1] + s[1]/2, c[2] + s[2]/2}
		if t := rayAABB(lo, ld, mn, mx); t >= 0 && t < bestT {
			mult, has := m.Mult[p.Region]
			if !has {
				mult = 1
			}
			best, bestT, ok = Hitbox{Part: p.Region, Name: p.Name, Mult: mult}, t, true
		}
	}
	return best, bestT, ok
}

// rayAABB returns the entry distance of ray o+t·d into the box, or -1.
func rayAABB(o, d, mn, mx Vec3) float64 {
	t0, t1 := 0.0, math.Inf(1)
	for a := 0; a < 3; a++ {
		if math.Abs(d[a]) < 1e-12 {
			if o[a] < mn[a] || o[a] > mx[a] {
				return -1
			}
			continue
		}
		ta, tb := (mn[a]-o[a])/d[a], (mx[a]-o[a])/d[a]
		if ta > tb {
			ta, tb = tb, ta
		}
		t0, t1 = math.Max(t0, ta), math.Min(t1, tb)
		if t0 > t1 {
			return -1
		}
	}
	return t0
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

//gojs:shared

package game

// The player model's pose and hit test, shared with the client: cmd/gojs
// translates this file into web/game/shared.gen.js, which web/gen/boxmodel.js
// runs for the predicted hit (rayModel) and the seeded part list it draws
// (partsFor). The model is data (web/assets/models/player.json); the client
// draws every part, the server ray-traces the parts with hit != false as
// oriented boxes. There are no separate hitboxes.

import (
	"math"
	"slices"

	"github.com/gigurra/counterslop/internal/content/rng"
	"github.com/gigurra/counterslop/internal/detmath"
)

type partShape struct {
	C     Vec3  `json:"c"`
	S     Vec3  `json:"s"`
	Pivot *Vec3 `json:"pivot"`
}

type ModelPart struct {
	Name   string     `json:"name"`
	Region string     `json:"region"`
	Hit    *bool      `json:"hit"`
	Team   teamSet    `json:"team"`
	C      Vec3       `json:"c"`
	S      Vec3       `json:"s"`
	Pivot  *Vec3      `json:"pivot"`
	Crouch *partShape `json:"crouch"`
	// Seeded per-player variation (see Model.Vars).
	When   map[string]float64 `json:"when"`   // shown only if each var >= min
	Vary   map[string]varyBy  `json:"vary"`   // c, s += var × vector
	Jitter *Vec3              `json:"jitter"` // c += ±jitter, drawn per player
	src    int                // PartsFor: the index of the part in Model.Parts it came from
}

type varyBy struct {
	C *Vec3 `json:"c"`
	S *Vec3 `json:"s"`
}

// teamSet is a part's team filter: absent = everyone, "vibe", or a list
// (["", "spec"]; "" is teamless, e.g. bots).
type teamSet []string

// teamSet_fromJSON is the client's decoding of a team filter (UnmarshalJSON's
// twin; JSON values are outside the subset).
//
//gojs:extern ./hitbox.extern.js
func teamSet_fromJSON(v any) teamSet { panic("client only") }

func (t teamSet) matches(team string) bool { return t == nil || slices.Contains(t, team) }

// ModelVar is a seeded per-player value: an int in [a, b] or a float in [a, b).
type ModelVar struct {
	Name  string      `json:"name"`
	Int   *[2]int     `json:"int"`
	Range *[2]float64 `json:"range"`
}

type Model struct {
	Mult  map[string]float64 `json:"mult"`
	Seed  uint32             `json:"seed"`
	Vars  []ModelVar         `json:"vars"`
	Parts []ModelPart        `json:"parts"`
}

// Seeded draws a player's variation: the vars in order from rng(seed + id),
// then one value per jitter axis for every part with a jitter, in part order
// (whether or not the part is shown, so the stream never shifts).
func (m *Model) Seeded(id int) (map[string]float64, []*Vec3) {
	r := rng.New(m.Seed + uint32(id))
	vars := make(map[string]float64, len(m.Vars))
	for _, v := range m.Vars {
		if v.Int != nil {
			vars[v.Name] = float64(r.Int(v.Int[0], v.Int[1]))
		} else if v.Range != nil {
			vars[v.Name] = r.Range(v.Range[0], v.Range[1])
		}
	}
	jit := make([]*Vec3, len(m.Parts))
	for i := range m.Parts {
		if j := m.Parts[i].Jitter; j != nil {
			jit[i] = &Vec3{float64(signed(r) * j[0]), float64(signed(r) * j[1]), float64(signed(r) * j[2])}
		}
	}
	return vars, jit
}

// PartsFor returns the parts present for st (team outfit, seeded options)
// with the player's seeded variation applied (Seeded): vary adds var × vector
// to c and s (stand and crouch shapes), jitter adds ±jitter to c.
func (m *Model) PartsFor(st PoseState) []ModelPart {
	vars, jit := m.Seeded(st.ID)
	out := make([]ModelPart, 0, len(m.Parts))
	for i := range m.Parts {
		p := m.Parts[i]
		p.src = i
		if !p.Team.matches(st.Team) {
			continue
		}
		shown := true
		for k, min := range p.When {
			if vars[k] < min {
				shown = false
			}
		}
		if !shown {
			continue
		}
		if p.Vary == nil && jit[i] == nil {
			out = append(out, p)
			continue
		}
		keys := sortedKeys(p.Vary) // a fixed order: the sums are the same bits everywhere
		adj := func(v0 Vec3, size bool) Vec3 {
			v := v0
			for _, k := range keys {
				d := p.Vary[k]
				f := d.C
				if size {
					f = d.S
				}
				if f != nil {
					x := vars[k]
					v = Vec3{v[0] + float64(x*f[0]), v[1] + float64(x*f[1]), v[2] + float64(x*f[2])}
				}
			}
			if j := jit[i]; !size && j != nil {
				v = Vec3{v[0] + j[0], v[1] + j[1], v[2] + j[2]}
			}
			return v
		}
		p.C, p.S = adj(p.C, false), adj(p.S, true)
		if p.Crouch != nil {
			c := *p.Crouch
			if !zero3(c.C) {
				c.C = adj(c.C, false)
			}
			if !zero3(c.S) {
				c.S = adj(c.S, true)
			}
			p.Crouch = &c
		}
		out = append(out, p)
	}
	return out
}

// Src is the index in Model.Parts of a part PartsFor returned.
func (p *ModelPart) Src() int { return p.src }

func sortedKeys(m map[string]varyBy) []string {
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	for i := 1; i < len(keys); i++ { // insertion sort: a handful of keys
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}

// signed is a draw in [-1, 1).
func signed(r *rng.Rand) float64 { return float64(float64(r.Float())*2) - 1 }

func zero3(v Vec3) bool { return v[0] == 0 && v[1] == 0 && v[2] == 0 }

// Hitbox is what a ray hit: the part's region and its damage multiplier.
type Hitbox struct {
	Part string // region: head | body | legs
	Name string // model part
	Mult float64
}

// box returns the part's shape for this crouch state (crouch overrides per field).
func (p *ModelPart) box(crouched bool) (Vec3, Vec3, *Vec3) {
	c, s, pivot := p.C, p.S, p.Pivot
	if crouched && p.Crouch != nil {
		if !zero3(p.Crouch.C) {
			c = p.Crouch.C
		}
		if !zero3(p.Crouch.S) {
			s = p.Crouch.S
		}
		if p.Crouch.Pivot != nil {
			pivot = p.Crouch.Pivot
		}
	}
	return c, s, pivot
}

// PoseState is the replicated state a model is posed from.
type PoseState struct {
	Pos        Vec3
	Yaw, Pitch float64
	Crouch     bool
	Team       string
	ID         int // seeds per-player model variation
}

// toPartFrame maps a world point (w=1) or direction (w=0) into the part's frame
// (before its own translate(c)): the inverse of translate(pos) rotY(yaw)
// [translate(pivot) rotX(pitch)].
func toPartFrame(v0 Vec3, point bool, st PoseState, pivot *Vec3) Vec3 {
	v := v0
	if point {
		v = Vec3{v[0] - st.Pos[0], v[1] - st.Pos[1], v[2] - st.Pos[2]}
	}
	// rotY(yaw): wx = c*x + s*z, wz = -s*x + c*z  =>  x = c*wx - s*wz, z = s*wx + c*wz
	c, s := detmath.Cos(st.Yaw), detmath.Sin(st.Yaw)
	v = Vec3{float64(c*v[0]) - float64(s*v[2]), v[1], float64(s*v[0]) + float64(c*v[2])}
	if pivot != nil {
		if point {
			v = Vec3{v[0] - pivot[0], v[1] - pivot[1], v[2] - pivot[2]}
		}
		// rotX(pitch): wy = c*y - s*z, wz = s*y + c*z  =>  y = c*wy + s*wz, z = -s*wy + c*wz
		c, s := detmath.Cos(st.Pitch), detmath.Sin(st.Pitch)
		v = Vec3{v[0], float64(c*v[1]) + float64(s*v[2]), float64(-s*v[1]) + float64(c*v[2])}
	}
	return v
}

// RayModel returns the nearest hittable part of the posed model along o+t*d.
func RayModel(m *Model, st PoseState, o, d Vec3) (Hitbox, float64, bool) {
	best, bestT, ok := Hitbox{}, math.Inf(1), false
	parts := m.PartsFor(st)
	for i := range parts {
		p := &parts[i]
		if p.Hit != nil && !*p.Hit {
			continue
		}
		c, s, pivot := p.box(st.Crouch)
		lo, ld := toPartFrame(o, true, st, pivot), toPartFrame(d, false, st, pivot)
		// float64(): the compiler makes /2 a *0.5 and would fuse it (exact
		// either way, but TestNoFusedMultiplyAdd keeps the rule simple).
		mn := Vec3{c[0] - float64(s[0]/2), c[1] - float64(s[1]/2), c[2] - float64(s[2]/2)}
		mx := Vec3{c[0] + float64(s[0]/2), c[1] + float64(s[1]/2), c[2] + float64(s[2]/2)}
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

// segPointDist is the distance from c to the segment a-b.
func segPointDist(a, b, c Vec3) float64 {
	ab := Vec3{b[0] - a[0], b[1] - a[1], b[2] - a[2]}
	ac := Vec3{c[0] - a[0], c[1] - a[1], c[2] - a[2]}
	ll := float64(ab[0]*ab[0]) + float64(ab[1]*ab[1]) + float64(ab[2]*ab[2])
	t := 0.0
	if ll > 0 {
		t = math.Max(0, math.Min(1, (float64(ac[0]*ab[0])+float64(ac[1]*ab[1])+float64(ac[2]*ab[2]))/ll))
	}
	p := Vec3{a[0] + float64(ab[0]*t), a[1] + float64(ab[1]*t), a[2] + float64(ab[2]*t)}
	dx, dy, dz := c[0]-p[0], c[1]-p[1], c[2]-p[2]
	return math.Sqrt(float64(dx*dx) + float64(dy*dy) + float64(dz*dz))
}

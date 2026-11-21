package sim

import (
	"math"

	"github.com/gigurra/counterslop/internal/content"
)

// Moving solids (content.Mover). A mover's boxes are a pure function of time:
// World.T is the start of the command being stepped on the player's command
// clock (internal/game: tickBase + seq ticks), so the server and the client's
// prediction (replays included) put every mover in the same place, bit for bit.
//
// Per sub-step (t0 -> t1) the movers near the body become solids: the box at
// t0 (B0: what the body stood on), at t1 (B1: what it collides with) and how
// far the surface moved (D). Then, before the body's own movement:
//   - riding: a body on the ground whose feet rest on a B0 top is carried by
//     its D (vertically, then x and z with the usual step-up), so standing
//     still on a tread keeps the same height above it for the whole ride;
//   - lifting: a B1 that rose into the body by up to a step lifts it onto it;
//   - shoving: one that moved into it sideways shoves it out along its travel.
//
// A rider who walks off a tread steps down onto the next (stepDown) instead
// of falling.
//
// Neither moves the body into a static box (no crushing, no tunnelling: a
// blocked push just doesn't happen). Leaving a mover keeps no momentum.

const (
	treadInset = 0.005 // treads stay this far off the balustrades: no shared faces
	treadDepth = 2     // each tread reaches this many rises down (risers: no gaps between them)
	moverReach = 2.0   // movers farther than this from the body's box are skipped
	restEps    = 1e-6  // feet this close to a box's top rest on it
)

// solid is one mover box over a sub-step.
type solid struct {
	B0, B1   content.Box
	On0, On1 bool // the box exists (shows) at t0 / t1
	D        Vec3
}

// treads is an escalator's step count.
func treads(m *content.Mover) int { return int((m.Max[1]-m.Min[1])/m.Rise + 0.5) }

// treadTop is the top of the tread u treads from the foot (u = 0: its back at
// the foot's edge): level with the floor for the first Flat treads, equal
// steps up the flight, level with the upper floor for the last Flat.
func treadTop(m *content.Mover, u float64) float64 {
	n := float64(treads(m))
	k := n - float64(2*m.Flat) // treads that climb
	if k <= 0 {
		k = n
	}
	step := float64(n*m.Rise) / k
	return m.Min[1] + float64(step*math.Max(0, math.Min(u+1-m.Flat, k)))
}

// treadBox is tread u's box (false: it is under a comb plate, nothing shows).
func treadBox(m *content.Mover, u float64, out *content.Box) bool {
	a, c := m.Axis, 2-m.Axis
	length := float64(treads(m)) * m.Run
	s0, s1 := math.Max(0, u*m.Run), math.Min(length, (u+1)*m.Run)
	if s1-s0 <= 0 {
		return false
	}
	low := m.Min[a]
	if m.Up < 0 {
		low = m.Max[a]
	}
	up := float64(m.Up)
	e0, e1 := low+float64(up*s0), low+float64(up*s1)
	top := treadTop(m, u)
	out.Min[a], out.Max[a] = math.Min(e0, e1), math.Max(e0, e1)
	out.Min[c], out.Max[c] = m.Min[c]+treadInset, m.Max[c]-treadInset
	out.Min[1], out.Max[1] = math.Max(m.Min[1]-m.Rise, top-float64(treadDepth*m.Rise)-0.02), top
	out.Kind = m.Mat
	return true
}

// slideAt is how far (0..1) a slide mover is towards Min+Move at time t.
func slideAt(m *content.Mover, t float64) float64 {
	if m.Period <= 0 {
		return 0
	}
	q := t/m.Period + m.Phase
	q -= math.Floor(q)
	return 1 - math.Abs(float64(2*q)-1)
}

func slideBox(m *content.Mover, f float64, out *content.Box) {
	for i := range 3 {
		d := float64(m.Move[i] * f)
		out.Min[i], out.Max[i] = m.Min[i]+d, m.Max[i]+d
	}
	out.Kind = m.Mat
}

// near: the mover's swept volume comes within moverReach of the body.
func near(m *content.Mover, s *Body) bool {
	lo, hi := m.Min, m.Max
	if m.Kind == content.MoverEscalator {
		lo[1] -= m.Rise
	} else {
		for i := range 3 {
			lo[i] = math.Min(lo[i], m.Min[i]+m.Move[i])
			hi[i] = math.Max(hi[i], m.Max[i]+m.Move[i])
		}
	}
	r := R + moverReach
	return s.Pos[0]+r > lo[0] && s.Pos[0]-r < hi[0] && s.Pos[1]+s.H+moverReach > lo[1] && s.Pos[1]-moverReach < hi[1] &&
		s.Pos[2]+r > lo[2] && s.Pos[2]-r < hi[2]
}

// NearMovers: some mover could touch s soon (moving its clock now would
// move what it stands on or bumps into).
func (w *World) NearMovers(s *Body) bool {
	for i := range w.Movers {
		if near(&w.Movers[i], s) {
			return true
		}
	}
	return false
}

// MoverBoxes is every mover box showing at time t (the client draws these).
func MoverBoxes(movers []content.Mover, t float64) []content.Box {
	var out []content.Box
	for i := range movers {
		m := &movers[i]
		if m.Kind == content.MoverEscalator {
			k := t * m.Speed / m.Run
			p := k - math.Floor(k)
			for j := -1; j < treads(m); j++ {
				var b content.Box
				if treadBox(m, float64(j)+p, &b) {
					out = append(out, b)
				}
			}
		} else if m.Kind == content.MoverSlide {
			var b content.Box
			slideBox(m, slideAt(m, t), &b)
			out = append(out, b)
		}
	}
	return out
}

// movers sets the solids of the movers near s for the sub-step t0 -> t1
// (w.dyn) and the boxes it collides with (w.cur: B1 of each that shows).
func (w *World) movers(s *Body, t0, t1 float64) {
	w.dyn = w.dyn[:0]
	w.cur = w.cur[:0]
	for i := range w.Movers {
		m := &w.Movers[i]
		if !near(m, s) {
			continue
		}
		if m.Kind == content.MoverEscalator {
			k := t0 * m.Speed / m.Run
			p := k - math.Floor(k)
			du := (t1 - t0) * m.Speed / m.Run
			up := float64(m.Up)
			// -2 .. n: one tread more each end, so the flight is covered at t1 too.
			for j := -2; j <= treads(m); j++ {
				u0 := float64(j) + p
				u1 := u0 + du
				var d solid
				d.On0 = treadBox(m, u0, &d.B0)
				d.On1 = treadBox(m, u1, &d.B1)
				if !d.On0 && !d.On1 {
					continue
				}
				d.D[m.Axis] = float64(up * float64(du*m.Run))
				d.D[1] = treadTop(m, u1) - treadTop(m, u0)
				w.dyn = append(w.dyn, d)
			}
		} else if m.Kind == content.MoverSlide {
			var d solid
			f0, f1 := slideAt(m, t0), slideAt(m, t1)
			slideBox(m, f0, &d.B0)
			slideBox(m, f1, &d.B1)
			d.On0, d.On1 = true, true
			for a := range 3 {
				d.D[a] = d.B1.Min[a] - d.B0.Min[a]
			}
			w.dyn = append(w.dyn, d)
		}
	}
	for i := range w.dyn {
		if w.dyn[i].On1 {
			w.cur = append(w.cur, w.dyn[i].B1)
		}
	}
}

// under: the body's footprint is over b's top face.
func under(s *Body, b *content.Box) bool {
	return s.Pos[0]+R > b.Min[0]+eps && s.Pos[0]-R < b.Max[0]-eps && s.Pos[2]+R > b.Min[2]+eps && s.Pos[2]-R < b.Max[2]-eps
}

// ride carries, lifts and shoves s by the movers of this sub-step.
func (w *World) ride(s *Body) {
	if len(w.dyn) == 0 {
		return
	}
	w.riding = false
	if s.OnGround {
		for i := range w.dyn {
			d := &w.dyn[i]
			if d.On0 && math.Abs(d.B0.Max[1]-s.Pos[1]) <= restEps && under(s, &d.B0) {
				if d.D[1] != 0 {
					w.moveV(s, d.D[1])
					s.OnGround = true // carried: still standing (it lands again in the sub-step)
				}
				w.moveH(s, 0, d.D[0])
				w.moveH(s, 2, d.D[2])
				w.riding = true
				break
			}
		}
	}
	y, lift := s.Pos[1], s.Pos[1]
	for i := range w.cur {
		b := &w.cur[i]
		if w.overlaps(s.Pos[0], y, s.Pos[2], b, s.H) && b.Max[1]-y <= stepUp {
			lift = math.Max(lift, b.Max[1])
		}
	}
	if lift > y && w.free(s.Pos[0], lift, s.Pos[2], s.H) {
		s.Pos[1] = lift
		s.EyeOff -= lift - y
		s.OnGround = true
		s.Vel[1] = math.Max(0, s.Vel[1])
	}
	for i := range w.dyn {
		d := &w.dyn[i]
		if !d.On1 || !w.overlaps(s.Pos[0], s.Pos[1], s.Pos[2], &d.B1, s.H) {
			continue
		}
		ax := 0
		if math.Abs(d.D[2]) > math.Abs(d.D[0]) {
			ax = 2
		}
		to := 0.0
		if d.D[ax] > 0 {
			to = d.B1.Max[ax] + R
		} else if d.D[ax] < 0 {
			to = d.B1.Min[ax] - R
		} else {
			continue
		}
		x, z := s.Pos[0], s.Pos[2]
		if ax == 0 {
			x = to
		} else {
			z = to
		}
		if w.free(x, s.Pos[1], z, s.H) {
			s.Pos[ax] = to
		}
	}
}

// stepDown keeps a rider who walked off a tread on the tread below (up to a
// step lower) instead of falling to it while the treads move on: walking down
// an escalator stays on the ground, as walking up does with the step-up.
func (w *World) stepDown(s *Body) {
	y := s.Pos[1]
	w.moveV(s, -stepUp)
	if !s.OnGround {
		s.Pos[1] = y
		return
	}
	s.EyeOff += y - s.Pos[1]
}

// Replaced: a static map box the movers stand in for in movement (an
// escalator's static treads, inside its volume).
func Replaced(b *content.Box, movers []content.Mover) bool {
	for i := range movers {
		m := &movers[i]
		if m.Kind != content.MoverEscalator || b.Kind != m.Mat {
			continue
		}
		in := true
		for a := range 3 {
			if b.Min[a] < m.Min[a]-1e-6 || b.Max[a] > m.Max[a]+1e-6 {
				in = false
			}
		}
		if in {
			return true
		}
	}
	return false
}

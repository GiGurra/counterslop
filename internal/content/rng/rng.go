// Package rng is the seeded PRNG shared with the client: cmd/gojs translates it
// to web/gen/rng.gen.js, which web/gen/rng.js wraps. It is
// mulberry32, so a seed produces the same sequence in Go and in the browser.
package rng

// Rand is a mulberry32 generator.
type Rand struct{ a uint32 }

func New(seed uint32) *Rand { return &Rand{a: seed} }

// Float returns the next value in [0, 1).
func (r *Rand) Float() float64 {
	r.a += 0x6d2b79f5
	t := r.a
	t = (t ^ t>>15) * (t | 1)
	t ^= t + (t^t>>7)*(t|61)
	return float64(t^t>>14) / 4294967296
}

// Range returns a value in [a, b).
func (r *Rand) Range(a, b float64) float64 { return a + float64(r.Float()*(b-a)) }

// Int returns an int in [a, b].
func (r *Rand) Int(a, b int) int { return a + int(r.Float()*float64(b-a+1)) }

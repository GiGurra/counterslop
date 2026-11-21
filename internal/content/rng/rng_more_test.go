package rng

import (
	"math"
	"testing"
)

func TestDeterministicPerSeed(t *testing.T) {
	a, b, c := New(42), New(42), New(43)
	same := true
	for range 100 {
		x, y, z := a.Float(), b.Float(), c.Float()
		if x != y {
			t.Fatal("same seed diverged")
		}
		same = same && x == z
	}
	if same {
		t.Fatal("different seeds gave the same sequence")
	}
}

func TestRangeAndInt(t *testing.T) {
	r := New(7)
	seen := map[int]int{}
	for range 10000 {
		f := r.Float()
		if f < 0 || f >= 1 {
			t.Fatalf("Float %v outside [0, 1)", f)
		}
		v := r.Range(-2, 3)
		if v < -2 || v >= 3 {
			t.Fatalf("Range %v outside [-2, 3)", v)
		}
		i := r.Int(1, 6)
		if i < 1 || i > 6 {
			t.Fatalf("Int %d outside [1, 6]", i)
		}
		seen[i]++
	}
	for i := 1; i <= 6; i++ {
		if n := seen[i]; n < 1500 || n > 1850 { // ~1667 each
			t.Fatalf("Int(1, 6) gave %d %d times of 10000: %v", i, n, seen)
		}
	}
	// Range and Int consume exactly one Float, mapped linearly.
	a, b := New(99), New(99)
	f := a.Float()
	if got := b.Range(10, 20); math.Abs(got-(10+f*10)) > 1e-12 {
		t.Fatalf("Range %v, want %v", got, 10+f*10)
	}
	f = a.Float()
	if got := b.Int(0, 9); got != int(f*10) {
		t.Fatalf("Int %d, want %d", got, int(f*10))
	}
	if got := New(1).Int(5, 5); got != 5 {
		t.Fatalf("Int(5, 5) = %d", got)
	}
}

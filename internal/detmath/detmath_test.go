package detmath

import (
	"math"
	"math/rand/v2"
	"runtime"
	"testing"
)

func ulps(a, b float64) int64 {
	d := int64(math.Float64bits(a)) - int64(math.Float64bits(b))
	if d < 0 {
		return -d
	}
	return d
}

// Sin/Cos are Go's algorithm: on amd64 (where the standard library has no
// assembly or FMA for them) the results are the standard library's, bit for
// bit. Exp and Hypot replace assembly versions, so they may differ by an ulp.
func TestMatchesStdlib(t *testing.T) {
	r := rand.New(rand.NewPCG(3, 4))
	worst := map[string]int64{}
	for range 200000 {
		x := (r.Float64()*2 - 1) * 40
		y := (r.Float64()*2 - 1) * 40
		for name, d := range map[string]int64{
			"sin":   ulps(Sin(x), math.Sin(x)),
			"cos":   ulps(Cos(x), math.Cos(x)),
			"exp":   ulps(Exp(x/4), math.Exp(x/4)),
			"hypot": ulps(Hypot(x, y), math.Hypot(x, y)),
		} {
			worst[name] = max(worst[name], d)
		}
	}
	t.Logf("max ulps vs stdlib: %v", worst)
	limit := map[string]int64{"sin": 1, "cos": 1, "exp": 2, "hypot": 1} // amd64 math.Exp is assembly, 1-2 ulps off the pure-Go algorithm
	if runtime.GOARCH == "amd64" {
		limit["sin"], limit["cos"] = 0, 0
	}
	for k, v := range worst {
		if v > limit[k] {
			t.Errorf("%s: %d ulps from the standard library (limit %d)", k, v, limit[k])
		}
	}
}

func TestSpecialValues(t *testing.T) {
	inf, nan := math.Inf(1), math.NaN()
	for _, c := range []struct {
		name      string
		got, want float64
	}{
		{"sin0", Sin(0), 0}, {"sin-0", Sin(math.Copysign(0, -1)), math.Copysign(0, -1)},
		{"cos0", Cos(0), 1}, {"exp0", Exp(0), 1}, {"exp-inf", Exp(-inf), 0}, {"exp+inf", Exp(inf), inf},
		{"hypot-inf", Hypot(-inf, nan), inf}, {"hypot0", Hypot(0, 0), 0}, {"hypot345", Hypot(3, 4), 5},
		{"sinBig", Sin(1e12), math.Sin(math.Mod(1e12, twoPi))},
	} {
		if math.Float64bits(c.got) != math.Float64bits(c.want) {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
	for _, v := range []float64{Sin(nan), Sin(inf), Cos(-inf), Exp(nan), Hypot(nan, 1)} {
		if !math.IsNaN(v) {
			t.Errorf("want NaN, got %v", v)
		}
	}
}

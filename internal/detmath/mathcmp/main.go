// Command mathcmp is the Go-vs-V8 math evidence in docs/shared-logic.md:
//
//	go run ./internal/detmath/mathcmp | node internal/detmath/mathcmp/cmp.mjs
//
// Prints, for seeded inputs in the ranges the sim uses, the bit patterns of
// Go's math.Sin/Cos/Exp/Hypot/Pow/Atan2/Sqrt, one JSON line per function.
package main

import (
	"encoding/json"
	"math"
	"math/rand/v2"
	"os"
)

func main() {
	r := rand.New(rand.NewPCG(7, 9))
	const n = 200000
	in := make([][2]float64, n)
	for i := range in {
		in[i] = [2]float64{(r.Float64()*2 - 1) * 20, (r.Float64()*2 - 1) * 20}
	}
	f1 := map[string]func(float64) float64{"sin": math.Sin, "cos": math.Cos, "exp": func(x float64) float64 { return math.Exp(x / 4) }, "sqrt": func(x float64) float64 { return math.Sqrt(math.Abs(x)) }, "atan": math.Atan, "log": func(x float64) float64 { return math.Log(math.Abs(x)) }}
	f2 := map[string]func(a, b float64) float64{"hypot": math.Hypot, "atan2": math.Atan2, "pow": func(a, b float64) float64 { return math.Pow(math.Abs(a), b/10) }}
	out := map[string]any{"in": in}
	res := map[string][]uint64{}
	for k, f := range f1 {
		for _, v := range in {
			res[k] = append(res[k], math.Float64bits(f(v[0])))
		}
	}
	for k, f := range f2 {
		for _, v := range in {
			res[k] = append(res[k], math.Float64bits(f(v[0], v[1])))
		}
	}
	strs := map[string][]string{}
	for k, v := range res {
		for _, b := range v {
			strs[k] = append(strs[k], hex(b))
		}
	}
	out["go"] = strs
	json.NewEncoder(os.Stdout).Encode(out)
}

func hex(b uint64) string {
	const d = "0123456789abcdef"
	s := make([]byte, 16)
	for i := 15; i >= 0; i-- {
		s[i] = d[b&15]
		b >>= 4
	}
	return string(s)
}

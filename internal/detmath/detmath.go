// Package detmath is deterministic float64 math for the code shared with the
// client (cmd/gojs transpiles it to web/gen/detmath.gen.js). The standard
// library's Sin/Cos/Exp/Hypot are not usable there: V8's Math.sin & co. differ
// from Go's in the last bit for 15-38% of inputs (docs/shared-logic.md), and
// Go's own results differ between architectures (assembly Exp/Hypot on amd64,
// fused multiply-add on arm64 and GOAMD64=v3).
//
// Every function here is written in the gojs subset with only IEEE-exact
// operations (+ - * /, Sqrt, Abs, Floor, Ceil), and every product that feeds
// an add or subtract is wrapped in float64(), the conversion the Go spec says
// forces rounding, so no compiler may fuse it. Same input, same bits: on
// amd64, arm64 and in every browser.
//
// Sin and Cos are Go's math.sin/math.cos (Cephes), Exp is Go's math.exp.
// On amd64 builds Sin and Cos return exactly what the standard library does.
package detmath

import "math"

var sinC = [6]float64{
	1.58962301576546568060e-10,
	-2.50507477628578072866e-8,
	2.75573136213857245213e-6,
	-1.98412698295895385996e-4,
	8.33333333332211858878e-3,
	-1.66666666666666307295e-1,
}

var cosC = [6]float64{
	-1.13585365213876817300e-11,
	2.08757008419747316778e-9,
	-2.75573141792967388112e-7,
	2.48015872888517045348e-5,
	-1.38888888888730564116e-3,
	4.16666666666665929218e-2,
}

const (
	pi4A = 7.85398125648498535156e-1 // Pi/4 split into three parts
	pi4B = 3.77489470793079817668e-8
	pi4C = 2.69515142907905952645e-15
	// Above this the Cody-Waite reduction loses precision (Go's
	// reduceThreshold). Game angles never get near it; we reduce mod 2π
	// first (an exact operation in both languages) instead of Payne-Hanek.
	reduceMax = 1 << 29
	twoPi     = 2 * math.Pi
)

// reduce returns the octant j (0..7, made even as in Cephes) and the
// remainder z of |x| (x >= 0, finite).
func reduce(x float64) (int, float64) {
	if x >= reduceMax {
		x = math.Mod(x, twoPi)
	}
	j := int(x * (4 / math.Pi))
	y := float64(j)
	if j%2 == 1 {
		j++
		y++
	}
	j %= 8
	z := ((x - float64(y*pi4A)) - float64(y*pi4B)) - float64(y*pi4C)
	return j, z
}

func sinPoly(z, zz float64) float64 {
	p := float64(sinC[0]*zz) + sinC[1]
	p = float64(p*zz) + sinC[2]
	p = float64(p*zz) + sinC[3]
	p = float64(p*zz) + sinC[4]
	p = float64(p*zz) + sinC[5]
	return z + float64(float64(z*zz)*p)
}

func cosPoly(zz float64) float64 {
	p := float64(cosC[0]*zz) + cosC[1]
	p = float64(p*zz) + cosC[2]
	p = float64(p*zz) + cosC[3]
	p = float64(p*zz) + cosC[4]
	p = float64(p*zz) + cosC[5]
	return 1.0 - float64(0.5*zz) + float64(float64(zz*zz)*p)
}

// Sin is math.Sin, deterministic.
func Sin(x float64) float64 {
	if x == 0 || x != x {
		return x // ±0, NaN
	}
	if x-x != 0 {
		return math.NaN() // ±Inf
	}
	sign := false
	if x < 0 {
		x = -x
		sign = true
	}
	j, z := reduce(x)
	if j > 3 {
		sign = !sign
		j -= 4
	}
	zz := z * z
	var y float64
	if j == 1 || j == 2 {
		y = cosPoly(zz)
	} else {
		y = sinPoly(z, zz)
	}
	if sign {
		y = -y
	}
	return y
}

// Cos is math.Cos, deterministic.
func Cos(x float64) float64 {
	if x != x || x-x != 0 {
		return math.NaN()
	}
	sign := false
	x = math.Abs(x)
	j, z := reduce(x)
	if j > 3 {
		j -= 4
		sign = !sign
	}
	if j > 1 {
		sign = !sign
	}
	zz := z * z
	var y float64
	if j == 1 || j == 2 {
		y = sinPoly(z, zz)
	} else {
		y = cosPoly(zz)
	}
	if sign {
		y = -y
	}
	return y
}

// Exp is math.Exp (Go's pure-Go exp), deterministic.
func Exp(x float64) float64 {
	const (
		ln2Hi     = 6.93147180369123816490e-01
		ln2Lo     = 1.90821492927058770002e-10
		log2e     = 1.44269504088896338700e+00
		overflow  = 7.09782712893383973096e+02
		underflow = -7.45133219101941108420e+02
		nearZero  = 1.0 / (1 << 28)
	)
	if x != x {
		return x
	}
	if x > overflow {
		return math.Inf(1)
	}
	if x < underflow {
		return 0
	}
	if -nearZero < x && x < nearZero {
		return 1 + x
	}
	k := 0
	if x < 0 {
		k = int(float64(log2e*x) - 0.5)
	} else if x > 0 {
		k = int(float64(log2e*x) + 0.5)
	}
	hi := x - float64(float64(k)*ln2Hi)
	lo := float64(k) * ln2Lo
	return expmulti(hi, lo, k)
}

func expmulti(hi, lo float64, k int) float64 {
	const (
		p1 = 1.66666666666666657415e-01
		p2 = -2.77777777770155933842e-03
		p3 = 6.61375632143793436117e-05
		p4 = -1.65339022054652515390e-06
		p5 = 4.13813679705723846039e-08
	)
	r := hi - lo
	t := r * r
	q := p4 + float64(t*p5)
	q = p3 + float64(t*q)
	q = p2 + float64(t*q)
	q = p1 + float64(t*q)
	c := r - float64(t*q)
	y := 1 - ((lo - float64(r*c)/(2-c)) - hi)
	return ldexp(y, k)
}

// ldexp is y·2^k by exact doublings (k is small for every Exp the game takes;
// near the subnormal range this may round differently from math.Ldexp, the
// same way on every platform).
func ldexp(y float64, k int) float64 {
	p := 1.0
	for k > 0 {
		p *= 2
		k--
	}
	for k < 0 {
		p *= 0.5
		k++
	}
	return y * p
}

// Hypot is math.Hypot's algorithm (Go's pure-Go hypot), deterministic.
func Hypot(p, q float64) float64 {
	p, q = math.Abs(p), math.Abs(q)
	if p > math.MaxFloat64 || q > math.MaxFloat64 {
		return math.Inf(1)
	}
	if p != p || q != q {
		return math.NaN()
	}
	if p < q {
		p, q = q, p
	}
	if p == 0 {
		return 0
	}
	q = q / p
	return p * math.Sqrt(1+float64(q*q))
}

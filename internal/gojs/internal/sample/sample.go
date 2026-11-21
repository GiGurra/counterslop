// Package sample exercises every construct of the gojs subset whose JS
// translation needs care. gojs_test.go runs Run in Go and the translated
// module in node and requires the same numbers.
package sample

const legs = 1.8 - 1.2 // exact in Go (0.6), 0.6000000000000001 if recomputed in float64

type Vec = [3]float64

type Body struct {
	Pos  Vec
	Hits int
	Tag  string
}

type Pair struct {
	A, B Body
}

func (b *Body) bump(by int) { b.Hits += by }

func (b Body) sum() float64 { return b.Pos[0] + b.Pos[1] + b.Pos[2] }

func split(x float64) (int, float64) {
	i := int(x)
	return i, x - float64(i)
}

func fresh() Body { return Body{Pos: Vec{1, 2, 3}, Tag: "f"} }

func keep(b Body) Body { return b } // returns a parameter: the caller must get a copy

func b2f(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

// Run returns one number per checked behaviour.
func Run() []float64 {
	r := make3()
	// struct copy on assignment
	a := Body{Pos: Vec{1, 2, 3}}
	b := a
	b.Pos[0] = 9
	b.Hits = 4
	r[0] = a.Pos[0] // 1
	r[1] = float64(a.Hits)
	// array copy
	v := a.Pos
	v[1] = 7
	r[2] = a.Pos[1] // 2
	// pointer mutation through a method
	(&a).bump(3)
	a.bump(2)
	r[3] = float64(a.Hits) // 5
	// value receiver
	r[4] = b.sum() // 9+2+3
	// parallel assignment (swap)
	x, y := 1.5, 2.5
	x, y = y, x
	r[5] = x - y // 1
	// integer division and remainder truncate
	r[6] = float64(-7 / two())
	r[7] = float64(-7 % two())
	r[8] = float64(int(-2.7 * one()))
	// exact constants
	r[9] = legs
	// shadowing
	s := 1.0
	{
		s := s + 10
		r[10] = s // 11
	}
	r[11] = s // 1
	// switch: no fallthrough, break leaves the switch only
	n := 0
	for i := range 6 {
		switch i % 3 {
		case 0:
			n += 1
		case 1, 2:
			if i > 3 {
				break
			}
			n += 10
		}
	}
	r[12] = float64(n) // 2 + 30
	// tagless switch with default first
	k := 0.0
	switch {
	default:
		k = 3
	case n > 100:
		k = 1
	case n > 10:
		k = 2
	}
	r[13] = k
	// labeled continue, range over slice with value
	total := 0
	rows := [][]int{{1, 2, 3}, {4, 5, 6}}
outer:
	for _, row := range rows {
		for j, c := range row {
			if j == 2 {
				continue outer
			}
			total += c
		}
	}
	r[14] = float64(total) // 1+2+4+5
	// multiple results
	ip, fp := split(3.25)
	r[15] = float64(ip) + float64(fp*10) // 3 + 2.5
	// fresh values are not copied, returned params are
	f := fresh()
	g := keep(f)
	g.Pos[2] = 100
	r[16] = f.Pos[2] // 3
	// nested struct copy
	p := Pair{A: f}
	q := p
	q.A.Pos[0] = 50
	r[17] = p.A.Pos[0] // 1
	// strings and bools
	r[18] = b2f(f.Tag == "f" && !(q.A.Tag != "f"))
	// float64() conversion (FMA barrier) is a no-op in JS
	r[19] = float64(x*y) + 1
	return r
}

func one() float64 { return 1 }
func two() int     { return 2 }

func make3() []float64 {
	return []float64{
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	}
}

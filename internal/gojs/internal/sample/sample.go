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

// ---- added with maps, closures, sized integers, append, JSON ----

// Mulberry is mulberry32 (uint32 wrap-around, multiply, shifts, xor).
func Mulberry(seed uint32, n int) []float64 {
	a := seed
	var out []float64
	for range n {
		a += 0x6d2b79f5
		t := a
		t = (t ^ t>>15) * (t | 1)
		t ^= t + (t^t>>7)*(t|61)
		out = append(out, float64(t^t>>14)/4294967296)
	}
	return out
}

// Sized runs the other sized integer types through their wrap-around.
func Sized() []float64 {
	var a uint8 = 250
	a += 10
	var b int8 = 127
	b++
	var c int16 = -32768
	c--
	var d uint16 = 3
	d -= 5
	var e int32 = 2147483647
	e *= 3
	var f uint32 = 1
	f <<= 31
	g := ^uint32(0) >> 4
	h := int32(-7) >> 1
	w300 := 300
	i := uint8(w300)
	u200 := uint8(200)
	j := int8(u200)
	k := 0x0f &^ 0x05
	return []float64{float64(a), float64(b), float64(c), float64(d), float64(e), float64(f), float64(g), float64(h), float64(i), float64(j), float64(k)}
}

type Held struct {
	ID   string `json:"id"`
	Ammo int    `json:"ammo"`
}

type Kit struct {
	Slots   map[string]*Held `json:"slots"`
	Current string           `json:"cur"`
	Weights []float64        `json:"weights"`
	Pos     Vec              `json:"pos"`
	Main    Held             `json:"main"`
	Extra   *Held            `json:"extra"`
	Team    *string          `json:"team"`
	note    string
}

func (k *Kit) held() *Held { return k.Slots[k.Current] }

func count(xs ...int) int {
	n := 0
	for _, x := range xs {
		n += x
	}
	return n
}

// Split returns two results (exported multi-result: an array in JS).
func Split(x float64) (int, float64) { return split(x) }

// Collections exercises maps, closures, append, make, slicing, variadics.
func Collections() []float64 {
	m := map[string]int{"a": 1}
	m["b"] += 2
	m["c"]++
	v, ok := m["zz"]
	delete(m, "a")
	sum := 0
	for k, x := range m {
		if k != "" {
			sum += x
		}
	}
	ids := map[int]string{3: "x", 1: "y"}
	keys := 0
	for k := range ids {
		keys += k
	}
	var s []int
	s = append(s, 1, 2)
	s = append(s, []int{3, 4}...)
	t := s[1:3]
	u := make([]float64, 3)
	u[1] = 2.5
	acc := 0
	add := func(d int) { acc += d }
	for i := range 4 {
		add(i)
	}
	fs := []func() int{}
	for i := range 3 {
		fs = append(fs, func() int { return i * 10 })
	}
	k := &Kit{Slots: map[string]*Held{"p": {ID: "p", Ammo: 30}}, Current: "p"}
	h := k.held()
	h.Ammo -= 5
	get := k.held
	var nilSlice []int
	ip, fp := Split(2.5)
	str := "abc" + "de"
	return []float64{
		float64(len(m)), float64(sum), float64(v), b2f(ok), float64(keys),
		float64(len(s)), float64(t[0] + t[1]), u[1] + float64(len(u)), float64(acc),
		float64(fs[0]() + fs[2]()), float64(get().Ammo), float64(len(nilSlice)), b2f(nilSlice == nil),
		float64(count(1, 2, 3) + count(s...)), float64(ip) + fp, float64(len(str)), float64(str[1]),
		b2f(str < "abd"), b2f(k.Extra == nil),
	}
}

// Describe reads a Kit built from JSON (T_fromJSON in JS, encoding/json in Go).
func Describe(k Kit) []float64 {
	team := 0.0
	if k.Team != nil && *k.Team == "red" {
		team = 1
	}
	extra := -1.0
	if k.Extra != nil {
		extra = float64(k.Extra.Ammo)
	}
	return []float64{float64(len(k.Slots)), float64(k.held().Ammo), float64(len(k.Weights)), k.Pos[2], float64(k.Main.Ammo), extra, team, float64(len(k.note))}
}

// Zeros: nil maps (zero value, absent in JSON) read, range and len as empty
// and compare == nil; make of a struct slice fills real structs.
func Zeros(k Kit) []float64 {
	var m map[string]int
	hs := make([]Held, 2)
	hs[1].Ammo = 3
	var bare Kit
	n := 0
	for range m {
		n++
	}
	b := func(v bool) float64 {
		if v {
			return 1
		}
		return 0
	}
	return []float64{b(m == nil), float64(len(m) + m["x"] + n), float64(hs[0].Ammo + hs[1].Ammo), b(bare.Slots == nil),
		b(k.Slots == nil), float64(len(k.Slots))}
}

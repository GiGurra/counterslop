package sim

import (
	"math"

	"github.com/gigurra/counterslop/internal/detmath"
)

// Cars (docs/racing.md): the vehicles of Slop n Roll Racing, stepped by their
// driver's commands (the driver's prediction runs this same code) or, with
// nobody at the wheel, with no input: they roll to a stop.
//
// A car is arcade physics, not a rigid body: Pos is the centre of its
// footprint where its wheels (or its air cushion) meet the ground, Yaw its
// heading (as a player's: 0 faces -z), Vel its velocity. On the ground it
// follows the ground's height under its four corners (Pitch and Roll are the
// tilt that gives); driving up a ramp gives it the slope's climb rate, so off
// the ramp's end it flies with it, under gravity, until it lands. The type's
// CarSpec (web/assets/cars.json) decides the rest:
//
//   - Drive "wheels": the throttle pushes along the heading up to TopSpeed
//     (reverse slower), pressing against the motion brakes; the steering turns
//     it at TurnRate, less at a crawl (a car can't turn standing); the tyres'
//     Grip takes the sideways speed away (it carves), the handbrake swaps it
//     for SlideGrip and brakes (it slides: drift).
//   - Drive "tracks": as wheels, but steering turns it at any speed, also on
//     the spot (a pivot by differential), and it grips hard.
//   - Drive "hover": no wheels: thrust along the heading, little Grip (it
//     drifts), the turn rate follows slowly (TurnK), and water is ground to it
//     (it floats over it). Wheels and tracks in water bog down instead.
//
// Walls: three discs (Width/2 radius) along its length, from StepUp over its
// ground to its Height, against the map's boxes: pushed out, the speed into
// the box taken away (like the boat's hull). A box or wedge top no higher than
// StepUp over it is ground, not wall (curbs, ramps). The other cars are the
// same discs (World.Cars, set per stepped car): pushed out of them.
const (
	CarGrav      = 20.0 // m/s²: a car falls (a bit lighter than a player's 22: longer jumps)
	carStepUp    = 0.6  // m: a box or wedge top this far over the car's ground is driven onto
	carSnapDown  = 0.08 // m: on the ground it follows the ground down this far per step, plus the speed's share
	carCrawl     = 0.5  // m/s: slower than this counts as stopped (brake vs reverse)
	carAirTurn   = 0.25 // of the turn rate in the air
	carAirDrag   = 0.02 // per second, in the air
	carTilt      = 8.0  // per second: the drawn tilt follows the ground
	carWaterDrag = 2.5  // per second: wheels or tracks in water
	carWaterGrav = 0.3  // of gravity, sinking in water
	carSteerK    = 8.0  // per second: the steering follows the input
)

// Drive types (CarSpec.Drive).
const (
	DriveWheels = "wheels"
	DriveTracks = "tracks"
	DriveHover  = "hover"
)

// CarSpec is one car type (web/assets/cars.json; the client passes the same
// JSON object, its keys are the fields' JS names).
type CarSpec struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Tier      int        `json:"tier"`
	Price     int        `json:"price"`
	Drive     string     `json:"drive"`     // wheels, tracks or hover
	Mass      float64    `json:"mass"`      // kg (rams, phase 3)
	TopSpeed  float64    `json:"topSpeed"`  // m/s ahead
	Reverse   float64    `json:"reverse"`   // m/s astern
	Accel     float64    `json:"accel"`     // m/s² ahead
	Brake     float64    `json:"brake"`     // m/s² braking (and the handbrake's 0.6 of it)
	Drag      float64    `json:"drag"`      // per second, coasting
	Grip      float64    `json:"grip"`      // per second: the sideways speed taken away
	SlideGrip float64    `json:"slideGrip"` // the same with the handbrake on
	TurnRate  float64    `json:"turnRate"`  // rad/s, full lock at speed
	TurnFull  float64    `json:"turnFull"`  // m/s: wheels turn fully from this speed up
	TurnK     float64    `json:"turnK"`     // per second: the turn rate follows the steering
	HP        int        `json:"hp"`        // the car's own health
	Size      [3]float64 `json:"size"`      // width, height, length (m): the hitbox and the walls' discs
	Wheelbase float64    `json:"wheelbase"` // m, front to rear axle (the ground probes)
	Track     float64    `json:"track"`     // m, left to right wheel
	Ride      float64    `json:"ride"`      // m: the body's floor over the ground (drawn; the hover's cushion)
	Travel    float64    `json:"travel"`    // m: suspension travel (drawn: Comp)
	Seat      [3]float64 `json:"seat"`      // the driver's feet in the car's frame (x right, y up, z aft)
	Front     string     `json:"front"`     // the built-in front weapon (phase 3)
	Rear      string     `json:"rear"`      // the built-in rear item (phase 3)
}

// Car is one car's simulated state.
type Car struct {
	Pos      Vec3
	Vel      Vec3
	Yaw      float64
	YawVel   float64
	Pitch    float64    // nose up (rad), from the ground under it (in the air: easing level)
	Roll     float64    // right side down (rad)
	Steer    float64    // -1 left .. 1 right, the steering as it follows the input (drawn: front wheels)
	OnGround bool       // any wheel (the cushion) on the ground
	Contact  int        // bit i: wheel i on the ground (0 front left, 1 front right, 2 rear left, 3 rear right)
	Comp     [4]float64 // wheel i's suspension, 0 hanging .. 1 compressed (drawn)
	HP       int        // the car's health (CarSpec.HP when new)
	Wet      bool       // in water (wheels, tracks: bogged down; the hover floats on it)
}

// CarInput is the driver's: Throttle -1 (brake, reverse) .. 1, Steer -1 left
// .. 1 right, Handbrake held.
type CarInput struct {
	Throttle, Steer float64
	Handbrake       bool
}

// CarDisc is one disc of another car (World.Cars): centre, radius, and the
// height it spans.
type CarDisc struct {
	X  float64 `json:"x"`
	Z  float64 `json:"z"`
	R  float64 `json:"r"`
	Y0 float64 `json:"y0"`
	Y1 float64 `json:"y1"`
}

// NewCar is a car of type sp standing at pos facing yaw.
func NewCar(sp *CarSpec, pos Vec3, yaw float64) *Car {
	return &Car{Pos: pos, Vel: Vec3{0, 0, 0}, Yaw: yaw, YawVel: 0, Pitch: 0, Roll: 0, Steer: 0, OnGround: true, Contact: 15, Comp: [4]float64{0.5, 0.5, 0.5, 0.5}, HP: sp.HP, Wet: false}
}

// CarSpeed is the car's speed over the ground (m/s, horizontal).
func CarSpeed(c *Car) float64 { return detmath.Hypot(c.Vel[0], c.Vel[2]) }

// CarForward is the car's speed along its heading (m/s; < 0 reversing).
func CarForward(c *Car) float64 {
	hx, hz := BoatHeading(c.Yaw)
	return float64(c.Vel[0]*hx) + float64(c.Vel[2]*hz)
}

// CarDiscs appends the car's three wall discs (as World.Cars takes them).
func CarDiscs(sp *CarSpec, c *Car, out []CarDisc) []CarDisc {
	hx, hz := BoatHeading(c.Yaw)
	r := float64(sp.Size[0] / 2) // (a barrier: /2 is a product)
	off := math.Max(0, float64(sp.Size[2]/2)-r)
	for i := -1; i <= 1; i++ {
		o := float64(i) * off
		out = append(out, CarDisc{X: c.Pos[0] + float64(hx*o), Z: c.Pos[2] + float64(hz*o), R: r, Y0: c.Pos[1], Y1: c.Pos[1] + sp.Size[1]})
	}
	return out
}

// StepCar moves car c of type sp by one step of dt with the driver's input.
func (w *World) StepCar(sp *CarSpec, c *Car, in CarInput, dt float64) {
	n := 1 + int(CarSpeed(c)*dt/0.25) // sub-steps: no more than 25 cm each (thin walls)
	if n > 4 {
		n = 4
	}
	h := dt / float64(n)
	for range n {
		w.carDrive(sp, c, in, h)
		w.carMove(sp, c, h)
	}
}

// carDrive applies the steering, then the engine, the brakes and the grip.
// The heading turns first and the velocity stays: the tyres' grip then takes
// the sideways part away (turned into speed along the heading: it carves) or,
// sliding, mostly lets it be (it drifts).
func (w *World) carDrive(sp *CarSpec, c *Car, in CarInput, dt float64) {
	thr, st := clampAbs(in.Throttle, 1), clampAbs(in.Steer, 1)
	c.Steer += float64((st - c.Steer) * math.Min(1, carSteerK*dt))
	hover := sp.Drive == DriveHover
	tracks := sp.Drive == DriveTracks
	grounded := c.OnGround || (hover && c.Wet)
	slide := in.Handbrake && !hover
	fwd := CarForward(c)
	// The steering: right (Steer > 0) turns right (yaw falls).
	turn := -float64(c.Steer * sp.TurnRate * carAirTurn)
	if grounded {
		k := 1.0
		if !tracks && !hover { // wheels: no turning standing, less at a crawl and at speed
			k = math.Min(1, math.Abs(fwd)/math.Max(0.1, sp.TurnFull))
			k *= 1 - float64(0.4*math.Min(1, math.Abs(fwd)/sp.TopSpeed))
		}
		if fwd < -carCrawl && !hover {
			k = -k // reversing: the wheels (or the tracks) swap it
		}
		turn = -float64(c.Steer * sp.TurnRate * k)
		if slide {
			turn *= 1.4 // the rear lets go: it swings round
		}
	}
	c.YawVel += float64((turn - c.YawVel) * math.Min(1, sp.TurnK*dt))
	c.Yaw += float64(c.YawVel * dt)
	hx, hz := BoatHeading(c.Yaw)
	rx, rz := -hz, hx // the car's right
	fwd = float64(c.Vel[0]*hx) + float64(c.Vel[2]*hz)
	lat := float64(c.Vel[0]*rx) + float64(c.Vel[2]*rz)
	if grounded {
		eng := 1.0
		if c.Wet && !hover {
			eng = 0.3
		}
		switch {
		case slide:
			fwd = towards(fwd, 0, float64(sp.Brake*0.6)*dt)
		case thr > 0 && fwd < -carCrawl && !hover:
			fwd = towards(fwd, 0, float64(sp.Brake*thr)*dt)
		case thr < 0 && fwd > carCrawl && !hover:
			fwd = towards(fwd, 0, float64(-sp.Brake*thr)*dt)
		case thr > 0:
			if fwd < sp.TopSpeed {
				fwd = math.Min(sp.TopSpeed, fwd+float64(float64(sp.Accel*thr*eng)*dt))
			}
		case thr < 0:
			if fwd > -sp.Reverse {
				fwd = math.Max(-sp.Reverse, fwd+float64(float64(sp.Accel*thr*eng)*dt))
			}
		}
		roll := 0.3 // m/s² rolling resistance
		if hover {
			roll = 0.8
		}
		fwd = towards(fwd, 0, float64(float64(sp.Drag*math.Abs(fwd))+roll)*dt)
		if fwd > sp.TopSpeed { // downhill over the top: the engine holds it back
			fwd = towards(fwd, sp.TopSpeed, float64(sp.Brake*0.25)*dt)
		}
		// The slope pulls it down (it rolls back off a ramp; a banked turn holds it in).
		fwd -= float64(float64(CarGrav*0.5*detmath.Sin(c.Pitch)) * dt)
		lat += float64(float64(CarGrav*0.5*detmath.Sin(c.Roll)) * dt)
		g := sp.Grip
		if in.Handbrake {
			g = sp.SlideGrip
		}
		nl := float64(lat * math.Max(0, 1-float64(g*dt)))
		if !slide && fwd > carCrawl { // gripping: the sideways speed turns into speed along the heading
			fwd = math.Min(math.Max(fwd, sp.TopSpeed), math.Sqrt(float64(fwd*fwd)+float64(lat*lat)-float64(nl*nl)))
		}
		lat = nl
		if c.Wet && !hover {
			k := math.Max(0, 1-float64(carWaterDrag*dt))
			fwd *= k
			lat *= k
		}
	} else {
		d := math.Max(0, 1-float64(carAirDrag*dt))
		fwd *= d
		lat *= d
	}
	c.Vel[0] = float64(fwd*hx) + float64(lat*rx)
	c.Vel[2] = float64(fwd*hz) + float64(lat*rz)
}

// towards is v moved by up to d (>= 0) towards to.
func towards(v, to, d float64) float64 {
	if v > to {
		return math.Max(to, v-d)
	}
	return math.Min(to, v+d)
}

// carMove moves the car by its velocity: the walls, then the ground under it
// (on it, following it, or falling).
func (w *World) carMove(sp *CarSpec, c *Car, dt float64) {
	y0 := c.Pos[1]
	c.Pos[0] += float64(c.Vel[0] * dt)
	c.Pos[2] += float64(c.Vel[2] * dt)
	for range 2 {
		w.carWalls(sp, c)
	}
	hover := sp.Drive == DriveHover
	wt := w.waterTop(c.Pos[0], c.Pos[1]+0.05, c.Pos[2])
	wet := wt > -1e300 && wt > c.Pos[1]
	// The ground under the four corners (the hover: water too).
	hx, hz := BoatHeading(c.Yaw)
	rx, rz := -hz, hx
	fl, fr := float64(sp.Wheelbase/2), float64(sp.Track/2)
	var g [4]float64
	top := c.Pos[1] + carStepUp
	sum, cnt := 0.0, 0
	for i := range 4 {
		a, s := fl, -fr // front left
		if i == 1 || i == 3 {
			s = fr
		}
		if i >= 2 {
			a = -fl
		}
		x := c.Pos[0] + float64(hx*a) + float64(rx*s)
		z := c.Pos[2] + float64(hz*a) + float64(rz*s)
		g[i] = w.carGround(x, z, top, hover)
		if g[i] > -1e300 {
			sum += g[i]
			cnt++
		}
	}
	gy := math.Inf(-1)
	if cnt > 0 {
		gy = sum / float64(cnt)
	}
	if hover && wet { // on the water: it floats
		gy = math.Max(gy, wt)
		wet = false
		c.Wet = true
	} else {
		c.Wet = wet && !hover
	}
	snap := carSnapDown + float64(CarSpeed(c)*dt*0.9)
	fall := CarGrav
	if c.Wet {
		fall = float64(CarGrav * carWaterGrav)
	}
	if c.OnGround && gy > -1e300 && c.Pos[1]-gy <= snap {
		c.Pos[1] = gy // on the ground: following it (up a ramp, down a slope)
		c.Vel[1] = (gy - y0) / dt
	} else {
		c.Vel[1] -= float64(fall * dt)
		if c.Wet {
			c.Vel[1] = math.Max(c.Vel[1], -2)
		}
		c.Pos[1] += float64(c.Vel[1] * dt)
		c.OnGround = false
		if gy > -1e300 && c.Pos[1] <= gy { // landed
			c.Pos[1] = gy
			c.Vel[1] = 0
			c.OnGround = true
		}
	}
	// The tilt and the suspension: a plane through the corners' ground (a
	// corner over nothing: at the mean), each wheel in contact if its ground
	// is within its travel of the plane (drawn: Comp; Contact).
	var ok [4]bool
	for i := range 4 {
		ok[i] = g[i] > -1e300
		if !ok[i] {
			g[i] = gy
		}
	}
	pitch, roll := 0.0, 0.0
	c.Contact = 0
	if c.OnGround {
		dp := float64((float64(g[0]+g[1]) - float64(g[2]+g[3])) / 4) // the plane's rise from the middle to the front axle
		dr := float64((float64(g[0]+g[2]) - float64(g[1]+g[3])) / 4) // and to the left wheels
		pitch = carAtan(dp * 2 / math.Max(0.1, sp.Wheelbase))
		roll = carAtan(dr * 2 / math.Max(0.1, sp.Track))
		for i := range 4 {
			pl := gy + dp + dr
			if i == 1 || i == 3 {
				pl = gy + dp - dr
			}
			if i >= 2 {
				pl -= float64(2 * dp)
			}
			res := g[i] - pl
			c.Comp[i] = 0
			if ok[i] && math.Abs(res) < sp.Travel {
				c.Contact |= 1 << i
				c.Comp[i] = math.Max(0, math.Min(1, 0.5+float64(res/math.Max(0.01, sp.Travel))))
			}
		}
	} else {
		c.Comp = [4]float64{0, 0, 0, 0}
	}
	k := math.Min(1, carTilt*dt)
	c.Pitch += float64((pitch - c.Pitch) * k)
	c.Roll += float64((roll - c.Roll) * k)
}

// carGround is the highest solid top (a wedge: its slope) at x, z that is no
// higher than top; the hover: water surfaces too. -Inf: none (off the edge).
func (w *World) carGround(x, z, top float64, hover bool) float64 {
	best := math.Inf(-1)
	for i := range w.Boxes {
		b := &w.Boxes[i]
		if x < b.Min[0] || x > b.Max[0] || z < b.Min[2] || z > b.Max[2] {
			continue
		}
		if t := TopAt(b, x, z); t <= top && t > best {
			best = t
		}
	}
	if hover {
		for i := range w.Water {
			b := &w.Water[i]
			if x >= b.Min[0] && x <= b.Max[0] && z >= b.Min[2] && z <= b.Max[2] && b.Max[1] <= top && b.Max[1] > best {
				best = b.Max[1]
			}
		}
	}
	return best
}

// carWalls pushes the car's discs out of the map's boxes that are walls to it
// (their top over its ground by more than carStepUp) and out of the other
// cars (World.Cars), taking away its speed into them.
func (w *World) carWalls(sp *CarSpec, c *Car) {
	hx, hz := BoatHeading(c.Yaw)
	r := float64(sp.Size[0] / 2) // (a barrier: /2 is a product)
	off := math.Max(0, float64(sp.Size[2]/2)-r)
	lo, hi := c.Pos[1]+carStepUp, c.Pos[1]+sp.Size[1]
	for j := -1; j <= 1; j++ {
		o := float64(j) * off
		for i := range w.Boxes {
			bx := &w.Boxes[i]
			if bx.Max[1] <= lo || bx.Min[1] >= hi {
				continue
			}
			cx, cz := c.Pos[0]+float64(hx*o), c.Pos[2]+float64(hz*o)
			px, pz := math.Max(bx.Min[0], math.Min(bx.Max[0], cx)), math.Max(bx.Min[2], math.Min(bx.Max[2], cz))
			if TopAt(bx, px, pz) <= lo { // a wedge low enough here: ground
				continue
			}
			dx, dz := cx-px, cz-pz
			d := detmath.Hypot(dx, dz)
			if d >= r {
				continue
			}
			var nx, nz, push float64
			if d > 1e-9 {
				nx, nz, push = dx/d, dz/d, r-d
			} else { // the centre is in the box: out of its nearest side
				e := [4]float64{cx - bx.Min[0], bx.Max[0] - cx, cz - bx.Min[2], bx.Max[2] - cz}
				m := 0
				for k := 1; k < 4; k++ {
					if e[k] < e[m] {
						m = k
					}
				}
				nn := [4][2]float64{{-1, 0}, {1, 0}, {0, -1}, {0, 1}}
				nx, nz, push = nn[m][0], nn[m][1], e[m]+r
			}
			c.carPush(nx, nz, push)
		}
		for i := range w.Cars {
			q := &w.Cars[i]
			if q.Y1 <= c.Pos[1] || q.Y0 >= hi {
				continue
			}
			cx, cz := c.Pos[0]+float64(hx*o), c.Pos[2]+float64(hz*o)
			dx, dz := cx-q.X, cz-q.Z
			d := detmath.Hypot(dx, dz)
			if d >= r+q.R {
				continue
			}
			if d > 1e-9 {
				c.carPush(dx/d, dz/d, r+q.R-d)
			} else {
				c.carPush(1, 0, r+q.R)
			}
		}
	}
}

// carPush moves the car by push along (nx, nz) and takes its speed into it away.
func (c *Car) carPush(nx, nz, push float64) {
	c.Pos[0] += float64(nx * push)
	c.Pos[2] += float64(nz * push)
	if vn := float64(c.Vel[0]*nx) + float64(c.Vel[2]*nz); vn < 0 {
		c.Vel[0] -= float64(vn * nx)
		c.Vel[2] -= float64(vn * nz)
	}
	c.YawVel *= 0.5
}

// carAtan is atan(x), deterministic (only + * /: the same bits in JS): a
// rational fit good to 1e-5 rad, folded for |x| > 1.
func carAtan(x float64) float64 {
	if x > 1 {
		return math.Pi/2 - carAtan(1/x)
	}
	if x < -1 {
		return -math.Pi/2 - carAtan(1/x)
	}
	x2 := float64(x * x)
	p := float64(0.0208351*x2) - 0.0851330
	p = float64(p*x2) + 0.1801410
	p = float64(p*x2) - 0.3302995
	p = float64(p*x2) + 0.9998660
	return float64(x * p)
}

// CarSeatWorld is where the driver's feet are in car c of type sp: its seat
// (x right, y up, z aft) turned by its heading, raised and lowered by its tilt.
func CarSeatWorld(sp *CarSpec, c *Car) Vec3 {
	hx, hz := BoatHeading(c.Yaw)
	x, y, z := sp.Seat[0], sp.Seat[1], sp.Seat[2]
	lift := -float64(detmath.Sin(c.Pitch)*z) - float64(detmath.Sin(c.Roll)*x)
	return Vec3{c.Pos[0] - float64(hz*x) - float64(hx*z), c.Pos[1] + y + lift, c.Pos[2] + float64(hx*x) - float64(hz*z)}
}

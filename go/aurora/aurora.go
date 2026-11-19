// Package aurora draws the northern lights as a matrix.Scene: folded
// curtains of light that ripple and drift over a dark, starry sky above a
// silhouette of hills and pines, reflected in a still lake.
//
// Each curtain glows like a real aurora, where the colour depends on
// height: a thin blue-violet fringe along the bottom edge (nitrogen), a
// bright green band (oxygen at about 100–200 km) and a faint red crown
// (oxygen higher up). Bright waves sweep along the curtains, and rays
// shimmer up and down them.
package aurora

import (
	"fmt"
	"image/color"
	"math"
	"math/rand"
	"time"

	"dotmatrix/matrix"
)

// Options configure a Scene.
type Options struct {
	Speed float64 // animation speed, up to 10; 0 is 1
}

// Scene implements matrix.Scene.
type Scene struct {
	Speed float64
}

// New returns a Scene, or an error for a speed out of range.
func New(o Options) (*Scene, error) {
	if o.Speed < 0 || o.Speed > 10 {
		return nil, fmt.Errorf("aurora: speed must be between 0 and 10")
	}
	if o.Speed == 0 {
		o.Speed = 1
	}
	return &Scene{Speed: o.Speed}, nil
}

const frameInterval = time.Second / 25

// The lake's shore: rows from here down mirror the sky.
const shore = 27

// curtain is one fold of the aurora. Its lower edge waves across the sky;
// the light fades upwards from it over height pixels.
type curtain struct {
	base, height      float64    // lower edge's mean row, and how far the glow reaches up
	waves             [3]wave    // the lower edge's folds
	bottom, body, top [3]float64 // colours of the fringe, the band and the crown
	glow              float64    // overall brightness
	sweep             wave       // bright pulses travelling along the curtain
}

// wave is amp·sin(k·x + w·t + phase).
type wave struct{ amp, k, w, phase float64 }

func (v wave) at(x, t float64) float64 { return v.amp * math.Sin(v.k*x+v.w*t+v.phase) }

var (
	violet  = [3]float64{0.45, 0.15, 1.0}
	blue    = [3]float64{0.05, 0.3, 1.0}
	green   = [3]float64{0.15, 1.0, 0.35}
	teal    = [3]float64{0.0, 0.9, 0.7}
	red     = [3]float64{1.0, 0.1, 0.25}
	magenta = [3]float64{0.9, 0.1, 0.7}
)

var curtains = []curtain{
	// High and faint, mostly red: the crown of a big display.
	{base: 10, height: 6, waves: [3]wave{{3.0, 0.07, 0.21, 0}, {1.5, 0.17, -0.33, 1}, {0.6, 0.41, 0.8, 2}},
		bottom: magenta, body: red, top: red, glow: 0.75, sweep: wave{1, 0.09, -0.5, 0}},
	// The main curtain: violet fringe, bright green, red crown.
	{base: 18, height: 9, waves: [3]wave{{4.5, 0.09, 0.27, 2}, {2.0, 0.21, -0.41, 0}, {0.7, 0.53, 1.1, 4}},
		bottom: violet, body: green, top: red, glow: 1.2, sweep: wave{1, 0.11, 0.7, 1}},
	// A lower, blue fold in front.
	{base: 23, height: 5, waves: [3]wave{{2.5, 0.12, -0.31, 4}, {1.2, 0.27, 0.5, 3}, {0.5, 0.61, -1.3, 1}},
		bottom: violet, body: blue, top: teal, glow: 1.1, sweep: wave{1, 0.14, -0.9, 3}},
}

// light returns the aurora's colour at sky position (x, y) at time t,
// before tone mapping.
func light(x, y, t float64) [3]float64 {
	var sum [3]float64
	for _, c := range curtains {
		edge := c.base
		for _, w := range c.waves {
			edge += w.at(x, t)
		}
		d := edge - y // how far above the lower edge
		var I float64
		if d < 0 {
			I = math.Exp(-d * d / 0.6) // a sharp lower edge
		} else {
			I = math.Exp(-d / c.height)
		}
		if I < 0.003 {
			continue
		}
		// Rays: fine vertical streaks drifting along the curtain.
		rays := 0.6 + 0.25*math.Sin(1.3*x+0.9*t+c.base) + 0.15*math.Sin(2.9*x-1.7*t)
		// Waves of brightness sweeping along it, and a slow breathing.
		pulse := 0.08 + 0.92*math.Pow(0.5+0.5*math.Sin(c.sweep.k*x+c.sweep.w*t+c.sweep.phase), 2)
		breath := 0.8 + 0.2*math.Sin(0.13*t+c.base)
		I *= rays * pulse * breath * c.glow

		// Colour by height above the edge: fringe → band → crown.
		f := math.Exp(-math.Max(d+0.3, 0) * 1.4)          // fringe, right at the edge
		top := smoothstep(c.height*0.35, c.height*1.1, d) // crown
		for i := range sum {
			col := c.body[i]*(1-top) + c.top[i]*top
			col = col*(1-f) + c.bottom[i]*f
			sum[i] += I * col
		}
	}
	return sum
}

func smoothstep(a, b, x float64) float64 {
	t := math.Max(0, math.Min(1, (x-a)/(b-a)))
	return t * t * (3 - 2*t)
}

// Stars are scattered once; each twinkles at its own pace.
type star struct{ x, y, bright, rate, phase float64 }

var stars = func() []star {
	r := rand.New(rand.NewSource(7))
	s := make([]star, 34)
	for i := range s {
		s[i] = star{
			x: float64(r.Intn(matrix.Width)), y: float64(r.Intn(shore - 4)),
			bright: 0.25 + 0.55*r.Float64(), rate: 1 + 3*r.Float64(), phase: 6.3 * r.Float64(),
		}
	}
	return s
}()

// land is the height of the hills and trees above the shore at each column.
var land = func() [matrix.Width]int {
	var h [matrix.Width]int
	for x := range h {
		fx := float64(x)
		hill := 2.2 + 1.6*math.Sin(fx*0.11+0.6) + 0.9*math.Sin(fx*0.29+2)
		h[x] = int(math.Max(1, hill))
	}
	// Pines: a pointed tip, widening downwards.
	for _, p := range [][2]int{{6, 7}, {9, 5}, {44, 8}, {48, 6}, {51, 4}, {58, 6}} {
		for dx := -2; dx <= 2; dx++ {
			x := p[0] + dx
			if x < 0 || x >= matrix.Width {
				continue
			}
			th := p[1] - 2*abs(dx)
			if dx == 0 {
				th = p[1]
			}
			if th > h[x] {
				h[x] = th
			}
		}
	}
	return h
}()

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// Render implements matrix.Scene.
func (s *Scene) Render(c *matrix.Canvas, elapsed time.Duration) time.Duration {
	speed := s.Speed
	if speed <= 0 {
		speed = 1
	}
	t := elapsed.Seconds() * speed

	var sky [shore][matrix.Width][3]float64
	for y := 0; y < shore; y++ {
		// Deep blue night, a little lighter towards the horizon.
		h := float64(y) / shore
		base := [3]float64{0.0, 0.01 + 0.02*h, 0.04 + 0.06*h}
		for x := 0; x < matrix.Width; x++ {
			l := light(float64(x), float64(y), t)
			for i := range l {
				sky[y][x][i] = base[i] + l[i]
			}
		}
	}
	for _, st := range stars {
		tw := st.bright * (0.6 + 0.4*math.Sin(st.rate*t+st.phase))
		p := &sky[int(st.y)][int(st.x)]
		for i := range p {
			p[i] += tw * [3]float64{0.8, 0.85, 1}[i]
		}
	}

	for y := 0; y < shore; y++ {
		for x := 0; x < matrix.Width; x++ {
			c.Pix[y][x] = tone(sky[y][x], 1)
		}
	}
	// The lake mirrors the sky above the shore, rippling and darker.
	for y := shore; y < matrix.Height; y++ {
		depth := y - shore
		for x := 0; x < matrix.Width; x++ {
			ripple := int(math.Round(0.7 * math.Sin(float64(y)*1.7+t*1.3+float64(x)*0.05)))
			sx := x + ripple
			if sx < 0 {
				sx = 0
			} else if sx >= matrix.Width {
				sx = matrix.Width - 1
			}
			sy := shore - 1 - depth - 3 // reflect from above the hills
			c.Pix[y][x] = tone(sky[sy][sx], 0.7-0.1*float64(depth))
		}
	}
	// Hills and pines in silhouette, standing on the shore.
	for x := 0; x < matrix.Width; x++ {
		for y := shore - land[x]; y < shore; y++ {
			c.Pix[y][x] = color.RGBA{2, 3, 6, 255}
		}
	}
	return frameInterval
}

// tone maps linear light to a pixel, compressing highlights so overlapping
// curtains saturate gracefully instead of clipping.
func tone(v [3]float64, gain float64) color.RGBA {
	var out [3]uint8
	for i, x := range v {
		x = 1 - math.Exp(-x*gain*1.6)
		out[i] = uint8(math.Max(0, math.Min(1, x))*255 + 0.5)
	}
	return color.RGBA{out[0], out[1], out[2], 255}
}

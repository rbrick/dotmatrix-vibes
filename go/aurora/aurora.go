// Package aurora draws the northern lights as a matrix.Scene: folded
// curtains of light that ripple and drift over a dark, starry sky above
// snow-capped mountains and a pine treeline, all mirrored in a lake.
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

// The lake's shore: rows from here down mirror everything above.
const shore = 23

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
	{base: 7, height: 6, waves: [3]wave{{3.0, 0.07, 0.21, 0}, {1.5, 0.17, -0.33, 1}, {0.6, 0.41, 0.8, 2}},
		bottom: magenta, body: red, top: red, glow: 0.75, sweep: wave{1, 0.09, -0.5, 0}},
	// The main curtain: violet fringe, bright green, red crown.
	{base: 13, height: 8, waves: [3]wave{{4.5, 0.09, 0.27, 2}, {2.0, 0.21, -0.41, 0}, {0.7, 0.53, 1.1, 4}},
		bottom: violet, body: green, top: red, glow: 1.2, sweep: wave{1, 0.11, 0.7, 1}},
	// A lower, blue fold in front.
	{base: 17, height: 5, waves: [3]wave{{2.5, 0.12, -0.31, 4}, {1.2, 0.27, 0.5, 3}, {0.5, 0.61, -1.3, 1}},
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
			x: float64(r.Intn(matrix.Width)), y: float64(r.Intn(shore - 9)),
			bright: 0.25 + 0.55*r.Float64(), rate: 1 + 3*r.Float64(), phase: 6.3 * r.Float64(),
		}
	}
	return s
}()

// peaks are the mountain range behind the lake: x, top row, steepness.
var peaks = [][3]float64{{10, 11, 1.0}, {29, 14, 1.1}, {46, 12, 0.9}, {62, 15, 1.2}}

// ridge is the top row of the mountains in each column, and slope tells
// which way the ground faces there: -1 rising to the right (lit), +1
// falling (in shadow).
var ridge, slope = func() (top [matrix.Width]int, side [matrix.Width]float64) {
	for x := range top {
		fx := float64(x)
		best := float64(shore)
		for _, p := range peaks {
			h := p[1] + math.Abs(fx-p[0])*p[2]
			if h < best {
				best = h
				side[x] = math.Copysign(1, fx-p[0])
			}
		}
		// A little crag.
		best += 0.6 * math.Sin(fx*1.3)
		top[x] = int(math.Min(shore-2, math.Round(best)))
	}
	return
}()

// summit is the top row of the peak each column belongs to.
var summit = func() (top [matrix.Width]int) {
	for x := range top {
		fx, best := float64(x), math.Inf(1)
		for _, p := range peaks {
			if h := p[1] + math.Abs(fx-p[0])*p[2]; h < best {
				best, top[x] = h, int(p[1])
			}
		}
	}
	return
}()

// pine is the treeline's colour: black, in silhouette against the dimly
// lit mountains behind.
var pine = [3]float64{0, 0.01, 0.005}

// pines stand along the shore: x and height.
var pines = [][2]int{{2, 5}, {5, 7}, {8, 4}, {19, 3}, {22, 5}, {36, 4}, {39, 6}, {42, 3}, {54, 5}, {57, 7}, {60, 4}}

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

	// Build the picture in linear light, then mirror it into the lake.
	var img [matrix.Height][matrix.Width][3]float64
	for y := 0; y < shore; y++ {
		// Night: dark enough that the panel leaves it off, so the
		// mountains and the aurora stand out against black. (The panel has
		// 16 levels a channel and lights anything brighter than about 16/255
		// at its lowest level, which would turn a dim sky solid navy.)
		h := float64(y) / shore
		base := [3]float64{0.0, 0.002 * h, 0.006 * h}
		for x := 0; x < matrix.Width; x++ {
			l := light(float64(x), float64(y), t)
			for i := range l {
				img[y][x][i] = base[i] + l[i]
			}
		}
	}
	for _, st := range stars {
		tw := st.bright * (0.6 + 0.4*math.Sin(st.rate*t+st.phase))
		p := &img[int(st.y)][int(st.x)]
		for i := range p {
			p[i] += tw * [3]float64{0.8, 0.85, 1}[i]
		}
	}

	// Mountains: snow catches the aurora's colour from the sky above.
	for x := 0; x < matrix.Width; x++ {
		var glow [3]float64
		for y := 2; y < 12; y++ {
			for i := range glow {
				glow[i] += img[y][x][i] / 10
			}
		}
		// A dark silhouette against the aurora, lit just enough for the
		// panel to show it (black would vanish into the night sky), with
		// the far side of each peak a shade darker.
		body := [3]float64{0.07, 0.10, 0.26}
		if slope[x] > 0 {
			body = [3]float64{0.05, 0.07, 0.21}
		}
		for y := ridge[x]; y < shore; y++ {
			var col [3]float64
			switch {
			case y == ridge[x]:
				// The skyline catches the aurora's light, outlining the range.
				for i := range col {
					col[i] = 0.12 + body[i] + glow[i]*0.9
				}
			case y-summit[x] < 3 && y-ridge[x] < 2:
				// A little snow on the summits.
				for i := range col {
					col[i] = [3]float64{0.22, 0.26, 0.40}[i] + glow[i]*0.4
				}
			default:
				for i := range col {
					col[i] = body[i] + glow[i]*0.08
				}
			}
			img[y][x] = col
		}
	}
	// A dark treeline along the shore.
	for _, p := range pines {
		for k := 0; k < p[1]; k++ {
			y := shore - p[1] + k
			half := k / 2
			if k == p[1]-1 {
				half = 0 // the trunk
			}
			for dx := -half; dx <= half; dx++ {
				if x := p[0] + dx; x >= 0 && x < matrix.Width {
					img[y][x] = pine
				}
			}
		}
	}
	for x := 0; x < matrix.Width; x++ {
		img[shore-1][x] = pine
	}

	// The lake mirrors it all, darker and bluer, rippling more further out.
	for y := shore; y < matrix.Height; y++ {
		depth := float64(y - shore)
		sy := 2*shore - 1 - y
		for x := 0; x < matrix.Width; x++ {
			ripple := (0.2 + depth*0.12) * math.Sin(depth*1.9+t*1.5+float64(x)*0.07)
			sx := min(max(int(math.Round(float64(x)+ripple)), 0), matrix.Width-1)
			src := img[sy][sx]
			// Light streaks drifting across the water.
			streak := math.Pow(math.Max(0, math.Sin(float64(x)*0.25-t*0.8+depth*2.3)), 12) * 0.25
			gain := 0.85 - 0.035*depth
			for i := range src {
				img[y][x][i] = src[i]*gain + streak*[3]float64{0.5, 0.7, 1}[i]
			}
		}
	}

	for y := range img {
		for x := range img[y] {
			c.Pix[y][x] = tone(img[y][x], 1)
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

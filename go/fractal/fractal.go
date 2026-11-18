// Package fractal draws animated, colourful Mandelbrot and Julia sets as a
// matrix.Scene.
//
// The Mandelbrot set dives into one of a handful of famous spots on its
// boundary and back out again, then picks the next. The Julia set's
// constant c travels round the edge of the Mandelbrot set's main cardioid,
// so the set melts between spirals, rabbits, dendrites and starbursts. Both
// are coloured with smooth (fractional) escape counts run through cycling
// cosine palettes.
package fractal

import (
	"fmt"
	"image/color"
	"math"
	"time"

	"dotmatrix/matrix"
)

// The sets a Scene can show.
const (
	Mandelbrot = "mandelbrot"
	Julia      = "julia"
	Both       = "both" // take turns: one Mandelbrot dive, then one Julia loop
)

// Options configure a Scene.
type Options struct {
	Set   string  // Mandelbrot, Julia or Both; "" is Both
	Speed float64 // animation speed; 0 is 1
}

// Scene implements matrix.Scene.
type Scene struct {
	Set   string
	Speed float64
}

// New returns a Scene, or an error for an unknown set or negative speed.
func New(o Options) (*Scene, error) {
	switch o.Set {
	case "":
		o.Set = Both
	case Mandelbrot, Julia, Both:
	default:
		return nil, fmt.Errorf("fractal: unknown set %q (want %q, %q or %q)", o.Set, Mandelbrot, Julia, Both)
	}
	if o.Speed < 0 || o.Speed > 10 {
		return nil, fmt.Errorf("fractal: speed must be between 0 and 10")
	}
	if o.Speed == 0 {
		o.Speed = 1
	}
	return &Scene{Set: o.Set, Speed: o.Speed}, nil
}

const frameInterval = time.Second / 25

// How long one Mandelbrot dive (in and back out) and one Julia loop take at
// speed 1.
const (
	diveSeconds  = 30.0
	juliaSeconds = 30.0
)

// Render implements matrix.Scene.
func (s *Scene) Render(c *matrix.Canvas, elapsed time.Duration) time.Duration {
	speed := s.Speed
	if speed <= 0 {
		speed = 1
	}
	t := elapsed.Seconds() * speed
	switch s.Set {
	case Mandelbrot:
		drawMandelbrot(c, t)
	case Julia:
		drawJulia(c, t)
	default:
		// Alternate whole cycles so each switch happens at a zoomed-out view.
		turn := int(t / (diveSeconds + juliaSeconds))
		in := math.Mod(t, diveSeconds+juliaSeconds)
		if in < diveSeconds {
			drawMandelbrot(c, float64(turn)*diveSeconds+in)
		} else {
			drawJulia(c, float64(turn)*juliaSeconds+in-diveSeconds)
		}
	}
	return frameInterval
}

// Spots on the Mandelbrot set's boundary worth zooming into.
// Each was nudged by a search for windows with many colour bands and
// little black at every depth of the dive.
var targets = [][2]float64{
	{-0.745747806737, 0.136308022709},  // Seahorse Valley
	{0.264071578314, -0.002655424998},  // Elephant Valley
	{-0.079209113827, 0.658278028326},  // Triple Spiral Valley
	{-0.761726881497, -0.089168506186}, // double spirals
	{-1.772675450205, 0.005220806495},  // along the main antenna
	{-0.165302305873, -1.039393627598}, // filaments at the top of the set
	{-0.735946468554, 0.192183531446},  // seahorses' tails
	{0.302599608368, 0.021291083679},   // the elephants' outer edge
}

// home is the zoomed-out view: the whole set fits the 2:1 panel.
var home = [2]float64{-0.6, 0}

const (
	homeWidth = 3.6   // width of the panel in the complex plane, zoomed out
	maxZoom   = 2e4   // how far each dive goes
	bailout   = 256.0 // a big escape radius smooths the colour bands
)

func drawMandelbrot(c *matrix.Canvas, t float64) {
	n := int(t / diveSeconds)
	phase := math.Mod(t, diveSeconds) / diveSeconds
	target := targets[n%len(targets)]
	// Ease in and back out: 0 at either end of the dive, 1 in the middle.
	depth := (1 - math.Cos(2*math.Pi*phase)) / 2
	zoom := math.Pow(maxZoom, depth)
	width := homeWidth / zoom
	// The target drifts to the middle of the panel as the view narrows, so
	// a dive both starts and ends exactly at home.
	cx := target[0] + (home[0]-target[0])/zoom
	cy := target[1] + (home[1]-target[1])/zoom
	maxIter := 64 + int(48*math.Log10(zoom))
	pal := palettes[n%len(palettes)]
	shift := t * 0.08

	step := width / matrix.Width
	render(c, step, func(x, y float64) color.RGBA {
		x0, y0 := cx+x, cy+y
		if inMainBulbs(x0, y0) {
			return color.RGBA{A: 255}
		}
		mu, escaped := escape(x0, y0, x0, y0, maxIter)
		return shade(pal, mu, escaped, shift)
	})
}

// juliaC is the Julia constant at angle θ round the Mandelbrot set's main
// cardioid. Constants on its edge make the most intricate Julia sets; the
// path weaves a little inside (fat, filled shapes) and outside (lacy
// dendrites) so the set keeps changing character.
func juliaC(theta float64) (float64, float64) {
	sin, cos := math.Sincos(theta)
	sin2, cos2 := math.Sincos(2 * theta)
	r := 1.02 + 0.05*math.Sin(3*theta)
	return r * (cos/2 - cos2/4), r * (sin/2 - sin2/4)
}

func drawJulia(c *matrix.Canvas, t float64) {
	cr, ci := juliaC(2 * math.Pi * t / juliaSeconds)
	// Turn the picture slowly so it never sits still.
	sin, cos := math.Sincos(t * 0.15)
	const width = 3.6
	pal := palettes[(int(t/juliaSeconds)+2)%len(palettes)]
	shift := t * 0.1
	render(c, width/matrix.Width, func(x, y float64) color.RGBA {
		mu, escaped := escape(x*cos-y*sin, x*sin+y*cos, cr, ci, 128)
		return shade(pal, mu, escaped, shift)
	})
}

// render fills the canvas from sample, which colours the point (x, y)
// relative to the middle of the panel, step apart per pixel. Pixels that
// differ sharply from a neighbour are supersampled, which calms the
// glittering noise along the sets' edges without paying for every pixel.
func render(c *matrix.Canvas, step float64, sample func(x, y float64) color.RGBA) {
	for py := 0; py < matrix.Height; py++ {
		for px := 0; px < matrix.Width; px++ {
			c.Pix[py][px] = sample((float64(px)-matrix.Width/2+0.5)*step, (float64(py)-matrix.Height/2+0.5)*step)
		}
	}
	var edge [matrix.Height][matrix.Width]bool
	for py := 0; py < matrix.Height; py++ {
		for px := 0; px < matrix.Width; px++ {
			if px+1 < matrix.Width && far(c.Pix[py][px], c.Pix[py][px+1]) {
				edge[py][px], edge[py][px+1] = true, true
			}
			if py+1 < matrix.Height && far(c.Pix[py][px], c.Pix[py+1][px]) {
				edge[py][px], edge[py+1][px] = true, true
			}
		}
	}
	// Four more samples in a rotated grid, averaged with the centre.
	offsets := [4][2]float64{{-0.375, -0.125}, {0.125, -0.375}, {0.375, 0.125}, {-0.125, 0.375}}
	for py := 0; py < matrix.Height; py++ {
		for px := 0; px < matrix.Width; px++ {
			if !edge[py][px] {
				continue
			}
			p := c.Pix[py][px]
			r, g, b := int(p.R), int(p.G), int(p.B)
			for _, o := range offsets {
				q := sample((float64(px)-matrix.Width/2+0.5+o[0])*step, (float64(py)-matrix.Height/2+0.5+o[1])*step)
				r, g, b = r+int(q.R), g+int(q.G), b+int(q.B)
			}
			c.Pix[py][px] = color.RGBA{uint8(r / 5), uint8(g / 5), uint8(b / 5), 255}
		}
	}
}

// far reports whether two colours differ enough to be worth supersampling.
func far(a, b color.RGBA) bool {
	d := abs(int(a.R)-int(b.R)) + abs(int(a.G)-int(b.G)) + abs(int(a.B)-int(b.B))
	return d > 120
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// escape iterates z → z² + c from z and returns the smooth escape count, or
// false if z hasn't escaped after maxIter steps.
func escape(zr, zi, cr, ci float64, maxIter int) (float64, bool) {
	for i := 0; i < maxIter; i++ {
		zr2, zi2 := zr*zr, zi*zi
		if zr2+zi2 > bailout*bailout {
			// Fractional iteration count: no bands between whole steps.
			return float64(i) + 1 - math.Log2(math.Log(zr2+zi2)/2), true
		}
		zr, zi = zr2-zi2+cr, 2*zr*zi+ci
	}
	return 0, false
}

// inMainBulbs reports whether c is inside the main cardioid or the period-2
// bulb, which never escape: skipping them saves most of the work zoomed out.
func inMainBulbs(x, y float64) bool {
	q := (x-0.25)*(x-0.25) + y*y
	if q*(q+(x-0.25)) <= 0.25*y*y {
		return true
	}
	return (x+1)*(x+1)+y*y <= 1.0/16
}

// A palette is a cosine gradient, a + b·cos(2π(c·t + d)) per channel.
type palette struct{ a, b, c, d [3]float64 }

var palettes = []palette{
	{[3]float64{0.5, 0.5, 0.5}, [3]float64{0.5, 0.5, 0.5}, [3]float64{1, 1, 1}, [3]float64{0, 0.33, 0.67}},    // rainbow
	{[3]float64{0.5, 0.5, 0.5}, [3]float64{0.5, 0.5, 0.5}, [3]float64{1, 1, 0.5}, [3]float64{0.8, 0.9, 0.3}},  // fire and teal
	{[3]float64{0.5, 0.5, 0.5}, [3]float64{0.5, 0.5, 0.5}, [3]float64{1, 0.7, 0.4}, [3]float64{0, 0.15, 0.2}}, // sunset
	{[3]float64{0.5, 0.5, 0.5}, [3]float64{0.5, 0.5, 0.5}, [3]float64{2, 1, 0}, [3]float64{0.5, 0.2, 0.25}},   // magenta and lime
	{[3]float64{0.8, 0.5, 0.4}, [3]float64{0.2, 0.4, 0.2}, [3]float64{2, 1, 1}, [3]float64{0, 0.25, 0.25}},    // candy
}

// shade colours a point by its smooth escape count; points in the set are
// black. shift moves the colours along the palette over time.
func shade(p palette, mu float64, escaped bool, shift float64) color.RGBA {
	if !escaped {
		return color.RGBA{A: 255}
	}
	// Spread the colours by the log of the count so deep zooms, where
	// counts are large, still show many bands.
	t := math.Log(mu+1)*0.6 + shift
	// Fade in from black near the outside so the background isn't a flat
	// wash of the first colour.
	fade := math.Min(1, mu/6)
	var rgb [3]uint8
	for i := range rgb {
		v := p.a[i] + p.b[i]*math.Cos(2*math.Pi*(p.c[i]*t+p.d[i]))
		v = math.Max(0, math.Min(1, v)) * fade
		rgb[i] = uint8(v*255 + 0.5)
	}
	return color.RGBA{rgb[0], rgb[1], rgb[2], 255}
}

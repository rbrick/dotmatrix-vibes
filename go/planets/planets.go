// Package planets renders rotating bodies of the solar system as a
// matrix.Scene: textured, lit spheres over a twinkling star field.
package planets

import (
	"fmt"
	"image/color"
	"math"
	"math/rand/v2"
	"strings"
	"time"

	"dotmatrix/matrix"
)

type body struct {
	name     string
	surface  func() *texture
	clouds   func() *texture // optional layer that drifts over the surface
	radius   float64         // pixels
	tilt     float64         // lean of the axis in the picture, degrees clockwise
	tip      float64         // how far the north pole leans towards the viewer, degrees
	period   float64         // seconds per turn at speed 1; negative turns backwards
	emissive bool            // glows instead of being lit (the Sun)
	rings    bool
}

var bodies = []body{
	{name: "sun", surface: sunTexture, radius: 13.5, period: 25, emissive: true},
	{name: "mercury", surface: mercuryTexture, radius: 10, tip: 5, period: 14},
	{name: "venus", surface: venusTexture, radius: 12, tip: 5, period: -20},
	{name: "earth", surface: earthTexture, clouds: earthClouds, radius: 13, tilt: 23, tip: 15, period: 12},
	{name: "moon", surface: moonTexture, radius: 10, tip: 5, period: 16},
	{name: "mars", surface: marsTexture, radius: 11, tilt: 25, tip: 15, period: 12},
	{name: "jupiter", surface: jupiterTexture, radius: 14, tilt: 3, tip: 5, period: 8},
	{name: "saturn", surface: saturnTexture, radius: 8.5, tilt: -12, tip: 22, period: 8, rings: true},
	{name: "uranus", surface: uranusTexture, radius: 12, tilt: 98, tip: 10, period: 10},
	{name: "neptune", surface: neptuneTexture, radius: 12, tilt: 28, tip: 10, period: 10},
	{name: "pluto", surface: plutoTexture, radius: 9, tilt: 10, tip: 5, period: 14},
}

// Names lists the bodies in order from the Sun.
func Names() []string {
	names := make([]string, len(bodies))
	for i, b := range bodies {
		names[i] = b.name
	}
	return names
}

// Tour is the Body value that cycles through every body.
const Tour = "tour"

// Options configures a Scene.
type Options struct {
	Body  string        // one of Names, or Tour
	Label bool          // show the name in the corner
	Speed float64       // rotation speed multiplier, default 1
	Dwell time.Duration // time per body on the tour, default 12s
}

// Scene implements matrix.Scene.
type Scene struct {
	Options
	stars []star
}

type star struct {
	x, y          int
	bright, phase float64
	rate          float64
}

// New validates opts and returns a Scene.
func New(opts Options) (*Scene, error) {
	opts.Body = strings.ToLower(opts.Body)
	if opts.Body == "" {
		opts.Body = Tour
	}
	if opts.Body != Tour && index(opts.Body) < 0 {
		return nil, fmt.Errorf("planets: unknown body %q (have %s, %s)", opts.Body, strings.Join(Names(), ", "), Tour)
	}
	if opts.Speed <= 0 {
		opts.Speed = 1
	}
	if opts.Dwell <= 0 {
		opts.Dwell = 12 * time.Second
	}
	rng := rand.New(rand.NewPCG(42, 42))
	s := &Scene{Options: opts}
	for i := 0; i < 45; i++ {
		s.stars = append(s.stars, star{
			x: rng.IntN(matrix.Width), y: rng.IntN(matrix.Height),
			bright: 0.15 + 0.5*rng.Float64()*rng.Float64(), phase: rng.Float64() * 2 * math.Pi, rate: 0.5 + 2*rng.Float64(),
		})
	}
	return s, nil
}

func index(name string) int {
	for i, b := range bodies {
		if b.name == name {
			return i
		}
	}
	return -1
}

const (
	frameInterval = time.Second / 30
	fadeTime      = 0.5 // seconds to fade between bodies on the tour
	samples       = 3   // supersampling per axis, for smooth edges
)

// Sunlight comes from the upper left, slightly in front of the viewer
// (x right, y down, z towards the viewer).
var light = vec3{-0.7, -0.35, 0.62}.normalize()

// Render implements matrix.Scene.
func (s *Scene) Render(c *matrix.Canvas, elapsed time.Duration) time.Duration {
	t := elapsed.Seconds()
	b, fade := s.current(t)

	cx := float64(matrix.Width) / 2
	if s.Label {
		cx += 6 // leave room for the name in the top-left corner
	}
	cy := float64(matrix.Height) / 2
	spin := 2 * math.Pi * t * s.Speed / b.period

	reach := b.radius * 1.4 // covers the Sun's glow
	if b.rings {
		reach = b.radius * 2.35
	}
	for py := 0; py < matrix.Height; py++ {
		for px := 0; px < matrix.Width; px++ {
			bg := s.starAt(px, py, t)
			if math.Hypot(float64(px)+0.5-cx, float64(py)+0.5-cy) > reach+1 {
				c.Set(px, py, toColor(bg.scale(fade)))
				continue
			}
			var sum rgb
			cover := 0.0
			for i := 0; i < samples; i++ {
				for j := 0; j < samples; j++ {
					dx := (float64(px) + (float64(i)+0.5)/samples - cx) / b.radius
					dy := (float64(py) + (float64(j)+0.5)/samples - cy) / b.radius
					col, a := b.sample(dx, dy, spin, t*s.Speed)
					sum = rgb{sum.r + col.r, sum.g + col.g, sum.b + col.b}
					cover += a
				}
			}
			n := float64(samples * samples)
			cover /= n
			out := rgb{sum.r/n + bg.r*(1-cover), sum.g/n + bg.g*(1-cover), sum.b/n + bg.b*(1-cover)}
			c.Set(px, py, toColor(out.scale(fade)))
		}
	}
	if s.Label {
		c.DrawSmallText(1, 1, b.name, color.RGBA{uint8(150 * fade), uint8(150 * fade), uint8(150 * fade), 255})
	}
	return frameInterval
}

// current returns the body on screen at t and its fade-in/out level.
func (s *Scene) current(t float64) (body, float64) {
	if s.Body != Tour {
		return bodies[index(s.Body)], 1
	}
	dwell := s.Dwell.Seconds()
	i := int(t/dwell) % len(bodies)
	in := math.Mod(t, dwell)
	return bodies[i], clamp01(math.Min(in, dwell-in) / fadeTime)
}

func (s *Scene) starAt(x, y int, t float64) rgb {
	for _, st := range s.stars {
		if st.x == x && st.y == y {
			v := st.bright * (0.7 + 0.3*math.Sin(t*st.rate+st.phase))
			return rgb{v, v, v}
		}
	}
	return rgb{}
}

// sample returns the premultiplied colour and coverage of the body at
// (dx, dy), measured in planet radii from its centre (y down).
func (b *body) sample(dx, dy, spin, t float64) (rgb, float64) {
	// Rotate into the planet's frame: undo the lean in the picture, then
	// the tip towards the viewer.
	tilt, tip := b.tilt*math.Pi/180, b.tip*math.Pi/180
	x1 := dx*math.Cos(tilt) + dy*math.Sin(tilt)
	up := -(-dx*math.Sin(tilt) + dy*math.Cos(tilt))

	r2 := dx*dx + dy*dy
	onSphere := r2 < 1
	var sphere rgb
	sphereZ := -math.MaxFloat64
	if onSphere {
		nz := math.Sqrt(1 - r2)
		sphereZ = nz
		upP := up*math.Cos(tip) + nz*math.Sin(tip)
		zP := -up*math.Sin(tip) + nz*math.Cos(tip)
		lat := math.Asin(math.Max(-1, math.Min(1, upP)))
		lon := math.Atan2(x1, zP) + spin
		sphere, _ = b.surface().at(lat, lon)
		if b.clouds != nil {
			cl, a := b.clouds().at(lat, lon+0.15*t) // clouds drift over the ground
			sphere = mix(sphere, cl, a*0.9)
		}
		if b.emissive {
			sphere = sphere.scale(0.55 + 0.45*nz) // limb darkening
		} else {
			d := math.Max(0, vec3{dx, dy, nz}.dot(light))
			sphere = sphere.scale(0.03 + 0.97*d)
		}
	}

	if b.rings {
		if ring, a, z, ok := saturnRing(x1, up, tip); ok {
			if !onSphere {
				return ring.scale(a), a
			}
			if z > sphereZ { // ring passes in front of the planet
				return mix(sphere, ring, a), 1
			}
		}
	}
	if onSphere {
		return sphere, 1
	}
	if b.emissive {
		if r := math.Sqrt(r2); r < 1.4 {
			a := math.Pow(1-(r-1)/0.4, 2) * 0.45 // corona glow
			return hex(0xff5010).scale(a), a
		}
	}
	return rgb{}, 0
}

// saturnRing finds where the view ray through (x, up) crosses the ring
// plane, returning the ring's colour, opacity and depth there.
func saturnRing(x, up, tip float64) (rgb, float64, float64, bool) {
	sin, cos := math.Sin(tip), math.Cos(tip)
	if sin < 0.05 {
		return rgb{}, 0, 0, false
	}
	r := math.Hypot(x, up/sin)
	z := -up * cos / sin
	var c rgb
	var a float64
	switch {
	case r < 1.24 || r > 2.27:
		return rgb{}, 0, 0, false
	case r < 1.52: // C ring: faint
		c, a = hex(0x8a7a60), 0.35
	case r < 1.95: // B ring: bright
		c, a = hex(0xe8d8a8), 0.95
	case r < 2.03: // Cassini division
		c, a = hex(0x605848), 0.1
	default: // A ring
		c, a = hex(0xc8b48c), 0.75
	}
	return c.scale(0.95), a, z, true
}

func toColor(c rgb) color.RGBA {
	return color.RGBA{uint8(clamp01(c.r)*255 + 0.5), uint8(clamp01(c.g)*255 + 0.5), uint8(clamp01(c.b)*255 + 0.5), 255}
}

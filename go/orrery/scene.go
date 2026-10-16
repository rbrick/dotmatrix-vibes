// Package orrery shows the planets where they really are around the Sun,
// seen from above the plane of the solar system at an angle.
//
// Directions are exact (heliocentric ecliptic longitudes from JPL's orbital
// elements), but distances are not to scale: true distances would put
// Mercury, Venus, Earth and Mars inside the Sun's pixels. Each orbit is
// given its own ring instead, and distances between rings are interpolated,
// so eccentric orbits such as Mercury's and Pluto's keep their shape.
package orrery

import (
	"image/color"
	"math"
	"strings"
	"time"

	"dotmatrix/matrix"
)

// Options configures a Scene.
type Options struct {
	// Speed runs time faster, in simulated days per second; 0 shows the
	// present.
	Speed float64
	// Label names each planet in turn in the top-left corner and makes it
	// blink.
	Label bool
}

// Scene implements matrix.Scene.
type Scene struct {
	Options
	now func() time.Time // overridable for tests
}

func New(opts Options) *Scene {
	if opts.Speed < 0 {
		opts.Speed = 0
	}
	return &Scene{Options: opts}
}

// ringRadius is the screen radius (along the long axis) of each planet's
// orbit, by semi-major axis.
var ringRadius = []float64{4.5, 7.5, 10.5, 13.5, 17, 20.5, 24, 27, 29.5}

// screenRadius maps a true distance from the Sun in AU to pixels, linearly
// between the rings. Beyond Neptune the scale is squeezed, so that Pluto's
// very eccentric orbit (30-49 AU) stays on the panel.
func screenRadius(au float64) float64 {
	prevAU, prevPx := 0.0, 0.0
	for i, p := range planets {
		if au <= p.el.a || i == len(planets)-1 {
			px := prevPx + (au-prevAU)*(ringRadius[i]-prevPx)/(p.el.a-prevAU)
			if au > p.el.a { // beyond Pluto's average distance
				px = ringRadius[i] + (au-p.el.a)*0.12
			}
			return px
		}
		prevAU, prevPx = p.el.a, ringRadius[i]
	}
	return 0 // unreachable
}

// The view looks down on the ecliptic from 29° above it, which squashes the
// orbits into ellipses half as tall as they are wide, to fit the panel.
var (
	viewSin = 0.48
	viewCos = math.Sqrt(1 - viewSin*viewSin)
)

const cx, cy = 31.5, 15.5

// project returns the screen position of a heliocentric position and
// whether it is on the far side of the Sun.
func project(v vec) (x, y float64, far bool) {
	r := v.length()
	if r == 0 {
		return cx, cy, false
	}
	s := screenRadius(r) / r
	return cx + v.x*s, cy - (v.y*viewSin+v.z*viewCos)*s, v.y > 0
}

var (
	planetColors = []color.RGBA{
		{170, 160, 150, 255}, // Mercury
		{240, 220, 150, 255}, // Venus
		{60, 140, 255, 255},  // Earth
		{255, 80, 30, 255},   // Mars
		{235, 175, 110, 255}, // Jupiter
		{230, 200, 110, 255}, // Saturn
		{120, 230, 230, 255}, // Uranus
		{50, 90, 255, 255},   // Neptune
		{190, 150, 120, 255}, // Pluto
	}
	sunColor  = color.RGBA{255, 210, 40, 255}
	glowColor = color.RGBA{200, 90, 0, 255}
	ringColor = color.RGBA{150, 130, 90, 255} // Saturn's rings
	dateColor = color.RGBA{130, 130, 130, 255}
)

const (
	labelDwell    = 3 * time.Second // per planet when labelling
	blink         = 300 * time.Millisecond
	frameInterval = time.Second / 30
)

// Render implements matrix.Scene.
func (s *Scene) Render(c *matrix.Canvas, elapsed time.Duration) time.Duration {
	now := time.Now()
	if s.now != nil {
		now = s.now()
	}
	at := now.Add(time.Duration(s.Speed * float64(elapsed) * 86400))

	c.Clear()
	for i, p := range planets {
		drawOrbit(c, p.orbitAt(at), planetColors[i])
	}
	type dot struct {
		i    int
		x, y int
	}
	var near, far []dot
	for i := range planets {
		x, y, behind := project(position(i, at))
		d := dot{i, int(math.Floor(x)), int(math.Floor(y))}
		if behind {
			far = append(far, d)
		} else {
			near = append(near, d)
		}
	}
	labelled := -1
	if s.Label {
		labelled = int(elapsed/labelDwell) % len(planets)
	}
	blinkOn := elapsed%(2*blink) < blink
	drawPlanets := func(ds []dot) {
		for _, d := range ds {
			col := planetColors[d.i]
			if d.i == labelled && blinkOn {
				col = color.RGBA{255, 255, 255, 255}
			}
			drawPlanet(c, d.i, d.x, d.y, col)
		}
	}
	drawPlanets(far)
	drawSun(c)
	drawPlanets(near)

	if s.Label {
		outlined(c, 1, 1, strings.ToUpper(planets[labelled].name), planetColors[labelled])
	}
	if s.Speed > 0 {
		date := strings.ToUpper(at.Format("Jan 2006"))
		outlined(c, matrix.Width-matrix.SmallTextWidth(date)-1, matrix.Height-matrix.SmallTextHeight-1, date, dateColor)
		return frameInterval
	}
	// Planets move slowly; redraw each minute, or to blink.
	next := now.Truncate(time.Minute).Add(time.Minute).Sub(now)
	if s.Label {
		next = min(next, blink-elapsed%blink)
	}
	return next
}

// orbitDots is the spacing in pixels of the dots that trace each orbit;
// solid lines would merge where the ellipses crowd together.
const orbitDots = 4

// drawOrbit traces the whole orbit as faint dots tinted like the planet.
// The dots are evenly spaced on screen, starting from the planet's
// perihelion so they stay put as time passes.
func drawOrbit(c *matrix.Canvas, o orbit, col color.RGBA) {
	faint := color.RGBA{col.R / 8, col.G / 8, col.B / 8, 255} // the panel's lowest level
	const steps = 720
	px, py, _ := project(o.at(0))
	travelled := float64(orbitDots) // draw the first dot
	for k := 1; k <= steps; k++ {
		x, y, _ := project(o.at(2 * math.Pi * float64(k) / steps))
		travelled += math.Hypot(x-px, y-py)
		px, py = x, y
		if travelled >= orbitDots {
			travelled -= orbitDots
			c.Set(int(math.Floor(x)), int(math.Floor(y)), faint)
		}
	}
}

func drawSun(c *matrix.Canvas) {
	for y := matrix.Height/2 - 4; y <= matrix.Height/2+3; y++ {
		for x := matrix.Width/2 - 4; x <= matrix.Width/2+3; x++ {
			d := math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy)
			switch {
			case d < 1.6:
				c.Set(x, y, sunColor)
			case d < 2.4:
				c.Set(x, y, glowColor)
			}
		}
	}
}

// drawPlanet draws one pixel per planet, except the giants: Jupiter is 2x2
// and Saturn 2x2 with its rings either side.
func drawPlanet(c *matrix.Canvas, i, x, y int, col color.RGBA) {
	c.Set(x, y, col)
	switch planets[i].name {
	case "Saturn":
		c.Set(x-1, y, ringColor)
		c.Set(x+2, y, ringColor)
		fallthrough
	case "Jupiter":
		c.Set(x+1, y, col)
		c.Set(x, y+1, col)
		c.Set(x+1, y+1, col)
	}
}

// outlined draws small text with a black outline, so it stays readable
// over the orbits.
func outlined(c *matrix.Canvas, x, y int, s string, col color.Color) {
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			c.DrawSmallText(x+dx, y+dy, s, color.Black)
		}
	}
	c.DrawSmallText(x, y, s, col)
}

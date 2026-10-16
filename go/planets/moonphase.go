package planets

import (
	"math"

	"dotmatrix/matrix"
)

// DrawMoon draws the Moon centred at (cx, cy) with the given radius in
// pixels, lit as it appears from Earth when it is elongation degrees east of
// the Sun (0 new, 90 first quarter, 180 full, 270 last quarter). As seen
// from the northern hemisphere the waxing Moon is lit on the right; mirrored
// flips it for the southern hemisphere. Only pixels on the disc are drawn,
// blended onto what is already there at the edge.
func DrawMoon(c *matrix.Canvas, cx, cy, radius, elongation float64, mirrored bool) {
	e := elongation * math.Pi / 180
	sun := vec3{math.Sin(e), 0, -math.Cos(e)} // x right, y down, z towards the viewer
	if mirrored {
		sun.x = -sun.x
	}
	tex := moonTexture()
	for py := int(cy - radius - 1); py <= int(cy+radius+1); py++ {
		for px := int(cx - radius - 1); px <= int(cx+radius+1); px++ {
			var sum rgb
			cover := 0.0
			for i := 0; i < samples; i++ {
				for j := 0; j < samples; j++ {
					dx := (float64(px) + (float64(i)+0.5)/samples - cx) / radius
					dy := (float64(py) + (float64(j)+0.5)/samples - cy) / radius
					r2 := dx*dx + dy*dy
					if r2 >= 1 {
						continue
					}
					nz := math.Sqrt(1 - r2)
					ground, _ := tex.at(math.Asin(-dy), math.Atan2(dx, nz))
					// Stretch the contrast so the dark seas survive the
					// panel's 16 levels.
					ground = rgb{maria(ground.r), maria(ground.g), maria(ground.b)}
					// The full Moon looks evenly bright rather than shaded like
					// a ball, so light the day side almost flat with a soft
					// terminator. The night side is a flat trace of earthshine
					// at the panel's lowest level, so even a new Moon shows
					// as a faint disc.
					day := smooth(clamp01((vec3{dx, dy, nz}.dot(sun) + 0.04) / 0.16))
					col := mix(earthshine, ground, day)
					sum = rgb{sum.r + col.r, sum.g + col.g, sum.b + col.b}
					cover++
				}
			}
			if cover == 0 {
				continue
			}
			n := float64(samples * samples)
			bg := c.At(px, py)
			br, bgg, bb, _ := bg.RGBA()
			under := rgb{float64(br) / 0xffff, float64(bgg) / 0xffff, float64(bb) / 0xffff}.scale(1 - cover/n)
			c.Set(px, py, toColor(rgb{sum.r/n + under.r, sum.g/n + under.g, sum.b/n + under.b}))
		}
	}
}

// maria stretches the contrast of the Moon texture around its average.
func maria(v float64) float64 { return math.Max(0.3, 0.62+(v-0.62)*1.5) }

var earthshine = rgb{0.09, 0.09, 0.095}

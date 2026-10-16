package planets

import "math"

// hash returns a pseudo-random value in [0, 1) for an integer lattice point.
func hash(x, y, z int32, seed uint32) float64 {
	h := uint32(x)*0x8da6b343 ^ uint32(y)*0xd8163841 ^ uint32(z)*0xcb1ab31f ^ seed*0x9e3779b9
	h ^= h >> 13
	h *= 0x5bd1e995
	h ^= h >> 15
	return float64(h) / (1 << 32)
}

func smooth(t float64) float64 { return t * t * (3 - 2*t) }

func lerp(a, b, t float64) float64 { return a + (b-a)*t }

// noise is 3D value noise in [0, 1). Sampling it at points on a sphere gives
// textures without a seam or pinched poles.
func noise(x, y, z float64, seed uint32) float64 {
	fx, fy, fz := math.Floor(x), math.Floor(y), math.Floor(z)
	ix, iy, iz := int32(fx), int32(fy), int32(fz)
	tx, ty, tz := smooth(x-fx), smooth(y-fy), smooth(z-fz)
	c := func(dx, dy, dz int32) float64 { return hash(ix+dx, iy+dy, iz+dz, seed) }
	return lerp(
		lerp(lerp(c(0, 0, 0), c(1, 0, 0), tx), lerp(c(0, 1, 0), c(1, 1, 0), tx), ty),
		lerp(lerp(c(0, 0, 1), c(1, 0, 1), tx), lerp(c(0, 1, 1), c(1, 1, 1), tx), ty),
		tz)
}

// fbm adds octaves of noise at doubling frequency and halving weight,
// normalised to [0, 1).
func fbm(p vec3, scale float64, octaves int, seed uint32) float64 {
	sum, weight, total := 0.0, 1.0, 0.0
	for i := 0; i < octaves; i++ {
		sum += weight * noise(p.x*scale, p.y*scale, p.z*scale, seed+uint32(i))
		total += weight
		weight /= 2
		scale *= 2
	}
	return sum / total
}

type vec3 struct{ x, y, z float64 }

func (a vec3) dot(b vec3) float64 { return a.x*b.x + a.y*b.y + a.z*b.z }

func (a vec3) normalize() vec3 {
	l := math.Sqrt(a.dot(a))
	return vec3{a.x / l, a.y / l, a.z / l}
}

// onSphere converts latitude and longitude (radians) to a unit vector.
func onSphere(lat, lon float64) vec3 {
	return vec3{math.Cos(lat) * math.Cos(lon), math.Sin(lat), math.Cos(lat) * math.Sin(lon)}
}

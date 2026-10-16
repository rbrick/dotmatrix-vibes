package planets

import (
	"math"
	"math/rand/v2"
	"sync"
)

// rgb is a colour with channels in [0, 1].
type rgb struct{ r, g, b float64 }

func hex(v uint32) rgb {
	return rgb{float64(v>>16&0xff) / 255, float64(v>>8&0xff) / 255, float64(v&0xff) / 255}
}

func mix(a, b rgb, t float64) rgb {
	t = clamp01(t)
	return rgb{lerp(a.r, b.r, t), lerp(a.g, b.g, t), lerp(a.b, b.b, t)}
}

func (c rgb) scale(f float64) rgb { return rgb{c.r * f, c.g * f, c.b * f} }

func clamp01(t float64) float64 { return math.Max(0, math.Min(1, t)) }

// texture is an equirectangular map: x is longitude, y is latitude (north
// at the top). Alpha is only used by cloud layers.
type texture struct {
	w, h  int
	pix   []rgb
	alpha []float64
}

const texW, texH = 256, 128

// bake evaluates f for every texel; lat and lon are in radians and p is the
// matching point on the unit sphere.
func bake(f func(p vec3, lat, lon float64) (rgb, float64)) *texture {
	t := &texture{w: texW, h: texH, pix: make([]rgb, texW*texH), alpha: make([]float64, texW*texH)}
	for y := 0; y < texH; y++ {
		lat := math.Pi/2 - (float64(y)+0.5)/texH*math.Pi
		for x := 0; x < texW; x++ {
			lon := (float64(x)+0.5)/texW*2*math.Pi - math.Pi
			t.pix[y*texW+x], t.alpha[y*texW+x] = f(onSphere(lat, lon), lat, lon)
		}
	}
	return t
}

func (t *texture) at(lat, lon float64) (rgb, float64) {
	x := int((lon/(2*math.Pi) + 0.5) * float64(t.w))
	x = ((x % t.w) + t.w) % t.w
	y := int((0.5 - lat/math.Pi) * float64(t.h))
	y = max(0, min(t.h-1, y))
	return t.pix[y*t.w+x], t.alpha[y*t.w+x]
}

// lazy bakes a texture the first time it is needed.
func lazy(f func(p vec3, lat, lon float64) (rgb, float64)) func() *texture {
	var once sync.Once
	var t *texture
	return func() *texture {
		once.Do(func() { t = bake(f) })
		return t
	}
}

func opaque(f func(p vec3, lat, lon float64) rgb) func() *texture {
	return lazy(func(p vec3, lat, lon float64) (rgb, float64) { return f(p, lat, lon), 1 })
}

// spot reports how far inside an ellipse centred at (lat0, lon0) a point is:
// 1 at the centre, 0 at the edge and beyond.
func spot(lat, lon, lat0, lon0, rLat, rLon float64) float64 {
	dLon := math.Remainder(lon-lon0, 2*math.Pi)
	d := math.Hypot((lat-lat0)/rLat, dLon/rLon)
	return clamp01(1 - d)
}

type crater struct {
	center vec3
	size   float64 // angular radius, as 1 - cos(angle)
}

func craters(n int, seed uint64) []crater {
	rng := rand.New(rand.NewPCG(seed, 1))
	cs := make([]crater, n)
	for i := range cs {
		v := vec3{rng.NormFloat64(), rng.NormFloat64(), rng.NormFloat64()}.normalize()
		angle := 0.04 + 0.12*math.Pow(rng.Float64(), 2)
		cs[i] = crater{v, 1 - math.Cos(angle)}
	}
	return cs
}

// cratered darkens crater floors and brightens their rims.
func cratered(c rgb, p vec3, cs []crater) rgb {
	for _, cr := range cs {
		d := (1 - p.dot(cr.center)) / cr.size // 0 at the centre, 1 at the rim
		switch {
		case d < 0.8:
			c = c.scale(0.75)
		case d < 1.15:
			c = c.scale(1.2)
		}
	}
	return c
}

var (
	sunTexture = opaque(func(p vec3, lat, lon float64) rgb {
		n := fbm(p, 6, 4, 1)
		return mix(hex(0xff7a00), hex(0xffd23a), n*1.6-0.3)
	})

	mercuryCraters = craters(60, 2)
	mercuryTexture = opaque(func(p vec3, lat, lon float64) rgb {
		n := fbm(p, 3, 5, 2)
		return cratered(mix(hex(0x6e6a66), hex(0xb0aaa2), n*1.8-0.4), p, mercuryCraters)
	})

	venusTexture = opaque(func(p vec3, lat, lon float64) rgb {
		// Swirling cloud tops streaked along latitude.
		n := fbm(vec3{p.x * 0.6, p.y * 3, p.z * 0.6}, 2.5, 5, 3)
		return mix(hex(0xc8963c), hex(0xf2dc9a), n*1.8-0.4)
	})

	earthTexture = opaque(func(p vec3, lat, lon float64) rgb {
		h := fbm(p, 1.6, 6, 4)
		absLat := math.Abs(lat) * 180 / math.Pi
		ice := absLat + 8*fbm(p, 4, 3, 5)
		switch {
		case ice > 76:
			return hex(0xf4f8ff)
		case h < 0.53:
			return mix(hex(0x06205e), hex(0x1048b0), (h-0.35)/0.18) // deeper ocean is darker
		}
		// Deserts around 25° and in continental interiors, green elsewhere.
		dry := clamp01(1-math.Abs(absLat-24)/14) * fbm(p, 3, 4, 6) * 1.6
		land := mix(hex(0x2f8a2a), hex(0xc8a456), dry)
		if h > 0.66 {
			land = mix(land, hex(0x7a6a50), (h-0.66)*6) // mountains
		}
		return land
	})
	earthClouds = lazy(func(p vec3, lat, lon float64) (rgb, float64) {
		n := fbm(vec3{p.x, p.y * 1.8, p.z}, 2.5, 5, 7)
		return hex(0xffffff), clamp01((n - 0.55) * 4)
	})

	moonCraters = craters(70, 8)
	moonTexture = opaque(func(p vec3, lat, lon float64) rgb {
		maria := fbm(p, 1.5, 4, 8)
		c := mix(hex(0xb4b2ac), hex(0x5c5a58), (maria-0.5)*4) // dark "seas"
		return cratered(c, p, moonCraters)
	})

	marsTexture = opaque(func(p vec3, lat, lon float64) rgb {
		n := fbm(p, 2, 6, 9)
		c := mix(hex(0xd2693a), hex(0x6e2c18), (n-0.45)*4)
		if math.Abs(lat)*180/math.Pi+6*fbm(p, 5, 2, 10) > 80 {
			c = hex(0xf2eee8) // polar caps
		}
		return c
	})

	jupiterTexture = opaque(func(p vec3, lat, lon float64) rgb {
		l := lat + 0.07*(fbm(p, 4, 4, 11)-0.5) // turbulent band edges
		bands := []rgb{hex(0xe8dcc0), hex(0xb07848), hex(0xf2e6cc), hex(0xc89060), hex(0xe0c8a0), hex(0x9a6440)}
		i := int(math.Floor((l + math.Pi/2) / (math.Pi / 14)))
		c := bands[((i%len(bands))+len(bands))%len(bands)]
		if s := spot(lat, lon, -22*math.Pi/180, 0.6, 0.12, 0.3); s > 0 {
			c = mix(c, hex(0xc8502c), s*2.5) // Great Red Spot
		}
		return c
	})

	saturnTexture = opaque(func(p vec3, lat, lon float64) rgb {
		l := lat + 0.03*(fbm(p, 4, 3, 12)-0.5)
		t := 0.5 + 0.5*math.Sin(l*16)
		return mix(hex(0xc8a868), hex(0xf0dca8), t)
	})

	uranusTexture = opaque(func(p vec3, lat, lon float64) rgb {
		return mix(hex(0x8ccfd8), hex(0xb8eef0), 0.5+0.5*math.Sin(lat*6))
	})

	neptuneTexture = opaque(func(p vec3, lat, lon float64) rgb {
		l := lat + 0.05*(fbm(p, 3, 3, 13)-0.5)
		c := mix(hex(0x203c9c), hex(0x3c6ee0), 0.5+0.5*math.Sin(l*9))
		if s := spot(lat, lon, -20*math.Pi/180, 1.5, 0.12, 0.25); s > 0 {
			c = mix(c, hex(0x101e58), s*3) // Great Dark Spot
		}
		if s := spot(lat, lon, -12*math.Pi/180, 1.9, 0.03, 0.3); s > 0 {
			c = mix(c, hex(0xffffff), s*3) // bright companion cloud
		}
		return c
	})

	plutoTexture = opaque(func(p vec3, lat, lon float64) rgb {
		n := fbm(p, 2.5, 5, 14)
		c := mix(hex(0x6a4630), hex(0xc8a080), n*2-0.5)
		// The heart (Tombaugh Regio): (x²+y²-1)³ - x²y³ < 0.
		x := math.Remainder(lon-0.8, 2*math.Pi) / 0.45
		y := (lat - 0.05) / 0.45
		if q := x*x + y*y - 1; q*q*q-x*x*y*y*y < 0 {
			c = mix(c, hex(0xf4e8dc), 0.9)
		}
		return c
	})
)

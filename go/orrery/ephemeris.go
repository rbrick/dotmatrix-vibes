package orrery

import (
	"math"
	"time"

	"dotmatrix/prayer"
)

// elements are a planet's Keplerian orbital elements at J2000 and their
// rates of change per Julian century, from JPL's "Keplerian Elements for
// Approximate Positions of the Major Planets" (E. M. Standish), valid
// 1800-2050 to well under a degree.
type elements struct {
	a, e, i, L, peri, node                   float64 // AU, -, degrees...
	aDot, eDot, iDot, LDot, periDot, nodeDot float64 // ...per century
}

type planet struct {
	name string
	el   elements
}

var planets = []planet{
	{"Mercury", elements{0.38709927, 0.20563593, 7.00497902, 252.25032350, 77.45779628, 48.33076593,
		0.00000037, 0.00001906, -0.00594749, 149472.67411175, 0.16047689, -0.12534081}},
	{"Venus", elements{0.72333566, 0.00677672, 3.39467605, 181.97909950, 131.60246718, 76.67984255,
		0.00000390, -0.00004107, -0.00078890, 58517.81538729, 0.00268329, -0.27769418}},
	{"Earth", elements{1.00000261, 0.01671123, -0.00001531, 100.46457166, 102.93768193, 0,
		0.00000562, -0.00004392, -0.01294668, 35999.37244981, 0.32327364, 0}},
	{"Mars", elements{1.52371034, 0.09339410, 1.84969142, -4.55343205, -23.94362959, 49.55953891,
		0.00001847, 0.00007882, -0.00813131, 19140.30268499, 0.44441088, -0.29257343}},
	{"Jupiter", elements{5.20288700, 0.04838624, 1.30439695, 34.39644051, 14.72847983, 100.47390909,
		-0.00011607, -0.00013253, -0.00183714, 3034.74612775, 0.21252668, 0.20469106}},
	{"Saturn", elements{9.53667594, 0.05386179, 2.48599187, 49.95424423, 92.59887831, 113.66242448,
		-0.00125060, -0.00050991, 0.00193609, 1222.49362201, -0.41897216, -0.28867794}},
	{"Uranus", elements{19.18916464, 0.04725744, 0.77263783, 313.23810451, 170.95427630, 74.01692503,
		-0.00196176, -0.00004397, -0.00242939, 428.48202785, 0.40805281, 0.04240589}},
	{"Neptune", elements{30.06992276, 0.00859048, 1.77004347, -55.12002969, 44.96476227, 131.78422574,
		0.00026291, 0.00005105, 0.00035372, 218.45945325, -0.32241464, -0.01262724}},
	{"Pluto", elements{39.48211675, 0.24882730, 17.14001206, 238.92903833, 224.06891629, 110.30393684,
		-0.00031596, 0.00005170, 0.00004818, 145.20780515, -0.04062942, -0.01183482}},
}

// vec is a heliocentric position in AU in ecliptic coordinates: x towards
// the vernal equinox, z towards the north ecliptic pole.
type vec struct{ x, y, z float64 }

func (v vec) length() float64 { return math.Sqrt(v.x*v.x + v.y*v.y + v.z*v.z) }

// longitude is the heliocentric ecliptic longitude in degrees [0, 360).
func (v vec) longitude() float64 {
	return math.Mod(math.Atan2(v.y, v.x)*180/math.Pi+360, 360)
}

// orbit holds a planet's elements evaluated at one time.
type orbit struct {
	a, e                          float64
	i, node, argPeri, meanAnomaly float64 // radians
}

func (p planet) orbitAt(t time.Time) orbit {
	T := prayer.DaysSinceJ2000(t) / 36525 // Julian centuries since J2000
	el := p.el
	rad := math.Pi / 180
	peri := el.peri + el.periDot*T
	node := el.node + el.nodeDot*T
	return orbit{
		a:           el.a + el.aDot*T,
		e:           el.e + el.eDot*T,
		i:           (el.i + el.iDot*T) * rad,
		node:        node * rad,
		argPeri:     (peri - node) * rad,
		meanAnomaly: math.Remainder((el.L+el.LDot*T-peri)*rad, 2*math.Pi),
	}
}

// at returns the position on the orbit at eccentric anomaly E.
func (o orbit) at(E float64) vec {
	// In the orbital plane, x towards perihelion.
	xp := o.a * (math.Cos(E) - o.e)
	yp := o.a * math.Sqrt(1-o.e*o.e) * math.Sin(E)
	cw, sw := math.Cos(o.argPeri), math.Sin(o.argPeri)
	cn, sn := math.Cos(o.node), math.Sin(o.node)
	ci, si := math.Cos(o.i), math.Sin(o.i)
	return vec{
		(cw*cn-sw*sn*ci)*xp + (-sw*cn-cw*sn*ci)*yp,
		(cw*sn+sw*cn*ci)*xp + (-sw*sn+cw*cn*ci)*yp,
		sw*si*xp + cw*si*yp,
	}
}

// position solves Kepler's equation for where the planet is.
func (o orbit) position() vec {
	M := o.meanAnomaly
	E := M + o.e*math.Sin(M)
	for i := 0; i < 10; i++ {
		d := (E - o.e*math.Sin(E) - M) / (1 - o.e*math.Cos(E))
		E -= d
		if math.Abs(d) < 1e-12 {
			break
		}
	}
	return o.at(E)
}

// position returns where planets[i] is at t: heliocentric ecliptic
// coordinates in AU.
func position(i int, t time.Time) vec { return planets[i].orbitAt(t).position() }

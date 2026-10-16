package moonphase

import (
	"math"
	"time"

	"dotmatrix/prayer"
)

// Phase describes the Moon as seen from Earth at one instant.
type Phase struct {
	// Elongation is how far the Moon is east of the Sun along the ecliptic,
	// in degrees [0, 360): 0 is new, 90 first quarter, 180 full and 270
	// last quarter.
	Elongation float64
	// Illumination is the lit fraction of the disc, 0-1.
	Illumination float64
}

// At calculates the phase at t.
func At(t time.Time) Phase {
	e := elongation(t)
	return Phase{Elongation: e, Illumination: (1 - math.Cos(e*math.Pi/180)) / 2}
}

// Waxing reports whether the lit part is growing (new moon to full moon).
func (p Phase) Waxing() bool { return p.Elongation < 180 }

// Name returns the conventional name of the phase, e.g. "Waxing Gibbous".
// The new, full and quarter moons are named within a day or so either side
// of the exact moment.
func (p Phase) Name() string {
	const near = 12 // degrees, about a day of the Moon's motion
	switch e := p.Elongation; {
	case e < near || e >= 360-near:
		return "New Moon"
	case e < 90-near:
		return "Waxing Crescent"
	case e < 90+near:
		return "First Quarter"
	case e < 180-near:
		return "Waxing Gibbous"
	case e < 180+near:
		return "Full Moon"
	case e < 270-near:
		return "Waning Gibbous"
	case e < 270+near:
		return "Last Quarter"
	default:
		return "Waning Crescent"
	}
}

// Elongations of the principal phases, for Next and Previous.
const (
	NewMoon      = 0.0
	FirstQuarter = 90.0
	FullMoon     = 180.0
	LastQuarter  = 270.0
)

// meanRate is the average daily increase of the elongation (360° per
// synodic month of 29.53 days).
const meanRate = 360 / 29.530589

// Next returns the first moment after t when the elongation reaches target
// (e.g. FullMoon), accurate to within a few minutes.
func Next(t time.Time, target float64) time.Time {
	ahead := normalize(target - elongation(t))
	if ahead == 0 {
		ahead = 360
	}
	return refine(t.Add(days(ahead/meanRate)), target)
}

// Previous returns the last moment at or before t when the elongation was
// target; Previous(t, NewMoon) is the start of the current lunation.
func Previous(t time.Time, target float64) time.Time {
	return refine(t.Add(-days(normalize(elongation(t)-target)/meanRate)), target)
}

// refine improves a guess for when the elongation equals target. The
// elongation always increases, by 10-14° a day, so this converges quickly.
func refine(t time.Time, target float64) time.Time {
	for i := 0; i < 6; i++ {
		diff := signed(target - elongation(t))
		if math.Abs(diff) < 1e-4 {
			break
		}
		t = t.Add(days(diff / meanRate))
	}
	return t
}

func days(d float64) time.Duration { return time.Duration(d * 24 * float64(time.Hour)) }

// elongation is the Moon's ecliptic longitude minus the Sun's, in degrees.
func elongation(t time.Time) float64 {
	return normalize(moonLongitude(prayer.DaysSinceJ2000(t)) - prayer.SunEclipticLongitude(t))
}

// moonLongitude returns the Moon's geocentric ecliptic longitude in degrees
// for d days after J2000, using the largest terms of the series in Meeus,
// Astronomical Algorithms, chapter 47 (good to about 0.3°, or half an hour
// in the time of a phase).
func moonLongitude(d float64) float64 {
	rad := math.Pi / 180
	L := 218.3164 + 13.17639648*d // mean longitude
	D := (297.8502 + 12.19074912*d) * rad
	M := (357.5291 + 0.98560028*d) * rad // Sun's mean anomaly
	Mp := (134.9634 + 13.06499295*d) * rad
	F := (93.2721 + 13.22935024*d) * rad
	return normalize(L +
		6.288774*math.Sin(Mp) +
		1.274027*math.Sin(2*D-Mp) +
		0.658314*math.Sin(2*D) +
		0.213618*math.Sin(2*Mp) -
		0.185116*math.Sin(M) -
		0.114332*math.Sin(2*F) +
		0.058793*math.Sin(2*D-2*Mp) +
		0.057066*math.Sin(2*D-M-Mp) +
		0.053322*math.Sin(2*D+Mp) +
		0.045758*math.Sin(2*D-M) -
		0.040923*math.Sin(M-Mp) -
		0.034720*math.Sin(D) -
		0.030383*math.Sin(M+Mp))
}

// normalize maps degrees to [0, 360).
func normalize(d float64) float64 {
	d = math.Mod(d, 360)
	if d < 0 {
		d += 360
	}
	return d
}

// signed maps degrees to [-180, 180).
func signed(d float64) float64 { return normalize(d+180) - 180 }

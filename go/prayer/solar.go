package prayer

import (
	"math"
	"time"
)

const (
	// See: https://en.wikipedia.org/wiki/Julian_day
	UnixStart  = 2440587.5 // Julian date for January 1, 1970 at 00:00:00 UTC
	J2000Start = 2451545.0 // Julian date for January 1, 2000 at 12:00:00 UTC
)

func normalizeDegrees(d float64) float64 {
	d = math.Mod(d, 360.0)
	if d < 0 {
		d += 360.0
	}
	return d
}

func normalizeSignedDegrees(d float64) float64 {
	d = math.Mod(d, 360.0)
	if d > 180 {
		d -= 360
	}
	if d < -180 {
		d += 360
	}
	return d
}

func unixToJulianDate(t time.Time) float64 {
	return (float64(t.UnixNano())/86400.0)/1e9 + UnixStart
}

func solarMean(d float64) (anomaly, longitude float64) {
	// mean anomaly of the sun
	// meanAnomaly := normalizeDegrees(357.529 + 0.98560028*d)
	meanAnomaly := normalizeDegrees(357.528 + 0.9856003*d)

	// mean longitude of the sun
	// meanLongitude := normalizeDegrees(280.459 + 0.98564736*d)
	meanLongitude := normalizeDegrees(280.460 + 0.9856474*d)
	return meanAnomaly, meanLongitude
}

func eclipticLongitude(meanAnomaly, meanLongitude float64) float64 {
	// ecliptic longitude of the sun
	return normalizeDegrees(meanLongitude + 1.915*math.Sin(degToRad(meanAnomaly)) + 0.020*math.Sin(2*degToRad(meanAnomaly)))
}

// R = 1.00014 – 0.01671 cos g – 0.00014 cos 2g
func approxDistFromEarth(meanAnomaly float64) float64 {
	// approximate distance from the Earth to the sun in astronomical units
	return 1.00014 - 0.01671*math.Cos(degToRad(meanAnomaly)) - 0.00014*math.Cos(2*degToRad(meanAnomaly))
}

func eclipticObliquity(d float64) float64 {
	// obliquity of the ecliptic
	return 23.439 - 0.00000036*d
}

func radToDeg(r float64) float64 {
	return r * 180 / math.Pi
}

func degToRad(d float64) float64 {
	return d * math.Pi / 180
}

type SolarAngles struct {
	// right ascension of the sun
	RightAscension float64
	// declination of the sun
	Declination float64
	// equation of time
	EquationOfTime float64

	TFunc func(t float64, lat float64, dhuhr time.Time) float64
	AFunc func(angle float64, lat float64, dhuhr time.Time) float64
}

func arccot(x float64) float64 {
	return math.Atan2(1, x)
}

func GetSolarAnglesFromTime(t time.Time) *SolarAngles {
	jd := unixToJulianDate(t)
	daysSinceJ2000 := jd - J2000Start

	// See: https://aa.usno.navy.mil/faq/sun_approx
	// mean anomaly of the sun
	meanAnomaly, meanLongitude := solarMean(daysSinceJ2000)

	// ecliptic longitude of the sun
	eclipticLongitude := eclipticLongitude(meanAnomaly, meanLongitude) // L
	// obliquity of the ecliptic
	obliquity := eclipticObliquity(daysSinceJ2000)

	// right ascension of the sun
	rightAscension := normalizeDegrees(radToDeg(math.Atan2(math.Cos(degToRad(obliquity))*math.Sin(degToRad(eclipticLongitude)), math.Cos(degToRad(eclipticLongitude)))))

	// declination of the sun
	declination := radToDeg(math.Asin(
		math.Sin(degToRad(obliquity)) *
			math.Sin(degToRad(eclipticLongitude)),
	))

	delta := normalizeSignedDegrees(meanLongitude - rightAscension)
	eqT := 4.0 * delta // minutes

	return &SolarAngles{
		RightAscension: rightAscension,
		Declination:    declination,
		EquationOfTime: eqT,
		AFunc: func(t, lat float64, dhuhr time.Time) float64 {

			// Asr Formula - returns the time in hours after dhuhr for asr

			L := degToRad(lat)
			D := degToRad(declination)

			x := math.Sin(arccot(t + math.Tan(math.Abs(L-D))))
			y := math.Sin(L) * math.Sin(D)
			numerator := x - y
			denominator := math.Cos(L) * math.Cos(D)

			return radToDeg(math.Acos(numerator/denominator)) / 15.0 // convert to hours
		},
		TFunc: func(angle float64, lat float64, dhuhr time.Time) float64 {
			// L := degToRad(eclipticLongitude)
			// L := degToRad(lat)
			// D := degToRad(declination)

			// H := radToDeg(math.Acos(

			// 	-math.Sin(degToRad(angle))-math.Sin(L)*math.Sin(D)) / (math.Cos(L) * math.Cos(D)))

			latRad := degToRad(lat)
			decRad := degToRad(declination)
			altRad := degToRad(-math.Abs(angle)) // negative

			cosH := (math.Sin(altRad) - math.Sin(latRad)*math.Sin(decRad)) /
				(math.Cos(latRad) * math.Cos(decRad))

			H := radToDeg(math.Acos(cosH))

			return H / 15.0
		},
	}
}

// DaysSinceJ2000 returns the (fractional) number of days between t and the
// J2000.0 epoch, the time variable the solar and lunar series use.
func DaysSinceJ2000(t time.Time) float64 {
	return unixToJulianDate(t) - J2000Start
}

// SunEclipticLongitude returns the Sun's ecliptic longitude at t in degrees
// [0, 360), accurate to about 0.01°.
func SunEclipticLongitude(t time.Time) float64 {
	return eclipticLongitude(solarMean(DaysSinceJ2000(t)))
}

package weather

import (
	"math"
	"time"

	"dotmatrix/prayer"
)

// solarDay uses the same solar angles and -0.833° horizon as the prayer clock.
// The day is centred on local solar noon, not UTC midnight (sunset may be on
// the following UTC date west of Greenwich).
func solarDay(now time.Time, lat, long float64) (rise, set time.Time, hours float64) {
	utc := now.UTC()
	ref := time.Date(utc.Year(), utc.Month(), utc.Day(), 12, 0, 0, 0, time.UTC)
	compute := func() (time.Time, float64) {
		angles := prayer.GetSolarAnglesFromTime(ref)
		noon := ref.Add(-time.Duration((long*4 + angles.EquationOfTime) * float64(time.Minute)))
		return noon, angles.TFunc(0.833, lat, noon)
	}
	noon, hours := compute()
	if now.Before(noon.Add(-12 * time.Hour)) {
		ref = ref.AddDate(0, 0, -1)
		noon, hours = compute()
	} else if now.After(noon.Add(12 * time.Hour)) {
		ref = ref.AddDate(0, 0, 1)
		noon, hours = compute()
	}
	if math.IsNaN(hours) || hours < 0 || hours > 12 {
		// shortcut: no sunrise or sunset on polar days; use solar altitude alone.
		return time.Time{}, time.Time{}, 0
	}
	d := time.Duration(hours * float64(time.Hour))
	return noon.Add(-d), noon.Add(d), hours
}

// sunAngles returns the Sun's real elevation and hour angle (west of noon).
func sunAngles(now time.Time, lat, long float64) (altitude, hourAngle float64) {
	a := prayer.GetSolarAnglesFromTime(now)
	utc := now.UTC()
	minutes := float64(utc.Hour()*60+utc.Minute()) + float64(utc.Second())/60 + float64(utc.Nanosecond())/6e10
	hourAngle = math.Mod((minutes+a.EquationOfTime+long*4-720)/4+540, 360) - 180
	l, d, h := lat*math.Pi/180, a.Declination*math.Pi/180, hourAngle*math.Pi/180
	altitude = math.Asin(math.Sin(l)*math.Sin(d)+math.Cos(l)*math.Cos(d)*math.Cos(h)) * 180 / math.Pi
	return
}

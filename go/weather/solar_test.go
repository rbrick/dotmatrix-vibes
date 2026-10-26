package weather

import (
	"math"
	"testing"
	"time"

	"dotmatrix/prayer"
)

func TestSolarTimesMatchPrayerClock(t *testing.T) {
	at := time.Date(2025, 3, 20, 18, 0, 0, 0, time.UTC)
	const lat, long = 41.8781, -87.6298
	rise, set, _ := solarDay(at, lat, long)
	p := prayer.CalculatePrayerTimesFor(at, lat, long, prayer.Methods["ISNA"])
	noon := time.UnixMilli(int64(p.Dhuhr.Time))
	wantSet := time.UnixMilli(int64(p.Maghrib.Time))
	wantRise := noon.Add(-wantSet.Sub(noon))
	if math.Abs(set.Sub(wantSet).Seconds()) > 1 || math.Abs(rise.Sub(wantRise).Seconds()) > 1 {
		t.Errorf("solar times %v, %v differ from prayer clock %v, %v", rise, set, wantRise, wantSet)
	}
	// In Chicago sunset is after midnight UTC; it must not reset at UTC midnight.
	before, after := time.Date(2025, 3, 20, 23, 59, 0, 0, time.UTC), time.Date(2025, 3, 21, 0, 1, 0, 0, time.UTC)
	_, first, _ := solarDay(before, lat, long)
	_, second, _ := solarDay(after, lat, long)
	if !first.Equal(second) {
		t.Errorf("sunset jumped at UTC midnight: %v -> %v", first, second)
	}
}

func TestSunAltitudeFollowsDay(t *testing.T) {
	noon := time.Date(2025, 3, 20, 12, 0, 0, 0, time.UTC)
	day, _ := sunAngles(noon, 0, 0)
	night, _ := sunAngles(noon.Add(12*time.Hour), 0, 0)
	if day < 85 || night > -85 {
		t.Errorf("equatorial equinox sun: noon %.1f°, midnight %.1f°", day, night)
	}
}

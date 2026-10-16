package prayer

import (
	"testing"
	"time"
)

func chicago(t *testing.T) (*PrayerTimes, func(day, h, m int) time.Time) {
	loc, err := time.LoadLocation("America/Chicago")
	if err != nil {
		t.Fatal(err)
	}
	at := func(day, h, m int) time.Time { return time.Date(2026, 10, day, h, m, 0, 0, loc) }
	return CalculatePrayerTimesFor(at(13, 12, 0), 41.8781, -87.6298, Methods["ISNA"]), at
}

func TestCurrentAndNextPrayer(t *testing.T) {
	p, at := chicago(t)
	for _, tc := range []struct {
		now           time.Time
		current, next string
		nextDay       int
	}{
		{at(13, 3, 0), "Isha", "Fajr", 13}, // before Fajr: last night's Isha is current
		{at(13, 9, 0), "Fajr", "Dhuhr", 13},
		{at(13, 14, 0), "Dhuhr", "Asr", 13},
		{at(13, 17, 0), "Asr", "Maghrib", 13},
		{at(13, 19, 0), "Maghrib", "Isha", 13},
		{at(13, 22, 0), "Isha", "Fajr", 14}, // after Isha: tomorrow's Fajr
		{at(20, 22, 0), "Isha", "Fajr", 21}, // a week after p was calculated
	} {
		cur, next := p.CurrentPrayerAt(tc.now), p.NextPrayerAt(tc.now)
		if cur.Name != tc.current {
			t.Errorf("%v: current = %s, want %s", tc.now, cur.Name, tc.current)
		}
		if next.Name != tc.next || next.at().In(tc.now.Location()).Day() != tc.nextDay {
			t.Errorf("%v: next = %s on day %d, want %s on day %d", tc.now, next.Name, next.at().Day(), tc.next, tc.nextDay)
		}
		if !next.at().After(tc.now) {
			t.Errorf("%v: next prayer %v is not in the future", tc.now, next.at())
		}
	}
	if before := p.CurrentPrayerAt(at(13, 3, 0)); before.at().Day() != 12 {
		t.Errorf("Isha before Fajr should be from the 12th, got %v", before.at())
	}
	if p.Fajr.Time != CalculatePrayerTimesFor(at(13, 12, 0), p.Lat, p.Long, p.Method).Fajr.Time {
		t.Error("NextPrayerAt must not modify p")
	}
}

func TestHighLatitudeSummer(t *testing.T) {
	// Oslo at midsummer: the sun never gets 18° below the horizon.
	loc, _ := time.LoadLocation("Europe/Oslo")
	p := CalculatePrayerTimesFor(time.Date(2026, 6, 21, 12, 0, 0, 0, loc), 59.91, 10.75, Methods["MWL"])
	fajr, maghrib, isha := p.Fajr.at(), p.Maghrib.at(), p.Isha.at()
	if fajr.Year() != 2026 || isha.Year() != 2026 {
		t.Fatalf("times not finite: fajr %v isha %v", fajr, isha)
	}
	if !(fajr.Before(p.Dhuhr.at()) && maghrib.Before(isha) && isha.Sub(maghrib) < 3*time.Hour) {
		t.Errorf("implausible times: fajr %v maghrib %v isha %v", fajr, maghrib, isha)
	}
}

func TestModerateLatitudeUnchangedByHighLatitudeRule(t *testing.T) {
	p, at := chicago(t)
	// Published ISNA times for Chicago, 13 Oct 2026, are about 5:45 and 7:30.
	for _, tc := range []struct {
		pt   PrayerTime
		want time.Time
	}{{p.Fajr, at(13, 5, 45)}, {p.Isha, at(13, 19, 30)}} {
		if d := tc.pt.at().Sub(tc.want); d < -5*time.Minute || d > 5*time.Minute {
			t.Errorf("%s = %v, want about %v", tc.pt.Name, tc.pt.at(), tc.want)
		}
	}
}

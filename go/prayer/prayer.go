package prayer

import (
	"math"
	"time"
)

type PrayerTime struct {
	Name string
	Time uint64
}

type PrayerTimes struct {
	Fajr    PrayerTime
	Dhuhr   PrayerTime
	Asr     PrayerTime
	Maghrib PrayerTime
	Isha    PrayerTime

	Lat, Long float64
	Method    Method
}

// Refresh recalculates the prayer times for today.
func (p *PrayerTimes) Refresh() *PrayerTimes {
	*p = *CalculatePrayerTimes(p.Lat, p.Long, p.Method)
	return p
}

func (pt PrayerTime) at() time.Time { return time.UnixMilli(int64(pt.Time)) }

func (p *PrayerTimes) forDay(t time.Time) *PrayerTimes {
	return CalculatePrayerTimesFor(t, p.Lat, p.Long, p.Method)
}

// CurrentPrayer returns the prayer whose time is in effect now.
func (p *PrayerTimes) CurrentPrayer() PrayerTime { return p.CurrentPrayerAt(time.Now()) }

// CurrentPrayerAt returns the prayer whose time is in effect at now. Times are
// calculated for now's day, so this stays correct however long ago p was
// calculated. Before Fajr, the current prayer is the previous night's Isha.
func (p *PrayerTimes) CurrentPrayerAt(now time.Time) PrayerTime {
	today := p.forDay(now)
	switch {
	case now.Before(today.Fajr.at()):
		return p.forDay(now.AddDate(0, 0, -1)).Isha
	case now.Before(today.Dhuhr.at()):
		return today.Fajr
	case now.Before(today.Asr.at()):
		return today.Dhuhr
	case now.Before(today.Maghrib.at()):
		return today.Asr
	case now.Before(today.Isha.at()):
		return today.Maghrib
	default:
		return today.Isha
	}
}

// NextPrayer returns the next prayer after now.
func (p *PrayerTimes) NextPrayer() PrayerTime { return p.NextPrayerAt(time.Now()) }

// NextPrayerAt returns the next prayer after now; after Isha, that is
// tomorrow's Fajr. Like CurrentPrayerAt it does not depend on when p was
// calculated, and it does not modify p.
func (p *PrayerTimes) NextPrayerAt(now time.Time) PrayerTime {
	today := p.forDay(now)
	for _, pt := range []PrayerTime{today.Fajr, today.Dhuhr, today.Asr, today.Maghrib, today.Isha} {
		if now.Before(pt.at()) {
			return pt
		}
	}
	return p.forDay(now.AddDate(0, 0, 1)).Fajr
}

func CalculatePrayerTimes(lat, long float64, method Method) *PrayerTimes {
	return CalculatePrayerTimesFor(time.Now(), lat, long, method)
}

// CalculatePrayerTimesFor calculates the prayer times for the calendar day
// of now, in now's time zone.
func CalculatePrayerTimesFor(now time.Time, lat, long float64, method Method) *PrayerTimes {
	noon := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, time.UTC)
	solarAngles := GetSolarAnglesFromTime(noon)

	longCorrectionMinutes := long * 4.0
	// Solar noon in UTC
	dhuhr := noon.
		Add(-time.Duration(longCorrectionMinutes * float64(time.Minute))).
		Add(-time.Duration(solarAngles.EquationOfTime * float64(time.Minute)))

	dhuhr = dhuhr.In(now.Location())

	hours := func(h float64) time.Duration { return time.Duration(h * float64(time.Hour)) }

	sunset := solarAngles.TFunc(0.833, lat, dhuhr) // hours after dhuhr
	night := 24 - 2*sunset                         // hours from sunset to sunrise
	// twilight returns the hours between dhuhr and the sun reaching angle
	// below the horizon. Far from the equator in summer the sun may never get
	// that low (TFunc returns NaN) or only does so very late; then the
	// angle-based rule applies: twilight lasts at most angle/60 of the night.
	twilight := func(angle float64) float64 {
		h := solarAngles.TFunc(angle, lat, dhuhr) - sunset
		if limit := angle / 60 * night; math.IsNaN(h) || h > limit {
			h = limit
		}
		return sunset + h
	}

	fajr := dhuhr.Add(-hours(twilight(method.FajrDegrees)))
	isha := dhuhr.Add(hours(twilight(method.IshaDegrees)))
	maghrib := dhuhr.Add(hours(sunset))
	asr := dhuhr.Add(time.Duration(solarAngles.AFunc(1, lat, dhuhr) * 60.0 * 60.0 * float64(time.Second)))

	return &PrayerTimes{
		Fajr:    PrayerTime{Name: "Fajr", Time: uint64(fajr.UnixMilli())},
		Dhuhr:   PrayerTime{Name: "Dhuhr", Time: uint64(dhuhr.UnixMilli())},
		Asr:     PrayerTime{Name: "Asr", Time: uint64(asr.UnixMilli())},
		Maghrib: PrayerTime{Name: "Maghrib", Time: uint64(maghrib.UnixMilli())},
		Isha:    PrayerTime{Name: "Isha", Time: uint64(isha.UnixMilli())},

		// Store the location and method for refreshing
		Lat:    lat,
		Long:   long,
		Method: method,
	}
}

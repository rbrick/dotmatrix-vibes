// Package prayerclock is a matrix.Scene showing the time of day and the next
// prayer:
//
//	🌙 9:41:07PM   time-of-day emoji and the current time
//	    FAJR       next prayer
//	   5:51AM      its time
//	 in 8:09:53    countdown
package prayerclock

import (
	"fmt"
	"image/color"
	"sort"
	"strings"
	"time"

	"dotmatrix/matrix"
	"dotmatrix/prayer"
)

// Clock calculates prayer times for a location with the code in package
// prayer and renders them. It implements matrix.Scene.
type Clock struct {
	Lat, Long  float64
	MethodName string // key of prayer.Methods, e.g. "ISNA"
	Clock24    bool   // 24-hour times instead of 12-hour
	Location   *time.Location

	now func() time.Time // overridable for tests
}

var (
	clockColor     = color.RGBA{255, 255, 255, 255}
	nameColor      = color.RGBA{255, 170, 0, 255}
	countdownColor = color.RGBA{110, 110, 110, 255}
)

// New validates the settings and returns a Clock in the local time zone.
func New(lat, long float64, method string, clock24 bool) (*Clock, error) {
	if lat < -90 || lat > 90 || long < -180 || long > 180 {
		return nil, fmt.Errorf("prayerclock: invalid coordinates %v, %v", lat, long)
	}
	if _, ok := prayer.Methods[method]; !ok {
		return nil, fmt.Errorf("prayerclock: unknown method %q (have %s)", method, strings.Join(MethodNames(), ", "))
	}
	return &Clock{Lat: lat, Long: long, MethodName: method, Clock24: clock24, Location: time.Local}, nil
}

// MethodNames lists the calculation methods New accepts.
func MethodNames() []string {
	var names []string
	for k := range prayer.Methods {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// event is a prayer (or sunrise) rounded to the minute, as timetables show it.
type event struct {
	name string
	at   time.Time
}

func (c *Clock) day(t time.Time) []event {
	p := prayer.CalculatePrayerTimesFor(t, c.Lat, c.Long, prayer.Methods[c.MethodName])
	at := func(pt prayer.PrayerTime) time.Time {
		return time.UnixMilli(int64(pt.Time)).In(c.Location).Round(time.Minute)
	}
	dhuhr, maghrib := at(p.Dhuhr), at(p.Maghrib)
	// Sunrise mirrors sunset (Maghrib, at -0.833°) around solar noon.
	sunrise := dhuhr.Add(-maghrib.Sub(dhuhr))
	return []event{
		{"Fajr", at(p.Fajr)}, {"Sunrise", sunrise}, {"Dhuhr", dhuhr},
		{"Asr", at(p.Asr)}, {"Maghrib", maghrib}, {"Isha", at(p.Isha)},
	}
}

// timeOfDay picks the emoji for the period that contains now.
func timeOfDay(today []event, now time.Time) string {
	switch {
	case now.Before(today[0].at): // before Fajr
		return ":moon:"
	case now.Before(today[1].at): // Fajr to sunrise
		return ":sunrise:"
	case now.Before(today[3].at): // sunrise to Asr
		return ":sun:"
	case now.Before(today[4].at): // Asr to Maghrib
		return ":afternoon:"
	case now.Before(today[5].at): // Maghrib to Isha
		return ":sunset:"
	default:
		return ":moon:"
	}
}

// NextPrayer returns the next prayer after now (sunrise is not a prayer).
func (c *Clock) NextPrayer(now time.Time) (name string, at time.Time) {
	now = now.In(c.Location)
	for _, e := range c.day(now) {
		if e.name != "Sunrise" && e.at.After(now) {
			return e.name, e.at
		}
	}
	fajr := c.day(now.AddDate(0, 0, 1))[0]
	return fajr.name, fajr.at
}

func (c *Clock) formatTime(t time.Time) string {
	if c.Clock24 {
		return t.Format("15:04")
	}
	return t.Format("3:04PM")
}

func (c *Clock) formatClock(t time.Time) string {
	if c.Clock24 {
		return t.Format("15:04:05")
	}
	return t.Format("3:04:05PM")
}

func formatCountdown(d time.Duration) string {
	s := int(d / time.Second)
	if s < 3600 {
		return fmt.Sprintf("%dm%02ds", s/60, s%60)
	}
	return fmt.Sprintf("%dh%02dm%02ds", s/3600, s/60%60, s%60)
}

// emojiGap is the space between the time-of-day emoji and the clock; a full
// space would make "12:59:59PM" too wide for the panel.
const emojiGap = 3

// Render implements matrix.Scene. It redraws every second.
func (c *Clock) Render(cv *matrix.Canvas, _ time.Duration) time.Duration {
	now := time.Now()
	if c.now != nil {
		now = c.now()
	}
	now = now.In(c.Location)
	name, at := c.NextPrayer(now)

	cv.Clear()
	emoji, clock := timeOfDay(c.day(now), now), c.formatClock(now)
	x := (matrix.Width - (matrix.TextWidth(emoji) + emojiGap + matrix.TextWidth(clock))) / 2
	cv.DrawText(x, 0, emoji, clockColor)
	cv.DrawText(x+matrix.TextWidth(emoji)+emojiGap, 0, clock, clockColor)

	lines := []struct {
		text string
		col  color.Color
	}{
		{strings.ToUpper(name), nameColor},
		{c.formatTime(at), clockColor},
		{formatCountdown(at.Sub(now.Truncate(time.Second))), countdownColor},
	}
	for i, l := range lines {
		cv.DrawText((matrix.Width-matrix.TextWidth(l.text))/2, (i+1)*matrix.LineHeight, l.text, l.col)
	}
	return now.Truncate(time.Second).Add(time.Second).Sub(now)
}
